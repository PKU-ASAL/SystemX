#!/usr/bin/env python3
import json
import sys
from collections import Counter
from pathlib import Path


def load_ndjson(path):
    rows = []
    p = Path(path)
    if not p.exists():
        return rows
    for line in p.read_text(errors="replace").splitlines():
        line = line.strip()
        if not line:
            continue
        rows.append(json.loads(line))
    return rows


def event_refs(signal):
    return signal.get("eventRefs") or signal.get("event_refs") or []


def signal_where(signal):
    return signal.get("where", "")


def signal_name(signal):
    return signal.get("name", "")


def is_endpoint_signal(signal):
    return signal_where(signal) == "SIGNAL_WHERE_ENDPOINT"


def is_attack_signal(signal, scenario):
    return (
        is_endpoint_signal(signal)
        and signal.get("scenario") == scenario
        and not str(signal_name(signal)).startswith("sensor_")
    )


def event_digest(event):
    return {
        "id": event.get("id"),
        "behavior": event.get("behavior"),
        "subject_proc": event.get("subjectProc") or event.get("subject_proc"),
        "object": event.get("object"),
        "lineage_id": event.get("lineageId") or event.get("lineage_id"),
        "raw_ref": event.get("rawRef") or event.get("raw_ref"),
    }


def linked_signal(signal, events_by_id, missing_refs):
    matched = []
    for ref in event_refs(signal):
        event = events_by_id.get(ref)
        if event is None:
            missing_refs.append({
                "signal_id": signal.get("id"),
                "signal_name": signal_name(signal),
                "event_ref": ref,
            })
            continue
        matched.append(event_digest(event))
    return {
        "signal_id": signal.get("id"),
        "signal_name": signal_name(signal),
        "where": signal_where(signal),
        "scenario": signal.get("scenario"),
        "base_risk": signal.get("baseRisk") or signal.get("base_risk"),
        "lineage_id": signal.get("lineageId") or signal.get("lineage_id"),
        "entities": signal.get("entities", []),
        "event_refs": event_refs(signal),
        "events": matched,
    }


def main():
    if len(sys.argv) != 6:
        raise SystemExit(
            "usage: local_signal_report.py <scenario> <events.ndjson> <signals.ndjson> <summary.json> <linked.json>"
        )
    scenario, events_path, signals_path, summary_path, linked_path = sys.argv[1:6]
    event_frames = load_ndjson(events_path)
    signal_frames = load_ndjson(signals_path)

    events = [frame.get("event", {}) for frame in event_frames]
    signals = [frame.get("signal", {}) for frame in signal_frames]
    events_by_id = {event.get("id"): event for event in events if event.get("id")}

    endpoint_signals = [sig for sig in signals if is_endpoint_signal(sig)]
    attack_signals = [sig for sig in endpoint_signals if is_attack_signal(sig, scenario)]
    linked_signals = []
    linked_attack_signals = []
    missing_refs = []
    for sig in signals:
        linked = linked_signal(sig, events_by_id, missing_refs)
        linked_signals.append(linked)
        if is_attack_signal(sig, scenario):
            linked_attack_signals.append(linked)

    signals_by_name = Counter(signal_name(sig) for sig in signals)
    attack_by_name = Counter(signal_name(sig) for sig in attack_signals)
    linked_with_refs = [sig for sig in linked_signals if sig["event_refs"]]
    linked_with_events = [sig for sig in linked_signals if sig["events"]]
    attack_with_refs = [sig for sig in linked_attack_signals if sig["event_refs"]]
    attack_with_events = [sig for sig in linked_attack_signals if sig["events"]]
    multi_event_signals = [sig for sig in linked_signals if len(sig["event_refs"]) > 1]
    multi_event_attack_signals = [sig for sig in linked_attack_signals if len(sig["event_refs"]) > 1]
    summary = {
        "topology": "vm",
        "scenario": scenario,
        "agent_mode": "local",
        "events": len(events),
        "signals": len(signals),
        "signals_by_name": dict(sorted(signals_by_name.items())),
        "endpoint_signals": len(endpoint_signals),
        "attack_signals": len(attack_signals),
        "attack_signals_by_name": dict(sorted(attack_by_name.items())),
        "signals_with_event_refs": len(linked_with_refs),
        "signals_with_resolved_events": len(linked_with_events),
        "linked_attack_signals": len(linked_attack_signals),
        "attack_signals_with_event_refs": len(attack_with_refs),
        "attack_signals_with_resolved_events": len(attack_with_events),
        "multi_event_signals": len(multi_event_signals),
        "multi_event_attack_signals": len(multi_event_attack_signals),
        "multi_event_attack_signal_names": dict(sorted(Counter(sig["signal_name"] for sig in multi_event_attack_signals).items())),
        "missing_event_refs": missing_refs,
        "pass": 1 if len(events) > 0 else 0,
        "fail": 0 if len(events) > 0 else 1,
        "skip": 0,
    }
    Path(summary_path).write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n")
    Path(linked_path).write_text(json.dumps({
        "signals": linked_signals,
        "attack_signals": linked_attack_signals,
    }, indent=2, sort_keys=True) + "\n")


if __name__ == "__main__":
    main()
