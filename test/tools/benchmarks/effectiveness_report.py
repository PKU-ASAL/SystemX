#!/usr/bin/env python3
import argparse
import csv
import fnmatch
import json
import statistics
import sys
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


def as_list(value):
    if not value:
        return []
    if isinstance(value, list):
        return value
    return [value]


def number(value, default=0.0):
    try:
        return float(value)
    except (TypeError, ValueError):
        return default


def present(value):
    return value not in ("", None)


def unwrap_event(row):
    body = row.get("event") if isinstance(row, dict) else None
    if isinstance(body, dict):
        return body
    return row if isinstance(row, dict) else {}


def unwrap_signal(row):
    body = row.get("signal") if isinstance(row, dict) else None
    if isinstance(body, dict):
        return body
    return row if isinstance(row, dict) else {}


def unwrap_incidents(data):
    if isinstance(data, list):
        return [x for x in data if isinstance(x, dict)]
    if isinstance(data, dict):
        for key in ("incidents", "items", "data"):
            value = data.get(key)
            if isinstance(value, list):
                return [x for x in value if isinstance(x, dict)]
    return []


def event_behavior(event):
    return str(event.get("behavior") or event.get("kind") or "").lower()


def event_binary(event):
    proc = event.get("subjectProc") or event.get("subject_proc") or event.get("process") or {}
    return str(proc.get("binary") or "")


def event_argv(event):
    proc = event.get("subjectProc") or event.get("subject_proc") or event.get("process") or {}
    argv = proc.get("argv") or proc.get("arguments") or []
    if isinstance(argv, list):
        return " ".join(str(x) for x in argv)
    return str(argv)


def event_object_value(event):
    obj = event.get("object") or {}
    values = []
    for key in ("socketAddr", "socket_addr", "addr", "dst", "filePath", "file_path", "path", "key"):
        if obj.get(key):
            values.append(str(obj.get(key)))
    return values


def event_matches(want, event):
    if not isinstance(want, dict):
        raw = json.dumps(event, sort_keys=True, ensure_ascii=False)
        return str(want) in raw
    kind = str(want.get("kind", "")).upper()
    behavior = event_behavior(event)
    values = event_object_value(event)
    binary = event_binary(event)
    argv = event_argv(event)

    if kind == "EXEC":
        if behavior and behavior != "process.exec":
            return False
        pattern = str(want.get("binary") or "")
        if not pattern:
            return True
        return fnmatch.fnmatch(binary, pattern) or fnmatch.fnmatch(binary.split("/")[-1], pattern.split("/")[-1])

    if kind == "CONNECT":
        if behavior and behavior != "network.connect":
            return False
        dst = str(want.get("dst") or "")
        return bool(dst and (dst in values or dst in argv))

    if kind in ("WRITE", "CHMOD"):
        want_path = str(want.get("path") or "")
        if kind == "WRITE" and behavior and behavior != "file.write":
            return False
        if kind == "CHMOD" and behavior and behavior != "file.chmod":
            return False
        return bool(want_path and (want_path in values or want_path in argv))

    if kind in ("OPEN", "READ"):
        want_path = str(want.get("path") or "")
        if behavior and behavior not in ("file.open", "file.read"):
            return False
        return bool(want_path and (want_path in values or want_path in argv))

    raw = json.dumps(event, sort_keys=True, ensure_ascii=False)
    return all(str(v) in raw for v in want.values() if not isinstance(v, bool))


def event_hit(want, events):
    return any(event_matches(want, unwrap_event(row)) for row in events)


def signal_where(signal):
    return str(signal.get("where") or "").upper()


def is_endpoint_signal(signal):
    where = signal_where(signal)
    return not where or "ENDPOINT" in where


def is_cloud_signal(signal):
    where = signal_where(signal)
    return "CLOUD" in where or "MANAGER" in where


def entity_keys(signal):
    keys = set()
    for ent in signal.get("entities") or []:
        if not isinstance(ent, dict):
            continue
        kind = str(ent.get("kind") or "")
        key = str(ent.get("key") or "")
        if key:
            keys.add(key)
        if kind and key:
            keys.add(f"{kind}:{key}")
    return keys


