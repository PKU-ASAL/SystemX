# Harness Library

结论：`test/shared/harness/lib` 是 shell 级通用工具库，只处理路径、端口、进程、等待和清理；产品断言留在具体 suite。

## 当前 Helpers

| Helper | 作用 |
|---|---|
| `sa_init_repo_paths` | 初始化 `ROOT`、`RESULTS`、`BIN` |
| `sa_make_tmp` | 创建带命名空间的临时目录 |
| `sa_pick_ports` | 分配 manager HTTP/gRPC 端口和 `MGR_URL` |
| `sa_kill_pid_ref` | 按 pid 文件清理进程 |
| `sa_cleanup_tmp` | 清理临时目录 |
| `sa_wait_contains` | 重试命令直到输出包含目标字符串 |
| `sa_wait_url_contains` | 重试 URL 直到响应包含目标字符串 |
| `sa_wait_glob` | 等待文件出现 |
| `sa_wait_no_glob` | 等待文件消失或队列清空 |
| `sa_build_all` | 构建项目常用二进制 |
| `sa_build_go_bins` | 构建指定 Go binary |
| `sa_start_memory_manager` | 启动 in-memory manager |
| `sa_manager_ctl` | 通过 `sysarmorctl --manager-url "$MGR_URL"` 调 manager API |

## 维护规则

1. helper 只做机制，不写 scenario 语义。
2. 不在 lib 中硬编码具体 test case 的 expected event/signal。
3. 需要复用三次以上的 shell glue 再下沉到这里。
4. 新 helper 保持短小，错误显式返回。
