# Router Go-Idiom Plan

_Which parts of `internal/router` still carry the shape of the language they were ported
from, what to do about each, and in what order. Companion to
[`router-architecture.md`](./router-architecture.md), which describes the design as it
stands._

## TL;DR

1. **Nothing here is urgent.** The router works, is well tested, and is unusually well
   commented. This is a cleanup backlog, not a remediation.
2. **Only item 1 changes safety.** Everything else makes the code tidier without making it
   safer. If exactly one item is ever done, make it that one.
3. The weakness is not that the code is wrong — it is that **several correctness properties
   live in comments instead of in the types**, and comments do not fail a build.
4. Item 1 is best done **opportunistically**, when the surrounding code is already open for
   another reason. It does not justify a standalone project.

**Status: items 2, 3 and 4 are done.** Items 1, 5 and 6 remain, on their original triggers.

## What is already good Go

Worth stating, so the plan is not read as a rewrite:

- The **queue layer** — `Consumer`/`Publisher` interfaces with `init()`-time driver
  registration per backend — is the `database/sql` pattern, and it is why adding a backend or
  a LocalStack test harness is painless.
- **Context threading** is thorough and correct; `drainGroup`, `runImmediate` and `Stop` all
  handle `ctx.Done()` with real care about what happens to the in-hand message.
- **Panic isolation** per message is correctly scoped.
- The interfaces that exist (`Mediator`, `ConsumerRestarter`, `MetricsSource`) are **narrow
  and declared near their use**.
- `MediationOutcome` as a value with a result enum you switch over is clear and
  allocation-light.
- The **passive-pool split** — pools do not poll — is sound in any language.

---

## 1. Give each message group its own goroutine

**Priority: highest. The only item that changes safety.**

`groupQs` is a mutex-guarded `map[string]*groupQueue`, each carrying a `working bool`. A
drainer is spawned only when the flag is false, and only the drainer that set it may clear
it. The code has to *state* that invariant in a comment:

> the single-drainer invariant holds because tryDrainGroup spawns only when working is false,
> and only the drainer that set the flag clears it

That is a hand-maintained correctness property — a future edit can violate it silently and
nothing will complain.

**Why it looks like this:** the JVM emulates single-consumer ordering across a *shared* worker
pool, because threads are too expensive to dedicate one per group. Goroutines are not.

**Change:** one goroutine per live group, owning a channel of messages. Ordering becomes
structural — a single goroutine reads its channel in order — and the flag, the buffer mutex,
and the invariant all disappear. The pool-wide concurrency semaphore **stays**; this removes
the flag and the buffer lock, not the cap.

- **Risk:** high. Touches drain, retry re-fronting, release, group flush, and shutdown flush.
- **Trigger:** next time the group machinery is opened for another reason.
- **Done when:** no `working` flag exists and no comment needs to explain why two drainers
  cannot co-exist.

## 2. Stop swapping the semaphore channel — **done**

Concurrency was a buffered channel held in an `atomic.Value` and **replaced wholesale** to
resize, which forced every worker to snapshot the channel locally so its release returned to
the same channel it acquired from. That hazard was documented in a comment and bought purely
in exchange for online resizing — a transplanted `ThreadPoolExecutor.setCorePoolSize()`.

**Done:** `semaphore` (`semaphore.go`) — a counting semaphore whose limit is just a number,
so acquire and release agree on nothing but the one object. Ctx-aware, FIFO waiters, and the
cap read lock-free for the capacity gauge. `loadSem` and `Pool.concurrency` are gone;
`Pool.Concurrency()` now reads `sem.capacity()`, the single owner of that figure.

A shrink is admission-only: running deliveries are never interrupted, so the pool converges
on the new cap as they finish (the old swap allowed `old_in_flight + new_cap` meanwhile).
`semaphore_test.go` pins the bound, FIFO service, cancellation taking no slot, grow/shrink,
and — the property the swap could not offer — that resizing under load conserves slots.

## 3. Collapse the overlapping views of "in flight" — **done**

