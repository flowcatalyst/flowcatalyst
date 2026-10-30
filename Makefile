.PHONY: build go-build build-release frontend frontend-install frontend-dev \
	run run-server dev dev-debug dev-full check setup init fresh db-reset \
	test test-unit test-integration test-platform test-verbose watch-test \
	lint lint-fix analyze fmt fmt-check sqlc sqlc-verify ui-generate ui-fmt ui-verify ui-kit-sync ci clean \
	dump-spec api-bump api-diff release-dev sdk-spec sdk-generate \
	release-ts-sdk release-laravel-sdk build-java-sdk release-java-sdk \
	release-fn-ts-sdk release-fn-rust-sdk release-fn-go-sdk release-ingestor release-all-sdks install-tools help \
	test-go-sdk test-go-sdk-tests lint-go-sdk go-sdk-smoke release-go-sdk e2e-platform \
	tools tidy-verify vet ci-fast pre-push install-hooks

GO ?= go
PNPM ?= pnpm
BINARIES := fc-server fcdev
BIN_DIR ?= bin

# ── Pinned tool versions ─────────────────────────────────────────────
# Everything CI runs is reproducible locally with `make ci`, with the SAME
# tool versions. Bump a pin here AND in .github/workflows (golangci-lint-action
# `version:`), fixing any new findings in the same commit.
GOLANGCI_LINT_VERSION ?= v2.15.0
SQLC_VERSION          ?= v1.31.1
AIR_VERSION           ?= v1.67.4
GOTESTSUM_VERSION     ?= v1.13.0

# Pinned tools are built with the LOCAL Go toolchain (prebuilt golangci-lint
# releases lag the Go release and cannot read newer stdlib export data) into a
# directory keyed by tool version AND Go version, so a Go upgrade rebuilds them.
GOVERSION_LOCAL := $(shell $(GO) env GOVERSION 2>/dev/null)
TOOLS_DIR       := $(CURDIR)/.tools
GOLANGCI_LINT   := $(TOOLS_DIR)/golangci-lint-$(GOLANGCI_LINT_VERSION)-$(GOVERSION_LOCAL)/golangci-lint
SQLC            := $(TOOLS_DIR)/sqlc-$(SQLC_VERSION)-$(GOVERSION_LOCAL)/sqlc

FC_API_PORT ?= 8080

build: frontend go-build ## Build the frontend then every Go binary

go-build: ## Build all Go binaries (skips frontend; assumes frontend/dist exists)
	@for b in $(BINARIES); do \
		echo ">> building $$b"; \
		$(GO) build -o $(BIN_DIR)/$$b ./cmd/$$b || exit 1; \
	done

build-release: frontend ## Build optimized binaries (trimpath, stripped)
	@for b in $(BINARIES); do \
		echo ">> building (release) $$b"; \
		$(GO) build -trimpath -ldflags='-s -w' -o $(BIN_DIR)/$$b ./cmd/$$b || exit 1; \
	done

check: ## Fast compile check of every package
	$(GO) build ./...

frontend: frontend-install ## Build the Vue SPA into frontend/dist (required for `go-build` to embed it)
	@echo ">> building frontend/dist"
	@cd frontend && $(PNPM) build

frontend-install: ## Install frontend deps (idempotent; pnpm skips when up-to-date)
	@cd frontend && $(PNPM) install --frozen-lockfile

frontend-dev: ## Run the Vite dev server (proxies API to fcdev)
	@cd frontend && VITE_BACKEND_PORT=$(FC_API_PORT) $(PNPM) dev

# ── Run / dev loop ───────────────────────────────────────────────────
# fcdev runs every subsystem against an embedded Postgres — no Docker,
# no compose, no separate migrate step (unlike the Rust repo's justfile).

run: ## Run fcdev once against embedded Postgres (no reload)
	$(GO) run ./cmd/fcdev start

run-server: ## Run the unified fc-server (subsystems toggled via env)
	$(GO) run ./cmd/fc-server

dev: ## Run fcdev with live reload (requires air; see install-tools)
	@which air >/dev/null 2>&1 || { echo "air not found — run 'make install-tools'"; exit 1; }
	air

dev-debug: ## Run fcdev with live reload + debug logging
	@which air >/dev/null 2>&1 || { echo "air not found — run 'make install-tools'"; exit 1; }
	FC_LOG_LEVEL=debug air

dev-full: ## Run fcdev + Vite together (Ctrl-C stops both)
	@trap 'kill 0' EXIT; \
	FC_API_PORT=$(FC_API_PORT) $(GO) run ./cmd/fcdev start & \
	( cd frontend && VITE_BACKEND_PORT=$(FC_API_PORT) $(PNPM) dev ) & \
	wait

