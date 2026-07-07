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


def write_ndjson(path, frames):
    with Path(path).open("w") as f:
        for frame in frames:
            f.write(json.dumps(frame, separators=(",", ":"), sort_keys=True) + "\n")


def read_text(path):
    p = Path(path)
    if not p.exists():
        return ""
    return p.read_text(errors="replace").strip()


def load_json(path):
    p = Path(path)
    if not p.exists() or p.stat().st_size == 0:
        return {}
    try:
        data = json.loads(p.read_text(errors="replace"))
    except json.JSONDecodeError:
        return {}
    return data if isinstance(data, dict) else {}


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


def frame_labels(frame):
    labels = {}
    body = frame.get("event") or frame.get("signal") or {}
    if isinstance(body, dict) and isinstance(body.get("labels"), dict):
        labels.update(body.get("labels"))
    return labels


def marker_detail(markers, marker_name):
    for marker in markers:
        if marker.get("phase") == marker_name:
            return marker.get("detail", "")
    return ""


def scope_filters(out_dir, markers):
    labels = load_json(out_dir / "scope-labels.json")
    if labels:
        return {str(key): str(value) for key, value in labels.items() if str(key)}
    filters = {}
    workload = marker_detail(markers, "workload_start")
    scenario = marker_detail(markers, "scenario_start")
    if workload and workload != "none":
        filters["workload"] = workload
    if scenario and scenario != "none":
        filters["scenario"] = scenario
    for marker in markers:
        if marker.get("phase") == "recorder_start":
            continue
    return filters


def scope_frames_for_dir(frames, out_dir, markers):
    filters = scope_filters(out_dir, markers)
    if not filters:
        return list(frames)
    scoped = []
    for frame in frames:
        labels = frame_labels(frame)
        if all(labels.get(key) == value for key, value in filters.items()):
            scoped.append(frame)
    return scoped


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


def marker_time(markers, marker_name):
    for marker in markers:
        if marker.get("phase") == marker_name:
            return parse_time(marker.get("ts"))
    return None


def valid_interval(start, end):
    return start and end and end > start


def subtract_intervals(base_intervals, remove_intervals):
    result = []
    for base_start, base_end in base_intervals:
        pieces = [(base_start, base_end)]
        for rem_start, rem_end in remove_intervals:
            if not valid_interval(rem_start, rem_end):
                continue
            next_pieces = []
            for piece_start, piece_end in pieces:
                if rem_end <= piece_start or rem_start >= piece_end:
                    next_pieces.append((piece_start, piece_end))
                    continue
                if rem_start > piece_start:
                    next_pieces.append((piece_start, min(rem_start, piece_end)))
                if rem_end < piece_end:
                    next_pieces.append((max(rem_end, piece_start), piece_end))
            pieces = next_pieces
        result.extend((start, end) for start, end in pieces if valid_interval(start, end))
    return result


def standard_phase_intervals(markers, raw_bounds):
    recorder_start = marker_time(markers, "recorder_start")
    recorder_stop = marker_time(markers, "recorder_stop")
    steady_start = marker_time(markers, "steady_start")
    workload_start = marker_time(markers, "workload_start")
    workload_done = marker_time(markers, "workload_done") or recorder_stop
    activity_start = marker_time(markers, "scenario_start")
    activity_done = marker_time(markers, "scenario_done")
    persistence_start = marker_time(markers, "scenario_observe_start")
    persistence_done = marker_time(markers, "scenario_observe_done")

    first_start = None
    if raw_bounds:
        first_start = min(start for start, _ in raw_bounds.values())
    startup_start = recorder_start or first_start
    startup_end = steady_start or workload_start or recorder_stop

    phases = {}
    if valid_interval(startup_start, startup_end):
        phases["startup"] = [(startup_start, startup_end)]
    if valid_interval(steady_start, workload_start):
        phases["steady"] = [(steady_start, workload_start)]

    activity = []
    if valid_interval(activity_start, activity_done):
        activity = [(activity_start, activity_done)]
        phases["activity"] = activity

    persistence = []
    if valid_interval(persistence_start, persistence_done):
        persistence = [(persistence_start, persistence_done)]
        phases["persistence"] = persistence

    workload_total = []
    if valid_interval(workload_start, workload_done):
        workload_total = [(workload_start, workload_done)]
    workload_only = subtract_intervals(workload_total, activity + persistence)
    if workload_only:
        phases["workload"] = workload_only

    return phases


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
    events_delta = sample_delta("events_captured") if any(row.get("events_captured") not in ("", None) for row in rows) else sample_delta("events_seen")
    signals_delta = sample_delta("signals_captured") if any(row.get("signals_captured") not in ("", None) for row in rows) else sample_delta("signals_seen_since_cursor")
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


