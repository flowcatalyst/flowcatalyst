# Field-worker identity, P1 build spec (platform core)

Branch `feat/field-identity`. Parent design: `docs/field-worker-identity-design.md` (read §2–§4 and
§9 first; this spec settles what the design left open and splits P1 into two slices). Where this
spec and the design disagree, this spec wins. Each "Decision" below was made by the main session
on 2026-11-03 and can be overridden by the owner.

## 0. Scope

In P1: schema, domain, admin API, permissions, events, audit, PIN hashing and policy, lockout,
device enrolment, device-proof protocol, login/refresh/logout/PIN change, field token minting,
introspection, purge, the Go SDK verifier, fixtures, wire-contract section, Java hand-off doc.

Not in P1: badges (`field_badges`), unlock keys (`field_unlock_keys`), attestation parsing (the
`attestationChain` field is accepted and stored raw, level recorded as reported-but-unverified
`NONE`/`SOFTWARE`), offline receipts, legacy import endpoint (P8; the `pin_scheme` column supports
it now), console pages (P2), the other four SDKs (P3), mobile libraries (P4).

## 1. Ground rules for implementers

- Template modules: `internal/platform/portalidentity` (entity/repo/operations/events/api) and
  `internal/platform/portalauth` (public endpoints outside the API middleware, chi handlers).
  Mirror their structure, naming and test style.
- Writes go through the usecase envelope: read `docs/usecase-pattern.md` and
  `docs/usecase-envelope-migration-handover.md`. A use case is a business operation (e.g.
  `ResetWorkerPin`, `RevokeDevice`, `EnrolDevice`), not "write X".
- CRUD helpers: `apiroute`, `repocommon`, `validate` (see `CONVENTIONS.md`).
- Migration: one goose file, the next free number (095 today: `internal/migrate/sql`). Follow
  `docs/migration-bootstrap.md`. sqlc queries in `internal/sqlc/queries`; regenerate with the
  Makefile target, never hand-edit generated code.
- TSID prefixes: add `fwu fwd fwe fws` to `internal/tsid/tsid.go` (badge/unlock-key prefixes later).
- Integration tests are `*_pg_test.go` and need `-tags=integration`; plain `go test` skips them
  silently. Harness: `internal/testpg`.
- Never `pkill`/`killall` by pattern; never touch a running fcdev or its database. Never
  `git commit -a`; stage explicit paths. Do not push.
- After API changes: `make api-lock` equivalent (`go run ./tools/dump-spec > api/openapi.lock.json`)
  then `make api-diff` must pass.

## 2. Schema (migration 095)

As design §4.4 for `field_identities`, `field_identity_apps`, `field_devices`, `field_device_apps`,
`field_enrolment_codes`, `field_sessions`, `field_policies`, plus:

```
field_nonces   nonce_hash BYTEA PK, expires_at TIMESTAMPTZ NOT NULL   -- single-use (§4)
```

Column notes: `field_identities.pin_scheme` ∈ `PEPPER_V1 | LEGACY` (LEGACY = an imported
unpeppered bcrypt/argon2i hash, verified on the raw PIN, then rehashed to PEPPER_V1 on first
success). `field_sessions.amr` TEXT[]; `revoked_reason` ∈ `ROTATED | REUSE | LOGOUT |
DEVICE_REVOKED | WORKER_SUSPENDED | ADMIN | PIN_CHANGED`. Add `field_sessions.rotated_at`
(for the retry grace, §5.3) and `field_sessions.pin_change_required BOOL`. Index sessions on
`(identity_id) WHERE revoked_at IS NULL`, `(device_id) WHERE revoked_at IS NULL`, `(family_id)`.
`field_policies` rows are optional: absent = defaults (§3.2).

## 3. Slice A: domain, storage, admin API

### 3.1 Packages
- `internal/platform/fieldidentity`: entities (Identity, Device, EnrolmentCode, Session, Policy),
  repository, operations (use cases), events.
