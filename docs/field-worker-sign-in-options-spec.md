# Field-worker sign-in options: unenrolled devices, badges, impersonation, test identities

Status: SPEC, 2026-11-06. Branch `feat/field-sign-in-options`. Builds on P1
(`docs/field-worker-identity-design.md`, `docs/field-worker-identity-p1-spec.md`); where this spec
and those disagree, this spec wins. Owner direction (2026-11-06):

- Device enrolment is **opt-in**. A worker code + PIN must work from any device. Biometrics (and
  anything else that depends on knowing the device) still need an enrolled device.
- Workers can sign in with a **printed badge** (QR code).
- **Impersonation**, all three kinds: a supervisor on a device, an admin from the console, and test
  identities.

Decisions marked **(D)** were made by the main session and can be overridden by the owner.

## 0. Scope

Slice 1 (platform core, this spec §1–§6): schema, domain, `/field/*` protocol, admin API,
permissions, events, tokens, introspection, wire contract, fixtures, Java hand-off.

Later slices (separate agents, after slice 1 freezes the wire shapes): console pages (§7), server
SDKs + frameworks (§8), `field-js` (§9).

Not in scope: server-verified biometric unlock keys (design P5), offline login (P6), passkeys (P7),
legacy PIN import (P8), 1D barcodes (QR only; see §3.1).

## 1. Unenrolled devices

### 1.1 Rule

