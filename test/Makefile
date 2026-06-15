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

.PHONY: capture assert perf report

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

# ── 常用组合 ──────────────────────────────────────────

.PHONY: e2e e2e-agent-daemon e2e-agent-sensor-restart e2e-agent-sensor-recover e2e-agent-spool e2e-agent-health e2e-agent-all e2e-agent-daemon-container e2e-agent-managed-container e2e-agent-managed-recover-container e2e-agent-managed-restart-container e2e-agent-systemd-vm e2e-agent-managed-vm e2e-agent-real-tetragon-vm e2e-agent-real-tetragon-owned-vm e2e-agent-apt-container e2e-agent-staged-container e2e-agent-benign-container e2e-agent-detection-container-all clean

e2e: up capture assert        ## 一键: 起拓扑 → 跑场景 → 断言

e2e-agent-daemon:             ## v2: 本机 fake sensor daemon → health/upload → manager 查询
	bash harness/e2e-agent-daemon.sh

e2e-agent-sensor-restart:     ## v2: fake tetragon bundle → restart/degraded/tamper → manager 查询
	bash harness/e2e-agent-sensor-restart.sh

e2e-agent-sensor-recover:     ## v2: fake tetragon bundle → degrade once → recover to running/ok
	bash harness/e2e-agent-sensor-recover.sh

e2e-agent-spool:              ## v2: manager outage → durable spool → recovery drain
	bash harness/e2e-agent-spool.sh

e2e-agent-health:             ## v2: sysarmorctl agents/agent-health/metrics 查询 smoke
	bash harness/e2e-agent-health.sh

e2e-agent-all: e2e-agent-daemon e2e-agent-sensor-restart e2e-agent-sensor-recover e2e-agent-spool e2e-agent-health

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

e2e-agent-real-tetragon-vm:   ## v2: VM systemd real Tetragon subscription → detection smoke
	bash harness/e2e-agent-real-tetragon-vm.sh

e2e-agent-real-tetragon-owned-vm:   ## v2: VM systemd agent-owned real Tetragon process → detection smoke
	bash harness/e2e-agent-real-tetragon-owned-vm.sh

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
	@echo "  report      汇总报告"
	@echo "  e2e         up + capture + assert"
	@echo "  e2e-agent-daemon  v2 fake daemon + health/upload manager smoke"
	@echo "  e2e-agent-sensor-restart  v2 managed sensor restart + tamper smoke"
	@echo "  e2e-agent-sensor-recover  v2 managed sensor degraded to recovered smoke"
	@echo "  e2e-agent-spool  v2 manager outage + durable spool recovery smoke"
	@echo "  e2e-agent-health  v2 sysarmorctl agent health query smoke"
	@echo "  e2e-agent-all  run all local v2 agent smoke tests"
	@echo "  e2e-agent-daemon-container  v2 container topology daemon smoke"
	@echo "  e2e-agent-managed-container  v2 container managed fake Tetragon bundle smoke"
	@echo "  e2e-agent-managed-recover-container  v2 container managed degraded to recovered smoke"
	@echo "  e2e-agent-managed-restart-container  v2 container managed fake Tetragon restart/tamper smoke"
	@echo "  e2e-agent-systemd-vm  v2 VM systemd daemon health/restart smoke"
	@echo "  e2e-agent-managed-vm  v2 VM systemd managed fake Tetragon bundle smoke"
	@echo "  e2e-agent-real-tetragon-vm  v2 VM systemd real Tetragon detection smoke"
	@echo "  e2e-agent-real-tetragon-owned-vm  v2 VM systemd agent-owned real Tetragon process smoke"
	@echo "  e2e-agent-apt-container  v2 container apt-fileless-c2 via agent-managed Tetra"
	@echo "  e2e-agent-staged-container  v2 container apt-staged-drop via agent-managed Tetra"
	@echo "  e2e-agent-benign-container  v2 container benign-ci-noise via agent-managed Tetra"
	@echo "  e2e-agent-detection-container-all  run all container managed detection smokes"
	@echo "  provision   VM: rsync + re-provision"
	@echo "  status      查看运行态"
	@echo "  clean       全部清理 + 删样本"
