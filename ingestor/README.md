# Ingestor

The runtime that reads sources and turns what it reads into dispatch jobs on a FlowCatalyst platform.

An **ingestion** is defined on the platform (see `docs/ingestion-design.md` in the platform repository):
a source, how a payload is split into records, and the dispatch job each record becomes. The platform
stores the definitions, the leases, the state and the reports. This runtime is the part that touches the
sources. It is optional: a platform without a runtime simply has no ingestions running.

```
platform  <--  /api/ingestion-runtime/v1  --  ingestor runtime  --  SFTP, S3, SQS, RabbitMQ, a database,
 (poll, report, state, jobs, secrets)                                 an API, HTTP push, a Camel route,
                                                                      or a kind an extension adds
```

The runtime talks to the platform only through that API, as a service account:

| Call | What for |
|---|---|
| `POST /poll` | every `INGEST_POLL_INTERVAL`: the live ingestions this runtime should run, the lease length, pending "test connection" requests. Sends `contractVersion` 2, the kinds the runtime adds (`kinds`) and the built-in kinds it runs (`builtinKinds`) |
| `POST /report` | route states, exchange counts, failures and test answers |
| `GET/PUT /state/{ingestionId}/{key}` | the watermark or cursor of a polled source, kept on the platform so it follows the ingestion to another runtime |
| `POST /ingestions/{id}/jobs` | the **only** way jobs are created: the runtime sends records, the platform builds each job from the ingestion's own target (URL, pool, connection, signer, client) |
| `GET /ingestions/{id}/secrets/{name}` | the **only** way secrets are read: only secrets the ingestion's live definition binds; logged on the platform as security telemetry (not an audit row per read; a refused fetch is audited) |

### Contract version 2, and what was added to it

Every addition keeps `contractVersion` 2; a platform that predates a field ignores it.

- **`builtinKinds`** in the poll: `sftp`, `http`, `rabbitmq`, `sqs`, `s3`, `database-poll`, `api-poll` (not the
  `custom` envelope; extension kinds and `camel-route` are in `kinds`). A newer platform hands an ingestion of a
  built-in kind only to a runtime that names it.
- **The poll answer is read one ingestion at a time.** An ingestion whose definition this runtime cannot read
  (a source kind, a variant or an enum value of a newer platform) is reported as a `FAILED` route with
  `definition not supported by this runtime version` (and the source kind when it is one the runtime does not
  know); a route an earlier version of it had is stopped. The other ingestions run. A test whose definition
  cannot be read is answered `FAILED` with the same reason. The poll itself never fails for one of them.
- **`category`** on a `REJECTED` failure: `payload` when the payload could not be read, `rejected` when the
  platform refused its jobs for good. The platform shows a `rejected` one as a notice to dismiss, with the
  platform's own message.
- **`acceptAnyHostKey`** on an `sftp` source (below).
- **Idempotency keys** (`externalId`) on the records (below).

### When the platform refuses jobs

A jobs request the platform answers with a client error it would repeat (any 4xx but 401, 403, 408, 409,
425 and 429), or with a refusal of one of its records, is **permanent**: the payload is let go of exactly as
one that cannot be read (an SFTP or S3 file moves to `.rejected`, an SQS message is deleted, a RabbitMQ message
is rejected without requeue so it dead-letters, a database poll's watermark or status and an API poll's cursor
move past it, an HTTP caller gets `422 REFUSED`), so the source keeps moving, and a `REJECTED` failure with
`category: rejected` and the platform's message is reported. Jobs already made from earlier records of the
payload stay made.

Anything else (5xx, no answer, 408, 409 such as a lease or a paused target, 425, 429, and 401/403 after the
token was dropped so the next call fetches a new one) is **transient**: the batch is sent again after 0.5 s and
1.5 s, and if the third attempt fails the exchange fails and the source keeps the payload and offers it again
(the file stays, the message comes back, the watermark stays).

### Sizes

