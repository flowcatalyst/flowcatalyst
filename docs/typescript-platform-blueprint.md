# FlowCatalyst TypeScript Platform — Blueprint

_Decision record and build plan, 2026-09-03. Owner decisions from the 2026-09-01..03
sessions; evidence from implementing the same ~30 owner rulings in both the Go and Rust
codebases with parallel agent lanes. This document is the entry point for building the
TypeScript platform; the detailed behavioural authority it points to lives in the
sibling repos._

## 1. The decision

**One language everywhere people and agents live; one compiled appliance where the
hard-won delivery semantics live.**

| Layer | Technology | Why |
|---|---|---|
| Platform core (domain, API, auth, scheduler, outbox/stream loops, sync) | **TypeScript on Deno 2** | Fastest change loop, largest LLM corpus, readable by humans and BAs; the platform's bug classes (authz, scoping, semantics) are compiler-agnostic — tests + specs catch them, in any language |
| Message router | **Rust (`flowcatalyst-rust` `bin/fc-router`)** — kept, sealed | The one component where compile-checked exhaustiveness and cancellation structure measurably paid (see §6); rarely changes once conformant; pinned by the conformance corpus |
| Business services / per-client custom apps | TypeScript (or Java where a client demands it) | Ruled earlier: never Rust/Go; BA-readable |
| Frontend | TypeScript (Vue today; unchanged by this plan) | |
| SDKs | TS / Laravel / Java, unchanged | |

**Go**: maintenance-only after cutover. **Javalin**: retired as an implementation; its
specs, conventions and process docs are promoted to shared blueprint (§3).

## 2. The TypeScript stack (decided, smoke-tested on Deno 2.9)

- **Runtime: Deno 2** — default-deny permissions (supply-chain containment), `deno
  compile` (single-binary `fc-dev`), first-class TS.
  **Exit hedge (mandatory, CI-enforced):** web-standard/Node-compatible APIs only;
  every `Deno.*` call behind one adapter module, linted elsewhere; never adopt
  Deno-proprietary services (KV, Deploy primitives); a monthly CI job runs the suite on
  Node so the exit stays measured in days. Node LTS is the no-regrets fallback.
