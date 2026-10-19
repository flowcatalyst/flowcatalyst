# Generic runner plumbing - costing (2026-10-19)

*Status: costing only, nothing built. Owner-approved step: build ONE generic mechanism shared by a
future Java (`jvm`) runner and a future Rust/wasmtime (`component`) runner. Costing from reading;
no code run, no process started.*

**Verified by reading:** `docs/java-function-runner-plan.md` (all 952 lines); the Rust parity plan
section 4 (`../flowcatalyst-rust/docs/parity-plan-2026-10-18.md`); in `../flowcatalyst-rust`:
`fc-fnhost-core/src/{control_plane,heartbeat,desired}.rs` headers, `listener/route_path.rs`,
`bin/fc-server/Cargo.toml`; in this repo: `internal/functions/runtimes/runtimes.go`,
`internal/functions/control/control.go` (Heartbeat), `internal/platform/function/control/{desired,document}.go`
(heads), `entity.go` (`TargetURL`, `RunnerPool`), `cmd/fcdev/{start,stop,pidfile}.go` (heads/relevant
spans), `release-fcdev.yml` header, `upgrade.go` function list.
**Inferred (not read):** Go `fn_run.go`/`localCP` internals, the promote operation, heartbeat handler
tolerance of unknown fields, `upgrade.go` fail-open sidecar behaviour (taken from the Java plan),
whether `api/openapi.lock.json` contains the control routes, Rust guest/WIT details beyond the parity
plan, the Java host's wire (taken from the Java plan section 1.2 and the Rust crate, which says it
mirrors Java "field for field").

---

## 0. Answer to the owner question

Yes, it is feasible and the mechanical part is small. `fcdev start` already supervises one function
runner (in-process), already mints a runner service account (`bootstrapLocalCredentials`,
`start.go:245`), already writes a pid file and stops by PID (`pidfile.go`, `stop.go`), already
computes `FunctionsRunnerURL` from a port (`envcfg.go:94`), and already has download + sha256 +
extract + atomic replace code (`upgrade.go`). Launching a downloaded binary as a child, health-polling
it and SIGTERMing it is S-M of new code.

**Is it worth it?** The plumbing alone delivers nothing a user can see until a runner exists, and
neither runner is close:

