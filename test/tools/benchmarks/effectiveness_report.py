#!/usr/bin/env python3
import argparse
import csv
import fnmatch
import json
import os
import statistics
import sys
from datetime import datetime, timezone
from pathlib import Path

try:
    import yaml
except ImportError:
    sys.exit("need pyyaml: pip install pyyaml")


def load_json(path):
    p = Path(path)
    if not p.exists() or p.stat().st_size == 0:
        return {}
    try:
        return json.loads(p.read_text(errors="replace"))
    except json.JSONDecodeError:
        return {}


def load_yaml(path):
    p = Path(path)
    if not p.exists() or p.stat().st_size == 0:
        return {}
    return yaml.safe_load(p.read_text()) or {}


def load_ndjson(path):
    p = Path(path)
    if not p.exists():
        return []
    rows = []
    for line in p.read_text(errors="replace").splitlines():
        if not line.strip():
            continue
        try:
            rows.append(json.loads(line))
        except json.JSONDecodeError:
            rows.append({"_raw": line})
    return rows


def number(value, default=0.0):
    try:
        return float(value)
    except (TypeError, ValueError):
        return default


def present(value):
    return value not in ("", None)


def parse_time(value):
    if not value:
        return None
    try:
        return datetime.fromisoformat(str(value).replace("Z", "+00:00")).astimezone(timezone.utc)
    except ValueError:
        return None


def frame_time(row):
    if not isinstance(row, dict):
        return None
    ts = parse_time(row.get("observedAt") or row.get("observed_at"))
    if ts:
        return ts
    body = row.get("event") or row.get("signal") or {}
    if isinstance(body, dict):
        return parse_time(body.get("observedAt") or body.get("observed_at"))
    return None


def marker_time(markers, phase):
    for marker in markers or []:
        if marker.get("phase") == phase:
            ts = parse_time(marker.get("ts"))
            if ts:
                return ts
    return None


def effectiveness_window(bench_summary):
    markers = (bench_summary or {}).get("markers") or []
    start = marker_time(markers, "workload_start")
    end = marker_time(markers, "workload_done") or marker_time(markers, "recorder_stop")
    if start and end and end > start:
        return start, end, "workload"
    return None, None, "all"


def filter_frames_by_window(frames, start, end):
    if not start or not end:
        return frames
    out = []
    for frame in frames:
        ts = frame_time(frame)
        if ts and start <= ts < end:
            out.append(frame)
    return out


def unwrap_event(row):
    if not isinstance(row, dict):
        return {}
    body = row.get("event")
    return body if isinstance(body, dict) else row


def unwrap_signal(row):
    if not isinstance(row, dict):
        return {}
    body = row.get("signal")
    return body if isinstance(body, dict) else row


def basename(value):
    value = str(value or "")
    return os.path.basename(value.rstrip("/")) if value else ""


def lower(value):
    return str(value or "").strip().lower()


def proc_ref(event):
    return event.get("subjectProc") or event.get("subject_proc") or event.get("process") or {}


def obj_ref(event):
    return event.get("object") or {}


def event_id(event):
    return str(event.get("id") or event.get("eventId") or event.get("event_id") or "")


def event_behavior(event):
    return lower(event.get("behavior") or event.get("kind"))


def event_binary(event):
    proc = proc_ref(event)
    return str(proc.get("binary") or "")


def event_argv(event):
    proc = proc_ref(event)
    argv = proc.get("argv") or proc.get("arguments") or []
    if isinstance(argv, list):
        return " ".join(str(x) for x in argv)
    return str(argv)


def event_path(event):
    obj = obj_ref(event)
    for key in ("filePath", "file_path", "path", "key"):
        if obj.get(key):
            return str(obj.get(key))
    return ""


def event_socket(event):
    obj = obj_ref(event)
    for key in ("socketAddr", "socket_addr", "dst", "addr"):
        if obj.get(key):
            return str(obj.get(key))
    return ""


