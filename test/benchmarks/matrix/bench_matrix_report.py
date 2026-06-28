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


def load_csv(path):
    p = Path(path)
    if not p.exists() or p.stat().st_size == 0:
        return []
    with p.open(newline="") as f:
        return list(csv.DictReader(f))


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: bench_matrix_report.py <bench-matrix-dir>")
    out_dir = Path(sys.argv[1])
    rows = []
    cases_dir = out_dir / "cases"
    for case_dir in sorted(p for p in cases_dir.iterdir() if p.is_dir()) if cases_dir.exists() else []:
        status = load_json(case_dir / "status.json")
        bench_run_id = status.get("bench_run_id", "")
        source = out_dir.parents[1] / "bench-collection-vm" / bench_run_id / "matrix.csv"
        matrix_rows = load_csv(source)
        variant = status.get("variant", "")
        matcher_strategy = status.get("matcher_strategy", "")
        workload = status.get("workload", "")
        scenario = status.get("scenario", "")
        if not matrix_rows:
            rows.append({
                "name": status.get("name", case_dir.name),
                "variant": variant,
                "matcher_strategy": matcher_strategy,
                "workload": workload,
                "scenario": scenario,
                "status": status.get("status", "unknown"),
                "bench_run_id": bench_run_id,
                "policy_dir": "",
            })
            continue
        for row in matrix_rows:
            if not isinstance(row, dict):
                continue
            merged = {
                "name": status.get("name", case_dir.name),
                "variant": variant,
                "matcher_strategy": matcher_strategy,
                "workload": workload,
                "scenario": scenario,
                "status": status.get("status", "unknown"),
                "bench_run_id": bench_run_id,
            }
            merged.update(row)
            rows.append(merged)

    deprecated_matrix_json = out_dir / "matrix.json"
    if deprecated_matrix_json.exists():
        deprecated_matrix_json.unlink()
    fields = []
    for base in ("name", "variant", "matcher_strategy", "workload", "scenario", "status", "bench_run_id", "policy_dir", "policy_id", "policy_version"):
        if any(base in row for row in rows):
            fields.append(base)
    extra = sorted({key for row in rows for key in row.keys()} - set(fields))
    fields.extend(extra)
    with (out_dir / "matrix.csv").open("w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=fields)
        writer.writeheader()
        for row in rows:
            writer.writerow(row)


if __name__ == "__main__":
    main()
