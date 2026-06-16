.PHONY: api build test clean-bin

PROTO_FILES := $(shell find api/proto -name '*.proto' | sort)
GOCACHE ?= /tmp/sysarmor-go-cache
GOBIN_PATH := $(shell go env GOPATH)/bin
BIN_DIR ?= bin

api:
	PATH="$(GOBIN_PATH):$$PATH" protoc --go_out=. --go_opt=paths=source_relative $(PROTO_FILES)
	PATH="$(GOBIN_PATH):$$PATH" protoc --go-grpc_out=. --go-grpc_opt=paths=source_relative $(PROTO_FILES)

build:
	mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/sysarmor-agent ./cmd/sysarmor-agent
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/sysarmor-manager ./cmd/sysarmor-manager
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/sysarmor-worker ./cmd/sysarmor-worker
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go build -o $(BIN_DIR)/sysarmorctl ./cmd/sysarmorctl

test:
	CGO_ENABLED=0 GOCACHE=$(GOCACHE) go test ./...

clean-bin:
	rm -rf $(BIN_DIR)
