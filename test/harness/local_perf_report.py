#!/usr/bin/env python3
import csv
import json
import sys
from datetime import datetime, timezone
from pathlib import Path


def load_json(path):
    p = Path(path)
    if not p.exists():
        return {}
    return json.loads(p.read_text(errors="replace"))


def parse_time(value):
    if not value:
        return None
    return datetime.fromisoformat(value.replace("Z", "+00:00")).astimezone(timezone.utc)


def number(value):
    try:
        return float(value)
    except (TypeError, ValueError):
        return 0.0


def sensor_events(health):
    return int(health.get("sensor", {}).get("eventsSeen") or health.get("sensor", {}).get("events_seen") or 0)


def metric(rows, field):
    values = [number(row.get(field)) for row in rows if row.get(field) not in ("", None)]
    if not values:
        return {"avg": 0.0, "max": 0.0}
    return {
        "avg": round(sum(values) / len(values), 2),
        "max": round(max(values), 2),
    }


def main():
    if len(sys.argv) != 6:
        raise SystemExit("usage: local_perf_report.py <scenario> <health-before.json> <health-after.json> <perf.csv> <out.json>")
    scenario, before_path, after_path, perf_path, out_path = sys.argv[1:6]
    before = load_json(before_path)
    after = load_json(after_path)
    before_ts = parse_time(before.get("observedAt") or before.get("observed_at"))
    after_ts = parse_time(after.get("observedAt") or after.get("observed_at"))
    duration = (after_ts - before_ts).total_seconds() if before_ts and after_ts else 0.0
    event_delta = max(0, sensor_events(after) - sensor_events(before))
    rows = []
    p = Path(perf_path)
    if p.exists():
        with p.open(newline="") as f:
            for row in csv.DictReader(f):
                if row.get("scenario") == scenario:
                    rows.append(row)
    report = {
        "scenario": scenario,
        "duration_s": round(duration, 3),
        "events_delta": event_delta,
        "eps": round(event_delta / duration, 2) if duration > 0 else 0.0,
        "samples": len(rows),
        "edr_cpu_pct": metric(rows, "edr_cpu_pct"),
        "edr_rss_mb": metric(rows, "edr_rss_mb"),
        "agent_cpu_pct": metric(rows, "agent_cpu_pct"),
        "agent_rss_mb": metric(rows, "agent_rss_mb"),
        "tetragon_cpu_pct": metric(rows, "tetragon_cpu_pct"),
        "tetragon_rss_mb": metric(rows, "tetragon_rss_mb"),
        "tetra_cpu_pct": metric(rows, "tetra_cpu_pct"),
        "tetra_rss_mb": metric(rows, "tetra_rss_mb"),
        "workload_cpu_pct": metric(rows, "workload_cpu_pct"),
        "workload_rss_mb": metric(rows, "workload_rss_mb"),
        "dropped_events_after": int(after.get("sensor", {}).get("eventsDropped") or after.get("sensor", {}).get("events_dropped") or 0),
        "parse_errors_after": int(after.get("sensor", {}).get("parseErrors") or after.get("sensor", {}).get("parse_errors") or 0),
    }
    Path(out_path).write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")


if __name__ == "__main__":
    main()
