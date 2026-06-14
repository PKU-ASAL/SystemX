#!/usr/bin/env python3
"""汇总各场景断言结果为通过矩阵；并（待 perf 数据就绪后）渲染基线曲线。"""
import glob, json, os, sys

ORDER = ["lifecycle-smoke", "apt-fileless-c2", "apt-staged-drop",
         "benign-ci-noise", "perf-getevents"]


def main():
    res = {}
    for f in glob.glob(".results/*.json"):
        d = json.load(open(f)); res[d["scenario"]] = d
    if not res:
        sys.exit("no results under .results/ — run harness/run.sh first")

    print("\n==== SysArmor 测试通过矩阵 ====")
    print(f"{'scenario':<20} {'pass':>5} {'fail':>5} {'skip':>5}  status")
    overall = 0
    for s in ORDER:
        d = res.get(s)
        if not d:
            print(f"{s:<20} {'-':>5} {'-':>5} {'-':>5}  not-run"); continue
        status = "OK" if d["fail"] == 0 else "FAIL"
        overall |= (1 if d["fail"] else 0)
        print(f"{s:<20} {d['pass']:>5} {d['fail']:>5} {d['skip']:>5}  {status}")

    # TODO: perf-getevents 读取 /tmp/perf-getevents.csv → 渲染 事件率 vs CPU/RSS/丢失率 曲线
    print("\nperf baseline: see /tmp/perf-getevents.csv (M3 deliverable)")
    sys.exit(1 if overall else 0)


if __name__ == "__main__":
    main()