- The Rust host cannot be installed today: it speaks the Java wire (not the Go control protocol),
  refuses core modules (the only artifact Go's SDKs produce), and is a *feature of `fc-server`*, not a
  binary (`bin/fc-server/Cargo.toml`: "the former fc-fnhost binary had it").
- The Java runner is ON HOLD with 16 unanswered decisions.

So: the plumbing is worth building only if the owner has committed to at least one of the two
runners within the horizon. It is a shared ~M+M+S platform/fcdev investment (section 2) and the
runner-specific work (L each) dwarfs it. Mitigation proposed: prove the plumbing end to end against a
tiny in-repo **test runner** (a ~200-line Go program speaking the Go protocol with runtime
`echo`), so the plumbing ships verified and no user-facing claim depends on Java/Rust landing.

---

## 1. Protocols - exactly what each runner speaks

| | Go wazero (exists) | Java jar host (`flowcatalyst-javalin/function-host`) | Rust component host (`fc-fnhost-core`) |
|---|---|---|---|
| Control wire | **Go protocol v1**: `GET /control/functions/desired?pool&wait` long-poll + ETag + `revision`; heartbeat `{runnerId,pool,startedAt,revision,budget,versions[]}` with states LOADED/COMPILED/FAILED/RETRYING (FAILED latched + fenced); `artifacts/{digest}` (302 presign); single-event emit; `dispatch-jobs` | **Java wire**: `GET .../desired-state?pool` timer poll, flat entry-per-version, `artifactRef`, `signer`, `unload[]`, no revision; heartbeat `{hostId,pool,state,loaded[]}` expects 204; `artifacts/{versionId}`; batch-of-one emit; no dispatch | **Java wire** (verified: `control_plane.rs` URLs `/desired-state`, `/heartbeat`, `/events`; `desired.rs` "Java DesiredDocument field for field"); heartbeat already carries `runtimes[]` (verified `heartbeat.rs`); Sigstore bundles required unless `FC_FN_SIGNATURES=off` |
| Guest artifact | core wasm module, ABI v1 (`fc_abi_v1`, `fc_alloc`, `fc_handle`, `fc_describe`; imports `fc.call/fc.take`; 11 ops); JS on shared QuickJS | jar loaded in a class loader (`Function` interface); jar carries manifest | WASI 0.2 **component** exporting `wasi:http/incoming-handler`, typed WIT imports `flowcatalyst:function@0.1.2`; **core modules refused** (`WASM_CORE_MODULE_UNSUPPORTED`); JS on V8 |
| Describe | artifact self-describes (`fc_describe`), platform runs it at publish | manifest in jar (Java plan: `META-INF/flowcatalyst/function.json`, platform reads zip, never executes) | publisher-supplied manifest; no `emits`/`dispatches` |
| Invocation URL | `/fn/{addr}[@alias\|@vN]/{path}` | `/functions/{addr}[:{v}]/{path}` | same as Java (verified `route_path.rs`) |
| Address | 2 labels `app.name` | 3 labels `app.service.name` | 3 labels (`FunctionAddress`, "wide refactor" per parity plan) |
| Outcomes | 2xx ack, 429+Retry-After retry, 422+`FlowCatalyst-Outcome: reject` | no `reject` | per Java |
| Dispatch cap | yes (op 11) | none | none |

**Protocol adapter definition.** Everything except the guest ABI is a *control/invoke adapter*:
(1) control client: long-poll desired -> internal model, revision echo, F22 state mapping
(transient never FAILED), digest-keyed artifact download with no cross-host bearer forwarding, single
emit, dispatch; (2) describe reader (Go `abi.Describe` -> internal manifest); (3) listener route
`/fn/...` + Go error JSON + outcome headers; (4) auth claims -> `Caller`; (5) 2-label address value
type. The guest ABI (core module vs component vs jar) is *not* adaptable and is each runner's own.

Where the adapter lives, per runner:
- **Java:** the Java plan already chose copy-and-trim with a **runner-side** rewrite to the Go
  protocol (gap table G1-G22). No platform-side shim. Nothing else to decide.
- **Rust:** two options. (R-a) runner-side: a second `ControlPlane` impl behind the existing trait
  (`control_plane.rs` has `desired_state/heartbeat/emit`), plus `/fn/` route, Go describe mapper,
  2-label address - this is the parity plan's "(a) part 1" at **M**, and its stated biggest risk is the
  3-label `FunctionAddress` refactor. (R-b) a platform-side Java-wire shim in Go serving
  `/desired-state`, `/heartbeat`, `/artifacts/{versionId}`, batch events: **rejected** - it forces
  three-label addresses, signatures and a second desired-document renderer into the Go platform to
  spare Rust one client. Recommend R-a, and treat 2-label addressing as an adapter-layer concern
  (synthesize `app.default.name` internally, do not widen the wire).

Net: **one platform protocol (Go v1), every runner is a Go-protocol client.** The plumbing never
learns about Java wire.

---

## 2. Generic plumbing - design and change list

Sizes S <= 1 day, M a few days, L >= a week. Agent sessions in section 4.

### 2.1 Platform (Go)

| # | Change | Where | Size | Risk |
|---|---|---|---|---|
| P1 | **Runtime registry.** Replace the `Valid` two-value test with a registry of known runtimes (`wasm`,`js`, then `jvm`,`component` registered by their phases). Migration relaxes `fng_versions.runtime` CHECK; recommend a pattern CHECK (`^[a-z][a-z0-9]{1,15}$`) plus code validation so later runtimes need no migration. Next free migration is 083 (per Java plan; re-confirm). | `internal/functions/runtimes`, `internal/migrate/sql`, `publish.go`, DTO docs | S | low; migration is the usual goose/sqlc gotcha |
| P2 | **`runtimes` filter on desired state.** Query `runtimes=a,b`; `buildVersions` drops versions whose runtime is not requested; absent => `wasm,js`. ETag already hashes the rendered doc, so each set gets its own. **Must ship before any non-wasm runtime exists**: today an unknown runtime in a Go runner is a FAILED report which the platform latches (Java plan 2.4, citing `runner.go:357` and `heartbeat.go` - not re-read by me). Also make the Go runner skip, not report, an unsupported runtime. | `control/{desired,document}.go`, `fncontrol`, `runner.go` | S-M | high if skipped (latched failures); easy to test |
| P3 | **Capability advertising.** `Heartbeat.Capabilities {protocol, runtimes[], runnerKind, version, extra{}}`, stored in the existing raw-report JSON; additive. Surface in function status API. | `fncontrol.Heartbeat`, `heartbeat.go` | S | unknown-field tolerance of the handler unverified (check it ignores/stores) |
| P4 | **Per-runtime runner routing.** `TargetURL(template, pool, address, path)` gains runtime; template chosen from `FC_FUNCTIONS_RUNNER_URL_<RT>` (RT upper-case runtime tag), falling back to `FC_FUNCTIONS_RUNNER_URL`. Two call sites verified: `operations/wiring.go:164`, `control/dispatch.go:214`. Env doc + envdoc test + fcdev `envcfg.go`. | `entity.go`, 2 callers, `internal/server/envcfg.go`, `docs/environment-variables.md` | S | **Requires `Function.Runtime` fixed per function** (else delivery URLs go stale on live flip). That is a model change: nullable `fng_functions.runtime`, `RUNTIME_MISMATCH` on publish (M, see P5). |
| P5 | **Function-level runtime + publish strategy.** `Function.Runtime` set by first version; `DescribeReader` interface per runtime (wasm/js existing; `jvm` jar-zip reader; `component` reader). Keeps hostile-artifact limits per reader. Artifact content type per runtime (control `artifacts.go`, upload input). | `function/entity.go`, `operations/publish.go`, `control/artifacts.go`, `api/api.go` | M | medium; touches publish for all runtimes - existing wasm/js tests are the net |
| P6 | **Promote guard** `POOL_LACKS_RUNTIME` (no live runner in pool advertises the runtime) and better `waitReady` / status text. Promote op not read; insertion point unverified. | `operations` promote, `fn_platform.go` waitReady | S-M | low |
| P7 | **Docs/contract**: `docs/wire-contract.md` section for `runtimes`, `capabilities`, per-runtime URL; regenerate `api/openapi.lock.json` if control routes are in it (unverified); SPA runtime badge/filter (optional, S). | docs, lock, frontend | S | low |
| P8 | Optional publish-permission per runtime (`platform:function:publish:jvm`) - **runner-specific, not plumbing**; leave to the Java phase. | - | - | - |

Platform subtotal: ~M+M+S (P1-P4, P6, P7 are S each; P5 is the M).

### 2.2 fcdev

| # | Change | Where | Size | Risk |
|---|---|---|---|---|
| F1 | **Runner registry + `runner.json`.** Embedded table of kinds `{name, provides[runtimes], protocol, repo, tagPrefix, assetPattern, entry, defaultPort, healthPath, envContract}`; each tarball also carries `runner.json` (kind, version, protocol, runtimes, entry) which `install` validates against the embedded entry. | new `cmd/fcdev/runner_registry.go` | S | low |
| F2 | **`fcdev runner install\|list\|remove\|path\|doctor`** (+ `upgrade` later). Storage `~/.flowcatalyst/runners/<kind>/<ver>/`, `installed.json` (per-file sha256), `current` pointer file (not symlink), atomic extract-then-rename. Reuse `httpGet`, `verifySHA256`, `extractFromTarGz/Zip`, `latestRelease` (parameterised tag prefix). | new `cmd/fcdev/runner*.go`, light refactor of `upgrade.go` helpers | M | medium: path traversal in extract (existing guards reused), partial installs |
| F3 | **Verification.** Phase 1: **fail-closed** sha256 sidecar (a missing sidecar is an error, unlike `fcdev upgrade`) + hash pin in an embedded `runners.lock` for the default version. Phase 2: cosign/Sigstore bundle verify via `sigstore-go` against the runner workflow identity. | `runner_verify.go`, `runners.lock` | S (sha), M (sigstore) | sigstore-go dependency weight; release-fcdev signs Linux only today (header verified) so macOS/Windows need signing added |
| F4 | **Supervisor.** On `fcdev start --runners a,b` (or auto: installed + platform has a function of that runtime - recommend explicit-only first): allocate port, mint per-runner SA (`bootstrapLocalCredentials` with a per-kind client id, `platform:function-runner`), set `FC_FUNCTIONS_RUNNER_URL_<RT>` BEFORE `devEnvCfg` runs (`envcfg.go:94` precedent), exec child with **scrubbed env** and credentials via env not argv, health-poll `healthPath` with timeout, capture stdout/stderr to `<data>/flowcatalyst/runners/<kind>.log` and tee prefixed lines, restart-with-backoff max 3 then log loudly (dev tool: never silent). | `start.go`, new `runner_supervisor.go` | M | medium: port TOCTOU (pick, release, child binds - retry once on bind failure), startup ordering (runner needs platform up: start after `server.Run` is listening or tolerate 401/connection refused with backoff - runners already long-poll, so start immediately and let them retry) |
| F5 | **Shutdown/orphan safety.** On shutdown: SIGTERM child PID, wait grace (default 30s = runner DrainGrace), SIGKILL **that PID only**. Children recorded in `runners.pid` (kind, pid, exe path); on next `fcdev start`/`stop` a stale entry is reaped only if pid alive AND `/proc`/`ps` exe path equals the installed binary (guards PID reuse). Linux: `Pdeathsig`; macOS: pid file only; Windows: job object, deferred. Never `pkill`/name matching (standing rule). `fcdev stop` also stops recorded children. | `stop.go`, `pidfile.go`, supervisor | M | medium; the one place a bug kills an unrelated process - needs unit tests with fake pid files |
| F6 | **`fn` integration for non-wazero runtimes**: `runtimeFor` by extension, `waitReady` text, `fn status` shows runner/runtimes. `fn dev`/`fn run` loopback control plane (generalise `localCP` to HTTP so any runner can be pointed at it): **runner-specific phase**, not plumbing; list as optional P-dev. | `fn.go`, `fn_platform.go` | S (plumbing part) / M (loopback CP) | `localCP` internals unread |
| F7 | Config/env: `--runners`, `FC_DEV_RUNNERS`, `FCDEV_RUNNERS_DIR`, per-kind `FC_RUNNER_<KIND>_PORT/ARGS/ENABLED`; `docs/environment-variables.md` + test. | `start.go`, `envcfg.go` | S | low |

fcdev subtotal: ~M (F2) + M (F4) + M (F5) + S (F1,F3-sha,F6,F7) ~ 3 M-sized pieces.

### 2.3 Release pipeline per runner

Existing: `release-fcdev.yml` (tag `fcdev/vX.Y.Z`, one ubuntu job cross-compiling Go, sha256 sidecar,
Linux-only cosign). Generic design, size **M** once, **S per additional runner**:

- Reusable workflow `release-fcrunner.yml` (`workflow_call`) + thin per-kind callers; tag
  `fcrunner-<kind>/vX.Y.Z`; `make release-fcrunner-<kind> BUMP=...` (one per command).
- Contract of every runner tarball: `runner.json`, `bin/<entry>`, `LICENSE`, `THIRD_PARTY_NOTICES`,
  asset name `fcrunner-<kind>-v<ver>-<os>-<arch>.tar.gz`, `.sha256`, `.sigstore` bundle on EVERY OS.
- Build is per-kind (the only non-generic step): Java = jlink matrix on native runners (Java plan
  4.5); Rust = a **new thin binary** (does not exist: host is a feature of `fc-server`) built with
  cargo on native runners, macOS needing macOS runners.
- fcdev release bumps `runners.lock`; compatibility = runner `protocol` vs fcdev range vs platform
  advertised protocol, refused at `install` with an actionable message.

### 2.4 Resulting config/env surface (generic)

Platform: `FC_FUNCTIONS_RUNNER_URL_<RT>`. Runner contract (every kind, same names as the Go runner so
operators learn one set): `FC_FUNCTIONS_{POOL,PLATFORM_URL,CLIENT_ID,CLIENT_SECRET,PORT,CACHE_DIR,
MEMORY_LIMIT_MB,...}` plus `PROTOCOL` printed by `<entry> --protocol`. Rust's `FC_FN_*` names must be
read as aliases or renamed in its adapter phase.

### 2.5 Test runner (recommended, S-M)

`cmd/fcrunner-echo` or `internal/functions/testrunner`: a Go-protocol client advertising runtime
`echo`, no real guest. Used by (a) fcdev supervisor tests, (b) platform P2-P6 e2e (the runtimes
filter, capability stored, delivery URL, POOL_LACKS_RUNTIME), (c) the conformance harness skeleton
the Java plan section 6 wants. This is what makes the plumbing verifiable without waiting for L-size
runners.

---

## 3. What the Java and Rust plans must change to use it

**Java plan (`docs/java-function-runner-plan.md`)**
- Section 2.1/2.4/2.5: drop the bespoke `jvm` runtime/`runtimes`/`TargetURL`/`capabilities` work -
  it becomes P1-P5 here; Java keeps only: register `jvm`, jar `DescribeReader` (2.2), content type,
  `platform:function:publish:jvm`.
- Section 5.1: its `RunnerKind` sketch becomes F1/F2/F4; change asset naming to the generic
  `fcrunner-<kind>-...` + `runner.json`; verify = F3; `fcdev runner install java` unchanged.
- Section 5.6/8 P6/P8: the pipeline and installer phases shrink to a Java build job calling the
  reusable workflow; P1 platform phase is replaced by "plumbing P1-P6 done".
- Decisions 1, 3, 9, 10, 13, 14, 15 are subsumed by this document's decisions (section 5). Q13
  ("wasmtime: specify nothing") is now overridden by the owner step: the registry is explicitly
  shared with a second kind.
- Heartbeat `capabilities`: Java fills `{runnerKind:"java", runtimes:["jvm"], jvm:{javaMajor,apiVersions}}` in
  the `extra` field.

**Rust parity plan (`../flowcatalyst-rust/docs/parity-plan-2026-10-18.md`)**
- Option (a) is the *only* Rust option that uses the plumbing, and it must be re-scoped: **not**
  "run Go core modules on wasmtime" (part 2, L, preview1 + fc.call/fc.take + 11 ops + QuickJS) but
  "run **components** as runtime `component` against the Go control plane": part 1 only (Go-protocol
  client, M) + `/fn/` route + 2-label adapter + a component `DescribeReader` on the Go side. The Go
  parity of ABI v1 on wasmtime stays a separate optional item. (Inference: this changes what the
  Go platform can do with Rust SDK artifacts: they run only on the `component` runtime.)
- New deliverable: a runner binary target. Either a thin `bin/fc-fnhost` re-created from the
  `function_host` module of `fc-server`, or a documented `fc-server --functions-only` mode - needs a
  size, binary name, and the release job.
- Signatures: Rust requires Sigstore bundles; Go never signs (`events.go:144`). The runner must run
  with `FC_FN_SIGNATURES=off` when installed by fcdev against a Go platform (trust = digest +
  publish permission, plus plumbing verification of the *runner binary*, not of functions).
- P4 (Rust serving Go's control plane, option c) and this are independent: after P4 the same
  plumbing would let a Rust platform spawn runners, but that is out of scope.
- Dispatch (op 11) for components has no WIT interface: `dispatches[]` declarations would be
  rejected at publish for `component` until a WIT `dispatch` import exists (runner-specific gap).

---

## 4. Ordered build plan

Effort in agent sessions (one bounded batch each, per the orchestration rule: specs in main
session, bounded implementation to Sonnet, main reviews; verify test claims).

| Phase | Content | Depends | Parallel with | Gate | Sessions |
|---|---|---|---|---|---|
| 0 | Owner decisions D1-D6 (section 5); confirm migration number; read promote op and heartbeat handler (resolve the three unverified points) | - | - | decisions logged | 0.5 (main) |
| 1 | **Platform core**: P1 registry+migration, P2 filter (+ Go runner skip), P3 capabilities, P7 contract doc | 0 | Phase 2 | `go test` incl. `-tags=integration` for the PG tests; spec diff vs lock; **explicit failure check before any commit** | 2 (Sonnet, main reviews) |
| 2 | **Test runner** (2.5) + conformance harness skeleton | 0 | Phase 1 | echo runner loads an `echo` version via filtered desired state; unmodified Go runner never sees it | 1 |
| 3 | **Routing + function-level runtime**: P4, P5, P6 | 1 | Phase 4 | e2e: subscription and dispatch to `echo` runtime hit the right URL; runtime mismatch rejected; wasm/js regression suite green | 2 |
| 4 | **fcdev install**: F1, F2, F3-sha, F7 (+ `runners.lock`) | 0 | Phase 3 | install/list/remove/doctor against a local fake release dir (`httptest`); tampered asset and missing sidecar both rejected; traversal-archive rejected | 2 |
| 5 | **Supervisor + lifecycle**: F4, F5 | 2, 4 | - | `fcdev start --runners echo` -> platform sees runner heartbeat with capabilities; kill -TERM fcdev -> child gone; stale pid file reaping test using fake exe; **no process-name kills anywhere** (brief must ban them, set FC_FN_CLI_CREDENTIALS_PATH for any test fcdev) | 2 |
| 6 | **Release pipeline**: reusable workflow, sigstore on all OS, make target, F3 sigstore verify | 4 | 5 | dry run on a throwaway tag against the echo runner; cosign verify-blob passes on all 5 OS archives; `fcdev runner install echo` end to end from a real release | 2 |
| 7 | fcdev fn integration F6 (+ optional loopback CP) | 3, 5 | 6 | `fn deploy` of an echo artifact reports "no runner in pool supports X" correctly | 1-2 |
| - | **Plumbing total** | | | | **~10-12 sessions** (main session ~3 for specs/review) |
| R-java | Java runner phases P0, P2-P5, P9 of its plan (plumbing P1/P6/P8 removed) | 1,3 | Rust | its own first-slice exit criteria (5 vectors on both runners) | ~15-20 (L) |
| R-rust | Go-protocol client + `/fn/` + describe + binary + release job | 1,3,6 | Java | same vectors | ~8-10 (M-L) |

Parallelism: after Phase 0, {1, 2} run together, then {3, 4}, then {5, 6}. Phase 3 and 4 touch disjoint
trees (platform vs `cmd/fcdev`); phases 1 and 3 both touch `internal/platform/function/{control,
operations}`, so sequence them or use a worktree each and merge in order.

Top risks: (1) latched FAILED if P2 slips behind any runtime registration - ship P2 with P1; (2) PID
safety in F5; (3) Rust adapter depends on the 3-label refactor outcome, outside this plumbing; (4)
signing gap on macOS/Windows archives; (5) per-runtime routing assumes runtime fixed per function.

---

## 5. Decisions needed from the owner (max 6)

1. **Runtime fixed per function (set by first version), not per version.** It makes `TargetURL`
   deterministic (P4) and is Java plan Q3. *Recommend yes.*
2. **Rust runner scope: components only on the Go platform (no Go core-module ABI on wasmtime).**
   Cuts the Rust option (a) from L to M-L and removes the preview1/QuickJS port; Rust SDK artifacts
   run only as `component`. *Recommend yes; keep ABI-v1-on-wasmtime parked.*
3. **Protocol adapter location: runner-side only; the Go platform speaks only Go v1.** No
   Java-wire shim in Go. *Recommend yes.*
4. **Commit gate: build the plumbing only with the echo test runner, before either real runner is
   approved?** The plumbing is ~10-12 sessions with no user-visible value until a runner ships.
   *Recommend: build phases 0-3 (platform + test runner, ~6 sessions) now because P2 is also a safety
   fix for any future runtime; defer phases 4-6 (fcdev installer/supervisor/pipeline) until the Java or
   Rust runner is scheduled.*
5. **Start policy and verification level.** `fcdev start` launches runners only when `--runners` /
   `FC_DEV_RUNNERS` says so; install is fail-closed sha256 + compiled pin first, Sigstore in phase 6.
   *Recommend explicit start, sha256+pin first, sigstore with the pipeline.*
6. **Where the shared pieces and runner sources live.** Plumbing, test runner and the reusable
   release workflow in this repo (contract and fcdev change together); Rust runner binary stays in
   `flowcatalyst-rust` (its release job calls the reusable workflow cross-repo or publishes assets
   to this repo's release naming scheme); Java runner per its Q8 (this repo). *Recommend yes; if
   cross-repo release is awkward, the Rust repo owns its own release workflow honouring the same
   tarball contract (`runner.json`, naming, sha256, cosign).*
