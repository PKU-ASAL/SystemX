# SysArmor 测试环境 Makefile
# 用法: make <target>   (从 test/ 目录运行)
#
# suites/    = 高层测试套件入口,定义测什么和评估边界
# harness/   = 通用执行胶水,负责启动/等待/采集/清理
# tools/     = recorder/report/benchmark/diagnostic 等可复用工具
# scenarios/ = 安全语义契约
# workloads/ = 压力输入,不做安全断言

TOPO      ?= container
SCENARIO  ?= apt-fileless-c2
DIAG_SCENARIO ?= mixed-edr-storm
DUR       ?= 30
EVALUATION_SCOPE ?= full

# ── 拓扑生命周期 ──────────────────────────────────────

.PHONY: up down provision status

up:                           ## 启动拓扑
	bash harness/start-$(TOPO).sh

down:                         ## 停止拓扑
	bash harness/stop-$(TOPO).sh

provision:                    ## VM: rsync + re-provision
	cd env/vm && vagrant rsync node-a && vagrant provision node-a

status:                       ## 查看运行态
	$(MAKE) status-$(TOPO)

status-container:
	docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Status}}'

status-vm:
	cd env/vm && vagrant status

# ── 场景采集 + 断言 ──────────────────────────────────

.PHONY: capture assert perf perf-resource perf-resource-container perf-resource-vm perf-resource-all recorder-vm-start recorder-vm-mark recorder-vm-stop recorder-vm-report diag-tetragon-vm diag-tetragon-vm-workload bench-collection-vm bench-matrix-vm bench-edr-lifecycle-vm bench-e2e-vm effectiveness-report report

capture:                      ## 跑场景 + tetragon 抓事件
	bash harness/capture-$(TOPO).sh $(SCENARIO) $(DUR)

assert:                       ## 通过 sysarmorctl 执行契约断言
ifeq ($(TOPO),vm)
	bash harness/assert-vm-local.sh $(SCENARIO)
else
	python3 harness/assert.py --scenario $(SCENARIO) \
		--topology $(TOPO) \
		--expected scenarios/$(TOPO)/$(SCENARIO)/expected.yaml
endif

report:                       ## 汇总报告矩阵
	python3 harness/report.py

# ── 性能 / Benchmark / 诊断 ──────────────────────────

perf:                         ## 采集 getevents 性能基线
	bash harness/perf-getevents.sh $(TOPO) $(DUR)

perf-resource:                ## legacy: 短窗口 EDR 资源占用时间序列
	bash harness/perf-resource.sh $(TOPO) $(DUR) $(SCENARIO)

perf-resource-container:      ## 采集 container EDR idle 资源占用
	bash harness/perf-resource.sh container $(DUR) edr-idle

perf-resource-vm:             ## 采集 VM EDR idle 资源占用
	bash harness/perf-resource.sh vm $(DUR) edr-idle

perf-resource-all: perf-resource-container perf-resource-vm ## 聚合采集 container/VM EDR idle 资源占用

recorder-vm-start:            ## 启动 VM 长期 performance recorder
	RUN_ID=$(or $(RUN_ID),manual) bash tools/recorder/recorder-vm.sh start

recorder-vm-mark:             ## 给 VM recorder 打 marker: PHASE=<name> [DETAIL=...]
	RUN_ID=$(or $(RUN_ID),manual) PHASE=$(PHASE) DETAIL="$(DETAIL)" bash tools/recorder/recorder-vm.sh mark

recorder-vm-stop:             ## 停止 VM recorder 并拉取 timeline
	RUN_ID=$(or $(RUN_ID),manual) bash tools/recorder/recorder-vm.sh stop

recorder-vm-report:           ## 根据 timeline + markers 生成 recorder summary
	RUN_ID=$(or $(RUN_ID),manual) bash tools/recorder/recorder-vm.sh report

diag-tetragon-vm:             ## 对运行中的 VM agent-owned Tetragon 采集 strace/perf/pprof 诊断产物
	bash tools/diagnostics/diagnose-tetragon-vm.sh

diag-tetragon-vm-workload:    ## 对运行中的 VM Tetragon 在 workload 下采集 perf/pprof 诊断产物
	SCENARIO=$(DIAG_SCENARIO) SYSARMOR_DIAG_MODE=workload SYSARMOR_DIAG_PREFIX=tetragon-diagnostics-vm.$(DIAG_SCENARIO).workload bash tools/diagnostics/diagnose-tetragon-vm.sh

bench-collection-vm:          ## 对多份 collection policy 跑 VM 真路径资源/eps benchmark matrix
	DIAG_SCENARIO=$(DIAG_SCENARIO) bash tools/benchmarks/bench-collection-vm.sh

