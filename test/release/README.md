# 公开发行包容器测试

该目录验证公开 standalone 发行包在 Ubuntu 22.04、Ubuntu 24.04 和 Debian 12 中的真实安装与运行链路。默认测试覆盖 Agent/Tetragon 健康、Event、内置 Signal 和 `namespace/self` 隔离；设置 `RESTART_TEST=1` 时额外验证 Agent 异常退出后的容器级恢复。

```bash
make -C test/release doctor
make -C test/release test
```

缺省会查询 GitHub 最新 pre-release 的 `install.sh`。也可以固定版本，保证测试可复现：

```bash
make -C test/release test \
  URL=https://github.com/PKU-ASAL/sysarmor/releases/download/<tag>/install.sh
```

开发中的未发布构建可以通过本地 HTTP 地址测试，但这不替代公开链接验收。正式发布验收必须显式传入
新 tag 的完整 `URL`，避免误测其他 pre-release。测试结果保存在 `test/.results/release/<run-id>/`。

测试参数集中在 `config.sh`。可以通过 Make 参数覆盖镜像、攻击脚本和重启验证：

```bash
make -C test/release test \
  URL=https://github.com/PKU-ASAL/sysarmor/releases/download/<tag>/install.sh \
  IMAGES="ubuntu2204 debian12" \
  ATTACK_SCRIPT=/path/to/attack.sh \
  RESTART_TEST=0 \
  FRESH_DOWNLOAD=1
```

`FRESH_DOWNLOAD=1` 为默认值，会使用 `docker build --no-cache`，确保每次正式验收都重新执行公开下载。仅在调试测试脚本时使用 `FRESH_DOWNLOAD=0` 复用本地镜像层。

默认流程不包含 Agent 重启；`RESTART_TEST=1` 仅用于单独验证重启后的序列恢复。GitHub 直连失败时默认尝试 `https://gh-proxy.org`，也可以通过 `RELEASE_PROXY_URL=` 禁用或指定其他代理。

`run.sh` 只负责容器生命周期和场景调度；`attacks/` 中的脚本负责制造行为；`assert.sh` 通过容器内 `sysarmorctl` 查询 Event/Signal，并使用 `jq` 验证关联关系与 namespace 隔离。

## 运行条件

容器必须使用 `--privileged --cgroupns=host`，并挂载宿主机 BTF 与 bpffs。测试按镜像串行执行，避免多个 Tetragon 实例竞争同一宿主机的 eBPF 资源。
生产运行还应配置 `--restart unless-stopped` 或等价编排策略；设置 `RESTART_TEST=1` 后，测试会 kill Agent，确认入口使容器失败退出，再显式重启并验证恢复。

## Vulhub 边界

容器入口会原样执行 Dockerfile 的业务命令，因此可以包装已有服务入口。但 Vulhub 镜像经常在原 `ENTRYPOINT` 中完成配置展开、权限切换、数据库初始化或漏洞环境准备；直接覆盖它会破坏场景。接入时应把原入口及参数显式放到 SysArmor 入口之后，并逐镜像验证，不能把基础镜像测试结果等同于所有 Vulhub 镜像兼容。

容器内 root 可以 kill Agent。入口会把这种情况转化为容器失败，配合 restart policy 恢复；这不等于防篡改。针对容器内高权限攻击者的强保护仍应部署在宿主机侧。
