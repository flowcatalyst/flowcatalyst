# Field-worker identity: platform-held PINs, device-bound sessions, biometric unlock

Status: DESIGN (2026-10-19), not built. Owner decisions in §9 are open.
Related: `docs/portal-identity-plan.md` (why a separate plane), `docs/portal-apps-and-access-gate.md`
(per-app grants), `docs/2fa-implementation-plan.md` (trusted-device upgrade path names exactly this
work), `docs/auth-hardening-plan.md`, `docs/wire-contract.md`.

Markers: **[repo]** = checked in this repo today; **[web]** = checked against a current source on
2026-10-19 (sources in §11); **[recalled]** = from memory, not re-checked; **[inferred]** = design
judgement.

## 0. Problem and assumptions

Drivers (execution apps), pickers (picking apps) and yard-gate staff (yard-access apps) sign in with
PINs held **inside each application**. The owner wants this moved into the platform, with server SDK
support (Laravel, TS, Go, Java, Rust) and client support for React Native + WebView today and native
Kotlin later.

Two facts are unknown, so the design covers both cases and makes each one a setting on the device
or the client, not a fork in the code:

- **A1 Device tenancy.** Both kinds exist: PERSONAL (a driver's phone or an assigned handheld) and
  SHARED (pool handhelds, gate kiosks, people wearing gloves). Each device is enrolled with a
  `mode`, and the rules for each mode differ (§3).
- **A2 Connectivity.** Yards have patchy coverage. A session started online must keep working
  offline until the end of the shift. Logging in while offline (a shift change at a dead spot) is
  an optional, later phase that the client must turn on (§3.5).
- **A3** Field workers often have no email address. A worker is identified by a **worker code**
  (employee or badge number) unique within the client.
- **A4** Fleet devices are mostly Android. iOS gets PIN-only support plus the WebView parity notes.

## 1. Threat model

| Threat | Field-specific shape | Mitigation (owner) |
|---|---|---|
| Online brute force | 4–6 digit PIN = 10^4–10^6 space. With only a soft lock (5 tries per 15 min) an attacker gets about 480 tries a day, so a 4-digit PIN falls in about 10 days. | Hard lock after N failures **since the last success**; 6-digit minimum; blocklist (platform) |
| PIN spraying | A stolen shared device tries `123456` against 500 worker codes, one try each, so no per-user lock ever trips | **Per-device** failure budget + per-client ceiling (platform) |
| Offline crack of a DB dump | Argon2id cannot save a 10^6 space: the full search costs seconds to hours | **Pepper**: an HMAC under a server secret before Argon2id, so a dump alone is useless (platform) |
| Shoulder surfing | Gate kiosks, queues | Never echo the PIN; optional scrambled keypad (trades off against gloves); badge + PIN on shared devices (client lib) |
| Device theft | A logged-in handheld leaves the yard | Tokens bound to a non-extractable device key (a stolen token without the key is useless); remote revoke; short access tokens; shift-length absolute cap (platform + lib) |
| Token exfiltration | WebView storage dump, rooted device | Proof of key possession on every refresh (DPoP-style); hardware-backed key where available (lib) |
| Shared-device biometrics | Android biometrics belong to the *device*: any enrolled finger passes and the app is not told whose finger it was **[recalled]** | Biometrics are **never** a worker identifier on SHARED devices (policy, platform-enforced) |
| Badge cloning | QR codes photocopy; low-end NFC UIDs clone **[recalled]** | A badge is an identifier ("have"), never the whole credential: badge + PIN (platform) |
| Offline login abuse | A PIN verifier cached on the device can be brute-forced if extracted | Verifier wrapped by a device Keystore key (useless off-device); time-boxed; reconciled on sync (§3.5) |
| Leaver or insider | Worker leaves; PIN shared with a colleague | Central suspend kills every device session at once; per-login audit with device id (platform) |

## 2. What the platform owns vs what stays out

The rule from `portal-identity-plan.md` carries over unchanged: **the platform proves who someone
is; apps decide what they mean.**

Platform owns:
- **PIN credential policy**: length (default 6, minimum 4 only when a client opts in), a blocklist
  (repeats, sequences, a top-N list), temporary PINs with `pinChangeRequired`, and an optional
  rotation period. This mirrors `internal/platform/auth/passwordpolicy`.
- **Hashing**: `argon2id(HMAC-SHA256(pepper, clientId‖identityId‖pin))`, stored in the
  `passwordhash` PHC envelope. Today `passwordhash` hashes at `m=65536,t=3,p=4` **[repo]**. That
  costs 64 MiB per verify, which bursts at 06:00 shift start (around 300 logins a minute per
  replica). Recommend `m=19456,t=2,p=1` (the OWASP minimum **[recalled]**) for PINs, because the
  pepper, not the cost, does the real work against an offline attack. The pepper is derived from
  `FLOWCATALYST_APP_KEY` with HKDF info `"fc-field-pin"`, and a `pepper_version` column allows
  rotation. `passwordhash` already verifies bcrypt and argon2i and flags them for rehash **[repo]**,
  which gives a free import path for legacy app hashes (§9 D6).
- **Lockout and rate limiting**: reuse `loginbackoff` (per-pair exponential backoff plus a global
  ceiling, `FC_LOGIN_*` defaults 3 free tries, 2 s base, 300 s cap **[repo]**) and the
  `shared/ratelimit` Store (Redis or Postgres **[repo]**) under new buckets:
  `field_pin_identity` (soft lock 5 → 15 min; **hard lock at 10 cumulative**, cleared by a
  supervisor or admin unlock), `field_pin_device` (20 failures per hour across any workers → device
  throttled and an alert event raised), and `field_pin_client` (spray ceiling).
- **Device registry**: enrolment, mode, key, attestation level, assigned worker, suspend and
  revoke.
- **Sessions**: rotating refresh families, reuse detection, idle and absolute deadlines.
- **Revocation and events**: `field-worker.suspended`, `field-device.revoked` and
  `field-session.revoked`, published through the existing event pipeline (as
  `portalidentity/events.go` does) so app backends can drop their caches.
- **Audit**: every login, unlock, lock, PIN reset and enrolment, with `deviceId` and `authMethod`
  (`iam_login_attempts` plus audit-log entries).
- **Admin console**: Workforce → Field Workers / Devices / Policy (§4.3).

Stays in apps:
- What a worker may *do*: routes, loads, gates, picking zones, supervisor hierarchy. The platform
  stores only "worker X may sign in to app Y" (grants, as portal apps do).
- Offline business data and its sync.

Client library owns (platform-maintained code, not product code):
- The PIN pad component (optional), device key handling, biometric prompt, secure storage, and
  request signing.

### Biometric local unlock vs server-verified biometric (crisp)

In both cases **the biometric template never leaves the device**: Android BiometricPrompt and the
iOS Secure Enclave match on-device, and the app receives only "succeeded" or a key operation it
could not otherwise perform.

- **Local unlock.** The biometric releases a secret that is *already on the device*, typically the
  refresh token held under a Keystore key. The server sees an ordinary refresh and cannot tell
  whether a biometric happened. Appropriate when the device is PERSONAL, the session is already
  established by PIN, and the goal is convenience inside a shift. It is cheap: no server change.
- **Server-verified.** The biometric gates a private key with auth-per-use: either a Keystore key
  created with `setUserAuthenticationParameters(0, AUTH_BIOMETRIC_STRONG)` **[web]**, or a WebAuthn
  passkey with `userVerification=required`. The server sends a fresh challenge and verifies the
  signature: the UV flag for WebAuthn, the key-attestation properties for Keystore. The server can
  enforce and audit "worker present at T, biometric-class factor". Appropriate for re-auth without
  a PIN across shifts on a PERSONAL device, or for step-up on high-value actions (opening a gate,
  releasing a load).
- **Neither fits SHARED devices**: the device cannot say *which* enrolled person matched.

**Passkey vs app-held Keystore key** **[inferred]**: Credential Manager passkeys live in a credential
provider tied to the device's Google account and are synced. That suits a driver's own phone and is
wrong for a pool handheld. A Keystore key generated by our library is device-bound, works without a
Google account, and its attestation shows hardware backing. Recommendation: the Keystore unlock key
is the default; passkeys are an option for PERSONAL devices (phase P7).

## 3. Credential and token model

### 3.1 A new identity plane, not principals or portal identities

`iam_principals` is the wrong home: its email is globally unique (`idx_iam_principals_email`)
**[repo]**, email-domain routing drives login, and its tokens carry platform authority. The
profile-only gate exists precisely because authority leaked there **[repo]**. `portal_identities` is
also wrong: it is keyed on (client, email), has no refresh tokens by design, and works through
browser redirects **[repo]**.

Proposal: **`field_identities` (`fwu_`)**, one row per (client, worker_code). It reuses the same
machinery (hashing, policy, backoff, ratelimit, the JWT signer, audit, events) with separate storage
and separate `/field/*` entry points. This is the same choice the portal plan made, for the same
reason: "the failure mode of separation is credential duplication (benign)". Apps that serve field
workers are ordinary platform **Applications**; a grant (identity → application) gates sign-in, the
same as `portal_identity_apps`.

### 3.2 Enrolment: a device becomes a key

1. An admin or supervisor creates an enrolment code: `{clientId, mode, appCodes, assignedWorkerId?}`.
   It is single-use, valid for 10 min, and shown as a QR code.
2. The device library generates an EC P-256 key (Kotlin: Keystore, StrongBox first, falling back to
   the TEE on `StrongBoxUnavailableException` **[web]**; RN: the same, through a native module;
   web-only fallback: a non-extractable WebCrypto key, attestation `SOFTWARE`).
3. `POST /field/devices/enrol {code, publicKeyJwk, attestationChain?, platform, model}`. The platform
   stores the key thumbprint (`jkt`) and the attestation level (`STRONGBOX|TEE|SOFTWARE|NONE`). A
   client policy may require at least `TEE`.

### 3.3 Login → device-bound session

Every `/field/*` call after enrolment carries a `DPoP`-style proof: a JWS by the device key over
`{htm, htu, nonce, iat, ath?}` with a **server nonce**. Rugged devices have unreliable clocks, so
replay protection must not rest on `iat` alone. The format is modelled on RFC 9449 **[recalled]**.

`POST /field/auth/login {appCode, workerCode | badgeToken, pin}`. The platform checks, in order:
device ACTIVE → device budget → worker ACTIVE + grant + app active → per-identity backoff and lock →
PIN. As on the portal gate, grant failures are reported *after* the PIN check so a wrong guess
learns nothing. The response:

- **access token**: JWT, `typ: fc-field+jwt`, `token_use: "field"`, `sub: fwu_…`, `aud: <app code>`,
  `client_id`, `worker_code`, `device_id`, `device_mode`, `amr: ["pin"|"badge","pin"|"hwk"|"uv"]`,
  `auth_time`, and `cnf.jkt`. Lifetime **10 min** (`FC_FIELD_ACCESS_TOKEN_TTL_SECS`).
- **refresh token**: opaque, hashed at rest, rotated on every use, reuse kills the family. It is
  usable only with a proof from the same device key.
  - PERSONAL device: idle limit 2 h; absolute limit = shift cap (default 14 h, per-client policy).
  - SHARED device: idle limit 15 min; absolute limit 12 h; an explicit "end shift / hand over"
    action logs out.

**Middleware gotcha [repo]**: the platform API middleware rejects only `token_use == "identity"`
and treats an absent marker permissively (`shared/middleware/middleware.go:195`). A new `"field"`
value would be **accepted as an API bearer** unless that check becomes an allowlist
(`api` | absent). This change ships in P1 with a test. The SDK validators get the same allowlist,
and the field verifier accepts *only* `fc-field+jwt`.
*Open item:* the platform allowlist is done (F34, 2026-10-19); the `pkg/fcsdk/auth` validator allowlist and the `fc-field+jwt` typ check remain for P1.

### 3.4 Unlock on a PERSONAL device (no PIN)

- **Local unlock (P4, default)**: the refresh token is stored under a Keystore key with
  user-authentication; the biometric releases it, and the library refreshes. No server change.
- **Server-verified (P5)**: after a PIN login, the library creates a per-worker **unlock key**
  (auth-per-use, `BIOMETRIC_STRONG`, `setInvalidatedByBiometricEnrollment(true)` **[web]**) and
  registers it with `POST /field/auth/unlock-keys`. Later, `POST /field/auth/unlock` signs a nonce
  through `BiometricPrompt.CryptoObject(Signature)` **[web]** and receives a new session with
  `amr: ["hwk","uv"]`. A PIN is still forced every *N* days (policy, default 7) and after any
  invalidated key.
- Rejected outright when `device.mode = SHARED`.

### 3.5 Offline

- **Continuation (P1)**: the device keeps working on cached data. Expired access tokens matter only
  when the device is online again, at which point it refreshes. The refresh absolute deadline still
  holds. Work captured offline is queued in the app; the app's sync call is authorized by a fresh
  token.
- **Offline login (P6, opt-in per client)**: at each online login on a device, the platform returns
  an offline verifier, `HMAC(k_offline_device, pin)`. The device stores it under a Keystore key so it
  is useless off-device. The verifier is valid only for workers who signed in online on that device
  within 7 days, and offline logins are allowed for at most 12 h since the last sync. The library
  rate-limits locally (3 failures → 5 min). Each offline login produces a **device-signed receipt**
  `{workerId, appCode, at, counter}`. On reconnect, `POST /field/auth/offline-receipts`
  reconciles them: receipts for workers suspended *before* `at` are flagged to the app through an
  event, and the session continues only after a normal refresh. This is weaker than an online
  login, and the receipt's `amr: ["pin","offline"]` says so.

## 4. API surface and schema sketch

### 4.1 Device-facing routes (`/field/*`; device proof; outside the platform API middleware, like `/portal/*`)

| Method | Path | Body → Response |
|---|---|---|
| POST | `/field/devices/enrol` | `{code, publicKeyJwk, attestationChain?, platform, model, label?}` → `{deviceId, mode, clientId, appCodes[], nonce}` |
| POST | `/field/auth/nonce` | `{}` → `{nonce, expiresAt}` (also returned as a `DPoP-Nonce` header on every response) |
| POST | `/field/auth/login` | `{appCode, workerCode?, badgeToken?, pin}` → `FieldTokenResponse` · 401 `INVALID_CREDENTIALS` · 423 `WORKER_LOCKED` · 429 + `Retry-After` · 403 `NO_APP_ACCESS` / `DEVICE_REVOKED` |
| POST | `/field/auth/refresh` | `{refreshToken}` → `FieldTokenResponse` |
| POST | `/field/auth/pin` | `{currentPin, newPin}` (bearer) → 204 · 422 `PIN_POLICY` |
| POST | `/field/auth/unlock-keys` | `{publicKeyJwk, attestationChain?}` (bearer, PERSONAL only) → `{unlockKeyId}` |
| POST | `/field/auth/unlock` | `{workerId, appCode, signature}` → `FieldTokenResponse` |
| POST | `/field/auth/logout` | `{refreshToken, scope: "session" or "device-all"}` → 204 |
| POST | `/field/auth/offline-receipts` | `{receipts[]}` → `{accepted[], flagged[]}` (P6) |

`FieldTokenResponse = {accessToken, tokenType:"DPoP", expiresIn, refreshToken, refreshExpiresAt,
idleExpiresAt, worker:{id, workerCode, name}, pinChangeRequired, offlineVerifier?}`, in camelCase
per the wire contract. The new surface gets its own section in `docs/wire-contract.md`.

### 4.2 Admin API (`/api/*`, huma, permission-gated, client-boxed through `CanAccessClient`)

- `GET/POST /api/field-workers` (search `q`, `appCode`, `status`, paging as for portal users);
  `GET/PUT/DELETE /api/field-workers/{id}`; `POST /api/field-workers/{id}/pin-reset` → `{temporaryPin}`
  shown **once** (or `{pinChangeRequired:true}` for supervisor-entered PINs);
  `POST /{id}/unlock`, `/{id}/suspend`, `/{id}/activate`; `POST /{id}/apps`,
  `DELETE /{id}/apps/{appCode}`; `POST/DELETE /{id}/badges`; `GET /{id}/sign-ins`.
- `POST /api/field-devices/enrolment-codes` → `{code, qrPayload, expiresAt}`; `GET /api/field-devices`;
  `GET /api/field-devices/{id}` (introspection: mode, attestation, last seen, active sessions);
  `POST /{id}/suspend`, `/{id}/revoke`.
- `POST /api/field-tokens/introspect {token}` → `{active, sub, deviceId, appCode, amr, exp}`. Its
  purpose is an online revocation check before high-value actions.
- `GET/PUT /api/field-policies/{clientId}`.
- Permissions (four-segment, client-delegable, seeded in `internal/platform/seed/permissions.go`):
  `platform:iam:field-worker:view`, `platform:iam:field-worker:manage`,
  `platform:iam:field-device:view`, `platform:iam:field-device:manage`,
  `platform:iam:field-token:introspect`. Seeded role `platform:field-administrator` (the first four);
  the app's confined service account gets `field-worker:manage` + `field-token:introspect` so a
  supervisor screen *in the app* can reset PINs through the SDK.

### 4.3 Console (Vue, `frontend/src/pages/field/`)

Field Workers (list, search, app chips, lock state, PIN reset with a one-time display, badges),
Devices (enrol → QR, mode, attestation badge, revoke), Policy (per client), and sign-in history.
`canAccessPath` mappings follow `portal-apps-and-access-gate.md` §3.

### 4.4 Schema (one goose migration; take the **next free number at merge time**, which is 083 today
**[repo]**; other agents are adding migrations; follow the goose seeding rules in
`docs/migration-bootstrap.md`)

