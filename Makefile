.PHONY: build go-build frontend frontend-install test test-unit test-integration lint analyze fmt sqlc sqlc-verify ci clean dump-spec api-bump api-diff

GO ?= go
PNPM ?= pnpm
BINARIES := fc-platform-server fc-router fc-stream-processor fc-outbox-processor fc-mcp-server fc-server fc-dev

build: frontend go-build ## Build the frontend then every Go binary

go-build: ## Build all Go binaries (skips frontend; assumes frontend/dist exists)
	@for b in $(BINARIES); do \
		echo ">> building $$b"; \
		$(GO) build -o bin/$$b ./cmd/$$b || exit 1; \
	done

frontend: frontend-install ## Build the Vue SPA into frontend/dist (required for `go-build` to embed it)
	@echo ">> building frontend/dist"
	@cd frontend && $(PNPM) build

frontend-install: ## Install frontend deps (idempotent; pnpm skips when up-to-date)
	@cd frontend && $(PNPM) install --frozen-lockfile

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

dump-spec: ## Emit the current huma-generated OpenAPI spec to stdout
	@$(GO) run ./tools/dump-spec

api-bump: ## Regenerate api/openapi.lock.json from the current code
	@$(GO) run ./tools/dump-spec > api/openapi.lock.json
	@echo ">> wrote api/openapi.lock.json"

api-diff: ## Fail if the committed lockfile differs from the live spec
	@$(GO) run ./tools/dump-spec > tmp/openapi.live.json
	@diff -u api/openapi.lock.json tmp/openapi.live.json || \
		(echo "openapi.lock.json out of date; run 'make api-bump' and commit the diff" && exit 1)

ci: lint sqlc-verify test analyze api-diff ## Run everything CI runs

clean:
	rm -rf bin/ tmp/ coverage.*

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