def signal_matches(want, signal, layer):
    if layer == "endpoint_signals" and not is_endpoint_signal(signal):
        return False
    if layer == "cloud_signals" and not is_cloud_signal(signal):
        return False

    if isinstance(want, dict):
        name = want.get("name")
        if name and signal.get("name") != name:
            return False
        terminal = want.get("terminal")
        if terminal is not None and bool(signal.get("terminal", False)) != bool(terminal):
            return False
        if want.get("cross_lineage") is not None and bool(signal.get("crossLineage") or signal.get("cross_lineage")) != bool(want.get("cross_lineage")):
            return False
        keys = entity_keys(signal)
        for expected_key in want.get("entities_keys") or []:
            if str(expected_key) not in keys:
                return False
        if want.get("has_evidence_bundle") and not signal.get("evidence"):
            return False
        return bool(name or want.get("entities_keys") or terminal is not None)

    return signal.get("name") == str(want)


def signal_hit(want, signals, layer):
    return any(signal_matches(want, unwrap_signal(row), layer) for row in signals)


def layer_signals(signals, layer):
    unwrapped = [unwrap_signal(row) for row in signals]
    if layer == "endpoint_signals":
        return [s for s in unwrapped if is_endpoint_signal(s)]
    if layer == "cloud_signals":
        return [s for s in unwrapped if is_cloud_signal(s)]
    return unwrapped


def terminal_signal_count(signals, layer="endpoint_signals"):
    return sum(1 for signal in layer_signals(signals, layer) if signal.get("terminal") is True)


def signals_have_entities(signals, layer):
    selected = layer_signals(signals, layer)
    return bool(selected) and all(bool(sig.get("entities")) for sig in selected)


def incident_matches(key, want, incident):
    if key == "count":
        return None
    if key == "lineage_ids_min":
        values = incident.get("lineage_ids") or incident.get("lineageIds") or []
        return len(values) >= int(want)
    if key == "converge_method":
        converge = incident.get("converge") or {}
        return converge.get("method") == want or incident.get("converge_method") == want
    if key == "terminals_include_binary":
        raw = json.dumps(incident, ensure_ascii=False)
        return all(str(x) in raw for x in as_list(want))
    if key in ("evidence_subgraph_path", "evidence_subgraph_contains_node"):
        raw = json.dumps(incident, ensure_ascii=False)
        return all(str(x) in raw for x in as_list(want))
    raw = json.dumps(incident, ensure_ascii=False)
    return str(want) in raw


def incident_hit(key, want, incidents):
    if not incidents:
        return None
    if key == "count":
        return len(incidents) == int(want)
    return any(incident_matches(key, want, inc) for inc in incidents)


def flatten_expected(expected):
    checks = []
    for want in as_list(expected.get("events", {}).get("must_contain")):
        checks.append({"layer": "events", "requirement": "must_contain", "want": want})
    for layer in ("endpoint_signals", "cloud_signals"):
        spec = expected.get(layer, {})
        for want in as_list(spec.get("must_contain")):
            checks.append({"layer": layer, "requirement": "must_contain", "want": want})
        for want in as_list(spec.get("may_contain")):
            checks.append({"layer": layer, "requirement": "may_contain", "want": want})
        if spec.get("must_have_entities"):
            checks.append({"layer": layer, "requirement": "must_have_entities", "want": True})
    incident = expected.get("incident")
    if isinstance(incident, dict):
        for key, value in incident.items():
            checks.append({"layer": "incident", "requirement": key, "want": value})
    negative = expected.get("negative")
    if isinstance(negative, dict):
        for key, value in negative.items():
            checks.append({"layer": "negative", "requirement": key, "want": value})
    for want in as_list(expected.get("control_assertions")):
        checks.append({"layer": "control", "requirement": "control_assertion", "want": want})
    return checks


def score(hit, total):
    return round(hit / total, 4) if total else None


def scope_allows(scope, layer):
    if scope == "full":
        return True
    if scope == "local":
        return layer in ("events", "endpoint_signals", "negative")
    if scope == "manager":
        return layer in ("events", "endpoint_signals", "cloud_signals", "incident", "negative")
    if scope == "control":
        return layer == "control"
    return True


