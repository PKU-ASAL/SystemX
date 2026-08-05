# v0.1.0-rc.5 managed Agent 状态 fixture

`agent-managed-v1.sql` 使用 `v0.1.0-rc.5`（commit
`454b69d6c01f778add5836e0af1c9ba3299fd5b1`）的完整 localstore baseline schema，
并加入确定性的非敏感 managed enrollment、device identity 和 endpoint policy 数据。

fixture 表达旧版本的持久状态契约，不包含证书、私钥、enrollment token、completion token、
WAL 或机器相关数据。VM E2E 只把 SQL 导入临时数据库，并在副本中绑定运行时生成的身份、
Gateway 和凭据路径。

修改 SQL 后必须同步更新 `manifest.json` 中的 SHA-256。不得为方便测试向 schema v1
预先加入新版本字段。
