# Java hand-off: the payload table, payload store and request caps

For the Java platform port, and equally for the Rust port: everything below is a storage and wire
contract between implementations that share one database and one payload store. The Go code is
`internal/platform/shared/payloadstore` (the table, the reference, the codec, the stores, the cache,
health), the event and dispatch-job repositories (`internal/platform/event/repository.go`,
`internal/platform/dispatchjob/repository.go`, `lifecycle.go`), the delivery callback
(`internal/platform/dispatchjob/processing/processing.go`), the ingest routes
(`internal/platform/event/api`, `internal/platform/shared/sdk`), the platform sink
(`internal/platform/shared/platformsink`), the stream processor (`internal/stream`) and the outbox
processor (`internal/outbox`). Design: `docs/payload-offload-design.md`. Wire reference:
`docs/wire-contract.md`, the three sections dated 2026-11-20 on payloads and request caps.
Operators: `docs/environment-variables.md`, "Payload store" and "Turning payload storage on".
Every fact here is taken from the Go code as built; section 15 lists where it differs from the
design document.

## 1. What changes, in one paragraph

A payload (an event's data, a dispatch job's payload) is stored once, as the producer's bytes, in a
row of a new partitioned table `msg_payloads`. The event row and every job the event raises point at
that row by `payload_id`; a job created directly, and a job whose payload is edited, get a row of
their own. When a payload store is configured and a payload is larger than a threshold, its row
holds a reference into the store instead of the bytes; the store holds the payloads of one write,
compressed one by one, as one object (a segment). The writers are behind a switch,
`FC_PAYLOAD_TABLE_WRITES`, off by default; the readers accept both shapes whatever the switch says.
A port must ship the readers first (section 7) before any instance turns the switch on.

## 2. Schema

### 2.1 Migration 122 (`122_payload_table.sql`; take the next free number at merge)

Mirror it exactly. It is the file in `internal/migrate/sql`, unchanged from the design's section 9.1:

```sql
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

-- One msg_payloads_YYYY_MM partition for every month msg_events or msg_dispatch_jobs has a
-- partition, with that partition's own bounds (pg_get_expr(relpartbound)), in a DO block:
-- see the Go file for the loop.

ALTER TABLE msg_events        ADD COLUMN IF NOT EXISTS payload_id VARCHAR(13);
ALTER TABLE msg_events_read   ADD COLUMN IF NOT EXISTS payload_id VARCHAR(13);
ALTER TABLE msg_dispatch_jobs ADD COLUMN IF NOT EXISTS payload_id VARCHAR(13);

-- Down: drop the three columns, then the table.
```

Ops notes (`docs/large-table-schema-changes.md`): the three tables are large. The migration is a
catalogue change only: a new empty table and its empty partitions, and three nullable columns with
no default (no rewrite, no scan). Each `ALTER` takes a brief `ACCESS EXCLUSIVE` lock on a
partitioned parent and its partitions; `lock_timeout = '5s'` makes it fail fast instead of queueing
behind a long query: rerun it at a quiet moment. No index on `payload_id`: a payload is read from the
row that points at it, never the other way round. No ops script is needed. The Go effect probe is
`msg_payloads` partitioned and the three `payload_id` columns present.

The partition manager keeps `msg_payloads` like the other messaging tables (it is in
`stream.PartitionedTables`): forward monthly partitions, and the drop of partitions past
`FC_STREAM_PARTITION_RETENTION_DAYS` (the global retention, not the scheduled-job one).

### 2.2 Migration 123 (`123_payload_edited_payload_ids.sql`)

Data only, on the small event-type tables. It replaces the seeded `1.0` JSON schema of
`platform:messaging:dispatch-job:payload-edited` in `msg_event_type_spec_versions` with the seed's
new one, which adds two optional properties, `previousPayloadId` and `payloadId` (each
`{"type":["string","null"]}`, not required; `additionalProperties` stays `false`). It replaces the
schema only while it is still exactly what a seed wrote (the rule of migrations 072 and 091), so a
schema someone changed is left alone. No version bump: the change is additive and optional, as 091's
was. Down puts the old schema back under the same condition. No effect probe (data-only migrations
have none). Idempotent. Section 9.8 says what the fields carry.

### 2.3 The payload row rule

- One row per stored payload, inserted once and never updated.
- `id` is a new untyped TSID (13 characters, Crockford base 32), made by the writer.
- **`created_at` is the owner's**: the `created_at` of the event or job the payload is stored for,
  never the time the row was written. A payload is therefore always read by primary key,
  `(payload_id, created_at)` with the pointing row's own `created_at`, in one partition, and
  retention drops it in the same month as that row. A job the fan-out raises has its event's
  `created_at`, so it reads its event's row in the same partition.
