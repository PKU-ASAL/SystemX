#!/usr/bin/env python3
"""Generate minimal SensorEvent protojson lines for Phase 1 MVP replay.

This is a bridge while the Tetragon JSON adapter is being completed: capture still
stores raw Tetragon output, then this script emits contract-level SensorEvent
facts that exercise the real agent normalize/fastpath/uploader path.
"""
import argparse
import json


def proc(pid, ppid, binary, argv=None, start=0):
    return {
        "pid": pid,
        "ppid": ppid,
        "binary": binary,
        "argv": argv or [binary],
        "uid": 0,
        "start_time_ns": start or pid * 1000,
    }


def ev(kind, pid, ppid, binary, obj=None, argv=None, idx=0):
    return {
        "mono_ns": idx * 1000000,
        "kind": kind,
        "proc": proc(pid, ppid, binary, argv, idx * 1000 + pid),
        "object": obj or {},
        "raw_ref": f"raw-{idx}",
    }


def apt_fileless_c2():
    return [
        ev("EVENT_KIND_EXEC", 100, 0, "/usr/bin/java-web", argv=["/usr/bin/java-web"], idx=1),
        ev("EVENT_KIND_EXEC", 101, 100, "/bin/bash", argv=["/bin/bash", "-c"], idx=2),
        ev("EVENT_KIND_CONNECT", 102, 101, "/usr/bin/curl", {"dst": "10.66.0.99:8080"}, ["/usr/bin/curl"], idx=3),
        ev("EVENT_KIND_WRITE", 102, 101, "/usr/bin/curl", {"path": "/dev/shm/x.sh"}, ["/usr/bin/curl"], idx=4),
        ev("EVENT_KIND_EXEC", 103, 101, "/bin/bash", argv=["/bin/bash", "/dev/shm/x.sh"], idx=5),
        ev("EVENT_KIND_CONNECT", 104, 103, "/bin/bash", {"dst": "10.66.0.99:443"}, ["/bin/bash", "-i"], idx=6),
        ev("EVENT_KIND_OPEN", 105, 104, "/bin/cat", {"path": "/root/.ssh/id_rsa"}, ["/bin/cat"], idx=7),
    ]


def apt_staged_drop():
    return [
        ev("EVENT_KIND_EXEC", 200, 0, "/bin/bash", argv=["/bin/bash", "-c"], idx=1),
        ev("EVENT_KIND_CONNECT", 201, 200, "/usr/bin/curl", {"dst": "10.66.0.99:8080"}, ["/usr/bin/curl"], idx=2),
        ev("EVENT_KIND_WRITE", 201, 200, "/usr/bin/curl", {"path": "/var/lib/app/plugins/helper"}, ["/usr/bin/curl"], idx=3),
        ev("EVENT_KIND_EXEC", 300, 0, "/var/lib/app/plugins/helper", argv=["/var/lib/app/plugins/helper", "--report"], idx=20),
        ev("EVENT_KIND_CONNECT", 301, 300, "/bin/bash", {"dst": "10.66.0.99:443"}, ["/bin/bash"], idx=21),
    ]


def benign_ci_noise():
    rows = []
    for i in range(3):
        base = 400 + i * 10
        rows.extend([
            ev("EVENT_KIND_EXEC", base, 0, "/bin/bash", argv=["/bin/bash", "/usr/local/bin/build.sh"], idx=base),
            ev("EVENT_KIND_CONNECT", base + 1, base, "/usr/bin/curl", {"dst": "10.66.0.99:8080"}, ["/usr/bin/curl"], idx=base + 1),
            ev("EVENT_KIND_WRITE", base + 1, base, "/usr/bin/curl", {"path": f"/tmp/ci-artifact-{i}"}, ["/usr/bin/curl"], idx=base + 2),
        ])
    return rows


def lifecycle_smoke():
    return [
        ev("EVENT_KIND_EXEC", 500, 0, "/usr/bin/env", argv=["/usr/bin/env", "true"], idx=1),
    ]


SCENARIOS = {
    "apt-fileless-c2": apt_fileless_c2,
    "apt-staged-drop": apt_staged_drop,
    "benign-ci-noise": benign_ci_noise,
    "lifecycle-smoke": lifecycle_smoke,
}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--scenario", required=True, choices=sorted(SCENARIOS))
    ap.add_argument("--out", required=True)
    args = ap.parse_args()
    with open(args.out, "w") as f:
        for row in SCENARIOS[args.scenario]():
            f.write(json.dumps(row, separators=(",", ":")) + "\n")


if __name__ == "__main__":
    main()
