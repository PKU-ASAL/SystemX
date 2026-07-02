# SysArmor test entrypoints.
#
# The test system is organized by scope:
#   unit      - local Go tests and microbenchmarks, no environment
#   endpoint  - one endpoint host: sysarmor-agent, sensor, local control, cost
#   topology  - multi-node product path: manager, endpoint, attacker
#   platform  - manager APIs, storage, policy, RBAC, control-plane contracts

ENV       ?= container
SCENARIO  ?= apt-fileless-c2
DUR       ?= 30
RUN_ID    ?= manual
SYSARMOR_BENCH_PROFILE ?= quick
SYSARMOR_BENCH_WORKLOAD ?= business-normal
SYSARMOR_BENCH_SCENARIO ?=
SYSARMOR_BENCH_POLICIES ?=
EVALUATION_SCOPE ?= local

.PHONY: up down provision status clean help
.PHONY: test-unit test-endpoint test-topology test-platform test-platform-full test-all
.PHONY: bench-endpoint bench-topology bench-module bench-rule-engine bench-matcher
.PHONY: diag-endpoint recorder-start recorder-mark recorder-stop recorder-report
.PHONY: capture-endpoint capture-topology report effectiveness-report

# Environment lifecycle -------------------------------------------------------

up:
	$(call require_env)
	$(call start_env)

down:
	$(call require_env)
	$(call stop_env)

provision:
	$(call require_vm_env)
	cd environments/$(ENV) && vagrant rsync node-a && vagrant provision node-a

status:
	$(call require_env)
	$(call status_env)

clean:
	bash shared/harness/cleanup.sh $(ENV) --results

# Scope suites ----------------------------------------------------------------

test-unit:
	cd .. && go test ./...

test-endpoint:
	ENV=vm-endpoint SYSARMOR_VM_ENV=vm-endpoint bash e2e/endpoint/e2e-real-tetragon-owned-vm.sh

test-topology:
	ENV=vm-topology SYSARMOR_VM_ENV=vm-topology bash e2e/topology/e2e-systemd-vm.sh
	ENV=vm-topology SYSARMOR_VM_ENV=vm-topology bash shared/diagnostics/capture-vm.sh $(SCENARIO) $(DUR)

test-platform:
	bash e2e/platform/e2e-control-contract.sh
	bash e2e/platform/e2e-gateway-local-ingest.sh
	bash e2e/platform/e2e-agent-gateway-manager-local.sh
	bash e2e/platform/e2e-control-roundtrip-local.sh
	bash e2e/platform/e2e-manager-api-policy.sh
	bash e2e/platform/e2e-response-policy-deny.sh
	bash e2e/platform/e2e-response-scope-deny.sh
	bash e2e/platform/e2e-response-approval.sh
	bash e2e/platform/e2e-store-status.sh

test-platform-full:
	bash shared/harness/start-container.sh
	bash e2e/topology/e2e-scenarios-container.sh

test-all: test-unit test-endpoint test-topology test-platform

# Capture ---------------------------------------------------------------------

capture-endpoint:
	ENV=vm-endpoint SYSARMOR_VM_ENV=vm-endpoint bash shared/diagnostics/capture-vm.sh $(SCENARIO) $(DUR)

capture-topology:
	ENV=vm-topology SYSARMOR_VM_ENV=vm-topology bash shared/diagnostics/capture-vm.sh $(SCENARIO) $(DUR)

# Benchmarks and diagnostics --------------------------------------------------

bench-endpoint:
	ENV=vm-endpoint SYSARMOR_VM_ENV=vm-endpoint SYSARMOR_BENCH_PROFILE=$(SYSARMOR_BENCH_PROFILE) SYSARMOR_BENCH_WORKLOAD=$(SYSARMOR_BENCH_WORKLOAD) SYSARMOR_BENCH_SCENARIO=$(SYSARMOR_BENCH_SCENARIO) SYSARMOR_BENCH_POLICIES="$(SYSARMOR_BENCH_POLICIES)" bash benchmarks/endpoint/run.sh

bench-topology:
	ENV=vm-topology SYSARMOR_VM_ENV=vm-topology bash benchmarks/topology/run.sh

bench-module: bench-rule-engine bench-matcher

bench-rule-engine:
	cd .. && go test ./internal/endpoint/detection -run '^$$' -bench 'BenchmarkEngineProcess' -benchtime="$(or $(BENCHTIME),200ms)" -count="$(or $(COUNT),1)" -benchmem