bench-matrix-vm:              ## 固定跑 VM policy x workloads/scenarios 采集矩阵并生成效果报告
	bash tools/benchmarks/bench-matrix-vm.sh

bench-edr-lifecycle-vm:       ## 采集 VM EDR baseline/start/apply/steady/workload 生命周期曲线
	DIAG_SCENARIO=$(DIAG_SCENARIO) bash tools/benchmarks/bench-edr-lifecycle-vm.sh

bench-e2e-vm:                 ## 用 recorder 包住 VM 功能 E2E,同时产出功能结果和性能曲线
	SCENARIO=$(SCENARIO) bash tools/benchmarks/bench-e2e-vm.sh

effectiveness-report:         ## 从 expected.yaml + 已有结果生成 scenario 效果指标报告
	python3 tools/benchmarks/effectiveness_report.py --topology $(TOPO) --scope $(EVALUATION_SCOPE) --scenarios $(or $(SCENARIOS),apt-fileless-c2 apt-staged-drop benign-ci-noise) --output-dir .results/effectiveness/$(or $(RUN_ID),manual)

# ── Suite 入口 ───────────────────────────────────────

.PHONY: test-local-agent bench-local-agent test-agent-runtime test-manager-cloud test-control-plane test-reliability test-storage diagnostics-vm

test-local-agent:             ## suite: local agent VM real Tetragon endpoint smoke
	bash suites/local-agent/e2e-real-tetragon-owned-vm.sh

bench-local-agent:            ## suite: local agent policy x workload/scenario benchmark matrix
	bash suites/local-agent/bench-matrix-vm.sh

test-agent-runtime:           ## suite: local fake-sensor agent runtime health/capability/session smoke
	bash suites/agent-runtime/e2e-daemon.sh
	bash suites/agent-runtime/e2e-sensor-restart.sh
	bash suites/agent-runtime/e2e-sensor-recover.sh
	bash suites/agent-runtime/e2e-capability.sh
	bash suites/agent-runtime/e2e-capability-btf.sh
	bash suites/agent-runtime/e2e-capability-bpffs.sh
	bash suites/agent-runtime/e2e-parse-health.sh
	bash suites/agent-runtime/e2e-dropped-health.sh
	bash suites/agent-runtime/e2e-session.sh

test-manager-cloud:           ## suite: manager ingest/query/cloud signal/incident/policy/response
	bash suites/manager-cloud/e2e-manager-idempotency.sh
	bash suites/manager-cloud/e2e-agent-health.sh
	bash suites/manager-cloud/e2e-scenarios-container.sh
	bash suites/manager-cloud/e2e-policy.sh
	bash suites/manager-cloud/e2e-response.sh
	bash suites/manager-cloud/e2e-incident.sh

test-control-plane:           ## suite: control stream contract and mTLS identity
	bash suites/control-plane/e2e-contract.sh
	bash suites/control-plane/e2e-mtls.sh

test-reliability:             ## suite: spool/outage/shutdown/backpressure reliability
	bash suites/reliability/e2e-spool.sh
	bash suites/reliability/e2e-outage-soak.sh
	bash suites/reliability/e2e-shutdown.sh
	bash suites/reliability/e2e-backpressure.sh

test-storage:                 ## suite: Postgres/store projection and query contract
	bash suites/storage/e2e-store-status.sh
	bash suites/storage/e2e-query-pagination.sh
	bash suites/storage/e2e-postgres.sh

diagnostics-vm:               ## suite: VM diagnostic capture for agent-owned Tetragon
	bash tools/diagnostics/diagnose-tetragon-vm.sh

# ── 常用组合 / 细粒度 legacy 入口 ─────────────────────