```
field_identities      id fwu_ PK, client_id, worker_code (normalised upper), name, email NULL,
                      status ACTIVE|DISABLED, pin_hash NULL, pin_scheme, pepper_version,
                      pin_set_at, pin_change_required, failed_since_success INT, locked_until,
                      hard_locked BOOL, last_login_at, created_at, updated_at;
                      UNIQUE(client_id, worker_code); text_pattern_ops on (client_id, lower(name))
field_identity_apps   (identity_id, application_id) PK, source, granted_at
field_badges          id fwb_, identity_id, client_id, kind NFC|QR, token_hash, status;
                      UNIQUE(client_id, token_hash)
field_devices         id fwd_, client_id, mode PERSONAL|SHARED, label, platform, model, jkt UNIQUE,
                      public_key_jwk, attestation_level, assigned_identity_id NULL,
                      status ACTIVE|SUSPENDED|REVOKED, enrolled_by, enrolled_at, last_seen_at
field_device_apps     (device_id, application_id) PK
field_enrolment_codes id fwe_, client_id, code_hash, mode, app_ids[], assigned_identity_id,
                      expires_at, used_at, created_by
field_unlock_keys     id fwk_, device_id, identity_id, jkt, public_key_jwk, attestation_level,
                      created_at, last_used_at, revoked_at; UNIQUE(device_id, identity_id)
field_sessions        id fws_, family_id, identity_id, device_id, application_id,
                      refresh_hash UNIQUE, amr[], issued_at, idle_expires_at,
                      absolute_expires_at, revoked_at, revoked_reason
field_policies        client_id PK, pin_min_length, soft_lock_threshold, soft_lock_secs,
                      hard_lock_threshold, shared_idle_secs, personal_idle_secs, shift_max_secs,
                      require_attestation, offline_login_enabled, offline_max_secs, pin_reauth_days
```