| | The platform takes | The runtime sends |
|---|---|---|
| a jobs request | 10 MiB of JSON, 1000 records | batches cut at the ingestion's `batchSize` (at most 1000) **and** at 8 MiB of serialised JSON, measured as sent; a record over 8 MiB goes alone, one over 10 MiB is the payload's fault (`REJECTED`, `category: payload`) |
| a report | 4 MiB of JSON; 2000 routes and windows, 500 failures, 20 test answers | under 4 MiB by measured size, oldest first; what does not fit goes in the next report. At most 100 unsent test answers are kept |

NUL characters are taken out of every message, error, step and excerpt (the platform's database cannot store
them). A report the platform refuses with a client error is dropped with a log line. A report that fails 5
times in a row while the platform answers polls is taken to carry something the platform cannot store: its
failures and test answers are dropped with an error log line, so reporting goes on; the counts are kept.

## Kinds

Built in: `http`, `sftp`, `s3`, `sqs`, `rabbitmq`, `database-poll`, `api-poll`. Added by extensions: any
other kind (the `generator` example is in `ingestor-extensions/generator`). `camel-route` is a kind the
runtime reports only when `INGEST_CAMEL_ROUTES_ENABLED=true`; the platform then requires
`platform:ingestion:ingestion:route` of whoever writes one, a permission only anchor users can hold, because a
route runs with the runtime's own network, identity and environment. See [docs/camel-routes.md](docs/camel-routes.md).

A polled source (`sftp`, `s3`, `database-poll`, `api-poll`, any extension kind, `camel-route`) runs on one
runtime at a time, under a lease the poll renews; a runtime that cannot renew it stops the route before it
ends. `http`, `sqs` and `rabbitmq` run on every runtime that carries them.

HTTP ingestions are posted to the runtime: `POST /ingest/{client}/{code}` (the client's identifier or its
platform id), or `POST /ingest/{application}/{client}/{code}` when two applications have an ingestion of the
same code for the client (the short form answers `409` then). Failed logins are limited: an address that fails
20 times in a minute against one ingestion is answered `429` (with `Retry-After`) without its credentials being
checked until its wait is over; other addresses and other ingestions are not held up.

### SFTP host keys

An `sftp` source checks the server's host key against `knownHosts` (a `known_hosts` line). Without one the
route **does not start** and is reported `FAILED` (and logged at WARN) with `the sftp source has no known host
key`, unless the definition sets `acceptAnyHostKey: true`, which connects without checking who the server is
(anyone who can intercept the connection is sent the login) and logs a WARN line each time the route starts.

### Which addresses a source may reach

