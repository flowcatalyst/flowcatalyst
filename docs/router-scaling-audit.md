# Go router: CPU-scaling audit (SQS)

_Static, read-only audit at `c9f640c` (2026-10-09). Nothing here was measured: every
"likely" is an inference from the code and from the numbers already in
[`router-implementation-comparison.md`](./router-implementation-comparison.md) §9–§12. The
companion design docs are [`router-architecture.md`](./router-architecture.md) and
[`router-go-idiom-plan.md`](./router-go-idiom-plan.md). This file is not committed._

**Question.** The Go router delivers 17.8k / 29.7k / 45.6k msg/s at 1 / 2 / 4 CPUs (SQS,
like-for-like table, §10.8): 1.67x from 1 to 2 CPUs and 1.54x from 2 to 4. It matches Java
at 1 CPU. Contention or I/O limits are suspected rather than raw CPU speed.

**Status legend.** `CLEAN`: checked and fine. `FINDING`: a real issue, ranked below.
`ALREADY-FIXED`: an earlier fix, checked to still be in the code. `UNVERIFIED`: it could
matter and the code alone cannot settle it.

---

## TL;DR

1. **Measure before changing anything.** The current numbers do not tell the hypotheses
   apart. "Held in the router ≈ 0" at 2 and 4 CPUs fits both an intake-bound router and a
   CPU-bound one, because Go's scheduler runs a newly spawned goroutine next (`runnext`),
   so deliveries always keep up with intake. The one number that separates them is
   **router CPU use as a fraction of its quota**. Nobody recorded it for the like-for-like
   runs. The only reading there is (§9.13, an older build) was **53–71% of quota at 2–4
   CPUs**: a router with spare CPU that still does not scale is waiting on something
   serial.
2. **Part of the shortfall is the measurement setup, not the code.**
   - The 1-CPU figure runs a different runtime mode. `runtimetune` sets `GOMAXPROCS=1`
     there, which measured +21% (§9.6) and removes every cross-thread cost at once. Some
     of the 1→2 step is just leaving that mode.
   - The rig gives the router a CPU quota but no CPU set (`run.sh:610`). The router
     therefore shares cores with the sink and fixture (pinned to 2–9) and with the
     unpinned seeder (`run.sh:642`), on an M4 Pro with 10 performance and 4 efficiency
     cores.
   - All three routers fall short from 2 to 4 CPUs: Rust 1.77x, Java 1.72x, Go 1.54x.
     In §10.1 Go's scaling (1.77x, 1.60x) matches Rust's (1.68x, 1.59x).
   - So the clearly Go-specific problem is a **constant ~1.7x per-message cost** (§12.4),
     with possibly some extra loss from 2 to 4 CPUs on top.
3. **Top code suspect: every delivery goes through one HTTP/2 connection.**
   - Every bench message, like every production dispatch job, targets one URL.
   - The per-host pool adds a second connection only once the first has 100 requests in
     flight (`host_pool.go:111,220`). At ~45k msg/s with millisecond latency it probably
     never does.
   - On that one connection, every request takes `ClientConn.wmu` around a write syscall
     (`x/net/http2/transport.go:1583-1597`). Every response is decoded by one `readLoop`
     goroutine (`transport.go:575`), and each request starts its own goroutine
     (`transport.go:987`).
   - Java sizes its HTTP/2 client to the CPU count, with up to 32 connections
     (`VertxMediationClient.java:63,82-98`).
4. **Next suspects:**
   - one serial poll loop per queue, with `QueueConfig.Connections` parsed and then
     ignored;
   - about ten acquisitions of process-wide locks per message (the in-flight tracker's
     RWMutex takes three write locks per message, `HealthService.mu` takes a write lock
     per message, and every message to the one target hits the same circuit-breaker
     mutex twice);
   - allocation and GC. This is mostly the AWS SDK, which decodes every response into
     `map[string]any`. Our own code adds two pointer-holding retained buffers, and one of
     them keeps **every delivery outcome for 30 minutes**.
5. **No proposed change needs a dependency that is not already there.** The HTTP/2 knobs
   are in `golang.org/x/net`, which is already in `go.mod`, or in the standard library's
   bundled HTTP/2. Everything else is standard library.

---

## 0. What the existing numbers already say

| Steady msg/s | 1 CPU | 2 CPUs | 4 CPUs | 1→2 | 2→4 |
|---|---|---|---|---|---|
| Go, like-for-like (§10.8) | 17.8k | 29.7k | 45.6k | 1.67x | 1.54x |
| Go, cold flood (§10.1) | 17.9k | 31.6k | 50.6k | 1.77x | 1.60x |
| Rust, like-for-like | 29.5k | 44.5k | 78.9k | 1.51x | 1.77x |
| Rust, cold flood (§10.1) | 32.6k | 54.9k | 87.3k | 1.68x | 1.59x |
| Java, like-for-like (paced) | 16.3k | 44.1k | 75.9k | 2.71x | 1.72x |

- **Single runs, ±15% run-to-run spread** (§9.12). Go's 1.54x and Rust's 1.77x from 2 to
  4 CPUs are each one run. The difference between them is within the noise of two
  single runs.
- **CPU time per message decides it.** At 1 CPU, Go runs at ~100% of the quota:
  1 s / 17.8k ≈ **56 µs per message**. At 4 CPUs and 45.6k msg/s:
  - if the router uses its whole quota, the cost is 4 s / 45.6k ≈ **88 µs per message**
    (+57%). That is contention or scheduler overhead, which the profiles will show;
  - if it uses ~70% of the quota, the cost is ≈ **61 µs per message** (+9%). Per-message
    cost is then flat, and something serial limits throughput (a lock that sleeps, one
    goroutine, or a round trip). In that case the CPU profile shows nothing and the
    execution trace shows everything.
