# Integral subscription records vs FlowCatalyst dispatch jobs (2026-10-19)

Read-only comparison. Integral's `integral-service-v2` "subscription records" operator UI is the model;
our dispatch jobs UI/API is the subject. Nothing in the Integral repo was edited.

Paths: `IS2` = `/Users/andrewgraaff/Developer/inhance/InhanceMono/packages_root/packages/integral-service-v2`.
`FE` = `IS2/frontend/src/pages/integration/subscription-records`. `CTL` =
`IS2/src/Admin/Http/Controllers/AdminSubscriptionRecordController.php`. `REPO` =
`IS2/src/Admin/Repositories/AdminSubscriptionRecordRepository.php`.

What I could not determine is called out in section 6.

---

## 1. What the Integral page can do

Integral stores subscription records in one MySQL table, `integration_poll_records`, range-partitioned
monthly on `partition_key` (YYYYMM), PK `(id, partition_key)`, `id` a ULID
(`IS2/database/migrations/core/2024_09_09_120000_igs_consolidate1.php`, table DDL from the `CREATE TABLE
integration_poll_records` statement). It is the equivalent of one table that is both our write table and
our read projection: there is no CQRS split, and the UI reads the live row.

### 1.1 Navigation and permissions

`frontend/src/config/navigation.ts:55-60`: one group "Subscription Records" with four entries, all gated by
`integral:sr:view`: Errors, Blocked, Enquiry, Event Type Enquiry. The router
(`frontend/src/router/index.ts:384-410`) also has the detail page `integration/subscription-records/:id`.
Every page carries a `SelectButton` "mode bar" (Errors / Blocked / Enquiry / Event Type) so an operator
hops between lenses with the filter state kept per page (`usePersistedFilters`, URL + local storage).

`SubscriptionRecordIssuesPage.vue` exists (627 lines) but is **not referenced by the router or any other
file** (`grep SubscriptionRecordIssuesPage` finds only its own `name:`). It is the older, generic
paginated list that fronts `GET /integration/subscription-records` with `view=errors|blocked`. Treat it as
dead code; the four mode pages replaced it. The `index` endpoint it uses is still routed (`routes/api.php:463`).

Permissions (all checked in `CTL`, not just in the UI):

| Action | Permission | Where |
|---|---|---|
| Any read (list, detail, errors, measurements, facets) | `integral:sr:view` | every method in `CTL` |
| Batch change status | `integral:sr:edit` or `integral:sr:admin` | `CTL:156-175` |
| Batch resend, Process now | `integral:sr:edit`, `integral:sr:resend` or `integral:sr:admin` | `CTL:177-206` |
| Edit payload, Edit by-latest | `integral:sr:edit` only | `CTL:346-388` |

The UI hides the buttons with the same `authStore.hasAnyPermission(...)` checks (for example
`ErrorManagementPage.vue:121-122`). Tenant scoping: a non-`@inhancesc.com` user is silently pinned to the
current tenant (`CTL:resolveTenantId/resolveOptionalTenantId`, bottom of file); an Integral staff user may
pass `tenant_id`.

### 1.2 Endpoints (routes/api.php:463-482)

| Method + path | Controller method | Purpose |
|---|---|---|
| GET `/integration/subscription-records` | `index` | Generic offset-paged list, `view=all\|blocked\|errors` |
| GET `.../status-counts` | `statusCounts` | Dashboard counts, cached 5 min |
| GET `.../filter-options` | `filterOptions` | Facet values (statuses, event types, subscribers, tenants) |
| GET `.../tenants`, `.../subscription-names`, `.../event-type-names` | | Dropdown sources, cached 5 min |
| GET `.../errors` | `errors` | Error lens |
| GET `.../blocked` | `blocked` | "Potentially blocked" lens |
| GET `.../enquiry` | `enquiry` | Four search modes, keyset paged |
| GET `.../event-type-enquiry` and `/count` | | Event-type lens + explicit count |
| GET `.../{id}`, `.../{id}/errors/{errorId}`, `.../{id}/measurements`, `.../{id}/related-blocking` | | Detail pieces |
| PATCH `.../{id}/payload`, `.../{id}/by-latest` | | Single-record edits |
| POST `.../batch-change-status`, `.../batch-resend`, `.../{id}/process-now` | | Actions |

### 1.3 Status enum

`QUEUED, PENDING, IN_PROGRESS, COMPLETED, ERROR, IGNORE, PAUSED`. `IGNORE` is the "operator skipped it"
terminal; `PAUSED` is a hold. Fixed list, hard-coded in `getFilterOptions` (`REPO:230`) and in the batch
validator (`CTL:164`: `in:QUEUED,PENDING,IN_PROGRESS,COMPLETED,ERROR,IGNORE,PAUSED`). The change-status
action accepts **any** status to **any** status; there is no transition table.

### 1.4 Per-page feature inventory

**Common to all four lens pages.** Collapsible filter panel (persisted), row selection with a toolbar
that appears when rows are ticked, a "Created At (SAST)" column, an eye link to the detail page, SAST
date pickers (converted to UTC with `sastWallToUtcSqlString`), URL-synced filters. Every list is a
bounded array; the server returns `truncated: true` when `count >= limit` and the page shows
"Results limited to N records. Some results are omitted. Narrow your filters."
(e.g. `PotentiallyBlockedPage.vue:92-95`). Row limit options 50 / 200 / 500 / 1000, default 200
(`api/subscription-records.ts`, `SUBSCRIPTION_RECORD_LIMIT_OPTIONS`). The server default when the param is
absent is 1001 (`CTL:errors/blocked`: `(int) $request->query('limit','1001')`), and the `limit` is
**not clamped server-side** on `errors`/`blocked` (a caller can send `limit=1000000`). The summary columns
are a fixed select list, no payloads (`REPO:32-37`).

#### Errors (ErrorManagementPage.vue, GET `/errors`)

Filters, in the order the page shows them, and what each does in SQL (`REPO:291-314`, `listErrors`):

| Filter (UI) | Param | SQL | Served by |
|---|---|---|---|
| Tenant (Integral staff only; All Tenants allowed here) | `tenant_id` | `tenant_id = ?` (omitted if null) | leading column of nearly every index |
| Status (default `ERROR`; also `IGNORE`, etc. via SelectButton) | `status` (CSV) | `status IN (...)` | `idx_integration_poll_records_tenant_id_status (tenant_id, status)` when tenant is set; `idx_integration_poll_records_status (status)` otherwise |
| From / To (SAST) | `created_at_from/to` | `createdAt >= ?`, `createdAt <= ?`, plus `partition_key >= floor(from)` | partition pruning (`REPO:596-610`); no composite with status+createdAt |
| Subscription Name (server) | `subscription_names` (CSV) | `subscriptionName IN (...)` | residual filter |
| Order | `sort_order` | `ORDER BY createdAt asc\|desc` (default desc in UI, asc server default) | filesort over the status range, bounded by LIMIT |
| Row limit | `limit` | `LIMIT n` | |
| Event Type, Message Group | in-memory | none (filtered in the browser over the loaded rows) | |

The page default window is **none** (`createdAtFrom = null`, `ErrorManagementPage.vue` ref init); the
bound is the status filter itself, because `ERROR` is a small set. Columns: ID, Tenant (staff), Subscription
Name, Description, Event Type (`event_type` minus the `com.inhance.` prefix, else `domain.code`), Search Key,
Message Group, Error (60-char truncation with tooltip of `errorDescription`), Status tag, Created At.
Actions on selection: **Resend** (confirm dialog "Resend N selected records?") and **Set to Ignore**
(confirm dialog), both on the ticked ids. No "act on everything this filter matches".

#### Blocked / "Potentially blocked" (PotentiallyBlockedPage.vue, GET `/blocked`)

Definition (`REPO:343-361`, `listPotentiallyBlocked`):

```sql
SELECT <summary columns> FROM integration_poll_records
 WHERE status IN ('PENDING','QUEUED')
   AND createdAt < NOW() - INTERVAL 5 MINUTE
   [AND tenant_id = ?] [AND createdAt >= ? AND partition_key >= floor(?)] [AND createdAt <= ?]
 ORDER BY createdAt <asc|desc> LIMIT n
```

So "potentially blocked" is **not** the `shouldBlock` column or a group-blocking computation. It is "still
PENDING/QUEUED more than five minutes after creation". The name says "potentially" because the query
cannot tell a record that is really held by a blocker from one that is merely backlogged. The
authoritative blocking model lives elsewhere: a `shouldBlock=true` record is processable only if no other
record with the same `subscriptionName`+`messageGroup` is `IN_PROGRESS` (`docs/subscription-records/05-business-rules.md:234-330`, the `scopeBlocked`/`scopeNotBlocked` examples).
The detail page's "Related Blocking" tab (1.4, below) is where the real blockers are shown. No window
default; the 5-minute floor plus the status range is the bound. Filters: Tenant, From/To, Order, Row Limit;
in-memory Event Type / Message Group / Subscription Name. Actions: **Change Status** (dialog with the 7
statuses, any to any) and **Resend**.

Separately, the generic list has its own meaning of blocked: `view=blocked` is
`status IN ('ERROR','PENDING') AND shouldBlock = true` (`REPO:55-57`), and `view=errors` is `status = 'ERROR'`.
Only the unrouted Issues page uses it.

