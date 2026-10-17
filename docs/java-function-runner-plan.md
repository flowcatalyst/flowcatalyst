# Java function runner — plan

*Status: **ON HOLD** (owner, 2026-10-17: documented, not started; the sixteen decisions in section 9 are unanswered). Design written 2026-10-17 for owner review. Nothing here is built. Direction (owner,
2026-10-17): the Go platform and the wazero runner stay. A Java runner is added next to them so
JVM functions (jars) run beside Wasm/JS ones, installable like a plugin
(`fcdev runner install java`), and the same artifact is the production service.*

*Sources read: this repo (`internal/functions/**`, `internal/platform/function/**`, `cmd/fcdev/**`,
`.github/workflows/release-fcdev.yml`, `docs/function-runner-plan.md`, `docs/fcdev-fn-dev.md`,
`docs/java-handoff-2026-10-15-function-dispatch.md`, `clients/fn-ts`) and, read-only,
`../flowcatalyst-javalin` (`function-host/`, `function-api/`, `docs/function-runner-{plan,report,workplan}.md`,
`docs/spec/function-{invocation,host-*,manifest-authoring}.md`). Nothing was run, built or
connected to; every behavioural claim is from reading source and is marked **(unverified)** where
the code was not followed all the way.*

## Contents

0. Summary and recommendations
1. The two protocols today, and the gap
2. Go platform changes
3. Function interface (Java SDK)
4. The runner process
5. `fcdev` changes
6. Conformance and tests
7. Security
8. Phases, sizes, delegation, risks, first slice
9. Open owner decisions
10. What I could not verify

---

## 0. Summary and recommendations

The Java function host in `flowcatalyst-javalin` is a good *engine* (class-loader isolation, metaspace
fence, permits, deadlines, Vert.x listener, metrics) bolted to a *different platform contract*. It
cannot talk to this platform as is, and it cannot be lifted out as is either: `function-host` depends
on the whole Java platform module (`flowcatalyst-server`: `FunctionAddress`, `Manifest`, `Digest`,
`Json`, the artifact stores, JWKS auth, `Database`) and on Endive/Extism for Wasm. The recommended
route is **copy-and-trim into a standalone runner module that depends only on a small API jar and on
this repo's control protocol**, not "adapt the Java host in place".

| Decision | Recommendation |
|---|---|
| Wire protocol | The Go control protocol (`internal/functions/control`) is the single contract. The Java runner implements it; the Java platform's own contract is not touched. |
| Version/runtime tag | `jvm` joins `wasm` and `js` as a version runtime; **a function's runtime is fixed at its first version** (a function does not change runtime). |
| One describe shape | A jar carries `META-INF/flowcatalyst/function.json` that is exactly the existing describe document plus a small `jvm` block. The platform reads it by opening the zip; it never executes jar code at publish. |
| Capability negotiation | Runner sends `runtimes=` on the desired-state request and `capabilities` in the heartbeat. The platform serves each runner only versions it can run. |
| Delivery URL | Runner URL template gains per-runtime override (`FC_FUNCTIONS_RUNNER_URL_JVM`); the Java runner serves the same `/fn/{address}[@alias\|@vN]/{path}` shape as the Go runner. |
| Java SDK | Declarations in code via a registry/builder (same shape as `clients/fn-go`, `clients/fn-ts`), describe generated at build time by running the declaration code. No annotations. |
| Runtime packaging | jlink runtime image bundled in a per-OS/arch tarball. JRE-on-PATH is rejected for the installable artifact. No GraalVM native (cannot load jars at runtime). |
| Install | `fcdev runner install java` (top-level `runner` noun, not under `fn`). |
| Where it lives | `runners/jvm/` and `clients/fn-java/` in **this repo**, so the contract, conformance suite, fcdev and both runners change in one PR. |
| Trust | JVM functions are first-party only; separate pools; a distinct publish permission. Client-supplied code stays on Wasm/JS. |
| wasmtime | Mechanism designed generically (§5.1); not in scope now. |

---

## 1. The two protocols today, and the gap

### 1.1 What the Go platform's runner contract is

All in `internal/functions/control/control.go` (types, shared by server and runner client) and
`internal/platform/function/control/*.go` (server side). Routes under the platform base URL, bearer
token required, principal must be anchor-scope (`auth.RequireAnchor`) **and** hold
`platform:function:runner:control` (`auth.CanControlFunctionRunner`, `internal/platform/shared/auth/auth.go:1175`);
the built-in role is `platform:function-runner`.

| Concern | Go platform behaviour (source) |
|---|---|
| Desired state | `GET /control/functions/desired?pool=&wait=<s>` + `If-None-Match` (`desired.go`). **Long-poll**: re-renders at most every `ReRenderInterval`, woken early by `NOTIFY fng_desired` (`listener.go`); 200 + body, or 304 after `wait` (cap `MaxWait` = 60 s). ETag = sha256 of the *rendered* document (not the revision), `Cache-Control: no-store`. |
| Desired body | `Desired{pool, revision, auth{issuer,audience}, functions[], routes[]}`. `Function{id, address, applicationId, clientId, limits{memoryMb,maxConcurrency,timeoutMs,maxBodyBytes}, warm, webhookSecret, config{}, secrets{}, db{name:dsn}, versions[]}`. `Version{number, digest, runtime, abi, describe(raw JSON), roles[live\|candidate\|alias:<n>]}`. Absence of a version = unload it. Contains secrets. |
| Versions served | `buildVersions` (`document.go`): every aliased version plus every `PUBLISHED` version as `candidate` so runners prove them loadable before promote. |
| Heartbeat | `POST /control/functions/heartbeat` `Heartbeat{runnerId, pool, startedAt, revision, budget{...}, versions[{functionId, number, state, reason}]}`. States `LOADED`, `COMPILED`, `FAILED`, `RETRYING` (F22). `heartbeat.go`: `LOADED`→`PUBLISHED→READY` once; `FAILED`→`PUBLISHED→FAILED`, **latched** (only a re-publish re-queues it) and fenced by `requeued_at_revision` (a FAILED counts only from a runner whose applied `revision` is at least the revision of the last re-queue). `RETRYING`/`COMPILED` are informational. All transitions share one pool-revision bump. |
| Artifact | `GET /control/functions/artifacts/{digest}` keyed by **sha256 digest** (`artifacts.go`): only digests referenced by some `fng_versions` row; 302 to a presigned URL when the store supports it (the runner must not forward its bearer cross-host, see `Client.Artifact`), else streams the bytes; `Content-Type` from runtime (`application/wasm`, `text/javascript`). |
| Emit | `POST /control/functions/events` `EmitRequest{functionId, version, event, data(base64)}` → `{eventId}`; platform checks type against the version's describe `emits` and application ownership (`events.go`). |
| Dispatch | `POST /control/functions/dispatch-jobs` `DispatchRequest{functionId, version, invocationId, requestCorrelationId, dispatch, payload}` → `{jobId}` (`dispatch.go`, ABI op 11). Platform checks (target, path) against `describe.dispatches` (each `{target, path, connection (required), pool?}`, F23/F24), target ownership, builds the delivery URL with `function.TargetURL(RunnerURLTemplate, target.RunnerPool(), target.Address, d.Path)`, and the connection's service account signs delivery. |
| Host capabilities | ABI v1 ops 1-11 (`internal/functions/abi/meta.go`): log, config.get, secret.get, http.fetch, event.emit, db.query/exec/begin/commit/rollback, dispatch. In the Go runner these are served from `capabilities.Call` (`internal/functions/runner/capabilities.go`) and are declaration-gated: config/secret keys must be in describe; fetch is `describe.httpAllow` + `internal/netguard`; emit goes through the control route; DB DSNs come from the desired document. |
| Invoke | `/fn/{address}[@{alias}\|@v{n}]/{path...}` (`invoke.go:52`). `@vN` needs `platform:function:version:invoke` + reach. Auth is per **endpoint** in the describe (`webhook` = platform HMAC `X-FlowCatalyst-Signature/-Timestamp` under `function.webhookSecret`; `platform` = bearer verified against `desired.auth`; `none`). |
| Outcomes | 2xx ack; `429`+`Retry-After` retry that spends no budget (`respond.retry`); `422` + `FlowCatalyst-Outcome: reject` terminal (`respond.reject`, §13.2); 200 + `ack:false` is a router-side deferral. |
| Addressing | `{application-code}.{name}`, **two** labels (`function.BuildAddress`). |
| Pools | Runner pool = `Function.Pool` setting (default `default`); a different thing from the *dispatch* pool (`DispatchPoolCode` = `fn-<address>` for subscription delivery; `dispatches[].pool` for function-created jobs). |
| Schema | `fng_*` tables; `fng_versions.runtime` is `CHECK (runtime IN ('wasm','js'))` (`internal/migrate/sql/061_function_version_runtime.sql`); latest migration today is `082`. |

### 1.2 What the Java host speaks today

From `function-host/src/main/java/io/flowcatalyst/fnhost/**` and `docs/spec/function-*.md`. The
Java platform and host were built as a pair, then partly aligned (the manifest and invoke model
were deliberately made similar); several things differ.

