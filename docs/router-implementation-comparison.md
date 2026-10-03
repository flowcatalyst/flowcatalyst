# What building the router in Go, Rust and Java taught us

**Status:** findings from a benchmarking and hardening exercise, 2026-10-02 to
2026-10-03. Written for the decision about which implementation to carry
forward. It records measurements and mechanisms, and says where the evidence
stops. It does not make the decision: the criteria that matter most (how readable
the code is at 3am, supply chain, ecosystem momentum, the domain-modelling
plans) are not benchmark results.

## 1. The shape of the problem

The router is an I/O-bound message mover:

- N queues, each with a consumer on its own broker connection (NATS JetStream,
  SQS or Postgres);
- M processing pools, each with a configured concurrency and a buffer of
  `concurrency × 40` waiting messages;
- one HTTP delivery per message to a target, over HTTP/2;
- per-message state while a message is in the router: tracking, receipt handle,
  retry bookkeeping, metrics.

What scales is not CPU per message. It is **cost per connection** (threads,
buffers, clients), **cost per buffered message** (memory), **where back-pressure
is applied**, and **how each HTTP and broker library behaves at its limits**.
Most of what went wrong, in all three languages, was in those four places.

## 2. How we measured

The rig is `bench/router` in the Java repository (see section 8).

- NATS JetStream in memory, 500,000 messages seeded before the router starts.
- **100 queues, 100 pools of 64 workers (6,400 concurrent deliveries)** unless
  stated. Earlier rounds used 1, 8 and 16 queues feeding one pool of 256.
- The target is a trivial HTTP/2 (h2c) sink that answers immediately, so the
  router's own work is what is measured.
- The router container has a CPU **quota** (`--cpus=N`, time-sliced in 100 ms
  periods), not a core assignment. The sink and the NATS server are pinned to
  separate cores. Docker Desktop VM, 14 CPUs, 16 GB, macOS.
- "Steady rate" is deliveries per second between 10% and 90% of messages
  delivered. The whole-run average hides stalls and tails; we report both when
  they differ.
- Each figure is one or two runs unless a range is given. Run-to-run spread was
  about 5% for Go and Java and as much as 15% for Rust's memory.

What this does **not** tell us: latency percentiles, behaviour against slow or
failing targets, SQS FIFO ordering under load, hours-long GC or memory behaviour,
real disks (storage was in memory; file storage made no measurable difference up
to 500k messages), or production traffic.

## 3. Results

### 3.1 Throughput by CPU quota (100 queues, 100 pools, steady rate)

| CPUs | Go | Rust | Java before NATS fix | Java after NATS fix |
|---|---|---|---|---|
| 0.5 | – | 14.8k/s | – | – |
| 1 | 18.0–18.6k/s | 29.3–30.6k/s | 12.5k/s | **21.5k/s** |
| 2 | 32.3k/s | not measured | 26.3k/s | **34.1k/s** |
| 4 | 51.5k/s | not measured | 42.2k/s | 39.4k/s |

- Go scaled 1.74× at 2 CPUs and 2.8× at 4. The sink and the NATS server share the
  machine and probably start to limit near 50k/s, so the last step is not a clean
  measure of Go.
- Java's fix mattered at 1 and 2 CPUs and not at 4, where something other than
  the router limits (about 39-42k/s).
- At 1 CPU, Rust is about 1.6× Go and about 1.4× Java after the fix. We did not
  measure Rust at 2 or 4 CPUs.

### 3.2 Memory and context switches (100 queues, 100 pools)

| | Go | Rust | Java |
|---|---|---|---|
| Peak memory, 1 CPU | 237–289 MB | 477–658 MB | 1.9 GB (limit 2 GB), 1,024 MB at a 1 GB limit |
| Context switches per message | 0.22–0.34 | 0.001–0.007 | 2.4 before the NATS fix, **1.2** after |
| Threads | few | few | 837 before, **38** after |
| Messages held in the router mid-run | 1,900–3,100 | 57,000–77,000 | not measured |

