# Java hand-off: field-worker identity (P1, slices A and B)

For the Java platform port. Everything the Go platform does for the field-worker plane, so the
Java side can mirror it. Authority, in order: this document's "Decisions" (they record rulings
and choices the spec left open), `docs/field-worker-identity-p1-spec.md`, `docs/wire-contract.md`
("Field identity: the admin API" and "Field identity (`/field/*`)"), the code. Go packages:
`internal/platform/fieldidentity` (domain, repos, operations, `pin`, `api`),
`internal/platform/fieldauth` (device protocol), `internal/platform/auth/authservice/fieldtoken.go`
(token), `pkg/fcsdk/auth/field.go` (SDK verifier), `tools/field-fixtures` (shared vectors).

A Java SDK/verifier is not part of this hand-off beyond the fixtures: `api/field-fixtures/`
(`jwks.json`, `cases.json`) is the language-neutral conformance suite for a field-token verifier.

## 1. Schema (goose migration 095; take the next free number at merge)

All tables are new, no foreign keys to other modules (`client_id` / `application_id` are plain
references). Ids: TSID prefixes `fwu` (worker), `fwd` (device), `fwe` (enrolment code), `fws`
(session row), `fwl` (login attempt).

| Table | Notes |
|---|---|
| `field_identities` | `(client_id, worker_code)` unique; `worker_code` stored upper-case; `status` ACTIVE or DISABLED; `pin_hash` NULL until set; `pin_scheme` PEPPER_V1 or LEGACY; `pepper_version`; `pin_change_required`; `failed_since_success`, `locked_until` (soft lock), `hard_locked`; `last_login_at` |
| `field_identity_apps` | `(identity_id, application_id)` PK, `source` ADMIN or APP, `granted_at` |
| `field_devices` | `jkt` unique (RFC 7638 thumbprint), `public_key_jwk`, `mode` PERSONAL or SHARED, `status` ACTIVE or SUSPENDED or REVOKED, `attestation_level` (NONE or SOFTWARE in P1), `attestation_chain` TEXT (raw as reported, unverified; added to 095 in slice B), `assigned_identity_id`, `enrolled_by`, `last_seen_at` |
| `field_device_apps` | `(device_id, application_id)` |
| `field_enrolment_codes` | `code_hash` = SHA-256 hex of the code (unique), `mode`, `app_ids TEXT[]`, `assigned_identity_id`, `expires_at`, `used_at` |
| `field_sessions` | One row per issued refresh token. `family_id` (= the login = the access token's `sid`), `refresh_hash` SHA-256 hex (unique), `amr TEXT[]`, `auth_time`, `issued_at`, `idle_expires_at`, `absolute_expires_at`, `rotated_at`, `revoked_at`, `revoked_reason` (ROTATED, REUSE, LOGOUT, DEVICE_REVOKED, WORKER_SUSPENDED, ADMIN, PIN_CHANGED), `pin_change_required`. Partial indexes on `identity_id` and `device_id` WHERE `revoked_at IS NULL`; index on `family_id` |
| `field_policies` | One optional row per client; absent means defaults |
| `field_nonces` | `nonce_hash BYTEA` PK (SHA-256), `expires_at`; consumed with `DELETE ... RETURNING` |
| `field_login_attempts` | Sign-in log: client, identity (NULL if the code is unknown), worker_code, device, application, `auth_method` (PIN or PIN_CHANGE), `outcome`, `failure_reason`, `ip_address`. Dedicated table, not `iam_login_attempts` (that one keys on a principal and an email-shaped identifier and feeds the employee throttle). **Not purged**: retention is an open item |

Purger (every minute with the other auth purges): delete expired nonces; enrolment codes used or
expired more than 1 day ago; sessions whose `absolute_expires_at` or `revoked_at` is more than 30
days old.

## 2. Admin API (huma, `/api/*`)

See the wire contract. Permissions `platform:iam:field-worker:view|manage`,
`platform:iam:field-device:view|manage`, `platform:iam:field-token:introspect`; role
`platform:field-administrator` holds the first four. Event types (all `platform:iam:...`):
`field-worker:created | ensured | updated | deleted | suspended | activated | pin-reset |
pin-changed | unlocked | app-granted | app-revoked`, `field-device:enrolment-code-created |
enrolled | suspended | activated | revoked | throttled`, `field-session:revoked`,
`field-policy:updated`. `pin-changed` (self-service change, payload `familyId`) was added in slice B.
Every use case writes its audit row through the usecase envelope; PINs, codes and refresh tokens are
never in an event, an audit row or a log.

## 3. PIN hashing

`input = base64url(HMAC-SHA256(pepper, clientId || 0x00 || identityId || 0x00 || pin))`, hashed with
Argon2id (m=19456 KiB, t=2, p=1, 16-byte salt, 32-byte key), verified with the platform's shared
password verifier. `pepper = HKDF-SHA256(FLOWCATALYST_APP_KEY, salt=none, info="fc-field-pin", 32)`;
`pepper_version` 1. `LEGACY` = an imported unpeppered bcrypt or Argon2i hash: verified on the RAW
PIN, rehashed to PEPPER_V1 on the first successful verify (inside the login transaction). A worker
that does not exist, is not ACTIVE or has no PIN costs one dummy verify with the same parameters
(timing parity). **No app key: fail closed.** Every PIN set/verify errors, every `/field/*` route
answers `503 FIELD_IDENTITY_UNCONFIGURED`, admin PIN reset answers the same. PIN policy: digits
only, 6..12 (4 only when the client row sets it, then the hard-lock default is 5), blocklist of
single-digit repeats, straight runs (ascending, descending, wrap-around), a short common list and
the worker code itself.

## 4. Device protocol (`/field/*`)

Public chi routes, outside the API auth middleware, behind a per-IP governor (own env:
`FC_FIELD_RATE_PER_MIN` 600, `FC_FIELD_BURST` 200, looser than browser flows because a depot shares
one NAT address). Wire details are in the wire contract; the points a port must get exactly right:

- **Proof**: ES256 only; the JWS header carries the public JWK; thumbprint per RFC 7638 recomputed
  from the decoded coordinates (do not trust the sent member encoding); `htu` compared with the
  CONFIGURED public base URL plus the request path, never the Host header; `iat` accepted within
  +-24 h; `jti` and `iat` required; `nonce` required on every route but `/field/auth/nonce`; `ath`
  on bearer routes. The proof signature is raw `r||s` (64 bytes).
- **Nonces**: random 32 bytes base64url, stored as SHA-256, TTL 5 min, single use. A fresh nonce on
  EVERY response (the Go code issues and stores one before the handler runs, including for 4xx).
  Missing, used or expired: `401 use_dpop_nonce` + `WWW-Authenticate: DPoP error="use_dpop_nonce"`.
  The nonce is consumed BEFORE any device or DB lookup, so a replayed proof always fails on the nonce.
- **Order of checks in `prelude`**: proof structure/signature, htm, htu, nonce, device by `jkt`
  (UNKNOWN_DEVICE / SUSPENDED / REVOKED), then for the bearer route the access token.
- **Login order** (spec 4.2): device ACTIVE, device and client failure buckets (read-only count),
  worker lookup (unknown: dummy verify, count failure), worker ACTIVE and has PIN (else dummy verify),
  hard lock 423, soft lock 429 + Retry-After, PIN verify, then access checks, then success.
  Locking: the whole sign-in runs in ONE transaction that `SELECT ... FOR UPDATE`s the worker row,
  so concurrent wrong PINs serialise (no lost increments, and parallel guessing is impossible per
  worker). The failure path COMMITS the counter update; the refusal paths roll back. The Go hot
  paths use the repository inside a transaction (`fieldidentity.AuthTx`) rather than the usecase
  envelope for exactly this reason (commit-on-failure); everything else changes state through
  the envelope (see section 6).
- **Lock arithmetic**: failure n increments `failed_since_success`; `n >= soft_lock_threshold` sets
  `locked_until = now + soft_lock_secs` (re-armed by every further failure); `n >= hard_lock_threshold`
  sets `hard_locked`. Soft-locked and hard-locked requests are refused BEFORE the PIN is verified and
  do not count. A success resets the counter and `locked_until`. Admin unlock and PIN reset clear all.
- **Buckets** (rate-limit store): `field_pin_device:<deviceId>` (20/h) and `field_pin_client:<clientId>`
  (300/h) count FAILURES only (unknown worker and wrong PIN; not suspended-worker or access failures).
  The login pre-check reads the count without recording, so the store gained a read-only `Count`
  capability (Postgres and Redis); a store without it (the no-op store used when rate limiting is
  disabled) means no pre-check. When a failure takes a bucket to its limit, one
  `field-device.throttled` event (scope DEVICE or CLIENT) is emitted per bucket per window, deduped by
  a marker bucket (`field_throttle_notice_*`, limit 1 per window). Block condition: `count >= limit`.
- **Refresh** (rotation, reuse, grace): lock order is identity row first, then session row (the
  self-service PIN change takes the same order). `Rotate` stamps `revoked_at = rotated_at = now`,
  reason ROTATED, and the child is inserted with `issued_at = now` (same instant). A ROTATED token
  presented within 60 s of `rotated_at` by the session's own device is a retry: the family's live
  head is rotated and a fresh child issued. Outside 60 s, or from a different device (live or
  rotated token), the whole family is revoked REUSE through the envelope and
  `field-session:revoked` is emitted; the response is `401 INVALID_GRANT`. The child's
  `pin_change_required` is taken from the worker's CURRENT flag (so refreshing after a PIN change
  drops `pcr`); idle = now + mode idle; absolute deadline is inherited.
