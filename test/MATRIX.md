# Test Matrix

The benchmark matrix intentionally separates performance workload from detection scenario.

## Dimensions

| Dimension | Label Scope | Purpose | Examples |
|---|---|---|---|
| policy | `labels.policy_profile` | Collection strategy under test | `minimal-high-signal`, `edr-balanced`, `incident-deep` |
| workload | `labels.workload=<name>` | Background resource pressure | `business-normal` |
| scenario | `labels.scenario=<name>` | Endpoint detection effectiveness against labeled ground truth | `apt-fileless-c2`, `apt-staged-drop`, `benign-ci-noise` |

Workloads are benign pressure inputs. They should not intentionally touch C2 IoCs, payload paths, persistence paths, or sensitive credentials. Scenarios are security semantics inputs. They may be malicious or benign, and they carry labels used by effectiveness evaluation.

## Outputs

`bench-matrix-vm.sh` writes:

- `test/.results/bench-matrix-vm/<run-id>/matrix.csv`: policy x case performance summary.
- `test/.results/effectiveness/<run-id>/matrix.csv`: event/signal effectiveness metrics per policy and scenario/workload label file.
- `test/.results/effectiveness/<run-id>/truth_steps.csv`: label-level match details for explaining hit/miss cases.

By default `MATRIX_MODE=cross`, which runs `policy(3) x scenario(3)` with `business-normal` as the single background workload (9 cases). Use `MATRIX_MODE=workload` for cost-only runs (policy x business-normal, no scenario). Use `MATRIX_MODE=scenario` for scenario-only runs. Additional workloads (`host-activity-heavy`, `edr-activity-heavy`) can be enabled via `WORKLOADS` env var for stress testing.

## Ground Truth

Effectiveness labels live beside the case:

- `test/data/scenarios/vm/<scenario>/labels.yaml`
- `test/data/workloads/vm/<workload>/labels.yaml`

For malicious scenarios, labels describe required event and signal entities. For benign cases, labels normally contain no positive event/signal truth and instead define forbidden attack signals under `policy.forbidden_signal_names`.
