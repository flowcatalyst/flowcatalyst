# Environment variables

FlowCatalyst is configured environment-first: there are no config files. The
central loader is `EnvCfg` in `internal/server/envcfg.go` (`LoadEnv()`), which
reads every knob the unified `fc-server` binary uses. A handful of services
additionally read env via package-local `FromEnv`-style constructors at startup
(`encryption`, `email`, `ratelimit`, `loginbackoff`, the scheduled-job
scheduler, `mcp`), and `cmd/fc-dev` seeds its CLI flag defaults from the same
variable names.

Conventions used in the tables below:

- **Aliases** are listed in priority order — the first set (non-empty) value
  wins. Where two names exist, the `FC_*` name is canonical and the alias is
  kept for drop-in compatibility with existing Rust/TS deployment configs.
- Boolean variables accept `1/true/yes/on` and `0/false/no/off`
  (case-insensitive); anything else falls back to the default.
- Unparseable numeric values silently fall back to the default.
- "(required for X)" marks variables whose absence makes a feature error or
  refuse to start, as opposed to degrading gracefully.
- `—` means no default / no alias.
- File citations are package paths; consult the package for the exact read
  site.

Note: `internal/secrets` also exposes an `env://VAR_NAME` secret-provider
scheme — any variable can be referenced that way; those reads are dynamic and
not listed here.

## 1. Core server & subsystem toggles

