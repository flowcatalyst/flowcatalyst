# HTTP ingestion parity with the ingest functions, and the event target

*Owner rulings 2026-10-31. Spec for implementation; the rulings are the main session's, the work is
delegated in three parts (A platform, B ingestor runtime, C mono-repo).*

## 0. Why

`mono-repo` has two TypeScript functions that are only "authenticated POST → one dispatch job":
`client-code/bevco/functions/yard-trip-ingest` and
`client-code/merensky/functions/tower-trip-dispatch-ingest`. An HTTP ingestion on the ingestor's
listener (`POST /ingest/{client}/{code}`) already does that with idempotency, rate limiting, traces
and pause. They move to HTTP ingestions; **both URLs stay live until the functions are shut down**.
At the same time every ingestion gains an **event** target (one platform event per record, fanned
out by subscriptions) next to today's dispatch-job target.

## 1. Rulings

| # | Ruling |
|---|---|
| R1 | The listener keeps answering **202**. The body is `{"accepted":true,"jobId":"…"}` when the ingestion has no split (`split.kind` none) and `{"accepted":true,"jobIds":["…",…]}` (record order) when it splits. For an event target the keys are `eventId` / `eventIds`. A resend with the same `Idempotency-Key` answers the same ids. |
| R2 | The listener honours `X-Message-Group` and `X-Correlation-Id`. |
| R3 | An HTTP source can restrict content types and check the body (415/400 as the functions do). |
| R4 | A dispatch-job target gains `dataOnly` (deliver the raw record) next to today's envelope (the default, unchanged for existing ingestions). No new ingestion-specific wrapper. |
| R5 | The platform envelope gains `time` and `dataContentType`. A raw (`dataOnly`) delivery sends the job's own content type, not always `application/json`. |
| R6 | Target `kind`: `DISPATCH_JOB` (default, today) or `EVENT`. |
| R7 | Event data is JSON. A record that is not JSON is wrapped as `{"contentType": "<type>", "body": "<text>"}` **only when the target says `wrapNonJson: true`**; when the records cannot be known to be JSON, saving requires it, so the wrapping is always configured, never implicit. |
| R8 | The merensky processor accepts both its old wrapper and the platform envelope; the bevco processor is unchanged (raw XML via `dataOnly`). |

## 2. Contract

### 2.1 Ingestion definition (Go `internal/platform/ingestion`, JSON and the definition file)

**HTTP source** gains an optional `accept`:

```json
"source": { "kind": "http", "auth": {…}, "rateLimit": {…},
            "accept": { "contentTypes": ["application/xml", "text/xml"], "body": "XML" } }
```

- `contentTypes`: media types (no parameters), compared case-insensitively against the request's
  media type with parameters (`; charset=…`) ignored. Absent or empty = any.
- `body`: `NONE` (default) | `JSON` (parses) | `JSON_OBJECT` (a non-empty JSON object, not a
  scalar, array or `{}`) | `XML` (well-formed; parsed with DTDs and external entities disabled).
- Validation at save: each content type is `type/subtype`; `body` one of the four.

**Target** gains `kind`, `dataOnly`, `wrapNonJson`:

```json
"target": { "kind": "DISPATCH_JOB", "code": "…", "targetUrl": "…", "pool": "…", "connection": "…",
            "mode": "IMMEDIATE", "batchSize": 100, "dataOnly": true }
"target": { "kind": "EVENT", "code": "<event type code>", "batchSize": 100, "wrapNonJson": true }
```

- `kind` absent = `DISPATCH_JOB` (every stored definition stays valid and unchanged). Stored and
  marshalled with `kind` present from now on is fine; reading must accept it absent.
- `DISPATCH_JOB`: as today, plus `dataOnly` (bool, default **false** = envelope). `wrapNonJson` is
  refused.
- `EVENT`: `code` is an **event type** code that must exist (resolved at save like pool/connection;
  unknown → the same `INGESTION_TARGET_INVALID` style refusal naming `target.code`). `batchSize` as
  today. `targetUrl`, `pool`, `connection`, `serviceAccount`, `mode`, `dataOnly` are refused
  (subscriptions decide delivery). A config entry's `targetUrl` does not apply to an event target.
- `wrapNonJson` (bool, default false) for `EVENT`. **Required true** unless the records are known
  JSON: split `json`, or split none with an HTTP source whose `accept.body` is `JSON` or
  `JSON_OBJECT`. Otherwise saving is refused with: `target.wrapNonJson: these records are not known
  to be JSON; set wrapNonJson to true to deliver each as {"contentType", "body"}`.

### 2.2 Runtime jobs route (`POST /api/ingestion-runtime/v1/ingestions/{id}/jobs`)

Request unchanged (`JobRecord`: payload, payloadContentType, messageGroup, externalId,
correlationId, subject). Response gains `kind`:

```json
{ "kind": "DISPATCH_JOB", "created": 1, "results": [ { "index": 0, "id": "…", "existing": false } ] }
```

- `DISPATCH_JOB`: as today, and the batch item's `DataOnly` is the target's `dataOnly`.
- `EVENT`: one event per record through the same insert path `POST /api/events/batch` uses, so the
  normal fan-out to subscriptions happens: type = target code, source = the source string ingestion
  jobs use today, subject = record subject, client = ingestion client, messageGroup and
  correlationId from the record, deduplication id = `ing:{ingestionId}:{externalId}` when the
  record has an externalId. `data`: the record when its content type is JSON (`application/json`
  or `+json`) and it parses; else, with `wrapNonJson`, `{"contentType": payloadContentType or
  "text/plain", "body": payload}`; else that record's result is an error ("the record is not JSON
  and the target's wrapNonJson is off"). A record whose deduplication id already exists answers
  the **existing** event id with `existing: true` (the insert is ON CONFLICT DO NOTHING today: look
  the existing row up). `results[].id` is the event id.