- **"Held ≈ 0" is not evidence of an intake bound.** In Go, `go p.runImmediate(...)`
  (`pool.go:578`) puts the new goroutine in its P's `runnext` slot, so it runs before the
  poll goroutine that the netpoller has just woken. A CPU-bound Go router therefore also
  shows held ≈ 0. §10.6 already noted that in Go and Rust the coupling between intake and
  delivery "falls out of one scheduler".

---

## Ranked findings

Ranked by how likely each is to explain the scaling shortfall. The ✱ items are
correctness or stall risks under bursts, ranked separately at the end.

| # | Finding | Status | Likelihood | How to confirm | Single change |
|---|---|---|---|---|---|
| F1 | Measurement setup: GOMAXPROCS=1 mode at 1 CPU, router not CPU-set-pinned, seeder unpinned, P/E cores, no CPU-use or throttling record | FINDING (rig) | **High** for part of the gap | Protocol E1–E3: CPU % of quota, `cpu.stat` throttling, µs of CPU per message, Rust control run in the same session | Rig only: pin the router's CPU set apart from the sink, fixture and seeder; record CPU use per run |
| F2 | All deliveries go through one HTTP/2 connection (one `wmu`, one `readLoop`, one sink `serve` loop) | FINDING | **High** | `/proc/net/tcp` connection count; no "grew per-host" log line; trace shows the `readLoop` goroutine busy ≥60% of the time, or `-pprof=sync` puts `http2` `wmu`/`mu` at the top; E4 split-host run gains ≥15% at 4 CPUs | Keep at least `GOMAXPROCS` client slots per host and pick the least loaded (`host_pool.go`) |
| F3 | One serial poll loop per queue; `Connections` is ignored | FINDING | **Medium–High** if CPU is under quota | Trace: per-cycle split between waiting on the network, waiting for a P, and running; `ReceiveMessage` calls per second per queue from the sqsfix `/stats` | Honour `QueueConfig.Connections` for SQS (N loops per queue) |
| F4 | ~10 acquisitions of process-wide locks per message; RWMutex under a write-heavy load; contended shared atomics | FINDING | **Medium** (more under one pool or many CPUs) | Trace `-pprof=sync`, or mutex profile once enabled (it is off today); top call sites `inflight.go:81/142/224`, `health.go:144`, `circuit_breaker.go:103/125` | Shard `InFlightTracker` by message-id hash |
| F5 | Allocation and GC: SDK `map[string]any` decode; our pointer-holding retained buffers (`rollingCounter` keeps 30 minutes of outcomes, metrics ring reallocation) | FINDING (constant factor) | Medium for the 1.7x **constant**, low for scaling | Allocs-profile delta in bytes and objects per message at 1/2/4 CPUs; `gctrace`; heap `inuse_space` at `health.go:391` growing over a run | Per-pool bucketed counter with no `time.Time` slices (`health.go:362-392`) |
| F6 | Two goroutines and ~6–8 wake-ups per message, cross-P once GOMAXPROCS>1 | UNVERIFIED | Medium–Low | Context switches per message at 2/4 CPUs (was 0.007 at GOMAXPROCS=1); CPU-profile share of `runtime.schedule`/`findRunnable`/`futex`/`wakep` | A fixed worker set per pool instead of `go runImmediate` per message |
| F7 | Shared cache lines (RWMutex reader counts on `poolMu`/`consumerMu`, the one breaker's atomics, the host slot's `inFlight`/`lastUsedNs`, the `protoCounts` counter) | FINDING | Low–Medium; grows with CPU count | `perf` cache-miss counters are not available on Docker Desktop; diff CPU-profile self time in `sync/atomic` and `RLock` between 2 and 4 CPUs | Remove the per-message `lastActivity`/`lastUsedNs` stores, or sample them |
| ✱S1 | Acks taken on the poll goroutine wait out the 5 s linger | FINDING | Not for steady bench; real under duplicate or malformed bursts | Poll-loop stalls in a duplicate-storm test | Mark those acks urgent |
| ✱S2 | HTTP/2 transports have no `ReadIdleTimeout`/`PingTimeout`; one half-open connection hangs all traffic for up to 15 minutes | FINDING | Stall risk, not throughput | Fault injection (drop sink packets) | Set `ReadIdleTimeout` 30 s and `PingTimeout` 15 s |
| ✱S3 | Error storms: one slog line plus one `WarningService.Add` per message, with an O(n log n) sort at eviction | FINDING | Under 4xx or 500 storms only | 404-target run: router throughput and CPU | Rate-limit `warnConfig` per target |

---

## 2. Lock contention

The hot path for an IMMEDIATE SQS message (the bench and the common production shape) is:
poll loop → `route` → `pool.submit` → `enqueueImmediate` → per-pool dispatcher →
`sem.acquire` → `go runImmediate` → `processOneReleasing` → `Mediate` → `ackTracked` →
batched delete → `tracker.Remove`.

### 2.1 Process-wide locks per message (one global lock?) — FINDING (F4)

There is no single global router lock. Instead, about ten acquisitions of locks shared by
the whole process happen per message. In the bench they are all shared by 100 pools and
up to 6,400 workers.

