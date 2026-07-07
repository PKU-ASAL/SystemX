# Product Topology

结论：`product-topology` 验证三 VM 下真实产品接入链路。它不再是 fake sensor smoke，而是通过 manager artifact registry、channel、enrollment 和自动证书签发，从 `mgr` 分发并安装真实 `sysarmor-agent` 到 `node-a`。

## VM Topology

```text
mgr manager/artifact/channel/enrollment -> node-a agent install
node-a agent -> gateway -> Kafka -> worker -> manager/store
```

| VM | 角色 |
|---|---|
| `mgr` | manager、gateway、worker、Postgres、Kafka、Redis、OpenSearch |
| `node-a` | 被保护主机；从 manager enrollment 下载并安装 agent |
| `attacker` | C2/恶意脚本/攻击辅助，主要用于 effectiveness topology |

## 运行入口

```bash
make -C test product-topology
```

当前流程：

```text
1. 启动 vm-topology platform。
2. 将当前构建的 sysarmor-agent 和 Tetragon bundle 打成 signed distribution tar.gz。
3. 在 mgr 上通过 manager artifacts upload 上传为 active artifact。
4. 通过 manager channels upsert 绑定 `topology-test -> artifact_id`。
5. 通过 manager enrollments create 绑定 channel。
6. node-a 从 install_url 下载 bootstrap，校验 manifest/signature，安装 agent，并通过 CSR 申请 agent mTLS 证书。
7. 验证 manager 能查询 agent health、agent session、artifact、channel、enrollment。
8. 验证 systemd 能重启 agent。
```

主要产物：

```text
test/.results/e2e-agent-systemd-vm.artifact.json
test/.results/e2e-agent-systemd-vm.channel.json
test/.results/e2e-agent-systemd-vm.enrollment.json
test/.results/e2e-agent-systemd-vm.channels.json
test/.results/e2e-agent-systemd-vm.health.json
test/.results/e2e-agent-systemd-vm.sessions.json
test/.results/e2e-agent-systemd-vm.systemd.txt
test/.results/e2e-agent-systemd-vm.journal.txt
```

## 边界

`product-topology` 证明：

```text
signed artifact -> channel -> enrollment -> node-a bootstrap -> CSR certificate issuance -> systemd agent -> mTLS gateway/manager access
```

它不证明：

```text
Tetragon kernel telemetry
attack detection recall/precision
signal/incident quality
manager/gateway/worker resource profile
```

真实检测效果看：

```bash
make -C test effectiveness-topology \
  ENV=vm-topology \
  POLICIES='test/data/policies/collection-balanced.json' \
  WORKLOADS='business-normal' \
  SCENARIOS='apt-fileless-c2'
```

## 部署缓存

`vm-topology` 的部署输入缓存位于：

```text
test/environments/vm-topology/deploy/
```

- `platform/` 保存轻量平台部署包；
- `images/` 保存可复用 Docker 镜像包和 manifest；
- `test/.results/` 只保存本次测试输出。
