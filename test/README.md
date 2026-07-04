# SysArmor 测试怎么测

结论：SysArmor 的测试可以先记三句话：

1. **产品功能测试**：系统通不通。
2. **检测效果测试**：抓得准不准。
3. **性能测试**：端侧成本高不高。

更细的目录、输出、报告口径和完整 target 清单见 [DETAILS.md](DETAILS.md)。

## 三类测试

| 测试类型 | 关心的问题 | 典型命令 |
|---|---|---|
| `product` | 产品功能和链路是否正常 | `make -C test product-*` |
| `effectiveness` | 检测是否有效、误报是否可控 | `make -C test effectiveness-topology` |
| `performance` | agent/sensor 占多少 CPU/RSS | `make -C test performance-endpoint` |

通俗理解：

```text
product       系统有没有接通
effectiveness 攻击能不能报，正常行为会不会误报
performance   agent/sensor 在宿主机上花多少 CPU 和内存
```

## 三种环境

| 环境 | 长什么样 | 用来测什么 |
|---|---|---|
| `container` | 本机 Docker compose | 快速验证 manager/gateway/worker/Kafka/Postgres 等平台链路 |
| `vm-endpoint` | 单台 VM，只装 agent/sensor | 端侧检测和 CPU/RSS 结论 |
| `vm-topology` | 三台 VM：`mgr`、`node-a`、`attacker` | 完整产品链路和攻击场景 |

直观来看：

```text
container:
  本机 Docker 里跑平台组件，快，适合平台功能冒烟。

vm-endpoint:
  一台被保护主机 node-a，只关注 agent/sensor 自己。
  这是评估端侧 CPU/内存的主环境。

vm-topology:
  mgr      = manager/gateway/worker/数据库/消息队列
  node-a   = 被保护主机，跑 agent/sensor
  attacker = 攻击辅助/C2
  用来验证真实 agent -> gateway -> worker -> manager 链路。
```

## 推荐顺序

新人或日常回归可以按这个顺序理解和运行：

```bash
make -C test product-platform-full
make -C test product-topology
make -C test performance-endpoint SYSARMOR_BENCH_PROFILE=quick
make -C test effectiveness-topology
```

含义是：

1. `product-platform-full`：在 container 环境确认 gateway、Kafka、worker、manager 能串起来。
2. `product-topology`：在三 VM 环境确认 agent 通过 mTLS 接入 gateway，数据最终能被 manager 查询。
3. `performance-endpoint`：在单 VM 上采 agent/sensor CPU/RSS。`quick` 是短测，`medium` 是中等长度，`long` 是长窗口。
4. `effectiveness-topology`：在三 VM 环境跑攻击/良性场景，看 event、signal、incident 是否符合预期。

## 性能测什么

性能测试不是简单看一眼 `top`，而是按阶段采样：

```text
startup      启动和策略应用
steady       空闲保护状态
workload     正常业务负载
activity     攻击/场景执行
persistence  场景后观察
overall      整体
```

主要看：

```text
agent CPU 平均值、最大值
sensor CPU 平均值、最大值
agent RSS 内存
sensor RSS 内存
event/signal 数量
```

它回答的是：SysArmor agent 平时占多少资源，业务负载下占多少，攻击发生时会不会飙高，攻击结束后会不会持续高。

## 检测效果测什么

检测效果测试会跑攻击和良性场景，例如：

```text
apt-fileless-c2
apt-staged-drop
benign-ci-noise
```

它检查：

- 预期 event 有没有出现；
- 预期 signal 有没有出现；
- 是否生成 incident；
- evidence 是否能关联；
- 良性场景是否没有误报；
- manager 查询接口能不能看到结果。

一句话：不只是 agent 本地看到了事件，还要证明事件能走完整产品链路，最终在 manager 侧可查询、可解释。

## 常用入口

启动环境：

```bash
make -C test up-container
make -C test up-vm-endpoint
make -C test up-vm-topology
```

停止环境：

```bash
make -C test down-container
make -C test down-vm-endpoint
make -C test down-vm-topology
make -C test down-all
```

产品功能：

```bash
make -C test product-platform
make -C test product-platform-full
make -C test product-endpoint
make -C test product-topology
```

性能：

```bash
make -C test performance-endpoint SYSARMOR_BENCH_PROFILE=quick
make -C test performance-endpoint SYSARMOR_BENCH_PROFILE=medium
make -C test performance-endpoint SYSARMOR_BENCH_PROFILE=long
```

检测效果：

```bash
make -C test effectiveness-topology
```

## 一句话速记

```text
product 测“系统有没有接通”
effectiveness 测“检测准不准”
performance 测“端侧成本高不高”

container 快，适合平台功能
vm-endpoint 准，适合 agent 性能
vm-topology 真，适合完整链路和攻击场景
```

真正做结论时：

- 端侧 CPU/内存结论看 `performance-endpoint`
- 检测准确性看 `effectiveness-topology`
- 产品链路健康看 `product-*`
- 测完统一 `make -C test down-all` 清理环境
