.PHONY: build run test

build:
	go build -o bin/promtui ./cmd/promtui

run: build
	./bin/promtui -config config.toml

test:
	go test ./...
