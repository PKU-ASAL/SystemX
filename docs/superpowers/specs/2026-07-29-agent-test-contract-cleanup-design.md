# Agent 测试契约清理设计

## 目的

以当前 Agent 配置解析器和统一安装器为唯一事实来源，清理仓库内可执行配置、诊断脚本和 E2E 入口中的过期实现，防止测试资产继续使用已删除的身份、传输、策略路径和构建契约。

历史结果与发布记录不在本次范围内。

## 原则

- 不修改 Agent 运行时、统一安装器标准或产品行为。
- 仍有有效调用方且没有等价替代的入口原地更新。
- 只有零有效调用方且存在明确替代覆盖的旧入口才删除。
- 临时策略路径只有在脚本直接运行 Agent 时保留；经过统一安装器的配置必须指向安装目标。
- 新增仓库级契约测试，阻止已删除模式重新进入当前测试资产。

## 删除范围

| 对象 | 原因 | 替代覆盖 |
| --- | --- | --- |
| `test/shared/diagnostics/capture-container.sh` | 依赖已移除的 Tetragon sidecar，并使用已禁用的直接云配置 | `test/suites/product/topology/e2e-scenarios-container.sh` |
| `test/suites/product/endpoint/e2e-real-tetragon-owned-container.sh` | 无有效执行入口，并使用旧的直接云配置 | `test/suites/product/endpoint/e2e-namespace-self-container.sh` 与 topology 场景 |
| `test/suites/product/platform/e2e-gateway-local-ingest.sh` | 旧配置文件直连 Gateway 路径已由 enrollment 模型替代 | `test/suites/product/platform/e2e-agent-gateway-manager-local.sh` |
| `configs/agent.fake.yaml` | 零引用且当前解析器无法加载 | `configs/agent.example.yaml` 与 `test/fixtures/agent/configs/fake.yaml` |
| `sa_build_all` | 零调用且仍调用语义已改变的 `make build` | 按需使用 `sa_build_go_bins` 或 `make build-binary` |

同步删除上述入口在 `test/Makefile`、覆盖清单和仅服务于被删脚本的契约断言中的引用。

## 更新范围

### VM 启动

`test/shared/harness/start-vm.sh` 使用 `make build-binary` 构建全部 Go 二进制，不再调用需要 `SERVICE` 参数的容器服务构建目标 `make build`。

### VM 诊断

`test/shared/diagnostics/capture-vm.sh`：

- 仅通过 `agent.label.scenario` 设置场景标签，不再写云身份字段。
- 删除 `manager.transport: local`。
- 调用开发安装器时显式提供 Agent、CTL 和内容签名三个二进制。
- 配置 `policy.path` 为统一安装目标 `/etc/sysarmor/agent/policy.json`。
- 删除安装后重复复制 CTL 的步骤。
- 安装失败时输出 `/tmp/sysarmor-install-agent.log` 后显式失败。

### Endpoint VM E2E

`test/suites/product/endpoint/e2e-real-tetragon-owned-vm.sh` 与 VM 诊断采用相同安装契约：

- 使用场景标签而非云身份字段。
- 显式传入 CTL 二进制。
- 使用标准策略安装路径。
- 安装失败时输出安装日志。

测试命令中的 `--agent-id` 和 `--tenant-id` 仅作为控制命令兼容参数保留，不再作为配置身份来源。

## 防回归契约

扩展 `internal/contracts/schema/agent_test_assets_test.go`，扫描当前测试资产并拒绝：

- YAML `agent` 段中的 `id`、`host_id`、`tenant_id` 和 `token`。
- YAML `manager` 段及旧的 `manager.transport: local`。
- `sensor.policy_path`。
- VM harness 对无 `SERVICE` 参数的 `make build` 调用。
- 开发安装器调用遗漏 `SYSARMOR_CTL_BIN`。

扫描跳过 `.results`、VM deployment 复制目录以及历史结果文档。对需要临时路径直接运行 Agent 的测试不限制 `policy.path`；只验证经过安装器的 VM 脚本使用标准安装目标。

## 验收

按以下顺序执行：

1. 新增或扩展契约测试，确认旧实现下测试失败。
2. 完成最小删除和更新后，确认契约测试通过。
3. 运行 `go test ./...`。
4. 运行 `make -C test product-platform`，确认替代平台链完整。
5. 运行 `make -C test product-endpoint`。
6. 运行 `make -C test product-topology`。
7. 清理由本轮测试生成的 `.results`、VM deployment 复制目录和其他未跟踪生成物；保留测试虚机生命周期所需的受控状态，除非清理脚本明确要求销毁。

## 提交边界

- `test: remove obsolete agent test paths`：删除旧入口、引用和覆盖清单记录。
- `fix(test): align vm tests with agent install contract`：更新 VM harness、诊断、Endpoint E2E 及契约测试。

每个提交独立通过其直接相关的契约测试，最终完整验收结果基于两者合并后的工作树。