- **PIN change**: `/field/auth/pin` verifies the current PIN under the same lock/lockout accounting
  as login, resets the soft counters on success, then runs the `ChangeOwnPin` use case (policy
  check, hash, event, revoke the worker's OTHER families `PIN_CHANGED`, clear `pcr` on the current
  family). `422 PIN_POLICY {violations[]}`; the new PIN equal to the current one is
  `SAME_AS_CURRENT`.
- **Logout**: `session` revokes the family, `device-all` every session on the proof's device
  (reason LOGOUT). An unknown, foreign-device or already-dead token is a silent `204`. No event.

## 5. Access token

Signed with the existing platform signer and `kid`; JOSE `typ: fc-field+jwt`; TTL
`FC_FIELD_ACCESS_TOKEN_TTL_SECS` (default 600). Claims: `iss`, `sub` (worker id), `aud` (the
application CODE as a bare string), `exp`, `iat`, `nbf`, `jti`, `token_use:"field"`, `client_id`,
`client_identifier`, `worker_code`, `name`, `device_id`, `device_mode`, `sid` (family id),
`amr:["pin"]`, `auth_time`, `cnf:{jkt}`, `pcr:true` only when a PIN change is required. No roles,
scope, applications or clients. Introspection (`POST /api/field-tokens/introspect`) verifies
signature, issuer, expiry, typ and token_use, then reads live state: the family has a non-revoked,
unexpired session, the worker and the device are ACTIVE, and the token is NOT `pcr`. Never an error
for a bad token, just `active:false`.

