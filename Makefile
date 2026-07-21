.DEFAULT_GOAL := help

.PHONY: api build build-agent-binary build-agent-tools build-binary business-docx install-agent uninstall-agent test test-help test-doctor test-unit test-performance test-opensearch-lifecycle test-business-docx up deploy down status reset clean clean-bin pki auth-init doctor release web-install web-dev web-up web-build web-preview web-status web-stop help

PROTO_FILES := $(shell find api/proto -name '*.proto' | sort)
GOCACHE ?= /tmp/sysarmor-go-cache
GOBIN_PATH := $(shell go env GOPATH)/bin
BIN_DIR ?= dist/bin
RELEASE_DIR ?= dist/release
BUSINESS_DOCX_SOURCE ?= docs/business/sysarmor-project-proposal.zh-CN.md
BUSINESS_DOCX_OUTPUT ?= dist/docs/sysarmor-project-proposal.zh-CN.docx
PACKAGE_BASE_URL ?= http://packages
RELEASE_VERSION ?= dev
RELEASE_OS ?= linux
RELEASE_ARCH ?= amd64
RELEASE_CHANNELS ?= dev-agent linux-systemd-dev linux-container-dev
RELEASE_AGENT_BIN ?= $(BIN_DIR)/sysarmor-agent
RELEASE_SIGNING_KEY ?= $(PKI_RUNTIME_DIR)/artifact-signing-key.pem
RELEASE_PUBLIC_KEY ?= $(PKI_RUNTIME_DIR)/artifact-public.pem
TETRAGON_ARCHIVE_CANDIDATE := $(firstword $(wildcard .cache/tetragon-v1.7.0-amd64.tar.gz .scratchpad/.cache/tetragon-v1.7.0-amd64.tar.gz))
TETRAGON_ARCHIVE ?= $(or $(SYSARMOR_TETRAGON_ARCHIVE),$(if $(TETRAGON_ARCHIVE_CANDIDATE),$(abspath $(TETRAGON_ARCHIVE_CANDIDATE))))
PROFILE ?= quick
WORKLOAD ?= business-normal
SCENARIO ?=
POLICIES ?=
COMPOSE ?= docker compose
PLATFORM_COMPOSE ?= deployments/compose.platform.yaml
PKI_RUNTIME_DIR ?= deployments/pki/agent-plane-mtls/runtime
WEB_DIR ?= web/manager
WEB_HOST ?= 127.0.0.1
WEB_DEV_PORT ?= 5173
WEB_PREVIEW_PORT ?= 4173
WEB_DEV_FLAGS ?= --webpack
WEB_RUN_DIR ?= .run
WEB_LOG ?= $(WEB_RUN_DIR)/manager-console.log
WEB_PID ?= $(WEB_RUN_DIR)/manager-console.pid

api:
	PATH="$(GOBIN_PATH):$$PATH" protoc --go_out=. --go_opt=paths=source_relative $(PROTO_FILES)
	PATH="$(GOBIN_PATH):$$PATH" protoc --go-grpc_out=. --go-grpc_opt=paths=source_relative $(PROTO_FILES)

build:
	@if [ -z "$(SERVICE)" ]; then \
		echo "usage: make build SERVICE=manager"; \
		exit 2; \
	elif [ "$(SERVICE)" = "packages" ]; then \
		echo "service packages uses image nginx:alpine; no build needed"; \
	else \
		$(COMPOSE) -f $(PLATFORM_COMPOSE) build $(SERVICE); \
	fi

build-agent-binary:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/sysarmor-agent ./cmd/sysarmor-agent

build-agent-tools: build-agent-binary
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/sysarmorctl ./cmd/sysarmorctl

install-agent: build-agent-tools
	sudo SYSARMOR_AGENT_BIN=$(BIN_DIR)/sysarmor-agent SYSARMOR_CTL_BIN=$(BIN_DIR)/sysarmorctl deployments/agent/install-agent.sh

uninstall-agent:
	sudo systemctl disable --now sysarmor-agent 2>/dev/null || true
	sudo rm -f /etc/systemd/system/sysarmor-agent.service /usr/local/bin/sysarmorctl
	sudo rm -rf /opt/sysarmor/agent /run/sysarmor/agent
	@if [ "$(PURGE)" = "1" ]; then sudo rm -rf /etc/sysarmor/agent /var/lib/sysarmor/agent; fi
	sudo systemctl daemon-reload

