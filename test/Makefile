# SysArmor test entrypoints.
#
# The test system is organized by evaluation suite:
#   unit      - local Go tests and microbenchmarks, no environment
#   performance - resource cost and local microbenchmarks
#   effectiveness - detection truth labels, recall, precision, incident linkage
#   product   - product functionality: endpoint, platform, topology

ENV       ?= container
SCENARIO  ?= apt-fileless-c2
DUR       ?= 30
RUN_ID    ?= manual
SYSARMOR_BENCH_PROFILE ?= quick
SYSARMOR_BENCH_WORKLOAD ?= business-normal
SYSARMOR_BENCH_SCENARIO ?=
SYSARMOR_BENCH_POLICIES ?=
EVALUATION_SCOPE ?= local
TETRAGON_ARCHIVE ?= $(abspath $(or $(SYSARMOR_TETRAGON_ARCHIVE),$(firstword $(wildcard ../.cache/tetragon-v1.7.0-amd64.tar.gz ../.scratchpad/.cache/tetragon-v1.7.0-amd64.tar.gz))))
export SYSARMOR_TETRAGON_ARCHIVE := $(TETRAGON_ARCHIVE)

.DEFAULT_GOAL := help

.PHONY: doctor up down up-container up-vm-endpoint up-vm-topology up-all down-container down-vm-endpoint down-vm-topology down-all provision status clean help
.PHONY: test-unit test-all
.PHONY: product-endpoint product-endpoint-standalone product-endpoint-release-container product-endpoint-namespace-container product-topology product-platform product-platform-smoke product-platform-full
.PHONY: performance-endpoint performance-local-store performance-platform performance-modules performance-rule-engine performance-matcher
.PHONY: effectiveness-topology effectiveness-report
.PHONY: diag-endpoint recorder-start recorder-mark recorder-stop recorder-report
.PHONY: capture-endpoint capture-topology report effectiveness-report

# Environment lifecycle -------------------------------------------------------

doctor:
	@failed=0; \
	for tool in go docker vagrant virsh; do \
		if command -v "$$tool" >/dev/null 2>&1; then \
			echo "[doctor][OK] $$tool"; \
		else \
			echo "[doctor][ERROR] 缺少 $$tool" >&2; \
			echo "  修复: 安装 $$tool 并确保它位于 PATH 中" >&2; \
			failed=1; \
		fi; \
	done; \
	if command -v vagrant >/dev/null 2>&1; then \
		plugins="$$(vagrant plugin list 2>&1)" || { \
			echo "[doctor][ERROR] 无法读取 Vagrant 插件: $$plugins" >&2; \
			echo "  修复: 确保 VAGRANT_HOME 可写，或取消指向空目录的 VAGRANT_HOME" >&2; \
			failed=1; \
			plugins=""; \
		}; \
		if [ -z "$$plugins" ]; then :; \
		elif echo "$$plugins" | grep -q 'vagrant-libvirt'; then \
			echo "[doctor][OK] vagrant-libvirt"; \
		else \
			echo "[doctor][ERROR] 缺少 vagrant-libvirt provider" >&2; \
			echo "  修复: 运行 vagrant plugin install vagrant-libvirt" >&2; \
			failed=1; \
		fi; \
	fi; \
	if ! command -v virsh >/dev/null 2>&1; then :; \
	elif virsh -c "$${LIBVIRT_DEFAULT_URI:-qemu:///system}" list >/dev/null 2>&1; then \
		echo "[doctor][OK] libvirt $${LIBVIRT_DEFAULT_URI:-qemu:///system}"; \
	else \
		echo "[doctor][ERROR] 无法连接 libvirt $${LIBVIRT_DEFAULT_URI:-qemu:///system}" >&2; \
		echo "  修复: 启动 libvirtd，并确认当前用户属于 libvirt 组" >&2; \
		failed=1; \
	fi; \
	if [ -n "$(TETRAGON_ARCHIVE)" ] && [ -f "$(TETRAGON_ARCHIVE)" ]; then \
		echo "[doctor][OK] Tetragon archive: $(TETRAGON_ARCHIVE)"; \
	else \
		echo "[doctor][ERROR] 未找到 Tetragon archive" >&2; \
		echo "  修复: 设置 SYSARMOR_TETRAGON_ARCHIVE=/path/to/tetragon-v1.7.0-amd64.tar.gz" >&2; \
		echo "  或放置到 .cache/ 或 .scratchpad/.cache/ 目录" >&2; \
		failed=1; \
	fi; \
	if [ "$$failed" -ne 0 ]; then exit 1; fi; \
	echo "[doctor] all checks passed"

