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

A source never holds a secret, only its **name** (`credential: {name: sftp-password}`).

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
| `code` | The dispatch job's code |
| `targetUrl` | Where the job is delivered. Checked at save with the platform's outbound-URL guard (no private or metadata addresses) |
| `pool` | A dispatch pool **code**; it must exist and be usable by the ingestion's client |
| `connection` | Optional connection code. The connection's service account signs each delivery, and pausing the connection holds this ingestion's jobs |
| `serviceAccount` | Optional service account code, to sign without a connection. Not together with `connection` |
| `mode` | `IMMEDIATE`, `NEXT_ON_ERROR` or `BLOCK_ON_ERROR` |
| `batchSize` | Records per job batch |

With neither `connection` nor `serviceAccount` the delivery is unsigned. The job's
client is the ingestion's client. Anything that does not resolve is a `400` when
the ingestion is saved, not a failure in the runtime later.

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
  every: PT5M
split:
  kind: csv
  header: true
target:
  code: acme:orders:order:received
  targetUrl: ${env:ORDERS_HOOK}
  pool: orders
  connection: orders-conn
  mode: NEXT_ON_ERROR
  batchSize: 100
```

A file may hold several definitions separated by `---`; keep one per file. An
extension's source is written flat (`kind: camel-route`, then its settings).
Unknown fields are errors. `${env:NAME}` is read from the environment when
`fcdev` reads the file, for what differs between environments (a pool, an
address); an unset variable stops the command. Nothing else is replaced, and
comment lines are left alone.

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
receive: a saved version does not run until it is made live. Saving through
the API's update or through a set apply makes the new version live; saving a
version on its own does not.

**Pause** stops an ingestion running; **resume** starts it again. Both need
`update`. A paused ingestion keeps its versions and state.

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
| `platform:ingestion:ingestion:view` | List, read, versions, history, failures, definition export, source kinds |
| `platform:ingestion:ingestion:create`, `update`, `delete` | Change definitions; `update` includes pause and resume |
| `platform:ingestion:ingestion:promote` | Make a version live |
| `platform:ingestion:ingestion:test` | Run test connection |
| `platform:ingestion:ingestion:route` | Create or change an ingestion of a kind that declares it needs this (`camel-route`); removing one needs only `delete` |
| `platform:ingestion:set:apply` | Apply or release a set |
| `platform:ingestion:runtime:operate` | The runtime API: poll, report, state |

Seeded roles: `platform:ingestion-admin` (everything but the runtime),
`platform:ingestion-operator` (view, update, test), `platform:ingestion-viewer`,
and `platform:ingestion-runtime` for a runtime's service account.

**Reach.** A caller below anchor tier sees and changes only the ingestions of its
own clients. An application-scoped service account reaches only its own
application's ingestions. Listing sets and their applies needs `view` and a
caller that reaches every client and application, because a set spans them.

## Secrets

A definition names secrets and never holds one. A runtime reads a named secret as
the ingestion's **client's** value in the platform's configuration (see
"Platform configuration"), through the runtime API's own secrets call
(`GET /api/ingestion-runtime/v1/ingestions/{id}/secrets/{name}`). That call
reveals only the secrets the ingestion's live definition names, and every read
is audited (refused if the audit cannot be written). The runtime's service
account needs `platform:ingestion-runtime` and nothing else: no config-reveal
permission. It always reads at the ingestion's own client, with no fall-back to a
global value, so one client's ingestion cannot read another's. Set the secret
for each client that needs it before the ingestion runs.

## Runtimes

A **runtime** is the program that actually reads sources. It is a separate,
optional component with its own release: it polls the platform, runs the
ingestions it is given, reports their state and failures, keeps small values
(a watermark, a cursor) through the platform's state calls, and creates the
dispatch jobs as its service account. The platform never calls it, so a
runtime can sit behind a firewall.

- A runtime reports the source kinds it carries. An ingestion of a kind no live
  runtime reports shows as "no runtime carries this kind", which is not a failure.
- Polled sources (`sftp`, `s3`, `database-poll`, `api-poll`, `custom`) run on one
  runtime at a time, under a lease. `http`, `sqs` and `rabbitmq` run on every
  runtime that carries them.
- Create the runtime's service account with `platform:ingestion-runtime`.

## Turning it on

`FC_INGESTION_ENABLED` is `false` by default, and `true` under `fcdev`. Off, the
ingestion routes are not mounted (`404`), the navigation entry is absent, and the
tables exist but stay empty. The permissions and roles are seeded either way.
The runtime API is mounted only when the flag is on.

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

`apply` refuses a folder with no definitions, since applying none would delete
everything the set owns; use `sets release` to let go of a set.

`test` exits `1` when the source failed, was refused, is not supported, or no
runtime answered in time. `NO_RESULTS` is not a failure: the source answered
and had nothing. `export` followed by `apply` of the same files changes nothing;
replace what differs between environments with `${env:NAME}` before keeping them.
