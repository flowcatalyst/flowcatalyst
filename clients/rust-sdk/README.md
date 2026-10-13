# flowcatalyst-sdk (Rust)

The Rust member of the FlowCatalyst SDK family, beside the Go SDK
(`pkg/fcsdk`), the Java SDK (`clients/java-sdk`), TypeScript and Laravel. It
writes the same outbox rows, the same event, dispatch-job and audit payloads,
the same TSIDs, and verifies the same delivery signatures. The wire authority
is the platform: `api/openapi.lock.json`, `docs/wire-contract.md` and the Go
handlers. Stable Rust, edition 2024, MSRV 1.98 (toolchain 1.98.1), Apache-2.0.

Status: **not published** (`publish = false`, version 0.1.0). See
`docs/rust-sdk.md` in this repository for how it will be released.

## Features

| Feature | Default | What |
|---|---|---|
| (always) | | `tsid`: time-sorted ids, `Tsid` newtype, entity prefixes |
| `outbox` | yes | `CreateEventDto`, `DispatchJob`, `CreateAuditLogDto`, `OutboxManager`, the canonical `outbox_messages` DDL |
| `outbox-postgres` | | `PgOutboxDriver` (sqlx) |
| `outbox-mysql` | | `MySqlOutboxDriver` (sqlx) |
| `webhook` | yes | `WebhookVerifier`, `sign`, `MediationResponse`, `X-FlowCatalyst-Client` parsing |
| `client` | yes | `FlowCatalystClient` (platform HTTP API), `ClientCredentials`, `sync` (definition sync) |
| `auth` | | access-token claims, JWKS (RS256) and HS256 validation, OAuth authorization-code client |

Dependencies are deliberately few: serde, serde_json, thiserror, getrandom
always; chrono and url for the outbox; reqwest (rustls only) and tokio
(`sync`, `time`) for the client; sha2 for webhooks; sqlx only with a driver
feature (no TLS and no runtime choice beyond tokio: your own sqlx dependency
decides); jsonwebtoken and base64 only with `auth`.

## Quick start

```rust
use flowcatalyst_sdk::outbox::{CreateEventDto, DispatchJob, OutboxManager, PgOutboxDriver};

let driver = PgOutboxDriver::new(pool.clone());
let outbox = OutboxManager::new(driver.clone(), "clt_0HZXEQ5Y8JY5Z")?;

let event = CreateEventDto::new("shop:orders:order:placed", &order_placed)?
    .message_group(format!("orders:order:{id}"))
    .client_code("acme");
let job = DispatchJob::builder("shop:fulfilment:pick", "https://warehouse.example.com/hooks/pick")
    .json_payload(&pick)?
    .build()?;

let mut tx = pool.begin().await?;
// ... business writes on &mut *tx ...
driver.insert_in(&mut tx, &[outbox.prepare_event(&event)?, outbox.prepare_dispatch_job(&job)?]).await?;
tx.commit().await?;
```

```rust
use flowcatalyst_sdk::client::{ClientCredentials, FlowCatalystClient};

let client = FlowCatalystClient::new("https://platform.example.com")
    .with_token_source(ClientCredentials::new("https://platform.example.com", "svc-orders", secret));
let pools = client.dispatch_pools().list(&Default::default()).await?;
```

```rust
use flowcatalyst_sdk::webhook::{WebhookVerifier, SIGNATURE_HEADER, TIMESTAMP_HEADER};

let verifier = WebhookVerifier::from_env()?; // FLOWCATALYST_SIGNING_SECRET
verifier.verify(&raw_body, headers.get(SIGNATURE_HEADER), headers.get(TIMESTAMP_HEADER))?;
```

Runnable examples: `list-event-types`, `fc-sync` (feature `client`),
`outbox-postgres` (feature `outbox-postgres`), `webhook-verify` (feature
`webhook`).

## Build, test, lint

```bash
make ci          # fmt --check, clippy -D warnings (all and no features), tests, Go conformance, cargo deny
make test-db-up  # throwaway Postgres 18 (127.0.0.1:55433) and MySQL 8.1 (:55434) containers
make test-db     # the sqlx driver tests against them
make test-db-down
```

