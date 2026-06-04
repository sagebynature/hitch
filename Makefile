.PHONY: help build test test-go test-adapters run serve status doctor install-dry-run clean

BINARY ?= bin/hitch
CLIENT_BINARY ?= bin/hitch-client
CONFIG ?= config/default.config.toml
GO_PACKAGES ?= ./...
ADAPTER_TESTS ?= adapters/**/*.test.ts

help:
	@printf '%s\n' \
		'Common targets:' \
		'  make build           Build hitch and hitch-client into bin/' \
		'  make test            Run Go and adapter tests' \
		'  make test-go         Run Go tests' \
		'  make test-adapters   Run TypeScript adapter tests with bun' \
		'  make run             Run hitch from source' \
		'  make serve           Run the local Hitch server' \
		'  make status          Print CLI status as JSON' \
		'  make doctor          Run CLI doctor as JSON' \
		'  make install-dry-run Preview integration placeholder install' \
		'  make clean           Remove build outputs'

build:
	@mkdir -p $(dir $(BINARY)) $(dir $(CLIENT_BINARY))
	go build -o $(BINARY) ./cmd/hitch
	go build -o $(CLIENT_BINARY) ./cmd/hitch-client

test: test-go test-adapters

test-go:
	go test $(GO_PACKAGES)

test-adapters:
	bun test $(ADAPTER_TESTS)

run:
	go run ./cmd/hitch

serve:
	go run ./cmd/hitch serve --config $(CONFIG)

status:
	go run ./cmd/hitch status --json

doctor:
	go run ./cmd/hitch doctor --json

install-dry-run:
	go run ./cmd/hitch install --all --dry-run --json

clean:
	rm -rf bin