- `internal/platform/fieldidentity/pin`: policy check + hashing (below). Pure, unit-tested.
- `internal/platform/fieldidentity/api`: huma admin endpoints (design §4.2).

### 3.2 PIN policy and hashing
- Defaults (overridable per client in `field_policies`): `pin_min_length` 6 (4 allowed only when a
  client row sets it; then `hard_lock_threshold` defaults to 5), digits only, max length 12;
  blocklist: all-same digit, ascending/descending runs (`123456`, `654321`, wrap-around too), and a
  short top-N list (`121212`, `112233`, `696969`, `000000`-style already covered, plus the worker
  code itself). `soft_lock_threshold` 5, `soft_lock_secs` 900, `hard_lock_threshold` 10,
  `personal_idle_secs` 7200, `shared_idle_secs` 900, `shift_max_secs` 50400 (PERSONAL) and 43200
  (SHARED; constant, not a column), `pin_reauth_days` 7, `require_attestation` false.
- Hash input: `base64url(HMAC-SHA256(pepper, clientId ‖ 0x00 ‖ identityId ‖ 0x00 ‖ pin))`, then
  `passwordhash.HashWithParams(input, Params{argon2id m=19456 t=2 p=1})`. Verify with
  `passwordhash.Verify`.
- Pepper: HKDF-SHA256 over `FLOWCATALYST_APP_KEY`, info `"fc-field-pin"`, `pepper_version` 1.
  **Decision: fail closed.** No app key ⇒ every PIN set/verify returns an error; `/field/*`
  answers `503 FIELD_IDENTITY_UNCONFIGURED`; admin PIN reset answers the same. Never hash
  without the pepper.
- Not-found worker on login: run one dummy verify with the same params (timing parity, like
  `passwordhash.EqualizeTiming`).

### 3.3 Admin API (huma, `/api/*`, client-boxed via `AuthContext.CanAccessClient`)
Design §4.2, minus badges. Concretely:
- `POST /api/field-workers` `{clientId, workerCode, name, email?, phone?, appCodes?[], pin?}` → 201
  `{id, ..., temporaryPin?}`. Idempotent ensure on `(clientId, workerCode)`: existing row →
  200 with `created:false`, fields not overwritten except `appCodes` grants added. If `pin`
  given it must pass policy and is stored with `pin_change_required=true` (supervisor-entered);
  if absent, no PIN is set (worker cannot log in until reset).
- `GET /api/field-workers?clientId&q&appCode&status&page&pageSize` (paging like portal users),
  `GET/PUT/DELETE /api/field-workers/{id}` (PUT: name, email, phone; DELETE = hard delete, revokes
  sessions first).
- `POST /api/field-workers/{id}/pin-reset` `{pin?}` → `{temporaryPin?, pinChangeRequired:true}`.
  No `pin` ⇒ a random policy-valid temporary PIN returned **once**. Revokes all the worker's
  sessions (`PIN_CHANGED`). Clears lock state.
- `POST /{id}/unlock` (clears soft + hard lock, counters), `/{id}/suspend`, `/{id}/activate`
  (suspend revokes all sessions, emits `field-worker.suspended`).
