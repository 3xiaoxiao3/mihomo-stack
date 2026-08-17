.PHONY: test test-race vet web-test web-build verify build

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

web-test:
	npm --prefix web ci
	npm --prefix web run typecheck
	npm --prefix web run test

web-build:
	npm --prefix web ci
	npm --prefix web run build

verify: test test-race vet web-test web-build

build:
	mkdir -p bin
	go build -trimpath -o bin/mihomo-guardian ./cmd/mihomo-guardian
