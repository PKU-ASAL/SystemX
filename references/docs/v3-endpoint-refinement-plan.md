# SysArmor v3 Endpoint Refinement Plan

Date: 2026-06-16
Status: draft

## 1. Goal

这一阶段的目标是把 sysarmor-agent 从 **observe-only endpoint MVP** 推进成一个 concrete、可控制、可验证的 **EDR Sensor Runtime**。

阶段完成后,我们希望可以通过 `sysarmorctl` 直接完成端侧控制闭环。这个阶段先采用 **local-manager mode**: `sysarmorctl` 通过本地 gRPC over Unix Domain Socket 直连 agent,扮演本地 manager。未来 manager / Agent Gateway 复用同一套 proto/control frames,只替换 transport。

```text
sysarmorctl
  -> grpc+unix:///var/run/sysarmor/agent.sock
  -> sysarmor-agent local control service
  -> agent applies runtime policy / response command
  -> agent produces local event / signal / evidence / health
  -> sysarmorctl can query or watch the result on the fly
```

本阶段优先级不是继续扩展云端复杂分析,而是把端侧做扎实:

- agent 稳定管理 Tetragon / sensor runtime。
- agent 按 collection policy 采集内核事件。
- agent 按 detection policy 生成本地 signal。
- agent 按 response policy 处理 observe/enforce command。
- agent 按 resource / upload policy 调节本地行为。
- ctl 能下发控制、查询状态、实时观察本地 event/signal。

## 2. Design Principles

1. **Endpoint first**
   - EDR 的可信根在端侧。
   - 云端可以异步分析,但端侧必须先做到稳定可控、可观测。

2. **Resource names are domain nouns**
   - `sysarmorctl` 的 resource 不揉入作用域。
   - 用 `policy --agent-id ...`,不用 `agent-policy`。
   - 用 `signal --agent-id ...`,不用 `agent-signal`。

3. **Control contract first**
   - policy schema 先分层设计完整。
   - 每个 section 都要定义 `applied / rejected / unsupported / requires_restart / failed`。
   - 能力可以分阶段实现,但契约不能含糊。

4. **Observe by default**
   - response/enforce 默认 observe-only。
   - 真实阻断必须经过 policy、scope、权限、审批和审计。

5. **Sensor is not the whole agent**
   - Tetragon 是当前 sensor backend。
   - kill/block/quarantine 等动作可以由 agent 本地 executor 完成,不强行塞进 Tetragon backend。

6. **Collection policy aligns with Tetragon, but is not raw Tetragon**
   - SysArmor CollectionPolicy 是产品契约。
   - Tetragon TracingPolicy 是当前 backend target。
   - compiler 负责把 SysArmor policy 编译成 Tetragon DSL。

7. **Local first, manager compatible**
   - 本阶段 `sysarmorctl` 先直连 agent local control service。
   - local control transport 使用 gRPC over Unix Domain Socket。
   - proto message 必须保留 tenant / agent / scope / request_id,不能做成本机特化协议。
   - 后续 manager 通过 Agent Gateway stream 转发同一套 control frames。

8. **Provenance tags, not endpoint graph**
   - agent 不维护完整 provenance graph,也不在端侧做多 terminal 图搜索。
   - agent 必须给 Event / Signal 打稳定的 lineage、entity、scope、event_refs 标记。
   - agent 用轻量 CEP runtime 维护窗口状态和短序列,不是引入端侧图数据库。
   - 云端负责长期图重建、跨主机/跨 workload stitching、结构收敛和 Incident 裁决。

## 3. Final Expected Experience

### Agent State

本阶段默认使用 local agent socket:

```bash
export SYSARMOR_AGENT_SOCK=/var/run/sysarmor/agent.sock
```

```bash
sysarmorctl agent list --tenant-id default --output table
sysarmorctl agent get --tenant-id default --agent-id agent-a --output json
sysarmorctl agent health --tenant-id default --agent-id agent-a --output json
sysarmorctl agent capability --tenant-id default --agent-id agent-a --output json
```

应能看到:

- identity: tenant / agent / host。
- runtime scope: host / container / cgroup / namespace / pod。
- sensor backend and version。
- supported policy sections。
- supported collection behaviors and selector capability matrix。
- supported response actions。
- current policy id / version / mode。
- health: running / policy_loaded / events_seen / dropped / parse_errors / spool / upload。

### Policy Control