| Concern | Java host behaviour (source) |
|---|---|
| Desired | `GET /control/functions/desired-state?pool=` + `If-None-Match`, plain poll on a timer (`HttpControlPlane`, `ReconcileLoop`); no `wait`. |
| Desired body | `DesiredDocument{functions[Entry], unload[{address,version}], publicRoutes[]}`. One `Entry` per (function, version) with a single `Role` (`LIVE\|CANDIDATE\|ALIAS`) and `Mode` (`WARM\|LAZY`), `digest`, `artifactRef`, `signatureBundle`, `signer`, `manifest` (Java `Manifest`: runtime, entrypoint, pool, warm, limits{maxDurationMs,maxConcurrency,wasmMemoryMb}, endpoints, subscriptions, schedules, public, config, secrets, db{name,secretRef,poolSize}, httpAllow), `webhookSigningSecret`, `applicationId`, `clientId`, `config`, `secrets`, `missingSettings`, `aliases`. Explicit `unload`. No `revision`, no `auth{issuer,audience}` (issuer comes from host env). |
| Token | `client_credentials` against `<platform>/oauth/token`, cached to 60 s before expiry, refresh-once-on-401 (`TokenSource`). Same grant the Go runner client uses. |
| Heartbeat | `{hostId, pool(DnsLabel), state ACTIVE\|DRAINING, loaded[{address, version, state registered\|loaded\|failed(error)}]}`, expects **204**. No budget, no revision, no `RETRYING`. |
| Artifact | `GET /control/functions/artifacts/{versionId}` (`PlatformArtifactStore`), 200 stream; digest recomputed locally; alternative `file://`, `oci://` stores; Sigstore bundle verification is JDK-only (`Signatures`/`Verification`). |
| Emit | `POST /control/functions/events` `{hostId, address, version, events[...]}`, batch of one, `dedupId` required. |
| Dispatch | **Absent** (`docs/java-handoff-2026-10-15-function-dispatch.md`: "Nothing here is implemented on the Java side"). Manifest has no `emits` or `dispatches`. |
| Invoke | `/functions/{address}[:{version}]/{path}`; endpoints/auth modes the same three; `Result.ack/retry/fail/http/json` (`function-api`), `retry` = 429+`Retry-After`, `fail` = 500 JSON and per its javadoc cannot force a non-retryable failure; **no `reject`**. |
| Addressing | `{app}.{service}.{name}`, three labels (`FunctionAddress`, `DnsLabel`; types live in `flowcatalyst-server`). |
| Isolation | `JvmFunctionLoader`: pre-scan of the jar (native libs, `java.security.Provider` service entry, shadowing of the API package => refusal reasons `NATIVE_LIBRARY`, `SECURITY_PROVIDER`, `BUNDLES_API`), then a named `URLClassLoader` per version over `ApiOnlyParentLoader` (platform loader for `java.`/`javax.`/`jdk.`/`sun.`/`com.sun.`/`org.w3c.`/`org.xml.`, host loader for exactly `io.flowcatalyst.function`, everything else `ClassNotFoundException`, resources and `META-INF/services` filtered too). Metaspace: `MetaspaceGuard` refuses loads under a reserve of `max(64 MiB, 5 % of pool max)`; `OutOfMemoryError: Metaspace` is caught around one load; `-XX:MaxMetaspaceSize` is 25 % of the container (`docker/jvm-opts.sh`). |
| Listener | Vert.x `HttpServer` (`FnHttpServer`, ~1080 lines) with invocations on virtual threads; separate observability server (`/health`, `/ready`, `/metrics`); `Permits` (host-global default 512 + per function); `InvocationDeadline`; JFR `FunctionInvocationEvent`. |
| Measured (their report, Docker Desktop, directional) | Lean function ~0.05-0.09 MB RSS each over a ~110 MB baseline; a "typical" function with Jackson + json-schema-validator shaded in ~4.4 MB metaspace and ~5.4-5.9 MB RSS each; the 25 % metaspace fence on a 2 GiB host stopped at 109 typical functions, a 4 GiB host should hold ~220-230. `/io` warm throughput 4k-50k req/s at 2 CPUs depending on concurrency. |

### 1.3 Gap table: Java host -> Go platform

"Adapter" = a thin translation layer in front of unchanged internals. "Rewrite" = replace the
class. Sizes are for the work in the copy-and-trim module (§4.1).

| # | Area | Java today | Needed for the Go platform | Verdict |
|---|---|---|---|---|
| G1 | Desired path + polling | `/desired-state`, timer poll | `/desired?pool=&wait=` long-poll, send `runtimes=` | Adapter (path, query, loop waits on the server) - S |
| G2 | Desired document | `Entry`-per-version, `unload[]`, `manifest`, `artifactRef`, signature/signer | `Function{versions[]}` with roles, `describe`, `limits`, `warm`, `db` DSN map, `auth`, `routes`; absence = unload | **Rewrite `DesiredDocument` (335 lines)** and the diffing in `Reconciler` (1214 lines) that consumes it - M. The reconciler's *load/unload/swap/idle* logic is reusable; its *input model* is not. |
| G3 | Address | 3-label `app.service.name`, `FunctionAddress` in server module | 2-label `app.name` | Rewrite the value type in the runner module (do not take `flowcatalyst-server`) - S |
| G4 | Limits source | In the manifest (`maxDurationMs`, `maxConcurrency`, `wasmMemoryMb`) | In `Function.limits` (platform setting, `memoryMb`, `maxConcurrency`, `timeoutMs`, `maxBodyBytes`) - the Go principle "code declares needs; platform decides how much" | Adapter - S. `memoryMb` has no per-function meaning on a shared heap, see §4.4. |
| G5 | Heartbeat | `loaded[]` registered/loaded/failed, 204 | `versions[]` LOADED/COMPILED/FAILED/RETRYING, `runnerId`, `startedAt`, `revision`, `budget` | Rewrite builder - S. **Semantics matter**: see G6, G7. |
| G6 | FAILED is latched | A metaspace-headroom refusal is a retriable load failure re-checked next cycle (`LOAD:METASPACE_HEADROOM`) | Anything reported `FAILED` permanently fails the version until re-publish (`heartbeat.go`) | Java transient failures (metaspace headroom, artifact fetch 5xx/timeouts) **must** report `RETRYING`, not `FAILED` - the F22 rule. Only permanent refusals (`NATIVE_LIBRARY`, `SECURITY_PROVIDER`, `BUNDLES_API`, `ENTRYPOINT_*`, `UNREADABLE_JAR`, bad describe) are `FAILED`. Easy to get wrong; needs a conformance vector. |
| G7 | Revision fence | none | Heartbeat carries `revision` of the applied desired document; a FAILED report is ignored from a runner that has not applied the re-queue revision | Adapter - S |
| G8 | Artifact download | by `versionId`, 200 | by sha256 digest, 302 presigned **or** 200; do not forward bearer across hosts | Adapter - S (JDK `HttpClient` with manual redirect handling; digest recomputed as today) |
| G9 | Signatures | Sigstore bundle verified by host and platform | Go platform has no signature policy; artifacts are trusted by digest + upload permission | Drop in the new runner (see §7.5, an owner question) |
| G10 | Emit | batch of one, `{hostId,address,version,events[]}` | `{functionId,version,event,data}` -> `{eventId}`; type must be in `describe.emits` | Rewrite `HttpControlPlane.emit` mapping - S. Errors map the same way (5xx/unreachable retryable `UNAVAILABLE`, 403 `NOT_ALLOWED`, other 4xx `BAD_REQUEST`). |
| G11 | Dispatch | not implemented | ABI op 11 / `dispatch-jobs` route, declared `(target, path, connection, pool?)`, 256 KiB cap, `NOT_DECLARED` enforced by the runner before calling the platform | **New** - M (Java SDK + host + control call) |
| G12 | Describe vs Manifest | Java `Manifest` strict reader; includes `public`, `pool`, `warm`, `limits`; lacks `emits`, `dispatches` | `abi.Describe` (`endpoints, subscriptions, schedules, config, secrets, db[]string, httpAllow, emits, dispatches`); `db` is a list of names; pool/limits/warm/public routes are platform settings | Replace `Manifest` with a `Describe` reader in the runner (strict, same rules as `abi.ParseDescribe`) - M. This is also the place two runtimes can drift; §6 vectors cover it. |
| G13 | DB | manifest `db{name, secretRef, poolSize}`; host resolves DSN from `secrets` | `desired.functions[].db{name:dsn}` (decrypted in the document); Go ABI offers `db.query/exec/begin/commit/rollback` | Adapter over `DbPools`/`Dsn` - S. Java also exposes a `DataSource`; keep it (JDBC drivers must come from the function jar, `ApiOnlyParentLoader` refuses host drivers). Postgres-only vs other drivers: **unverified**. |
| G14 | Invoke URL | `/functions/{a}[:{v}]/{p}` | `/fn/{a}[@alias\|@vN]/{p}` (platform builds it, `TargetURL`) | Rewrite route parse in `FnHttpServer` - S. Adopt the Go shape; do not make the platform emit two shapes. |
| G15 | Version-invoke permission | reach-based | `platform:function:version:invoke` + reach | Adapter in `WebhookVerifier`/auth - S |
| G16 | Platform auth | `BearerAuthenticator`/`JwksKeySource` from the server module; issuer from env | `desired.auth{issuer,audience}`; claims `sub,type,tier,scope,clients,roles,applications,all_applications,token_use` (`runner/auth.go` `platformClaims`) | Rewrite claim mapping against this platform's tokens; verify with JDK/`nimbus`-class lib or the small JWKS verifier already present - M. The `Caller` shape in the API jar must match `abi.Caller` (§3.2). |
| G17 | Public routes | `publicRoutes[{hostname,pathPrefix,address,aliasPrefixes}]`, `PublicRouteTable`, `TrustedProxies` | `routes[{hostname,pathPrefix,address,alias,aliasPrefixes}]`, public entry port (`FC_FUNCTIONS_PUBLIC_PORT`) | Adapter - S; defer to a later phase (not first slice). |
| G18 | Result model | `ack/retry/fail/http/json`, no `reject` | `reject` = 422 + `FlowCatalyst-Outcome: reject`; `retry` 429 | Add `Result.reject(reason)` to the SDK - S |
| G19 | Wasm | Endive/Extism in-process | Wasm stays on wazero | **Remove** the `wasm/` package and Endive/Extism deps from the new runner (fewer deps, smaller image, no second Wasm engine to conform) |
| G20 | Schema | `fn_*` (jOOQ, Java platform) | `fng_*` (this repo) | **Not shared and not touched.** The Java host reads no DB (it reads only the control plane), so no change; the runner module must not link `flowcatalyst-server`. When the Go platform is used, `fn_*` does not exist. |
| G21 | Pool name type | `DnsLabel` | arbitrary string, default `default` | Relax - S |
| G22 | Draining | `state=DRAINING` heartbeat | Go heartbeat has no drain field; a draining Go runner just goes not-ready | Drop the field, or add an optional additive `draining` bool to the heartbeat (platform ignores unknown today **(unverified)**); recommend dropping. |

**Overall verdict: reuse ~60 % of the host's internals (loader, guard, permits, deadline, metrics,
HTTP server skeleton, context services) behind a rewritten control/model layer (G2, G5, G12, G16)
and a new capability (G11). Not "adapt in place".**

---

## 2. Go platform changes

Goal: one registry, one describe shape, one promote/publish flow, two runner kinds. Names below are
the packages that change.

### 2.1 Runtime tag

- `internal/platform/function/entity.go`: add `RuntimeJVM = "jvm"`; extend `ValidRuntime`. Add a
  `Runtime` field to `Function` (nullable until the first version; set by the first publish;
  subsequent publishes must match, error `RUNTIME_MISMATCH`). Rationale: the delivery URL is
  materialised at promote into subscriptions/scheduled jobs; a function whose runtime changed
  between versions would need its subscription targets rewritten at every live flip and dispatches
  would have to resolve the target's live-version runtime per call. A fixed function runtime makes
  `TargetURL` a function of the function alone. (Recommendation; owner question 3.)
- `internal/functions/runtimes/runtimes.go`: add `JVM = "jvm"` to `Valid`. This package is shared
  by the platform (publish) and the Go runner. The Go runner's `Prepare` must **refuse** `jvm`
  (it does not run it); see §2.5 for why it must never even be served one.
