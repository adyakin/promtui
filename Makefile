.PHONY: build build-linux release release-local run test

LINUX_ARCH ?= amd64
export TAG

build:
	go build -o bin/promtui ./cmd/promtui

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(LINUX_ARCH) go build -o bin/promtui-linux-$(LINUX_ARCH) ./cmd/promtui

release:
	@test -n "$$TAG" || { echo "Usage: make release TAG=vX.Y.Z" >&2; exit 1; }
	gh workflow run release.yml -f tag="$$TAG"

release-local:
	goreleaser release --snapshot --clean

run: build
	$(if $(wildcard config.yaml config.yml config.toml),./bin/promtui -config $(firstword $(wildcard config.yaml config.yml config.toml)),./bin/promtui)

test:
	go test ./...
