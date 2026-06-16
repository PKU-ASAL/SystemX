# SysArmor 测试环境 Makefile
# 用法: make <target>   (从 test/ 目录运行)
#
# env/       = 搭建 (拓扑定义 + 镜像 + 资源)
# scenarios/ = 执行 (攻击脚本 + 期望契约 + PolicyEnvelope)
# harness/   = 编排 (start/stop/cleanup/capture/assert/report)

TOPO      ?= container
SCENARIO  ?= apt-fileless-c2
DUR       ?= 30

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

.PHONY: capture assert perf perf-resource report

capture:                      ## 跑场景 + tetragon 抓事件
	bash harness/capture-$(TOPO).sh $(SCENARIO) $(DUR)

assert:                       ## 通过 sysarmorctl 执行契约断言
	python3 harness/assert.py --scenario $(SCENARIO) \
		--topology $(TOPO) \
		--expected scenarios/$(TOPO)/$(SCENARIO)/expected.yaml

report:                       ## 汇总报告矩阵
	python3 harness/report.py

perf:                         ## 采集 getevents 性能基线
	bash harness/perf-getevents.sh $(TOPO) $(DUR)

perf-resource:                ## 采集 EDR 资源占用时间序列
	bash harness/perf-resource.sh $(TOPO) $(DUR) $(SCENARIO)

# ── 常用组合 ──────────────────────────────────────────

.PHONY: e2e e2e-manager-idempotency e2e-agent-daemon e2e-agent-sensor-restart e2e-agent-sensor-recover e2e-agent-spool e2e-agent-outage-soak e2e-agent-shutdown e2e-agent-restart-unacked e2e-agent-reliability-soak e2e-agent-capability e2e-agent-capability-btf e2e-agent-capability-bpffs e2e-agent-retry-backoff e2e-agent-parse-health e2e-agent-dropped-health e2e-agent-backpressure e2e-agent-health e2e-agent-all e2e-agent-runtime-all e2e-policy-endpoint-disable e2e-policy-agent-refresh e2e-policy-cloud-disable e2e-policy-all e2e-response-observe-only e2e-response-policy-deny e2e-response-scope-deny e2e-response-audit e2e-response-approval e2e-response-all e2e-graph-evidence e2e-incident-lifecycle e2e-incident-attach-evidence e2e-incident-merge e2e-graph-all e2e-store-status e2e-query-pagination e2e-postgres-all e2e-link1-session e2e-link1-downlink e2e-link1-frames e2e-link1-grpc-stream e2e-link1-stream-upload e2e-link1-policy-downlink e2e-link1-response-command e2e-link1-stream-all e2e-agent-daemon-container e2e-agent-managed-container e2e-agent-managed-recover-container e2e-agent-managed-restart-container e2e-agent-systemd-vm e2e-agent-managed-vm e2e-agent-managed-recover-vm e2e-agent-real-tetragon-vm e2e-agent-real-tetragon-owned-vm e2e-agent-real-tetragon-owned-container e2e-agent-apt-container e2e-agent-staged-container e2e-agent-benign-container e2e-agent-detection-container-all clean

e2e: up capture assert        ## 一键: 起拓扑 → 跑场景 → 断言

e2e-manager-idempotency:      ## v2: repeated Link1 batch does not amplify manager ingest
	bash harness/e2e-manager-idempotency.sh

e2e-agent-daemon:             ## v2: 本机 fake sensor daemon → health/upload → manager 查询
	bash harness/e2e-agent-daemon.sh

e2e-agent-sensor-restart:     ## v2: fake tetragon bundle → restart/degraded/tamper → manager 查询
	bash harness/e2e-agent-sensor-restart.sh

e2e-agent-sensor-recover:     ## v2: fake tetragon bundle → degrade once → recover to running/ok
	bash harness/e2e-agent-sensor-recover.sh

e2e-agent-spool:              ## v2: manager outage → durable spool → recovery drain
	bash harness/e2e-agent-spool.sh

e2e-agent-outage-soak:        ## v2: manager outage queues multiple batches → recovery drains all
	bash harness/e2e-agent-outage-soak.sh

e2e-agent-shutdown:           ## v2: SIGTERM graceful shutdown → shutdown drain flush
	bash harness/e2e-agent-shutdown.sh

e2e-agent-restart-unacked:    ## v2: mismatched ack keeps batch → agent restart → valid ack drains
	bash harness/e2e-agent-restart-unacked.sh

e2e-agent-reliability-soak:   ## v2: longer reliability soak for outage/retry/restart/shutdown
	bash harness/e2e-agent-reliability-soak.sh

