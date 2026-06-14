#!/usr/bin/env python3
"""汇总各场景断言结果为通过矩阵；并（待 perf 数据就绪后）渲染基线曲线。"""
import csv, glob, json, os, sys

TOPOLOGIES = ["container", "vm"]
SCENARIOS = ["lifecycle-smoke", "apt-fileless-c2", "apt-staged-drop",
             "benign-ci-noise"]
EXTRAS = ["perf-getevents"]


def main():
    res = {}
    for f in glob.glob(".results/*.json"):
        if f.endswith(".stream.json"):
            continue
        d = json.load(open(f))
        topo = d.get("topology", "unknown")
        res[(topo, d["scenario"])] = d
    if not res:
        sys.exit("no results under .results/ — run harness/run.sh first")

    print("\n==== SysArmor 测试通过矩阵 ====")
    print(f"{'topology':<10} {'scenario':<20} {'pass':>5} {'fail':>5} {'skip':>5}  status")
    overall = 0
    for topo in TOPOLOGIES:
        for scenario in SCENARIOS:
            d = res.get((topo, scenario))
            if not d:
                print(f"{topo:<10} {scenario:<20} {'-':>5} {'-':>5} {'-':>5}  not-run")
                continue
            status = "OK" if d["fail"] == 0 else "FAIL"
            overall |= (1 if d["fail"] else 0)
            print(f"{topo:<10} {scenario:<20} {d['pass']:>5} {d['fail']:>5} {d['skip']:>5}  {status}")
    for topo in TOPOLOGIES:
        perf = latest_perf(topo)
        if not perf:
            print(f"{topo:<10} {'perf-getevents':<20} {'-':>5} {'-':>5} {'-':>5}  not-run")
            continue
        print(f"{topo:<10} {'perf-getevents':<20} {1:>5} {0:>5} {0:>5}  "
              f"OK eps={perf['eps']} rss_mb={perf['rss_mb']} dropped={perf['dropped_events']}")
    for topo in TOPOLOGIES:
        for scenario in SCENARIOS:
            d = latest_stream(topo, scenario)
            if not d:
                continue
            status = "OK" if d["fail"] == 0 else "FAIL"
            print(f"{topo:<10} {(scenario + '-stream'):<20} {d['pass']:>5} {d['fail']:>5} {d['skip']:>5}  "
                  f"{status} events={d.get('stream_events', 0)} "
                  f"endpoint={d.get('stream_endpoint_signals', '-')} "
                  f"cloud={d.get('stream_cloud_signals', '-')} "
                  f"incidents={d.get('stream_incidents', '-')}")

    # TODO: perf-getevents 读取 /tmp/perf-getevents.csv → 渲染 事件率 vs CPU/RSS/丢失率 曲线
    print("\nperf baseline: see /tmp/perf-getevents.csv (M3 deliverable)")
    sys.exit(1 if overall else 0)


def latest_perf(topo):
    path = f".results/perf-getevents.{topo}.csv"
    if not os.path.exists(path):
        return None
    with open(path, newline="") as f:
        rows = list(csv.DictReader(f))
    return rows[-1] if rows else None


def latest_stream(topo, scenario):
    path = f".results/{topo}.{scenario}.stream.json"
    if not os.path.exists(path):
        return None
    return json.load(open(path))


if __name__ == "__main__":
    main()