- `data` is the payload as text, byte for byte as received; `ref` is the reference (section 3) when
  it was offloaded; exactly one of the two is set. `size` is the payload's length in bytes as sent
  (before compression), at most 2 147 483 647.
- A payload is accepted into the table only if it is valid UTF-8 and holds no NUL byte (a Postgres
  `TEXT` cannot hold either). This is checked for an offloaded payload too, so whether a payload is
  accepted never depends on its size (`PAYLOAD_NOT_TEXT`).

## 3. The reference (`msg_payloads.ref`)

Exactly as `payloadstore/ref.go` writes and parses it. A reader must refuse anything else.

```
v1:<key>:<offset>:<length>:<codec>:<crc32c>
```

- Six fields split on `:`; any other count is malformed.
- `v1`: literally, lower case. Any other version is malformed.
- `<key>`: the object key in the store, relative to the store's prefix (section 4). 1 to 512
  characters; segments separated by `/`, each non-empty, not `.` or `..`, made only of
  `A-Z a-z 0-9 . _ -`. So a key never holds a colon, a leading or trailing `/`, `//` or a
  backslash.
- `<offset>`, `<length>`: non-negative decimal integers in canonical form: digits only, no sign, no
  leading zero (except `0` itself), at most 19 digits, fitting a signed 64-bit integer. `length` must
  be greater than 0. `offset + length` must not overflow a signed 64-bit integer.
