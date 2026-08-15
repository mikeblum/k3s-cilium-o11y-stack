## otel-clickhouse-stack 📊 + 🏠 = 🤖

MAKEFLAGS += --silent

BINARY := otel-clickhouse-stack
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
TOOL := go tool -modfile=tools/go.mod

all: help

## help: Prints a list of available build targets.
help:
	@echo "\033[1;34mUsage:\033[0m make \033[1;36m<OPTIONS>\033[0m ... \033[1;36m<TARGETS>\033[0m"
	@echo ""
	@echo "\033[1;35mAvailable targets are:\033[0m"
	@echo ''
	@grep -E '^## .+:' ${PWD}/Makefile | sed -e 's/^## //' | awk 'BEGIN{FS=": "} {printf "  \033[32m%-14s %s\033[0m\n", $$1, substr($$0, index($$0, ": ")+2)}'
	@echo
	@echo "\033[1;35mTargets run by default are:\033[0m \033[1;32m`sed -n 's/^all: //p' ./Makefile | sed -e 's/ /, /g' | sed -e 's/\(.*\), /\1, and /'`\033[0m"

## lint: Lint with golangci-lint
lint:
	$(TOOL) golangci-lint run --verbose --color always ./...

## docs: 📖 Serve Go package docs locally at `http://localhost:6060`
docs:
	go run golang.org/x/pkgsite/cmd/pkgsite@latest -http=:6060 .

## fmt: Format with gofmt
fmt:
	go fmt ./...

## fmt-check: Fail if any file needs gofmt (non-mutating CI gate)
fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; echo "$$unformatted"; exit 1; \
	fi

## build: Build the otel-clickhouse-stack binary
build:
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o $(BINARY) .

## install: Build otel-clickhouse-stack into $GOBIN (or $GOPATH/bin) so it's on PATH
install:
	@bindir=$$(go env GOBIN); [ -n "$$bindir" ] || bindir=$$(go env GOPATH)/bin; \
	go build -ldflags "-s -w -X main.version=$(VERSION)" -o "$$bindir/$(BINARY)" . && \
	echo "installed $(BINARY) $(VERSION) to $$bindir/$(BINARY)"

## tidy: Tidy with go mod tidy
tidy:
	go mod tidy

## pre-commit: Chain fmt-check + lint + test
pre-commit: fmt-check lint test

## port-forward: Forward ClickHouse's native TCP port (9000) from the cluster
port-forward:
	$(MAKE) -C ../k8s/o11y pf-clickhouse-native

## run: otel-clickhouse-stack
run:
	go run .

## test: Test with go test
test:
	go test -test.v -race -covermode=atomic -coverprofile=coverage.out ./...

## test-html: Run tests and open HTML coverage report (deletes coverage.out)
test-html:
	go test -test.v -race -covermode=atomic -coverprofile=coverage.out ./... && go tool cover -html=coverage.out && rm coverage.out

.PHONY: help lint docs fmt fmt-check build install tidy pre-commit port-forward run test test-html
