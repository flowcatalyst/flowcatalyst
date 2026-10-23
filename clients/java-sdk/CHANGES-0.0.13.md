# Java SDK 0.0.13 (unreleased)

Changes since `java-sdk/v0.0.12`. Nothing is tagged or released: `VERSION` and `pom.xml` stay at
0.0.12 in git, and `make release-java-sdk BUMP=patch` cuts 0.0.13 when the owner decides. Until
then a local build is installed under a separate version (below).

Sources of truth: `api/openapi.lock.json`, `docs/platform-bug-fixes.md` (F13, F14, F15),
`docs/wire-contract.md`, the framework's `docs/contracts/write-path.md` section 7a and its
conformance vectors (`flowcatalyst-framework/conformance/outbox/outbox.json`,
`conformance/sql/outbox/insert.sql`).

## Local build for the Camel application

```
io.flowcatalyst:flowcatalyst-sdk:0.0.13-local
~/.m2/repository/io/flowcatalyst/flowcatalyst-sdk/0.0.13-local/flowcatalyst-sdk-0.0.13-local.jar
```

Built from this tree with the version changed only in a temporary copy (`mvn -o install`, tests
run, 123 tests). To rebuild it:

```
tmp=$(mktemp -d) && tar -C clients/java-sdk --exclude=./target -cf - . | tar -C "$tmp" -xf - \
  && sed -i '' 's#<version>0.0.12</version>#<version>0.0.13-local</version>#' "$tmp/pom.xml" \
  && (cd "$tmp" && mvn -o install)
```

## Outbox: dispatch-job rows the platform accepts

- **`metadata` is the platform's array** `[{"key":"…","value":"…"}]`, in insertion order. The SDK
  wrote an object, which `POST /api/dispatch-jobs/batch` refuses for the whole batch
  (`400 INVALID_JSON`); every 0.0.12 dispatch-job row with metadata failed terminally.
- **Members the batch endpoint ignores are no longer written**: `headers`, `retryStrategy`,
  `scheduledFor`, `expiresAt`, `idempotencyKey`, `connectionId`. Their `with…` methods stay,
  `@Deprecated`, so 0.0.12 code compiles.
- **The job id is in the payload and is the row id** (`withId`, or a generated 13-character
  TSID). A batch the outbox processor sends again creates a job with the same id, so a receiver
  that runs a job once per id runs it once. `createDispatchJob` returns that id.
- **A client by its identifier**: `withClientCode("acme")` (trimmed), resolved by the platform
  (F14): a code the caller does not reach is `403`, an unknown one `400` for an anchor and `403`
  otherwise, either refusing the whole batch. `withPlatformClientId(id)` sends `clientId`. Setting
  both is refused locally.