bench-matcher:
	cd .. && go test ./internal/endpoint/matcher -run '^$$' -bench . -benchtime="$(or $(BENCHTIME),200ms)" -count="$(or $(COUNT),1)" -benchmem

diag-endpoint:
	ENV=vm-endpoint SYSARMOR_VM_ENV=vm-endpoint SCENARIO=$(SYSARMOR_BENCH_WORKLOAD) SYSARMOR_DIAG_MODE=workload SYSARMOR_DIAG_PREFIX=endpoint-diagnostics.$(SYSARMOR_BENCH_WORKLOAD) bash shared/diagnostics/diagnose-tetragon-vm.sh

recorder-start:
	ENV=vm-endpoint SYSARMOR_VM_ENV=vm-endpoint RUN_ID=$(RUN_ID) bash shared/recorder/recorder-vm.sh start

recorder-mark:
	ENV=vm-endpoint SYSARMOR_VM_ENV=vm-endpoint RUN_ID=$(RUN_ID) PHASE=$(PHASE) DETAIL="$(DETAIL)" bash shared/recorder/recorder-vm.sh mark

recorder-stop:
	ENV=vm-endpoint SYSARMOR_VM_ENV=vm-endpoint RUN_ID=$(RUN_ID) bash shared/recorder/recorder-vm.sh stop

recorder-report:
	ENV=vm-endpoint SYSARMOR_VM_ENV=vm-endpoint RUN_ID=$(RUN_ID) bash shared/recorder/recorder-vm.sh report

effectiveness-report:
	python3 benchmarks/matrix/effectiveness_report.py --bench-matrix-dir .results/bench-topology/$(RUN_ID) --topology vm --scope $(EVALUATION_SCOPE) --scenarios $(or $(SCENARIOS),apt-fileless-c2 apt-staged-drop benign-ci-noise) --workloads $(or $(WORKLOADS),business-normal host-activity-heavy edr-activity-heavy) --output-dir .results/effectiveness/$(RUN_ID)

report:
	python3 shared/reports/report.py

help:
	@echo "SysArmor test scopes:"
	@echo "  test-unit        local Go tests"
	@echo "  test-endpoint    single VM endpoint agent/sensor/local control"
	@echo "  test-topology    multi-VM manager/agent/attacker path"
	@echo "  test-platform    manager/control/storage/platform contracts"
	@echo "  test-platform-full container gateway/worker/manager product path"
	@echo ""
	@echo "Environments:"
	@echo "  ENV=container | vm-endpoint | vm-topology"
	@echo ""
	@echo "Common:"
	@echo "  make -C test up ENV=vm-endpoint"
	@echo "  make -C test bench-endpoint SYSARMOR_BENCH_PROFILE=quick SYSARMOR_BENCH_WORKLOAD=business-normal"
	@echo "  make -C test bench-endpoint SYSARMOR_BENCH_PROFILE=medium SYSARMOR_BENCH_WORKLOAD=business-normal SYSARMOR_BENCH_SCENARIO=apt-fileless-c2-local SYSARMOR_BENCH_POLICIES='test/data/policies/collection-balanced.json'"
	@echo "  make -C test bench-endpoint SYSARMOR_BENCH_PROFILE=long SYSARMOR_BENCH_WORKLOAD=business-normal"
	@echo "  make -C test bench-topology ENV=vm-topology"
	@echo "  make -C test recorder-start RUN_ID=my-run"
	@echo ""
	@echo "Endpoint benchmark phases: startup, steady, workload, activity, persistence, overall"

define require_env
	@case "$(ENV)" in container|vm-endpoint|vm-topology) ;; *) echo "unsupported ENV=$(ENV)" >&2; exit 2 ;; esac
endef

define require_vm_env
	@case "$(ENV)" in vm-endpoint|vm-topology) ;; *) echo "ENV=$(ENV) is not a VM environment" >&2; exit 2 ;; esac
endef

define start_env
	@case "$(ENV)" in \
	  container) bash shared/harness/start-container.sh ;; \
	  vm-endpoint|vm-topology) bash shared/harness/start-vm.sh "$(ENV)" ;; \
	esac
endef

define stop_env
	@case "$(ENV)" in \
	  container) bash shared/harness/stop-container.sh ;; \
	  vm-endpoint|vm-topology) bash shared/harness/stop-vm.sh "$(ENV)" ;; \
	esac
endef

define status_env
	@case "$(ENV)" in \
	  container) docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}' ;; \
	  vm-endpoint|vm-topology) cd environments/$(ENV) && vagrant status ;; \
	esac
endef
