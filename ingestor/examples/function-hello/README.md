# function-hello

A JVM function served by the ingestor runtime (`docs/jvm-functions-in-ingestor.md`), ported from
flowcatalyst-javalin's example. One class, `HelloFunction`, and its describe document
`src/main/resources/META-INF/flowcatalyst/function.json`:

| Endpoint | Auth | What it does |
|---|---|---|
| `GET /healthz` | none | `{"status":"ok"}` |
| `GET /hello/{name}` | platform token | greets the caller |
| `POST /events/greeting-requested` | webhook (signed delivery) | the subscription to `hello:greeting:greeting:requested`; emits `hello:greeting:greeting:sent` |

The function API and the framework are `provided`: the host gives them to every function, so the jar
holds only the function's own classes and `function.json`. One instance serves every call at once, each
on its own virtual thread, so a function keeps no mutable state of its own.

## Run it locally

`fcdev start` (after `make ingestor-build`) runs the ingestor, which serves JVM functions on its port (`FC_INGESTOR_PORT`, default
8091; `fcdev ingestor status` shows it). Then:

```sh
make function-hello      # installs the function API into ~/.m2, then builds this jar
fcdev fn deploy hello.greeting ingestor/examples/function-hello/target/function-hello-0.1.0.jar --create
fcdev fn set hello.greeting --config GREETING=Howdy
fcdev fn promote hello.greeting 1          # if deploy stopped for the missing GREETING value
curl http://127.0.0.1:8091/fn/hello.greeting/healthz
```

The application `hello` must exist with an active service account (it signs the subscription's
deliveries), and so must the two event types.
