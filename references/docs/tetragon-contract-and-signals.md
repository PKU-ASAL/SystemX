# Tetragon Contract And Endpoint Signals

This document explains how SysArmor maps Tetragon runtime facts into the stable SysArmor event contract, and how endpoint signals are produced from canonical events.

The important boundary is:

```text
Tetragon raw event or kernel hook
  -> SysArmor SensorEvent
  -> SysArmor CanonicalEvent.behavior
  -> endpoint detection rule
  -> Signal
```

Tetragon is the current backend. `behavior` and `Signal` are SysArmor product contracts.

## Why This Mapping Exists

Tetragon exposes backend-specific event envelopes and hook names. Those names are useful for the sensor implementation, but they should not leak into collection policies, detection rules, manager queries, or benchmark labels.

SysArmor normalizes backend events into a small behavior vocabulary:

```text
process.exec
process.fork
process.exit
file.open
file.read
file.write
file.chmod
network.connect
```

This keeps collection and detection portable:

- collection policies ask for `network.connect`, not `security_socket_connect`;
- detection rules reason about `file.write`, not a Tetragon YAML kprobe;
- a future native SysArmor sensor can emit the same behaviors without changing detection content;
- tests and benchmarks compare behavior-level facts rather than backend artifacts.

## Raw Tetragon Shapes

The adapter currently consumes Tetragon JSON rows with these broad shapes:

```json
{"process_exec": {...}}
```

```json
{"process_exit": {...}}
```

```json
{
  "process_kprobe": {
    "function_name": "security_socket_connect",
    "args": []
  }
}
```

The top-level Tetragon envelope type tells the adapter which parser path to use. For kprobe events, `function_name` identifies the kernel hook that produced the row.

Implementation entry point:

```text
internal/sensor/tetragon/adapter.go
  ParseLine
```

## Behavior Mapping

Current mapping:

| Tetragon source | Kernel/backend hook | SysArmor behavior | Notes |
|---|---|---|---|
| `process_exec` | Tetragon process event | `process.exec` | Native Tetragon process execution event. |
| `process_exec` with clone flag | Tetragon process event | `process.fork` | Derived from process flags. |
| `process_exit` | Tetragon process exit event | `process.exit` | Process termination. |
| `process_kprobe` | `security_bprm_creds_from_file` | `process.exec` | LSM hook on executable preparation; semantically an exec. |
| `process_kprobe` | `security_socket_connect` | `network.connect` | Socket address and port become the event object. |
| `process_kprobe` | `security_file_permission` | `file.open`, `file.read`, `file.write` | Permission arg decides read/write; default is open. |
| `process_exec` heuristic | curl/wget `-o <path>` argv | `file.write` | Inferred write for common download tools. |
| `process_exec` heuristic | chmod argv | `file.chmod` | Inferred chmod when direct file permission events are unavailable. |

`security_bprm_creds_from_file` maps to `process.exec` because it fires while the kernel prepares credentials for a new executable file. It is a backend hook name, not a product event name.

## Collection Policy To Tetragon Policy

Collection policies are behavior-first. The Tetragon backend compiles each requested behavior into backend-specific hooks/selectors.

Examples:

| SysArmor behavior | Tetragon target |
|---|---|
| `process.exec` | `process_exec` and/or `security_bprm_creds_from_file` |
| `process.exit` | `process_exit` or `do_exit` |
| `network.connect` | kprobe on `security_socket_connect` |
| `file.open` / `file.read` / `file.write` | kprobe on `security_file_permission` |
| `file.chmod` | inferred chmod and file permission coverage |

Implementation entry point:

```text
internal/sensor/tetragon/backend.go
  Capability
  buildTracingPolicy
```

The generated Tetragon policy is backend machinery. It is not the stable SysArmor policy API.

## Canonical Event Contract

After normalization, endpoint detection consumes `CanonicalEvent`.

Important fields:

- `behavior`: stable event semantic, such as `network.connect`;
- `subject_proc`: process that performed the behavior;
- `object`: file, socket, process target, or other object;
- `lineage_id`: local execution chain;
- `labels`: environment, policy, benchmark, or deployment context;
- `raw_ref`: pointer back to raw sensor evidence;
- `scope`: host/container/cgroup/namespace/pod boundary.