build-binary: build-agent-binary
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/sysarmor-gateway ./cmd/sysarmor-gateway
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/sysarmor-manager ./cmd/sysarmor-manager
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/sysarmor-worker ./cmd/sysarmor-worker
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/sysarmorctl ./cmd/sysarmorctl

business-docx:
	bash tools/docs/build-business-docx.sh "$(BUSINESS_DOCX_SOURCE)" "$(BUSINESS_DOCX_OUTPUT)"

test:
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go test ./...

test-help:
	$(MAKE) -C test help

test-doctor:
	$(MAKE) -C test doctor SYSARMOR_TETRAGON_ARCHIVE="$(TETRAGON_ARCHIVE)"

test-unit:
	$(MAKE) -C test test-unit

test-performance:
	$(MAKE) -C test performance-endpoint \
		SYSARMOR_TETRAGON_ARCHIVE="$(TETRAGON_ARCHIVE)" \
		SYSARMOR_BENCH_PROFILE=$(PROFILE) \
		SYSARMOR_BENCH_WORKLOAD=$(WORKLOAD) \
		SYSARMOR_BENCH_SCENARIO=$(SCENARIO) \
		SYSARMOR_BENCH_POLICIES="$(POLICIES)"

test-opensearch-lifecycle:
	bash test/suites/product/platform/opensearch-alias-lifecycle.sh

test-business-docx:
	bash test/suites/docs/business-docx.sh

pki:
	@if [ ! -f "$(PKI_RUNTIME_DIR)/gateway.pem" ] || [ ! -f "$(PKI_RUNTIME_DIR)/gateway-key.pem" ] || [ ! -f "$(PKI_RUNTIME_DIR)/ca.pem" ]; then \
		echo "Generating local agent-plane mTLS material in $(PKI_RUNTIME_DIR)"; \
		SYSARMOR_GATEWAY_IPS=127.0.0.1 tools/pki/gen-agent-plane-mtls.sh "$(PKI_RUNTIME_DIR)" default agent-prod-001 localhost; \
	fi
	@bash tools/pki/gen-manager-jwt.sh "$(PKI_RUNTIME_DIR)"

auth-init: pki
	@bash tools/auth/init-bootstrap-admin.sh "$(PKI_RUNTIME_DIR)"

doctor:
	@PLATFORM_COMPOSE="$(PLATFORM_COMPOSE)" PKI_RUNTIME_DIR="$(PKI_RUNTIME_DIR)" bash tools/doctor.sh

release: build-agent-binary pki
	bash deployments/packages/build-release.sh \
	  --version "$(RELEASE_VERSION)" \
	  --os "$(RELEASE_OS)" \
	  --arch "$(RELEASE_ARCH)" \
	  --output-dir "$(RELEASE_DIR)" \
	  --base-url "$(PACKAGE_BASE_URL)" \
	  --channels "$(RELEASE_CHANNELS)" \
	  --agent-bin "$(RELEASE_AGENT_BIN)" \
	  --tetragon-archive "$(TETRAGON_ARCHIVE)" \
	  --signing-key "$(RELEASE_SIGNING_KEY)" \
	  --public-key "$(RELEASE_PUBLIC_KEY)"

up: release auth-init
	@if [ -n "$(SERVICE)" ]; then \
		$(COMPOSE) -f $(PLATFORM_COMPOSE) up -d --remove-orphans $(SERVICE); \
	else \
		$(COMPOSE) -f $(PLATFORM_COMPOSE) up -d --remove-orphans; \
	fi

deploy: build-binary release auth-init
	$(COMPOSE) -f $(PLATFORM_COMPOSE) up -d --build --remove-orphans

down:
	@if [ -n "$(SERVICE)" ]; then \
		$(COMPOSE) -f $(PLATFORM_COMPOSE) stop $(SERVICE); \
		$(COMPOSE) -f $(PLATFORM_COMPOSE) rm -f $(SERVICE); \
	else \
		$(COMPOSE) -f $(PLATFORM_COMPOSE) down --remove-orphans; \
	fi

