# Test Taxonomy Design

## 目的

统一 SysArmor 测试体系的命名口径，使分类名称表达测试关注点，而不是混合产品范围、运行环境和发布阶段。

本设计只规范测试分类及其边界。现有测试实现、环境和数据契约原则上复用，后续通过兼容入口渐进迁移。

## 分类

| 英文名称 | 中文名称 | 回答的问题 |
|---|---|---|
| Unit | 基础逻辑 | 独立代码单元和模块边界是否正确 |
| Functional | 产品功能 | 组件和产品链路是否按契约工作 |
| Detection | 检测质量 | 恶意行为是否检出、良性行为是否避免误报 |
| Performance | 资源成本 | 给定环境和负载下的 CPU、内存、吞吐和丢弃情况如何 |
| Distribution | 发行包和安装兼容性 | 可交付制品能否构建、校验、安装、启动和升级 |
| Release | 发布门禁 | 当前候选版本是否满足约定的发布条件 |

前五项是测试关注点。Release 不是与前五项并列的测试 Suite，而是按发布阶段聚合所需测试的门禁。

## 边界

### Unit

Unit 包含本地 Go 单元测试和聚焦模块测试。它不依赖 VM、容器产品拓扑或公开发行制品。

模块 benchmark 不属于 Unit，通过 Performance 管理。

### Functional

Functional 继承当前 Product Suite 的职责，按产品边界保留 Endpoint、Platform 和 Topology：

```text
Functional
├── Endpoint
├── Platform
└── Topology
```

它验证安装后的组件行为、健康状态、控制面和数据面链路、注册、证书及恢复能力，但不输出检测准确率或长期资源结论。

### Detection

Detection 继承当前 Effectiveness Suite 的职责。它使用 Policy、Workload、Scenario、truth labels、Event 和 Signal 判断检测结果。

Incident 可作为诊断证据采集；只有评分器实际消费的字段才构成自动门禁。

### Performance

Performance 保留 Endpoint、Platform 和 Module 三个成本面，分别验证真实端侧成本、平台服务成本和局部算法成本。

短时 smoke 只验证性能测试接线，不形成稳定性能结论。

### Distribution

Distribution 收纳当前散落在 Product Endpoint、`test/release/` 和发布工作流中的制品测试，分为两类：

```text
Distribution
├── Package     当前源码生成的本地签名制品
└── Published   已发布到指定公开地址的制品
```

Package 验证包结构、签名、标准路径、安装 profile、升级和事务行为。Published 验证用户实际下载入口及支持系统镜像中的安装、启动和基础运行兼容性。

Distribution 不负责决定版本能否发布，只向 Release 门禁提供结果。

### Release

Release 是聚合门禁，不拥有重复的测试实现：

```text
Release Candidate Gate
  = Unit
  + required Functional
  + required Detection
  + selected Performance baseline
  + Distribution Package

Published Release Gate
  = Distribution Published
```

具体门禁清单由发布流程维护，并显式记录未纳入门禁的测试范围。

## 现有映射

| 当前分类或入口 | 目标分类 |
|---|---|
| `test-unit`、`go test ./...` | Unit |
| `test/suites/product/endpoint` 中的产品行为测试 | Functional / Endpoint |
| `test/suites/product/platform` | Functional / Platform |
| `test/suites/product/topology` | Functional / Topology |
| `test/suites/effectiveness` | Detection |
| `test/suites/performance` | Performance |
| Endpoint 包结构、安装、升级和事务测试 | Distribution / Package |
| `test/release/` 公开地址多镜像测试 | Distribution / Published |
| `.github/workflows/release-*.yml` 中的测试组合 | Release gates |

单个脚本只归属一个主要关注点。若一个入口同时验证产品行为和制品兼容性，应拆分断言或明确选择主要归属，避免重复执行同一条重型链路。

## 环境与输入

Local、container、vm-endpoint 和 vm-topology 是运行环境，不是测试分类。Policy、Workload、Scenario 和 Content 是测试输入，也不是测试分类。

环境和默认输入由具体测试入口声明。只有 Detection、Performance 等矩阵测试才向调用者公开输入组合参数，Functional 使用稳定的内部 fixture。

## 入口原则

公共入口遵循 `test-<classification>[-<scope>]`：

```text
test-unit
test-functional-endpoint
test-functional-platform
test-functional-topology
test-detection
test-performance
test-distribution-package
test-distribution-published
test-release-candidate
test-release-published
```

旧入口在迁移期作为兼容别名，并输出废弃提示。兼容入口不得形成第二套参数默认值。

`make release` 继续表示构建发行制品；`make test-release-*` 表示发布门禁，二者不得混用。

## 结果契约

所有分类继续将生成结果写入 `test/.results/`。每次正式运行至少提供可机器读取的运行元数据和最终状态，记录分类、范围、环境、制品版本、输入、Git 提交和运行 ID。

历史结果文档不因目录或入口迁移而修改。

## 非目标

- 不建立任意维度组合的通用测试矩阵编排器。
- 不要求所有分类接受相同参数。
- 不把运行环境、制品来源或生命周期提升为新的顶层分类。
- 不在命名迁移中重写已经通过验收的测试场景。
- 不一次性删除旧入口；删除前必须确认零引用且已有替代入口。

## 成功标准

1. 文档、Make 入口和 CI 对六个术语使用一致。
2. 现有测试均能唯一映射到 Unit、Functional、Detection、Performance 或 Distribution。
3. Release 仅聚合测试结果，不复制测试实现。
4. 公开入口不再使用 Product 或 Effectiveness 作为分类名称。
5. 兼容期内旧入口与新入口执行相同实现和相同默认值。
6. 历史结果文档保持不动。