A host a definition names (an SFTP or RabbitMQ host, a JDBC URL's hosts in the `//host` or Oracle `@host` form,
an API and its token or login endpoint) is resolved, and **every** address it resolves to is checked, when the
route starts and again before each connection (each SFTP or database poll, each API request, each RabbitMQ
connect and reconnect). Refused: loopback (`127.0.0.0/8`, `::1`), link-local (`169.254.0.0/16`, `fe80::/10`,
where cloud metadata services answer), unspecified and "this network" (`0.0.0.0/8`, `::`), and the metadata
addresses `100.100.100.200` and `fd00:ec2::254`, also inside an IPv4-compatible or NAT64 IPv6 address. Private
ranges (`10/8`, `172.16/12`, `192.168/16`, `fc00::/7`) are allowed. Loopback is allowed only when `APP_ENV` is
`local` or `test` (fcdev). The JVM keeps a resolved name for 30 s (`networkaddress.cache.ttl`), and every
client resolves through the JVM (the HTTP client's own DNS resolver is switched off, `vertx.disableDnsResolver`),
so the client that connects right after the check gets the same answer and a name re-pointed in between (DNS
rebinding) does not get through. A start that is refused is a `FAILED` route; a poll that is refused is a `SOURCE` failure and
the route is `FAILING`. S3 and SQS are not checked (their hosts are AWS's, or `INGEST_AWS_ENDPOINT`), nor are
extensions' own connections or Camel routes (which run with the runtime's network by design).

### Idempotency keys

Each record's job carries `externalId` = the SHA-256 (hex) of the **identity of the payload it came from** and
the record's position in it (`<identity>#<index>`, from 0). The platform answers a record sent again under the
same key with the job it made first, so a payload read twice (a retry after a timeout, a message redelivered, a
file read again because it was not moved, the ingestion moving to another runtime) makes its jobs once. The key
is made from where the payload came from, never from its content alone: two different payloads must not share
one.

| Kind | Identity | Without one |
|---|---|---|
| `sftp` | `<host>:<absolute path>@<modified time>\|<size>` | |
| `s3` | `<bucket>/<key>@<ETag>` | |
| `sqs` | `<queue URL>\|<message id>` | |
| `rabbitmq` | `<queue>\|<message-id property>`, when the publisher set one | no key |
| `database-poll`, `status` track | `<ingestion id>\|<key column>=<value>`; each row is one record (`#0`) | |
| `database-poll`, `watermark` track | `<ingestion id>\|<column>=<value>/<place among the rows of that value>` | |
| `api-poll` with a `cursor` | the request: `<url>?<param>=<cursor sent>` | no key without a cursor (every poll asks the same address) |
| `http` | the caller's `Idempotency-Key` request header | no key: a request sent again makes its jobs again |
| an extension kind | what the extension gives (`PayloadIdentity`, see [docs/extensions.md](docs/extensions.md)) | no key |
| `camel-route` | the exchange property `ingestPayloadIdentity`, if a step sets it | no key |

Consequences to know: the platform keeps a key for 7 days, so a `status`-track row set back to pending within
that time is **not** dispatched again (give it a new key value, or use a watermark); rows that share a watermark value are keyed by
their place in the database's order, which is stable only if the order of ties is; an API poll whose answer
gives no new cursor asks the same request again, and its records are keyed as before (new records at new
positions get new keys, a record at a position already keyed is taken as already sent).

The jobs of one payload share a `messageGroup` (and `correlationId`): `<ingestion id>:` and the start of the
SHA-256 of its identity, or of its content when it has no identity and is held whole (an HTTP body, a
message), so a payload read again lands in the same group.

## Running it

You need a platform with ingestion enabled (`FC_INGESTION_ENABLED=true`, on by default in `fcdev`), and a
service account for the runtime holding the platform role `platform:ingestion-runtime`. The role is platform-wide: the runtime routes reach every client's and every application's ingestions, however the
account was provisioned, so one fleet serves them all. With more than one runtime instance the platform spreads the polled
sources evenly over the live ones (fair share), and an instance that stops polling hands its share to the others once its
leases expire.

```
java -jar ingestor-runtime-X.Y.Z-all.jar        # from a directory that holds config/application.yaml
```

The jar reads `config/application.yaml` from its working directory and does not start without it (the image has it); without the file `flowcatalyst.application` is unset and the start fails. Or the image `flowcatalyst/ingestor:X.Y.Z`. Under `fcdev`, the dev tool fetches the released jar and runs
it for you; a deployment runs it as its own service (a container or a process beside the platform).

### Configuration

Every value is a key in `config/application.yaml` and an environment variable named after its path.

| Environment variable | Key | Default | |
|---|---|---|---|
| `FLOWCATALYST_BASE_URL` | `flowcatalyst.base-url` | `http://localhost:8080` | the platform. Anything but `localhost` development must be `https` |
| `FLOWCATALYST_CLIENT_ID` | `flowcatalyst.client-id` | | OAuth client id of the runtime's service account. **Required** |
| `FLOWCATALYST_CLIENT_SECRET` | `flowcatalyst.client-secret` | | its secret. **Required**; or `config/secrets.local.yaml` (never committed) |
| `FLOWCATALYST_APPLICATION` | `flowcatalyst.application` | `ingestor` | names the account's application to the client library; the runtime API does not use it |
| `INGEST_RUNTIME_ID` | `ingest.runtime-id` | `runtime-<hostname>` | names this runtime; two running runtimes must never share one. `[A-Za-z0-9._:-]`, at most 100 |
| `INGEST_EXTENSIONS_DIR` | `ingest.extensions-dir` | unset | a directory of extension jars, loaded at start in addition to those on the class path |
| `INGEST_EXTENSIONS_MANIFEST` | `ingest.extensions-manifest` | unset | `s3://<bucket>/<key>` of a manifest pinning the extension jars (same bucket, SHA-256 checked) to load at start; exclusive with jars in `INGEST_EXTENSIONS_DIR`. See [docs/extensions.md](docs/extensions.md) |
| `INGEST_CAMEL_ROUTES_ENABLED` | `ingest.camel-routes-enabled` | `false` | report and run the `camel-route` kind |
| `INGEST_POLL_INTERVAL` | `ingest.poll-interval` | `PT1M` | how often to poll and report. The platform's lease is 3 minutes |
| `INGEST_ADVERTISED_URL` | `ingest.advertised-url` | unset | where this runtime can be reached; shown by the platform for information, never called |
| `INGEST_SECRETS_CACHE_TTL` | `ingest.secrets-cache-ttl` | `PT1M` | how long a fetched secret is kept, and how often each running route's secrets are read again (each fetch is a security log line and a "last fetched" update on the platform, not an audit row) |
| `FLOWCATALYST_TIMEOUT` | `flowcatalyst.timeout` | `PT30S` | the longest a call to the platform waits; the poll and the report wait at most a sixth of the lease |
| `INGEST_AWS_ENDPOINT` | `ingest.aws.endpoint` | unset | a stand-in for AWS (LocalStack, an S3-compatible store) |
| `HTTP_PORT` / `HTTP_HOST` | `http.port` / `http.host` | `8091` / `0.0.0.0` | the listener for HTTP ingestions and `/health/live`, `/health/ready` |
| `APP_ENV` | | `local` (the image: `production`) | `local` also reads `config/application-local.yaml` and `config/secrets.local.yaml`; only `local` and `test` accept a platform on plain `http` and sources on loopback addresses |

### Secrets from the environment (`env:`)

The documented fallback for a secret not yet moved to the platform: an ingestion that names `env:<name>` reads
the runtime's environment variable

    INGEST_SECRET_V2_<client>__<name>

where `<client>` is the client's identifier and `<name>` the name after `env:`, each written with its ASCII
letters and digits **as they are (case kept)** and every other character as `_` followed by its two upper-case
hex digits (four, after `_U`, beyond U+00FF):

| Character | Written |
|---|---|
| `-` | `_2D` |
| `_` | `_5F` |
| `.` | `_2E` |
| `:` | `_3A` |

So client `acme` and `env:sftp-password` read `INGEST_SECRET_V2_acme__sftp_2Dpassword`; client `acme--eu` and
`env:db.password` read `INGEST_SECRET_V2_acme_2D_2Deu__db_2Epassword`. An encoded part never contains `__`, so
the `__` between client and name is never ambiguous, and no two clients and names share a variable.

The earlier form, `INGEST_SECRET_<CLIENT>__<NAME>` (upper-cased, every other character `_`,
`INGEST_SECRET_ACME__SFTP_PASSWORD`), is still read when the new variable is not set, **only when it cannot
belong to another client and name**: neither encoded part contains `__` or starts or ends with `_` (so a client
identifier with `--` in it, such as `acme--eu`, must use the new form). Reading it logs a WARN line once naming
the new variable to set. Infrastructure code should write the new form.

Every other name is read from the platform.

### Secrets on a running route

Every `INGEST_SECRETS_CACHE_TTL` (and no more often than a poll), the runtime reads each running route's
platform secrets again; when one has another value the route is rebuilt (stopped, letting the exchange in
flight finish, and started with the new value), so a rotated password or key is picked up without a restart.
A secret that cannot be read at that moment leaves the route as it is. An HTTP ingestion checks its secret on
every request (kept for the TTL) and is not rebuilt; `env:` secrets do not change while the runtime runs. Kept
values past the TTL, and everything kept for an ingestion no longer running, are dropped from memory every
round.

### Leases, stopping and shutting down

A single-owner (polled) route runs under the lease the poll renews (3 minutes on the platform). It is stopped:

- by the round, when a poll fails and the lease would end before the next round could renew it;
- by a **watchdog** that runs every second apart from the rounds, once the lease is within its margin of the
  end without a renewal: 30 s or twice the poll's timeout, whichever is longer, at most half the lease (with
  the defaults, 1 minute). A round stuck on a slow platform cannot keep a lapsed lease's routes running;