# ── Bootstrap / database ─────────────────────────────────────────────

setup: init ## First-time setup: bootstrap, then print next steps
	@echo ""
	@echo "Setup complete. Run 'make run' (or 'make dev' for live reload)."
	@echo "  API:     http://localhost:$(FC_API_PORT)"
	@echo "  Metrics: http://localhost:9090/metrics"

init: ## Bootstrap admin user + default tenant + .env
	$(GO) run ./cmd/fcdev init

fresh: ## Truncate every FlowCatalyst table (preserves schema)
	$(GO) run ./cmd/fcdev fresh

db-reset: ## Wipe the embedded Postgres data dir, then start fresh
	$(GO) run ./cmd/fcdev start --embedded-db-reset

test: test-unit test-integration ## Run all tests

test-unit: ## Run unit tests (no DB required), the platform's and the Go SDK module's
	$(GO) test -race -short ./...
	$(GO) -C pkg/fcsdk test -race -short ./...

# The Go SDK (pkg/fcsdk) is its own module: `./...` at the root does not reach
# it, so it has its own test/lint targets (also folded into test-unit / lint).
# Its Postgres-backed tests live in a second nested module, pkg/fcsdk/tests
# (embedded Postgres), so the SDK's own go.mod carries no test-only database
# dependencies; test-go-sdk-tests runs it (part of test-integration).
test-go-sdk: ## Build, vet and test the Go SDK module on its own (pkg/fcsdk)
	$(GO) -C pkg/fcsdk build ./...
	$(GO) -C pkg/fcsdk vet ./...
	$(GO) -C pkg/fcsdk test -race ./...

test-go-sdk-tests: ## Run the Go SDK's Postgres-backed tests (pkg/fcsdk/tests module, own embedded Postgres)
	$(GO) -C pkg/fcsdk/tests vet ./...
	$(GO) -C pkg/fcsdk/tests test -race -p 1 ./...

go-sdk-smoke: ## Consume the Go SDK from a scratch module (local replace + `go get` of HEAD as published)
	@scripts/go-sdk-smoke.sh all

e2e-platform: ## Isolated platform for app end-to-end checks; CMD=up|down|status|env|verify|drop (docs/e2e-platform.md)
	@scripts/e2e-platform.sh $(or $(CMD),up)

test-integration: ## Run integration tests (embedded Postgres via internal/testpg)
	# -p 1 serializes test binaries: each integration package boots its own
	# embedded Postgres (internal/testpg.RunMain), and the first-ever run
	# also downloads the postgres binaries — concurrent extraction races.
	$(GO) test -race -p 1 -tags=integration ./...
	$(MAKE) test-go-sdk-tests

test-platform: ## Run platform package tests (no DB)
	$(GO) test -race -short ./internal/platform/...

test-verbose: ## Run unit tests with verbose output
	$(GO) test -race -short -v ./...

watch-test: ## Re-run unit tests on file changes (requires gotestsum)
	@which gotestsum >/dev/null 2>&1 || { echo "gotestsum not found — run 'make install-tools'"; exit 1; }
	gotestsum --watch -- -short ./...

# frontend/embed.go has `//go:embed all:dist`; with no SPA build the package
# fails to typecheck and golangci aborts. Same stub CI's lint job drops in.
define ensure_dist_stub
	@if [ -z "$$(ls -A frontend/dist 2>/dev/null)" ]; then \
		mkdir -p frontend/dist && touch frontend/dist/.gitkeep; \
		echo ">> frontend/dist was empty: wrote a .gitkeep stub so the embed typechecks"; \
	fi
endef

lint: $(GOLANGCI_LINT) lint-go-sdk ## Run the PINNED golangci-lint incl. the integration build tag (platform + Go SDK modules)
	$(ensure_dist_stub)
	$(GOLANGCI_LINT) run --build-tags integration ./...

lint-go-sdk: $(GOLANGCI_LINT) ## Run the pinned golangci-lint on the Go SDK module (pkg/fcsdk) and its tests module
	cd pkg/fcsdk && $(GOLANGCI_LINT) run ./...
	cd pkg/fcsdk/tests && $(GOLANGCI_LINT) run ./...

lint-fix: $(GOLANGCI_LINT) ## Run the pinned golangci-lint with autofix
	$(GOLANGCI_LINT) run --fix --build-tags integration ./...

