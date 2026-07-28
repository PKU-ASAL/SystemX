# Fresh Medium 与三镜像 Release 验收报告

## 结论

本轮验收通过。

- Fresh medium 在全新 Ubuntu 22.04 VM 上完成 `collection-minimal`、
  `collection-balanced`、`collection-deep` 三档测试，未发生 Event 丢弃、解析错误或
  watcher 错误。
- Ubuntu 22.04、Ubuntu 24.04、Debian 12 三镜像均完成无缓存构建、签名 Release 安装、
  Agent/Tetragon 健康检查、五个攻击场景检测和 namespace/self 隔离验证。
- 三个镜像均检出 5/5 场景，每镜像产生 16 个 Signal，Precision 和 Recall 均为 1.0000。
- `collection-balanced` 与 `collection-deep` 的日常负载成本接近；当前没有证据表明 deep
  在本轮工作负载下带来足以抵消额外配置复杂度的收益，日常建议优先使用 balanced。

本报告不将测试通过直接等同于正式 Release Go。当前性能套件没有配置硬性 CPU/RSS
失败预算，medium 结果适合同条件日常比较，不替代 long 基线或生产容量评估。

## 测试对象

- 日期：2026-07-28（Asia/Shanghai）
- 源码基线：`19a146b3915cfdd9787d87e243716894042dc014`
- 工作区修复：
  - `test/suites/performance/endpoint/run.sh`
  - `test/suites/performance/endpoint/test_run_contract.py`
- Release 版本：`local-rulepack-20260728`
- Release 包 SHA-256：
  `796734ad6a3e4c2759df566b2bd7938c0e5c2982803007707b262a6ec7221a6e`
- 内容签名 key ID：`acceptance-20260728`
- 默认内容引用数量：11
- Sensor：Tetragon `v1.7.0`
- 容器 Scope：`namespace/self`

源码基线之后存在上述两个未提交的测试契约修复，因此复现本报告时必须包含这两处改动。
两把测试私钥仅在本地临时目录中生成，验收结束后已连同临时 Release 制品永久删除。

## 修复说明

`collection-minimal` 有意不采集 `file.read`。检测规则扩展后，
`credential_file_read` 和 `account_database_read` 都会在 minimal 模式下报告缺少
`file.read` 输入，但性能脚本仍只允许前一条规则，导致合法的预期降级被误判为失败。

本次修复保持严格匹配，只允许以下两个精确缺口：

```text
credential_file_read -> file.read
account_database_read -> file.read
```

任何额外规则、额外缺失行为或 balanced/deep 的降级仍会使测试失败。新增契约测试已在
Git HEAD 旧版脚本上验证为失败，在当前修复上验证为通过。

## Fresh Medium

### 方法

```bash
make test-performance PROFILE=medium
```

测试销毁旧 VM 并重新创建 Ubuntu 22.04 VM，重新构建并同步当前 SysArmor 二进制。
每个策略执行约 10 分钟，包含 host baseline、Agent idle、Sensor idle、policy apply、
steady、300 秒 `business-normal` workload 和 cooldown。

运行 ID：`20260728T015749Z`。

### 资源结果

| 策略 | 稳态 EDR CPU | 稳态 EDR RSS | 负载 EDR CPU | 负载 EDR RSS | 负载 Event | 负载 Signal |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| minimal | 0.84% | 130.01 MB | 2.39% | 146.66 MB | 0 | 0 |
| balanced | 0.86% | 145.97 MB | 3.98% | 167.12 MB | 5,545 | 6 |
| deep | 0.95% | 141.65 MB | 4.04% | 170.38 MB | 5,545 | 6 |

EDR 指 Agent 与 Tetragon Sensor 的合计。正式比较优先使用 steady 和 workload 平均值；
policy apply 阶段的 114% 至 125% Sensor CPU 为 BPF reload 瞬时峰值，不代表持续成本。

### Agent 与 Sensor

| 策略 | 负载 Agent CPU | 负载 Agent RSS | 负载 Sensor CPU | 负载 Sensor RSS |
| --- | ---: | ---: | ---: | ---: |
| minimal | 0.10% | 38.47 MB | 2.29% | 108.18 MB |
| balanced | 1.34% | 57.47 MB | 2.64% | 109.66 MB |
| deep | 1.36% | 61.32 MB | 2.68% | 109.06 MB |

