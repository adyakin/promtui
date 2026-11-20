.PHONY: build build-linux run test

LINUX_ARCH ?= amd64

build:
	go build -o bin/promtui ./cmd/promtui

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(LINUX_ARCH) go build -o bin/promtui-linux-$(LINUX_ARCH) ./cmd/promtui

run: build
	$(if $(wildcard config.yaml config.yml config.toml),./bin/promtui -config $(firstword $(wildcard config.yaml config.yml config.toml)),./bin/promtui)

test:
	go test ./...
