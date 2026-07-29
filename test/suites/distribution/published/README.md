# Distribution Published 测试

该目录验证公开 standalone 发行包在 Ubuntu 22.04、Ubuntu 24.04 和 Debian 12 中的真实安装与运行链路。测试运行真实 Node.js HTTP 业务，覆盖 Agent/Tetragon 健康、三条内置检测规则的完整 Event/Signal 证据和 `namespace/self` 隔离。

```bash
make -C test/suites/distribution/published doctor
make -C test distribution-published
```

缺省会查询 GitHub 最新 pre-release 的 `install.sh`。也可以固定版本，保证测试可复现：

```bash
make -C test distribution-published \
  URL=https://github.com/PKU-ASAL/sysarmor/releases/download/<tag>/install.sh
```

开发中的未发布构建可以通过本地 HTTP 地址测试，但这不替代公开链接验收。正式发布验收必须显式传入
新 tag 的完整 `URL`，避免误测其他 pre-release。测试结果保存在 `test/.results/release/<run-id>/`。

测试参数集中在 `config.sh`。可以通过 Make 参数覆盖镜像和下载行为：

```bash
make -C test distribution-published \
  URL=https://github.com/PKU-ASAL/sysarmor/releases/download/<tag>/install.sh \
  IMAGES="ubuntu2204 debian12" \
  FRESH_DOWNLOAD=1
```

`FRESH_DOWNLOAD=1` 为默认值，会使用 `docker build --no-cache`，确保每次正式验收都重新执行公开下载。仅在调试测试脚本时使用 `FRESH_DOWNLOAD=0` 复用本地镜像层。

测试依次验证 `web_runtime_spawns_shell`、`download_by_lolbin`、`reverse_shell_pattern`、`suspicious_exec_connect` 和 `payload_lifecycle`。其中反向 Shell 场景由 Bash 直接建立控制连接，exec-connect 和 lifecycle 场景会真实下载并执行 payload。GitHub 直连失败时默认尝试 `https://gh-proxy.org`，也可以通过 `RELEASE_PROXY_URL=` 禁用或指定其他代理。

`run.sh` 只负责容器生命周期和场景调度；`fixtures/` 提供真实业务与本地攻击服务器；`attacks/` 负责调用业务入口；`assert.sh` 通过容器内 `sysarmorctl` 查询 Event/Signal，并使用 `jq` 验证关联关系与 namespace 隔离。

## 运行条件

目标系统安装阶段需要 `bash`、`curl`、`jq`、`sha256sum` 和 `tar`。

容器必须使用 `--privileged --cgroupns=host`，并挂载宿主机 BTF 与 bpffs。测试按镜像串行执行，避免多个 Tetragon 实例竞争同一宿主机的 eBPF 资源。
生产运行还应配置 `--restart unless-stopped` 或等价编排策略。Agent 异常退出、业务退出和信号转发由独立的容器入口测试覆盖，不属于本 Release 矩阵。

## Vulhub 边界

容器入口会原样执行 Dockerfile 的业务命令，因此可以包装已有服务入口。但 Vulhub 镜像经常在原 `ENTRYPOINT` 中完成配置展开、权限切换、数据库初始化或漏洞环境准备；直接覆盖它会破坏场景。接入时应把原入口及参数显式放到 SysArmor 入口之后，并逐镜像验证，不能把基础镜像测试结果等同于所有 Vulhub 镜像兼容。

容器内 root 可以 kill Agent。入口会把这种情况转化为容器失败，配合 restart policy 恢复；这不等于防篡改。针对容器内高权限攻击者的强保护仍应部署在宿主机侧。
