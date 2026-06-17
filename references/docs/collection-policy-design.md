# SysArmor Collection Policy Design

Date: 2026-06-17
Status: draft

## 1. Positioning

Collection Policy 定义 **端侧 sensor 应该采集哪些客观事实,以及哪些过滤条件可以尽量下推到 sensor/backend**。

它不是检测规则,也不是威胁语义。比如:

- `process.exec` 是 collection behavior。
- `file.open` 是 collection behavior。
- `network.connect` 是 collection behavior。
- `credential_read`、`payload_drop`、`reverse_shell`、`apt_activity` 是 detection / rule 语义,不应该出现在 collection behavior 名称里。

一句话:

```text
Collection Policy answers: what facts should the sensor collect?
Detection Policy answers: what do those facts mean?
Response Policy answers: what are we allowed to do?
Context / IOC answers: which objects are interesting right now?
```

这样做的原因很直接:采集层必须客观、稳定、可编译到 Tetragon/eBPF;检测语义和威胁情报会频繁变化,应该通过 rule content、context set、IOC pack 更新。

## 2. Layering

推荐分层:

```text
AgentRuntimePolicy
  scope
  collection
    behaviors
      selectors
      pushdown hints
      refs to context / IOC
  detection
    rule refs
    rule mode
  response
    allowed actions
    enforce gates
  resource
    rate / cpu / memory / backpressure
  upload
    local spool / batch / retry

ContextSet
  managed workload scopes
  sensitive path sets
  trusted binary sets
  approved admin tools

IOCPack
  ip / cidr / domain / url / hash / path / certificate / signer
  versioned threat intelligence

TetragonCompiler
  CollectionPolicy + ContextSet + IOCPack
  -> Tetragon TracingPolicy / runtime filters
```

边界约定:

- Collection Policy 可以引用 context / IOC,但不直接承载大体量情报内容。
- IOC 更新可以独立于 policy 更新。agent 应支持在不改 policy id/version 的情况下刷新 `ioc:c2-ip-feed@latest`。
- 能下推到 Tetragon selector 的过滤尽量下推;不能下推的留在 agent detection / enrichment 层。
- Response / enforce 不由 collection policy 直接决定,collection 最多表达 sensor backend 支持的动作能力,真正执行必须经过 response policy 授权。

## 3. Behavior Catalog

Collection behavior 使用客观事件族命名:

| Behavior | Meaning | Typical event kind |
|---|---|---|
| `process.exec` | 进程启动/执行新程序 | `EXEC` |
| `process.fork` | 进程 fork/clone | `FORK` |
| `process.exit` | 进程退出 | `EXIT` |
| `network.connect` | 主动外联/connect | `CONNECT` |
| `file.open` | 文件打开,可区分读写 intent | `OPEN` |
| `file.read` | 文件读取路径,通常由 open/access 近似表达 | `OPEN` / `READ` |
| `file.write` | 文件写入/落地 | `WRITE` |
| `file.chmod` | 权限修改 | `CHMOD` |
| `file.unlink` | 删除文件 | `UNLINK` |
| `module.load` | 内核模块加载 | `MODULE_LOAD` |
| `capability.use` | 使用敏感 Linux capability | backend-specific |
| `capability.change` | capability 变化 | backend-specific |
| `namespace.change` | setns/unshare 等 namespace 变化 | backend-specific |

不推荐的 behavior 命名:

| Bad name | Reason | Better model |
|---|---|---|
| `file.sensitive_read` | `sensitive` 是上下文/检测语义 | `file.open` + `ctx:credential-paths` |
| `network.c2_connect` | `c2` 是 IOC/检测语义 | `network.connect` + `ioc:c2-ip-feed` |
| `process.payload_drop` | 这是规则判断 | `process.exec` + `file.write` -> detection rule |
| `apt-focused` as behavior | APT 是威胁画像,不是内核事件 | policy preset 引用多个 behaviors |

可以有 preset,但它只是 policy 模板:

- `edr-balanced`: 面向长期运行,低噪声、低资源占用。
- `incident-deep`: 面向调查窗口,短时间扩大采样。
- `apt-investigation`: 面向威胁狩猎,引用更多上下文和 IOC。

这些 preset 不应替代底层 behavior。

## 4. CollectionPolicyV2 Shape

当前实现已收敛为 `behaviors + behavior-scoped selectors` 的产品路径,不再接受 `profiles/kinds` 作为 collection policy 输入。

建议结构:

