# SysArmor Detection Policy Design

Date: 2026-06-17
Status: draft

## 1. Positioning

Detection Policy 定义 **哪些检测内容在什么 scope 下生效、以什么模式运行、如何引用 context/IOC、如何产出 Signal**。

它不直接定义 sensor 采什么。采集由 Collection Policy 决定;检测消费已经归一化的 Event、ContextSet、IOCPack 和轻量状态,再产出 Signal。

一句话:

```text
Collection Policy answers: what facts should the sensor collect?
Detection Policy answers: which rule content explains those facts?
RuleSet / RulePack answers: what detection content exists?
Context / IOC answers: which objects are interesting right now?
Signal answers: what security judgment was produced?
```

Detection Policy 的核心设计目标:

- 规则内容可版本化、可发布、可回滚。
- policy 应用规则内容时可以按租户、agent、host、container、pod、namespace 做 scope 控制。
- detection 能引用 ContextSet / IOCPack,但不把大体量情报内容塞进 policy。
- agent 能检查 detection 所需输入是否被 collection policy 满足。
- signal 输出契约稳定,供本地 ctl、response、spool、manager、云端 analytics 复用。

## 2. Layering

推荐分层:

```text
DetectionPolicy
  mode
  scope
  ruleset_refs
  rule_overrides
  context_refs
  ioc_refs
  dependency checks

RuleSet / RulePack
  rule metadata
  rule logic refs
  rule dependencies
  default severity / mode / response intent
  supported runtimes

Rule
  id / version
  where: endpoint | cloud | xdr
  inputs: event behaviors / fields / context / ioc
  logic: builtin | expr | graph | external
  output: Signal contract

ContextSet
  stable environment knowledge

IOCPack
  threat intelligence content

DetectionEngine
  resolved policy + resolved rulesets + context/ioc snapshot
  -> process Event stream
  -> emit Signal
```

边界约定:

- RuleSet 是内容包,不等于运行策略。
- DetectionPolicy 只引用 RuleSet/Rule,并控制应用方式。
- rule logic 可以分阶段实现:先 builtin detection engine,后续再支持表达式/图规则/云端规则。
- ContextSet / IOCPack 可以同时被 collection 和 detection 引用,但两者使用目的不同。
- Incident 不由 endpoint rule 直接创建。rule 产出 Signal;Incident 由后续聚合、图收敛和生命周期逻辑创建。

## 3. Core Objects

### DetectionPolicy

DetectionPolicy 是“把哪些检测内容应用到哪里”的策略:

```yaml
detection:
  policy_id: endpoint-detection-default
  version: 1
  mode: observe
  scope:
    type: host
    selector: ""
  rulesets:
    - ref: ruleset:endpoint-linux-baseline
      version: 2026.06.17
      enabled: true
    - ref: ruleset:credential-access
      version: latest
      enabled: true
    - ref: ruleset:ioc-correlation
      version: latest
      enabled: true
      ioc_refs:
        - ioc:c2-ip-feed
        - ioc:malware-hash-feed
  rule_overrides:
    - rule_id: reverse_shell_pattern
      enabled: true
      mode: observe
      severity: critical
      response_intent:
        action: collect_evidence
        confidence: 80
    - rule_id: noisy_admin_tool_exec
      enabled: false
      reason: allowed in this environment
  context_refs:
    - ref: ctx:credential-path-prefixes
      version: latest
    - ref: ctx:trusted-admin-binaries
      version: 4
  ioc_refs:
    - ref: ioc:c2-ip-feed
      version: latest
```

### RuleSet / RulePack

RuleSet 是一组规则的发布单元:

```yaml
ruleset:
  id: ruleset:endpoint-linux-baseline
  version: 2026.06.17
  target:
    where: endpoint
    os: linux
  rules:
    - rule_id: payload_dropped
      version: 1
    - rule_id: reverse_shell_pattern
      version: 1
    - rule_id: suspicious_exec_connect
      version: 1
```

