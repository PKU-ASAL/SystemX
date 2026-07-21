# SysArmor 测试入口

正式测试方法、环境说明和结果判定见
[测试指南](../docs/development/testing.md)，测试数据契约见
[数据说明](data/README.md)。

从仓库根目录开始：

```bash
make test-doctor
make test-unit
make test-performance PROFILE=medium \
  WORKLOAD=business-normal \
  SCENARIO=apt-fileless-c2-local \
  POLICIES='test/data/policies/collection-balanced.json'
make test-help
```

测试实现位于 `suites/`，运行环境位于 `environments/`，复用机制位于
`shared/`，输入数据位于 `data/`。生成结果统一写入 `.results/`，不得提交。

`quick` 只验证性能测试链路；资源结论使用可比的 `medium` 或 `long` 运行。
Product 测试只证明产品链路可用，检测结论以 Effectiveness 测试为准。