```bash
sysarmorctl policy current \
  --tenant-id default \
  --agent-id agent-a \
  --output yaml

sysarmorctl policy apply \
  --tenant-id default \
  --agent-id agent-a \
  --type agent-runtime \
  --file runtime-policy.yaml \
  --output json

sysarmorctl policy apply collection \
  --tenant-id default \
  --agent-id agent-a \
  --behavior process.exec \
  --behavior network.connect \
  --file-prefix /dev/shm \
  --file-prefix /var/lib/app/plugins \
  --socket-family AF_INET \
  --scope-type container \
  --scope-selector <container-id> \
  --output json

sysarmorctl policy apply detection \
  --tenant-id default \
  --agent-id agent-a \
  --endpoint-rule payload_dropped@1 \
  --endpoint-rule reverse_shell_pattern@1 \
  --output json

sysarmorctl policy apply upload \
  --tenant-id default \
  --agent-id agent-a \
  --batch-size 256 \
  --flush-interval 1s \
  --spool-max-bytes 268435456 \
  --output json
```

每次变更都应返回 `ControlAck`。

### Local Event And Signal Watch

```bash
sysarmorctl event watch \
  --tenant-id default \
  --agent-id agent-a \
  --behavior process.exec \
  --output json

sysarmorctl signal watch \
  --tenant-id default \
  --agent-id agent-a \
  --where endpoint \
  --output json

sysarmorctl signal list \
  --tenant-id default \
  --agent-id agent-a \
  --since 5m \
  --rule-id payload_dropped \
  --output table
```

本阶段 `event watch` / `signal watch` 优先表示 **agent local stream / recent buffer**,不是 OpenSearch 全局 search。

### Response / Enforce

第一阶段先做安全动作:

```bash
sysarmorctl response create \
  --tenant-id default \
  --agent-id agent-a \
  --action collect_evidence \
  --target process:p123 \
  --mode observe \
  --reason "triage terminal signal" \
  --output json
```

后续再开启真实阻断:

```bash
sysarmorctl response create \
  --tenant-id default \
  --agent-id agent-a \
  --action kill_process \
  --target pid:1234 \
  --mode enforce \
  --output json
```

agent 必须校验 tenant / agent / scope / policy / approval,并回传 response ack。

## 4. sysarmorctl Resource Model

统一语法:

```text
sysarmorctl <resource> <verb> [selector flags] [input flags] [output flags]
```

主资源:

```text
agent
policy
rule
event
signal
incident
evidence
response
```

运维/debug 资源:

```text
session
metric
```

| Resource | Purpose |
|---|---|
| `agent` | inventory、capability、health、runtime state |
| `policy` | platform policy 和 agent runtime policy 的 version/apply/current/assign |
| `rule` | rule metadata/content pack |
| `event` | 客观事实,可按 agent/scope 查询或 watch |
| `signal` | 安全判断,可按 agent/scope/rule 查询或 watch |
| `incident` | 告警单元、lifecycle、investigation entry |
| `evidence` | evidence graph/timeline/pullback result |
| `response` | response command、approval、ack、audit |
| `session` | Agent Gateway session/cursor/downlink state |
| `metric` | metrics、lag、resource、health summary |

推荐 verb:

| Verb | Meaning | Examples |
|---|---|---|
| `list` | 查询集合 | `agent list`, `response list`, `incident list` |
| `get` | 查询单个对象 | `agent get`, `response get`, `incident get` |
| `watch` | 流式观察 | `event watch`, `signal watch` |
| `current` | 查询当前生效配置 | `policy current` |
| `capability` | 查询能力 | `agent capability` |
| `apply` | 应用声明式配置 | `policy apply` |
| `create` | 创建命令/请求 | `response create`, `evidence create` |
| `approve` | 审批控制动作 | `response approve` |
| `ack` | 回写执行结果,通常由 agent 使用 | `response ack` |
| `delete` | 删除或撤销 | `policy delete`, `session delete` |
| `publish` | 发布版本 | `policy publish` |
| `assign` | 绑定 policy 到 scope/agent | `policy assign` |
| `revoke` | 撤销绑定/证书/命令 | `policy revoke`, `agent revoke` |

通用 selector flags:

```text
--tenant-id
--agent-id
--host-id
--scope-type
--scope-selector
--policy-id
--policy-version
--rule-id
--incident-id
--evidence-id
--response-id
--request-id
```

通用输出/控制 flags:

```text
--agent-sock /var/run/sysarmor/agent.sock
--output json|table|yaml
--since 5m
--limit 100
--timeout 30s
```

规则:

- 查询类使用 `list/get/current/capability/watch`。
- 变更类使用 `apply/create/approve/delete/publish/assign/revoke`。
- agent/scope/tenant 永远用 selector flags 表达。
- local mode 默认读取 `SYSARMOR_AGENT_SOCK` 或 `/var/run/sysarmor/agent.sock`。
- 所有变更命令都返回结构化 ack。
- watch 命令输出 newline-delimited JSON,便于管道消费。

### Naming Migration