Four structures described the same population: the `InFlightTracker`, the `mediating` map,
`activeWorkers`, and `queueSize`. The code itself noted that `mediating`'s size always
equalled `activeWorkers` — two sources of truth kept in step by discipline.

**Done:** one owner per fact, the rest derived.

- `mediating` owns "inside a worker"; `ActiveWorkers()` is its size, and the `activeWorkers`
  counter is gone. It is now keyed **per worker** rather than per message id, which also
  fixes a latent dashboard bug: two copies of one message can briefly sit in two workers
  (that is what the process-time dedup backstop is for), and id-keying under-reported the
  count and let the loser's exit delete the owner's entry.
- `queueSize` keeps ownership of the pre-dispatch backlog — it cannot be derived from
  `groupQs`, because the IMMEDIATE path never buffers — and the capacity ceiling it is
  checked against is now derived in one place (`Pool.queueCapacity()`).
- The `InFlightTracker` boundary is stated in the struct: process-wide, ownership rather
  than work, and reaped. The unused `Pool.InFlight() int64` shim is deleted.

## 4. Narrow the manager's lock — **done**

A single `Manager.mu` covered pools, consumers and queues, and was taken on the **hot path**
for every message via `poolForMessage`. Since per-client fallback pools were added it could
also *create* a pool while held.

**Done:** `poolMu` (pools) and `consumerMu` (consumers + queues), both `RWMutex`. Routing and
ack/nack resolution are read-locked; no method holds one while taking the other — they are
taken in sequence, never nested — so there is no lock ordering to get wrong. Synthesis of a
per-client fallback pool moved out of the read path into a double-checked `ensureFallbackPool`,
which is the only write routing performs.

`Reconfigure` no longer holds a data lock across a **broker connect**: it deregisters under
the lock, then builds and tears down connections outside it, re-taking the lock per consumer
to install it (double-checked against the stalled-consumer watchdog having respawned the same
queue meanwhile). Whole reconciles are serialised against each other by a separate
`reconfigureMu`, which guards no state — it only keeps two reconfigures, or a reconfigure and
a shutdown, from interleaving as they could not before the split.

## 5. Restructure the outcome enum before it grows again

`processResult` now carries five states — `processDone`, `processDiscarded`, `processRetry`,
`processRelease`, `processDuplicate` — and every caller must handle all of them, though only
the ordered drainer cares about the difference between a successful completion and a
discarded failure.

**Change:** a small struct carrying the broker action plus the group effect, saying
explicitly what the enum currently encodes positionally.

- **Risk:** low, but it touches every call site.
- **Trigger:** the moment a sixth state is proposed.

## 6. Retire the enterprise suffixes

`HealthService`, `WarningService`, `LifecycleManager`, `PoolMetricsCollector`,
`StallDetector`, `PoolStatsProvider`, `BreakerRegistry`, `HostPoolRegistry`,
`GroupFlushRegistry`.

Purely cosmetic, and the clearest single signal of where the design came from. Go's own
vocabulary is plainer: `Server`, `Client`, `Pool`, `Ticker`.

- **Risk:** none, but it churns diffs and blame.
- **Do:** opportunistically, when a file is already being edited.
- **Do not:** spend a dedicated pass on it.

---

## Sequencing

| Order | Item | Changes | Standalone? | Status |
|---|---|---|---|---|
| 1 | Group goroutine | **Safety** | No — do it alongside other group work | open |
| 2 | Semaphore | Removes a documented footgun | Yes | **done** |
| 3 | In-flight state | Clarity | Yes | **done** |
| 4 | Manager lock | Contention | Yes | **done** |
| 5 | Outcome enum | Clarity | On the next state added | open |
| 6 | Naming | Cosmetic | Never on its own | open |

Item 1 is unaffected by the three above: the `working` flag and the single-drainer comment
are still there, and the pool-wide concurrency cap it must keep is now `sem` rather than a
channel — an `acquire`/`release` pair a per-group goroutine can call exactly as the drainer
does today.
