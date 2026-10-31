# Ingestion

An **ingestion** takes data from wherever a system keeps it, splits each payload
into records, and turns every record into a **dispatch job** (or, if the target says so,
a platform **event**). It is the inbound
twin of subscriptions: a subscription delivers events out, an ingestion brings
data in and hands it to the same delivery machinery.

The platform stores, versions and governs ingestions. It does not read sources
itself: a separate **runtime** does (see "Runtimes" below). An installation
without a runtime simply has no ingestions running.

Ingestion is behind a flag, `FC_INGESTION_ENABLED` (see "Turning it on").

## The model

```
ingestion = (application, client, code) + source + split + target + flags
```

| Field | Meaning |
|---|---|
| `application` | The owning application, by code. **Required, and of type `INTEGRATION`.** It carries the permissions, the audit scope and the answer to "who owns this". |
| `client` | **Required.** A client id (`clt_…`) or identifier. Data that is not one client's goes under a client the business keeps for the purpose; there is no client-less ingestion. |
| `code` | Stable, lower-case, unique within `(application, client)`. |
| `name`, `active` | A label, and whether the ingestion runs. `active` has three states in a file: `true`, `false`, or not said. When it is not said, an ingestion whose source names a settings entry is created **paused** (its settings and secrets are set after the apply, and resuming checks they are there) and any other is created active; an ingestion that already exists keeps whatever state it is in. When it is said, every apply enforces it. |
| `keepFailedPayloads` | Off unless the client has agreed. When on, a failure may carry the first 2 KB of the payload that failed. |
| `source` | Where the data comes from. |
| `split` | How a payload becomes records. |
| `target` | The dispatch job (or the platform event) each record becomes. |

An ingestion is addressed as **`application/client/code`** (for example
`ingest/acme/orders-sftp`) by `fcdev ingest`, or by its `ing_…` id.

### Source kinds

| Kind | Reads |
|---|---|
| `http` | Data pushed to the runtime over HTTP (see "HTTP ingestions") |
| `sftp`, `s3` | Files in a directory or bucket, polled |
| `sqs`, `rabbitmq` | Messages from a queue |
| `database-poll`, `api-poll` | A table or an HTTP API, polled on an interval |
| `custom` | A kind a runtime adds as an extension (`kind: custom`, `type`, `settings`) |

An `http` source may set `accept`, `rateLimit` and one of three `auth` kinds; the endpoint, the
checks, the answers and the headers are described under "HTTP ingestions".

`GET /api/ingestion-source-kinds` lists the built-in kinds and the ones the live
runtimes report. A kind that needs a special permission (a Camel route is one)
declares it, and saving an ingestion of that kind needs it (`platform:ingestion:ingestion:route`).
The platform remembers what a runtime last declared for its kind, so the
permission still applies while no runtime that reports the kind is running.

**Camel routes are written by a pipeline or by an ingestion administrator.** A
route runs with the runtime's network, identity and environment, so the route
permission counts only for an anchor-tier caller (below it a `camel-route`
ingestion is refused, `INGESTION_ROUTE_ANCHOR_REQUIRED`), and only for:

- a **pipeline's service account** holding `platform:ingestion-route-author`,
  which pushes routes through the API as drafts (it holds no promote) that an
  administrator makes live; or
- an **ingestion administrator** (`platform:ingestion-admin`), who pushes routes
  with `fcdev ingest apply` or the API.

A person who holds the route permission without being an administrator is
refused (`INGESTION_ROUTE_PIPELINE_REQUIRED`), and an operator, who holds no
route permission, cannot write or change a route at all.

A source never holds a secret, only its **name** (`credential: {name: sftp-password}`).

**What a source may point at.** Every address a source makes the runtime connect
to (an `api-poll` URL and its login or token URL, an `sftp` host, a RabbitMQ URI,
a database's JDBC URL, a URL in an extension's settings) is checked when the
ingestion is saved and when it is tested. Private networks are allowed (an
on-premises database is the normal case); the runtime's own machine (loopback,
`localhost`), link-local addresses, the cloud metadata services
(`169.254.169.254`, `metadata.google.internal` and the like), unspecified and
multicast addresses, and hosts written as bare numbers (`2130706433`) are not. A
JDBC URL must name its server (`jdbc:<driver>://host:port/...`, or Oracle's
`@host:port`); an embedded database, a file or a TNS alias is refused, and so are
driver properties that load code or read files on the runtime (`socketFactory`,
`allowLoadLocalInfile` and similar). The runtime checks the address it actually
connects to as well.

**Two more permissions guard what the runtime holds:**

- An `sqs` or `s3` source without a `credential` is read with the **runtime
  fleet's own AWS identity**. Saving one (new, or with a different source than
  the live version) needs `platform:ingestion:ingestion:runtime-identity`. Name an
  access key in `credential` to read with the client's own.
- Naming a secret makes the runtime present it to the host the definition names,
  so naming one is reading it. Saving a definition that names secrets, when it is
  new or differs from the live version in where it points (a host, a URL, a JDBC
  URL) or in which secrets it names, needs
  `platform:ingestion:ingestion:bind-secrets`. For an extension kind every text
  setting counts as a secret name, so any change of its settings counts. A change
  elsewhere (a directory, a name, the target) needs neither.

**An sftp source checks who the server is.** Give its `knownHosts` (the server's
known_hosts line), or set `acceptAnyHostKey: true` (not both). The runtime refuses
to start an sftp source with neither and reports it `FAILED`; the platform saves
it but answers with a warning, shown on the ingestion. Accepting any host key
sends the credential to whoever answers at the address, so turning it on needs
`platform:ingestion:ingestion:bind-secrets`.

### Split

| Kind | Records |
|---|---|
| `none` | The payload is one record |
| `csv` | One per row; `header: true` when the first row names the columns |
| `json` | One per element of the array at `arrayPath` |
| `xml` | One per `element` |