The JVM sizes its heap to the container, so Java's resident memory says little
about what it needs.

### 3.3 Before the fixes

At 100 queues and 100 pools the Go router delivered 92k-167k of 500k messages and
then stopped, at both 1 GB and 2 GB. Java finished but at about a third of Rust's
rate, with memory at its limit. Section 4 explains why.

## 4. What went wrong, and why

Each item: symptom, cause, fix, and which implementations it touched.

### 4.1 A one-second floor on deferral spacing (Go, Java)

- **Symptom:** at 16 queues, throughput ran at full speed until ~99% delivered,
  then fell to about one message per second for the remaining ~5,600.
- **Cause:** when a pool is full the router defers a message to a reserved return
  slot. For brokers that do not hold delayed messages in order (NATS) the slot
  spacing had a floor of one second, applied to every message. The floor exists to
  stop *ordered* group messages swapping order on redelivery. Applied to unordered
  messages it turned N deferrals into N seconds.
- **Fix:** apply the floor to ordered messages only (Go `a93b631`, Java
  `fcaec867`). Rust has no such floor.
- **Lesson:** a safety rule written for one case (ordering) was applied to all.

### 4.2 Over-pulling into full pools (Go, Java; design)

Consumers keep polling while any pool has room, and, when all are full, until a
deferral budget of 15,000 is spent. That was introduced to fix head-of-line
blocking (a slow pool starving the others on a shared queue) and it does, but at
the cost of bouncing messages through the broker. The proper fix is to park
overflow at the platform with a `not_before` and apply back-pressure at the
source. This is designed, not built: `docs/router-pool-backpressure-plan.md`.

### 4.3 A task, goroutine or thread per buffered message (all three)

- **Symptom (Go):** at 100 pools the router held 282,428 goroutines, 256,000 of
  them parked waiting for a concurrency slot. That is 100 pools × 2,560 buffered
  messages, about 1.2 GB of stack. It stalled and was killed at its memory limit.
- **Cause:** every accepted unordered message started its own goroutine, which
  parked on the semaphore. Memory scaled with the **sum of every pool's buffer**,
  not with work in flight. Java (virtual threads) and Rust (tokio tasks) had the
  same shape.
- **Fix:** waiting messages are plain data in a FIFO; one dispatcher starts a
  worker only when a slot is free, so workers track messages in flight (Go
  `10d4b5b`, Rust `68be197d`, Java `73b32e89`).
