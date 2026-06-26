# MVP Matrix Analysis: 20260625T171849Z

本文记录 slim VM matrix 运行 `20260625T171849Z` 的结果，并分析 MVP 阶段检测效果不足、CPU 占用偏高的原因。本文已按修复后的 effectiveness report 重新生成，修复点包括 recorder scoped/all cursor 竞态，以及 signal 引用事件缺失时的 report 补偿逻辑。

本文的 CPU 数据来自 `test/.results/bench-matrix-vm/20260625T171849Z/matrix.csv`，effectiveness 数据来自修复后的 `effectiveness_report.py` 重新计算结果。报告产物已简化为原始 case artifacts、`matrix.csv` 和 `truth_steps.csv`。

## 1. Test Scope

运行配置：

| Item | Value |
|---|---|
| Run ID | `20260625T171849Z` |
| Matrix mode | `cross` |
| Policies | `collection-minimal-high-signal`, `collection-edr-balanced`, `collection-incident-deep` |
| Workload | `business-normal` |
| Scenarios | `apt-fileless-c2`, `apt-staged-drop`, `benign-ci-noise` |
| Cases | 9 |
| Status | all ok |
| Dropped events | 0 |
| Parse errors | 0 |

## 2. Performance Summary

| Scenario | Policy | Events | Signals | Steady CPU | Workload CPU | Drops |
|---|---|---:|---:|---:|---:|---:|
| `apt-fileless-c2` | minimal | 0 | 0 | 1.8% | 3.0% | 0 |
| `apt-fileless-c2` | balanced | 702 | 3 | 13.2% | 22.8% | 0 |
| `apt-fileless-c2` | deep | 1112 | 0 | 39.8% | 34.9% | 0 |
| `apt-staged-drop` | minimal | 0 | 0 | 2.4% | 3.7% | 0 |
| `apt-staged-drop` | balanced | 868 | 3 | 28.9% | 29.9% | 0 |
| `apt-staged-drop` | deep | 1220 | 3 | 40.2% | 41.8% | 0 |
| `benign-ci-noise` | minimal | 5 | 5 | 2.7% | 4.0% | 0 |
| `benign-ci-noise` | balanced | 905 | 6 | 24.8% | 30.7% | 0 |
| `benign-ci-noise` | deep | 1378 | 3 | 41.0% | 41.5% | 0 |

结论：

- `minimal` 成本低，但攻击覆盖失败；
- `balanced` 检测效果最好，但 CPU 未达到个位数目标；
- `deep` 事件量和 CPU 都高，且本次检测效果反而不稳定，说明它目前更像调试/调查档，不适合常开，也不能直接假设“采更多就检得更准”。

## 3. Effectiveness Summary

| Scenario | Policy | Effectiveness | Event Recall | Signal Recall | Signal F1 | Signal Precision | Observed Signals |
|---|---|---:|---:|---:|---:|---:|---:|
| `apt-fileless-c2` | minimal | 0.05 | 0.00 | 0.00 | 0.00 | 1.00 | 0 |
| `apt-fileless-c2` | balanced | 0.90 | 1.00 | 1.00 | 1.00 | 1.00 | 3 |
| `apt-fileless-c2` | deep | 0.05 | 0.00 | 0.00 | 0.00 | 1.00 | 0 |
| `apt-staged-drop` | minimal | 0.05 | 0.00 | 0.00 | 0.00 | 1.00 | 0 |
| `apt-staged-drop` | balanced | 0.8833 | 1.00 | 1.00 | 0.80 | 0.6667 | 3 |
| `apt-staged-drop` | deep | 0.0333 | 0.00 | 0.00 | 0.00 | 0.6667 | 3 |
| `benign-ci-noise` | minimal | 0.30 | n/a | n/a | n/a | 0.00 | 5 |
| `benign-ci-noise` | balanced | 0.30 | n/a | n/a | n/a | 0.00 | 6 |
| `benign-ci-noise` | deep | 0.30 | n/a | n/a | n/a | 0.00 | 3 |

Attack signal matrix:

| Policy | `apt-fileless-c2` | `apt-staged-drop` |
|---|---|---|
| minimal | P=1.00 R=0.00 F1=0.00 | P=1.00 R=0.00 F1=0.00 |
| balanced | P=1.00 R=1.00 F1=1.00 | P=0.67 R=1.00 F1=0.80 |
| deep | P=1.00 R=0.00 F1=0.00 | P=0.67 R=0.00 F1=0.00 |

## 4. Signal Details

### 4.1 `apt-fileless-c2`

| Policy | Signals |
|---|---|
| minimal | none |
| balanced | `download_by_lolbin`, `payload_dropped`, `reverse_shell_pattern(terminal=true)` |
| deep | none |

Balanced 已经能完整命中 fileless C2 的主干 signal，包括 terminal reverse shell。修复后的 report 会用 `events-all.ndjson` 补回被 scoped stream 漏掉但被 signal 引用的事件，并在事件本体仍缺失时标记 `event_missing_from_recorder`。因此该 case 的 event recall 从假阴性的 0.00 恢复为 1.00。