- on shutdown.

The poll and the report wait at most `FLOWCATALYST_TIMEOUT` and at most a sixth of the lease (30 s for 3
minutes). A route is stopped gracefully: its exchange in flight is given 10 s to finish (its jobs sent, its
file moved, its message acknowledged), then cut off, and its payload stays with the source. Several routes are
stopped at the same time; on shutdown all of them within 12 s (fcdev waits 15 s before it kills the JVM).
The platform has no call to give a lease back, so a stopped runtime's polled ingestions move to another
runtime when their lease runs out (at most 3 minutes after its last poll); their watermarks and cursors are on
the platform, so nothing is read twice or skipped beyond what the idempotency keys absorb.

"Test connection" requests run on their own threads, at most 2 minutes each (or until the request expires);
the round waits up to 10 s for them and reports the rest when they finish, or as failed at their deadline. A
round that throws anything is logged and the next one runs; so does a polled source or an API poll whose poll
throws. Running out of memory ends the process (`-XX:+ExitOnOutOfMemoryError` in the image, and the runtime's
own handlers halt the JVM) so that it is restarted healthy.

### The image

`APP_ENV=production` (a plain-`http` platform and loopback sources are refused), `-XX:+ExitOnOutOfMemoryError`,
and `/app` (the jar and its config) and `/opt/ingestor/extensions` owned by root and not writable by the
runtime's user; mount extension jars read-only or build an image of your own that copies them in as root.

