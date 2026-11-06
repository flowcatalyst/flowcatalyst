# Functions

A **function** is a small program the platform stores, versions and runs for
you. You write it in Go, Rust or TypeScript against a FlowCatalyst SDK (or in Java, see
"JVM functions"), publish it, and promote a version to `live`. The platform then wires it up:
subscriptions deliver events to it, schedules fire it, and callers invoke it
over HTTP — with no server of yours to run.

Functions are **first-party code**: written and published by the people who operate
this platform to provide services to their customers. Customers do not write or run
functions of their own, and nothing in the platform or the SDKs is built to let them.

Functions run in the **function runner**, a sandbox built on WebAssembly (it isolates
faults and limits what each function can reach; it is not hardened against hostile code).
A Java function is a jar served by the ingestor runtime instead (see "JVM functions"). A
function reaches nothing except what it declares: its config and secret keys,
its databases, the hosts it may call, and the event types it may emit. The
runner holds every credential; the function never sees one.

## Addresses, versions and aliases

| Concept | Meaning |
|---|---|
| **Address** | `{application}.{name}` — for example `billing.invoice-pdf`. The application owns the function; the name is lower-case letters, digits and hyphens. The address never changes. |
| **Version** | One published build, numbered 1, 2, 3… Versions are immutable. |
| **Alias** | A name pointing at a version. `live` is what subscriptions, schedules and ordinary calls reach; other aliases (`qa`, `canary`) are reachable over HTTP only. |

### Old versions are pruned

A function keeps its **20 most recent untagged versions** (platform setting
`FC_FUNCTIONS_KEEP_VERSIONS`). After each publish, older untagged versions are
deleted for good: the version record and its stored artifact. A version is
*tagged* when any alias points at it (`live`, `canary`, `qa`…): tagged versions
are never deleted and do not count towards the 20. The newest version, and a
version published within the last hour that no runner has loaded yet, are also
never deleted. Version numbers are never reused, so `billing.invoice-pdf@v7`
never means two different builds; a pruned number simply stops existing. To
keep a build for good, point an alias at it.

A function that is already over the limit is pruned on its next publish. To do
it sooner, or to look first:

```
fcdev fn prune billing.invoice-pdf --dry-run
fcdev fn prune billing.invoice-pdf            # or --keep 10
```

(the API is `POST /api/functions/{id}/versions/prune`).

Publishing a version changes nothing that is running. Promoting a version to
`live` is the moment it takes effect — and the moment its subscriptions and
schedules are created, updated or removed to match what the version declares.
Promoting an older version back restores its wiring too.

## The local loop

```sh
fcdev fn init --lang go --app billing invoice-pdf   # or --lang rust / ts
cd invoice-pdf && go mod tidy                         # npm install for ts
fcdev fn build                                        # build + check the manifest
fcdev fn run function.wasm --config GREETING=Hi --watch
curl -H "Authorization: Bearer dev" localhost:8099/fn/local.fn/hello/Aroha
```

- `fcdev fn build` builds with the project's own toolchain and then reads the
  function's manifest exactly as publishing will, so a function that builds
  here publishes.
- `fcdev fn run` serves one function on the real runner with no platform:
  settings come from `--config`, `--secret` and `--db`; emitted events are
  printed; platform endpoints accept any bearer token; `--watch` picks up
  every rebuild.

## Publishing and promoting

```sh
fcdev fn deploy billing.invoice-pdf . --create   # build, publish, wait until loaded, promote live
fcdev fn set billing.invoice-pdf --config GREETING=Hi --secret STRIPE_KEY=sk_…
fcdev fn set billing.invoice-pdf --service-account bevco-integration   # who signs its webhook deliveries
fcdev fn publish billing.invoice-pdf function.wasm   # publish only
fcdev fn promote billing.invoice-pdf 7                # point live at version 7 (--alias for others)
fcdev fn invoke billing.invoice-pdf@qa hello/Aroha    # call through a runner
fcdev fn status billing.invoice-pdf
```

A version must be loaded by a runner before it can be promoted (`deploy` waits for this).
Promotion is refused while a config key, secret or database the version declares has no value.

### Secrets that point at AWS

A secret or database setting may be a **reference** to a secret kept in AWS instead of the secret, so it is
set and rotated in one place:

```sh
fcdev fn set billing.invoice-pdf --secret STRIPE_KEY=aws-sm://prod/billing/stripe#apiKey
fcdev fn set billing.invoice-pdf --secret ERP_TOKEN=aws-ps:///billing/np/erp-token
```

