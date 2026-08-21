# Java SDK Plan

Plain-Java client SDK for FlowCatalyst, modeled on the canonical TypeScript SDK
(`clients/typescript-sdk`). No framework integration (explicitly: no Spring
starter). Java 25 baseline.

> **Status (2026-08-21): v1 BUILT — M1–M5 complete.** `clients/java-sdk`
> exists, `make build-java-sdk` is green (34 tests), and the client was
> live-verified against a running platform (client-credentials mint + list).
> Remaining before first release: live write-path round-trip (definition sync
> + outbox poller pickup) against fc-dev, and a distribution-channel decision.
> Deferred modules (usecase envelope, runner/locks, cache stores, PKCE login)
> are tracked below.

## Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Location | `clients/java-sdk` | Sibling of the other two SDKs; releases cut from this repo |
| Java baseline | 25 (LTS) | Records, sealed interfaces, pattern-matching switch, virtual threads |
| Build tool | Maven, single module | Simplest for a library; no multi-module needed without a Spring starter |
| Coordinates | `io.flowcatalyst:flowcatalyst-sdk`, package `io.flowcatalyst.sdk` | Mirrors `@flowcatalyst/*` / `flowcatalyst/laravel-sdk` naming |
| HTTP | `java.net.http.HttpClient` | Zero HTTP deps; blocking API, safe on virtual threads |
| JSON | Jackson (`databind` + `jsr310`) | Only real runtime dependency |
| API style | Synchronous/blocking | Callers scale via virtual threads; no CompletableFuture surface |
| Errors | `sealed interface SdkError` hierarchy, thrown as one `FlowCatalystException` carrying the variant | Mirrors the TS tagged union; exhaustive `switch` handling |
| DTOs | **Generated models only** (openapi-generator, models-only, Jackson) from a vendored `openapi/openapi.json`; hand-written resource layer on top | Same shape as both existing SDKs: spec vendored per-SDK, refreshed by `make sdk-spec`, codegen at build time. 231 schemas is too many to hand-write and keep in sync |
| Declaration | Runtime annotations + **explicit class registration** (caller passes classes); no classpath scanning | Keeps zero-dep; an annotation processor can automate discovery later |

## Scope: mirror of the canonical TS SDK

v1 ports these TS modules:

1. **Core client** — `FlowCatalystClient` with two auth modes
   (client-credentials via `/oauth/token` with single-flight refresh + 60s
   expiry buffer; or caller-supplied token/supplier), retry with exponential
   backoff on 408/429/502/503/504, one-shot refresh on 401.
2. **Resources (15)** — applications, audit-logs, clients, connections,
   dispatch-pools, event-types, me, permissions, principals, processes, roles,
   router, scheduled-jobs, subscriptions (+ index parity with TS exports).
3. **Outbox** — TSID generator (port of TS `tsid.ts`, same wire layout),
   qualified-code helper, `CreateEventDto` / `CreateDispatchJobDto` /
   `CreateAuditLogDto` builders, `OutboxStatus` smallint contract,
   `OutboxDriver` SPI + one reference `JdbcOutboxDriver` (plain
   `java.sql.Connection`, joins the caller's transaction), raw SQL migrations
   copied from the TS SDK (`migrations/postgresql`, `migrations/mysql`).
4. **Declaration + sync** — `@AsEventType`, `@AsSubscription`,
   `@AsDispatchPool`, `@AsRole` annotations; `DefinitionSynchronizer` posting
   the app-scoped endpoints `POST /api/applications/{appCode}/{kind}/sync`
   (with `removeUnlisted`), plus the programmatic `SyncDefinitionSet` path that
   skips annotations entirely.
5. **Webhook** — HMAC-SHA256 firing verification (port of TS
   `webhook/signature.ts`: `X-FlowCatalyst-Signature`/`-Timestamp`,
   timestamp-dot-payload, constant-time compare, tolerance window).

Deferred (explicitly out of v1, tracked for later):

- **Usecase envelope** (`usecase/` — Operation/Plan/OutboxUnitOfWork). Port
  once the Go `usecaseop` migration settles; the TS/PHP ports are themselves
  still pending, so Java should follow the settled contract, not chase it.
- **Scheduled-job runner + lock providers** (`runner/`).
- **Cache stores** (`cache/`).
- OIDC authorization-code/PKCE login flow (Laravel-only concern today).
- Framework adapters (TS `fastify/`, `effect/` have no plain-Java analogue —
  intentionally nothing here).

## Layout

```
clients/java-sdk/
  pom.xml
  openapi/openapi.json          # vendored, refreshed by `make sdk-spec`
  src/main/java/io/flowcatalyst/sdk/
    FlowCatalystClient.java     # entry point, config record, resource accessors
    auth/                       # TokenProvider, ClientCredentialsTokenManager
    error/                      # sealed SdkError + FlowCatalystException
    http/                       # transport: retry, backoff, JSON codec
    resources/                  # 15 hand-written resource classes
    outbox/                     # manager, DTO builders, driver SPI, JdbcOutboxDriver
    tsid/                       # Tsid, QualifiedCode
    annotations/                # @AsEventType, @AsSubscription, @AsDispatchPool, @AsRole
    sync/                       # DefinitionSynchronizer, SyncDefinitionSet, results
    webhook/                    # WebhookSignature
  src/main/java/.../generated/  # openapi-generator models-only output (git-ignored, built)
  migrations/{postgresql,mysql}/
  examples/                     # list-event-types, order-service (outbox in a tx)
  src/test/java/                # JUnit 5; stub server on jdk httpserver
```

## Milestones

- **M1 — scaffold + codegen.** Maven project (release 25), vendored spec,
  openapi-generator models-only wired into the build, `make sdk-spec` extended
  to also write `clients/java-sdk/openapi/openapi.json`, `make build-java-sdk`.
- **M2 — core client.** Auth, retry, sealed errors, transport, the 15
  resources typed against generated models. Stub-server tests per resource
  family (happy path + error mapping).
- **M3 — outbox.** TSID port with the TS test vectors, DTO builders,
  driver SPI + JDBC reference driver, migrations. Unit tests against embedded
  PG (reuse the repo's embedded-PG dev loop) for the driver.
- **M4 — declaration + sync.** Annotations, synchronizer, programmatic set;
  round-trip test against a live fc-dev instance.
- **M5 — polish + release.** Webhook verification, two runnable examples,
  README, `scripts/release.sh java` (tags `java-sdk/vX.Y.Z`). Distribution
  channel (Maven Central vs GitHub Packages vs JitPack) decided at this point —
  scaffolding does not block on it.

## Verification

- Unit: JUnit 5, jdk `com.sun.net.httpserver` stubs (no WireMock dep).
- TSID: byte-for-byte parity with TS `tsid.ts` outputs (shared test vectors).
- HMAC: shared vectors with TS `webhook/signature.ts`.
- Live e2e: examples run against local fc-dev (owner runs long-lived daemons;
  SDK e2e uses short-lived fc-dev with overridden ports).
- Wire drift: build regenerates models from the vendored spec; `make sdk-spec`
  refresh + rebuild surfaces breaking changes at compile time in the
  hand-written resource layer.