.PHONY: e2e e2e-agent-manager-contract e2e-manager-idempotency e2e-agent-mtls e2e-agent-daemon e2e-agent-sensor-restart e2e-agent-sensor-recover e2e-agent-spool e2e-agent-outage-soak e2e-agent-shutdown e2e-agent-reliability-soak e2e-agent-capability e2e-agent-capability-btf e2e-agent-capability-bpffs e2e-agent-parse-health e2e-agent-dropped-health e2e-agent-backpressure e2e-agent-health e2e-agent-all e2e-agent-runtime-all e2e-policy-endpoint-disable e2e-policy-agent-refresh e2e-policy-cloud-disable e2e-policy-publish e2e-operator-role-bindings e2e-policy-all e2e-response-observe-only e2e-response-policy-deny e2e-response-scope-deny e2e-response-audit e2e-response-approval e2e-response-multi-approval e2e-response-all e2e-graph-evidence e2e-incident-lifecycle e2e-incident-attach-evidence e2e-incident-merge e2e-rarity-baseline e2e-graph-all e2e-store-status e2e-query-pagination e2e-postgres-store e2e-postgres-idempotency e2e-postgres-policy-persistence e2e-postgres-manager-api e2e-postgres-agent-projection e2e-postgres-ingest-projection e2e-postgres-event-query e2e-postgres-signal-query e2e-postgres-incident-query e2e-postgres-control-projection e2e-postgres-control-audit-projection e2e-postgres-response-projection e2e-postgres-response-write e2e-postgres-response-query e2e-postgres-policy-projection e2e-postgres-policy-write e2e-postgres-policy-query e2e-postgres-policy-get e2e-postgres-effective-policy-query e2e-postgres-incident-projection e2e-postgres-observability-projection e2e-postgres-agent-session-projection e2e-postgres-rarity-projection e2e-postgres-all e2e-agent-session e2e-agent-control-plane-contract e2e-agent-control-plane-all e2e-agent-daemon-container e2e-agent-managed-container e2e-agent-managed-recover-container e2e-agent-managed-restart-container e2e-agent-systemd-vm e2e-agent-managed-vm e2e-agent-managed-recover-vm e2e-agent-real-tetragon-owned-vm e2e-agent-real-tetragon-owned-container e2e-agent-apt-container e2e-agent-staged-container e2e-agent-benign-container e2e-agent-detection-container-all clean

e2e: up capture assert        ## 一键: 起拓扑 → 跑场景 → 断言

e2e-agent-manager-contract:   ## v3: AgentControlPlaneService.Connect sequence/replay and DataAck error-class contract
	cd .. && go test ./internal/agentplane -run 'Test(DataPlaneAppendBatch|ControlPlaneConnectSequenceRejectsReplayAndGap|ControlPlaneConnectReconnectReturnsResumeAndPendingCommands|ControlPlaneConnectRequiresRequestID|DataAckClassifiesRetryableBackendError|DataPlaneAppendBatchRequiresAgentIdentity)'
	cd .. && go test ./internal/agent/daemon -run 'Test(ControlChannelKeepsLongLivedContract|AgentRuntimeControlChannelProcessesPendingResponse)'

e2e-manager-idempotency:      ## v2: repeated DataBatch does not amplify manager ingest
	bash suites/manager-cloud/e2e-manager-idempotency.sh

e2e-agent-mtls:               ## v3: manager gRPC mTLS requires client cert and binds cert identity to agent_id
	bash suites/control-plane/e2e-mtls.sh

e2e-agent-daemon:             ## v2: 本机 fake sensor daemon → health/data-plane → manager 查询
	bash suites/agent-runtime/e2e-daemon.sh

e2e-agent-sensor-restart:     ## v2: fake tetragon bundle → restart/degraded/tamper → manager 查询
	bash suites/agent-runtime/e2e-sensor-restart.sh

e2e-agent-sensor-recover:     ## v2: fake tetragon bundle → degrade once → recover to running/ok
	bash suites/agent-runtime/e2e-sensor-recover.sh

e2e-agent-spool:              ## v2: manager outage → durable spool → recovery drain
	bash suites/reliability/e2e-spool.sh

e2e-agent-outage-soak:        ## v2: manager outage queues multiple batches → recovery drains all
	bash suites/reliability/e2e-outage-soak.sh

e2e-agent-shutdown:           ## v2: SIGTERM graceful shutdown → shutdown drain flush
	bash suites/reliability/e2e-shutdown.sh

e2e-agent-reliability-soak:   ## v2: longer reliability soak for outage/retry/restart/shutdown
	bash suites/reliability/e2e-reliability-soak.sh

e2e-agent-capability:         ## v2: startup capability/bundle failure → degraded health
	bash suites/agent-runtime/e2e-capability.sh

e2e-agent-capability-btf:     ## v2: required BTF capability failure → degraded health
	bash suites/agent-runtime/e2e-capability-btf.sh

e2e-agent-capability-bpffs:   ## v2: required bpffs capability failure → degraded health
	bash suites/agent-runtime/e2e-capability-bpffs.sh

e2e-agent-parse-health:       ## v2: sensor parse errors → degraded health/tamper signal
	bash suites/agent-runtime/e2e-parse-health.sh

e2e-agent-dropped-health:     ## v2: sensor dropped events → degraded health/tamper signal
	bash suites/agent-runtime/e2e-dropped-health.sh