### Target

| Field | Meaning |
|---|---|
| `kind` | `DISPATCH_JOB` (the default when absent) or `EVENT`: what each record becomes |
| `code` | For `EVENT`, the code of an existing event type. For a job, the dispatch job's code, `application:subdomain:aggregate:action`. Its first part must be the ingestion's own application: an ingestion makes jobs of its application only |
| `targetUrl` | Where the job is delivered. Checked at save with the platform's outbound-URL guard (no private or metadata addresses). May be left out only when the source names a config entry whose value gives it (see "Settings per environment"): exactly one of the two |
| `pool` | A dispatch pool **code**; it must exist and be usable by the ingestion's client |
| `connection` | Optional connection code. The connection's service account signs each delivery, and pausing the connection holds this ingestion's jobs |
| `serviceAccount` | Optional service account code, to sign without a connection. Not together with `connection`. It must be one of the ingestion's application's own accounts and reach the ingestion's client (an account linked to no client reaches all of its application's clients); a platform account, or another application's, is refused |
| `mode` | `IMMEDIATE`, `NEXT_ON_ERROR` or `BLOCK_ON_ERROR` |
| `batchSize` | Records per job batch |
| `dataOnly` | `DISPATCH_JOB` only, default false. Deliver the record itself, with its own content type, instead of the platform envelope |
| `wrapNonJson` | `EVENT` only, default false. Deliver a record that is not JSON as `{"contentType", "body"}`. **Required** unless the records are known to be JSON: a `json` split, or split `none` on an HTTP source whose `accept.body` is `JSON` or `JSON_OBJECT` |

An `EVENT` target stores one platform event per record, through the same path as
`POST /api/events/batch`, so subscriptions to the event type deliver it. It takes
only `code`, `batchSize` and `wrapNonJson`: `targetUrl`, `pool`, `connection`,
`serviceAccount`, `mode` and `dataOnly` are refused, because the subscriptions decide
where the event goes. A record with an `externalId` is stored once per ingestion:
sending it again answers the first event.

With neither `connection` nor `serviceAccount` the delivery is unsigned. The job's
client is the ingestion's client. Anything that does not resolve is a `400` when
the ingestion is saved, not a failure in the runtime later.

### Settings per environment

A definition file is the same in every environment, so what differs per
environment or per client is kept in the platform's configuration of that
environment, and the source **names** it instead of holding it. A source holds
its settings inline **or** names a config entry, never both:

```yaml
source:
  kind: mix              # a built-in kind (kind: api-poll, kind: sftp, ...) or an extension's
  config: mix-positions  # the entry's key; or `config: true`, the ingestion's own code
```

In JSON the reference is `{"kind": "api-poll", "config": "mix-positions"}` for a
built-in kind and `{"kind": "custom", "type": "mix", "config": "mix-positions"}`
for an extension's. The key follows the rules of a code. A source with `config`
holds nothing else; any other setting beside it is a validation problem at
`source.config`.

The entry is platform configuration of the ingestion's **application**, section
`ingest`, property = the key, scope `CLIENT` = the ingestion's client, type
`PLAIN`. Its value is a JSON object: the kind's settings in the shape an inline
source has (a built-in kind's fields without `kind`; an extension kind's
`settings` themselves), and optionally `targetUrl`. Secrets stay where they are,
in section `secrets` at the client; a secret setting in the value holds the
secret's **name** (see "Secrets are entered as values" below: the settings form,
the API and fcdev take the value and name the secret for you). The delivery address is in the file's `target.targetUrl` **or**
in the value's `targetUrl`, exactly one. Several ingestions of one application
and client may name one key (one login for two feeds); the value must then suit
each of them.

Wherever the platform uses a definition to do something it uses the **effective
definition**, the reference with the entry's settings in place: the poll (a
runtime is given the effective source and never sees `config`), the secrets call
(the secrets the effective live source names), the jobs call (its delivery
address), tests, and the drawer. An ingestion whose entry is **missing** or
**invalid** is not listed to any runtime, takes no lease and no jobs, and shows as
needing configuration (`config.status` `MISSING` or `INVALID` on the ingestion,
with the problems); it starts by itself once the entry is valid.

