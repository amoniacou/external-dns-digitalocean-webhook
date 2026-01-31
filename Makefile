.PHONY: build test lint lint-fix clean docker-build run release snapshot help

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.commit=$(COMMIT) \
	-X main.date=$(DATE)

## build: Build the binary
build:
	go build -ldflags="$(LDFLAGS)" -o bin/webhook ./cmd/webhook

## test: Run tests
test:
	go test -v -race -cover ./...

## lint: Run linter
lint:
	golangci-lint run

## lint-fix: Run linter and fix issues
lint-fix:
	golangci-lint run --fix

## clean: Remove build artifacts
clean:
	rm -rf bin/
	rm -rf dist/

## docker-build: Build docker image using GoReleaser artifacts
docker-build:
	goreleaser build --snapshot --clean
	docker build \
		--build-arg VERSION=$(VERSION) \
		-t external-dns-digitalocean-webhook:$(VERSION) .

## run: Run the webhook locally
run: build
	DO_TOKEN=$${DO_TOKEN} ./bin/webhook --log-level=debug

## release: Create a new release (requires GITHUB_TOKEN)
release:
	goreleaser release --clean

## snapshot: Create a snapshot release
snapshot:
	goreleaser release --snapshot --clean --skip=publish

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := build