All read in `internal/server/envcfg.go`.

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FC_API_PORT` | `8080` | `PORT` | `internal/server/envcfg.go` | Unified API listener port (Rust default was 3000 — see README operator notes). |
| `FC_METRICS_PORT` | `9090` | — | `internal/server/envcfg.go` | Prometheus metrics listener port. |
| `FC_PLATFORM_ENABLED` | `true` | `PLATFORM_ENABLED` | `internal/server/envcfg.go` | Run the platform API (IAM, events, dispatch, BFF). |
| `FC_ROUTER_ENABLED` | `false` | `MESSAGE_ROUTER_ENABLED` | `internal/server/envcfg.go` | Run the message router subsystem. |
| `FC_SCHEDULER_ENABLED` | `false` | `DISPATCH_SCHEDULER_ENABLED` | `internal/server/envcfg.go` | Run the dispatch-job scheduler (currently NOOP publisher — see `internal/server/subsystems.go` warning). |
| `FC_SCHEDULED_JOB_ENABLED` | `false` | `SCHEDULED_JOB_SCHEDULER_ENABLED` | `internal/server/envcfg.go` | Run the scheduled-job cron + dispatch engine. |
| `FC_STREAM_PROCESSOR_ENABLED` | `false` | `STREAM_PROCESSOR_ENABLED` | `internal/server/envcfg.go` | Run the stream processor (CQRS projections + fan-out + partition manager). |
| `FC_OUTBOX_ENABLED` | `false` | `OUTBOX_PROCESSOR_ENABLED` | `internal/server/envcfg.go` | Run the outbox processor. |
| `FC_MCP_ENABLED` | `false` | — | `internal/server/envcfg.go` | Run the MCP HTTP server. |
| `FC_DEFAULT_BROKER` | `""` (no pools start) | — | `internal/server/envcfg.go` | Fallback queue backend when no `FLOWCATALYST_CONFIG_URL` is set; `postgres` synthesises a single `default` pool on the shared pool (fc-dev sets this). |

## 2. Database & AWS Secrets Manager

Resolution precedence (mirrors Rust fc-server): full URL
(`FC_DATABASE_URL`/`DATABASE_URL`) > Secrets Manager (`DB_SECRET_ARN` +
`DB_HOST`) > explicit `DB_*` credentials > local-dev default
`postgresql://postgres@localhost:5432/flowcatalyst`.

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FC_DATABASE_URL` | local-dev DSN (see above) | `DATABASE_URL` | `internal/server/envcfg.go`, `internal/server/dbsecret.go` | Full Postgres connection string; when set, all other DB vars are ignored. |
| `DB_HOST` | — | — | `internal/server/envcfg.go`, `internal/server/dbsecret.go` | Postgres host (may include `:port`); required to activate explicit-creds or Secrets-Manager mode. |
| `DB_PORT` | `5432` | — | `internal/server/envcfg.go`, `internal/server/dbsecret.go` | Postgres port; in SM mode the secret's own `port` field wins over this. |
| `DB_NAME` | `flowcatalyst` | — | `internal/server/envcfg.go`, `internal/server/dbsecret.go` | Database name. |
| `DB_USERNAME` | `postgres` | — | `internal/server/envcfg.go` | Username for explicit-creds mode (SM mode takes it from the secret JSON). |
| `DB_PASSWORD` | `""` | — | `internal/server/envcfg.go` | Password for explicit-creds mode (URL-escaped into the DSN). |
| `DB_SECRET_ARN` | — | — | `internal/server/dbsecret.go` | AWS Secrets Manager secret (RDS-style JSON: username/password/port). With `DB_HOST` set and no full URL, enables SM mode; region is taken from the ARN, credentials from the standard AWS chain. |
| `DB_SECRET_PROVIDER` | `aws` | — | `internal/server/dbsecret.go` | Secret provider; only `aws` is supported — any other value errors. |
| `DB_SECRET_REFRESH_INTERVAL_MS` | `300000` (5 min) | — | `internal/server/dbsecret.go` | Rotation poll cadence for `DBSecretRefresher`; `0` or negative disables rotation (single fetch at startup). |

## 3. Auth & JWT

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FC_JWT_ISSUER` | `http://localhost:8080` | `FC_EXTERNAL_BASE_URL`, `EXTERNAL_BASE_URL` | `internal/server/envcfg.go` | JWT issuer + audience and the platform's external base URL. |
| `FC_JWT_SIGNING_KEY_PATH` | — | — | `internal/server/envcfg.go` | Path to the PEM RSA private signing key (preferred source; fc-dev auto-creates one). |
| `FLOWCATALYST_JWT_PRIVATE_KEY` | — | — | `internal/server/signing_key.go` | Inline PEM RSA private key (the Rust/IaC name; checked before the Go alias). Mangled SSM values (`\n`, quotes, base64) are normalized. |
| `FC_JWT_SIGNING_KEY_PEM` | — | — | `internal/server/signing_key.go` | Go-native inline-PEM alias, checked after `FLOWCATALYST_JWT_PRIVATE_KEY`. If no key source is set, an **ephemeral** key is generated (tokens don't survive restarts and replicas reject each other's tokens — production must set one). |
| `FLOWCATALYST_JWT_PREVIOUS_PUBLIC_KEY` | — | — | `internal/server/envcfg.go` | Validation-only previous RSA public key for zero-downtime signing-key rotation; optional — skipped unless it parses as a PEM. |
| `AUTH_MODE` | — | — | `internal/server/run.go` | `NONE` (case-insensitive) forces router HTTP BasicAuth off regardless of creds; any other value (incl. `BASIC` or unset) uses the resolved creds. |
| `FC_ROUTER_AUTH_USER` | `""` (auth disabled) | `AUTH_BASIC_USERNAME` | `internal/server/run.go` | Router HTTP BasicAuth username; empty disables auth on the router surface. |
| `FC_ROUTER_AUTH_PASS` | `""` | `AUTH_BASIC_PASSWORD` | `internal/server/run.go` | Router HTTP BasicAuth password. |
| `FC_AUTH_ALLOW_TEST_HEADERS` | `false` | — | `internal/server/envcfg.go` | Enables the `X-FC-Test-Principal` dev fallback in the platform Authenticator (fc-dev turns it on; never in production). |