def canonical_event(row):
    event = unwrap_event(row)
    binary = event_binary(event)
    path = event_path(event)
    socket = event_socket(event)
    entities = set()
    if binary:
        entities.add(f"process:{binary}")
        entities.add(f"process:{basename(binary)}")
    if path:
        entities.add(f"file:{path}")
    if socket:
        entities.add(f"socket:{socket}")
    return {
        "id": event_id(event),
        "behavior": event_behavior(event),
        "binary": binary,
        "binary_base": basename(binary),
        "argv": event_argv(event),
        "path": path,
        "socket": socket,
        "entities": entities,
        "raw": event,
    }


def signal_id(signal):
    return str(signal.get("id") or signal.get("signalId") or signal.get("signal_id") or "")


def signal_name(signal):
    return str(signal.get("name") or signal.get("ruleId") or signal.get("rule_id") or "")


def signal_terminal(signal):
    if bool(signal.get("terminal")):
        return True
    return bool(signal.get("responseIntent") or signal.get("response_intent"))


def signal_where(signal):
    return str(signal.get("where") or "").upper()


def signal_event_refs(signal):
    refs = signal.get("eventRefs") or signal.get("event_refs") or []
    if isinstance(refs, list):
        return [str(x) for x in refs]
    return []


def signal_entities(signal):
    entities = set()
    for ent in signal.get("entities") or []:
        if not isinstance(ent, dict):
            continue
        kind = str(ent.get("kind") or "").strip()
        key = str(ent.get("key") or "").strip()
        if kind and key:
            entities.add(f"{kind}:{key}")
        if key:
            entities.add(key)
    return entities


def canonical_signal(row):
    signal = unwrap_signal(row)
    return {
        "id": signal_id(signal),
        "name": signal_name(signal),
        "terminal": signal_terminal(signal),
        "where": signal_where(signal),
        "entities": signal_entities(signal),
        "event_refs": signal_event_refs(signal),
        "raw": signal,
    }


def matches_pattern(value, pattern):
    if pattern in ("", None):
        return True
    value = str(value or "")
    pattern = str(pattern)
    return fnmatch.fnmatch(value, pattern) or fnmatch.fnmatch(basename(value), pattern) or pattern in value


def event_label_matches(label, event):
    behavior = lower(label.get("behavior"))
    if behavior and event["behavior"] != behavior:
        return False
    match = label.get("match") or {}
    if not isinstance(match, dict):
        return False
    if "path" in match and not matches_pattern(event["path"], match.get("path")):
        return False
    if "socket" in match and str(match.get("socket")) != event["socket"]:
        return False
    if "dst" in match and str(match.get("dst")) != event["socket"]:
        return False
    if "dst_ip" in match:
        host = event["socket"].split(":", 1)[0]
        if str(match.get("dst_ip")) != host:
            return False
    if "dst_port" in match:
        port = event["socket"].rsplit(":", 1)[-1] if ":" in event["socket"] else ""
        if str(match.get("dst_port")) != port:
            return False
    if "process" in match and not (
        matches_pattern(event["binary"], match.get("process"))
        or matches_pattern(event["binary_base"], match.get("process"))
        or matches_pattern(event["argv"], match.get("process"))
    ):
        return False
    if "entity" in match and str(match.get("entity")) not in event["entities"]:
        return False
    return True


def signal_label_matches(label, signal):
    name = str(label.get("name") or "")
    if name and signal["name"] != name:
        return False
    if "terminal" in label and bool(label.get("terminal")) != signal["terminal"]:
        return False
    where = str(label.get("where") or "").upper()
    if where and where not in signal["where"]:
        return False
    for entity in label.get("entities") or []:
        if str(entity) not in signal["entities"]:
            return False
    return True


def ratio(hit, total):
    return round(hit / total, 4) if total else ""


def f1_score(precision, recall):
    if precision == "" or recall == "":
        return ""
    precision = number(precision)
    recall = number(recall)
    if precision + recall == 0:
        return 0.0
    return round((2 * precision * recall) / (precision + recall), 4)


def score_or_default(value, default=1.0):
    return default if value == "" else number(value)


def required(labels):
    return [x for x in labels if bool(x.get("required", True))]


