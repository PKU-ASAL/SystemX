# Test Policies

结论：`test/data/policies/` 保存测试用 policy/content 契约样例。benchmark 最常用的是 collection JSON；完整 manager 下发、版本化、启停、audit 闭环仍由 platform/topology 测试逐步覆盖。

## 文件说明

| 文件 | 用途 |
|---|---|
| `collection-minimal.json` | 最小常开采集面，适合作为低成本下界 |
| `collection-balanced.json` | 默认平衡采集面，适合日常 endpoint/topology benchmark |
| `collection-deep.json` | 调查/高风险窗口采集面，成本更高 |
| `collection.yaml` | collection intent 设计样例 |
| `detection.yaml` | 检测规则和收敛参数样例 |
| `detection-additive.yaml` | 误报对照用 additive threshold 样例 |
| `detection-cep-endpoint.json` | endpoint CEP 检测样例 |
| `resource.yaml` | 端侧资源上限样例 |
| `telemetry.yaml` | telemetry batch/flush 配置样例 |
| `response.yaml` | response intent 样例，当前偏 observe/audit |

## Benchmark 用法

常用单 policy：

```bash
make -C test performance-endpoint \
  SYSARMOR_BENCH_PROFILE=medium \
  SYSARMOR_BENCH_WORKLOAD=business-normal \
  SYSARMOR_BENCH_SCENARIO=apt-fileless-c2-local \
  SYSARMOR_BENCH_POLICIES='test/data/policies/collection-balanced.json'
```

topology benchmark 使用 `POLICIES`：

```bash
make -C test effectiveness-topology \
  POLICIES='test/data/policies/collection-balanced.json' \
  WORKLOADS='business-normal' \
  SCENARIOS='apt-fileless-c2'
```

## 与 TracingPolicy 的区别

| 类型 | 谁读取 | 作用 |
|---|---|---|
| TracingPolicy | Tetragon/sensor | 低层采集配置 |
| SysArmor policy | sysarmor-agent / manager 控制面 | collection、detection、resource、telemetry、response 语义 |

测试中某些 e2e 会临时生成最小 policy，这是为了缩短路径；benchmark 和设计对齐优先使用本目录 policy。