## 4. Encryption & secrets

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FLOWCATALYST_APP_KEY` | — | — | `internal/platform/shared/encryption`, `internal/server/subsystems.go`, `cmd/fc-dev`, `cmd/decrypt-check` | Field-encryption key (base64, AES-GCM). Unset → encryption disabled: confidential OAuth client-secret minting fails and TOTP enrollment degrades; the dispatch scheduler **refuses to start** (its HMAC dispatch-auth secret is HKDF-derived from this key). fc-dev generates + persists one. |
| `FLOWCATALYST_APP_KEY_PREVIOUS` | — | — | `internal/platform/shared/encryption` | Previous encryption key; decryption falls back to it during key rotation (new writes always use the current key). |
| `FLOWCATALYST_SIGNING_SECRET` | — | — | `pkg/fcsdk/webhook` | Webhook HMAC-SHA256 signing secret for consumer apps using the Go SDK's `ValidatorFromEnv` (required for SDK webhook validation — errors when unset). |

## 5. Rate limiting

Distributed cluster-wide policies (`FC_RL_*`) are read in
`internal/platform/shared/ratelimit` (`PoliciesFromEnv`); the in-memory
per-instance governors (`FC_OAUTH_TOKEN_*`, `FC_OIDC_*`) sit in front of them
as defence-in-depth.

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FC_RL_OAUTH_TOKEN_IP_PER_MIN` | `600` | — | `internal/platform/shared/ratelimit` | Cluster-wide `/oauth/token` requests per IP per minute. |
| `FC_RL_OAUTH_TOKEN_CLIENT_PER_MIN` | `300` | — | `internal/platform/shared/ratelimit` | Cluster-wide `/oauth/token` requests per client_id per minute. |
| `FC_RL_OAUTH_AUTHORIZE_IP_PER_MIN` | `600` | — | `internal/platform/shared/ratelimit` | Cluster-wide `/oauth/authorize` requests per IP per minute. |
| `FC_RL_OAUTH_AUTHORIZE_CLIENT_PER_MIN` | `300` | — | `internal/platform/shared/ratelimit` | Cluster-wide `/oauth/authorize` requests per client_id per minute. |
| `FC_RL_PASSWORD_RESET_IP_PER_HOUR` | `20` | — | `internal/platform/shared/ratelimit` | Password-reset requests per IP per hour. |
| `FC_RL_PASSWORD_RESET_EMAIL_PER_HOUR` | `5` | — | `internal/platform/shared/ratelimit` | Password-reset requests per target email per hour. |
| `FC_OAUTH_TOKEN_IP_RATE_PER_MIN` | `120` | — | `internal/platform/shared/ratelimit` | Per-instance in-memory `/oauth/token` sustained rate per IP. |
| `FC_OAUTH_TOKEN_IP_BURST` | `60` | — | `internal/platform/shared/ratelimit` | Per-instance instantaneous burst allowance per IP at `/oauth/token`. |
| `FC_OAUTH_TOKEN_CLIENT_RATE_PER_MIN` | `60` | — | `internal/platform/shared/ratelimit` | Per-instance sustained rate per client_id at `/oauth/token`. |
| `FC_OAUTH_TOKEN_CLIENT_BURST` | `30` | — | `internal/platform/shared/ratelimit` | Per-instance burst allowance per client_id at `/oauth/token`. |
| `FC_OIDC_RATE_PER_MIN` | `60` | — | `internal/platform/shared/ratelimit` | Per-instance sustained rate per IP on the `/auth/oidc/*` bridge routes. |
| `FC_OIDC_BURST` | `30` | — | `internal/platform/shared/ratelimit` | Per-instance burst allowance on the OIDC bridge routes. |
| `FC_RATE_LIMIT_DISABLE` | unset | — | `internal/platform/shared/ratelimit` | `1` replaces the distributed store with a no-op (everything allowed). |
| `FC_REDIS_URL` | — | — | `internal/platform/shared/ratelimit` | Redis backend for the distributed rate-limit store; set + reachable → Redis, else falls back to the Postgres store. |

## 6. Login backoff