def label_file(root, kind, topology, name):
    if kind == "workload":
        return root / "workloads" / topology / name / "labels.yaml"
    return root / "scenarios" / topology / name / "labels.yaml"


def case_kind(workload, scenario):
    if workload and scenario:
        return "cross"
    if workload:
        return "workload"
    return "scenario"


def case_paths(results, bench_case_dir=None):
    if not bench_case_dir:
        return None, None, None
    events_path = bench_case_dir / "events.ndjson"
    signals_path = bench_case_dir / "signals.ndjson"
    incidents_path = next(iter(sorted(bench_case_dir.glob("*incident*.json"))), None)
    return (
        events_path if events_path.exists() else None,
        signals_path if signals_path.exists() else None,
        incidents_path if incidents_path and incidents_path.exists() else None,
    )


def evaluate_case(labels_doc, events, signals, bench_summary):
    event_labels = labels_doc.get("labels", {}).get("events") or []
    signal_labels = labels_doc.get("labels", {}).get("signals") or []
    policy = labels_doc.get("policy") or {}
    kind = labels_doc.get("kind") or "unknown"
    observed_events = [canonical_event(row) for row in events]
    observed_signals = [canonical_signal(row) for row in signals]
    forbidden_signal_names = set(labels_doc.get("policy", {}).get("forbidden_signal_names") or [
        "download_by_lolbin",
        "payload_dropped",
        "payload_lifecycle",
        "suspicious_exec_connect",
        "reverse_shell_pattern",
        "sensitive_cred_read",
        "web_runtime_spawns_shell",
    ])

    event_matches = {}
    event_label_rows = []
    matched_event_ids = set()
    for label in event_labels:
        hits = [ev for ev in observed_events if event_label_matches(label, ev)]
        label_id = str(label.get("id") or "")
        event_matches[label_id] = hits
        for ev in hits:
            if ev["id"]:
                matched_event_ids.add(ev["id"])
        event_label_rows.append({
            "label_type": "event",
            "label_id": label_id,
            "required": bool(label.get("required", True)),
            "matched": bool(hits),
            "matched_count": len(hits),
            "matched_ids": " ".join(ev["id"] for ev in hits if ev["id"]),
            "match_quality": "",
        })

    signal_label_rows = []
    matched_signal_ids = set()
    linked_signal_count = 0
    for label in signal_labels:
        hits = [sig for sig in observed_signals if signal_label_matches(label, sig)]
        label_id = str(label.get("id") or "")
        link_ids = [str(x) for x in label.get("link_events") or []]
        link_total = len(link_ids)
        link_hit = 0
        for link_id in link_ids:
            expected_event_ids = {ev["id"] for ev in event_matches.get(link_id, []) if ev["id"]}
            if expected_event_ids and any(expected_event_ids.intersection(set(sig["event_refs"])) for sig in hits):
                link_hit += 1
        if hits:
            linked_signal_count += 1 if link_total == 0 or link_hit > 0 else 0
        for sig in hits:
            if sig["id"]:
                matched_signal_ids.add(sig["id"])
        quality = ratio(link_hit, link_total) if link_total else ""
        signal_label_rows.append({
            "label_type": "signal",
            "label_id": label_id,
            "required": bool(label.get("required", True)),
            "matched": bool(hits),
            "matched_count": len(hits),
            "matched_ids": " ".join(sig["id"] for sig in hits if sig["id"]),
            "match_quality": quality,
        })

    required_events = required(event_labels)
    required_signals = required(signal_labels)
    required_terminal = [label for label in required_signals if bool(label.get("terminal"))]
    event_hit = sum(1 for row in event_label_rows if row["required"] and row["matched"])
    signal_hit = sum(1 for row in signal_label_rows if row["required"] and row["matched"])
    terminal_hit = sum(1 for row in signal_label_rows if row["required"] and row["matched"] and any(l.get("id") == row["label_id"] and l.get("terminal") for l in signal_labels))
    terminal_observed = [sig for sig in observed_signals if sig["terminal"]]
    terminal_allowed = int(policy.get("terminal_signals_allowed", 999999))
    terminal_fp = max(0, len(terminal_observed) - terminal_allowed)
    if kind == "benign":
        false_positive_signals = len([sig for sig in observed_signals if sig["name"] in forbidden_signal_names or sig["terminal"]])
    else:
        false_positive_signals = len([sig for sig in observed_signals if sig["id"] not in matched_signal_ids])
    event_noise = len([ev for ev in observed_events if ev["id"] not in matched_event_ids])

    event_recall = ratio(event_hit, len(required_events))
    signal_recall = ratio(signal_hit, len(required_signals))
    terminal_recall = ratio(terminal_hit, len(required_terminal))
    signal_precision = ratio(len(observed_signals) - false_positive_signals, len(observed_signals))
    if signal_precision == "" and not observed_signals:
        signal_precision = 1.0
    signal_f1 = f1_score(signal_precision, signal_recall)
    event_noise_ratio = ratio(event_noise, len(observed_events))
    if not event_labels:
        event_noise_ratio = ""
    signal_event_link_rate = ratio(linked_signal_count, len([row for row in signal_label_rows if row["matched"]]))

    if kind == "benign":
        terminal_policy_score = 1.0 if terminal_fp == 0 else 0.0
        fp_policy_score = 1.0 if false_positive_signals == 0 else 0.0
        effectiveness = round(0.7 * fp_policy_score + 0.3 * terminal_policy_score, 4)
    else:
        effectiveness = round(
            0.45 * score_or_default(event_recall, 0.0)
            + 0.40 * score_or_default(signal_recall, 0.0)
            + 0.10 * score_or_default(terminal_recall, 0.0)
            + 0.05 * score_or_default(signal_precision, 1.0),
            4,
        )

    workload_phase = (bench_summary or {}).get("phases", {}).get("workload", {})
    drops = int(workload_phase.get("dropped_events_delta") or 0)
    parse_errors = int(workload_phase.get("parse_errors_delta") or 0)
    events_delta = int(workload_phase.get("events_delta") or len(observed_events))
    edr_cpu = ((workload_phase.get("edr_cpu_pct") or {}).get("avg") or 0.0)
    cost_per_1k = round(float(edr_cpu) / events_delta * 1000, 4) if events_delta > 0 else 0.0

    return {
        "metrics": {
            "label_kind": kind,
            "effectiveness_score": effectiveness,
            "event_recall": event_recall,
            "signal_recall": signal_recall,
            "signal_f1": signal_f1,
            "terminal_recall": terminal_recall,
            "signal_precision": signal_precision,
            "event_noise_ratio": event_noise_ratio,
            "signal_event_link_rate": signal_event_link_rate,
            "false_positive_signals": false_positive_signals,
            "terminal_false_positive_signals": terminal_fp,
            "observed_events": len(observed_events),
            "observed_signals": len(observed_signals),
            "matched_event_labels": event_hit,
            "required_event_labels": len(required_events),
            "matched_signal_labels": signal_hit,
            "required_signal_labels": len(required_signals),
            "drop_rate": round(drops / events_delta, 4) if events_delta > 0 else 0.0,
            "parse_error_rate": round(parse_errors / events_delta, 4) if events_delta > 0 else 0.0,
            "cost_per_1k_events_cpu": cost_per_1k,
        },
        "truth_steps": event_label_rows + signal_label_rows,
    }


