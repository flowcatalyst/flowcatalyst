# E2E platform: an isolated FlowCatalyst for application end-to-end checks

`scripts/e2e-platform.sh` (or `make e2e-platform CMD=...`) runs a second, fully separate
FlowCatalyst platform next to a developer's own `fcdev`, seeded for the Camel ingestor
(`ingest-runtime` + `ingest-control-plane`, in the inhance mono-repo). Applications run their
end-to-end checks against it without touching anyone's real data.

```sh
make e2e-platform                # = scripts/e2e-platform.sh up: build, start, seed, print the env
make e2e-platform CMD=verify     # health, OIDC login (both users), token mint, dispatch round trip
make e2e-platform CMD=status     # pid, ports, database
make e2e-platform CMD=env        # print the generated env (holds secrets)
make e2e-platform CMD=down       # stop; the database and credentials are kept
make e2e-platform CMD=drop       # stop, DROP DATABASE e2e_camel, delete the state dir
```

`up` is idempotent: run it again and it reuses the running instance and the stored credentials,
and re-applies the seed.

## What runs

| | Value |
|---|---|
| Platform (API, OIDC issuer, admin SPA) | `http://localhost:18080` |
| Metrics | `http://localhost:19090` |
| Database | `e2e_camel` in the embedded Postgres on `127.0.0.1:15432` (shared mode), or in a cluster the script starts on `127.0.0.1:25432` (own mode) |
| Process | `tmp/e2e-platform/bin/fcdev start`, built from this checkout by `up` |
| State dir (gitignored) | `tmp/e2e-platform/`: `e2e.env` and `secrets.env` (0600), `logs/fcdev.log`, `fcdev.pid`, signing key, encryption key, a private `home/` |
| Subsystems | platform, router (Postgres broker), dispatch scheduler, scheduled-job scheduler, stream processor. Off: outbox processor, MCP server, function runner |

Everything is overridable: `E2E_API_PORT`, `E2E_METRICS_PORT`, `E2E_DB_NAME` (must match
`e2e_*`), `E2E_DB_MODE` (`auto`, `shared`, `own`), `E2E_OWN_PG_PORT`, `E2E_STATE_DIR`,
`E2E_CONTROL_PLANE_URL` (default `http://localhost:8090`; the login client's redirect URI is
`<it>/auth/callback`), `E2E_SKIP_BUILD=1`.

## Seeded data

| What | Value |
|---|---|
| Anchor admin | `admin@e2e-camel.test`, platform super-admin, also holds `ingestor:operator`. Password: `FC_E2E_ADMIN_PASSWORD` in `tmp/e2e-platform/e2e.env` (generated) |
| Client | `acme` (identifier), id in `FC_E2E_ACME_CLIENT_ID`; the `ingestor` application is enabled for it |
| CLIENT-tier user | `operator@acme.test`, home client acme, holds `ingestor:operator`. Password: `FC_E2E_OPERATOR_PASSWORD` |
| Application | `ingestor`, role `ingestor:operator` granting `ingestor:admin:ingestion:manage` |
| Login client | PUBLIC, PKCE, linked to `ingestor` (`provision-login-client`); redirect URIs `http://localhost:8090/auth/callback` and `http://127.0.0.1:8090/auth/callback`. Id in `AUTH_CLIENT_ID` |
| Runtime credentials | the `ingestor` application's own service account (`provision-service-account`: role `platform:application-service`, which carries `platform:messaging:batch:dispatch-jobs-write`), widened to **all applications**; ANCHOR reach, so every client. Its webhook secret signs `ingestor:*` deliveries. `FLOWCATALYST_CLIENT_ID` / `FLOWCATALYST_CLIENT_SECRET` |
| Dispatch pool | `ingest-default` (platform-wide, concurrency 10), id in `FC_E2E_DISPATCH_POOL_ID` |

No event types or subscriptions: the runtime creates `TASK` dispatch jobs directly through
`POST /api/dispatch-jobs/batch`, and the control plane only signs people in.

## Pointing the Camel apps at it

Load the generated file into the app's environment (`set -a; . tmp/e2e-platform/e2e.env; set +a`)
or copy the values. The keys the framework reads:

**ingest-control-plane** (`AuthModule`; plain http needs `APP_ENV=local` or `test`):

```sh
APP_ENV=local
AUTH_ISSUER=http://localhost:18080          # auth.issuer: the platform base URL = the tokens' iss
AUTH_CLIENT_ID=<AUTH_CLIENT_ID from e2e.env> # auth.client-id; PUBLIC client, so no AUTH_CLIENT_SECRET
AUTH_PUBLIC_BASE_URL=http://localhost:8090  # auth.public-base-url; must match the registered redirect URI
```