def summarize_effectiveness(expected, events, signals, incidents=None, bench_summary=None, scope="full"):
    incidents = incidents or []
    checks = flatten_expected(expected)
    evaluated = []
    out_of_scope = []
    totals = {
        "required": [0, 0],
        "event": [0, 0],
        "signal": [0, 0],
        "endpoint_signal": [0, 0],
        "cloud_signal": [0, 0],
        "incident": [0, 0],
        "negative": [0, 0],
    }

    has_cloud_source = bool(layer_signals(signals, "cloud_signals"))
    has_incident_source = bool(incidents)

    for check in checks:
        layer = check["layer"]
        requirement = check["requirement"]
        want = check["want"]
        required = requirement not in ("may_contain", "control_assertion")
        hit = None
        in_scope = scope_allows(scope, layer)

        if not in_scope:
            out = dict(check)
            out["hit"] = None
            out["required"] = required
            out["evaluated"] = False
            out["out_of_scope"] = True
            out["out_of_scope_reason"] = f"{layer} is outside evaluation_scope={scope}"
            evaluated.append(out)
            out_of_scope.append(out)
            continue

        if layer == "events" and requirement == "must_contain":
            hit = event_hit(want, events)
            totals["event"][0] += 1
            totals["event"][1] += int(hit)
        elif layer in ("endpoint_signals", "cloud_signals") and requirement in ("must_contain", "may_contain"):
            if layer == "cloud_signals" and not has_cloud_source:
                hit = None
            else:
                hit = signal_hit(want, signals, layer)
                if requirement == "must_contain":
                    totals["signal"][0] += 1
                    totals["signal"][1] += int(hit)
                    totals["endpoint_signal" if layer == "endpoint_signals" else "cloud_signal"][0] += 1
                    totals["endpoint_signal" if layer == "endpoint_signals" else "cloud_signal"][1] += int(hit)
        elif layer in ("endpoint_signals", "cloud_signals") and requirement == "must_have_entities":
            if layer == "cloud_signals" and not has_cloud_source:
                hit = None
            else:
                hit = signals_have_entities(signals, layer)
                totals["signal"][0] += 1
                totals["signal"][1] += int(hit)
                totals["endpoint_signal" if layer == "endpoint_signals" else "cloud_signal"][0] += 1
                totals["endpoint_signal" if layer == "endpoint_signals" else "cloud_signal"][1] += int(hit)
        elif layer == "incident":
            if not has_incident_source:
                hit = None
            else:
                hit = incident_hit(requirement, want, incidents)
                totals["incident"][0] += 1
                totals["incident"][1] += int(hit)
        elif layer == "negative" and requirement == "endpoint_terminal_count":
            hit = terminal_signal_count(signals) == int(want)
            totals["negative"][0] += 1
            totals["negative"][1] += int(hit)
        elif layer == "negative" and requirement == "endpoint_terminal_required":
            want_bool = bool(want)
            hit = terminal_signal_count(signals) > 0 if want_bool else terminal_signal_count(signals) == 0
            totals["negative"][0] += 1
            totals["negative"][1] += int(hit)
        elif layer == "control":
            required = False
            hit = None

        if required and hit is not None:
            totals["required"][0] += 1
            totals["required"][1] += int(hit)

        out = dict(check)
        out["hit"] = hit
        out["required"] = required
        out["evaluated"] = hit is not None
        out["out_of_scope"] = False
        evaluated.append(out)

    workload_phase = (bench_summary or {}).get("phases", {}).get("workload", {})
    drops = int(workload_phase.get("dropped_events_delta") or 0)
    parse_errors = int(workload_phase.get("parse_errors_delta") or 0)
    events_delta = int(workload_phase.get("events_delta") or len(events))
    edr_cpu = ((workload_phase.get("edr_cpu_pct") or {}).get("avg") or 0.0)
    cost_per_1k = round(float(edr_cpu) / events_delta * 1000, 4) if events_delta > 0 else 0.0

    return {
        "evaluation_scope": scope,
        "checks": evaluated,
        "out_of_scope_checks": out_of_scope,
        "out_of_scope_checks_total": len(out_of_scope),
        "required_checks_total": totals["required"][0],
        "required_checks_hit": totals["required"][1],
        "effectiveness_score": score(totals["required"][1], totals["required"][0]) or 0.0,
        "required_hit_rate": score(totals["required"][1], totals["required"][0]) or 0.0,
        "required_event_total": totals["event"][0],
        "required_event_hit": totals["event"][1],
        "required_event_hit_rate": score(totals["event"][1], totals["event"][0]),
        "required_signal_total": totals["signal"][0],
        "required_signal_hit": totals["signal"][1],
        "signal_hit_rate": score(totals["signal"][1], totals["signal"][0]),
        "endpoint_signal_hit_rate": score(totals["endpoint_signal"][1], totals["endpoint_signal"][0]),
        "cloud_signal_hit_rate": score(totals["cloud_signal"][1], totals["cloud_signal"][0]),
        "incident_hit_rate": score(totals["incident"][1], totals["incident"][0]),
        "negative_hit_rate": score(totals["negative"][1], totals["negative"][0]),
        "observed_events": len(events),
        "observed_signals": len(signals),
        "observed_incidents": len(incidents),
        "terminal_signal_count": terminal_signal_count(signals),
        "drop_rate": round(drops / events_delta, 4) if events_delta > 0 else 0.0,
        "parse_error_rate": round(parse_errors / events_delta, 4) if events_delta > 0 else 0.0,
        "cost_per_1k_events_cpu": cost_per_1k,
    }