- **HTTP: Fastify 5** (verified under Deno's npm compat).
- **Wire schemas: TypeBox** — a TypeBox schema *is* JSON Schema: the artifact validated
  at the boundary is byte-identical to what `openapi.lock.json` publishes; compiled
  validators sit on the ingest hot path.
- **Domain types: zod 4** — parse-don't-validate; **`.brand()` implements ruling X-10**
  (branded `ClientId`, `RoleName`, `PoolCode`… constructed only through the parser).
- **The TypeBox/zod split is principled, not duplication** (gRPC analogy: open types at
  the wire, real types inside), under three disciplines: (1) TypeBox/OpenAPI is the
  contract's single source of truth — zod never leaks to the wire, TypeBox never enters
  domain logic; (2) exactly one wire→domain translation site per operation (the usecase
  envelope's command constructor); (3) CI diffs the emitted TypeBox schema against the
  lockfile (`dump-spec` pattern).
- **Data: kysely + kysely-codegen + goose** — sqlc's philosophy in TS: schema is the
  truth (codegen introspects the live DB), SQL is visible, no relations magic. Goose
  stays the single migration path, which keeps Go and TS structurally locked to one
  schema during the dual-stack transition. (Drizzle rejected: a year of breaking
  pre-releases with `latest` parked on 0.45 — churn, not modesty. Kysely 0.29 is 0.x by
  label, API-boring in practice.)
- **Strictness as tooling, not culture:** `strictest` tsconfig, exhaustiveness via
  `assertNever`, ESLint/ts-morph rules enforcing the usecase envelope and banning
  `any`/`as`, X-06 in TS (no lenient enum parses; loud read failures), npm lockdown
  (pnpm-style `minimumReleaseAge`, ignore-scripts, provenance, exact pins — never
  floating dist-tags).

## 3. The blueprint to build from: the Javalin repo

`../flowcatalyst-javalin` is retired as an implementation but is the **best process
artifact in the system** — it authored the method every port now uses. The TS build
starts by adapting, not rewriting:

- **`CONVENTIONS.md`** — §2 layout of an aggregate, §3 authorization placement
  (LOCKED: coarse permission at the controller, resource scope in the use case), §4
  wire contract, §8 "how a corner becomes <language> — spec, implement, audit", §9
  new-aggregate checklist. Translate §1 (stack) to §2 above; the rest carries almost
  verbatim.
- **`docs/usecase-envelope.md`** — the envelope + audit-log interpretation, worked
  example, deliberate-deviation format. Port to TS as the foundation module (mirrors
  Go's `pkg/fcsdk/usecaseop`).
- **`docs/process/agent-prompts.md`** — the spec → implement → audit agent pipeline
  (one agent per aggregate, up to three in parallel; audit agents on landed work).
  This is the porting engine; reuse as-is with TS lane briefs.
- **`Claude.md`** — multi-agent translation guardrails ("assert that it WORKS, not that
  it EXISTS", build hygiene under multiple agents). Adopt wholesale.
- **`docs/spec/*.md` (39 files)** — the behavioural specs per aggregate, extracted from
  Go and argued row by row. **These are the port's requirements documents.** Check each
  spec's "GO DRIFT" banner and re-extract against current Go `main` before porting that
  aggregate — Go has moved (owner-rulings work) since most were written.
- **`docs/database.md`** — migrations/partitions ground rules; swap jOOQ codegen for
  kysely-codegen, keep the regenerate-and-diff CI check idea.
- **`docs/language-direction.md`** — the parked record that seeded this decision; keep
  as history.
- **`conformance/`** — the corpus (see §5).

Shared decision authority (applies to TS as to Rust/Go):
`flowcatalyst-rust/docs/owner-questions.md` — the ruled ledger (Part A + every ruled
item + X-01..X-11) — and `flowcatalyst-rust/docs/router-gap-analysis.md` for router
context. `flowcatalyst-go/docs/owner-rulings-todo.md` maps rulings to the Go
implementation for reference.

## 4. The router: Rust, and what "the appliance" includes

**Judgment: keep the Rust router over the Go router** for the long term. Both are now
rulings-conformant; Rust carries the corpus at 26/28 outright, the disposition model as
a pure function, and the compile-checked exhaustiveness that pays in exactly this
component (§6). The Go router remains the **production incumbent during transition**.

**The appliance boundary is not just `fc-router` the crate — it is the standalone
router binary**: `fc-router` + `fc-queue` (broker backends) + `fc-standby` (leader
election), i.e. what `bin/fc-router` builds. Everything else Rust ships today —
platform, outbox/stream processors, scheduler, fc-dev — is **replaced by the TS
platform**, because those are DB-centric loops whose bugs are semantic, not
concurrency-shaped. The TS platform's embedded broker is the same PG tables the router
already consumes; the seam is HTTP (config poll, delivery, `/api/dispatch/process`,
`/api/dispatch/settled`) plus those tables, and it is already proven in production
shape.

**Router completion work before it replaces Go's** — STATUS 2026-09-03: items 2–5
DONE (operator-surface parity incl. mediating/in-flight/force-ack/blocked-groups/
group-flushes + dashboard tabs, api module split, breaker recording centralised in the
mediator, per-client fallback pools with idle eviction). Remaining: item 1 (settled
client + ACK branch, gated on the TS platform) and item 6 (inert force-NACK key fix).
Original list:
1. `/api/dispatch/settled` client + the A-01 ACK-siblings branch — becomes shippable
   only once the TS platform implements the settled endpoint + reaper (ledger A-01
   forbids shipping it earlier; Go's implementation is the reference).
2. Centralise breaker recording in the mediator (Go's `3c3ec7a` lesson; kills the
   forgotten-arm bug class — one instance already found and fixed in Rust).
3. Split `api/mod.rs` (2.7k lines); wire blocked-groups/flush monitoring endpoints to
   the now-public registry accessors.
4. Non-blocking `update_concurrency` shrink (admission-only convergence, X-11).
5. R-59 per-client fallback-pool synthesis + idle eviction — **DONE (ruled ADD, c020f444)**
   (add for Go parity/noisy-neighbour isolation, or skip).
6. Latent (inert) fix: stall force-NACK `in_pipeline` key mismatch (ledger addendum).

## 5. Sequencing

1. **Foundation module** (weeks 0–2): usecase envelope in TS, TypeBox/zod boundary
   toolkit, kysely + codegen against the live schema, the ESLint/ts-morph guardrails,
   `deno compile` fc-dev skeleton, CI (drift guard, Node-compat run, npm lockdown).
2. **Platform port, aggregate by aggregate** (spec → implement → audit lanes per the
   Javalin pipeline), against the **same live schema** and the **unchanged Go router**.
   The wire contract (`openapi.lock.json`) is the drop-in gate per aggregate. Security
   floor first (the rulings already landed in Go define it: PR-3/4 shape, X-02
   containment, X-06 strictness — port behaviour, not bugs).
3. **Data plane last** within the platform: scheduler, outbox ingest, stream loops,
   `/api/dispatch/process` + `settled` + reaper (Go's `718a821` is the reference).
4. **Router swap** as its own cutover, after the platform is live on the Go router:
   finish §4 items, run the conformance corpus + a strict-routing parity soak
   (Rust router shadowing or replacing per environment), then retire the Go router.
   Never swap platform and router in the same step.
### 5a. Router staging soak (owner decision 2026-09-03: run Rust in staging before switching)

**Expected divergence — do not file as a bug:** until the settled endpoint exists in the
target platform's deployment AND the Rust router gains its settled client, a terminally
failed `BLOCK_ON_ERROR` head makes the Rust router **cascade-NACK** the untried siblings
(they redeliver on broker timing — churn, no loss), where Go ACKs them and reports to
`/api/dispatch/settled`. Ledger A-01 forbids Rust shipping the ACK branch before the
platform half; siblings behind a FAILED head redelivering repeatedly in staging is this,
working as designed.

**Soak entry criteria:** operator-parity lane landed (in-flight/force-ack, blocked-groups,
group-flushes, breaker reset, dashboard tabs); conformance corpus green in CI;
`FC_ROUTER_STRICT_ROUTING` OFF (parity posture; flip only as its own experiment).

**Signals to compare against Go over the soak window:**
- Disposition mix per pool (ack-success / ack-failure / release / retry) — same traffic,
  same proportions; any skew is a classification divergence.
- Warning stream by category/severity — new categories firing that Go never fired (or
  vice versa) are the drift detector.
- Breaker open/close episodes per endpoint — count and duration should track Go's.
- Redelivery rate per queue (broker metrics) — expect the BLOCK_ON_ERROR delta above,
  nothing else.
- One induced drain: park a few thousand messages, kill/restart the router mid-drain,
  verify detach/finish + release-remainder behaviour and the drain-rate SLO.
- Leadership flap drill: force loss→regain, confirm polling pauses/resumes and nothing
  in flight is aborted (the class of last week's Go incident).

**Exit criteria:** the corpus green against the staged binary, the six signals within
tolerance for the agreed window, and one clean induced-drain + leadership drill.

5. **Throughput SLO** (reframed by owner): not "30k msg/s" but **"a 1M-message backlog
   drains in under N minutes without degrading live traffic."** The levers live in the
   ingest write path (batching/COPY, partition pruning, payload-by-reference, S3
   claim-check) — staged, pulled on measurement.

## 6. Why this split — the evidence (one paragraph)

Implementing identical rulings in Go and Rust with equivalent agents: every platform
bug found (IDOR, cross-tenant sweeps, a rate-limiter CTE off-by-one, lenient-enum
laundering, a fabricated fixture value) was semantic — no compiler catches them; specs
and tests did, identically in both languages. Every place the compiler measurably paid
(exhaustive matches propagating a new outcome variant, merges failing at build time
instead of runtime, cancellation structure) was in the router. The Javalin repo's own
`language-direction.md` reached the same conclusion from the other side: three
historical defects came out of Go's `default:` switch arm; sealed/abstract outcome
taxonomies made them uncompilable. Hence: compiled exhaustiveness where outcomes and
concurrency live; TypeScript with enforced discipline where change velocity and
readability rule; the conformance corpus and the decision ledger as the treaty that
keeps every implementation honest.

## 7. Open items

- **R-59** (per-client fallback pools in Rust): owner add/skip pending.
- Migration 052 (strict enums for `ParseDispatchStatus` + dispatchjob trio) after the
  X-06 tranche lands; `ParseDispatchMode` stays lenient by ruling X-01; any dispatch
  status CHECK must admit legacy `'ERROR'`.
- ~~Migration 051/052 enum pre-scans against real production data~~ — **DONE 2026-09-03: both scripts returned zero rows on staging and production; deploy gates closed.**
- Deno-vs-Node one-day spike is superseded by the smoke tests only partially: the
  foundation module (step 1) is the real spike; hold the Node fallback decision open
  until it lands.
- Repo naming/creation for the TS platform (suggestion: `flowcatalyst-ts`, with this
  document copied in as `docs/blueprint.md`).