All read in `internal/platform/auth/loginbackoff` (`PolicyFromEnv`) —
brute-force protection on the password login endpoint.

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FC_LOGIN_BACKOFF_FREE_ATTEMPTS` | `3` | — | `internal/platform/auth/loginbackoff` | Failures per (identifier, IP) allowed before any delay. |
| `FC_LOGIN_BACKOFF_BASE_SECS` | `2` | — | `internal/platform/auth/loginbackoff` | Delay at the first throttled attempt; doubles per further failure. |
| `FC_LOGIN_BACKOFF_MAX_SECS` | `300` | — | `internal/platform/auth/loginbackoff` | Cap on the per-(identifier, IP) exponential backoff delay. |
| `FC_LOGIN_GLOBAL_WINDOW_SECS` | `3600` | — | `internal/platform/auth/loginbackoff` | Sliding window for the per-identifier global failure ceiling. |
| `FC_LOGIN_GLOBAL_CEILING` | `100` | — | `internal/platform/auth/loginbackoff` | Failures across all IPs in-window that trigger a lock. |
| `FC_LOGIN_GLOBAL_LOCK_SECS` | `900` | — | `internal/platform/auth/loginbackoff` | Lock duration once the global ceiling trips. |

## 7. Email / SMTP

All read in `internal/platform/shared/email` (`FromEnv`). When no host is set,
emails (password-reset links, 2FA PINs) are **logged instead of sent**.

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FC_SMTP_HOST` | — (unset → log-only mailer) | `SMTP_HOST` | `internal/platform/shared/email` | SMTP server host; required to send real email. |
| `FC_SMTP_PORT` | `587` | `SMTP_PORT` | `internal/platform/shared/email` | SMTP port. |
| `FC_SMTP_USERNAME` | `""` | `SMTP_USERNAME` | `internal/platform/shared/email` | SMTP auth username (empty → unauthenticated). |
| `FC_SMTP_PASSWORD` | `""` | `SMTP_PASSWORD` | `internal/platform/shared/email` | SMTP auth password. |
| `FC_SMTP_FROM` | `noreply@flowcatalyst.local` | `SMTP_FROM` | `internal/platform/shared/email` | From address. |
| `FC_SMTP_SECURE` | `false` (STARTTLS) | `SMTP_SECURE` | `internal/platform/shared/email` | `true` → implicit TLS (e.g. :465); `false` → STARTTLS (e.g. :587). |

## 8. WebAuthn (passkeys)

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FC_WEBAUTHN_RP_ID` | `localhost` | — | `internal/server/wire_services.go` | Relying-party ID — the registrable parent domain covering all passkey origins. |
| `FC_WEBAUTHN_ORIGINS` | `http://localhost:8080` | `FC_WEBAUTHN_RP_ORIGIN` (legacy, singular) | `internal/server/envcfg.go` | Comma-separated allowed origins; go-webauthn matches exact scheme+host, so every origin must be listed verbatim. |

## 9. Subsystems

### Router

Toggle (`FC_ROUTER_ENABLED`) and BasicAuth are in families 1 and 3; broker
config (`FLOWCATALYST_CONFIG_URL`, `FC_DEFAULT_BROKER`), notifications and
ALB self-registration are in families 11 and 1.

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FC_ROUTER_HTTP_PREFIX` | `/router` | — | `internal/server/envcfg.go` | Mount prefix for the router HTTP surface on the unified API listener. |
| `FC_DRAIN_TIMEOUT_SECONDS` | `60` | — | `internal/server/envcfg.go` | Upper bound for the router's graceful in-flight drain on shutdown. |
| `FLOWCATALYST_DEV_MODE` | `false` | — | `internal/server/envcfg.go` | Swaps in the router's dev mediator (relaxed TLS, longer timeouts). |

### Outbox processor

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FC_OUTBOX_PLATFORM_URL` | — (**required**: processor logs an error and skips startup without it) | `FC_OUTBOX_API_URL`, `FC_API_BASE_URL`, `FLOWCATALYST_URL` | `internal/server/envcfg.go` | Platform API base URL the outbox delivers batches to. |
| `FC_OUTBOX_PLATFORM_AUTH_TOKEN` | `""` | `FC_OUTBOX_TOKEN`, `FC_API_TOKEN` | `internal/server/envcfg.go` | Bearer token / OAuth client_secret used against the platform. |
| `FC_OUTBOX_BATCH_SIZE` | `0` (library default `100`) | — | `internal/server/envcfg.go`, `cmd/fc-dev` | Rows per poll. |
| `FC_OUTBOX_MAX_IN_FLIGHT` | `0` (library default `1000`) | — | `internal/server/envcfg.go`, `cmd/fc-dev` | Cap on outstanding HTTP requests. |
| `FC_OUTBOX_POLL_INTERVAL_MS` | `0` (library default `1000`) | — | `internal/server/envcfg.go`, `cmd/fc-dev` | Sleep between empty polls. |
| `FC_OUTBOX_MAX_CONCURRENT_GROUPS` | `0` (library default `10`) | `FC_MAX_CONCURRENT_GROUPS` | `internal/server/envcfg.go` | Max message groups processed concurrently. |
| `FC_OUTBOX_BLOCK_ON_ERROR` | `true` | — | `internal/server/envcfg.go` | Stop a group on a failing item so the rest re-run in order behind it. |
| `FC_OUTBOX_ADMIN_PORT` | `0` (off) | — | `internal/server/envcfg.go` | Serves the operational admin API (pause/resume/unblock/skip groups) on `127.0.0.1:<port>`. |
| `FC_OUTBOX_BACKEND` | `postgres` | `FC_OUTBOX_DB_TYPE` (Rust name) | `internal/server/envcfg.go` | Storage backend: `postgres` (shared pool) or `mongo`; anything else errors clearly. |
| `FC_OUTBOX_MONGO_URI` | — | `FC_OUTBOX_DB_URL` | `internal/server/envcfg.go` | Mongo connection string (required when backend is `mongo`). |
| `FC_OUTBOX_MONGO_DB` | `flowcatalyst` | — | `internal/server/envcfg.go` | Mongo database name. |
| `FC_OUTBOX_SOURCE_DB_URL` | — | — | `cmd/fc-dev` | `fc-dev outbox` only: the external app's Postgres URL to poll (flag default). |