现有 flat command 保留兼容一段时间,新文档和新 e2e 使用资源/动作风格。

```text
agents                  -> agent list
agent-health            -> agent health
effective-policy        -> policy current 或 policy effective
responses               -> response list
signals                 -> signal list 或 signal search
events                  -> event list 或 event search
agent-gateway-sessions  -> session list
metrics                 -> metric get
```

## 5. Event / Signal / Incident / Evidence

这四个对象必须严格区分。

### Event

Event 是客观事实,回答:

```text
发生了什么?
```

例子:

```text
process exec /bin/bash
file write /dev/shm/x.sh
socket connect 10.66.0.99:443
read serviceaccount token
```

特点:

- 来自 sensor / normalizer。
- 尽量客观。
- 不一定危险。
- 数量最大。
- 是 signal / incident 的原材料。
- 在 SysArmor 产品模型里,`CanonicalEvent` 就是正式 Event: 它不是 Tetragon raw JSON,而是 sensor-neutral 的统一事实。
- 每条 Event 只携带建图和回查所需的最小稳定标记,大上下文通过 `raw_ref`、evidence pullback 或云端 enrichment 解决。

Event 必须逐步补齐的 provenance tags:

```text
tenant_id
agent_id
host_id
scope.type
scope.selector
container_id / cgroup / namespace / pod
occurred_at / mono_ns
event_id
lineage_id
subject process stable_id
parent process stable_id
object entity key
raw_ref
```

这些字段的目标不是让端侧保存一张图,而是让云端能可靠重建图。序列化原则是只放短小 join key,不把完整 raw event、完整环境变量、完整 K8s metadata 或大对象塞进每条 Event。

### Signal

Signal 是安全判断,回答:

```text
这个事实或一组事实有什么安全含义?
```

例子:

```text
payload_dropped
reverse_shell_pattern
web_runtime_spawns_shell
sensitive_cred_read
```

特点:

- 由 endpoint 或 cloud rule 产生。
- 带 rule id/version、risk、entities、event refs、lineage、response intent。
- 比 event 少。
- 是 incident 的候选证据。
- endpoint Signal 由轻量 CEP / rule runtime 产生,可以引用多个 Event。
- `event_refs` 必须是稳定 event id,不是展示字符串。
- Signal 应携带 `entity_refs`、`context_refs`、`ioc_refs`,使云端能把它作为 terminal anchor 继续建图。

### Incident

Incident 是面向人的告警单元,回答:

```text
这是不是一条值得处理的攻击故事?
```

特点:

- 由 signal / evidence / graph 收敛而来。
- 有 severity、status、timeline、evidence graph、response history。
- 有 lifecycle: open / triaged / suppressed / closed。
- 数量应远少于 signal。

### Evidence

Evidence 是支撑 incident 的证据材料,回答:

```text
凭什么说这是这个 incident?
```

例子:

```text
process:p1 -> wrote file:/dev/shm/x.sh
process:p2 -> connected socket:10.66.0.99:443
raw event refs
lineage graph
collected process tree
file metadata
```

特点:

- 可以来自 event/signal,也可以来自 agent pullback。
- 支撑 incident,但不等于 incident。
- 可以是 graph、timeline、raw refs、file/process/network metadata。

本阶段 evidence 的端侧边界:

- agent 只产出 evidence seed 和本地回查结果。
- evidence seed 主要由 `event_refs`、`raw_refs`、`entity_refs`、`lineage_id`、`scope` 组成。
- 完整 evidence graph 由云端根据 Event / Signal / Entity 重建。
- `sysarmorctl` 应能根据 Signal 的 `event_refs` 回查对应 Event,用于本地验证。

### Endpoint Provenance Boundary

端侧能力边界固定为:

```text
SensorEvent
  -> CanonicalEvent with provenance tags
  -> lightweight CEP window state
  -> Signal(event_refs, entity_refs, lineage_id, scope, content refs)
  -> local recent store / spool / upload
```

端侧可以做:

- lineage 维护。
- 最近事件缓冲。
- 按 lineage / process / scope 分组的轻量窗口状态。
- sequence / expr 类短序列检测。
- terminal signal 标记。
- event_refs / evidence seed 生成。

端侧不做:

- 长期 provenance graph。
- 任意路径搜索。
- Steiner Tree / hopset 构造。
- 跨主机或跨 workload stitching。
- 最终 Incident 裁决。

## 6. Agent Runtime Contract

端侧统一抽象:

```text
AgentRuntime
  identity
  capability
  scope
  collection
  normalization / provenance tags
  detection
  response
  resource
  upload
  health
  local event/signal stream
  control ack
```

### Capability

agent hello / health 应上报:

