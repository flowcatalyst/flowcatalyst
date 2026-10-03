# Router pool back-pressure: stop over-pulling, park overflow at the platform

**Status:** draft design for owner review, 2026-10-02. Nothing here is built.
Findings from a multi-queue benchmark investigation are still arriving; the
numbers in §2 are the measured ones so far.

## 1. Problem

When a processing pool is full, the router currently keeps pulling messages
for it and hands each one back to the broker with a delay (a *deferral*, §3).
That was introduced on 2026-09-22 to fix head-of-line blocking: a dedicated
pool at concurrency 1 with a 10,000-message backlog used to stop its whole
queue's consumer for fourteen hours, starving every other pool that shared
the queue.

It fixes that case, but at a cost that grows with the number of queues
feeding a pool, and on NATS the cost is severe.

## 2. Evidence

Benchmark: NATS JetStream (in-memory storage), 500,000 messages pre-seeded
across N queues, one shared pool of concurrency 256, router limited to one
CPU, a trivial HTTP/2 target. Time series sampled every 2 s.

| Queues | Observed (this router) |
|---|---|
| 1 | Runs at 18–32k/s but freezes for 26–28 s at a time (see below) |
| 8 | 20.9k/s in one run; stalled at 432k delivered in another |
| 16 | Full speed (~19k/s) until ~494k delivered, then ~1 message/s |

1. **The 16-queue tail is deferral spacing.** At 16 queues the pool counter
   `fc_messages_rejected_total{reason="capacity"}` read 5,658, matching the
   ~5,600 messages left undelivered. At 1 and 8 queues it read zero.
   `admissionDelay` (`internal/router/pool_admission.go`) books each deferral
   one slot after the previous, and for brokers whose
   `HonoursDelayedReturn()` is false (NATS only) floors the slot at
   `deferralOrderedSpacing` = 1 s regardless of pool speed. 5,658 deferrals
   therefore return at one per second, about 94 minutes. The floor protects
   *ordered* groups from swapping order on redelivery; it is applied to
   IMMEDIATE messages as well.
2. **More queues means more overshoot.** One consumer cannot out-pull a pool
   that drains faster than one round trip, so a single queue never fills the
   buffer. N consumers each decide to poll from the same stale view of
   "does a pool this queue fed last time have room" (`hasCapacityFor` /
   `destHasRoom`, `internal/router/manager.go`) and, together, overshoot the
   shared buffer by up to N batches. When every pool is full the consumer
   still polls until `defaultDeferralBudget` (15,000 outstanding deferrals,
   `deferral_ledger.go`) is spent, so nearly everything pulled past the buffer
   is bounced.
3. **The single-queue freezes are a separate defect** in how the NATS
   consumer uses the client library (`internal/queue/nats/nats.go`,
   `forward()`): after the pool buffer fills and drains, `Next()` blocks for
   exactly the client's default pull-request expiry (30 s) with messages
   waiting on the server. Reproduced with only nats.go v1.52.0 and the same
   permit-gated loop; intermittent; bounded to a few seconds by setting
   `PullExpiry` to 2–5 s; not seen with a `FetchNoWait` loop.
4. **Not the cause:** CPU quota (identical at 1 and 2 CPUs), the NATS server
   (2–28% CPU), the target, the broker's `max_ack_pending` (removing the
   limit did not fix it), storage (the rig uses memory).

## 3. Current behaviour

- `Pool.queueCapacity()` = `max(concurrency × 40, 100)`; 10,240 at
  concurrency 256 (`internal/router/pool.go`). It counts accepted but
  not-yet-being-delivered messages.
- `Pool.submit` rejects a message for a full pool and `deferMsg` hands it back
  with a computed delay. `admissionDelay` books a slot after `nextReturn`,
  sized by the pool's measured completion rate, floored at 5 s
  (`deferralMinDelay`), plus the 1 s spacing floor on NATS.
- `Manager.hasCapacityFor` keeps a consumer polling while *any* pool its last
  batch fed has room, and, if all are full, while its deferral budget lasts.
