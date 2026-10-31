# JVM functions, hosted by the ingestor runtime

*Owner rulings 2026-10-31. Supersedes the runner shape of `docs/java-function-runner-plan.md`
(standalone `runners/jvm`, `fcdev runner install`); that document stays as background for the
contract gap analysis (§1), security (§7) and risks (§8.2).*

## 0. Rulings

| # | Ruling |
|---|---|
| R1 | JVM functions are served by the **ingestor runtime** (Java 25, framework Jetty on virtual threads), as a second role next to ingestion. fcdev, which already starts the ingestor, gets JVM functions by default. Whether deployed ingestors and function hosts are separate deployments is a later, config-only choice (one artifact, two roles). |
| R2 | **Copy from `../flowcatalyst-javalin`** (`function-api`, `function-host`) and change what our platform's API needs; do not redesign. |
| R3 | **Concurrent**: one loaded instance per function version serves every call at once, each call on its own virtual thread; only the per-function permit count and the host cap limit concurrency. Nothing serialises calls of one function. |
| R4 | A function sees the **JDK, the function API and the framework** (with the Jackson it exposes) from the host; everything else it bundles. |
| R5 | Runtime tag `jvm`, fixed per function (set by its first version; a different runtime later is `RUNTIME_MISMATCH`). Pool, warm and limits are **platform settings**, as for Wasm functions, not in the jar. |
| R6 | Functions are first-party only (2026-10-21): the class-loader filter is fault isolation, not a sandbox. |

## 1. What a JVM function is