- `<codec>`: `zstd` or `none`, exact case (`ZSTD`, ` zstd` are malformed).
- `<crc32c>`: the CRC-32C (Castagnoli polynomial, as Java's `java.util.zip.CRC32C`) of the
  **stored** bytes, that is the `length` bytes at `offset` as they are in the object (compressed),
  written as exactly 8 lower-case hexadecimal digits, zero-padded (`%08x`).
- The bucket and prefix of the store are not in the reference: they are the deployment's
  configuration, so a bucket can be renamed or copied without touching rows.

Example: `v1:_/2026-11-20/0K9X3QH2M4R7T:0:1873:zstd:0a1b2c3d`. A reference of about 60 to 80
characters.

A malformed reference is read as a missing payload (section 6), logged at error with the payload id.

## 4. The store: keys, segments and the codec

### 4.1 Keys

A segment's key is

```
v1/<client segment>/<yyyy-mm-dd>/<segment id>
```

- `<client segment>`: the client id of the event or job when it is 1 to 64 characters of
  `A-Z a-z 0-9 _ -` (no `.`); otherwise `_`. An empty client is `_`. (`client_id` on the messaging
  tables comes from unchecked SDK input, hence the fallback.)
- `<yyyy-mm-dd>`: the UTC date **of the write** (the clock when the batch is prepared), not the
  owner's `created_at`. One date per write.
- `<segment id>`: a new untyped TSID.

In S3 the object is `s3://<bucket>/<prefix>/<key>` (`<prefix>` trimmed of `/`; no prefix: the key at
the bucket's root). In a file store it is `<dir>/<key>`, the key's `/` mapped to the platform's path
separator. A store must refuse a key outside the grammar of section 3 (the same rule keeps a file
store inside its directory).

### 4.2 Segments

One write (one batch, one request) packs the offloaded payloads of each client into one segment: a
batch with payloads of two clients writes two segments. A segment is the compressed payloads one
after the other, in the order of the batch, with **no header, no index and no padding**: each
reference carries its own offset and length. Each payload is compressed on its own, so the range of
one payload decompresses alone. A segment is written whole (S3 `PutObject`; a file store writes a
temporary file beside the final path, flushes it to disk and renames it into place), so a reader
never sees part of one. Every segment write finishes before any row that references it is inserted;
a segment whose rows are then not inserted (a failed insert, a dropped duplicate, an idempotency key
already held) is unreferenced and expires with the store.

### 4.3 Codec

- `zstd`: one complete zstd frame per payload (Go `klauspost/compress/zstd` `EncodeAll` at its
  default level; any conforming encoder's frame is fine, the level is not part of the contract).
- `none`: the bytes as sent.

The codec of new segments is `FC_PAYLOAD_CODEC`; each reference records its own, so changing it
leaves stored payloads readable.

### 4.4 Reading one payload (resolving a reference)

In this order:

1. The row has `data`: that is the payload. The row has neither `data` nor `ref` (or no row was
   found under `(payload_id, created_at)`): missing.
2. The row has `ref` and no store is configured: "store not configured" (section 6).
3. The cache (section 4.5) by payload id: a hit is the payload.
4. Parse the reference; malformed is missing.
5. Read `length` bytes at `offset` from the key (S3: `GetObject` with
   `Range: bytes=<offset>-<offset+length-1>`; never read more than `length` bytes of the body). No
   such object (S3 `NoSuchKey`, `NotFound` or a bare 404; file not found) is missing. A range past
   the end of the object (S3 416) is read as zero bytes. Any other failure is "store unavailable",
   retryable.
6. The number of bytes read must equal `length`, else missing.
7. The CRC-32C of those bytes must equal the reference's, else missing.
8. Decode with the reference's codec into **at most `size` bytes** (the row's `size`): the decoder
   must never produce more than that, whatever the frame claims (Go: `DecodeAll` with a buffer of
   capacity `size` and `WithDecodeAllCapLimit`, memory bounded to 2^31 - 1). A decode error, or a
   decoded length other than `size`, is missing.
9. Cache it, return it.

A missing payload is counted (`fc_payload_missing_total`) and logged at error with the payload id
and key and the reason, never the bytes. A wrong range must never reach a subscriber.

Note for S3 (documented S3 behaviour, not tested here): a role with `s3:GetObject` but not
`s3:ListBucket` gets `403 AccessDenied`, not 404, for a key that does not exist. The Go store treats
a 403 as a store failure (retryable), not as missing. So the role is given `s3:ListBucket` on the
bucket, limited to the store's prefix; without it a job whose object is gone (removed by hand, or
expired by a lifecycle rule shorter than the retention needs) is retried and never fails with
`PAYLOAD_MISSING`.

### 4.5 Cache

An in-process cache of resolved offloaded payloads, keyed by payload id (a payload row never
changes), least recently used out, bounded by the sum of the payloads' lengths
(`FC_PAYLOAD_CACHE_BYTES`, 64 MiB; 0 disables it). A payload longer than an eighth of the bound is
not cached. It keeps its own copies. It lets the N jobs of one event and a retry share one store
read. Payloads held in the database are not cached (they come with their row).

### Golden vectors

`internal/platform/shared/payloadstore/testdata/payload_vectors.json` holds two segments this
implementation wrote (one per codec) and, for each payload in them, its reference, its size and
the payload itself. A port must resolve every reference of that file to its payload: parse the
reference, take `length` bytes at `offset` of the segment, check the CRC-32C, decode with the size
as the bound, and compare. `TestPayloadVectors` keeps the file in step with the Go code. A port's
own encoder need not produce the same bytes: any standard zstd frame that decodes to the payload
is valid, and the Go reader must be shown one in the port's tests.

## 5. Configuration

Read once at startup, per process. A value that is not a whole number (or not a codec, or not a
boolean) **stops the server**, unlike most numeric variables; so does a combination the rules below
refuse.

| Variable | Default | Rule |
|---|---|---|
| `FC_PAYLOAD_STORE` | empty | Empty: no store. `s3://bucket/prefix` (bucket required; prefix optional, trimmed of `/`; the default AWS credential chain) or `file:///abs/path` (an absolute directory). Anything else stops the server. |
| `FC_PAYLOAD_OFFLOAD_MIN_BYTES` | `5120` | A payload larger than this (strictly) is offloaded. `0` stops offloading. Negative, or 1 to 2047, is refused. With a store, must be less than `FC_PAYLOAD_INLINE_MAX_BYTES` (when not 0). |
| `FC_PAYLOAD_INLINE_MAX_BYTES` | `1048576` | The largest payload kept in the database. Must be positive and at most 2 147 483 647. |
| `FC_PAYLOAD_MAX_BYTES` | `5000000` | The largest payload while offloading. Positive, at most 2 147 483 647; with a store, at least `FC_PAYLOAD_INLINE_MAX_BYTES`. |
| `FC_PAYLOAD_CODEC` | `zstd` | `zstd` or `none`, exact (surrounding spaces of the variable are trimmed first). |
| `FC_PAYLOAD_CACHE_BYTES` | `67108864` | The cache bound; `0` disables it; negative refused. |
| `FC_PAYLOAD_TABLE_WRITES` | `false` | The writers' switch. `1`, `true`, `yes`, `on` / `0`, `false`, `no`, `off`, case-insensitive; anything else stops the server. |
| `FC_INGEST_BATCH_MAX_BYTES` | `16777216` | Request body cap of the batch routes. Positive; must be at least twice the largest payload (below), computed with the actual switch, store and threshold. |
| `FC_OUTBOX_MAX_REQUEST_BYTES` (outbox processor) | `0` = `8388608` | The largest request body the outbox processor sends (section 12). Operators keep it at or under half the platform's `FC_INGEST_BATCH_MAX_BYTES`; not checked by either side. |

**The largest payload** (Go `largestPayload`, the one definition every limit reads):
`FC_PAYLOAD_MAX_BYTES` when, and only when, `FC_PAYLOAD_TABLE_WRITES` is on **and** a store is
configured **and** the threshold is not 0; otherwise `FC_PAYLOAD_INLINE_MAX_BYTES`. So with the
switch off, or no store, or the threshold 0, it is 1 MiB by default, and setting the threshold to 0
lowers it.

Derived:

- **Payload request cap** (a route that carries one payload): `2 × largest + 65536` bytes:
  2 162 688 by default, 10 065 536 while offloading at the defaults.
- **Batch request cap**: `FC_INGEST_BATCH_MAX_BYTES`.

## 6. Errors

| Condition | Go sentinel | Where it surfaces |
|---|---|---|
| Larger than the largest payload | `ErrTooLarge` | `PAYLOAD_TOO_LARGE`, permanent |
| Not UTF-8, or a NUL byte | `ErrNotText` | `PAYLOAD_NOT_TEXT` (ingest), `PAYLOAD_NOT_UTF8` (the edit), permanent |
| Store could not be written and the payload is over the inline maximum; or the store could not be read | `ErrStoreUnavailable` | `PAYLOAD_STORE_UNAVAILABLE` (writes), `PAYLOAD_UNAVAILABLE` (sign), 503 at delivery; retryable |
| Row or object gone, or not matching its reference | `ErrMissing` | `PAYLOAD_MISSING`, never retried |
| A reference with no store configured | `ErrStoreNotConfigured` | delivery `503 "payload store not configured"`; reads as unavailable. No error code of its own reaches a caller. |

## 7. Readers (ship first; correct whatever the switch says)

A row with `payload_id` NULL is the legacy shape: its payload is `msg_events.data` (JSONB, read as
`data::text`), `msg_events_read.data` (text) or `msg_dispatch_jobs.payload`. A row with a
`payload_id` reads the `msg_payloads` row `(payload_id, created_at)` and ignores its own column.

- **Lists never call the payload store.** Select the page first (in a subquery, with its own
  filters, order and limit), then join each row to its payload row by primary key, skipped for a
  legacy row; Go uses `LEFT JOIN LATERAL (SELECT p.data, p.ref, p.size FROM msg_payloads p WHERE
  e.payload_id IS NOT NULL AND p.id = e.payload_id AND p.created_at = e.created_at) p ON TRUE`. Data
  held in the database is returned; offloaded data is left out.
  - `GET /api/events`, `/api/events/list-raw`, `/api/events/raw`, `/bff/events`,
    `/bff/debug/events`: each item gains `dataSize` (bytes: the row's `size`, or for a legacy row the
    length of its stored data; omitted when there is none) and `dataOffloaded: true` (omitted when
    false) with `data` absent.
  - `GET /bff/debug/dispatch-jobs`: `payloadLength` is the payload row's `size` (the payload is never
    in this list).