def read_bench_rows(bench_matrix_dir):
    if not bench_matrix_dir:
        return []
    path = Path(bench_matrix_dir) / "matrix.csv"
    if not path.exists():
        return []
    with path.open(newline="") as f:
        return list(csv.DictReader(f))


def discover_bench_cases(root, results, matrix_dir):
    cases = []
    if not matrix_dir:
        return cases
    matrix_dir = Path(matrix_dir)
    cases_dir = matrix_dir / "cases"
    if cases_dir.exists():
        for case_dir in sorted(p for p in cases_dir.iterdir() if p.is_dir()):
            status = load_json(case_dir / "status.json")
            bench_run_id = status.get("bench_run_id", "")
            bench_root = results / "bench-collection-vm" / bench_run_id
            if not bench_root.exists():
                continue
            workload = status.get("workload", "")
            scenario = status.get("scenario", "")
            kind = case_kind(workload, scenario)
            name = scenario or workload
            for policy_dir in sorted(p for p in bench_root.iterdir() if p.is_dir()):
                cases.append({
                    "kind": kind,
                    "name": name,
                    "workload": workload,
                    "scenario": scenario,
                    "policy": policy_dir.name,
                    "bench_case_dir": policy_dir,
                })
    return cases


def build_rows(args):
    root = Path(args.root)
    results = root / ".results"
    bench_cases = discover_bench_cases(root, results, args.bench_matrix_dir)
    rows = []
    truth_rows = []
    details = {}
    scenario_filter = set(args.scenarios or [])
    workload_filter = set(args.workloads or [])

    for case in bench_cases:
        if case["kind"] in ("scenario", "cross") and scenario_filter and case.get("scenario") not in scenario_filter:
            continue
        if case["kind"] in ("workload", "cross") and workload_filter and case.get("workload") not in workload_filter:
            continue
        label_kind = "scenario" if case.get("scenario") else "workload"
        label_name = case.get("scenario") or case.get("workload") or case["name"]
        labels_path = label_file(root, label_kind, args.topology, label_name)
        if not labels_path.exists():
            continue
        labels_doc = load_yaml(labels_path)
        events_path, signals_path, incidents_path = case_paths(results, case["bench_case_dir"])
        all_events = load_ndjson(events_path) if events_path else []
        all_signals = load_ndjson(signals_path) if signals_path else []
        bench_summary = load_json(case["bench_case_dir"] / "summary.json")
        window_start, window_end, window_name = effectiveness_window(bench_summary)
        events = filter_frames_by_window(all_events, window_start, window_end)
        signals = filter_frames_by_window(all_signals, window_start, window_end)
        evaluation = evaluate_case(labels_doc, events, signals, bench_summary)
        metrics = evaluation["metrics"]
        base = {
            "kind": case["kind"],
            "name": case["name"],
            "workload": case.get("workload", ""),
            "scenario": case.get("scenario", ""),
            "policy": case["policy"],
            "label_file": str(labels_path),
            "effectiveness_window": window_name,
            "observed_events_total": len(all_events),
            "observed_signals_total": len(all_signals),
            "events_path": str(events_path) if events_path else "",
            "signals_path": str(signals_path) if signals_path else "",
            "incidents_path": str(incidents_path) if incidents_path else "",
        }
        row = {**base, **metrics}
        rows.append(row)
        key = f"{case['kind']}:{case['name']}:{case['policy']}"
        details[key] = {
            **base,
            "labels": labels_doc,
            "metrics": metrics,
            "truth_steps": evaluation["truth_steps"],
        }
        for step in evaluation["truth_steps"]:
            truth_rows.append({
                "kind": case["kind"],
                "name": case["name"],
                "workload": case.get("workload", ""),
                "scenario": case.get("scenario", ""),
                "policy": case["policy"],
                **step,
            })
    return rows, truth_rows, details