An **unenrolled** request is one whose DPoP proof key is not an enrolled device's key. The proof
is still required on every `/field/*` call: the library generates a key on any device
(non-extractable WebCrypto, or the native module's key) without telling the platform about it
first. Tokens stay bound to that key (`cnf.jkt`), so a stolen token is still useless without the
key. What an unenrolled device loses:

| Feature | Enrolled | Unenrolled |
|---|---|---|
| PIN sign-in | yes | yes, unless the client's policy has `requireEnrolledDevice` |
| Badge + PIN | yes | yes, unless `requireEnrolledDevice` |
| Badge only (no PIN) | if the client's policy allows it | **never** (D: a photocopied badge must not work from any phone) |
| Biometric local unlock (field-js) | PERSONAL only | never (field-js already checks `device.mode`) |
| Supervisor act-as / console sign-in code | yes | yes, unless `requireEnrolledDevice` |
| Device revoke / suspend, per-device failure budget, device-all logout | yes | no (there is no device row) |
| App restriction of the device (`field_device_apps`) | yes | no |

### 1.2 Policy

`field_policies` gains:

- `require_enrolled_device BOOLEAN NOT NULL DEFAULT FALSE`. When true the protocol behaves exactly
  as P1 (an unknown key gets 401 `UNKNOWN_DEVICE` on every device route).
- `unenrolled_idle_secs INTEGER NOT NULL DEFAULT 900`. Idle limit of an unenrolled session.
  The absolute limit is `SharedShiftMaxSecs` (12 h), the same as SHARED (D: an unknown device
  gets the shared-device rules). Validation: the same bounds the policy validator applies to
  `sharedIdleSecs`.

`DefaultPolicy`, the policy API, the policy validator and the `field-policy:updated` event carry
both fields.

### 1.3 Protocol changes

Route preludes become: `routeNonce`, `routeEnrol` unchanged; `routeDevice` → **device optional**
(look the key up; if found apply the P1 status checks; if not found, `c.device = nil`);
`routeBearer` → device optional, and the access token's `cnf.jkt` must equal the proof `jkt`
(already checked) and, when the token carries `device_id`, the proof key must be that ACTIVE
device.

`POST /field/auth/login` body gains `clientIdentifier` (a client's identifier, as in the
`client_identifier` claim) and `badgeToken` (§3):

```
{ appCode, clientIdentifier?, workerCode? | badgeToken?, pin? }
```

- Exactly one of `workerCode` / `badgeToken` (400 `INVALID_REQUEST` otherwise).
- Client resolution: enrolled device → the device's client; a `clientIdentifier` that names a
  different client → 400 `CLIENT_MISMATCH`. Unenrolled + `workerCode` → `clientIdentifier` is
  required (400 `INVALID_REQUEST` if missing); an unknown or inactive client is answered as
  `INVALID_CREDENTIALS` after a dummy verify (no client enumeration). A badge carries its client
  (§3), so `clientIdentifier` is optional with a badge and, when given, must match (else
  `INVALID_CREDENTIALS`).
- `requireEnrolledDevice` is checked as soon as the client is known: unenrolled + policy true →
  401 `UNKNOWN_DEVICE` (same body as P1).
- Throttling on an unenrolled device: there is no device bucket. Use a new per-IP bucket
  `field_pin_ip` (`FC_FIELD_IP_FAILURES_PER_HOUR`, default 100: a depot shares one NAT address)
  plus the per-client bucket: unenrolled failures count in `field_pin_client_unenrolled`
  (`FC_FIELD_UNENROLLED_CLIENT_FAILURES_PER_HOUR`, default 300), which never refuses an enrolled
  device; an unenrolled key is refused by either per-client bucket. Throttle-notice event: scope `IP` with `ipAddress` in the data
  (new notice bucket `field_throttle_notice_ip`). Enrolled devices keep the device bucket and do
  not count against the IP bucket.
- On success the session has `device_id = NULL`, `jkt = proof jkt`, idle =
  `unenrolled_idle_secs`, absolute = `SharedShiftMaxSecs`.
- Grant checks after the PIN: as P1, minus `DEVICE_APP_NOT_ENROLLED` and
  `DEVICE_ASSIGNED_TO_OTHER` when unenrolled.

`POST /field/auth/refresh`: the session's `jkt` must equal the proof `jkt`; a mismatch is theft
(revoke the family, as a different device is today). For a session with a device the P1 device
check still applies. A device that became enrolled after the session started (same key) keeps the
session's unenrolled limits until the next login (D: no mid-session upgrade).

`POST /field/auth/logout`: the session must belong to the proof key (`jkt`), not only the device.
`scope: "device-all"` on an unenrolled key → 400 `INVALID_REQUEST` ("device-all needs an enrolled
device").

`POST /field/auth/pin`: works unenrolled (bearer + proof key match).

### 1.4 Schema

```
ALTER TABLE field_sessions ALTER COLUMN device_id DROP NOT NULL;
ALTER TABLE field_sessions ADD COLUMN IF NOT EXISTS jkt VARCHAR(64);
-- backfill: jkt from field_devices for existing rows (tables are small; new feature)
```

New sessions always set `jkt`. Index `(jkt) WHERE revoked_at IS NULL` is not needed (lookups are by
refresh hash). `field_login_attempts` gains `auth_method` values (§3, §4) and an `actor_id`
column (§4); `device_id` is NULL for unenrolled attempts.

### 1.5 Token claims

Unenrolled: `device_id` is **omitted**; `device_mode` = `"UNENROLLED"`. Everything else as P1.
`TokenUsable` / introspection: skip the device liveness check when the token has no `device_id`;
the session row check still applies.

## 2. (reserved)

## 3. Badges (QR)

### 3.1 Badge token

- Format: `FWB1` + 26 Crockford base32 characters (130 random bits), upper case. Upper-case
  alphanumerics use the QR alphanumeric mode (small, robust codes) and type correctly through a
  rugged scanner's keyboard wedge. Compare case-insensitively (normalise to upper before hashing).
- Stored as SHA-256 hex in `field_badges.token_hash` (UNIQUE). The token is shown **once**, at
  issue; reprinting means reissuing (D: someone who can read the database cannot make badges).
- One ACTIVE badge per worker (D): issuing revokes the worker's earlier ACTIVE badges in the same
  transaction. A lost badge = revoke (or reissue).
- QR content is the bare token string (no URL), so any scanner app or wedge yields exactly the
  token.

### 3.2 Schema

```
field_badges  id fwb_ PK, client_id, identity_id (FK field_identities ON DELETE CASCADE),
              token_hash VARCHAR(64) UNIQUE, label VARCHAR(255) NULL,
              status ACTIVE|REVOKED, created_by VARCHAR(100), created_at, revoked_at,
              last_used_at
              INDEX (identity_id) WHERE status = 'ACTIVE'
```

`field_policies` gains `badge_only_login BOOLEAN NOT NULL DEFAULT FALSE`.

### 3.3 Sign-in with a badge

`POST /field/auth/login` with `badgeToken`:

1. Normalise and hash; look up an ACTIVE badge. Unknown or REVOKED → dummy verify, count a
   failure (device or IP bucket, plus the client bucket when known), attempt reason
   `UNKNOWN_BADGE` / `BADGE_REVOKED`, answer 401 `INVALID_CREDENTIALS`.
2. The badge gives the worker and client. Then the normal order (policy, enrolment requirement,
   worker ACTIVE, locks).
3. PIN: required unless the policy has `badgeOnlyLogin` **and** the device is enrolled. When the
   PIN is required and missing → 400 `PIN_REQUIRED` (after steps 1–2 only, so it reveals nothing
   beyond "this badge exists", which holding the badge already proves). A wrong PIN is the normal
   failure path (counts towards the worker's lock).
4. Badge-only success still requires: worker ACTIVE and not locked (hard or soft), grant, app,
   device checks.
5. `amr` = `["badge","pin"]` or `["badge"]`; attempt `auth_method` = `BADGE_PIN` / `BADGE`.
   `last_used_at` updated on success.

### 3.4 Admin API (permission `platform:iam:field-worker:manage`, client-boxed like the other
worker routes)

- `POST /api/field-workers/{id}/badges` body `{label?}` → 201 `{badge: FieldBadge, token}`.
  `token` appears only here.
- `GET /api/field-workers/{id}/badges` → `{badges: FieldBadge[]}` (all statuses, newest first;
  never the token or hash).
- `POST /api/field-workers/{id}/badges/{badgeId}/revoke` → `FieldBadge`.
- `POST /api/field-workers/badges` (bulk, for printing a sheet) body `{workerIds: string[]}`
  (1–200, all in clients the caller can access; else 403/404 per P1 rules, nothing issued) →
  201 `{issued: [{workerId, workerCode, name, badge, token}]}`. One use case, one transaction.
- `FieldBadge = {id, workerId, label?, status, createdAt, createdBy?, revokedAt?, lastUsedAt?}`.
- `GET /api/field-workers/{id}` gains `badge: {id, status, createdAt, lastUsedAt?} | null` (the
  ACTIVE badge).

Events: `platform:iam:field-worker:badge-issued` `{workerId, clientId, badgeId, revokedBadgeIds[]}`,
`platform:iam:field-worker:badge-revoked` `{workerId, clientId, badgeId}`. Bulk issue emits one
`badge-issued` per worker. Deleting a worker deletes its badges (cascade).

## 4. Impersonation

All impersonated sessions share one model:

- `field_sessions` gains `actor_type VARCHAR(20) NULL` (`FIELD_WORKER` | `PRINCIPAL`) and
  `actor_id VARCHAR(17) NULL` (`fwu_…` / a principal id), copied to every rotated child.
- The token carries `act: {sub, typ, name}` (RFC 8693 shape; `typ` = `"field_worker"` or
  `"principal"`; `name` = the actor's display name) and `amr` ends with `"imp"`.
- The `sub`, `worker_code`, `name`, `client_*` claims are the **worker's**: apps see the worker,
  and must read `act` to know someone else is acting.
- Impersonated sessions: `pcr` is never set (the worker's pending PIN change is not forced on the
  actor); absolute limit = min(the normal limit, `impersonation_max_secs`, and for a supervisor
  the supervisor's own session absolute expiry); idle limit as normal.
- Forbidden while impersonating: `POST /field/auth/pin` (403 `IMPERSONATING`), act-as (403
  `IMPERSONATING`, no nesting).
- `field_policies` gains `impersonation_max_secs INTEGER NOT NULL DEFAULT 14400` (validation
  300–43200).
- `field_login_attempts` gains `actor_id VARCHAR(17) NULL`; method `ACT_AS` / `SIGN_IN_CODE`.
- Event `platform:iam:field-worker:impersonated` `{workerId, clientId, appCode, actorType,
  actorId, method: "ACT_AS"|"SIGN_IN_CODE", deviceId?, sessionId}`.
- Revocation: suspending, disabling, deleting or locking-out-by-admin a FIELD_WORKER actor also
  revokes the sessions where it is the actor (`revoked_reason = ACTOR_SUSPENDED`), in the same
  transaction as the existing worker revocation. Revoking the worker revokes impersonated sessions
  of that worker too (they are the worker's sessions).
- Introspection returns `act` (`{sub, typ}`) when present.

### 4.1 Supervisors on a device (`can_impersonate`)

`field_identities` gains `can_impersonate BOOLEAN NOT NULL DEFAULT FALSE`. Set through
`POST /api/field-workers` (ensure) and `PUT /api/field-workers/{id}` as `canImpersonate`;
changing it requires `platform:iam:field-worker:impersonate` (§4.4) in addition to `manage`
(D: granting the power to act as others is the impersonate permission's call). Clearing it
revokes sessions where this worker is the actor. Event: the existing `field-worker:updated` carries
`canImpersonate`.

`POST /field/auth/act-as` (bearer = the supervisor's own field token, proof, nonce):

```
{ workerCode? | workerId?, pin }   →  FieldTokenResponse (for the worker, with act)
```

Order:
1. Bearer + proof as P1. The bearer token's session must not be impersonated (403
   `IMPERSONATING`).
2. Load the supervisor (token `sub`), lock row; ACTIVE, `can_impersonate`, not locked. Not
   permitted → 403 `IMPERSONATION_FORBIDDEN` (no PIN check; the supervisor is already
   authenticated).
3. Verify `pin` against the **supervisor's** PIN with full P1 lockout accounting (wrong → 401
   `INVALID_CREDENTIALS`, counts towards the supervisor's lock and the device/IP + client
   buckets). Step-up (D): an unattended signed-in handheld must not let anyone act as anyone.
4. Target: same client as the supervisor; exists and ACTIVE (a locked target is allowed: covering
   for a locked-out worker is a normal reason); not `can_impersonate` (D: no acting as another
   supervisor); not the supervisor; granted the token's application. Any failure → 403
   `NO_APP_ACCESS` with the reason recorded only in the attempt row (same no-leak rule as login).
5. Device checks as login (enrolled device app restriction; a PERSONAL device assigned to someone
   else refuses, unless it is assigned to the supervisor).
6. New session family for the worker on the same key/device, `actor_type = FIELD_WORKER`,
   `actor_id = supervisor`; `amr = ["pin","imp"]` (the supervisor's step-up PIN). Attempt row
   `ACT_AS` with `actor_id`. Event `impersonated`.

The supervisor's own session is untouched. The library keeps it and returns to it when the
impersonated session ends (logout of that family).

### 4.2 Admins from the console (sign-in codes)

A platform principal cannot hold a device key in the console, so the console issues a short-lived
code that the target app redeems on whatever device the admin is using.

`field_sign_in_codes`: `id fwi_ PK, client_id, identity_id, application_id, code_hash VARCHAR(64)
UNIQUE, created_by (principal id) NOT NULL, created_by_name, expires_at, used_at, created_at`.
Code: `FWS1` + 26 Crockford base32 (as badges), valid 10 minutes, single use.

Admin API:
`POST /api/field-workers/{id}/sign-in-codes` body `{appCode}` →
201 `{code, expiresAt, workerId, appCode}`.
- Permission: `platform:iam:field-worker:impersonate`; **or** `platform:iam:field-worker:manage`
  when the target is a test identity (§4.3). Client-boxed as other worker routes.
- Caller must be a human principal (USER), not a service account, unless the target is a test
  identity (D: service accounts may drive automated QA with test identities only).
- Target ACTIVE and granted the app; else 409 `WORKER_UNAVAILABLE` / `NO_APP_ACCESS`.
- Event `platform:iam:field-worker:sign-in-code-created` `{workerId, clientId, appCode,
  createdBy, expiresAt}`. Audit through the envelope as usual.

Device side: `POST /field/auth/sign-in-code` (proof + nonce, device optional) body `{code}` →
`FieldTokenResponse` with `act = {sub: <principal id>, typ: "principal", name}`, `amr =
["code","imp"]`.
- Unknown / used / expired code → 401 `INVALID_CODE`; counts against the device/IP bucket.
- Consume atomically (`UPDATE … SET used_at = now() WHERE code_hash = $1 AND used_at IS NULL AND
  expires_at > now() RETURNING …`).
- Re-check at redemption: worker ACTIVE + grant + app + client active; `requireEnrolledDevice`;
  device app restriction. The creator principal must still be ACTIVE (look it up; inactive →
  401 `INVALID_CODE`).
- Session: `actor_type = PRINCIPAL`, `actor_id = creator`. Attempt row `SIGN_IN_CODE`. Event
  `impersonated` (method `SIGN_IN_CODE`).
- The purger deletes codes 1 day after expiry.

### 4.3 Test identities

`field_identities` gains `is_test BOOLEAN NOT NULL DEFAULT FALSE`, set through ensure/update as
`isTest` (`manage` permission). Effects:
- Tokens carry `"test": true` (omitted otherwise); introspection returns `test`.
- Sign-in codes for a test identity need only `manage`, and service accounts may create them
  (§4.2), so app QA can sign in as a test worker without a PIN.
- Marking an EXISTING real worker as a test identity (`isTest` false to true on update) needs what signing
  in as a real worker needs (`impersonate`, human caller; otherwise `403`), so `manage` alone cannot
  reach the looser test rule. Creating a new test worker needs only `manage`. Unmarking (true to false)
  expires every unredeemed sign-in code of the worker in the same transaction.
- Listed with a filter: `GET /api/field-workers?isTest=true|false`.
- Events carry `isTest` on `created`/`ensured`/`updated`.
- Otherwise a normal identity (PIN, locks, grants, badges all work), so QA can exercise them.

### 4.4 Permission

New `platform:iam:field-worker:impersonate`, seeded in `seed/permissions.go`, added to the seeded
role `platform:field-administrator`. The seeder does not update existing roles, so the migration
also inserts the permission for that role (`INSERT … SELECT … ON CONFLICT DO NOTHING`, as
migration 100 does).

## 5. Wire contract, fixtures, docs

- `docs/wire-contract.md` "Field identity": the new login fields, the unenrolled claim shape, the
  `act`/`test` claims, the new routes and error codes (`CLIENT_MISMATCH`, `PIN_REQUIRED`,
  `IMPERSONATING`, `IMPERSONATION_FORBIDDEN`, `INVALID_CODE`), the new policy fields.
- `api/field-fixtures/`: add cases `valid_unenrolled` (no `device_id`, `device_mode:
  "UNENROLLED"`), `valid_impersonated_supervisor`, `valid_impersonated_principal`, `valid_test`.
  Existing cases unchanged. Each case lists the expected principal fields
  (`actor`, `test`, empty `deviceId`), so the SDKs can assert them.
- Go SDK verifier (`pkg/fcsdk/auth/field.go`): `FieldPrincipal` gains `Actor *FieldActor{ID,
  Type, Name}` and `Test bool`; `DeviceID` may be empty. Fixture test passes.
- `docs/published/` user doc for field workers if one exists; otherwise none.
- Design doc: add a short status note pointing here; mark "badges" as built.
- Java hand-off: `docs/java-handoff-2026-11-06-field-sign-in-options.md`, in the style of
  `docs/java-handoff-2026-11-03-field-identity.md`.

## 6. Tests (integration, `-tags=integration`)

At minimum, each with a mutation check by the implementer on the guarded line:

Unenrolled: PIN login with `clientIdentifier` succeeds, token has no `device_id` and
`device_mode=UNENROLLED`; missing `clientIdentifier` → 400; unknown client → 401 indistinguishable
from wrong PIN; `requireEnrolledDevice` → 401 `UNKNOWN_DEVICE`; refresh with another key → family
revoked; idle = `unenrolled_idle_secs`; IP bucket throttles and emits the IP notice; enrolled
devices still use the device bucket; logout `device-all` refused; PIN change works.

Badges: issue returns token once and revokes the previous; login badge+PIN ok (`amr`);
badge-only refused without the policy (400 `PIN_REQUIRED`), allowed with policy on an enrolled
device, refused (`PIN_REQUIRED`) on an unenrolled one even with the policy; revoked badge →
401; lower-case token accepted; locked worker refused on badge-only; bulk issue all-or-nothing on
a foreign client.

Impersonation: act-as happy path (claims `act`, `amr`, no `pcr`, absolute capped by the
supervisor's session); wrong supervisor PIN counts towards the supervisor's lock; non-supervisor
403; target supervisor / other client / no grant → 403 `NO_APP_ACCESS`; nested act-as 403; PIN
change while impersonating 403; suspending the supervisor revokes the impersonated session;
clearing `canImpersonate` revokes them; setting `canImpersonate` without the impersonate
permission → 403. Sign-in code: create needs impersonate (or manage for a test identity); service
account refused for a real worker; redeem once (second → 401); expired → 401; inactive creator →
401; token `act.typ=principal`; test identity token has `test: true`.

Gates: `go build ./...`, `go vet ./...`, the integration packages for fieldidentity, fieldauth,
fieldidentity/api, migrate, seed, ratelimit, `pkg/fcsdk/auth` tests,
`go run ./tools/dump-spec > api/openapi.lock.json` then `make api-diff`, `make lint` if it runs
in reasonable time.

## 7. Console (later slice)

`/ui` field pages: worker drawer shows the badge (issue / reissue / revoke) with a printable
badge card (name, worker code, client, QR as inline SVG from `boombuler/barcode`, already a
dependency, see `internal/ui/pages/auth_mfa_qr.go`; `@media print` styles); a "Print badges"
bulk action on the worker list (selection → sheet of cards); `canImpersonate` and `isTest`
switches (the first only with the impersonate permission); a TEST tag in the list and a filter;
"Sign in as…" on the worker (pick app → code + QR shown with its expiry); sign-in history shows
method and actor; policy page gets the four new fields.

## 8. SDKs + frameworks (later slice)

TS, Laravel, Java SDK verifiers and the Go/Java framework `FieldTokens`: `deviceId` optional
(the Go framework currently refuses an empty `device_id`: `../flowcatalyst-framework/go/auth/field.go`
line ~199), `actor` and `test` exposed, new fixture cases pass. Framework
`FieldPermissions` by-device-mode helper accepts `UNENROLLED`. Audit inside apps: wherever the
SDK or framework records who performed an action (framework audit/execution context, the SDKs'
audit-batch helpers), an impersonated request records the worker as the subject and the `act`
actor as the operator, so an app's own audit trail shows "done by X as worker Y". Admin wrappers gain badge,
sign-in-code and the new identity flags where the SDK has admin wrappers.

## 9. field-js (later slice)

`login({workerCode | badgeToken, pin?, clientIdentifier?})` works without enrolment (generates and
keeps a key); `redeemSignInCode(code)`; `actAs({workerCode, pin})` keeping the supervisor
session and `endImpersonation()` returning to it; session exposes `actor`, `test`,
`deviceMode`; biometric unlock still PERSONAL-enrolled only; the bridge protocol gains the new ops.
