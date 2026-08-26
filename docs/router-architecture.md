# Message Router Architecture

_How a message travels through `internal/router`, what each delivery outcome does to the
broker, and where ordering is actually enforced. Scope: `internal/router` and
`internal/queue`. The companion plan for Go-idiom cleanup is
[`router-go-idiom-plan.md`](./router-go-idiom-plan.md)._

Published reference page: **Router Anatomy** —
<https://claude.ai/code/artifact/d58db84a-5d40-4377-bb72-e320a96b78b5> (same content, with
the lifecycle and before/after diagrams drawn).

## TL;DR

1. The router is a **poller, not a server**. A `Manager` owns one consumer per queue; pools
   are **passive** and never touch a broker. Because a pool serves many queues, every
   ack/nack resolves back to the message's source consumer via `QueueIdentifier`.
2. Delivery outcomes collapse to **three broker actions**: acknowledge (message is over),
   retry in place (stays in this process — the broker's expiry can never reach it), or
   release (message *and its whole group* go back to the broker).
3. A **release takes the whole group**, never just the failed head. Releasing the head alone
   would leave successors buffered here, so the head returns *behind* them on redelivery —
   reordering the one thing an ordered group exists to protect.
4. For the platform's own dispatch jobs the **router never sees delivery failure**:
   `/api/dispatch/process` records the outcome itself and returns `200 {ack:true}` either
   way. Their mode semantics are enforced platform-side. The router's mode handling governs
   messages whose failures it *can* observe.

---

## 1. Topology

| Component | Responsibility |
|---|---|
| `Server` | Process lifecycle: spawns supervisors, gates pools on leadership, drains on shutdown. |
| `Manager` | Owns consumers, pools, queues, publishers. Polls, deduplicates at route time, selects the pool. |
| `Pool` | Passive worker set. Concurrency cap, rate limit, per-group FIFO buffer. Decides the broker action. |
| `Mediator` | Delivery. Owns circuit breakers, the bounded retry burst, and HTTP-response classification. |
| `InFlightTracker` | Duplicate suppression, receipt-handle freshness, stall detection input. |
| `HostPoolRegistry` | Per-host `http.Client` slots, borrowed through a `SlotGuard`. |
| `ConfigSource` | Polls `FLOWCATALYST_CONFIG_URL` for pool/queue definitions; merges and reconciles. |
| `LifecycleManager` | Supervises consumers, warning cleanup, coordinated shutdown. |

The split that carries the design: **a pool does not own a queue.** The manager polls every
queue and routes each message to the pool named by its `PoolCode`, so acknowledgement has to
travel back to the queue the message actually arrived on.

The mediator boundary is the other one worth naming: **the mediator decides what the
response means; the pool decides what that means for the broker.**

## 2. A message, end to end

1. **Poll and deduplicate.** Each consumer long-polls. At route time the manager registers
   the message with the in-flight tracker; a copy arriving while another owns the pipeline is
   ACKed and dropped. This is the first of three duplicate-suppression layers (route-time
   register, process-time `EnsureTracked` backstop, and the SQS `pendingDelete` guard).

   This rests on a contract every backend owes: **`BrokerMessageID` must be stable across
   redeliveries.** A changed broker id under a known message id means "a different copy
   exists", and the router deletes that arrival. A per-delivery id therefore makes the broker
   destroy its own copy on every redelivery — which is what the NATS backend did by folding
   the JetStream *consumer* sequence into the id, turning at-least-once into at-most-once.
   SQS uses `MessageId`, Postgres the row id, NATS the *stream* sequence.
2. **Select the pool.** `PoolCode` names it. An unknown code falls back to `DEFAULT-POOL`
   with a routing warning. A code ending `-DEFAULT-POOL` is a per-client fallback and is
   **synthesised on demand**, because nothing emits `processingPools` — the router polls an
   external config service, so those codes never appear in config.
3. **Admit or push back.** A pool at buffer capacity NACKs with a short delay. This is the
   only backpressure signal the router sends the broker. Upstream of it, a consumer pauses
   polling when the pools **its own last batch fed** are full — judging a queue by the whole
   process meant one idle pool kept every consumer fetching messages it would immediately
   bounce.
4. **Branch on dispatch mode.** `IMMEDIATE` dispatches concurrently, one goroutine per
   message, bounded only by the pool semaphore. Ordered modes enqueue into their message
   group's FIFO buffer, drained serially by a single drainer.
5. **Deliver.** The mediator checks the endpoint breaker, takes a rate-limit token, borrows
   an HTTP client slot for the target host, and delivers — retrying a **bounded burst**
   internally (`deliverWithRetry`: `MaxRetries` total attempts) before returning a verdict.
   So the pool always sees the verdict *after* retrying, never a first response.
6. **Resolve.** The pool turns the outcome into exactly one broker action, and for an ordered
   group also decides whether the rest of the group continues.

## 3. Delivery outcomes

This table is the router's operational contract.

| Response | Broker action | Why |
|---|---|---|
| `2xx` | ack | Delivered. The outcome carries the **real** status — a 201 is a 201. |
| `3xx` | ack | Redirect **not followed**, and permanent: the target reproduces it every time. Following it is not the alternative — 301/302/303 downgrade POST to GET and drop the body, so the redirect target would receive nothing and the router would record a success. |
| `2xx` + `ack:false` | retry in place | Target is healthy and deferring deliberately. Own curve, 5s→60s. |
| `4xx` | ack | Client error; retrying cannot change the answer. |
| `500` | ack | **After the burst.** The app ran the request and threw, so the fault is likely the message. Re-send once resolved. |
| `501` | ack | The deliberate 5xx exception — app reached, endpoint not implemented. |
| `502` `503` `504` | **release group** | Never reached a working app. Nothing about the message is wrong. |
| transport / timeout | **release group** | Same class as a gateway error. |
| circuit open | **release group** | No delivery attempted. Must match the transport path, or the group is pinned in memory on redelivery. |
| `429` | retry in place | Target answered, so it is reachable. Honours `Retry-After`. |
| duplicate copy | ack | Another copy owns the pipeline; this one is deleted with its own receipt. |

Two properties worth not breaking:

- **Release is what makes "retry until the broker expires it" true at all.** An in-place
  retry never hands the message back, so the broker's expiry and DLQ can never act on it.
  This is also why in-place retry is **bounded** (`maxInPipelineAttempts`, 10): past the
  budget the message is released with its backoff as the redelivery delay. Unbounded, an
  endpoint answering `429` or `ack:false` for ever pinned the message — and its whole ordered
  group — in memory, exempt from reaping and, until recently, silent to the stall detector.
- **After a release the cadence is the broker's** (visibility timeout / ack-wait), not our
  backoff curve, and the **circuit breaker** is what actually spares the target from the
  redelivery rate. Of the three backends only SQS ignores a nack delay (its `Nack` is a
  deliberate no-op); Postgres honours it via `makeVisible`, NATS via `NakWithDelay`.

## 4. Ordering

The three modes share the same FIFO buffer and the same in-order delivery. They differ in
exactly one respect — **what a terminal failure does to the rest of the group**:

| Mode | On a terminal failure |
|---|---|
| `IMMEDIATE` | No group at all; messages dispatch concurrently. |
| `NEXT_ON_ERROR` | Group moves on to the next message. **The default when no mode is given.** |
| `BLOCK_ON_ERROR` | Group stops; the untried successors are **ACKed off the queue**, not released. |

**Why `BLOCK_ON_ERROR` acks rather than releases.** Releasing the successors reads as the
safer option and is the opposite. They would redeliver on the **broker's own timer**, which
has no connection to the failure being resolved — and by then the head has been ACKed away,
so the first successor becomes the new head and is delivered. "Add item" applied to an order
that was never created: precisely what the mode is named after preventing. The messages are
not lost by acking — they remain as dispatch-job rows in the platform store, which is the
system of record; the queue copy is a delivery attempt, not the data. **Known gap:** nothing
marks those rows for review, so they sit at `QUEUED`/`PROCESSING` rather than `FAILED`.

**Ordering is defined only relative to a group.** A message in an ordered mode that
carries no `messageGroupId` dispatches concurrently, exactly as `IMMEDIATE` would. This is
what makes an ordered default safe: ungrouped messages share the group key `""`, so
honouring the mode literally would file a pool's entire ungrouped volume into one buffer
behind one drainer — concurrency 20 delivering one message at a time.

Ordering survives a release because both brokers redeliver a group in order: the Postgres
queue makes only the earliest visible message of each group claimable, and SQS FIFO orders by
`MessageGroupId`. **Ordered work on a non-FIFO SQS queue would reorder on release.**

### Where it is enforced for dispatch jobs

`/api/dispatch/process` delivers to the real subscriber, records the outcome, and returns
`200 {ack:true}` regardless — so the router sees success even when delivery failed. Dispatch
jobs' mode semantics are therefore enforced **platform-side**, at two points:

- **Claim time** — `filterByDispatchMode` holds back a `BLOCK_ON_ERROR` job while an
  **earlier** job in its group is holding the group up; `IMMEDIATE` and `NEXT_ON_ERROR` keep
  flowing.
- **Delivery time** — `GroupHeldBefore`: a `BLOCK_ON_ERROR` job held by an earlier sibling is
  ACKed off the queue and reset to PENDING without consuming retry budget. This gate exists
  because messages already on the queue when the sibling ahead of them stalled would
  otherwise arrive and deliver past it.

**Holding** (`GroupHoldingStatusSQL`) means FAILED/ERROR **or PENDING with a future
`scheduled_for`** — a job sitting out a retry backoff. That second case is easy to miss: such
a job is excluded from the claim query by its own `scheduled_for` and is not FAILED, so
nothing treated it as holding anything, and its successors were delivered while it waited. A
backed-off job still owns its place at the front of the group.

QUEUED and PROCESSING deliberately do **not** hold: that is the normal flow, where the poller
hands a group's whole eligible run to the router in one batch and the router's per-group FIFO
sequences it. Treating them as holders would reduce every ordered group to one job per poll.

The comparison is **positional**, over the same `(sequence, created_at, id)` the claim orders
by — not set membership. "This group contains a held job" would include the held job itself
the moment its backoff expired, and the group would never move again. Note that `sequence` is
a *subscription* priority, so in a group fanned out to several subscribers a stalled job in
the earlier-sequenced subscription holds the later-sequenced one too.

The router's own mode handling governs operator-submitted messages (which set `DispatchMode`
from the request and target a real URL) and any producer pointing a message at a subscriber
directly.