def case_paths(results, topology, scenario, bench_case_dir=None):
    candidates = []
    if bench_case_dir:
        candidates.append(bench_case_dir / "events.ndjson")
        candidates.extend(sorted(bench_case_dir.glob("*/events.ndjson")))
    candidates.extend([
        results / f"{topology}.{scenario}.events.ndjson",
        results / f"{scenario}.{topology}.events.ndjson",
        results / f"{scenario}.{topology}.tetragon.jsonl",
    ])
    events_path = next((p for p in candidates if p.exists()), None)

    signal_candidates = []
    if bench_case_dir:
        signal_candidates.append(bench_case_dir / "signals.ndjson")
        signal_candidates.extend(sorted(bench_case_dir.glob("*/signals.ndjson")))
    signal_candidates.extend([
        results / f"{topology}.{scenario}.signals.ndjson",
        results / f"{scenario}.{topology}.signals.ndjson",
    ])
    signals_path = next((p for p in signal_candidates if p.exists()), None)

    incident_candidates = []
    if bench_case_dir:
        incident_candidates.extend(sorted(bench_case_dir.glob("*incident*.json")))
    incident_candidates.extend([
        results / f"{topology}.{scenario}.incidents.json",
        results / f"{scenario}.{topology}.incidents.json",
        results / f"e2e-agent-{scenario}.incidents.json",
    ])
    incidents_path = next((p for p in incident_candidates if p.exists()), None)
    return events_path, signals_path, incidents_path


