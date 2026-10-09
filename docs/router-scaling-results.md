# Go router: CPU-scaling measurements (SQS)

_Measurement phase for [`router-scaling-audit.md`](./router-scaling-audit.md), 2026-10-09. No
router code was changed, and the rig was not changed either: all placement went through
`run.sh`'s existing CPU-argument parameter. Not committed._

**Builds.**
- Go: `c9f640c` (main), `./cmd/fc-server` cross-compiled linux/arm64 onto alpine 3.20, as
  `bench-real-go:scale-c9f640c`. The tag `bench-real-go:latest` was also moved to this build;
  the earlier image is now untagged, id `6743c6b3bc54`.
- Rust: `idiomatic-rust` at `fcf2427a`, from the production Dockerfile's `builder` stage
  (mimalloc global allocator, default `FC_TASKDUMP=1`), with `fc-server` copied onto
  debian-bookworm-slim, as `bench-real-rust:scale-fcf2427a`.

## Headline

| Steady msg/s (median of 6 runs) | 1 CPU | 2 CPUs | 4 CPUs | 1→2 | 2→4 | 1→4 |
|---|---|---|---|---|---|---|
| Rust (control, E2) | 32.3k | 54.6k | 85.3k | 1.69x | 1.56x | 2.64x |
| Go (E3 baseline) | 18.4k | 32.4k | 49.0k | 1.76x | 1.51x | 2.66x |
| Go ÷ Rust | 0.57 | 0.59 | 0.57 | | | |
| Go CPU µs per message | 54.2 | 60.6 | 67.5 | +12% | +11% | +25% |
| Rust CPU µs per message | 31.0 | 36.6 | 45.9 | +18% | +25% | +48% |
| Go CPU, % of quota | 100 | 98 | **83** | | | |
| Rust CPU, % of quota | 100 | 100 | 98 | | | |

1. **Go scales from 1 to 4 CPUs as well as Rust does (2.66x against 2.64x).**
   - The Go-specific gap is a constant per-message CPU cost: 54 µs against 31 µs at 1 CPU
     (1.75x).
   - Go's per-message cost grows *less* with CPU count than Rust's does.
2. **One Go-specific scaling loss remains, at 4 CPUs.**
   - Go uses only 82–84% of its quota there, with zero throttling. Rust uses 98%.
   - The execution trace names the cause: the `InFlightTracker` `RWMutex` (F4) forms a lock
     convoy. A goroutine snapshot at 4 CPUs found **2,030 goroutines parked on it**, against 0
     at 1 CPU.
   - It is 36% of all sync-blocking time in the trace, and the top cause of scheduler
     latency.
3. **The audit's top code suspect (F2, one HTTP/2 connection) does not hold in this bench.**
   - The per-host pool grows to `MaxSlotsPerHost` = 8 within 80 ms of start-up, because the
     warm-up burst trips the 100-stream watermark.
   - The router then holds 17 TCP connections to the sink for the whole run.
4. **Leaving the GOMAXPROCS=1 mode costs 3–9% (E1), not the 21% measured in §9.6.** Go's real
   scaling from 1 to 2 Ps is 1.80–1.92x.

**Recommendation:** make the first change F4, a sharded `InFlightTracker` (details at the end).

---

## Setup

- **Engine.** The engine is **OrbStack**, not Docker Desktop: `docker context` = `orbstack`,
  with the kernel command line `init=/opt/orb/vinit … isolcpus=13`.
  - The VM has 14 vCPUs, and CPU 13 is isolated: the scheduler never places a task there
    unless the task is pinned to it. So 0–12 are usable.
  - The host is an M4 Pro (10 performance + 4 efficiency cores). vCPUs are host threads that
    macOS may move between core types.
