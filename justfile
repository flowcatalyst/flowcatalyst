# FlowCatalyst Rust - Development Tasks
#
# Quick start:
#   just setup     — first time: up + migrate + dev
#   just dev       — start fc-dev with hot reload (backend only)
#   just dev-full  — start both backend + frontend with hot reload
#
# Prerequisites:
#   cargo install cargo-watch
#   docker / docker compose
#   pnpm (for frontend)

set dotenv-filename := ".env.development"

# Defaults
FC_DATABASE_URL := env("FC_DATABASE_URL", "postgresql://flowcatalyst:flowcatalyst@localhost:5432/flowcatalyst")
FC_API_PORT := env("FC_API_PORT", "8080")
RUST_LOG := env("RUST_LOG", "info,fc_platform=debug,fc_dev=debug")

# List available recipes
default:
    @just --list

# ─── Full Setup ─────────────────────────────────────────────────────────────

# First-time setup: start infra, migrate, seed, then show instructions
setup: up wait-for-db migrate seed
    @echo ""
    @echo "Setup complete! Run 'just dev' to start the server."
    @echo ""
    @echo "  API:     http://localhost:{{ FC_API_PORT }}"
    @echo "  Health:  http://localhost:{{ FC_API_PORT }}/health"
    @echo "  Metrics: http://localhost:9090/metrics"
    @echo ""
    @echo "  Dev credentials:"
    @echo "    admin@flowcatalyst.local / DevPassword123!"
    @echo "    alice@acme.com / DevPassword123!"
    @echo "    bob@acme.com / DevPassword123!"

# ─── Infrastructure ────────────────────────────────────────────────────────

# Start PostgreSQL, LocalStack, Redis
up:
    docker compose up -d
    @echo ""
    @echo "Services:"
    @echo "  PostgreSQL: localhost:5432"
    @echo "  LocalStack: localhost:4566"
    @echo "  Redis:      localhost:6379"

# Stop all Docker services
down:
    docker compose down

# Tail Docker service logs
logs:
    docker compose logs -f

# Show Docker service status
ps:
    docker compose ps

# Wait for PostgreSQL to accept connections
[private]
wait-for-db:
    @echo "Waiting for PostgreSQL..."
    @until docker exec fc-postgres pg_isready -U flowcatalyst -q 2>/dev/null; do sleep 1; done
    @echo "PostgreSQL is ready."

# ─── Database ──────────────────────────────────────────────────────────────