up:
	$(call require_env)
	$(call start_env)

down:
	$(call require_env)
	$(call stop_env)

up-container:
	$(MAKE) up ENV=container

up-vm-endpoint:
	$(MAKE) up ENV=vm-endpoint

up-vm-topology:
	$(MAKE) up ENV=vm-topology

up-all: up-container up-vm-endpoint up-vm-topology

down-container:
	$(MAKE) down ENV=container

down-vm-endpoint:
	$(MAKE) down ENV=vm-endpoint

down-vm-topology:
	$(MAKE) down ENV=vm-topology

down-all:
	-$(MAKE) down ENV=container
	-SYSARMOR_VM_STOP_BEST_EFFORT=1 $(MAKE) down ENV=vm-topology
	-SYSARMOR_VM_STOP_BEST_EFFORT=1 $(MAKE) down ENV=vm-endpoint

provision:
	$(call require_vm_env)
	cd environments/$(ENV) && vagrant rsync node-a && vagrant provision node-a

status:
	$(call require_env)
	$(call status_env)

clean:
	bash shared/harness/cleanup.sh $(ENV) --results

# Product suites ---------------------------------------------------------------

test-unit:
	cd .. && go test ./...

product-endpoint: doctor
	ENV=vm-endpoint SYSARMOR_VM_ENV=vm-endpoint bash suites/product/endpoint/e2e-real-tetragon-owned-vm.sh

product-endpoint-standalone:
	bash suites/product/endpoint/standalone-local-store.sh
	bash suites/product/endpoint/unified-agent-installation.sh
	bash suites/product/endpoint/transactional-agent-installation.sh
	bash suites/product/endpoint/standalone-release-package.sh
	bash suites/product/endpoint/container-entrypoint.sh
	bash suites/product/endpoint/standalone-github-assets.sh
	bash suites/product/endpoint/release-notes.sh
	bash suites/product/endpoint/release-workflow-contract.sh
	bash suites/product/endpoint/release-container-e2e-contract.sh

product-endpoint-release-container:
	$(MAKE) -C release test

product-endpoint-namespace-container:
	bash suites/product/endpoint/e2e-namespace-self-container.sh

product-topology: doctor
	ENV=vm-topology SYSARMOR_VM_ENV=vm-topology bash suites/product/topology/e2e-systemd-vm.sh

product-platform:
	bash suites/product/platform/e2e-control-contract.sh
	bash suites/product/platform/e2e-gateway-local-ingest.sh
	bash suites/product/platform/e2e-agent-gateway-manager-local.sh
	bash suites/product/platform/e2e-control-roundtrip-local.sh
	bash suites/product/platform/e2e-manager-api-policy.sh
	bash suites/product/platform/e2e-response-policy-deny.sh
	bash suites/product/platform/e2e-response-scope-deny.sh
	bash suites/product/platform/e2e-response-approval.sh
	bash suites/product/platform/e2e-store-status.sh

product-platform-smoke: product-platform

product-platform-full:
	bash shared/harness/start-container.sh
	bash suites/product/topology/e2e-scenarios-container.sh

test-all: test-unit product-endpoint product-topology product-platform

# Capture ---------------------------------------------------------------------

capture-endpoint: doctor
	ENV=vm-endpoint SYSARMOR_VM_ENV=vm-endpoint bash shared/diagnostics/capture-vm.sh $(SCENARIO) $(DUR)

capture-topology: doctor
	ENV=vm-topology SYSARMOR_VM_ENV=vm-topology bash shared/diagnostics/capture-vm.sh $(SCENARIO) $(DUR)