- **Validated before it is written** (`validate()`, write-path 7a, the vector's messages): code
  `{application}:{rest}` with a lower-case application segment, at most 255; `targetUrl` absolute
  `http(s)` with a host and no user info; ids `[0-9A-Za-z_-]{1,17}`; `sequence`,
  `timeoutSeconds`, `maxRetries` at least 1 (the platform turns 0 into its default); `source`,
  `subject`, `externalId`, `messageGroup`, `descriptor` 1 to 255; `queue` `DEFAULT` or
  `HIGH_PRIORITY` (ignoring case, written upper-case); at most 100 metadata entries with
  non-blank keys; `payloadContentType` 1 to 100. An invalid job throws
  `IllegalArgumentException` and nothing is written (a batch call writes nothing).
- **New entry point `CreateDispatchJobDto.of(code, targetUrl)`** with the platform's defaults, the
  shape the conformance vectors pin: kind `TASK`, source the code's application segment, payload
  `{}` as `application/json`, `dataOnly` false, `sequence`/`timeoutSeconds`/`maxRetries`/`mode`
  left to the platform (99, 30, 3, `NEXT_ON_ERROR`). New builders: `withId`, `withKind`
  (`Kind.EVENT|TASK`), `withSource`, `withPayload(text, contentType)`, `withJsonPayload(value)`,
  `withPlatformClientId`, `withClientCode`, `withSubscriptionId`, `withServiceAccountId`,
  `withDispatchPoolId`, `withDescriptor`, `withMetadata(key, value)`; `toBatchItem()` returns the
  validated item for a direct call.
- **`create(source, code, targetUrl, payload, dispatchPoolId)` keeps its 0.0.12 behaviour**:
  four-segment code, `dataOnly` true, `timeoutSeconds` 30, `maxRetries` 5, no `kind` (the
  platform's `EVENT`). Changed only where 0.0.12 wrote a row the platform could never store: a
  null `source` now defaults to the application segment and a null `payload` to `{}` (both
  `NOT NULL` on the platform), and the validation above applies.
- **`client.dispatchJobs().createBatch(jobs)`** (new `DispatchJobsResource`):
  `POST /api/dispatch-jobs/batch` directly, for jobs that need not be atomic with a database
  write; returns the positional `{id, status, error}` results; a whole-batch refusal is a
  `FlowCatalystException`; at most 1000 jobs.

## Outbox: events and audit rows

- **`deduplicationId` defaults to the row id** when not set (the vectors always carry one), so a
  re-sent batch does not create a second event. A given one is kept.
- **A blank `source` is left out**; the platform defaults it to the type's
  `{application}:{subdomain}` (F14). `null` was already left out.
- **`data` / `operationData` stay one encoded JSON string**, byte for byte what the framework's
  writer and the vectors write (the framework's `OutboxCompatibilityTest` pins that text). The
  platform unwraps such a string at ingest (F13), so new and old rows store the object. Writing
  the value itself (as the Go SDK does) was not done: it would break that byte equality.
- **`OutboxManager(driver, clientId, InstantSource clock)`**: the clock dates each row and an
  audit entry without its own `performedAt`. The two-argument constructor uses the system clock.
- **`JdbcOutboxDriver.insertSql()`**: the statement, now byte for byte the framework's
  `insert.sql` (the trailing newline is gone).
- Old rows already in `outbox_messages` are unaffected and still accepted (the platform reads
  both the string and the object form; only the 0.0.12 dispatch-job rows with object
  `metadata` were never accepted, and still are not).

## Sync

- **`SyncResult.Category.Synced` gains `skippedRoles()` and `passwordHashIgnored()`** (F13,
  F15): the principal sync's role names that are not roles (not assigned; the rest applied) and
  the emails of existing users whose `passwordHash` was not applied. Empty lists when the
  platform omits them; always empty for other categories. The five-argument constructor stays.
  `syncedCodes` is never null now.
- **`Definitions.Principal` gains `passwordHash`** (`withPasswordHash`), sent on the wire; stored
  only on a user the sync creates. The four-argument constructor stays. The doc no longer claims
  the platform prefixes role names: roles are full names (`orders:admin`); a bare `admin` is
  skipped and reported.
- **Documentation pages**: `Definitions.Doc(slug, title, content)`, `DefinitionSet.withDocs`,
  `SyncCategory.DOCS`, `SyncResult.docs()`; `POST /api/applications/{app}/docs/sync` (no
  `removeUnlisted`: each sync replaces the application's whole page set). Skipped when a set
  declares no pages; duplicate slugs across merged sets fail locally. `SyncResult` keeps its
  ten-argument constructor (`docs` = `SKIPPED`).
- **Dispatch pools**: `withRateLimit` / `withConcurrency` refuse values below 1 (the platform
  refuses them on sync); `withoutRateLimit()`. Docs corrected: an omitted `rateLimit` removes an
  existing pool's limit (no limit), an omitted `concurrency` sets 10; there is no default of 100.
- **`SubscriptionMode.NEXT_ON_ERROR`** added; documented that the application sync accepts
  `mode` but does not apply it.

## Spec

`openapi/openapi.json` refreshed from `api/openapi.lock.json` (identical to `tools/dump-spec`
output). Generated-model changes: `SyncResultResponse.skippedRoles` / `passwordHashIgnored`,
`SyncUsersResponse.passwordHashIgnored`, `CreateEventRequest.source` optional,
`CreateSubscriptionRequest.clientScoped` / `sequence`,
`AssignApplicationAccessRequest.confirmSelfRestriction`, the requeue-by-filter request and
response models, and descriptions.

## Tests

`mvn -o clean verify`: 123 tests, 0 failures (1 skipped: the real-Postgres JDBC test without
`FC_JAVA_SDK_TEST_PG_URL`). New:

- `outbox/OutboxConformanceTest`: the framework's vectors (copied into
  `src/test/resources/conformance`) for every row the SDK writes: every column, the payload text
  byte for byte and `payload_size`, the `INSERT` statement, the eleven refused dispatch jobs with
  their messages; and, when the framework checkout is beside this repo (or `FC_FRAMEWORK_DIR`
  names it), that the copies are the framework's files. One allowed difference: an event's row
  id is the SDK's own TSID (the framework uses the event id; the SDK carries it as
  `deduplicationId`), since the framework's `OutboxCompatibilityTest` writes both rows into one
  table.
- `outbox/DispatchJobPayloadTest`, `outbox/EventPayloadTest`, `sync/PlatformSyncAlignmentTest`,
  `resources/DispatchJobsResourceTest`.

Also run against the local build, from `flowcatalyst-framework` with
`-Dflowcatalyst-sdk.version=0.0.13-local`: `OutboxCompatibilityTest`, `OutboxConformanceTest`,
`DispatchJobOutboxTest`, `DefinitionSyncTest`, `SyncCommandTest`, `PlatformApiCallTest`,
`FlowCatalystModuleTest`, `ScheduledJobReceiverTest` (81 tests, all pass; the framework compiles
unchanged against it).

## Migration note for the Camel application (inhance mono-repo)

The ingest apps reach this SDK only through the framework (`framework-flowcatalyst` depends on
it; `framework-bom` pins `flowcatalyst-sdk` 0.0.11). To use the local build, add to the root
`pom.xml` `<dependencyManagement>` (a directly managed version wins over the imported BOM):

```xml
<dependency>
  <groupId>io.flowcatalyst</groupId>
  <artifactId>flowcatalyst-sdk</artifactId>
  <version>0.0.13-local</version>
</dependency>
```

Every API the framework uses from 0.0.11/0.0.12 is still there with the same signatures, so the
installed framework jars need no rebuild.

`ingest-runtime`'s `PlatformDispatchJobSink` builds `POST /api/dispatch-jobs/batch` items by hand.
It can build them with the SDK instead and keep its own `ServiceClient` call (the framework's
`PlatformClient` does not expose its `FlowCatalystClient`):

```java
Map<String, Object> item = CreateDispatchJobDto.of(job.code(), job.targetUrl())
        .withPlatformClientId(job.clientId())      // or .withClientCode(...) if it holds the identifier
        .withDispatchPoolId(job.dispatchPoolId())
        .withMode(CreateDispatchJobDto.DispatchMode.valueOf(job.mode().name()))
        .withMessageGroup(job.messageGroup())
        .withSequence(job.sequence())               // at least 1, as the sink already ensures
        .withCorrelationId(job.correlationId())
        .withPayload(job.payload(), job.payloadContentType())
        .withDataOnly(true)                         // what the sink sends today
        .toBatchItem();                              // validated; throws IllegalArgumentException
```

That keeps today's wire shape (kind `TASK`, `dataOnly` true) and adds local validation (a code
must be `{application}:{rest}` with a lower-case application segment; ids at most 17
characters). Decide which client field to use: the sink sends `clientId`, which the platform
stores as given after checking the caller reaches that client id (`403 No access to client`
otherwise); if `Ingestion.clientId` is the client's identifier rather than its platform id, send
it as `clientCode` instead and the platform resolves it (F14). A sink that should write the job
in the same transaction as its own state can use `OutboxManager.createDispatchJob` with the same
DTO.

## The mirror in flowcatalyst-javalin

`clients/java-sdk` here is the canonical source. `flowcatalyst-javalin/sdk` was byte-identical to
0.0.12 apart from `VERSION` (0.0.10 there), `pom.xml` `<version>` (0.0.1-SNAPSHOT there) and
`openapi/openapi.json` (each repo vendors its own server's lock; the drift script allows that one).
This change is not copied there, so its `tools/sdk-drift.sh` reports drift until the directory is
copied across (everything but `openapi/openapi.json`; `VERSION` and `pom.xml` already drift).

## Platform config: reveal a SECRET

- New `client.platformConfig()` (`PlatformConfigResource`): `list`, `get`, `set`, `delete`, and
  `revealSecret(app, section, property, clientId)` for `GET /api/config/{app}/{section}/{property}/secret`.
- `get`/`list`/`set` always return `"***"` for `valueType` SECRET; `revealSecret` is the only way to
  read plaintext. Needs `platform:admin:config:reveal` or the application's own service account.
  403 is `SdkError.Forbidden`, 404 `NotFound`, 400 `NOT_A_SECRET` is `HttpStatus` (status 400).
- Every reveal is audited server-side. The SDK does not cache it or log it, and `RevealedSecret.toString()` masks the value.
- `openapi/openapi.json` and `openapi.admin.json` refreshed from the current spec (models regenerate at build).
