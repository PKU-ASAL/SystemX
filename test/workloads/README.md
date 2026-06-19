# SysArmor Workloads

`workloads/` 用于性能评估,和 `scenarios/` 分开。

- `scenarios/`:功能攻击/故障输入,带安全语义和断言。
- `workloads/`:稳定制造行为压力,不做安全断言。

目标 workload:

| workload | 目的 |
|---|---|
| `exec-storm` | 放大 process exec/fork/exit 路径 |
| `file-write-storm` | 放大 payload/persistence file write/chmod 路径 |
| `file-read-storm` | 放大 credential/secret read 路径 |
| `network-connect-storm` | 放大 socket connect 路径 |
| `mixed-edr-storm` | 混合 exec/file/network,用于默认 sensor benchmark |
| `benign-business` | 模拟正常构建/文件/校验业务,评估 EDR 对非攻击业务的基础干扰 |

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