# Performance and effectiveness ----------------------------------------------

performance-endpoint: doctor
	ENV=vm-endpoint SYSARMOR_VM_ENV=vm-endpoint SYSARMOR_BENCH_PROFILE=$(SYSARMOR_BENCH_PROFILE) SYSARMOR_BENCH_WORKLOAD=$(SYSARMOR_BENCH_WORKLOAD) SYSARMOR_BENCH_SCENARIO=$(SYSARMOR_BENCH_SCENARIO) SYSARMOR_BENCH_POLICIES="$(SYSARMOR_BENCH_POLICIES)" bash suites/performance/endpoint/run.sh

performance-local-store:
	bash suites/performance/endpoint/local-store.sh

performance-platform:
	ENV=vm-topology SYSARMOR_VM_ENV=vm-topology bash suites/performance/platform/run.sh

effectiveness-topology:
	ENV=vm-topology SYSARMOR_VM_ENV=vm-topology bash suites/effectiveness/topology/run.sh

performance-modules: performance-rule-engine performance-matcher

performance-rule-engine:
	cd .. && go test ./internal/endpoint/detection -run '^$$' -bench 'BenchmarkEngineProcess' -benchtime="$(or $(BENCHTIME),200ms)" -count="$(or $(COUNT),1)" -benchmem

performance-matcher:
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
	python3 shared/reports/effectiveness_report.py --bench-matrix-dir .results/effectiveness-topology/$(RUN_ID) --topology vm --scope $(EVALUATION_SCOPE) --scenarios $(or $(SCENARIOS),apt-fileless-c2 apt-staged-drop benign-ci-noise) --workloads $(or $(WORKLOADS),business-normal host-activity-heavy edr-activity-heavy) --output-dir .results/effectiveness/$(RUN_ID)

report:
	python3 shared/reports/report.py

help:
	@echo "SysArmor test suites:"
	@echo "  doctor           verify tools, libvirt provider, connection, and Tetragon archive"
	@echo "  test-unit        local Go tests"
	@echo "  product-endpoint single VM endpoint with owned real Tetragon"
	@echo "  product-endpoint-standalone local state and standalone release package contracts"
	@echo "  product-endpoint-release-container public release install and namespace/self container matrix"
	@echo "  product-endpoint-namespace-container container agent with namespace/self Tetragon scope"
	@echo "  product-topology multi-VM manager artifact/enrollment install path"
	@echo "  product-platform local platform contracts/smoke"
	@echo "  product-platform-smoke alias for product-platform"
	@echo "  product-platform-full container gateway/worker/manager product path"
	@echo "  performance-endpoint endpoint CPU/RSS benchmark"
	@echo "  performance-platform platform service CPU/memory benchmark over vm-topology"
	@echo "  performance-modules local module microbenchmarks"
	@echo "  effectiveness-topology detection effectiveness over vm-topology"
	@echo ""
	@echo "Environments:"
	@echo "  ENV=container | vm-endpoint | vm-topology"
	@echo "  up-container | up-vm-endpoint | up-vm-topology | up-all"
	@echo "  down-container | down-vm-endpoint | down-vm-topology | down-all"
	@echo ""
	@echo "Common:"
	@echo "  make -C test up ENV=vm-endpoint"
	@echo "  make -C test down-all"
	@echo "  make -C test performance-endpoint SYSARMOR_BENCH_PROFILE=quick SYSARMOR_BENCH_WORKLOAD=business-normal"
	@echo "  make -C test performance-endpoint SYSARMOR_BENCH_PROFILE=medium SYSARMOR_BENCH_WORKLOAD=business-normal SYSARMOR_BENCH_SCENARIO=apt-fileless-c2-local SYSARMOR_BENCH_POLICIES='test/data/policies/collection-balanced.json'"
	@echo "  make -C test performance-endpoint SYSARMOR_BENCH_PROFILE=long SYSARMOR_BENCH_WORKLOAD=business-normal"
	@echo "  make -C test performance-platform SYSARMOR_PLATFORM_PERF_DURATION=600"
	@echo "  make -C test effectiveness-topology ENV=vm-topology"
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
