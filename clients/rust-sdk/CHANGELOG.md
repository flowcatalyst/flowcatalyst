# Changelog

All notable changes to `flowcatalyst-sdk` (Rust). Versions are tagged
`rust-sdk/vX.Y.Z` in the platform repository once a release line exists.

## Unreleased (0.1.0)

First version, seeded from `flowcatalyst-rust/crates/fc-sdk` with the wire
shapes fixed to the platform contract. The README lists every change from
fc-sdk and the wire-contract ambiguities found. Summary:

### Platform config
- `platform_config()` resource: `list`, `get`, `set`, `delete` and
  `reveal_secret(app, section, property, client_id)`. `SECRET` properties come
  back as `"***"` from list/get/set; `reveal_secret` (`GET
  /api/config/{app}/{section}/{property}/secret`) is the only way to read the
  plaintext. It is audited server-side, never cached by the SDK, and returns a
  `SecretValue` that prints `***` in `Debug`/`Display` (`expose()` reads it).
  Errors: 403, 404, 400 `NOT_A_SECRET`.

### Outbox
- `outbox_messages` DDL is the Java SDK's migration byte for byte (PostgreSQL
  and MySQL); payload bound as `TEXT`, `status` as `SMALLINT`.
- Dispatch jobs are the platform's batch item: `metadata` as a key/value
  array, ignored fields never written, numbers only when set (at least 1),
  validated builder (`DispatchJob::builder`), job id = row id.
- Events in the Java SDK's key order with `clientCode`; four-segment type
  required; `data` must be an object.
- Audit entries require `principalId`; `applicationCode`, `clientCode` added;
  operation data redacted with the shared vectors.
- `OutboxManager::prepare_*` + `PgOutboxDriver::insert_in` /
  `MySqlOutboxDriver::insert_in` for the caller's transaction.

### Webhooks
- Java SDK rules: fail closed on an empty secret, case-insensitive hex, digits
  only for Unix seconds, 300 s / 60 s window, optional bearer gate; Java's
  refusal codes. `MediationResponse`, `parse_client_header`.

### TSIDs
- OS randomness; 65-bit strings refused; `Tsid` newtype.

### Client
- `TokenSource`: `StaticToken`, `ClientCredentials` (cached, single-flight,
  refreshed once on `401`).
- Retries on `408/429/502/503/504` and connection failures with backoff.
- `ClientError::Api` carries the `{error, message}` envelope.
- Path segments percent-encoded.
- `connections().create/pause/activate` return the record; app-owned
  connections (`applicationCode`); connections and docs sync endpoints;
  application `website`/`logo`/`logoMimeType`/`hasLoginClient`.
- Removed `applications().update_client_config` (no such route on the Go
  platform) and `event_types().archive` (sent a hard delete).

### Definition sync
- Category order: roles, event types, dispatch pools, connections,
  subscriptions, processes, scheduled jobs, principals, OpenAPI, docs (pools
  before subscriptions so a new pool reference resolves on the first sync).
- New connections and docs categories; subscriptions carry `connectionCode`,
  `sharedConnection` and the request `clientId`; subscriptions are not sent
  when the connections category failed.
- Local checks before any request (the category fails, naming the values):
  duplicate codes/names, event-type codes (four segments, first = the
  application), role permissions outside the application (also lowercased),
  subscription targets (absolute http(s), path targets resolved against
  `with_subscription_target_base_url`), `sharedConnection` without
  `connectionCode`, scheduled-job crons that are not 6-field seconds-first,
  docs slugs.
- Scheduled jobs grouped per `client_id`, one request per group;
  `synced_codes` excludes archived codes.
- Principal role names without `app:` are prefixed with the application
  (the Go platform stores names verbatim).
- `SyncResult` is `#[must_use]`; `into_result()` turns any failed category
  into a `DefinitionSyncError` carrying the whole result.

### Auth
- `aud` string or array; `nbf`, `jti`, `name` optional; `azp`.
- Unknown `kid`: one JWKS refetch, then refused (no fallback key).
- Full percent-encoding of OAuth query parameters; secrets redacted in
  `Debug`; `tracing` and `rand` dependencies dropped.