- **Lesson (owner's rule):** *a buffered message must not be a task; the task is
  the worker.* This was fatal in Go, improved Java by 30-38%, and made no
  measurable difference to Rust's memory.

### 4.4 HTTP/2 streams above a server's limit (library-specific)

A server advertises a maximum number of concurrent streams per connection (250
for Go servers, commonly 128 for nginx and ALBs). What a client does beyond that
differs completely by library:

| Client | Behaviour above the limit |
|---|---|
| Go `x/net/http2`, `StrictMaxConcurrentStreams=true` | **Deadlocks**: requests queued beyond the limit are never served (the router delivered nothing after ~10 s). Reproduced with only the library. |
| Go, strict off | Opens more connections; 51k/s at 6,400 in flight |
| JDK `HttpClient` | **Fails the request** ("too many concurrent streams"): 84-86% failed at 1,600 and 6,400 in flight |
| Vert.x `HttpClient` | Queues; no failures |
| Apache HttpClient 5 | Queues; no failures |

- **Fix (Go):** set strict off (`426851a`), with a regression test that hangs on the
  old setting.
- **Java:** the deployed router already uses Vert.x, so it did not have the
  failure; the JDK client is only used in dev mode. We first assumed otherwise,
  tested the wrong client, and corrected it. The check cost a few hours and is
  worth recording: **find out which client runs in the configuration you are
  measuring.**
- **Client comparison** (1-CPU container, same sink, ok/s, one client per 50 in
  flight unless noted):

| In flight | Vert.x pooled | Vert.x one client, bigger pool | Vert.x default | Apache pooled | Apache single | JDK pooled |
|---|---|---|---|---|---|---|
| 1,600 | 38.8k | 41.1k | 32.2k | 15.1k | 31.5k | 15.1k |
| 6,400 | 28.9k | 32.4k | 26.8k | 12.3k | 27.0k | 2.9k (timeouts) |

  Many separate clients did not beat one client with a larger pool: each Apache or
  JDK client brings its own threads and connections. We did not tune Vert.x
  further.

### 4.5 NATS client behaviour (each language, different)

- **Go (`nats.go`):** the pull request defaults to a 30 s expiry. With the
  router's design (take a permit, then call `Next`), a freeze of exactly that
  length could follow a full pool draining: 26-28 s of zero deliveries, repeatedly,
  reproduced with only the client library. A 3 s expiry (`c6b534d`) bounds it;
  it is a mitigation, not a fix. A fetch loop that requests exactly the free room
  avoided it in testing and is the structural fix, not yet built.
- **Java (`jnats`):** seven platform threads per connection by default, 700 for 100
  queues, about 90% of all context switches and the reason the container was
  throttled in 110 of 111 CPU-quota periods. The client accepts your own executors
  and thread factories; giving each connection a virtual-thread executor took
  threads from 837 to 38 and switches per message from 2.4 to 1.2 (`d347a70e`).
  The knob exists but is discoverable only by reading the options API.
- **Rust (`async-nats`):** the reconnect-delay callback is invoked before the
  **first** connect as well, with `attempts == 1`. A callback returning 2 s
  delayed every consumer's first connect, and consumers were also created one at
  a time, so 16 queues took 32 s to come online. 16-queue throughput looked like half
  of 8-queue throughput. Fixed (`31fa2a04`).

### 4.6 Rust's memory (understood, left as is)

Mid-run Rust held 57,000-77,000 messages in the pipeline (52,000-72,000 queued);
Go held 1,900-3,100 and its queues stayed empty. Rust's resident memory followed
that count at roughly 4-5 KB per held message (386 to 611 MB as the queue built;
Go flat at 209-236 MB). The heap profile attributes about 395 MB of live data
roughly as: NATS forwarder's pending-message map ~100 MB, tracking maps and
waiting queue ~80 MB, pool creation ~45 MB, in-flight deliveries ~54 MB, NATS
client connections ~29 MB.

- Go's consumers are the bottleneck on one CPU, so they never get ahead of
  delivery; Rust's pull faster than its HTTP deliveries complete, so the pools'
  queues fill.
- Memory did **not** follow speed: half a CPU gave 14.8k/s and 545 MB, one CPU
  29.3k/s and 497 MB. It did not follow buffer size either (pool concurrency 16,
  32 and 64 gave 372, 581 and 477 MB). It was not the flight recorder, allocator
  arenas or per-message tasks.
- Possible reductions, not pursued (the owner is content with the current memory):
  slim the pending map to what acknowledging needs, drop duplicate message copies
  in the tracking maps, or pull only what pools can start soon.

### 4.7 Rig and measurement traps

- The rig forced `max-ack-pending=1000` and `max-deliver=10` on NATS consumers
  while the routers' defaults had become unlimited. It also made the shared-pool
  shape look like a queue-count cliff, because N×1000 crossed the pool's capacity
  at 16 queues.
- An average hid the tail: Go and Java "1,500/s at 16 queues" was ~19k/s for the
  first 28 seconds and ~1/s after.
- Benchmarks run while something else compiled were contaminated (a Go run took
  53 s against 27 s clean).
- A sampler can stop before the last data point; use the router's own summary for
  finish times.
- Single runs of Rust's memory ranged from 477 to 658 MB at identical settings.
- Our first Java HTTP diagnosis measured the wrong client (4.4).

## 5. The languages and ecosystems, generically

These are judgements from this exercise plus general knowledge, flagged as such.

### Go

- **Concurrency model:** goroutines are cheap and blocking code is the norm, so
  one connection costs a few goroutines and a few KB. That is why Go ran 100 NATS
  connections in 0.22-0.34 context switches per message and flat memory while Java
  needed 700 threads.
- **Standard library:** one obvious HTTP and one obvious way to do most things.
  Fewer choices to get wrong. The two library traps we hit were both single
  defaults (a 30 s pull expiry, strict HTTP/2 streams), each a one-line fix once
  found.
- **Weaknesses:** no sum types, so state machines are enforced by convention;
  goroutine-per-item designs are easy to write and expensive at scale (4.3); the
  garbage collector keeps memory above live data (about 2×).
- **Build and supply chain:** fastest builds of the three (a full build and vet in
  seconds), small dependency graphs.
- **Measured here:** lowest memory (237-299 MB), good scaling (1.74× at 2 CPUs),
  slowest per CPU of the three at one core.

### Rust

- **Performance:** about 1.6× Go and 1.4× Java at one CPU in this workload, with
  almost no context switching (0.001-0.007 per message). No garbage collector, so
  no pauses (we did not measure latency percentiles).
- **Correctness tools:** drop guards that release counters and entries on every
  exit including panics, messages that nack themselves when dropped, sealed
  commit types. They remove classes of leaks and lost-message bugs that Go handles
  by discipline. They do not prevent interaction bugs (4.1, 4.2) or protocol-level
  ones.
- **Costs:** the concurrency core is harder to read (guards, cancellation at
  `await`, several types per idea); builds are slow (a release image is a 15+ minute
  compile); per-message memory was higher in our code (clones into maps and
  queues); library semantics still surprise (4.5).
- **Ecosystem:** strong and moving fast (runtimes, HTTP, async clients); more
  choice of crates than Go, and a heavier supply-chain process (the Rust repository runs
  `cargo-deny` and keeps a `supply-chain` directory).

### Java

- **Modern Java is much better than its reputation:** records, sealed types with
  exhaustive switches, virtual threads (blocking-style code that scales), strong
  diagnostics (Flight Recorder, thread dumps, per-thread counters), the largest
  test suite of the three.
- **Footprint:** the JVM sizes its heap to the container and fills it (1.4-1.9 GB
  at a 2 GB limit). The JVM's default garbage collector normally depends on CPU count and memory
  (serial on a small quota, G1 on larger ones; we did not confirm which ran), so
  behaviour can change with the quota.
- **Libraries are the risk:** several mature HTTP clients with very different
  behaviour (2.9k to 41k/s at 6,400 in flight, and one that fails requests above a
  limit); a NATS client that starts seven platform threads per connection. There is
  usually a way to configure it, but finding it takes reading the API.
- **Scaling:** good once the library overheads are fixed (12.5k to 42k/s from 1 to 4
  CPUs before the fix; 21.5k to 39k/s after).
- **Build and supply chain:** the largest dependency tree and the slowest build
  (a container build is several minutes), though Maven is predictable.

### On "one way to do things"

Go had the fewest surprises per component, and two of them were still severe.
Java had the most *choice* and so the most ways to be wrong, but each was fixable
by configuration. Rust's surprises were mostly in how a specific library's
semantics differ from what the name suggests. None of the three removed the need
to test each library above its limits.

## 6. For this shape of problem specifically

1. **Cost per connection is the scaling dimension.** With 100 queues, anything
   that costs threads or buffers per connection multiplies by 100. Check what each
   broker client allocates per connection before choosing a design.
2. **Cost per buffered message decides memory.** A pool buffer of `concurrency × 40`
   across 100 pools is 256,000 messages. Whatever you hold per message (a task, a
   thread, two copies of the payload) is paid 256,000 times.
3. **Apply back-pressure at the consumer.** Go's permit-gated consumer keeps the
   pipeline nearly empty and memory flat; consumers that pull ahead (Rust here)
   build queues that cost memory. Over-pulling and bouncing through the broker is
   the other failure (4.2).
4. **Test every HTTP and broker client above its limits.** Streams per
   connection, pull-request lifetimes, connect delays and threads per connection
   were each the cause of a failure that looked like a language problem.
5. **Measure with time series, at the production shape, at the production CPU
   quota.** A quota with many threads throttles in bursts. The 100×100 shape found
   problems that 1, 8 and 16 queues on one pool did not.
6. **Memory per CPU matters differently per language.** Go: small and flat. Rust:
   small to moderate, follows buffering. Java: large and elastic, needs an explicit
   fence.

## 7. What the data supports

- **Raw efficiency at one CPU:** Rust > Java (after fix) > Go. At 2-4 CPUs Go and
  Java are within about 30% of each other, Go ahead at 4 (Rust unmeasured).
- **Memory:** Go (about 250 MB) < Rust (about 500 MB) << Java (about 1.5-1.9 GB at a
  2 GB limit).
- **Operational simplicity of the concurrency model:** Go and Java (virtual threads)
  read straight-line; Rust's is more structured and more scattered.
- **Fixes needed to reach these numbers:** Go four (floor, dispatcher, HTTP/2
  setting, pull expiry), Rust three (connect and concurrency, buffer size,
  dispatcher), Java three (floor, dispatcher, NATS threads). Every router needed
  fixes at this shape.

What the data does not decide: which is easier to maintain, how each behaves under
slow targets and real latency, the supply-chain and hiring questions, or the value
of Rust's type system for the planned domain services.

## 8. Reproducing and where things are

- **Rig:** `bench/router` in the Java repository. `run.sh run <label> <image>
  "<docker cpu args>" [ENV=v ...]`. Useful knobs: `QUEUES`, `POOLS` (one pool per
  queue), `POOL_CONCURRENCY`, `TOTAL_MESSAGES`, `BROKER=nats|sqs|postgres`,
  `NATS_MAXPEND` and `NATS_MAXDELIVER` (default unlimited), `TIMEOUT_S`. It writes a
  one-second delivery time series and prints steady rate and tail separately.
- **Commits:**
  - Go: `a93b631` floor, `10d4b5b` dispatcher, `426851a` HTTP/2, `c6b534d` pull
    expiry, `2d191b2` back-pressure plan.
  - Rust: `31fa2a04` connect and concurrent creation, `dca54cd4` buffer 40×,
    `68be197d` dispatcher.
  - Java: `fcaec867` floor, `73b32e89` dispatcher, `d347a70e` NATS virtual threads.
- **Open work:** the platform park with `not_before` (back-pressure at the source),
  a fetch-based NATS consumer in Go, and committing the rig changes in the Java
  repository.

## 9. The SQS round (2026-10-03)

Sections 1–8 are the NATS-era findings. This section records the SQS work that
followed. The SQS numbers in §3 and earlier notes that quote ElasticMQ were limited by
the emulator (about 2.5k msg/s on its REST path) and should not be quoted as router
capacity.

### 9.1 The measuring instrument

- **`sqsfix`** (`bench/router/sqsfix`, Java repo): a stdlib-only in-memory SQS server that
  speaks only the AWS JSON protocol, with lazy queues, long polling and visibility
  timeouts. It used 20–40% of a core in every run here, so results are router-bound. It
  counts calls per operation (`/stats`), which is how batching efficiency was checked
  (about 10 messages per `ReceiveMessage`, about 6 acks per `DeleteMessageBatch`).
- **Rig options** (`SQS_EMULATOR=sqsfix`): `WARMUP_MESSAGES` drains a small batch first;
  `WARMUP_CHUNK` / `WARMUP_INTERVAL_S` feed that warm-up slowly; `MAIN_RATE` feeds the
  main backlog at a fixed messages-per-second; `ID_OFFSET` keeps message ids unique
  across seeder invocations. Main-phase rate is `500k / (seeding wall + series length)`.
- **Measurement traps found the hard way:** the rig's time series starts *after* the main
  seeding finishes, so a router that is already delivering during seeding looks faster
  than it is (Rust and Go had delivered 40k and 28k of the main messages before the
  clock started). A router container left running by an interrupted command consumed
  messages from the shared fixture and silently invalidated several runs (messages were
  "acked but never delivered"). Check `docker ps` before every run, and never reuse an
  existing label's numbers without the fixture's own counters.

### 9.2 What dominated the cost, per language

All three routers sent one `DeleteMessage` per message and one `ChangeMessageVisibility`
per deferral, from the thread or goroutine doing the work.

| Share of one CPU | Java (fill phase) | Go (steady) |
|---|---|---|
| SQS receive | 45% | 2.7% |
| SQS delete | 23.5% | 6.5% (after batching) |
| Delivery to the sink | 12.6% | 19.6% |
| GC pauses | 15–25% of the window | allocation and GC about 30% |

Java's HTTP delivery path is cheap (a drain of already-buffered messages reaches
29–34k msg/s); its SQS SDK calls are the expensive part. Go's SQS calls are cheap; its
cost is allocation, GC and one write syscall per HTTP/2 request. Rust's SQS cost was not
profiled.

