VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE     ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS  := -s -w \
  -X github.com/volodymyrsmirnov/dotfather/internal/version.Version=$(VERSION) \
  -X github.com/volodymyrsmirnov/dotfather/internal/version.Commit=$(COMMIT) \
  -X github.com/volodymyrsmirnov/dotfather/internal/version.Date=$(DATE)

.PHONY: build test fmt fmt-check lint vet vulncheck clean help

build: ## Build the binary
	go build -ldflags '$(LDFLAGS)' -o dotfather .

test: ## Run tests with race detector and coverage
	go test -race -coverprofile=coverage.out -covermode=atomic ./...

fmt: ## Format code
	gofmt -s -w .

fmt-check: ## Check code formatting
	@test -z "$$(gofmt -s -l .)" || (echo "Run 'make fmt' to fix formatting"; gofmt -s -l .; exit 1)

lint: vet ## Run linters
	golangci-lint run ./...

vet: ## Run go vet
	go vet ./...

vulncheck: ## Run vulnerability check
	govulncheck ./...

clean: ## Remove build artifacts
	rm -f dotfather coverage.out coverage.html

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'
