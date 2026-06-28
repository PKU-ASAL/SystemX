我看下来，你的怀疑是有道理的，但要拆成两层确认：事件量变多和每个事件上的规则匹配成本变高。现在用户态规则引擎确实有明显可优化点。
关键热点
CEP 规则每个事件都会遍历所有 enabled rule：见 [engine.go (line 590)](/home/jiandong/workspace/oslab/sysarmor/sysarmor-next-project/internal/endpoint/detection/engine.go:590)。规则数越多，CPU 近似线性增长。
条件匹配没有预编译：每次都做 strings.ToLower、拼 values、解析 field、遍历 ref values：见 [engine.go (line 727)](/home/jiandong/workspace/oslab/sysarmor/sysarmor-next-project/internal/endpoint/detection/engine.go:727)。
event field 是按条件反复动态提取，比如 argv join、socket addr split、scope/file/path 等：见 [engine.go (line 811)](/home/jiandong/workspace/oslab/sysarmor/sysarmor-next-project/internal/endpoint/detection/engine.go:811)。
sequence rule 每步命中后会构造完整 eventFieldMap，即使后续只用少数字段：见 [engine.go (line 747)](/home/jiandong/workspace/oslab/sysarmor/sysarmor-next-project/internal/endpoint/detection/engine.go:747)。
policy 差异不一定只有“规则数”。minimal 比 balanced 少了 file.read，而 collection.yaml 还采 process.fork/exit，这会直接增加 normalizer + detection 的事件输入量。
我建议的优化路线
先做自研引擎的算法优化，不要一上来换第三方引擎。
当前问题主要是缺少 rule indexing 和 condition precompile。这些改动小、收益直接，而且不会破坏现在的规则语义。

给规则建立 behavior index。
编译时把规则分到：
exprByBehavior[behavior]
sequenceByStepBehavior[behavior]
builtin 保持现在 switch dispatch
这样 process.exec 事件只看关心 process.exec 的规则，不再扫所有 CEP 规则。对“规则很多但单事件只命中少数行为”的场景，这通常是最大收益点。

预编译 condition。
把 ConditionSpec 编译成内部结构：
op 变 enum，避免热路径 strings.ToLower
field 变 enum/function pointer，避免 switch 字符串反复匹配
eq/in 的 values 编译成 map[string]struct{}
prefix values 编译成 prefix matcher/trie
Ref 在 apply policy 时展开，不在每个事件里查 content
socket port/IP 这种 IOC 用 set/map，而不是 slice 线性扫

每个事件构造一次轻量 EventView。
在 Process 开头提取：
behavior
process binary/base
argv joined
file path
socket addr/port
lineage/scope/container
后续 builtin 和 CEP 都读这个 view，避免同一事件内重复 Join/Split/Get...。

sequence rule 做更细索引。
当前 sequence rule 每个事件都可能触发 groupKey、state 查找和 step 匹配。可以编译出：
first step behavior
all possible step behaviors
current active group 的 next behavior
最低成本版本：只有当事件 behavior 属于该 sequence 的 step behaviors 时才评估。更进一步：active group 按 next behavior 分桶。

加测量指标，先确认收益。
增加 detection metrics：
events_by_behavior
rules_scanned
rules_evaluated
conditions_evaluated
cep_groups_active
process_ns_total
再做两个 benchmark：
固定事件流，变化规则数，验证 CPU 是否线性下降
固定规则集，变化 collection policy，区分“事件量”与“规则成本”

关于第三方规则引擎
Expr：适合替代 stateless expression 条件。它是 Go 表达式语言，官方 README 强调安全、无副作用、总会终止，并使用优化编译器和 bytecode VM，比较适合作为 expr runtime 后端。来源：Expr README。
CEL-Go：更标准、更安全，适合长期做 policy expression。CEL 文档强调 parse/check 比 evaluation 贵，应提前编译；cel.Program 是 stateless、thread-safe、可缓存的。来源：CEL-Go README。
Grule/RETE 类引擎：更像专家系统/业务规则系统。Grule 自述是受 Drools 启发的 Go rule engine，但它的 loading 成本和 allocation 不小；对 EDR 这种高频 streaming event path，我不建议作为第一选择。来源：Grule README。
Hyperscan/gohs：适合大量 regex/literal 多模式匹配，但会带来 C/C++/平台依赖，x86 SIMD/ARM Vectorscan 也要考虑。更适合后续处理大量 path/argv regex，不适合直接承载 sequence/CEP。来源：gohs README。
结论
最优路径我会选：先优化现有引擎的数据结构和编译模型，再评估 Expr/CEL 作为表达式后端。不要先引入 Grule/RETE，这类引擎解决的是“复杂事实推理”，而你现在的瓶颈更像“每事件规则扫描 + 字符串/字段匹配没有预编译”。
下一步最适合先做一个小改：给 Engine 增加 compiledRules 和 cepByBehavior，同时加 benchmark。这样能很快确认 CPU 是否从“随总规则数增长”变成“随相关规则数增长”。

