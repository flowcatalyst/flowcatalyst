// Package fcsdk is the entry-point overview for the FlowCatalyst Go SDK,
// module github.com/flowcatalyst/go-sdk. There is no Go code
// in the root; this file exists so that `go doc` on the module path returns
// a map of what's where. It is also the dependency of the FlowCatalyst
// application framework's FlowCatalyst module.
//
// Wire parity: TSIDs, webhook signatures and audit redaction match the other
// SDKs byte for byte (shared vectors). Outbox rows are the application
// framework's conformance vector under its comparison rule; the one
// deliberate difference is that an event's data and an audit row's
// operationData are written as the JSON value itself, never as encoded JSON
// in a string (see package outbox).
//
// # Mental model
//
// A FlowCatalyst consumer app runs each business operation through the
// use-case envelope. The operation's Plan is applied in one transaction: the
// aggregate row, the domain event and (if enabled) the audit row; the event
// and audit rows land in outbox_messages, and the platform's outbox processor
// forwards them to /api/events/batch and /api/audit-logs/batch. Dispatch jobs
// written in the same transaction go to /api/dispatch-jobs/batch. Operations
// never call the platform directly during a transaction.
//
//	HTTP request → command DTO
//	             → usecase.ExecutionContext (principal + correlation)
//	             → usecaseop.Run(ctx, uow, op, cmd, ec)
//	                  ↓ Validate / Authorize / Execute → Plan
//	             → the Plan is applied in a single transaction
//	                  ↳ <Repo>.Persist
//	                  ↳ Sink.WriteEvent → outbox_messages
//	                  ↳ Sink.WriteAudit → outbox_messages (optional)
//	             → committed event → (E, error)
//	             → HTTP 201 / 4xx / 500
//
// The Sink slot is what makes the SDK reusable. Consumer apps wire
// outboxpgx.Sink (writes to outbox_messages); the platform itself wires
// its own sink that writes directly to msg_events.
//
// # Package map
//
// Domain primitives (no I/O):
//
//   - usecase    — DomainEvent + EventMetadata + ExecutionContext + typed
//     Error, audit redaction, and the sealed Result[E]: Success requires a
//     sealed.Token only pkg/fcsdk packages can mint.
//   - usecaseop  — the enforced use-case envelope: Operation[C,E]
//     {Validate, Authorize, Execute→Plan} + Run, plus TxOperation / RunTx
//     for multi-aggregate ops. An Execute can reach the DB only by
//     returning a Plan that Run applies atomically.
//   - outbox     — the outbox rows (EVENT, AUDIT_LOG, DISPATCH_JOB), the
//     INSERT statement and DispatchJob, held to the framework's
//     conformance vector.
//   - tsid       — Time-Sorted IDs (Crockford Base32), typed prefixes and
//     GenerateWithPrefix for app-specific IDs.
//
// UnitOfWork driver:
//
//   - usecasepgx — pgx-backed UoW. Entry points: Commit / CommitDelete /
//     CommitAll / EmitEvent / CommitSync / Run / RunErr. The usecaseop
//     envelope (Operation/Plan/Run) is built on top of it.
//
// Outbox writers:
//
//   - outboxpgx  — the usecasepgx Sink, writing to outbox_messages via pgx;
//     WriteDispatchJob for dispatch jobs.
//   - outboxsql  — Writer for database/sql (Postgres, MySQL) and the table
//     DDL (InitSchema).
//
// HTTP I/O:
//
//   - client     — *FlowCatalystClient + resource families (event types,
//     subscriptions, dispatch pools, applications, processes, principals,
//     roles, permissions, audit logs, clients, connections, me, router,
//     scheduled jobs, openapi, docs). Retry on transient 5xx, typed
//     *APIError. Bearer token or TokenProvider auth.
//   - auth       — AccessTokenClaims + AuthContext; TokenValidator (RS256
//     via JWKS auto-discovery through lestrrat-go/jwx/v2);
//     HmacTokenValidator (HS256); OAuthClient (PKCE auth-code, refresh,
//     revoke, introspect, userinfo, RP-initiated logout);
//     ClientCredentialsProvider (satisfies client.TokenProvider).
//   - webhook    — Two HMAC-SHA256 validators. Verifier matches the
//     platform's router (uppercase headers, ISO8601 timestamps); Validator
//     matches the SDK-peer shape (mixed-case headers, Unix-second
//     timestamps).
//   - sync       — DefinitionSet + Synchronizer for declarative
//     reconciliation of roles, event types, dispatch pools, connections,
//     subscriptions, processes, scheduled jobs, principals, the OpenAPI
//     document and documentation pages. One call per category.
//   - scheduledjobs — consumer-side Runner. Register HandlerFuncs by job
//     code; serialises via a lock.Provider, streams logs back, reports
//     completion.
//
// Infrastructure:
//
//   - cache      — pluggable byte-oriented Cache + Get/Set/GetOrSet JSON
//     helpers. MemoryCache ships here; cache/postgrescache and
//     cache/rediscache are opt-in sub-packages.
//   - lock       — distributed-lock Provider + Handle. Memory ships here;
//     lock/postgreslock and lock/redislock are opt-in sub-packages.
//
// Internal:
//
//   - internal/sealed — Token type that gates usecase.Success.
//     Constructable only by packages of this module (Go's internal rule),
//     which scopes the seal to the SDK.
//
// # License
//
// Copyright 2026 Belac (FlowCatalyst). Licensed under the Apache License,
// Version 2.0 (see the LICENSE and NOTICE files in this directory), the same
// licence as the rest of the repository.
package fcsdk