```json
{
  "policy_id": "edr-balanced-linux",
  "version": 1,
  "mode": "observe",
  "scope": {
    "type": "host",
    "selector": ""
  },
  "behaviors": [
    {
      "id": "process.exec",
      "enabled": true,
      "selectors": {
        "binary": {
          "prefixes": ["/bin/", "/usr/bin/", "/tmp/", "/dev/shm/"],
          "prefix_refs": ["ctx:managed-runtime-binary-prefixes"]
        },
        "parent": {
          "binary_prefixes": []
        },
        "namespace": [],
        "cgroup": [],
        "workload": {}
      }
    },
    {
      "id": "network.connect",
      "enabled": true,
      "selectors": {
        "socket": {
          "families": ["AF_INET", "AF_INET6"],
          "addr_refs": ["ioc:c2-ip-feed"],
          "cidr_refs": ["ioc:c2-cidr-feed"],
          "port_refs": ["ioc:c2-port-feed"],
          "exclude_cidrs": ["127.0.0.0/8", "::1/128"]
        },
        "process": {
          "binary_prefixes": ["/bin/", "/usr/bin/", "/tmp/", "/dev/shm/"]
        }
      }
    },
    {
      "id": "file.open",
      "enabled": true,
      "selectors": {
        "file": {
          "prefix_refs": ["ctx:credential-path-prefixes", "ctx:secret-volume-prefixes"]
        },
        "access": {
          "read": true,
          "write": false
        }
      }
    }
  ],
  "context_refs": [
    {
      "ref": "ctx:credential-path-prefixes",
      "version": 3
    }
  ],
  "ioc_refs": [
    {
      "ref": "ioc:c2-ip-feed",
      "version": "latest",
      "apply_to": ["network.connect"]
    }
  ]
}
```

核心变化:

- selector 归属于 behavior,不再是全局平铺。
- selector 同时支持 literal value 和 `ctx:` / `ioc:` 引用。
- `scope` 统一支持 `host | container | cgroup | namespace | pod`。
- policy preset 可以生成这类结构,但 agent 实际应用的是明确结构。

## 5. Selector Model

推荐 selector 分类:

| Selector | Examples | Pushdown target |
|---|---|---|
| `process.binary` | exact/prefix/regex/hash refs | Tetragon `matchBinaries` for path; hash in agent |
| `process.parent` | parent binary / args | Tetragon `matchParentBinaries` where possible |
| `process.args` | argv contains/prefix/regex | Tetragon `matchArgs` for hook args; richer matching in agent |
| `file` | path exact/prefix/ref, mount, inode later | Tetragon `matchArgs` file `Prefix` |
| `socket` | family, addr, cidr, port | Tetragon sockaddr operators where supported |
| `pid` | host/ns pid, follow fork | Tetragon `matchPIDs` where supported |
| `namespace` | pid/mnt/net/user namespace | Tetragon `matchNamespaces` |
| `cgroup` | cgroup id/path | backend dependent |
| `workload` | container/pod labels | Tetragon `matchWorkloads` in K8s-like context |
| `capability` | effective/permitted/inheritable | Tetragon capability selectors |
| `return` | errno/success/failure | Tetragon `matchReturnArgs` |
| `rate_limit` | per process/scope/behavior | agent-side first; backend if supported |

优先级原则:

1. 先按 scope 收敛:host/container/cgroup/namespace/pod。
2. 再按 behavior 启停。
3. 再按 selector 下推。
4. 下推失败必须返回 `unsupported` 或降级说明,不能静默变成全量采集。

## 6. Context And IOC

### ContextSet

ContextSet 是组织/环境相关的稳定集合,例如:

```json
{
  "id": "ctx:credential-path-prefixes",
  "version": 3,
  "type": "path_prefix",
  "values": [
    "/root/.ssh/",
    "/home/*/.ssh/",
    "/var/run/secrets/",
    "/run/secrets/",
    "/etc/kubernetes/"
  ]
}
```

它回答的是“在这个环境中,哪些对象值得关注”。

### IOCPack

IOCPack 是威胁情报集合,例如:

```json
{
  "id": "ioc:c2-ip-feed",
  "version": "2026-06-17T00:00:00Z",
  "type": "ip",
  "ttl": "24h",
  "values": [
    "203.0.113.10",
    "198.51.100.0/24"
  ]
}
```

它回答的是“当前情报认为哪些对象可疑”。

设计约定:

