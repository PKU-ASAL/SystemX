# 调试疑点

## Tetragon 文件事件偶发缺失进程身份

- 状态：待验证，不作为已确认缺陷或 Release 阻塞。
- 现象：2026-07-27 medium 性能测试的 13,764 条 scoped Event 中，15 条
  `file.read` Event 缩略进程对象缺少 `process.binary`、argv 和父进程身份。
- 已确认：SysArmor Tetragon gRPC 适配器和规范化链路没有清空这些字段；缺失值来自
  Tetragon 上报的 `Process` 对象。
- 现场分类：7 条对应 Tetragon 启动前已存在的 `systemd-logind`，7 条对应 CRON
  fork 子进程，1 条对应此前能够正确识别的 sshd 进程。
- 未确认：CRON 是否由 fork 身份继承竞态导致，sshd 是否由进程缓存生命周期或查询失败
  导致。不能仅凭现有结果归因于进程过快或缓存淘汰。
- 安全边界：身份缺失时继续保留原始 Event 和 Signal，不将空 binary 视为可信，也不猜测
  或伪造程序路径。
- 后续验证：分别复现 Tetragon 启动前进程、fork 后 exec 前读取和长生命周期 sshd 读取，
  同步保存原始 gRPC Process 对象、Tetragon 版本与进程缓存指标。
