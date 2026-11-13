# function-hello

A JVM function served by the ingestor runtime (`docs/jvm-functions-in-ingestor.md`), ported from
flowcatalyst-javalin's example. One class, `HelloFunction`, and its describe document
`src/main/resources/META-INF/flowcatalyst/function.json`:

| Endpoint | Auth | What it does |
|---|---|---|
| `GET /healthz` | none | `{"status":"ok"}` |
| `GET /hello/{name}` | platform token | greets the caller |
| `POST /events/greeting-requested` | webhook (signed delivery) | the subscription to `hello:greeting:greeting:requested`; emits `hello:greeting:greeting:sent` |

A `platform token` endpoint is answered only for a caller that reaches the function's client: an anchor-tier caller
always, otherwise a caller whose clients include the function's client (a function with no client answers anchor
callers only). Any other caller gets `403 FORBIDDEN` before the body is read. An endpoint meant for callers of any
client declares `"reach": "any"` in `function.json` (valid on `auth: "platform"` only), and its handler then does its
own checks on `in.caller()`. `GET /hello/{name}` here keeps the default, so it needs no `reach`. This changed on
2026-11-13: an endpoint that relied on any client's token being enough must now declare `"reach": "any"`.

The function API and the framework are `provided`: the host gives them to every function, so the jar
holds only the function's own classes and `function.json`. One instance serves every call at once, each
on its own virtual thread, so a function keeps no mutable state of its own.

A function that calls another first-party application declares `platformToken` and calls `ctx.platformToken()`: see
[docs/function-platform-token.md](../../docs/function-platform-token.md). This example does not use it.

## Run it locally

From a checkout of the platform repository, with a JDK 25 (building) and nothing else installed:

```sh
make build               # the platform (frontend + Go binaries), the ingestor jar and the function API in ~/.m2
make function-hello      # this jar: ingestor/examples/function-hello/target/function-hello-0.1.0.jar
./bin/fcdev start        # runs the ingestor beside the platform; it serves JVM functions on its port
```

Use `./bin/fcdev`, the binary `make build` made: an installed `fcdev` that predates JVM functions (0.11.5 and
older) rejects a jar as an invalid Wasm module. `make build` skips the ingestor jar while a running `fcdev` holds
it; stop `fcdev`, run `make ingestor-build`, and start it again to pick up a rebuilt ingestor.

The ingestor listens on `FC_INGESTOR_PORT` (default 8091; `./bin/fcdev ingestor status` shows it) and `fcdev`
points the platform's `FC_FUNCTIONS_RUNNER_URL_JVM` at it. Then, once:

1. **The application `hello`, with a service account.** Create it in the console (Applications) or with
   `POST /api/applications` (`{"code":"hello","name":"Hello"}`), then give it a service account:
   `POST /api/applications/{id}/provision-service-account`. The service account signs the subscription's
   deliveries, and the function verifies them with it; without one the function cannot verify a delivery
   (it answers `503 WEBHOOK_SECRET_PENDING` until it has the secret).
2. **The two event types** the describe names, in the console (Event types) or with `POST /api/event-types`:
   `hello:greeting:greeting:requested` and `hello:greeting:greeting:sent`.

And for every build:

```sh
./bin/fcdev fn deploy hello.greeting ingestor/examples/function-hello/target/function-hello-0.1.0.jar --create
./bin/fcdev fn set hello.greeting --config GREETING=Howdy
./bin/fcdev fn promote hello.greeting 1          # if deploy stopped because GREETING had no value yet
curl http://127.0.0.1:8091/fn/hello.greeting/healthz
./bin/fcdev fn invoke hello.greeting hello/Aroha --runner http://127.0.0.1:8091   # a platform-token call
```

`deploy` publishes the jar (the runtime, `jvm`, is chosen from the `.jar` extension and is fixed for the function
from then on), waits until the ingestor has loaded the version, and promotes it to `live` when every declared
config key has a value. Promoting is what creates the subscription to `hello:greeting:greeting:requested`: publish
that event (for example from the console) with `{"name": "Aroha"}` as its data, and the function emits
`hello:greeting:greeting:sent`.

A function that is JVM from its first version cannot take a Wasm or JS version later (`RUNTIME_MISMATCH`).
