# SysArmor test entrypoints.
#
# The test system is organized by concern:
#   unit         - local code and module correctness
#   functional   - endpoint, platform, and topology product behavior
#   detection    - truth labels, detection quality, and evidence linkage
#   performance  - resource cost, throughput, and local microbenchmarks
#   distribution - package and published installation compatibility
#   release      - aggregate gates over the required test concerns

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
.PHONY: test-unit functional-core
.PHONY: functional-endpoint functional-endpoint-local functional-endpoint-container functional-platform functional-platform-full functional-topology
.PHONY: detection-topology detection-report distribution-package distribution-published release-candidate release-published
.PHONY: product-endpoint product-endpoint-standalone product-endpoint-release-container product-endpoint-namespace-container product-topology product-platform product-platform-smoke product-platform-full effectiveness-topology effectiveness-report test-all
.PHONY: performance-endpoint performance-local-store performance-platform performance-modules performance-rule-engine performance-matcher
.PHONY: diag-endpoint recorder-start recorder-mark recorder-stop recorder-report
.PHONY: capture-endpoint capture-topology report

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

# Unit and functional suites --------------------------------------------------

test-unit:
	cd .. && go test ./...

functional-endpoint: doctor
	ENV=vm-endpoint SYSARMOR_VM_ENV=vm-endpoint bash suites/functional/endpoint/e2e-real-tetragon-owned-vm.sh

functional-endpoint-local:
	bash suites/functional/endpoint/standalone-local-store.sh
	bash suites/functional/endpoint/container-entrypoint.sh

functional-endpoint-container:
	bash suites/functional/endpoint/e2e-namespace-self-container.sh

functional-topology: doctor
	ENV=vm-topology SYSARMOR_VM_ENV=vm-topology bash suites/functional/topology/e2e-systemd-vm.sh

functional-platform:
	bash suites/functional/platform/e2e-control-contract.sh
	bash suites/functional/platform/e2e-agent-gateway-manager-local.sh
	bash suites/functional/platform/e2e-control-roundtrip-local.sh
	bash suites/functional/platform/e2e-manager-api-policy.sh
	bash suites/functional/platform/e2e-response-policy-deny.sh
	bash suites/functional/platform/e2e-response-scope-deny.sh
	bash suites/functional/platform/e2e-response-approval.sh
	bash suites/functional/platform/e2e-store-status.sh

functional-platform-full:
	bash shared/harness/start-container.sh
	bash suites/functional/topology/e2e-scenarios-container.sh

functional-core: functional-endpoint functional-topology functional-platform

# Capture ---------------------------------------------------------------------

capture-endpoint: doctor
	ENV=vm-endpoint SYSARMOR_VM_ENV=vm-endpoint bash shared/diagnostics/capture-vm.sh $(SCENARIO) $(DUR)

capture-topology: doctor
	ENV=vm-topology SYSARMOR_VM_ENV=vm-topology bash shared/diagnostics/capture-vm.sh $(SCENARIO) $(DUR)

# Detection and performance --------------------------------------------------

performance-endpoint: doctor
	ENV=vm-endpoint SYSARMOR_VM_ENV=vm-endpoint SYSARMOR_BENCH_PROFILE=$(SYSARMOR_BENCH_PROFILE) SYSARMOR_BENCH_WORKLOAD=$(SYSARMOR_BENCH_WORKLOAD) SYSARMOR_BENCH_SCENARIO=$(SYSARMOR_BENCH_SCENARIO) SYSARMOR_BENCH_POLICIES="$(SYSARMOR_BENCH_POLICIES)" bash suites/performance/endpoint/run.sh

performance-local-store:
	bash suites/performance/endpoint/local-store.sh

performance-platform:
	ENV=vm-topology SYSARMOR_VM_ENV=vm-topology bash suites/performance/platform/run.sh

detection-topology:
	ENV=vm-topology SYSARMOR_VM_ENV=vm-topology bash suites/detection/topology/run.sh

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

detection-report:
	python3 shared/reports/effectiveness_report.py --bench-matrix-dir .results/detection-topology/$(RUN_ID) --topology vm --scope $(EVALUATION_SCOPE) --scenarios $(or $(SCENARIOS),apt-fileless-c2 apt-staged-drop benign-ci-noise) --workloads $(or $(WORKLOADS),business-normal host-activity-heavy edr-activity-heavy) --output-dir .results/detection/$(RUN_ID)

# Distribution and release gates ---------------------------------------------

distribution-package:
	bash suites/distribution/package/unified-agent-installation.sh
	bash suites/distribution/package/transactional-agent-installation.sh
	bash suites/distribution/package/standalone-release-package.sh
	bash suites/distribution/package/standalone-github-assets.sh
	bash suites/distribution/package/release-workflow-contract.sh
	bash suites/distribution/package/release-container-e2e-contract.sh

distribution-published:
	$(MAKE) -C suites/distribution/published test

release-candidate: test-unit functional-core detection-topology performance-modules distribution-package

release-published: distribution-published

# Deprecated compatibility targets -------------------------------------------

product-endpoint: functional-endpoint
	@echo "[deprecated] use functional-endpoint"

product-endpoint-standalone: functional-endpoint-local distribution-package
	@echo "[deprecated] use functional-endpoint-local and distribution-package"

product-endpoint-release-container: distribution-published
	@echo "[deprecated] use distribution-published"

product-endpoint-namespace-container: functional-endpoint-container
	@echo "[deprecated] use functional-endpoint-container"

product-topology: functional-topology
	@echo "[deprecated] use functional-topology"

product-platform: functional-platform
	@echo "[deprecated] use functional-platform"

product-platform-smoke: functional-platform
	@echo "[deprecated] use functional-platform"

product-platform-full: functional-platform-full
	@echo "[deprecated] use functional-platform-full"

effectiveness-topology: detection-topology
	@echo "[deprecated] use detection-topology"

effectiveness-report: detection-report
	@echo "[deprecated] use detection-report"

test-all: test-unit functional-core
	@echo "[deprecated] use test-unit and functional-core"

report:
	python3 shared/reports/report.py

help:
	@echo "SysArmor test suites:"
	@echo "  doctor           verify tools, libvirt provider, connection, and Tetragon archive"
	@echo "  test-unit               local Go tests"
	@echo "  functional-endpoint     single VM endpoint with owned real Tetragon"
	@echo "  functional-platform     local platform behavior contracts"
	@echo "  functional-topology     multi-VM artifact, enrollment, and systemd path"
	@echo "  detection-topology      truth-labeled detection quality over vm-topology"
	@echo "  performance-endpoint endpoint CPU/RSS benchmark"
	@echo "  performance-platform platform service CPU/memory benchmark over vm-topology"
	@echo "  performance-modules local module microbenchmarks"
	@echo "  distribution-package   local package and installer contracts"
	@echo "  distribution-published published package compatibility matrix"
	@echo "  release-candidate       aggregate pre-publish release gate"
	@echo "  release-published       aggregate post-publish release gate"
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
	@echo "  make -C test detection-topology"
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