- **Saving a definition** that names a missing entry saves it, with a
  `CONFIG_MISSING` warning in the answer (create, update, save version, and each
  item of a set's plan), so applying a set never depends on someone having entered
  the environment's values first. When the entry exists, the effective definition
  is validated and judged as an inline one is: the outbound guard, `bind-secrets`
  when it points secrets somewhere new, `runtime-identity`, and the kind's
  permission, against the effective live definition.
- **An entry is written only through its ingestion.** `PUT /api/ingestions/{id}/config`
  with `{"value": {...}}` (and `DELETE`, `GET`) needs the permission to update the
  ingestion, also when a set owns it: the entry is the environment's part of it, not
  the set's. It is checked for **every** ingestion of that application and client whose
  live or latest saved version names the key: `Validate` of the effective definition,
  the kind's schema, and the same authoring checks as a save, with the effective
  definition under the old value as the live one and under the new value as the next.
  A problem names the ingestion; nothing is written unless all pass. The answer lists
  the ingestions it was checked against. A write is audited on the ingestion (the
  value is not in the audit row) and emits `platform:ingestion:ingestion:config-set`;
  it is not a version of the ingestion. `DELETE` makes the ingestions that name the
  entry `MISSING` and lists them (`platform:ingestion:ingestion:config-deleted`).
- **Secrets are entered as values.** In the settings form every secret setting is a
  password input with a Set / Not set tag; leave it blank to keep the value there is.
  The platform names the secret: the name the entry already holds for that setting, else
  `<entry key>-<setting in kebab case>` (key `mix-positions`, `clientSecret`:
  `mix-positions-client-secret`; a nested setting joins its segments). The API takes
  `PUT .../config` with `secretFields` (`{"<setting path>": "<value>"}`, the platform
  names it) or `secrets` (`{"<secret name>": "<value>"}`, a name the value uses) beside
  `value`; the entry and its secrets are written in one transaction, stored as `SECRET`
  values at the ingestion's client. `GET .../config` lists `secrets: [{field, name, set,
  updatedAt}]` for a caller who can update ingestions (names and state, never a value).
  A value you supply needs no `bind-secrets`; a name without a value, or an address
  change that keeps the secrets, still does. The JSON editor used for a kind no
  runtime reports keeps secret settings as names.
- **The kind's schema.** For a built-in kind it is the definition itself (strict
  decoding and `Validate`; an unknown setting is a problem). For an extension kind it
  is the field list a live runtime reports for it (type, required, choices, pattern,
  bounds, list counts), checked by the platform. With no live runtime reporting the
  kind the value is stored with a `SCHEMA_UNCHECKED` warning (`schemaChecked: false`
  in `GET .../config`), and the runtime checks it before it builds the route.
- **The generic configuration API and page refuse section `ingest`** (`400
  CONFIG_SECTION_RESERVED`) for a write or a delete; reading it is as for any section, except that the value is shown only to a caller who can update ingestions (`ingestion:update`): everyone else sees the entry's status and problems, `GET .../config` answers `valueHidden: true` without `value`, and the generic config reads show `***`. What the entry supplies is withheld the same way: the drawer's where, delivery URL and secrets, `GET .../secrets` (`namesHidden: true`, no names) and the warnings that describe the entry's settings.

## HTTP ingestions

An `http` source is a push endpoint: a system posts a payload to the runtime and the
runtime turns it into dispatch jobs (or events) **before it answers**. The endpoint is
served by the ingestor runtime (port `8091` by default; under `fcdev`,
`http://127.0.0.1:8091`), not by the platform.

### The endpoint

```
POST /ingest/{client}/{code}
POST /ingest/{application}/{client}/{code}
```

`{client}` is the client's identifier or its platform id (`clt_…`); `{code}` is the
ingestion's code. The short form is enough while only one application has an
ingestion of that code for the client; when two do, it answers
`409 AMBIGUOUS` and the caller posts to the application-qualified form. An unknown
ingestion, or one that is not an `http` source, is `404`. Only `POST` is served.

The checks run in this order, and the first to fail answers: the ingestion, the
login, the rate limit, `accept` (media type), the headers, the body.

### Who may post (`source.auth`)

