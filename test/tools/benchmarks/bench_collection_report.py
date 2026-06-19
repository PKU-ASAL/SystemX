#!/usr/bin/env python3
import csv
import json
import sys
from pathlib import Path


def load_json(path):
    p = Path(path)
    if not p.exists() or p.stat().st_size == 0:
        return {}
    try:
        return json.loads(p.read_text(errors="replace"))
    except json.JSONDecodeError:
        return {}


def number(value):
    try:
        return float(value)
    except (TypeError, ValueError):
        return 0.0


def metric(phase, name, field):
    value = phase.get(name, {})
    if not isinstance(value, dict):
        value = {}
    return number(value.get(field))


def phase(summary, name):
    phases = summary.get("phases", {})
    value = phases.get(name, {})
    return value if isinstance(value, dict) else {}


def marker_detail(summary, marker_name):
    for marker in summary.get("markers", []):
        if marker.get("phase") == marker_name:
            return marker.get("detail", "")
    return ""


def apply_report(path):
    data = load_json(path)
    policy_id = data.get("policyId") or data.get("policy_id") or ""
    policy_version = data.get("policyVersion") or data.get("policy_version") or ""
    report = data.get("reportJson") or data.get("report_json")
    resolved_refs = 0
    generated_policy_hash = ""
    if isinstance(report, str) and report:
        try:
            report = json.loads(report)
        except json.JSONDecodeError:
            report = {}
    if isinstance(report, dict):
        refs = report.get("resolved_refs") or report.get("resolvedRefs") or []
        if isinstance(refs, list):
            resolved_refs = len(refs)
        generated_policy_hash = report.get("generated_policy_hash") or report.get("generatedPolicyHash") or ""
    return policy_id, policy_version, generated_policy_hash, resolved_refs


def phase_fields(prefix, data):
    return {
        f"{prefix}_duration_s": number(data.get("duration_s")),
        f"{prefix}_samples": int(number(data.get("samples"))),
        f"{prefix}_events_delta": int(number(data.get("events_delta"))),
        f"{prefix}_eps": number(data.get("eps")),
        f"{prefix}_signals_delta": int(number(data.get("signals_delta"))),
        f"{prefix}_dropped_events_delta": int(number(data.get("dropped_events_delta"))),
        f"{prefix}_parse_errors_delta": int(number(data.get("parse_errors_delta"))),
        f"{prefix}_agent_cpu_avg_pct": metric(data, "agent_cpu_pct", "avg"),
        f"{prefix}_agent_cpu_max_pct": metric(data, "agent_cpu_pct", "max"),
        f"{prefix}_agent_rss_avg_mb": metric(data, "agent_rss_mb", "avg"),
        f"{prefix}_agent_rss_max_mb": metric(data, "agent_rss_mb", "max"),
        f"{prefix}_sensor_cpu_avg_pct": metric(data, "sensor_cpu_pct", "avg"),
        f"{prefix}_sensor_cpu_max_pct": metric(data, "sensor_cpu_pct", "max"),
        f"{prefix}_sensor_rss_avg_mb": metric(data, "sensor_rss_mb", "avg"),
        f"{prefix}_sensor_rss_max_mb": metric(data, "sensor_rss_mb", "max"),
        f"{prefix}_edr_cpu_avg_pct": metric(data, "edr_cpu_pct", "avg"),
        f"{prefix}_edr_cpu_max_pct": metric(data, "edr_cpu_pct", "max"),
        f"{prefix}_edr_rss_avg_mb": metric(data, "edr_rss_mb", "avg"),
        f"{prefix}_edr_rss_max_mb": metric(data, "edr_rss_mb", "max"),
    }


def build_row(policy_dir):
    summary = load_json(policy_dir / "summary.json")
    policy_id, policy_version, policy_hash, resolved_refs = apply_report(policy_dir / "collection-apply.json")
    row = {
        "policy_dir": policy_dir.name,
        "policy_id": policy_id,
        "policy_version": policy_version,
        "generated_policy_hash": policy_hash,
        "resolved_refs": resolved_refs,
        "workload": marker_detail(summary, "workload_start"),
    }
    for name in ("baseline", "policy_apply", "settle", "steady", "workload"):
        row.update(phase_fields(name, phase(summary, name)))
    return row


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: bench_collection_report.py <bench-run-dir>")
    out_dir = Path(sys.argv[1])
    rows = []
    for child in sorted(out_dir.iterdir()):
        if child.is_dir() and (child / "collection-apply.json").exists():
            rows.append(build_row(child))
    matrix_json = out_dir / "matrix.json"
    matrix_csv = out_dir / "matrix.csv"
    matrix_json.write_text(json.dumps(rows, indent=2, sort_keys=True) + "\n")
    fields = [
        "policy_dir",
        "policy_id",
        "policy_version",
        "generated_policy_hash",
        "resolved_refs",
        "workload",
    ]
    for name in ("baseline", "policy_apply", "settle", "steady", "workload"):
        fields.extend(phase_fields(name, {}).keys())
    with matrix_csv.open("w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=fields)
        writer.writeheader()
        for row in rows:
            writer.writerow(row)


if __name__ == "__main__":
    main()