def minmax_score(value, values, reverse=False):
    nums = [number(v) for v in values if present(v)]
    if not nums:
        return 0.0
    lo, hi = min(nums), max(nums)
    if hi == lo:
        return 1.0
    base = (number(value) - lo) / (hi - lo)
    if reverse:
        base = 1.0 - base
    return round(base, 4)


def average_field(rows, field):
    vals = [number(r.get(field)) for r in rows if present(r.get(field))]
    return statistics.mean(vals) if vals else None


def build_policy_comparison(effect_rows, bench_rows):
    policies = sorted({r.get("policy") for r in effect_rows if r.get("policy")} | {r.get("policy_dir") for r in bench_rows if r.get("policy_dir")})
    workload_rows = [r for r in bench_rows if r.get("kind") == "workload"]
    cpu_values = [r.get("workload_edr_cpu_avg_pct") for r in workload_rows]
    rss_values = [r.get("workload_edr_rss_avg_mb") for r in workload_rows]
    comparison = []
    for policy in policies:
        erows = [r for r in effect_rows if r.get("policy") == policy]
        brows = [r for r in workload_rows if r.get("policy_dir") == policy]
        eff = average_field(erows, "effectiveness_score") or 0.0
        event_recall = average_field(erows, "event_recall")
        signal_recall = average_field(erows, "signal_recall")
        signal_precision = average_field(erows, "signal_precision")
        signal_f1 = average_field(erows, "signal_f1")
        cpu = statistics.mean([number(r.get("workload_edr_cpu_avg_pct")) for r in brows]) if brows else 0.0
        rss = statistics.mean([number(r.get("workload_edr_rss_avg_mb")) for r in brows]) if brows else 0.0
        drops = sum(number(r.get("workload_dropped_events_delta")) for r in brows)
        parse_errors = sum(number(r.get("workload_parse_errors_delta")) for r in brows)
        cpu_score = minmax_score(cpu, cpu_values, reverse=True)
        rss_score = minmax_score(rss, rss_values, reverse=True)
        resource_score = round((cpu_score * 0.75) + (rss_score * 0.25), 4)
        stability_score = 1.0 if drops == 0 and parse_errors == 0 else 0.0
        overall = round(eff * 0.65 + resource_score * 0.25 + stability_score * 0.10, 4)
        comparison.append({
            "policy": policy,
            "overall_score": overall,
            "effectiveness_score": round(eff, 4),
            "event_recall": round(event_recall, 4) if event_recall is not None else "",
            "signal_recall": round(signal_recall, 4) if signal_recall is not None else "",
            "signal_precision": round(signal_precision, 4) if signal_precision is not None else "",
            "signal_f1": round(signal_f1, 4) if signal_f1 is not None else "",
            "resource_score": resource_score,
            "stability_score": stability_score,
            "workload_edr_cpu_avg_pct": round(cpu, 4),
            "workload_edr_rss_avg_mb": round(rss, 4),
            "dropped_events_total": int(drops),
            "parse_errors_total": int(parse_errors),
        })
    return sorted(comparison, key=lambda r: r["overall_score"], reverse=True)


