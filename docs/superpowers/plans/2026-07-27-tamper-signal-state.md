# Tamper Signal State Implementation Plan

**Goal:** 消除正常安静期的 sensor 失明误报和同一故障的重复 Signal。

**Architecture:** tamper detector 只消费明确健康故障，并按连续故障状态去重；daemon 不再构造基于业务事件沉默的 grace period。

### Task 1: 锁定行为

- [ ] 增加安静期不报警测试。
- [ ] 增加同一故障去重、恢复后复发测试。
- [ ] 运行测试并确认旧实现失败。

### Task 2: 最小实现

- [ ] 删除 `NoEventGracePeriod` 和 `tamperNoEventGracePeriod`。
- [ ] 按稳定 reason 状态去重，并在健康恢复时清空状态。
- [ ] 运行 tamper、daemon 和全量相关测试。

### Task 3: 环境验收

- [ ] 重跑 fresh medium，要求 tamper Signal 为 0、drop/parse/watch error 为 0。
- [ ] medium 通过后运行三镜像 Release 矩阵。