---

# Endpoint Rule Engine CPU Optimization Roadmap - 2026-06-27

This roadmap records the current analysis and proposed optimization path for
reducing SysArmor endpoint user-space CPU usage when running non-minimal
policies.

## Problem Statement

Observed behavior:

- `collection-minimal` has much lower CPU usage.
- Other policies show much higher CPU usage.
- The visible policy difference is partly rule/behavior coverage, so the
  endpoint user-space rule engine is a likely contributor.

Current hypothesis:

CPU growth comes from two combined effects:

1. Broader collection policies produce more events for the agent to normalize
   and inspect.
2. The user-space detection engine does too much per event, especially as the
   number of CEP/runtime rules grows.

The goal is to reduce user-space CPU while preserving detection coverage and
policy semantics.

## Current Hot Path

Main endpoint detection path:

```text
sensor event
  -> normalize.CanonicalEvent
  -> detection.Engine.Process
  -> builtin rule dispatch
  -> CEP expr/sequence runtime
  -> signal emission
```

Important code locations:

- `internal/endpoint/detection/engine.go`
- `internal/agent/daemon/endpoint_runtime.go`
- `internal/endpoint/dataappend/stream.go`
- `internal/endpoint/normalize/normalize.go`

Current rule-engine observations:

- Builtin rules are dispatched by event behavior with a switch.
- CEP/runtime rules are scanned for every event.
- Condition matching is interpreted on the hot path.
- Event fields are extracted repeatedly by string key.
- Content refs are resolved repeatedly during condition evaluation.
- Sequence rules create and update rule/group state per matching event.

## Evidence

### CEP rules are evaluated by scanning all enabled CEP rules

`Engine.detectCEPRules` iterates over all `e.rules`, checks whether each rule is
enabled and CEP-backed, then evaluates it against the event.

Effect:

- Per-event cost grows roughly with total CEP rule count.
- A `process.exec` event still loops over CEP rules that only care about
  `file.read`, `network.connect`, or other behaviors.

### Conditions are interpreted repeatedly

`matchCondition` performs repeated hot-path work:

- extracts event fields by string;
- builds a values slice from `Value`, `Values`, and `Ref`;
- lowercases and normalizes op strings;
- linearly scans value lists for equality, prefix, suffix, and contains checks;
- resolves content refs during each event evaluation.

Effect:

- CPU grows with rule count, condition count, and content value count.
- Large context/IOC sets amplify per-event cost.

### Event field extraction is repeated

`eventField` dynamically switches on string field names and may repeatedly do:

- `strings.Join(argv, " ")`;
- socket host/port split;
- scope/container/cgroup extraction;
- process/object getter chains.

Effect:

- Multiple conditions in one rule can repeat the same field work.
- Multiple rules can repeat the same field work again for the same event.

### Sequence rules store broad event maps

Sequence state stores `eventFieldMap(ev)` for a matched step. The map currently
includes many fields, even when later `same_as` conditions only need a small
subset.

Effect:

- Extra allocations and string work on sequence-heavy policies.

### Collection policy affects event volume

The CPU difference is not necessarily only rule count:

- `collection-minimal` does not collect `file.read`.
- `collection-balanced` adds `file.read`.
- `collection.yaml` includes broader behaviors such as `process.fork`,
  `process.exit`, and wider `network.connect` capture.

Effect:

- Wider collection increases normalizer and detection input volume.
- Rule-engine CPU must be measured separately from sensor/event-volume effects.

## Optimization Direction

Do not replace the rule engine first. The current bottleneck is mostly missing
indexing and precompilation, not a lack of advanced inference capability.

