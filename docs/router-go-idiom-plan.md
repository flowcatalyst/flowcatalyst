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
4. Items 1 and 2 are best done **opportunistically**, when the surrounding code is already
   open for another reason. Neither justifies a standalone project.

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

## 2. Stop swapping the semaphore channel

Concurrency is a buffered channel held in an `atomic.Value` and **replaced wholesale** to
resize, which forces every worker to snapshot the channel locally so its release returns to
the same channel it acquired from. That hazard is documented in a comment and bought purely
in exchange for online resizing — a transplanted `ThreadPoolExecutor.setCorePoolSize()`.

**Change:** a weighted semaphore (`x/sync/semaphore`), or a fixed channel with resizing
handled as admission control rather than by swapping the primitive.

- **Risk:** moderate; well covered by the existing concurrency tests.
- **Independent of item 1** — can be done alone.

## 3. Collapse the overlapping views of "in flight"

Four structures describe the same population: the `InFlightTracker`, the `mediating` map,
`activeWorkers`, and `queueSize`. The code itself notes that `mediating`'s size always equals
`activeWorkers` — two sources of truth kept in step by discipline.

They serve genuinely different readers (dedup, dashboard, metrics, backpressure), so this is
**consolidation, not deletion**. Establish one owner per fact and derive the rest.

- **Risk:** moderate. Each has a distinct consumer that must keep working.

## 4. Narrow the manager's lock

A single `Manager.mu` covers pools, consumers and queues, and is taken on the **hot path** for
every message via `poolForMessage`. Since per-client fallback pools were added it can also
*create* a pool while held.

**Change:** separate locks per concern; move pool creation off the routing path (pre-create
on first config sync, or create under a dedicated lock).

- **Risk:** low. Mechanical; contention is the only thing at stake.

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

| Order | Item | Changes | Standalone? |
|---|---|---|---|
| 1 | Group goroutine | **Safety** | No — do it alongside other group work |
| 2 | Semaphore | Removes a documented footgun | Yes |
| 3 | In-flight state | Clarity | Yes |
| 4 | Manager lock | Contention | Yes |
| 5 | Outcome enum | Clarity | On the next state added |
| 6 | Naming | Cosmetic | Never on its own |