### 9.3 Fixes and their effect

| Change | Where | Effect |
|---|---|---|
| `DeleteMessageBatch` (up to 10, no fill window, 4 drainers per queue, each ack still waits for its own entry) | Go `eed51ba`, Rust `eef9a7c7`, Java `25d0377f` | 100k msgs at 1 CPU: Java 4.2k to 7.0–7.6k, Go 6.3k to 9.0k, Rust 8.7k to about 20k msg/s |
| Pending-delete map pruned from the front of a time-ordered list instead of scanning every entry | same commits | removes a per-message scan that grew with the 15-minute TTL |
| Rust receipt map the same | Rust `8d574f4d` | no measurable change at 500k (so it was not the Rust slowdown, §9.6) |
| Deferral fallback: spread the 30 s fallback across the queue (30 s / queued) and trust a rate only after a worker's-worth of completions | Go `bf420ef`, Java `33d39cd2` | the old 1 s-per-message fallback booked a full 2,560-message buffer 43 minutes out |
| Hold-back: 30 s rate window, wait half the buffer's drain time, faster of a 5 s and 30 s rate, wait capped at 20 s | Java `d64c5005`, `86daa2d2` | see §9.5; the 20 s cap is wrong for genuinely slow pools and should be reverted |
| Batched, non-blocking deferral and nack (`ChangeMessageVisibilityBatch`, bounded queue) | Java `1cd8aedf`, Go `034b9fd`, Rust `7726e12d` | Java 500k backlog: deferral CPU no longer starves delivery |
| Poll pacing: after a batch that was at least half deferred wait 50 ms, doubling to a 200 ms cap, reset on a mostly-admitted batch | Java `4b44b9c3` | 500k backlog: 93–271 s and 110–210k deferrals became 65–77 s and about 20k |
| Mediator `Client.Timeout` moved to a context deadline, SQS MD5 validation off and no unused attributes, lazy per-message logger, cached target keys, periodic receipt prune | Go `b53b05c`, `ea53e7e`, `c7c21a2` | Go 500k: 9.6k to 11.1k msg/s and 576 to 433 MB (pprof goroutine labels are now off unless the debug endpoints are on) |
| A delete batcher must keep working after `close()` (the consumer contract); drainers exit when idle | Java `1cd8aedf` | fixed a regression this work introduced |

