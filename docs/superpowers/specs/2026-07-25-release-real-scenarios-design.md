# Release 真实业务多规则测试设计

## 目的

将 `test/release` 从合成 marker 冒烟测试升级为基于真实 Node.js 业务、真实进程父子关系、真实文件下载和真实网络连接的 Release 端到端测试。测试覆盖三个 Linux 用户态发行版，并验证以下内置检测规则：

- `web_runtime_spawns_shell`
- `download_by_lolbin`
- `payload_lifecycle`

本次同时移除 Release 测试中的 Agent 重启逻辑，并保证相同 `RUN_ID` 重跑不会混入旧证据。

## 范围

### 包含

- Ubuntu 22.04、Ubuntu 24.04、Debian 12 容器中的公开 Release 安装。
- 无第三方 npm 依赖的 Node.js HTTP 业务夹具。
- 同一 Docker 网络中的本地攻击服务器。
- 三个独立真实攻击场景及其 Event/Signal 关联断言。
- 真实父进程身份驱动的 Web Runtime Shell 检测。
- 兄弟容器和宿主机 namespace 隔离验证。
- `RUN_ID` 校验及旧结果目录清理。
- 对应单元测试、Shell 契约测试和 Release E2E 验证。

### 不包含

- 发布工作流自动运行真实矩阵。
- VM 或发行版原生内核矩阵。
- ARM64、systemd profile、升级和卸载测试。
- Agent 异常退出及编排器自动重启验证。
- 默认检测规则全集覆盖。

## 目录与职责

```text
test/release/
├── fixtures/
│   ├── web-app/server.js
│   └── payload-server/server.js
├── attacks/
│   ├── web-runtime-shell.sh
│   ├── download-by-lolbin.sh
│   └── payload-lifecycle.sh
├── scenarios.sh
├── assert.sh
├── config.sh
└── run.sh
```

- `fixtures/web-app` 提供健康检查和三个攻击入口。
- `fixtures/payload-server` 在 `8080` 提供 payload，在 `8443` 接收控制连接。
- `attacks` 只负责调用一个业务入口并传入唯一 marker。
- `scenarios.sh` 定义场景名、预期规则、严重级别和必需 Event 行为。
- `assert.sh` 通过 `sysarmorctl` 查询并验证 Signal 及全部关联 Event。
- `run.sh` 只负责镜像、网络、容器、场景调度、证据收集和清理。

## 真实业务与攻击链

### Web Runtime 启动 Shell

测试请求 Node 服务的 `/rce`。Node 使用 `child_process.spawn` 直接启动 `/bin/sh`。Shell 参数包含 marker，但不包含 `node`、`nginx` 等伪造 Runtime token。

检测引擎记录已观察进程的 `stableId -> binary`，处理 Shell exec Event 时通过 `parentStableId` 解析直接父进程，确认父进程二进制为 Node.js Runtime。删除通过 Shell argv substring 推断 Web Runtime 的路径，并增加伪造 argv 不触发的负向测试。

该方案要求 Agent 观察到 Node 启动事件。Release 容器由 SysArmor entrypoint 先启动 Agent、再启动业务，因此满足条件。Agent 启动前既有进程的父元数据恢复不在本次范围内。

### LOLBin 下载

测试请求 `/download`。Node 启动真实 `/usr/bin/curl`，从攻击服务器 `8080` 下载带 marker 的资源。`8080` 是默认 download IOC 端口。

断言 `download_by_lolbin` Signal 为 `medium`，关联 Event 包含 `network.connect`，目标端口为 `8080`，进程二进制为 curl，argv 或 URL 中包含 marker。

### Payload 生命周期

测试请求 `/payload`，形成以下链路：

1. curl 从攻击服务器 `8080` 下载脚本到 `/tmp/.sysarmor-attack/<marker>`。
2. `/bin/sh` 执行该 payload。
3. payload 使用 curl 连接攻击服务器 `8443`，该端口是默认 control IOC 端口。

断言 `payload_lifecycle/high` Signal 的关联 Event 至少覆盖 `file.write`、`process.exec` 和 `network.connect`，不存在缺失 Event 引用，并能通过 marker 将证据绑定到本次场景。该场景不依赖 `file.chmod`。

