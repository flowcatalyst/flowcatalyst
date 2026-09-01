# FlowCatalyst

A multi-tenant event router and webhook delivery platform, written in Go.

Applications publish domain events. Other applications (or yours, or external
services) consume them via webhook subscriptions. FlowCatalyst handles routing,
FIFO ordering, retry, rate-limiting, circuit breaking, and audit — across
multiple tenants — so you don't have to reimplement that machinery in every
consumer.

The Postgres schema, HTTP API contracts, OpenAPI spec, and Vue frontend are
stable: existing SDK consumers and webhook subscribers keep working unchanged
across releases.

---

## Architecture at a glance

```
┌───────────────────┐                          ┌──────────────────────┐
│ Consumer apps     │  ──── events ──────▶     │   FlowCatalyst       │
│ (SDK / outbox)    │                          │                      │
│                   │  ◀──── webhooks ────     │   Platform · Router  │
└───────────────────┘                          │   Scheduler · Stream │
                                                │   Outbox             │
                                                └──┬───────────┬───────┘
                                                   │           │
                                                   ▼           ▼
                                              PostgreSQL    SQS FIFO
                                                   │           │
                                                   ▼           ▼
                                                Redis       Customer IDPs
                                              (HA leader)  (OIDC bridge)
```

In local development everything above collapses into a single `fcdev` process
backed by an **embedded Postgres** — no Docker, no SQS, no Redis. (The embedded
Postgres is vanilla; to add PostGIS see
[`docs/embedded-postgres-postgis.md`](docs/embedded-postgres-postgis.md).)

---

## Quick start

### As a developer publishing events

```sh
# 1. Install fcdev (the local development binary)
curl -fsSL https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.sh | sh

# 2. Run it
fcdev

# 3. Open http://localhost:8080
```

Windows (PowerShell 5.1+):

```powershell
irm https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.ps1 | iex
```