e2e-agent-backpressure:       ## v2: spool max_bytes backpressure/drop → degraded health
	bash suites/reliability/e2e-backpressure.sh

e2e-agent-health:             ## v2: sysarmorctl agents/agent-health/metrics 查询 smoke
	bash suites/manager-cloud/e2e-agent-health.sh

e2e-agent-all: e2e-manager-idempotency e2e-agent-mtls e2e-agent-daemon e2e-agent-sensor-restart e2e-agent-sensor-recover e2e-agent-spool e2e-agent-outage-soak e2e-agent-shutdown e2e-agent-capability e2e-agent-capability-btf e2e-agent-capability-bpffs e2e-agent-parse-health e2e-agent-dropped-health e2e-agent-backpressure e2e-agent-health

e2e-agent-runtime-all: e2e-agent-all e2e-agent-detection-container-all e2e-agent-real-tetragon-owned-container e2e-agent-real-tetragon-owned-vm ## v2: end-to-end runtime and ownership gates

e2e-policy-endpoint-disable: ## v3: agent effective policy disables an endpoint rule
	bash suites/manager-cloud/e2e-policy-endpoint-disable.sh

e2e-policy-agent-refresh: ## v3: agent refreshes effective policy without restart
	bash suites/manager-cloud/e2e-policy-agent-refresh.sh

e2e-policy-cloud-disable: ## v3: manager policy assignment disables a cloud rule
	bash suites/manager-cloud/e2e-policy-cloud-disable.sh

e2e-policy-publish: ## v3: draft policy must be published before assignment/effective use and is audited
	bash suites/manager-cloud/e2e-policy-publish.sh

e2e-operator-role-bindings: ## v3: operator actor role bindings authorize control-plane writes
	cd .. && go test ./internal/managerapi -run TestOperatorRoleBindingsAuthorizeControlPlaneWrites
	cd .. && go test ./cmd/sysarmorctl -run TestOperatorRoleBindingsCommand
	cd .. && go test ./internal/store -run TestExportImportStateRoundTrip

e2e-policy-all: e2e-policy-endpoint-disable e2e-policy-agent-refresh e2e-policy-cloud-disable e2e-policy-publish e2e-operator-role-bindings ## v3: policy/control-plane gates

e2e-response-observe-only: ## v3: observe-only response command and audit
	bash suites/manager-cloud/e2e-response-observe-only.sh

e2e-response-policy-deny: ## v3: response policy denies destructive actions by default
	bash suites/manager-cloud/e2e-response-policy-deny.sh

e2e-response-scope-deny: ## v3: response command must match agent runtime scope
	bash suites/manager-cloud/e2e-response-scope-deny.sh

e2e-response-audit: ## v3: response intent converts into auditable observe decision
	bash suites/manager-cloud/e2e-response-audit.sh

e2e-response-approval: ## v3: approval-required response waits before agent control delivery
	bash suites/manager-cloud/e2e-response-approval.sh

e2e-response-multi-approval: ## v3: response policy can require multi-approval threshold and roles
	cd .. && go test ./internal/managerapi -run TestResponsePolicyCanRequireMultiApprovalRoles

e2e-response-all: e2e-response-observe-only e2e-response-policy-deny e2e-response-scope-deny e2e-response-audit e2e-response-approval e2e-response-multi-approval ## v3: response/enforce observe-only gates

e2e-graph-evidence: ## v3: incident evidence is queryable as graph JSON
	bash suites/manager-cloud/e2e-graph-evidence.sh

e2e-incident-lifecycle: ## v3: incident lifecycle status can be updated and queried
	bash suites/manager-cloud/e2e-incident-lifecycle.sh

e2e-incident-attach-evidence: ## v3: incident evidence can be attached and queried
	bash suites/manager-cloud/e2e-incident-attach-evidence.sh

e2e-incident-merge: ## v3: incidents can be merged by explicit id
	bash suites/manager-cloud/e2e-incident-merge.sh

e2e-rarity-baseline: ## v3: rarity baseline is updated by ingest and queryable
	cd .. && go test ./internal/managerapi -run TestUploadUpdatesRarityBaselineWithoutDuplicateAmplification

e2e-graph-all: e2e-graph-evidence e2e-incident-lifecycle e2e-incident-attach-evidence e2e-incident-merge e2e-rarity-baseline ## v3: graph/evidence/incident/rarity gates

e2e-store-status: ## v3: store backend and migration status are queryable
	bash suites/storage/e2e-store-status.sh

e2e-query-pagination: ## v3: query APIs support limit/offset pagination
	bash suites/storage/e2e-query-pagination.sh