Sign in at `http://localhost:8090/auth/login` as either user. The id_token carries
`roles: ["ingestor:operator"]` (roles are narrowed to the login client's application), which the
control plane maps to `ingestor:admin:ingestion:manage`. The admin's token says
`tier ANCHOR, clients ["*"]`; the acme operator's says `tier CLIENT, clients ["clt_…:acme"]`.

**ingest-runtime** (`FlowCatalystModule`):

```sh
FLOWCATALYST_BASE_URL=http://localhost:18080   # flowcatalyst.base-url (FlowCatalystModule.BASE_URL)
FLOWCATALYST_APPLICATION=ingestor              # flowcatalyst.application
FLOWCATALYST_CLIENT_ID=<from e2e.env>          # flowcatalyst.client-id
FLOWCATALYST_CLIENT_SECRET=<from e2e.env>      # flowcatalyst.client-secret
```

**Ingestions** written to the control plane: `clientId` = `FC_E2E_ACME_CLIENT_ID` (the `clt_`
id: the batch endpoint takes `clientId` as an id, and only `clientCode` as the identifier `acme`),
`target.dispatchPoolId` = `FC_E2E_DISPATCH_POOL_ID`, `target.code` any
`application:subdomain:aggregate:action`. A `FlowCatalystToken` HTTP source accepts a token from
`POST http://localhost:18080/oauth/token` (`grant_type=client_credentials`) by a service account
that may act for the ingestion's client.

The control plane also needs its own Postgres database and its runtime/admin tokens; those are
the application's, not the platform's. Do not point it at `e2e_camel`.

## Checking a delivery

The runtime's jobs appear in the admin SPA (`http://localhost:18080`, Dispatch Jobs) and through
`GET /api/dispatch-jobs/{id}` (with the admin's session cookie from `POST /auth/login`). `verify` shows the whole path: it
creates a job as the runtime, runs a throwaway receiver on `127.0.0.1:18099`, and waits for
`COMPLETED` with a signed delivery.

## Isolation

The owner's ruling (2026-10-14) was a separate schema in the embedded Postgres, falling back to a
separate embedded instance. A schema does not work, so this uses a **separate database in the
same cluster** (the cluster already holds several):

| Concern | Schema in `flowcatalyst` | Separate database |
|---|---|---|
| Migration bootstrap | **Broken.** `internal/migrate` probes `information_schema.tables` by name across every schema. With `search_path=e2e_camel` it sees the owner's `public.goose_db_version`, then fails selecting it unqualified; with `public` also on the path it reads the owner's ledger as its own, skips every migration and the platform then reads and writes the owner's tables | Each database has its own catalog: migrations run clean |
| Advisory locks (`pg_advisory_xact_lock`, last-admin guard) | shared key space, would serialise against the owner | keyed per database |
| `LISTEN/NOTIFY fng_desired` | would cross-wake the owner's function control plane | per database |
| Extensions, types | none created by the migrations | n/a |
| Roles | none created; both connect as `postgres` | same |
| Connections | shared `max_connections` (100) | shared; capped with `pool_max_conns=8` and `FC_SCHEDULER_DB_MAX_CONNECTIONS=4` (about 11 in use) |
| Redis / NATS | not used (standby off, Postgres broker) | not used |

The script never starts, stops or alters the cluster on 15432; it runs `CREATE DATABASE e2e_camel`
and, on `drop`, `DROP DATABASE e2e_camel WITH (FORCE)`, the name guarded to `e2e_*`. When nothing
answers on 15432 at first `up`, it initialises its own cluster in the state dir (binaries from
fcdev's cache, loopback TCP only) on 25432 and stays in that mode until `drop`.

Everything else the process writes stays in the state dir: it runs with `env -i` (no inherited
`FC_*`), `HOME` and `XDG_*` pointed at `tmp/e2e-platform/home` (fcdev otherwise writes the MCP
credential file into the shared `~/Library/Caches/flowcatalyst-dev/`), its own PID file, JWT
signing key and app key. Listeners: 18080 and 19090 only; the function runner (8095/8096), MCP
(8090) and the outbox admin API are off.

## Caveats

- **New clients and pools after start-up**: the router reads its configuration (which client
  queues to consume) at start and every 300 s. `up` reloads it after seeding; after creating a
  client yourself, `POST /router/config/reload` as the admin, or wait up to 5 minutes, or jobs
  for that client sit `QUEUED`.
- **Signing**: deliveries are signed by the target code's application's service account. Only
  `ingestor` has one here, so jobs with other applications' codes are delivered unsigned (the
  attempt says why). Create and provision that application if a receiver verifies signatures.
- **Dev conveniences stay on**: it is `fcdev`, so the `X-FC-Test-Principal` header is honoured
  and the listeners bind every interface, as on the developer's own instance.
- **The admin SPA** is whatever `frontend/dist` held at build time (embedded); run
  `make frontend` first if the UI matters.
- No post-logout redirect URI is registered on the login client; add one
  (`PUT /api/oauth-clients/{id}`) if the control plane uses single logout.
- `verify` leaves its completed test jobs (`ingestor:e2e:check:ping`) in the database. The set-up
  session also left one `FAILED` job, `orders:sales:order:import` (a check that the runtime may
  create jobs for another application; its target did not exist).

## Currently running

Started 2026-10-14 by the e2e set-up session (shared mode): pid `27814`, API `18080`, metrics
`19090`, database `e2e_camel` on `127.0.0.1:15432`. `verify`: 16/16 passed. Check with
`make e2e-platform CMD=status`; this note goes stale once it is stopped or restarted.