The new TSID prefixes `fwu fwd fwb fwe fwk fws` do not collide with the existing `fn*`/`fd*`/`fr*`
prefixes **[repo]** (`internal/tsid/tsid.go`). Code goes in `internal/platform/fieldidentity`
(entity, repo, operations on the usecase envelope), `internal/platform/fieldauth` (the `/field/*`
endpoints, like `portalauth`), and token minting in `auth/authservice`
(`GenerateFieldAccessToken`).

## 5. Client matrix

| Capability | RN + WebView (today) | Kotlin native (future) | iOS |
|---|---|---|---|
| PIN pad | Web component in `@flowcatalyst/field-js`, posting straight to `/field/*` | Compose component in the library | Same as RN |
| Device key | RN native module (Keystore EC, StrongBox first); the WebView reaches it through a `postMessage` bridge. Web-only fallback: non-extractable WebCrypto (`SOFTWARE`) | Keystore + StrongBox + key attestation | Secure Enclave key **[recalled]** |
| Local biometric unlock | Native module + BiometricPrompt | BiometricPrompt | LocalAuthentication **[recalled]** |
| Server-verified unlock key | Native module: `CryptoObject(Signature)`, per-use **[web]** | Same | SE key with `biometryCurrentSet` **[recalled]** |
| Passkeys | **Not out of the box.** react-native-webview v16.0.0 (2026-07-11) ships androidx.webkit 1.14.0 but never calls `setWebAuthenticationSupport` (`WEB_AUTHENTICATION_SUPPORT_FOR_APP`, webkit ≥ 1.12.0) **[web]**; issue #3807 is unresolved, with patch workarounds **[web]**. Options: patch the WebView, or call `react-native-passkeys` (an Expo module on Credential Manager **[web]**) through the bridge | Credential Manager `androidx.credentials` **[web]** | WKWebView passkeys need Associated Domains (webcredentials) **[web, forum]** |
| Badge | Built-in rugged scanners (keyboard wedge) or the RN NFC module → `badgeToken` | NFC / scanner intents | QR via camera |
| PIN-only fallback | Always available | Always available | Always available |

