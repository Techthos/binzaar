.DEFAULT_GOAL := build

GO ?= go

.PHONY: run build test fmt lint tidy check

run:
	$(GO) run .

build:
	$(GO) build -o bin/binzaar .

test:
	$(GO) test ./... -race -cover

fmt:
	@command -v gofumpt >/dev/null 2>&1 || { echo "gofumpt not found: go install mvdan.cc/gofumpt@latest"; exit 1; }
	gofumpt -w .

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not found: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest"; exit 1; }
	golangci-lint run

tidy:
	$(GO) mod tidy

check: fmt tidy lint test
