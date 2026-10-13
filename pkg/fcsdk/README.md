# FlowCatalyst Go SDK

The Go member of the FlowCatalyst SDK family (TypeScript, Laravel, Java and
Rust live beside it under `clients/`). It is a separate Go module:

```
github.com/flowcatalyst/flowcatalyst/pkg/fcsdk
```

It is also the dependency of the FlowCatalyst application framework's
FlowCatalyst module (`../flowcatalyst-framework`): the framework builds on
this module rather than talking to the platform itself.

What it gives an application:

- **use cases and an outbox**: one business operation writes its aggregate,
  its domain event and its audit row in one transaction, into
  `outbox_messages`; the platform's outbox processor delivers them;
- **dispatch jobs** queued in the same transaction;
- **declarative sync** of everything an application owns on the platform
  (roles, event types, dispatch pools, connections, subscriptions,
  processes, scheduled jobs, principals, its OpenAPI document and its
  documentation pages);
- a **platform API client**, **OIDC/OAuth** helpers, **webhook** signature
  validation, a **scheduled-job runner**, **TSIDs**, and pluggable **cache**
  and **lock** providers.

## Install

Go 1.26 or later.

```
go get github.com/flowcatalyst/flowcatalyst/pkg/fcsdk@latest
```

Releases are tags `pkg/fcsdk/vX.Y.Z` on `github.com/flowcatalyst/flowcatalyst`
(`make release-go-sdk` in this repository; see `docs/releasing.md`). Until the
first release, depend on a commit (`@main`) or on a checkout:

```
go mod edit -require=github.com/flowcatalyst/flowcatalyst/pkg/fcsdk@v0.0.0-00010101000000-000000000000 \
            -replace=github.com/flowcatalyst/flowcatalyst/pkg/fcsdk=../flowcatalyst/pkg/fcsdk
```

## A use case, end to end

`examples/order-service` is the complete, runnable version of this section
(its test runs it against Postgres when `FC_TEST_DATABASE_URL` is set).

```
HTTP request → command
             → usecase.ExecutionContext (principal, correlation, execution ids)
             → usecaseop.Run(ctx, uow, PlaceOrder(repo), cmd, ec)
                  Validate → Authorize → Execute → Plan
                  the Plan is applied in ONE transaction:
                    repository Persist      → orders
                    Sink.WriteEvent         → outbox_messages (EVENT)
                    Sink.WriteAudit         → outbox_messages (AUDIT_LOG)
             → (event, error) → HTTP 201 / 4xx / 500
```

An operation is a value with named phases; `Execute` can reach the database
only by returning a `Plan` (`usecaseop.Save`, `Delete`, `Emit`, `SaveAll`,
`Sync`), and `usecaseop.Run` is the only thing that applies one, so "row
written ⇒ event and audit written, atomically" holds by construction.

```go
func PlaceOrder(repo OrderRepository) usecaseop.Operation[PlaceOrderCommand, OrderPlaced] {
	return usecaseop.Operation[PlaceOrderCommand, OrderPlaced]{
		Name: "PlaceOrder",
		Validate: func(_ context.Context, cmd PlaceOrderCommand) error {
			if cmd.TotalCents <= 0 {
				return usecase.Validation("TOTAL_INVALID", "totalCents must be positive")
			}
			return nil
		},
		Authorize: usecaseop.Public[PlaceOrderCommand], // or a real check
		Execute: func(_ context.Context, cmd PlaceOrderCommand, ec usecase.ExecutionContext) (usecaseop.Plan[OrderPlaced], error) {
			order := &Order{ID: tsid.GenerateWithPrefix("ord"), CustomerID: cmd.CustomerID, TotalCents: cmd.TotalCents}
			event := OrderPlaced{Meta: newMeta(ec, "sales", "order", "placed", order.ID), OrderID: order.ID}
			return usecaseop.Save(order, repo, event), nil
		},
	}
}

// wiring
sink := outboxpgx.NewSink(outboxpgx.Config{
	ClientCode:      "acme",   // the client's identifier slug (optional)
	ApplicationCode: "orders", // required on audit rows for an application-scoped credential
	AuditEnabled:    true,
})
uow := usecasepgx.New(pool, sink)

// in the handler
event, err := usecaseop.Run(r.Context(), uow, PlaceOrder(repo), cmd, usecase.NewExecutionContext(principalID))
```