- `POST /{id}/apps {appCode}`, `DELETE /{id}/apps/{appCode}` (revoking a grant revokes that
  worker's sessions for that application).
- `GET /{id}/sign-ins` (from audit/login-attempt records, newest first, paged).
- `POST /api/field-devices/enrolment-codes {clientId, mode, appCodes[], assignedWorkerId?, label?}`
  → `{code, qrPayload, expiresAt}`. Code: 10 chars from an unambiguous alphabet
  (`23456789ABCDEFGHJKMNPQRSTUVWXYZ`), stored hashed (SHA-256), TTL 10 min, single use.
  `qrPayload` = `fcfield:1:<platformBaseUrl>:<code>`. SHARED with `assignedWorkerId` → 400.
- `GET /api/field-devices?clientId&status&mode`, `GET /api/field-devices/{id}` (incl. active
  session count, last seen), `POST /{id}/suspend` (sessions revoked; resumable), `/{id}/activate`,
  `/{id}/revoke` (terminal; sessions revoked; emits `field-device.revoked`).
- `GET/PUT /api/field-policies/{clientId}` (PUT validates ranges; GET returns effective policy
  with `isDefault`).
- `POST /api/field-tokens/introspect {token}` → `{active, sub, clientId, workerCode, deviceId,
  appCode, amr, authTime, exp}`; `active` false if signature/exp fail, or the session (`sid`
  claim) is revoked, or worker/device not ACTIVE. Never errors on a bad token, just
  `active:false`.
- Permissions (seed in `internal/platform/seed/permissions.go`, four segments, client-delegable):
  `platform:iam:field-worker:view|manage`, `platform:iam:field-device:view|manage`,
  `platform:iam:field-token:introspect`; role `platform:field-administrator` = first four.
  Application-confined service accounts may hold `field-worker:manage` and
  `field-token:introspect`; enforce that such an SA can only grant/act on its own application's
  `appCode` (mirror `sdksync.requireAppAccess` reasoning).
- App codes resolve to platform Applications (active). Unknown/inactive → 400 `UNKNOWN_APPLICATION`.

### 3.4 Events and audit
Events through the existing pipeline, shaped like `portalidentity/events.go`:
`field-worker.created`, `field-worker.suspended`, `field-worker.activated`,
`field-worker.pin-reset`, `field-device.enrolled`, `field-device.suspended`,
`field-device.revoked`, `field-session.revoked` (emitted for admin/reuse/worker revocations, not
for routine rotation or logout). Every use case writes its audit row via the envelope. Logins
(success and failure, with `deviceId`, `authMethod`, failure reason) are recorded; reuse
`iam_login_attempts` if its columns fit a non-principal subject, else audit rows only, and say
which in the hand-off.

## 4. Slice B: device protocol (`internal/platform/fieldauth`)

Public routes mounted like `portalauth` (outside the API middleware, behind the per-IP governor).
All bodies JSON, camelCase. Errors: `{error, message}` shape used by `portalauth`.

### 4.1 Proof (every `/field/*` request)
Header `DPoP: <JWS>`. JOSE header `typ: dpop+jwt`, `alg: ES256` only, `jwk` = public EC P-256 key
(no private members). Claims: `htm` (method), `htu` (scheme+host+path, no query; compare against
the configured public base URL + path, not the Host header), `iat` (present; **Decision:** checked
only within ±24 h, devices have bad clocks), `jti` (present), `nonce` (required except on
`/field/auth/nonce`), `ath` (required on bearer-authenticated calls: base64url SHA-256 of the
access token). Thumbprint = RFC 7638 JWK SHA-256 (`jkt`).

**Nonces (Decision):** random 32 bytes, base64url; stored as SHA-256 in `field_nonces`, TTL 5
min, **single-use** (consumed with `DELETE … RETURNING`). Every `/field/*` response, success or
error, carries a fresh `DPoP-Nonce` header. A missing/used/expired nonce → `401`
`{error:"use_dpop_nonce"}` with `WWW-Authenticate: DPoP error="use_dpop_nonce"` and a fresh nonce,
so the client retries once. Single-use nonces make a `jti` cache unnecessary platform-side.

After enrolment, the proof key must equal an ACTIVE device's `jkt` (`403 DEVICE_REVOKED` if
REVOKED, `403 DEVICE_SUSPENDED` if SUSPENDED, `401 UNKNOWN_DEVICE` if no device). Update
`last_seen_at` at most once a minute.

