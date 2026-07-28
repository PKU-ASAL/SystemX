# Tamper Signal State Design

## 结论

业务事件沉默不能证明采集链路失明。当前系统没有独立的 event-stream heartbeat，因此删除基于 `LastEventAt` 的失明判断，只保留明确的 sensor 故障信号。

## 判断边界

- 告警：policy 未加载、sensor 未运行、sensor 明确错误、重启/解析/drop 超过配置阈值。
- 不告警：sensor running 且 policy loaded，但一段时间没有业务事件。
- 同一连续故障只产生一条 Signal；恢复后相同故障再次发生时重新告警。

## 兼容性

`NoEventGracePeriod` 和 daemon 的 grace 计算只服务于错误推断，直接删除，不保留冗余兼容入口。Signal schema 不变，原始 Event 保留逻辑不变。

## 验收

- 安静期不产生 `sensor_tamper_or_blindness`。
- 同一故障重复健康检查只产生一条 Signal。
- 恢复后故障复发产生新 Signal。
- 明确 sensor 故障与阈值告警继续工作。