# Run all SQL migrations
migrate: wait-for-db
    @echo "Running migrations..."
    @for f in migrations/*.sql; do \
        echo "  $f"; \
        docker exec -i fc-postgres psql -U flowcatalyst -d flowcatalyst -q < "$f" 2>&1 | grep -v "^$" || true; \
    done
    @echo "Migrations complete."

# Drop and recreate database + re-migrate
db-reset: wait-for-db
    docker exec fc-postgres psql -U flowcatalyst -d postgres -c "DROP DATABASE IF EXISTS flowcatalyst;"
    docker exec fc-postgres psql -U flowcatalyst -d postgres -c "CREATE DATABASE flowcatalyst;"
    just migrate

# Open a psql shell
db-shell:
    docker exec -it fc-postgres psql -U flowcatalyst -d flowcatalyst

# Seed development data
seed:
    @echo "Seeding development data..."
    FC_DATABASE_URL={{ FC_DATABASE_URL }} cargo run --bin fc-dev -- --seed 2>/dev/null || \
        echo "  (seed flag not yet implemented — will seed on first startup)"

# ─── Development ───────────────────────────────────────────────────────────

# Run fc-dev with auto-restart on source changes
dev:
    FC_API_PORT={{ FC_API_PORT }} FC_DATABASE_URL={{ FC_DATABASE_URL }} cargo watch -w crates -w bin -x 'run --bin fc-dev'

# Run fc-dev with debug logging
dev-debug:
    RUST_LOG=debug FC_DATABASE_URL={{ FC_DATABASE_URL }} cargo watch -w crates -w bin -x 'run --bin fc-dev'

# Run fc-dev once (no watch)
run:
    FC_DATABASE_URL={{ FC_DATABASE_URL }} cargo run --bin fc-dev

# ─── SDKs ─────────────────────────────────────────────────────────────────

# Regenerate every SDK from the live platform's OpenAPI spec.
# Requires fc-dev (or fc-platform-server) to be serving on FC_API_PORT.
regen-sdks:
    @curl -fsS http://localhost:{{ FC_API_PORT }}/q/openapi >/dev/null \
        || (echo "✗ Platform not reachable at http://localhost:{{ FC_API_PORT }}/q/openapi — run 'just run' (or 'just dev') first."; exit 1)
    @echo "▸ Refreshing OpenAPI snapshots from /q/openapi"
    @curl -fsS http://localhost:{{ FC_API_PORT }}/q/openapi -o clients/typescript-sdk/openapi/openapi.json
    @curl -fsS http://localhost:{{ FC_API_PORT }}/q/openapi -o clients/laravel-sdk/openapi/openapi.json
    @curl -fsS http://localhost:{{ FC_API_PORT }}/q/openapi -o frontend/openapi/openapi.json
    @echo "▸ TypeScript SDK"
    cd clients/typescript-sdk && pnpm build
    @echo "▸ Laravel SDK"
    # XDEBUG_MODE=off — Homebrew's Xdebug defaults to step-debug mode and
    # silently blocks every CLI invocation waiting for a debugger on :9003.
    cd clients/laravel-sdk && XDEBUG_MODE=off php scripts/prepare-openapi.php && XDEBUG_MODE=off vendor/bin/jane-openapi generate --config-file=jane-openapi.php
    @echo "▸ Frontend generated client"
    cd frontend && pnpm api:generate
    @echo "✓ SDKs regenerated"

# ─── Frontend ─────────────────────────────────────────────────────────────

# Install frontend dependencies
frontend-install:
    cd frontend && pnpm install

# Run Vite dev server (hot-reload, proxies API to Rust backend)
frontend-dev:
    cd frontend && VITE_BACKEND_PORT={{ FC_API_PORT }} pnpm dev

# Build frontend for production
frontend-build:
    cd frontend && pnpm build

# Run both Rust backend + Vite frontend concurrently (full dev experience)
dev-full:
    cd frontend && FC_API_PORT={{ FC_API_PORT }} FC_DATABASE_URL={{ FC_DATABASE_URL }} pnpm dev:full

# Run fc-dev serving the built frontend (production-like)
dev-static: frontend-build
    FC_STATIC_DIR=frontend/dist FC_DATABASE_URL={{ FC_DATABASE_URL }} cargo run --bin fc-dev

# Watch and run tests on file changes
watch-test:
    cargo watch -x 'test --lib'

# Watch platform crate tests
watch-test-platform:
    cargo watch -w crates/fc-platform -x 'test --package fc-platform --lib'

# ─── Build ─────────────────────────────────────────────────────────────────

# Build all binaries (debug)
build:
    cargo build --all-targets

# Build all binaries with --release profile
build-release:
    cargo build --release --all-targets

# Fast compile check
check:
    cargo check --all-targets

# Build individual binaries
build-dev:
    cargo build --bin fc-dev

build-router:
    cargo build --bin fc-router-bin

build-platform:
    cargo build --bin fc-platform-server

build-outbox:
    cargo build --bin fc-outbox-processor

build-stream:
    cargo build --bin fc-stream-processor

build-server:
    cargo build --bin fc-server

# Run unified server (all subsystems via env vars)
run-server:
    FC_DATABASE_URL={{ FC_DATABASE_URL }} cargo run --bin fc-server

# ─── Testing ───────────────────────────────────────────────────────────────

# Run all tests
test:
    cargo test --all-targets

# Run library tests only (faster)
test-lib:
    cargo test --lib

# Run platform tests
test-platform:
    cargo test --package fc-platform

# Run SDK tests
test-sdk:
    cargo test --package fc-sdk --all-features

# Run tests with output
test-verbose:
    cargo test --all-targets -- --nocapture

# ─── Code Quality ──────────────────────────────────────────────────────────

# Format all code (default-features only — fast)
fmt:
    cargo fmt --all

# `cargo fmt --all` only walks files reachable from each crate's default
# features, so cfg-gated code (e.g. the `oidc-flow`-gated module in
# fc-router) can drift unnoticed until CI's `cargo fmt --check` catches it.
# This recipe formats every .rs file under crates/ and bin/ instead.

# Format every .rs file including feature-gated modules (slower; matches CI)
fmt-all:
    find crates bin -name '*.rs' -not -path '*/target/*' -exec rustfmt --edition 2021 {} +

# Check formatting (matches CI)
fmt-check:
    cargo fmt --all -- --check

# Run clippy linter
lint:
    cargo clippy --all-targets -- -D warnings

# Run clippy with auto-fix
lint-fix:
    cargo clippy --all-targets --fix --allow-dirty

# ─── Cleanup ───────────────────────────────────────────────────────────────

# Clean build artifacts
clean:
    cargo clean

# Remove everything including Docker volumes
nuke: down
    docker volume rm flowcatalyst-rust_fc-pgdata 2>/dev/null || true
    @echo "All data removed. Run 'just setup' to start fresh."

# ─── Release ───────────────────────────────────────────────────────────────

# Show the current fc-dev version
release-version:
    @grep -E '^version = ' bin/fc-dev/Cargo.toml | head -1 | sed -E 's/^version = "([^"]+)"/\1/'

# Cut an fc-dev release. Pass `patch`, `minor`, `major`, or `X.Y.Z[-suffix]`
release bump:
    #!/usr/bin/env bash
    set -euo pipefail

    if [ -n "$(git status --porcelain)" ]; then
        echo "✗ Working tree is dirty. Commit or stash first." >&2
        git status --short
        exit 1
    fi

    current=$(grep -E '^version = ' bin/fc-dev/Cargo.toml | head -1 | sed -E 's/^version = "([^"]+)"/\1/')
    clean_re='^[0-9]+\.[0-9]+\.[0-9]+$'

    case "{{ bump }}" in
        patch|minor|major)
            if [[ ! "$current" =~ $clean_re ]]; then
                echo "✗ Cannot auto-bump '$current' (has prerelease suffix). Pass an explicit version." >&2
                exit 1
            fi
            ;;
    esac

    case "{{ bump }}" in
        patch) new=$(echo "$current" | awk -F. -v OFS=. '{$3++; print}') ;;
        minor) new=$(echo "$current" | awk -F. -v OFS=. '{$2++; $3=0; print}') ;;
        major) new=$(echo "$current" | awk -F. -v OFS=. '{$1++; $2=0; $3=0; print}') ;;
        *)
            if [[ "{{ bump }}" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
                new="{{ bump }}"
            else
                echo "✗ '{{ bump }}' is not patch|minor|major|X.Y.Z[-suffix]" >&2
                exit 1
            fi
            ;;
    esac

    if [ "$new" = "$current" ]; then
        echo "✗ Computed version $new is the same as current. Did you forget to bump?" >&2
        exit 1
    fi

    echo ""
    echo "  fc-dev: $current → $new"
    echo ""

    # The bare `version = "..."` line is unique in this file (workspace
    # bump uses `version.workspace = true` elsewhere in the workspace,
    # but fc-dev pins its own version literal).
    awk -v new="$new" '/^version = "[^"]+"$/ && !done {print "version = \"" new "\""; done=1; next} {print}' \
        bin/fc-dev/Cargo.toml > bin/fc-dev/Cargo.toml.tmp
    mv bin/fc-dev/Cargo.toml.tmp bin/fc-dev/Cargo.toml

    echo "Refreshing Cargo.lock…"
    if ! FC_SKIP_FRONTEND_BUILD=1 cargo check -p fc-dev --quiet; then
        echo "✗ cargo check failed. Reverting." >&2
        git checkout -- bin/fc-dev/Cargo.toml Cargo.lock 2>/dev/null || true
        exit 1
    fi

    echo ""
    echo "Changes:"
    git --no-pager diff --stat bin/fc-dev/Cargo.toml Cargo.lock
    echo ""
    read -r -p "Commit 'fc-dev v$new', tag fc-dev/v$new, and push? [y/N] " confirm || confirm="n"
    case "$confirm" in
        y|Y|yes|YES) ;;
        *)
            echo "Aborted. Reverting."
            git checkout -- bin/fc-dev/Cargo.toml Cargo.lock
            exit 1
            ;;
    esac

    git add bin/fc-dev/Cargo.toml Cargo.lock
    git commit -m "fc-dev v$new"
    git tag "fc-dev/v$new"
    git push origin HEAD "fc-dev/v$new"

    echo ""
    echo "✓ Released fc-dev v$new"
    echo ""
    echo "  Workflow:  https://github.com/flowcatalyst/flowcatalyst/actions/workflows/release-fc-dev.yml"
    echo "  Release:   https://github.com/flowcatalyst/flowcatalyst/releases/tag/fc-dev/v$new"

# ─── Tools ─────────────────────────────────────────────────────────────────

# Install development tools
install-tools:
    cargo install cargo-watch just
    @echo ""
    @echo "For faster builds, install lld linker:"
    @echo "  macOS:  brew install llvm"
    @echo "  Linux:  sudo apt install lld clang"