- **A read of one record resolves** the payload (section 4.4): `GET /api/events/{id}`,
  `/bff/events/{id}`, `/bff/debug/events/{id}`, `GET /api/dispatch-jobs/{id}`, `/{id}/raw`, and the
  job returned by `POST /{id}/cancel`, `/{id}/complete` and `PUT /{id}/payload`. When it cannot be
  produced (unavailable, missing or not configured) the record is still returned, without it:
  `dataUnavailable: true` on an event (beside `dataSize`), `payloadUnavailable: true` on a job;
  logged at error, never a 500. An event's `data` is embedded as a JSON value (compacted, keys in the
  order sent, repeated keys kept); a job's `payload` as a string, byte for byte.
- **`POST /api/dispatch-jobs/{id}/sign`** resolves the payload and builds the plan over it:
  `409 PAYLOAD_MISSING` when it is missing, `500 PAYLOAD_UNAVAILABLE` when it cannot be read.
- The claim, status transitions and operator actions load a job without resolving its payload; only
  delivery, the sign plan, the edit and the detail reads resolve.

## 8. Delivery (`POST /api/dispatch/process`)

- Load the job. If its payload is offloaded, resolve it **before** claiming the job.
  - Store unavailable, or not configured: a platform fault, not the subscriber's. No attempt is
    recorded or counted, the job is not moved, and the callback answers `503 {"ack": false,
    "message": "payload store unavailable"}` (or `"payload store not configured"`), as a failed load
    does, so the queue redelivers.
  - Missing: the job becomes `FAILED` with `completed_at` set and a `last_error` starting
    `PAYLOAD_MISSING` (Go: `PAYLOAD_MISSING: the payload could not be read from the payload store
    (<cause>); not retried`, never payload bytes), no attempt counted, no retry scheduled, nothing
    sent, and the message acked: `200 {"ack": true, "message": "payload missing"}`. Like any failure
    it holds a `BLOCK_ON_ERROR` group until an operator edits the payload and requeues, or cancels.