Wire compatibility is tested both ways. `tests/fixtures/wire/*.json` are
golden payloads: `tests/wire_fixtures.rs` asserts the SDK produces them byte
for byte, and `conformance/wire_test.go` (run by the platform's `go test
./...`) decodes the same files with the platform's own types (`BatchItem`
strictly, `BatchEventItem`, `AuditBatchItem`), checks every signature vector
with the Go SDK's signer and every TSID vector with the Go SDK's encoder, and
checks that the outbox DDL and the redaction vectors are the Java and Go SDK
files byte for byte. The full dispatch-job golden is the framework's Java test
string (`DispatchJobOutboxTest`), verbatim.

## What changed from fc-sdk

Seeded from `flowcatalyst-rust/crates/fc-sdk` (Apache-2.0, unpublished). Not
taken: the dependency on `fc-common-types` (its TSID, outbox status and audit
redaction code is copied in, Apache-2.0), the use-case/unit-of-work, cache,
lock, axum and scheduled-job-runner modules (the framework owns those; decision
11 of the framework's Rust design).

Outbox:

1. **Table.** fc-sdk created `payload JSONB`, `TEXT`/`INTEGER` columns and other
   indexes. Now the Java SDK's migration files byte for byte (`migrations/`):
   `VARCHAR(26)` id, `VARCHAR(20)` type, `TEXT` payload, `SMALLINT` status and
   retry count, the processor's partial indexes `idx_outbox_messages_pending`
   (`WHERE status = 0`) and `idx_outbox_messages_stuck` (`WHERE status = 9`),
   and `idx_outbox_client_pending`. `init_*_schema` creates it only when absent.
2. **Insert.** fc-sdk bound the payload as `$4::jsonb`, which re-formats the JSON
   (key order, spacing) so the stored bytes and `payload_size` disagreed with
   what was built. Now bound as text; `status` as `SMALLINT`; `created_at` set
   per row by the SDK (as Java does) instead of one `NOW()` per transaction; a
   batch is one multi-row statement; rows can be written on the caller's
   transaction (`insert_in`); table names are validated as identifiers.
3. **Dispatch jobs** (framework `write-path.md` 7a). fc-sdk wrote `metadata` as an
   object (the batch endpoint answers `400 INVALID_JSON` for the whole batch),
   wrote `headers`, `retryStrategy`, `scheduledFor`, `expiresAt`,
   `idempotencyKey` and `connectionId` (ignored by the platform), always wrote
   `timeoutSeconds`/`maxRetries` (default retries 5 against the platform's 3),
   required a pool and defaulted `dataOnly` to true. Now: the platform's
   `BatchItem` in its key order; `metadata` an array in insertion order;
   ignored fields not offered; the numbers only when set and at least 1; `kind`
   default `TASK`, `dataOnly` default false, payload default `{}`; the job id
   (also the row id) generated as a 13-character TSID unless given; a builder
   that refuses what the platform would refuse terminally (code shape, target
   URL scheme/host/user info, id and column lengths, enums, metadata keys).
4. **Events.** Java's key order and members, plus `clientCode` (missing in
   fc-sdk); the type must have four non-empty segments (Java `QualifiedCode`);
   `data` must be a JSON object.
5. **Audit logs.** `principalId` is required (the platform refuses an entry
   without one, terminally); `applicationCode` and `clientCode` added;
   `performedAt` formatted like Java's `Instant.toString`; operation data
   redacted with the shared vectors before it is written.

Webhooks (now the Java SDK's rules): an empty secret fails closed (fc-sdk
verified with an empty HMAC key); an empty header counts as missing; the hex
signature is compared ignoring case; Unix seconds must be ASCII digits (fc-sdk
accepted `+123`); a timestamp over 64 characters is refused; optional bearer
gate compared as SHA-256 digests; no `unwrap`/`expect` (fc-sdk could panic on
the clock); refusal codes match Java's `ErrorCode`. The shared signature vector
is pinned. Added `MediationResponse` (`ack`, `delaySeconds`, `flushGroup`) and
`X-FlowCatalyst-Client` parsing.

TSIDs: the 10-bit random field and the per-millisecond sequence start came from
the clock XOR a counter in fc-sdk (correlated across processes minting in the
same millisecond); now from the OS CSPRNG, as Go uses `crypto/rand`. A 13-
character string whose first character exceeds `F` (65 bits) is refused instead
of silently wrapping (Go still wraps). A typed id's TSID is the part after the
last `_`. `Tsid` newtype with `Display`/`FromStr`.

Client and tokens: fc-sdk took a static token only, never retried, and returned
the raw error body. Now a `TokenSource` (static, or `ClientCredentials`:
`client_credentials` grant with the credentials in the form body, cached until
60 s before expiry, single-flight, cleared and refetched once on a `401`);
retries with exponential backoff on `408`, `429`, `502`, `503`, `504` and
connection failures (Java's set; not `500`); `Accept: application/json`
always, `Content-Type` only with a body; `ClientError::Api` carries the
platform's `{error, message}` envelope; every caller-supplied path segment is
percent-encoded (fc-sdk interpolated codes raw, so `/`, `?`, `#`, `%` or a space
changed the route). `connections().create/pause/activate` return the record the
platform sends; `CreateConnectionRequest.applicationCode` (app-owned
connections) and the application `website`/`logo`/`hasLoginClient` fields were
added; `applications().update_client_config` (a PUT the Go platform does not
serve) and `event_types().archive` (which sent a hard `DELETE`) were removed.

Auth: `aud` may be an array (first entry, as the Go SDK); `nbf`, `jti` and `name`
may be absent; `azp` added; a token whose `kid` is not in the issuer's key set
triggers one JWKS refetch and is then refused (fc-sdk fell back to the first
RSA key); an empty JWKS is an error; query parameters are fully percent-
encoded; `Debug` never prints a client secret; no `tracing` or `rand`
dependency.