| Reference | Resolves to |
|---|---|
| `aws-sm://<name-or-arn>` | the secret's string value |
| `aws-sm://<name-or-arn>#<key>` | field `<key>` of the secret read as a JSON object (a string as is, a number or bool as its text; a missing, null, object or array field is an error) |
| `aws-ps://<parameter>` | the parameter's value, decrypted (a path parameter is `aws-ps:///path/to/param`) |

The platform stores the reference, never the secret, and reads the secret when a runner fetches the
function's settings, caching it for `FC_SECRET_REFERENCE_TTL` (5 minutes): a rotation in AWS reaches the
function within that time with nothing to change here. If AWS cannot be reached the last value is served for
up to `FC_SECRET_REFERENCE_MAX_STALE` (1 hour). A reference that cannot be resolved is **left out** of the
function's settings, so `secrets().require(...)` fails at the point of use, and the platform logs an error
naming the function, the key, the reference and the reason (`SECRET_REFERENCE_NOT_FOUND`, `_DENIED`,
`_KEY_MISSING`, `_UNAVAILABLE`, `_NOT_ALLOWED`). The platform only accepts references that start with one of
the prefixes in `FC_SECRET_REFERENCE_ALLOW` (otherwise `400 SECRET_REFERENCE_NOT_ALLOWED`), and its AWS role
must be allowed to read them (`secretsmanager:GetSecretValue`, `ssm:GetParameter`, `kms:Decrypt`). Other
schemes (`gcp-sm://`, `vault://`, `env://`) are refused (`400 SECRET_REFERENCE_UNSUPPORTED`). A reference is
not a secret: the settings list shows it, where a real secret is blank.
Against a local `fcdev` the commands sign in by themselves; in CI pass `--platform`,
`--client-id` and `--client-secret` (or `FC_PLATFORM_URL`, `FC_CLIENT_ID`, `FC_CLIENT_SECRET`)
for a service account holding `platform:function-deployer` (view, create, publish, promote, plus client and
application view; function manage also permits deleting functions), or `platform:function-publisher` to deploy
functions that already exist.

## Declaring what the function needs

Everything a function needs is declared **in its code**, and the platform
reads the declarations from the built artifact. There is no separate manifest
file to keep in step.

Go (`github.com/flowcatalyst/flowcatalyst/clients/fn-go`) — handlers are
plain `net/http` handlers:

```go
var greeting = fn.Config("GREETING")
var orders   = fn.DB("main")

func init() {
	fn.Webhook("POST /events/order-created", onOrderCreated)
	fn.Platform("GET /orders/{id}", getOrder)
	fn.Open("GET /healthz", health)

	fn.Subscribe("shop:orders:order:created", "/events/order-created")
	fn.Schedule("0 */15 * * * *", "/events/order-created")
	fn.HTTPAllow("api.stripe.com")
	fn.Emits("shop:orders:order:invoiced")
}

func main() {}
```

TypeScript (`@flowcatalyst/fn`) and Rust (`flowcatalyst-fn`) declare the same
things with the same names; see each SDK's README.

### Endpoints and how callers authenticate

Every endpoint names exactly one auth mode — there is no default:

| Declared with | Who calls it | What the runner checks first |
|---|---|---|
| `Webhook` | subscriptions and schedules | the platform's delivery signature |
| `Platform` | people and services holding platform tokens | a valid platform access token; your code then checks the caller's permissions (`Principal.HasPermission`) and client access (`CanAccessClient(id)` or `CanAccessClientCode("bevco")`) |
| `Open` | anyone | nothing — for health checks and third-party webhooks you verify yourself |

A webhook endpoint accepts `POST` only. Paths use Go's routing patterns:
`/orders/{id}`, `/files/{rest...}`.

#### Which service account signs a webhook delivery

A `Webhook` endpoint verifies the delivery signature with the signing secret of a service
account. By default that is the application's service account (the oldest active account linked
to the application). If the deliveries to your function are signed by another account — for
example a connection's own account — give the function that account:

```sh
fcdev fn deploy shop.orders . --create --service-account bevco-integration   # at creation
fcdev fn set shop.orders --service-account bevco-integration                 # change it
fcdev fn set shop.orders --clear-service-account                             # back to the application's
```

The value is the account's code (or its id). The platform checks the account exists, is active
and that you may use it. The API field is `serviceAccountId` on `POST /api/functions` and
`PATCH /api/functions/{id}` (`clearServiceAccount: true` to clear); reads return `serviceAccountId` and
`serviceAccountCode`. A function that names an account uses only that account's secret: if the
account is deactivated or deleted, the secret is empty and every webhook delivery is refused
(`401 INVALID_SIGNATURE`) — the platform logs the reason (`function webhook secret is empty`) and
does not quietly use the application's account instead. Service-account credentials are cached for
one minute, so a change or a rotated secret reaches runners within about a minute.