e2e-agent-capability:         ## v2: startup capability/bundle failure → degraded health
	bash harness/e2e-agent-capability.sh

e2e-agent-capability-btf:     ## v2: required BTF capability failure → degraded health
	bash harness/e2e-agent-capability-btf.sh

e2e-agent-capability-bpffs:   ## v2: required bpffs capability failure → degraded health
	bash harness/e2e-agent-capability-bpffs.sh

e2e-agent-retry-backoff:      ## v2: upload retry/backoff soak against transient 503s
	bash harness/e2e-agent-retry-backoff.sh

e2e-agent-parse-health:       ## v2: sensor parse errors → degraded health/tamper signal
	bash harness/e2e-agent-parse-health.sh

e2e-agent-dropped-health:     ## v2: sensor dropped events → degraded health/tamper signal
	bash harness/e2e-agent-dropped-health.sh

e2e-agent-backpressure:       ## v2: spool max_bytes backpressure/drop → degraded health
	bash harness/e2e-agent-backpressure.sh

e2e-agent-health:             ## v2: sysarmorctl agents/agent-health/metrics 查询 smoke
	bash harness/e2e-agent-health.sh

e2e-agent-all: e2e-manager-idempotency e2e-agent-daemon e2e-agent-sensor-restart e2e-agent-sensor-recover e2e-agent-spool e2e-agent-outage-soak e2e-agent-shutdown e2e-agent-restart-unacked e2e-agent-capability e2e-agent-capability-btf e2e-agent-capability-bpffs e2e-agent-retry-backoff e2e-agent-parse-health e2e-agent-dropped-health e2e-agent-backpressure e2e-agent-health

e2e-agent-runtime-all: e2e-agent-all e2e-agent-detection-container-all e2e-agent-real-tetragon-owned-container e2e-agent-real-tetragon-owned-vm ## v2: end-to-end runtime and ownership gates

e2e-policy-endpoint-disable: ## v3: agent effective policy disables an endpoint rule
	bash harness/e2e-policy-endpoint-disable.sh

e2e-policy-agent-refresh: ## v3: agent refreshes effective policy without restart
	bash harness/e2e-policy-agent-refresh.sh

e2e-policy-cloud-disable: ## v3: manager policy assignment disables a cloud rule
	bash harness/e2e-policy-cloud-disable.sh

e2e-policy-all: e2e-policy-endpoint-disable e2e-policy-agent-refresh e2e-policy-cloud-disable ## v3: policy/control-plane gates

e2e-response-observe-only: ## v3: observe-only response command and audit
	bash harness/e2e-response-observe-only.sh

e2e-response-policy-deny: ## v3: response policy denies destructive actions by default
	bash harness/e2e-response-policy-deny.sh

e2e-response-scope-deny: ## v3: response command must match agent runtime scope
	bash harness/e2e-response-scope-deny.sh

e2e-response-audit: ## v3: response intent converts into auditable observe decision
	bash harness/e2e-response-audit.sh

e2e-response-approval: ## v3: approval-required response waits before agent downlink
	bash harness/e2e-response-approval.sh

e2e-response-all: e2e-response-observe-only e2e-response-policy-deny e2e-response-scope-deny e2e-response-audit e2e-response-approval ## v3: response/enforce observe-only gates

e2e-graph-evidence: ## v3: incident evidence is queryable as graph JSON
	bash harness/e2e-graph-evidence.sh

e2e-incident-lifecycle: ## v3: incident lifecycle status can be updated and queried
	bash harness/e2e-incident-lifecycle.sh

e2e-incident-attach-evidence: ## v3: incident evidence can be attached and queried
	bash harness/e2e-incident-attach-evidence.sh

e2e-incident-merge: ## v3: incidents can be merged by explicit id
	bash harness/e2e-incident-merge.sh

e2e-graph-all: e2e-graph-evidence e2e-incident-lifecycle e2e-incident-attach-evidence e2e-incident-merge ## v3: graph/evidence/incident gates

e2e-store-status: ## v3: store backend and migration status are queryable
	bash harness/e2e-store-status.sh

e2e-query-pagination: ## v3: query APIs support limit/offset pagination
	bash harness/e2e-query-pagination.sh

e2e-postgres-all: e2e-store-status e2e-query-pagination ## v3: current Postgres/store foundation gates

e2e-link1-session: ## v3: Link1 upload updates session cursor state
	bash harness/e2e-link1-session.sh

e2e-link1-downlink: ## v3: Link1 downlink frames expose policy and response commands
	bash harness/e2e-link1-downlink.sh