- Message-copies in the queue are delivery attempts, not data: the dispatch
  job row in `msg_dispatch_jobs` is the system of record. This is already
  relied on twice:
  - `ackBuffered` ACKs a blocked group's siblings and reports them to
    `POST /api/dispatch/settled`; `dispatchjob.RunReaper` covers a router
    crash between the ACK and the report.
  - `/api/dispatch/process` ACKs the queue message and calls
    `Repository.Reschedule` to put a job back to PENDING with a future
    `scheduled_for`, *without* consuming retry budget ("cooperative
    back-pressure"), for `ack=false`, HTTP 429 and held groups. The scheduler
    poller (`internal/platform/scheduler/poller.go`) is the single
    re-dispatch driver and honours `scheduled_for`.

## 4. Proposal

Three layers. Each is useful alone; together they remove the over-pull.

### 4.1 Router: never pull what you cannot place

Replace "keep polling into a full pool and bounce" with pull gating:

1. **Pause when nothing can be placed.** If every pool a queue is known to
   feed is full, park the consumer (the existing `awaitCapacity` gate) and do
   not poll. "Known to feed" becomes the set of pools seen on that queue over
   a window (minutes), not the last batch only.
2. **Probe, rate-limited.** To discover a pool hiding behind a backlog, allow
   one probe batch per fixed interval (default 5 s) while parked, instead of
   spending a 15,000-message deferral budget immediately. A probe that finds
   another pool's message routes it normally; messages for the full pool go
   through §4.2.
3. **Size the pull to the room.** Where the broker supports it (NATS pull,
   SQS `MaxNumberOfMessages`), request `min(batch, free room across the pools
   this queue feeds)`.
4. **Unordered messages never use the 1 s spacing floor.** Keep
   `deferralOrderedSpacing` only for ordered-mode messages, where it
   protects ordering. This is also the fix for the 16-queue tail on its own.

Standalone routers (no platform) keep the existing deferral as the overflow
path, with the floor change above.

### 4.2 Park overflow at the platform

When a message must be shed (a probe found a full pool, a race overshot, or a
pool is being drained), the router asks the platform to *park* the job instead
of bouncing it through the broker:

```
POST /api/dispatch/park
{ "poolCode": "BENCH",
  "notBefore": "2026-10-02T14:21:07Z",
  "reason": "pool at capacity",
  "jobs": [ { "id": "djb_…", "token": "<scheduler-signed token>" }, … ] }
```

Platform behaviour per job, in one `UPDATE … WHERE id = ANY($1)` guarded on
status:

- verify the HMAC token exactly as `/api/dispatch/settled` does (no new
  credential);
- only `QUEUED` (and `PROCESSING` by the same stale-lease rule as the
  reaper) jobs are changed; anything else is reported back as `skipped`;
- set `status = 'PENDING'`, `scheduled_for = notBefore`, **without bumping
  `attempt_count`** (the existing `Reschedule` semantics);
- respond with the ids actually parked.

The router **ACKs only the ids the platform confirmed**, then tracker-removes
them. If the call fails or times out, it falls back to the existing deferral
for those messages. Order is therefore *park, then ack*: a crash between them
leaves a PENDING job whose queue copy redelivers, and the processing endpoint
already resolves the duplicate (the lease/status guard); a job that is
parked but whose ack is lost cannot be lost, only duplicated.

Parked jobs are not in flight at the broker, so they do not count against SQS
in-flight limits (20,000 for FIFO) or NATS `max_ack_pending`. That is the
reason this layer exists: holding overflow in router memory would still hold
it in flight at the broker and hit those ceilings.

### 4.3 `notBefore` from measured pool speed

The router already measures each pool's completion rate
(`PoolMetricsCollector.CompletionRate`, 5-minute window). For a batch of `k`
messages shed from a pool with `q` buffered and rate `r`:

```
notBefore = now + max(minPark, (q + k) / r) , capped at maxPark
```

with `minPark` 5 s (same floor as today), `maxPark` configurable (default
10 min) and ±10% jitter so a park of thousands does not return as one wave.
With no rate yet, use today's 30 s fallback. Because the platform, not the
broker, now holds the message, there is no per-slot spacing floor: the
poller's claim batch size is the pacing.

### 4.4 Pool pressure: stop new work reaching a full pool

Parking alone moves the backlog, but the poller would keep claiming *new*
jobs for the saturated pool. The platform records the pressure the router
reported and the poller respects it:

- Table `msg_pool_pressure (pool_code text primary key, accept_after
  timestamptz not null)`. A park call upserts
  `accept_after = GREATEST(existing, notBefore)`. Rows expire by time
  (`accept_after < now()` is "no pressure"); a janitor deletes old rows.
- `pollOnce` adds an anti-join: skip jobs whose pool has
  `accept_after > NOW()`. This is the same shape as the existing
  paused-subscription filter (`pausedCache.PausedSubscriptionIDs`).
- A pool that drains faster than predicted can clear pressure early: the
  router sends `POST /api/dispatch/pool-pressure/clear` (or a park call with
  `notBefore` in the past) once the pool is below a low-water mark.

This is back-pressure at the source: the dispatch scheduler stops publishing
for a saturated pool, so the queue stays short and other pools on the same
queue are not blocked behind a slow pool's backlog. It replaces both the
bounce loop and the head-of-line workaround.

Persisting pressure in the database (not scheduler memory) keeps it correct
across leader changes in an HA deployment; the extra read is one indexed
lookup per poll.

### 4.5 Ordered groups

For ordered modes the unit that must be parked is a group suffix, not a
single message: parking one job of a group while later ones stay queued would
reorder it. The router parks the in-hand message and every message buffered
behind it for that group in one call. The poller's existing total order
(`message_group, sequence, created_at, id`) and the group hold-back logic
(`GroupHeldBefore`) then re-queue the group in order when `scheduled_for`
falls due. A parked job counts as "holding" its group in the same way a
retry-backoff job does today, so later jobs of the group are not queued past
it.

### 4.6 Failure modes

| Failure | Result |
|---|---|
| Park call fails or times out | Existing deferral for those messages; no loss |
| Router crashes after park, before ACK | Queue copy redelivers; job is PENDING with `scheduled_for`; the processing endpoint's status guard rejects a delivery that arrives early; the poller republishes at `scheduled_for`; duplicate possible, loss not |
| Router crashes after ACK, before the tracker is updated | Same as today for ACKed messages |
| Platform down | Router falls back to deferral; pull gating (§4.1) still limits how much it sheds |
| Parked job never re-queued | `RunReaper` already finds QUEUED/PROCESSING jobs stranded behind failures; extend its sweep (or add a sibling sweep) for PENDING rows with `scheduled_for` far in the past and not claimed |
| Park storms (thousands per second) | Batch `UPDATE … ANY($1)`; park rate is bounded by §4.1 gating; cap one call at 10,000 jobs like `settled` |

### 4.7 Observability

- Router: `fc_router_parked_total{pool}`, `fc_router_park_failures_total`,
  `fc_router_pool_pressure_active{pool}`; park decisions in the flight
  recorder.
- Platform: parked and cleared counts, current pressure rows, and a warning
  when a pool has been under pressure longer than a configurable threshold
  ("pool X saturated for 30 minutes: raise concurrency or give it its own
  queue").
- A gauge on total messages in flight per router, with an alert, instead of a
  global memory cap: if pull gating is correct it is bounded by pool
  capacity, and a rising value is a leak signal.

## 5. Alternatives considered

| Option | Why not (alone) |
|---|---|
| Keep broker deferral, fix the spacing floor | Fixes the tail but keeps over-pulling and the redelivery churn; each bounce spends a broker round trip and a redelivery count |
| Large in-memory overflow buffer | Held messages are still un-acked at the broker: they count against SQS in-flight limits (20,000 FIFO) and NATS `max_ack_pending`, need visibility/ack-wait extension, and are lost to a crash as redeliveries |
| Global memory semaphore, all pollers stop when full | Safe for memory but recreates the original head-of-line block: one slow pool's backlog consumes the global budget and stops pollers for every other pool. Unnecessary once pull gating bounds intake |
| One queue (or NATS filtered consumer) per pool | The cleanest structural fix and still recommended for pools that can starve each other, but it requires publisher and deployment changes and does not help a shared queue already in service |

## 6. Interim fixes (small, independent, do first)

1. Apply the 1 s spacing floor only to ordered-mode messages
   (`pool_admission.go`).
2. Stop polling into a full pool when the queue's known pools are all full
   (§4.1 steps 1–2), with a probe interval instead of the 15,000 budget.
3. Set a short `PullExpiry` (2–5 s) and a matching heartbeat on the NATS
   consumer's `Messages()` call (`internal/queue/nats/nats.go`), or move to a
   `FetchNoWait` loop; owner's call, since the continuous subscription was an
   explicit ruling.
4. Update the benchmark rig's NATS consumer settings: it still forces
   `max-ack-pending=1000` and `max-deliver=10`, while the router defaults
   changed to unlimited on 2026-09-22.

## 7. Rollout

1. Interim fixes (§6), behind no flag.
2. Pull gating (§4.1) with the probe interval configurable; default on.
3. Park endpoint, pressure table and poller filter (§4.2–4.4) behind
   `FC_ROUTER_PARK_ENABLED` (router) and a platform setting; default off.
   Requires the platform base URL the settled reporter already uses.
4. Ordered-group parking (§4.5) after the unordered path has soaked.

Migration: one new table (`msg_pool_pressure`), additive. No change to
`msg_dispatch_jobs`.

## 8. Test plan

- **Unit:** pull gating decisions (all-full, probe interval, mixed pools);
  `notBefore` arithmetic including no-rate and cap; spacing floor applied only
  to ordered messages.
- **Integration (`-tags=integration`):** park endpoint verifies tokens,
  skips jobs not QUEUED, does not bump `attempt_count`, upserts pressure with
  `GREATEST`; poller skips pressured pools and resumes at `accept_after`;
  ordered group parked as a suffix re-queues in order.
- **Failure injection:** platform unreachable (falls back to deferral), park
  succeeds but ACK fails (no loss, duplicate resolved), router killed
  mid-park, pressure row cleared early.
- **Benchmark matrix (the rig in `bench/router`, same shape as §2):** 1, 8
  and 16 queues on NATS and SQS (an in-process SQS emulator is enough for
  routing behaviour), 500,000 messages, 1 CPU, repeated, with a time series.
  Pass criteria: no stall, throughput at 16 queues within 15% of 1 queue,
  zero broker deferrals in the single-pool case.
- **Head-of-line regression:** the 2026-09-22 scenario (one slow pool at
  concurrency 1 with a 10,000 backlog sharing a queue with a fast pool): the
  fast pool must keep flowing and the slow pool's backlog must end up parked,
  not in flight.

## 9. Open questions

1. Where should pool pressure live: a table (proposed, HA-safe) or scheduler
   memory (cheaper, lost on leader change)?
2. Is parking worth supporting for messages that did not originate from the
   platform's dispatch jobs, or is deferral the right fallback there?
3. `maxPark` default, and whether it should follow the pool's own timeout
   configuration.
4. Should a pool's pressure also count queue age (oldest parked job), so an
   operator sees saturation before throughput visibly drops?
5. Which of the NATS consumer fixes in §6 item 3 the owner prefers: short
   pull expiry on the continuous subscription, or a no-wait fetch loop.

### 4.8 Queue mode: block or defer (decision noted 2026-10-03, not built)

Deferral (§3) exists so a slow pool on a **shared** queue cannot hold up the other
pools' messages behind it. It is the wrong tool for a queue that feeds **one** pool: there
is nothing to protect, and deferring a large backlog just moves it around the broker.

- **Per-queue mode, `block` or `defer`.** `defer` (the default, today's behaviour):
  a full pool defers its messages and the queue keeps being read. `block`: when the
  queue's pool is full the consumer stops polling and the messages wait in the broker.
- **Derive it, allow an override.** A queue that feeds exactly one pool can default to
  `block`; a queue feeding several defaults to `defer`; an explicit setting wins.
- **Dedicated queue per pool.** A pool may be marked as having its own queue. The
  platform, which already knows each message's pool, publishes that pool's messages to
  the queue named for the pool; the router reads it in `block` mode. This is the
  worked-out form of §4.2 (parking at the platform) and needs no router-side queue
  creation. Moving messages between queues inside the router is not proposed: send plus
  delete is not atomic (duplicates or loss), and it affects FIFO groups and IAM.
- **Buffer sizing in `block` mode.** Polled messages wait in the pool buffer, so for a
  slow pool the buffer must be small, or visibility must be extended: 100 buffered
  messages at 2 s each is 200 s, well past a 30 s visibility timeout, and would redeliver.
- **The motivating case:** one pool at concurrency 1 taking 2 s per message, 10,000
  arriving in one go on a shared queue. Lowering the maximum deferral is not an option
  for it; a dedicated `block` queue removes the churn.
- **Metrics** (needed either way): per pool, full (0/1), time at capacity, deferrals
  total and outstanding; per queue, paused-for-capacity and its mode.
