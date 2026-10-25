# FlowCatalyst Java SDK

Plain-Java client SDK for the FlowCatalyst platform. Java 25+, blocking API
(scale with virtual threads), Jackson as the only runtime dependency. Mirrors
the TypeScript SDK's surface: control-plane resources, transactional outbox,
declaration sync, and webhook signature verification.

```java
var client = FlowCatalystClient.builder()
        .baseUrl("https://your-instance.flowcatalyst.io")
        .clientCredentials("oac_your_client_id", "your_client_secret")
        .build();

var eventTypes = client.eventTypes().list(null);
```

This SDK is standalone: its `pom.xml` has no parent and no dependency on any
other FlowCatalyst module, so the directory builds on its own
(`mvn test` in this directory). It is mirrored byte-for-byte between
`flowcatalyst-go` (`clients/java-sdk`) and `flowcatalyst-javalin` (`sdk`),
apart from `openapi/openapi.json`; `tools/sdk-drift.sh` in the Javalin repo
checks they agree. The use-case framework (`Operation`, `UnitOfWork`,
`Result`) is a separate module, `flowcatalyst-usecase`, which builds on this
SDK and is not part of it.

## Modules

| Package | What it does |
|---|---|
| `io.flowcatalyst.sdk` | `FlowCatalystClient` — builder config, resource accessors |
| `…sdk.resources` | 18 typed resource families (ingestions, ingestion sets, event types, subscriptions, dispatch pools, dispatch jobs, connections, roles, permissions, applications, clients, principals, processes, scheduled jobs, audit logs, me, router) |
| `…sdk.error` | `sealed interface SdkError` + `FlowCatalystException` — handle failures with a pattern-matching `switch` |
| `…sdk.outbox` | Transactional outbox: `OutboxManager`, DTO builders, `OutboxDriver` SPI + `JdbcOutboxDriver`, raw SQL migrations in `migrations/` |
| `…sdk.tsid` | TSID generation (13-char Crockford Base32, platform-compatible; collision-free monotonic sequence) |
| `…sdk.sync` | `DefinitionSynchronizer` + `DefinitionSet` — bulk-sync roles / event types / connections / subscriptions / dispatch pools / principals / processes / scheduled jobs / OpenAPI / documentation pages per application, optionally scoped to one client (`forClient`) and merged across sets (`syncGrouped`) |
| `…sdk.annotations` | `@AsEventType` / `@AsConnection` / `@AsSubscription` (with `connectionCode` / `sharedConnection` / `client`) / `@AsDispatchPool` / `@AsRole` + `DefinitionScanner` (explicit class registration — no classpath scanning) |
| `…sdk.webhook` | `WebhookSignature.verify(...)` — HMAC-SHA256 verification of signed deliveries |

## Auth modes

- **Client credentials** (service account): `.clientCredentials(id, secret)` —
  tokens are cached with a 60s expiry buffer, refreshed single-flight, and
  transparently refreshed once on a 401.
- **User token**: `.accessToken(String)` or `.accessToken(Supplier<String>)` —
  the host app owns refresh; 401s are surfaced, not retried.

Transient statuses (408/429/502/503/504) retry with exponential backoff
(default 3 attempts, 100ms base delay).

## Error handling

Every failure is a `FlowCatalystException` carrying one `SdkError` variant:

```java
try {
    client.eventTypes().get(id);
} catch (FlowCatalystException e) {
    switch (e.error()) {
        case SdkError.NotFound nf -> handleMissing();
        case SdkError.RateLimited rl -> backOff(rl.retryAfter());
        case SdkError.Validation v -> log(v.errors());
        default -> throw e;
    }
}
```

## Transactional outbox

Events are not published over HTTP — they are written to your database's
`outbox_messages` table inside your own transaction (migrations in
`migrations/postgresql` and `migrations/mysql`), and the outbox poller ships
them:

```java
var driver = new JdbcOutboxDriver(dataSource);
var outbox = new OutboxManager(driver, "clt_your_client_tsid");

driver.withTransaction(tx -> {
    // business writes on (Connection) tx ...
    outbox.createEvent(CreateEventDto
            .create("orders:sales:order:placed", Map.of("orderId", orderId))
            .withMessageGroup(orderId), tx);
    return null;
});
```

