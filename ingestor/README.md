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
| `POST /poll` | every `INGEST_POLL_INTERVAL`: the live ingestions this runtime should run, the lease length, pending "test connection" requests. Sends `contractVersion` 2 and the kinds the runtime adds |
| `POST /report` | route states, exchange counts, failures and test answers |
| `GET/PUT /state/{ingestionId}/{key}` | the watermark or cursor of a polled source, kept on the platform so it follows the ingestion to another runtime |
| `POST /ingestions/{id}/jobs` | the **only** way jobs are created: the runtime sends records, the platform builds each job from the ingestion's own target (URL, pool, connection, signer, client) |
| `GET /ingestions/{id}/secrets/{name}` | the **only** way secrets are read: audited on the platform, only secrets the ingestion's live definition names |

## Kinds

Built in: `http`, `sftp`, `s3`, `sqs`, `rabbitmq`, `database-poll`, `api-poll`. Added by extensions: any
other kind (the `generator` example is in `ingestor-extensions/generator`). `camel-route` is a kind the
runtime reports only when `INGEST_CAMEL_ROUTES_ENABLED=true`; the platform then requires
`platform:ingestion:ingestion:route` of whoever writes one, because a route runs with the runtime's own
network, identity and environment. See [docs/camel-routes.md](docs/camel-routes.md).

A polled source (`sftp`, `s3`, `database-poll`, `api-poll`, any extension kind, `camel-route`) runs on one
runtime at a time, under a lease the poll renews; a runtime that cannot renew it stops the route before it
ends. `http`, `sqs` and `rabbitmq` run on every runtime that carries them.

HTTP ingestions are posted to the runtime: `POST /ingest/{client}/{code}` (the client's identifier or its
platform id), or `POST /ingest/{application}/{client}/{code}` when two applications have an ingestion of the
same code for the client (the short form answers `409` then).

## Running it

You need a platform with ingestion enabled (`FC_INGESTION_ENABLED=true`, on by default in `fcdev`), and a
service account for the runtime holding the platform role `platform:ingestion-runtime`. An account provisioned for one
application polls only that application's ingestions; a runtime shared by several integration applications needs an
account created with `allApplications`. With more than one runtime the platform spreads the polled sources evenly over
the live ones (fair share), and a runtime that stops polling hands its share to the others once its leases expire.

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
| `INGEST_CAMEL_ROUTES_ENABLED` | `ingest.camel-routes-enabled` | `false` | report and run the `camel-route` kind |
| `INGEST_POLL_INTERVAL` | `ingest.poll-interval` | `PT1M` | how often to poll and report. The platform's lease is 3 minutes |
| `INGEST_ADVERTISED_URL` | `ingest.advertised-url` | unset | where this runtime can be reached; shown by the platform for information, never called |
| `INGEST_SECRETS_CACHE_TTL` | `ingest.secrets-cache-ttl` | `PT1M` | how long a revealed secret is kept (each reveal is an audit row on the platform) |
| `INGEST_AWS_ENDPOINT` | `ingest.aws.endpoint` | unset | a stand-in for AWS (LocalStack, an S3-compatible store) |
| `HTTP_PORT` / `HTTP_HOST` | `http.port` / `http.host` | `8091` / `0.0.0.0` | the listener for HTTP ingestions and `/health/live`, `/health/ready` |
| `APP_ENV` | | `local` | `local` also reads `config/application-local.yaml` and `config/secrets.local.yaml` |

`INGEST_SECRET_<CLIENT>__<NAME>` is the documented fallback for a secret not yet moved to the platform: an
ingestion that names `env:sftp-password` reads it from the variable made of the client's identifier and the
name, upper-cased with every character other than a letter or digit as `_`
(`INGEST_SECRET_ACME__SFTP_PASSWORD`). Every other name is read from the platform.

### Secrets on the platform

A secret is a platform config property of the ingestion's application, section `secrets`, at the ingestion's
client. The runtime's service account needs no config permission: the platform reveals a secret to a runtime
only when the ingestion's live definition names it, the runtime carries the kind and (for a polled source)
holds the lease. A "test connection" of a polled source from a runtime that does not hold the lease cannot read
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