### Stream processor

Per-projection batch-size precedence: per-projection env var >
`FC_STREAM_BATCH_SIZE` > the Rust default (events/dispatch `100`, fan-out
`200`).

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FC_STREAM_EVENTS_ENABLED` | `true` | — | `internal/server/envcfg.go` | Event projection sub-toggle (defaults ON so the top-level flag suffices). |
| `FC_STREAM_DISPATCH_JOBS_ENABLED` | `true` | — | `internal/server/envcfg.go` | Dispatch-job projection sub-toggle. |
| `FC_STREAM_FAN_OUT_ENABLED` | `true` | — | `internal/server/envcfg.go` | Event fan-out sub-toggle. |
| `FC_STREAM_PARTITION_MANAGER_ENABLED` | `true` | `FC_STREAM_PARTITIONS_ENABLED` (back-compat) | `internal/server/envcfg.go` | Partition-manager sub-toggle (leader-only DDL). |
| `FC_STREAM_BATCH_SIZE` | `0` (per-projection defaults) | — | `internal/server/envcfg.go` | Global batch-size override for all projections. |
| `FC_STREAM_EVENTS_BATCH_SIZE` | `0` (default `100`) | — | `internal/server/subsystems.go` | Event-projection batch size. |
| `FC_STREAM_DISPATCH_JOBS_BATCH_SIZE` | `0` (default `100`) | — | `internal/server/subsystems.go` | Dispatch-job-projection batch size. |
| `FC_STREAM_FAN_OUT_BATCH_SIZE` | `0` (default `200`) | — | `internal/server/subsystems.go` | Fan-out batch size. |
| `FC_STREAM_FAN_OUT_SUBS_REFRESH_SECS` | `0` (default 5s) | — | `internal/server/envcfg.go` | Fan-out subscription-cache TTL. |
| `FC_STREAM_PARTITION_MONTHS_FORWARD` | `0` (default `3`) | — | `internal/server/envcfg.go` | Months of partitions to pre-create. |
| `FC_STREAM_PARTITION_RETENTION_DAYS` | `0` (default `90`) | — | `internal/server/envcfg.go` | Partition retention before drop. |
| `FC_STREAM_PARTITION_TICK_HOURS` | `0` (default `24`) | — | `internal/server/envcfg.go` | Partition-manager tick cadence. |

### Scheduled-job scheduler

All read in `internal/platform/scheduledjob/scheduler` (`ConfigFromEnv`);
unset or unparseable → the Rust default.

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FC_SCHEDULED_JOB_POLL_SECONDS` | `30` | — | `internal/platform/scheduledjob/scheduler` | Cron-poller wake-up interval. |
| `FC_SCHEDULED_JOB_DISPATCH_SECONDS` | `5` | — | `internal/platform/scheduledjob/scheduler` | Dispatcher wake-up interval. |
| `FC_SCHEDULED_JOB_DISPATCH_BATCH` | `32` | — | `internal/platform/scheduledjob/scheduler` | Max QUEUED instances per dispatch tick. |
| `FC_SCHEDULED_JOB_HTTP_TIMEOUT_SECONDS` | `10` | — | `internal/platform/scheduledjob/scheduler` | Per-webhook HTTP timeout. |