- **After a stale-claim takeover** (the claim was lost to an attempt whose lease ran out, and this
  call took the claim over), the job is reloaded, so resolve its payload again, with the same
  outcomes; a store that cannot be read then leaves the claim to lapse and the next copy to take it
  over.
- The body: `dataOnly` sends the payload row's bytes exactly (the producer's bytes); the envelope
  parses the payload for `data` as before. The signature is computed over the body sent, unchanged.

## 9. Writers (behind `FC_PAYLOAD_TABLE_WRITES`)

With the switch **off** a writer writes exactly what it wrote before (the payload in the row's own
column, an event's data as JSONB) and never calls the store, with two exceptions that hold with the
switch off too: the largest-payload check of the ingest routes (`PAYLOAD_TOO_LARGE`, at 1 MiB by
default) and the edit's handling of a job of the new shape (9.8). With the switch **on**:

**Prepare** (Go `Service.Prepare`, called before any transaction, never inside one) takes the
payloads of one request, each with its client and its owner's `created_at`, and for each, in order:

1. Larger than the largest payload computed as if the switch were on (`MAX_BYTES` with a store and a
   threshold not 0, else `INLINE_MAX_BYTES`): refused, `ErrTooLarge`.
2. Not UTF-8, or a NUL byte: refused, `ErrNotText`.
3. A new payload id. If a store is configured, the threshold is not 0 and the length is greater than
   the threshold: to be offloaded, grouped by client (clients in order of first appearance).
   Otherwise its row holds the bytes.
4. For each client group: build the segment (4.2), write it. On success each row gets its reference.
   On failure (counted, logged once per call at warn): each payload of the group up to
   `INLINE_MAX_BYTES` is kept in its row as `data` instead (counted in
   `fc_payload_inline_fallback_total`); a larger one is refused, `ErrStoreUnavailable`.

**PrepareInline** (the platform's own events, inside the business transaction): never calls the
store; every row holds the bytes; over `INLINE_MAX_BYTES` is `ErrTooLarge` and fails the write (and
with it the use case); not text is `ErrNotText`.

The rows are then inserted with their owners, each pair in **one statement** so that a payload row
exists only when its owner row was written.

### 9.1 Event ingest (`POST /api/events`, `POST /api/events/batch`, `POST /bff/events/batch`)

1. Each item's data over the largest payload is refused in its own slot (switch on or off).
2. The duplicate query first (by `deduplication_id`, against stored events and earlier items of the
   batch; the first occurrence wins), as before.
3. Prepare the data of the events that remain; an event with no data is given the data `{}` (as the
   legacy shape stores it), so it always gets a payload row and its subscribers still receive `{}`.
4. One pipelined batch, one statement per event:

```sql
WITH e AS (
  INSERT INTO msg_events (id, spec_version, type, source, subject, time, correlation_id,
                          causation_id, deduplication_id, message_group, client_id, context_data,
                          created_at, payload_id)
  VALUES ($1, ..., $12::jsonb, $13, $14)
  ON CONFLICT DO NOTHING
  RETURNING 1
), p AS (
  INSERT INTO msg_payloads (id, created_at, data, ref, size)
  SELECT $14::varchar, $15::timestamptz, $16::text, $17::text, $18::integer FROM e
)
SELECT count(*)::int FROM e
```

`msg_events.data` is NULL. `$13` and `$15` are the same instant (the event's `created_at`). A dropped
conflict inserts no payload row.

Answers: on a batch route each refused item answers in its own slot and the rest are stored:
`BAD_REQUEST` with an error starting `PAYLOAD_TOO_LARGE: ` or `PAYLOAD_NOT_TEXT: `, or
`INTERNAL_ERROR` starting `PAYLOAD_STORE_UNAVAILABLE: ` (retryable). The single route answers
`400 PAYLOAD_TOO_LARGE`, `400 PAYLOAD_NOT_TEXT` or `503 PAYLOAD_STORE_UNAVAILABLE`.

### 9.2 The platform's own events (the sink, inside the use case's transaction)

PrepareInline, then one statement on the transaction: `WITH p AS (INSERT INTO msg_payloads ...)
INSERT INTO msg_events (..., payload_id) VALUES (...)` (no `ON CONFLICT`, as before: a dedup
duplicate fails the transaction). Client segment `_` (no client). The store is never called.

### 9.3 The events projection (`msg_events` to `msg_events_read`)

Copies `payload_id`; copies `data::text` only when `payload_id` is NULL.

### 9.4 The fan-out

Selects `payload_id` with each claimed event and `data` only when `payload_id` is NULL. A job raised
from an event with a `payload_id` gets that `payload_id`, its own `payload` NULL, and the event's
`created_at` (as before). A legacy event's jobs copy its data as before (`null` when it has none).
The fan-out never calls the store and never writes a payload row.

### 9.5 Direct dispatch jobs (`POST /api/dispatch-jobs`, `POST /api/dispatch-jobs/batch`)

1. Each job's payload over the largest payload is refused (switch on or off): the single route
   `400 PAYLOAD_TOO_LARGE`, the batch route in the item's slot.
2. Prepare the payloads of the jobs that remain (a job with no payload gets no payload row and is
   written with neither, as before; a job with no `created_at` gets now). A refusal answers in the
   item's slot (batch) or as in 9.1 (single).
3. Insert the rest (the batch stays all-or-nothing for what is inserted, with the other per-batch
   validations unchanged), each job with its payload row in one statement:

```sql
WITH j AS (
  INSERT INTO msg_dispatch_jobs (..., payload, payload_id, ...) VALUES (..., NULL, $n, ...)
  ON CONFLICT (id, created_at) DO NOTHING
  RETURNING <job row columns>
), p AS (
  INSERT INTO msg_payloads (id, created_at, data, ref, size)
  SELECT $a::varchar, $b::timestamptz, $c::text, $d::text, $e::integer FROM j
)
SELECT <job row columns> FROM j
```

The payload row's `created_at` is the job's. A job whose idempotency key is already held is not
created; its prepared segment, if any, is unreferenced.

### 9.6 A function's `dispatch.create` (`POST /control/functions/dispatch-jobs`) and events (`POST /control/functions/events`)

As 9.5 and 9.1 for one item. The 256 KiB cap of `dispatch.create` stays, before anything else
(`DISPATCH_PAYLOAD_TOO_LARGE`), and the events route keeps the default 1 MiB request cap. Refusals
are `400 PAYLOAD_TOO_LARGE` / `400 PAYLOAD_NOT_TEXT` / `503 PAYLOAD_STORE_UNAVAILABLE`.

### 9.7 The ingestion runtime (`POST /api/ingestion-runtime/v1/ingestions/{id}/jobs`)

Jobs: prepared as 9.5 before the transaction that makes them; events: as 9.1. A record whose payload
is refused makes no job or event and answers in its `results[].error`, starting with the same three
codes; the other records are made. Note: this path does not apply the largest-payload check with the
switch off (only its own request cap, `FC_INGESTION_JOBS_MAX_BYTES`, 10 MiB).

### 9.8 Editing a job's payload (`PUT /api/dispatch-jobs/{id}/payload`, `/bff/dispatch-jobs/{id}/payload`)

- Validation: `PAYLOAD_REQUIRED`; over the largest payload `400 PAYLOAD_TOO_LARGE` (switch on or
  off; with it off the largest is always the inline maximum); not UTF-8 `400 PAYLOAD_NOT_UTF8`; JSON
  content type and not JSON `400 PAYLOAD_NOT_JSON`; status not `PENDING` / `FAILED` / legacy `ERROR`
  `409 NOT_EDITABLE`.
- The payload it replaces is resolved for the audit event's `previousSize` / `previousSha256`. Store
  unavailable: `500 PAYLOAD_STORE_UNAVAILABLE` (try again). Missing, or not configured: the edit goes
  on (it is how a `PAYLOAD_MISSING` job is repaired), with `previousSize` the payload row's `size`
  and `previousSha256` empty.
- **Switch on**: Prepare the new payload under the job's `created_at` and client (a refusal:
  `400 PAYLOAD_TOO_LARGE`, `400 PAYLOAD_NOT_UTF8`, or `500 PAYLOAD_STORE_UNAVAILABLE`), then one
  statement:

```sql
WITH j AS (
  UPDATE msg_dispatch_jobs
     SET payload = NULL, payload_id = $4, payload_content_type = COALESCE($3, payload_content_type),
         updated_at = NOW()
   WHERE id = $1 AND created_at = $2 AND status IN (<editable statuses>)
  RETURNING 1
), p AS (
  INSERT INTO msg_payloads (id, created_at, data, ref, size)
  SELECT $4::varchar, $5::timestamptz, $6::text, $7::text, $8::integer FROM j
)
SELECT count(*)::int FROM j
```

  The event and the other jobs that shared the old row keep it, and the old row stays. `updated_at`
  moves, so a claim that read the job before the edit is refused, as before.
- **Switch off**: `UPDATE msg_dispatch_jobs SET payload = $3, payload_id = NULL, ...` with the same
  guard. Clearing `payload_id` is required: a reader prefers `payload_id`, so a job of the new shape
  would otherwise keep reading its old row and lose the edit.
- **The audit event** `platform:messaging:dispatch-job:payload-edited` carries, beside
  `dispatchJobId`, `status`, `previousSize`, `previousSha256`, `size`, `sha256`, `contentType`,
  `previousContentType`, two optional fields (migration 123): `previousPayloadId`, the payload row the
  job pointed at before the edit (absent when its payload was the legacy column), and `payloadId`, the
  new row (absent with the switch off). Never the payload.

## 10. Retention and clean-up

- Rows: `msg_payloads` partitions are dropped with the others, by the partition manager.
- **S3: the platform never deletes.** The infrastructure sets a lifecycle rule on the bucket:
  objects under `<prefix>/v1/` expire after the partition retention plus 35 days (a monthly partition
  keeps a row up to 31 days past the retention): 125 days at the default 90. The platform neither
  writes nor checks it. Its role needs `s3:PutObject` and `s3:GetObject` under the prefix, and `s3:ListBucket` on the bucket
  limited to the prefix (the note in section 4 says why).
- **File store: the partition manager's daily pass** (the leader of the stream processor) removes,
  under `<dir>/v1/`, every date directory `<client segment>/<yyyy-mm-dd>` whose date is before the
  UTC date of `now - (retention + 35 days)`, then every client directory left empty. It never touches
  anything that does not match the layout: an entry under `v1` that is not a directory named as a
  client segment is (4.1), a date directory not named exactly `yyyy-mm-dd`, a symbolic link, anything
  outside `v1`. That process must see the same directory as the writers.

## 11. Request caps and the 413 change

- The batch routes (`POST /api/events/batch`, `/bff/events/batch`, `POST /api/dispatch-jobs/batch`):
  `FC_INGEST_BATCH_MAX_BYTES`.
- The single-payload routes (`POST /api/events`, `POST /api/dispatch-jobs`,
  `PUT /api/dispatch-jobs/{id}/payload` and its `/bff` twin, the console's payload edit form): the
  payload request cap (section 5).
- **A body over a route's cap is `413`** with `{"error": "BODY_TOO_LARGE", "message": "request body
  is too large limit=<n> bytes"}`. This applies to **every huma route** (every other route keeps the
  1 MiB default cap): before, an oversized body there answered `400 VALIDATION` with the same
  message. The two dispatch-job routes, plain handlers that had no cap, answer the same.
- Unchanged: the ingestion runtime's caps, the function routes' caps, the 256 KiB of
  `dispatch.create`.

## 12. The outbox processor

- The ungrouped rows of one item type in a claim are sent in requests cut, in order, at
  `FC_OUTBOX_BATCH_SIZE` rows (100) or `FC_OUTBOX_MAX_REQUEST_BYTES` bytes of request body (8 MiB),
  whichever comes first; a request always carries at least one row, so a larger row goes alone. The
  body is `{"items":[<item>,<item>,...]}`, each item as the processor encodes it; the byte count is
  that exact body. A grouped row is always sent on its own, so the cut never changes a group's
  order. A row whose payload is not JSON is a terminal `BAD_REQUEST` and is not sent.
- The requests of a cut batch go one after the other. When one fails as a whole in a way that holds
  the batch (a transport error, a 5xx, a `401`, any unmapped status, a `429`, the leadership lost),
  the later ones are not sent: their rows go back to `PENDING` untouched.
- **`413`** on a request of several rows: split it in two and send the halves in order, the second
  only if the first did not fail as a whole; recursively. A split is not a failure. **`413` on one
  row**: the platform will never take it; a terminal `BAD_REQUEST` with the message `PAYLOAD_TOO_LARGE:
  the item (<n> bytes as sent) is larger than the platform accepts in one request (HTTP 413)`, stored
  with the `[blocking] ` marker, blocking its group under block-on-error.
- Per-item results: `BAD_REQUEST` (including `PAYLOAD_TOO_LARGE` and `PAYLOAD_NOT_TEXT`) is terminal;
  `INTERNAL_ERROR` (including `PAYLOAD_STORE_UNAVAILABLE`) is retried with the usual backoff.
- Keep `FC_OUTBOX_MAX_REQUEST_BYTES` at or under half the platform's `FC_INGEST_BATCH_MAX_BYTES`.

## 13. Health and metrics

- `GET /health/payload-store` on the API listener of a process that runs the platform API: no
  authentication, always `200`, JSON `{status, configured, kind, reachable, offloading,
  lastSuccessAt, lastFailureAt, lastFailure, windowSeconds}`. `status`: `NOT_CONFIGURED` (no store,
  healthy), `UP`, or `DEGRADED` (a store write or read failed within the last 300 seconds; a write
  failure means payloads were kept in the database or refused). `reachable`: `true` when the last
  outcome was an answer from the store (a missing object or a checksum mismatch is an answer),
  `false` when it was a failure, absent while nothing has been tried. `offloading`: switch on, store,
  threshold not 0. `lastFailure`: `put`, `get` or `startup`. `kind`: `s3`, `file` (never the bucket,
  prefix or directory). The metrics listener's `/ready` carries `"payload_store": "<status>"` (absent
  in a process without the platform API).
- A health request never calls the store. One check at startup: the file store makes its directory
  and a temporary file in it and removes it (success or failure recorded); the S3 store records a
  failure only when its credential chain does not resolve. A store that is down never makes the
  process unready and never fails liveness.
- Metrics (Prometheus, no key, id or payload in a label): `fc_payload_offloaded_total`,
  `fc_payload_offloaded_bytes{form="raw"|"stored"}`, `fc_payload_store_put_seconds`,
  `fc_payload_store_get_seconds`, `fc_payload_store_errors_total{operation="put"|"get"}` (a missing
  object is not an error here), `fc_payload_inline_fallback_total`, `fc_payload_cache_hits_total`,
  `fc_payload_missing_total`.

## 14. Not built

- Collecting the offloaded payloads of concurrent small requests into one segment (one segment per
  client per request only).
- A threshold per client or per event type.
- A retention for payloads shorter than for their events and jobs.
- The later migration that drops `msg_events.data`, `msg_events_read.data` and
  `msg_dispatch_jobs.payload`, and the switch's removal (one retention period after the switch is on
  everywhere).