A delivery that arrives before the runner has the function's signing secret at all (the
account was only just attached and the runner has not yet fetched its next document) is
answered `503 WEBHOOK_SECRET_PENDING` with `Retry-After: 5`, **not** `401`. The platform
retries a `503`; it would not retry a `401`, and the delivery would be lost. This is the
same on the WebAssembly runner and on the ingestor's JVM host.

### Subscriptions and schedules

`Subscribe(eventType, path)` asks for an event type to be delivered to one of
your webhook endpoints; `Schedule(cron, path)` asks for a cron firing to be. The cron expression is
6-field, seconds first (`sec min hour day-of-month month day-of-week`, so
`0 */15 * * * *` is every 15 minutes); a 5-field expression is rejected at
publish.
The path must be a literal path that lands on a webhook endpoint. A
subscription without a mode keeps each message group in order and moves on
past a failure (`NEXT_ON_ERROR`), like every other subscription; ask for
`IMMEDIATE` if you do not need ordering.

## Trace logs

When something goes wrong in production you want to see what the function decided, without
paying to log everything all the time. Trace logs are opt-in and time-boxed: the function
writes to named **contexts**, and nothing is stored unless an operator switches a context on,
for a limited time. The rest of the time a trace call is a cheap no-op.

A **context** is a name (lowercase letter first, then lowercase letters, digits or `-`, at most
40 characters, at most 20 per function) that you declare in code, with an optional description
of up to 200 bytes of UTF-8 that operators see next to the switch. `http` is built in and needs no
declaration: while it is on, every call to the function also records its request, response,
timing and outcome, together with everything the call printed or logged. A function cannot
write to `http` itself.

```ts
import { fn } from "@flowcatalyst/fn";

fn.traceContexts({ name: "orders", description: "What the order mapping decided, line by line." });

fn.webhook("POST /events/order-created", (req) => {
  const order = req.json() as { tripNumber: string; lines: unknown[] };
  req.setReference(order.tripNumber); // the business key; stamps every later record of this call

  req.trace("orders", "mapping order", { lines: order.lines.length });
  if (req.tracing("orders")) {
    // only build an expensive payload when someone is looking
    req.trace("orders", "full order", { order }, { level: "debug" });
  }
  return fn.status(202);
});
```

| | Go | Rust |
|---|---|---|
| Declare | `fn.TraceContexts(fn.TraceContext{Name: "orders", Description: "..."})` | `app.trace_context("orders", "...")` |
| Record | `fn.Trace(ctx, "orders", "mapping order", map[string]any{"lines": n}, fn.WithLevel(fn.TraceWarn), fn.WithReference(trip))` | `ctx.trace("orders", "mapping order", Some(&data), TraceOpts::new().level(TraceLevel::Warn).reference(trip))` |
| Ask | `fn.Tracing(ctx, "orders")` | `ctx.tracing("orders")` |
| Business key | `fn.SetReference(ctx, trip)` | `ctx.set_reference(trip)` |

`level` is `debug`, `info` (the default), `warn` or `error`. `data` is an optional JSON object. A
`reference` is your business key (a trip or order number, at most 200 characters). `setReference`
tells the runner, which stamps it on every later record of the call, the built-in `http` and `log`
records included (a runner that predates this stamps only your own trace records). A reference
passed to a single trace call is that record's only; it does not replace the one set with
`setReference`. Tracing never fails a call: trace and tracing
calls never throw or return an error, whatever happens. If the context is off, was not sampled in,
is not declared (one warning per context is written to the function log) or the runner is an older
one that does not know trace logs, the call does nothing and `tracing` answers false. Ask `tracing`
before building anything expensive: its answer depends on the invocation, so do not remember it.

