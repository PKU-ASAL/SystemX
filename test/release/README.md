# 公开发行包容器测试

该目录验证公开 standalone 发行包在 Ubuntu 22.04、Ubuntu 24.04 和 Debian 12 中的真实安装与运行链路。测试覆盖 Agent/Tetragon 健康、Event、内置 Signal、`namespace/self` 隔离，以及 Agent 异常退出后的容器级恢复。

```bash
make -C test/release doctor
make -C test/release test
```

缺省会查询 GitHub 最新 pre-release 的 `install.sh`。也可以固定版本，保证测试可复现：

```bash
make -C test/release test \
  SYSARMOR_INSTALL_URL=https://github.com/PKU-ASAL/sysarmor/releases/download/<tag>/install.sh
```

开发中的未发布构建可以通过本地 HTTP 地址测试，但这不替代公开链接验收。正式发布验收必须显式传入
新 tag 的完整 `SYSARMOR_INSTALL_URL`，避免误测其他 pre-release。测试结果保存在 `test/.results/release/<run-id>/`。

## 运行条件

容器必须使用 `--privileged --cgroupns=host`，并挂载宿主机 BTF 与 bpffs。测试按镜像串行执行，避免多个 Tetragon 实例竞争同一宿主机的 eBPF 资源。
生产运行还应配置 `--restart unless-stopped` 或等价编排策略；测试会 kill Agent，确认入口使容器失败退出，再显式重启并验证恢复。

## Vulhub 边界

容器入口会原样执行 Dockerfile 的业务命令，因此可以包装已有服务入口。但 Vulhub 镜像经常在原 `ENTRYPOINT` 中完成配置展开、权限切换、数据库初始化或漏洞环境准备；直接覆盖它会破坏场景。接入时应把原入口及参数显式放到 SysArmor 入口之后，并逐镜像验证，不能把基础镜像测试结果等同于所有 Vulhub 镜像兼容。

容器内 root 可以 kill Agent。入口会把这种情况转化为容器失败，配合 restart policy 恢复；这不等于防篡改。针对容器内高权限攻击者的强保护仍应部署在宿主机侧。
