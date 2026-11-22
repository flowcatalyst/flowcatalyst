# Payload storage: a payload table, as text, offloaded above a size

Status: built on the branch `feat/payload-table` (2026-11-20 and 21), not merged, not deployed, and
not run against real S3. Sections 1 to 17 are the design as the owner approved it; where the code
differs, section 18 says how, and the code and `docs/java-handoff-2026-11-20-payload-table.md` are
right. `docs/environment-variables.md`, "Turning payload storage on", is the operator's page.

Decided by the owner on 2026-11-20: payloads live in a table of their own that events and jobs
point at, so an edited job gets a payload row of its own (section 3.1); offloaded payloads are
packed into segments (section 5); an event list leaves an offloaded payload out and says so
(section 11); no subscriber depends on the key order JSONB produces today (section 3.2); when the
store cannot be written a payload is kept in the database (section 7); the largest payload is 5 MB
where a store is configured, for documents such as a planning solve (sections 4 and 8); and the
expiry of stored objects is set by the infrastructure code, as a lifecycle rule on the bucket
(section 7). Nothing is open; section 15 lists what was decided, section 17 the order it is built
in.

## 1. Why

The platform is deployed for businesses of very different sizes: from one node with a few thousand
events a day to 150 million or 1.5 billion events a month. Today every payload is stored in
PostgreSQL, several times, whatever its size:

| Table | Column | Copies per event |
|---|---|---|
| `msg_events` | `data` JSONB | 1 |
| `msg_events_read` | `data` TEXT | 1 (the projection copies `data::text`) |
| `msg_dispatch_jobs` | `payload` TEXT | 1 per matching subscription (the fan-out copies it) |
| `msg_dispatch_job_attempts` | `response_body` TEXT | up to 64 KiB per attempt (not part of this proposal, section 14) |

So an event with payload P and N subscriptions holds (2 + N) x P, for the 90 days the partitions
are kept. Nothing is offloaded; the only object stores the platform has are for function artifacts
and app updates.

This proposal makes three changes:

1. **One copy, in a payload table.** A payload is one row of `msg_payloads`, written once and never
   updated. The event and every job point at it.
2. **As text.** The payload is stored as the bytes the producer sent, not as JSONB.
3. **Offloaded above a configured size.** When a payload store is configured and a payload is larger
   than the threshold (5 KiB by default), its row holds a reference to an object in the store
   instead of the bytes.

A small deployment configures no store and gets changes 1 and 2 with no new infrastructure. A large
one sets a store and a threshold.

What it does not change: the rows and indexes that are not payload, roughly 5 KB per event with one
subscription (my estimate, not measured). At 1.5 billion events a month that is the larger problem;
section 14 lists what addresses it.

## 2. What was checked and measured

- Nothing queries inside `msg_events.data`: no JSON operator, no GIN index. The only casts are
  `data::text` on the way out (`internal/stream/events.go`, the fan-out's read).
- A job the fan-out raises gets its event's `created_at` (`buildJobs` in `internal/stream/fan_out.go`:
  `CreatedAt: e.CreatedAt`), and the projection gives a read row its event's `created_at` too. So an
  event, its read row and its jobs are in the same partition month and are dropped together.
- The queue message is a pointer (`common.Message`: id, target, group), never the payload.
- Delivery is one job per call: `POST /api/dispatch/process` loads the job with
  `Repository.FindByID` and sends it. There is one place to resolve a payload for delivery.