### Standby / leader election

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FC_STANDBY_ENABLED` | `false` | `STANDBY_ENABLED` | `internal/server/envcfg.go` | Enable Redis leader election for HA (single-active subsystems gate on it; election failure fails closed). |
| `FC_STANDBY_REDIS_URL` | `redis://127.0.0.1:6379` | `REDIS_URL` | `internal/server/envcfg.go` | Redis used for leader election. |
| `FC_STANDBY_LOCK_KEY` | `fc:server:leader` | — | `internal/server/envcfg.go` | Election lock key; background subsystems elect on subsystem-suffixed keys (e.g. `…:stream`). |

### MCP server

Resolution: env vars → `mcp-credentials.json` in the OS cache dir
(`<user-cache>/flowcatalyst-dev/mcp-credentials.json`, written by fc-dev's
bootstrap) → defaults. With client_id+secret it mints OAuth tokens; with only
a secret it uses it as a static bearer token; with neither it calls the
platform unauthenticated (local dev only — the standalone server fails fast).

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FC_MCP_PORT` | `8090` | — | `internal/server/envcfg.go` | MCP HTTP listener port. |
| `FC_MCP_BIND` | `127.0.0.1` (localhost-only) | — | `internal/server/envcfg.go` | MCP bind host; set `0.0.0.0` to expose (e.g. in a container). |
| `FLOWCATALYST_URL` | `http://localhost:8080` (after credentials-file fallback) | `FC_MCP_PLATFORM_URL` (in-process server only) | `internal/mcp`, `internal/server/envcfg.go` | Platform API root the MCP server proxies into. Also the last alias of `FC_OUTBOX_PLATFORM_URL`. |
| `FLOWCATALYST_CLIENT_ID` | — (credentials-file fallback) | — | `internal/mcp`, `internal/server/envcfg.go` | OAuth client_credentials client id for token minting. |
| `FLOWCATALYST_CLIENT_SECRET` | — (credentials-file fallback) | — | `internal/mcp`, `internal/server/envcfg.go` | OAuth client secret (or static bearer token when used alone). |

## 10. Bootstrap & seeding

