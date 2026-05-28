.PHONY: build go-build build-release frontend frontend-install frontend-dev \
	run run-server dev dev-debug dev-full check setup init fresh db-reset \
	test test-unit test-integration test-platform test-verbose watch-test \
	lint lint-fix analyze fmt fmt-check sqlc sqlc-verify ci clean \
	dump-spec api-bump api-diff install-tools help

GO ?= go
PNPM ?= pnpm
BINARIES := fc-server fc-dev
FC_API_PORT ?= 8080

build: frontend go-build ## Build the frontend then every Go binary

go-build: ## Build all Go binaries (skips frontend; assumes frontend/dist exists)
	@for b in $(BINARIES); do \
		echo ">> building $$b"; \
		$(GO) build -o bin/$$b ./cmd/$$b || exit 1; \
	done

build-release: frontend ## Build optimized binaries (trimpath, stripped)
	@for b in $(BINARIES); do \
		echo ">> building (release) $$b"; \
		$(GO) build -trimpath -ldflags='-s -w' -o bin/$$b ./cmd/$$b || exit 1; \
	done

check: ## Fast compile check of every package
	$(GO) build ./...

frontend: frontend-install ## Build the Vue SPA into frontend/dist (required for `go-build` to embed it)
	@echo ">> building frontend/dist"
	@cd frontend && $(PNPM) build

frontend-install: ## Install frontend deps (idempotent; pnpm skips when up-to-date)
	@cd frontend && $(PNPM) install --frozen-lockfile

frontend-dev: ## Run the Vite dev server (proxies API to fc-dev)
	@cd frontend && VITE_BACKEND_PORT=$(FC_API_PORT) $(PNPM) dev

# ── Run / dev loop ───────────────────────────────────────────────────
# fc-dev runs every subsystem against an embedded Postgres — no Docker,
# no compose, no separate migrate step (unlike the Rust repo's justfile).

run: ## Run fc-dev once against embedded Postgres (no reload)
	$(GO) run ./cmd/fc-dev start

run-server: ## Run the unified fc-server (subsystems toggled via env)
	$(GO) run ./cmd/fc-server

dev: ## Run fc-dev with live reload (requires air; see install-tools)
	@which air >/dev/null 2>&1 || { echo "air not found — run 'make install-tools'"; exit 1; }
	air

dev-debug: ## Run fc-dev with live reload + debug logging
	@which air >/dev/null 2>&1 || { echo "air not found — run 'make install-tools'"; exit 1; }
	FC_LOG_LEVEL=debug air

dev-full: ## Run fc-dev + Vite together (Ctrl-C stops both)
	@trap 'kill 0' EXIT; \
	FC_API_PORT=$(FC_API_PORT) $(GO) run ./cmd/fc-dev start & \
	( cd frontend && VITE_BACKEND_PORT=$(FC_API_PORT) $(PNPM) dev ) & \
	wait

# ── Bootstrap / database ─────────────────────────────────────────────

setup: init ## First-time setup: bootstrap, then print next steps
	@echo ""
	@echo "Setup complete. Run 'make run' (or 'make dev' for live reload)."
	@echo "  API:     http://localhost:$(FC_API_PORT)"
	@echo "  Metrics: http://localhost:9090/metrics"

init: ## Bootstrap admin user + default tenant + .env
	$(GO) run ./cmd/fc-dev init

fresh: ## Truncate every FlowCatalyst table (preserves schema)
	$(GO) run ./cmd/fc-dev fresh

db-reset: ## Wipe the embedded Postgres data dir, then start fresh
	$(GO) run ./cmd/fc-dev start --embedded-db-reset

test: test-unit test-integration ## Run all tests

test-unit: ## Run unit tests (no DB required)
	$(GO) test -race -short ./...

test-integration: ## Run integration tests (testcontainers Postgres)
	$(GO) test -race -tags=integration ./...

test-platform: ## Run platform package tests (no DB)
	$(GO) test -race -short ./internal/platform/...

test-verbose: ## Run unit tests with verbose output
	$(GO) test -race -short -v ./...

watch-test: ## Re-run unit tests on file changes (requires gotestsum)
	@which gotestsum >/dev/null 2>&1 || { echo "gotestsum not found — run 'make install-tools'"; exit 1; }
	gotestsum --watch -- -short ./...

lint: ## Run golangci-lint
	golangci-lint run ./...

lint-fix: ## Run golangci-lint with autofix
	golangci-lint run --fix ./...

analyze: ## Run custom UoW seal analyzer
	$(GO) run ./tools/analyzer/uowseal ./internal/platform/...

fmt: ## Format the codebase
	$(GO) fmt ./...
	$(GO) tool goimports -w .

fmt-check: ## Check formatting without writing (CI-style)
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "These files need 'make fmt':"; echo "$$unformatted"; exit 1; \
	fi

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

install-tools: ## Install dev tools (air, gotestsum, golangci-lint, sqlc)
	$(GO) install github.com/air-verse/air@latest
	$(GO) install gotest.tools/gotestsum@latest
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GO) install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
	@echo ">> tools installed to $$($(GO) env GOPATH)/bin — ensure it's on your PATH"

clean:
	rm -rf bin/ tmp/ coverage.*

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