- `internal/platform/function/operations/publish.go`: `PublishCommand.Runtime` validation message
  changes from `"wasm" or "js"` to include `jvm`; branch on runtime for the describe source
  (§2.2). `internal/platform/function/api/dto.go:127` doc string and the openapi lock follow.
- Migration `083_function_runtime_jvm.sql` (next free number after `082_dispatch_job_connection.sql`):
  drop and re-add `chk_fng_versions_runtime` to `('wasm','js','jvm')`; add nullable
  `fng_functions.runtime` with the same check. Down migration restores the 2-value check (fails if
  a `jvm` row exists; document that). No sqlc type-override impact expected **(unverified)**; run
  `make` codegen to see.

### 2.2 Publish-time validation of a jar (never execute it)

Today `Publish` runs `loader.Describe` which instantiates the guest with no capabilities
(`runtimes.Loader.Describe`). For a jar the platform must **not** load or run any class. Add a
`jar` reader package (`internal/platform/function/jar`, pure `archive/zip`):

1. Open as zip with limits: max compressed 64 MiB (existing `maxPublishArtifactBytes`), max entries
   (e.g. 20 000), max total uncompressed (e.g. 512 MiB), max single entry (e.g. 128 MiB), reject
   absolute paths / `..` / duplicate entry names (zip-bomb and path-trick hardening).
2. Require `META-INF/flowcatalyst/function.json`. Parse with `abi.ParseDescribe` plus the `jvm`
   block (below). The same `abi.Describe.Validate` rules apply, **so the same describe validates
   whichever runtime produced it**, plus the existing platform checks (emits ownership, cron,
   timezone, dispatch ownership) which already run after `ParseDescribe` in `publish.go`.
3. Static scan, the Go port of `JvmFunctionLoader.scanForRefusal` (pure name checks, no class
   definition): refuse entries ending `.so .dll .dylib .jnilib`, the entry
   `META-INF/services/java.security.Provider`, and any class under `io/flowcatalyst/function/`
   (shadowing the API). Error codes keep the Java reasons (`NATIVE_LIBRARY`, `SECURITY_PROVIDER`,
   `BUNDLES_API`) so users see one vocabulary.
4. Entrypoint class present as `a/b/C.class`; read the class-file magic and **major version**
   (bytes 6-7) and refuse a version newer than the newest Java the platform knows the runners
   support (config, default 69 = Java 25). Whether the class *implements the function module
   interface* is not checkable without parsing the constant pool; do it at runner load
   (`ENTRYPOINT_NOT_A_FUNCTION`, reported FAILED, not published-then-silently-broken). The build
   (§5.3) can run the runner's own `check` against the same jar before publish to catch it early.

`function.json` shape (the `jvm` block is the only addition; everything else is today's describe,
documented in `internal/functions/abi/describe.go`):

```json
{
  "abi": 1,
  "jvm": { "entrypoint": "com.bevco.ingest.BevcoIngest", "apiVersion": 1, "javaRelease": 21 },
  "endpoints": [ { "method": "POST", "path": "/ingest/trip", "auth": "platform" } ],
  "secrets": ["BEVCO_API_KEY"],
  "httpAllow": ["api.bevco.example"],
  "dispatches": [ { "target": "bevco-integration.yard-trip-processor", "path": "/events/yard-trip",
                    "connection": "bevco-yard", "pool": "yard-process-pool" } ]
}
```

