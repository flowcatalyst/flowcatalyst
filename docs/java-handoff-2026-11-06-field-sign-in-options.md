# Java hand-off: field-worker sign-in options (unenrolled devices, badges, impersonation, test identities)

For the Java platform port. Delta on top of `docs/java-handoff-2026-11-03-field-identity.md`; read
that first. Authority, in order: this document's "Decisions", `docs/field-worker-sign-in-options-spec.md`,
`docs/wire-contract.md` ("Field identity: the admin API" and "Field identity (`/field/*`)"), the code.
Go packages: `internal/platform/fieldidentity` (`badge_ops.go`, `impersonation_ops.go`, `badge_repository.go`,
the changed `entity.go`, `operations.go`, `session_repository.go`, `auth_repository.go`),
`internal/platform/fieldidentity/api` (`badges.go`, `api.go`), `internal/platform/fieldauth`
(`session.go`, `impersonate.go`, `endpoints.go`), `internal/platform/auth/authservice/fieldtoken.go`,
`pkg/fcsdk/auth/field.go`, `tools/field-fixtures`. Shared vectors: `api/field-fixtures/` gains
`valid_unenrolled`, `valid_impersonated_supervisor`, `valid_impersonated_principal`, `valid_test`; each
carries a `principal` object (`deviceId`, `deviceMode`, `amr`, `actor?`, `test`) a verifier must expose.

## 1. Schema (goose migration 101; take the next free number at merge)

Re-runnable. `field_sessions`: `device_id` becomes NULLable; new `jkt VARCHAR(64)` (backfilled from
`field_devices` on the device id; new sessions always set it), `actor_type` (`FIELD_WORKER | PRINCIPAL`),
`actor_id`; partial index on `actor_id` for live sessions. `field_identities`: `can_impersonate`,
`is_test` (both BOOLEAN NOT NULL DEFAULT FALSE). `field_policies`: `require_enrolled_device` (false),
`unenrolled_idle_secs` (900), `badge_only_login` (false), `impersonation_max_secs` (14400).
`field_login_attempts`: `actor_id`. New `field_badges` (`fwb_`: client, identity FK ON DELETE CASCADE,
`token_hash` UNIQUE, label, status ACTIVE|REVOKED, created_by, created_at, revoked_at, last_used_at;
UNIQUE partial index on `identity_id WHERE status = 'ACTIVE'`: one active badge per worker) and
`field_sign_in_codes` (`fwi_`: client, identity FK cascade, application, `code_hash` UNIQUE,
`created_by` (principal id) NOT NULL, `created_by_name`, expires_at, used_at, created_at). TSID prefixes
`fwb`, `fwi`. The migration inserts `platform:iam:field-worker:impersonate` into the role
`platform:field-administrator` (the seeder never updates an existing role) and updates the seeded 1.0
schemas of the changed event types while they are still exactly what the old seed wrote. Add an effect
probe (`field_badges` exists). The purger also deletes sign-in codes 1 day after expiry
(`SessionRepo.Purge`, counted with the enrolment codes).

## 2. Unenrolled devices

The proof is still required on every route; a key with no device row is *unenrolled* (`c.device == nil`).
Route preludes: nonce and enrol as before; device and bearer routes look the key up and apply the P1 status
checks only when found (a SUSPENDED / REVOKED device is still 403, never unenrolled). Bearer: the token's
`cnf.jkt` must equal the proof `jkt`; when the token carries `device_id` the proof key must be that ACTIVE
device. `requireEnrolledDevice` is checked per handler once the client is known (login, PIN change,
act-as, sign-in code, and refresh of an unenrolled session) and answers `401 UNKNOWN_DEVICE`.

Login body gains `clientIdentifier` and `badgeToken`; the client resolution, `CLIENT_MISMATCH`, the
no-enumeration rule and the order of checks are in the wire contract and `fieldauth/session.go` (read it
top to bottom: it is the spec). An unenrolled success writes the session with `device_id` NULL,
`jkt` = proof, idle = `unenrolled_idle_secs`, absolute = 43200. Token: no `device_id`, `device_mode`
`"UNENROLLED"`. Throttling: an unenrolled key has no device bucket; it counts against a per-IP bucket
`field_pin_ip` (`FC_FIELD_IP_FAILURES_PER_HOUR`, default 100; marker bucket `field_throttle_notice_ip`)
and the client bucket; throttle event scope `IP` with `ipAddress`. Enrolled devices keep the device bucket
and do not count against the IP bucket. Refresh: `session.jkt` must equal the proof `jkt` and an enrolled
session also needs its device; otherwise revoke the family (`REUSE`). A key enrolled after the session
began keeps the unenrolled limits and an unenrolled token (no mid-session upgrade): the token follows the
session row, not the request. Logout: the session must belong to the proof key (`jkt`); `device-all` on an
unenrolled key is `400 INVALID_REQUEST`. Introspection / `TokenUsable`: skip the device check when the
token has no `device_id`.