**Turning a context on.** Open the function's *Diagnostics* section (the switches), and its *View trace logs* page for the records, or run `fcdev fn trace on
<address> <context> --for 5m`. An operator picks how long (at most 60 minutes for your own
contexts, 15 for `http`, whose bodies can be sensitive) and may sample, recording one call in N. The
switch takes effect within about a second and turns itself off when the time is up. One of your own
contexts can also be left on *until switched off* (the choice in the dialog, which asks for
confirmation, or `fcdev fn trace on ... --no-limit`); it still switches itself off after at most
7 days (the platform's `FC_FUNCTIONS_TRACE_INDEFINITE_MAX_DAYS`), and it needs
`platform:function:trace:indefinite`. `http` never stays on until switched off: its 15-minute cap
always applies. Switching contexts needs `platform:function:trace:manage` and reading records needs
`platform:function:trace:view`, which shows each record's time, context, kind, level, invocation,
correlation id, reference and version, and of an `http` record only its method, path (without the
query string), status, duration, outcome and body sizes. The rest (request and response headers,
the query string, the bodies, the lines the call printed or logged, and the `data` of your trace
records) needs `platform:function:trace:view-bodies`, which switching `http` on needs too. The
read-only `platform:viewer` role does not read trace records. Switching a context on or off is
recorded in the audit log.

**From the command line.** `fcdev fn trace` takes the same platform, profile and credential flags
as the other `fn` commands.

```sh
fcdev fn trace on  billing.invoice-pdf orders --for 10m --sample 2   # live until 12:20:45 (10m)
fcdev fn trace off billing.invoice-pdf orders
fcdev fn trace status billing.invoice-pdf        # contexts, live or off, time left, sample
fcdev fn trace log billing.invoice-pdf --reference T-1042 --since 30m
fcdev fn trace log billing.invoice-pdf --context http --bodies -f
fcdev fn trace run billing.invoice-pdf orders    # on, follow, and back as it was on Ctrl-C
```

`--for` takes a duration (`5m`, `1h`) and is rounded up to whole minutes; `--sample N` records one
call in N. `log` prints the newest `--limit` records (default 100), oldest first, filtered by
`--context`, `--kind`, `--level`, `--reference`, `--correlation`, `--invocation`, `--version`,
`--since` and `--until` (a duration back from now, an RFC 3339 time or unix milliseconds). An `http`
record is one line, `POST /orders -> 200 (12ms, ok)`; `--bodies` adds the request and response
bodies (JSON is pretty-printed; base64 and truncation are noted, and bodies are hidden without
`platform:function:trace:view-bodies`). `-f` keeps polling every 2 seconds and prints only new
records until Ctrl-C; `--json` prints one raw JSON record per line. `run` shows only records from
now on (unless `--since` says otherwise) and stops when the window ends. If the context is already
on for at least as long as `--for`, `run` only follows it and leaves it on; otherwise it switches it
on and, on exit, puts it back as it found it (off, or the window it had with its time left) unless
you pass `--keep`. A window someone else changes while `run` is following is left as they set it. A CI credential with only `platform:function-deployer` may not use
trace logs.

**Limits.** A message is at most 4096 bytes and `data` at most 32768 bytes (a record over the limit
is dropped, never an error), and the platform keeps up to 200 records a second per function; the
excess is counted as dropped (the limit is per platform process). One call keeps at most 500 log
records and 500 trace records. HTTP bodies are cut at 64 KiB. The value of any request or response
header whose name contains `auth`, `key`, `token`, `secret`, `password`, `signature`, `cookie` or
`session` is replaced by `[redacted]` (so `authorization`, `x-api-key`, `x-functions-key`,
`ocp-apim-subscription-key`, `set-cookie` and the delivery signature `x-flowcatalyst-signature`
are never stored), and so is the value of any query parameter with one of those names or named
`access_token`, `code`, `token`, `sig`, `signature`, `X-Amz-*`, `key`, `apikey`, `api_key`,
`password` or `secret`.

**What is stored, and for how long.** Each record has the time, function version, invocation id,
correlation id (the request's `X-Correlation-Id`), reference, context, kind (`trace`, `http`, `log` or, for a JVM function, `outbound`),
level, message and data. Records are kept for 30 days after they reach the platform and then
deleted. Records are shipped
by the runner in the background, so they survive whatever happens to the call itself.

## What a function can reach

| Capability | Notes |
|---|---|
| Config and secrets | Only declared keys. Values are set on the platform per function; secrets are write-only. Promotion is refused while a declared key has no value. |
| Databases | `fn.DB("main")` gives a `database/sql` handle (Go) or `query`/`exec`/`transaction` (Rust, TS). The connection string is a platform setting; the pool belongs to the runner. Statements are bound with `?` placeholders, results come back at most 10 000 rows or 8 MiB, and a transaction still open when the call ends is rolled back. |
| Outbound HTTP | Only to hosts in `HTTPAllow` (`api.example.com` or `*.example.com`), including on redirects. Responses over 16 MiB are refused. |
| Dispatching work | Only `(function, path)` pairs declared with `Dispatches` (`fn.dispatches` in TypeScript), and only functions of your own application whose live version has a signed `webhook` endpoint at that path. The platform creates the dispatch job, builds the URL, and the delivery is signed by the service account of the dispatch connection the job names (a connection is required; a dispatch pool is optional). The receiving function verifies with its own webhook service account (see above), so the two must be the same account; you hold no credential. It is not de-duplicated: a retry after an ambiguous failure can create a second job. |
| Calling other applications | A function that declares the platform token gets a FlowCatalyst access token from its host, to send as a bearer to another first-party application that accepts FlowCatalyst tokens. The token is the one of the service account linked to your function's **application** (not the function's webhook service account), with that account's permissions, and lives as long as an ordinary access token; the host caches it and renews it shortly before it expires, and the call can force a fresh one (for example after a 401 because the account's roles changed). You hold no client id or secret, and must never log the token. Refused when the application has no linked service account or the account is deactivated. |
| Emitting events | Only types declared with `Emits`, and only your application's own. Every event needs a dedup id — derive it from what you are handling (for example the delivered event's id) so a redelivery emits once. |
| Logging | `fn.Log(ctx)` (Go), `ctx.log_*` (Rust), `fn.log` and `console` (TS). |

Every call has a deadline, and it reaches databases, HTTP and emits too: when
the call's time is up, everything it started stops.

## Answering deliveries

| Your answer | What the platform does |
|---|---|
| any 2xx | delivered |
| `fn.Retry(w, d)` — 429 with `Retry-After` | tries again after the delay, without spending an attempt |
| `fn.Reject(w, reason)` — 422 with `FlowCatalyst-Outcome: reject` | gives up at once: retrying cannot help |
| anything else, a crash or a timeout | a failed attempt, retried until the subscription's or schedule's limit |

## Limits

| Limit | Default | Set per function |
|---|---|---|
| Memory per instance | 64 MB | yes |
| Concurrent calls | 16 | yes |
| Timeout | 30 s | yes, and per endpoint |
| Request body | 1 MiB | yes, and per endpoint |

A function past its concurrency limit answers `429` (deliveries wait and try
again without spending an attempt). One that needs more memory than it may
have fails only its own call — never its neighbours.

## JVM functions

A function may also be written in Java (25) and published as a **jar**. It is the same
function in every other way — addresses, versions, aliases, declarations, subscriptions,
schedules, settings, promotion — but it runs in the **ingestor runtime** instead of the
WebAssembly runner. The jar is first-party code like any other function; the class-loader
filter that keeps a function away from the host's internals is fault isolation, not a sandbox.

### How it runs

- The host loads **one instance of your `Function` class per published version** and calls
  it for every request at once, **each call on its own virtual thread**. Nothing queues
  behind anything else. Keep the handler **stateless and thread-safe**: no mutable fields,
  or guard them.
- The function's **concurrency limit** (default 16, settable per function like any other) is a
  permit count: a call that finds none free is answered `429 FUNCTION_BUSY` with
  `Retry-After: 1` at once, never queued. Deliveries treat that as a deferral.
- Each call has a deadline (the endpoint's `timeoutMs`, else the function's, default 30 s). A
  call past it is answered `504 FUNCTION_TIMEOUT`; its permit comes back when the handler
  really returns. A handler that throws is answered `500 FUNCTION_FAILED` (the exception is
  in the host's log, not in the answer).
- The pool, `warm` and the limits are **platform settings** of the function, as for every
  runtime; none of them is in the jar.
- A function is **JVM or not from its first version**. Publishing a jar for a function that has
  Wasm or JS versions, or the reverse, is refused `RUNTIME_MISMATCH`.

### Writing one

One class implements `io.flowcatalyst.function.Function`:

```java
public final class HelloFunction implements Function {