Passkey prerequisites for any Android route: Digital Asset Links with the `get_login_creds`
relation **[web]**, and the platform's WebAuthn `RPOrigins` (`internal/platform/webauthn/service.go`
**[repo]**) must include the app origin `android:apk-key-hash:<b64 sha256>` **[web]**. The field
plane needs its own RP config because its users are not `iam_principals`. Keystore caveats: keys
need a secure lock screen; some OEMs invalidate keys after security patches
(`UnrecoverableKeyException` reports on Samsung **[web]**). The library must treat a lost key as
"re-enrol the unlock key, PIN required", never as a crash.

## 6. SDK surface (keep it small)

**Server SDKs** (Laravel `clients/laravel-sdk`, TS `clients/typescript-sdk`, Go `pkg/fcsdk/auth`,
Java `clients/java-sdk`, Rust `clients/rust-sdk`). Each SDK provides:

1. `verifyFieldToken(token, {dpopProof?, method, url}) → FieldPrincipal {workerId, clientId,
   workerCode, name, appCode, deviceId, deviceMode, amr[], authTime}`. Local JWKS, reusing the
   existing validators (Go: `pkg/fcsdk/auth/jwks.go` `TokenValidator`). It enforces
   `typ=fc-field+jwt`, `aud` = the configured app code, and `cnf.jkt` when a proof is supplied.