- Any deletion or lifecycle configuration of S3 by the platform; presigned URLs; a route that reads
  `msg_payloads` by id alone; deduplication of identical payloads.
- A console page for the store's health (the JSON endpoint only).

## 15. Differences from the design document

- **The largest payload** depends on the switch and the threshold too: 5 MB only with the switch
  on, a store and a threshold that is not 0; otherwise 1 MiB. Setting the threshold to 0 lowers it.
  The 1 MiB check holds with the switch off on the event and dispatch-job routes (not on the
  ingestion runtime).
- **`PAYLOAD_NOT_TEXT`** (and `PAYLOAD_NOT_UTF8` on the edit): a payload that is not UTF-8 or holds a
  NUL byte is refused, because the payload table stores text. Not in the design.
- **No `PAYLOAD_STORE_NOT_CONFIGURED` code** reaches a caller: delivery answers 503 `"payload store
  not configured"`, reads set `dataUnavailable` / `payloadUnavailable`.
- **An event with no data** gets a payload row holding `{}` (the design said no payload row and a NULL
  `payload_id`).
- **Request caps on the single-payload routes** (twice the largest plus 64 KiB) and **413
  `BODY_TOO_LARGE` on every huma route** (was 400 `VALIDATION`). The design named only the batch cap.
- **The edit**: its cap is the largest payload as defined above (the inline maximum with the switch
  off); store unavailable answers `500`, not 503; with the switch off it clears `payload_id`; the
  replaced payload is read from the store for the audit digest, and a missing one does not block the
  edit.