def interval_duration_s(intervals):
    return sum((end - start).total_seconds() for start, end in intervals)


def row_in_intervals(row, intervals):
    ts = parse_time(row.get("ts"))
    return bool(ts) and any(start <= ts < end for start, end in intervals)


def count_frames_intervals(frames, intervals):
    total = 0
    for frame in frames:
        ts = frame_time(frame)
        if ts and any(start <= ts < end for start, end in intervals):
            total += 1
    return total


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
        events_delta = phase_counter_delta(all_rows, start, end, "events_captured", "events_seen")
    if signal_frames:
        signals_delta = count_frames(signal_frames, start, end)
    else:
        signals_delta = phase_counter_delta(all_rows, start, end, "signals_captured", "signals_seen_since_cursor")
    summary["events_delta"] = events_delta
    summary["eps"] = round(events_delta / summary["duration_s"], 2) if summary["duration_s"] > 0 else 0.0
    summary["signals_delta"] = signals_delta
    return summary


def summarize_intervals(all_rows, intervals, event_frames=None, signal_frames=None):
    phase_rows = [row for row in all_rows if row_in_intervals(row, intervals)]
    summary = summarize(phase_rows, interval_duration_s(intervals))
    if event_frames:
        events_delta = count_frames_intervals(event_frames, intervals)
    else:
        events_delta = sum(phase_counter_delta(all_rows, start, end, "events_captured", "events_seen") for start, end in intervals)
    if signal_frames:
        signals_delta = count_frames_intervals(signal_frames, intervals)
    else:
        signals_delta = sum(phase_counter_delta(all_rows, start, end, "signals_captured", "signals_seen_since_cursor") for start, end in intervals)
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
    signal_frames = load_ndjson(out_dir / "signals.ndjson")
    event_scope_frames = scope_frames_for_dir(event_frames, out_dir, markers)
    signal_scope_frames = scope_frames_for_dir(signal_frames, out_dir, markers)
    write_ndjson(out_dir / "events.scope.ndjson", event_scope_frames)
    write_ndjson(out_dir / "signals.scope.ndjson", signal_scope_frames)
    deprecated_signal_scope = out_dir / "signal.scope.ndjson"
    if deprecated_signal_scope.exists():
        deprecated_signal_scope.unlink()
    rows = []
    if timeline.exists():
        with timeline.open(newline="") as f:
            rows = list(csv.DictReader(f))
    raw_bounds = phase_bounds(markers)
    raw_phases = {}
    for name, (start, end) in raw_bounds.items():
        phase_rows = []
        for row in rows:
            ts = parse_time(row.get("ts"))
            if ts and start <= ts < end:
                phase_rows.append(row)
        raw_phases[name] = summarize_phase(rows, phase_rows, start, end, event_scope_frames, signal_scope_frames)
    phases = {}
    for name, intervals in standard_phase_intervals(markers, raw_bounds).items():
        phases[name] = summarize_intervals(rows, intervals, event_scope_frames, signal_scope_frames)
    overall = summarize(rows)
    phases["overall"] = overall
    summary = {
        "markers": markers,
        "phases": phases,
        "raw_phases": raw_phases,
        "overall": overall,
        "scoped_events_total": len(event_scope_frames),
        "events_seen_since_cursor_total": len(event_frames),
        "scoped_signals_total": len(signal_scope_frames),
        "signals_seen_total": len(signal_frames),
        "diagnostics": {
            "event_watch_errors": tail_lines(read_text(out_dir / "event-watch.err")),
            "signal_watch_errors": tail_lines(read_text(out_dir / "signal-watch.err")),
        },
    }
    (out_dir / "summary.json").write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n")


if __name__ == "__main__":
    main()
