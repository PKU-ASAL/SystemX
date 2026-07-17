# Detection Effectiveness

Effectiveness tests answer whether malicious and benign behavior produces the
expected events, signals, and incident reports across the real product path.

```bash
make -C test effectiveness-topology
```

The suite runs `business-normal` with `apt-fileless-c2`, `apt-staged-drop`, and
`benign-ci-noise` under balanced and deep collection policies. Manager query
results are the scoring source; local Agent watch output is diagnostic only.

```text
test/.results/effectiveness-topology/<run-id>/matrix.csv
test/.results/effectiveness/<run-id>/matrix.csv
test/.results/effectiveness/<run-id>/truth_steps.csv
```

`collection-minimal` intentionally narrows visibility and is not part of the
default full-detection gate. CPU/RSS captured during effectiveness runs is
context, not a formal performance baseline.