def build_rows(args):
    root = Path(args.root)
    results = root / ".results"
    scenarios_root = root / "scenarios" / args.topology
    rows = []
    details = {}

    bench_cases = {}
    if args.bench_matrix_dir:
        matrix_dir = Path(args.bench_matrix_dir)
        for case_dir in sorted((matrix_dir / "scenario").glob("*")) if (matrix_dir / "scenario").exists() else []:
            if not case_dir.is_dir():
                continue
            status = load_json(case_dir / "status.json")
            bench_run_id = status.get("bench_run_id", "")
            bench_root = results / "bench-collection-vm" / bench_run_id
            for policy_dir in sorted(p for p in bench_root.iterdir() if p.is_dir()) if bench_root.exists() else []:
                bench_cases[(case_dir.name, policy_dir.name)] = policy_dir

    scenario_names = args.scenarios or sorted(p.name for p in scenarios_root.iterdir() if p.is_dir())
    policy_names = sorted({policy for _, policy in bench_cases.keys()}) or [""]
    for scenario in scenario_names:
        expected_path = scenarios_root / scenario / "expected.yaml"
        if not expected_path.exists():
            continue
        expected = yaml.safe_load(expected_path.read_text()) or {}
        for policy in policy_names:
            bench_case_dir = bench_cases.get((scenario, policy))
            events_path, signals_path, incidents_path = case_paths(results, args.topology, scenario, bench_case_dir)
            events = load_ndjson(events_path) if events_path else []
            signals = load_ndjson(signals_path) if signals_path else []
            incidents = unwrap_incidents(load_json(incidents_path)) if incidents_path else []
            bench_summary = load_json(bench_case_dir / "summary.json") if bench_case_dir else {}
            assertion = load_json(results / f"{scenario}.json")
            if not assertion:
                assertion = load_json(results / f"{args.topology}.{scenario}.json")
            eff = summarize_effectiveness(expected, events, signals, incidents, bench_summary, args.scope)
            key = f"{scenario}:{policy or 'default'}"
            details[key] = {
                "scenario": scenario,
                "policy": policy,
                "evaluation_scope": args.scope,
                "expected": expected,
                "events_path": str(events_path) if events_path else "",
                "signals_path": str(signals_path) if signals_path else "",
                "incidents_path": str(incidents_path) if incidents_path else "",
                "assertion": assertion,
                **eff,
            }
            rows.append({
                "scenario": scenario,
                "policy": policy,
                "evaluation_scope": args.scope,
                "effectiveness_score": eff["effectiveness_score"],
                "required_hit_rate": eff["required_hit_rate"],
                "required_event_hit_rate": eff["required_event_hit_rate"],
                "signal_hit_rate": eff["signal_hit_rate"],
                "endpoint_signal_hit_rate": eff["endpoint_signal_hit_rate"],
                "cloud_signal_hit_rate": eff["cloud_signal_hit_rate"],
                "incident_hit_rate": eff["incident_hit_rate"],
                "negative_hit_rate": eff["negative_hit_rate"],
                "observed_events": eff["observed_events"],
                "observed_signals": eff["observed_signals"],
                "observed_incidents": eff["observed_incidents"],
                "terminal_signal_count": eff["terminal_signal_count"],
                "drop_rate": eff["drop_rate"],
                "parse_error_rate": eff["parse_error_rate"],
                "cost_per_1k_events_cpu": eff["cost_per_1k_events_cpu"],
                "out_of_scope_checks_total": eff["out_of_scope_checks_total"],
                "assert_pass": assertion.get("pass", ""),
                "assert_fail": assertion.get("fail", ""),
                "assert_skip": assertion.get("skip", ""),
                "events_path": str(events_path) if events_path else "",
                "signals_path": str(signals_path) if signals_path else "",
                "incidents_path": str(incidents_path) if incidents_path else "",
            })
    return rows, details


def read_bench_rows(bench_matrix_dir):
    if not bench_matrix_dir:
        return []
    path = Path(bench_matrix_dir) / "matrix.csv"
    if not path.exists():
        return []
    with path.open(newline="") as f:
        return list(csv.DictReader(f))


def minmax_score(value, values, reverse=False):
    nums = [number(v) for v in values]
    if not nums:
        return 0.0
    lo, hi = min(nums), max(nums)
    if hi == lo:
        return 1.0
    base = (number(value) - lo) / (hi - lo)
    if reverse:
        base = 1.0 - base
    return round(base, 4)


