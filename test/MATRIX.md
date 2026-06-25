# Test Matrix

The benchmark matrix intentionally separates performance workload from detection scenario.

## Dimensions

| Dimension | Label Scope | Purpose | Examples |
|---|---|---|---|
| policy | `labels.policy_profile` | Collection strategy under test | `minimal-high-signal`, `edr-balanced`, `incident-deep`, `debug-wide` |
| workload | `labels.case_type=workload`, `labels.workload=<name>` | Agent runtime cost, drop rate, parse errors, resource pressure | `benign-business`, `exec-storm`, `file-read-storm`, `file-write-storm`, `network-connect-storm`, `mixed-edr-storm` |
| scenario | `labels.case_type=scenario`, `labels.scenario=<name>` | Endpoint detection effectiveness against labeled ground truth | `apt-fileless-c2`, `apt-staged-drop`, `benign-ci-noise` |

Workloads are benign pressure inputs. They should not intentionally touch C2 IoCs, payload paths, persistence paths, or sensitive credentials. Scenarios are security semantics inputs. They may be malicious or benign, and they carry labels used by effectiveness evaluation.

## Outputs

`bench-matrix-vm.sh` writes:

- `test/.results/bench-matrix-vm/<run-id>/matrix.csv`: policy x case performance summary.
- `test/.results/effectiveness/<run-id>/matrix.csv`: event/signal effectiveness metrics per policy and scenario/workload label file.
- `test/.results/effectiveness/<run-id>/attack_signal_matrix.csv`: compact policy x attack table with signal precision, recall, and F1.
- `test/.results/effectiveness/<run-id>/policy_comparison.csv`: combined effectiveness, resource, and stability score.

## Ground Truth

Effectiveness labels live beside the case:

- `test/scenarios/vm/<scenario>/labels.yaml`
- `test/workloads/vm/<workload>/labels.yaml`

For malicious scenarios, labels describe required event and signal entities. For benign cases, labels normally contain no positive event/signal truth and instead define forbidden attack signals under `policy.forbidden_signal_names`.