    @Override
    public void init(FunctionContext ctx) {            // once, before the first call
        ctx.logger().log(Level.INFO, "starting {0} v{1}", ctx.address(), ctx.version());
    }

    @Override
    public Result handle(Request in, FunctionContext ctx) throws Exception {
        return switch (in.route()) {                  // the pattern the describe declares
            case "GET /healthz" -> Result.json(200, "{\"status\":\"ok\"}");
            case "GET /hello/{name}" -> hello(in);
            case "POST /events/greeting-requested" -> greetingRequested(in, ctx);
            default -> Result.fail("no handler for " + in.route());
        };
    }

    @Override
    public void stop() { }                            // once, after the last call returned
}
```

The host routes and authenticates **before** `handle` runs, so a path is always one the
describe declares. A class needs a public no-argument constructor.

**`Request`** carries `id`, `address`, `version`, `method`, `path`, `rawQuery`, `headers`,
`route` (the matched pattern, `GET /hello/{name}`), `pathParams`, `caller`, `deadline`, `host`,
`remoteAddr`, `isPublic` and the body (`body()`, `bodyText()`); `header(name)` and `query()`
match as you would expect.

**`Caller`** is one of three, one per endpoint `auth`:

| `auth` | `Caller` | Meaning |
|---|---|---|
| `webhook` | `Platform` | A verified delivery (subscription, dispatch job or schedule). Parse the body with `Webhook.event(in)`, `Webhook.dataOnly(in)` or `Webhook.schedule(in)` |
| `platform` | `Principal` | A platform access token the host verified: `id`, `type`, `tier`, `clients`, `clientCodes`, `roles`, `applications`, `permissions`, with `hasPermission`, `canAccessClient`, `canAccessClientCode`, `canAccessApplication` and `canAccessApplicationCode` |
| `none` | `Anonymous` | Nothing was checked; the function authenticates for itself |

**`Result`** is the HTTP answer, with the delivery conventions as factories:

| Factory | Answer | A delivery treats it as |
|---|---|---|
| `Result.ack()` | `200` | delivered |
| `Result.retry(Duration)` | `429` + `Retry-After` | try again after the delay, without spending an attempt |
| `Result.reject(reason)` | `422` + `FlowCatalyst-Outcome: reject` | give up at once |
| `Result.fail(reason)` | `500` `{"error": …}` | a failed attempt, retried on the schedule |
| `Result.json(status, json)`, `text`, `status`, `http` | what you say | what a browser or API client receives |

**`FunctionContext`** is everything a function may reach, limited to what the describe declares:

| Service | Notes |
|---|---|
| `logger()` | A `System.Logger` carrying the function, version and invocation id |
| `config()`, `secrets()` | `get(key)` and `require(key)` for the declared keys; secret values never appear in a `toString` or a log |
| `dataSource(name)` | A JDBC `DataSource` for a declared database `name`. **PostgreSQL only.** The platform setting is the function's DSN (`postgres://…` or `key=value`); the host keeps **one pool per distinct DSN, shared by every function** bound to it (default 4 connections a pool, 32 pools in all, an idle pool closed after 10 minutes and made again by the next call). Borrow a connection per call and close it; never keep one. A DSN the host cannot represent (a Unix socket, an unknown `sslmode`) is refused with a message that never echoes it |
| `http()` | Outbound calls only to the hosts in `httpAllow` (`api.example.com`, or `*.example.com` for subdomains, not the apex); `https` only (plain `http` to loopback); redirects are not followed, you see the 3xx; a response over 16 MiB fails the call; the timeout is the shorter of your call's (default 30 s) and the time left before the deadline |
| `events().emit(OutboundEvent)` | Only types listed in `emits`, only your application's; a dedup id is required. Answers `Emitted` or `Refused` (with `retryable()`), never throws for either |
| `dispatcher().dispatch(DispatchJob)` | Only `(target, path)` pairs declared in `dispatches`; `DispatchJob.to(target, path)` builder; not deduplicated. `DispatchException` says whether a retry makes sense (`retryable()`) |
| `platformToken()`, `platformToken(boolean refresh)` | The access token (a `String`) of the application's linked service account, for calling other first-party applications; requires `platformToken: true` in the describe. `PlatformTokenException` carries the code and `retryable()` |
| `clock()` | The host's clock; never read the system clock directly, so a test can fix time |
| `address()`, `version()` | This function and the loaded version |
| `trace(context, message)`, `trace(context, message, data)`, `trace(context, message, data, TraceOptions)` | Writes a record to a trace context you declared in `traceContexts` of the describe (`[{ "name": "orders", "description": "..." }]`); see "Trace logs". Never throws; a no-op unless an operator switched the context on and this call was sampled in. `data` is a `Map` (at most 32768 bytes of JSON); `TraceOptions(level, reference)` sets `debug`/`info`/`warn`/`error` and this record's own reference |
| `tracing(context)` | Whether the context is on for this call, to guard building something expensive; `http` can be asked too |
| `setReference(reference)` | The call's business key (at most 200 characters), stamped on every later record of the call, the `http` record included |
| `conceal(value)` | Keeps a credential the function obtained at run time (a session id, a token) out of every trace record, in a URL's path as well as in bodies and queries; call it whenever one is obtained, in `init` too. Declared secrets need no call |

