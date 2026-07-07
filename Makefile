.DEFAULT_GOAL := help

.PHONY: api build test up deploy down status clean clean-bin pki help

PROTO_FILES := $(shell find api/proto -name '*.proto' | sort)
GOCACHE ?= /tmp/sysarmor-go-cache
GOBIN_PATH := $(shell go env GOPATH)/bin
BIN_DIR ?= dist/bin
COMPOSE ?= docker compose
PLATFORM_COMPOSE ?= deployments/compose.platform.yaml
PKI_RUNTIME_DIR ?= deployments/pki/agent-plane-mtls/runtime

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
	@echo "Test suites:"
	@echo "  make -C test help"