Open point carried from the Java plan: its manifest also carried `pool`, `warm`, `limits`, `public`.
**Recommendation: drop them from the jar** (platform settings, as for Wasm). The Java plan authored
them in the manifest; the Go principle (plan §1.2: "pool, limits and secret values are deployment
settings") is deliberately the opposite, and a jar that sets its own pool can silently place itself
on a different trust level. A migrating Java author loses nothing: those values go in
`fcdev fn set`. Owner question 5.

`Describe` struct change: add `JVM *JVMBlock` (`entrypoint`, `apiVersion`, `javaRelease`), valid
only when the version's runtime is `jvm`, and **rejected** for wasm/js (strict parser stays strict).
Alternatively keep `Describe` untouched and parse `jvm` separately in the jar reader; recommended,
because `abi.Describe` is also the Wasm guests' exported document and the three guest SDKs must not
have to know about `jvm`.

### 2.3 Artifact kind, size, digest

- `internal/platform/function/api/api.go:235` `putArtifactInput.RawBody` is
  `contentType:"application/wasm"`; add `application/java-archive` (and keep `application/octet-stream`
  accepted **(unverified: what huma does for a body whose content type is not the declared one;
  check how `fcdev fn publish` uploads today)**). The upload route is runtime-agnostic: storage is by
  digest.
- `internal/platform/function/control/artifacts.go`: `contentType` switch gains
  `application/java-archive` for `RuntimeJVM`.
- Size: the 64 MiB cap (`maxArtifactBytes`, `maxPublishArtifactBytes`) is adequate for a shaded jar
  with Jackson (~10-30 MB) but not for one embedding a large library set. Keep 64 MiB for now; an
  EDI/XML schema-heavy jar approaching it should be split or its libs trimmed (owner question 12).
- Digest: sha256 of the jar bytes, as today. **Jars are not reproducible by default** (entry
  timestamps, manifest `Created-By`/`Build-Jdk`); every rebuild is a new digest and a new version, so
  the "publish same digest = idempotent" property (`GetVersionByDigest`) is lost unless the build is
  reproducible. The scaffold (§5.3) sets `project.build.outputTimestamp` and the descriptor-injection
  step writes entries with fixed timestamps and sorted order. Not enforced by the platform.

### 2.4 Heartbeat `runtimes` and pool routing

Additive, backward compatible both ways:

- `fncontrol.Heartbeat` gains `Capabilities` (`{"protocol":1,"runtimes":["jvm"],"jvm":{"javaMajor":25,"apiVersions":[1]}}`);
  stored as part of the existing report JSON (`UpsertRunnerHeartbeat` already stores the raw report)
  so no column is needed unless we want an indexed lookup.
- `desiredInput` gains `Runtimes string` query (`runtimes=jvm` or `wasm,js`). `buildDesired`/`buildVersions`
  (`document.go`) filter versions to the requested set; **absent => `wasm,js`**. ETag already hashes the
  rendered document, so each runtime set naturally gets its own ETag.
  - This is not optional: the existing Go runner turns an unknown runtime into a version error
    (`runner.go:357` `newVersion` returns `RUNTIME: "jvm" is not a runtime this runner has`), which
    becomes a `FAILED` heartbeat report, which the platform latches (`heartbeat.go`). An un-upgraded
    Go runner that merely *sees* a `jvm` version in its pool would permanently fail it. Filtering
    server-side by declared capability avoids the whole class (including a future wasmtime runner).
  - Also harden the Go runner anyway: an unsupported runtime is skipped and not reported.
- Promote guard (`operations` promote path; file not read in detail **(unverified)**): when the
  pool has at least one runner with a recent heartbeat (window = 3 x heartbeat period), and none
  lists the version's runtime, refuse `POOL_LACKS_RUNTIME` naming pool and runtime. When no runner
  has reported at all, behaviour is unchanged: a version only turns `READY` when some runner loads
  it, so promote of a never-loaded version already cannot succeed; improve `fcdev fn deploy`
  `waitReady` (`fn_platform.go:318`) and the function status endpoint to say "no runner in pool X
  supports runtime jvm" instead of timing out.
- Pools stay a free string and should be one runner kind each in practice (the URL template below
  makes that mandatory for delivery). Mixed pools are *safe* (filtered) but not *deliverable*.

### 2.5 Delivery target / `TargetURL`

`function.TargetURL(template, pool, address, path)` (`entity.go:137`) is called from
`operations/wiring.go:164` (promote wiring of subscriptions and schedules) and
`control/dispatch.go:214` (function-created dispatch jobs, built from the *target's* runner pool).
One runner URL template (`FC_FUNCTIONS_RUNNER_URL`, `internal/server/envcfg.go:383`, default
`http://127.0.0.1:8095`) cannot address two runner kinds in `fcdev start`, where both would live on
`127.0.0.1` on different ports.

Recommendation: add `FC_FUNCTIONS_RUNNER_URL_JVM` (same `{pool}` templating). `TargetURL` gains a
`runtime` argument and picks the template by the target function's `Runtime`; unset falls back to
the generic template. In production the pool name in `{pool}` already selects the ECS service, so
`FC_FUNCTIONS_RUNNER_URL=http://runner-{pool}.internal` keeps working with pool names such as
`jvm-default`; the per-runtime template only matters for the single-host dev case. The Java runner
serves `/fn/{address}...` so the path half is unchanged. Update `docs/environment-variables.md`
(an env-doc test, `functions_envdoc_test.go`, enforces this) and fcdev's env defaults
(`cmd/fcdev/envcfg.go`).

Dispatch to a JVM target from a Wasm function (and the reverse) works unchanged: it is an ordinary
dispatch job whose `targetUrl` points at the target function's runner, signed by the connection's
service account; the receiving function verifies it as a `webhook` endpoint.

### 2.6 Other Go packages touched

| Package | Change |
|---|---|
| `internal/functions/control` | `Heartbeat.Capabilities`, `Desired` request `runtimes` param constant, `Version.Runtime` doc. Keep this package dependency-light: it is the contract the Java types are generated/validated against. |
| `internal/platform/function/control` | `desired.go`, `document.go` (runtime filter), `heartbeat.go` (store capabilities; no state change), `artifacts.go` (content type). |
| `internal/platform/function/operations` | `publish.go` (jar branch, function runtime fix/mismatch), `wiring.go` (`TargetURL` with runtime), promote guard. |
| `internal/platform/function/jar` (new) | zip reader/scan/descriptor extraction + tests with hostile zips. |
| `internal/platform/function/api` | upload content type, DTO docs, status explaining missing-runtime. |
| `internal/platform/shared/auth` | optional new permission `platform:function:publish:jvm` (§7.4). `platform:function-runner` role unchanged. Built-in-role/permission lists have tests that enumerate them **(unverified)**. |
| `internal/server` | env `FC_FUNCTIONS_RUNNER_URL_JVM`; `functions.go` unchanged for Wasm. |
| `internal/migrate/sql` | `083_...`. |
| `api/openapi.lock.json`, `docs/wire-contract.md` | regenerate/extend: `runtime` enum value, upload content type, `runtimes` query, heartbeat `capabilities`. Whether the control routes are in the lock at all is **unverified** (they may be excluded like Java's `functions.openapi.json`); check by diffing specs per project practice. |
| `cmd/fcdev` | §5. |
| `frontend/src/pages/functions` | show runtime badge; filter; "no runner supports jvm" status. Generated types follow the lock. |

---

## 3. Function interface (Java SDK)

### 3.1 Compare: Java `function-api` today vs ABI v1

| Capability | ABI v1 / Go-family SDKs (`clients/fn-go`, `fn-ts`) | Java `function-api` today | Gap / recommendation |
|---|---|---|---|
| Entry | `fn.webhook/platform/open("METHOD /path", handler)` registered at module init; the *runner* routes and authenticates per endpoint from describe | `Function.handle(Request, FunctionContext)` single method; routing by `Request.path` is the function's own | Add a registry layer (§3.3) so a Java function declares endpoints once and the SDK routes; the runner still pre-authenticates by describe (it already does: `WebhookVerifier`, endpoint auth). |
| Request | method, path, params, query, headers, body, caller | `Request` record: address, version, invocationId, method, path, originalHost/Path, pathParams, query, headers, body, remoteAddress, `Caller` | Equivalent; keep. `Caller` must carry exactly `abi.Caller` fields (principal id/type/tier/clients/roles/permissions + webhook vs platform vs anonymous); verify against `internal/functions/abi/meta.go` before freezing. |
| Response | `json/text/status/retry/reject` | `Result.ack/retry/fail/http/json` | Add `reject`; make `fail` honest (500) or drop it in favour of throwing; add `text`/`status` helpers. |
| Webhook verify | runner verifies HMAC before the guest runs (`runner/auth.go`) | `WebhookVerifier` in host + public `Webhook` parsers (`Webhook.event/schedule`) | Same model. The SDK also exposes `Webhook.event(Request)` to parse the platform envelope (kept). |
| Platform-auth | runner verifies bearer; guest gets `Caller`; guest decides authorisation (`caller.hasPermission`, `*` segment wildcard mirrored across SDKs) | `Caller` | Port the exact wildcard matching and cover it with the shared vectors (§6). |
| dispatch | op 11, `fn.dispatch`: declared `(target, path, connection[, pool])` only, no URL/credential, no per-call override of target/path/pool/connection | none | **New** `Dispatch` handle created at declaration time from a declared target; call sites pass only payload + job fields (code, subject, messageGroup, mode, correlationId, causationId, contentType, descriptor, maxRetries, timeoutSeconds, dataOnly, metadata). Field list from `abi.Dispatch` (`meta.go:208`). Declaration-gating enforced in the runner (`NOT_DECLARED` before the control call), mirroring `capabilities.dispatch`. |
| emit | op 5, type must be in describe `emits`, `dedupId` | `Events` + `OutboundEvent`, `EmitResult` (`Emitted`/`Refused`) | Keep; add the declaration `emits("a:b:c:d")` so describe carries it. |
| config/secret | op 2/3, keys must be declared | `Config`, `Secrets` (manifest-declared keys, masked `toString`) | Keep; declaration in code. |
| db | ops 6-10 (`query/exec/begin/commit/rollback`, JSON rows) | `DataSource dataSource(String name)` (JDBC) | **Differs by design**: JVM gets a real `DataSource`. Decision: keep JDBC for Java (richer, what Java developers expect); conformance tests the observable behaviour (declared name only, pool bounds, per-invocation tx release), not the call shape. The `db` host-capability on the control side is DSN-in-desired-document either way. |
| fetch | op 4: `describe.httpAllow` + `netguard` (SSRF guard) | `HttpCaller` (`AllowlistHttpCaller`, `HttpAllowlist`) | Keep the interface; **be honest** that for the JVM a function can open its own `java.net` socket, so the allowlist is a convention (the Java plan says exactly this) and egress control belongs to the network (§7). Port `netguard` rules (private/loopback/link-local, redirects) into `AllowlistHttpCaller` so the *supported* path is as strict as Go's. |
| log | op 1 | `System.Logger` with MDC keys | Keep. Logging frameworks inside the jar must route to the host logger or stdout JSON; a bundled logback needs no config from the host. |
| clock | n/a | `FunctionContext.clock()` | Keep (testability). |
| version/address | in request | `ctx.address()`, `ctx.version()` | Keep. |
| Lifecycle | none (instance per call/reuse) | `init(ctx)`, `handle`, `stop()` | Keep; `stop()` is how a function releases pools/threads before unload. |

### 3.2 Recommended Java declaration style: registry/builder, not annotations

The framework rule is declarations in code, and the three existing guest SDKs register handlers in
code and compute describe from the registry (`clients/fn-ts/src/registry.ts`, `describe.ts`;
`clients/fn-go/registry.go`, `describe.go`). A Java function should do the same:

- One declaration surface (`FunctionModule.declare(Fn fn)`), handlers bound to endpoints at the same
  place, so describe cannot drift from the handler set.
- No annotation processor to maintain, no reflection scan at load, no second routing mechanism.
- Annotations remain a possible *sugar* later (`@Webhook("POST /x")` -> same registry), but they add
  nothing now.

The same registry is what makes the **fake host** (§3.5) trivial: `fn.testHost()` invokes handlers
with a `Request` and fake capabilities.

### 3.3 Example: the bevco ingest function

Dispatches to `bevco-integration.yard-trip-processor` on the `yard-process-pool` dispatch pool through
the `bevco-yard` connection. API sketch (not final signatures); handler path is illustrative.

```java
package com.bevco.ingest;

import io.flowcatalyst.function.*;   // API jar only; JDK XML is available (javax.xml.*)

public final class BevcoIngest implements FunctionModule {

    @Override
    public void declare(Fn fn) {
        Secret apiKey = fn.secret("BEVCO_API_KEY");
        fn.httpAllow("api.bevco.example");

        // Declared once; there is no call-site override of target/path/pool/connection.
        Dispatch yardTrip = fn.dispatch(Dispatch.to("bevco-integration.yard-trip-processor")
                .path("/events/yard-trip")
                .connection("bevco-yard")
                .pool("yard-process-pool"));

        fn.platform("POST /ingest/trip", (req, ctx) -> {
            Trip trip = TripXml.parse(req.body());              // javax.xml / StAX, lives in this jar
            String enriched = ctx.http().send(HttpCall.get(
                    "https://api.bevco.example/trips/" + trip.id())
                    .header("Authorization", "Bearer " + apiKey.get(ctx))).bodyText();

            JobId job = yardTrip.create(ctx, TripJson.map(trip, enriched),
                    Job.group("trip-" + trip.id())
                       .correlationId(req.correlationId())
                       .contentType("application/json")
                       .metadata("source", "bevco-ingest"));
            return Result.json(202, "{\"jobId\":\"" + job.value() + "\"}");
        });

        // Delivery from the platform for a subscribed event (signed by the connection's SA).
        fn.webhook("POST /events/trip-closed", (req, ctx) -> {
            ctx.logger().log(System.Logger.Level.INFO, "closed {0}", Webhook.event(req).subject());
            return Result.ack();
        });
    }
}
```

`describe` (generated, §3.4) contains the `dispatches` entry exactly as in §2.2; the platform's
publish check for `connection` required and the per-call `DISPATCH_*` checks are unchanged.

### 3.4 How describe is generated

Same principle as the other SDKs (the declaration code *is* the source). The difference is when the
code runs: Wasm guests export `fc_describe` and the platform runs it; for a jar the **build** runs it,
and the platform only reads the result.

1. `fn init --lang java` scaffolds a `pom.xml` with an `exec-maven-plugin` execution at the `package`
   phase that runs `io.flowcatalyst.function.tools.Describe <entrypoint>` (shipped in the SDK, in a
   `tools` classifier or the same jar) on the project classpath and writes
   `target/classes/META-INF/flowcatalyst/function.json` **before** the jar plugin packs it. Gradle gets
   an equivalent `JavaExec` task.
2. `Describe` loads the entrypoint with a plain class loader (this is the author's own machine),
   calls `declare` against a recording `Fn`, validates with the same rules as `abi.Describe.Validate`
   (a Java port; the conformance vectors keep the two in agreement), and fails the build with the
   Go-style error messages.
3. `fcdev fn build` therefore only runs `mvn -q package` / `gradle build` and checks the jar for
   `function.json`. It needs no runner installed and no Java logic of its own beyond tool detection.

### 3.5 Testing support

A `flowcatalyst-function-testing` artifact: `FakeHost` implements all capability interfaces in
memory (config/secrets maps, an event/dispatch recorder, a canned `HttpCaller`, an in-memory or
Testcontainers-agnostic `DataSource` supplier, controllable `Clock`, caller builders for
webhook/platform/anonymous), `TestRequest` builders, assertions for `Result`, and
`FakeHost.check()` that runs the same `Describe` validation as the build. Same role as the Go SDK's
`capabilities_test.go`/`testutil_test.go`, the TS SDK's `node --test` path, and the Rust SDK's mock
host. The runner's own `fcdev fn dev --send` loop (§5.5) covers integration-level replay.

### 3.6 Java release: 21 vs 25

`function-api` is compiled `--release 21` in the Java repo (`function-api/pom.xml`) while the host
compiles to 25. Recommendation: **SDK jars release 21, runner JDK 25 (LTS)**. A function jar built
on 21-25 runs; the runner refuses a jar whose entrypoint class-file major version exceeds its own
(§2.2 item 4). 21 keeps the door open for authors on older toolchains and for the Java framework's
own modules; the cost is none for the SDK (it uses no 22+ API, per its zero-dependency shape). If
the Java framework mandates 25 everywhere, release the SDK on 25 too; owner question 7. Virtual
threads (21+) are the invocation model, and pinning/`synchronized` interactions are a known runtime
risk with arbitrary library code (a library holding a monitor across blocking I/O pins the carrier;
JDK 24+ largely removed that for monitors **(unverified for the exact build)**).

---

## 4. The runner process

### 4.1 Source layout and reuse

New standalone Maven module `runners/jvm/` (artifact `fc-runner-java`), starting as a **copy** of
`flowcatalyst-javalin/function-host` with these removals/replacements; it must not depend on
`flowcatalyst-server`, and it drops `wasm/` and the Endive/Extism/`extism-endive` dependencies.

| Reuse as is (re-homed) | Rewrite | Remove |
|---|---|---|
| `load/` (`JvmFunctionLoader`, `ApiOnlyParentLoader`, `MetaspaceGuard`, `MetaspaceGauge`, `FunctionRegistry`, `LoadedFunction`, `Reason`), `http/Permits`, `InvocationDeadline`, `InvocationRunner`, `CorsPolicy`, `RoutePath`, `FnObservability` (health/ready/metrics), `FnMetrics`, `GuardedLog`, `FunctionInvocationEvent` (JFR), `context/` (`HostFunctionContext`, `AllowlistHttpCaller`, `DbPools`, `HostLogger`, `Dsn`, maps) | `reconcile/DesiredDocument`, `Reconciler` input model, `HttpControlPlane`, `HeartbeatReport`, `TokenSource` (keep logic, change types), the `Manifest` use in `FnHttpServer`/`Reconciler`, `FunctionAddress`/`DnsLabel` value types, platform-token verification, dispatch capability (new) | `wasm/*`, `route/` public listener (until phase 4), Sigstore `Signatures`/`Verification`/artifact stores other than the control-plane downloader (`oci://`, `file://`) |

Source home (owner question 8): **this repo**. Reasons: the contract (`internal/functions/control`),
the conformance suite, `fcdev`, the release workflows and signing, and both runners must change
together; a Java contract change that lands in another repo days later is exactly how the two
protocols got different. This repo already hosts polyglot code (`clients/laravel-sdk`, `java-sdk`,
`python-sdk`, `fn-rust`, `fn-ts`). CI cost: a JDK 25 + Maven job (path-filtered on `runners/jvm/**`
and `clients/fn-java/**`). Alternatives considered: `flowcatalyst-javalin` (has the toolchain and
the history but ties the runner to a repo whose platform it no longer serves); a new repo (clean,
but three repos to keep in step). The function-api jar can later be consumed by the Java platform
if it ever wants to share it.

### 4.2 Control-plane adapter

`ControlPlane` interface kept (it is the test seam), with the Go protocol behind it:
`desired(pool, runtimes, etag, wait)`, `heartbeat(...)`, `artifact(digest)`, `emit(...)`,
`dispatch(...)`. Behaviours to port exactly from `internal/functions/control/client.go` and
`runner/runner.go`: long-poll with `wait` and ETag, 304 handling, token cache + one refresh on 401,
manual redirect for artifacts without forwarding the bearer cross-host, retry classification
(`LoadRetry`: 5xx/429/503/network/timeouts retry with jitter and report `RETRYING`; 404, digest
mismatch, invalid jar/describe, refusals are permanent `FAILED`), `revision` echoed in the heartbeat,
and the F22 rule that nothing transient is ever `FAILED`. Environment: reuse the `FC_FUNCTIONS_*`
names where meaning is identical (`FC_FUNCTIONS_POOL`, `_PLATFORM_URL`, `_CLIENT_ID`, `_CLIENT_SECRET`,
`_PORT`, `_PUBLIC_PORT`, `_CACHE_DIR`, `_IDLE_EVICT_SECONDS`, `_LOAD_RETRY_*`, `_MEMORY_LIMIT_MB`) so an
operator who knows the Go runner knows this one; add `FC_FUNCTIONS_JVM_*` only for JVM-specific knobs.
(The Java host used `FC_FN_*`; a one-time rename in the copy.)

### 4.3 Listener

Recommendation: **keep Vert.x** for the listener (as in the Java host and the Java port generally),
with each invocation handed to a virtual thread and the response written back on the request
context, plus a *separate* Vert.x server for `/health /ready /metrics` so a saturated function port
cannot make the process look dead (their `function-host-process.md` §2). Reasons: it is already
written, benchmarked and tested (`FnHttpServer` tests: ~50 test classes under `http/`), and
re-implementing 1000+ lines of HTTP edge cases (body caps, header limits, CORS preflight,
trusted proxies, chunked/limits) on `com.sun.net.httpserver`/Helidon/Javalin is cost without benefit.
Counter-view: Vert.x is a large dependency in a jlink image; the `ApiOnlyParentLoader` already hides
it from functions, so footprint is the only cost. Owner question 6.

Route shape: `/fn/{target}/{path...}` and `/fn/{target}` with `target = address[@alias|@vN]`
(parse as `parseTarget` in `invoke.go:84`), identical error JSON (`writeError`) to the Go runner so
callers and the router see one behaviour.

### 4.4 Limits, concurrency, budget: Go vs JVM

| Concern | Go runner | Java runner (recommended) |
|---|---|---|
| Memory model | `budget.Budget`: container limit (cgroup v2/v1/RAM, `DetectLimit`) minus reserve; per-version admission against the remainder; wazero linear memory via mmap allocator that returns nil to fail a grow; "memory is a budget, never an OOM" | Shared heap and metaspace; **no per-function memory enforcement is possible**. `-Xmx`/`-XX:MaxMetaspaceSize` computed from the container limit by the launcher (port `docker/jvm-opts.sh`); `MetaspaceGuard` refuses loads under the reserve and reports `RETRYING` (not FAILED). `limits.memoryMb` becomes an **admission weight**: sum of loaded functions' `memoryMb` must fit under a configured share of heap; no per-call enforcement. A leaking or allocating function can take the whole runner down; `-XX:+ExitOnOutOfMemoryError` and the orchestrator restart it. Document this loudly. |
| CPU/time | Deadline by preemption (`WithCloseOnContextDone`) | Cooperative: `InvocationDeadline` exposes the deadline to the function and bounds the caller (504/timeout), but **a CPU-bound or non-interruptible thread cannot be killed**. Mitigation: interrupt the virtual thread on deadline; count abandoned invocations per version; past a threshold (default 3) unload the version, report it unhealthy, and if still stuck exit the process for the orchestrator to restart. |
| Concurrency | per-function `MaxConcurrency` permits (`pool.go`/`runner`) | Port as is: host-global (`Permits`, default 512) + per-function from `limits.maxConcurrency`; exhaustion -> `429`+`Retry-After` ("busy", spends no retry budget), same as Go. |
| Body caps | `limits.maxBodyBytes`, per-endpoint `maxBodyBytes`/`timeoutMs` from describe | Same, in the listener. |
| Instances | pool of Wasm instances, recycle after 10 000 calls, idle TTL 60 s | One `Function` instance per loaded version (the function is thread-safe by contract; documented), no recycling. |
| Idle unload | `IdleEvict` (default 30 min) drops compiled code | Idle unload of the **class loader** (default 30 min, same env) for lazy versions, so metaspace is reclaimed (needs `stop()`, then GC; the Java plan's leak test asserts the loader is collectable via `WeakReference`). `warm` versions never unloaded. |
| Load | compile (disk cache) | download -> verify digest -> scan -> define classes on first use (lazy) or at reconcile (warm). Cache dir holds jars by digest. |
| Drain | `draining` flag: not ready, new calls refused, wait `DrainGrace` (30 s) | Port: shutdown hook, `/ready` 503, refuse new, wait in-flight up to `DrainGrace`, then exit 0 (`FC_DRAIN_TIMEOUT_SECONDS` equivalent). ECS: `stopTimeout` >= grace. |
| Health/metrics | `/metrics` on the private port | Separate observability port (`/health` liveness that is true until startup completes, `/ready`, `/metrics` Prometheus). Metric names: keep the Go runner's names where a counterpart exists so one dashboard works for both runners **(unverified: Go metric names not read; check `runner/metrics.go`)**; JFR events stay as a Java extra. |
| Restart policy | n/a | The Java plan restarts hosts nightly "regardless"; recommend a *soft* equivalent: a rolling restart schedule at the orchestrator level, not in the process. |

Numbers to carry as expectations, not promises (Java repo's report, Docker Desktop, directional):
baseline ~110-150 MB RSS before any function; ~5.5 MB RSS per typical loaded function; the metaspace
fence is the binding constraint, not heap. Warm-state steady numbers are what matter (a long-running
service); the Java benchmark rig in `../flowcatalyst-javalin/bench/function-host` is the starting point
for the phase-0 measurement on this protocol. No Go-vs-Java per-call comparison exists for *this* runner;
the plan's §2.3 gives only Endive-vs-wazero for Wasm, which is not relevant to jar execution.

### 4.5 Packaging

- **Artifact (installable and production, the same bytes):** tarball
  `fcrunner-java-v<ver>-<os>-<arch>.tar.gz` containing
  `bin/fc-runner-java` (POSIX launcher; computes JVM flags from the cgroup/RAM limit the way
  `docker/jvm-opts.sh` does; `.cmd` for Windows), `lib/fc-runner-java.jar` (shaded), `runtime/` (jlink
  image), `PROTOCOL` (control protocol integer), `VERSION`, `LICENSE`, `THIRD_PARTY_NOTICES`.
- **jlink module set matters more than for the Java host.** User jars may use `java.xml`, `java.sql`,
  `java.net.http`, `java.naming`, `java.logging`, `jdk.crypto.ec`, `jdk.zipfs`, `jdk.unsupported`,
  `java.management`. `ApiOnlyParentLoader` resolves JDK types against the *platform loader*, so a
  module missing from the image means `NoClassDefFoundError` in a customer jar. Recommendation:
  ship `--add-modules java.se,jdk.crypto.ec,jdk.unsupported,jdk.zipfs,jdk.management` (the
  full `java.se` aggregate; image grows to roughly 60-80 MB **(unverified size)**) rather than a
  `jdeps`-minimised set, and say in docs which are present. The Java host's own `Dockerfile` minimises
  with `jdeps` because it controls the code; a function host does not.
- **Per OS/arch matrix:** darwin-arm64, linux-amd64, linux-arm64 first; darwin-amd64 and windows-amd64
  if there are users **(owner question)**; the Go fcdev ships all five. Linux target is **glibc**; the
  production image uses a glibc base (distroless/debian-slim `java-base`-style) and unpacks the
  verified tarball, so ECS runs the same bytes. musl (Alpine) is not offered for the installable.
  Build jlink on native runners per OS (cross-jlink is possible with foreign jmods **(unverified)**).
  macOS arm64 requires the launcher/`java` binary to carry at least an ad-hoc signature to run; a
  jlink image's executables are copied from the JDK, but whether `codesign -s -` is needed after
  `jlink` is **unverified** and listed as a risk.
- **JRE on PATH (rejected for the installable):** runner needs a specific Java (class-file
  support, `--enable-native-access`/`--add-opens` flags, virtual-thread behaviour), users have
  whatever Java their project uses, and a function author on Java 21 would silently run the runner on
  21. A bundled runtime pins it, makes `install` hermetic, and matches production. The only cost is
  the larger download. (The *author's* JDK for compiling is separate and required anyway for
  `fn build`.)
- **GraalVM native: not possible for the runner.** A native image has no runtime class loading of
  arbitrary bytecode (`URLClassLoader` over a jar cannot define new classes in a closed-world image);
  the entire isolation model (§4.1) depends on exactly that. Consequences: the runner is always a JVM
  process; startup is JVM startup (~hundreds of ms to a second; AppCDS/AOT cache is an optional later
  win, see the owner's Java AOT cache TODO); `fcdev` itself stays a native Go binary that merely *launches*
  the runner as a child process, which is also why the install mechanism exists. (The Java repo's own
  `fcdev` is a GraalVM native binary with `fcdev/native-config`; how it hosts jar functions given that
  is **not verified** and irrelevant here.)

---

## 5. `fcdev` changes

Existing relevant code: `cmd/fcdev/upgrade.go` (download, sha256 sidecar verify, extract tar.gz/zip,
atomic replace), `.github/workflows/release-fcdev.yml` (cross-compile; per-archive `.sha256` sidecar;
**cosign keyless signature + certificate for Linux archives only**), `cmd/fcdev/fn*.go` (init, build
via `buildProject`/`detectLang`, describe, run (`localCP`), dev (watch, rebuild, replay), publish,
promote, deploy, set, invoke, status, delete). `fn run`/`fn dev` run the Wasm runner **in-process**
against an in-memory control plane (`fn_run.go` `localCP`).

### 5.1 The installable-runner mechanism (generic)

A small registry, embedded in `fcdev`, of runner kinds. Java is the only entry specified.

```
RunnerKind {
  name              "java"                       // CLI name
  provides          ["jvm"]                      // runtimes (matches platform runtime tags)
  protocol          1                            // control-protocol major it speaks
  source            github:flowcatalyst/flowcatalyst-go, tag prefix "fcrunner-java/v"
  asset             "fcrunner-java-v{ver}-{os}-{arch}.tar.gz" (+ .sha256, + .sigstore bundle)
  verify            sha256 (required), sigstore (required once phase 4)
  entry             "bin/fc-runner-java" (.cmd on windows)
  serve             args/env contract (below)
  requires          none (bundled runtime)
}
```

- **Verbs:** `fcdev runner list` (installed kinds/versions and available kinds), `install <kind>
  [--version V] [--force]`, `uninstall <kind> [--version V]`, `upgrade [<kind>]` (to the newest
  version *compatible with this fcdev's protocol*), `path <kind>` (prints the entry, for CI and
  Docker use), `doctor` (verifies hashes of installed files against the recorded manifest).
  `wasmtime` later = one more registry entry plus its own artifact pipeline; nothing else in
  `fcdev` changes.
- **Name: `fcdev runner install java`** (top-level `runner` noun). Justification: (a) the thing is
  tool-level, like `upgrade`, `db`, `profile`, not a per-function-project verb like everything under
  `fn` (`fn` subcommands act on a function project or a deployed function); (b) the runner is also
  used by `fcdev start` (local platform with a JVM runner) and by CI/Docker (`fcdev runner path java`)
  with no function project in sight; (c) `fn runner install` nests an install under a command whose
  other verbs all take a project directory or address. A thin alias `fcdev fn runner ...` is not
  worth the surface. Owner question 1.
- **Storage:** `~/.flowcatalyst/runners/<kind>/<version>/` (fcdev already keeps `~/.flowcatalyst/` state,
  e.g. the JWT key, `cmd/fcdev/start.go:199`), a `current` pointer file (not a symlink: Windows), and
  `~/.flowcatalyst/runners/installed.json` recording kind, version, source URL, per-file sha256,
  install time. `FCDEV_RUNNERS_DIR` overrides. Atomic install: extract to a temp dir beside the target,
  verify, rename.
- **Download and verify:** reuse `upgrade.go` helpers (`httpGet`, `verifySHA256`, `extractFromTarGz`,
  `latestRelease` with a different tag prefix). Differences from `fcdev upgrade`, deliberate because
  this code is executed with the developer's credentials in reach: **fail closed** - a missing
  `.sha256` is an error (upgrade only warns, `upgrade.go:115-125`); the archive hash is compared
  against both the sidecar and a **pin compiled into fcdev** (`runners.lock`: kind/version/os/arch ->
  sha256, updated by the release process) for the default version, so a replaced release asset is
  detected even if the sidecar was replaced with it; `--version` for an unpinned version requires the
  Sigstore bundle. Phase 4 adds Sigstore verification in Go (`sigstore-go`) against the identity
  regexp of the runner release workflow, mirroring the `cosign verify-blob` incantation printed in
  `release-fcdev.yml`; it also closes the current gap that only Linux archives are signed. Bundle
  files are extracted with the same path-traversal guards as `extractFromTarGz`.
- **Compatibility:** each runner's `PROTOCOL` file vs `fcdev`'s supported protocol range and the
  platform's protocol (heartbeat `capabilities.protocol`); `install` refuses an incompatible
  runner with an actionable message; `upgrade fcdev` prints "runner java vX is older/newer than this
  fcdev supports".
- **Start as child process:** `fcdev start` (with `--runners java`, or auto when installed and a
  jvm-runtime function exists **(owner question 9)**) and `fcdev fn dev` exec the `entry` with a
  scrubbed environment: only `FC_FUNCTIONS_*` it needs and credentials passed via env (not argv,
  per the credential-hygiene practice in `fn_platform.go`). Lifecycle follows the project's process
  rules: fcdev records the child PID in its own pidfile mechanism (`pidfile.go`), stops it by that PID
  (never by name/pattern), forwards SIGTERM and waits `DrainGrace`, and kills only the PID it
  started. If the child exits early, fcdev prints its last log lines.

### 5.2 JRE handling

Recommendation: **bundled jlink runtime** (§4.5). `fcdev runner install java` needs no JDK. Doctor
checks `bin/java -version` inside the install. The JDK requirement for *building* a Java function
is separate (§5.3) and checked by `fn build` with a clear message.

### 5.3 `fn init --lang java`, `fn build`

- `detectLang` (`fn.go:43`) gains `langJava`: `pom.xml` (Maven) or `build.gradle[.kts]` (Gradle);
  Maven wins if both exist, `--tool` overrides. Prefer the wrapper (`./mvnw`, `./gradlew`), else `mvn` /
  `gradle` on PATH, else a message naming what to install.
- `fn init --lang java [--package ...]` writes: `pom.xml` (depends on `flowcatalyst-function-api` with
  scope `provided` - the runner supplies it through the API-only parent loader and the jar must **not**
  bundle it (`BUNDLES_API`), shade plugin config relocating everything else, `outputTimestamp`,
  `exec-maven-plugin` describe step), a `BevcoIngest`-style module class, a JUnit test using `FakeHost`, a
  `.gitignore`, `README`. Where the API jar comes from: Maven Central (publish release) vs a project-local
  repo shipped in the scaffold (what the Java fcdev does with `lib/m2`, `function-manifest-authoring.md`
  M3). Recommend Maven Central once released; until then fcdev embeds the jar and writes the local repo
  (owner question).
- `buildProject` (`fn.go:73`) gains `case langJava`: runs the tool, finds `target/*-function.jar`
  (shade output name fixed by the scaffold), and fails if `META-INF/flowcatalyst/function.json` is
  missing. `runtimeFor` (`fn.go:64`) maps `.jar` to `runtimes.JVM`.
- `describeArtifact` (`fn.go:125`) gains a jar path that uses the Go `jar` reader (§2.2) instead of
  instantiating Wasm; so `fn describe` and publish read the artifact identically (the same invariant
  the `runtimes` package states for Wasm/JS).

### 5.4 `fn dev` / `fn run` for Java

Today `fn dev` = build + serve on the in-process Wasm runner at `127.0.0.1:8099` + watch + reload +
replay (`docs/fcdev-fn-dev.md`). For Java the runner must be the real Java runner, so:

- **One code path in the runner:** fcdev starts a **loopback HTTP control plane** (a small server
  reusing the types in `internal/functions/control`, generalising `localCP`) that serves one function,
  its describe, the jar bytes by digest, records `emit`/`dispatch` for the UI (`would create job: ...`,
  the existing printing), and accepts heartbeats. fcdev then launches the installed runner against it
  with `FC_FUNCTIONS_PLATFORM_URL=http://127.0.0.1:<cp>`. No special "standalone" mode inside the
  runner: the dev loop exercises the same adapter as production. This also means `fn run`/`fn dev` for
  Java need no platform and no credentials.
- **Reload:** fcdev's watcher (`fn_dev_watch.go`, polling 300 ms) rebuilds; on success it swaps the
  jar and bumps the local desired state; the runner loads the new version in a **new class loader**,
  switches the live alias, drains and closes the old one (the Java host's "deploy new -> switch alias
  -> undeploy old", plan §5) - no JVM restart. A failed build leaves the previous version serving, as
  today.
- **Flags** unchanged where meaningful (`--send`, `--path`, `--secret`, `--config`, `--db`, `--perms`,
  `--webhook-secret`, `--port`); `--memory` maps to the admission weight; `--runtime` is implied.
- Distinct-from-production caveats printed once, as for Wasm: dispatch targets/pools/connections are
  not resolved; subscriptions/schedules are not fired.

### 5.5 `deploy`, profiles, CI

- `fn publish/promote/deploy` unchanged: `runtimeFor` selects `jvm`; upload content type §2.3;
  `waitReady` messaging improved (§2.4). Prod guard and profile handling (`fn_target.go`,
  `profile_*`) untouched.
- **Profile impact:** none structurally. A target platform without JVM runners yields the new
  `POOL_LACKS_RUNTIME` message; a profile-level `runner` hint is not needed.
- **GitHub Actions for client-code Java functions:** the existing `fn_ci.go` emits workflows for
  the current languages; add a Java template: `actions/setup-java` (JDK 25 or 21), `mvn -B package`,
  install fcdev (`fcdev upgrade` or the release archive with sha256), `fcdev fn deploy --yes` with
  secrets from the repo. No runner is installed in CI (publishing needs only `jar` reading in the Go
  platform). Caveat: the repository is the "client code" in this sentence only for **first-party**
  repos; client-supplied Java must not be deployed to a JVM pool at all (§7).

### 5.6 Release pipeline

New workflow `release-fcrunner-java.yml`, tag `fcrunner-java/vX.Y.Z` (via a `make release-fcrunner-java BUMP=…` target, one per command, consistent with `release-via-make`). Matrix job per OS/arch builds the shaded jar once, runs the conformance suite (§6) against the jlink image, builds jlink, packages, `sha256sum` sidecar, **cosign sign-blob on every OS** (extending the current Linux-only practice), uploads. A second job builds and pushes the container image from the *released, verified* tarball. A fcdev release bumps `runners.lock`. Coupling rule: a runner release declares the `PROTOCOL` it speaks; fcdev and platform releases declare the range they accept.

---

## 6. Conformance and tests

### 6.1 One behaviour suite, two runners

Today the closest thing is `TestSDKExamples` (`internal/functions/runner/examples_test.go`): built
guest artifacts through the in-process runner with a fake control plane (`newFakeCP`), covering
describe plus calls through every auth mode, and the `fnfixture` guest. It is Go-runner-specific by
construction (in-process, wazero). Extend it into runner-agnostic **vectors**:

- `conformance/functions/vectors/*.json` (or `internal/functions/conformance/`): each vector is
  `{name, describe, config/secrets/db, caller (webhook-signed | platform-token with claims | none),
  request{method,path,headers,body}, expect{status, headers (subset), body (exact|json-subset)},
  controlCalls[] (the exact emit/dispatch requests the runner must make)}`. Vectors are data, not code.
- `conformance/functions/fixtures/`: one **fixture function** per runtime implementing the same
  behaviours (echo, headers, config/secret read, emit, dispatch declared/undeclared, fetch
  allowed/blocked/private-address, log, panic/exception, slow, over-body, retry, reject, db
  query/tx). Wasm: Go/Rust/TS builds already exist in `clients/fn-*/examples`; Java: a fixture jar
  built from `clients/fn-java/examples`. Keeping them generated from one table is a stretch goal;
  a hand-kept parity checklist enforced by a test that every vector name has a fixture for every runtime
  is the minimum.
- Harness (Go): starts a **fake HTTP control plane** (the same server fcdev's dev loop uses, §5.4,
  extended to record every call), launches the runner under test (wazero in-process via `runner.New`,
  or the Java runner binary as a child process on an ephemeral port), pushes desired state, waits for
  `LOADED`, executes vectors over HTTP, and compares both the HTTP result and the recorded control
  calls. Control-plane vectors: ETag/304, long-poll wake, unknown runtime skipped, transient fetch
  failure -> `RETRYING` then `LOADED`, permanent failure -> `FAILED` with reason, revision echoed,
  version removed -> unloaded, drain behaviour, 401 -> one token refresh.
- What the vectors must pin (the places the two protocols have already drifted): `retry` (429 +
  integer `Retry-After`, rounded **up**), `reject` (422 + `FlowCatalyst-Outcome: reject`), busy (429),
  webhook HMAC + timestamp tolerance (against `pkg/fcsdk/webhook`), platform token claim mapping
  and `Caller` shape including `*` segment wildcard permission matching, 404 vs 405 + `Allow`, HEAD from
  GET, body-cap and per-endpoint timeout behaviour, CORS preflight, `@vN` needing the permission,
  `NOT_DECLARED` dispatch refusal before any control call, dispatch field limits (256 KiB, metadata
  rules, no `fn.` prefix), emit declared-type check, config/secret undeclared key refusal, fetch
  allowlist and private-address guard, the describe validation matrix (valid/invalid documents; both
  `abi.ParseDescribe` and the Java `Describe` must accept/reject the same corpus - generate the corpus
  from the Go tests).
- CI: Go runner always; Java runner in the `runners/jvm` job and nightly (child process, slower).
  Conformance results printed as a table (vector x runner) so divergence is visible.

### 6.2 Java-specific tests (port from the Java host, re-pointed)

From `function-host/src/test/.../load/` (names verified present): `ApiOnlyParentLoaderTest`,
`ShadowedClassTest`, `HostOnlyClassTest`, `IndependentStaticStateTest`, `ContextClassLoaderTest`,
`FunctionLeakTest` (loader collectable after undeploy), `MetaspaceGuardTest`, `RefusalTest`,
`ServiceLoaderTest`, `SharedFunctionAddressTableTest`, and in `reconcile/`: `MetaspaceFenceForkTest` /
`ReconcilerMetaspaceHeadroomTest` (a real forked JVM with a small `MaxMetaspaceSize`). Add:
class-file-major-version refusal; thread-leak refusal/cleanup; `System.exit` inside a function is
turned into a version failure (a `Runtime.exit` guard via a shutdown-hook-free approach, see §7);
abandoned-invocation counter unloads the version; `RETRYING` not `FAILED` for headroom; JFR/metrics
cardinality cleanup when a function leaves desired state; jlink-image module coverage test (a fixture jar
touching `java.xml`, `java.sql`, `java.net.http`, `javax.crypto`, TLS EC curves).

---

## 7. Security

### 7.1 What the JVM gives, and what it does not

The Java plan is candid and this section restates it without softening. A class loader isolates
*class identity*, not the JVM. What the existing code enforces:

- Pre-load jar scan refusing native libraries, a `java.security.Provider` service entry, and a bundled
  copy of the API package (`Reason.NATIVE_LIBRARY | SECURITY_PROVIDER | BUNDLES_API`).
- `ApiOnlyParentLoader`: functions see the JDK and exactly one API package; no host SDK, Vert.x,
  Jackson, JDBC drivers; `META-INF/services` and resources are filtered so `ServiceLoader` cannot
  discover host providers.
- A named loader per version; a leak test asserting collectability; the metaspace fence.

What remains **fully exposed** to any code in a jar:

| Exposure | Consequence |
|---|---|
| `System.exit` / `Runtime.halt` | Kills the runner and every other function on it. SecurityManager is permanently disabled as of JDK 24 (JEP 486), so there is no supported interception. Mitigation is detection after the fact + restart, or a Java agent/`-javaagent` transformer rewriting `System.exit` call sites (fragile, bypassable via reflection/MethodHandles/JNI-less tricks). Treat as unmitigated. |
| Threads | `new Thread`, executors, timers outlive `stop()`, keep the loader pinned (leak) and run unaccounted CPU. Detectable (thread count by classloader) not preventable. |
| Reflection / `setAccessible` | Within the unnamed module a function can reflect over other loaders' *loaded* classes it can obtain references to, and over JDK internals where `--add-opens` was given (do not give any). It cannot see host classes by name (parent filter) but can reach them through objects handed to it (the `FunctionContext` implementation's class loader, thread-context loaders, `Thread.getAllStackTraces` frames, `ClassLoader.getSystemClassLoader()`!). The system loader returns the host's classpath, i.e. the host's classes by name. The API-only parent does **not** stop `ClassLoader.getSystemClassLoader().loadClass("io.vertx...")`. Unmitigated; first-party only. |
| Files | Full read/write of anything the process user can (the cache dir with other tenants' jars and the desired-state-derived secrets in env/config, `/proc/self/environ` for the runner's own client secret). |
| Network | Any socket, any host: the `httpAllow` list governs only the supported `HttpCaller`. SSRF to the metadata endpoint (`169.254.169.254`), the platform's private API, the database. |
| Credentials in-process | The runner's `client_credentials` secret and the token in memory are reachable (heap inspection via reflection on host objects reached as above). |
| Memory/CPU | One heap, one metaspace, shared cores: noisy neighbour and OOM are runner-wide (§4.4). |
| JNI / foreign function (Panama) | Native libs are refused at scan, but `Linker.nativeLinker()` can call libc without a bundled library unless `--enable-native-access` is withheld (JDK 24+ warns by default, a later release refuses); set `--enable-native-access=ALL-UNNAMED` **off** and add the runtime flag that denies it **(unverified: exact flag/behaviour by JDK 25; test it)**. |

### 7.2 What that means for tenancy

- **First-party code (the owner's, their team's, reviewed and built by their CI): acceptable.** The
  threat model is mistakes, not malice: leaks, runaway loops, exits, version skew. The mitigations in
  §4.4/§7.3 are for blast-radius control.
- **Client-supplied code must not run on this runner.** State it as a platform rule, not a
  guideline: it is arbitrary code in the same process as other tenants' secrets and the runner's own
  credential, with no sandbox. Client-supplied functions stay on Wasm/JS (wazero: capability-only
  imports, memory budget, preemptive deadlines). The platform enforces this, §7.4.
- Multi-tenant *first-party* (several of the owner's own clients' integrations, written by the owner)
  is still weaker than Wasm: one bad deploy takes down the pool. Separate pools per customer-facing
  blast domain.

### 7.3 Minimum mitigations

1. **Separate pools and processes by trust level and by client.** Never share a JVM pool between
   clients' functions unless the owner accepts the shared-fate risk explicitly; one ECS service per
   pool.
2. **Container limits** (task memory, CPU, `pids`, read-only root filesystem, non-root user, no
   capability adds) and `-XX:+ExitOnOutOfMemoryError`; restart is the recovery model. Writable only
   to the artifact cache and `/tmp`.
3. **Network egress control at the network layer** (security groups / egress proxy allowlist), not the
   `httpAllow` convention. Block the instance metadata endpoint (IMDSv2 hop limit 1, or `169.254.169.254`
   denied) so a function cannot fetch the task role's credentials; the runner's own platform credential is
   the only secret in process and is scoped to `platform:function-runner`.
4. **Least privilege for the runner credential** (it is anchor-scope by the existing rule; use a
   dedicated service account per pool, rotated, never reused for humans).
5. **Detect and quarantine**: abandoned-invocation counter, thread-count-per-loader gauge, a metric for
   classes loaded per function; unload on threshold; alarms.
6. **Provenance**: only artifacts published through an identity holding `platform:function:publish:jvm`
   (§7.4); digest pinned end to end (control-plane download recomputes sha256).
7. **Reviewed build**: first-party CI builds from a protected branch; no arbitrary third-party plugin
   in the function's `pom` unreviewed; SBOM attached to the version metadata **(nice-to-have)**.
8. **Do not rely on `System.exit` guards or an allowlisting agent as security controls.**

### 7.4 Platform enforcement

- New permission `platform:function:publish:jvm`, held by a role such as `platform:function-jvm-publisher`
  (anchor-scoped CI service accounts). `PublishCommand` for `runtime=jvm` additionally requires it;
  ordinary `platform:function:publish` is insufficient. Client-scoped principals never get it by default.
- `Function.ClientID` for a function with `runtime=jvm`: allow only platform-owned (`ClientID == nil`) or a
  configured allowlist (`FC_FUNCTIONS_JVM_ALLOWED_CLIENTS`), so a client-scoped admin cannot create a JVM
  function even if somehow permitted. Owner question 11.
- Pool binding: a `jvm` function may only be assigned to pools declared as JVM pools (by name prefix
  `jvm-` as a cheap convention, or a pool table later). Prevents accidental placement; the capability
  negotiation in §2.4 already prevents running on a Wasm runner.

### 7.5 Signatures

The Java platform verified Sigstore bundles at publish. This platform has no signer policy today;
trust = digest + upload permission. Recommendation: do not port the Sigstore verification into the
runner for now (G9); rely on §7.4's permission and a separate first-party CI identity. If a signer
policy is wanted later it belongs in the Go platform's publish path for *all* runtimes, not in the
Java runner. Owner question 10.

---

## 8. Phases, sizes, delegation, risks, first slice

Sizes: S = a day or less of focused work, M = a few days, L = a week or more.

| Phase | Content | Size | Depends on | Delegation |
|---|---|---|---|---|
| P0 | Spike + measurement: copy-and-trim host to `runners/jvm`, control adapter for desired/heartbeat/artifact only, serve one hello jar; measure RSS, first-call, steady throughput in a Linux container (rig in `../flowcatalyst-javalin/bench/function-host`) | M | owner go-ahead | Mostly main session (design judgment: G2, G6, G16); mechanical copy/trim to Sonnet with a spec |
| P1 | Platform: runtime tag + migration 083, jar reader/scan, desired `runtimes` filter + heartbeat `capabilities`, content types, runner skips unknown runtime, tests incl. hostile zips | M | rulings Q3, Q5 | **Sonnet** (bounded spec: file list in §2.6, error codes, test matrix); migration/lock review in main |
| P2 | Runner core: full control adapter (emit, RETRYING, revision, redirects), `/fn/...` listener, `Describe` reader, auth modes, limits/permits, drain, metrics, idle unload | L | P0, P1 | Split: adapter + listener to Sonnet against conformance vectors; auth/claims and G6 semantics reviewed by main |
| P3 | Dispatch capability (G11) in SDK + runner | M | P1 | Sonnet (spec = `java-handoff-2026-10-15-function-dispatch.md` + §3) |
| P4 | Java SDK: `Fn` registry, `Describe` tool, `Result.reject`, `FakeHost`, scaffold pom/gradle, example (bevco) | L | Q4, Q7 | Sonnet for FakeHost/tests/scaffold; **main for the public API surface** (it is a contract) |
| P5 | Conformance harness + vectors + fixtures (Go harness, Java fixture jar) | M | P2 | Sonnet (vector authoring from existing tests), main reviews what each vector pins |
| P6 | `fcdev runner` mechanism + `runners.lock` + install/verify/uninstall/upgrade/doctor/path + child-process lifecycle | M | P2 artifact exists | **Sonnet** (spec §5.1) |
| P7 | `fn init/build/dev/run --lang java`, loopback HTTP control plane, class-loader reload | M | P4, P6 | Sonnet for init/build; main for the dev control-plane design |
| P8 | Release pipeline: jlink matrix, cosign on all OS, container image from verified tarball, ECS task docs, `make release-fcrunner-java` | M | P2 | Sonnet (workflows) with main reviewing signing identity regexes |
| P9 | Hardening: abandoned-invocation quarantine, thread/class gauges, load test vs Go, runbook | M | P2, P8 | main |
| P10 | Public routes (G17), Sigstore in `fcdev` (§5.1), alias prefixes | M | P2 | Sonnet |
| P11 | `wasmtime` runner kind | L | generic mechanism (P6) | parked (Q13) |

Needs owner rulings before work starts: Q1 (command), Q3 (runtime fixed per function), Q5 (limits/pool out of the jar), Q8 (home), Q10-11 (trust model), Q4 (declaration style).

### 8.1 Smallest useful first slice

**Goal:** a Java runner serves one jar function against the Go platform in local dev, no production
packaging, no `fcdev runner install`.

1. Platform (P1 minimum): `jvm` accepted by `ValidRuntime`/`runtimes.Valid`; migration 083;
   `jar` reader that extracts `function.json` and applies `abi.ParseDescribe` (skip the zip-hardening
   extras initially but keep the size/entry caps); desired-state `runtimes` filter (so the existing
   in-process Go runner in `fcdev start` is not served the jar); upload content type;
   `FC_FUNCTIONS_RUNNER_URL_JVM`.
2. Runner (P0): `runners/jvm` with `GET /desired?runtimes=jvm`, artifact by digest (200 only, no
   redirect), heartbeat with `LOADED/FAILED/RETRYING`, `/fn/{addr}/{path}` listener with webhook
   and `platform` auth, `config`/`secret` capabilities, `emit`. No dispatch, db, public routes,
   drain polish.
3. SDK: minimal `Fn` registry (`webhook`/`platform`/`open`), `Describe` main, `Result` including
   `reject`.
4. Manual loop: `mvn package` (with the describe step), `fcdev start`, run the runner from its
   build directory with `FC_FUNCTIONS_*` env, `fcdev fn deploy hello.jar`, send a signed webhook,
   observe the emitted event. Exit criteria: 5 conformance vectors (echo, signed webhook accepted/
   rejected, platform token accepted/rejected, retry) pass on **both** runners.

Delivers a real answer to "does the contract hold and how does the JVM behave", before any
installer, pipeline, or SDK polish is built.

### 8.2 Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Two runners drift on observable behaviour (retry/reject/auth/describe) | high | high | §6 vectors in CI from P5; the first-slice exit criteria already include them |
| Transient failures reported `FAILED` (latched, no recovery short of re-publish) | high (easy bug) | medium | G6 rule + dedicated vectors + code review checklist |
| Existing Go runner fails any `jvm` version it sees | certain without §2.4 | high | server-side `runtimes` filter + runner skips unknown runtime; ship together |
| No per-function memory/CPU enforcement; one function takes down a pool | medium | high | pools by trust/client, quarantine, restart-as-recovery, honest docs; first-party only |
| `System.exit`/threads/reflection from a jar | medium | high (shared fate) | first-party only (§7.4), separate pools, no reliance on agents |
| Client-supplied code ends up on a JVM pool | low-medium | severe | publish permission + client allowlist + pool convention; policy stated here |
| Copy-and-trim forks the Java host and bug fixes in `../flowcatalyst-javalin` stop flowing | medium | medium | explicit "forked at <sha>" note in `runners/jvm/README`, periodic diff review; decide P0 whether to make it the upstream (Q8) |
| jlink image misses a JDK module a customer jar needs | medium | medium | ship `java.se`, module-coverage test, document the set |
| macOS arm64 jlink image won't execute without codesign; notarization/quarantine for downloaded binaries | medium | medium (dev UX) | CI codesign step; fcdev downloads don't set quarantine **(unverified)**; test on a clean Mac in P0/P8 |
| Reproducibility: every build is a new digest/version | high | low | scaffold sets `outputTimestamp`, deterministic descriptor injection |
| Mixed-runtime URL routing mistakes (delivery to the wrong runner) | medium | high | runtime fixed per function; per-runtime template; e2e test of subscription and dispatch to each runner kind |
| Describe/`function.json` validation implemented twice (Go, Java) | certain | medium | one corpus, both must agree (§6); Go is authoritative at publish |
| Virtual-thread pinning / blocking in third-party libraries inflates latency | low-medium | medium | measure in P0/P9; consider a bounded platform-thread executor for known-blocking libraries |
| Cold start of a JVM runner (seconds) vs readiness expectations | low | low | readiness gate on first reconcile; AOT/CDS later |
| Windows support cost (launcher `.cmd`, paths) | medium | low | defer Windows (owner question 15) |
| Maintenance: a third SDK language (Java) + a second runner + a pipeline | certain | medium | the owner's stated reason is core-domain edge functions; revisit scope after P5 |

---

## 9. Open owner decisions (with recommendation)

1. **Command name.** `fcdev runner install java` vs `fcdev fn runner install java`. *Recommend
   `fcdev runner ...`* (tool-level noun; used by `start`, CI and Docker too; §5.1).
2. **JRE: require on PATH vs bundled jlink image.** *Recommend bundled jlink* with the `java.se`
   aggregate; `fn build` separately requires an author JDK (§4.5, §5.2).
3. **Runtime per function vs per version.** *Recommend fixed per function* (set by the first
   version; mismatch is `RUNTIME_MISMATCH`); changing runtime = new function (§2.1, §2.5).
4. **Declarations: annotations vs builder/registry.** *Recommend the registry* mirroring
   `clients/fn-go`/`fn-ts` (declarations in code, handler bound where declared, describe generated at
   build by running the declaration code; annotations only later as sugar) (§3.2).
5. **Pool / limits / warm / public routes in the jar's manifest (Java plan) vs platform settings (Go
   plan).** *Recommend platform settings only*; the jar carries the describe plus a `jvm` block (§2.2).
6. **Listener: Vert.x vs other.** *Recommend Vert.x with virtual-thread invocation* (proven in the Java
   host; dependency hidden by the API-only loader) (§4.3).
7. **Java target version.** *Recommend SDK `--release 21`, runner on JDK 25 (LTS)*; make the SDK 25 only
   if the Java framework requires it (§3.6).
8. **Where the source lives.** *Recommend this repo* (`runners/jvm`, `clients/fn-java`) so contract,
   conformance, fcdev and releases move together; fork the Java host at a recorded sha (§4.1).
9. **When does `fcdev start` launch the Java runner?** *Recommend only when installed and
   `--runners java` or the platform has a `jvm` function* (explicit first, auto later); `fn dev` always
   uses it for Java projects (§5.1).
10. **Signing / verification.** *Recommend:* fail-closed sha256 sidecar + hash pin compiled into fcdev
    for the default version now; Sigstore bundle verification in fcdev (all OSes signed) in P10; no
    signer-policy port into the platform now (§5.1, §7.5).
11. **Who may publish a `jvm` function.** *Recommend a new permission
    `platform:function:publish:jvm`, platform-owned functions only (or a client allowlist), JVM
    pools by naming convention* (§7.4).
12. **Artifact size cap.** *Recommend keep 64 MiB* until a real jar needs more; revisit with the first
    EDI/XML-heavy function (§2.3).
13. **`wasmtime` runner now?** *Recommend no.* Specify nothing beyond the generic `RunnerKind`
    registry; add when a measured need appears (the Go runner already covers Wasm).
14. **Release pipeline.** *Recommend a separate tag/workflow* `fcrunner-java/vX.Y.Z`, cosign on every
    OS, container image built from the released tarball, `make release-fcrunner-java`, a `runners.lock`
    bump in fcdev releases (§5.6).
15. **OS/arch matrix.** *Recommend darwin-arm64, linux-amd64, linux-arm64 first*; darwin-amd64 and
    windows later on demand (glibc only for Linux).
16. **Soft fate of the Java platform's own function host.** *Recommend untouched;* it keeps serving the
    Java platform. This plan neither deprecates nor depends on it, other than the one-time fork.

---

## 10. What I could not verify

- I did not run, build or test anything, and connected to no platform, DB or port. All of §1 is from
  reading source; sizes and effort are estimates.
- Whether `huma` accepts a `PUT` body with a content type other than the declared
  `application/wasm` (affects §2.3), and what `fcdev fn publish` actually sends for a `.js` artifact.
- Whether the control routes are part of `api/openapi.lock.json` (so whether the API-lock diff in §2.6
  is real or empty); which tests enumerate built-in permissions/roles.
- The promote operation's exact checks (`operations` promote path was not read in detail), so the
  `POOL_LACKS_RUNTIME` insertion point is a recommendation, not a located line.
- The Go runner's metric names (`runner/metrics.go` not read) for the dashboard-parity claim.
- Heartbeat tolerance of unknown fields on the Go side (`Capabilities`, `draining`) and the HTTP
  status of `apicommon.Empty` (Java expects 204; Go's `Empty` may be 200/204).
- jlink image size with `java.se`, macOS arm64 jlink/codesign behaviour, cross-platform jlink,
  JDK 25 behaviour of `--enable-native-access` restrictions, virtual-thread pinning on the exact build.
- Database support breadth in the Java `DbPools` (Postgres-only vs others) and how the Java repo's
  native `fcdev` hosts jar functions.
- Whether `fcdev start` already provisions a runner service account (client id/secret) that a
  separate Java runner process can reuse; the in-process runner may use an internal token.
- The Java manifest-to-describe equivalence beyond the key sets compared in §1 (`Manifest.java`
  was read for its key sets, not for every validation rule); the authoritative check is the shared
  corpus in §6.
