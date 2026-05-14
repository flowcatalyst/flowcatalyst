# Runbooks

Operational playbooks for common incidents and routine procedures. Each runbook is short and actionable — diagnose, decide, act.

For background, see the per-component architecture docs and [observability.md](observability.md).

---

## Routine procedures

### Rolling deploy

For active/standby pairs (Topology 2):

1. Deploy fc-server v(N) to the standby node first. Wait for `/health` to show `STANDBY` (not `STARTING`).
2. Force failover (kill the leader's Redis lock or SIGTERM the leader). Standby acquires leadership.
3. Deploy fc-server v(N) to the previously-leader node. Wait for `/health` to show `STANDBY`.

For platform-only nodes (no background work): just rolling restart. No special procedure.

For mixed topologies: deploy the API tier first (no leadership concerns), then deploy the background tier (one leader at a time).

### Database migration

Migrations run automatically at startup. Procedure:

1. Write the migration (`NNN_alter_<table>_<change>.sql`, idempotent).
2. Deploy normally. Every starting node applies new migrations; `_schema_migrations` provides serialisation.
3. Watch logs at startup for `Applied migration` lines and any `Migration content changed` warnings (the latter only on shipped-and-edited migrations — see [postgres.md](postgres.md#migrations)).

For a long-running migration (a backfill, a big index build): prefer staging it as a separate job. Add the column / index in one migration (fast); run the backfill from a one-off script that won't block deploys.

### Rotating database credentials

If using AWS Secrets Manager rotation (recommended):

1. Trigger the rotation in Secrets Manager (or wait for the scheduled rotation).
2. New connections fail with auth errors for up to `DB_SECRET_REFRESH_INTERVAL_MS` (default 5 min).
3. After that window, all pools have the new credentials. No restart needed.

If manual (no rotation provider):

1. Change the password on the Postgres user.
2. Update the secret store with the new password.
3. Rolling-restart every platform node.

If the rolling restart can't be coordinated within the brief window where some nodes have new creds and some have old: rotate to a transitional state where Postgres accepts both passwords (only possible with multi-user rotation), or accept a brief auth-error window.

### Rotating JWT signing keys

See [secrets-and-rotation.md](secrets-and-rotation.md#jwt-signing-key-rotation).

### Adding a new email-domain mapping (new tenant via OIDC)

1. Verify the IDP is configured: `GET /api/identity-providers` (look for the IDP record).
2. `POST /api/email-domain-mappings`:
   - `email_domain`: e.g. `newco.com`
   - `identity_provider_id`: from step 1
   - `scope_type`: `Client`
   - `client_id`: the target tenant's client ID
   - `auto_create_principal`: typically `true`
   - `role_assignments`: which roles to grant on first login
3. Test login: have a user in the new domain attempt `POST /auth/check-domain`. Should return `{ method: "oidc", provider_id: "idp_..." }`. Then complete the OIDC flow.

### Pausing a misbehaving webhook receiver

1. Find the connection: `GET /api/connections?q=<endpoint>`.
2. `POST /api/connections/{id}/pause` (or via admin UI).
3. Within 60 seconds the scheduler stops dispatching to that connection. PENDING jobs stack up.
4. When fixed: `POST /api/connections/{id}/resume`. Pending jobs drain naturally.

This is the "stop the bleeding" lever. Persistent — survives restarts. Compare to the router's circuit breakers, which are automatic and transient.

---

## Incident playbooks

### Symptom: dispatch backlog growing

**Watch:** `fc_scheduler_pending_jobs` rising over time.

**Diagnose:**

1. Check if a connection is paused: `GET /api/connections?status=PAUSED`. Each paused connection holds back its jobs.
2. Check if the scheduler is running: `/health` should show `scheduler: UP` on the leader.
3. Check the leader: `redis-cli GET fc:server:leader`. Should be one of the fc-server nodes.
4. Check throughput vs ingest: compare `rate(fc_events_received_total[5m])` to `rate(fc_scheduler_jobs_queued_total[5m])`. If queueing is keeping up, the bottleneck is downstream (router or receiver).
5. If queueing is keeping up but `fc_router_messages_processed_total` is much lower: the router is the bottleneck (pool concurrency, rate limit, or circuit breaker).
6. If both are keeping up but the backlog is still growing: ingest rate exceeds sustainable throughput. Scale up.

**Act:**

- Paused connection → un-pause once safe.
- Scheduler not running → restart the leader (or check Redis for lock state).
- Router bottleneck → raise pool concurrency via admin UI or `PUT /monitoring/pools/{code}`.
- Saturation → scale up nodes or split pools.

### Symptom: webhook deliveries failing

**Watch:** `fc_router_messages_failed_total{reason}` rising.

**Diagnose:**

1. Per-endpoint failure rate: check `GET /monitoring/circuit-breakers` — which endpoints are open?
2. Look at `GET /monitoring/warnings` for recent warnings filtered by endpoint.
3. For specific dispatch jobs: `GET /api/dispatch-jobs/{id}/attempts` shows attempt history with status codes and bodies.
4. From the receiver's side: are they getting requests at all? Check their logs.

**Act:**

- 4xx errors → tell the operator who set up the connection that their config is wrong (URL, auth, payload format).
- 5xx errors → contact the receiver's operator; circuit breaker will protect us until they recover.
- 429 errors → reduce pool rate limit, or work with receiver on a higher quota.
- Connection timeouts → networking issue; check VPC routing, security groups.

### Symptom: stale-recovery firing repeatedly

**Watch:** `rate(fc_scheduler_stale_jobs_recovered_total[5m]) > 0` sustained.

**Diagnose:**

This means jobs are sitting in `QUEUED` longer than the threshold (15 min default) without finishing. The router took them, but didn't ACK or NACK within 15 min. Causes:

1. **Receiver is hanging.** A `target_url` that doesn't respond. Look at `fc_router_dispatch_duration_seconds` — if the p99 is at the 15-min request timeout, this is the cause.
2. **Router callback failure.** The router POSTed to `/api/dispatch/process` but the platform didn't acknowledge — check the platform's logs for errors in `dispatch_process_api`.
3. **Process crash.** The router panicked mid-dispatch. Check for `panic` messages in router logs.
4. **SQS message lost.** Rare, but the FIFO redelivery has unusual edge cases.

**Act:**

- (1) Talk to receiver about their endpoint hang.
- (2) Look at platform errors; usually a `dispatch_process_api` bug or a DB outage.
- (3) Restart the router; investigate the panic.
- (4) Accept and let recovery handle it. If it becomes frequent, audit SQS configuration (FIFO settings, dedup window).

### Symptom: HA failover took too long

**Watch:** failover window exceeded `FC_STANDBY_LOCK_TTL_SECONDS`.

**Diagnose:**

1. Was the standby actually healthy when the leader died? `/health` on the standby should show `STANDBY` not `STARTING` before the incident.
2. Is the standby running the same version as the leader was? Mixed versions during deploys can cause initialisation hiccups.
3. Does the standby have access to Redis? Check `fc_standby_*` metrics or logs.
4. Lock TTL — is it set absurdly high?

**Act:**

- Healthy standby + reasonable TTL → failover should be 30 s. Anything longer indicates a problem in the standby's initialisation (DB pool, migrations, etc.).
- If migrations ran on the new leader, that adds latency. Heavy migrations should be applied to the standby first (before failover) so the new leader doesn't pay that cost.

### Symptom: platform login failing for everyone

**Watch:** spike in `fc_platform_auth_token_validations_total{result="failure"}`.

**Diagnose:**

1. JWT signing keys missing or corrupt? Check `/health` for startup errors.
2. JWT issuer mismatch? `FC_JWT_ISSUER` must match `FC_EXTERNAL_BASE_URL`.
3. OIDC IDP unreachable? `fc_platform_oidc_callbacks_total{result="failure"}` for the affected provider.
4. JWKS cache stale (rotation occurred but cache hasn't refreshed)? Restart the platform — the cache rebuilds.

**Act:**

- Missing key → restore from backup, restart.
- Issuer mismatch → fix env var, restart.
- IDP outage → wait or fail over to the IDP's secondary if one exists.
- JWKS rotation issue → restart, then look at how rotation was performed (IDP-side change).

### Symptom: certificate / TLS errors

**Watch:** logs filled with `failed to fetch JWKS` or `TLS handshake failed`.

**Diagnose:**

- The platform makes outbound HTTPS calls to IDPs (JWKS), webhooks (dispatch), Redis (if TLS), Postgres (if TLS), Secrets Manager.
- Common causes: expired CA bundle in container, system clock drift, IDP rotated their cert.

**Act:**

- Update CA bundle (`apt update && apt install ca-certificates` or equivalent during image build).
- Sync clock (NTP).
- For IDP rotations: usually no action — `rustls`/`reqwest` follows the latest trust roots.

### Symptom: Postgres pool exhausted

**Watch:** `sqlx_pool_idle == 0`, requests slow/timing out.

**Diagnose:**

1. Long-running queries? `pg_stat_activity WHERE state='active'`. Look at `query_start`.
2. Connection leak? `pg_stat_database.numbackends` per app user climbing without correlated traffic.
3. Genuinely overloaded? Check `fc_platform` request rate and `sqlx_pool_acquire_duration_seconds` distribution.

**Act:**

- Long queries: identify, optimise, or `pg_terminate_backend(pid)` to free the connection while the underlying issue is fixed.
- Leaks: bug. Investigate; restart as immediate mitigation.
- Overload: raise pool size (code change today — `create_pool` takes `max_connections`), or scale horizontally.

---

## Backup and restore

### Routine backup verification

Once a quarter, restore the most recent backup to a non-production environment. Confirm the restore is bootable (platform starts, migrations idempotent-re-apply, you can log in). The most common backup-related failure is "backup was being taken but the restore procedure was never tested".

### Restore from scratch

If primary Postgres is lost:

1. Restore from the latest snapshot/backup to a new instance.
2. Confirm `_schema_migrations` table matches what current code expects (no missing migrations).
3. Bring up the platform pointing at the restored instance. Migrations apply if any are pending.
4. Verify by logging in and checking a sample of clients, subscriptions, recent events.

What's not in the database:

- JWT signing keys — restore from secret store.
- `FLOWCATALYST_APP_KEY` — restore from secret store. **Mandatory**; without it, `oauth_clients.encrypted_client_secret` is unreadable.
- SQS in-flight messages — accept loss or rely on stale-recovery.
- Redis state — re-acquired on first leader election.

So a working restore needs: PG backup + secrets backup (JWT + app key). Test this together, not just the DB part.

### Recovering deleted data

`aud_logs` is the long-retention audit trail. Every UoW write goes through it. If a row is deleted accidentally, `aud_logs` will have a record of the delete (and the prior state for an update). The audit log isn't a transactional rewind tool — you'd reconstruct state manually from the audit history — but it's better than nothing for forensics.

Events (`msg_events`) and dispatch jobs (`msg_dispatch_jobs`) are partitioned with 90-day retention. Beyond 90 days the data is gone. If you need longer retention, raise `retention_days` (code change in `bin/fc-server/src/main.rs::spawn_stream_processor`) or set up periodic exports to archival storage.

---

## Capacity planning quick reference

| Resource | Scale-up signal | Action |
|---|---|---|
| Postgres CPU | Sustained > 70% | Larger instance class |
| Postgres connections | `sqlx_pool_idle` consistently 0 | Raise `max_connections` (parameter group), raise pool size (code) |
| Postgres disk | Approaching 80% | Enable storage autoscale (RDS), or shorten `retention_days` |
| Router CPU | High and pool concurrency at cap | Raise concurrency or split traffic across more pools |
| Router memory | `fc_router_in_pipeline_count` growing without bound | Callback leak — investigate; restart as mitigation |
| Scheduler | `fc_scheduler_pending_jobs` growing | Raise `FC_SCHEDULER_MAX_CONCURRENT_GROUPS` or `FC_SCHEDULER_BATCH_SIZE` |
| Stream processor | `fc_stream_processing_lag_seconds` growing | Raise `FC_STREAM_*_BATCH_SIZE` |
| Outbox processor (sidecar) | Application's `outbox_messages` backlog growing | Raise `FC_API_BATCH_SIZE` or `FC_MAX_CONCURRENT_GROUPS` |

---

## Code references

- Per-binary entry points: `bin/*/src/main.rs`.
- Combined health handler: `bin/fc-server/src/main.rs::combined_health_handler`.
- Router monitoring API: `crates/fc-router/src/api/`.
- Metrics: `crates/fc-router/src/metrics.rs`, `crates/fc-platform/src/shared/monitoring_api.rs`.
- Audit log: `crates/fc-platform/src/audit/`.