| Lock | Acquisitions per message | Where | Kind |
|---|---|---|---|
| `InFlightTracker.mu` (one `RWMutex` over two maps) | **3 write + 1 read**: `Register`, `EnsureTracked`, `Remove`; `CurrentReceipt` | `inflight.go:18`, `:81` (Register), `:142` (ensureTracked, write lock even on the common "already tracked" path), `:175`, `:224`; called from `manager.go:773-774`, `pool.go:1924-1925`, `pool.go:291`, `pool.go:308` | Critical sections hash a two-string `brokerKey` and touch two maps |
| `HealthService.mu` | **1 write** (`Lock`, not `RLock`, though the map only grows when a pool is first seen) | `health.go:143-152`, via `pool.go:1823-1826` → `manager.go:277-281` | Every pool's every outcome serialises here, then also takes `rollingCounter.mu` (`health.go:381`) |
| Circuit-breaker `mu` for the target | **2** (`Allow`, `RecordSuccess`) | `circuit_breaker.go:103`, `:125`; `mediator.go:372-373`, `:387` | With one target URL (the bench, and `/api/dispatch/process` in production) this is **one mutex for all traffic** |
| `BreakerRegistry.mu` | 1 read | `circuit_breaker.go:241` | Read-mostly; reader-count cache line is shared |
| `HostPoolRegistry.mu` and `HostConnectionPool.mu` | 1 read + 1 read | `host_pool.go:393`, `:216` | Read-mostly; plus the slot's `inFlight` add and `lastUsedNs` store on one shared slot (`host_pool.go:179-180,227`) |
| `Manager.poolMu` | 1 read (`poolForMessage`) | `manager.go:883` | Read-mostly |
| `Manager.consumerMu` | 2 reads (`resolveConsumer` through `honoursDelayedReturn` and `ackTracked`) | `manager.go:448`; `pool.go:267-270`, `:295`, `:1977` | Read-mostly |
| HTTP/2 `ClientConn.mu`/`wmu` | ~3–4 | `x/net/http2/transport.go:1242`, `:1583` | See §4.1. **The one held across I/O.** |
| `sync.Map` loads | 2 (`cachedBreakerKey`, `cachedHostKey`) | `targetcache.go:39-61` | CLEAN: read-only after warm-up |
| Contended atomic | 1 (`recordProto` adds to one counter for `"HTTP/2.0"`) | `mediator.go:113-116` | One cache line written by every delivery |

How likely this is to be the bound: each critical section is well under a microsecond. At
45k msg/s the tracker lock is busy roughly 180k × ~0.2 µs ≈ 4% of the time, and the
breaker lock ≈ 1%. That is too low to cap throughput through queueing alone. Contention
costs more than its hold time once `sync.Mutex` starts handing off and waking with futex,
and those costs grow with the number of Ps. So this is **a contributor that grows with
CPU count, not the likely ceiling at 45k.** It becomes the ceiling if one pool carries
all the traffic, because the per-pool locks below then become global too.

### 2.2 Per-pool locks (global when there is one pool) — CLEAN in the bench, FINDING for single-pool deployments

Per message, per pool: `immMu` ×2 (`pool.go:484,500`), `sem.mu` ×2
(`semaphore.go:43,77`), `mediatingMu` ×2 (`pool.go:778,793`), `metrics.mu` ×1
(`metrics.go:198`), the `x/time/rate` limiter's internal mutex ×2 even when unlimited
(`ratelimit.go:61-63` calls `Wait` without checking `rpm == 0` first, as `IsLimited`
does at `ratelimit.go:41-46`), `rollingCounter.mu` ×1.