RulePack 可以作为更大的分发包,包含多个 RuleSet、默认 ContextSet 引用、兼容性声明和签名:

```yaml
rulepack:
  id: rulepack:sysarmor-endpoint-core
  version: 2026.06.17
  signature: ...
  rulesets:
    - ruleset:endpoint-linux-baseline@2026.06.17
    - ruleset:credential-access@2026.06.17
    - ruleset:ioc-correlation@2026.06.17
```

### Rule

Rule 是检测内容的最小单位:

```yaml
rule:
  rule_id: reverse_shell_pattern
  version: 1
  where: endpoint
  severity: critical
  enabled_by_default: true
  tags: [network, shell, c2]
  mitre: [T1059, T1571]
  runtime:
    type: builtin
    entrypoint: builtin.reverse_shell_pattern
  requires:
    events:
      - behavior: process.exec
        fields: [process.binary, process.argv, lineage_id]
      - behavior: network.connect
        fields: [process.binary, process.argv, socket.addr, socket.port, lineage_id]
    context:
      optional:
        - ctx:trusted-admin-binaries
    ioc:
      optional:
        - ioc:c2-ip-feed
  output:
    signal_name: reverse_shell_pattern
    terminal: true
    response_intent:
      action: collect_evidence
      confidence: 80
```

关键点:

- `rule_id + version` 是规则内容身份。
- `where` 决定规则运行在 endpoint、cloud 还是 xdr correlation。
- `runtime` 决定执行方式,早期可以是 builtin Go detection engine。
- `requires` 是 collection/detection 契约校验的基础。
- `output` 约束 Signal 的命名、风险、响应意图和证据引用。

## 4. Rule Overrides

`rule_overrides` 用于在不修改 RuleSet/RulePack 原始内容的情况下,对某个租户、agent 或 scope 做局部覆盖。

允许覆盖:

- `enabled`
- `mode`
- `severity`
- `scope`
- `response_intent`
- threshold / window / allowlist 这类显式参数

不建议覆盖:

- rule id。
- rule version。
- runtime type。
- 核心检测逻辑 AST / builtin entrypoint。
- MITRE / tags 等内容包元数据,除非作为 tenant-local annotation。

示例:

```yaml
rule_overrides:
  - rule_id: credential_file_read
    enabled: true
    severity: medium
    params:
      min_reads: 3
      window: 60s
  - rule_id: reverse_shell_pattern
    mode: observe
    severity: critical
    response_intent:
      action: collect_evidence
  - rule_id: noisy_admin_tool_exec
    enabled: false
    reason: expected admin workflow
```

这样可以保留上游 RuleSet 的可升级性,同时允许环境差异。

## 5. Context And IOC In Detection

Detection 和 Collection 都可以引用 ContextSet / IOCPack,但语义不同:

| Layer | Usage |
|---|---|
| Collection | 用 context/ioc 过滤采集范围,尽量减少噪声和资源占用 |
| Detection | 用 context/ioc 解释事件,降低误报、提高置信度、标记命中情报 |

### ContextSet

ContextSet 是环境知识,例如:

- `ctx:credential-path-prefixes`
- `ctx:trusted-admin-binaries`
- `ctx:managed-runtime-binary-prefixes`
- `ctx:production-workload-scopes`
- `ctx:approved-egress-destinations`

Detection 使用示例:

```text
file.open path in ctx:credential-path-prefixes
  AND process.binary not in ctx:trusted-admin-binaries
  -> signal credential_file_read
```

### IOCPack

IOCPack 是威胁情报,例如:

- `ioc:c2-ip-feed`
- `ioc:malware-hash-feed`
- `ioc:suspicious-domain-feed`
- `ioc:known-webshell-paths`

Detection 使用示例:

```text
network.connect dst in ioc:c2-ip-feed
  OR process.hash in ioc:malware-hash-feed
  -> signal ioc_match
```

设计约定:

