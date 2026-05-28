# Router alignment with Rust reference

**Status: all three phases landed.** Tests green across `internal/router`
and `internal/router/api`.

Tracks the work to bring `internal/router` to parity with `crates/fc-router`
on three gaps Gemini flagged: pool telemetry, broker stats cache, and the
dashboard + dashboard-shaped API surface.

Cross-references:
- Rust source: `../flowcatalyst-rust/crates/fc-router/src/`
- Rust API: `../flowcatalyst-rust/crates/fc-router/src/api/mod.rs`
- Dashboard HTML (already copied): `internal/router/api/dashboard.html`

## Phase 1 — Pool telemetry (foundation)

The dashboard endpoints all read pool stats with windowed metrics, so
metrics must land before the API can be filled in.

- [x] **Common types** — add `ProcessingTimeMetrics`, `WindowedMetrics`,
      `EnhancedPoolMetrics` to `internal/common` (camelCase JSON, mirrors
      `fc_common::lib.rs` lines 815-918).
- [x] **PoolMetricsCollector** — `internal/router/metrics.go`:
  - all-time atomics: `total_success`, `total_failure`, `total_rate_limited`
  - bounded ring of `{ts, duration_ms, success}` samples (max 10k, 30-min cap)
  - separate rate-limited timestamp ring (long-window cap)
  - `record_success/record_failure/record_transient/record_rate_limited`
  - `Snapshot() EnhancedPoolMetrics` — derives all-time + 5-min + 30-min
  - percentiles p50/p95/p99 by sorting a copy of the window slice
    (skip hdrhistogram-go to keep the dep tree small)
- [x] **Pool atomics** — replace `inFlight atomic.Int64` with
      `queueSize` + `activeWorkers` atomics so PoolStats has live values.
      `processOne` records into the collector with elapsed ms.
- [x] **PoolStats** — extend `internal/router/health_types.go` with
      `Metrics *common.EnhancedPoolMetrics` (`json:"metrics,omitempty"`).
- [x] **Pool.Stats()** — assembles `router.PoolStats` from collector +
      atomics. Add `Manager.PoolStats() []router.PoolStats`. Drop the
      `nil` TODO in `api.FromServer`.
- [x] **Hot reconfigure** — `Pool.UpdateConcurrency(uint32) bool` and
      `Pool.UpdateRateLimit(*uint32)` (the registry already supports
      hot-swap via `SetRateLimit`; concurrency needs a bounded resize).
      Add `Manager.UpdatePool(code string, cfg common.PoolConfig)`.
- [x] **Tests** — `metrics_test.go` mirroring the Rust tests
      (`crates/fc-router/src/metrics.rs::tests`).

## Phase 2 — Broker stats cache + queue metrics

- [x] **`CachedBrokerStats`** in `internal/router/broker_stats.go`:
  - 60s background refresh that pulls `Consumer.Metrics(ctx)` from each
    running pool's consumer (Manager fan-out)
  - 30-min counter-history ring (oldest-first) keyed by queue identifier
  - `GetWindowed(window time.Duration) []queue.Metrics` overlays cached
    SQS attrs onto live counters and applies the window delta
  - `Refresh(ctx)` for on-demand refresh
- [x] **Manager surface** — `Manager.QueueMetrics(ctx) []queue.Metrics`
      and `Manager.QueueCounters() []queue.Metrics` (live atomics only).
- [x] **Server wiring** — spawn the refresh goroutine in `Server.Run`,
      tied to the existing shutdown.

## Phase 3 — API surface + embedded dashboard

- [x] **Embed HTML** — `//go:embed dashboard.html` in
      `internal/router/api/`. Handler substitutes the `__FC_API_BASE__`
      placeholder with the mount prefix supplied at register time
      (default empty). Mount at `/monitoring/dashboard` and
      `/dashboard.html`.
- [x] **Dashboard reads** (all camelCase, match dashboard fetches):
  - `GET /monitoring/pool-stats?time_window=5min|30min|all-time`
  - `GET /monitoring/queue-stats?time_window=&refresh=`
  - `GET /monitoring/queues`
  - `GET /monitoring/circuit-breakers`
  - `GET /monitoring/in-flight-messages?limit=&messageId=&poolCode=`
- [x] **Mutations**:
  - `POST /monitoring/broker-stats/refresh`
  - `POST /monitoring/warnings/{id}/acknowledge` (alias for `/warnings/...`)
  - `POST /monitoring/circuit-breakers/{name}/reset`
  - `POST /monitoring/circuit-breakers/reset-all`
  - `PUT  /monitoring/pools/{poolCode}` — body `{concurrency?, rate_limit_per_minute?}`
- [x] **Tests** — handler shape tests + dashboard HTML smoke test.

## Phase 4 — Huma migration + missing endpoints (delivered)

