# Published 安装测试 Fail-Closed 设计

## 结论

公开发行包的三个系统镜像必须在 `curl | bash` 任一阶段失败时立即终止 Docker
构建，禁止生成未安装 SysArmor 的测试镜像。

## 根因

当前 Dockerfile 使用默认 `/bin/sh` 执行管道。`curl` 下载失败时，管道状态取决于
末尾 `bash`；空输入的 `bash` 可以返回成功，导致下载失败被延迟到容器启动时才暴露。

## 方案

Ubuntu 22.04、Ubuntu 24.04 和 Debian 12 的测试 Dockerfile 统一声明 Bash 严格管道
Shell：

```dockerfile
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
```

保留现有安装命令和镜像矩阵。扩展现有 Release container E2E 契约，要求三个
Dockerfile 均包含该声明，防止后续回退为 fail-open。

## 验收

1. 新契约在当前 Dockerfile 上失败。
2. 三个 Dockerfile 修复后契约转绿。
3. 本地 Distribution 契约通过。
4. Debian 12 的 `v0.1.0-rc.5` post-publish 验收在网络可用时通过。
5. 修复仅通过 PR 合入 `dev`，不更新 Release 分支或 RC。