A jar (fat for its own dependencies, not for R4's shared ones) with
`META-INF/flowcatalyst/function.json`: **our describe document** (`internal/functions/abi`
`Describe`: `abi`, `endpoints`, `subscriptions`, `schedules`, `config`, `secrets`, `db`,
`httpAllow`, `emits`, `dispatches`, `traceContexts`) plus one block:

```json
{ "abi": 1, "jvm": { "entrypoint": "com.example.HelloFunction" },
  "endpoints": [ { "method": "GET", "path": "/hello/{name}", "auth": "platform" } ], … }
```

This replaces javalin's `manifest.json`: its `endpoints[].methods` (a list) becomes one endpoint per
method (`method`, empty = any); `subscriptions`, `config`, `secrets` carry over as they are; its
`runtime`, `pool`, `warm`, `limits` are dropped (R5). The entrypoint class implements the API's
`Function` (`init`, `handle`, `stop`), as in javalin.

## 2. Function API (`ingestor/function-api`, package `io.flowcatalyst.function`)

Copied from javalin `function-api`; zero dependencies beyond the JDK; `--release 25`. Changes:

1. `FunctionAddress`: our two labels `{applicationCode}.{name}` (`application()`, `name()`,
   `render()`, `parse`), not javalin's three.
2. `Request`: the fields of our `abi.Request` (`id` = invocation id, `address`, `version`, `method`,
   `path`, `rawQuery`, `headers`, `route`, `pathParams`, `caller`, `deadline`, `host`, `remoteAddr`,
   `isPublic`) plus the body bytes.
3. `Caller`: `Platform` (a verified webhook delivery), `Anonymous` (`auth: none`), `Principal` with
   our `abi.Caller` fields (`id`, `type`, `tier`, `clients`, `clientCodes`, `roles`, `applications`,
   `allApplications`, `permissions`) and the helpers fn-ts offers (`hasPermission`,
   `canAccessClient`, `canAccessClientCode`, `canAccessApplication` with the `id:code` rule).
4. `Result`: keep; add the webhook result conventions of `clients/fn-ts` (`reject` = not retried,
   `ack`, …) with exactly fn-ts's statuses and bodies.
5. `FunctionContext`: keep `logger`, `config`, `secrets`, `dataSource(name)`, `http`, `events`,
   `clock`, `address`, `version`; **add `dispatcher()`** (a dispatch job to a declared
   `dispatches` target: `target`, `path`, `payload`, `contentType`, `messageGroup`,
   `correlationId`, `metadata`, `mode`, mirroring fn-ts `fn.dispatch`). Function traces
   (`traceContexts`) are a later slice; leave a note, not an API.
6. `Webhook` / `Event`: parse our delivery envelope (`id`, `type`, `source`, `subject`,
   `correlationId`, `messageGroup`, `clientId`, `clientCode`, `time`, `dataContentType`,
   `attemptNumber`, `data`) and a `dataOnly` body; `Schedule`: our scheduled-job firing body.
7. A test helper (`FakeFunctionContext` style, from javalin's example) so a function is unit-tested
   without a host.

## 3. The host (ingestor runtime, package `io.flowcatalyst.ingestor.functions`)

Copied from javalin `function-host`, keeping its engine and replacing its platform edges.

**Keep (adapt names/packages):** `load/` (`JvmFunctionLoader` with the metaspace fence and jar
scan, `LoadedFunction` with retain/release drain, the parent loader — widened per R4 to the JDK,
`io.flowcatalyst.function`, `io.flowcatalyst.framework..` and the Jackson packages the framework
exposes), `http/InvocationRunner` (virtual thread per call, deadline as a ScopedValue, MDC),
`PinnedVersions`, `CorsPolicy`, `context/` (`HostFunctionContext`, `HostLogger`, `DbPools`, the
allowlisting HTTP caller — built on the framework's HTTP client so calls are traced),
`reconcile/Reconciler` + `ReconcileLoop` (new-before-old swap, drain, lazy vs warm load).

**Replace:**
- Control plane → **our** protocol (`internal/functions/control`): `GET
  /control/functions/desired?pool=P&wait=30s&runtimes=jvm` (ETag/revision), `POST
  /control/functions/heartbeat` (versions `LOADED` / `FAILED` / `RETRYING` with the Go runner's
  rule that only a permanent failure is `FAILED`; `capabilities {protocol, runtimes:["jvm"],
  runnerKind:"ingestor-jvm", version}`), `GET /control/functions/artifacts/{digest}` (follow a 302),
  `POST /control/functions/events`, `POST /control/functions/dispatch-jobs`. Auth: the ingestor's
  client credentials, holding the role `platform:function-runner` as well.
- Listener → `/fn/{address}[@{alias}|@v{n}]/{path}` on the ingestor's framework HTTP server, with
  the Go runner's invocation path (`docs/function-runner-plan.md` §6.2): unknown address 404; known
  but loading 503 + `Retry-After`; endpoint match 404/405; auth per endpoint — `webhook` exactly as
  `pkg/fcsdk/webhook.Validator` (port it and its test vectors), `platform` with the ingestor's
  platform-token check, `none`; body limit 413; permit (non-blocking) 429 `Retry-After: 1`;
  deadline 504; handler exception 500 `{"error":"FUNCTION_FAILED"}` (message to the log only).
  Explicit `@v{n}` needs a platform token with `platform:function:version:invoke`.
- Desired document → our `Desired` (`functions[]` with `limits`, `warm`, `webhookSecret`, `config`,
  `secrets`, `db`, `versions[]` with `runtime`, `describe`, `roles`).

**Drop:** `wasm/*`, Sigstore, public routes and domains (later slice), javalin's manifest classes.

**Concurrency (R3) — must be tested:** N concurrent slow calls to one function finish in about one
call's time, not N; permits exhausted → 429 at once (no queueing); a deadline frees its permit
only when the call really returns (javalin H7); drain waits for in-flight calls; no `synchronized`
or single lock on the invoke path.

**Role switch:** functions on unless the ingestor is told otherwise (a framework setting, e.g.
`INGEST_FUNCTIONS_ENABLED`, default true); ingestion likewise switchable, so a deployment can run
either role or both. A function-only process takes no ingestion leases.

## 4. Platform and fcdev (Go)

- `jvm` joins `ValidRuntime` / `runtimes`; the desired-state runtimes filter already exists — check
  the in-process Go runner never asks for `jvm`.
- Publish of a jar (zip magic): read `META-INF/flowcatalyst/function.json` **without executing
  anything**, under caps (artifact ≤ 64 MiB as today, describe ≤ 1 MiB, bounded entry count),
  validate it with `abi.ParseDescribe` plus `jvm.entrypoint` (a Java binary class name). A jar
  without it, or with a bad one, is refused with the same error style as a bad Wasm describe.
- fcdev: when the ingestor child runs, the platform's `FC_FUNCTIONS_RUNNER_URL_JVM` points at it
  (`http://127.0.0.1:{ingestorPort}`), its local service account also holds
  `platform:function-runner`, and `fcdev fn deploy` accepts a `.jar`.
- Deployed: document the role and URL settings (iac changes are not part of this work).

## 5. Phases

| Phase | Work | Who |
|---|---|---|
| J1a | Platform + fcdev (§4) | agent |
| J1b | Function API (§2) | agent |
| J2 | Host in the ingestor (§3), after J1b | agent |
| J3 | Example function (port javalin `examples/function-hello` to our describe), end to end on an isolated fcdev: deploy, webhook, platform call, subscription delivery, emit, dispatch; concurrency check | main session |

Each phase is verified in the main session (tests, a deliberate break per new behaviour, and J3's
end-to-end run) before it is committed.