- IOC 更新可以独立于 DetectionPolicy 更新。
- agent 应记录 signal 使用的 IOC/context snapshot version。
- 大体量 IOC 应有 TTL、版本、签名和增量更新机制。
- hash/domain/cert/signer 等通常更适合 detection/enrichment,不适合 collection kernel pushdown。

## 6. Dependency Check

DetectionPolicy apply 时,agent 应检查启用规则所需输入是否可用。

校验输入:

- 当前 CollectionPolicy enabled behaviors。
- sensor capability。
- event normalization capability。
- local context/ioc availability。
- rule runtime support。

可能结果:

| Status | Meaning |
|---|---|
| `applied` | 所有启用规则都满足 |
| `degraded` | policy 生效,但部分规则缺少输入或 context/ioc |
| `unsupported` | agent 不支持某些 rule runtime 或 required behavior |
| `rejected` | policy/schema/version 非法 |
| `requires_restart` | 需要重启 agent/sensor 才能完整应用 |
| `failed` | 应用过程失败 |

示例:

```text
rule reverse_shell_pattern requires network.connect,
but current collection policy does not collect network.connect.
```

ControlAck 应返回:

```json
{
  "status": "degraded",
  "policy_type": "detection",
  "message": "1 rule enabled but missing collection inputs",
  "details": [
    {
      "rule_id": "reverse_shell_pattern",
      "missing_behaviors": ["network.connect"]
    }
  ]
}
```

这是 DetectionPolicy 和 CollectionPolicy 联动的关键契约。

## 7. Local Content Package Protocol

本阶段暂不推进 manager。`sysarmorctl` 扮演 **local manager**,通过 gRPC over Unix Domain Socket 把内容包下发给本机 agent,用于验证 RuleSet / ContextSet / IOCPack 的完整交互协议。

目标不是把内容包永久绑定到本机 CLI,而是让本地路径和未来 manager 下发路径使用同一套 envelope:

```text
sysarmorctl
  -> ContentPackage envelope
  -> AgentControlService.ApplyContent
  -> agent verify signature / digest
  -> agent stage content snapshot
  -> rebuild EffectiveDetectionPolicy
  -> dependency check
  -> atomic switch
  -> ControlAck
```

### Package Types

| Type | Purpose |
|---|---|
| `rulepack` | 分发一个或多个 RuleSet / Rule |
| `contextset` | 分发稳定环境上下文 |
| `iocpack` | 分发威胁情报 |
| `content-bundle` | 一次性携带 rulepack + contextset + iocpack |

### Envelope

所有本地内容包都使用统一 envelope:

```json
{
  "api_version": "sysarmor.content/v1",
  "kind": "content-bundle",
  "metadata": {
    "id": "bundle:endpoint-local-lab",
    "version": "2026.06.17.1",
    "tenant_id": "default",
    "created_at": "2026-06-17T00:00:00Z",
    "ttl": "24h"
  },
  "spec": {
    "rulepacks": [],
    "contextsets": [],
    "iocpacks": []
  },
  "integrity": {
    "digest_alg": "sha256",
    "digest": "hex-or-base64",
    "signature_alg": "ed25519",
    "key_id": "local-dev-key",
    "signature": "base64"
  }
}
```

签名范围:

- `api_version`
- `kind`
- `metadata`
- `spec`

不包含 `integrity` 自身。签名前必须使用 canonical JSON,字段排序稳定。

本地开发可允许 `--allow-unsigned`,但 ack 必须明确返回:

```text
status=applied
section=content
message="unsigned content accepted by local debug policy"
```

默认产品行为应拒绝 unsigned content。

### RulePack File

