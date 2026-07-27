# Credential Read Low-Noise Design

## 目的与结论

默认启用 `credential_file_read`，保留全部原始 Event，仅减少正常系统行为和重复行为产生的 Signal。实现复用现有内容条件树与抑制器，不增加 Go 规则语法、Signal 格式或兼容入口。

## 现状

Medium 测试的 272 条 credential Signal 中，240 条来自 Recorder 每 10 秒执行一次 `sudo sysarmorctl agent health`。现有规则仅信任编辑器，并按 `process.stable_id + file.path` 抑制；每次 `sudo` 都有新的 stable ID，因此无法跨调用降噪。

## 方案

1. Recorder 已由 root 启动，内部健康查询直接调用 `sysarmorctl`，不再创建 `sudo` 自噪声。
2. 新增系统凭据读取基线 Context，首批限定 `/usr/bin/sudo`、`/usr/sbin/sshd`。
3. 规则只在 binary 命中系统基线且 `process.uid == 0` 时排除；`cat`、`curl`、Shell、未知或字段缺失进程继续告警。
4. 抑制键改为 `lineage_id + process.binary + file.path`，窗口保持 5 分钟。不同 lineage 和不同敏感路径分别告警。
5. 删除默认 Policy 中关闭该规则的 override，使其随默认 ruleset 启用。

## 数据与安全边界

- Collection Policy 不变，所有 `file.read` Event 继续采集、存储和上传。
- 抑制只影响重复 Signal，不删除 Event 或 Event Evidence。
- 不将 `sudo`、`sshd` 加入无条件可信列表；必须同时满足 root UID。
- 缺失 binary、UID 或 lineage 时采取保守语义，不得静默进入可信基线。

## 验收

- root `sudo`、root `sshd` 正常读取基线路径不产生 Signal；非 root 或非基线 binary 仍产生。
- `cat`、`curl`、Shell、未知 binary 读取 `/etc/shadow` 产生 Signal。
- 同 lineage、binary、path 在 5 分钟内只产生一条；不同 lineage 或 path 分别产生。
- Event 数量不因 Signal 抑制减少，所有 Signal EventRefs 可解析。
- quick/medium 无 dropped event、parse error 或 watch error；攻击链关键 Signal 保持命中。

