# flowcatalyst-sdk (Rust): build, test and lint. Run from clients/rust-sdk.
# Releases are cut from the repository root like the other SDKs; see
# docs/rust-sdk.md (no release line is wired yet, and nothing here publishes).

CARGO ?= cargo
PG_URL ?= postgres://sdk:sdk@127.0.0.1:55433/sdk_test
MYSQL_URL ?= mysql://root:sdk@127.0.0.1:55434/sdk_test

.PHONY: all build test test-db fmt fmt-check lint deny conformance ci test-db-up test-db-down help

all: ci

build: ## Build every feature combination that matters
	$(CARGO) build --no-default-features
	$(CARGO) build
	$(CARGO) build --all-features

test: ## Unit, golden-fixture and doc tests (database tests skip)
	$(CARGO) test --all-features

test-db: ## Database driver tests against the throwaway containers (test-db-up)
	FC_SDK_TEST_POSTGRES_URL=$(PG_URL) FC_SDK_TEST_MYSQL_URL=$(MYSQL_URL) \
		$(CARGO) test --all-features --test outbox_db

test-db-up: ## Start throwaway Postgres (55433) and MySQL (55434) containers
	docker run -d --rm --name fc-rust-sdk-pg -e POSTGRES_USER=sdk -e POSTGRES_PASSWORD=sdk \
		-e POSTGRES_DB=sdk_test -p 127.0.0.1:55433:5432 postgres:18-alpine
	docker run -d --rm --name fc-rust-sdk-mysql -e MYSQL_ROOT_PASSWORD=sdk \
		-e MYSQL_DATABASE=sdk_test -p 127.0.0.1:55434:3306 mysql:8.1

test-db-down: ## Stop the throwaway containers
	-docker stop fc-rust-sdk-pg fc-rust-sdk-mysql

fmt: ## Format
	$(CARGO) fmt

fmt-check: ## Check formatting
	$(CARGO) fmt --check

lint: ## Clippy, warnings are errors
	$(CARGO) clippy --all-features --all-targets -- -D warnings
	$(CARGO) clippy --no-default-features --all-targets -- -D warnings

deny: ## Licences, advisories, bans, sources
	$(CARGO) deny --all-features check

conformance: ## The Go side of the golden fixtures (platform decoders, Go SDK)
	cd ../.. && go test ./clients/rust-sdk/conformance/

ci: fmt-check lint test conformance ## Everything CI runs (deny when installed)
	@command -v cargo-deny >/dev/null && $(MAKE) deny || echo ">> cargo-deny not installed; skipped"

help:
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'