### 4.2 Endpoints
Exactly design §4.1 minus unlock-keys/unlock/offline-receipts:
- `POST /field/auth/nonce` → `{nonce, expiresAt}`.
- `POST /field/devices/enrol {code, publicKeyJwk, attestationChain?, platform, model, label?}`:
  proof `jwk` must equal `publicKeyJwk`; code valid+unused+unexpired (consume atomically);
  `jkt` unique (409 `DEVICE_KEY_EXISTS`); `require_attestation` policy → reject (`422
  ATTESTATION_REQUIRED`) since P1 cannot verify. → `{deviceId, mode, clientId, appCodes[]}`.
- `POST /field/auth/login {appCode, workerCode, pin}` → `FieldTokenResponse`. Order (design
  §3.3): device ACTIVE → device budget → worker exists (else dummy verify, count device budget,
  401 `INVALID_CREDENTIALS`) → worker ACTIVE (else 401 `INVALID_CREDENTIALS`, after a dummy
  verify) → hard lock (`423 WORKER_LOCKED`) → soft lock (`429` + `Retry-After`) → PIN verify →
  on failure: increment, maybe lock, 401 → on success: **then** check grant + app active + app in
  device apps + (PERSONAL with assigned worker ⇒ must be that worker) → 403 `NO_APP_ACCESS`
  (same code for each, reason in audit only). Success resets counters, sets `last_login_at`,
  rehashes LEGACY.
- `POST /field/auth/refresh {refreshToken}` → `FieldTokenResponse` (§4.3).
- `POST /field/auth/pin {currentPin, newPin}`, bearer (`Authorization: DPoP <access>`; a `pcr`
  token is accepted here and only here) → 204 / 422 `PIN_POLICY` `{violations[]}`. Revokes the
  worker's other sessions (`PIN_CHANGED`), keeps the current family; the client then refreshes to
  get a token without `pcr`.
- `POST /field/auth/logout {refreshToken, scope:"session"|"device-all"}` → 204 (device-all =
  every session on this device, used for SHARED hand-over). Idempotent.

### 4.3 Sessions and tokens
- Refresh token: 32 random bytes base64url prefixed `fwr_`; store SHA-256. One row per issued
  token; a family = one login. Rotation: mark old row `revoked_at`, `revoked_reason=ROTATED`,
  `rotated_at`; insert the child with the same `family_id`, `amr`, `absolute_expires_at`; new
  `idle_expires_at = now + idle`.
- **Reuse (Decision, retry grace):** a ROTATED token presented with a valid proof from the
  session's device key within **60 s** of its rotation is treated as a lost-response retry: revoke
  the child (`ROTATED`) and issue a fresh child. Otherwise a ROTATED token ⇒ revoke the whole
  family (`REUSE`) + `field-session.revoked` event + 401 `INVALID_GRANT`. Rationale: yard networks
  drop responses; a thief without the device key cannot produce the proof anyway.
- Refresh also requires: proof `jkt` = session's device `jkt`; device, worker ACTIVE; grant still
  present; idle and absolute deadlines not passed (else 401 `SESSION_EXPIRED`).
- Idle: PERSONAL `personal_idle_secs`, SHARED `shared_idle_secs`. Absolute: PERSONAL
  `shift_max_secs`, SHARED 43200.