## 5. Pool codes

The scheduler resolves the published pool code **at publish time**, so the router routes by
the code it is handed and needs to know nothing about clients.

| Job state | Published `poolCode` |
|---|---|
| pool set, pool has a client identifier | `{clientIdentifier}-{poolCode}` |
| pool set, platform-level pool | `{poolCode}` — no prefix |
| no pool, job's client resolves | `{clientIdentifier}-DEFAULT-POOL` |
| neither | `DEFAULT-POOL` |

Namespacing is required because `msg_dispatch_pools` is unique on `(code, client_id)` — two
clients may each own a pool coded `FAST` — while the router keys pools by code alone and
treats one code with differing settings as a **conflict to reject**, not two pools.

**The composed code is opaque.** Both halves may contain hyphens, so it can never be split
back apart. The only permitted structural read is a suffix test for `-DEFAULT-POOL`.

## 6. Lifecycle

**Startup** spawns supervisors first — notifier, stall detector, queue-health monitor,
in-flight reaper, broker-stats refresher — then starts pools. With standby election enabled,
pools start **only on acquiring leadership**: the per-group drainer is in-process, so two
active routers would dispatch a group's messages concurrently and lose the ordering the
buffer exists to provide.

**Shutdown** reverses deliberately:

1. Deregister from the load balancer, so no new traffic arrives while in-flight work
   finishes.
