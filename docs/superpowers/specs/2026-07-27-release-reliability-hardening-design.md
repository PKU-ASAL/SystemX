# Release Reliability Hardening Design

## 结论

发布前关闭三个独立可靠性缺口：安装回滚不得吞错或删除唯一备份；tetra stdout 必须在
进程 Wait 前完整排空；默认 collection 必须覆盖启用规则要求的 `file.chmod`。

## 安装回滚

回滚逐目标记录成功与失败。恢复失败时保留对应 backup，输出目标与备份路径，并让安装器
非零退出；清理只删除 stage 和已成功处理的 backup。合同测试注入 backup-to-target 的
`mv` 失败，验证备份仍存在且旧服务不会被报告为恢复成功。

## Tetra 输出

不再并发调用 `Cmd.Wait` 与读取 `StdoutPipe`。supervisor 使用 `io.Pipe` 接收命令 stdout；
Wait 在复制结束后关闭 writer，scanner 读取真实 EOF。取消订阅时先终止进程，scanner 排空
已有数据后退出。测试覆盖大量尾部事件、dropped 统计、立即退出和重复运行。

## Detection Coverage

`degraded` 的准确原因是默认 collection 缺少启用规则声明的 `file.chmod`。将该 behavior
加入默认 policy，并增加默认 policy 对默认 ruleset 的 coverage 合同。验收要求 report 和
三镜像 health 的 `detection.lastApplyStatus` 均为 `applied`。
