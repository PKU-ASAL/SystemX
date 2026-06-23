# Local Agent Suite

Scope: `local`

System under test:

- `sysarmor-agent`;
- sensor runtime;
- local agent spool/WAL;
- local `sysarmorctl --socket` APIs.

This suite compares endpoint collection and endpoint detection behavior under different policies and workloads. It also measures local cost: agent/sensor CPU, RSS, EPS, drops, and parse errors.

Effectiveness reports are generated with `evaluation_scope=local`. Requirements from `expected.yaml` that need manager analytics, cloud signals, incidents, or graph evidence are carried through as structured `out_of_scope` checks instead of being counted as local failures.

Out of scope:

- manager cloud signals;
- incidents and graph/evidence;
- manager storage/query behavior;
- control-plane downlink semantics.