Event `type`s must be fully qualified `application:subdomain:aggregate:action`
strings — the SDK rejects bare codes. An event without a `source` (or with a
blank one) leaves it out and the platform uses the type's
`application:subdomain`; an event without a `deduplicationId` carries its row
id, so a batch the outbox processor sends again does not create a second event.
`data` and audit `operationData` are written as one encoded JSON string, as the
framework's writer does; the platform stores the value it holds.

### Dispatch jobs

A dispatch job row is one item of `POST /api/dispatch-jobs/batch`, checked by
`validate()` before it is written (a job the platform would refuse is never
stored). `CreateDispatchJobDto.of(code, targetUrl)` uses the platform's
defaults (kind `TASK`, source the code's application segment, payload `{}`,
`dataOnly` false, sequence/timeout/retries left to the platform);
`create(source, code, targetUrl, payload, dispatchPoolId)` keeps the 0.0.12
defaults (`dataOnly` true, 30 s, 5 retries, a four-segment code).

```java
outbox.createDispatchJob(CreateDispatchJobDto
        .of("ingest:orders:record", "https://api.example.com/records")
        .withId(recordId)                  // the row id and the job id; generated when unset
        .withClientCode("acme")            // the client's identifier; or withPlatformClientId(id)
        .withPayload(line, "text/csv")
        .withDataOnly(true)
        .withMessageGroup(fileId)
        .withSequence(lineNumber)          // at least 1
        .withMetadata("source", "sftp"), tx);
```

`metadata` is written as the platform's `[{"key","value"}]` array, and the
members the batch endpoint ignores (`headers`, `retryStrategy`,
`scheduledFor`, `expiresAt`, `idempotencyKey`, `connectionId`) are not written.
An unknown `clientCode` refuses the whole batch (400 for an anchor, 403
otherwise). Jobs that need not be atomic with a database write can go straight
to the platform: `client.dispatchJobs().createBatch(jobs)`.

The rows match the framework's conformance vectors
(`flowcatalyst-framework/conformance/outbox`, copied into
`src/test/resources/conformance`; `OutboxConformanceTest`).

## Creating users and invitations

`principals().createUser(...)` accepts two optional flags that control who
sends the "set your password" mail. Prefer pattern 1 when you want the user
to land back inside your own application.

**1. Login-detected (recommended).** Create the user with `sendInvitation:
false` and send your own email that links to your application. The platform
detects the passwordless account at the hosted login and handles password
creation itself, then returns the user to your stored OAuth redirect.

```java
PrincipalResponse user = client.principals().createUser(new CreateUserRequest()
        .email("new.user@example.com")
        .name("New User")
        .sendInvitation(false));
// Send your own "welcome" email now, linking to your app.
```

**2. Embedded link.** Create the user with `returnInviteLink: true` and read
`inviteLink` from the response to embed in your own email. The user sets a
password on the platform and lands on the platform's own landing page.
`returnInviteLink: true` always suppresses the platform's own invite email,
even when `sendInvitation` is left at its default — the token can only be
minted once, so asking for the link back means you are taking over delivery.

```java
PrincipalResponse user = client.principals().createUser(new CreateUserRequest()
        .email("new.user@example.com")
        .name("New User")
        .returnInviteLink(true));
String inviteLink = user.getInviteLink();
// Embed inviteLink in your own email; never log it or store it in plaintext —
// it is a live 72-hour bearer credential, exactly like a password.
```

## Application admin pages: your own events, jobs and users