- Whole router API migrated to `huma/v2` so it shares the Swagger surface with the platform aggregates.
- Mount pattern (`cmd/fc-router`):
  ```go
  api := humachi.New(r, huma.DefaultConfig("FlowCatalyst Router API", routerapi.Version))
  routerapi.Register(api, routerapi.FromServer(srv))
  routerapi.MountDashboard(r) // chi mount for embedded HTML
  ```
- `Manager` now tracks the queue config per pool and lazily caches
  `queue.Publisher`s by pool code, used by `POST /messages` and
  `POST /api/seed/messages` so messages flow through the same broker
  the consumer reads from.
- Added: `POST /messages`, `POST /api/seed/messages`, `GET /api/config`,
  `POST /config/reload`, `GET /monitoring/standby-status`,
  `GET /monitoring/traffic-status`, `GET /monitoring/circuit-breakers/{name}/state`,
  `GET /monitoring/in-flight-messages/check`,
  `POST /monitoring/in-flight-messages/check-batch`,
  `GET /monitoring/warnings/unacknowledged`,
  `GET /monitoring/warnings/severity/{severity}`,
  and the `/api/test/*` mock surface (fast/slow/faulty/fail/success/pending/
  client-error/server-error/stats/stats/reset) + `/api/benchmark/*` aliases.
- Tests cover every group (`internal/router/api/api_test.go`) using
  `humatest`. Pool / in-flight / warning / mock state are exercised via
  small stub providers so the API tests don't need a live broker.

## Phase 5 — Operational hardening (delivered)

- **BasicAuth middleware** (`internal/router/api/auth.go`): env-driven
  (`FC_ROUTER_AUTH_USER`/`PASS`). Skips public paths (`/health*`,
  `/metrics*`, `/docs/*`, `/openapi.*`). Constant-time compare.
- **Prometheus `/metrics`** (`internal/router/api/prometheus.go`): proper
  text exposition via `prometheus/client_golang`. Emits per-pool gauges +
  counters (success/failure/rate_limited), latency percentiles, per-queue
  gauges + counters (polled/acked/nacked/deferred + pending/in-flight),
  per-target breaker open gauge + call counters, and a global in-flight
  gauge. Mounted at `/metrics` and `/q/metrics` in `cmd/fc-router`.
- **`/config/reload`** now performs a real fetch + reconfigure via
  `Server.Reload(ctx)`; the watcher-only no-op message is gone.
- **Traffic strategy** (`internal/router/traffic.go`): ALB target-group
  register/deregister via `aws-sdk-go-v2/service/elasticloadbalancingv2`.
  Env: `FC_TRAFFIC_ENABLED`, `FC_TRAFFIC_TG_ARN`,
  `FC_TRAFFIC_INSTANCE_IP`, `FC_TRAFFIC_PORT`, `AWS_REGION`. Wired into
  leader transitions: register on leader-gain, deregister BEFORE drain on
  leader-loss + shutdown. Non-standby deployments also register on startup.
  `/monitoring/traffic-status` reports live state.
- **Stream health stubs** (`/monitoring/stream-health{,/live,/ready}`)
  return `{enabled: false, status: "NOT_CONFIGURED"}` so the dashboard
  doesn't 404.

## Phase 6 — Stream health tracking (delivered)

- `internal/stream/health.go`: per-projection `Health` (atomics for
  running/processed/errors/last-poll) + `HealthService` aggregator
  (Register / IsLive / IsReady / Aggregate). Mirrors
  `crates/fc-stream/src/health.rs`.
- `Projector.Health` + `PartitionManager.Health` (optional fields).
  Loops toggle running on entry/exit, bump processed per non-empty
  step, record errors on Step failures. nil-safe so existing tests
  keep working.
- `cmd/fc-stream-processor` now exposes
  `GET /monitoring/stream-health{,/live,/ready}` with real data.
- `internal/server/subsystems.StartStreamProcessorWithHealth` lets
  fc-server hand a `stream.HealthService` back so it can also be wired
  into the router's API state.
- Router API gained `StreamHealthProvider` on `State`; when set the
  stream-health endpoints reflect live data, otherwise they keep the
  `NOT_CONFIGURED` stub. Adapters in fc-server (not yet wired — see
  below) translate `stream.HealthService` → `routerapi.StreamHealthProvider`.

## Open (small) follow-up

- fc-server doesn't currently mount the router HTTP. When it does, the
  glue is one adapter that converts the shared `stream.HealthService`
  into `routerapi.StreamHealthProvider`. The path is unblocked.

## Design notes

**Percentiles without hdrhistogram-go.** The Rust path uses HdrHistogram
for O(1) record + percentile reads. With a bounded 10k sample ring, a
`sort.Slice` on snapshot is fine (sub-ms for 10k float64s, called at
most once per `/monitoring/pool-stats` hit) and avoids the dep.

**Windowed deltas vs windowed samples.** The Rust pool path uses
*samples* for windowed counts (so it can compute throughput and
per-window percentiles). The Rust queue path uses *cumulative counter
snapshots* (because consumers expose only running totals). We mirror
both — `PoolMetricsCollector` uses samples, `CachedBrokerStats` uses
snapshot deltas.
