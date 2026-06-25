// Package fcsdk is the entry-point overview for the FlowCatalyst Go
// SDK. There is no Go code in the root — this file exists so that
// `go doc github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk` returns a
// useful map of what's where.
//
// The SDK has byte-for-byte wire parity with the Rust, TypeScript, and
// Laravel SDKs: a token, event payload, or TSID minted by any one of
// them is identical to the same value minted by another.
//
// # Mental model
//
// A FlowCatalyst consumer app emits domain events through a UnitOfWork.
// The UoW writes the aggregate row, the event, and (if enabled) the
// audit log inside one transaction; on commit, the event lands in
// outbox_messages, and an outbox processor forwards it to the
// platform's /api/events/batch. Use cases never call the platform
// directly during a transaction.
//
//	HTTP request → command DTO
//	             → usecase.ExecutionContext (principal + correlation)
//	             → usecaseop.Run(uow, op, cmd, ec)
//	                  ↓ Validate / Authorize / Execute → Plan
//	             → the Plan is applied in a single transaction
//	                  ↳ <Repo>.Persist
//	                  ↳ Sink.WriteEvent → outbox_messages
//	                  ↳ Sink.WriteAudit → outbox_messages (optional)
//	             → committed event → (T, error)
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
//   - usecase    — Result + DomainEvent + ExecutionContext + typed Error.
//     Result[E] is a sealed sum: Success requires a
//     sealed.Token only pkg/fcsdk packages can mint, so the
//     only path to Success outside the SDK is through the
//     usecasepgx Commit* functions. Compile-time enforced.
//   - usecaseop  — the enforced use-case envelope: Operation[C,E]
//     {Validate, Authorize, Execute→Plan} + Run, plus
//     TxOperation / RunTx for multi-aggregate ops. The
//     handler-facing entry point; an Execute can reach the DB
//     only by returning a Plan that Run applies atomically.
//   - tsid       — Time-Sorted IDs (Crockford Base32). 35 typed
//     EntityType prefixes plus GenerateWithPrefix for
//     app-specific IDs.
//
// UnitOfWork driver:
//
//   - usecasepgx — pgx-backed UoW. Entry points: Commit / CommitDelete /
//     CommitAll / EmitEvent / CommitSync / Run / RunErr. The usecaseop
//     envelope (Operation/Plan/Run) is built on top of it.
//
// Sinks:
//
//   - outboxpgx  — writes to outbox_messages via pgx.
//   - outboxsql  — outbox-table DDL (InitSchema) for database/sql consumer apps.
//
// HTTP I/O:
//
//   - client     — *FlowCatalystClient + resource families
//     (event_types, subscriptions, dispatch_pools,
//     applications, processes, principals, roles,
//     permissions, audit_logs, clients, connections, me,
//     router, scheduled_jobs, openapi). Retry on transient
//     5xx, typed *APIError. Bearer token or TokenProvider
//     auth.
//   - auth       — AccessTokenClaims + AuthContext; TokenValidator
//     (RS256 via JWKS auto-discovery through
//     lestrrat-go/jwx/v2); HmacTokenValidator (HS256);
//     OAuthClient (PKCE auth-code, refresh, revoke,
//     introspect, userinfo, RP-initiated logout);
//     ClientCredentialsProvider (satisfies
//     client.TokenProvider).
//   - webhook    — Two HMAC-SHA256 validators. Verifier matches this Go
//     platform's router (uppercase headers, ISO8601
//     timestamps); Validator matches the Rust SDK shape
//     (mixed-case headers, Unix-second timestamps).
//   - sync       — DefinitionSet + Synchronizer for declarative
//     reconciliation. One call per category.
//   - scheduledjobs — consumer-side Runner. Register HandlerFuncs by
//     job code; serialises via a lock.Provider, streams
//     logs back, reports completion.
//
// Infrastructure:
//
//   - cache      — pluggable byte-oriented Cache + Get/Set/GetOrSet
//     JSON helpers. MemoryCache ships here;
//     cache/postgrescache and cache/rediscache are opt-in
//     sub-packages.
//   - lock       — distributed-lock Provider + Handle. Memory ships
//     here; lock/postgreslock and lock/redislock are
//     opt-in sub-packages.
//
// Internal:
//
//   - internal/sealed (under pkg/fcsdk/internal/) — Token type that
//     gates usecase.Success. Constructable only by packages under
//     github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/ (Go's internal
//     rule), which scopes the seal to the SDK.
package fcsdk