def build_policy_comparison(effect_rows, bench_rows):
    policies = sorted({r.get("policy") for r in effect_rows if r.get("policy")} | {r.get("policy_dir") for r in bench_rows if r.get("policy_dir")})
    workload_rows = [r for r in bench_rows if r.get("kind") == "workload"]
    comparison = []
    cpu_values = [r.get("workload_edr_cpu_avg_pct") for r in workload_rows]
    rss_values = [r.get("workload_edr_rss_avg_mb") for r in workload_rows]

    for policy in policies:
        erows = [r for r in effect_rows if r.get("policy") == policy]
        brows = [r for r in workload_rows if r.get("policy_dir") == policy]
        eff = statistics.mean([number(r.get("effectiveness_score")) for r in erows]) if erows else 0.0
        event_vals = [number(r.get("required_event_hit_rate")) for r in erows if present(r.get("required_event_hit_rate"))]
        signal_vals = [number(r.get("signal_hit_rate")) for r in erows if present(r.get("signal_hit_rate"))]
        event_eff = statistics.mean(event_vals) if event_vals else None
        signal_eff = statistics.mean(signal_vals) if signal_vals else None
        cpu = statistics.mean([number(r.get("workload_edr_cpu_avg_pct")) for r in brows]) if brows else 0.0
        rss = statistics.mean([number(r.get("workload_edr_rss_avg_mb")) for r in brows]) if brows else 0.0
        eps = statistics.mean([number(r.get("workload_eps")) for r in brows]) if brows else 0.0
        drops = sum(number(r.get("workload_dropped_events_delta")) for r in brows)
        parse_errors = sum(number(r.get("workload_parse_errors_delta")) for r in brows)
        cpu_score = minmax_score(cpu, cpu_values, reverse=True)
        rss_score = minmax_score(rss, rss_values, reverse=True)
        resource_score = round((cpu_score * 0.75) + (rss_score * 0.25), 4)
        stability_score = 1.0 if drops == 0 and parse_errors == 0 else 0.0
        overall = round(eff * 0.6 + resource_score * 0.3 + stability_score * 0.1, 4)
        comparison.append({
            "policy": policy,
            "overall_score": overall,
            "effectiveness_score": round(eff, 4),
            "event_hit_rate": round(event_eff, 4) if event_eff is not None else "",
            "signal_hit_rate": round(signal_eff, 4) if signal_eff is not None else "",
            "resource_score": resource_score,
            "stability_score": stability_score,
            "workload_edr_cpu_avg_pct": round(cpu, 4),
            "workload_edr_rss_avg_mb": round(rss, 4),
            "workload_eps_avg": round(eps, 4),
            "dropped_events_total": int(drops),
            "parse_errors_total": int(parse_errors),
        })
    return sorted(comparison, key=lambda r: r["overall_score"], reverse=True)


def write_csv(path, rows, fields):
    with path.open("w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=fields)
        writer.writeheader()
        for row in rows:
            writer.writerow(row)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--root", default=str(Path(__file__).resolve().parents[2]))
    ap.add_argument("--topology", default="vm")
    ap.add_argument("--scenarios", nargs="*")
    ap.add_argument("--bench-matrix-dir")
    ap.add_argument("--output-dir", required=True)
    ap.add_argument("--scope", choices=("local", "manager", "full", "control"), default="full")
    args = ap.parse_args()

    out_dir = Path(args.output_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    rows, details = build_rows(args)
    bench_rows = read_bench_rows(args.bench_matrix_dir)
    comparison = build_policy_comparison(rows, bench_rows)
    summary = {
        "topology": args.topology,
        "evaluation_scope": args.scope,
        "score_model": {
            "overall_score": "0.6 * effectiveness_score + 0.3 * resource_score + 0.1 * stability_score",
            "resource_score": "0.75 * inverse_cpu_score + 0.25 * inverse_rss_score",
            "stability_score": "1.0 when workload drops and parse errors are both zero, else 0.0",
        },
        "rows": rows,
        "policy_comparison": comparison,
        "details": details,
    }
    (out_dir / "summary.json").write_text(json.dumps(summary, indent=2, sort_keys=True, ensure_ascii=False) + "\n")
    matrix_fields = [
        "scenario",
        "policy",
        "evaluation_scope",
        "effectiveness_score",
        "required_hit_rate",
        "required_event_hit_rate",
        "signal_hit_rate",
        "endpoint_signal_hit_rate",
        "cloud_signal_hit_rate",
        "incident_hit_rate",
        "negative_hit_rate",
        "observed_events",
        "observed_signals",
        "observed_incidents",
        "terminal_signal_count",
        "drop_rate",
        "parse_error_rate",
        "cost_per_1k_events_cpu",
        "out_of_scope_checks_total",
        "assert_pass",
        "assert_fail",
        "assert_skip",
        "events_path",
        "signals_path",
        "incidents_path",
    ]
    write_csv(out_dir / "matrix.csv", rows, matrix_fields)
    comparison_fields = [
        "policy",
        "overall_score",
        "effectiveness_score",
        "event_hit_rate",
        "signal_hit_rate",
        "resource_score",
        "stability_score",
        "workload_edr_cpu_avg_pct",
        "workload_edr_rss_avg_mb",
        "workload_eps_avg",
        "dropped_events_total",
        "parse_errors_total",
    ]
    write_csv(out_dir / "policy_comparison.csv", comparison, comparison_fields)
    (out_dir / "policy_comparison.json").write_text(json.dumps(comparison, indent=2, sort_keys=True) + "\n")


if __name__ == "__main__":
    main()