e2e-postgres-store: ## v3: Postgres snapshot adapter can persist state and close database resources
	cd .. && go test ./internal/store/backend -run TestOpenPostgresRunsMigrationAndPersistsSnapshot
	cd .. && go test ./internal/store/backend -run TestOpenPostgresResultClosesDatabase

e2e-postgres-idempotency: ## v3: Postgres backend preserves duplicate ingest idempotency across reopen
	cd .. && go test ./internal/store/backend -run TestOpenPostgresPreservesIdempotentIngestAcrossReopen

e2e-postgres-policy-persistence: ## v3: Postgres backend persists policy assignment, audit, and incident state
	cd .. && go test ./internal/store/backend -run TestOpenPostgresPersistsPolicyAndIncidentStateAcrossReopen

e2e-postgres-manager-api: ## v3: Postgres backend supports manager ingest/query/policy/incident APIs
	cd .. && go test ./internal/store/backend -run TestOpenPostgresBacksManagerIngestQueryPolicyAndIncidentAPI

e2e-postgres-agent-projection: ## v3: Postgres backend projects agents and health into table paths
	cd .. && go test ./internal/store/backend -run TestOpenPostgresProjectsAgentInventoryTables

e2e-postgres-ingest-projection: ## v3: Postgres backend projects events and signals into table paths
	cd .. && go test ./internal/store/backend -run TestOpenPostgresProjectsEventSignalTables

e2e-postgres-event-query: ## v3: Postgres backend queries events through table path
	cd .. && go test ./internal/store/backend -run TestOpenPostgresQueriesEventsFromTablePath

e2e-postgres-signal-query: ## v3: Postgres backend queries signals through table path
	cd .. && go test ./internal/store/backend -run TestOpenPostgresQueriesSignalsFromTablePath

e2e-postgres-incident-query: ## v3: Postgres backend queries incidents through table path
	cd .. && go test ./internal/store/backend -run TestOpenPostgresQueriesIncidentsFromTablePath

e2e-postgres-control-projection: ## v3: Postgres backend projects rules and evidence pullbacks into table paths
	cd .. && go test ./internal/store/backend -run TestOpenPostgresProjectsRuleAndPullbackTables

e2e-postgres-control-audit-projection: ## v3: Postgres backend projects policy audit and operator roles into table paths
	cd .. && go test ./internal/store/backend -run TestOpenPostgresProjectsControlAuditTables

e2e-postgres-response-projection: ## v3: Postgres backend projects response audit into table path
	cd .. && go test ./internal/store/backend -run TestOpenPostgresProjectsResponseAuditTable

e2e-postgres-response-write: ## v3: Postgres backend writes response audit through table path
	cd .. && go test ./internal/store/backend -run TestOpenPostgresWritesResponseAuditTablePath

e2e-postgres-response-query: ## v3: Postgres backend queries response audit through table path
	cd .. && go test ./internal/store/backend -run TestOpenPostgresQueriesResponseAuditFromTablePath

e2e-postgres-policy-projection: ## v3: Postgres backend projects policies and assignments into table paths
	cd .. && go test ./internal/store/backend -run TestOpenPostgresProjectsPolicyTables

e2e-postgres-policy-write: ## v3: Postgres backend writes policy control tables directly
	cd .. && go test ./internal/store/backend -run TestOpenPostgresWritesPolicyControlTablePaths

e2e-postgres-policy-query: ## v3: Postgres backend queries policy control tables
	cd .. && go test ./internal/store/backend -run TestOpenPostgresQueriesPolicyControlTables

e2e-postgres-policy-get: ## v3: Postgres backend gets a policy through table path
	cd .. && go test ./internal/store/backend -run TestOpenPostgresGetsPolicyFromTablePath

e2e-postgres-effective-policy-query: ## v3: Postgres backend resolves effective policy through table paths
	cd .. && go test ./internal/store/backend -run TestOpenPostgresQueriesEffectivePolicyFromTablePath

e2e-postgres-incident-projection: ## v3: Postgres backend projects incidents and evidence into table paths
	cd .. && go test ./internal/store/backend -run TestOpenPostgresProjectsIncidentEvidenceTables

e2e-postgres-observability-projection: ## v3: Postgres backend projects incident events and metrics into table paths
	cd .. && go test ./internal/store/backend -run TestOpenPostgresProjectsIncidentEventsAndMetricsTables

e2e-postgres-agent-session-projection: ## v3: Postgres backend projects agent sessions into table path
	cd .. && go test ./internal/store/backend -run TestOpenPostgresProjectsAgentSessionTable

e2e-postgres-rarity-projection: ## v3: Postgres backend projects rarity baseline into table path
	cd .. && go test ./internal/store/backend -run TestOpenPostgresProjectsRarityBaselineTable

