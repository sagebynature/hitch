.PHONY: help build test test-go vet lint check run serve status doctor install-dry-run clean preview-pages docker-build docker-prepare docker-run

BINARY ?= bin/hitch
CLIENT_BINARY ?= bin/hitch-client
CONFIG ?= internal/config/default.config.toml
GO_PACKAGES ?= ./...
DOCKER_IMAGE ?= hitch:local
DOCKER_PORT ?= 8799
HITCH_CONFIG_DIR ?= $(HOME)/.config/hitch
HITCH_UID ?= $(shell id -u)
HITCH_GID ?= $(shell id -g)

help:
	@printf '%s\n' \
		'Common targets:' \
		'  make build           Build hitch and hitch-client into bin/' \
		'  make docker-build    Build the Hitch container image' \
		'  make docker-run      Run the Hitch container image' \
		'  make test            Run Go tests' \
		'  make run             Run hitch from source' \
		'  make serve           Run the local Hitch server' \
		'  make status          Print CLI status as JSON' \
		'  make doctor          Run CLI doctor as JSON' \
		'  make install-dry-run Preview integration placeholder install' \
		'  make clean           Remove build outputs' \
		'  make preview-pages   Preview the Hitch documentation pages'

build:
	@mkdir -p $(dir $(BINARY)) $(dir $(CLIENT_BINARY))
	go build -o $(BINARY) ./cmd/hitch
	go build -o $(CLIENT_BINARY) ./cmd/hitch-client

docker-build:
	docker build -t $(DOCKER_IMAGE) .


docker-prepare:
	@mkdir -p "$(HITCH_CONFIG_DIR)/extensions" "$(HITCH_CONFIG_DIR)/backups"
	@if [ ! -e "$(HITCH_CONFIG_DIR)/config.toml" ]; then \
		cp internal/config/default.config.toml "$(HITCH_CONFIG_DIR)/config.toml"; \
	fi
docker-run: docker-prepare
	DOCKER_IMAGE="$(DOCKER_IMAGE)" DOCKER_PORT="$(DOCKER_PORT)" HITCH_CONFIG_DIR="$(HITCH_CONFIG_DIR)" HITCH_UID="$(HITCH_UID)" HITCH_GID="$(HITCH_GID)" docker compose rm -sf hitch
	DOCKER_IMAGE="$(DOCKER_IMAGE)" DOCKER_PORT="$(DOCKER_PORT)" HITCH_CONFIG_DIR="$(HITCH_CONFIG_DIR)" HITCH_UID="$(HITCH_UID)" HITCH_GID="$(HITCH_GID)" docker compose up -d --build --remove-orphans
test:
	go test $(GO_PACKAGES)

test-go: test

vet:
	go vet $(GO_PACKAGES)

lint:
	@if command -v golangci-lint >/dev/null; then \
		golangci-lint run; \
	else \
		printf '%s\n' 'golangci-lint not installed; skipping local lint'; \
	fi

check: lint vet test-go build

run:
	go run ./cmd/hitch

serve:
	go run ./cmd/hitch serve --config $(CONFIG)

status:
	go run ./cmd/hitch status --json

doctor:
	go run ./cmd/hitch doctor --json

install-dry-run:
	go run ./cmd/hitch-client install --all --dry-run --json

clean:
	rm -rf bin

preview-pages:
	python3 -m http.server 8770 --directory docs