```json
{
  "api_version": "sysarmor.content/v1",
  "kind": "rulepack",
  "metadata": {
    "id": "rulepack:sysarmor-endpoint-core",
    "version": "2026.06.17.1"
  },
  "spec": {
    "rulesets": [
      {
        "id": "ruleset:endpoint-linux-baseline",
        "version": "2026.06.17.1",
        "target": {
          "where": "endpoint",
          "os": "linux"
        },
        "rules": [
          {
            "rule_id": "reverse_shell_pattern",
            "version": 1,
            "enabled_by_default": true,
            "severity": "critical",
            "runtime": {
              "type": "builtin",
              "entrypoint": "builtin.reverse_shell_pattern"
            },
            "requires": {
              "events": [
                {
                  "behavior": "process.exec",
                  "fields": ["process.binary", "process.argv", "lineage_id"]
                },
                {
                  "behavior": "network.connect",
                  "fields": ["socket.addr", "socket.port", "lineage_id"]
                }
              ],
              "context": {
                "optional": ["ctx:trusted-admin-binaries"]
              },
              "ioc": {
                "optional": ["ioc:c2-ip-feed"]
              }
            },
            "output": {
              "signal_name": "reverse_shell_pattern",
              "terminal": true,
              "response_intent": {
                "action": "collect_evidence",
                "confidence": 80
              }
            }
          }
        ]
      }
    ]
  },
  "integrity": {}
}
```

### ContextSet File

```json
{
  "api_version": "sysarmor.content/v1",
  "kind": "contextset",
  "metadata": {
    "id": "ctx:credential-path-prefixes",
    "version": "2026.06.17.1"
  },
  "spec": {
    "value_type": "path_prefix",
    "merge_strategy": "replace",
    "values": [
      "/root/.ssh/",
      "/home/*/.ssh/",
      "/var/run/secrets/",
      "/run/secrets/"
    ]
  },
  "integrity": {}
}
```

### IOCPack File

```json
{
  "api_version": "sysarmor.content/v1",
  "kind": "iocpack",
  "metadata": {
    "id": "ioc:c2-ip-feed",
    "version": "2026.06.17.1",
    "ttl": "24h"
  },
  "spec": {
    "value_type": "ip_or_cidr",
    "merge_strategy": "replace",
    "values": [
      "203.0.113.10",
      "198.51.100.0/24"
    ]
  },
  "integrity": {}
}
```

### Incremental Update

增量更新使用同一个 envelope,但 `spec` 中声明 `base_version` 和 op list:

```json
{
  "api_version": "sysarmor.content/v1",
  "kind": "iocpack",
  "metadata": {
    "id": "ioc:c2-ip-feed",
    "version": "2026.06.17.2"
  },
  "spec": {
    "base_version": "2026.06.17.1",
    "value_type": "ip_or_cidr",
    "merge_strategy": "patch",
    "ops": [
      {"op": "add", "value": "192.0.2.44"},
      {"op": "remove", "value": "203.0.113.10"}
    ]
  },
  "integrity": {}
}
```

agent apply 规则:

- 如果本地没有 `base_version`,返回 `rejected` 或要求 full snapshot。
- patch 必须幂等。
- patch 应生成新的 resolved snapshot digest。
- DetectionEngine 只读取 resolved snapshot,不直接读 patch log。

### sysarmorctl Local Manager Commands

推荐命令:

```bash
sysarmorctl content apply \
  --agent-sock /var/run/sysarmor/agent.sock \
  --tenant-id default \
  --agent-id agent-a \
  --file content-bundle.json \
  --allow-unsigned \
  --output json

sysarmorctl content list \
  --agent-sock /var/run/sysarmor/agent.sock \
  --tenant-id default \
  --agent-id agent-a \
  --kind iocpack \
  --output table

sysarmorctl content get \
  --agent-sock /var/run/sysarmor/agent.sock \
  --tenant-id default \
  --agent-id agent-a \
  --ref ioc:c2-ip-feed \
  --output json

sysarmorctl content diff \
  --file old-iocpack.json \
  --file new-iocpack.json \
  --output json
```

对应 agent control API:

```text
ApplyContent(ContentApplyRequest) returns ControlAck
ListContent(ContentListRequest) returns ContentListResponse
GetContent(ContentGetRequest) returns ContentGetResponse
```

第一阶段也可以复用 `ApplyPolicy(policy_type=content, policy_json=<envelope>)`,但 proto 上建议保留独立 content RPC,避免 policy 和 content 生命周期混在一起。

### Local State

agent 本地维护 content store:

```text
/var/lib/sysarmor/content/
  rulepack/
    rulepack_sysarmor-endpoint-core/2026.06.17.1.json
  contextset/
    ctx_credential-path-prefixes/2026.06.17.1.json
  iocpack/
    ioc_c2-ip-feed/2026.06.17.1.json
  snapshots/
    effective-detection/default/agent-a.json
```

写入流程:

1. write temp file。
2. verify digest/signature。
3. validate schema。
4. apply full/patch。
5. write resolved snapshot。
6. rebuild detection engine。
7. atomic rename current pointer。
8. emit ControlAck。

### Test Flow

本地验证路径:

```bash
sysarmorctl content apply --file ctx-credential-paths.json --allow-unsigned
sysarmorctl content apply --file ioc-c2-feed.json --allow-unsigned
sysarmorctl content apply --file rulepack-endpoint-core.json --allow-unsigned

sysarmorctl policy apply detection --file detection-policy.json
sysarmorctl signal watch --include-recent --limit 50 --output json
```

验收点:

- content apply 返回 digest/version。
- detection apply 能解析 ruleset/context/ioc refs。
- 缺失 context/ioc 返回 `degraded`。
- signal 输出 `rule_version/ruleset_ref/context_refs/ioc_refs`。
- ioc patch 更新后,不改 DetectionPolicy 也能影响后续 signal。

## 8. Signal Contract

Rule 输出 Signal。Signal 是安全判断,不是 Incident。

Signal 必须至少带:

- `signal_id`
- `rule_id`
- `rule_version`
- `ruleset_ref`
- `where`
- `severity`
- `risk`
- `confidence`
- `mode`
- `lineage_id`
- `scope`
- `event_refs`
- `entities`
- `context_refs`
- `ioc_refs`
- `response_intent`
- `evidence_seed`

要求:

- 单个 Signal 可以引用多个 Event。
- `event_refs` 必须是稳定 event id,不是展示字符串。
- 如果 rule 使用 context/ioc,Signal 应记录命中的 ref/version。
- terminal signal 可以带 response intent,但是否执行由 Response Policy 决定。
- Incident 创建属于云端或本地聚合层,不属于单条 rule 的职责。

## 9. Runtime Model

第一阶段不需要马上做通用 DSL。推荐运行时分层:

```text
DetectionPolicyResolver
  -> resolve rulesets/rules/overrides/context/ioc
  -> build EffectiveDetectionPolicy

DetectionCompiler
  -> validate dependencies
  -> choose endpoint rule runtime
  -> produce DetectionProgram

DetectionEngine
  -> consume CanonicalEvent
  -> maintain O(active lineage/entity) state
  -> emit Signal
```

Rule runtime 类型:

| Runtime | Usage |
|---|---|
| `builtin` | 当前 Go detection engine 规则,适合端侧低延迟规则 |
| `expr` | 后续轻量表达式规则,适合单事件/短窗口判断 |
| `sequence` | 多事件序列和 lineage 状态 |
| `graph` | 云端图分析规则 |
| `external` | 高级扩展或沙箱化规则,谨慎使用 |

端侧原则:

- 端侧维护轻量状态,复杂图分析上云。
- 端侧 rule 必须有资源预算。
- 端侧 rule 失败不能阻塞采集主路径。
- 规则热更新必须原子切换,失败回滚。

## 10. Example Rule Families