| `auth.kind` | The caller sends | What is checked |
|---|---|---|
| `bearer` | `Authorization: Bearer <token>` | The token equals the secret named in `token` (`{name: …}`, a secret of the ingestion's client; see "Secrets") |
| `basic` | `Authorization: Basic <base64 user:password>` | The user name equals `username` in the definition and the password equals the secret named in `password`. A user name may not contain `:` |
| `flowcatalyst-token` | `Authorization: Bearer <platform access token>` | The token verifies against the platform's signing keys, and its principal may act for **the ingestion's client** and may use the application named in `application` (a code). `principalId` (`prn_…`) narrows it to one principal. A service account's `sac_` id is not in its tokens: name its principal |

A missing or wrong credential is `401`. A valid platform token whose principal lacks
the client, the application or the named principal is `403 FORBIDDEN`. If the platform's
keys or the secret store cannot be read at that moment the answer is `503` (send it
again). The secret is read through the runtime's secrets call and kept for a short
while, so a rotated secret is in force within about a minute.

### What the request may be (`source.accept`)

```yaml
source:
  kind: http
  auth: {…}
  accept:
    contentTypes: [application/xml, text/xml]
    body: XML
```

- `contentTypes`: media types as `type/subtype`, compared case-insensitively with the
  request's `Content-Type`, parameters such as `charset` ignored. Absent or empty
  accepts any. Anything else is `415 UNSUPPORTED_MEDIA_TYPE`; so is a request with no
  `Content-Type` when a list is given.
- `body` is checked before the payload is split, and a failure is `400`: see the table below.
- XML is parsed with DTDs and external entities off, and **a `DOCTYPE` is refused
  outright** (`INVALID_XML`, "a DOCTYPE is not accepted"): an integration payload has no
  need of one, and the body is passed on to a processor whose own parser this check
  cannot vouch for. The refusal says where a body is malformed (line and column),
  never what it says.
- Once `accept` is set, a blank body is `400 EMPTY_BODY`. Without `accept` a blank body
  makes no records (the answer is `202` without an id).

| `body` | Accepts | Refusal |
|---|---|---|
| `NONE` (default) | anything | |
| `JSON` | one JSON value | `INVALID_JSON` |
| `JSON_OBJECT` | one JSON object with at least one property (not a scalar, an array or `{}`) | `INVALID_JSON`, or `PAYLOAD_MUST_BE_A_NON_EMPTY_JSON_OBJECT` |
| `XML` | well-formed XML with a root element | `INVALID_XML` |

Every refusal has the body `{"error": "<CODE>"}`, with a `message` added where there is
something to say (the media types taken, where the XML is malformed).

### Request headers

| Header | Meaning |
|---|---|
| `Idempotency-Key` | Names the request. Each record is keyed by it and its position, so **sending the same request again makes no new jobs or events** and the answer carries the same ids. Without it, a request sent again makes everything again. |
| `X-Message-Group` | 1 to 128 of `A-Z a-z 0-9 _ . : -`, else `400 INVALID_MESSAGE_GROUP`. Every record of the payload gets the group `{ingestionId}:{value}`: the ingestion's id is put in front so that two ingestions that use the same value never share a group. Without it the group is `{ingestionId}:` and a hash of the `Idempotency-Key` (else of the payload), so a request sent again lands in the same group |
| `X-Correlation-Id` | At most 100 characters, else `400 INVALID_CORRELATION_ID`. It becomes every record's correlation id. Without it the correlation id is the record's group |

### The answers

| Status | Body | Meaning |
|---|---|---|
| `202` | `{"accepted": true, …}` | The platform has the jobs (or events). **Only now is it safe to forget the payload** |
| `503` | `{"error": "UNAVAILABLE"}` | The platform has none of them, or only some: **send the same request again** (with an `Idempotency-Key`, nothing is made twice) |
| `422` | `REFUSED` or `UNPROCESSABLE` | The platform refused the records for good, or the payload could not be read: do not send it again unchanged |
| `429` | `{"error": "RATE_LIMITED"}` with `Retry-After` | See "Rate limits" |
| `400`, `401`, `403`, `404`, `409`, `415` | | As above |

The `202` names what was made, in record order:

| Target | Split `none` | Any other split |
|---|---|---|
| dispatch job | `{"accepted": true, "jobId": "…"}` | `{"accepted": true, "jobIds": ["…", …]}` |
| event | `{"accepted": true, "eventId": "…"}` | `{"accepted": true, "eventIds": ["…", …]}` |

A payload that makes no records has no ids to name: the list is empty, or the answer is
`{"accepted": true}` alone (a blank payload). A resend under the same
`Idempotency-Key` answers the **same** ids.

### Rate limits

```yaml
rateLimit: {requests: 120, per: PT1M}     # inside source:
```

`rateLimit` allows at most `requests` per `per` (1 to 1,000,000 requests, `per` from 1 ms to
1 day) to one ingestion; beyond it the answer is `429 RATE_LIMITED` with `Retry-After`
(seconds). A caller is counted only **after** it has logged in, so a stranger cannot use up
an ingestion's budget. The count is each runtime's own: with several runtimes behind a
load balancer each allows the full limit.

Separately, failed logins are limited. An address that fails to log in to one ingestion
more than 20 times in a minute is answered `429` (with `Retry-After`) **without its
credentials being checked** until the wait is over. Other addresses and other ingestions
are not held up. This applies to every ingestion, with or without `rateLimit`. Only a
refusal the caller caused counts: a `503` because the platform's keys or the secret store
cannot be read does not, so an outage locks nobody out.

### What the record becomes

Whatever the source, a record is one piece of the payload (the whole body for split
`none`). Its content type is what the split makes of it: `application/json` for `json`,
`application/xml` for `xml`, `text/csv` for `csv`. With split `none` the record is the
whole body, so it keeps the **`Content-Type` its HTTP caller sent** (as sent, parameters
included, such as `application/xml; charset=UTF-8`); `application/json` when the caller
sent none, or one over 100 characters, and for a source other than HTTP.

**Target `DISPATCH_JOB`** (the default). The record becomes a dispatch job and is delivered
to `targetUrl` as in "Delivery" (messaging page):

- **envelope** (`dataOnly: false`, the default): the platform's JSON envelope, the record in
  its `data` (parsed when it is JSON, else as a string), with `dataContentType` naming the
  record's content type and `time` the job's creation time;
- **`dataOnly: true`**: the record itself, byte for byte, with the job's content type as
  the request's `Content-Type`. The envelope's fields travel as headers
  (`X-Dispatch-Job-Id`, `X-Event-Type` (the job's code), `X-FlowCatalyst-Client`, and the signature headers).

**Target `EVENT`.** The record becomes one platform event of the event type named in
`target.code`, stored through the same path as `POST /api/events/batch`, so **the event
type's subscriptions deliver it** and there is no `targetUrl`, pool or connection. The
event's source is `ingestion:{application}:{code}`, its client is the ingestion's, its
subject, message group and correlation id come from the record. Event `data` is JSON:

- a record whose content type is JSON and that parses is the data as it is;
- any other record is wrapped as `{"contentType": "<type>", "body": "<text>"}`, **only when
  the target says `wrapNonJson: true`**; without it that record is refused (its result is an
  error and nothing is made for it);
- `wrapNonJson` is therefore **required** at save unless the records are known to be JSON:
  a `json` split, or split `none` on an HTTP source whose `accept.body` is `JSON` or
  `JSON_OBJECT`. The platform refuses the definition otherwise (`target.wrapNonJson: these
  records are not known to be JSON; set wrapNonJson to true…`). It is never implicit.

An event is deduplicated per ingestion when its request carried an `Idempotency-Key`: the
event's deduplication id is `ing:{ingestionId}:` followed by a hash of the key and the record's position, and a resend answers the
first event's id instead of storing a second.

### Example: XML posted by a partner, delivered raw to a processor

`POST /ingest/bevco/yard-trip` with the caller's own FlowCatalyst token. Each posted XML
document is one dispatch job, delivered unchanged to the function that processes it.

```yaml
application: bevco-integration       # an INTEGRATION application; also the first part of target.code
client: bevco
code: yard-trip
name: Yard trip (XML)
source:
  kind: http
  auth:
    kind: flowcatalyst-token
    application: bevco-integration     # the caller's token must reach this application and client bevco
  accept:
    contentTypes: [application/xml, text/xml]
    body: XML
  rateLimit: {requests: 600, per: PT1M}
split:
  kind: none                           # the posted body is the one record
target:
  kind: DISPATCH_JOB
  code: bevco-integration:function:yard-trip-ingest:dispatch
  targetUrl: https://functions.example.com/fn/bevco-integration.yard-trip-processor/deliver
  pool: yard-process-pool              # a dispatch pool code
  connection: bevco-yard               # a connection code; its service account signs the delivery
  mode: IMMEDIATE
  batchSize: 1
  dataOnly: true                       # deliver the XML itself, not the envelope
```

```
curl -X POST https://ingest.example.com/ingest/bevco/yard-trip \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/xml" \
  -H "Idempotency-Key: trip-4711" --data-binary @trip.xml
{"accepted":true,"jobId":"…"}
```

A definition that is the same in every environment keeps `auth`, `accept`, `rateLimit` and
the delivery address out of the file: the source names a settings entry instead
(`source: {kind: http, config: yard-trip}`) and the entry holds
`{"auth": {…}, "accept": {…}, "targetUrl": "…"}` per environment (see "Settings per
environment").

### Example: JSON records posted by an application, delivered as events

`POST /ingest/acme/orders` with a bearer token held as a secret. Each order in the posted
array becomes an `shop:orders:order:received` event that the event type's subscribers
receive.

```yaml
application: shop-integration
client: acme
code: orders
name: Orders (push)
source:
  kind: http
  auth:
    kind: bearer
    token: {name: orders-push-token}   # a secret of client acme, set on the platform
  accept:
    contentTypes: [application/json]
    body: JSON
split:
  kind: json
  arrayPath: $.orders                  # one record per element of the array
target:
  kind: EVENT
  code: shop:orders:order:received     # an existing event type
  batchSize: 100
  wrapNonJson: false                   # known JSON (a json split), so not required
```

```
curl -X POST https://ingest.example.com/ingest/acme/orders \
  -H "Authorization: Bearer $PUSH_TOKEN" -H "Content-Type: application/json" \
  -H "X-Message-Group: tenant-7" -H "Idempotency-Key: batch-2026-10-31-1" \
  -d '{"orders": [{"id": 1}, {"id": 2}]}'
{"accepted":true,"eventIds":["…","…"]}
```

Sending the same request again (same `Idempotency-Key`) answers the same two event ids and
stores nothing new.

## Definition files and sets

An ingestion is also a **definition file**: YAML with the fields above plus
`application` and `client`.

```yaml
application: ingest
client: acme
code: orders-sftp
name: Orders over SFTP
source:
  kind: sftp
  host: sftp.example.com
  directory: /in/orders
  username: feed
  credential:
    name: sftp-password
  knownHosts: sftp.example.com ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAA...
  every: PT5M
split:
  kind: csv
  header: true
target:
  code: ingest:orders:order:received
  targetUrl: https://hooks.example.com/orders
  pool: orders
  connection: orders-conn
  mode: NEXT_ON_ERROR
  batchSize: 100
```

A file may hold several definitions separated by `---`; keep one per file. An
extension's source is written flat (`kind: camel-route`, then its settings).
Unknown fields are errors. A file is the same in every environment: what
differs between environments or clients (a vendor's login, an organisation id,
the delivery address) is not in the file but in the platform's configuration of
that environment (see "Settings per environment").

A folder of files is applied as one **set**. Applying a set makes its
ingestions what the files say. For each definition:

| Plan | When |
|---|---|
| `create` | No ingestion has that `(application, client, code)` |
| `unchanged` | The set owns it and it says the same |
| `change` | The set owns it and it differs: saved as the next live version. `active` is set from the file only when the file says it; otherwise a pause or resume made since stays |
| `adopt` | It was made by hand: taken into the set (a new version only if it differs) |
| `remove` | The set owns it but no longer lists it: **deleted**, with its versions, leases and state |

An apply is one transaction. An ingestion owned by another set is a
`409 INGESTION_SET_OWNED`, and any unresolvable definition fails the whole apply;
nothing is written. A dry run does every check and writes nothing.

While a set owns an ingestion it is **read-only** (update, save version, make
live, delete); pause and resume still work. It changes by changing its file and
applying again. Releasing the set ends ownership and changes nothing else.

Each apply that changed something is kept with the files it was given, the
newest 10 per set. Re-applying one goes back to it as a new apply.

## Versions, live, pause and resume

Every change saves a **version**. The **live** version is the one runtimes
receive: a saved version does not run until it is made live. A version that is
not live and was saved after the live one (or while nothing is live) is a
**draft**: it waits for someone who holds `platform:ingestion:ingestion:promote`
to make it live.

Making a version live needs promote, however it happens. Without it a save is
**a draft, never a refusal**:

- a create (`POST /api/ingestions`) without promote makes an ingestion whose
  first version is a draft, so nothing runs until it is made live, whatever
  `live` says;
- an update (`PUT /api/ingestions/{id}`) without promote saves the definition
  as a draft; with promote it is saved and made live;
- `POST /api/ingestions/{id}/versions` saves a draft, or a live version when it
  says `live: true` (which needs promote).

A save is **deduplicated by the digest** of the definition (sha256 of its
canonical form, shown on each version as `digest`): a definition the live
version already says changes nothing; one a draft already says saves nothing
new and answers that draft (or, with promote, makes that draft live instead of
saving a copy). The answer of a create or update carries `save`: what it did
(`SAVED`, `MADE_LIVE`, `DRAFT`, `DRAFT_EXISTS` or `UNCHANGED`), the version, and
whether it awaits promote. The pages say "Saved as a draft; someone with promote
makes it live", mark drafts on the Versions tab, and the history shows a draft
as a draft. A set apply makes its versions live under `set:apply`, as before.

**Pause** stops an ingestion running; **resume** starts it again. Both need
`update`. A paused ingestion keeps its versions and state.

## Failures and notices

A runtime reports what went wrong. Each failure has a **category**:

| Category | Means |
|---|---|
| `payload` | A record or a payload the runtime could not take (the usual kind) |
| `rejected` | The platform refused the runtime's jobs for good: the target no longer resolves (a pool suspended, a connection gone), the client is suspended. Nothing more is made until someone puts it right |

A `rejected` failure is a **notice**: the ingestion's row in the list and its
drawer show how many there are and the newest one's message until someone
dismisses them. Every failure can be dismissed
(`POST /api/ingestions/{id}/failures/{failureId}/dismiss`, or Dismiss on the
Failures tab), which needs `update` and the ingestion's reach and is recorded in
the ingestion's history. A dismissed failure stays listed, marked, until
retention removes it. `GET .../failures?undismissed=true&category=rejected`
lists the open notices.

Failures and exchange counts are kept for 30 days and job idempotency keys for a
week, counted from when the platform recorded them, not from times the runtime
sent. A time from a runtime that is more than five minutes in the future is
recorded as now.

## Traces

"The ingestion is running but nothing shows." A **trace** answers it: an operator
switches tracing on for an ingestion and sees, for a minute or so, what each runtime's
polls and exchanges did, with the HTTP calls an extension or an `api-poll` made and
their requests and responses. Tracing is **never on by default**; switched off it costs
a runtime a map lookup per poll or exchange, and it never blocks, slows or fails
ingestion.

- **A window per ingestion, a capture per runtime.** Switching on opens a new
  *window* (`windowSecs`: 60, 300 or 900; `sampleEverySecs`: 1 to 60, default 5).
  A runtime learns of it at its next sync (a minute in production, 15 s under
  `fcdev`) and captures for `windowSecs` **from the moment it first sees the
  window**, so a one-minute window is a minute of capture however long the sync
  took. The platform keeps a **hard stop**, the switch-on time plus the window plus
  a pickup grace (`FC_INGESTION_TRACE_PICKUP_GRACE`, 2 minutes), and refuses records
  dated after it (10 seconds of slack). Switching off sets the hard stop to now.
- **Sampling.** A runtime starts a capture of an ingestion only if its last one
  started `sampleEverySecs` or more ago. A capture is **one poll cycle** (a polled
  source's poll, everything it called and dispatched) **or one exchange** (a push,
  file or queue message), captured whole.
- **Empty polls show.** A captured poll that found nothing is recorded. Without
  tracing too, every runtime reports how many polls it finished and when the last
  ended, so Activity shows "polling, nothing new" (`polls` in the counters, `Last
  poll`) instead of nothing.
- **Records** are `poll`, `exchange`, `http` and `error`, grouped by capture. Bodies are
  kept to 32 KiB; credentials are redacted before a record leaves the runtime:
  headers and query values whose name contains `auth`, `key`, `token`, `secret`,
  `password`, `signature`, `cookie` or `session`, the same fields in JSON and form
  bodies, user info in URLs, and the secret values the runtime fetched for the
  ingestion wherever they appear verbatim. Records are kept 7 days
  (`FC_INGESTION_TRACE_RETENTION_DAYS`) and are deleted with the ingestion.
- **Permissions.** `platform:ingestion:trace:manage` switches tracing on and off;
  `platform:ingestion:trace:view` reads a record's metadata (an HTTP call's client,
  method, URL path, status, duration and sizes; an exchange's counts and outcome; a
  poll's counters); `platform:ingestion:trace:view-bodies` reads headers, query strings,
  bodies, payload excerpts and error text, and is also needed to switch tracing on.
  `platform:ingestion-admin` holds all three, `platform:ingestion-operator` holds
  `view`; the viewer and the pipeline's route author hold none. A reader without
  `view-bodies` gets records marked `redacted: true` with those parts withheld.
- **Audit.** Switching on and off emit `platform:ingestion:trace:enabled` and
  `platform:ingestion:trace:disabled` (off only when a live window ended), which read
  in the ingestion's history.

| Call | Needs | Does |
|---|---|---|
| `GET /api/ingestions/{id}/trace` | `trace:view` | `{live, windowId, windowSecs, sampleEverySecs, enabledAt, hardStop, enabledBy, maxWindowSecs}`; the fields are null when it was never on |
| `PUT /api/ingestions/{id}/trace` `{windowSecs, sampleEverySecs}` | `trace:manage` and `trace:view-bodies` | New window; `400 TRACE_WINDOW_INVALID` / `TRACE_SAMPLE_INVALID` |
| `DELETE /api/ingestions/{id}/trace` | `trace:manage` | Hard stop = now; idempotent; `204` |
| `GET /api/ingestions/{id}/trace-log?kind&runtimeId&captureId&since&until&limit&cursor` | `trace:view` | Newest first, keyset `nextCursor`, limit 100 (at most 500) |

The **Activity page** (`/ui/ingestions/{id}/activity`, also "Activity" in the list's row
menu and "Open activity" in the drawer's Activity tab) shows the counters for the last
24 hours and 7 days (polls, completed, failed, records, jobs), the last poll, exchange
and failure, the runtimes, the trace switch, and the records, each capture with its
HTTP calls and exchanges nested and each call expanding to its request and response.
It refreshes every 5 seconds while a window is live.

## Test connection

A test tries a source once, from a runtime's own network and with its own
secrets, and dispatches nothing. It is **pull-based**: the request is queued on
the platform, and a runtime that carries the source's kind takes it in its next
poll (they poll about every minute), so **a result takes up to a minute**.

| Status | Meaning |
|---|---|
| `PENDING` | No runtime has taken it yet |
| `RUNNING` | A runtime is trying the source |
| `DONE` | Answered, with an outcome: `RESULTS`, `NO_RESULTS`, `REFUSED`, `FAILED` or `NOT_SUPPORTED`, the steps taken, and how many records the split makes |
| `EXPIRED` | No runtime answered in time |

While no live runtime carries the kind the test carries a notice saying so; the
source has not failed, nothing is trying it. The result never contains a secret.
An excerpt of the answer appears only for an ingestion that keeps failed
payloads. A test needs `platform:ingestion:ingestion:test`, because it makes a
real outbound call.

## Permissions and roles

| Permission | Grants |
|---|---|
| `platform:ingestion:ingestion:view` | List, read, versions, history, failures, definition export, source kinds, and the secret names an ingestion binds with when each runtime last fetched them (never a value) |
| `platform:ingestion:ingestion:create`, `update`, `delete` | Change definitions; `update` includes pause and resume |
| `platform:ingestion:ingestion:promote` | Make a version live: a create that is live, an update that is live, a version saved live, make live. Without it those saves are drafts |
| `platform:ingestion:ingestion:test` | Run test connection |
| `platform:ingestion:ingestion:route` | Create or change an ingestion of a kind that declares it needs this (`camel-route`): at anchor tier, from a pipeline's service account or an ingestion administrator only; removing one needs only `delete` |
| `platform:ingestion:ingestion:runtime-identity` | Save an `sqs` or `s3` source the runtime reads with its own AWS identity |
| `platform:ingestion:ingestion:bind-secrets` | Save a definition that names secrets and points them somewhere new |
| `platform:ingestion:set:apply` | Apply or release a set |
| `platform:ingestion:trace:manage`, `view`, `view-bodies` | Switch tracing on and off; read trace records (metadata); read their headers, bodies and error text and switch tracing on (see Traces) |
| `platform:ingestion:runtime:operate` | The runtime API: poll, report, state, jobs, secrets |

Seeded roles: `platform:ingestion-admin` (everything but the runtime's, route
included, and the only role with `runtime-identity`),
`platform:ingestion-operator` (view, update, test and trace view: it cannot write routes),
`platform:ingestion-viewer`, `platform:ingestion-route-author` (the **pipeline**
role, for a service account: view, create, update, route and bind-secrets; it
writes drafts that an administrator makes live, and a user given it cannot write
a route), and `platform:ingestion-runtime` for a runtime's service account.

**Reach.** A caller below anchor tier sees and changes only the ingestions of its
own clients. An application-scoped service account reaches only its own
application's ingestions. Listing sets and their applies needs `view` and a
caller that reaches every client and application, because a set spans them.

## Secrets

A definition names secrets and never holds one. A runtime reads a named secret as
the ingestion's **client's** value in the platform's configuration (see
"Platform configuration"), through the runtime API's own secrets call
(`GET /api/ingestion-runtime/v1/ingestions/{id}/secrets/{name}`). That call
answers only the secrets the ingestion's live definition binds, to a runtime
that carries the kind and, for a polled source, holds the lease, for a live
ingestion of an active client. The runtime's service account needs
`platform:ingestion-runtime` and nothing else: no config-reveal permission. It
always reads at the ingestion's own client, with no fall-back to a global value,
so one client's ingestion cannot read another's. Set the secret for each client
that needs it before the ingestion runs.

A secret's value is set on the platform configuration page (application = the ingestion's, section `secrets`, property = the name, client = the ingestion's, type SECRET), not through the ingestion. An editor's drawer lists each secret the ingestion binds as Set or Missing, with a link to that property's add or edit form, and resuming an ingestion whose secrets are missing is refused with their names.

**A runtime's fetch is not a config reveal and is not audited per read.** A
runtime re-reads each secret about once a minute per route, so a fetch is
recorded as **security telemetry** instead:

- a structured log line at INFO, `security.ingestion_secret_fetch`, with
  `ingestionId`, `applicationCode`, `clientId`, `secretName`, `runtimeId`,
  `principalId` and `version` (never the value);
- the counter `fc_ingestion_secret_fetch_total{outcome}` (`success`, `refused`,
  `error`) on the metrics endpoint;
- a **last fetched** record per ingestion, secret and runtime (when, under which
  version, how many times), shown in the ingestion's drawer (Secrets) and by
  `GET /api/ingestions/{id}/secrets`; it goes once the runtime has been gone for
  7 days.

A **refused** fetch (a name the live definition does not bind, no value at the
ingestion's client, no lease where one is needed, an inactive client, a paused
ingestion) is still audited, as `SECRET_FETCH_REFUSED` on the ingestion (it
reads in the ingestion's history), and logged at WARN with the reason.

## Runtimes

A **runtime** is the program that actually reads sources. It is a separate,
optional component with its own release: it polls the platform, runs the
ingestions it is given, reports their state and failures, keeps small values
(a watermark, a cursor) through the platform's state calls, and creates the
dispatch jobs as its service account. The platform never calls it, so a
runtime can sit behind a firewall.

- A runtime reports the source kinds it carries: the built-in ones it has
  (`builtinKinds`; all of them when it does not say) and the kinds its
  extensions add. It is given only ingestions of those kinds. An ingestion of a
  kind no live runtime reports shows as "no runtime carries this kind", which is
  not a failure.
- Polled sources (`sftp`, `s3`, `database-poll`, `api-poll`, `custom`) run on one
  runtime at a time, under a lease. `http`, `sqs` and `rabbitmq` run on every
  runtime that carries them. The polled ingestions of each kind are shared
  evenly over the runtimes that carry that kind, so a runtime that alone carries
  a kind runs all of it.
- The ingestions of a suspended or deactivated client run nowhere and take no
  jobs.
- The runtime's account must be a **service account at anchor tier that belongs
  to no application** (linked to no client, and not confined to a list of
  applications, which an application's own account always is), holding
  `platform:ingestion-runtime`. Anything else is refused
  on every runtime call, even with the role: a person
  (`INGESTION_RUNTIME_SERVICE_ACCOUNT_REQUIRED`), a client-tier account
  (`INGESTION_RUNTIME_ANCHOR_REQUIRED`), an application's account
  (`INGESTION_RUNTIME_APPLICATION_BOUND`). `fcdev` provisions such an account
  for its own runtime.

## Turning it on

`FC_INGESTION_ENABLED` is `false` by default, and `true` under `fcdev`. Off, the
ingestion routes are not mounted (`404`), the navigation entry is absent, and the
tables exist but stay empty. The permissions and roles are seeded either way.
The runtime API is mounted only when the flag is on.

A runtime's jobs request takes up to 10 MiB (`FC_INGESTION_JOBS_MAX_BYTES`) and
its report up to 4 MiB (`FC_INGESTION_REPORT_MAX_BYTES`); its other calls take
1 MiB. The retention sweep deletes up to 500 chunks of 10,000 rows of each
high-volume table an hour (`FC_INGESTION_RETENTION_MAX_CHUNKS` raises it).

## fcdev ingest

The commands use `fcdev` profiles (`--profile`, or `FC_PROFILE`), as `fcdev fn`
does, and print the connection banner on stderr first. Every command that
changes something asks for confirmation on a prod profile, and in CI needs
`--yes` and `allow_noninteractive = true` in the profile (see "Using fcdev in CI").
`--json` prints one JSON object on stdout and moves the human text to stderr. An
error is exit code `1`. Refer to an ingestion as `application/client/code` or by id.

| Command | Does |
|---|---|
| `fcdev ingest list [--application A] [--client C] [--q text] [--json]` | List ingestions |
| `fcdev ingest get <ref> [--json]` | Show one |
| `fcdev ingest apply <folder \| file...> [--set name] [--dry-run] [--json]` | Apply the files as a set |
| `fcdev ingest export <ref>... \| --all [--out dir] [--application A] [--client C]` | Write definition files to `<dir>/<application>/<client>/<code>.yaml` |
| `fcdev ingest versions <ref>` | List versions, the live one marked |
| `fcdev ingest live <ref> <version>` | Make a version live |
| `fcdev ingest pause <ref>`, `resume <ref>` | Stop or start an ingestion. Resume is refused (`409`) while the settings entry the source names is not set (`INGESTION_CONFIG_NOT_SET`) or a secret it binds has no value (`INGESTION_SECRET_MISSING`, naming the secrets) |
| `fcdev ingest test <ref \| file> [--wait] [--timeout d]` | Queue a test; `--wait` polls and prints the steps |
| `fcdev ingest config get <ref> [--json]` | Show the settings entry the ingestion's source names: key, status, problems, the value |
| `fcdev ingest config set <ref> <file.json \| -> [--secrets] [--json]` | Write the entry from a JSON object (a file, or stdin with `-`). `--secrets` asks (hidden, on the terminal) for the value of each secret the entry uses, Enter keeps the one there is, and writes them with the entry; it is refused when stdin holds the file and is not a terminal |
| `fcdev ingest config delete <ref> [--json]` | Delete the entry; the ingestions that name it stop |
| `fcdev ingest trace on <ref> [--for 1m\|5m\|15m] [--every 5s]` | Switch tracing on with a new window (default 1m, one capture every 5s) |
| `fcdev ingest trace off <ref>` | Switch tracing off now |
| `fcdev ingest trace status <ref>` | Whether tracing is live, its window, sampling and time left |
| `fcdev ingest trace log <ref> [--kind poll\|exchange\|http\|error] [--runtime id] [--capture id] [--since d] [--limit n] [--bodies] [--json] [-f]` | Print the newest trace records oldest first; `-f` follows new ones until Ctrl-C |
| `fcdev ingest sets list` | List the sets |
| `fcdev ingest sets applies <set>` | List a set's kept applies |
| `fcdev ingest sets reapply <set> <apply-id> [--dry-run]` | Go back to an earlier apply |
| `fcdev ingest sets release <set>` | Let go of a set |

`apply` takes a folder (every `*.yaml` and `*.yml` under it, in name order; the
set is named after the folder unless `--set` says otherwise) or the files named
(then `--set` is required). It prints the plan, one line per ingestion with the
fields that change:

```
$ fcdev ingest apply client-code/acme/ingest --set acme --dry-run
set acme, dry run: nothing was changed, 3 definition(s)
  change    ingest/acme/orders-sftp (target, split)
  create    ingest/acme/stock-feed
  unchanged ingest/acme/invoices
```

A definition that names a settings entry which is not set yet is saved, and the
plan prints its `CONFIG_MISSING` warning under its line (also with `--dry-run`);
`pause`, `resume`, `live` and `get` print the warnings the platform returns in the
same way. A file that still holds `${env:NAME}` outside a comment is refused with
its line: environment variables are not read, put the value in the ingestion's
settings instead.

Per-environment settings are written with `config set`, after the apply has created
the ingestion:

```
$ fcdev ingest config set bevco-integration/bevco/mix-positions mix-positions.json
bevco-integration/bevco/mix-positions: settings written to mix-positions, SET
  checked against mix-positions, mix-positions-b
  warning: SCHEMA_UNCHECKED: ...
```

The file is a JSON object (anything else is refused before it is sent): the kind's
settings and, if the file's target has no `targetUrl`, `targetUrl`. A secret setting
holds the secret's name. A refused value prints each problem with its field path and
exits `1`; nothing is written unless every ingestion naming the entry accepts it.
`config get` prints the status (`INLINE`, `SET`, `MISSING`, `INVALID`), the problems and
whether the value is checked against a kind's schema (`schemaChecked`); `--json` prints
the raw answer. `config set` and `config delete` ask for confirmation on a prod profile
like the other writes, and `delete` lists the ingestions that stop.

`apply` refuses a folder with no definitions, since applying none would delete
everything the set owns; use `sets release` to let go of a set.

`test` exits `1` when the source failed, was refused, is not supported, or no
runtime answered in time. `NO_RESULTS` is not a failure: the source answered
and had nothing. `export` followed by `apply` of the same files changes nothing;
what differs between environments is not in the files (see "Settings per
environment").