主要运行成本来自 Sensor。minimal 显著缩窄可见性，在本轮业务负载窗口没有采集到 Event；
balanced 和 deep 的负载吞吐均为 18.3 EPS，资源成本接近。

### 可靠性

三档策略均满足：

- `dropped_events_delta=0`
- `parse_errors_delta=0`
- Event watcher error lines = 0
- Signal watcher error lines = 0
- recorder 正常停止并生成 `summary.json`
- 最终生成三行 `matrix.csv`

medium 未配置攻击 Scenario，因此其中的 Signal 只表示检测引擎产生了输出，不能单独用于
计算攻击检测 Precision/Recall。检测效果以三镜像 Release 矩阵为准。

## 三镜像 Release 矩阵

### 方法

使用同一签名 Release 包和同一 `install.sh`，三个镜像均启用 `--no-cache`：

```bash
URL=http://127.0.0.1:18080/install.sh \
RUN_ID=local-rulepack-20260728-three-images \
FRESH_DOWNLOAD=1 RELEASE_PROXY_URL= \
IMAGES='ubuntu2204 ubuntu2404 debian12' \
bash test/release/run.sh
```

每个镜像依次验证：签名内容安装、Agent/Tetragon health、五个攻击场景、Signal EventRef、
sibling container marker 隔离和 host marker 隔离。镜像串行运行，避免多个 Tetragon 实例
竞争宿主机 eBPF 资源。

### 结果

| 镜像 | Health | Detection | 场景 | 隔离 | Signal | Precision | Recall |
| --- | --- | --- | ---: | --- | ---: | ---: | ---: |
| Ubuntu 22.04 | 通过 | applied | 5/5 | 通过 | 16 | 1.0000 | 1.0000 |
| Ubuntu 24.04 | 通过 | applied | 5/5 | 通过 | 16 | 1.0000 | 1.0000 |
| Debian 12 | 通过 | applied | 5/5 | 通过 | 16 | 1.0000 | 1.0000 |

三个 health 快照均满足：

- 顶层 `status=ok`
- `defaultManifestVersion=local-rulepack-20260728`
- detection `lastApplyStatus=applied`
- 11 个内容 ref 已加载
- `scope.type=namespace`
- `scope.selector=self`

### Signal 分布

| Rule ID | Ubuntu 22.04 | Ubuntu 24.04 | Debian 12 |
| --- | ---: | ---: | ---: |
| `account_database_read` | 1 | 1 | 1 |
| `download_by_lolbin` | 3 | 3 | 3 |
| `payload_dropped` | 4 | 4 | 4 |
| `payload_lifecycle` | 2 | 2 | 2 |
| `reverse_shell_pattern` | 1 | 1 | 1 |
| `suspicious_exec_connect` | 2 | 2 | 2 |
| `web_runtime_spawns_shell` | 3 | 3 | 3 |

三镜像 Signal 数量和规则分布完全一致。五个声明攻击场景均命中预期规则、severity、
behavior 和端口；未发现缺失 EventRef。测试范围外的 sibling container marker 与 host
marker 均未被业务容器采集。

## 验证结果

```text
python3 -m unittest -v test_run_contract.py
Ran 5 tests ... OK

make test-performance PROFILE=medium
exit 0

bash test/release/run.sh
ubuntu2204 ok
ubuntu2404 ok
debian12 ok
all images passed

git diff --check
exit 0
```

证据目录：

- `test/.results/performance-endpoint/20260728T015749Z/`
- `test/.results/release/local-rulepack-20260728-three-images/`

## 风险与边界

- medium 是单次、单 VM、单 workload 结果；balanced 与 deep 的小幅差异可能包含运行顺序
  和系统噪声，不能据此宣称统计显著。
- 本轮没有执行 long profile，不能作为长期内存增长、泄漏或持续高负载结论。
- medium 没有硬性资源预算；建议在建立稳定基线后为 steady/workload CPU、RSS、丢弃和
  解析错误设置明确失败阈值。
- 三镜像矩阵验证容器内 `namespace/self` 范围，不代表宿主机模式、其他内核版本或所有
  业务镜像均已验证。
- 结果目录为生成证据，不纳入源码提交；临时 HTTP 服务已停止，端口 18080 已释放。
