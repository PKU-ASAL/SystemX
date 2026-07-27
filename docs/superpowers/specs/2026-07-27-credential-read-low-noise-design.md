# Credential Read Low-Noise Design

## 目的与结论

默认启用 `credential_file_read`，保留全部原始 Event，仅减少正常系统行为和重复行为产生的 Signal。实现复用现有内容条件树与抑制器，不增加 Go 规则语法、Signal 格式或兼容入口。

## 现状

Medium 测试的 272 条 credential Signal 中，240 条来自 Recorder 每 10 秒执行一次 `sudo sysarmorctl agent health`。现有规则仅信任编辑器，并按 `process.stable_id + file.path` 抑制；每次 `sudo` 都有新的 stable ID，因此无法跨调用降噪。

## 方案

1. Recorder 已由 root 启动，内部健康查询直接调用 `sysarmorctl`，不再创建 `sudo` 自噪声。
2. 建立精确的系统凭据读取命令基线，覆盖 sudo 实际执行目标为 `sysarmorctl` 的命令和已知 sshd 守护进程形态。
3. 规则只在 `process.binary` 和 `process.argv` 同时命中同一条基线时排除；`cat`、`curl`、Shell、未知或字段缺失进程继续告警。
4. 抑制键改为 `lineage_id + process.binary + file.path`，窗口保持 5 分钟。不同 lineage 和不同敏感路径分别告警。
5. 删除默认 Policy 中关闭该规则的 override，使其随默认 ruleset 启用。

## 数据与安全边界

- Collection Policy 不变，所有 `file.read` Event 继续采集、存储和上传。
- 抑制只影响重复 Signal，不删除 Event 或 Event Evidence。
- 不将 `sudo`、`sshd` 加入无条件可信列表；必须同时满足对应 argv 特征。
- sudo 基线解析 argv 中的实际目标命令，跳过 sudo 选项及其参数；只有目标命令 basename
  为 `sysarmorctl` 时才可信。参数文本中仅出现该名称的 Shell 等命令仍告警。
- Tetragon 仅提供格式化后的 arguments 字符串。适配器只有在参数由安全 token 和普通单空格
  组成、边界可无歧义恢复时才设置 `argv_boundaries_trusted`；含引号、Tab、换行、反斜线、
  首尾或重复空格时保持不可信。sudo 基线必须同时要求该标志，默认 false 并 fail-closed。
- `sudoedit`、Shell、登录 Shell、查询/帮助等非执行模式不解析为目标命令。
- 缺失 binary、argv 或 lineage 时采取保守语义，不得静默进入可信基线。

## 验收

- 精确的 `sudo sysarmorctl`、带 sudo 选项的 SysArmor CLI、sshd 守护命令读取基线路径
  不产生 Signal；`sudo bash -c '...sysarmorctl...'` 和其他非基线命令仍产生。
- `cat`、`curl`、Shell、未知 binary 读取 `/etc/shadow` 产生 Signal。
- 同 lineage、binary、path 在 5 分钟内只产生一条；不同 lineage 或 path 分别产生。
- Event 数量不因 Signal 抑制减少，所有 Signal EventRefs 可解析。
- quick/medium 无 dropped event、parse error 或 watch error；攻击链关键 Signal 保持命中。
