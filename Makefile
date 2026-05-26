.PHONY: build test test-unit test-integration lint analyze fmt sqlc sqlc-verify ci clean

GO ?= go
BINARIES := fc-platform-server fc-router fc-stream-processor fc-outbox-processor fc-mcp-server fc-server fc-dev

build: ## Build all binaries
	@for b in $(BINARIES); do \
		echo ">> building $$b"; \
		$(GO) build -o bin/$$b ./cmd/$$b || exit 1; \
	done

test: test-unit test-integration ## Run all tests

test-unit: ## Run unit tests (no DB required)
	$(GO) test -race -short ./...

test-integration: ## Run integration tests (testcontainers Postgres)
	$(GO) test -race -tags=integration ./...

lint: ## Run golangci-lint
	golangci-lint run ./...

analyze: ## Run custom UoW seal analyzer
	$(GO) run ./tools/analyzer/uowseal ./internal/platform/...

fmt: ## Format the codebase
	$(GO) fmt ./...
	$(GO) tool goimports -w .

sqlc: ## Regenerate sqlc dbq from internal/sqlc/queries + internal/migrate/sql
	@which sqlc >/dev/null 2>&1 || $(GO) install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	sqlc generate

sqlc-verify: ## Verify sqlc dbq matches the queries (no diff). For CI.
	@which sqlc >/dev/null 2>&1 || $(GO) install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	sqlc generate
	@git diff --exit-code internal/sqlc/dbq/ || \
		(echo "sqlc out of date; run 'make sqlc' and commit the diff" && exit 1)

ci: lint sqlc-verify test analyze ## Run everything CI runs

clean:
	rm -rf bin/ tmp/ coverage.*

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