e2e-postgres-all: e2e-store-status e2e-query-pagination e2e-postgres-store e2e-postgres-idempotency e2e-postgres-policy-persistence e2e-postgres-manager-api e2e-postgres-agent-projection e2e-postgres-ingest-projection e2e-postgres-event-query e2e-postgres-signal-query e2e-postgres-incident-query e2e-postgres-control-projection e2e-postgres-control-audit-projection e2e-postgres-response-projection e2e-postgres-response-write e2e-postgres-response-query e2e-postgres-policy-projection e2e-postgres-policy-write e2e-postgres-policy-query e2e-postgres-policy-get e2e-postgres-effective-policy-query e2e-postgres-incident-projection e2e-postgres-observability-projection e2e-postgres-agent-session-projection e2e-postgres-rarity-projection ## v3: current Postgres/store foundation gates

e2e-agent-session: ## v3: data append updates agent session cursor state
	bash suites/agent-runtime/e2e-session.sh

e2e-agent-control-plane-contract: ## v3: long-lived AgentControlPlaneService.Connect policy/resume/response/evidence contract
	cd .. && go test ./internal/agent/daemon -run 'Test(ControlChannelKeepsLongLivedContract|AgentRuntimeControlChannelProcessesPendingResponse)'

e2e-agent-control-plane-all: e2e-agent-session e2e-agent-control-plane-contract ## v3: current AgentControlPlaneService.Connect foundation gates

e2e-agent-daemon-container:   ## v2: container topology fake daemon → manager 查询
	bash suites/agent-runtime/e2e-daemon-container.sh

e2e-agent-managed-container:  ## v2: container topology managed fake Tetragon bundle → manager 查询
	bash suites/agent-runtime/e2e-managed-container.sh

e2e-agent-managed-recover-container: ## v2: container managed fake Tetragon degraded to recovered smoke
	bash suites/agent-runtime/e2e-managed-recover-container.sh

e2e-agent-managed-restart-container: ## v2: container managed fake Tetragon restart/tamper smoke
	bash suites/agent-runtime/e2e-managed-restart-container.sh

e2e-agent-systemd-vm:         ## v2: VM systemd agent daemon → health/restart smoke
	bash suites/agent-runtime/e2e-systemd-vm.sh

e2e-agent-managed-vm:         ## v2: VM systemd managed fake Tetragon bundle → manager 查询
	bash suites/agent-runtime/e2e-managed-vm.sh

e2e-agent-managed-recover-vm: ## v2: VM systemd managed fake Tetragon degraded to recovered smoke
	bash suites/agent-runtime/e2e-managed-recover-vm.sh

e2e-agent-real-tetragon-owned-vm:   ## v3 endpoint: VM systemd agent-owned real Tetragon -> local ctl events/signals
	bash suites/local-agent/e2e-real-tetragon-owned-vm.sh

e2e-agent-real-tetragon-owned-container: ## v2: container agent-owned real Tetragon process → detection smoke
	bash suites/local-agent/e2e-real-tetragon-owned-container.sh

e2e-agent-apt-container:      ## v2: container apt-fileless-c2 via agent-managed Tetra subscription
	bash suites/manager-cloud/e2e-scenario-apt-container.sh

e2e-agent-staged-container:   ## v2: container apt-staged-drop via agent-managed Tetra subscription
	bash suites/manager-cloud/e2e-scenario-staged-container.sh

e2e-agent-benign-container:   ## v2: container benign-ci-noise via agent-managed Tetra subscription
	bash suites/manager-cloud/e2e-scenario-benign-container.sh

e2e-agent-detection-container-all: e2e-agent-apt-container e2e-agent-staged-container e2e-agent-benign-container ## v2: all container managed detection smokes

clean:                        ## 全部清理 (含样本)
	bash harness/cleanup.sh $(TOPO) --results

# ── 帮助 ──────────────────────────────────────────────

.PHONY: help