#### Enquiry (EnquiryPage.vue, GET `/enquiry`) - the search page

Four modes chosen by a SelectButton (`type=message_group|search_key|record_id|event_id`). A **Search
button** runs the query; nothing loads on page open (`hasSearched` gate).

| Mode | Required | SQL (`REPO:423-525`) | Index |
|---|---|---|---|
| `message_group` | tenant + prefix (+ date range, default last 3 days SAST, `EnquiryPage.vue:298`) | `tenant_id = ? AND messageGroup LIKE 'prefix%' [AND createdAt range AND partition_key >= floor] [AND (event_type NOT IN (...) OR event_type IS NULL)] [AND (createdAt,id) < (?,?)] ORDER BY createdAt, id LIMIT 201` | `idx_integration_poll_records_tenant_id_messagegroup (tenant_id, messageGroup)` (see 6.1 on the doc/DDL mismatch) |
| `search_key` | tenant + prefix (+ date range) | same shape on `searchKey` | `idx_integration_poll_records_tenant_id_searchkey (tenant_id, searchKey)` |
| `record_id` | id | `find($id)` by primary key | PK `(id, partition_key)`, so all partitions probed by `id` prefix |
| `event_id` | event id | `eventId = ? ORDER BY createdAt DESC LIMIT 101` | `idx_sr_event_id (eventId)` |

Details that matter:

- **Prefix, not contains.** `wherePrefix` (`REPO:481-490`) escapes `%`, `_`, `\` with `addcslashes`, only ever appends one
  trailing `%` (a leading wildcard is "impossible by construction"), and terms shorter than 3 characters
  (`MIN_PREFIX_LENGTH`, `REPO:42`) fall back to `=`. The reason is stated in the comment at `REPO:39-41`:
  a short prefix covers too much of the index range to bound the scan.
- **Keyset pagination** (the only paged lens). Page size 200 (`ENQUIRY_PAGE_SIZE`, `CTL:31`); the repository
  fetches `limit+1` and `has_more` is `count > pageSize`, so no count query ever runs (`CTL:259-344`). The cursor
  is `(createdAt, id)` using the raw DB value at microsecond precision (`getRawOriginal('createdAt')`,
  commented "a formatted/serialized value would round-trip lossily and skip rows"), applied as
  `(createdAt, id) < (?, ?)` or `>` for ascending (`REPO:464-467`). Both cursor params or neither, else 422;
  an unparseable time is 422 (`CTL:274-281`). A "Load More" button appends pages and keeps the **snapshot of the
  first search's params** so Load More continues that query, not the edited form (`EnquiryPage.vue`, `lastSearchParams`).
- **Server-side event-type exclusion.** `excluded_event_types` (CSV, max 100, else 422, `CTL:33,270`) applies
  `event_type NOT IN (...) OR event_type IS NULL` (NULL kept deliberately) as a residual filter on the
  index-bounded rows. Built because vehicle telemetry (one event per minute) drowned message groups before
  the 501-row limit (`docs/change/sr-enquiry-prefix-eventtype-pagination/requirements.md`, "Problem" item 2).
  The exclusion dropdown is fed by `GET /event-type-names` (cached 5 min, `REPO:573-589`).
- **In-memory facets** over the loaded rows: Event Type, Subscription Name, Status (the page labels them
  "(in-memory)"); a composable (`useSubscriptionRecordFacets.ts`) derives the options from the loaded rows and drops
  stale selections after a reload so no ghost chips show.
- Tenant is required in the UI (`tenantOptionsRequired`, no "All"). Server-side the enquiry does
  `where('tenant_id', $tenantId)` with a possibly-null id for staff (`REPO:425`); the event-type endpoints
  422 on a missing tenant (`CTL:437-440`), the enquiry does not. I did not test what the null does.
- Actions on selected rows: **Change Status** and **Resend**.

#### Event Type Enquiry (EventTypeEnquiryPage.vue, GET `/event-type-enquiry`, `/count`)

Tenant **required** (422 otherwise, `CTL:437-440`). Filters: Event Type (multi-select from `event-type-names`),
Status (multi, default all of QUEUED/PENDING/IN_PROGRESS/COMPLETED/ERROR/IGNORE, `CTL:buildEventTypeFilter`),
From/To (default 3 days, `EventTypeEnquiryPage.vue:258`), Order, Row Limit. SQL (`REPO:551-568`):

```sql
WHERE tenant_id = ? [AND event_type IN (...)] [AND status IN (...)] [AND createdAt >= ? AND partition_key >= floor(?)] [AND createdAt <= ?]
ORDER BY createdAt LIMIT n
```

A separate **Count button** (`/event-type-enquiry/count`) runs `COUNT(*)` of the same WHERE "ignoring the row limit".
It is explicit and operator-initiated, never automatic, and the result opens in a dialog. Action: **Resend** on selection.

#### Detail page (SubscriptionRecordDetailPage.vue, GET `/{id}`)

Header buttons: Change Status, Edit Payload, Edit By Latest (all `integral:sr:edit` /admin), Resend, Process now.
Five tabs:

1. **General**: ID, Event ID, Tracing ID, Subscription + code, Event Type, Application Identity, Search Key,
   Message Group, Status, Runner, Attempts / Max Attempts, Should Block, Queue, Rate Limit type + value, Process
   Pool and its throttle, Invokable, Transformer, Created / Polled At / Not Before / By Latest (SAST), Alert; then
   code blocks for Payload, Payload Out, Error Dump, Config Data (`GetSubscriptionRecordUseCase.php`).
2. **Errors**: preview of the **latest 10** `integration_errors` rows (id, message, time) plus a total count
   (`ERRORS_PREVIEW_LIMIT = 10`); expanding one fetches `/errors/{errorId}` for the dump, payload and payload_out
   of **that attempt**. So the failure history keeps the payload as it was at each failure.
3. **Measurements**: HTTP call timings (type, duration, path, method, status, started/ended), server-paginated
   `simplePaginate` 50 per page, max 200, ordered `id DESC` "so it stays on the relation_id index"
   (`REPO:145-160`). Loaded on demand. Comment says some records have 60k+ measurements, hence no eager loads.
4. **Audit Log**: latest 50 `integral_audit_logs` rows (user, email, changes JSON, time).
5. **Related Blocking**: GET `/{id}/related-blocking`, `REPO:384-399`: other records in the same
   `subscriptionName` + `messageGroup` + tenant with `status IN ('ERROR','IN_PROGRESS')`, oldest first, limit 100.
   This is the "what is actually holding my group" view, and it is the one place the real blocking relation is shown.

Process now (`ProcessSubscriptionRecordNowUseCase.php`): runs the record **in the HTTP request** as the record's
tenant, bypassing queue, message router and group blocking; a failure goes straight to `ERROR` with no retries; 409
if the record is already in progress (a claim). The confirm text says exactly that
(`SubscriptionRecordDetailPage.vue:691-693`). It is audited (`action: process-now`).

Resend (`REPO:180-195`, shared with `SubscriptionRecordResendHelper.php`): `UPDATE ... SET status='PENDING', attempts=0,
errorDescription=NULL, errorDump=NULL, hasError=0, not_before=NOW() WHERE id IN (...)`. `not_before` reset is
tested ("makes a resent record due now, not when its last backoff said", `tests/Feature/Admin/BatchResendSubscriptionRecordsTest.php:52`).
Note the helper's comment (`SubscriptionRecordResendHelper.php`) says admin resend "does NOT reset ... max_attempts" and
the persisted `max_attempts=127` from TRANSIENT retries survives.

### 1.5 Bulk actions, caps, safety and audit

- **No cap and no preview.** `batch-resend` and `batch-change-status` take `ids: array|min:1`, with **no max**
  (`CTL:158-163,179-182`), no count confirmation and no "everything the filter matches" mode. The only
  safety is the confirm dialog ("Resend N selected records?") and that the id list can only contain rows
  that were loaded into the browser, which is itself capped by the row limit (<= 1000, or 200 per Load More on
  the paged enquiry). The blast radius is therefore bounded by the UI, not by the API.
- **Audit**: one `integral_audit_logs` row per id (`BatchResendSubscriptionRecordsUseCase.php`): after the UPDATE,
  a loop does `findById` + `AuditLogService::log(...)` per record (an N+1: 2 queries per id, swallowing exceptions).
  Process now audits `action: process-now` + resulting status. **Edit payload and edit by-latest are not audited**
  (`CTL:346-388` save the model and return; no `AuditLogService` call). The audit rows show on the detail page.
- **Optimistic reporting**: batch actions return `{message, count}` of rows updated (MySQL affected rows).
- **Confirmation patterns**: PrimeVue `confirm.require` for resend, ignore and process-now; a dialog with a status
  Select for change-status; the truncation banner; the Count button.

### 1.6 Operational counters

`GET /status-counts` (`REPO:200-220`): `COUNT(*) GROUP BY status WHERE tenant_id=? AND status IN ('QUEUED','PENDING','IN_PROGRESS','ERROR')`,
cached 5 minutes per tenant (`Cache::remember("sr_status_counts:{tenant}", 5 min)`). The comment at `REPO:27-29` is the policy:
"COMPLETED in particular can be in the millions per tenant and is not actionable, so it is never counted."
I found the API method and the TS wrapper (`subscriptionRecordsApi.statusCounts`) but did not find a page
that calls it in the four lens pages; it presumably feeds a dashboard widget elsewhere (not determined).

Facet caches (`REPO:225-286`): event-type list cached 1 hour (`distinct domain, code WHERE tenant_id`), subscribers
cached 30 min and **bounded to the last 3 days** (`createdAt >= now()-3d`), tenants 5 min, subscription names 5 min.

---

## 2. How the design achieves index coverage

The pattern is a deliberate narrowing of what the UI will ask, enforced in three places: the UI shape, the
controller, and the repository. These are the rules I can read out of the code.

1. **One leading equality, always the tenant.** Almost every index starts with `tenant_id` (list in 2.3).
   Non-staff users are pinned to their tenant server-side; staff must choose one for the searching pages
   (Enquiry, Event Type use `tenantOptionsRequired`, no "All Tenants"; the Event Type endpoints 422 without one).
2. **A bounded key on top of that**: either a status (small, actionable set: Errors, Blocked), or a
   prefix of an indexed free-text column (messageGroup / searchKey), or an event-type list, or a primary-key /
   eventId point lookup. The Enquiry page cannot be searched without one of these. There is no "show me everything
   for the tenant newest first" page.
3. **A time window that also prunes partitions.** `applyCreatedAtRange` (`REPO:596-610`) adds
   `partition_key >= Ym(from) - 1 month` whenever `from` is given. The reason for the one-month widening is documented in
   `SubscriptionRecordPartitionHelper.php`: `partition_key` is stamped from the insert month while `createdAt` can be
   backdated (resent records), so only a **lower** bound is safe. The Enquiry and Event Type pages default `from` to
   "3 days ago" (`sastDaysAgoStart(3)`); Errors and Blocked default to none and rely on rule 2 instead.
4. **Prefix LIKE only, escaped, with a minimum length.** (1.4, Enquiry.) This is what makes a text search
   index-usable. Contains-search is not offered.
5. **Keyset, never offset, on the one large unbounded result.** The paged enquiry uses `(createdAt, id)` cursors
   with `limit+1` instead of counts or `OFFSET`. Every other lens is a single bounded page with a `truncated` flag.
   The old offset list (`paginate`, `REPO:44-114`) survives only behind the unrouted Issues page and even has a
   workaround for deep limits (`REPO:101-105`: pluck N newest ids then `WHERE id IN (...)`).
6. **No count unless asked.** Paged lens uses `limit+1`; the only `COUNT(*)` is the explicit Count button (event-type)
   and the cached status-counts, which excludes COMPLETED/IGNORE/PAUSED so it never touches the largest group.
7. **Residual filters stay residual.** Event-type exclusion, subscription names, and the in-memory facets do not add an
   index; they filter rows the index already bounded. When a group is mostly telemetry the doc names the escalation
   index and **declines it** (`requirements.md`, "Out of scope": "deliberately not added now (the table already
   carries ~24 secondary indexes paid on every insert)").
8. **Facet queries are cached and time-bounded** (1.6) instead of `DISTINCT` over the table on every page load.
9. **Detail sub-resources are bounded and lazy**: 10 errors, 50 audit rows, measurements paged 50/200, relations
   fetched with explicit limits; the comment at `REPO:118-120` forbids eager loads.

### 2.1 Queries deliberately not offered (no supporting index), as visible in the code

- Free-text search on `description`, `errorDescription`, `payload`: no filter exists. (The detail page shows them.)
- "Contains" on message group or search key.
- Tenant-wide, newest-first browsing of all statuses without a key.
- Filter by `subscriptionName` alone across a long window: it is only a residual filter on Errors, and
  `subscriptionName` has a single-column index but the Enquiry never offers it as a primary key.
- Count of COMPLETED, or any count that is not operator-initiated.
- Cross-tenant message-group or search-key searches (tenant is the leading column).

### 2.2 What the design does not protect (honest limits)

- `errors`/`blocked` `limit` is user-supplied and not clamped server-side (`CTL:errors`, `blocked`).
- `blocked` with no window and a huge PENDING backlog scans the `(tenant_id, status)` range for that status; the
  `createdAt < now-5min` and ORDER BY are then filter + filesort over what the index returned. Bounded by the
  `limit` only after the sort. I did not measure it.
- The Errors lens has no `(tenant_id, status, createdAt)` index; it relies on status being small.
- The `batch-*` endpoints have no id-count cap.

### 2.3 Index list on `integration_poll_records` (consolidated DDL)

From the `CREATE TABLE` at `2024_09_09_120000_igs_consolidate1.php` (the only migration in this package that
defines the table's indexes; `2026_03_28_000003` just adds `alert_id`):

Single-column: `tracingId`, `subscriptionName`, `domain`, `code`, `shouldBlock`, `createdAt`, `subscriptionCode`, `not_before`,
`event_type`, `polled_at`, `messageGroup`, `status`, `eventId (idx_sr_event_id)`.
Composite: `(tenant_id, tenant_code)`, `(tenant_id, domain, code)`, `(tenant_id, createdAt)`, `(domain, code)`,
`(tenant_id, messageGroup, subscriptionName, shouldBlock)`, `(tenant_id, messageGroup)`, `(tenant_id, searchKey)`,
`(tenant_id, status)`, `(domain, code, tenant_id)`, `(domain, code, tenant_id, createdAt)`,
`(status, domain, code, not_before, createdAt, sequence)` (the poller's "next pending"),
`(tenant_id, shouldBlock, status, createdAt, messageGroup, subscriptionName)` (`idx_subquery_blocking`, the blocking subquery),
`(tenant_id, queue, status, not_before, createdAt, sequence)` (`idx_main_pending_queue`, the main pending queue),
`(createdAt, tenant_code, queue, pool_code, subscriptionName, rate, rate_type)`. That is roughly 28 secondary indexes
(the requirements doc says "~24"). Some of these serve the dispatch engine, not the UI.

Related tables: `integration_errors (record_id)` index (`2025_09_01_001_integration_errors_index.php`); `integral_audit_logs
(model_type, model_id)` and `(user_id)` (`2025_02_03_000001_create_integral_audit_logs_table.php`).

---

## 3. What FlowCatalyst dispatch jobs have today

### 3.1 Storage, CQRS split and indexes

Two partitioned tables, both `PARTITION BY RANGE (created_at)` monthly, PK `(id, created_at)`, `id` a 13-char TSID
(`internal/migrate/sql/019_partition_messaging_tables.sql`):

- **Write table `msg_dispatch_jobs`**: the engine's table. Rich query indexes were deliberately removed (migration 015,
  `internal/migrate/sql/015_dispatch_jobs_write_indexes.sql:11-18`). After migrations 045, 067 and 068 the indexes are
  `idx_dispatch_jobs_status_group (status, message_group, sequence, created_at, id)` (067, ordinary) and the projector's
  partial `idx_msg_dispatch_jobs_dirty (created_at) WHERE projected_at IS NULL OR updated_at > projected_at` (045),
  plus the PK. The retired queue table (066/067/068) is documented in `docs/db-index-plan.md:62-69`.
- **Read projection `msg_dispatch_jobs_read`** (denormalised: adds `application`, `subdomain`, `aggregate`, `is_completed`,
  `is_terminal`; omits payload): **the list, filter counts, filter-options and by-event reads all use it**
  (`internal/platform/dispatchjob/repository.go:276-296,364-390`). Kept current by the stream projector, so reads are
  **eventually consistent** with the write table.

Indexes on `msg_dispatch_jobs_read` as I can reconstruct them from the migrations (`004`, `019:238-243`, `036`, `037:32-33`):

| Index | Source |
|---|---|
| PK `(id, created_at)` | 019 |
| `application` | 019:240 |
| `subscription_id` | 019:241 |
| `message_group` | 019:242 |
| `created_at` | 019:243 |
| `event_id` | 036 |
| `dispatch_pool_id` | 036 |
| `code` | 036 |
| `(client_id, created_at DESC)` | 036 (singles on `client_id` and `status` dropped in 037:32-33) |
| `(status, created_at DESC)` | 036 |

Nine secondary indexes. **I found no index on `subdomain`, `aggregate`, `source`, `kind` or `scheduled_for`** in any migration;
`FilterParams`' comment says the facets are "backed by their own indexes" (`repository.go:44-48`) and `where()` says the same
(`repository.go:157`), which is true for `application` but not, as far as the migrations show, for subdomain/aggregate. I could
not inspect a live database, so this is from the DDL only.

Retention is by dropping monthly partitions: `FC_STREAM_PARTITION_RETENTION_DAYS` default 90 (`docs/environment-variables.md:241`).
Write amplification is the stated reason to stay lean: `ingest-throughput-plan` notes the read table's 10 indexes are
re-upserted on every status change, "biggest write amplification" (memory note, `ingest-throughput-plan.md`; target
~20k msg/s aggregate).

Attempts live in `msg_dispatch_job_attempts` (PK `(id, created_at)`, unique `(dispatch_job_id, attempt_number)` per 018).

### 3.2 API (internal/platform/dispatchjob/api/api.go)

Registered at `api.go:75-97`, mirrored under `/bff/dispatch-jobs` (`api.go:112-128`):

| Route | Purpose | Permission gate |
|---|---|---|
| GET `/api/dispatch-jobs` | list, `listInput` at `api.go:183-190` | `CanReadDispatchJobs` + application confinement (F20) + SQL tenant scoping |
| GET `/api/dispatch-jobs/list-raw`, `/{id}/raw` | same data, raw view | `...:dispatch-job:view-raw` |
| GET `/api/dispatch-jobs/filter-options` | statuses, codes, clientIds, dispatchPoolIds, subscriptionIds, kinds | `...:dispatch-job:view` |
| GET `/api/dispatch-jobs/event/{eventId}` (+`by-event` alias) | jobs spawned by an event | view |
| GET `/api/dispatch-jobs/{id}` | detail | view |
| GET `/api/dispatch-jobs/{id}/attempts` | attempt history | view |
| POST `/api/dispatch-jobs/requeue` | reset ids to PENDING | **view** perm (`api.go:486-500`) |
| POST `.../requeue-by-filter/preview` | counts + `byStatus` + `eligibleCount` | view |
| POST `.../requeue-by-filter` | reset up to `min(limit, confirmCount)` | view |
| POST `/{id}/cancel` | FAILED to CANCELLED | view |
| POST `/{id}/complete` | FAILED to COMPLETED (delivered out of band) | view |
| PUT `/{id}/payload` | edit payload of PENDING or FAILED job | view |
| POST `/{id}/sign` | build the delivery (signing account, headers, real signature) without sending | view-raw |

Notable: **one permission** (`dispatch-job:view`) gates reads and every mutating action (`api.go:70`, used by
requeue/cancel/complete/edit). There is no separate edit/resend permission as Integral has.

Filter parameters (`DispatchJobFilter`, `api.go:165-181`): `status`, `clientId`, `dispatchPoolId`, `subscriptionId`,
`code`, `since`, `until` (RFC3339), `messageGroup` (exact), `clientIds`, `statuses`, `applications`, `subdomains`,
`aggregates`, `codes` (CSV), `source`. List extras: `limit`, `size` (default 100 per the repo, "default 50, max 1000" in the doc
string at `api.go:188`, `repository.go:287-292`: clamp 1000, default 100), `offset`, `sort=createdAt.asc|desc`.
The same struct is the body of requeue-by-filter and its preview, via one `toParams` conversion, so "what the list shows" and
"what a bulk requeue touches" cannot diverge (`api.go:160-165`). `since`/`until` unparseable: list ignores it (non-strict),
bulk requeue returns 400 (strict) (`api.go:207-246`).

Status enum (`internal/common/dispatch_status.go`): `PENDING, QUEUED, PROCESSING, COMPLETED, FAILED, CANCELLED, EXPIRED`;
legacy `IN_PROGRESS` (= PROCESSING) and `ERROR` (= FAILED) parse as aliases. `FAILED`/`ERROR` hold their message group.

Operations (envelope use cases, each audited): `operations/cancel.go`, `complete.go`, `edit_payload.go`, `resend.go`,
`resend_by_filter.go`. Safeguards in requeue-by-filter (`resend_by_filter.go`):

- Preview: `SELECT status, client_id, count(*) FROM msg_dispatch_jobs_read WHERE <filter> GROUP BY status, client_id`
  (`repository.go:178-185`), the tenant rule applied per group: `eligibleCount`.
- Confirm: request carries `confirmCount`; if more are eligible now, nothing happens and the answer is 409
  `FILTER_MATCH_CHANGED` with the new counts (`resend_by_filter.go`, `now.Eligible > confirmCount`).
- Cap: `limit` default and maximum 10,000 (`ResendByFilterMaxLimit`), batches of 500 ids, one transaction each,
  walking ids by keyset `id > last` so a job that stops matching cannot shift the next page.
- Partial failure is reported (`REQUEUE_INTERRUPTED`, with the committed count); `Remaining` tells the caller to run again.
- Audit: one rollup `DispatchJobsResent` event per batch (not one per job) (`resend.go`, `operations/events.go:24-27`).
- Caveat: `ResendDispatchJobs` resets a job **in any status**, so a filter with no status condition requeues
  COMPLETED, QUEUED and PROCESSING jobs too (`resend_by_filter.go` `MatchSummary.Eligible` comment: "ResendDispatchJobs resets a job
  in any status, so status does not make a job ineligible"). The UI defends against this by presets, not the API.

Edit payload (`edit_payload.go`): PENDING or FAILED/ERROR only (409 otherwise), 256 KiB, UTF-8, valid JSON when JSON
content type; status and attempts unchanged; the audit event records sizes and digests, never the payload
(`AuditMaskedFields`). This is stricter than Integral's, which has no status rule and no audit.

### 3.3 Vue console (frontend/src/pages/dispatch-jobs/)

`DispatchJobListPage.vue` (851 lines):

- Filters (all server-side, URL-synced): Client, Application, Subdomain, Aggregate, Code (cascading, `codeLevels`), Status,
  Message group (exact), Created from / to (whole days, viewer timezone), "source" search box (`filters.search` is sent as `source`,
  `DispatchJobListPage.vue` `buildParams`). **No default window**: `from`/`to` initial value is empty.
- Sort: `createdAt` only (server-side, `onSortChange`). Page size 50/100/200/500/1000, default 200; one page, no Load More
  (the banner at the bottom says "size limit reached - narrow filters or increase size"). The API has `offset` but the Vue
  page does not use it; the keyset cursor exists only inside the templ UI (3.4).
- Columns: Job ID, Descriptor, Code (tiered), Client (identifier), Group (clickable to filter by group), Status tag, Additional
  data (metadata preview with tooltip), Created, Actions.
- Selection: checkbox column with a Gmail-style banner "Select all N matching jobs", where N comes from the preview endpoint,
  called **automatically** when the whole page is selected (`DispatchJobListPage.vue` watch on `bannerState`).
- Actions: Requeue selected (ids), **Requeue all matching** (opens `RequeueByFilterDialog` with filter summary, preview counts by status,
  and a confirm), "Requeue everything in error" preset (clears filters, sets `statuses=[FAILED, ERROR]`, opens the dialog).
- Drawer (`DispatchJobDetailDrawer.vue`): job facts (group, event id, attempts N / max, completed), payload with copy and
  **Edit payload** (`PayloadEditDialog.vue`, only for editable statuses), attempts list newest first with response code, error type,
  request headers and the response (what was sent vs what came back), **Sign** (needs view-raw) to build and show the delivery
  with a verify command, **Requeue**.
- **Cancel and Complete have no UI**: `grep -i cancel|complete` in `frontend/src/pages/dispatch-jobs/*.vue` and
  `frontend/src/api/dispatch-jobs.ts` finds only the "Completed" date label. They exist only as API routes.

### 3.4 Go-served templ UI (internal/ui/pages/dispatch_jobs.go/.templ)

Same capability set, server-rendered with Datastar SSE patches: facets, group, created-at days (`From, To string // YYYY-MM-DD, UTC days`,
`dispatch_jobs.go:131-147`), sizes `djSizes`, **keyset paging** ("Older", `ListCursor`, `repository.go:105-126`,
`dispatch_jobs.go:313-314`), row requeue, requeue selected, requeue by filter dialog, drawer with attempts, sign, edit payload
(`dispatch_jobs.go:891-922`). Gate: `platform:messaging:dispatch-job:view` (and view-raw for sign) (`dispatch_jobs.go:74-75`).
No cancel/complete here either.

### 3.5 Blocked-group and lifecycle concepts

- A job **holds its message group** when it is `FAILED/ERROR`, or `PENDING` with a future `scheduled_for` (backoff)
  (`internal/platform/dispatchjob/group_hold.go:1-30`). `QUEUED`/`PROCESSING` deliberately do not hold. Hold-back is evaluated
  at claim time and delivery time from the **write table** through `idx_dispatch_jobs_status_group`.
- A reaper (`reaper.go`, every 2 minutes) resets QUEUED/PROCESSING siblings stranded behind a FAILED head to PENDING.
- The backlog gauge counts PENDING "up to 100,001 matches" (`docs/db-index-plan.md:67`, `internal/platform/scheduler/backlog.go`):
  a bounded count already exists as a pattern.
- No operator page shows held groups, stuck jobs, or status counters for dispatch jobs. The only way to see a held group is to
  filter by `messageGroup` and read the statuses.

### 3.6 Query behaviour of the list today

`FindWithFilters` (`repository.go:276-318`): `SELECT <slim columns> FROM msg_dispatch_jobs_read WHERE <filters> ORDER BY created_at DESC, id DESC LIMIT n [OFFSET m]`
with an optional keyset condition (`created_at <= $ AND (created_at < $ OR id < $)`, the plain range first "so the created_at index and
partition pruning apply", `repository.go:115-127`). `since`/`until` are plain `created_at` bounds, so they prune partitions the same way
Integral's `partition_key` floor does, with no widening needed because the projection is partitioned on `created_at` itself.

`DistinctValues` for `/filter-options` (`repository.go:364-390`): six sequential `SELECT DISTINCT <col> FROM msg_dispatch_jobs_read WHERE <col> IS NOT NULL
[AND (client_id IS NULL OR client_id = ANY(...))] ORDER BY 1 LIMIT 200`, **with no time bound and no cache**. The Vue page calls it on mount
(`DispatchJobListPage.vue` `onMounted` -> `dispatchJobsApi.filterOptions()`).

---

## 4. Gap analysis

Legend: Have = yes / partial / no. Work = UI only / API / schema+index. Priority: P1 (do first), P2, P3, Decline/Decide.

| # | Integral capability | Have? | What it would take | Priority |
|---|---|---|---|---|
| 1 | **Errors lens** (status-keyed list with error text column, Resend + Ignore) | Partial: status filter + "Requeue everything in error" preset; no error text column (`lastError` is in the row but not shown), no dedicated page | UI only (add a "Failed jobs" view = list with `statuses=FAILED,ERROR`, add `lastError` column). Served by `(status, created_at DESC)` | P1 |
| 2 | **Potentially blocked / stuck lens** (PENDING/QUEUED older than 5 min) | No | API (a `stuck` preset: `status IN (PENDING,QUEUED) AND created_at < now()-Δ`, and for QUEUED/PROCESSING `updated_at < now()-Δ`), UI page. Served by `(status, created_at DESC)` range + residual `updated_at`. PENDING with a future `scheduled_for` must be excluded (it is a retry backoff, not stuck) | P1 |
| 3 | **Real blocker view** (Related Blocking: ERROR/IN_PROGRESS in same group) | No (group filter only) | API `GET /api/dispatch-jobs/{id}/group` (jobs in the group with status in FAILED/ERROR/PROCESSING/QUEUED, plus the count of PENDING behind) + drawer tab. Serve from `msg_dispatch_jobs` via the existing `idx_dispatch_jobs_status_group` (status prefix, then message_group): no new index. Read-your-writes bonus: write table is not subject to projection lag | P1 |
| 4 | **Held-groups overview** (not in Integral; the equivalent of its shouldBlock model) | No | API `GET /api/dispatch-jobs/held-groups` (`GroupHoldingStatuses` heads, count behind). Same index. Closes the "draining but nothing happening" question the router's Blocked Groups tab cannot answer | P2 |
| 5 | **Default time window / required key** (3 days default on search pages; status or prefix bound on the rest) | No: list has no window, no required key | UI default (e.g. 7 days) + API rule (see 5.1, owner decision) | P1 |
| 6 | **Bounded counts** (limit+1; explicit Count button; never count COMPLETED) | Partial: preview count is exact and unbounded; fired automatically by the select-all banner | API (cap the preview count subquery at 10,001 and return `countIsLowerBound`), UI (button or lazy) | P1 |
| 7 | **Keyset "Load more"** with cursor snapshot | Partial: repository + templ UI have it; the huma API and Vue do not (only `offset`) | API (`cursorCreatedAt`+`cursorId` on `listInput`, `hasMore`, `nextCursor`), UI | P2 |
| 8 | **Facet queries cached and time-bounded** | No: six unbounded `DISTINCT` reads per page load, uncached | API (bound to last N days, cache 5-30 min per scope; or derive from the small config tables: applications, subscriptions, pools, clients, event types, which already exist as entities) | P1 |
| 9 | **Prefix search on message group** (Integral: tenant + prefix + window) | No (exact only) | Schema+index in Postgres: `message_group varchar_pattern_ops` index; see 5.2 for why I recommend a swap, not an add | P3 |
| 10 | **Search by event id** | Yes (`/event/{eventId}`, `event_id` index) | none | done |
| 11 | **Search by record id** | Partial: `GET /{id}` exists, FindByID is unpruned (TSID carries the time but is not used to bound partitions) | API: derive `created_at` bounds from the TSID | P3 |
| 12 | **Server-side exclusion of noisy event types** | No (we have `code`/`codes` include only; no exclude) | API param `excludeCodes` on `DispatchJobFilter` as a residual filter (no index) | P3 |
| 13 | **Resend selected ids** | Yes | none | done |
| 14 | **Batch resend by filter with preview, confirm, cap, audit** | Yes, **better than Integral** (preview, `confirmCount`, 409, 10k cap, keyset walk, partial-failure report). Integral has none of these | Tighten: refuse non-failed statuses by default (see 5.3) | P2 |
| 15 | **Change status to any value** (incl. IGNORE) | Partial and safer: cancel (FAILED to CANCELLED), complete (FAILED to COMPLETED) exist in the API only | UI only: add Cancel and Complete to row/drawer actions and to selection (ids). Bulk-by-filter cancel needs an API (reuse the requeue-by-filter machinery with a different op) | P1 for UI; P2 for bulk-by-filter |
| 16 | **Process now** (synchronous delivery from the console, skips queue/blocking) | No (we have Sign, a dry-run) | API + risk: bypasses per-group ordering. See 5.4, owner decision | Decide |
| 17 | **Edit payload** | Yes, with status rule, size cap and digest-only audit | none | done |
| 18 | **Edit "by latest"** (a deadline) | No (we have `expires_at`, not operator-editable) | API only; low value | P3 |
| 19 | **Attempt history with request/response** | Yes, richer than Integral (headers, signing account, response body per attempt) | none | done |
| 20 | **Per-attempt payload as it was at failure** | No (our edit overwrites the payload; attempts keep request summary, not the body) | schema: only if edits become common | P3 |
| 21 | **Measurements / timing tab** | Partial: per-attempt `durationMillis` | none | P3 |
| 22 | **Audit log tab on detail** | No in the drawer, though aud_logs has rows (`/api/audit-logs/entity/{entityType}/{entityId}` exists, `audit/api/api.go:38`). Requeue is audited as a batch rollup, not per job | UI only for cancel/complete/edit; per-job rows for requeue need the operation to write per-id audit or reference ids in the rollup | P2 |
| 23 | **Status counters** (QUEUED/PENDING/IN_PROGRESS/ERROR, cached 5 min, never COMPLETED) | No (only a Prometheus backlog gauge) | API: `GET /api/dispatch-jobs/status-counts` bounded and cached; see 5.5 for index implications | P2 |
| 24 | **Server-enforced permissions per action** (view / edit / resend) | No: one `dispatch-job:view` gates every mutation | Permission model change; owner decision | Decide |
| 25 | **Tenant pinning** | Yes (SQL-level `AccessibleClientIDs`, plus app confinement F20) | none | done |
| 26 | **Truncation banner** | Partial (Vue shows the notice only when exactly `pageSize` rows came back) | UI only | P3 |
| 27 | **Persisted filters, mode bar** | Yes (URL sync via `useListState`); no mode bar | UI only, optional | P3 |
| 28 | **Event-type enquiry done right** (type-first, windowed, keyset, bounded count, cached facet): Integral's version is poorly indexed, see section 8 | Events: partial (type-first index `(type, created_at DESC)` exists, multi-type + no window is weak). Dispatch jobs by event type (`code`): no composite | API (required window, keyset, capped count, facet from `msg_event_types`), schema+index (8.4) | P1 for query shape; P2 for the index |
| 29 | **Facet/rollup table for event-type activity** (types seen per client per day, counts) | No | schema (small rollup table maintained by the projector), see 8.5 | P3, only if facets stay slow after item 28 |

### 4.1 Where our schema does not support an Integral-style filter at our scale

Scale context: ~20k msg/s aggregate (ingest-throughput-plan), monthly partitions, 90-day retention default. At 20k/s that is
~1.7 billion rows/day; the figure for a busy deployment is hypothetical, but the shape of the risk does not change with a
smaller number: **we have no tenant leading column**, because `client_id` is nullable (platform-scoped jobs are NULL) and sits second
in the only index that starts with it only in `(client_id, created_at DESC)`.

1. **`source` filter (the Vue search box)** has no index. A `source = ?` filter with no `since` is a scan of every partition's
   heap. Worst case in the console today. Mitigation first: require a window when `source` is set. Do not add the index (see 5.2).
2. **`subdomain`, `aggregate` filters** have no index (3.1). They are normally combined with `applications` (indexed), which makes the
   application index do the work and `subdomain/aggregate` residual; with no `applications` selected they scan. The Vue UI cascades
   (Application then Subdomain then Aggregate) so this is mostly masked, but the API accepts them alone.
3. **`code` / `codes` / `application` with no window and ORDER BY `created_at DESC` LIMIT n.** The planner either walks `created_at`
   backwards filtering for the code/application (cheap for common codes, pathological for rare ones: reads until it has n matches
   across up to 90 days), or uses the `code`/`application` index and sorts everything matching (pathological for common codes).
   No composite `(code, created_at DESC)` exists. This is the Integral failure mode its design avoids by making the window mandatory.
4. **`DistinctValues` for filter-options**: `SELECT DISTINCT code FROM msg_dispatch_jobs_read ...` over all partitions every page load.
   PostgreSQL has no loose index scan before 18; the dev embedded server is PG 18 (`docs/embedded-postgres-postgis.md:21`), which has btree skip
   scan, but **I do not know the production PG major** and `client_id`/`kind`/`subscription_id` queries add a scope predicate that may defeat it.
   At volume this is the single most expensive read the console issues, and it runs on every list page open.
5. **Preview count with no window** (`CountMatchesSQL`, `repository.go:178-185`): `count(*)` grouped by status and client over every
   matching row. Fine for `statuses=FAILED` (small set via `(status, created_at)`), unbounded for any filter that does not include a
   rare status. It is fired automatically when the user ticks the page checkbox.
6. **`messageGroup` exact** uses the single-column `message_group` index (no `created_at` second column), so "follow this group newest first"
   reads every row of the group over 90 days and sorts. A busy aggregate with millions of jobs is the bad case.
7. **Prefix search on message group** is not possible with the existing btree under a non-C collation (needs `*_pattern_ops` or a C-collated column).
8. **Offset pagination** exists in the API (`offset`); deep offsets cost O(offset). The Vue page does not use it today.
9. **Projection lag**: lists and counts read `msg_dispatch_jobs_read`. A "stuck" lens or a "just requeued" refresh can briefly show stale statuses.
   Counts used for confirm (`confirmCount`) are from the projection too, so a mismatch can only fail safe (409) in one direction:
   if the projection is behind, jobs that were already requeued still count, and jobs newly FAILED are not yet counted. Not a correctness
   issue for the cap, but worth knowing. I did not measure typical lag.

### 4.2 Index recommendations (only where justified)

Write-path cost frame: every status change updates a row whose `status` is indexed, so the update cannot be HOT and **every** secondary
index on `msg_dispatch_jobs_read` is rewritten for that row (my reading of PostgreSQL semantics plus the memory note on write amplification;
not measured). Each extra index on the read table is therefore paid on every PENDING, QUEUED, PROCESSING and terminal transition of
every job. The only no-regret moves are **swaps** (replace a single-column index by a composite whose leading column is the same).

| # | Proposal | Serves | Cost |
|---|---|---|---|
| I1 | `CREATE INDEX idx_msg_dispatch_jobs_read_group_created ON msg_dispatch_jobs_read (message_group, created_at DESC);` then `DROP INDEX idx_msg_dispatch_jobs_read_message_group;` | "Follow one group newest first": `WHERE message_group = $1 ORDER BY created_at DESC, id DESC LIMIT n` (the existing `messageGroup` filter; also serves the per-group window). A single-column index cannot give the order | Net zero indexes. Build per-partition `CONCURRENTLY` and attach, as 036's note requires on a populated DB |
| I2 | `CREATE INDEX idx_msg_dispatch_jobs_read_sub_created ON msg_dispatch_jobs_read (subscription_id, created_at DESC);` then drop `idx_msg_dispatch_jobs_read_subscription_id` | "Jobs of this subscription, newest first" (`subscriptionId` filter; subscription detail drill-down) | Net zero |
| I3 | No new index for the stuck lens, the real-blocker view or the held-groups view (4 rows 2-4): `(status, created_at DESC)` on the read table and `idx_dispatch_jobs_status_group` on the write table already serve them | | none |
| I4 | **Not recommended**: `(source, created_at DESC)`, `(code, created_at DESC)`, `(application, created_at DESC)`, `(subdomain|aggregate, ...)`. Each is +1 index rewritten on every transition. Prefer the mandatory-window rule (5.1) which makes these filters safe with the indexes we have, because partition pruning bounds the work to the window's partitions. Revisit only if `pg_stat_statements` shows a window-bounded query still slow | | high |
| I5 | Prefix search on `message_group`: `... (message_group varchar_pattern_ops, created_at DESC)` replacing I1's index (one index serves both equality+order and `LIKE 'x%'`). Only if the owner wants prefix search | Integral's `messageGroup LIKE 'prefix%'` | Same as I1 |

Everything above applies to the **read** projection. Do not add indexes to the write table: 015/067 removed them for the
dispatch engine and `docs/db-index-plan.md` is explicit about it.

---

## 5. Notes behind the recommendations

### 5.1 Mandatory window or key (owner decision)

Integral's rule, as implemented, is: the page cannot query without either a status (Errors), a status+age (Blocked), a tenant+prefix+3-day
window (Enquiry), or a tenant+event-type+3-day window (Event Type). Proposed FlowCatalyst rule, to be enforced in the API (400
`WINDOW_REQUIRED`) and defaulted in both UIs:

- If the filter has neither `since` nor a `statuses` made only of rare states (`FAILED`, `ERROR`, `CANCELLED`, `EXPIRED`), the UI defaults `since` to
  now-7d, and the API rejects a list/preview/requeue-by-filter without `since` unless `statuses` is rare **or** `messageGroup`/`eventId` is set.
- Apply to `list`, `requeue-by-filter/preview`, `requeue-by-filter`, `filter-options`.

This needs an owner decision because it changes API behaviour for SDK callers of `/api/dispatch-jobs` (Laravel/TS SDKs use `by-event`, `raw`, list). A softer
variant is UI-only defaults plus the API enforcing the rule **only for the bulk and count endpoints**, which no SDK uses.

### 5.2 Why swaps, not adds

The read projection already has nine secondary indexes and is re-upserted on every transition (memory: ingest-throughput-plan, "biggest
write amplification"). Integral accepted ~28 indexes on a table at lower volume with a MySQL buffer pool; its own change doc declined one
more ("deliberately not added now"). Our default should be the same discipline.

### 5.3 Make requeue-by-filter refuse jobs that are in flight

`ResendDispatchJobs` has no status guard (`resend.go`; the comment in `resend_by_filter.go`). A requeue-by-filter built from a page with no status
filter resets COMPLETED jobs (re-delivery) and QUEUED/PROCESSING jobs (concurrent double delivery). Recommend: the bulk path (not the id-list path,
which is a deliberate operator act) defaults to `FAILED/ERROR/CANCELLED/EXPIRED` only, with an explicit `includeStatuses` override. This is an API
change that alters documented behaviour ("status does not make a job ineligible") so it needs an owner ruling.

### 5.4 Process now (owner decision)

Integral's "process now" runs delivery inline and skips group blocking. For us, the equivalent is a platform-side synchronous
delivery of one job to its connection, bypassing the router. It conflicts with per-group FIFO and the router's in-flight tracking. The existing
**Sign** action already answers "which side holds the wrong secret". A safer variant is **"deliver now, only if this job heads its
group and the group is idle"**; I would not build it before items 1-8. Decide whether it is wanted at all.

### 5.5 Status counters

`GET /api/dispatch-jobs/status-counts` bounded like Integral's: only PENDING/QUEUED/PROCESSING/FAILED, never COMPLETED, cached 30-60 s per scope.
`SELECT status, count(*) FROM ... WHERE status IN (...) GROUP BY status` uses the status index but scans all rows of those statuses, which for
PENDING during a burst is the backlog (hundreds of thousands to millions). The backlog gauge already caps at 100,001; follow the same cap
(`SELECT ... FROM (SELECT 1 FROM ... WHERE status=$1 LIMIT 100001) t`) and return `{count, capped}` per status.

---

## 6. What I could not determine

1. **Integral's actual production index set.** The requirements doc names `idx_sr_tenant_mg_created` and `idx_sr_tenant_searchkey_created`
   as `(tenant_id, messageGroup|searchKey, createdAt)` (`docs/change/sr-enquiry-prefix-eventtype-pagination/requirements.md:27`), but **no migration
   in this package creates an index with those names**; the only definitions in the package are `(tenant_id, messageGroup)` and `(tenant_id, searchKey)`
   in the consolidated DDL. The extra indexes were presumably added by another package (the v1 integral service) or by hand. Per the search rules I did not
   look outside the package. So the claim "createdAt is filtered in the index" is the doc's, not something I verified.
2. No `EXPLAIN` output was available for either system; every statement about plans and index use is from reading the SQL and DDL.
3. Production PostgreSQL major version for FlowCatalyst (affects `DISTINCT` skip scan, 4.1 item 4). Embedded dev is PG 18.
4. Whether any Integral page calls `GET /status-counts`; the wrapper exists but the four lens pages do not call it.
5. Typical projection lag for `msg_dispatch_jobs_read`.
6. The behaviour of Integral's enquiry when a staff user omits `tenant_id` (the UI prevents it; not tested).
7. Actual row counts and cardinalities in either system.

---

## 7. Recommended plan

Each increment is sized for one agent session. The first three are the highest operational value per unit of risk.

**1. Failed-jobs lens plus Cancel and Complete in the UI (UI only, Vue and templ).**
Add the `lastError` column to the list; a "Failed jobs" mode (preset `statuses=FAILED,ERROR`, window default 30 days, because FAILED is small and
`(status, created_at DESC)` serves it); per-row and per-selection **Cancel** and **Complete** (id-list; the API exists: `POST /{id}/cancel|complete`)
with confirm dialogs; show the audit rows in the drawer using `/api/audit-logs/entity/...`. Zero schema or API change; closes the largest
gap between "can see the failure" and "can resolve it without a requeue".

**2. Default window, bounded counts, cached facets (API + UI).**
(a) Vue/templ default `since` = 7 days (owner ruling needed for the API enforcement, 5.1); (b) preview count capped at 10,001 with `capped: true`, and fired
by a button or when the user opens the dialog, not on page-select; (c) `filter-options`: bound the DISTINCT reads to the last N days and cache 5 min per
visibility scope, or source applications, clients, pools and subscriptions from their config tables. This is the cheapest protection against the
unbounded reads in 4.1 items 4 and 5 and requires no schema change.

**3. Stuck lens and real-blocker view (API + UI, no new index).**
`GET /api/dispatch-jobs?stuck=true&olderThan=5m` (or a dedicated endpoint): `PENDING/QUEUED` older than Δ with `scheduled_for IS NULL OR scheduled_for <= now()`,
and `QUEUED/PROCESSING` with `updated_at < now()-Δ`; and `GET /api/dispatch-jobs/{id}/group` returning the group's FAILED/ERROR/PROCESSING/QUEUED jobs (oldest
first, limit 100) plus a bounded count of PENDING behind, read from `msg_dispatch_jobs` through `idx_dispatch_jobs_status_group`. Drawer tab "Group".
This answers the recurring "draining but nothing happening" and "which job is holding my group" questions that today need a message-group filter and eyeballing.

**3b. Event-type queries, query shape first (API + UI, both products; no schema).** Required window and keyset on the type-first paths, capped counts, facets from the type catalogue instead of `DISTINCT`. Details in 8.3. For FlowCatalyst this is the events list `types` filter, the dispatch-jobs `codes` filter, and `filter-options`; do it together with increment 2.

**7b. Event-type indexes (schema, both products).** Integral: `(tenant_id, event_type, createdAt)`. FlowCatalyst: `(code, created_at DESC)` swap on `msg_dispatch_jobs_read`; events already have `(type, created_at DESC)`. Exact shapes and costs in 8.4. After 3b and a measurement.

**8b. Event-type activity rollup (schema, optional).** 8.5.

**4. Held-groups overview page (API + UI).** `GET /api/dispatch-jobs/held-groups`: per group, the earliest holder (FAILED/ERROR head, or PENDING in backoff),
how many jobs wait behind, oldest wait, with Cancel/Complete/Requeue on the head. Same index.

**5. Keyset Load More in the huma API and Vue (API + UI).** Add `cursorCreatedAt`+`cursorId` to `listInput`, return `hasMore`/`nextCursor`, and keep the first
search's params for continuation as Integral does. The repository and templ already implement the cursor.

**6. Bulk-by-filter Cancel and Complete, and a status guard on bulk requeue (API; owner decision on 5.3).** Reuse the preview/confirm/cap walk with a
`fromStatuses` guard.

**7. Index swaps I1 and I2 (schema).** Per-partition `CREATE INDEX CONCURRENTLY` then attach, then drop the old single-column indexes; verify with a plan test in the style
of `poller_plan_pg_test.go`. Do this when the group-follow and subscription drill-down views are in regular use.

**8. Status counters endpoint and dashboard tiles (API + UI)**, capped as in 5.5.

**9. Optional/low: server-side exclude-codes; TSID-bounded `FindByID`; prefix search on message group (I5); separate edit/resend permissions; editable `expires_at`.**

### Needs an owner decision

- 5.1: make the time window mandatory for list/preview/requeue-by-filter in the API (SDK impact), or only default it in the UIs.
- 5.3: bulk requeue-by-filter refuses non-failed statuses by default (changes documented behaviour).
- 5.4: whether "process now" is wanted at all.
- Item 24: split `dispatch-job:view` into view / resend / edit permissions (also affects how Integral-style roles map).
- 8.3: single-select event type (or union of single-type queries) and a required, capped window on Integral's event-type enquiry; default status list excluding COMPLETED.
- 8.4: approve the Integral `(tenant_id, event_type, createdAt, id)` index (+1 index entry per insert) and the FlowCatalyst `code` index swap.
- Whether prefix search on message group is wanted (I5) given the write-path cost.
- Retention: 90 days default means a mandatory-window rule of 7 days is a UI convenience, not a data limit; confirm that is acceptable.

---

## 8. Event-type queries (owner addition)

### 8.1 What Integral's event-type enquiry does and why it is poorly served

Endpoints and code: `GET /event-type-enquiry`, `/event-type-enquiry/count`, `/event-type-names`, plus the event-type facet in `filter-options`
(`routes/api.php:468-470`; `CTL:427-503`; `REPO:530-589`, `REPO:237-256`). Page: `EventTypeEnquiryPage.vue`.

List query (`REPO:530-568`):

```sql
SELECT <19 summary cols> FROM integration_poll_records
 WHERE tenant_id = ?
   [AND event_type IN (?, ...)]
   AND status IN ('QUEUED','PENDING','IN_PROGRESS','COMPLETED','ERROR','IGNORE')   -- default: every status but PAUSED
   [AND createdAt >= ? AND partition_key >= floor(?)] [AND createdAt <= ?]
 ORDER BY createdAt <asc|desc>
 LIMIT <50|200|500|1000, default 1000 server side>
```

Count query (`REPO:541-545`): the same WHERE, `COUNT(*)`, no limit, no cap.
Type list (`REPO:573-589`): `SELECT event_type FROM ... WHERE tenant_id = ? GROUP BY tenant_id, event_type`, **no time window**, cached 5 min.
Facet (`REPO:237-256`): `SELECT DISTINCT domain, code WHERE tenant_id = ?`, no window, cached 1 hour. (This one is the exception that is fine: `idx_tenant_id_domain_code (tenant_id, domain, code)` covers it, and MySQL can walk it as a loose scan. Note it keys on `domain, code`, not `event_type`.)

Why the list and count are poor, from the DDL (section 2.3):

1. **No index leads with `(tenant_id, event_type, ...)`.** The `event_type` index is single-column (`integration_poll_records_event_type_index`); the composites that contain a type use `domain`+`code`, not the `event_type` string: `(tenant_id, domain, code)`, `(domain, code, tenant_id)`, `(domain, code, tenant_id, createdAt)` (`idx_event_type_tenant_created_at`). The query filters on `event_type`, so none of the four type-aware composites is usable. `event_type` is also nullable and stores `com.inhance.x.y` or `domain.code` strings, so it is not derivable from the index.
2. The optimizer's realistic choices (my reading; no EXPLAIN was available): (a) `(tenant_id, createdAt)` range over the whole window, filtering `event_type` and `status` per row, which reads every row of the tenant in the window, ordered, then stops at LIMIT. Cheap only when the type is common; a rare type reads the whole tenant window. (b) `event_type` single-column index: reads that type across **all tenants and all time**, filters `tenant_id` per row, then filesorts for `ORDER BY createdAt`. (c) `(tenant_id, status)`: reads every row in each of the six statuses (almost the whole tenant) and filesorts. Which one MySQL picks depends on statistics; none is bounded by the thing the operator typed (the type).
3. **The sort is not served** whichever type index is used (no `createdAt` after `event_type`), and with `IN (several types)` no index can yield one ordered stream anyway.
4. **The default status list includes COMPLETED**, the largest class ("can be in the millions per tenant", `REPO:27-29`), so the status condition filters nothing and the page has none of the protection the Errors lens gets from a rare status.
5. **`COUNT(*)` is unbounded** and exact. On a type with millions of rows it is a covering-index-less scan. It is operator-initiated (the Count button) which is the only mitigation.
6. **Default `limit=1000` server-side** (`CTL:buildEventTypeFilter`, `'1000'`) and unclamped.
7. **`event-type-names` has no window**, `GROUP BY tenant_id, event_type` has no `(tenant_id, event_type)` index so it scans the tenant's whole history through whichever index it picks. The 5-minute cache hides it except after expiry.
8. **Partition floor only bounds the lower end** (`REPO:596-610`), so the "last 3 days" default is the real protection and the user can open the window to months.

What the design does right here: tenant required, 3-day default window, partition-floor pruning, Count on demand, row limit with truncated flag.

### 8.2 FlowCatalyst equivalents and whether they share the weakness

There are two: events by type and dispatch jobs by event type.

**Events** (`msg_events_read`, `internal/platform/event/repository.go:187-248`): filter `type = $1` / `type = ANY($n)` (the `types` CSV), plus `source`, `subject`, `client_id(s)`, `application(s)`, `subdomain(s)`, `aggregate(s)`, `correlation_id`, `since`/`until` on `created_at`, keyset cursor, `ORDER BY created_at DESC, id DESC LIMIT` (default 100, max 1000). Indexes after migrations 004/019/036/037: `application`, `subdomain`, `aggregate`, `correlation_id`, `created_at`, `(client_id, created_at DESC)`, `(type, created_at DESC)` (migration 036, which dropped the single `type` and `client_id`, `037:34-35`; `time` index dropped as dead, `037:26`).
Verdict: **better than Integral for a single type, and the same weakness for several**:
- Single `type = $1 ORDER BY created_at DESC LIMIT n` is the textbook access path, served in order by `(type, created_at DESC)` per partition (Merge Append across the partitions the window leaves), so cost is proportional to the page. Good with or without a window.
- `type = ANY($n)` with several types: the index returns each type's run in turn, not one stream ordered by `created_at`, so I expect a top-N sort over every matching row in the window (PostgreSQL does not merge several index scans of an `= ANY` list into one ordered stream; I have not run EXPLAIN). With no `since` that is up to 90 days of those types. This is the same shape as Integral's `IN (...)` problem.
- `application`/`subdomain`/`aggregate` (the Vue cascading facets) have single-column indexes without `created_at`: filter then sort, the same issue as dispatch jobs (4.1 item 3).
- Client + type together: `(client_id, created_at DESC)` or `(type, created_at DESC)`, never both, so a client-specific type search filters one against the other.
- **No count** is offered on events at all (good); the Vue page shows "size limit reached" at `pageSize` rows.
- `GET /api/events/filter-options` runs `SELECT DISTINCT type|source|subject|client_id|correlation_id|application|subdomain|aggregate FROM msg_events_read WHERE col IS NOT NULL [scope] ORDER BY 1 LIMIT 200` (`repository.go:423-450`), uncached and with no time bound. `subject` and `correlation_id` are near-unique values, so `DISTINCT ... LIMIT 200` has to find 200 distinct values and sort, and `type` has no plain index any more (only `(type, created_at DESC)`, usable as a full index scan). This is the same weakness as Integral's name list, worse because it is uncached. Whether the SPA asks for all of those columns is not determined: the handler is `api.go` `filterOptions`.
- Write cost: `msg_events_read` has 7 secondary indexes; events are inserted once and projected (not rewritten per status), so index cost is per event, not per transition. Much cheaper to index than the dispatch read table.

**Dispatch jobs by event type** (`code` in `msg_dispatch_jobs_read`; `application`/`subdomain`/`aggregate` are the split of `code`, `repository.go:44-48,142-160`): filters `code`, `codes` (CSV), `applications`, `subdomains`, `aggregates`; the same `ORDER BY created_at DESC, id DESC`. Indexes: single-column `code` (036) and `application` (019), and `(status, created_at DESC)`, `(client_id, created_at DESC)`. **No `(code, created_at DESC)`**, so this is Integral's failure mode almost verbatim: `code = X ORDER BY created_at DESC LIMIT 200` either walks `created_at` backwards filtering by `code` (cost depends on how common the code is) or reads all rows of that code across partitions and sorts. By event id (`event_id` index) is fine: a small fan-out set per event.
Counting: `requeue-by-filter/preview` runs `count(*) ... GROUP BY status, client_id` with the same WHERE, unbounded, and the Vue page fires it automatically on select-all (3.3). That is Integral's unbounded Count, made automatic.

**At 20k msg/s:** the events table grows at the ingest rate (one row per event); the dispatch read table at ingest rate times subscriber fan-out and with several rewrites per job. A single common type can be most of a tenant's day, so "newest N of a type" is fine where it is served in order and not fine where it is sorted. The shapes above are scale-independent; the numbers are not measured.

### 8.3 Query-shape changes first (both sides)

Same order of preference as the rest of this document, none needs a schema change:

1. **Type-first, one type at a time.** Make the primary event-type control single-select (or accept a list but run it as N single-type ordered queries merged in the application, each `LIMIT page+1`). Single-type ordered scans are what the two good indexes serve. The multi-type `IN`/`ANY` becomes a union of bounded queries instead of a sort of everything.
2. **Required window.** Integral: reject `created_at_from` absent on `/event-type-enquiry` and `/count`; cap the span (for example 31 days) because the `partition_key` floor is only a lower bound. FlowCatalyst: default `since` to 7 days in the UI and require it in the API for `types`/`codes`/`application` filters when no `clientId` is given (5.1).
3. **Keyset pagination, not a big LIMIT.** Integral's event-type lens still fetches up to 1000 rows in one go and shows `truncated`; give it the same `(createdAt, id)` cursor the Enquiry page already has (`REPO:464-467`). FlowCatalyst: expose the existing repository cursor in the huma list input (increment 5).
4. **Bounded count.** Replace `COUNT(*)` with `SELECT count(*) FROM (SELECT 1 FROM ... WHERE ... LIMIT 10001) t` and return `{count, capped}`; Integral keeps the button, FlowCatalyst stops auto-firing the preview.
5. **Status filter off by default.** Integral's default of every status except PAUSED adds nothing the index can use; default the page to "not COMPLETED" with COMPLETED opt-in, mirroring the Errors/Blocked lenses and `status-counts` policy (`REPO:27-29`).
6. **Facets from a catalogue or a window, not `DISTINCT` over history.** FlowCatalyst already has `msg_event_types` (`004_messaging_tables.sql:185`, indexed on `code`): serve the event-type dropdown for both events and dispatch jobs from it (and from `msg_subscriptions`/applications for the rest). Whether every emitted type is registered as an event type row is **not verified**; if not, fall back to a windowed `DISTINCT` (last 7 days) cached for 5 minutes. Integral: bound `event-type-names` to the last 30 days and use the cache already there.
7. **Clamp the limit server-side** (Integral `errors`/`blocked`/`event-type-enquiry`: no max today).

### 8.4 Index shapes (after 8.3, justified by the query each serves)

**Integral (MySQL, `integration_poll_records`):**

```sql
-- serves: tenant + event_type [IN list] + createdAt window, ORDER BY createdAt, LIMIT (list) and the capped count
-- status stays a residual filter; with a single event_type the sort is index-ordered
ALTER TABLE integration_poll_records
  ADD INDEX idx_sr_tenant_event_type_created (tenant_id, event_type, createdAt, id);
```

- With one type the plan is an index range on `(tenant_id, event_type, createdAt)` with the keyset condition `(createdAt, id) < (?,?)` pushed into the range. The trailing `id` makes the keyset condition index-resolvable.
- `event_type` is `varchar(255)` utf8mb4: key length 255*4 = 1020 bytes + tenant 8 + createdAt 7 + id 104 = about 1.1 KB per entry, near the 3072-byte InnoDB limit but legal, and large. If that is a concern, index a prefix `event_type(100)` (types are far shorter) or add a generated column.
- Write cost: +1 index on a table that already carries ~28 secondary indexes and is rewritten on every status change (status, attempts, errorDescription, not_before change often; the new index does not include them, so InnoDB updates only the indexes whose columns changed, and `createdAt`/`event_type`/`id` never change). That means **no write cost on status transitions, only on insert** (one more index entry per record). This is the key difference from our dispatch read table, where `status` is indexed and every transition rewrites all indexes.
- Candidate to drop once it is in and verified with `sys.schema_unused_indexes`: the single-column `integration_poll_records_event_type_index` (it is a leftmost-prefix duplicate only of an index that starts with `event_type`, which the new one does not, so it is **not** a duplicate; keep it unless usage shows otherwise).
- `event-type-names`: the new index is also a covering path for `SELECT DISTINCT event_type WHERE tenant_id = ?` (leading `(tenant_id, event_type)`), turning the full-history `GROUP BY` into a loose index scan.
- Also stop writing `event_type` in the `domain.code` fallback form for some rows if possible (the UI has `formatEventTypeOrNull` for rows with null `event_type`, `useSubscriptionRecordFacets.ts`); null-typed rows are invisible to a type-first path.

**FlowCatalyst (PostgreSQL), dispatch read table:**

```sql
-- swap, not add: replaces the single-column code index (036)
-- serves: WHERE code = $1 [AND created_at range] ORDER BY created_at DESC, id DESC LIMIT n   (codes filter, one code at a time)
CREATE INDEX CONCURRENTLY idx_msg_dispatch_jobs_read_code_created
    ON msg_dispatch_jobs_read (code, created_at DESC);
DROP INDEX idx_msg_dispatch_jobs_read_code;
```

(on the partitioned parent: create `ON ONLY` the parent, build per partition concurrently, attach, as migration 036's own note says.) Net zero indexes, so zero extra write-path cost; the `code` column never changes, so there is nothing new on transitions beyond what the single-column index already costs. This is the one index I would add on the dispatch read table; the others in 4.2 remain "not recommended".

**FlowCatalyst, events read table:** no new index for the single-type path. Optional, only if profiling shows client-scoped type searches slow:

```sql
CREATE INDEX CONCURRENTLY idx_msg_events_read_client_type_created
    ON msg_events_read (client_id, type, created_at DESC);
```

It serves `client_id = $1 AND type = $2 ORDER BY created_at DESC` (the per-tenant type view) and makes `(client_id, created_at DESC)` redundant for the client-only path only if the planner can still use it for client-only queries (it can, as a skip over `type` is not possible, so keep both). Cost: +1 index per event insert, no transition cost (events are append-only plus a projection stamp). Defer.

### 8.5 Denormalisation or a rollup table (only if justified)

Justified only for the **facet and "what is happening" views**, not for the listing:

- A small rollup `msg_event_type_daily (day date, client_id, type, event_count, first_at, last_at, PRIMARY KEY (day, client_id, type))`, upserted by the projector from the batch it already projects (it already touches each event once). Serves: the event-type dropdown for a window (`SELECT DISTINCT type FROM rollup WHERE day >= ...`, tiny), "event-type counts per day" (an exact count without scanning events), and a count preview for the events side. Write cost: one upsert per distinct (day, client, type) per projector batch, not per event, so negligible. Cost of building: new migration, projector change, backfill, and one more table to retire with partition retention. I would do it only after 8.3 item 6 proves insufficient, because `msg_event_types` already answers the dropdown for registered types.
- Integral equivalent: a `(tenant_id, day, event_type, count)` summary maintained at insert or by a nightly job, replacing `COUNT(*)` and `event-type-names`. Integral cannot hook the projector (no projection), so it would be a scheduled job over the last day(s); the existing 5-minute cache already covers the dropdown, so this is only worth it if the Count button is used on large types.
- Not justified: denormalising `event_type` into the dispatch read table (it is `code` already) or into a separate type-id column; a surrogate type id would shrink index keys (Integral's 1 KB entries) but needs a lookup table and migration for modest gain. Mention only.

### 8.6 What to measure before building indexes

`EXPLAIN (ANALYZE, BUFFERS)` of: (Integral) the list and count with one common type, one rare type and an `IN` of three, for a tenant with a long history; (FlowCatalyst) `types=ANY` with 1 and 3 types and no `since`, `codes` with a rare and a common code on the dispatch read table, and the six `DISTINCT` facet queries on a production-sized copy. I could run none of these.