The bench spreads these over 100 pools. A deployment that routes most traffic through one
pool (one dispatch pool per tenant, or the platform's own) turns all ten into global
locks, with the per-pool dispatcher (§3.1) as a serial stage in front of them.

### 2.3 Locks held across I/O, a channel send or logging

| Site | Status |
|---|---|
| HTTP/2 client `wmu` held across `Flush` (a write syscall) for **every** request on the connection: `x/net/http2/transport.go:1583-1597` (bradfitz's own TODO: "this flush is for latency, not bandwidth") | **FINDING** (F2) |
| `semaphore.notifyWaiters` closes a channel under `s.mu` (`semaphore.go:101-111`) | CLEAN: a close never blocks |
| `capacityGate.signal` closes outside its lock (`capacity_gate.go:50-56`) | CLEAN |
| `enqueueImmediate` starts the dispatcher goroutine after unlocking (`pool.go:483-494`) | CLEAN |
| `visibilityBatcher.ensureDrainers` starts goroutines under `b.mu` (`visibilitybatch.go:102-117`) | CLEAN: not blocking, and only on the deferral path |
| `deleteBatcher.delete` channel send without a lock (`deletebatch.go:125-135`) | CLEAN |
| `InFlightTracker.Reap` logs (`slog.Warn`) **while holding the write lock** (`inflight.go:390-403`) | FINDING (minor): only on the 5-minute reaper tick and only for entries past the age ceiling, but a burst of reaped entries holds every route and ack behind the JSON handler's write lock |
| `StallDetector.tick` → `tracker.Snapshot()` copies every entry under the read lock once a minute (`inflight.go:334-342`, `:607`) | CLEAN at current sizes (thousands of entries take about 1 ms) |

### 2.4 RWMutex under a write-heavy load — FINDING (F4)

`InFlightTracker.mu` is an `RWMutex` (`inflight.go:18`) whose hot-path mix is 3 writes to
1 read per message. RWMutex costs more than a plain Mutex for writers and gains nothing
here. `HealthService.mu` is an `RWMutex` whose hot path only ever write-locks
(`health.go:144`).

### 2.5 Metric updates under a shared lock per message

`PoolMetricsCollector.addSample` takes a per-pool lock (`metrics.go:196-216`): CLEAN for
contention, FINDING for allocation (§6). `HealthService.RecordPoolResult` takes a global
write lock plus a per-pool lock (`health.go:143-152`): **FINDING**. The histogram uses
atomics (`metrics.go:169-178`): CLEAN.

### 2.6 Sharding by group hash

There is none, and none is needed for groups. Ordered groups each have their own
goroutine and the pool map lock is taken only on enqueue and pop (`pool.go:1003-1018`,
`1116-1128`). Sharding is the fix for the **in-flight tracker**:
- Shard by a hash of the message id. Both maps' entries for one message then live in one
  shard. A broker key always belongs to exactly one message id (one broker message has
  one body), so `Register`'s two lookups stay in one shard.
- This needs an explicit invariant comment and a test for the external-requeue case
  (same message id, different broker id), which already resolves within the message-id
  shard.

**Proposed single change (F4):** a 32- or 64-way sharded `InFlightTracker` behind the
same API. Measure (§9) before writing it.

---

## 3. Single-goroutine funnels

### 3.1 One dispatcher or channel that every message passes through — CLEAN in the bench, three conditional funnels

| Funnel | Scope | Where | Status |
|---|---|---|---|
| Poll loop: poll → route 10 → poll, strictly serial | per queue | `manager.go:1084-1268` (`pollSafely` at `:1180`, `route` at `:1259`) | **FINDING (F3)**, see §5.1 |
| `dispatchImmediate`: pops one message, waits for a semaphore slot, starts a goroutine | per pool | `pool.go:523-579` | CLEAN at 100 pools; a serial stage for a single-pool topology |
| HTTP/2 `readLoop`: every response frame on a connection, and the wake-up of the waiting goroutine | per connection, so **global with one connection** | `x/net/http2/transport.go:575`, `:1903` | **FINDING (F2)** |
| Sink `serverConn.serve` + `readFrames`: one goroutine pair per connection | per connection on the **sink** | `x/net/http2/server.go:766,824` | Same cause as F2; whether it binds shows in sink CPU per thread |
| Delete batcher primary drainer, plus up to 3 helpers on demand | per queue | `deletebatch.go:204-281` | CLEAN; already redesigned (§9.10) |

### 3.2 Unbuffered hot channels — CLEAN

`deleteItem.done` is buffered (1) (`deletebatch.go:44`); `b.items` 1024
(`deletebatch.go:101`); `vis.items` 4096 (`visibilitybatch.go:66`). Semaphore waiters
and the capacity gate are closed, not sent on (`semaphore.go:51,109`;
`capacity_gate.go:50-56`). The IMMEDIATE queue is a slice under a mutex, not a channel
(`pool.go:53-56`).

### 3.3 `time.After` in hot select loops — CLEAN

It appears only on paths that are not per message: an empty or errored poll
(`manager.go:1213,1251`), a retry backoff (`pool.go:641,1217`; `mediator.go:436`).
Go ≥1.23 also collects unreferenced timers. The batcher's linger timer is per batch
(`deletebatch.go:218`).

### 3.4 Per-message timers instead of one heap — UNVERIFIED (low)

Two runtime timers are created and cancelled per message:
- `context.WithTimeout(15m)` per delivery (`mediator.go:462`);
- `callCtx` (25 s) per ack (`sqs.go:342`).

Go keeps timers in per-P heaps, so this does not contend, but there is add/delete churn.
Confirm in the CPU profile by the self time in `runtime.(*timers).*` and
`runtime.(*timer).modify`. The deferral ledger already uses one heap
(`deferral_ledger.go:36-84`): CLEAN.

### 3.5 O(n) scan for the next ready group — CLEAN

The IMMEDIATE path is a FIFO slice, O(1) (`pool.go:499-517`). Ordered groups have one
goroutine each, with no scan, and empty groups are deleted (`pool.go:1119-1125`).
`anyPoolHasRoom` is O(pools) but runs only before a consumer has learned its destination
pools (`manager.go:1347-1377`).

---

## 4. HTTP clients

### 4.1 One connection for all deliveries to one target — FINDING (F2, rank 2)

- `HostConnectionPool` starts with **one** slot (one `http2.Transport`, one connection)
  and grows only when **every** slot has ≥ `StreamsHighWatermark` = **100** requests in
  flight (`host_pool.go:109-118`, `:215-229`, `:231-261`). The cap is 8 slots. The sink's
  HTTP/2 server allows 250 streams per connection, so each slot stays one connection.
- By Little's law, in flight = rate × latency. At 45k/s and ~1–2 ms per delivery that is
  45–90 requests, so the router probably runs **one TCP connection** for the whole
  steady state. If the connection itself adds latency, the in-flight count climbs to 100,
  a second slot opens, and the system settles at about 100 streams per connection. In
  either case, throughput per connection is bounded by that connection's serial parts.
- **Serial parts per request on one connection (x/net v0.54.0):**
  - a `doRequest` goroutine per request (`transport.go:987`);
  - `wmu` taken for the headers (`:1242`), then again for the DATA frame, held **across
    `bw.Flush()`**, which is one write syscall per request (`:1583-1597`);
  - `cc.mu` for stream bookkeeping;
  - the single `readLoop` goroutine (`:575`, `:1903`) parses every HEADERS and DATA frame
    and wakes every waiting goroutine.
  - §9.2 already measured "one write syscall per HTTP/2 request" as a large share of Go's
    delivery cost.
- **Contrast:**
  - Java's Vert.x client gets one event loop per CPU and up to 32 HTTP/2 connections per
    origin (`flowcatalyst-javalin/.../VertxMediationClient.java:63`, `:82-98`), so its
    delivery path grows with the CPU count by construction.
  - Rust's client was not checked: UNVERIFIED.
- **Why this fits sub-linear scaling:** at 1 CPU a serial section is cheap because there
  is nobody to contend with (GOMAXPROCS=1). At 4 CPUs the `wmu` waiters, the `readLoop`
  and the sink's per-connection serve loop cap throughput, while CPU sits idle.
  - Arithmetic, with the costs inside a Docker Desktop VM assumed: a 5–20 µs locked flush
    per request puts `wmu` at 22–90% utilisation at 45k/s. A 5–10 µs `readLoop` cost per
    response puts that goroutine at 22–45% busy.
  - So the ceiling sits somewhere between "already binding" and "binding at about 2x".
    Only a measurement settles which.
- **Confirm:**
  1. `docker exec <router> cat /proc/net/tcp`: count rows with remote port `:2328`
     (9000) in state `01`. Expect 1.
  2. `docker logs <router> | grep 'grew per-host'`: expect none.
  3. A 2–3 s execution trace in the steady window: the
     `golang.org/x/net/http2.(*ClientConn).readLoop` goroutine's running share, and
     `go tool trace -pprof=sync` with `ClientConn` / `wmu` / `mu` at the top.
     **Confirmed if** the `readLoop` runs ≥60% of wall time, or `http2` sites take ≥20% of
     sync-blocking time.
  4. Experiment E4 (no router code change): split the bench target over K = 4 host
     aliases (IP, container name, network alias) so the router builds 4 host pools.
     **Confirmed if** 4-CPU throughput rises ≥15%. This also splits the breaker (§2.1),
     so read it together with step 3.
- **Proposed single change:** keep at least `MinSlotsPerHost = runtime.GOMAXPROCS(0)`
  slots per host, created on first use and never swept below that minimum, and pick the
  least loaded one (the selection code exists: `host_pool.go:350-364`). Standard library
  plus x/net, which is already a dependency. For the experiment, make
  `StreamsHighWatermark` and the minimum slot count env-tunable.

### 4.2 `DefaultTransport` / `MaxIdleConnsPerHost = 2` churn — CLEAN

- Mediation builds its own transport per slot with `MaxIdleConnsPerHost: 10`
  (`mediator.go:233-238`). For `http://` targets the HTTP/2 transport has no idle-pool
  churn (`mediator.go:261-267`).
- SQS uses the SDK's buildable client: `MaxIdleConnsPerHost` 10, `MaxConnsPerHost` 2048
  (`aws-sdk-go-v2@v1.47.1/aws/transport/http/client.go:20-22`). One client per queue
  (`sqs.go:118-122`), with at most 1 poll + 4 delete + 4 visibility calls in flight
  (`deletebatch.go:26`; `visibilitybatch.go:20`), which stays within the 10 idle
  connections. The SDK drains and closes response bodies itself (its close-response-body
  middleware).
- One exception, **dev mode** (`FLOWCATALYST_DEV_MODE`): HTTP/1.1 with one slot and 10
  idle connections (`mediator.go:79-87,240-242`; `host_pool.go:123-127`) churns
  connections above 10 concurrent deliveries per host. That is acceptable for dev, but it
  is why dev mode cannot stand in for E4.

### 4.3 One shared client per target type — FINDING for width, CLEAN for scaling

- **Mediation:** one registry and one client set per origin (`mediator.go:150-164`).
  CLEAN, apart from F2.
- **SQS:** a separate `aws.Config`, credential cache, signer and transport **per queue**
  (`sqs.go:108-142`). That shards the locks well (good for scaling), but at 1,000 queues
  it is the likely part of the 15–40% "width" cost in §9.13. Not a scaling finding.

### 4.4 Bodies drained and closed on every path — FINDING (low)

- 2xx: `io.ReadAll` and a deferred `Close` (`mediator.go:518,528`). CLEAN for reuse, but
  **unbounded**: a target answering 2xx with a large body is read into memory in full.
  Use `io.LimitReader`.
- Every non-2xx branch (`mediator.go:556-633`) closes **without draining**. On HTTP/2
  this costs one `RST_STREAM`; CLEAN. On HTTP/1.1 (dev mode, or an `https` target whose
  server does not offer h2 over ALPN) Go **drops the connection**, so a 4xx/5xx storm
  turns into a dial storm. Fix: `io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))`
  before the close.
- Connection reuse was verified by nothing so far: UNVERIFIED. `httptrace.GotConnInfo.Reused`
  in a test, or the `/proc/net/tcp` count during a run, settles it.

### 4.5 Timeouts everywhere — mostly CLEAN; one FINDING (✱S2)

| Call | Bound |
|---|---|
| Delivery | 15-minute context deadline (`mediator.go:459-464`), by contract |
| Dial / TLS | 30 s / 10 s (`mediator.go:66-77,225-238`) |
| SQS calls | 25 s (`sqs.go:101-106`) |
| Poll | 30 s (`manager.go:1179`) |

**Missing:** HTTP/2 `ReadIdleTimeout` / `PingTimeout` on both HTTP/2 transports
(`mediator.go:254-267`). A half-open TCP connection (NAT or load-balancer idle drop, a
peer that vanished) is noticed only when each request's 15-minute deadline expires. With
F2 putting all traffic on one connection, that is **every delivery stalled for up to
15 minutes**, with the pool slots held the whole time. Fix: `ReadIdleTimeout: 30s,
PingTimeout: 15s` on `h2cTransport` and on the transport `ConfigureTransports` returns.

---

## 5. SQS

### 5.1 Receiver goroutines: fixed, not scaled with GOMAXPROCS — FINDING (F3, rank 3)

- **One** poll loop per queue (`manager.go:1084`). `QueueConfig.Connections` is parsed
  (`internal/common/config.go:28,70-73`) and used only to detect config changes
  (`config_sync.go:420`); nothing starts more pollers.
- Each cycle is strictly `ReceiveMessage` → route 10 → `ReceiveMessage`
  (`manager.go:1180`, `:1259`). Intake per queue is therefore 10 / (receive round trip +
  SDK work + route time + time spent waiting for a P).
- At 45.6k/s over 100 queues that is **~22 ms per cycle**. Java, with the same one loop
  per queue and the same fixture, manages ~13 ms (76k/s).
- Whether that gap is CPU (the loop waits for a P), SDK work, or the round trip is
  exactly what the trace shows (protocol E5).
- If the router is under its quota, this is a prime suspect. If it is at its quota, more
  loops only move CPU around.
- **Proposed single change:** honour `Connections` for SQS (N loops per consumer, sharing
  the `Queue`, whose state is already lock-protected). SQS FIFO never gives one group's
  messages to two receivers at once, so per-group order holds. Scope it to SQS; NATS and
  Postgres have different receive semantics.

### 5.2 `MaxNumberOfMessages = 10`, `WaitTimeSeconds = 20` — CLEAN

See `sqs.go:48`, `:238-246`; `manager.go:1096`.

### 5.3 `DeleteMessageBatch` — ALREADY-FIXED, still in place

- Primary drainer with a 5 s linger, up to 3 helpers, ordered acks urgent
  (`deletebatch.go:20-37`, `:204-281`, `:136-141`; `pool.go:298-300`).
- Pending-delete memory: in flight plus 5 s, pruned from a FIFO (`sqs.go:40`,
  `:590-623`).
- Pooled delete items (`deletebatch.go:57-69`).

### 5.4 `ChangeMessageVisibility` batching — ALREADY-FIXED, still in place

Non-blocking `ChangeMessageVisibilityBatch` with 4 drainers per queue and a 4096-deep
queue (`visibilitybatch.go:16-29`, `:89-100`, `:180-208`); used by `Defer` and `Nack`
(`sqs.go:376-416`).

### 5.5 Other SQS hot-path items

| Item | Status |
|---|---|
| `plainBodyClient` lost-response fix (35db6f5) | ALREADY-FIXED: `httpclient.go:31-46`, installed at `sqs.go:169-176` |
| No MD5 check, no attributes requested (b53b05c) | ALREADY-FIXED: `sqs.go:171`, `:247-250` |
| Zero-copy body parse into a typed struct (3a66362) | ALREADY-FIXED: `sqs.go:303-321` |
| Five per-queue mutex acquisitions per message (`recordPolled`, `alreadyDeleted`, `markDeleted`, `forgetReceipt`, `markDeleteDone`) | CLEAN: sharded per queue (`sqs.go:630-687`) |
| Redelivery of an acked message: a **synchronous single** `DeleteMessage` inside `Poll`, one round trip each, in series (`sqs.go:267-279`) | FINDING (low): a redelivery storm serialises round trips into the poll loop. Route it through `del.delete` as urgent, or fire and forget |

---

## 6. Allocation and GC (F5)

- **Message decode: CLEAN.** It unmarshals into the typed `common.Message`
  (`sqs.go:310`), not into `map[string]any`.
- **The SDK decodes into `map[string]any`: FINDING (SDK, not our code).** Every
  `ReceiveMessage` response goes through `json.Decoder.UseNumber()` into `interface{}`,
  through a `TeeReader` into a 1 KB ring buffer, and is then walked into the output
  structs (`service/sqs@v1.42.27/deserializers.go:1877-1907`). This is the "750 objects,
  64 KB per receive" in §9.15. The only lever is a thin SQS client (SigV4 + JSON,
  doable with the standard library: `crypto/hmac`, `crypto/sha256`, `encoding/json`).
  That is a maintenance decision for the owner, already noted in §9.15.
- **Re-marshalling: CLEAN.** The delivery payload is built directly (`mediator.go:305-317`).
  The message body is never re-encoded.
- **Payload copies: CLEAN.** The body is viewed rather than copied (`sqs.go:310`);
  `QueuedMessage` (~250 B) is passed by value through `immediateItem` (`pool.go:471-474`).
- **Key concatenation: CLEAN.**
  - `brokerKey` is a two-string struct, not a joined string (`inflight.go:38-41`).
  - `batchID` is formatted once per batch (`manager.go:723`).
  - `strconv.Itoa(i)` for `i < 100` does not allocate (`deletebatch.go:290`).
- **Per-message buffers and objects in our code: FINDING (small, each one).**
  - Logger: one `lazyMessageHandler` + one `slog.Logger` + one `context.WithValue` per
    message (`logattrs.go:40-44`; `pool.go:1874-1875`), even though they are used only
    on rare paths. Build them on first use.
  - `http.NewRequestWithContext` parses the target URL per message (`mediator.go:466`).
    Caching a parsed `*url.URL` and cloning the request would save the parse.
  - A header map and four `Set` calls per request (`mediator.go:480-490`).
  - `io.ReadAll` per response (`mediator.go:528`).
  - `context.WithTimeout` per delivery and per ack (`mediator.go:462`; `sqs.go:342`).
  - `NewInFlightMessage` per message (`manager.go:773`). This one is necessary.
- **Pointer-holding retained buffers that grow GC work: FINDING.**
  - **`rollingCounter` keeps every outcome for 30 minutes** (`health.go:362-392`;
    `RollingWindow: 30 * time.Minute` at `health.go:43`). It has no count bound. Each
    entry is a 32-byte `rcEvent{time.Time, bool}`, and `time.Time` holds a `*Location`,
    so the GC scans the whole slice every cycle.
    - Retained memory = 32 B × messages in the last 30 minutes: ~2.6 GB at a sustained
      45k/s, ~58 MB at 1k/s.
    - A 1M-message bench run never expires anything, so the slice grows all run (≈32 MB
      at the end), with append-doubling copies on the way.
    - `successRate()` walks the whole slice under the pool's lock (`health.go:394-412`).
    - This is also a memory finding for long-running production instances.
  - `PoolMetricsCollector.samples`: capped at 10k but re-sliced from the front and then
    appended (`metrics.go:209-215`). It reallocates and copies ~240 KB about every 2,500
    messages per pool, and its `metricSample.ts time.Time` holds a pointer
    (`metrics.go:32-36`).
- **Our own share is small.** §9.15: our code is ~6% of allocations, the SDK >50%.
  - Go's allocator is per-P (`mcache`), so it does not have glibc's cross-thread lock
    that capped Rust (§9.14). Allocation explains the **constant** 1.7x much better than
    it explains the scaling.
  - §12.4 measured GC at 4–8% of CPU. Giving it 3x the memory did not close the gap.
- **Proposed single change (F5):**
  - replace `rollingCounter` with fixed time buckets (for example 60 × 30 s buckets of
    success and failure counts) held by the pool and resolved once in `wirePool`;
  - this also removes `HealthService.mu` from the hot path (§2.1);
  - a follow-up: store `metricSample.ts` as `int64` nanoseconds in a fixed ring, so the
    slice holds no pointers.

---

## 7. Logging on the hot path

| Item | Status |
|---|---|
| Lazy per-message logger, attributes built only when a line is emitted (c7c21a2) | ALREADY-FIXED: `logattrs.go:56-86`. Its 3 allocations per message remain (§6) |
| pprof goroutine labels off unless the debug endpoints are mounted | ALREADY-FIXED: `pool.go:1889-1896`, gated at `internal/server/run.go:112`. **Measurement trap:** enabling `FC_DEBUG_ENDPOINTS_ENABLED` to profile turns the labels **back on** (a label set and two label swaps per message), so a profiled run is not the throughput configuration (see §9) |
| Success path emits nothing above Debug: `routeOne` redelivery is Debug (`manager.go:783`), deferral is Debug (`pool_admission.go:170`), `sendDeleteBatch` logs only failures (`deletebatch.go:300-319`) | CLEAN |
| Handler: one `slog.JSONHandler` on stderr (`internal/logging/logging.go:37-38`), which writes under one mutex with one `write` per line | CLEAN for the success path; **FINDING (✱S3)** for error storms: every 4xx, every 500/505+ and every 3xx calls `warnConfig` → one `slog.Warn` (a global handler lock plus a syscall) **and** one `WarningService.Add` (a global lock; every 100 adds past the 1,000 cap it sorts all 1,000 under the lock, `warning.go:108-125`, `:373-390`) **per message** (`mediator.go:187-193`). A misconfigured subscription that 404s at line rate therefore serialises the router on its logging. Fix: rate-limit `warnConfig` per (target, status) |
| `InFlightTracker.Reap` logs under the write lock | FINDING (minor), see §2.3 |

---

## 8. Correctness under bursts

| Item | Status |
|---|---|
| Channel send while holding a lock | CLEAN (§2.3) |
| Pool slot released on every path: `runImmediate` scopes `defer slot.release()` in a closure (`pool.go:599-603`); the drainer does the same per iteration (`pool.go:1155-1159`); `processOneReleasing` releases before every ack (`:1931`, `:1958`, `:2005`); cancellation after acquire releases (`:569-575`) | CLEAN |
| …except `startImmediate`'s panic boundary (`pool.go:541-548`), which does not release a slot already taken at `:556` if something between `:556` and `:578` panics (`queueDec` → `capacityChanged` → `capacityFreed`, or `nackMsg`) | FINDING (theoretical): it would leak one slot per such panic. Track `acquired` and release it in the recover |
| Error, timeout and cancel paths in mediation: all return an outcome; the 15-minute ceiling is a contract (memory: router-backpressure-restart-deadlock) | CLEAN |
| Idle per-group state: empty `groupQueue` deleted (`pool.go:1119-1125`); parked groups swept (`pool.go:1438-`); synthesised pools evicted (`manager.go:995-`); `deferralLedger` pruned on read (`deferral_ledger.go:66-74`) | CLEAN |
| **Acks issued from the poll goroutine wait out the delete linger (✱S1).** `Ack` blocks until its `DeleteMessageBatch` entry completes (`deletebatch.go:143-164`), and a non-urgent ack waits up to `deleteLingerMax` = **5 s** for a full batch (`deletebatch.go:37`, `:218-226`). Three acks run **on the poll loop**: a malformed message (`sqs.go:284-287`), an external-requeue duplicate (`manager.go:800`), and strict-routing rejection (`manager.go:829`). On a quiet queue each of these stalls intake for up to 5 s. In the 2026-09-22 duplicate-storm shape (republished jobs arriving as `RegisterExternalRequeue`), intake drops to about one batch per linger | FINDING. Fix: `queue.WithUrgentAck(ctx)` on those three calls, or hand them to a goroutine. One-line changes, standard library |
| Redelivery-of-acked `DeleteMessage` in series inside `Poll` (`sqs.go:270-277`) | FINDING (low), see §5.5 |
| Half-open HTTP/2 connection stalls all deliveries for up to 15 minutes (✱S2) | FINDING, see §4.5 |
| `rollingCounter` with no count bound (memory under sustained load) | FINDING, see §6 |
| Unbounded 2xx body read | FINDING (low), see §4.4 |
| `ActiveWorkers()` = `len(mediating)` includes workers that released their slot and are waiting on the ack linger (`pool.go:1868-1869` defers `endMediating` past `ackTracked` at `:2006`) | FINDING (observability only): the dashboard's "Mediating n/limit" can read above the slot count on slow queues |

---

## 9. Measurement protocol

The rig is in `../flowcatalyst-javalin/bench/router`; its knobs are in the memory note
`router-benchmark-rig`. The rules from that note and from `elasticmq-sqs-caveat` apply
unchanged:
- **one rig run at a time**;
- `docker ps | grep bench` and `ps aux | grep 'run.sh run'` before and after every run;
- `SQS_EMULATOR=sqsfix`;
- report **steady (10–90%) and tail**, using the summary drain time rather than the
  `[steady-vs-tail]` label;
- no builds running concurrently, **including the Maven build running now**: wait for it.

### 9.1 Per-run record (every run, every router)

1. **GOMAXPROCS:** `docker logs <router> 2>&1 | grep '"runtime tuning"'` gives
   `gomaxprocs`, `cgroup_cpu_limit`, `gc_percent` and `memory_limit_bytes`
   (`runtimetune.go:236-245`).
2. **CPU use:** the rig already samples `docker stats` every second (`run.sh:686`).
   Report the steady-window mean as **a percentage of the quota**. Also read
   `docker exec <router> cat /sys/fs/cgroup/cpu.stat` at the start and end of the steady
   window: `usage_usec`, `nr_throttled`, `throttled_usec`.
3. **CPU per message:** Δ`usage_usec` / Δ(sink count). The headline derived number; see §0.
4. **Context switches per message:** sum `voluntary_ctxt_switches` and
   `nonvoluntary_ctxt_switches` over `/proc/1/task/*/status` at window start and end,
   divided by messages. Baseline: 0.007 at GOMAXPROCS=1 (§10.2).
5. **Connections to the sink:** `docker exec <router> cat /proc/net/tcp` (and `tcp6`),
   rows with remote port `2328` in state `01`. Do this for Go, Rust and Java alike.
   Also `docker logs <router> | grep -c 'grew per-host'` for Go.
6. **sqsfix `/stats`:** `ReceiveMessage` calls per second (÷ queues = cycles per queue),
   messages per receive, acks per `DeleteMessageBatch`.
7. **Sink and fixture CPU** from `docker stats` (sink and fixture each against one core),
   plus the seeder's CPU for fed runs.

### 9.2 Placement (rig changes: the owner's call; no router change)

- **Pin the router.** The router's CPU arguments are already a `run.sh` parameter, so pass
  `--cpus=N --cpuset-cpus=<N cores disjoint from 2-9>` with no rig edit. First check
  `docker info --format '{{.NCPU}}'`.
- **Pin the seeder.** The seeder runs unpinned during fed runs (`run.sh:500,642`). Add a
  `SEED_CPUSET` passthrough so it shares the sink's set, not the router's.
- **Keep total busy vCPUs at or below 10** (the M4 Pro's performance cores): router 4 +
  sink + fixture + seeder.
  - Docker Desktop's vCPUs are host threads that macOS may place on efficiency cores,
    and a CPU set inside the VM does not prevent that.
  - Compensate with a **Rust control run in the same session** (E2) and three repeats
    per point.

### 9.3 Throughput runs vs profile runs

- **Throughput runs:** `FC_DEBUG_ENDPOINTS_ENABLED` **off**, 3 runs per CPU count.
- **Profile runs:** debug endpoints **on**, which **turns per-message pprof labels on**
  (`run.go:112`, `pool.go:1889-1896`). Use them for shape, not rates. In the steady
  window take, with basic auth `bench:bench`, under `/router/debug/pprof/...`:
  - `profile?seconds=10` (CPU);
  - `trace?seconds=3`, then offline `go tool trace -pprof=sync|sched|net|syscall trace.out`.
    This gives sync-blocking and scheduler-latency profiles **with no code change**;
  - `goroutine?debug=1` ×3, ~2 s apart: count goroutines parked in
    `sync.(*Mutex).lockSlow` / `runtime_SemacquireMutex` per call site;
  - an `allocs` delta over a known message count, giving bytes and objects per message.
- **Mutex and block profiles are empty today.** Nothing calls
  `runtime.SetMutexProfileFraction` or `SetBlockProfileRate` (confirmed: no call site in
  `internal/` or `cmd/`). Enabling them is a small, env-gated, standard-library change
  for the next phase. Until then, the trace's `-pprof=sync` output takes their place.

### 9.4 Experiments, one variable each, in this order

| # | Run | Answers | Decision rule |
|---|---|---|---|
| E1 | Go at `--cpus=1` with `-e GOMAXPROCS=2` (runtimetune leaves an explicit value alone, `runtimetune.go:146-151`) | How much of 1→2 is leaving the GOMAXPROCS=1 mode | The 2-CPU rate ÷ this rate is Go's actual scaling from 1 to 2 Ps |
| E2 | Rust (mimalloc build) 1/2/4 CPUs, same session, same pinning | The rig's own scaling ceiling | If Rust's 2→4 is within ±10% of Go's, the gap from 2 to 4 is mostly the rig |
| E3 | Go 2 and 4 CPUs, pinned, with all of §9.1 | CPU-bound or serially bound | CPU ≥90% of quota and CPU per message rising >15% from 2→4: contention or scheduler, so read the CPU-profile diff (`lockSlow`, `futex`, `findRunnable`, `http2`). CPU <80% with no throttling: a serial stage, so read the trace (E4/E5) |
| E4 | Go 4 CPUs with the target split over 4 host aliases (rig only) | F2 (one connection) together with the one-breaker lock | ≥15% gain points at F2. Confirm with the `readLoop` share in the trace |
| E5 | Trace a poll-loop goroutine (from `runConsumer`) over 3 s | F3 | Share of each ~22 ms cycle in network wait (fixture round trip), runnable (waiting for a P) and running. Network wait ≥50%: F3 is real |
| E6 | `GODEBUG=gctrace=1` on 1 and 4 CPU runs, plus the `allocs` delta | F5 | GC CPU %, cycles per second, bytes and objects per message; heap `inuse_space` at `health.go:391` growing across the run confirms the `rollingCounter` part |

**Recommendation: measure first.** E1–E3 need no code change and settle whether the
router is CPU-bound at 2–4 CPUs. Every later step depends on that answer. The changes
for ✱S1 and ✱S2 are correctness fixes and can go ahead whatever the measurements say.

---

## 10. Already-fixed items: verified still in place

| Fix | Where it is now |
|---|---|
| `internal/runtimetune`: GOMAXPROCS=1 at ≤1 CPU quota, GC 200, soft memory limit 90% | `runtimetune.go:146-151`, `:160-179`, `:183-188`; applied at `cmd/fc-server/main.go:59` when the router or scheduler is enabled |
| SQS `DeleteMessageBatch` + linger/helper/urgent redesign | `deletebatch.go`; urgent marking at `pool.go:298-300` |
| Batched non-blocking deferral (034b9fd) | `visibilitybatch.go`; `sqs.go:407-416` |
| `plainBodyClient` (35db6f5) | `httpclient.go:31-46`; `sqs.go:174` |
| Lazy loggers; pprof labels gated | `logattrs.go:40-86`; `pool.go:1889`; `run.go:112` |
| Mediator `Client.Timeout` → context deadline | `mediator.go:270-276`, `:459-464` |
| Allocation trims (3a66362) | `sqs.go:310` (zero-copy view), `deletebatch.go:57-69` (pooled items), `mediator.go:305-317` (payload), `pool.go:625-627` (retry copy) |
| No goroutine per *buffered* IMMEDIATE message (one per running delivery only) | `pool.go:47-56`, `:523-579` |
| Narrowed manager locks (`poolMu`/`consumerMu`, idiom plan item 4) | `manager.go:84-91`, `:878-895`, `:447-463` |