## 容器生命周期

每个发行版串行执行：

1. 构建包含 Node 业务与攻击服务器夹具的 Release 测试镜像。
2. 创建该发行版专属 Docker 网络。
3. 使用同一镜像和覆盖 entrypoint 的方式启动攻击服务器。
4. 使用 Release 安装的 SysArmor entrypoint 启动 Node 业务容器。
5. 等待 Agent/Tetragon 健康和 Node `/healthz` 就绪。
6. 顺序运行三个攻击场景并保存独立结果。
7. 在兄弟容器和宿主机执行真实进程命令，验证 `namespace/self` 隔离。
8. 保存业务日志、攻击服务器日志、容器 inspect 和健康信息。
9. 删除容器和专属网络。

测试保持串行，避免多个 Tetragon 实例竞争宿主机 bpffs。

## 断言策略

每个场景使用唯一 marker。正向断言必须同时满足：

- Signal 的 `ruleId` 和 `severity` 符合场景定义。
- `missingEventRefs` 为空。
- Signal 返回的 `eventFrames` 覆盖该规则要求的全部 Event 行为。
- 关联 Event 的进程 argv、文件路径或网络请求中能够找到本次 marker。
- 网络规则额外验证目标端口。

隔离断言使用轮询观察窗口，而不是固定 sleep 后单次查询。窗口结束前只要发现外部 marker 即失败；窗口结束仍未发现才通过。

## 重启逻辑移除

完整删除以下内容：

- Makefile 的 `RESTART_TEST` 参数。
- `config.sh` 的重启配置。
- `doctor.sh` 的重启参数校验。
- `run.sh` 的 kill/start/recovery 分支。
- `assert.sh` 的 `stopped-nonzero` 模式。
- README 和静态契约中的相关说明。

入口进程退出和信号转发继续由 `test/suites/product/endpoint/container-entrypoint.sh` 负责。

## 结果目录与清理

`RUN_ID` 必须匹配 `[A-Za-z0-9][A-Za-z0-9._-]*`。测试启动后，在确认结果路径严格位于 `test/.results/release/` 下，再删除同名旧目录并创建空目录。

每个发行版保存：

- `build.log`
- `health.json` 与错误输出
- `business.log`
- `attacker.log`
- `container-inspect.json`
- 三个场景各自的 Signal/Event JSONL 与错误输出
- `events-sibling.jsonl`
- `events-host.jsonl`

失败清理只写入本轮容器和日志，不保留上一轮文件。

## 错误处理与安全边界

- 所有外部参数在使用前校验。
- 容器、网络和结果路径均由已校验的 `RUN_ID` 构造。
- trap 清理业务容器、攻击服务器和 Docker 网络。
- 攻击服务只绑定测试专属 Docker 网络，不映射宿主机端口。
- payload 仅写入测试容器的 `/tmp/.sysarmor-attack`。
- 不访问公网攻击目标，Release 资产下载除外。

## 验证计划

1. 检测引擎单元测试：真实 Node 父子关系触发；argv 伪造不触发；非 Web Runtime 父进程不触发。
2. Node 夹具契约测试：健康接口、三个 endpoint、marker 校验和失败响应。
3. Shell 契约测试：三场景映射、关键 Docker 参数、无 `RESTART_TEST`、结果清理与 RUN_ID 校验。
4. Go 相关包测试。
5. Shell 语法和 `git diff --check`。
6. 固定公开 Release URL 的三个发行版真实 E2E。

## 验收标准

- 三个发行版均运行真实 Node 业务并通过三个规则场景。
- `web_runtime_spawns_shell` 不再依赖 argv 中的 Runtime token。
- 三条 Signal 均携带完整且符合行为要求的 Event 引用。
- 兄弟容器和宿主机 marker 未进入业务容器的 namespace/self 事件流。
- 仓库中不存在 `RESTART_TEST` 的 Release 测试逻辑。
- 同一 `RUN_ID` 连续运行不会保留上一轮独有文件。
- 所有相关自动化测试通过。