### Secrets on the platform

A secret is a platform config property of the ingestion's application, section `secrets`, at the ingestion's
client. The runtime's service account needs no config permission: the platform answers a secret to a runtime
only when the ingestion's live definition binds it, the runtime carries the kind and (for a polled source)
holds the lease, and the client is active. This fetch is not a config reveal: the platform logs each one as
security telemetry (a `security.ingestion_secret_fetch` log line, the `fc_ingestion_secret_fetch_total`
counter and a "last fetched" record per ingestion, secret and runtime, shown on the ingestion) rather than
writing an audit row per read; a refused fetch is audited. A "test connection" of a polled source from a runtime that does not hold the lease cannot read
its secrets, and says so. A definition under test that is not saved cannot read secrets at all.

## Extensions

A jar in `INGEST_EXTENSIONS_DIR` adds a kind. Each jar has a class loader of its own that sees the runtime's
classes (the contract, the extension API, Camel). The jar's manifest must declare the extension API it was
built against, `Ingestor-Extension-Api: 1`; the runtime refuses to start with a jar of another major version,
or one that declares none, naming the jar. Extensions on the class path (a build that depends on
`ingestor-runtime` and its own extension modules) are not checked: they are compiled with the runtime.
See [docs/extensions.md](docs/extensions.md); `ingestor-extensions/generator` is the worked example.

## Build and test

Needs JDK 25 and the FlowCatalyst framework 0.1.2 in the local Maven repository (or the GitHub Packages
repository `flowcatalyst/framework`).

```
./mvnw -o -B verify              # offline; add -DskipTests to only package
ls ingestor-runtime/target/ingestor-runtime-*-all.jar
docker build -t flowcatalyst/ingestor:local .
```

Tests against real SFTP, RabbitMQ, a local AWS and PostgreSQL use Testcontainers and are skipped when no
Docker daemon answers; everything else runs against in-process fakes (a fake platform, Camel with direct
endpoints, compiled extension jars).

## Releasing

`make release-ingestor BUMP=patch|minor|major|X.Y.Z` in the platform repository bumps `ingestor/VERSION` and the
`<revision>` in `ingestor/pom.xml`, and tags `ingestor/vX.Y.Z`. The tag splits this directory to
`flowcatalyst/ingestor` (`.github/workflows/split-ingestor.yml`) and builds the fat jar and image
(`.github/workflows/ingestor-release.yml`).