2. Framework glue: a Laravel `field` guard, a Fastify plugin/preHandler, Go `net/http` middleware, a
   Javalin before-handler, and an axum extractor.
3. `introspectFieldToken(token)`, the online check.
4. Thin admin wrappers over the generated clients: `fieldWorkers.ensure|resetPin|setStatus|grantApp`,
   `fieldDevices.get|revoke`.

There is no enrolment, login or PIN handling in server SDKs, because PINs never reach app backends.
A conformance kit of signed JWT + proof fixtures, generated by Go into `api/field-fixtures/`, is
shared by all five SDKs.

**Mobile libraries** (the only code that touches PINs or keys):
- `clients/field-js`: headless `FieldAuth` with `enrol(code)`, `login({appCode, workerCode|badge,
  pin})`, `unlock()`, `refresh()`, `logout()`, `fetch()` (adds the bearer + proof), an optional
  `<fc-pin-pad>`, and a `KeyProvider` interface (native bridge | WebCrypto).
- `clients/field-react-native`: the native module (generate/sign device key, unlock key with
  biometric, secure store) and the WebView bridge host (one line to install on the `WebView`).
- `clients/field-android` (Kotlin, AAR, later): the same protocol with Credential Manager passkeys
  as an option.

## 7. Phased plan

| Phase | Scope | Size | Gate | Parallel? |
|---|---|---|---|---|
| P0 | Owner decisions (§9); wire-contract section; fixture generator | S | Owner sign-off | — |
| P1 | Migration, `fieldidentity` + `fieldauth`, pepper + Argon2id, policy, three-bucket lockout, enrolment, login/refresh/logout + proof + nonce, field token mint, **middleware allowlist fix**, admin API, events, audit | L | `go test -tags=integration` for brute-force, spray, refresh reuse, revoked device, proof replay, and field token rejected as API bearer; `make api-diff`; `/verify` | Single agent (the shared seams) |
| P2 | Console pages (§4.3) | M | `vue-tsc -b`, `npm run build`, `/verify` | After the P1 API freezes; ∥ P3/P4 |
| P3 | 5 server SDKs: verify + glue + admin wrappers | M (5×S) | Each SDK passes the shared fixtures | **One Sonnet agent per SDK**, spec = §6 + fixtures |
| P4 | `field-js` + RN native module: PIN, device key, local biometric unlock | M | Example RN app on a real rugged device: enrol, login, background, unlock, revoke from the console | ∥ P2/P3 |
| P5 | Server-verified unlock keys + attestation parsing | M | Attestation fixtures; SHARED device refused | After P4 |
| P6 | Offline login receipts (only if D4 = yes) | M–L | Airplane-mode shift-change script; suspended-worker receipt flagged | Later |
| P7 | Passkeys (PERSONAL), Kotlin library | M + M | Kotlin library passes the P4 scripts | Later |
| P8 | Legacy PIN import per app (`POST /api/field-workers/import`) | S per app | Old PIN works once, then rehashed with the pepper | Per app |

