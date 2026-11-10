# @flowcatalyst/sdk

Official TypeScript/JavaScript SDK for FlowCatalyst platform.

## Installation

The SDK is not published to npm. Install it from a git tag of the split
repository, [flowcatalyst/typescript-sdk](https://github.com/flowcatalyst/typescript-sdk)
(tags are `vX.Y.Z`; see the repository's releases for the latest):

```bash
npm install github:flowcatalyst/typescript-sdk#v0.9.9
# or
pnpm add github:flowcatalyst/typescript-sdk#v0.9.9
```

The import name is still `@flowcatalyst/sdk`.

## Local development with `fcdev`

For local work you need a FlowCatalyst control plane to talk to.
`fcdev` is the official one-binary dev environment — bundled
PostgreSQL, platform API, message router, scheduler, and frontend
in a single process.

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.sh | sh

# Windows (PowerShell)
irm https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.ps1 | iex

fcdev          # starts API on http://localhost:8080
```

If you publish events via the **outbox pattern**, you also need
`fcdev outbox` running as a sidecar — it polls your app's
`outbox_messages` table and forwards events to the platform:

```bash
# In your project directory (where this SDK is installed):

# Once (optional): create the `outbox_messages` table. `fcdev outbox`
# also creates it on first run for Postgres; use this for MySQL/MongoDB or
# to provision it ahead of time (the poller can read those; note this SDK
# itself only ships a Postgres driver, `PgOutboxDriver`). Postgres shown:
fcdev outbox create-table --db-type pg --db-url postgres://user:pass@localhost:5432/app

# Run the poller. It loads ./.env (override with --env-file) and reads
# FC_OUTBOX_SOURCE_DB_URL (your app database), FC_OUTBOX_PLATFORM_URL and
# the service-account credentials FLOWCATALYST_CLIENT_ID /
# FLOWCATALYST_CLIENT_SECRET (or FC_OUTBOX_PLATFORM_AUTH_TOKEN for a static
# token). Existing environment variables win over .env.
fcdev outbox
```

The SDK's own migration at
[`migrations/postgresql/001_create_outbox_messages.sql`](migrations/postgresql/001_create_outbox_messages.sql)
and `fcdev outbox`'s built-in `CREATE TABLE IF NOT EXISTS`
produce the same schema, so it doesn't matter which one runs first.

Event `type`s and dispatch-job `code`s you write to the outbox must be fully
qualified `application:subdomain:aggregate:action` strings (for example
`shop:orders:order:created`); the SDK rejects bare codes.

Complete reference: run `fcdev --help`, or see the
[FlowCatalyst repository](https://github.com/flowcatalyst/flowcatalyst).

## Usage

```typescript
import { FlowCatalystClient } from '@flowcatalyst/sdk';

// Initialize the client (client_credentials — a service account)
const client = new FlowCatalystClient({
  baseUrl: 'http://localhost:8080',
  clientId: 'your_client_id',
  clientSecret: 'your_client_secret',
});

// All methods return a neverthrow ResultAsync — no thrown errors
const result = await client.eventTypes().list();
result.match(
  (eventTypes) => console.log('Event types:', eventTypes.items),
  (error) => console.error('Error:', error.type, error.message),
);

// Create a new event type
const created = await client.eventTypes().create({
  code: 'orders:fulfillment:order:created',
  name: 'Order Created',
  description: 'Fired when a new order is placed',
});

// Create a subscription
const subscription = await client.subscriptions().create({
  code: 'notify-warehouse',
  name: 'Notify Warehouse',
  eventTypes: [{ eventTypeCode: 'orders:fulfillment:order:created' }],
  endpoint: 'https://myapp.com/webhooks',
});
```

## API Reference

### FlowCatalystClient

#### Constructor

```typescript
new FlowCatalystClient(config: FlowCatalystConfig)
```

**Config options** — two authentication modes:

- Client credentials (service account):
  - `baseUrl` (required): Base URL of the FlowCatalyst platform
  - `clientId` / `clientSecret` (required): OAuth client credentials
  - `tokenUrl` (optional): Custom token endpoint (default: `{baseUrl}/oauth/token`)
- User token (you already hold an access token):
  - `baseUrl` (required)
  - `accessToken` (required): the token, or a (sync/async) function returning the current one

Both modes also accept `timeout` (ms, default 30000), `retryAttempts`
(default 3), `retryDelay` (ms, default 100), and `routerBaseUrl` (message
router host for `client.router()`, defaults to `baseUrl`).

#### Resources

The API surface hangs off resource accessors, each wrapping the generated
typed client with auth, retries, and neverthrow results:

- `client.eventTypes()` — `list()`, `get(id)`, `create(...)`, `update(...)`, `addSchemaVersion(...)`, `archive(id)`, `sync(...)`
- `client.subscriptions()`, `client.dispatchPools()`, `client.connections()`, `client.processes()`
- `client.roles()`, `client.permissions()`, `client.applications()`, `client.clients()`, `client.principals()`, `client.me()`
- `client.ingestions()` — CRUD, `pause`/`resume`, versions (`saveVersion`, `makeVersionLive`), `getDefinition` (YAML), `listHistory`, `listFailures({category?, undismissed?, after?, limit?})`, `dismissFailure(id, failureId)`, `listSecrets(id)`, `listSourceKinds`, `testConnection`/`getTest`, and sets (`applySet(name, req, dryRun?)`, `reapplySet`, `releaseSet`, ...). Secrets in a definition are names, never values; the runtime API is not part of this SDK (see [Ingestions](#ingestions))
- `client.scheduledJobs()`, `client.auditLogs()`, `client.router()`, `client.definitions()`
- `client.platformConfig()` — `list(app)`, `get(app, section, property, {clientId}?)`, `set(..., {onBehalfOf}?)`, `delete(..., {clientId?, onBehalfOf?})`, `revealSecret(app, section, property, clientId?, {onBehalfOf}?)`

Anything not covered by a resource class is available through the generated
functions (importable from the package root or the `api` namespace), passed
through `client.request()` for auth + retries:

```typescript
import { api } from '@flowcatalyst/sdk';

const result = await client.request((httpClient, headers) =>
  api.listDispatchJobs({ client: httpClient, headers, query: { status: 'FAILED' } }),
);
```

### Platform config secrets

`platformConfig().list()` / `.get()` ALWAYS return the literal `"***"` as the
`value` of `SECRET` properties, for every caller. The only way to read the
plaintext is the audited reveal call:

```typescript
const r = await client.platformConfig().revealSecret('my-app', 'smtp', 'password');
if (r.isOk()) use(r.value.value); // r.value.updatedAt changes when the secret is rotated
```

Reveal is always a permission, never implied by being the application's
service account: `platform:admin:config:reveal` (a platform administrator) or
`platform:application-service:config:reveal` (an application's service
account; the `platform:application-service` role carries it). The platform
confines an application's service account to its own applications and to the
clients it is linked to (its own client list, else the clients its application
is enabled for), plus the application-wide value. Errors: `forbidden` (403),
`not_found` (404), `http_error` with `status: 400` / `body.error ===
'NOT_A_SECRET'` for a PLAIN property (use `get`). The call is never cached by
the SDK. The returned `value` is a non-enumerable property, so it does not
appear in `console.log`, spread copies or `JSON.stringify` (rendered as
`[REDACTED]`); never log it.

Writing a SECRET: the mask is not a value. Setting a secret to `"***"` is
refused (`400 SECRET_VALUE_MASKED`), as is an empty secret (`400
SECRET_VALUE_REQUIRED`); to leave a secret unchanged, do not set it.

Acting for a person: a service account that changes or reveals configuration
for one of its application's signed-in users passes `{ onBehalfOf: '<who>' }`
to `set`, `delete` or `revealSecret`. It is sent as `X-On-Behalf-Of` and
recorded in the audit row; the platform honours it only from a service account
(ignored from anyone else) and refuses more than 256 characters or a control
character (`400 INVALID_ON_BEHALF_OF`).

```typescript
await client.platformConfig().set('my-app', 'smtp', 'password',
  { value: newPassword, valueType: 'SECRET' }, { onBehalfOf: 'alice@example.com' });
```

### Ingestions

Without the promote permission, `create`, `update` and `saveVersion` save the
definition as a **draft**: the answer's `save.outcome` is `DRAFT` and
`save.awaitingPromote` is true until someone who holds promote makes it live
(`makeVersionLive`). A definition an existing version already says is not saved
again (`UNCHANGED`, `MADE_LIVE`, `DRAFT_EXISTS`). Versions carry `draft` and a
`digest` of the definition. `warnings` names what the definition says that will
not run as meant (for example an sftp source with neither `knownHosts` nor
`acceptAnyHostKey`).

Notices are failures the platform refused (category `rejected`); the ingestion
counts the open ones (`notices`, `latestNotice`). List and dismiss them:

```typescript
const open = await client.ingestions().listFailures(id, { category: 'rejected', undismissed: true });
await client.ingestions().dismissFailure(id, failureId); // stays in the list, stops counting
```

`listSecrets(id)` lists the secret names the live definition binds and, per
runtime, when it last fetched each one (`fetchCount`, `lastFetchedAt`). Names
only: no call here ever returns a secret value.

## Syncing Definitions

Declare your application's roles, permissions, event types, connections,
subscriptions, dispatch pools, and principals in code, then push them to the
platform with a single call:

```typescript
import { FlowCatalystClient, sync } from "@flowcatalyst/sdk";

const definitions = sync
  .defineApplication("orders")
  .withRoles([{ name: "admin", displayName: "Administrator" }])
  .withEventTypes([
    { code: "orders:fulfillment:shipment:shipped", name: "Shipment Shipped" },
  ])
  .build();

await client.definitions().sync(definitions);
```

See **[docs/syncing-definitions.md](./docs/syncing-definitions.md)** for the
full structure guide — how to name roles, the 4-part permission format,
event-type code conventions, subscription modes, dispatch pool sizing, and
principal management.

## Push to clients

Tell the people looking at a screen that something changed, without holding a
stream yourself. The app writes a `PUSH` row to its outbox **in the same
transaction as the change**; the platform's outbox processor sends it to the
push gateway, which fans it out over server-sent events to the clients listening
on the topic. Pushes are signals (the client reads the change again): durable
work stays events and dispatch jobs. The wire and the readers (field-js,
field-react-native `PushStream`, base URL `{platform}/api`) are unchanged.

> The platform's outbox processor must already know the `PUSH` type (fcdev and
> the processor image that ship the push gateway do). An older processor posts
> an unknown type to the bare platform URL and fails the row.

### Write a push

```typescript
import { OutboxManager, PgOutboxDriver, CreatePushDto } from "@flowcatalyst/sdk";

const outbox = new OutboxManager(new PgOutboxDriver(pool), "clt_0HZXEQ5Y8JY5Z", {
  clientCode: "acme", // the default client of a push; "_platform" leaves it out
});

await outbox.createPush(
  CreatePushDto.create("shop:orders:ord_2", "order.updated", { status: "shipped" }),
  tx, // the same transaction as the change
);
```

- `topic` is `{application}:{segment}(:{segment})*`, the first segment the
  application (your credential must be allowed to push for it); `type` is
  `[a-z][a-z0-9-]*(.[a-z][a-z0-9-]*)*` (`reset`, `auth-expired`, `subscribed`
  and `fw.*` are reserved); `data` is a JSON object (or JSON text holding one),
  16 KiB by default, up to 64 KiB with `.withMaxDataBytes(n)`.
- A push the platform would refuse is refused when it is built: `createPush`
  throws a `PushError` whose `code` is `TOPIC_INVALID`, `TYPE_INVALID`,
  `TYPE_RESERVED`, `DATA_INVALID`, `DATA_NOT_OBJECT`, `DATA_TOO_LARGE`,
  `PUSH_ID_INVALID` or `CLIENT_INVALID`.
- **Ordering.** The row's message group is the topic, so the pushes of one topic
  reach the gateway in order (one HTTP call per push). A signal-only topic that
  does not need order can opt out: `.withUngrouped()` (batched, concurrent, no
  order), or name another group with `.withMessageGroup(g)`.
- The row id is the `pushId` (a TSID unless you `.withId(...)`, at most 26
  characters); the platform writes a `pushId` once.
- **Instant delivery.** With `PgOutboxDriver` on Postgres the insert also runs
  `pg_notify('fc_outbox_push', '')` in the same statement, so the processor wakes
  when your transaction commits (a rollback notifies nobody), typically within a
  few milliseconds. A driver that cannot notify (anything that is not
  `PgOutboxDriver`, or a database other than Postgres) writes the plain row and
  the processor's poll delivers it (default 1 s). If you write your own driver
  for Postgres, run the exported `PUSH_INSERT_SQL` statement for `PUSH` rows.

### In a use case

```typescript
import { usecase, CreatePushDto } from "@flowcatalyst/sdk";

return usecase.Plan.andPush(
  usecase.Plan.save(order, orderRepo, event),
  CreatePushDto.create(`shop:orders:${order.id}`, "order.updated", { status: "placed" }),
  (e) => CreatePushDto.create("shop:board", "refresh", { eventId: e.eventId }).withUngrouped(),
);
```

The pushes are written after the event, in the plan's transaction. An invalid
push fails the plan with a validation error (its code is the push's) and the
transaction rolls back. The unit of work's `clientCode` option is the default
client. (The Effect flavour has no Plan; call `outboxManager.createPush(push, tx)`
inside its transaction.)

### Who may listen: rules, keys, tickets

A client listens to a topic when a **rule** allows it or it holds a **ticket**.
Declare both with the other definitions; they are a full replace at the gateway,
so give both halves every time:

```typescript
import { sync, TicketMinter } from "@flowcatalyst/sdk";

// PUSH_TICKET_KEY = "kid:base64url-seed" (Ed25519, 32-byte seed; a secret)
const tickets = TicketMinter.fromConfig("shop", process.env.PUSH_TICKET_KEY!, [
  // during a rotation: the previous key, still published so live tickets verify
]);

const definitions = sync
  .defineApplication("shop")
  .withPush({
    rules: [
      { pattern: "shop:orders:{id}", requires: ["shop:orders:order:view"] },
      { pattern: "shop:user:{principal}" },        // only your own
      { pattern: "shop:announcements", public: true }, // deliberately open
    ],
    keys: tickets, // sends the public JWKS (up to 4 keys)
  })
  .build();

await client.definitions().sync(definitions);          // push is one step of sync()
await client.definitions().syncPush("shop", { rules, keys: tickets }); // or alone
```

The first segment of every pattern is your application code. `{principal}` must
equal the subscriber's principal id, `{client}` the stream's client, any other
`{name}` matches one segment. A rule with neither `{principal}` nor `requires`
must say `public: true`. The gateway refuses (400, with a code) rules for
another application's patterns, duplicates, more than 200 rules, more than 4
keys, or keys that are not public Ed25519 JWKs. A set that does not declare
`push` leaves the gateway as it is (`skipPush: true` skips it too).

**Rotation:** add the new key (keep the old one as `previous`), sync, start
signing with the new key, wait one ticket lifetime (at most 5 minutes), drop the
old key, sync again.

### Mint a ticket

For what a rule cannot express (this user may see order 7 but not order 8), mint
a ticket in your own endpoint, after your own access check:

```typescript
const ticket = tickets.mint({
  client: "acme",                       // the stream's client ("_platform" for platform-wide)
  topics: ["shop:orders:ord_7", "shop:orders:ord_8:{part}"], // placeholders match one segment
  sub: "prn_1",                          // optional: recorded in logs
  // jkt: "<dpop key thumbprint>",       // optional: the subscribe must prove that key
  // ttlSeconds: 60,                     // 1 to 300, default 60: the time to subscribe with it
  // streamExp: new Date(...),           // optional: the latest the stream may live
});
// hand `ticket` to the browser; it POSTs {topics, tickets: [ticket]} to {platform}/api/push/subscriptions
```

A ticket is an EdDSA JWS (`iss` = your application code, `aud` `fw-push`,
header `kid`). `mint` refuses a topic outside your application, more than 100
topics and a lifetime over 5 minutes, because the gateway would grant nothing.
`tickets.jwks()` is the JWKS that `GET /push/keys` serves and sync sends.
Needs Node's `node:crypto` (Ed25519).

## Fastify integration

If you're running a Fastify app, `@flowcatalyst/sdk/fastify` is a drop-in
plugin that handles both the browser OIDC flow (redirect + encrypted
session cookie) and Bearer-token validation for API callers — exposing the
same `request.principal` on both. Roles come from the FlowCatalyst access
token; permissions are resolved locally from a catalogue you declare in code.

Optional peer dependencies: `fastify@^5`, `@fastify/cookie`, `jose`. Install
the ones you need:

```bash
pnpm add fastify @fastify/cookie jose
```

```typescript
import Fastify from "fastify";
import {
  flowcatalystAuth,
  defineRbac,
  generateSessionSecret,
} from "@flowcatalyst/sdk/fastify";

const rbac = defineRbac()
  .role("operant:admin").grants("operant:*")
  .role("operant:viewer").grants("operant:read")
  .role("support").grants("ticket:*")
  .build();

const app = Fastify();

await app.register(flowcatalystAuth, {
  baseUrl: "https://platform.example.com",
  clientId: process.env.FC_CLIENT_ID!,
  clientSecret: process.env.FC_CLIENT_SECRET!,
  cookie: { secret: process.env.SESSION_SECRET! }, // 32B base64url
  rbac,
});

// Web route: 302s to /auth/login if no session cookie.
app.get("/dashboard", { preHandler: app.fc.requireSession() }, async (req) => {
  return { hello: req.principal!.name };
});

// API route: 401 JSON if no valid Bearer token.
app.post("/api/orders", { preHandler: app.fc.requireBearer() }, async (req) => {
  if (!req.principal!.hasPermissionTo(["operant:write"])) {
    throw app.httpErrors.forbidden();
  }
  // ...
});

// Either: redirects browsers, 401s machines.
app.get("/api/me", { preHandler: app.fc.requireAuth() }, async (req) => ({
  id: req.principal!.id,
  roles: req.principal!.roles,
  permissions: req.principal!.hasAnyPermissionTo(["operant:read"]),
}));
```

Generate a session secret with `node -e "console.log(require('@flowcatalyst/sdk/fastify').generateSessionSecret())"`.

Other useful options: `scope` (default `openid profile email`),
`expectedAudience`, `sessionStore`, `routes` (override the default
`/auth/{login,callback,logout}` paths), `publicBaseUrl`, and `portal` —
set `portal: true` when the app is a customer portal, so logins enter
through the platform's portal identity plane (`/portal/authorize`) instead
of the standard authorize endpoint (see [Portal identities](#portal-identities)).

`client` names the FlowCatalyst client (its URL-safe slug, e.g. `"acme"`)
whose login branding the sign-in pages should wear — logo, colours, brand
name and footer, configured under **Clients → Login Branding**. A
per-request `?client=` on the login route overrides it. It is purely
cosmetic: it does not affect who may sign in or what they may access, and
an unrecognised value falls back to the platform-wide theme. Ignored in
`portal` mode, which has its own login surface.

### Principal helpers

`request.principal` is the same shape for cookie and Bearer callers:

```typescript
principal.id;                                    // "prn_..." or service principal id
principal.scope;                                 // "anchor" | "partner" | "client"
principal.clients;                               // ["clt_abc", ...] or ["*"] for anchors
principal.roles;                                 // ["operant:admin", ...]
principal.applications;                          // application ids (codes are in principal.applicationCodes)
principal.mechanism;                             // "session" | "bearer"

principal.hasRole("operant:admin");              // single
principal.hasRoles(["a", "b"]);                  // ALL
principal.hasAnyRole(["a", "b"]);                // ANY
principal.hasPermissionTo(["operant:read"]);     // ALL (wildcard-aware)
principal.hasAnyPermissionTo(["a", "b"]);        // ANY (wildcard-aware)
principal.isAnchor();
principal.canAccessClient("clt_abc");

principal.sessionData;                           // app's bag, persisted in the session
```

Permission wildcards use `:` as separator with `*` as a suffix at any
segment boundary. `operant:*` matches `operant:read` and `operant:reports:export`;
`*` matches anything. Mid-string globs (e.g. `operant:r*`) are not supported.

### Custom post-auth logic

The plugin doesn't expose a special hook — once it has populated
`request.principal`, register a normal Fastify `preHandler` to enrich it
or enforce app-specific checks:

```typescript
app.addHook("preHandler", async (req) => {
  if (!req.principal) return;
  const user = await db.user.upsert({
    where: { fcId: req.principal.id },
    update: { lastSeenAt: new Date() },
    create: { fcId: req.principal.id, email: req.principal.email },
  });
  req.principal.sessionData.localUserId = user.id;
});
```

For cookie sessions, anything you write to `principal.sessionData` from a
post-auth hook is **not** persisted automatically — the session is read
from the store on each request. Persist via `app.fc.logout` / re-issue if
you need a write path; or use the `PgSessionStore` / `RedisSessionStore`
backends if you need mutable server-side state.

### Logout

Default is local-only — clear the session cookie. To also terminate the
OIDC session at the platform, redirect to FlowCatalyst's logout page:

```typescript
app.post("/auth/logout", async (req, reply) => {
  await app.fc.logout(req, reply, {
    redirectTo: "https://platform.example.com/logout",
  });
});
```

### Server-side session storage

The default `CookieSessionStore` puts the encrypted session in the cookie
itself — zero infra, but limited to ~4KB. Swap to Postgres or Redis once
you want to store more per-session data or revoke sessions server-side:

```typescript
import { PgSessionStore, initSessionSchema } from "@flowcatalyst/sdk/fastify";
import pg from "pg";

const pool = new pg.Pool({ connectionString: process.env.DATABASE_URL });
await initSessionSchema(pool);

await app.register(flowcatalystAuth, {
  // ...
  sessionStore: new PgSessionStore({
    executor: pool,
    cookieName: "fc_session",
    cookieOptions: {
      path: "/",
      httpOnly: true,
      secure: true,
      sameSite: "lax",
      maxAge: 60 * 60 * 8,
    },
  }),
});
```

`RedisSessionStore` follows the same shape against an ioredis-compatible
client. With a server-side store, the cookie contains an opaque session id
only and the payload (including any size of `sessionData`) lives in the
backend.

`PgSessionStore` mints a fresh sid on every `write()` (including the
mid-session token refresh the plugin performs automatically), deleting the
row it supersedes in the same round trip — a long-lived session never
orphans one row per rotation. It also carries a periodic sweep of rows past
`expires_at` (reads already filter those out, so this is about table growth,
not correctness): the `flowcatalystAuth` plugin starts it automatically when
the store is registered, and stops it when the Fastify instance closes.
It defaults to an hourly interval and is `unref()`'d, so it never by itself
holds the process open; pass `reapIntervalMs` to change the interval or
`reapIntervalMs: false` to opt out entirely:

```typescript
new PgSessionStore({
  executor: pool,
  cookieName: "fc_session",
  cookieOptions: { /* ... */ },
  reapIntervalMs: false, // opt out — sweep expired rows yourself instead
});
```

## Using with Effect

If your project uses [Effect](https://effect.website/), the SDK ships an
optional Effect-flavored surface at `@flowcatalyst/sdk/effect/usecase` that
gives the write path (events, dispatch jobs, audit logs) compile-time
invariant guarantees — a use case that doesn't go through `UnitOfWork`
fails to compile, not at runtime. Effect is an optional peer dependency:
the default neverthrow surface is unchanged for everyone else.

See **[docs/effect-usage.md](./docs/effect-usage.md)** for the full
worked example, layer wiring, error handling with `Effect.catchTag`, and
testing with `TestUnitOfWork`.

## Field workers

Field workers (drivers, pickers, yard staff) sign in on an enrolled device with
a PIN at the platform's `/field/*`. The platform issues a short RS256 field
access token (`typ: fc-field+jwt`, `token_use: "field"`, `aud` = your app code)
bound to the device key, presented to your backend as
`Authorization: DPoP <token>` plus a `DPoP: <proof>` header (ES256 JWS). PINs
never reach your backend; the SDK only verifies.

```ts
import { fieldAuth } from "@flowcatalyst/sdk/fastify";

await app.register(fieldAuth, {
  issuer: "https://auth.example.com",
  appCode: "my-app",
  // The proof's htu is checked against publicBaseUrl + request path.
  // The Host header is never used.
  publicBaseUrl: "https://api.my-app.example.com",
});

app.post("/orders", { preHandler: app.fcField.require() }, async (req) => {
  const p = req.fieldPrincipal!; // workerId, workerCode, deviceId, clientId, appCode, ...
});
```

`fieldAuth` adds no global hook: it only runs on routes that use
`app.fcField.require()` and never touches `request.principal`, so it coexists
with `flowcatalystAuth`. A PIN-change-only token is answered `403
PIN_CHANGE_REQUIRED`; any other refusal is `401` with `WWW-Authenticate: DPoP
error="invalid_token"` and `{ "error": <reason> }`; a discovery/key-fetch
failure is `503`.

Without Fastify, use the verifier directly:

```ts
import { FieldTokenVerifier, FieldError } from "@flowcatalyst/sdk";

const verifier = new FieldTokenVerifier({ issuer, appCode: "my-app" });
try {
  const p = await verifier.verify(authHeader, dpopHeader, "POST", "https://api.my-app.example.com/orders");
} catch (e) {
  if (e instanceof FieldError) console.log(e.reason); // "expired", "htu_mismatch", ...
}
```

Refusal reasons: `malformed`, `wrong_typ`, `wrong_token_use`, `wrong_aud`,
`expired`, `invalid_token`, `pcr`, `missing_proof`, `invalid_proof`,
`jkt_mismatch`, `htm_mismatch`, `htu_mismatch`, `ath_mismatch`, `stale_proof`.
Only RS256 tokens and ES256 proofs are accepted. The proof's `iat` must be
within 5 minutes of now (`proofMaxAgeSeconds` widens it for devices with bad
clocks). Nonce replay protection is not done by the verifier.

Admin API (service account or user token):

```ts
await client.fieldWorkers().ensure({ clientId, workerCode: "W001", name: "Wendy", appCodes: ["my-app"] });
await client.fieldWorkers().resetPin(id);            // temporaryPin returned once
await client.fieldDevices().createEnrolmentCode({ clientId, mode: "SHARED", appCodes: ["my-app"] });
await client.fieldPolicies().put(clientId, { requireAttestation: true });
await client.introspectFieldToken(token);            // online check; session must still be live
```

Resources: `fieldWorkers()` (ensure, list, get, update, delete, resetPin,
unlock, suspend, activate, grantApp, revokeApp, signIns), `fieldDevices()`
(createEnrolmentCode, list, get, suspend, activate, revoke), `fieldPolicies()`
(get, put).

## Portal identities

Portal users (`ptu_…`) are a separate end-user population from platform
users — customers of a client, invited per client, with no platform SSO
reuse. The generated portal-user functions are re-exported from the package
root; pass them through `client.request()`:

```typescript
import { FlowCatalystClient, ensurePortalUser, listPortalUsers } from '@flowcatalyst/sdk';

// Invite (idempotent): returns identityId, created/invited flags, inviteUrl
const ensured = await client.request((c, headers) =>
  ensurePortalUser({
    client: c, headers,
    body: { clientId, email: 'pat@customer.example', returnInviteLink: true },
  }),
);

const users = await client.request((c, headers) =>
  listPortalUsers({ client: c, headers, query: { clientId } }));
```

`activatePortalUser`, `deactivatePortalUser`, and `deletePortalUser` cover
the rest of the lifecycle, and the Fastify plugin's `portal: true` option
handles the browser login flow. All credential UX (login page, invites,
self-service forgot-password) is platform-hosted — the portal app never
implements any of it. Full walkthrough:
[docs/portal-implementation-guide.md](../../docs/portal-implementation-guide.md).

## TypeScript Support

This SDK is written in TypeScript and provides full type definitions. All API responses are properly typed, and every generated wire type is re-exported from the package root.

```typescript
import type { EventTypeResponse, SubscriptionResponse, DispatchJobResponse } from '@flowcatalyst/sdk';
```

## Error Handling

All API methods return a [neverthrow](https://github.com/supermacro/neverthrow)
`ResultAsync` — errors are values, not exceptions:

```typescript
const result = await client.eventTypes().list();

result.match(
  (eventTypes) => console.log('Event types:', eventTypes.items),
  (error) => console.error('API Error:', error.type, error.message),
);

// Or use guards
if (result.isOk()) {
  console.log(result.value.items);
}
```

Error variants (`error.type`): `missing_credentials`, `invalid_credentials`,
`token_expired`, `token_fetch_failed`, `network`, `timeout`, `http_error`,
`validation`, `not_found`, `forbidden`, `conflict`, `rate_limited`.

## AI Agent Access (MCP Server)

If you're using an AI coding agent (Claude Code, Cursor, Windsurf, etc.), you can give it read-only access to your FlowCatalyst event types, schemas, and subscriptions via the FlowCatalyst MCP server. This lets the agent explore your event catalog and generate typed code for you.

The MCP server is built into the platform binaries; there is no separate npm package. Two ways to run it:

- **`fcdev mcp`**: stdio by default (the usual way an MCP client launches a server as a subprocess); pass `--http <bind>` (for example `--http 127.0.0.1:8090`) to serve streamable HTTP instead. Flags `--platform-url`, `--client-id` and `--client-secret` override the `FLOWCATALYST_URL`, `FLOWCATALYST_CLIENT_ID` and `FLOWCATALYST_CLIENT_SECRET` environment variables. `fcdev start --mcp` boots it alongside the dev environment.
- **`fc-server`** hosts it when `FC_MCP_ENABLED` is set.

### Quick setup (Claude Code)

```bash
claude mcp add flowcatalyst -- fcdev mcp
```

### Quick setup (Cursor / Windsurf / Claude Desktop)

Add to your MCP config file (`.cursor/mcp.json`, Claude Desktop config, etc.):

```json
{
  "mcpServers": {
    "flowcatalyst": {
      "command": "fcdev",
      "args": ["mcp"],
      "env": {
        "FLOWCATALYST_URL": "https://your-instance.flowcatalyst.io",
        "FLOWCATALYST_CLIENT_ID": "svc_abc123",
        "FLOWCATALYST_CLIENT_SECRET": "your_secret"
      }
    }
  }
}
```

You need a service account with the `AI Agent Read-Only` role.

## Development

```bash
# Install dependencies
npm install

# Build
npm run build

# Watch mode
npm run dev

# Type check
npm run lint
```

## License

Apache-2.0
