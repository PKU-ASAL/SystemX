#!/usr/bin/env python3
import csv
import json
import sys
from datetime import datetime, timezone
from pathlib import Path


def number(value):
    try:
        return float(value)
    except (TypeError, ValueError):
        return 0.0


def parse_time(value):
    if not value:
        return None
    try:
        return datetime.fromisoformat(str(value).replace("Z", "+00:00")).astimezone(timezone.utc)
    except ValueError:
        return None


def load_markers(path):
    markers = []
    p = Path(path)
    if not p.exists():
        return markers
    for line in p.read_text(errors="replace").splitlines():
        if not line.strip():
            continue
        try:
            markers.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    return markers


def load_ndjson(path):
    rows = []
    p = Path(path)
    if not p.exists():
        return rows
    for line in p.read_text(errors="replace").splitlines():
        if not line.strip():
            continue
        try:
            rows.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    return rows


def read_text(path):
    p = Path(path)
    if not p.exists():
        return ""
    return p.read_text(errors="replace").strip()


def tail_lines(value, limit=20):
    lines = [line for line in value.splitlines() if line.strip()]
    if len(lines) <= limit:
        return lines
    return lines[-limit:]


def frame_time(frame):
    ts = parse_time(frame.get("observedAt") or frame.get("observed_at"))
    if ts:
        return ts
    body = frame.get("event") or frame.get("signal") or {}
    if isinstance(body, dict):
        return parse_time(body.get("observedAt") or body.get("observed_at"))
    return None


def phase_bounds(markers):
    starts = []
    for marker in markers:
        phase = marker.get("phase", "")
        if phase.endswith("_start"):
            ts = parse_time(marker.get("ts"))
            if ts:
                starts.append((phase[:-6], ts))
    explicit_ends = {}
    for marker in markers:
        phase = marker.get("phase", "")
        if phase.endswith("_done"):
            ts = parse_time(marker.get("ts"))
            if ts:
                explicit_ends[phase[:-5]] = ts
    recorder_stop = next((parse_time(m.get("ts")) for m in markers if m.get("phase") == "recorder_stop"), None)
    bounds = {}
    for idx, (name, start) in enumerate(starts):
        end = explicit_ends.get(name)
        if not end and idx + 1 < len(starts):
            end = starts[idx + 1][1]
        if not end:
            end = recorder_stop
        if end and end > start:
            bounds[name] = (start, end)
    return bounds


def summarize(rows, duration_s=None):
    def values(field):
        return [number(row.get(field)) for row in rows if row.get(field) not in ("", None)]

    def metric(field):
        vals = values(field)
        if not vals:
            return {"avg": 0.0, "max": 0.0}
        return {"avg": round(sum(vals) / len(vals), 2), "max": round(max(vals), 2)}

    def sample_delta(field):
        vals = values(field)
        if not vals:
            return 0
        return int(max(vals) - min(vals))

    duration = number(duration_s)
    if duration <= 0 and rows:
        duration = number(rows[-1].get("elapsed_s")) - number(rows[0].get("elapsed_s"))
    events_delta = sample_delta("events_scoped") if any(row.get("events_scoped") not in ("", None) for row in rows) else sample_delta("events_seen")
    signals_delta = sample_delta("signals_scoped") if any(row.get("signals_scoped") not in ("", None) for row in rows) else sample_delta("signals")
    return {
        "duration_s": round(duration, 3),
        "samples": len(rows),
        "events_delta": events_delta,
        "eps": round(events_delta / duration, 2) if duration > 0 else 0.0,
        "signals_delta": signals_delta,
        "dropped_events_delta": sample_delta("dropped_events"),
        "parse_errors_delta": sample_delta("parse_errors"),
        "agent_cpu_pct": metric("agent_cpu_pct"),
        "agent_rss_mb": metric("agent_rss_mb"),
        "sensor_cpu_pct": metric("sensor_cpu_pct"),
        "sensor_rss_mb": metric("sensor_rss_mb"),
        "edr_cpu_pct": metric("edr_cpu_pct"),
        "edr_rss_mb": metric("edr_rss_mb"),
    }


def phase_counter_delta(rows, start, end, scoped_field, fallback_field):
    field = scoped_field if any(row.get(scoped_field) not in ("", None) for row in rows) else fallback_field
    if not field:
        return 0
    before = None
    at_end = None
    for row in rows:
        ts = parse_time(row.get("ts"))
        if not ts:
            continue
        value = number(row.get(field))
        if ts < start:
            before = value
        if ts <= end:
            at_end = value
    if at_end is None:
        return 0
    if before is None:
        before = 0
    return max(0, int(at_end - before))


def count_frames(frames, start, end):
    total = 0
    for frame in frames:
        ts = frame_time(frame)
        if ts and start <= ts < end:
            total += 1
    return total


def summarize_phase(all_rows, phase_rows, start, end, event_frames=None, signal_frames=None):
    summary = summarize(phase_rows, (end - start).total_seconds())
    if event_frames:
        events_delta = count_frames(event_frames, start, end)
    else:
        events_delta = phase_counter_delta(all_rows, start, end, "events_scoped", "events_seen")
    if signal_frames:
        signals_delta = count_frames(signal_frames, start, end)
    else:
        signals_delta = phase_counter_delta(all_rows, start, end, "signals_scoped", "signals")
    summary["events_delta"] = events_delta
    summary["eps"] = round(events_delta / summary["duration_s"], 2) if summary["duration_s"] > 0 else 0.0
    summary["signals_delta"] = signals_delta
    return summary


def main():
    if len(sys.argv) != 2:
        raise SystemExit("usage: bench_lifecycle_report.py <run-dir>")
    out_dir = Path(sys.argv[1])
    timeline = out_dir / "timeline.csv"
    markers = load_markers(out_dir / "markers.ndjson")
    event_frames = load_ndjson(out_dir / "events.ndjson")
    event_all_frames = load_ndjson(out_dir / "events-all.ndjson")
    signal_frames = load_ndjson(out_dir / "signals.ndjson")
    signal_all_frames = load_ndjson(out_dir / "signals-all.ndjson")
    rows = []
    if timeline.exists():
        with timeline.open(newline="") as f:
            rows = list(csv.DictReader(f))
    bounds = phase_bounds(markers)
    phases = {}
    for name, (start, end) in bounds.items():
        phase_rows = []
        for row in rows:
            ts = parse_time(row.get("ts"))
            if ts and start <= ts < end:
                phase_rows.append(row)
        phases[name] = summarize_phase(rows, phase_rows, start, end, event_frames, signal_frames)
    summary = {
        "markers": markers,
        "phases": phases,
        "overall": summarize(rows),
        "scoped_events_total": len(event_frames),
        "events_seen_since_cursor_total": len(event_all_frames),
        "scoped_signals_total": len(signal_frames),
        "signals_seen_total": len(signal_all_frames),
        "diagnostics": {
            "event_watch_errors": tail_lines(read_text(out_dir / "event-watch.err")),
            "event_all_watch_errors": tail_lines(read_text(out_dir / "event-all-watch.err")),
            "signal_watch_errors": tail_lines(read_text(out_dir / "signal-watch.err")),
            "signal_all_watch_errors": tail_lines(read_text(out_dir / "signal-all-watch.err")),
        },
    }
    (out_dir / "summary.json").write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n")


if __name__ == "__main__":
    main()
