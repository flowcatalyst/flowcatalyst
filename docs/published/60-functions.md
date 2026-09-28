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

## Declaring what the function needs

Everything a function needs is declared **in its code**, and the platform
reads the declarations from the built artifact. There is no separate manifest
file to keep in step.

Go (`github.com/flowcatalyst/flowcatalyst-go/clients/fn-go`) — handlers are
plain `net/http` handlers:

```go
var greeting = fn.Config("GREETING")
var orders   = fn.DB("main")

func init() {
	fn.Webhook("POST /events/order-created", onOrderCreated)
	fn.Platform("GET /orders/{id}", getOrder)
	fn.Open("GET /healthz", health)

	fn.Subscribe("shop:orders:order:created", "/events/order-created")
	fn.Schedule("*/15 * * * *", "/events/order-created")
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
| `Platform` | people and services holding platform tokens | a valid platform access token; your code then checks the caller's permissions (`Principal.HasPermission`) |
| `Open` | anyone | nothing — for health checks and third-party webhooks you verify yourself |

A webhook endpoint accepts `POST` only. Paths use Go's routing patterns:
`/orders/{id}`, `/files/{rest...}`.

### Subscriptions and schedules

`Subscribe(eventType, path)` asks for an event type to be delivered to one of
your webhook endpoints; `Schedule(cron, path)` asks for a cron firing to be.
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

## Running the runner

The runner is part of `fc-server`: `FC_FUNCTIONS_ENABLED=true` starts it, and
a runner-only deployment needs no database. It authenticates to the platform
as a service account holding the `platform:function-runner` role, learns what
to run from the platform, and uses all of its container's memory except a
reserve for itself. In `fcdev` it runs in-process on port 8095 (the public
entry on 8096).