e2e-link1-frames: ## v3: Link1 uplink frames accept upload, health, ack, evidence pullback result and error
	bash harness/e2e-link1-frames.sh

e2e-link1-grpc-stream: ## v3: Link1 gRPC bidi stream exchanges downlink and uplink frames
	cd .. && go test ./internal/transport/link1 -run TestGRPCStreamExchangesDownlinkAndUplinkFrames

e2e-link1-stream-upload: ## v3: agent uploader can upload through Link1 gRPC stream
	cd .. && go test ./internal/endpoint/uploader -run TestStreamUploaderUploadsThroughLink1Stream

e2e-link1-policy-downlink: ## v3: agent can fetch effective policy through Link1 stream downlink
	cd .. && go test ./internal/agent/daemon -run TestStreamPolicyClientFetchesDownlinkPolicy

e2e-link1-response-command: ## v3: agent can ack observe-only response command through Link1 stream
	cd .. && go test ./internal/agent/daemon -run TestStreamResponseClientFetchesCommandAndAcks

e2e-link1-evidence-pullback: ## v3: agent can handle evidence pullback through Link1 stream
	cd .. && go test ./internal/agent/daemon -run TestStreamEvidenceClientHandlesPullbackResult

e2e-link1-stream-all: e2e-link1-session e2e-link1-downlink e2e-link1-frames e2e-link1-grpc-stream e2e-link1-stream-upload e2e-link1-policy-downlink e2e-link1-response-command e2e-link1-evidence-pullback ## v3: current Link1 stream foundation gates

e2e-agent-daemon-container:   ## v2: container topology fake daemon → manager 查询
	bash harness/e2e-agent-daemon-container.sh

e2e-agent-managed-container:  ## v2: container topology managed fake Tetragon bundle → manager 查询
	bash harness/e2e-agent-managed-container.sh

e2e-agent-managed-recover-container: ## v2: container managed fake Tetragon degraded to recovered smoke
	bash harness/e2e-agent-managed-recover-container.sh

e2e-agent-managed-restart-container: ## v2: container managed fake Tetragon restart/tamper smoke
	bash harness/e2e-agent-managed-restart-container.sh

e2e-agent-systemd-vm:         ## v2: VM systemd agent daemon → health/restart smoke
	bash harness/e2e-agent-systemd-vm.sh

e2e-agent-managed-vm:         ## v2: VM systemd managed fake Tetragon bundle → manager 查询
	bash harness/e2e-agent-managed-vm.sh

e2e-agent-managed-recover-vm: ## v2: VM systemd managed fake Tetragon degraded to recovered smoke
	bash harness/e2e-agent-managed-recover-vm.sh

e2e-agent-real-tetragon-vm:   ## v2: VM systemd real Tetragon subscription → detection smoke
	bash harness/e2e-agent-real-tetragon-vm.sh

e2e-agent-real-tetragon-owned-vm:   ## v2: VM systemd agent-owned real Tetragon process → detection smoke
	bash harness/e2e-agent-real-tetragon-owned-vm.sh

e2e-agent-real-tetragon-owned-container: ## v2: container agent-owned real Tetragon process → detection smoke
	bash harness/e2e-agent-real-tetragon-owned-container.sh

e2e-agent-apt-container:      ## v2: container apt-fileless-c2 via agent-managed Tetra subscription
	bash harness/e2e-agent-apt-container.sh

e2e-agent-staged-container:   ## v2: container apt-staged-drop via agent-managed Tetra subscription
	bash harness/e2e-agent-staged-container.sh