```text
tenant_id
agent_id
host_id
version
scope.type
scope.selector
sensor.backend
sensor.version
supported_policy_sections
collection_behaviors
supported_collection_selectors
supported_response_actions
supports_enforce
supports_hot_reload
requires_restart_sections
supported_event_fields
supported_entity_kinds
supported_cep_features
```

### ControlAck

所有控制操作都应返回:

```text
request_id
tenant_id
agent_id
policy_id
policy_version
section
status: applied | rejected | unsupported | requires_restart | failed
message
observed_at
```

ctl 不应该只能靠查日志判断控制是否成功。

## 7. Agent Local Control Transport

本阶段先不依赖 manager 完成端侧 runtime 控制。`sysarmorctl` 作为本地 manager,通过 agent 暴露的本地控制服务交互。

第一版主路径只实现 gRPC over Unix Domain Socket:

```text
grpc+unix:///var/run/sysarmor/agent.sock
```

这样做的原因:

- 权限边界清晰,可以依赖文件 ownership/mode 控制本机访问。
- 不暴露网络监听端口,默认攻击面更小。
- 和 systemd / rootful agent / node-local debug 习惯匹配。
- `sysarmorctl` 与未来 manager 使用同一套 proto/control frames,只替换 transport。

未来可以预留 dev/debug TCP transport,但它不是第一版必须实现的主路径:

```text
grpc+tcp://127.0.0.1:<port>
```

TCP debug listener 的规则:

- 默认关闭,生产默认不启用。
- 必须通过显式配置开启,例如 `agent.control.debug_tcp_enabled: true`。
- 默认只能 bind loopback: `127.0.0.1` 或 `::1`。
- 非 loopback bind 必须要求额外危险开关,例如 `--insecure-debug-listen`,并产生醒目的审计日志。
- 即使是 debug TCP,也仍然走同一套 control auth、policy check 和 audit 语义。
- 常规 `sysarmorctl` 示例和 e2e 只使用 `--agent-sock`; 如后续实现 TCP,可单独提供 debug-only 参数,例如 `--debug-agent-addr 127.0.0.1:<port>`。

### AgentControlService

本地 gRPC service 应表达稳定 runtime 能力:

```text
AgentControlService
  Health(HealthRequest) returns (HealthResponse)
  Capability(CapabilityRequest) returns (CapabilityResponse)
  CurrentPolicy(CurrentPolicyRequest) returns (CurrentPolicyResponse)
  ApplyPolicy(ApplyPolicyRequest) returns (ControlAck)
  ExecuteResponse(ExecuteResponseRequest) returns (ResponseAck)
  PullEvidence(PullEvidenceRequest) returns (EvidenceResult)
  WatchEvents(WatchEventsRequest) returns (stream EventFrame)
  WatchSignals(WatchSignalsRequest) returns (stream SignalFrame)
```

所有 request 都必须保留:

```text
request_id
tenant_id
agent_id
scope.type
scope.selector
```

即使当前是本地 ctl,也不能省略这些字段。这样后续 manager / Agent Gateway 可以直接复用协议。

### Transport Boundary

```text
sysarmorctl local mode
  -> AgentControlService over Unix Domain Socket
  -> RuntimeController

manager mode, later
  -> Agent Gateway stream
  -> same control frames
  -> RuntimeController
```

协议语义必须独立于 transport:

```text
ControlRequest / ControlAck
EventFrame / SignalFrame
ResponseCommand / ResponseAck
EvidencePullbackRequest / EvidenceResult
Health / Capability
```

### Local Security

Unix socket 权限是本地安全边界:

```text
/var/run/sysarmor/agent.sock
owner: root
group: sysarmor
mode: 0660
```

原则:

- 默认只允许 root 或 `sysarmor` group 控制 agent。
- 所有 destructive response action 仍需 response policy 授权,不能只靠本地 socket 权限。
- local ctl 也要产生 ControlAck / audit event,后续可同步到 manager。

## 8. AgentRuntimePolicy

新增端侧 runtime policy:

```yaml
metadata:
  policy_id: runtime-policy-a
  version: 1
  tenant_id: default
  mode: observe
scope:
  type: host | container | cgroup | namespace | pod
  selector: ""
collection:
  ...
detection:
  ...
response:
  ...
resource:
  ...
upload:
  ...
```

### Collection Policy

Collection policy 控制 sensor runtime 采什么。详细设计见 [collection-policy-design.md](collection-policy-design.md)。

核心约定:

- Collection behavior 必须是客观采集行为,例如 `process.exec`、`process.fork`、`process.exit`、`network.connect`、`file.open`、`file.write`。
- `sensitive_read`、`c2_connect`、`payload_drop`、`apt_activity` 这类语义属于 detection/rule/context/IOC,不属于 collection behavior。
- selector 必须绑定到具体 behavior,不再长期使用全局平铺的 `file_prefixes/socket_addrs` 模型。
- ContextSet 和 IOCPack 独立版本化,collection policy 通过 `ctx:` / `ioc:` 引用它们。
- Tetragon TracingPolicy 是当前 backend target,不是 SysArmor 的公共 policy API。

目标形态:

```yaml
collection:
  backend: tetragon
  observe_only: true
  behaviors:
    - id: process.exec
      enabled: true
      selectors:
        binary:
          prefixes: ["/bin/", "/usr/bin/", "/tmp/", "/dev/shm/"]
          prefix_refs: ["ctx:managed-runtime-binary-prefixes"]
        parent:
          binary_prefixes: []
    - id: network.connect
      enabled: true
      selectors:
        socket:
          families: ["AF_INET", "AF_INET6"]
          addr_refs: ["ioc:c2-ip-feed"]
          cidr_refs: ["ioc:c2-cidr-feed"]
          port_refs: ["ioc:c2-port-feed"]
          exclude_cidrs: ["127.0.0.0/8", "::1/128"]
    - id: file.open
      enabled: true
      selectors:
        file:
          prefix_refs: ["ctx:credential-path-prefixes", "ctx:secret-volume-prefixes"]
        access:
          read: true
          write: false
```

第一阶段直接使用 `behaviors + behavior-scoped selectors` 作为产品路径,不保留 `profiles/kinds` 兼容入口。

### Detection Policy

Detection policy 控制哪些检测内容生效。详细设计见 [detection-policy-design.md](detection-policy-design.md)。

核心约定:

- RuleSet / RulePack 是可发布、可版本化的检测内容包。
- DetectionPolicy 是应用策略,负责启用哪些 ruleset、应用到哪个 scope、以什么 mode 运行、做哪些 rule override。
- ContextSet / IOCPack 独立版本化,detection policy 通过 `ctx:` / `ioc:` 引用它们。
- rule 必须声明 requires,agent apply policy 时要检查 collection policy 是否满足检测所需输入。
- rule 产出 Signal,不直接创建 Incident。

目标形态:

```yaml
detection:
  mode: observe
  rulesets:
    - ref: ruleset:endpoint-linux-baseline
      version: 2026.06.17
      enabled: true
    - ref: ruleset:credential-access
      version: latest
      enabled: true
  rule_overrides:
    - rule_id: reverse_shell_pattern
      mode: observe
      severity: critical
      response_intent:
        action: collect_evidence
        confidence: 80
    - rule_id: noisy_admin_tool_exec
      enabled: false
      reason: expected admin workflow
  context_refs:
    - ref: ctx:credential-path-prefixes
      version: latest
    - ref: ctx:trusted-admin-binaries
      version: 4
  ioc_refs:
    - ref: ioc:c2-ip-feed
      version: latest
```

第一阶段不再复用旧 fastpath 产品路径,而是建立 builtin RuleSet-backed detection engine,并让 agent 运行时直接消费 `rulesets + rule_overrides`。

### Endpoint CEP Runtime

端侧 detection runtime 的正式定位是轻量 CEP:

```text
CanonicalEvent stream
  -> group by lineage_id / process stable_id / scope
  -> evaluate expr / sequence within bounded window
  -> emit Signal with event_refs / entity_refs / content refs
```

CEP runtime 第一阶段能力:

- `expr`: 单事件字段条件,例如 binary、argv、file.path、socket.addr、socket.port、uid。
- `sequence`: 多事件短序列,例如 write -> chmod -> exec -> connect。
- `within`: 窗口上限,例如 30s / 5m。
- `by`: 分组键,例如 lineage_id、process.stable_id、scope。
- `all/any/not`: 简单组合条件。
- `emit`: 生成 Signal,并携带参与匹配的 event_refs。

CEP runtime 必须资源有界:

- 最大 active lineage/entity 数。
- 每个 lineage 最大状态条目数。
- 每个 signal 最大 event_refs 数。
- 状态 TTL。
- 内存水位和降级策略。
- health 中暴露 evicted_state、dropped_refs、cep_eval_errors。

不引入端侧 provenance graph cache。recent event store 和 entity touch cache 只服务于 rule window、Signal refs 和本地 evidence 回查。

### Response Policy

```yaml
response:
  allowed_actions:
    - noop
    - collect_evidence
    - kill_process
    - block_ip
    - quarantine_file
  allowed_modes:
    - observe
    - enforce
  allow_destructive: false
  approval_required: true
```

第一阶段实现 `noop`、`collect_evidence`、observe-only ack、unsupported ack。真实阻断后续 gated enable。

### Resource Policy