- **Placement.** The sink and the sqsfix fixture stay on 2–9 (the rig's default). The router
  is pinned to cores disjoint from that set:

  | Router CPUs | `--cpuset-cpus` | Feed rate (`MAIN_RATE`) |
  |---|---|---|
  | 1 | `10` | 40,000/s |
  | 2 | `10-11` | 70,000/s |
  | 4 | `1,10-12` | 120,000/s |

  - The router also has `--cpus=N --memory=2g`.
  - The seeder and the probers stay unpinned (no rig change).
  - A measurement helper sits on CPU 0.
  - **Contamination check:** VM-wide busy time on the router's cores minus the router's own
    cgroup usage was ≤0.04 cores in every run. So the unpinned seeder did not land on the
    router, and the `SEED_CPUSET` rig change of §9.2 was not needed.
- **Protocol.**
  - Shape: `BROKER=sqs SQS_EMULATOR=sqsfix QUEUES=100 POOLS=100` (64 workers per pool).
  - Warm-up: `WARMUP_MESSAGES=50000 WARMUP_PAUSE_S=5`.
  - Main phase: then 500k messages fed at the rates above, which are above capacity for every
    router, as in the like-for-like table §10.8. `TIMEOUT_S=300`.
- **Per-run record (§9.1).** A helper container (`--pid=host --cgroupns=host`, VM cgroup root
  mounted read-only, pinned to CPU 0) sampled every 0.5 s:
  - sink `/stats` and sqsfix `/stats`;
  - `cpu.stat` of every container cgroup;
  - the router's per-thread context switches (`/proc/<pid>/task/*/status`);
  - its connections to the sink: `/proc/<pid>/net/tcp{,6}`, remote port `2328`, state `01`;
  - VM per-CPU `/proc/stat`.

  Nothing ran inside the router container.
- **Windows.**
  - **Steady** is the main phase from 10% to 90% delivered (sink count from 100k to 500k). All
    CPU, throttling and context-switch figures are deltas over this window.
  - **Tail** is the time from 99% to 100% of the main phase.
  - **Main complete** is from the first fed message to the last delivery.
  - **Rig drain** is the rig's own summary `drain_time_s`, which starts after main seeding
    finishes.
- **Repeats.** Two passes of 3 runs per point.
  - Pass 1 had a sampler bug: a previous run's sampler could linger, so 2–3 samplers ran at
    once on CPU 0 and polled `/stats` more often. Router-side figures were unaffected, and the
    window maths was fixed.
  - **Pass 2 is clean** and is what the tables below show. The headline medians pool both
    passes; they agree within 2% everywhere.
  - One pass-2 Go 2-CPU run was an outlier (20.7k/s at 89.9 µs/msg, with the router at 93% of
    its quota and nothing else on its cores). It is most likely host core placement (an
    efficiency core). It was replaced by a fourth run and is excluded below.
- **Hygiene.** One rig run at a time, and `docker ps | grep bench` plus
  `ps aux | grep 'run.sh run'` were clean before and after every run (the driver aborts
  otherwise). No builds ran during runs. The helper container was removed at the end.
- **Raw data.** Scratchpad: `runs/*.log`, `m/*.jsonl`, `results*.jsonl`, `m/prof/`. The rig's
  own outputs are in `../flowcatalyst-javalin/bench/router/results/`, labels `e2-rust-*`,
  `go-*`, `e1a-go-*`, `e1b-go-*` and `p2-*`.

---

## E2: Rust control (mimalloc), 1/2/4 CPUs, same session and pinning

Clean pass, 3 runs per point, mean [min–max]:

| CPUs | Steady 10–90% | While fed | Main complete | Tail 99→100% | Rig drain | CPU % of quota | CPU µs/msg | Throttled periods / ms per s | Ctx switches/msg | Sink conns | Threads |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | 32,464 [32,204–32,601] | 32,398 | 15.3 s | 0.0 s | 7.7 s | 100 | 30.8 | 11% / 1 | 0.000 | 101 | 2 |
| 2 | 55,162 [54,812–55,440] | 55,069 | 9.2 s | 0.0 s | 7.1 s | 100 | 36.2 | 27% / 3 | 0.002 | 101 | 3 |
| 4 | 85,832 [85,447–86,377] | 85,451 | 5.8 s | 0.0 s | 6.5 s | 98 | 45.5 | 14% / 2 | 0.007 | 101 | 5 |

The rest of the record:

| CPUs | Sink | Fixture | Seeder | Poll cycle per queue | Msgs per receive | Acks per `DeleteMessageBatch` | Held at end of feed | RSS |
|---|---|---|---|---|---|---|---|---|
| 1 | 0.64 cores | 0.70 cores | 0.44 cores | 29.6 ms | 9.99 | 9.1 | 27k | 421 MB |
| 2 | 1.15 | 1.22 | 0.70 | 17.8 ms | 9.99 | 9.1 | 13k | 418 MB |
| 4 | 2.02 | 1.97 | 1.08 | 11.6 ms | 9.99 | 9.1 | 8k | 478 MB |

- Rust holds about 101 connections to the sink (about one per pool).
- Rust drives the sink and fixture to about 2 cores each at 85k/s, against the 8 cores they
  have. So **the rig is not the ceiling for Go at 49k/s.**

**Decision rule (E2):** "if Rust's 2→4 is within ±10% of Go's, the gap from 2 to 4 is mostly
the rig."
- **Met:** Rust 1.56x against Go 1.51x (medians), about 3% apart.
- Read literally, neither router gets 2x from 2 to 4 CPUs in this environment.
- **Caveat:** the mechanisms differ.
  - Rust stays at 98–100% of its quota while its per-message cost rises 25% from 2 to 4.
    Plausible causes are the VM and host (P/E cores) or its own contention.
  - Go's per-message cost rises only 11%, but it leaves 17% of its quota idle (E3). That idle
    part is Go-specific, and E3 attributes it.

## Go baseline (= E3): 1/2/4 CPUs, pinned, with the full record

Clean pass, 3 runs per point:

| CPUs | GOMAXPROCS | Steady 10–90% | While fed | Main complete | Tail 99→100% | Rig drain | CPU % of quota | CPU µs/msg | Throttled periods / ms per s | Ctx switches/msg | Sink conns | `grew per-host` lines | Threads |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | 1 | 18,379 [18,194–18,512] | 17,958 | 26.8 s | 0.2 s | 19.9 s | 100 | 54.4 | 4% / 0 | 0.011 | 17 (one sample showed 360) | 7 | 6 |
| 2 | 2 | 32,157 [31,625–32,625] | 30,911 | 15.7 s | 0.0 s | 13.6 s | 98 | 60.8 | 6% / 0 | 0.029 | 17 (up to 23) | 7 | 8 |
| 4 | 4 | 48,949 [48,067–50,433] | 44,494 | 10.4 s | 0.0 s | 11.2 s | **82** | 67.3 | **0% / 0** | 0.061 | 17 (up to 19) | 7 | 10 |

The rest of the record:

| CPUs | Sink | Fixture | Seeder | Poll cycle per queue | Msgs per receive | Acks per `DeleteMessageBatch` | Held at end of feed | RSS |
|---|---|---|---|---|---|---|---|---|
| 1 | 0.56 cores | 0.41 cores | 0.22 cores | 54.4 ms | 10.0 | 8.7 | 1.8k | 492 MB |
| 2 | 0.94 | 0.71 | 0.36 | 31 ms | 10.0 | 9.3 | 2.1k | 457 MB |
| 4 | 1.42 | 1.07 | 0.49 | 20.4 ms | 10.0 | 9.6 | 1.5k | 468 MB |

Pass 1 agrees: 18.4k / 32.6k / 49.5k at 100 / 98 / 84% of quota and 54.2 / 60.2 / 68.0 µs/msg.

**Decision rule (E3):**
- "CPU ≥90% and CPU per message rising >15% from 2→4: contention or scheduler." Not met.
- "CPU <80% with no throttling: a serial stage, so read the trace." Nearly met.
- 2 CPUs: 98% of quota, so CPU-bound.
- 4 CPUs: **82–84% with zero throttled periods**, and per-message cost +11% from 2→4. That
  is not the contention signature; it is idle quota with nothing throttled, so it points to a
  serial stage. The trace was therefore taken (below).

### Trace and profiles (debug endpoints on, so pprof labels on; used for shape, not rates)

The profiled runs delivered at the same rates as the throughput runs: 18.5k at 1 CPU, and
48.4k and 49.2k at 4 CPUs.

**4 CPUs, 2 s execution trace** (`go tool trace -pprof=sync|sched|net|syscall`):
- **Sync blocking:**
  - `sync.(*RWMutex).Lock` is **35.9%** of all blocked time. That is about 2,100 goroutine-
    seconds in a 2 s trace, so on average about 1,000 goroutines are waiting at any moment.
  - Callers: `InFlightTracker.Remove` 58%, `ensureTracked` 36%, `Register` 6%,
    `HealthService.RecordPoolResult` about 0%.
  - Almost all the rest is `selectgo`, which is idle waiting.
- **Scheduler latency:**
  - `sync.(*RWMutex).Unlock`, waking waiters through `semrelease1`, is the top cause at
    **39.5%**. Its callers are again `Remove` 45%, `ensureTracked` 38% and `Register` 17%.
  - The HTTP/2 client's `readLoop` waking request goroutines is next, at about 20% each for
    `processHeaders` and `endStream`.
- **Goroutine snapshots:**
  - 4 CPUs: 1,566 goroutines at `inflight.go:224` (`Remove`), 357 at `:142`
    (`ensureTracked`) and 91 at `:81` (`Register`), out of 3,583.
  - A second 4-CPU snapshot found about 850 parked there.
  - 1 CPU: **none**.
- **Poll loops (F3):** `runConsumer` is 3% of sync blocking. The `net` profile is dominated
  by the SQS HTTP/1.1 connections' read loops, which are idle connections. Nothing points at
  the poll loop as the serial stage (E5 not run separately).
- **Mechanism (inferred):** three write locks per message on one process-wide `RWMutex`, at
  about 147k acquisitions per second, push `sync.Mutex` into starvation mode. Each hand-off
  then needs a goroutine wake-up. The lock's throughput becomes the hand-off rate, not the
  critical-section time. That explains why the audit's hold-time arithmetic (4% busy)
  under-predicted it.

**CPU profile, 1 CPU (10 s, 54 µs/msg):**

| Share of CPU | What | Notes |
|---|---|---|
| 35% | Per-message delivery goroutine (`runImmediate`) | Includes **stack growth, 8.5% of all CPU** (`newstack`/`copystack`; 87% of it under `schemeRoundTripper.RoundTrip`, called from `context.(*valueCtx).Value`). Each per-message goroutine starts small and grows its stack on every delivery |
| 17% | HTTP/2 per-request goroutine (`clientStream.doRequest`) | Mostly one **`write` syscall per request**: 9.3% of all CPU in delivery writes, 12.6% for all writes |
| 21% | AWS SDK (SQS receive and delete) | |
| 7% | Poll loops (`runConsumer`) | |
| about 6% | Delete batcher | |
| 5% | GC background mark | Plus assists inside `mallocgc` |
| 0.1% | `HealthService` / `rollingCounter` | |
| 1.5% | `InFlightTracker` | |
| 0.8% | Circuit breaker | |

Allocation: 20.9 KB and 213 objects per message, unchanged from §10.2.

**CPU profile, 4 CPUs (5 s, 3.3 cores busy):**
- The same shape: stack growth 7.4%, `write` 12.7%, GC about 11%.
- Contention appears: `InFlightTracker` 3.9% (1.5% at 1 CPU), `Mutex.lockSlow`/`RWMutex`
  2.9% (0.2%), HTTP/2 `readLoop` 5.0% (2.8%).
- So of the +13 µs/msg from 1 to 4 CPUs, about 5–6 percentage points of CPU are lock and
  wake-up costs, mostly the tracker.

## E1: Go at `--cpus=1` with `-e GOMAXPROCS=2`

| Variant | Steady | While fed | Main complete | CPU % of quota | CPU µs/msg | Throttled periods / ms per s | Ctx switches/msg | Sink conns |
|---|---|---|---|---|---|---|---|---|
| Baseline, GOMAXPROCS=1, cpuset `10` | 18,379 | 17,958 | 26.8 s | 100 | 54.4 | 4% / 0 | 0.011 | 17 |
| E1a: GOMAXPROCS=2, cpuset `10` (1 core) | 17,963 [17,844–18,060] | 17,698 | 27.8 s | 100 | 55.6 | 6% / 0 | 0.032 | 17 (one sample 35) |
| E1b: GOMAXPROCS=2, cpuset `10-11` (quota only) | 16,631 [16,394–16,838] | 16,414 | 30.0 s | 100 | 60.1 | **99% / 935** | 0.033 | 17 |

- Pass 1 agrees: E1a 17.8k, E1b 16.9k.
- In E1b the two Ps run in parallel and burn the quota in bursts, so the cgroup throttles in
  almost every 100 ms period. That is the unpinned production shape.

**Decision rule (E1):** "the 2-CPU rate ÷ this rate is Go's actual scaling from 1 to 2 Ps."
- 32.4k ÷ 17.96k = **1.80x** (one core), or ÷ 16.86k = **1.92x** (quota only).
- So **most of Go's 1→2 shortfall is not the runtime mode**. Leaving GOMAXPROCS=1 costs 2.7%
  (pinned) to 9% (quota only), against §9.6's +21% (an older build).
- With 2 Ps, Go gets 0.90–0.96 of linear from 1 to 2 CPUs. This finding of the audit (F1) is
  real but small; `runtimetune`'s GOMAXPROCS=1 is still worth keeping.

## Not run

- **E4 (host aliases):** skipped, because it needs a rig code change (the seeder takes one
  `MEDIATION_TARGET`), and its premise (F2, one connection) is refuted here: 8 slots and 17
  connections from start-up.
- **E5/E6:** not run as separate experiments. The trace and profiles above cover their
  questions enough to pick the first change.

## What changes in the audit's ranking

| Finding | Audit likelihood | Measured |
|---|---|---|
| F1 (rig) | High | **Small.** Pinning gave Go +3/+9/+7% against §10.8 (17.8 / 29.7 / 45.6k → 18.4 / 32.4 / 49.0k). The GOMAXPROCS=1 mode is worth 3–9%. Seeder contamination ≈0. Both routers miss 2x from 2→4 by a similar amount |
| F2 (one HTTP/2 connection) | High | **Refuted for this bench:** 8 slots / 17 connections. It may still apply in production when load ramps up slowly, since the pool only grows above 100 streams per slot. The `write`-per-request cost is real (9–13% of CPU) but it is a constant per-message cost, not a serial stage |
| F3 (one poll loop per queue) | Medium–High | Not indicated: 3% of sync blocking; poll cycles scale with rate |
| F4 (process-wide locks, tracker `RWMutex`) | Medium | **Confirmed as the Go-specific 4-CPU loss:** a lock convoy, 2,000 goroutines parked, 36% of sync blocking, top cause of scheduler latency |
| F5 (alloc/GC, `rollingCounter`) | Medium (constant factor) | GC is about 5–11% of CPU. `HealthService`/`rollingCounter` is 0.1% of CPU. The rate does not decline within a run (thirds of the main phase: 17.5k / 18.9k / 19.0k). The `rollingCounter` memory bound still matters for long-running instances |
| F6 (goroutine per message) | Medium–Low | **Larger than expected as a constant cost:** stack growth of the per-message goroutine is 7.4–8.5% of CPU at every CPU count. Context switches per message stay low (0.011 / 0.03 / 0.06) |

Further observations:
- **Transient connection spikes.** In several 1-CPU Go runs, a single 0.5 s sample showed 35
  to 534 TCP connections to the sink (the steady count is 17). That is consistent with the
  HTTP/2 transport dialling extra connections when a slot hits the sink's 250-stream limit,
  then closing them. It did not affect throughput here, but it is a potential connection storm
  against a real target. UNVERIFIED cause.
- **Idle connections.** Connections drop from 17 to 9 during the 5 s idle pause.

## Recommendation: first change = F4, shard the `InFlightTracker`

**Why this first:**
- It is the only measured *scaling* defect that is Go-specific. Everything else (rig, P/E
  cores, sink) affects Rust equally.
- It is directly attributed by the trace, with three independent signals:
  - 36% of sync-blocking time;
  - the top cause of scheduler latency;
  - 2,030 goroutines parked at `inflight.go:224/142/81`.
- The change is small, uses the standard library only, and keeps the same API. The audit
  §2.6 already specifies it: shard by a hash of the message id (32 or 64 shards). On the
  "already tracked" path in `ensureTracked`, take a read lock or a plain `Mutex`, not a write
  lock. Add the invariant comment and the external-requeue test.
- **Expected effect:**
  - At 4 CPUs: the 17% idle quota is recovered, from 82–84% to ≥95%. At the current 67.5
    µs/msg that is about 56–59k msg/s, +15–20%. Lock and wake-up overhead (about 5–6% of CPU
    at 4 CPUs) also falls.
  - At 1 CPU: no change, because GOMAXPROCS=1 means no contention.

**Measurement that confirms it.** Same protocol: pinned, warm 50k then 500k fed, 3 runs per
point.
- 4 CPUs: CPU ≥95% of quota with throttling then appearing, and steady ≥54k (≥ +10% over
  49.0k). A goroutine snapshot shows ≈0 goroutines at `inflight.go`, and `RWMutex.Lock` is
  no longer the top `-pprof=sync` entry.
- 1 and 2 CPUs: unchanged within ±2% (18.4k / 32.4k).
- If the 4-CPU quota use does not rise, the convoy just moves to the next process-wide lock.
  Look at `HealthService.mu` and the breaker mutex in the next trace.

**Next, for the 1-CPU fleet (constant cost, not scaling), in order of measured size:**
1. Per-message goroutine stack growth, 7.4–8.5% of CPU: a fixed worker set per pool (F6), or
   keep the per-message goroutine but avoid the deep stack.
2. The SQS SDK, about 21% of CPU and over half of allocations: a thin client is the owner's
   call (§9.15).
3. One `write` syscall per HTTP/2 request, 9–13% of CPU: a library limitation of the
   standard HTTP/2 client.

Each is confirmed the same way: CPU µs/msg at 1 CPU. The current figure is 54.2 µs; the
spread across 6 runs is 53.9–55.0.

---

## Result: F4 (sharded `InFlightTracker`), measured

_Build `9d3b703` (`bench-real-go:scale-9d3b703`) against the `c9f640c` baseline image re-run
as a control in the same session, interleaved run by run. Same protocol, placement and
per-run record as above; 5 runs per point; medians [min–max]._

| CPUs | Steady, control | Steady, F4 | Δ | CPU % of quota (ctl → F4) | CPU µs/msg (ctl → F4) | Throttled periods (ctl → F4) | Tail 99→100% / rig drain (F4) |
|---|---|---|---|---|---|---|---|
| 1 | 18,456 [18,333–18,529] | 18,382 [17,635–18,612] | −0.4% | 100 → 100 | 54.1 → 54.4 | 6.5% → 9.3% | ≤0.5 s / 19.8 s |
| 2 | 32,603 [32,063–32,801] | 32,914 [31,243–33,294] | +1.0% | 98 → 98 | 60.0 → 59.6 | 5.7% → 5.7% | ≤0.5 s / 13.2 s |
| 4 | 49,913 [48,288–51,069] | **54,258** [53,091–57,059] | **+8.7%** | **84 → 94** | 66.1 → 69.4 | **0% → 2.8%** | 0 s / 10.2 s |

(The tail column's resolution is the 0.5 s sampler tick.)

Against the confirm criteria:
- **4 CPUs, steady ≥54k:** met, just (median 54.3k; +10.7% over the earlier 49.0k median).
- **4 CPUs, CPU ≥95% of quota with throttling appearing:** throttling appears (1.4–3.9% of
  periods, from zero), but CPU use is **94%** (93.8–94.9), so ≥95% is **narrowly not met**.
- **≈0 goroutines at `inflight.go`:** met. The 4-CPU goroutine snapshot found **1** (against
  2,030 and ~850 before), and none waiting on the tracker's lock.
- **`RWMutex.Lock` no longer the top `-pprof=sync` entry:** met. `sync.(*Mutex).Lock` is now
  3.2% of all blocked time (36.6% before); the tracker is absent from it.
- **1 and 2 CPUs within ±2%:** met (−0.4% and +1.0%).

**Next suspect, from the F4 trace (4 CPUs, 2 s):**
- **Sync blocking:** 76% of the remaining `Mutex.Lock` time is the HTTP/2 client's
  connection-pool lock, `net/http/internal/http2.(*clientConnPool).getClientConn`, taken on
  every request; 212 goroutines were parked at `x/net/http2.(*Transport).roundTripOpt`
  (`transport_wrap.go:180`) in the snapshot. Then `clientStream.cleanupWriteRequest` 14% and
  `transportResponseBody.Read` 7%. It is about 2.4% of all blocked time, so it is a small
  convoy, not a 17% one.
- **Scheduler latency** is now led by the delete batcher waking its acking workers
  (`sqs.(*deleteItem).finish` → `chansend1`, 30%) and the HTTP/2 `readLoop` waking request
  goroutines (`processHeaders` 26%, `endStream` 22%). With the router at 94% of its quota
  these are mostly goroutines waiting for a P, not a serial stage.