status:
	@if [ -n "$(SERVICE)" ]; then \
		$(COMPOSE) -f $(PLATFORM_COMPOSE) ps $(SERVICE); \
	else \
		$(COMPOSE) -f $(PLATFORM_COMPOSE) ps; \
	fi

clean:
	$(COMPOSE) -f $(PLATFORM_COMPOSE) down -v --remove-orphans

reset: clean up status

clean-bin:
	rm -rf $(BIN_DIR)

web-install:
	cd $(WEB_DIR) && pnpm install

web-dev:
	cd $(WEB_DIR) && pnpm exec next dev $(WEB_DEV_FLAGS) --hostname $(WEB_HOST) --port $(WEB_DEV_PORT)

web-up: web-build
	@WEB_DIR="$(CURDIR)/$(WEB_DIR)" WEB_HOST="$(WEB_HOST)" WEB_DEV_PORT="$(WEB_DEV_PORT)" WEB_PREVIEW_PORT="$(WEB_PREVIEW_PORT)" WEB_MODE=preview WEB_RUN_DIR="$(CURDIR)/$(WEB_RUN_DIR)" bash tools/web-console.sh up

web-build:
	cd $(WEB_DIR) && pnpm build

web-preview: web-build
	cd $(WEB_DIR) && pnpm start --hostname $(WEB_HOST) --port $(WEB_PREVIEW_PORT)

web-status:
	@WEB_HOST="$(WEB_HOST)" WEB_DEV_PORT="$(WEB_DEV_PORT)" WEB_PREVIEW_PORT="$(WEB_PREVIEW_PORT)" WEB_RUN_DIR="$(CURDIR)/$(WEB_RUN_DIR)" bash tools/web-console.sh status

web-stop:
	@WEB_HOST="$(WEB_HOST)" WEB_DEV_PORT="$(WEB_DEV_PORT)" WEB_PREVIEW_PORT="$(WEB_PREVIEW_PORT)" WEB_RUN_DIR="$(CURDIR)/$(WEB_RUN_DIR)" bash tools/web-console.sh stop

help:
	@echo "SysArmor project commands:"
	@echo "  make api        generate protobuf code"
	@echo "  make build SERVICE=manager  build a compose service image"
	@echo "  make build-binary           build agent/gateway/manager/worker/sysarmorctl"
	@echo "  make business-docx          build formal proposal DOCX under dist/docs/"
	@echo "  make install-agent          build and install a standalone Agent plus sysarmorctl"
	@echo "  make uninstall-agent        remove binaries; add PURGE=1 to remove config and local data"
	@echo "  make test       run Go tests"
	@echo "  make release    build signed agent release package and index"
	@echo "  make up         build release and start local platform"
	@echo "  make up SERVICE=packages    build release and start one service"
	@echo "  make deploy     build release, build images, and start local platform"
	@echo "  make down       stop local platform"
	@echo "  make down SERVICE=packages  stop and remove one service"
	@echo "  make status     show local platform service status"
	@echo "  make reset      DESTRUCTIVE: recreate data volumes and platform; preserve PKI"
	@echo "  make auth-init  create bootstrap admin and BFF secrets once"
	@echo "  make doctor     verify secrets, services, login, BFF, and Manager"
	@echo "  make clean      stop local platform and remove volumes/orphans"
	@echo "  make clean-bin  remove built binaries"
	@echo ""
	@echo "Web console:"
	@echo "  make web-install  install web dependencies"
	@echo "  make web-dev      start manager console dev server"
	@echo "  make web-up       build and start manager console preview in background"
	@echo "  make web-build    build manager console"
	@echo "  make web-preview  build and preview manager console"
	@echo "  make web-status   show manager console dev/preview status"
	@echo "  make web-stop     stop manager console dev/preview server"
	@echo "  WEB_DEV_FLAGS= make web-dev  use Next.js default dev bundler"
	@echo ""
	@echo "Test suites:"
	@echo "  make test-help         show all test suite commands"
	@echo "  make test-doctor       verify the complete test environment"
	@echo "  make test-unit         run local Go tests"
	@echo "  make test-business-docx validate the formal proposal DOCX build"
	@echo "  make test-performance PROFILE=medium WORKLOAD=business-normal SCENARIO=apt-fileless-c2-local POLICIES='test/data/policies/collection-balanced.json'"