### 2.3 Delivery (`internal/platform/dispatchjob/processing`, `buildPayload` and the request)

- Envelope: add `"time"` = the job's creation time (UTC, RFC 3339 with milliseconds, the format
  the platform's other timestamps use on the wire) and `"dataContentType"` = the job's payload
  content type when set. Additive; nothing else in the envelope changes.
- `Content-Type` of the request: envelope → `application/json` (unchanged); `dataOnly` → the job's
  `PayloadContentType` when non-empty, else `application/json`.
- Grep for any other place that renders a delivery body or envelope (`"attemptNumber"`) and keep
  them consistent.

### 2.4 Listener (Java `ingestor/ingestor-runtime`, `HttpIngest` and `IngestionRoute`)

Order of checks: route → auth (unchanged) → rate limit (unchanged) → **accept** → headers → body.

- `accept.contentTypes` mismatch → `415 {"error":"UNSUPPORTED_MEDIA_TYPE"}`.
- Empty body → `400 {"error":"EMPTY_BODY"}` (always, once `accept` is set; unchanged otherwise).
- `accept.body`: `JSON` invalid → `400 INVALID_JSON`; `JSON_OBJECT` not a non-empty object →
  `400 PAYLOAD_MUST_BE_A_NON_EMPTY_JSON_OBJECT`; `XML` not well-formed → `400 INVALID_XML` (with a
  short parser message, no payload echo). The check streams; it must not hold a second copy of a
  large body beyond what the route already holds.
- `X-Message-Group`: when present must match `^[A-Za-z0-9_.:-]{1,128}$`, else `400
  INVALID_MESSAGE_GROUP`. Every record of the payload gets group `{ingestionId}:{value}` (the
  ingestion prefix keeps groups of different ingestions apart). Absent → today's group.
- `X-Correlation-Id`: when present at most 100 characters, else `400 INVALID_CORRELATION_ID`; it
  becomes every record's `correlationId`. Absent → today's (the payload group).
- Answer per R1, from the jobs answer's `kind` and result ids, in record order; on a split
  payload that made no records, `jobIds: []`. Everything else about 202/503 is unchanged.
- Contract (`ingestor-contract`): `Source.Http` gains `@Nullable Accept accept` (record `Accept(List<String> contentTypes, Body body)`, enum `Body {NONE, JSON, JSON_OBJECT, XML}`); `JobsAnswer` gains `@Nullable String kind` (absent = `DISPATCH_JOB`). The runtime's answer reader already ignores unknown properties; check that an ingestion document with `accept` is read by the current runtime build and that one without it still is.

### 2.5 mono-repo (part C)

- `client-code/bevco/ingest/yard-trip.yaml`: HTTP, FlowCatalyst token auth for application
  `bevco-integration` (the rule `src/access.ts` applies), `accept: {contentTypes: [application/xml,
  text/xml], body: XML}`, no split, target `DISPATCH_JOB` with the function's job code
  (`src/names.ts` `DESCRIPTOR` or whatever code its jobs carry), pool `yard-process-pool`,
  connection `bevco-yard`, `mode: IMMEDIATE`, `dataOnly: true`, `targetUrl` from the per-environment
  settings entry the way `mix-positions.yaml` names one (`config:`), pointing at
  `yard-trip-processor`'s delivery path.
- `client-code/merensky/ingest/tower-trip-dispatch.yaml`: same pattern, application
  `merensky-integration`, `accept: {contentTypes: [application/json], body: JSON_OBJECT}`, pool
  `tower-pool`, connection `merensky-tower`, envelope (`dataOnly` false).
- Settings entries under `settings/np` and `settings/prod` for each, like the existing ones. The
  processor's URL is the function runner URL + `/fn/{address}/{path}`: take the runner base URLs
  from `../iac` (read only). **Do not touch** the existing, uncommitted
  `client-code/bevco/ingest/settings/np/mix-positions.json` and `webiq-raw-records.json`.
- `merensky/functions/tower-trip-dispatch-processor`: accept both shapes. Old wrapper:
  `{source: "merensky-odoo", contentType, receivedAt, record}` → `record`. Platform envelope (has
  `id`, `type` and `data`) → `data`. Anything else → reject as today. Tests for both and for the
  rejections.
- The two ingest functions are not changed or removed.

## 3. Tests (each part)

- A: definition validation (every refusal above, `kind` absent reads as `DISPATCH_JOB`), file
  round trip, jobs route for both kinds including wrap / no-wrap / idempotent resend for events,
  envelope `time` + `dataContentType`, raw delivery content type, an event created by an ingestion
  reaching a subscription's dispatch job (integration). `make api-bump` if the OpenAPI lock changes.
- B: listener: 415, each 400, header acceptance and refusal, group and correlation on the records,
  the four answer shapes (`jobId`, `jobIds`, `eventId`, `eventIds`), idempotent resend answering the
  same id.
- C: processor dual shape; the YAML files load with `fcdev ingest apply --dry-run` if a platform is
  not needed for that, otherwise by the definition file reader's own test path.

## 4. Out of scope

Shutting the functions down; promoting the ingestions in nonprod/prod; the Java platform mirror;
SPA changes.