analyze: ## Run custom analyzers (UoW seal, id conversions)
	$(GO) run ./tools/analyzer/uowseal ./internal/platform/...
	$(GO) run ./tools/analyzer/idconv ./...

fmt: ## Format the codebase
	$(GO) fmt ./...
	$(GO) -C pkg/fcsdk fmt ./...
	$(GO) -C pkg/fcsdk/tests fmt ./...
	$(GO) tool goimports -w .

fmt-check: ## Check formatting without writing (CI-style)
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "These files need 'make fmt':"; echo "$$unformatted"; exit 1; \
	fi

sqlc: $(SQLC) ## Regenerate sqlc dbq (pinned sqlc) from internal/sqlc/queries + internal/migrate/sql
	$(SQLC) generate

sqlc-verify: $(SQLC) ## Verify sqlc dbq matches the queries (no diff), with the pinned sqlc. For CI.
	$(SQLC) generate
	@git diff --exit-code internal/sqlc/dbq/ || \
		(echo "sqlc out of date; run 'make sqlc' and commit the diff" && exit 1)

# ── Server-rendered UI (internal/ui, templ + Datastar) ───────────────
# templ is pinned by the `tool` directive in go.mod, so `go tool templ` needs
# no global install. Generated *_templ.go files are committed (like sqlc's
# dbq); ui-verify fails CI when they are stale. See docs/ui/README.md.
TEMPL := $(GO) tool templ
UI_DIR := internal/ui

ui-generate: ## Generate Go code from the templ components under internal/ui
	$(TEMPL) generate -path $(UI_DIR)

ui-fmt: ## Format the templ components under internal/ui
	$(TEMPL) fmt $(UI_DIR)

ui-verify: ## Fail if generated *_templ.go files or templ formatting are stale (for CI)
	@$(TEMPL) generate -path $(UI_DIR) >/dev/null
	@untracked=$$(git ls-files --others --exclude-standard -- '$(UI_DIR)/*_templ.go'); \
	if [ -n "$$untracked" ]; then \
		echo "generated templ files not committed:"; echo "$$untracked"; exit 1; \
	fi
	@git diff --exit-code -- '$(UI_DIR)/*_templ.go' || \
		(echo "templ output out of date; run 'make ui-generate' and commit the diff" && exit 1)
	@for f in $$(find $(UI_DIR) -name '*.templ'); do \
		$(TEMPL) fmt -stdout $$f 2>/dev/null | diff -q - $$f >/dev/null || \
			{ echo "$$f is not templ-formatted; run 'make ui-fmt'"; exit 1; }; \
	done

ui-kit-sync: ## Rebuild internal/ui/assets from ../flowcatalyst-framework/ui-kit (KIT_DIR=... to override)
	@scripts/ui-kit-sync.sh

dump-spec: ## Emit the current huma-generated OpenAPI spec to stdout
	@$(GO) run ./tools/dump-spec

api-bump: ## Regenerate api/openapi.lock.json from the current code
	@$(GO) run ./tools/dump-spec > api/openapi.lock.json
	@echo ">> wrote api/openapi.lock.json"

api-diff: ## Fail if the committed lockfile differs from the live spec
	@mkdir -p tmp
	@$(GO) run ./tools/dump-spec > tmp/openapi.live.json
	@diff -u api/openapi.lock.json tmp/openapi.live.json || \
		(echo "openapi.lock.json out of date; run 'make api-bump' and commit the diff" && exit 1)

frontend-types-verify: ## Verify the SPA's generated API types match the lockfile (mirrors sqlc-verify)
	@cd frontend && $(PNPM) api:generate
	@git diff --exit-code frontend/src/api/generated/ || \
		(echo "generated API types out of date; run 'pnpm api:generate' in frontend/ and commit the diff" && exit 1)

# ── CI mirror ────────────────────────────────────────────────────────
# `make ci` runs, in order, every step of .github/workflows/ci.yml (test job
# then lint job) and go-sdk.yml (test, pg-tests, lint, smoke), with the same
# pinned tools, then the extra conformance checks CI also relies on
# (sqlc-verify, api-diff, frontend-types-verify). Integration steps boot
# embedded Postgres (no Docker needed; first run downloads the binaries).
# Not reproducible locally: the go-sdk.yml matrix leg on Go 1.26 (only the
# local toolchain is available) and the unused `parity-spec` placeholder job.
# Fast subset without Postgres: `make pre-push`.

tidy-verify: ## Fail if go.mod/go.sum of any module needs `go mod tidy` (does not modify files)
	$(GO) mod tidy -diff
	$(GO) -C pkg/fcsdk mod tidy -diff
	$(GO) -C pkg/fcsdk/tests mod tidy -diff