e2e-agent-benign-container:   ## v2: container benign-ci-noise via agent-managed Tetra subscription
	bash harness/e2e-agent-benign-container.sh

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
	@echo ""
	@echo "targets:"
	@echo "  up          启动拓扑"
	@echo "  down        停止拓扑"
	@echo "  capture     跑场景 + 抓事件"
	@echo "  assert      通过 sysarmorctl 执行契约断言"
	@echo "  perf        采集 getevents 性能基线"
	@echo "  perf-resource  采集 EDR 资源占用时间序列"
	@echo "  report      汇总报告"
	@echo "  e2e         up + capture + assert"
	@echo "  e2e-manager-idempotency  v2 repeated Link1 batch does not amplify manager ingest"
	@echo "  e2e-agent-daemon  v2 fake daemon + health/upload manager smoke"
	@echo "  e2e-agent-sensor-restart  v2 managed sensor restart + tamper smoke"
	@echo "  e2e-agent-sensor-recover  v2 managed sensor degraded to recovered smoke"
	@echo "  e2e-agent-spool  v2 manager outage + durable spool recovery smoke"
	@echo "  e2e-agent-outage-soak  v2 manager outage queues multiple batches and drains all"
	@echo "  e2e-agent-shutdown  v2 graceful shutdown drain flush smoke"
	@echo "  e2e-agent-restart-unacked  v2 agent restart preserves unacked batch smoke"
	@echo "  e2e-agent-reliability-soak  v2 longer outage/retry/restart/shutdown reliability soak"
	@echo "  e2e-agent-capability  v2 startup capability/bundle failure degraded health smoke"
	@echo "  e2e-agent-capability-btf  v2 required BTF capability failure degraded health smoke"
	@echo "  e2e-agent-capability-bpffs  v2 required bpffs capability failure degraded health smoke"
	@echo "  e2e-agent-retry-backoff  v2 upload retry/backoff soak against transient 503s"
	@echo "  e2e-agent-parse-health  v2 parse error degraded health/tamper smoke"
	@echo "  e2e-agent-dropped-health  v2 dropped events degraded health/tamper smoke"
	@echo "  e2e-agent-backpressure  v2 spool backpressure/drop degraded health smoke"
	@echo "  e2e-agent-health  v2 sysarmorctl agent health query smoke"
	@echo "  e2e-agent-all  run all local v2 agent smoke tests"
	@echo "  e2e-agent-runtime-all  run local, container detection, and real owned-path runtime gates"
	@echo "  e2e-policy-endpoint-disable  v3 agent effective policy disables an endpoint rule"
	@echo "  e2e-policy-agent-refresh  v3 agent refreshes effective policy without restart"
	@echo "  e2e-policy-cloud-disable  v3 manager policy assignment disables a cloud rule"
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
	@echo "  e2e-graph-all  run all v3 graph/evidence/incident gates"
	@echo "  e2e-store-status  v3 store backend and migration status are queryable"
	@echo "  e2e-query-pagination  v3 query APIs support limit/offset pagination"
	@echo "  e2e-postgres-all  run current v3 Postgres/store foundation gates"
	@echo "  e2e-link1-session  v3 Link1 upload updates session cursor state"
	@echo "  e2e-link1-downlink  v3 Link1 downlink frames expose policy and response commands"
	@echo "  e2e-link1-frames  v3 Link1 uplink frames accept upload, health, ack, evidence pullback result and error"
	@echo "  e2e-link1-grpc-stream  v3 Link1 gRPC bidi stream exchanges downlink and uplink frames"
	@echo "  e2e-link1-stream-upload  v3 agent uploader can upload through Link1 gRPC stream"
	@echo "  e2e-link1-policy-downlink  v3 agent can fetch effective policy through Link1 stream downlink"
	@echo "  e2e-link1-response-command  v3 agent can ack observe-only response command through Link1 stream"
	@echo "  e2e-link1-stream-all  run current v3 Link1 stream foundation gates"
	@echo "  e2e-agent-daemon-container  v2 container topology daemon smoke"
	@echo "  e2e-agent-managed-container  v2 container managed fake Tetragon bundle smoke"
	@echo "  e2e-agent-managed-recover-container  v2 container managed degraded to recovered smoke"
	@echo "  e2e-agent-managed-restart-container  v2 container managed fake Tetragon restart/tamper smoke"
	@echo "  e2e-agent-systemd-vm  v2 VM systemd daemon health/restart smoke"
	@echo "  e2e-agent-managed-vm  v2 VM systemd managed fake Tetragon bundle smoke"
	@echo "  e2e-agent-managed-recover-vm  v2 VM systemd managed degraded to recovered smoke"
	@echo "  e2e-agent-real-tetragon-vm  v2 VM systemd real Tetragon detection smoke"
	@echo "  e2e-agent-real-tetragon-owned-vm  v2 VM systemd agent-owned real Tetragon process smoke"
	@echo "  e2e-agent-real-tetragon-owned-container  v2 container agent-owned real Tetragon process smoke"
	@echo "  e2e-agent-apt-container  v2 container apt-fileless-c2 via agent-managed Tetra"
	@echo "  e2e-agent-staged-container  v2 container apt-staged-drop via agent-managed Tetra"
	@echo "  e2e-agent-benign-container  v2 container benign-ci-noise via agent-managed Tetra"
	@echo "  e2e-agent-detection-container-all  run all container managed detection smokes"
	@echo "  provision   VM: rsync + re-provision"
	@echo "  status      查看运行态"
	@echo "  clean       全部清理 + 删样本"
