# Tetragon Container ID Matching Design

## 结论

SysArmor 在 Tetragon backend 内统一比较不同长度的 Container ID，使 CVELab 等调用方可以始终传入 Docker 返回的完整 ID，不需要了解 Tetragon 输出 12、32 或 64 位 ID 的差异。

## 范围

- `container` scope、旧 `container_id_prefix` 和解析成功后的 `namespace/self` 共用同一 matcher。
- `cgroup`、`pod` 和普通 namespace selector 保持现有字符串前缀语义。
- 不改变 `namespace/self` 从 `/proc/self/cgroup` 解析自身 ID 的流程；private cgroup namespace 仍需 `--cgroupns=host`。
- 不修改 CVELab 或其他注入层。

## 匹配语义

输入先去除首尾空白并转为小写。

1. 保留正向兼容：`eventID` 以 `selector` 开头即匹配。该规则继续允许现有不足 12 位或非十六进制的短 selector。
2. 增加受限反向兼容：`selector` 以 `eventID` 开头时，仅当双方都是纯十六进制且 `eventID` 长度至少 12 位才匹配。
3. 空 selector 不匹配，避免空字符串命中所有事件。

反向限制确保完整 64 位 selector 可以匹配 Tetragon 截断 ID，同时避免单字符或普通字符串造成过宽匹配。

## 实现边界

- 在 `internal/sensors/linux/tetragon/scope_identity.go` 维护唯一的 `containerIDsMatch(eventID, selector string) bool`。
- `Backend.matchesScope` 的 container 和 legacy 分支改用该 helper。
- `namespace/self` 继续复用该 helper；只统一比较逻辑，不统一身份解析逻辑。

## 验证

表驱动单测覆盖：

- 64 位 selector 匹配 32 位 event ID；
- 12 位 selector 匹配 32/64 位 event ID；
- 大小写与首尾空白归一化；
- 不同容器不匹配；
- 反向匹配拒绝不足 12 位、非十六进制和空 ID；
- 原有短 legacy selector 正向匹配保持兼容；
- container、legacy 和 namespace/self 三个 Backend 分支使用一致语义。