vet: ## go vet every module (root, pkg/fcsdk, pkg/fcsdk/tests)
	$(ensure_dist_stub)
	$(GO) vet ./...
	$(GO) -C pkg/fcsdk vet ./...
	$(GO) -C pkg/fcsdk/tests vet ./...

ci: ## Run everything CI runs, same order, same pinned tools (docs/contributing.md)
	@echo "== ci 1/12: go mod tidy (all modules)";            $(MAKE) --no-print-directory tidy-verify
	@echo "== ci 2/12: templ output (ui-verify)";             $(MAKE) --no-print-directory ui-verify
	@echo "== ci 3/12: build";                                $(MAKE) --no-print-directory build
	@echo "== ci 4/12: unit tests (root + Go SDK)";           $(MAKE) --no-print-directory test-unit
	@echo "== ci 5/12: integration tests (+ SDK pg tests)";   $(MAKE) --no-print-directory test-integration
	@echo "== ci 6/12: UoW seal analyzer";                    $(MAKE) --no-print-directory analyze
	@echo "== ci 7/12: lint (pinned $(GOLANGCI_LINT_VERSION))"; $(MAKE) --no-print-directory lint
	@echo "== ci 8/12: Go SDK build/vet/test -race";          $(MAKE) --no-print-directory test-go-sdk
	@echo "== ci 9/12: Go SDK consumed from a scratch module"; $(MAKE) --no-print-directory go-sdk-smoke
	@echo "== ci 10/12: sqlc output (pinned $(SQLC_VERSION))"; $(MAKE) --no-print-directory sqlc-verify
	@echo "== ci 11/12: OpenAPI lock vs live spec";           $(MAKE) --no-print-directory api-diff
	@echo "== ci 12/12: SPA generated API types";             $(MAKE) --no-print-directory frontend-types-verify
	@echo "== ci: all steps passed"

# ── Fast pre-push gate ───────────────────────────────────────────────
# No Docker/Postgres. See scripts/pre-push.sh for the step list and
# docs/contributing.md. Install the git hook once with `make install-hooks`.
pre-push: $(GOLANGCI_LINT) ## Fast pre-push gate (build, vet, pinned lint, changed-package tests, conformance); no Docker
	@scripts/pre-push.sh

ci-fast: pre-push ## Alias for pre-push

install-hooks: ## Point git at the committed hooks in scripts/hooks (pre-push runs `make pre-push`)
	git config core.hooksPath scripts/hooks
	@echo ">> hooks installed (core.hooksPath=scripts/hooks). Bypass once: git push --no-verify  or  SKIP_PREPUSH=1 git push"

# ── Release ──────────────────────────────────────────────────────────
# Version source of truth is cmd/fcdev/VERSION (seeded from the Rust
# monorepo's last fcdev release so numbering continues). The release
# workflow (.github/workflows/release-fcdev.yml) fires on the pushed tag.

release-dev: ## Cut an fcdev release: BUMP=patch|minor|major|X.Y.Z (tags fcdev/vX.Y.Z, pushes)
	@scripts/release.sh dev "$(BUMP)"

# ── SDKs ─────────────────────────────────────────────────────────────
# The TS + Laravel client SDKs (clients/) are generated from the huma
# OpenAPI spec — `make dump-spec` emits it with no DB. Releases tag
# <sdk>/vX.Y.Z; the split-*-sdk workflows mirror each to its standalone
# repo. VERSION files are seeded from the Rust monorepo (0.6.15) so the
# numbering continues — first release is 0.6.16.

sdk-spec: ## Refresh each SDK's OpenAPI inputs: openapi.json (generator input) plus the app/admin split
	@for sdk in typescript laravel java; do \
		$(GO) run ./tools/dump-spec -generator > clients/$$sdk-sdk/openapi/openapi.json && \
		$(GO) run ./tools/dump-spec -split app > clients/$$sdk-sdk/openapi/openapi.app.json && \
		$(GO) run ./tools/dump-spec -split admin > clients/$$sdk-sdk/openapi/openapi.admin.json || exit 1; \
	done
	@echo ">> refreshed clients/{typescript,laravel,java}-sdk/openapi/{openapi,openapi.app,openapi.admin}.json"