The repository implements `usecasepgx.Persist[A]` (aggregates never persist
themselves); the event implements `usecase.DomainEvent` (usually by embedding
`usecase.EventMetadata`). A command may implement `AuditOperation() string`
to name its audit row's operation and `AuditData() any` to choose what the row
records; whatever is recorded is redacted first (`usecase.RedactAuditJSON`:
passwords, secrets, tokens, API keys...).

### The outbox rows

Package `outbox` builds every row; `outboxpgx` (pgx, the use-case Sink) and
`outboxsql` (`database/sql`, Postgres or MySQL) write them. The outbox
processor sends each row's payload unchanged:

| Row | Endpoint | Payload |
|---|---|---|
| `EVENT` | `POST /api/events/batch` | `specVersion, type, source, subject, data, correlationId, causationId, deduplicationId, messageGroup, clientCode, contextData` |
| `AUDIT_LOG` | `POST /api/audit-logs/batch` | `entityType, entityId, operation, operationData, principalId, performedAt, source, correlationId, applicationCode, clientCode` |
| `DISPATCH_JOB` | `POST /api/dispatch-jobs/batch` | the batch item (`id, kind, code, source, targetUrl, payload, ...`) |

- `data` and `operationData` are the JSON value itself, never a string of
  encoded JSON (the other SDKs encode them; the platform unwraps those).