- 默认产品 policy 不应硬编码 `10.66.0.99:443` 这类测试 C2。
- 测试可以用 test-only IOCPack 注入具体 IP/port。
- 大体量 IOC 不应全部强行编译进内核 selector;需要按 backend 能力、数量上限和资源预算决定 pushdown。
- domain、URL、hash、certificate、signer 等通常不能在内核侧完整判断,应在 agent enrichment / detection 中使用。

## 7. Tetragon DSL Capability Overview

Tetragon 的核心配置对象是 TracingPolicy。SysArmor 不应该把 raw Tetragon YAML 直接暴露为主产品契约,但 compiler 要理解它的能力。

常用 primitives:

| Tetragon primitive | Meaning |
|---|---|
| `spec.kprobes[]` | hook kernel function |
| `spec.tracepoints[]` | hook kernel tracepoint |
| `spec.lsmhooks[]` | hook LSM hook,适合安全决策点 |
| `spec.uprobes[]` | hook user-space function |
| `args[]` | 定义 hook 参数类型 |
| `returnArg` | 定义返回值参数 |
| `selectors.matchArgs[]` | 按参数过滤 |
| `selectors.matchReturnArgs[]` | 按返回值过滤 |
| `selectors.matchData[]` | 按事件 metadata 过滤 |
| `selectors.matchBinaries[]` | 按进程 binary 过滤 |
| `selectors.matchParentBinaries[]` | 按 parent binary 过滤 |
| `selectors.matchPIDs[]` | 按 pid 过滤 |
| `selectors.matchNamespaces[]` | 按 namespace 过滤 |
| `selectors.matchNamespaceChanges[]` | 按 namespace 变化过滤 |
| `selectors.matchCapabilities[]` | 按 capability 过滤 |
| `selectors.matchCapabilityChanges[]` | 按 capability 变化过滤 |
| `selectors.matchWorkloads[]` | 按 workload metadata 过滤 |
| `selectors.matchActions[]` | 命中后动作,如 Post/NoPost 等 |
| `selectors.matchReturnActions[]` | 返回路径动作 |
| `spec.lists[]` | 可复用 value list |
| `spec.selectorsMacros` | 可复用 selector macro |

Tetragon 很适合作为当前 Sensor Runtime backend,因为它同时覆盖:

- 进程执行可见性。
- 文件/LSM hook。
- 网络 connect hook。
- namespace/capability/workload selector。
- 部分 enforcement action。

但 Tetragon DSL 是 backend-specific。SysArmor Collection Policy 应保持 portable contract,由 compiler 输出 Tetragon target。

## 8. SysArmor To Tetragon Mapping

| SysArmor behavior/selector | Tetragon candidate | Current status |
|---|---|---|
| `process.exec` | `security_bprm_creds_from_file` / exec events | partially implemented |
| `process.fork` | fork/clone tracepoint or kprobe | planned |
| `process.exit` | `do_exit` | partially implemented |
| `network.connect` | `security_socket_connect` | partially implemented |
| `network.connect.socket.family` | sockaddr `Family` | implemented |
| `network.connect.socket.addr` | sockaddr `SAddr` on current hook | implemented with naming caveat |
| `network.connect.socket.port` | sockaddr `SPort` on current hook | implemented with naming caveat |
| `file.open` / `file.write` / `file.chmod` | `security_file_permission` | partially implemented |
| `file.path.prefix` | file arg `Prefix` | implemented |
| `process.binary.prefix` | file/binary prefix selector | partially implemented |
| `process.parent` | `matchParentBinaries` | planned |
| `pid` | `matchPIDs` | planned |
| `namespace` | `matchNamespaces` | planned |
| `capability` | `matchCapabilities` | planned |
| `workload` | `matchWorkloads` | planned |
| `return` | `matchReturnArgs` | planned |
| `ctx:` refs | expanded before compile | planned |
| `ioc:` refs | expanded or agent-side evaluated | planned |

Naming caveat:

- Tetragon v1.7 的 `sockaddr` selector 支持 `SAddr/SPort/Family`,不支持 `DAddr/DPort`。
- 在 `security_socket_connect` hook 中,该 sockaddr 参数表达 connect target;SysArmor schema 可以称为 destination addr/port,但 compiler 输出当前 Tetragon 支持的 `SAddr/SPort`。

## 9. Pushdown Strategy

推荐编译流程:

```text
CollectionPolicyV2
  -> validate behavior and selector schema
  -> resolve ContextSet / IOCPack refs
  -> split pushdown-capable filters and agent-side filters
  -> estimate selector size / resource cost
  -> render deterministic Tetragon TracingPolicy
  -> apply new policy
  -> verify policy active
  -> remove old generated policy
  -> emit ControlAck
```