The seeder creates the initial super-admin only when **both** email and
password are set and no anchor user exists yet; otherwise it logs a warning
and skips (production must opt in explicitly). fc-dev pre-sets
`admin@flowcatalyst.local` / `DevPassword123!` / `Local Admin` so the local
flow just works (existing env values win).

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FLOWCATALYST_BOOTSTRAP_ADMIN_EMAIL` | — | — | `internal/platform/seed` | Initial ANCHOR super-admin email (required together with the password to seed). |
| `FLOWCATALYST_BOOTSTRAP_ADMIN_PASSWORD` | — | — | `internal/platform/seed` | Initial super-admin password. |
| `FLOWCATALYST_BOOTSTRAP_ADMIN_NAME` | `Bootstrap Admin` | — | `internal/platform/seed` | Initial super-admin display name. |
| `FC_BOOTSTRAP_ADMIN_EMAIL` | — | — | `cmd/fc-dev` | `fc-dev init` only: flag default for `--admin-email`. |
| `FC_BOOTSTRAP_ADMIN_PASSWORD` | — | — | `cmd/fc-dev` | `fc-dev init` only: flag default for `--admin-password` (required with `--yes`). |

## 11. Logging, router config & misc

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FC_LOG_LEVEL` | `info` | — | `internal/logging` | slog level: `debug`, `warn`/`warning`, `error` (case-insensitive variants accepted). |
| `FLOWCATALYST_CONFIG_URL` | — | — | `internal/server/envcfg.go` | Router pool/broker configuration endpoint; unset → `FC_DEFAULT_BROKER` fallback (or no pools). |
| `FC_NOTIFY_WEBHOOK_URL` | — (log-only) | — | `internal/server/envcfg.go` | Webhook receiving router stall + backlog warnings. |
| `FC_ALB_ENABLED` | `false` | — | `internal/server/envcfg.go` | Router ALB self-registration: register this instance on leader-gain / start, deregister on leader-loss / shutdown. |
| `FC_ALB_TARGET_GROUP_ARN` | — | — | `internal/server/envcfg.go` | ELBv2 target group to (de)register with. |
| `FC_ALB_TARGET_ID` | — | `FC_ALB_INSTANCE_IP` | `internal/server/envcfg.go` | Target id (this instance's IP) for RegisterTargets. |
| `FC_ALB_TARGET_PORT` | `8080` | — | `internal/server/envcfg.go` | Target port registered with the ALB. |
| `FC_ALB_REGION` | — (AWS SDK default region chain) | — | `internal/server/envcfg.go` | AWS region for the ELBv2 client. |
| `FC_ALB_DEREGISTRATION_DELAY_SECONDS` | `0` | — | `internal/server/envcfg.go` | Wait after deregistration before shutdown proceeds. |
| `XDG_DATA_HOME` | OS app-data dir | — | `cmd/fc-dev` | Overrides the per-user data directory that hosts fc-dev's embedded-Postgres cluster, JWT key and app key. |

## 12. fc-dev (local dev CLI)

`fc-dev` seeds its flag defaults from env, reusing the server names —
`FC_API_PORT`, `FC_METRICS_PORT`, `FC_DATABASE_URL`, `FC_SCHEDULER_ENABLED`,
`FC_STREAM_PROCESSOR_ENABLED`, `FC_OUTBOX_ENABLED`, `FC_ROUTER_ENABLED`,
`FC_MCP_ENABLED` — but with dev-friendly defaults (scheduler/stream/router ON,
`FC_OUTBOX_PLATFORM_URL` defaulting to `http://localhost:8080` for
`fc-dev outbox`). Flags always win over env. Variables unique to fc-dev:

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FC_EMBEDDED_DB` | `true` | — | `cmd/fc-dev` | Start an embedded Postgres when no `--database-url` is given. |
| `FC_EMBEDDED_DB_PORT` | `15432` | — | `cmd/fc-dev` | Embedded Postgres port. |
| `FC_EMBEDDED_DB_PATH` | `<user-data-dir>/flowcatalyst/embedded-pg` | — | `cmd/fc-dev` | Embedded Postgres data directory (never /tmp). |
| `FC_DEV_UPGRADE_REPO` | `flowcatalyst/flowcatalyst` | — | `cmd/fc-dev` | GitHub repo `fc-dev upgrade` pulls releases from (point at a fork). |

The installer scripts (`install.sh` / `install.ps1`) additionally honour
`FC_DEV_VERSION`, `FC_DEV_INSTALL_DIR` and `FC_DEV_FORCE` — shell-side only,
documented in the README.

## 13. SDK example programs (not server configuration)

The runnable examples under `pkg/fcsdk/examples/` read their own variables;
listed for completeness only.

| Variable | Default | Aliases | Read in | Purpose |
|---|---|---|---|---|
| `FC_BASE_URL` | — (required) | — | `pkg/fcsdk/examples` | Platform base URL for the example clients. |
| `FC_TOKEN` | — (required by fc-sync / scheduled-jobs-runner; optional in list-event-types) | — | `pkg/fcsdk/examples` | Static bearer token. |
| `FC_APP` | — | — | `pkg/fcsdk/examples/list-event-types` | Application code to query. |
| `FC_ISSUER` | — | — | `pkg/fcsdk/examples/list-event-types` | OAuth issuer when minting a token instead of `FC_TOKEN`. |
| `FC_CLIENT_ID` | — | — | `pkg/fcsdk/examples/list-event-types` | OAuth client id for token minting. |
| `FC_CLIENT_SECRET` | — | — | `pkg/fcsdk/examples/list-event-types` | OAuth client secret for token minting. |
| `FC_DATABASE_URL` | — | — | `pkg/fcsdk/examples/order-service` | The example app's own outbox Postgres URL (same name, different process). |