For unit tests without a host, the API jar has `FakeFunctionContext`, `FakeRequest`,
`RecordingEvents` and `RecordingDispatcher` in `io.flowcatalyst.function.testing`.

**What the host provides and what the jar bundles.** The host gives a function the **JDK**, the
**function API** (`io.flowcatalyst.function`), the **FlowCatalyst framework**
(`io.flowcatalyst.framework.*`), the **Jackson** the framework exposes (`tools.jackson`, and
Jackson annotations), **SLF4J** and **jspecify**. Everything else the function bundles in its own
jar (a fat jar). Mark the function API and the framework `provided`. A jar that contains the API's
own package is refused; so is one with a native library (`.so`, `.dll`, `.dylib`) or a registered
`java.security.Provider`. A function cannot load the ingestor's own classes or a JDBC driver of the
host's: use `dataSource`.

### The describe document

The jar holds `META-INF/flowcatalyst/function.json`: the same describe document every function has
(`abi`, `endpoints`, `subscriptions`, `schedules`, `config`, `secrets`, `db`, `httpAllow`, `emits`,
`dispatches`, `traceContexts`), plus a `jvm` block naming the entrypoint class. A method-less endpoint matches every
method; list one endpoint per method otherwise.

```json
{
  "abi": 1,
  "jvm": { "entrypoint": "io.flowcatalyst.example.hello.HelloFunction" },
  "endpoints": [
    { "method": "GET",  "path": "/healthz", "auth": "none" },
    { "method": "GET",  "path": "/hello/{name}", "auth": "platform" },
    { "method": "POST", "path": "/events/greeting-requested", "auth": "webhook" }
  ],
  "subscriptions": [
    { "eventType": "hello:greeting:greeting:requested", "path": "/events/greeting-requested", "mode": "IMMEDIATE" }
  ],
  "config": ["GREETING"],
  "emits": ["hello:greeting:greeting:sent"]
}
```

