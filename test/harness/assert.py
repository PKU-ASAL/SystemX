#!/usr/bin/env python3
"""读 expected.yaml，对 sysarmorctl --json 输出逐条断言。

当前为骨架：sysarmorctl 尚未构建时以 DRY-RUN 跑通断言逻辑（打印将要校验的契约），
binary 就绪后把 _query() 接到真实 CLI 即可。
断言类型见 design-test-cases.md §5.3：正向存在 / 结构 / 契约完整性 / 负向缺失 / 对照。
"""
import argparse, json, os, shutil, subprocess, sys

try:
    import yaml
except ImportError:
    sys.exit("need pyyaml: pip install pyyaml")

DRY = shutil.which("sysarmorctl") is None


def _query(mgr, *args):
    """调 sysarmorctl 取 JSON；DRY-RUN 下返回 None。"""
    if DRY:
        print(f"  [dry-run] sysarmorctl --mgr {mgr} {' '.join(args)} --json")
        return None
    out = subprocess.check_output(["sysarmorctl", "--mgr", mgr, *args, "--json"])
    return json.loads(out)


class Result:
    def __init__(self): self.passed, self.failed, self.skipped = 0, 0, 0
    def check(self, name, cond):
        if cond is None:
            self.skipped += 1; print(f"  ~ SKIP {name} (dry-run)")
        elif cond:
            self.passed += 1; print(f"  ✓ PASS {name}")
        else:
            self.failed += 1; print(f"  ✗ FAIL {name}")


def assert_incident(exp, mgr, scenario, r):
    inc = exp.get("incident")
    if not inc:
        return
    data = _query(mgr, "incidents", "--scenario", scenario)
    if "count" in inc:
        r.check(f"incident.count=={inc['count']}",
                None if data is None else len(data.get("incidents", [])) == inc["count"])
    if data and inc.get("lineage_ids_min"):
        ok = any(len(i.get("lineage_ids", [])) >= inc["lineage_ids_min"]
                 for i in data.get("incidents", []))
        r.check(f"incident.lineage_ids>={inc['lineage_ids_min']}", ok)
    if "converge_method" in inc:
        r.check("incident.converge_method", None if data is None else any(
            i.get("converge", {}).get("method") == inc["converge_method"]
            for i in data.get("incidents", [])))


def assert_signals(exp, mgr, scenario, r):
    for layer in ("endpoint_signals", "cloud_signals"):
        spec = exp.get(layer)
        if not spec:
            continue
        data = _query(mgr, "signals", "--scenario", scenario, "--layer", layer.split("_")[0])
        names = [] if data is None else [s.get("name") for s in data]
        for want in spec.get("must_contain", []):
            nm = want["name"] if isinstance(want, dict) else want
            r.check(f"{layer}.must_contain[{nm}]", None if data is None else nm in names)
        if spec.get("must_have_entities"):
            r.check(f"{layer}.must_have_entities(D4)",
                    None if data is None else all(s.get("entities") for s in data))


def assert_negative(exp, mgr, scenario, r):
    neg = exp.get("negative", {})
    if "endpoint_terminal_count" in neg:
        data = _query(mgr, "signals", "--scenario", scenario, "--terminal")
        r.check(f"negative.endpoint_terminal_count=={neg['endpoint_terminal_count']}",
                None if data is None else len(data) == neg["endpoint_terminal_count"])
    if neg.get("endpoint_terminal_required"):
        data = _query(mgr, "signals", "--scenario", scenario, "--terminal")
        r.check("negative.endpoint_terminal_required",
                None if data is None else len(data) >= 1)


def assert_controls(exp, mgr, scenario, r):
    for ctl in exp.get("control_assertions", []):
        # 对照断言需改一个开关重跑（disable cross_lineage / switch converge.mode）
        label = ctl.get("disable") or ctl.get("switch")
        print(f"  [control] toggle '{label}' then re-run, expect: "
              f"{ {k: v for k, v in ctl.items() if k.startswith('then')} }")
        r.check(f"control[{label}]", None)  # TODO: 接编排后实测


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--scenario", required=True)
    ap.add_argument("--expected", required=True)
    ap.add_argument("--mgr", default="10.66.0.10")
    a = ap.parse_args()
    exp = yaml.safe_load(open(a.expected))
    print(f"[assert] scenario={a.scenario} dry_run={DRY}")
    r = Result()
    assert_signals(exp, a.mgr, a.scenario, r)
    assert_incident(exp, a.mgr, a.scenario, r)
    assert_negative(exp, a.mgr, a.scenario, r)
    assert_controls(exp, a.mgr, a.scenario, r)
    print(f"[assert] {a.scenario}: pass={r.passed} fail={r.failed} skip={r.skipped}")
    # 把结果落到 .results 供 report.py 汇总
    os.makedirs(".results", exist_ok=True)
    json.dump({"scenario": a.scenario, "pass": r.passed, "fail": r.failed, "skip": r.skipped},
              open(f".results/{a.scenario}.json", "w"))
    sys.exit(1 if r.failed else 0)


if __name__ == "__main__":
    main()