### 9.4 Java warm-up

A cold JVM at one CPU delivers 1–4k msg/s for its first 30 s while JIT compilation, GC
and SQS ingest share the CPU, then ramps to 22–28k msg/s by 45 s. Polling is not held
back, so a restart into a large backlog fills the pools before delivery is up to speed
and defers heavily. Restricting the JIT to C1 ramps about 10 s sooner but peaks at 14k.
The pools are created before the consumers, but they are cold (workers, HTTP/2
connections and JIT are all lazy). Java also fills at 12–29k msg/s against 0–5k delivered
in that window.

### 9.5 The comparison, with a warm-up

500k messages, 1 CPU, 100 queues × 100 pools, same fixture.

| | Cold, 500k at once | Warm (slow 100k first), 500k at once | Warm, 500k fed at 30k/s |
|---|---|---|---|
| Rust | 28.7 s (17.5k/s) | 29.7 s (16.8k/s, peak 1 s 20.6k) | 27.8 s: 18.2k/s during the feed, 16.5k/s after |
| Java | 44 s and 86 s | 26.7 s (18.7k/s, peak 34k) | 24.4 s: 16k/s during, 28.9k/s after |
| Go | 46 s (10.8k/s) | 44.7 s (11.1k/s, peak 12.9k) | 44.1 s: 11.3k/s during, 11.1k/s after |