Publishing reads this entry **without running anything** (at most 1 MiB, at most 50,000 jar
entries; the jar itself at most 64 MiB) and checks it as it checks a Wasm function's describe,
plus that `jvm.entrypoint` is present. A jar without it is refused.

### Building and deploying

Build with Maven. The function API is installed once into your local repository (`make
function-api-install` in the platform repository), and both it and the framework are `provided`:

```xml
<dependency>
  <groupId>io.flowcatalyst.ingestor</groupId><artifactId>function-api</artifactId>
  <version>${ingestor.version}</version><scope>provided</scope>
</dependency>
<dependency>
  <groupId>io.flowcatalyst.framework</groupId><artifactId>framework-http</artifactId>
  <version>${flowcatalyst-framework.version}</version><scope>provided</scope>
</dependency>
```

Put `function.json` in `src/main/resources/META-INF/flowcatalyst/`, build the jar (a shade plugin
if you bundle dependencies), and deploy it like any function. The runtime is picked from the
`.jar` extension:

```sh
fcdev fn deploy hello.greeting target/function-hello-0.1.0.jar --create
fcdev fn set hello.greeting --config GREETING=Howdy
fcdev fn promote hello.greeting 1        # if deploy stopped for the missing GREETING value
```

A jar cannot be run with `fcdev fn run` or `fcdev fn dev`: they run Wasm and JS. Deploy it to a
local `fcdev` (below) and call it there. The example, with its walkthrough, is
`ingestor/examples/function-hello` in the platform repository.

### Locally, with fcdev

`fcdev` starts the ingestor runtime beside the platform and points the platform's JVM function
URL at it, so JVM functions work with no setup. From a checkout of the platform repository:

```sh
make build                 # the platform, the ingestor jar and the function API
make function-hello        # the example jar
./bin/fcdev start
curl http://127.0.0.1:8091/fn/hello.greeting/healthz          # the ingestor's port (FC_INGESTOR_PORT)
```

Use the `fcdev` built by `make build` (`./bin/fcdev`): an installed `fcdev` that predates JVM
functions rejects a jar as an invalid Wasm module.

### Setting it up where you deploy

- **The runner URL.** Set `FC_FUNCTIONS_RUNNER_URL_JVM` on the platform to the ingestor's URL
  (`{pool}` is substituted, as in `FC_FUNCTIONS_RUNNER_URL`). A delivery to a function whose live
  version is a jar is built from it; the others use `FC_FUNCTIONS_RUNNER_URL`.
- **The ingestor's account.** The ingestor's service account must hold `platform:function-runner`
  as well as `platform:ingestion-runtime`.
- **The roles.** `INGEST_FUNCTIONS_ENABLED` (default `true`) turns the function role on and
  `INGEST_FUNCTIONS_POOL` (default `default`) names the pool it serves. `INGEST_INGESTION_ENABLED`
  (default `true`) turns the ingestion role on: set it `false` for a functions-only process (it
  takes no ingestion leases and serves no `/ingest/` endpoint) and `INGEST_FUNCTIONS_ENABLED=false`
  for an ingestion-only one.
- **Databases.** `INGEST_FUNCTIONS_DB_MAX_POOLS` (32), `INGEST_FUNCTIONS_DB_MAX_CONNS` (4 a pool) and
  `INGEST_FUNCTIONS_DB_IDLE_TTL` (`PT10M`).