```yaml
resource:
  max_cpu_percent: 10
  max_memory_bytes: 268435456
  max_parse_errors: 100
  max_dropped_events: 1000
  event_rate_limit_per_sec: 1000
  recent_event_buffer_size: 10000
  max_active_lineages: 4096
  max_events_per_lineage: 128
  cep_state_ttl: 5m
  backpressure:
    mode: drop_oldest | drop_new | pause_collection
```

第一阶段先实现 health/applied-policy 回显、recent buffer/CEP 状态上限和可安全热更新字段,不承诺完整 cgroup 隔离。

### Upload Policy

```yaml
upload:
  batch_size: 256
  flush_interval: 1s
  spool_max_bytes: 268435456
  retry_initial: 1s
  retry_max: 30s
  request_timeout: 10s
```

不可热更新字段必须返回 `requires_restart`。

## 9. Collection Compiler

Collection policy 可以和 Tetragon 采集 DSL 对齐,但不直接暴露为 Tetragon YAML。完整设计见 [collection-policy-design.md](collection-policy-design.md)。

推荐分层:

```text
SysArmor CollectionPolicy
  + ContextSet
  + IOCPack
  -> backend compiler
  -> Tetragon TracingPolicy
  -> tetra tracingpolicy add/delete/list
```

### Tetragon Target Mapping

| SysArmor CollectionPolicy | Tetragon TracingPolicy |
|---|---|
| `tetragon.hooks[].type: kprobe` | `spec.kprobes[]` |
| `tetragon.hooks[].type: tracepoint` | `spec.tracepoints[]` |
| `tetragon.hooks[].type: lsm` | `spec.lsmhooks[]` |
| `tetragon.hooks[].type: uprobe` | `spec.uprobes[]` |
| `hooks[].args` | `args[]` |
| `selectors.match_args` | `selectors.matchArgs[]` |
| `selectors.match_return_args` | `selectors.matchReturnArgs[]` |
| `selectors.match_data` | `selectors.matchData[]` |
| `selectors.match_binaries` | `selectors.matchBinaries[]` |
| `selectors.match_parent_binaries` | `selectors.matchParentBinaries[]` |
| `selectors.match_pids` | `selectors.matchPIDs[]` |
| `selectors.match_namespaces` | `selectors.matchNamespaces[]` |
| `selectors.match_namespace_changes` | `selectors.matchNamespaceChanges[]` |
| `selectors.match_capabilities` | `selectors.matchCapabilities[]` |
| `selectors.match_capability_changes` | `selectors.matchCapabilityChanges[]` |
| `selectors.match_workloads` | `selectors.matchWorkloads` |
| `selectors.actions` | `selectors.matchActions[]` |
| `selectors.return_actions` | `selectors.matchReturnActions[]` |
| `lists` | `spec.lists[]` |
| `selector_macros` | `spec.selectorsMacros` |

Compiler requirements:

- support kprobe、tracepoint、lsmhook、uprobe。
- support args、returnArg、matchArgs、matchReturnArgs、matchData。
- support binary、parent binary、PID、namespace、capability、workload selectors。
- support selector macros and lists。
- support Post / NoPost actions。
- support behavior-scoped selectors。
- support `ctx:` ContextSet refs and `ioc:` IOCPack refs。
- split selectors into kernel/Tetragon pushdown and agent-side evaluation。
- explicitly report unsupported/degraded selectors in ControlAck。
- destructive actions such as Signal / Sigkill / Override must be gated by response policy and disabled by default。
- render deterministic generated policy name。
- apply new policy, verify it, then delete old generated policy。
- rollback on failure。
- return ControlAck。

### Behavior Capability Matrix

第一批 behaviors:

| Behavior | Purpose |
|---|---|
| `process.exec` | 进程启动 |
| `process.fork` | 进程 fork/clone |
| `process.exit` | 进程退出 |
| `network.connect` | 外联 |
| `file.open` | 文件打开,可配合 path/access selector |
| `file.write` | 文件落地 |
| `file.chmod` | 权限修改 |
| `capability.use` | 权限能力使用 |
| `capability.change` | 权限变化 |
| `namespace.change` | namespace escape / setns |
| `module.load` | 内核模块加载 |

每个 behavior capability 定义:

```text
behavior
supported_backends
required_capabilities
default_hooks
default_args
default_selectors
adapter_mapping
output_schema
```

Raw Tetragon policy 可作为高级 escape hatch,但不能作为默认产品路径:

```yaml
collection:
  backend: tetragon
  raw_tetragon_policy_ref: advanced-policy-v1
```

它必须版本化、审计、受权限控制,失败时返回 rejected/failed。

## 10. Implementation Phases

### Phase 1: Local Control Protocol And CLI Surface

