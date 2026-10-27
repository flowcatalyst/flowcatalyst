# Ingestion

An **ingestion** takes data from wherever a system keeps it, splits each payload
into records, and turns every record into a **dispatch job**. It is the inbound
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
| `name`, `active` | A label, and whether the ingestion runs. `active` is true when a file does not say. |
| `keepFailedPayloads` | Off unless the client has agreed. When on, a failure may carry the first 2 KB of the payload that failed. |
| `source` | Where the data comes from. |
| `split` | How a payload becomes records. |
| `target` | The dispatch job each record becomes. |

An ingestion is addressed as **`application/client/code`** (for example
`ingest/acme/orders-sftp`) by `fcdev ingest`, or by its `ing_…` id.

### Source kinds

| Kind | Reads |
|---|---|
| `http` | Data pushed to the runtime over HTTP |
| `sftp`, `s3` | Files in a directory or bucket, polled |
| `sqs`, `rabbitmq` | Messages from a queue |
| `database-poll`, `api-poll` | A table or an HTTP API, polled on an interval |
| `custom` | A kind a runtime adds as an extension (`kind: custom`, `type`, `settings`) |

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
| `code` | The dispatch job's code, `application:subdomain:aggregate:action`. Its first part must be the ingestion's own application: an ingestion makes jobs of its application only |
| `targetUrl` | Where the job is delivered. Checked at save with the platform's outbound-URL guard (no private or metadata addresses). May be left out only when the source names a config entry whose value gives it (see "Settings per environment"): exactly one of the two |
| `pool` | A dispatch pool **code**; it must exist and be usable by the ingestion's client |
| `connection` | Optional connection code. The connection's service account signs each delivery, and pausing the connection holds this ingestion's jobs |
| `serviceAccount` | Optional service account code, to sign without a connection. Not together with `connection`. It must be one of the ingestion's application's own accounts and reach the ingestion's client (an account linked to no client reaches all of its application's clients); a platform account, or another application's, is refused |
| `mode` | `IMMEDIATE`, `NEXT_ON_ERROR` or `BLOCK_ON_ERROR` |
| `batchSize` | Records per job batch |

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
secret's **name**. The delivery address is in the file's `target.targetUrl` **or**
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
- **The kind's schema.** For a built-in kind it is the definition itself (strict
  decoding and `Validate`; an unknown setting is a problem). For an extension kind it
  is the field list a live runtime reports for it (type, required, choices, pattern,
  bounds, list counts), checked by the platform. With no live runtime reporting the
  kind the value is stored with a `SCHEMA_UNCHECKED` warning (`schemaChecked: false`
  in `GET .../config`), and the runtime checks it before it builds the route.
- **The generic configuration API and page refuse section `ingest`** (`400
  CONFIG_SECTION_RESERVED`) for a write or a delete; reading it is as for any section, except that the value is shown only to a caller who can update ingestions (`ingestion:update`): everyone else sees the entry's status and problems, `GET .../config` answers `valueHidden: true` without `value`, and the generic config reads show `***`.

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
| `change` | The set owns it and it differs: saved as the next live version. `active` is set from the file, so a pause made since is undone |
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
| `platform:ingestion:runtime:operate` | The runtime API: poll, report, state, jobs, secrets |

Seeded roles: `platform:ingestion-admin` (everything but the runtime's, route
included, and the only role with `runtime-identity`),
`platform:ingestion-operator` (view, update, test: it cannot write routes),
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
| `fcdev ingest pause <ref>`, `resume <ref>` | Stop or start an ingestion |
| `fcdev ingest test <ref \| file> [--wait] [--timeout d]` | Queue a test; `--wait` polls and prints the steps |
| `fcdev ingest config get <ref> [--json]` | Show the settings entry the ingestion's source names: key, status, problems, the value |
| `fcdev ingest config set <ref> <file.json \| -> [--json]` | Write the entry from a JSON object (a file, or stdin with `-`) |
| `fcdev ingest config delete <ref> [--json]` | Delete the entry; the ingestions that name it stop |
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
