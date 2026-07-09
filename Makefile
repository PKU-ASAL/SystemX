.DEFAULT_GOAL := help

.PHONY: api build test up deploy down status clean clean-bin pki web-install web-dev web-up web-build web-preview web-status web-stop help

PROTO_FILES := $(shell find api/proto -name '*.proto' | sort)
GOCACHE ?= /tmp/sysarmor-go-cache
GOBIN_PATH := $(shell go env GOPATH)/bin
BIN_DIR ?= dist/bin
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
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/sysarmor-agent ./cmd/sysarmor-agent
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/sysarmor-gateway ./cmd/sysarmor-gateway
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/sysarmor-manager ./cmd/sysarmor-manager
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/sysarmor-worker ./cmd/sysarmor-worker
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/sysarmorctl ./cmd/sysarmorctl

test:
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go test ./...

pki:
	@if [ ! -f "$(PKI_RUNTIME_DIR)/gateway.pem" ] || [ ! -f "$(PKI_RUNTIME_DIR)/gateway-key.pem" ] || [ ! -f "$(PKI_RUNTIME_DIR)/ca.pem" ]; then \
		echo "Generating local agent-plane mTLS material in $(PKI_RUNTIME_DIR)"; \
		SYSARMOR_GATEWAY_IPS=127.0.0.1 tools/pki/gen-agent-plane-mtls.sh "$(PKI_RUNTIME_DIR)" default agent-prod-001 localhost; \
	fi

up: pki
	$(COMPOSE) -f $(PLATFORM_COMPOSE) up -d

deploy: build pki
	$(COMPOSE) -f $(PLATFORM_COMPOSE) up -d --build

down:
	$(COMPOSE) -f $(PLATFORM_COMPOSE) down

status:
	$(COMPOSE) -f $(PLATFORM_COMPOSE) ps

clean:
	$(COMPOSE) -f $(PLATFORM_COMPOSE) down -v --remove-orphans

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
	@echo "  make build      build agent/gateway/manager/worker/sysarmorctl"
	@echo "  make test       run Go tests"
	@echo "  make up         start local platform: manager/gateway/worker/infra"
	@echo "  make deploy     build latest platform images and start them"
	@echo "  make down       stop local platform"
	@echo "  make status     show local platform service status"
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
	@echo "  make -C test help"