- Access token: `authservice.GenerateFieldAccessToken(FieldSubject)` using the existing signer and
  `kid`, JOSE header `typ: fc-field+jwt`. Claims: `iss`, `sub` (fwu_), `aud` (app code, string),
  `exp` (now + `FC_FIELD_ACCESS_TOKEN_TTL_SECS`, default 600), `iat`, `nbf`, `jti`, `token_use:
  "field"`, `client_id` (clt_), `client_identifier` (the client's identifier/code), `worker_code`,
  `name`, `device_id`, `device_mode`, `sid` (family id), `amr` (`["pin"]` in P1), `auth_time`,
  `cnf: {jkt}`, and `pcr: true` only when a PIN change is required. No roles, scope, applications
  or clients claims.
- `FieldTokenResponse` = design §4.1 (`tokenType: "DPoP"`), no `offlineVerifier`.

### 4.4 Lockout buckets
- Identity: `failed_since_success`, `locked_until`, `hard_locked` on the row (updated in the same
  tx as the verify outcome; use `SELECT … FOR UPDATE` to avoid lost increments).
  Soft lock at `soft_lock_threshold` consecutive failures for `soft_lock_secs`; each further
  failure past the threshold re-arms it; hard lock at `hard_lock_threshold`. Admin unlock or PIN
  reset clears both.
- Device: `shared/ratelimit` Store bucket `field_pin_device:<deviceId>`, 20 failures per rolling
  hour → `429` for logins from that device + one `field-device.throttled` event per hour.
- Client: bucket `field_pin_client:<clientId>`, 300 failures per hour → `429` for the client's
  logins + event. (Both thresholds are env-tunable `FC_FIELD_DEVICE_FAILURES_PER_HOUR`,
  `FC_FIELD_CLIENT_FAILURES_PER_HOUR`; document in `docs/environment-variables.md`.)

### 4.5 Purge
Hook into `StartPurger` (`internal/server/subsystems.go`): delete expired nonces, used/expired
enrolment codes older than 1 day, and sessions whose absolute expiry or revocation is older than
30 days.

### 4.6 Platform middleware and SDK
- Platform middleware: F34 already allowlists `token_use` (`api` | absent). Add an integration
  test that a real field token sent as `Bearer` to an `/api/*` route is 401.
- `pkg/fcsdk/auth`: (1) `TokenValidator.Validate` rejects `token_use` values other than `api`
  or absent (allowlist, with a test); (2) new `FieldTokenVerifier` (`NewFieldTokenVerifier(cfg
  {Issuer, AppCode, HTTPClient})`, `Verify(ctx, authorizationHeader, dpopHeader, method, url)
  (*FieldPrincipal, error)`) enforcing typ, token_use, aud, exp, `cnf.jkt` vs proof, htm/htu/ath,
  refusing `pcr`. Nonce checking at the app backend is out of scope for the SDK in P1 (document:
  apps needing replay protection use the framework's stateless nonce, design §6.1); the SDK checks
  `iat` within ±5 min by default with an option to widen. `net/http` middleware
  `FieldMiddleware(v) func(http.Handler) http.Handler` putting `FieldPrincipal` in the context.
- Fixtures: `tools/field-fixtures` (go run) writes `api/field-fixtures/`: a JWKS, and cases
  `{name, accessToken, dpopProof, method, url, now, expect: "valid"|"<reason>"}` covering valid,
  wrong typ, wrong token_use, wrong aud, expired, pcr, missing proof, jkt mismatch, htm/htu
  mismatch, ath mismatch, api token presented as field token. The Go SDK test runs them all.

## 5. Docs
- `docs/wire-contract.md`: a "Field identity (`/field/*`)" section (routes, proof, nonce, token
  claims, error codes) and the admin endpoints.
- `docs/environment-variables.md`: new env vars.
- `docs/java-handoff-2026-11-XX-field-identity.md` at the end of slice B: what the Java port must
  mirror (schema, routes, token claims, decisions in this spec).
- Update the design doc's status line when each slice lands.

## 6. Gates (each slice)
1. `go build ./...` and `go vet ./...` clean; golangci-lint as configured (`make lint` if present).
2. `go test ./...` and `go test -tags=integration ./internal/platform/fieldidentity/... ./internal/platform/fieldauth/... ./internal/platform/shared/middleware/... ./pkg/fcsdk/auth/...` green; report the exact commands and counts.
3. `make api-diff` clean after regenerating the lock.
4. Integration tests that must exist by end of slice B: brute force (soft then hard lock), spray
   (device bucket trips across many worker codes), refresh rotation, reuse kills family, 60 s
   retry grace, revoked device refused (refresh + login), suspended worker refused, proof replay
   (used nonce refused), wrong-key proof refused, field token rejected as API bearer, pcr token
   only works on `/field/auth/pin`, missing app key ⇒ 503.
5. Commit per slice with explicit paths; message ends with the attribution line given to you.