- **Logs.** A function's log is limited so that a loop in one function cannot flood the log: each
  function address may write `INGEST_FUNCTIONS_LOG_RATE` lines a second (100) with a burst of
  `INGEST_FUNCTIONS_LOG_BURST` (1000). Lines over the budget are dropped and counted, and the host
  writes one warning a minute for a function that dropped any ("function X dropped N log lines in
  the last minute (limit 100/s)"). A message longer than 8 KiB is cut and ends "(cut, M bytes)"; an
  exception keeps its stack trace, with its message cut the same way. A function that fails on
  every call is logged in full once (with the stack trace), then as one line a minute with a count.
- A request body is also capped by the ingestor server's own request-body limit, whatever the function's limit says.

### Trace logs for a JVM function

Declare the contexts in the describe (`"traceContexts": [{ "name": "orders", "description": "What the mapping decided" }]`)
and write to them with `ctx.trace`; everything under "Trace logs" applies (switches, sampling, limits, the
`fcdev fn trace` commands). A JVM function has the built-in `http` context too: while it is on, each call records its
`http` record, the lines the function logged through `ctx.logger()` (every level, whatever the host's log level) and an
`outbound` record for each call it made with `ctx.http()`. The `outbound` record has the shape of an ingestion's `http`
trace record (client, request URL, headers and body, response, duration). The host replaces your declared secrets, the
values you give `ctx.conceal` and any credential an outbound answer hands out (`SessionToken`, `access_token`, ... in a JSON
answer) wherever they appear in a record, and every query value but paging and time-window names (`limit`, `offset`,
`page`, `since`, `until`, ...) is `[redacted]`, because a vendor may take a password or a session in a parameter of any name.

```java
@Override
public void init(FunctionContext ctx) throws Exception {
    ctx.conceal(login(ctx));   // a session obtained outside any call is kept out of traces too
}

@Override
public Result handle(Request in, FunctionContext ctx) {
    ctx.setReference(in.pathParams().get("trip"));
    ctx.trace("orders", "mapping order", Map.of("lines", 3));
    if (ctx.tracing("orders")) {
        ctx.trace("orders", "full order", Map.of("order", order), new TraceOptions("debug", null));
    }
    return Result.ack();
}
```

### What a JVM function cannot do yet

Public routes and domains, per-function metrics and a readiness check are not built for JVM
functions. A PEM `sslkey` in a database DSN is passed to the driver as
it is, not converted.

## Stability

Source written against an SDK keeps building and behaving the same until that SDK's next
major version, and a change of runner never requires changing function source. What is promised,
what is frozen, the JavaScript environment a TypeScript function can rely on, and how changes are
deprecated are in the [function stability contract](../function-stability-contract.md).

## Running the runner

The runner is part of `fc-server`: `FC_FUNCTIONS_ENABLED=true` starts it, and
a runner-only deployment needs no database. It authenticates to the platform
as a service account holding the `platform:function-runner` role, learns what
to run from the platform, and uses all of its container's memory except a
reserve for itself. In `fcdev` it runs in-process on port 8095 (the public
entry on 8096).

## Public routes

A function's own endpoints (`Webhook`, `Platform`, `Open`) are always
reachable through `/fn/{address}[@alias]/{path}`. A **public route** maps a
plain hostname and path prefix straight to a function, so `https://api.acme.
example.com/webhooks/orders` can reach `shop.orders` without the `/fn/…`
shape — the prefix is stripped before the function's own endpoint matching,
so the function still declares `/orders` as usual.

Adding a route is two steps:

1. **Claim the zone.** A zone claim covers the hostname itself and everything
   under it — claiming `acme.example.com` also covers `api.acme.example.com`.
   A claim is verified by being made; there is no DNS check. Two different
   owners cannot claim overlapping zones (in either direction); the same
   owner can.

   ```sh
   curl -X POST https://platform.example.com/api/function-domains \
     -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
     -d '{"zone": "acme.example.com", "clientId": "clt_…"}'   # omit clientId for a platform zone (anchor only)
   ```

   Claiming and releasing a zone needs `platform:function:domain:manage`.
   Releasing one is refused while a route still uses a hostname it covers.

2. **Add the route.** The hostname must be covered by a zone your client (or,
   for a platform-owned function, a platform zone) has claimed.

   ```sh
   curl -X PUT https://platform.example.com/api/functions/$FUNCTION_ID/routes \
     -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
     -d '{"hostname": "api.acme.example.com", "pathPrefix": "/webhooks", "alias": ""}'
   ```

   `alias` names an existing alias to route to; empty routes to `live`. A
   `(hostname, pathPrefix)` pair belongs to exactly one function at a time —
   claiming one already routed elsewhere is refused. Adding, changing or
   removing a route takes effect immediately, the same way a promote does.
   Writing routes needs `platform:function:route:manage`; listing a
   function's own routes needs the ordinary `platform:function:function:view`.

For DNS to actually deliver traffic, point `api.acme.example.com` at the
runner pool's public entry — the same port `fcdev`'s `8096` stands in for
(a load balancer or ingress in front of the runner deployment in
production). The route only decides which function answers once a request
reaches that port; it is not itself a DNS record.