`behavior` is production contract. Test concepts such as workload or scenario should be labels, for example:

```text
labels["case_type"] = "scenario"
labels["scenario"] = "apt-staged-drop"
labels["workload"] = "benign-business"
```

## Endpoint Detection Entry Point

Endpoint detection is behavior-dispatched:

```text
process.exec
  -> web runtime shell detection
  -> payload execution tracking

network.connect
  -> download_by_lolbin
  -> reverse_shell_pattern
  -> suspicious_exec_connect / payload_lifecycle

file.open / file.read
  -> credential_file_read

file.write / file.chmod
  -> payload_dropped
```

Implementation entry point:

```text
internal/endpoint/detection/engine.go
  Engine.Process
```

The endpoint does not build a global attack graph. It keeps small per-lineage state and emits local, explainable signals.

## Network Connect Signal Paths

`network.connect` is not itself a signal. It is an event behavior that several rules may interpret differently.

### download_by_lolbin

Meaning:

```text
curl or wget made a network connection
```

Rule shape:

```text
event.behavior == network.connect
subject process basename in {curl, wget}
```

Output:

```text
Signal.name = download_by_lolbin
Signal.entities = process + socket
Signal.event_refs = current event
```

This signal is intentionally low-cost and local. It means "a living-off-the-land downloader connected out"; it does not by itself prove maliciousness.

### reverse_shell_pattern

Meaning:

```text
a shell-like process connected to a C2 socket
```

Rule shape:

```text
event.behavior == network.connect
subject process basename is shell-like, such as bash or sh
socket matches configured C2 socket policy
```

Endpoint state can raise the signal to terminal when the same lineage previously saw web runtime shell behavior or a downloader signal.

Output:

```text
Signal.name = reverse_shell_pattern
Signal.entities = process + socket
Signal.terminal = true when local lineage context is strong enough
```

### suspicious_exec_connect

Meaning:

```text
a process that looks like a payload connected to a C2 socket
```

Rule shape:

```text
event.behavior == network.connect
socket matches configured C2 socket policy
process is known or suspected payload:
  - previously observed payload exec stable id
  - parent is a known payload exec
  - process binary is under payload path prefixes
  - argv contains a known helper/payload pattern
```

Output:

```text
Signal.name = suspicious_exec_connect
Signal.entities = process + payload file + socket
```

### payload_lifecycle

Meaning:

```text
local endpoint observed enough pieces of a payload lifecycle
```

Rule shape:

```text
payload/write/download/exec context exists in lineage
current event is a C2 network connect
```

Output:

```text
Signal.name = payload_lifecycle
Signal.event_refs = local refs that explain the lifecycle
Signal.entities = process + payload file + socket
```

This remains an endpoint-local signal. Long-range cross-lineage or cross-host stitching belongs to cloud analytics.

## C2 Socket Semantics

IOC content provides C2 IP and port references. Endpoint control-channel matching should avoid treating every connection to a known test/infrastructure host as a C2 session.

Current intended semantics:

```text
control port must match configured C2 ports
if C2 IPs are configured, IP must also match
```

This prevents download ports such as `8080` from being interpreted as control-channel signals when they should only produce downloader-style signals.

Example:

```text
10.66.0.99:8080 -> download_by_lolbin may be valid
10.66.0.99:443  -> reverse_shell_pattern / suspicious_exec_connect may be valid
```

## Local State

The endpoint engine keeps compact per-lineage state:

- downloaded event refs;
- payload paths;
- payload exec stable ids;
- payload write/chmod refs;
- web-shell exec refs;
- reverse-shell seen flag.

This state is used only to make local signals more explainable. It is not a durable graph database.

## Contract Rules

Stable rules:

- Product code and policies should use SysArmor behaviors, not Tetragon hook names.
- Tetragon hook names belong in the sensor backend and diagnostics.
- Events are objective facts.
- Signals are rule-derived security facts.
- Labels carry benchmark, environment, deployment, and grouping context.
- Workload/scenario are labels, not top-level production fields.
- Endpoint signals should be local, low-cost, and explainable.
- Cloud analytics performs long-lived, cross-lineage, cross-scope graph reasoning.