See [Installing fcdev](#installing-fcdev) for pinning versions, manual
installs, checksum/signature verification, and self-update.

### As an operator deploying production

Deploy `fc-server` (the unified binary) with the subsystems you need enabled via
env vars — run one instance with everything on, or several instances each
running a subset (router-only, stream-only, …) for split topologies. See
[Operator notes](#operator-notes).

### As an engineer working on FlowCatalyst itself

Read [`docs/architecture.md`](docs/architecture.md), then
[`CONVENTIONS.md`](CONVENTIONS.md) before writing any code. The
[`docs/usecase-pattern.md`](docs/usecase-pattern.md) walks the
compile-time-sealed UseCase + UnitOfWork pattern with a worked example.

---

## Installing fcdev

`fcdev` is the all-in-one local-development binary. It ships pre-built for
macOS (Apple Silicon + Intel), Linux (x86_64 + arm64), and Windows (x86_64).
The install commands below fetch the latest release from GitHub, verify it, and
put it on your `PATH`.

Release assets are named with Go's **GOOS-GOARCH** convention
(`darwin-arm64`, `darwin-amd64`, `linux-amd64`, `linux-arm64`,
`windows-amd64`).

### macOS / Linux

```sh
curl -fsSL https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.sh | sh
```

The script detects your OS/arch from `uname`, downloads the matching archive,
verifies its SHA256 sidecar, installs to `/usr/local/bin/fcdev` if writable
(else `~/.local/bin/fcdev` — no `sudo` prompt), and on macOS strips the
Gatekeeper quarantine attribute.

### Windows

```powershell
irm https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.ps1 | iex
```

Downloads the `windows-amd64` zip, verifies its SHA256, extracts to
`%LOCALAPPDATA%\Programs\fcdev`, and adds that directory to your **user** PATH
(re-open your terminal afterwards). Windows-on-ARM runs the x64 binary under
emulation.

### Upgrading

Once `fcdev` is on your `PATH`, it self-updates — no need to re-run the
installer:

```sh
fcdev upgrade           # download & replace if a newer release exists
fcdev upgrade --check   # just check, don't install
fcdev upgrade --force   # re-install even if already current
```

`upgrade` uses the same release artifacts the installer scripts do, verifies the
SHA256, and atomically replaces the running binary.

### Environment variables

Both installer scripts honour the same three variables:

| Variable | Default | Purpose |
|---|---|---|
| `FC_DEV_VERSION` | latest stable | Pin a specific version (e.g. `0.5.0`). The leading `v` is optional. |
| `FC_DEV_INSTALL_DIR` | platform default (see above) | Where to write `fcdev`. |
| `FC_DEV_FORCE` | `0` | When `1`, reinstall even if the requested version is already present. |

Example — pin a version into `~/bin` on Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.sh \
  | FC_DEV_VERSION=0.5.0 FC_DEV_INSTALL_DIR="$HOME/bin" sh
```

### Manual install

If you'd rather not pipe a remote script into your shell, every archive on the
[releases page](https://github.com/flowcatalyst/flowcatalyst/releases) ships a
`.sha256` sidecar:

```sh
# 1. Pick your target (Go GOOS-GOARCH)
TARGET=darwin-arm64        # macOS Apple Silicon
# TARGET=darwin-amd64      # macOS Intel
# TARGET=linux-amd64       # Linux x86_64
# TARGET=linux-arm64       # Linux ARM64
# TARGET=windows-amd64     # Windows (use the .zip variant)

VERSION=0.5.0

# 2. Download archive + checksum
base="https://github.com/flowcatalyst/flowcatalyst/releases/download/fcdev/v${VERSION}"
curl -LO "${base}/fcdev-v${VERSION}-${TARGET}.tar.gz"
curl -LO "${base}/fcdev-v${VERSION}-${TARGET}.tar.gz.sha256"

# 3. Verify
shasum -a 256 -c "fcdev-v${VERSION}-${TARGET}.tar.gz.sha256"

# 4. Extract + install
tar -xzf "fcdev-v${VERSION}-${TARGET}.tar.gz"
sudo install -m 0755 "fcdev-v${VERSION}-${TARGET}/fcdev" /usr/local/bin/

# 5. (macOS only) strip Gatekeeper quarantine
xattr -d com.apple.quarantine /usr/local/bin/fcdev 2>/dev/null || true

fcdev version
```

On a **private** checkout or rate-limited network you can use the GitHub CLI
instead, which reuses your `gh` auth:

```sh
gh release download fcdev/v0.5.0 --repo flowcatalyst/flowcatalyst \
  --pattern 'fcdev-v0.5.0-darwin-arm64.tar.gz*'
```

### Verifying Linux archives (optional)

Linux release archives are additionally signed via Sigstore **cosign** keyless —
the signature is bound to the exact GitHub Actions run that built it, recorded in
the public Rekor transparency log:

```sh
cosign verify-blob \
  --signature  "fcdev-v${VERSION}-${TARGET}.tar.gz.sig" \
  --certificate "fcdev-v${VERSION}-${TARGET}.tar.gz.pem" \
  --certificate-identity-regexp '^https://github.com/flowcatalyst/flowcatalyst(-go)?/\.github/workflows/release-fcdev\.yml@refs/tags/fcdev/v' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  "fcdev-v${VERSION}-${TARGET}.tar.gz"
```

Each release's notes include the exact `--certificate-identity-regexp` for that
build. macOS and Windows archives are not yet codesigned.

### Troubleshooting

- **`fcdev: command not found` after install** — the install dir isn't on your
  `PATH` yet. Open a new terminal, or `source ~/.zshrc` (the script prints the
  exact line to add).
- **macOS "cannot be opened because it is from an unidentified developer"** —
  the installer strips the quarantine xattr automatically; for a manual download
  run `xattr -d com.apple.quarantine /usr/local/bin/fcdev`.
- **First run downloads Postgres** — `fcdev` fetches a platform-specific
  embedded-Postgres bundle on first launch, so the initial start needs network
  access and is slower than subsequent runs.

---

## Binaries

| Binary | Purpose |
|---|---|
| `fc-server` | Unified production server — every subsystem (platform API, router, scheduler, stream, outbox, MCP) toggleable via env vars. Run one instance with everything on, or several each running a subset for split topologies. |
| `fcdev` | Local development monolith: all subsystems in one process against an embedded Postgres. |

The repo ships these **two** binaries rather than a separate executable per
subsystem — split topologies are achieved by running multiple `fc-server`
instances with different subsystems enabled.

---

## Building from source

The repo builds with `make` (run `make help` for the full target list):

```sh
make build            # build the Vue frontend, then every Go binary into bin/
make build-release    # optimized build (trimpath, -s -w)
make go-build         # Go binaries only (assumes frontend/dist already built)
```

Local dev loop (embedded Postgres — no Docker, no separate migrate step):

```sh
make run              # run fcdev once
make dev              # run fcdev with live reload (requires air; see `make install-tools`)
make dev-full         # fcdev + the Vite frontend dev server together
make run-server       # run the unified fc-server (subsystems via env)
```

Bootstrap & database:

```sh
make setup            # bootstrap admin user + default tenant + .env, then print next steps
make init             # bootstrap only
make fresh            # truncate every FlowCatalyst table (keeps schema)
make db-reset         # wipe the embedded Postgres data dir and start fresh
```

Quality gates:

```sh
make test             # unit + integration (testcontainers Postgres)
make lint             # golangci-lint
make sqlc-verify      # ensure generated sqlc code is up to date
make ci               # everything CI runs
```

---

## SDK clients

| Language | Location |
|---|---|
| TypeScript / JavaScript | [`clients/typescript-sdk/`](clients/typescript-sdk/) |
| Laravel / PHP | [`clients/laravel-sdk/`](clients/laravel-sdk/) |
| Go (in-repo package) | [`pkg/fcsdk/`](pkg/fcsdk/) |

The TS and Laravel SDKs are generated from the huma OpenAPI spec
(`make sdk-generate`); releases are cut with `make release-ts-sdk` /
`make release-laravel-sdk` and mirrored to standalone repos. All SDKs cover the
outbox pattern for atomic event publishing, definition syncing for declaring
event types and roles, and webhook signature verification.

---

## Technology stack

| Layer | Technology |
|---|---|
| Language | Go 1.26 |
| HTTP router | go-chi/chi v5 |
| API / OpenAPI | huma v2 (OpenAPI 3.1, generated spec) |
| Database | PostgreSQL via pgx v5; handwritten SQL compiled with sqlc; goose migrations |
| Embedded DB (dev) | fergusstrange/embedded-postgres |
| Queue | AWS SQS FIFO (prod), embedded Postgres (dev); NATS supported |
| Auth | RS256 JWT (golang-jwt + lestrrat-go/jwx), OIDC bridge (coreos/go-oidc), WebAuthn/passkeys (go-webauthn), Argon2id passwords, HMAC webhook signing |
| Caching / HA | in-process cache + Redis (go-redis) for leader election |
| Metrics | Prometheus (client_golang) |
| Scheduling | robfig/cron |
| Secrets | AWS Secrets Manager, encrypted files |
| CLI | spf13/cobra |
| MCP | modelcontextprotocol/go-sdk (read-only server for LLM clients) |
| Frontend | Vue SPA, built and embedded via `//go:embed` |

PostgreSQL extensions: **none required**. Partitioning is managed in-process,
the same code in dev and prod — no `pg_partman` or `pg_cron`.

---

## Operator notes

### Default HTTP port

`fc-server` defaults to `FC_API_PORT=8080`. Older deployments defaulted to
`3000` — operators behind load-balancer rules that hard-code `3000` should set
`FC_API_PORT=3000` explicitly. The `FC_API_PORT` env var (or its legacy alias
`PORT`) overrides the binary default.

| Service       | Default             | Override env             |
|---------------|---------------------|--------------------------|
| Platform HTTP | `FC_API_PORT=8080`  | `FC_API_PORT` / `PORT`   |
| fcdev        | `--api-port 8080`   | `FC_API_PORT` / `--api-port` |
| Metrics       | `FC_METRICS_PORT=9090` | `FC_METRICS_PORT`        |
| MCP           | `127.0.0.1:8090`    | `FC_MCP_PORT` / `FC_MCP_BIND` |

The complete environment-variable reference (every variable the server, subsystems, and `fcdev` read, with defaults and aliases) is in [`docs/environment-variables.md`](docs/environment-variables.md).

---

## Documentation

- [`docs/architecture.md`](docs/architecture.md) — package layout and library choices
- [`CONVENTIONS.md`](CONVENTIONS.md) — engineering conventions (read before writing code)
- [`docs/usecase-pattern.md`](docs/usecase-pattern.md) — the sealed UseCase + UnitOfWork pattern, worked example
- [`docs/wire-contract.md`](docs/wire-contract.md) — the HTTP wire contract and how it's enforced
- [`docs/sqlc.md`](docs/sqlc.md) — sqlc workflow and conventions
- [`docs/oidc-security-audit.md`](docs/oidc-security-audit.md) — OIDC implementation security review
- [`docs/adr/`](docs/adr/) — architecture decision records

---

## License

Copyright © 2026 Belac (FlowCatalyst)

| Component | License | Text |
| --- | --- | --- |
| Platform (`cmd/`, `internal/`, and everything not listed below) | AGPL-3.0-or-later | [`LICENSE`](LICENSE) |
| Go SDK (`pkg/fcsdk/`) | MPL-2.0 | [`pkg/fcsdk/LICENSE`](pkg/fcsdk/LICENSE) |

The split is deliberate. The platform is the thing worth protecting, and the
AGPL is the only common copyleft licence that reaches a **hosted** fork —
running modified software as a service is not distribution, so the GPL alone
would let a competitor fork FlowCatalyst, operate it, and share nothing. The
SDK has the opposite requirement: it is imported directly into customer
applications, so a reciprocal licence there would reach into their code.
MPL-2.0 is file-level copyleft — changes to the SDK's own files stay open,
while an application that merely imports it remains under whatever licence its
authors choose.
