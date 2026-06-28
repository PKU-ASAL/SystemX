# Policy And Content

This document merges collection policy, detection policy, content package, and ruleset design.

## Separation Of Concerns

```text
Collection Policy answers: what facts should the sensor collect?
Detection Policy answers: what rules explain those facts?
Response Policy answers: what actions are allowed?
Resource Policy answers: how much endpoint budget may be used?
DataPlane Policy answers: how data leaves the endpoint?
ContextSet answers: which environment objects are important?
IOCPack answers: which threat intelligence values are active?
RuleSet / RulePack answers: what detection content exists?
```

Collection behavior must stay objective. Names like `file.sensitive_read`, `network.c2_connect`, or `process.payload_drop` mix detection semantics into collection and should be avoided.

## Collection Policy

Collection Policy is behavior-first:

```json
{
  "policy_id": "balanced-linux",
  "version": 2,
  "scope_type": "host",
  "scope_selector": "",
  "observe_only": true,
  "behaviors": [
    {
      "id": "network.connect",
      "enabled": true,
      "selectors": {
        "process": {
          "binary_prefixes": ["/usr/bin/curl", "/usr/bin/wget", "/bin/sh", "/bin/bash", "/usr/bin/sh", "/usr/bin/bash"]
        },
        "socket": {
          "families": ["AF_INET", "AF_INET6"],
          "addr_refs": ["ioc:c2-ip-feed"],
          "port_refs": ["ioc:c2-download-port-feed", "ioc:c2-control-port-feed"]
        }
      }
    },
    {
      "id": "file.write",
      "enabled": true,
      "selectors": {
        "file": {
          "prefix_refs": ["ctx:payload-path-prefixes", "ctx:persistence-path-prefixes"]
        }
      }
    }
  ]
}
```

The example is intentionally selective: `balanced` should keep shell/interpreter binaries in `network.connect` selectors where event frequency is lower, but should not broadly collect shell/interpreter `process.exec`. Shell `process.exec` can dominate CPU because normal system and benchmark tooling use bash/sh frequently.

Supported behavior families:

- `process.exec`;
- `process.fork`;
- `process.exit`;
- `file.open`;
- `file.read`;
- `file.write`;
- `file.chmod`;
- `network.connect`.

Later behavior families may include unlink, module, capability, namespace, mount, ptrace, and container/K8s events.

Selectors are scoped to behavior. They may include:

- process binary exact/prefix;
- parent binary;
- argv predicates where backend supports them;
- file path exact/prefix;
- socket family/address/port;
- namespace/cgroup/workload scope;
- references to ContextSet or IOCPack.

## ContextSet And IOCPack

ContextSet represents stable environment knowledge:

```json
{
  "api_version": "sysarmor.content/v1",
  "kind": "contextset",
  "metadata": {
    "id": "ctx:payload-path-prefixes",
    "version": "2026.06.18.1"
  },
  "spec": {
    "value_type": "path_prefix",
    "values": ["/dev/shm/", "/var/lib/app/plugins"]
  }
}
```

IOCPack represents threat intelligence:

```json
{
  "api_version": "sysarmor.content/v1",
  "kind": "iocpack",
  "metadata": {
    "id": "ioc:c2-ip-feed",
    "version": "2026.06.18.1"
  },
  "spec": {
    "value_type": "ip",
    "values": ["10.66.0.99", "203.0.113.10"]
  }
}
```

Policy references should be small and typed. Large or fast-changing intelligence should update through content packages, not by rewriting every policy.

## Tetragon Alignment

SysArmor does not expose raw Tetragon YAML as product policy. It compiles Collection Policy into backend-specific policy.

Mapping examples:

| SysArmor behavior | Tetragon target |
|---|---|
| `process.exec` | exec hook / process events |
| `process.fork` | fork/clone hook if supported |
| `process.exit` | exit hook if supported |
| `file.open` / `file.read` | open/read-like hooks or syscall/kprobe coverage |
| `file.write` | write/open-write/chmod-related hooks |
| `file.chmod` | chmod/fchmod hooks |
| `network.connect` | connect/sockaddr hooks |

Push down when possible:

- path exact/prefix;
- binary exact/prefix;
- socket family/address/port;
- return value filters where backend supports them.

Keep in agent or cloud:

- namespace/cgroup/workload filters when the backend cannot push them down;
- binary hash;
- domain/URL;
- signer/certificate;
- semantic labels such as sensitive, malicious, APT;
- cross-event sequences;
- large IOC feeds.

## Detection Policy

Detection Policy applies detection content to a scope:

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
  rule_overrides:
    - rule_id: payload_dropped
      enabled: false
      reason: allowed in this scope
  context_refs:
    - ref: ctx:payload-path-prefixes
      version: latest
  ioc_refs:
    - ref: ioc:c2-ip-feed
      version: latest
```

Rule overrides are local patches to applied content. They may change enabled state, mode, severity, thresholds, windows, allowlists, and response intent. They should not change rule identity, rule version, or runtime type.

## RuleSet / RulePack

RuleSet is a versioned group of rules:

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
    - rule_id: download_by_lolbin
      version: 1
```

RulePack is a distributable package that can contain multiple rulesets and compatibility metadata.

Rule fields:

- `rule_id`;
- `version`;
- `where`: endpoint, cloud, xdr;
- severity and confidence;
- tags and MITRE;
- runtime type: builtin, expr, sequence, graph, external;
- required event behaviors and fields;
- optional context/IOC refs;
- signal output contract;
- response intent.

## Endpoint Rule Runtime

Endpoint runtime should support three practical levels:

- builtin rule entrypoints for high-confidence local detections;
- expression rules for single-event field/context/IOC predicates;
- short sequence rules with bounded windows and keys such as lineage id.

Example sequence:

```text
within 60s by lineage_id:
  file.write /dev/shm/
  -> file.chmod same path
  -> process.exec same path
  -> network.connect
```

This is lightweight CEP, not a full cloud graph engine.

## Dependency Check

Detection policy should be checked against active collection policy:

- required behavior must be collected;
- required fields must be available;
- required ContextSet/IOCPack must be present or explicitly optional;
- unsupported runtime should reject or degrade with an explanation.

This prevents applying rules that can never fire because the sensor is not collecting the needed facts.

## Signal Contract

Signals should include:

- id;
- name;
- rule id/version;
- ruleset ref;
- where;
- severity and confidence;
- mode;
- lineage id;
- entity refs;
- event refs;
- context refs;
- IOC refs;
- labels;
- optional response intent.

## Defaults

Recommended collection presets:

- `minimal`: very small always-on high-risk surface;
- `balanced`: default long-running EDR surface;
- `deep`: short investigation window.

These are the supported default collection presets. `collection-debug-wide` is not part of the default policy set.

Balanced defaults should not treat all of `/tmp` or `/var/tmp` as malicious. Prefer concrete high-risk paths such as `/dev/shm/`, explicit attack/test prefixes, persistence paths, plugin directories, and active IOC/context packages. Balanced should also avoid broad shell/interpreter `process.exec`; keep shell/interpreter matching on lower-frequency `network.connect` or in short-lived `deep` windows.