可以下推:

- file path exact/prefix。
- binary exact/prefix。
- socket family/IP/port,受 Tetragon operator 限制。
- namespace / workload / pid / capability,受 backend 和环境支持限制。
- return value filter,受 hook 支持限制。

不适合或不能完全下推:

- binary hash。
- domain / URL。
- signer / certificate。
- 跨事件行为链。
- “sensitive”、“malicious”、“APT”这类语义。
- 大规模 IOC feed。

这些应留给 agent detection 或云端 analytics。

## 10. Preset Examples

### edr-balanced

长期运行默认策略,目标是低噪声和低资源占用:

- `process.exec`: 采集关键执行事实,结合 binary/path/parent selector 降噪。
- `network.connect`: 采集外联,排除 loopback/link-local,可引用 IOC feed。
- `file.open`: 仅覆盖 credential/secret/config 上下文路径。
- `file.write`: 覆盖 `/tmp/`、`/dev/shm/`、应用插件目录、启动项目录。

### apt-investigation

威胁狩猎/调查策略,目标是更高可见性:

- 在 `edr-balanced` 基础上增加 `process.fork`、`process.exit`。
- 增加 parent binary、namespace、capability selectors。
- 引用更多 IOC pack。
- 允许短时间提高 rate/resource budget。

区别不是“APT 是一个采集行为”,而是 preset 选择更广的 behavior 和更多 context/IOC。

### incident-deep

针对单 host/container/pod 的短窗口加深采集:

- scope 收窄到具体 workload。
- 开启更细的 process lineage。
- 对相关 path/socket/process 增加 evidence pullback。
- 设置 TTL,过期自动回到 `edr-balanced`。

## 11. Current Implementation Snapshot

当前代码已经具备一个最小闭环:

- `CollectionPolicy` 支持 `behaviors`、`binary_prefixes`、`file_prefixes`、`socket_families`、`socket_addrs`、`socket_ports`、`scope_type`、`scope_selector`,不再兼容 `profiles/kinds`。
- agent local control 可以 dry-run/apply collection policy。
- Tetragon backend 可以生成 runtime TracingPolicy 并通过 `tetra tracingpolicy add/delete` 热更新。
- 已验证 `security_socket_connect` 使用 `Family/SAddr/SPort` selector。
- 已验证 `security_file_permission` 使用 file `Prefix` selector。
- 已验证 `security_bprm_creds_from_file` 可用于 exec path prefix。

当前差距:

- 过滤条件仍是全局平铺,还不是 behavior-scoped selectors。
- `ctx:` / `ioc:` 引用尚未接入 compiler。
- parent binary、pid、namespace、capability、workload、return filters 尚未完整落地。
- 大规模 IOC 的内核侧下推策略和上限尚未定义。
- Tetragon 默认事件流仍可能带来噪声;agent-owned Tetragon 启动参数和默认 telemetry 需要被 collection policy 更完整地控制。
- policy apply ack 需要更细地区分 `applied / rejected / unsupported / degraded / failed`。

## 12. Benchmark Notes

近期 VM 实验说明:

| Experiment | Events | EPS | EDR avg CPU | Tetragon avg CPU | Notes |
|---|---:|---:|---:|---:|---|
| wider collection | 3775 | 45.29 | 15.74% | 11.78% | 默认流较宽,噪声较高 |
| narrowed selectors | 2635 | 32.06 | 14.74% | 11.73% | kprobe/file/connect 事件减少,但 Tetragon 默认 exec telemetry 仍占主要成本 |

结论:

- selector pushdown 能减少事件量,但资源占用下降不明显时,要检查是否还有 backend 默认 telemetry 未被 policy 控制。
- 后续 benchmark 应同时记录:policy id/version、resolved context/ioc refs、生成的 Tetragon policy hash、事件分布、signal 数量、CPU/RSS、dropped events、parse errors。

## 13. Design Decisions

1. Collection behavior 保持客观,不使用 `sensitive`、`c2`、`apt` 等检测语义。
2. selectors 必须归属于 behavior。
3. ContextSet 和 IOCPack 独立版本化,policy 只引用。
4. compiler 必须显式报告哪些 selector 被下推、哪些留在 agent-side。
5. Tetragon 是当前 backend,不是 SysArmor policy 的公共 API。
6. raw Tetragon policy 只能作为高级 escape hatch,必须版本化、审计、权限控制,不能成为默认产品路径。