| Rule family | Where | Inputs | Context/IOC |
|---|---|---|---|
| `payload_dropped` | endpoint | `file.write`, `process.exec` | writable path context |
| `reverse_shell_pattern` | endpoint | `process.exec`, `network.connect` | trusted binaries, optional C2 IOC |
| `credential_file_read` | endpoint | `file.open` | credential path context |
| `ioc_network_match` | endpoint/cloud | `network.connect` | C2 IP/CIDR/domain IOC |
| `malware_hash_match` | endpoint/cloud | `process.exec`, file hash enrichment | malware hash IOC |
| `payload_lifecycle` | endpoint/cloud | `file.write`, `process.exec`, `network.connect` | writable path context, lineage |
| `web_shell_chain` | cloud/xdr | endpoint signal + process graph + network graph | workload context |

## 11. Current Implementation Snapshot

当前 agent 侧已切到新的 detection 契约主路径:

- `internal/endpoint/detection.Engine` 消费 `CanonicalEvent` 并产出 `Signal`。
- 当前 builtin RuleSet 为 `ruleset:endpoint-linux-builtin`。
- 支持 `web_runtime_spawns_shell`、`download_by_lolbin`、`payload_dropped`、`reverse_shell_pattern`、`suspicious_exec_connect`、`payload_lifecycle`、`credential_file_read` 等 builtin 规则。
- `DetectionPolicy.rulesets` 控制规则包启用。
- `DetectionPolicy.rule_overrides` 可以启停单条规则,并覆盖 mode、severity、response intent 和 params。
- runtime policy refresh 会重建 detection engine。
- `sysarmorctl signal watch --include-recent --rule-id ...` 可以查看本地 signal。
- `payload_lifecycle` 已支持一个 signal 引用多个 event refs。
- `policy apply detection` 会执行 detection schema normalize 和 dependency check。
- `sysarmorctl content apply/list/get` 已作为 local manager 路径接入 agent。
- ContextSet / IOCPack 下发后会重建 detection engine,后续 signal 会携带对应 content version/digest。
- agent content store 支持本地文件持久化和启动加载。
- content envelope 支持 Ed25519 签名验签,本地调试仍可通过 `--allow-unsigned` 接收未签名内容。
- ContextSet / IOCPack 支持 `merge_strategy: patch` 的增量更新。
- RulePack 文件可被解析进 detection resolver,用于注册/覆盖 builtin rule metadata。

当前差距:

- RulePack 目前只支持 builtin rule entrypoint 的元数据注册/覆盖,还不支持通用 rule DSL。
- Ed25519 trust key 仍是本地静态配置,还没有 key rotation / revocation / audit。
- content bundle 展开仍未实现,当前测试路径优先 apply 独立 rulepack/contextset/iocpack。
- dependency check 已有最小实现,但还只检查 event kind,尚未检查字段级能力。
- Signal proto 已预留 `rule_id`、`rule_version`、`ruleset_ref`、`context_refs`、`ioc_refs`、`severity`、`confidence`、`mode` 字段。

## 12. Implementation Direction

推荐演进顺序:

1. 将 content bundle 展开为多个 addressable rulepack/contextset/iocpack record。
2. 完善 key rotation / revocation / audit。
3. dependency check 从 event kind 扩展到 field/capability/context/ioc availability。
4. 将 RulePack 从 builtin entrypoint 元数据扩展到 expr/sequence runtime。
5. 再考虑云端 manager/gateway 平移,本地 envelope 保持不变。

## 13. Design Decisions

1. DetectionPolicy 引用规则内容,不内嵌大块规则逻辑。
2. RuleSet/RulePack 是可发布内容包,DetectionPolicy 是应用策略。
3. ContextSet / IOCPack 独立版本化,collection 和 detection 都可以引用。
4. Rule 输出 Signal,不直接创建 Incident。
5. rule overrides 只覆盖运行属性和显式参数,不修改核心检测逻辑。
6. agent 必须能报告 detection 与 collection 的依赖不匹配。
7. 旧 fastpath 不再作为 agent detection 产品路径;当前端侧检测由 builtin RuleSet-backed detection engine 承载。