具体而言：

- `payload_download` 由 `events-all.ndjson` 中的 `vm-owned-tetragon-00000000000000002071` 补回；
- `payload_write` 由 `events-all.ndjson` 中的 `vm-owned-tetragon-00000000000000002072` 补回；
- `reverse_c2` 的 signal ref 指向 `vm-owned-tetragon-00000000000000002074`，该事件在 recorder 输出中仍缺失，因此被标记为 `event_missing_from_recorder`，不再误算为 detection/label miss。

### 4.2 `apt-staged-drop`

| Policy | Signals |
|---|---|
| minimal | none |
| balanced | `reverse_shell_pattern(terminal=true)`, `suspicious_exec_connect`, `payload_lifecycle` |
| deep | `download_by_lolbin`, `suspicious_exec_connect`, `payload_dropped` |

Balanced 是本次唯一同时满足 event recall 和 signal recall 的策略。Deep 采到了信号但没有匹配 ground truth，原因是它在 download socket `10.66.0.99:8080` 上触发了 `suspicious_exec_connect`，而 ground truth 期望的是 control socket `10.66.0.99:443`。

### 4.3 `benign-ci-noise`

| Policy | False Positive Signals |
|---|---|
| minimal | 5 x `download_by_lolbin` |
| balanced | 6 x `download_by_lolbin` |
| deep | 3 x `download_by_lolbin` |

这些 false positives 不是 `benign-ci-noise` 脚本本身产生的。事件中出现了 VM 内后台 `curl` 访问 `10.66.0.99:8080/deps.tar` 和 POST 到 C2 HTTP server，说明测试环境存在污染源。benign 场景原则上不应该触碰 C2、payload path、persistence path 或 credential path。

## 5. Why MVP Detection Was Not Good Enough

### 5.1 规则覆盖仍然是窄链路 MVP

当前规则主要验证：

```text
download -> payload drop -> exec/connect -> C2
```

这条链对 `curl/wget + bash/sh + known C2` 有效，但对以下攻击类仍然不足：

- interpreter download / reverse shell: python, perl, ruby, node, php；
- nc/socat/openssl tunnel；
- persistence: cron, systemd, authorized_keys, ld.so.preload；
- privilege escalation: sudoers modification, suid/sgid, pkexec/sudo abuse；
- lateral movement: ssh/scp/rsync/remote exec；
- exfiltration: credential read followed by external send。

因此，只用 `apt-fileless-c2` 和 `apt-staged-drop` 两个场景不能证明规则能覆盖大部分攻击路径，只能证明当前 MVP 主干在特定条件下可工作。

### 5.2 IoC 模型把 download port 和 control port 混在一起

当前 `ioc:c2-port-feed` 包含：

```text
443, 8443, 8080
```

这让 download server `8080` 和 control channel `443` 都被视作 C2 socket。结果是：

- `download_by_lolbin` 用 8080 是合理的；
- `reverse_shell_pattern` / `suspicious_exec_connect` 用 8080 就容易误判；
- deep 在 `apt-staged-drop` 中把 download socket 上的事件解释成 `suspicious_exec_connect`，导致 ground truth 不匹配。

建议拆分：

- `ioc:c2-download-port-feed`: 8080 等 payload hosting port；
- `ioc:c2-control-port-feed`: 443/8443 等 command/control port；
- download 规则只使用 download/control 的宽口径；
- reverse shell、payload C2、lifecycle terminal 只使用 control port。

### 5.3 Event stream 和 Signal stream 的评估窗口问题已定位并修复

旧报告中 `apt-fileless-c2` balanced 的 signal recall 为 1.0，但 event recall 为 0。这不是 policy 或 label 的真实失败，而是 recorder/report 的假阴性。

已定位的原因：

- recorder 之前让 scoped watch 和 all watch 共用 cursor；
- all watch 在 scoped watch 之后采到攻击事件，并推进 cursor；
- 下一轮 scoped watch 从新的 cursor 后开始，导致 `events.ndjson` 永久漏掉这些事件；
- effectiveness report 只使用 `events.ndjson` 做 label event 匹配，导致 signal 已命中但 event recall 被低估。

修复后：

- recorder 拆分 scoped/all cursor，避免 all watch 推进 scoped cursor；
- report 使用 `events-all.ndjson` 补回被 signal 引用但 scoped stream 漏掉的事件；
- 如果 signal ref 指向的事件仍没有被 recorder 保存，truth step 会标记 `event_missing_from_recorder`，用于区分 recorder 缺证和 detection miss；
- `apt-fileless-c2 + business-normal + balanced` 重新计算后为 `event_recall=1.00`、`signal_recall=1.00`、`signal_event_link_rate=1.00`。

### 5.4 Deep 不是“更多采集 = 更好检测”

Deep 采集了更多 `process.exec`、`process.exit`、`file.read`，但本次没有带来更高 recall。原因：