- **None keeps up with a 30k/s feed on one CPU**; backlogs reach 194k–304k.
- **Ingest does not hurt Java more than the others in absolute terms.** Its during-feed
  rate is within 12% of Rust's. Rust and Go are CPU-bound at a fixed rate whether or not
  they ingest; Java's delivery alone is faster, so its ingest cost shows up as a drop.
- **Memory:** Rust 433–495 MB, Go 470–540 MB, Java about 1.95 GB (the heap fills the
  container).
- **Java deferral tail:** on the gentle feed Java is bimodal: 24.7 s with no deferrals,
  or about 36 s when about 5k messages are deferred, because they return after the
  hold-back floor (5 s) plus the wait (up to 20 s) with the pools idle. The peak is a
  drain of buffered messages and flatters the average.

### 9.6 Go and Rust follow-ups

- **Go and the CPU quota.** Run under `--cpus=1`, Go used extra scheduler threads
  (0.5 context switches per message). `GOMAXPROCS=1` took it from 11.3k to 13.7k msg/s
  (0.035 switches per message); `GOGC=400` gave 12.95k at 1.09 GB; `GOMAXPROCS=1` with
  `GOGC=200` gave 15.3k at 714 MB (+35%). Why the runtime did not size itself to the
  quota on go1.27 inside this container runtime is **not established**; production
  containers should set `GOMAXPROCS` explicitly (or use a quota-aware setting) and the
  deploy environment should be checked.
