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


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: bench_matrix_report.py <bench-matrix-dir>")
    out_dir = Path(sys.argv[1])
    rows = []
    cases_dir = out_dir / "cases"
    for case_dir in sorted(p for p in cases_dir.iterdir() if p.is_dir()) if cases_dir.exists() else []:
        status = load_json(case_dir / "status.json")
        bench_run_id = status.get("bench_run_id", "")
        source = out_dir.parents[1] / "bench-collection-vm" / bench_run_id / "matrix.json"
        matrix_rows = load_json(source)
        if not isinstance(matrix_rows, list):
            matrix_rows = []
        workload = status.get("workload", "")
        scenario = status.get("scenario", "")
        if not matrix_rows:
            rows.append({
                "name": status.get("name", case_dir.name),
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
                "workload": workload,
                "scenario": scenario,
                "status": status.get("status", "unknown"),
                "bench_run_id": bench_run_id,
            }
            merged.update(row)
            rows.append(merged)

    (out_dir / "matrix.json").write_text(json.dumps(rows, indent=2, sort_keys=True) + "\n")
    fields = []
    for base in ("name", "workload", "scenario", "status", "bench_run_id", "policy_dir", "policy_id", "policy_version"):
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
