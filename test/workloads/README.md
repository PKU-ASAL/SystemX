# SysArmor Workloads

`workloads/` 用于性能评估,和 `scenarios/` 分开。

- `scenarios/`:功能攻击/故障输入,带安全语义和断言。
- `workloads/`:稳定制造行为压力,不做安全断言。

目标 workload:

| workload | 目的 |
|---|---|
| `business-normal` | 正常构建/缓存/校验业务,评估业务干扰和误报 |
| `host-activity-heavy` | 主机进程和普通文件活动很重,覆盖 exec/read/write |
| `edr-activity-heavy` | EDR 关注面活动很重,覆盖 exec/file/local-network |

每个 workload 目标结构:

```text
workloads/
  vm/<workload>/run.sh
  container/<workload>/run.sh
```

`run.sh` 约定:

```bash
DURATION=60 REPEAT=10 CONCURRENCY=1 ./run.sh
```

输出写到 stdout/stderr,不直接写 benchmark summary。Recorder 负责采样,benchmark 负责汇总。
