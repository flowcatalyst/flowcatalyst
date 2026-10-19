# Functions

A **function** is a small program the platform stores, versions and runs for
you. You write it in Go, Rust or TypeScript against a FlowCatalyst SDK,
publish it, and promote a version to `live`. The platform then wires it up:
subscriptions deliver events to it, schedules fire it, and callers invoke it
over HTTP — with no server of yours to run.

Functions run in the **function runner**, a sandbox built on WebAssembly. A
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

## What a function can reach

| Capability | Notes |
|---|---|
| Config and secrets | Only declared keys. Values are set on the platform per function; secrets are write-only. Promotion is refused while a declared key has no value. |
| Databases | `fn.DB("main")` gives a `database/sql` handle (Go) or `query`/`exec`/`transaction` (Rust, TS). The connection string is a platform setting; the pool belongs to the runner. Statements are bound with `?` placeholders, results come back at most 10 000 rows or 8 MiB, and a transaction still open when the call ends is rolled back. |
| Outbound HTTP | Only to hosts in `HTTPAllow` (`api.example.com` or `*.example.com`), including on redirects. Responses over 16 MiB are refused. |
| Dispatching work | Only `(function, path)` pairs declared with `Dispatches` (`fn.dispatches` in TypeScript), and only functions of your own application whose live version has a signed `webhook` endpoint at that path. The platform creates the dispatch job, builds the URL and signs the delivery as your application; you hold no credential. It is not de-duplicated: a retry after an ambiguous failure can create a second job. |
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
