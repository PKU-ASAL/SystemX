#!/usr/bin/env python3
"""Summarize scenario assertion results and recent perf/resource samples."""
import csv, glob, json, os, sys

ENVS = ["container", "vm-endpoint", "vm-topology"]
SCENARIOS = ["lifecycle-smoke", "apt-fileless-c2", "apt-staged-drop",
             "benign-ci-noise"]


def main():
    res = {}
    for f in glob.glob(".results/*.json"):
        if f.endswith(".stream.json"):
            continue
        d = json.load(open(f))
        env = d.get("env") or d.get("topology", "unknown")
        res[(env, d["scenario"])] = d
    if not res:
        sys.exit("no results under .results/ — run harness/run.sh first")

    print("\n==== SysArmor 测试通过矩阵 ====")
    print(f"{'env':<12} {'scenario':<20} {'pass':>5} {'fail':>5} {'skip':>5}  status")
    overall = 0
    for env in ENVS:
        for scenario in SCENARIOS:
            d = res.get((env, scenario))
            if not d:
                print(f"{env:<12} {scenario:<20} {'-':>5} {'-':>5} {'-':>5}  not-run")
                continue
            status = "OK" if d["fail"] == 0 else "FAIL"
            overall |= (1 if d["fail"] else 0)
            print(f"{env:<12} {scenario:<20} {d['pass']:>5} {d['fail']:>5} {d['skip']:>5}  {status}")
    for env in ENVS:
        perf = latest_perf(env)
        if not perf:
            print(f"{env:<12} {'perf-getevents':<20} {'-':>5} {'-':>5} {'-':>5}  not-run")
            continue
        print(f"{env:<12} {'perf-getevents':<20} {1:>5} {0:>5} {0:>5}  "
              f"OK eps={perf['eps']} rss_mb={perf['rss_mb']} dropped={perf['dropped_events']}")
    for env in ENVS:
        resource = latest_resource(env)
        if not resource:
            print(f"{env:<12} {'perf-resource':<20} {'-':>5} {'-':>5} {'-':>5}  not-run")
            continue
        print(f"{env:<12} {'perf-resource':<20} {1:>5} {0:>5} {0:>5}  "
              f"OK scenario={resource['scenario']} edr_cpu={resource['edr_cpu_pct']} "
              f"edr_rss_mb={resource['edr_rss_mb']}")
    for env in ENVS:
        for scenario in SCENARIOS:
            d = latest_stream(env, scenario)
            if not d:
                continue
            status = "OK" if d["fail"] == 0 else "FAIL"
            print(f"{env:<12} {(scenario + '-stream'):<20} {d['pass']:>5} {d['fail']:>5} {d['skip']:>5}  "
                  f"{status} events={d.get('stream_events', 0)} "
                  f"endpoint={d.get('stream_endpoint_signals', '-')} "
                  f"cloud={d.get('stream_cloud_signals', '-')} "
                  f"incidents={d.get('stream_incidents', '-')}")

    print("\nperf baseline: see .results/perf-getevents.<env>.csv")
    print("resource samples: see .results/perf-resource.<env>.<scenario>.csv")
    sys.exit(1 if overall else 0)


def latest_perf(env):
    path = f".results/perf-getevents.{env}.csv"
    if not os.path.exists(path):
        return None
    with open(path, newline="") as f:
        rows = list(csv.DictReader(f))
    return rows[-1] if rows else None


def latest_resource(env):
    paths = sorted(glob.glob(f".results/perf-resource.{env}.*.csv"))
    for path in reversed(paths):
        with open(path, newline="") as f:
            rows = list(csv.DictReader(f))
        if rows:
            return rows[-1]
    return None


def latest_stream(env, scenario):
    path = f".results/{env}.{scenario}.stream.json"
    if not os.path.exists(path):
        return None
    return json.load(open(path))


if __name__ == "__main__":
    main()