def policy_sort_key(policy):
    order = {
        "collection-minimal-high-signal": 0,
        "collection-edr-balanced": 1,
        "collection-incident-deep": 2,
    }
    return (order.get(policy, 99), policy)


def build_attack_signal_matrix(effect_rows):
    attacks = sorted({r.get("name") for r in effect_rows if r.get("label_kind") == "malicious" and r.get("name")})
    policies = sorted({r.get("policy") for r in effect_rows if r.get("label_kind") == "malicious" and r.get("policy")}, key=policy_sort_key)
    by_key = {}
    for r in effect_rows:
        if r.get("label_kind") != "malicious":
            continue
        by_key.setdefault((r.get("policy"), r.get("name")), []).append(r)
    rows = []
    for policy in policies:
        row = {"policy": policy}
        for attack in attacks:
            items = by_key.get((policy, attack)) or []
            precisions = [number(r.get("signal_precision")) for r in items if present(r.get("signal_precision"))]
            recalls = [number(r.get("signal_recall")) for r in items if present(r.get("signal_recall"))]
            f1s = [number(r.get("signal_f1")) for r in items if present(r.get("signal_f1"))]
            if not precisions or not recalls or not f1s:
                row[attack] = ""
            else:
                avg_p = statistics.mean(precisions)
                avg_r = statistics.mean(recalls)
                avg_f1 = statistics.mean(f1s)
                row[attack] = f"P={avg_p:.2f} R={avg_r:.2f} F1={avg_f1:.2f}"
        rows.append(row)
    return attacks, rows


def write_attack_signal_matrix_md(path, attacks, rows):
    lines = []
    lines.append("| policy | " + " | ".join(attacks) + " |")
    lines.append("|---|" + "|".join(["---"] * len(attacks)) + "|")
    for row in rows:
        lines.append("| " + row["policy"] + " | " + " | ".join(row.get(attack, "") for attack in attacks) + " |")
    path.write_text("\n".join(lines) + "\n")