Definition sync (Go platform behaviour confirmed per item): categories run
roles, event types, dispatch pools, connections, subscriptions, processes,
scheduled jobs, principals, OpenAPI, docs (fc-sdk synced pools after
subscriptions, and Go silently drops a `dispatchPoolCode` it cannot resolve
yet). New connections and docs categories, and subscription `connectionCode`,
`sharedConnection` and request `clientId` (code-first connections); if the
connections category fails, subscriptions are not sent. Each category is
checked locally before its request and fails, naming the values, on
duplicates, event-type codes not `{app}:x:y:z`, role permissions outside the
application (Go: `400 PERMISSION_OUTSIDE_APPLICATION`; permissions are
lowercased), subscription targets that are not absolute http(s) URLs (path
targets resolve against `with_subscription_target_base_url`), crons that are
not 6-field seconds-first (fc-sdk documented 5-field crons, which Go accepts
and never fires), and malformed docs slugs. Scheduled jobs are sent per
`client_id` group (Go refuses a platform-scoped sync from a non-anchor caller)
and `synced_codes` no longer includes archived codes. Principal role names
without `app:` are prefixed with the application (Go stores names verbatim,
contrary to fc-sdk's and the Java SDK's docs). `passwordHashIgnored` is read:
the platform stores a `passwordHash` only on a user the sync creates and
reports the others there (Go F13, as Java and Rust). `skippedRoles` is read:
the Go platform assigns no role name that is not a role and reports it there,
the rest of the sync applying (Go F15).
`SyncResult::into_result()` turns any failed category into an error.

## Wire-contract ambiguities found

These are reported, not decided here; the SDK follows the Java SDK (the
framework's reference) where SDKs disagree.

1. ~~**Event `data` and audit `operationData` are JSON strings.**~~ Resolved
   on the platform (Go F13, owner ruling 2026-10-12): `/api/events/batch` and
   `/api/audit-logs/batch` unwrap a string member whose text is an encoded
   JSON object or array, so subscribers, `dataOnly` bodies and the audit view
   get the object; the SDKs keep writing the string. The exact rule is in
   `docs/wire-contract.md` ("Encoded `data` and `operationData` at batch
   ingest").
2. ~~**The Go SDK's outbox payloads do not match the audit endpoint.**~~ Fixed
   (Go F13): `outboxpgx` writes camelCase audit rows with `operationData` as an
   object and an optional `applicationCode`.
3. **Audit `source`, `correlationId` and `metadata`** are written by the Java SDK
   and this one but ignored by `/api/audit-logs/batch`.
4. **Dispatch-job `kind` default.** The platform defaults to `EVENT`; the
   framework ruled `TASK` for jobs an application creates, which this SDK
   follows. The code rule is the framework's `{application}:{rest}`, looser
   than Java's four-segment `QualifiedCode` for events.
5. **`clientId` on dispatch jobs** must be the platform client id (at most 17
   characters); the outbox table's `client_id` is `VARCHAR(26)` while a client
   identifier may be 100 characters (the framework widens it in its own second
   migration). ~~The batch does not resolve a `clientCode`~~: resolved on the
   Go platform (F14, owner ruling 2026-10-12): both dispatch-job routes accept
   `clientCode`; an unknown one is refused (400 `CLIENT_NOT_FOUND` for an
   anchor, 403 otherwise), never stored unscoped. Java and Rust platforms not
   yet.
6. **Webhook timestamp.** `docs/wire-contract.md` describes only the millisecond
   ISO form; every SDK also accepts Unix seconds. The framework contract's
   "Unix seconds must be ASCII digits" applies to that form only.
7. ~~**Principal sync and password hashes.**~~ Fixed (Go F13): the Go platform
   now uses `passwordHash` only to create a user and reports the others in
   `passwordHashIgnored`, as Java (ruling 2026-09-25) and Rust.
8. **Role names in principal sync.** The SDKs' docs said the platform adds the
   `app:` prefix; Go stores names verbatim. This SDK now prefixes bare names;
   the other SDKs still document the old behaviour.
9. **`tests/golden/webhook/`**, named by `docs/wire-contract.md` and the router's
   `signWebhook` comment, does not exist; the shared vector lives in the SDKs'
   tests (now also in `tests/fixtures/wire/webhook-signatures.json`).
10. ~~**An event without `source` is refused.**~~ Resolved on the Go platform
    (F14, owner ruling 2026-10-12): an absent or blank `source` is stored as
    the type's `{application}:{subdomain}` (`event-minimal.json` →
    `orders:sales`), in the batch and the singular route. The Java and Rust
    platforms still refuse it until they mirror the rule.

## Licence

Apache-2.0 (`LICENSE`), like fc-sdk. Note this differs from the Go SDK
(`pkg/fcsdk`, MPL-2.0). `deny.toml` keeps strong-copyleft licences out of the
dependency graph.
