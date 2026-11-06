# Java hand-off: principals by id follow the list's visibility rule

Owner ruling 2026-11-06. Completes the PR-3/PR-4 ruling (`docs/owner-rulings-todo.md` #3):
an out-of-scope target answers the same 404 a missing id would.

## The leak

`GET /api/principals/{id}` and its sub-routes checked client access only when the
principal had a home client (`client_id IS NOT NULL`). A principal with none (an
anchor user, a global service account, any partner) was answered to any caller
holding `platform:iam:user:view`. So a client administrator could read an anchor
user's name, email and roles by id, though the list (`GET /api/principals`) hides
them. The leniency was carried over from the Rust port, not a product decision.

## Rule

Every by-id read applies the list's per-row rule (Go: `PrincipalVisible` in
`internal/platform/principal/api/api.go`):

- caller is anchor: visible;
- principal has a home client: visible when the caller can access that client;
- principal has none: visible only when it is a PARTNER granted (via
  `iam_client_access_grants`) a client the caller can access.

Not visible answers `404` with code `Principal_NOT_FOUND`, message
`Principal not found: <id>`, byte-identical to a missing id.

## Routes

- `GET /api/principals/{id}` (self exempt, as before)
- `GET /api/principals/{id}/version` (self exempt, as before)
- `GET /api/principals/{id}/roles`
- `GET /api/principals/{id}/application-access`
- `GET /api/principals/{id}/available-applications`

Writes were already refused by the operations for out-of-scope targets; unchanged.
The SDK revocation check reads its own principal (self) and fails open on non-200.

## Tests (Go: `platform_principal_scope_pg_test.go`)

For each route above, a client administrator of client A:
- an anchor user: 404, canonical not-found envelope;
- a partner granted only client B: 404;
- a partner granted client A: 200.
An anchor caller reads all three. A partner reads itself via `/{id}` and `/{id}/version`.
