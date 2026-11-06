# Java hand-off: linking a field worker to a platform user

For the Java platform port. Delta on top of `docs/java-handoff-2026-11-06-field-sign-in-options.md`.
Authority, in order: `docs/field-native-session-spec.md` section 5, `docs/wire-contract.md` ("Link to a
platform user", the introspection and access-token paragraphs), the code. Go: `internal/platform/fieldidentity`
(`operations.go` `LinkWorker`/`UnlinkWorker`, `entity.go`, `repository.go`, `events.go`),
`internal/platform/fieldidentity/api/api.go`, `internal/platform/fieldauth/endpoints.go` (`issue`),
`internal/platform/auth/authservice/fieldtoken.go`, `internal/ui/pages/field_workers_link.go`,
`pkg/fcsdk/{auth/field.go,client/field_workers.go}`, `tools/field-fixtures`. Vector: `api/field-fixtures/`
gains `valid_linked`, and every `principal` object gains `linkedPrincipalId` (empty when absent).

## 1. Schema (goose migration 102; take the next free number at merge)

Re-runnable. `field_identities.linked_principal_id VARCHAR(17)` NULL, no foreign key, and
`UNIQUE (client_id, linked_principal_id)` (NULLs never collide, so unlinked workers are unconstrained). Add an
effect probe (the column exists). No role or event-schema rewrite: `linked`/`unlinked` are new event types the
seeder catalogues.

## 2. Operations

`LinkWorker {workerId, principalId}` and `UnlinkWorker {workerId}` through the use case envelope, permission
`platform:iam:field-worker:manage`, client-boxed exactly like the other worker operations. The link target
rule (`linkable`): the principal exists, is a USER, is ACTIVE, and `scope.canAccessClient(workerClient, home,
grantedClients)` holds: ANCHOR any client, PARTNER only its granted clients, CLIENT only its home client. This is
the same rule that builds the token `clients` claim, so a link target is always someone who can see that client
on the web. Any failure is `422 INVALID_LINK` (reasons are coarse: not found, not a user, inactive, no access).
The use case type is a business-rule error (409 in the generic mapping); the admin controller maps the code
`INVALID_LINK` to 422. Another worker of the client already holding the user is `409 LINK_EXISTS` (checked in the
use case; the unique index is the backstop). Linking replaces any previous link; same user again and unlinking an
unlinked worker are no-ops that still record the event. Events `platform:iam:field-worker:linked | unlinked`
`{workerId, clientId, workerCode, principalId}` (`unlinked` carries the user that was linked, `""` if none).

## 3. API and token

`PUT|DELETE /api/field-workers/{id}/link` answer the refreshed worker. Worker responses (get, list, ensure)
carry `linkedPrincipal: {id, name, email} | null` (an unresolvable id keeps `id` with empty name/email; names are
batch-resolved from `iam_principals` for the list). `GET /api/field-workers` takes `linkedPrincipalId`.
Access token claim `linked_principal_id`, omitted when not linked, set from the worker row in the single place the
platform mints field tokens (login, refresh, act-as, console sign-in code all go through it), so a refresh picks
up a link added or removed after login. On an impersonated session it is the TARGET worker's link. Introspection
returns `linkedPrincipalId` from the token's claim (not re-read from the row: it is as stale as the token, at most
one access-token lifetime). The claim is audit correlation only; no authorisation reads it.

## 4. Console

Worker drawer section "Linked platform user" (state, Link/Change, Unlink; manage only for the actions). The
Go console finds the user by exact email (case-insensitive) and runs the same operation, so every refusal is the
operation's. The SPA (React) is untouched; its generated API types come from `api/openapi.lock.json`
(`pnpm api:generate`).

## 5. Tests worth porting

Link happy path with the response shape; refusals for a service account, an inactive user, a user of another
client, a partner without the grant, an unknown id (all 422 INVALID_LINK, nothing written, no event); duplicate in
the client 409 LINK_EXISTS; the same user in a second client; replace and unlink; permission (view cannot
change, 403) and client box (another client's worker 404); the claim at login and at refresh after a link made or
removed, on a sign-in-code session (target's link, not the actor's) and act-as; introspection; deleting the
principal leaves the link.