sdk-generate: sdk-spec ## Regenerate the TS + Laravel SDK clients from the spec
	@echo ">> TypeScript SDK"
	cd clients/typescript-sdk && $(PNPM) install --frozen-lockfile && $(PNPM) run generate && $(PNPM) run build
	@echo ">> Laravel SDK (XDEBUG_MODE=off — Homebrew Xdebug blocks CLI PHP otherwise)"
	cd clients/laravel-sdk && XDEBUG_MODE=off composer install --no-interaction \
		&& XDEBUG_MODE=off php scripts/prepare-openapi.php \
		&& XDEBUG_MODE=off vendor/bin/jane-openapi generate --config-file=jane-openapi.php
	@echo ">> SDKs regenerated — review the diff and commit before releasing"

build-java-sdk: ## Build + test the Java SDK (regenerates models from the vendored spec)
	cd clients/java-sdk && mvn -q clean verify

release-ts-sdk: ## Cut a TypeScript SDK release: BUMP=… (bumps package.json, tags typescript-sdk/vX.Y.Z)
	@scripts/release.sh ts "$(BUMP)"

release-laravel-sdk: ## Cut a Laravel SDK release: BUMP=… (tags laravel-sdk/vX.Y.Z)
	@scripts/release.sh laravel "$(BUMP)"

release-java-sdk: ## Cut a Java SDK release: BUMP=… (bumps pom.xml, tags java-sdk/vX.Y.Z)
	@scripts/release.sh java "$(BUMP)"

# The Go SDK (pkg/fcsdk) is published to its own repo, one commit per release
# (scripts/publish-go-sdk.sh): `go get github.com/flowcatalyst/go-sdk@vX.Y.Z`.
release-go-sdk: ## Cut a Go SDK release: BUMP=… (tags go-sdk/vX.Y.Z here, publishes vX.Y.Z to flowcatalyst/go-sdk)
	@scripts/release.sh go-sdk "$(BUMP)"

# Function-runner SDKs (docs/releasing.md). fn-go is consumed straight from
# this repo via the Go module proxy; fn-ts / fn-rust are split to standalone
# repos by workflows that are OFF until the SPLIT_FN_SDKS repo variable is set.
release-fn-ts-sdk: ## Cut a function TS SDK release: BUMP=… (bumps package.json, tags fn-ts-sdk/vX.Y.Z)
	@scripts/release.sh fn-ts "$(BUMP)"

release-fn-rust-sdk: ## Cut a function Rust SDK release: BUMP=… (bumps Cargo.toml, tags fn-rust-sdk/vX.Y.Z)
	@scripts/release.sh fn-rust "$(BUMP)"

release-all-sdks: ## Release every SDK line changed since its last tag: BUMP=patch|minor|major (ONLY=, SKIP=, WITH_DEV=1, RELEASE_DRY_RUN=1, RELEASE_ALL_PLAN=1)
	@scripts/release-all-sdks.sh "$(or $(BUMP),patch)"

release-fn-go-sdk: ## Cut a function Go SDK release: BUMP=… (tags clients/fn-go/vX.Y.Z)
	@scripts/release.sh fn-go "$(BUMP)"

release-ingestor: ## Cut an ingestor runtime release: BUMP=… (bumps ingestor/pom.xml <revision>, tags ingestor/vX.Y.Z)
	@scripts/release.sh ingestor "$(BUMP)"

# Pinned tools live in ./.tools (gitignored), built with the local Go toolchain.
tools: $(GOLANGCI_LINT) $(SQLC) ## Build the pinned lint/codegen tools into ./.tools with the local Go

$(GOLANGCI_LINT):
	@echo ">> building golangci-lint $(GOLANGCI_LINT_VERSION) with $(GOVERSION_LOCAL)"
	GOBIN=$(dir $@) GOTOOLCHAIN=local $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@$@ --version | grep -q "$(patsubst v%,%,$(GOLANGCI_LINT_VERSION))" || { echo "golangci-lint version mismatch (want $(GOLANGCI_LINT_VERSION))"; rm -f $@; exit 1; }

$(SQLC):
	@echo ">> building sqlc $(SQLC_VERSION) with $(GOVERSION_LOCAL)"
	GOBIN=$(dir $@) GOTOOLCHAIN=local $(GO) install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)

install-tools: tools ## Install dev tools: pinned lint/sqlc into ./.tools, plus air + gotestsum (pinned) into GOBIN
	$(GO) install github.com/air-verse/air@$(AIR_VERSION)
	$(GO) install gotest.tools/gotestsum@$(GOTESTSUM_VERSION)
	@echo ">> air/gotestsum installed to $$($(GO) env GOPATH)/bin; lint/sqlc are used from ./.tools by make"

clean:
	rm -rf bin/ tmp/ coverage.*   # keeps .tools (slow to rebuild); rm -rf .tools to reset

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