- **Rust's rate declines over a run** (19.3k to 16.6k msg/s across 500k messages) and
  does not recover while the backlog shrinks, so per-message cost grows with messages
  processed, not with the current backlog. Earlier 100k runs reached 20k and 500k runs
  17.8k, the same shape. Not yet explained; an accumulating structure that is only
  pruned by age is the leading suspect.

### 9.7 Decisions and open items

- **Queue mode (noted, not built):** see `router-pool-backpressure-plan.md` §4.8.
- **Revert the 20 s hold-back cap** (the faster-of-two-windows rate stays): a pool at
  concurrency 1 taking 2 s per message with 10,000 arriving at once needs the wait to
  reflect its real pace.
- **Pool-full and blocked metrics** (per pool: full, time at capacity, deferrals total and
  outstanding; per queue: paused for capacity and its mode) are cheap and not built.
- **Java cold start:** still erratic (44–86 s) with a large backlog; pre-opening each
  pool's HTTP/2 connections and holding polling until deliveries are flowing are
  candidates, neither tried.
- **Commits** (none pushed). Go: `eed51ba`, `bf420ef`, `b53b05c`, `ea53e7e`, `c7c21a2`,
  `034b9fd`. Rust: `eef9a7c7`, `8d574f4d`, `7726e12d`. Java: `25d0377f`, `33d39cd2`,
  `d64c5005`, `1cd8aedf`, `4b44b9c3`, `86daa2d2`, rig `080d83fe`, `4fc43695`.

### 9.8 Pressure test: slow pools (all three defer; the hold-back policies differ)