- The event row's id and `deduplicationId` are the event id.
- `clientCode` (the client's identifier slug) comes from the writer's
  configuration or from an event implementing `outbox.ClientCoded`. Leave it
  empty to let the platform scope by the credential: an anchor writes
  platform-scoped events, a single-client credential writes under its client;
  a code the credential cannot reach is refused (403, whole batch).
- An event without `source` gets the platform's default,
  `{application}:{subdomain}` of its type.

The rows, the `INSERT` statement and the dispatch-job rules are the
application framework's outbox conformance vector, pinned by
`outbox/conformance_test.go` (`outbox/testdata/conformance`).

### Dispatch jobs

```go
job, err := outbox.NewDispatchJob("orders:billing:export-order", "https://billing.example.com/exports").
	WithClientCode("acme"). // or WithPlatformClientID; the platform resolves the code
	WithCorrelationID(ec.CorrelationID).
	WithJSONPayload(map[string]string{"orderId": orderID})
if err != nil { /* ... */ }

err = usecasepgx.RunErr(ctx, uow, func(s *usecasepgx.TxScopedUnitOfWork) error {
	// ... EmitEventScoped / CommitScoped for the event ...
	return s.WithTx(ctx, func(tx pgx.Tx) error { return sink.WriteDispatchJob(ctx, tx, job) })
})
```

The job is validated before it is written (code `{application}:{rest}`,
absolute `http(s)` target with no user info, ids `[0-9A-Za-z_-]{1,17}`, ...):
a row the platform would refuse terminally is never stored. Defaults: kind
`TASK`, source the code's application, payload `{}` as `application/json`.

### `database/sql` and MySQL

```go
w, err := outboxsql.NewWriter(outboxsql.DialectMySQL, outboxsql.WriterConfig{ClientCode: "acme"})
// inside the application's own *sql.Tx:
err = w.WriteEvent(ctx, tx, event)
err = w.WriteAudit(ctx, tx, event, command)
err = w.WriteDispatchJob(ctx, tx, job)
```

`outboxsql.InitSchema` / `outboxpgx.InitSchema` create the table (use your
own migrations in production). See `examples/outbox-sql`.

## Syncing definitions

```go
set := sync.ForApplication("orders").
	AddRole(sync.MakeRole("admin").WithPermissions("orders:sales:order:manage")).
	AddEventType(sync.MakeEventType("orders:sales:order:placed", "Order placed")).
	AddDispatchPool(sync.MakeDispatchPool("orders-default", "Orders").WithConcurrency(8).WithRateLimit(600)).
	AddConnection(sync.MakeConnection("orders-billing", "Billing webhooks")).
	AddSubscription(sync.MakeSubscription("order-billing", "Billing", "https://billing.example.com/webhooks").
		WithConnectionCode("orders-billing").
		Bind("orders:sales:order:placed", "")).
	AddProcess(sync.MakeProcess("orders:sales:place-order", "Place order").WithBody("graph LR; A-->B")).
	AddScheduledJob(sync.MakeScheduledJob("orders:nightly", "Nightly", "0 0 2 * * *").
		WithTargetURL("https://orders.example.com/jobs")).
	AddPrincipal(sync.MakePrincipal("ops@example.com").WithName("Ops").WithRoles("orders:admin")).
	AddDoc(sync.MakeDoc("getting-started", "# Getting started\n"))

res := sync.NewSynchronizer(c).Sync(ctx, set, sync.DefaultOptions())
if res.HasErrors() { log.Println(res.Errors()) }
log.Println("unknown roles skipped:", res.Principals.SkippedRoles)
```

One call per category, in dependency order (roles, event types, dispatch
pools, connections, subscriptions, processes, scheduled jobs, principals,
OpenAPI, docs); a failed category does not stop the others, except that
subscriptions are not sent when connections failed. `sync.WithRemoveUnlisted()`
removes the SDK-sourced rows a set no longer lists (never UI-created ones);
docs are always a full replace. `set.ForClient("acme")` scopes connections and
subscriptions to a client; scheduled jobs carry their own `ForClient`.
`SyncUsers` syncs users without an application. See `examples/fc-sync`.

## Platform client and auth

```go
cc := auth.NewClientCredentialsProvider(auth.ClientCredentialsConfig{
	IssuerURL: "https://auth.flowcatalyst.io", ClientID: "svc-app", ClientSecret: secret,
})
c := client.New("https://api.flowcatalyst.io", client.WithTokenProvider(cc.Token))
types, err := c.EventTypes().List(ctx, &client.EventTypeFilters{Application: "orders"})
```

`client` retries transient 5xx and returns `*client.APIError` for non-2xx.
`auth.NewTokenValidator` validates incoming RS256 bearer tokens through OIDC
discovery and a cached JWKS; `auth.OAuthClient` does PKCE, refresh, revoke,
introspect, userinfo and RP-initiated logout. See `examples/list-event-types`.

Every `client` method follows the platform's OpenAPI lock: a route that
answers `204 No Content` (update, suspend, activate, archive, delete, pause,
resume, enable, disable) returns only an `error`; a create route that answers
`{ "id" }` returns `*client.CreatedResponse` (call `Get` for the record); and
a route that answers a body returns it. `go test ./client/` checks all of this
against `api/openapi.lock.json`.

## Packages

| Package | Purpose |
|---|---|
| `usecase` | `DomainEvent`, `EventMetadata`, `ExecutionContext`, typed `*Error`, the sealed `Result[E]`, audit redaction. No I/O. |
| `usecaseop` | The use-case envelope: `Operation[C,E]` (Validate / Authorize / Execute → `Plan`), `Run`, `TxOperation` / `RunTx` for multi-aggregate operations. |
| `usecasepgx` | pgx unit of work: `New`, `Persist[A]`, `Sink`, `Commit*` / `EmitEvent`, `Run` / `RunErr` with `TxScopedUnitOfWork`. |
| `outbox` | The outbox rows (event, audit, dispatch job), the `INSERT`, `DispatchJob` and its validation. No I/O. |
| `outboxpgx` | The use-case `Sink` writing to `outbox_messages` via pgx, plus `WriteDispatchJob`. |
| `outboxsql` | `Writer` for `database/sql` (Postgres, MySQL) and the table DDL. |
| `sync` | `DefinitionSet` + `Synchronizer`: declarative sync of every application category. |
| `client` | Platform HTTP API client: event types, subscriptions, dispatch pools, applications, processes, principals, roles, permissions, audit logs, clients, connections, me, router, scheduled jobs, OpenAPI, docs. |
| `auth` | Access-token claims, `TokenValidator` (JWKS), `HmacTokenValidator`, `OAuthClient`, `ClientCredentialsProvider`. |
| `webhook` | HMAC-SHA256 webhook signature verification (`Verifier` for the platform's router; `Validator` for the SDK-peer shape). |
| `scheduledjobs` | `Runner` for platform-fired scheduled jobs: handlers by job code, a lock per firing, logs and completion reported back. |
| `tsid` | 13-character Crockford Base32 TSIDs, typed prefixes. |
| `cache` | Byte cache with required TTLs and JSON helpers; `MemoryCache`, `cache/postgrescache`, `cache/rediscache`. |
| `lock` | Distributed locks; `Memory`, `lock/postgreslock`, `lock/redislock`. |

## Examples

All of them build with `go build ./...` (CI does this).

| Example | What |
|---|---|
| `examples/order-service` | A use case through `usecaseop.Run`, the pgx outbox, a dispatch job in the same transaction. |
| `examples/outbox-sql` | The `database/sql` writer: event, audit row and dispatch job in one `*sql.Tx`. |
| `examples/fc-sync` | Syncing every category of one application. |
| `examples/list-event-types` | `client` with a static token or `ClientCredentialsProvider`. |
| `examples/scheduled-jobs-runner` | `scheduledjobs.Runner` behind an HTTP handler. |
| `examples/webhook-receiver` | Webhook signature verification and its error switch. |

## Developing the SDK

From the repository root: `make test-go-sdk` (build, vet, race tests),
`make lint-go-sdk`, `make go-sdk-smoke` (a scratch module consumes the SDK
through a local `replace` and through `go get` of a git tag), and
`RELEASE_DRY_RUN=1 make release-go-sdk BUMP=0.1.0`. The platform module uses
this tree through `replace ... => ./pkg/fcsdk`. The SDK imports nothing from
the platform. Wire references: `docs/wire-contract.md`, the OpenAPI lock
`api/openapi.lock.json`.

## Changes before 0.1.0

The platform client was audited method by method against the OpenAPI lock.
These signatures changed:

- **204 routes return only `error`** (they decoded an empty body into a
  struct before): `Applications.Update`, `Applications.EnableForClient`,
  `Applications.DisableForClient`, `Clients.Update`, `Clients.EnableApplication`,
  `Clients.DisableApplication`, `Clients.UpdateApplications`,
  `DispatchPools.Update/Archive/Suspend/Activate`, `EventTypes.Update`,
  `Subscriptions.Update`, `Processes.Update`,
  `ScheduledJobs.Update/Pause/Resume/Archive/LogForInstance/CompleteInstance`.
- **`{id}` creates return `*CreatedResponse`**, not a fabricated aggregate:
  `DispatchPools.Create`, `EventTypes.Create`, `Subscriptions.Create`,
  `Processes.Create`. (`Connections.Create` is the opposite: the platform
  answers 201 with the whole connection, so it returns `*ConnectionResponse`.)
- **Responses that were discarded are returned**: `Connections.Pause/Activate`
  (`*ConnectionResponse`), `Principals.AddRole/RemoveRole`
  (`*PrincipalResponse`), `Principals.SetRoles` (`*SetRolesResponse`),
  `Principals.GrantClientAccess` (`*ClientAccessGrantResponse`).
  `Principals.SyncUsers` returns `*SyncUsersResult` (`syncedEmails`, not
  `syncedCodes`). `Roles.ListForApplication` returns `[]RoleResponse` (the
  platform answers a bare array). `ScheduledJobs.Fire` also carries `id` and
  `scheduledJobId`.
- **Routes the platform does not have, removed**: `Applications.GetServiceAccount`
  (`GET /api/applications/{id}/service-account` does not exist; read the
  account with the new `ServiceAccounts().Get`, and point an application at one
  with the new `Applications.LinkServiceAccount`) and
  `Applications.UpdateClientConfig` (`PUT .../clients/{clientId}` does not
  exist; use `Applications.GetClientConfig`, `EnableForClient` and
  `DisableForClient`). `ClientConfigRequest` and `SuccessResponse` are gone.
- **Query parameters match the lock**: `Applications.List(ctx, appType, active)`
  (no paging), `Clients.List(ctx)` (no filters or paging),
  `Connections.List(ctx, clientID, status)`, `EventTypes.List(ctx, *EventTypeFilters)`
  (adds subdomain and aggregate), `Processes.List(ctx, *ProcessFilters)`
  (application, subdomain, status; clientId was never accepted),
  `PrincipalFilters` gains roles, paging and sorting, and `InstanceFilters`
  drops `TriggerKind`, `From` and `To`.
- **Wrong routes fixed**: `Processes.Archive` called the hard `DELETE`; it now
  posts `/archive`, and `Processes.Delete` no longer sends a `?hard=true` the
  platform ignores. `DispatchPools.Archive` called `DELETE` (a real delete); it
  now posts `/archive`, and `DispatchPools.Delete` is new.
- **Required request members are always sent**: `UpdateConnectionRequest.Name`,
  `UpdateEventTypeRequest.Name` and `AddSchemaVersionRequest.Version` are
  required fields; `CreateRoleRequest.ClientManaged` and
  `CreateScheduledJobRequest.Concurrent/TracksCompletion` are sent when false.
  `SyncEventTypesRequest` takes `[]SyncEventTypeItem` (code, name, description:
  the sync schema has no schema or client members).

## Errors

See [ERRORS.md](ERRORS.md) for which sentinels use `errors.Is` and which typed
errors use `errors.As`, per package.

## License

Copyright © 2026 Belac (FlowCatalyst)

Licensed under the Apache License, Version 2.0 — see [`LICENSE`](LICENSE) and
[`NOTICE`](NOTICE) in this directory (the module ships its own copies; a module
download contains only this directory).