- `msg_dispatch_jobs_read` already leaves the payload out.
- The application outbox (`outbox_messages` in each application's database) is deleted on success.
  It is not part of this design.

**JSONB against TEXT**, on generated order documents with the default `pglz` compression: 1 367
against 1 352 bytes per row at 1.1 KB, 5 983 against 5 729 at 9.4 KB. TEXT saves 1 to 4% of one
copy. The reasons to move to TEXT are the other ones in section 3.2.

**Payload in the event row against a payload table.** An event row is updated twice after it is
inserted (`projected_at`, `fanned_out_at`), and neither update can be done in place because both
columns are in an index predicate. A payload kept in the row is written again with each version.
Measured with the tables and indexes of migration 019: insert, then the two updates, WAL and
on-disk bytes per event (events table plus payload table):

| Payload | In the event row: WAL | Payload table: WAL | In the event row: disk | Payload table: disk |
|---|---|---|---|---|
| 248 bytes | 3 048 | 2 744 | 1 874 | 1 464 |
| 818 bytes | 4 796 | 3 328 | 3 848 | 2 059 |
| 2 256 bytes | 6 756 | 3 977 | 6 486 | 2 792 |

The payload table costs one more row and one more primary-key entry per event and still writes
less at every size tried. The disk figures are before any vacuum and after two whole-table updates,
so the steady-state difference is smaller than shown; the WAL difference is not. Today's index set
on `msg_events` is larger than migration 019's, which makes the in-row case worse, not better.

## 3. The three rules

### 3.1 One copy, in a payload table

`msg_payloads` holds one row per stored payload: `id`, `created_at`, `data`, `ref`, `size`. A row is
inserted once and never updated, so it is never rewritten, never bloats, and carries one index, its
primary key.

`msg_events`, `msg_events_read` and `msg_dispatch_jobs` each get a `payload_id`.

- **`created_at` is the owner's.** A payload row is stored under the `created_at` of the event or
  job it was stored for, not under the time it was written. The payload of a row is therefore always
  read by primary key, `(payload_id, created_at)` with the row's own `created_at`, in one partition,
  and retention drops it in the same month as the row that points at it.
- **An event** with data gets a payload row. An event with no data gets none (`payload_id` NULL).
- **A job the fan-out raises** takes its event's `payload_id`. The fan-out no longer selects or
  copies the data.
- **A job created directly** (`POST /api/dispatch-jobs`, the batch route, a function's
  `dispatch.create`, ingestion) gets a payload row of its own.
- **The events projection** copies `payload_id`, not the data.
- **Editing a job's payload** (`PUT /api/dispatch-jobs/{id}/payload`) inserts a new payload row under
  the job's `created_at` and points the job at it. This is the same whether the job was raised from
  an event or created directly. The event and the other jobs of that event keep the original, and
  the original row is still there to compare with. The audit event records both payload ids beside
  the sizes and digests it records now. The existing guard stays: a claim that read the job before
  the edit is refused, because the edit changes the job row.

There is one way to resolve a payload, for every reader: the row's `payload_id`, then `data` or the
resolved `ref`.

### 3.2 As text

`msg_payloads.data` is TEXT. The ingest path stops parsing into JSONB and the readers stop
re-serialising. The API still refuses data that is not valid JSON, as it does today by decoding the
request; the platform's own event writer (`platformsink`) marshals its data itself.

One behaviour changes. JSONB re-orders keys and drops duplicate keys, so today a subscriber does not
receive the producer's bytes. With text, a **data-only** delivery (`data_only = true`) is the
producer's bytes. An envelope delivery is unchanged, because the envelope is built by parsing the
payload and marshalling it again (`buildPayloadWithGroup`). The signature is computed over the body
that is sent, so verification is unaffected. The owner has confirmed that no subscriber depends on
the present key order.

### 3.3 Offloaded above a size

At ingest, a payload larger than the threshold is written to the payload store and its
`msg_payloads` row holds `ref` instead of `data`. Nothing else differs, so the projection, the
fan-out, the scheduler and the queue never touch the store. A reader that needs the bytes (delivery,
a detail read) resolves the reference.

A subscriber always receives the whole payload. References never leave the platform.

## 4. Configuration

Per deployment, by environment variable, in the same form as `FC_FUNCTIONS_ARTIFACT_STORE` and
`FC_APP_UPDATES_STORE`:

| Variable | Default | Meaning |
|---|---|---|
| `FC_PAYLOAD_STORE` | empty | Empty: nothing is offloaded. `s3://bucket/prefix`: an S3 bucket, with the default AWS credential chain. `file:///abs/path`: a directory, for `fcdev`, tests and a single node. |
| `FC_PAYLOAD_OFFLOAD_MIN_BYTES` | `5120` | A payload larger than this is offloaded. `0` stops offloading while references already written stay readable. Refused below `2048`: under about 2 KB a store round trip costs more than it saves. Ignored when no store is set. |
| `FC_PAYLOAD_INLINE_MAX_BYTES` | `1048576` | The largest payload that may be kept in PostgreSQL. With no store it is the largest payload accepted (today that limit is implicit: the 1 MiB request cap). With a store it is the most the database takes when the store cannot be written (section 7). |
| `FC_PAYLOAD_MAX_BYTES` | `5000000` | The largest payload accepted when a store is configured. 5 MB and not 5 MiB, so that a delivery in an envelope still fits a receiver's 5 MiB limit (section 8). Ignored when no store is set. |
| `FC_INGEST_BATCH_MAX_BYTES` | `16777216` | Request cap of the two batch routes (section 8). Refused when it is less than twice the largest payload: the outbox sends data as a JSON string, and escaping makes it longer. |
| `FC_PAYLOAD_CODEC` | `zstd` | Compression of an offloaded payload: `zstd` or `none`. |
| `FC_PAYLOAD_CACHE_BYTES` | `67108864` | Size of the in-process cache of resolved payloads. |

Three profiles follow from this:

| Deployment | Setting | Result |
|---|---|---|
| Small, PostgreSQL only | no store | One copy as text, all in the database. No new infrastructure. |
| Single node or `fcdev` | `file://` | As a large deployment, on local disk. For development and tests; not for several instances unless the directory is shared. |
| Large | `s3://` | Payloads over the threshold in the bucket; their rows hold references. |

Changing the threshold, or setting it to `0`, is safe at any time, because a reader accepts both
forms. Removing the store while rows still hold references is not: those payloads become unreadable
(`PAYLOAD_STORE_NOT_CONFIGURED`). Set the threshold to `0`, wait one retention period, then remove
the store.

A threshold per client or per event type is not in this proposal. Because readers accept both forms
it can be added later as a platform setting with no schema change.

## 5. The store: segments

A batch route receives up to 1 000 items. One object per payload would be up to 1 000 `PUT` requests
for one batch. Instead the oversized payloads of one batch are written as **one object**, a segment,
and each payload row's reference says where in the segment its payload is. S3 reads a byte range of
an object in one request.

- **Segment.** The compressed payloads of one batch and one client, one after the other. No header
  and no index: each reference carries its own offset and length. Each payload is compressed on its
  own, so a range read returns something that can be decompressed alone.
- **Key.** `v1/<client id or _>/<yyyy-mm-dd>/<segment id>`, the date being the batch's ingest date
  in UTC and the id a TSID. Client first, so one client's payloads can be listed, measured or
  removed by prefix. A batch usually has one client, so it is usually one segment.
- **Reference** (`msg_payloads.ref`). A string, opaque to SQL and parsed in one Go package:
  `v1:<key>:<offset>:<length>:<codec>:<crc32c>`, about 80 characters. The bucket and prefix are not
  in it; they are the deployment's configuration.
- **Reading.** A range `GET` of `length` bytes at `offset`, the checksum verified, then decompressed
  and its size compared with the row's `size`. A mismatch is treated as a missing payload and logged
  as an error; a wrong range must never reach a subscriber.
- **Cache.** A bounded in-process cache keyed by payload id, so the N jobs of one event and a retry
  share one read. A payload larger than an eighth of the cache is not cached.

The package is new: `internal/platform/shared/payloadstore`. It owns both halves, the table and the
store: `Put(ctx, tx, items) ([]PayloadID, error)` packs what is oversized and inserts the rows, and
`Get(ctx, id, createdAt) ([]byte, error)` reads one. It has an S3 implementation, a file
implementation and a memory one for tests. The existing `artifact.Store` is not reused: it is
content-addressed, one object per digest, with no range read.

## 6. Each path, and what it costs per batch

| Path | Today | With this design | Store calls |
|---|---|---|---|
| Outbox processor to the batch routes | up to 100 items per request, cut by count only | cut by count and by bytes (section 8) | none |
| Event batch ingest (up to 1 000) | one duplicate query, one pipelined insert, data parsed to JSONB | the duplicate query first, then the oversized items of what remains packed into one segment per client, then one pipelined insert of the payload rows and the event rows | at most 1 `PUT` per client in the batch |
| Single event, single job, function `dispatch.create` | one insert | the same path with one item | at most 1 `PUT` |
| Dispatch job batch ingest | one insert per batch | as events | at most 1 `PUT` per client |
| Platform's own events (`platformsink`) | insert inside the business transaction | payload row and event row inside that transaction, always with `data`; the store is never called inside a transaction | none |
| Events projection | copies `data::text` | copies `payload_id` | none |
| Fan-out | reads the data of every claimed event, writes it into every job | reads and writes `payload_id` | none |
| Scheduler and queue | pointer | unchanged | none |
| Delivery (one job per call) | job row with its payload | job row, then its payload row by primary key; a reference is resolved through the cache | 0 or 1 range `GET` |
| Event list | data of every row, from the read table | the page joined to `msg_payloads` by primary key in the same query; an offloaded payload is not fetched | none |
| Event or job detail | one row | resolved on demand | 0 or 1 range `GET` |
| Edit payload | `UPDATE payload` | one new payload row, and the job re-pointed | at most 1 `PUT` |
| Requeue, resend | act on the existing row (as `FindByIDs` describes it; not traced further) | unchanged | none |
| Retention | partition drop | partition drop, `msg_payloads` included, and the store's own expiry (section 7) | none |

Order at ingest: the segment is written first, the rows second. A reference is never inserted before
its object exists. If the insert fails, the segment is unreferenced and expires with everything
else.

The event insert is `ON CONFLICT DO NOTHING` (a duplicate is dropped). The payload row should be
inserted by the same statement and only when the event row was kept, so a dropped duplicate leaves
no payload row behind. One that is left behind anyway is harmless: it is dropped with its partition.

Not in the first build, to be decided on measurement: collecting the oversized payloads of
concurrent small requests for a few milliseconds and writing them as one segment. It matters only
where most ingest is single events.

## 7. Failures and retention

| What happens | What the platform does |
|---|---|
| The store cannot be written at ingest | A payload up to `FC_PAYLOAD_INLINE_MAX_BYTES` goes in its row as `data`; a counter rises and health reports the store degraded. A larger one cannot be kept in the database: its item answers a retryable error and the outbox processor keeps it until the store is back. |
| The store cannot be read at delivery | A platform fault, not the subscriber's: no attempt is recorded or counted, the callback answers as it does for "load failed" and the queue redelivers. |
| The object is gone, or the checksum is wrong | The job fails with `PAYLOAD_MISSING` and is not retried. It is visible in the console. |
| The store cannot be read for a detail read | The record is returned without its data, with `dataUnavailable: true`. |

Retention stays what it is: the partition manager drops partitions older than
`FC_STREAM_PARTITION_RETENTION_DAYS`, and `msg_payloads` joins the tables it keeps
(`stream.PartitionedTables`). The store needs nothing from the platform per row:

- **S3.** S3 has no way to give an object an expiry in the request that writes it. (The `Expires`
  header is a caching hint, and an Object Lock retention date prevents deletion; neither deletes
  anything.) Expiry is always a lifecycle rule on the bucket, and that rule is the infrastructure
  code's (owner, 2026-11-20): expire objects under the store's prefix after the retention plus 35
  days (a monthly partition keeps a row up to 31 days past the retention), 125 days at the default
  retention. The platform neither writes the rule nor checks for it. A deployment that sets
  `FC_PAYLOAD_STORE` to a bucket with no such rule keeps every stored payload for ever.

  ```json
  {
    "ID": "fc-payloads",
    "Status": "Enabled",
    "Filter": { "Prefix": "<prefix>/v1/" },
    "Expiration": { "Days": 125 }
  }
  ```
- **File.** The partition manager's daily pass removes date directories older than the same cutoff.

An offloaded payload lives for the retention plus 35 days from the day it was stored, whatever
refers to it.

Because payloads are a table of their own, they can later be given a shorter retention than the
events and jobs that point at them (a job whose payload has gone shows that it has expired). That is
not in this proposal; it is the cheapest lever left for a very large deployment.

## 8. Size limits and the outbox processor

Found while reading, inferred from the code and not reproduced: the outbox processor sends up to 100
items in one request (`BatchSize: 100`), and the batch routes have the framework's default request
cap of 1 MiB. A batch averaging more than about 10 KB an item is refused as a whole, and the
processor has no path that splits and retries.

- The two batch routes get an explicit cap, `FC_INGEST_BATCH_MAX_BYTES` (16 MiB), registered on the
  route.
- The outbox processor cuts a batch at 100 items or at half that cap, whichever comes first, and
  always sends at least one item.
- An item over the largest payload (`FC_PAYLOAD_MAX_BYTES` with a store, `FC_PAYLOAD_INLINE_MAX_BYTES`
  without) is refused in its own result slot (`PAYLOAD_TOO_LARGE`). The processor must treat that as
  a permanent failure of the item, so it does not block its group by being retried for ever; how it
  handles a `BAD_REQUEST` result today was not checked.
- The cap on an edited payload rises from 256 KiB to the largest payload, so a large payload can be
  corrected. The 256 KiB cap on a function's `dispatch.create` stays.

A payload of up to 5 MB is delivered whole, so the receiver must accept it. The Go framework's
default request limit is 5 MiB (`httpfw.DefaultMaxBodyBytes`, "the same as the Java"), and a delivery
in an envelope is the payload plus the envelope's own fields. That is why the default is 5 000 000
bytes and not 5 MiB: it leaves about 240 KB for the envelope. A subscriber on another stack has to
be sized by whoever subscribes it to an event type with large payloads.

## 9. Schema

### 9.1 Migration 123 (numbered 122 until the merge with main, whose 122 is the field-worker photos)

Tested as written on PostgreSQL 18.6 with the tables of migration 019: it applies twice; the payload
partitions have the bounds of the job partitions of the same month; an event and two jobs read one
payload by primary key; editing one job gives it a row of its own and leaves the event and the other
job on the original; a row with neither or both of `data` and `ref` is refused; the Down section
removes everything.

```sql
-- +goose Up
-- Payloads get a table of their own (docs/payload-offload-design.md,
-- sections 3 and 9). An event and every dispatch job point at a row of
-- msg_payloads; nothing else holds a payload.
--
-- msg_payloads: one row per stored payload, written once and never updated.
--   id          a TSID.
--   created_at  the partition key, and NOT the time the row was written: it
--               is the created_at of the event or job the payload was stored
--               for. So the payload is always read by primary key from the
--               row that points at it, (payload_id, created_at), and it is
--               dropped by retention in the same month as that row.
--   data        the payload as the text the producer sent. NULL when it was
--               offloaded.
--   ref         where the payload is in the payload store when it was
--               offloaded (payloadstore.Ref, opaque to SQL). NULL when it is
--               in `data`.
--   size        the payload's length in bytes as sent.
--   Exactly one of data and ref is set (the CHECK).
--
-- payload_id on msg_events, msg_events_read and msg_dispatch_jobs: the
-- payload of that row, in the partition of the row's own created_at. NULL on
-- a row with no payload, and on every row written before this migration,
-- whose payload is still where it was (msg_events.data,
-- msg_dispatch_jobs.payload); those columns are dropped by a later migration,
-- after one retention period (section 10).
--   - A job the fan-out raises takes its event's payload_id (and has its
--     event's created_at, so it is the same partition).
--   - A job created directly gets a payload row of its own.
--   - Editing a job's payload inserts a new payload row under the job's
--     created_at and points the job at it; the event and the other jobs keep
--     the original.
--
-- Partitions: monthly, named msg_payloads_YYYY_MM like the other messaging
-- tables. One is created here for every month msg_events or
-- msg_dispatch_jobs has a partition, with that partition's own bounds, so a
-- job of an earlier month can still be given an edited payload. From then on
-- the partition manager keeps them (add msg_payloads to
-- stream.PartitionedTables in the same change).
--
-- Cost: one new, empty table and its empty partitions. Three nullable
-- columns with no default, so a catalogue change only (no rewrite, no scan)
-- on the three partitioned parents and every partition; each ALTER takes a
-- brief ACCESS EXCLUSIVE lock, and lock_timeout makes the migration fail fast
-- rather than queue behind a long query (rerun at a quiet moment). No index
-- on payload_id: a payload is read from the row that points at it, never the
-- other way round.
SET LOCAL lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS msg_payloads (
    id         VARCHAR(13) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    data       TEXT,
    ref        TEXT,
    size       INTEGER     NOT NULL,
    PRIMARY KEY (id, created_at),
    CONSTRAINT chk_msg_payloads_data_or_ref CHECK ((data IS NULL) <> (ref IS NULL))
) PARTITION BY RANGE (created_at);

-- +goose StatementBegin
DO $$
DECLARE
    p RECORD;
BEGIN
    FOR p IN
        SELECT DISTINCT ON (suffix) suffix, bound
          FROM (
            SELECT right(c.relname, 7) AS suffix,
                   pg_get_expr(c.relpartbound, c.oid) AS bound
              FROM pg_inherits i
              JOIN pg_class c ON c.oid = i.inhrelid
              JOIN pg_class parent ON parent.oid = i.inhparent
             WHERE parent.relname IN ('msg_events', 'msg_dispatch_jobs')
               AND c.relname ~ '_[0-9]{4}_[0-9]{2}$'
          ) existing
         ORDER BY suffix
    LOOP
        EXECUTE format('CREATE TABLE IF NOT EXISTS %I PARTITION OF msg_payloads %s',
                       'msg_payloads_' || p.suffix, p.bound);
    END LOOP;
END $$;
-- +goose StatementEnd

ALTER TABLE msg_events
    ADD COLUMN IF NOT EXISTS payload_id VARCHAR(13);

ALTER TABLE msg_events_read
    ADD COLUMN IF NOT EXISTS payload_id VARCHAR(13);

ALTER TABLE msg_dispatch_jobs
    ADD COLUMN IF NOT EXISTS payload_id VARCHAR(13);

-- +goose Down
ALTER TABLE msg_dispatch_jobs DROP COLUMN IF EXISTS payload_id;
ALTER TABLE msg_events_read   DROP COLUMN IF EXISTS payload_id;
ALTER TABLE msg_events        DROP COLUMN IF EXISTS payload_id;
DROP TABLE IF EXISTS msg_payloads;
```

The file is not in `internal/migrate/sql` yet. That directory also feeds sqlc, and the table and
columns do nothing until the code of section 10 uses them, so the migration should land in the same
change as that code, with `msg_payloads` added to `stream.PartitionedTables`.

### 9.2 The later migration (a retention period after step 2 below)

```sql
ALTER TABLE msg_events        DROP COLUMN IF EXISTS data;
ALTER TABLE msg_events_read   DROP COLUMN IF EXISTS data;
ALTER TABLE msg_dispatch_jobs DROP COLUMN IF EXISTS payload;
```

A catalogue change. By then every partition that held values in those columns has been dropped, so
there is no space to reclaim. `payload_content_type` stays on the job: an edit may change it.

Changing the type of `msg_events.data` in place is not an option: `ALTER COLUMN ... TYPE` rewrites
every partition under an ACCESS EXCLUSIVE lock (`docs/large-table-schema-changes.md`, "table
rewrite"). A new table and letting the old partitions age out needs no rewrite and no backfill.

## 10. Rollout

Readers first, writers second, as `CONVENTIONS.md` section 6 requires. An instance that has not been
taught `payload_id` would read a new row as having no data and deliver an empty body.

1. **Read both.** Migration 123, and every reader accepts both shapes: a row with a `payload_id` is
   read through `msg_payloads`; a row without one is read as today (`msg_events.data::text`,
   `msg_dispatch_jobs.payload`). Writers are unchanged. Safe to roll back.
2. **Write the new shape**, once no instance older than step 1 is running: ingest writes payload
   rows, the projection and the fan-out copy `payload_id`, an edit inserts a row. Step 1 instances
   read all of it correctly, so a rolling deploy is safe. Rolling back to step 1 is safe; rolling
   back past it is not once rows have been written.
3. **Offload**, per deployment and whenever wanted after step 2: set `FC_PAYLOAD_STORE` and the
   threshold.
4. **Drop the old columns**, one retention period after step 2 (section 9.2).

Steps 1 and 2 can be one release with the writers behind a switch that is turned on once every
instance runs it. That is an implementation choice for whoever builds it.

## 11. What a caller sees

- **Event list responses** gain `dataSize`. A payload held in the database is returned as today. An
  offloaded one is not fetched for a list: `data` is absent and `dataOffloaded` is `true`; the
  detail route returns it.
- **Job responses** keep `payload` on the routes that return it today, resolved; `payloadLength`
  comes from the payload row's `size`.
- **Data-only deliveries** carry the producer's bytes (section 3.2).
- **Batch results** can carry `PAYLOAD_TOO_LARGE` for an item, and a retryable error for an item
  too large for the database while the store cannot be written. A request over the batch cap is
  refused at 16 MiB where it is refused at 1 MiB today.
- **A job** can fail with `PAYLOAD_MISSING`.
- Nothing changes for an SDK or for the outbox table of an application.

`docs/wire-contract.md` and the committed OpenAPI document change with the first two.

## 12. Security and tenancy

- The bucket is private and encrypted at rest; only the platform's own role reads or writes it. No
  presigned URL is ever given to a caller. The role needs `s3:PutObject` and
  `s3:GetObject` under the prefix and `s3:ListBucket` on the bucket limited to it, and nothing that
  deletes or reconfigures. Without `s3:ListBucket` S3 answers a missing key with 403, not 404, and a
  job whose object is gone is retried instead of failing with `PAYLOAD_MISSING`.
- Who may see a payload is decided as today, by the scope of the event or job that points at it. A
  payload id or a reference is not a secret and grants nothing; no route reads `msg_payloads` by id
  alone.
- Payload bytes are never logged. Audit records of a payload edit keep sizes, digests and payload
  ids.
- A segment's key begins with the client id, so one client's stored payloads can be measured and
  removed by prefix.

## 13. Metrics and health

`fc_payload_offloaded_total` and `fc_payload_offloaded_bytes` (raw and stored),
`fc_payload_store_put_seconds`, `fc_payload_store_get_seconds`, `fc_payload_store_errors_total` by
operation, `fc_payload_inline_fallback_total`, `fc_payload_cache_hits_total` and
`fc_payload_missing_total`. Health reports the store as configured and reachable.

## 14. What it saves, and what it leaves

- **Copies.** (2 + N) payload copies become one. Every extra subscription adds a copy today and
  none afterwards.
- **Rewrites.** A payload is no longer written again when its event row or its job row changes
  (section 2's table for the event row; a job row changes more often than an event row does).
- **Large payloads.** Above the threshold, PostgreSQL holds about 200 bytes for a payload (its row
  and primary-key entry, by my arithmetic, not measured) and the store holds one compressed copy.

Left for separate proposals, because they act on the rows and indexes that are not payload:

- an attempt row only for an attempt that failed, and a short cap on a stored response body (today
  up to 64 KiB for every attempt, successes included);
- daily partitions at high volume, so 90 days is 90 days and not up to 120;
- a shorter retention for payloads than for the events and jobs that point at them (section 7);
- a short window in PostgreSQL with older partitions exported to the object store.

## 15. Decisions (owner, 2026-11-20)

| Question | Decided |
|---|---|
| Where a payload lives | A table of its own, `msg_payloads`, that events and jobs point at; an edited job gets its own row. |
| One object per payload, or segments | Segments. |
| Event lists and offloaded data | A list leaves an offloaded payload out and says so; the detail route returns it. |
| Data-only deliveries carry the producer's bytes | Yes; no subscriber depends on the present key order. |
| When the store cannot be written | Keep the payload in the database, up to the 1 MiB the database takes; a larger one waits in the outbox. |
| The largest payload | 5 MB where a store is configured (a planning solve can be that large), and those are offloaded. 1 MiB where there is no store. |
| Who sets an object's expiry | The infrastructure code, as a lifecycle rule on the bucket's prefix. The platform sets and checks nothing. |

## 16. The ports

The Rust and Java ports mirror migration 123 under the large-table rule. The reference format, the
segment layout and the codec are a storage contract between implementations and need a hand-off
document when this is built.

## 17. Build order

On the branch `feat/payload-table`. Each slice compiles, passes `make pre-push` and the integration
tests of the packages it touches, and changes no behaviour until the switch of slice 3 is on.

| Slice | What | Behaviour change |
|---|---|---|
| 1 | Migration 123, sqlc, `msg_payloads` in `stream.PartitionedTables`; the package `payloadstore` (reference, codec, segment packing, the memory, file and S3 stores, the table's reads and writes, the cache, its metrics); configuration and wiring | none |
| 2 | Readers accept both shapes (section 10, step 1): the event and dispatch job repositories, the delivery callback, the event and job responses (`dataSize`, `dataOffloaded`, `dataUnavailable`), `PAYLOAD_MISSING` | none: no row has a `payload_id` yet |
| 3 | Writers (step 2), behind `FC_PAYLOAD_TABLE_WRITES` (default `false`): event ingest, `platformsink`, the projection, the fan-out, direct job creation, the payload edit; offload above the threshold; the size limits | with the switch on |
| 4 | The byte cap on the batch routes and the outbox processor's cut by bytes (section 8) | a batch over 1 MiB is accepted |
| 5 | The wire contract page, the environment variable page, the hand-off for the ports | none |

`FC_PAYLOAD_TABLE_WRITES` is the switch of section 10: off, a new instance writes what an old one
does; on, it writes payload rows. Turn it on once every instance of a deployment runs slice 2 or
later. Its default becomes `true`, and the variable goes, in the release that drops the old columns.

## 18. As built: where the code differs from sections 1 to 17

All five slices of section 17 are built. Two migrations: `123_payload_table.sql` (section 9.1, as
written) and `124_payload_edited_payload_ids.sql` (the two payload ids on the audit event's seeded
schema; one row of a small table). The later migration of section 9.2 is not written.

1. **The largest payload** is 5 MB only when the switch is on, a store is configured and the
   threshold is not 0; otherwise it is 1 MiB. Setting the threshold to 0 therefore lowers it. The
   limit is checked per item on the event and dispatch job routes with the switch on or off; the
   ingestion runtime's routes, with the switch off, keep only their own request cap.
2. **`PAYLOAD_NOT_TEXT`** is new: a payload that is not UTF-8, or holds a NUL byte, is refused per
   item (a TEXT column cannot hold it, and one bad item must not fail a batch).
3. **An event with no data** still gets a payload row, holding `{}`. Section 3.1 said it gets none;
   the platform stores `{}` for such an event today, and a subscriber would otherwise receive `null`.
4. **The payload edit with the switch off** writes the payload column and clears `payload_id`, so
   an edit of a job that has a payload row is not lost. Its cap is the largest payload of item 1.
5. **Request caps** (section 8): besides the batch cap, each route that carries one payload takes
   twice the largest payload plus 64 KiB. An oversized body now answers `413 BODY_TOO_LARGE` on
   every API route, where it answered `400 VALIDATION`; the dispatch job routes had no cap at all.
6. **The outbox processor's cap** is its own setting, `FC_OUTBOX_MAX_REQUEST_BYTES` (8 MiB), to be
   kept at or under half the platform's batch cap. A 413 on several rows is split in two and
   sent again; a 413 on one row is a permanent failure of that row. A row with a message group was
   already sent alone, so neither the cut nor the split can reorder a group.
7. **No `FC_PAYLOAD_STORE_ON_FAILURE`**: the fallback into the database is fixed (section 7).
8. **Delivery** resolves the payload before the claim and again after taking over a stale claim.
   A store that is not configured answers like one that cannot be read (503, no attempt).
9. **A segment's date** is the UTC date of the write. **The package's API** is `Prepare` (packs and
   writes the store, inserts nothing), `Insert` and `QueueInsert`, `Get` and `Resolve`, not the
   `Put(ctx, tx, items)` of section 5; the store is never written while a transaction is open, and
   the platform's own events (written inside a transaction) are always kept in the database.
10. **Health**: `GET /health/payload-store` answers `NOT_CONFIGURED`, `UP` or `DEGRADED` (a store
    write or read failed in the last five minutes), from the outcomes of real operations; `/ready`
    carries the status and never fails because of the store.
11. **The file store's clean-up** runs in the partition manager's pass at the retention plus 35 days
    and removes client directories left empty. It runs only where the partition manager runs, which
    must see the writers' directory.
12. **Validation the design did not state**: with a store, the threshold must be below the inline
    maximum; the maximum must be at least the inline maximum; the batch cap must be at least twice
    the largest payload as configured; a malformed number, codec or boolean stops the server.
13. **`FindByIDs`** returns jobs in id order. The join to `msg_payloads` changed the order rows came
    back in, and a requeue racing a scheduler claim then deadlocked; the persisters lock rows in the
    order they are read.

Not done: a run against real S3; a shared `file://` store across several instances; the Rust and
Java ports; the SQLite and Mongo outbox repositories with the cut, end to end.