help:                         ## 显示帮助
	@echo "SysArmor 测试环境"
	@echo ""
	@echo "拓扑:  TOPO=container (默认) | vm"
	@echo "场景:  SCENARIO=apt-fileless-c2 (默认) | apt-staged-drop | benign-ci-noise"
	@echo "诊断场景: DIAG_SCENARIO=mixed-edr-storm (默认) | exec-storm | file-write-storm | network-connect-storm | VM attack.sh 场景"
	@echo ""
	@echo "targets:"
	@echo "  test-local-agent     suite: local agent endpoint smoke"
	@echo "  bench-local-agent    suite: local agent policy x workload/scenario benchmark"
	@echo "  test-manager-cloud   suite: manager ingest/query/cloud signal/incident/policy/response"
	@echo "  test-control-plane   suite: control stream contract and mTLS identity"
	@echo "  test-reliability     suite: spool/outage/shutdown/backpressure reliability"
	@echo "  test-storage         suite: Postgres/store projection and query contract"
	@echo "  diagnostics-vm       suite: VM diagnostic capture"
	@echo ""
	@echo "legacy/advanced targets:"
	@echo "  up          启动拓扑"
	@echo "  down        停止拓扑"
	@echo "  capture     跑场景 + 抓事件"
	@echo "  assert      通过 sysarmorctl 执行契约断言"
	@echo "  perf        采集 getevents 性能基线"
	@echo "  perf-resource  legacy 短窗口 EDR 资源占用时间序列"
	@echo "  perf-resource-all  legacy 聚合采集 container/VM EDR idle 资源占用"
	@echo "  recorder-vm-start/mark/stop/report  VM 长期 performance recorder"
	@echo "  diag-tetragon-vm  对运行中的 VM Tetragon 采集 strace/perf/pprof 诊断产物"
	@echo "  diag-tetragon-vm-workload  对运行中的 VM Tetragon 在 DIAG_SCENARIO 负载下采集 perf/pprof"
	@echo "  bench-collection-vm  对多份 collection policy 跑 VM 真路径资源/eps benchmark matrix"
	@echo "  bench-matrix-vm  固定跑 VM policy x workloads/scenarios 采集矩阵并生成效果报告"
	@echo "  bench-edr-lifecycle-vm  采集 VM EDR baseline/start/apply/steady/workload 生命周期曲线"
	@echo "  bench-e2e-vm  用 recorder 包住 VM 功能 E2E"
	@echo "  effectiveness-report  从 expected.yaml + 已有结果生成 scenario 效果指标报告"
	@echo "  report      汇总报告"
	@echo "  e2e         up + capture + assert"
	@echo "  e2e-manager-idempotency  v2 repeated DataBatch does not amplify manager ingest"
	@echo "  e2e-agent-mtls  v3 manager gRPC mTLS cert identity binding smoke"
	@echo "  e2e-agent-daemon  v2 fake daemon + health/data-plane manager smoke"
	@echo "  e2e-agent-sensor-restart  v2 managed sensor restart + tamper smoke"
	@echo "  e2e-agent-sensor-recover  v2 managed sensor degraded to recovered smoke"
	@echo "  e2e-agent-spool  v2 manager outage + durable spool recovery smoke"
	@echo "  e2e-agent-outage-soak  v2 manager outage queues multiple batches and drains all"
	@echo "  e2e-agent-shutdown  v2 graceful shutdown drain flush smoke"
	@echo "  e2e-agent-reliability-soak  v2 longer outage/sensor-restart/shutdown reliability soak"
	@echo "  e2e-agent-capability  v2 startup capability/bundle failure degraded health smoke"
	@echo "  e2e-agent-capability-btf  v2 required BTF capability failure degraded health smoke"
	@echo "  e2e-agent-capability-bpffs  v2 required bpffs capability failure degraded health smoke"
	@echo "  e2e-agent-parse-health  v2 parse error degraded health/tamper smoke"
	@echo "  e2e-agent-dropped-health  v2 dropped events degraded health/tamper smoke"
	@echo "  e2e-agent-backpressure  v2 spool backpressure/drop degraded health smoke"
	@echo "  e2e-agent-health  v2 sysarmorctl agent health query smoke"
	@echo "  e2e-agent-all  run all local v2 agent smoke tests"
	@echo "  e2e-agent-runtime-all  run local, container detection, and real owned-path runtime gates"
	@echo "  e2e-policy-endpoint-disable  v3 agent effective policy disables an endpoint rule"
	@echo "  e2e-policy-agent-refresh  v3 agent refreshes effective policy without restart"
	@echo "  e2e-policy-cloud-disable  v3 manager policy assignment disables a cloud rule"
	@echo "  e2e-operator-role-bindings  v3 operator actor role bindings authorize control-plane writes"
	@echo "  e2e-policy-all  run all v3 policy/control-plane gates"
	@echo "  e2e-response-observe-only  v3 observe-only response command and audit"
	@echo "  e2e-response-policy-deny  v3 response policy denies destructive actions by default"
	@echo "  e2e-response-scope-deny  v3 response command must match agent runtime scope"
	@echo "  e2e-response-audit  v3 response intent converts into auditable observe decision"
	@echo "  e2e-response-all  run all v3 response/enforce gates"
	@echo "  e2e-graph-evidence  v3 incident evidence is queryable as graph JSON"
	@echo "  e2e-incident-lifecycle  v3 incident lifecycle status can be updated and queried"
	@echo "  e2e-incident-attach-evidence  v3 incident evidence can be attached and queried"
	@echo "  e2e-incident-merge  v3 incidents can be merged by explicit id"
	@echo "  e2e-rarity-baseline  v3 rarity baseline is updated by ingest and queryable"
	@echo "  e2e-graph-all  run all v3 graph/evidence/incident/rarity gates"
	@echo "  e2e-store-status  v3 store backend and migration status are queryable"
	@echo "  e2e-query-pagination  v3 query APIs support limit/offset pagination"
	@echo "  e2e-postgres-agent-projection  v3 Postgres backend projects agents and health into table paths"
	@echo "  e2e-postgres-ingest-projection  v3 Postgres backend projects events and signals into table paths"
	@echo "  e2e-postgres-event-query  v3 Postgres backend queries events through table path"
	@echo "  e2e-postgres-signal-query  v3 Postgres backend queries signals through table path"
	@echo "  e2e-postgres-incident-query  v3 Postgres backend queries incidents through table path"
	@echo "  e2e-postgres-control-projection  v3 Postgres backend projects rules and evidence pullbacks into table paths"
	@echo "  e2e-postgres-control-audit-projection  v3 Postgres backend projects policy audit and operator roles into table paths"
	@echo "  e2e-postgres-response-projection  v3 Postgres backend projects response audit into table path"
	@echo "  e2e-postgres-response-write  v3 Postgres backend writes response audit through table path"
	@echo "  e2e-postgres-response-query  v3 Postgres backend queries response audit through table path"
	@echo "  e2e-postgres-policy-projection  v3 Postgres backend projects policies and assignments into table paths"
	@echo "  e2e-postgres-policy-write  v3 Postgres backend writes policy control tables directly"
	@echo "  e2e-postgres-policy-query  v3 Postgres backend queries policy control tables"
	@echo "  e2e-postgres-policy-get  v3 Postgres backend gets a policy through table path"
	@echo "  e2e-postgres-effective-policy-query  v3 Postgres backend resolves effective policy through table paths"
	@echo "  e2e-postgres-incident-projection  v3 Postgres backend projects incidents and evidence into table paths"
	@echo "  e2e-postgres-observability-projection  v3 Postgres backend projects incident events and metrics into table paths"
	@echo "  e2e-postgres-agent-session-projection  v3 Postgres backend projects agent sessions into table path"
	@echo "  e2e-postgres-rarity-projection  v3 Postgres backend projects rarity baseline into table path"
	@echo "  e2e-postgres-all  run current v3 Postgres/store foundation gates"
	@echo "  e2e-agent-session  v3 data append updates session cursor state"
	@echo "  e2e-agent-control-plane-contract  v3 long-lived AgentControlPlaneService.Connect contract"
	@echo "  e2e-agent-control-plane-all  run current v3 AgentControlPlaneService.Connect foundation gates"
	@echo "  e2e-agent-daemon-container  v2 container topology daemon smoke"
	@echo "  e2e-agent-managed-container  v2 container managed fake Tetragon bundle smoke"
	@echo "  e2e-agent-managed-recover-container  v2 container managed degraded to recovered smoke"
	@echo "  e2e-agent-managed-restart-container  v2 container managed fake Tetragon restart/tamper smoke"
	@echo "  e2e-agent-systemd-vm  v2 VM systemd daemon health/restart smoke"
	@echo "  e2e-agent-managed-vm  v2 VM systemd managed fake Tetragon bundle smoke"
	@echo "  e2e-agent-managed-recover-vm  v2 VM systemd managed degraded to recovered smoke"
	@echo "  e2e-agent-real-tetragon-owned-vm  v2 VM systemd agent-owned real Tetragon process smoke"
	@echo "  e2e-agent-real-tetragon-owned-container  v2 container agent-owned real Tetragon process smoke"
	@echo "  e2e-agent-apt-container  v2 container apt-fileless-c2 via agent-managed Tetra"
	@echo "  e2e-agent-staged-container  v2 container apt-staged-drop via agent-managed Tetra"
	@echo "  e2e-agent-benign-container  v2 container benign-ci-noise via agent-managed Tetra"
	@echo "  e2e-agent-detection-container-all  run all container managed detection smokes"
	@echo "  provision   VM: rsync + re-provision"
	@echo "  status      查看运行态"
	@echo "  clean       全部清理 + 删样本"