- **The audit event** carries the ids as two optional fields, `previousPayloadId` and `payloadId`,
  through migration 123 without a schema version bump.
- **Single routes** answer a store that cannot be written with `503 PAYLOAD_STORE_UNAVAILABLE`, batch
  items with `INTERNAL_ERROR`; the design said only "a retryable error".
- **Delivery** resolves the payload again after a stale-claim takeover (the design has one resolution
  point).
- **The outbox processor's byte cap** is its own setting, `FC_OUTBOX_MAX_REQUEST_BYTES` (8 MiB), not
  derived from the platform's cap; a `413` is split in halves, and a single row's `413` is terminal
  `PAYLOAD_TOO_LARGE`.
- **The segment's date** is the UTC date of the write, not the owner's `created_at` (for an edit of
  an old job these differ).
- **The service API** is `Prepare` (pack and write the store, insert nothing) plus `Insert` /
  `QueueInsert` and `Get` / `Resolve`, not `Put(ctx, tx, items)`: the store is never written with a
  transaction open.
- **Health** is a status with a 5-minute window (`NOT_CONFIGURED`, `UP`, `DEGRADED`), reachability
  from real operations and one startup check, at `GET /health/payload-store` and in `/ready`; the
  design said only "configured and reachable" and "degraded".
- **The file store's clean-up** uses the retention plus 35 days (the S3 rule's figure), not the
  partition cutoff alone, and also removes empty client directories.
- **Validation** adds rules the design did not state: the threshold must be below the inline maximum
  with a store, the maximum at least the inline maximum, the batch cap at least twice the largest
  payload as actually configured, and a malformed number, codec or boolean stops the server.