Platform API middleware: `token_use` is an allowlist (`api` or absent), so a field token as a
`Bearer` to `/api/*` is 401. The Go SDK `TokenValidator` has the same allowlist.

## 6. What goes through the usecase envelope, and what does not

| Operation | Path |
|---|---|
| EnrolDevice (consume code, insert device, `device:enrolled`) | `TxOperation`, Authorize = Public (the code is the credential), principal `field:enrolment` |
| ChangeOwnPin | `TxOperation`, Public (protocol layer authenticated), principal = worker id |
| RevokeSessionFamilyOnReuse | `TxOperation`, Public, principal = worker id |
| NoteThrottled (`device:throttled`) | `TxOperation`, Public, principal `field:throttle` |
| All admin writes | slice A `Ops` |
| Login, refresh, logout, failure counters, last_login, LEGACY rehash, session insert/rotate | `AuthTx` (pgx transaction) directly, no event, no audit row; the sign-in log (`field_login_attempts`) is the record |

## 7. SDK verifier (`pkg/fcsdk/auth`)

`FieldTokenVerifier.Verify(ctx, authorizationHeader, dpopHeader, method, url)`: the Authorization
scheme must be `DPoP` (Bearer is refused: it would skip the proof). Order and refusal reasons:
`malformed` (scheme), `wrong_typ`, `invalid_token` (signature, issuer), `expired`,
`wrong_token_use`, `wrong_aud`, `pcr`, `jkt_mismatch` (no `cnf.jkt`), `missing_proof`,
`invalid_proof`, then proof vs token: `jkt_mismatch`, `htm_mismatch`, `htu_mismatch`
(scheme, host, path; query ignored), `ath_mismatch`, `stale_proof` (iat beyond +-5 min, configurable).
The nonce is NOT checked at the application backend in P1 (replay protection beyond the iat window
is the framework's stateless nonce, design 6.1). Keys come from OIDC discovery or an explicit JWKS
URL. `FieldMiddleware` puts a `FieldPrincipal` in the request context. A Java port should run the
whole `api/field-fixtures/cases.json` suite.

## 8. Decisions (slice B, where the spec was silent)

1. Edited migration 095 in place to add `field_devices.attestation_chain` (the spec says the chain is
   "stored raw" but 095 had no column). Safe because 095 has not shipped; the Java port should
   include the column in its copy.
2. New event `field-worker:pin-changed` for the self-service change (reusing `pin-reset` would blur
   supervisor resets and the worker's own change). Catalogued with schema `worker + optional familyId`.
3. A `pcr` token is never `active` at introspection (an app must not treat a PIN-change-only token as
   a work credential); it IS accepted on `/field/auth/pin`.
4. The nonce endpoint requires a valid proof too (a proof without a nonce); any key is accepted.
5. Error codes the spec left open: `WORKER_LOCKED_TEMPORARILY` (soft lock 429),
   `TOO_MANY_ATTEMPTS` (bucket 429), `INVALID_GRANT` (bad, used or revoked refresh token),
   `SESSION_EXPIRED`, `INVALID_ENROLMENT_CODE` (401), `CLIENT_INACTIVE` (403), `invalid_dpop_proof` /
   `invalid_token` (401 with `WWW-Authenticate`), `INVALID_REQUEST` / `INVALID_PUBLIC_KEY` (400).
6. A refresh for a worker whose grant, application, device enrolment or client is gone answers
   `403 NO_APP_ACCESS`; for a worker that is no longer ACTIVE, `401 INVALID_GRANT`.
7. A rotated or live refresh token presented from a DIFFERENT enrolled device revokes the family at
   once (REUSE), even inside the 60 s grace.
8. Access checks (grant, app active, device apps, PERSONAL assignment, client ACTIVE) run only after
   the PIN verifies and do not change the failure counters; `NO_APP_ACCESS` is one response for all
   of them (the reason is in `field_login_attempts.failure_reason`: NO_GRANT, APP_UNAVAILABLE,
   DEVICE_APP_NOT_ENROLLED, DEVICE_ASSIGNED_TO_OTHER, CLIENT_INACTIVE).
9. PIN longer than 64 characters: dummy verify, `401 INVALID_CREDENTIALS`, not counted.
10. Enrol requires `platform` (max 40) and `model` (max 120) as the spec's signature lists them.
    The enrolment code is upper-cased and trimmed before hashing.
11. Throttle pre-check counts `>= limit` (the 21st attempt in an hour is blocked, not the 22nd).
    With `FC_RATE_LIMIT_DISABLE` the PIN-failure buckets are off (the per-worker locks still apply).
12. Fixtures are signed ES256 with fixed seed-derived keys and a deterministic nonce (HMAC of the
    key and digest): public test keys only, never a pattern for real signing.

Carried from slice A (rulings of 2026-11-03 recorded in the spec): idempotent ensure on
`(clientId, workerCode)`, supervisor-entered PIN forces a change, random temporary PIN shown once,
an application-confined service account may only name its own applications, enrolment code 10
characters from `23456789ABCDEFGHJKMNPQRSTUVWXYZ`, 10 minute TTL, single use, stored hashed, the QR
payload is `fcfield:1:<base URL>:<code>`, `SHARED` + `assignedWorkerId` is 400, PERSONAL idle 7200 s
and absolute 50400 s, SHARED idle 900 s and absolute 43200 s (constant), default policy values in the
wire contract.

## 9. Not done / open

Badges, unlock keys, offline receipts, legacy import endpoint, attestation verification, console
pages, the other SDKs and the mobile libraries (all later phases per the spec). `field_login_attempts`
retention. No per-IP failure bucket on enrolment (the 10-character code and the 10 minute TTL plus
the per-IP governor are the defence).