2. `Manager.StopPolling` — intake ends. Messages already routed keep running: a consumer
   carries **two** cancellations, one that ends its poll loop and one that tears the
   consumer down, and only the first is used here.
3. Drain — wait for the tracker to empty, against `DrainTimeout`, on a context of its own so
   the already-cancelled run context can't cut it short.
4. Stop the lifecycle manager, the manager, then the election.

Steps 1 and 2 are what make step 3 mean anything. Consumer contexts used to descend from the
run context, so the cancellation that started the drain simultaneously aborted every
in-flight HTTP delivery — the drain then waited its full timeout on work nothing was going
to finish. **A consumer's lifetime belongs to the Manager**, not to whoever called
`Reconfigure`: callers pass fetch-scoped or bootstrap contexts, and inheriting from one of
those kills every poll loop when it expires.

Leadership loss follows the same sequence, for the same reasons.

A pool stopping mid-drain flushes its buffered messages and **releases their tracker
entries**, so the broker's redeliveries re-enter cleanly instead of being dropped as
duplicates of copies that no longer exist.

---

_Coverage note: `pool.go`, `manager.go`, `mediator.go`, `inflight.go`, `server.go`'s shutdown
path and the queue backends have been read closely and are covered by tests. `host_pool.go`,
`health.go`, `lifecycle.go` and `notification.go` were surveyed by signature and remain
largely untested — treat statements about them as unverified._