## 3. Badges

Token `FWB1` + 26 Crockford base32 characters from 130 random bits (alphabet
`0123456789ABCDEFGHJKMNPQRSTVWXYZ`), normalised to upper case (and trimmed) before hashing; stored as
SHA-256 hex; shown once at issue. A malformed value is treated as unknown without a lookup. Issue = one
transaction: lock the worker row (`FOR UPDATE`), revoke the worker's ACTIVE badges, insert the new one,
emit `badge-issued {workerId, clientId, badgeId, revokedBadgeIds[]}`. Bulk issue (1..200 distinct ids) is
one transaction, every worker loaded and client-boxed first, one `badge-issued` per worker. Revoke is
idempotent (no second event). Login with a badge: see the wire contract; failures count against the
subject bucket (and the client bucket when the client is known); a badge of another client on an enrolled
device is indistinguishable from an unknown badge; `last_used_at` is stamped in the login transaction.
`badge_only_login` allows no PIN only on an enrolled device; otherwise `400 PIN_REQUIRED` (after the
worker-active and lock checks).

## 4. Impersonation

`act-as` is an AuthTx path like login: lock the supervisor row; not permitted (not ACTIVE, not
`can_impersonate`, no PIN, locked) is `403 IMPERSONATION_FORBIDDEN` without a PIN check; verify the
SUPERVISOR's PIN with the full lockout accounting (wrong: record the failure, commit, count the buckets);
a success resets the supervisor's failure counter (not `last_login_at`); then lock the target and apply the
target rules; cap the absolute expiry by `impersonation_max_secs` and by the supervisor's session (the live
session of the token's `sid` family, locked); insert the session (`actor_type FIELD_WORKER`, `amr
["pin","imp"]`, pcr never); commit. After the commit an envelope use case (`NoteImpersonated`) emits
`field-worker:impersonated` with the ACTOR as execution-context principal and the worker as the entity.
The sign-in code redemption is a single envelope transaction (`RedeemSignInCode`): consume the code
(`UPDATE ... used_at IS NULL AND expires_at > now() RETURNING`), check the creator principal is active,
lock the worker, re-check, insert the session, emit the event (principal = the code's creator). Any failure
rolls back, so the code stays unused. Sign-in code creation is an envelope use case (principal = the calling
admin); the permission rule (impersonate, or manage for a test identity; human principal unless the target is
a test identity) is in the controller (`CanSignInAsFieldWorker`). Revocation: suspend, delete and clearing
`can_impersonate` revoke the sessions where the worker is the actor (`ACTOR_SUSPENDED`) in the same
transaction (`field_sessions.actor_id = worker AND actor_type = 'FIELD_WORKER'`; delete removes them);
refresh of an impersonated session re-checks the actor (supervisor ACTIVE and still `can_impersonate`,
principal active).

## 5. Test identities

`is_test` on ensure/update (manage); tokens carry `test: true`; introspection returns `test`;
`GET /api/field-workers?isTest=true|false`; events `created | ensured | updated` carry `isTest`.

## 6. Token

`device_id` omitted when empty; `act: {sub, typ, name}` (`typ` `field_worker` or `principal`) only when
impersonated; `test: true` only for a test identity. `amr` values: `["pin"]`, `["badge","pin"]`, `["badge"]`,
`["pin","imp"]`, `["code","imp"]`.

## Decisions

- Migration number 101 (098 does not exist in the Go repo; the Java side takes its own next free number).
- `badge`-only success does not require the worker to have a PIN; a badge with a PIN presented requires one.
- A locked or inactive supervisor on act-as is `403 IMPERSONATION_FORBIDDEN` (the spec's literal reading),
  not the 423/429 of a login.
- The act-as event is emitted after the AuthTx commits (best effort, logged on failure; the session and the
  sign-in attempt row are the durable record). Sign-in code redemption is atomic with its event.
- Refresh of an impersonated session refuses when the actor can no longer act (not in the spec; closes the
  window between deactivating an admin and the session's expiry).
- `requireEnrolledDevice` also applies to refresh of an unenrolled session, to PIN change and to act-as
  (the spec names "every device route"); an existing unenrolled session is refused with `401 UNKNOWN_DEVICE`
  rather than revoked.
- A PUT of the policy replaces it: `requireEnrolledDevice` and `badgeOnlyLogin` are false when omitted.
- Admin-initiated `ResetWorkerPin` does not revoke the sessions the worker holds as an actor.
- `GET /api/field-workers` and `GET /{id}` return the active `badge` (batch-loaded for the list).