100 queues x 100 pools, **2 workers per pool**, a **20 ms sink delay** (about 100 msg/s per
pool, about 10k/s overall, ideal drain about 10 s), a pool buffer of 100, 100k messages
arriving at once, cold, 1 CPU.

| | Time for 100k | Deferrals (ChangeMessageVisibility entries) | Receives |
|---|---|---|---|
| Rust (flat 5 s bounce, no reservation schedule) | 22 s | 111.7k | 211.7k |
| Java (reservation schedule, hold-back changes of §9.3) | 94 s | 61.9k | 161.9k |
| Go (reservation schedule, earlier rules) | 188 s | 61.7k | 161.7k |

- **They do defer, and by the same gate.** Pool queues hit the 100 cap in Go and Rust
  within the first seconds; the earlier 500k runs never deferred in Go and Rust because
  their 64-worker pools drained as fast as they were fed (Go's pool queues stayed under
  about 1k), not because they were blocking.
- **The hold-back policy dominates the wall time here.** The ideal is about 10 s and a
  pool drains its 100-message buffer in about a second. The reservation schedule books
  the deferred messages tens of seconds out (the rate estimate is taken while the pool
  is still cold or full), so the pools sit idle while Java and Go wait for them (Go:
  the last 40k messages took about 60 s). Rust's flat 5 s bounce finishes in 22 s at the
  price of about 1.1 extra receive and deferral calls per message.
- **That does not make the flat bounce right.** For a pool at concurrency 1 taking 2 s
  per message with 10,000 arriving at once (a real case), a flat 5 s bounce means every
  message returns every 5 s for hours: roughly 2k broker calls a second of pure churn.
  The schedule exists for that case. The data says the schedule's early estimate is the
  weak part and that a block mode for dedicated queues (see the back-pressure plan §4.8)
  is the clean answer for slow pools.
- **Java showed the cold-start stall again** (pool queues full at 9.7k with no active
  workers for the first 18 s) even with only 200 workers. Not explained yet.
- **Measurement note:** Java's `fc_queue_messages_total` counters are refreshed by a
  periodic housekeeping task (about 60 s), so `acked` and `deferred` read 0 for the first
  minute in sampled runs; use the fixture's counters or the live gauges.

### 9.9 Re-measurement after the Rust leak fix (`1d70ac90`)

- **The leak fix did not change the decline.** Warm 500k flood: 18.0k msg/s (27.7 s) against
  16.8k (29.7 s) before, within run-to-run noise; RSS 407 MB against 458-495 MB. The delivery
  rate still falls from about 19.8k early to 16.5k late, and the 30k/s feed shows the same
  (19.7k to 15.6k).
- **It is not process age.** A second 500k batch in the same process starts at 19.1k and falls
  to 15.7k, the same shape as the first, so accumulated process state is not the cause.
- **It is not shrinking ack batches in Rust.** Rust's `DeleteMessageBatch` averages 5.6-6.6
  acks per call throughout; `ReceiveMessage` returns 10.
- **Tokio runtime during a flood:** the one worker is 100% busy, about 4k runnable tasks wait
  on the global queue, about 14-16k tasks are alive, and the pools hold only 18-28k queued
  messages (of 256k capacity), so Rust's intake tracks its delivery as Go's does. The task
  count and queue depth fall away in the final seconds. The decline is therefore likely a
  drain-tail effect on each batch; the cause is still **not established** and the cost is
  low priority at 17-18k msg/s.
- **Go's ack batches are small:** `DeleteMessageBatch` averages 2.2-2.4 acks per call in Go
  against about 6 in Rust and Java (messages per `ReceiveMessage` is 10 in all three), so Go
  makes about 2.6 times the delete calls per message. With 4 drainers per queue and no fill
  window each ack is taken as soon as it arrives. Fewer drainers or a 1-2 ms linger would
  raise it; the saving is bounded by the delete call's share of Go's CPU (about 6.5%).