The Java platform port is still mirrored (memory), so P1 also produces a
`docs/java-handoff-<date>-field-identity.md`.

## 8. Minimum viable first slice

P1 (without badges and policy UI) + `field-js` + RN module with **local** biometric unlock + the Go
and Laravel verifiers + one app migrated. Value delivered:

- PINs leave the apps.
- One proper lockout (with a hard lock and spray defence) instead of N ad-hoc ones.
- Suspending a leaver kills their sessions on every device and every app at once.
- A stolen device's tokens are useless without its key.
- An audit trail of who signed in on which device.
- Biometric convenience on personal phones at zero server cost.

Deliberately left out of the slice: offline login, passkeys, attestation enforcement, Kotlin,
badges.

## 9. Owner decisions (with recommendations)

1. **D1 Plane.** A new `field_identities` plane, or field workers as `iam_principals` /
   `portal_identities`? **Recommend a new plane** (§3.1): no email, no platform authority, refresh
   tokens needed. Promoting a field worker into a principal later remains possible; untangling
   leaked authority does not.
2. **D2 PIN intake.** A platform-owned client library calls `/field/*` directly, or a
   platform-hosted PIN page in the WebView / a Custom Tab (the portal rule "credentials only on
   platform pages")? **Recommend the library**: the app *backend* still never sees a PIN, Custom
   Tabs are hostile to gloves and kiosks, and the library is platform code. Record this as a
   narrow, deliberate exception to the portal rule.
3. **D3 Token delivery.** Platform-issued device-bound field tokens that app backends verify, or
   the portal pattern (id_token → app session)? **Recommend field tokens**: revocation, device
   binding and shift caps stay central; the app keeps zero session code.
4. **D4 Offline login.** **Recommend no in v1** (continuation only). Build P6 only after a measured
   need at a named site, and enable it per client.
5. **D5 Shared-device policy.** **Recommend**: biometrics and unlock keys are refused on SHARED
   devices; SHARED uses worker code + PIN, or badge + PIN where badges exist; the 15-minute idle
   limit and the hand-over action are mandatory.
6. **D6 PIN policy + migration.** **Recommend** a 6-digit default (4 only per client opt-in, with a
   hard lock at 5); hard lock at 10 cumulative failures; supervisor unlock through the app's service
   account. Import legacy hashes where `passwordhash` can verify them (bcrypt/argon2i) and rehash
   with the pepper on first success; otherwise issue temporary PINs at cut-over.

## 10. Risks, and the honest comparison with leaving PINs in the apps

Risks:
- **Platform availability becomes shift-start critical**: if the platform is down at 06:00, nobody
  starts. Mitigation: continuation holds existing sessions; P6 offline login is the real hedge.
- **WebView passkey support depends on patches.** Mitigation: the default design needs no
  WebAuthn.
- **OEM Keystore churn.** Mitigation: library recovery path.
- **The middleware allowlist must ship with P1** (§3.3).
- **Five SDKs + Java mirror cost.** Mitigation: the SDK surface is two functions plus wrappers.
- **Device clock skew breaks proofs.** Mitigation: server nonces.
- **Old Android rugged devices lack attestation/StrongBox.** Mitigation: attestation is recorded,
  enforced only by policy.

Leaving PINs in each app costs nothing to build, keeps each app's offline login, and spares the
platform an availability dependency. But every app re-implements hashing, lockout, reset and device
logic, and with 4-digit PINs that almost always means no hard lock and no spray defence
**[inferred]**. A leaver must be disabled in three apps. There is no cross-app identity for a worker
who both picks and drives, no device binding, and no audit. Moving PINs into the platform costs an L
phase plus five small SDK tasks and one client library, and buys one hardened implementation. It is
worth it once a second app adopts the plane; for a single app it would be marginal.

## 11. Sources checked 2026-10-19

- Android, "Authenticate users with WebView": developer.android.com/identity/sign-in/credential-manager-webview
  (webkit ≥ 1.12.0, `setWebAuthenticationSupport(…, WEB_AUTHENTICATION_SUPPORT_FOR_APP)`,
  Digital Asset Links required, no conditional mediation).
- react-native-webview: latest release v16.0.0, published 2026-07-11; `android/gradle.properties`
  pins webkit 1.14.0; a repo code search found no `setWebAuthenticationSupport`; issue #3807.
- react-native-passkeys README (Credential Manager, `get_login_creds` relation).
- passkeys.dev Android reference (embedded WebViews need extra work); OWASP MASTG MASVS-AUTH and the
  AndroidX `BiometricPrompt.CryptoObject` / `KeyGenParameterSpec.Builder` references (auth-per-use,
  StrongBox fallback, invalidation on enrolment change); Samsung developer forum
  (`UnrecoverableKeyException` after updates).
- Apple developer forums thread 723273 (WKWebView passkeys need Associated Domains).
- Repo: `auth/passwordhash`, `auth/loginbackoff`, `shared/ratelimit`, `auth/authservice`
  (`token_use`), `shared/middleware/middleware.go:195`, `webauthn/service.go`, `tsid/tsid.go`,
  `migrate/sql/082_*` (latest), `docs/portal-*.md`.