目标:

- 定义 AgentRuntimePolicy。
- 定义 ControlAck。
- 定义 AgentControlService proto。
- agent 暴露 gRPC over Unix Domain Socket。
- sysarmorctl 支持 `--agent-sock` 直连 agent。
- 扩展 agent capability/health。
- `sysarmorctl agent capability`、`policy current`、`policy apply --file` 可用。

验收:

```bash
sysarmorctl --agent-sock /var/run/sysarmor/agent.sock agent capability --agent-id agent-a --output json
sysarmorctl --agent-sock /var/run/sysarmor/agent.sock policy current --agent-id agent-a --output json
```

### Phase 2: Collection Policy Compiler

目标:

- CollectionPolicy validate/compile/apply/verify/ack。
- Tetragon compiler 支持 behaviors + behavior-scoped selectors + tetragon target。
- collection policy hot reload。

验收:

```bash
sysarmorctl policy apply collection \
  --agent-id agent-a \
  --behavior network.connect \
  --file-prefix /dev/shm \
  --output json
```

触发 `network.connect` 后可见事件;禁用 behavior 后新事件不再进入本地流。

### Phase 3: Canonical Event Provenance Tags And Local Watch

目标:

- 补齐 `CanonicalEvent` 的最小 provenance tags: tenant、scope、container/cgroup/namespace/pod、occurred_at、entity keys。
- agent 提供 local recent event store / stream。
- ctl 可 watch event/signal,并能按 event id 回查 recent event。
- signal 带 rule id/version、event refs、entities、lineage、scope、content refs。
- recent store 有容量、TTL、health 指标和超限降级语义。

验收:

```bash
sysarmorctl event watch --agent-id agent-a --behavior process.exec --output json
sysarmorctl event get --agent-id agent-a --event-id <event-id> --output json
sysarmorctl signal watch --agent-id agent-a --where endpoint --output json
```

触发 payload drop / reverse shell 后,ctl 能 on the fly 看到本地 signal;对 signal.event_refs 自动回查后,能保存对应 Event。

### Phase 4: Detection Policy And Lightweight CEP Runtime

目标:

- endpoint rules 从 string list 升级为 `DetectionPolicyV2`。
- 引入 builtin RuleSet,承载当前端侧 builtin detection 规则。
- detection engine 支持 rule id/version/enabled/severity/mode/response intent。
- 建立轻量 CEP runtime contract,支持 `expr`、`sequence`、`within`、`by`。
- 支持 rule overrides。
- 支持 ContextSet / IOCPack 引用的最小 resolver。
- 暂不推进 manager 下发,先通过 `sysarmorctl` 作为 local manager 下发 RulePack / ContextSet / IOCPack。
- agent local control 暴露 content apply/list/get。
- apply detection policy 时检查 rule dependencies 与 collection policy 是否匹配,从 event kind 级逐步升级到 field/capability 级。
- unsupported rule 返回 ControlAck。
- CEP 状态有 TTL、容量上限、eviction 指标和 degraded 标记。

验收:

禁用 `payload_dropped` 后,新事件不再生成该 signal;重新启用后恢复。

启用需要 `network.connect` 的规则,但当前 collection policy 未采集 `network.connect` 时,agent 返回 `degraded` 并说明缺失 behavior。

通过本地 content 路径下发内容包:

```bash
sysarmorctl content apply \
  --agent-sock /var/run/sysarmor/agent.sock \
  --file ioc-c2-feed.json \
  --allow-unsigned \
  --output json

sysarmorctl content list --kind iocpack --output json
sysarmorctl content get --ref ioc:c2-ip-feed --output json
```

后续 signal 必须携带 `rule_version`、`ruleset_ref`、`context_refs`、`ioc_refs`。

通过本地 content 路径下发一个 sequence rule:

```text
within 60s by lineage_id:
  file.write /tmp|/dev/shm
  -> file.chmod same path
  -> process.exec same path
  -> network.connect
```

触发后 Signal 必须引用参与匹配的多个 Event。

### Phase 5: Response Observe / Safe Action

目标:

- agent response executor 从 sensor.Enforce 抽象中拆出来。
- 实现 `noop` 和 `collect_evidence`。
- observe-only、unsupported、requires_approval 语义稳定。

验收:

```bash
sysarmorctl response create \
  --agent-id agent-a \
  --action collect_evidence \
  --target process:p1 \
  --mode observe \
  --output json
```

能看到 ack、evidence result、audit。

### Phase 6: Real Enforce MVP

目标:

- 在 policy allowlist + scope match + approval 后支持有限真实动作。
- 初始动作:
  - `kill_process`
  - `block_ip`
  - `quarantine_file`

验收:

默认 destructive action 被拒绝;显式授权后执行,并有结果、审计和失败降级。

### Phase 7: Resource / Upload / CEP State Policy

目标:

- 将本地 config 中可热更新的 resource/upload 参数接入 policy。
- 不可热更新参数返回 `requires_restart`。
- health 展示 applied resource/upload policy。
- resource policy 控制 recent event buffer、active lineage、events per lineage、CEP TTL、backpressure。
- 超限时 agent 明确上报 dropped_events、evicted_state、dropped_refs、cep_degraded。

验收:

ctl 修改 batch size / flush interval / CEP TTL / recent buffer size 后,新行为生效或返回明确 `requires_restart`。

### Phase 8: Identity And Trust

目标:

- 接入 agent-manager mTLS。
- agent certificate 绑定 tenant / agent / scope。
- dev token 只保留为测试模式。

验收:

证书身份和上报 agent identity 不匹配时,Gateway 拒绝连接。

## 11. Test Plan

本阶段先走 local-manager path:

```text
sysarmorctl
  -> grpc+unix:///var/run/sysarmor/agent.sock
  -> sysarmor-agent RuntimeController
  -> sensor / detection engine / spool / response executor
```

随后再接入真实平台路径:

```text
Tetragon
  -> sysarmor-agent
  -> local normalize
  -> local endpoint signal
  -> local spool
  -> Agent Gateway
  -> Kafka
  -> worker
  -> Postgres / OpenSearch
  -> sysarmorctl query/watch
```

需要补齐:

- local gRPC over Unix Domain Socket control e2e。
- sysarmorctl local agent capability / health / policy current。
- collection policy hot reload。
- Tetragon compiler behavior/selector coverage。
- local event watch。
- local event get by event id。
- local signal watch。
- signal refs backfill: `signal watch --include-recent --limit 200` 后自动回查 event_refs。
- CEP sequence rule with multiple event_refs。
- CEP state resource limit / eviction / degraded behavior。
- response collect evidence。
- destructive action policy deny。
- destructive action explicit allow。
- upload policy hot update。
- resource/backpressure policy。
- mTLS identity mismatch。
- agent restart 后 applied policy / cursor / spool 行为。

## 12. Current Gaps

当前已有:

- agent-managed Tetragon MVP。
- Tetragon event normalize。
- endpoint detection signal。
- local spool / retry / resume。
- health / tamper signal。
- effective policy fetch / endpoint rule refresh。
- response observe-only skeleton。

主要缺口:

- Tetragon/backend enforce 当前仍是 unsupported。
- agent response 执行结果被强制 observe-only。
- collection policy 热更新和 compiler 仍需扩大到 behavior-scoped selector、scope selector、ctx/ioc refs 和字段能力报告。
- collection compiler 只覆盖最小 CONNECT/file prefix 子集,还不能保证 detection rule 所需字段都被采集。
- `CanonicalEvent` 还缺 tenant/scope/container/cgroup/namespace/pod 等完整 provenance tags。
- Event/Signal 的 entity refs 还偏少,process/file/socket/container/user 等 join key 需要稳定化。
- local recent event store 还不是正式 evidence retention contract,缺 `event get --event-id` 和 signal refs 自动回查闭环。
- detection policy 已从 string list 走向 RuleSet/RulePack,但 runtime 仍主要是 builtin 状态机,还不是正式轻量 CEP contract。
- RulePack 还没有通用 `expr/sequence/within/by` 执行能力。
- rule dependency check 仍偏 event kind 级,缺 field/capability/context/ioc 粒度。
- CEP/lineage state 缺 TTL、容量上限、eviction 指标和 resource policy 联动。
- resource/upload policy 仍主要来自本地 config。
- local gRPC control service 和 sysarmorctl UDS 直连已在推进,仍需按资源模型补齐完整命令面和 e2e。
- mTLS agent identity 还没接入。

## 13. Non-goals

本阶段不做:

- 完整云端 worker 拆分。
- 完整通用 CEP 平台或云端复杂流处理引擎。
- 端侧长期 provenance graph、Steiner Tree、hopset 或大规模路径搜索。
- 默认启用危险阻断。
- 完整 UI。
- 大规模多租户商业权限系统。

## 14. Success Criteria

阶段完成时,我们应能明确说:

```text
sysarmor-agent 是一个可被控制面管理的 EDR Sensor Runtime。
它能采集、能给 Event/Signal 打稳定 provenance tags,
能用轻量 CEP 产出带 event_refs/entity_refs 的本地 signal,
能通过 refs 回查端侧证据,能可靠上传,能接收 runtime policy,
能返回控制 ack,能执行安全 response action,
并能通过 sysarmorctl 实时观察端侧状态、本地 event 和本地 signal。
```