An application's admin pages call the platform as the application's own service account
(after your application has checked the signed-in user's admin permission). The service
account is confined to its own application: it sees only events and dispatch jobs whose
code starts with its application code, and only the users it owns (CLIENT users at clients
the application is enabled for who hold one of its roles).

```java
var events = client.events().list(EventsResource.Filters.page(50, 0));        // newest first
var jobs = client.dispatchJobs().list(DispatchJobsResource.Filters.page(50, 0));
var attempts = client.dispatchJobs().attempts(jobs.getFirst().getId());

var users = client.applicationUsers();
var page = users.list("orders", ApplicationUsersResource.Filters.page(0, 50));
users.invite("orders", new InviteApplicationUserRequest()
        .email("ann@acme.test").name("Ann").clientId("acme").roles(List.of("orders:clerk")));
var pending = users.invitations("orders", "PENDING", 0, 50);   // never carries a link
users.resend("orders", userId, null);                          // fresh link, earlier one dies
users.revoke("orders", userId);                                // deletes the link, deactivates the user
```

The credential needs the built-in `platform:application-service` role's
`event:view`, `dispatch-job:view`, `user:view`, `user:create`, `invitation:view` and
`invitation:manage` (mint a new token after the role gains them). Another application's
data answers `403` (a code) or `404` (an id), and a credential bound to no application is
refused with `403`. `returnInviteLink` gives the live link back instead of emailing it: treat
it like a password.

## Ingestions

`client.ingestions()` and `client.ingestionSets()` manage ingestions: data taken from a source,
split into records, each record becoming a dispatch job. An ingestion has a required INTEGRATION
application code and a required client (id or identifier). `IngestionDefinitions` types the
`source` and `split`, which the OpenAPI document leaves as open objects tagged by `kind`; a kind a
runtime extension supplies (and `camel-route`) is `Source.Custom(type, settings)`. A secret is a
`SecretRef` name, never a value.

```java
var created = client.ingestions().create(new CreateIngestionRequest()
        .application("orders").client("acme").code("feed").name("Feed")
        .source(new Source.Sftp("sftp.acme.test", 22, "/out", "feed",
                new SecretRef("acme-sftp-key"), null, "*.csv", Duration.ofMinutes(5)))
        .split(new Split.Csv(true))
        .target(IngestionDefinitions.target("orders:feed:row", "https://api.example.com/rows",
                "orders-pool", Mode.NEXT_ON_ERROR, 100)));

var page = client.ingestions().list(IngestionsResource.ListFilters.page(null, 50));
var next = client.ingestions().list(IngestionsResource.ListFilters.page(page.getNextCursor(), 50));
String yaml = client.ingestions().getDefinition(created.getId());   // the file form
```

Update takes the live version you read as `expectedVersion` (409 when it moved); a code in use
answers 409 `INGESTION_CODE_EXISTS`. Sets apply definition files together:
`ingestionSets().apply(name, request, true)` is a dry run. The runtime operations
(`/api/ingestion-runtime/v1/*`) are the runtime's own client and are not in this SDK.

Without the promote permission a create or update saves the definition as a **draft**: the
answer's `getSave().getOutcome()` is `DRAFT` and `getAwaitingPromote()` is true until someone who
holds promote makes it live (`makeVersionLive`). A definition an existing version already says is
not saved again (`UNCHANGED`, `MADE_LIVE`, `DRAFT_EXISTS`); versions carry `draft` and a `digest`.
`getWarnings()` names what will not run as meant, such as an sftp source with neither `knownHosts`
nor `acceptAnyHostKey` (the `Source.Sftp` component that connects without checking the host key,
which needs the bind-secrets permission).

Notices are failures the platform refused (category `rejected`); the ingestion counts the open
ones (`getNotices()`, `getLatestNotice()`):

```java
var open = client.ingestions().listFailures(id, IngestionsResource.FailureFilters.openNotices());
client.ingestions().dismissFailure(id, open.getFailures().getFirst().getId()); // stays listed
var secrets = client.ingestions().listSecrets(id); // bound names and each runtime's last fetch
```

`listSecrets` returns names, whether the live definition binds them, and per runtime the last
fetch (`fetchCount`, `lastFetchedAt`); never a value.

## Platform configuration

`client.platformConfig()` lists, reads, sets and deletes an application's properties; a SECRET's
value is always `***` there, and `revealSecret` is the only way to read it. Reveal is always a
permission: `platform:admin:config:reveal`, or `platform:application-service:config:reveal` for an
application's service account, which the platform confines to its own applications and to the
clients it is linked to (its own client list, else the clients its application is enabled for)
plus the application-wide value. Setting a secret to `***` is refused (`400
SECRET_VALUE_MASKED`), as is an empty secret (`400 SECRET_VALUE_REQUIRED`).

A service account acting for one of its signed-in users passes who as the last argument of `set`,
`delete` or `revealSecret`; it is sent as `X-On-Behalf-Of` (`PlatformConfigResource.ON_BEHALF_OF`)
and recorded in the audit row (at most 256 printable characters, else `400
INVALID_ON_BEHALF_OF`; ignored from a caller that is not a service account).

## Declaring definitions

Programmatically:

```java
var set = Definitions.DefinitionSet.define("orders")
        .withEventTypes(List.of(
                Definitions.EventType.of("orders:sales:order:placed", "Order Placed")))
        .withRoles(List.of(Definitions.Role.of("admin")
                .withPermissions(List.of(Definitions.Permission.of("admin", "*", "*")))));

client.definitions().sync(set, SyncOptions.removingUnlisted());
```

Principals name roles in full (`orders:admin`; the platform adds no prefix).
A name that is not a role is reported in the principals result's
`skippedRoles()`, and the email of an existing user whose `passwordHash` was
not applied in `passwordHashIgnored()`. Documentation pages
(`withDocs(List.of(Definitions.Doc.of("getting-started", markdown)))`) replace
the application's whole page set on each sync.

Or with annotations and explicit registration:

```java
@AsEventType(code = "orders:sales:order:placed", name = "Order Placed")
public record OrderPlaced(String orderId) {}

@AsConnection(code = "billing-hook", name = "Billing Webhook")
final class BillingConnection {}

@AsSubscription(code = "order-placed", name = "Order Placed",
        target = "https://billing.example.com/hook",
        connectionCode = "billing-hook",
        eventTypes = {"orders:sales:order:placed"})
final class OrderPlacedHandler {}

var set = DefinitionScanner.scan("orders",
        List.of(OrderPlaced.class, BillingConnection.class, OrderPlacedHandler.class));
client.definitions().sync(set);
```

### Connections and per-client sync

A connection is application-owned — the platform assigns its own service
account, so a `Definitions.Connection` carries no credentials, only
`code`/`name`/`description`/`externalId`. Reference it from a subscription by
`connectionCode` rather than `connectionId`: codes are stable across
environments, ids are not. Connections sync BEFORE subscriptions in the same
set, so a subscription may name a connection the same sync just created.

```java
var set = Definitions.DefinitionSet.define("orders")
        .withConnections(List.of(Definitions.Connection.of("billing-hook", "Billing Webhook")))
        .withSubscriptions(List.of(Definitions.Subscription.of(
                        "order-placed", "Order Placed", "https://billing.example.com/hook",
                        List.of(Definitions.SubscriptionEventType.of("orders:sales:order:placed")))
                .withConnectionCode("billing-hook")));

client.definitions().sync(set, SyncOptions.removingUnlisted());
```

A bare `connectionCode` names a connection owned by the SAME application; add
`.withSharedConnection(true)` to name a shared (application-less) connection
instead — the two namespaces never fall back to one another.

To scope a whole set of connections/subscriptions to one client (tenant),
call `.forClient(clientIdOrIdentifier)` on the `DefinitionSet`. Sync several
sets that may share an application (and possibly a client) with
`syncGrouped(sets)` rather than `syncAll(sets, options)`: `syncGrouped` MERGES
every set sharing an application code before issuing one platform call per
`(application, client)` scope — global scope first, then each client scope in
first-seen order — so two sets contributing to the SAME scope never become
two separate `removeUnlisted` calls, the second of which would delete what
the first just created.

```java
var acme = Definitions.DefinitionSet.define("orders").forClient("acme")
        .withConnections(List.of(Definitions.Connection.of("hook", "Acme Hook")));
var beta = Definitions.DefinitionSet.define("orders").forClient("beta")
        .withConnections(List.of(Definitions.Connection.of("hook", "Beta Hook")));

Map<String, SyncResult> results = client.definitions().syncGrouped(List.of(acme, beta));
```

A single row can override which client scope it belongs to with
`Connection#withClient` / `Subscription#withClient` — it wins over the
owning `DefinitionSet`'s own client, useful for one codebase-defined set that
still needs to route a handful of rows to a specific tenant:

```java
var set = Definitions.DefinitionSet.define("orders")
        .withConnections(List.of(
                Definitions.Connection.of("shared-hook", "Shared Hook"),      // global
                Definitions.Connection.of("acme-hook", "Acme Hook").withClient("acme")));

client.definitions().sync(set, SyncOptions.removingUnlisted()); // two calls: global, then acme
```

### Subscription target base URL

A subscription's `target` may be a path (`/webhooks/orders`) instead of a
full URL — resolved at sync time against a base URL, so one definition
works across every environment. An absolute target (has a scheme) is always
sent verbatim. The base comes from, in order: the row's own `DefinitionSet`'s
`forClient(client, targetBaseUrl)` override, then the synchronizer's own
default (`FlowCatalystClient.Builder.subscriptionTargetBaseUrl(...)`, or the
second argument to `new DefinitionSynchronizer(transport, baseUrl)` when
building one directly). A path with no base available anywhere fails that
subscription's sync locally, naming it, rather than being silently dropped —
under `removeUnlisted` a dropped subscription would be deleted.

```java
var client = FlowCatalystClient.builder()
        .baseUrl("https://platform.flowcatalyst.io")
        .clientCredentials(id, secret)
        .subscriptionTargetBaseUrl("https://api.myapp.com") // the default for path-style targets
        .build();

var tenantSet = Definitions.DefinitionSet.define("orders")
        .forClient("acme", "https://acme.myapp.com") // this tenant's own host
        .withSubscriptions(List.of(Definitions.Subscription.of(
                        "order-placed", "Order Placed", "/webhooks/orders", // resolves per-tenant
                        List.of(Definitions.SubscriptionEventType.of("orders:sales:order:placed")))
                .withConnectionCode("billing-hook")));
```

`sync`, `syncAll` and `syncGrouped` all throw `DefinitionSyncException` (a
`FlowCatalystException` carrying `SdkError.PartialFailure`) if any category —
most commonly connections or subscriptions — could not be synced: a
duplicate code within one scope, or an HTTP failure. A connection sync
failure skips that SAME scope's subscription sync (their `connectionCode`s
may not resolve) without stopping any *other* scope or application from
being attempted; `syncAll`/`syncGrouped` run every set/application to
completion before throwing once, so a caller sees every failure at once
rather than stopping at the first. The exception carries whatever DID sync —
`result()` (`sync`), `results()` (`syncAll`) or `resultsByApplication()`
(`syncGrouped`) — so a deploy step can log the partial outcome instead of
silently treating "no exception" as success:

```java
try {
    client.definitions().sync(set, SyncOptions.removingUnlisted());
} catch (DefinitionSyncException e) {
    log.error("definition sync had failures: {}", e.getMessage());
    if (e.result().connections() instanceof SyncResult.Category.Synced synced) {
        log.info("connections DID sync: {} created", synced.created());
    }
    throw e;
}
```

## Webhook verification

```java
WebhookSignature.verify(rawBodyBytes,
        request.header("X-FlowCatalyst-Signature"),
        request.header("X-FlowCatalyst-Timestamp"),
        signingSecret);
```

The body must be the raw bytes as received (sign-then-parse).

## Building

```
make build-java-sdk        # from the repo root: regenerates models + verify
```

Models under `io.flowcatalyst.sdk.generated.model` are generated at build time
from `openapi/openapi.json` (refreshed by `make sdk-spec`) — wire drift
surfaces as compile errors in the hand-written resource layer.

Examples live in `src/test/java/examples/`; run with:

```
mvn -q test-compile org.codehaus.mojo:exec-maven-plugin:3.5.0:java \
    -Dexec.mainClass=examples.ListEventTypes -Dexec.classpathScope=test
```

Releases: `make release-java-sdk BUMP=minor` (tags `java-sdk/vX.Y.Z`).