- deep 的事件量主要来自 broad exec/exit/read 噪声；
- 检测规则仍依赖具体 state 和 socket/entity 匹配；
- 如果 C2 端口语义混淆或 event/signal 窗口不一致，更多事件不会自动修复匹配；
- broad 事件反而增加 CPU 和分析噪声。

Deep 应定位为短时调查窗口，而不是默认检测效果基线。

### 5.5 Benign 环境被 C2 下载污染

`benign-ci-noise` 本应是本地 CI 噪声，但结果中出现多次：

```text
/usr/bin/curl -> 10.66.0.99:8080/deps.tar
```

这导致三档策略都产生 `download_by_lolbin` false positive。该污染会显著拉低 benign effectiveness，使我们无法客观判断规则误报率。

## 6. Why CPU Was High

### 6.1 Balanced broad shell `process.exec` 是主因

Balanced 当前在 `process.exec` 中包含 `/bin/sh`、`/bin/bash`、`/usr/bin/sh`、`/usr/bin/bash`。VM、SSH、recorder、benchmark harness 和系统脚本大量使用 shell，导致：

- balanced steady CPU 达到 13.2% 到 28.9%；
- workload CPU 达到 22.8% 到 30.7%；
- 每个 case 产生 700 到 900 级别 scoped events。

这违背了“balanced 常开、个位数 CPU”的目标。

建议：

- 从 balanced `process.exec` 移除 `/bin/sh`、`/bin/bash`、`/usr/bin/sh`、`/usr/bin/bash`；
- 保留这些 binary 在 `network.connect` 中，因为 connect 频率远低于 exec，仍可抓 reverse shell；
- 若需要 shell exec 上下文，交给 `incident-deep` 或 terminal 触发后的 enhanced collection window。

### 6.2 Credential `file.read` 事件比预期多

Balanced 中新增了 credential path `file.read`。本次 event behavior summary 显示 balanced 在不同 case 中有数百到上千 `file.read` 事件。这说明：

- 系统正常流程也可能频繁读取 `/etc/passwd` 等路径；
- 单纯 path-limited read 仍可能不是足够低噪声；
- 需要更强的 trusted binary / trusted lineage suppression。

建议：

- 对 `credential_file_read` 保留规则级 suppress；
- collection 层可先只采更高风险路径，如 `/etc/shadow`、`/root/.ssh/`、`/run/secrets/`；
- `/etc/passwd` 可降级到 deep 或 context-only。

### 6.3 Deep 启用 fork/exit/broad exec/read 带来预期高成本

Deep steady/workload CPU 稳定在约 35% 到 42%。这是符合其 broad collection 设计的，不应作为常开档。

建议：

- deep 只由 terminal signal、incident、人工调查触发；
- 必须有时间窗和自动回退；
- deep benchmark 不参与 balanced 常开 CPU 目标。

### 6.4 Policy apply/settle 阶段 spike 明显

矩阵中 policy apply 和 settle 阶段 CPU 明显高于 steady。Tetragon live policy apply 本身会带来 reload 和 kprobe 更新成本。

建议：

- benchmark 报告继续分离 baseline / apply / settle / steady / workload；
- policy apply spike 不计入 steady-state SLO；
- 但 live replacement 的可见性 timing gap 需要继续修复。

## 7. Prioritized Fix Plan

### P0: 修复 benchmark/report 可信度

1. 清理 VM 中 benign 场景期间访问 C2 HTTP server 的后台 curl 源；
2. 已修复 event watcher 与 signal watcher 的窗口/sequence 不一致导致的假阴性；

### P1: 压 balanced CPU

1. 从 balanced `process.exec` 移除 shell/interpreter；
2. 保留 shell/interpreter 在 `network.connect`；
3. 收窄 credential read path，降低 `/etc/passwd` 噪声；
4. 重跑 3x3 matrix，目标 steady CPU < 10%。

### P2: 修复 C2 端口语义

1. 拆分 download port 和 control port refs；
2. `download_by_lolbin` 可匹配 8080；
3. `reverse_shell_pattern`、`suspicious_exec_connect`、`payload_lifecycle` 只匹配 control port；
4. 更新 labels 和 effectiveness report 的 entities 期望。

### P3: 扩展 MVP 规则族

1. `persistence_artifact_write`;
2. interpreter/nc/socat/openssl C2;
3. `credential_file_read -> C2` chain;
4. `lateral_movement_attempt`;
5. terminal-anchored `payload_lifecycle` aggregator。

## 8. Bottom Line

本次 MVP matrix 证明了：

- balanced 可以在特定主干攻击链上命中关键 event 和 signal；
- 当前规则体系仍不足以覆盖大部分攻击；
- 当前性能瓶颈主要来自 balanced 中 broad shell `process.exec`；
- 当前误报和 recall 数据仍被测试环境污染、C2 port 语义混淆影响；event/signal 窗口不一致导致的 fileless C2 假阴性已修复；
- 下一步应先修 benchmark 可信度和 balanced CPU，再扩展规则族。