def write_csv(path, rows, fields):
    with path.open("w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=fields, extrasaction="ignore")
        writer.writeheader()
        for row in rows:
            writer.writerow(row)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--root", default=str(Path(__file__).resolve().parents[2]))
    ap.add_argument("--topology", default="vm")
    ap.add_argument("--scenarios", nargs="*")
    ap.add_argument("--workloads", nargs="*")
    ap.add_argument("--bench-matrix-dir")
    ap.add_argument("--output-dir", required=True)
    ap.add_argument("--scope", choices=("local", "manager", "full", "control"), default="local")
    args = ap.parse_args()

    out_dir = Path(args.output_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    rows, truth_rows, details = build_rows(args)
    bench_rows = read_bench_rows(args.bench_matrix_dir)
    comparison = build_policy_comparison(rows, bench_rows)
    attack_signal_fields, attack_signal_rows = build_attack_signal_matrix(rows)
    summary = {
        "topology": args.topology,
        "evaluation_model": "labels.yaml supervised event/signal matching",
        "score_model": {
            "malicious_effectiveness": "0.45*event_recall + 0.40*signal_recall + 0.10*terminal_recall + 0.05*signal_precision",
            "benign_effectiveness": "0.70*signal_precision + 0.30*terminal_policy_score",
            "overall_score": "0.65*effectiveness_score + 0.25*resource_score + 0.10*stability_score",
        },
        "rows": rows,
        "truth_steps": truth_rows,
        "policy_comparison": comparison,
        "attack_signal_matrix": attack_signal_rows,
        "details": details,
    }
    (out_dir / "summary.json").write_text(json.dumps(summary, indent=2, sort_keys=True, ensure_ascii=False) + "\n")
    matrix_fields = [
        "kind",
        "label_kind",
        "name",
        "workload",
        "scenario",
        "policy",
        "effectiveness_score",
        "event_recall",
        "signal_recall",
        "signal_f1",
        "terminal_recall",
        "signal_precision",
        "event_noise_ratio",
        "signal_event_link_rate",
        "false_positive_signals",
        "terminal_false_positive_signals",
        "observed_events",
        "observed_events_total",
        "observed_signals",
        "observed_signals_total",
        "matched_event_labels",
        "required_event_labels",
        "matched_signal_labels",
        "required_signal_labels",
        "drop_rate",
        "parse_error_rate",
        "cost_per_1k_events_cpu",
        "effectiveness_window",
        "label_file",
        "events_path",
        "signals_path",
    ]
    truth_fields = [
        "kind",
        "workload",
        "scenario",
        "name",
        "policy",
        "label_type",
        "label_id",
        "required",
        "matched",
        "matched_count",
        "matched_ids",
        "match_quality",
    ]
    comparison_fields = [
        "policy",
        "overall_score",
        "effectiveness_score",
        "event_recall",
        "signal_recall",
        "signal_precision",
        "signal_f1",
        "resource_score",
        "stability_score",
        "workload_edr_cpu_avg_pct",
        "workload_edr_rss_avg_mb",
        "dropped_events_total",
        "parse_errors_total",
    ]
    write_csv(out_dir / "matrix.csv", rows, matrix_fields)
    write_csv(out_dir / "truth_steps.csv", truth_rows, truth_fields)
    write_csv(out_dir / "policy_comparison.csv", comparison, comparison_fields)
    write_csv(out_dir / "attack_signal_matrix.csv", attack_signal_rows, ["policy", *attack_signal_fields])
    write_attack_signal_matrix_md(out_dir / "attack_signal_matrix.md", attack_signal_fields, attack_signal_rows)
    (out_dir / "matrix.json").write_text(json.dumps(rows, indent=2, sort_keys=True) + "\n")
    (out_dir / "policy_comparison.json").write_text(json.dumps(comparison, indent=2, sort_keys=True) + "\n")


if __name__ == "__main__":
    main()