Recommended strategy:

1. Optimize the existing engine's compile-time data structures.
2. Add measurement to confirm CPU reduction.
3. Evaluate a third-party expression backend only after the engine has behavior
   indexing and precompiled conditions.

## Phase 1: Measure Before Changing Semantics

Add lightweight metrics to `detection.Engine`:

- `EventsProcessed`
- `EventsByBehavior`
- `CEPRulesScanned`
- `CEPRulesEvaluated`
- `ConditionsEvaluated`
- `ConditionsMatched`
- `FieldReads`
- `ProcessNanosTotal`
- `ProcessNanosByBehavior`

Add benchmarks:

1. Fixed event stream, variable rule count.
2. Fixed rule set, variable collection/event mix.
3. CEP expr-only policy.
4. CEP sequence-heavy policy.
5. Content-heavy prefix/IOC policy.

Expected result:

- Confirm whether CPU is dominated by event volume, rule scan count, condition
  evaluation, or normalization.

## Phase 2: Behavior-Indexed Rule Dispatch

Compile runtime rules into behavior-specific indexes:

```text
compiledEngine:
  exprByBehavior:
    process.exec      -> []compiledExprRule
    file.read         -> []compiledExprRule
    network.connect   -> []compiledExprRule

  sequenceByBehavior:
    file.write        -> []compiledSequenceRule
    file.chmod        -> []compiledSequenceRule
    process.exec      -> []compiledSequenceRule
    network.connect   -> []compiledSequenceRule

  behaviorAgnostic:
    []compiledRule
```

Behavior source:

- expr rules: first use `RequiredBehaviors` / `RequiredEvents`; if absent, fall
  back to condition fields or behavior-agnostic.
- sequence rules: index by each step behavior.

Expected improvement:

- Per-event CEP scan changes from `O(total_rules)` to
  `O(rules_relevant_to_behavior)`.
- Policies with many rules spread across behaviors should see immediate CPU
  reduction.

Compatibility:

- Preserve current rule semantics.
- Behavior-agnostic rules still run for every event.

## Phase 3: Precompile Conditions

Convert `ConditionSpec` into hot-path optimized compiled conditions:

```text
compiledCondition:
  field: fieldID
  op: opID
  values: []string
  valueSet: map[string]struct{}
  prefixes: prefixMatcher
  suffixes: []string
  contains: []string
  sameAsStep: string
  sameAsField: fieldID
```

Compile-time work:

- normalize op once;
- normalize field once;
- expand `Ref` values once;
- build maps for `eq`, `in`, `not_in`, `neq`;
- build prefix matcher for prefix-heavy content;
- validate unsupported fields/operators early.

Expected improvement:

- Less allocation and string normalization per event.
- Equality and IOC checks become O(1).
- Prefix-heavy context matching becomes cheaper.

Initial prefix matcher:

- Start with sorted prefix slices grouped by first byte/path segment.
- Consider trie/radix tree only if benchmarks show prefix matching still hot.

## Phase 4: EventView Cache Per Event

Build a small `eventView` at the start of `Engine.Process`:

```text
eventView:
  eventID
  behavior
  lineageID
  processStableID
  processBinary
  processBinaryBase
  processArgvJoined
  parentStableID
  filePath
  socketAddr
  socketHost
  socketPort
  scopeType
  scopeSelector
  containerID
  cgroup
  occurredAtNs
```

Use this view for:

- builtin rules;
- compiled condition field access;
- sequence group keys;
- signal entity extraction where possible.

Expected improvement:

- Avoid repeated `argv` joins and socket splits.
- Replace string-key field lookup with direct accessor functions or field IDs.

## Phase 5: Sequence Runtime Optimization

Short-term:

- Only evaluate a sequence rule if the event behavior appears in its step list.
- Store only fields required by future `same_as` conditions instead of full
  `eventFieldMap`.
- Precompile group-key fields into field IDs.

Medium-term:

- Track active groups by next expected behavior.
- For a sequence with active groups, evaluate only rules whose next step can
  consume the current event behavior.

Expected improvement:

- Lower CPU and allocation for sequence-heavy policies.
- Lower memory pressure from state maps.

## Phase 6: Collection-Side Pushdown Review

Separate rule-engine CPU from event-volume CPU:

- Compare CPU per event, not only total CPU.
- Track event rates by behavior for each policy.
- Measure agent CPU with identical event replay streams.

Policy-specific review:

- Keep `minimal` as the baseline.
- For `balanced`, quantify the added cost of `file.read`.
- For broad policies, inspect whether `process.fork`, `process.exit`, and wide
  `network.connect` collection are necessary for endpoint detection or should
  move behind deep mode.

Expected result:

- Distinguish sensor/normalizer pressure from rule engine pressure.
- Identify collection selectors that should be pushed down into Tetragon/eBPF
  instead of handled in user space.

## Third-Party Engine Evaluation

### Expr

Potential use:

- Backend for stateless `expr` rules.
- Good fit for user-authored field expressions once event fields are exposed as
  a compact input object.

Pros:

- Lightweight Go expression language.
- Designed for safe expression evaluation.
- Can compile expressions ahead of time.

Cons:

- Does not solve sequence/CEP state by itself.
- Still needs behavior indexing and event-view input design.
- Need benchmark before putting it in the endpoint hot path.

Recommended stance:

- Good candidate for Phase 7 expression backend after Phase 2-4 are done.

### CEL-Go

Potential use:

- Policy-standard expression language.
- Good fit if rules need a portable, well-defined expression contract.

Pros:

- Mature and widely used.
- Strong static checking model.
- Expressions can be parsed/checked/programmed once and reused.

Cons:

- Heavier integration surface than the current simple condition model.
- Does not solve sequence state by itself.
- Requires careful input typing and allocation control.

Recommended stance:

- Good candidate if SysArmor wants a long-term standard policy expression
  language. Probably not the first CPU optimization step.

### RETE / Grule-style rule engines

Potential use:

- Complex fact inference or business-rule style workflows.

Pros:

- Useful for multi-fact reasoning and rule authoring.

Cons:

- Likely too heavy for high-frequency endpoint event hot paths.
- Higher allocation and runtime overhead risk.
- More complex operational model than the current detection needs.

Recommended stance:

- Do not use as the endpoint hot-path engine now.

### Hyperscan / Multi-pattern Matching

Potential use:

- Large regex/literal matching over argv, path, command line, or strings.

Pros:

- Very fast for large pattern sets.
- Useful if rule content grows into many regex/string patterns.

Cons:

- Native dependency and platform constraints.
- Not a general rule engine.
- Does not handle sequence state.

Recommended stance:

- Consider later for large string-pattern packs, not for the first endpoint CPU
  reduction milestone.

## Recommended Implementation Order

1. Add rule-engine metrics and microbenchmarks.
2. Add behavior-indexed CEP dispatch.
3. Precompile conditions and content refs.
4. Add per-event `eventView`.
5. Optimize sequence state and group evaluation.
6. Re-run policy matrix with CPU-per-event reporting.
7. Evaluate Expr or CEL only for expression runtime replacement.

## Acceptance Criteria

For the same replayed event stream:

- CPU per event decreases as rule count grows.
- CEP rule scan count is close to relevant-rule count, not total-rule count.
- Condition evaluation count decreases or remains bounded by relevant rules.
- Existing endpoint detection tests pass unchanged.
- E2E scenarios still produce expected endpoint signals and incidents.

For policy matrix runs:

- Report both total CPU and CPU per normalized event.
- Report event counts by behavior.
- Explain residual CPU differences by collection volume, not only by rule
  count.

## Open Questions

- How many CEP/runtime rules are expected in near-term policies: tens,
  hundreds, or thousands?
- Are most future rules expression-only, sequence-based, or builtin-like?
- Will rules need regex matching over argv/path, or are exact/prefix/IOC checks
  enough for the next milestone?
- Should `file.read` be part of default balanced policy, or remain an
  deep mode behavior?
- What CPU budget should the endpoint agent target under idle, business-normal,
  and activity-heavy workloads?

## Conclusion

The best near-term path is to keep the current rule semantics and make the
engine behave like a compiled matcher:

```text
policy/content
  -> compile indexes, fields, ops, refs
  -> per-event eventView
  -> behavior-specific rule dispatch
  -> minimal condition/state evaluation
```

This should reduce user-space CPU with lower risk than replacing the engine
immediately. Third-party expression engines remain useful, but they should come
after indexing and precompilation, not before.
