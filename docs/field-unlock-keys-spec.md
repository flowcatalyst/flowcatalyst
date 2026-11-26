# Field-worker unlock keys: sign in again with a biometric, verified by the platform

Status: SPEC, 2026-11-26; server and console built on the branch (section 15). Branch `feat/field-unlock-keys`. This is phase P5 of
`docs/field-worker-identity-design.md` ("Server-verified unlock keys + attestation parsing", section 3.4,
decision D5). It builds on `docs/field-worker-identity-p1-spec.md`, `docs/field-worker-sign-in-options-spec.md`,
`docs/field-native-session-spec.md` and `docs/field-mobile-libraries-spec.md`; where this spec and those
disagree, this spec wins. Decisions marked **(D)** were made by the implementing session and can be
overridden by the owner.

The problem: a worker whose session has ended (the shift limit, the idle limit, a sign-out) signs in
again on their own phone with the PIN. A fingerprint or a face would do, but the usual shortcut (keep
the PIN on the phone behind a biometric and replay it) puts the PIN on the phone, hides from the
platform that a biometric was used, and cannot be limited or revoked. An **unlock key** replaces it:
a key pair made in the phone's secure hardware that can sign only after a strong biometric match.
The platform holds the public half, sends a fresh challenge, verifies the signature and issues a new
session whose `amr` says "hardware key plus user verification". Neither the platform nor the phone
ever stores the PIN, and the platform can see, limit and revoke every key.

## 0. Scope

This piece: the specification, the server (schema, domain, `/field/*` routes, admin API, policy,
revocation, lockout, audit, events, token claims, fixtures, wire contract) and the console.

Later pieces, built from this document alone (section 12): the device libraries
(`clients/field-js`, `field-android`, `field-ios`, `field-react-native`), the server SDKs' admin
wrappers, the framework.

Not in scope: passkeys (P7), offline login (P6), enforcement of anything the design leaves to later
(an attestation revocation list, Apple App Attest), a Java or Rust mirror (the Java port of the platform
is paused, so no hand-off note is written; section 13).

## 1. What exists today (the code is the fact)

| Design says | Code says | This spec |
|---|---|---|
| `field_unlock_keys ... UNIQUE(device_id, identity_id)` (design 4.4) | no table; P1 spec deferred it | a table of its own (section 5), one LIVE key per device (stricter, D2) |
| `pin_reauth_days` forces a PIN every N days (default 7) | the column, the API field and the console field exist (1..90, default 7) and nothing reads them | unlock keys read it; **the default becomes 0, never**, and 0..90 are valid (section 4.4, D3: a departure from the design the owner raised) |
| `require_attestation` | refuses every enrolment (`422 ATTESTATION_REQUIRED`), since P1 verifies nothing | unchanged; unlock keys have their own setting (section 7) |
| `POST /field/auth/unlock {workerId, appCode, signature}` signs "a nonce" | `/field/*` nonces are single use, five minutes, not bound to anything | a separate unlock challenge, bound to the device and the key (section 4.3) |
| `amr: ["hwk","uv"]` | tokens already carry any `amr` array | unchanged claim set; a new value |
| enrolment records `attestationLevel` | recorded as `NONE` or `SOFTWARE` (reported, unverified) | unchanged for devices; unlock keys parse the chain (section 6) |
| "biometric unlock" in `field-js` (P4) | local unlock: the refresh token behind a biometric, no server change | kept; an unlock key is the new, server-verified path |

## 2. Rules

### 2.1 Who may register an unlock key

`POST /field/auth/unlock-keys` is refused unless every row holds, checked in this order:

| # | Rule | Refusal | Why |
|---|---|---|---|
| 1 | The proof key is an enrolled, ACTIVE device | `401 UNKNOWN_DEVICE` (or the prelude's `403 DEVICE_SUSPENDED` / `DEVICE_REVOKED`) | an unlock key is tied to a device row that can be listed and revoked |
| 2 | The device is PERSONAL | `403 SHARED_DEVICE` | D5: a shared device's biometrics cannot say whose finger it was |
| 3 | The access token is not PIN-change-only (`pcr`) | `401 invalid_token` | as act-as and photos: a `pcr` token works only on `/field/auth/pin` |
| 4 | The access token's session is this device's (its `device_id` is this device) | `403 PIN_SIGN_IN_REQUIRED` | a session that began on the key before it was enrolled keeps its unenrolled limits (sign-in options 1.3); the worker signs in again on the device |
| 5 | The session is not impersonated (no `act`) | `403 IMPERSONATING` | the key would authenticate the worker; only the worker may make it |
| 6 | The client's policy has `unlockKeysEnabled` | `403 UNLOCK_KEYS_DISABLED` | default off: the client decides |
| 7 | The body is well formed: `publicKeyJwk` and `keyProof` present, a P-256 key that is not the device key, at most 10 certificates | `400 INVALID_REQUEST`, `400 INVALID_PUBLIC_KEY` | two keys with two jobs: the device key signs without a biometric |
| 8 | `keyProof` verifies (section 4.2) | `401 INVALID_UNLOCK_PROOF` | the caller holds the key and a biometric just unlocked it |
| 9 | The worker is ACTIVE, not hard or soft locked, and has no pending PIN change | `403 WORKER_UNAVAILABLE`, `423 WORKER_LOCKED`, `429 WORKER_LOCKED_TEMPORARILY`, `403 PIN_SIGN_IN_REQUIRED` | a temporary PIN must be replaced before anything rests on it |
| 10 | The session's login family was signed in with the worker's PIN: `amr` is `["pin"]` or `["badge","pin"]`, and its `auth_time` is at most **10 minutes** ago | `403 PIN_SIGN_IN_REQUIRED` | D1 |
| 11 | No other worker holds a live key on the device | `409 UNLOCK_KEY_OTHER_WORKER` | D2 |
| 12 | The key was never registered before (any status) | `409 UNLOCK_KEY_EXISTS` | a key is registered once; a new registration makes a new key. The purger removes a key 30 days after its revocation, which frees its thumbprint: "once" holds for those 30 days |
| 13 | When the policy has `unlockKeyAttestation: REQUIRED`, the attestation meets it (section 6.3) | `422 ATTESTATION_REQUIRED` | the policy's call; default not enforced |

Rules 1 to 8 are the protocol layer's (`internal/platform/fieldauth/unlock.go`), 9 to 13 the use case's
(`RegisterUnlockKey`, which checks the device, the mode and the policy again, holds the worker's row and runs in
one transaction). Every refusal is recorded as an attempt (section 9) with its reason.

**(D1) Freshness of the PIN sign-in: 10 minutes, no step-up PIN.** The library registers a key
right after the worker signs in with the PIN, so the window costs nothing in normal use. It stops
someone who picks up a signed-in phone hours into a shift from registering a key that outlives the
session. 10 minutes is the default access-token lifetime: the first token of the sign-in is enough.
A step-up PIN in the body (as act-as does) was considered and rejected: the library would have to
hold the PIN after the sign-in, which `field-mobile-libraries-spec.md` 1.1 forbids. Excluded by the
`amr` rule: a session from an unlock (`hwk`), a badge alone (`badge`), a supervisor's act-as or a
console sign-in code (`imp`), so an unlock can never mint the next unlock key.

**(D2) One live key per device; a second worker is refused** (a deliberate hand-over goes through the
device's own revoke, section 4.7). The design treats a PERSONAL device as
one worker's phone. Android's biometrics belong to the phone, not to a person: any finger enrolled on
it unlocks any key on it. If two workers held keys on one phone, worker A's finger would sign in as
worker B, which is the shared-device threat D5 rules out. So a device holds at most one live key
(a partial unique index), the same worker registering again replaces their own key in the same
transaction (reason `REPLACED`), and another worker is refused with `409 UNLOCK_KEY_OTHER_WORKER`
rather than silently ending the first worker's key: a phone changing hands is an administrator's
decision (revoke in the console) or the first worker's (revoke from the device), and the refusal is
recorded. A PERSONAL device assigned to a worker (`assignedWorkerId`) already admits only that worker.

### 2.2 What an unlock gives

| Item | Value | Why |
|---|---|---|
| `amr` | `["hwk","uv"]` | the design's values: a hardware-held key, user verification |
| `auth_time` | the unlock | the session is a new authentication |
| idle and absolute limits | the PERSONAL ones (`personalIdleSecs`, `shiftMaxSecs`); when the policy sets a periodic PIN, the absolute one capped at the key's PIN deadline (section 4.4) | an unlock never carries a session past a forced PIN |
| `pcr` | never set (a worker with a pending PIN change is refused) | |
| `act` | never set | an impersonated session cannot register a key, so a key is the worker's own |
| what it may not do | register an unlock key (rule 7); reset the PIN deadline (only a PIN does) | |
| what it may do | everything a PIN session may: refresh, change the PIN (the current PIN is checked), photos, act-as (the supervisor's step-up PIN is still asked) | |

### 2.3 What a session's tokens say about unlock keys

Every answer that carries a session (`FieldTokenResponse`: the PIN or badge login, every refresh, the unlock,
an act-as, a console sign-in code) carries `unlockKeys`, computed for that worker on that device when the
tokens are issued:

```
unlockKeys = {allowed: boolean, attestationRequired: boolean, registerExpiresIn?: integer}
```

| Field | Value |
|---|---|
| `allowed` | `true` only when the client's policy has `unlockKeysEnabled`, the session is on an ACTIVE PERSONAL enrolled device, and the session is the worker's own (not an act-as or console sign-in code session). `false` on a SHARED device, an unenrolled key, an impersonated session or with the policy off, without saying which |
| `attestationRequired` | `true` when `allowed` and the policy's `unlockKeyAttestation` is `REQUIRED` (a registration then needs a trusted Android attestation, section 6.3); `false` whenever `allowed` is |
| `registerExpiresIn` | whole seconds this session may still register a key: present only when `allowed`, the session began with the worker's PIN sign-in on this device (rule 7), no PIN change is pending, and the 10 minutes of D1 have not passed. Every token of the session counts down to the same moment (a refresh keeps the sign-in's time); absent for a session an unlock or a badge alone began |

It is **advisory**: a registration checks every rule of 2.1 again, and a policy changed after the answer is
seen at the session's next refresh (within an access token's lifetime), which is how a library learns of a
change without asking. It says nothing an unauthenticated caller can read (only answers to a valid sign-in or
refresh carry it) and nothing about another client: it is the policy of the session's own client. It does not
say whether a key already exists (the phone knows which keys it holds) or whether another worker holds the
device's key (`409 UNLOCK_KEY_OTHER_WORKER` at registration).

A registration whose body is empty or malformed is answered by the rule it reaches first (`403
UNLOCK_KEYS_DISABLED` with the policy off, recorded as an `UNLOCK_KEY_REGISTER` attempt; `400 INVALID_REQUEST`
with it on, recorded as nothing) and never counts towards the worker's lock or the sign-in failure buckets: no
registration refusal does.

## 3. Keys and signatures

- **The unlock key** is ECDSA P-256 (the device key's curve: one signer path in the libraries). The
  public half travels as a JWK `{"kty":"EC","crv":"P-256","x","y"}`, `x` and `y` canonical base64url
  of 32 bytes; private members are refused. The platform stores the canonical JWK and its RFC 7638
  thumbprint (`jkt`), exactly as for a device key (`fieldauth.ParseECJWK`).
- **Signatures** are compact JWS, ES256 only, the signature the raw 64-byte `r || s`, every segment
  canonical base64url (`docs/wire-contract.md`, "Token spelling"). The libraries already build ES256
  JWS for the DPoP proof; the unlock key is a second key behind the same signer.
- **Domain separation.** Three JWS kinds can be signed on a phone; each names itself in `typ` and in
  `aud`, and every verifier refuses the others: the DPoP proof `typ: dpop+jwt` (device key); the key
  proof `typ: fc-unlock-key+jwt`, `aud` = `<platform base URL>/field/auth/unlock-keys` (unlock key); the
  unlock assertion `typ: fc-unlock+jwt`, `aud` = `<platform base URL>/field/auth/unlock` (unlock key). The
  base URL is the one the platform compares `htu` with (`FC_JWT_ISSUER`), so an assertion for one
  platform is refused by another. `iat` is required and accepted within 24 hours, as for the DPoP
  proof: freshness comes from the nonce or the challenge, never from a phone's clock.

**Key proof** (registration). Header `{"typ":"fc-unlock-key+jwt","alg":"ES256"}` (no `jwk`, no `kid`:
the key is `publicKeyJwk`). Claims:

| Claim | Value |
|---|---|
| `aud` | `<base URL>/field/auth/unlock-keys` |
| `nonce` | the `nonce` of this request's DPoP proof (the single-use platform nonce) |
| `sub` | the worker id (`fwu_...`, the access token's `sub`) |
| `device_id` | the device id (the access token's `device_id`) |
| `iat` | seconds since the epoch |

The device key (DPoP proof) and the unlock key (key proof) sign the same single-use nonce in one
request, so neither can be replayed into another registration.

**Unlock assertion.** Header `{"typ":"fc-unlock+jwt","alg":"ES256","kid":"<unlockKeyId>"}`. Claims:

| Claim | Value |
|---|---|
| `aud` | `<base URL>/field/auth/unlock` |
| `nonce` | the unlock challenge (section 4.3) |
| `sub` | the worker id |
| `device_id` | the device id |
| `app` | the application code the session is for (the body's `appCode`) |
| `iat` | seconds since the epoch |

**Attestation challenge** (Android key attestation only, section 6). The library sets
`setAttestationChallenge` to the 32 bytes
`SHA-256("fc-unlock-key-attestation:v1\n" + sid + "\n" + workerId + "\n" + deviceId)` (UTF-8; `sid`,
`sub` and `device_id` are the access token's claims, which the library reads from its own token as it
reads `client_id` today). The key is made after the PIN sign-in and before the registration, so the
nonce of the registration request is not yet known; the session id binds the attestation to this
sign-in, whose freshness rule 7 already bounds.

## 4. Protocol

All routes are `POST` under `/field`, behind the prelude every `/field/*` route has (DPoP proof,
single-use nonce, the per-IP governor, a fresh `DPoP-Nonce` on every answer).

| Route | Prelude | Request | Success |
|---|---|---|---|
| `/field/auth/unlock-keys` | proof, nonce, bearer | `{publicKeyJwk, keyProof, attestationChain?}` | `201 FieldUnlockKeyRegistered` |
| `/field/auth/unlock/challenge` | proof, nonce, enrolled device | `{unlockKeyId}` | `200 {challenge, expiresAt, pinRequiredBy}` |
| `/field/auth/unlock` | proof, nonce, enrolled device | `{unlockKeyId, appCode, assertion}` | `200 FieldTokenResponse` |
| `/field/auth/unlock-keys/revoke` | proof, nonce, enrolled device | `{unlockKeyId}` | `204` |

`attestationChain` is an array of base64 (standard alphabet, padded) DER certificates, leaf first:
the `chain` of `DeviceKey.attestation` in `field-mobile-libraries-spec.md` section 2. At most 10
certificates of at most 16 KiB each. `FieldUnlockKeyRegistered = {unlockKeyId, deviceId, workerId,
createdAt, pinRequiredBy, replacedUnlockKeyId?, attestation: FieldUnlockKeyAttestation}` (section 8.1).

### 4.1 Register

The rules of section 2.1, in order. On success, in one transaction: the worker's own live key on the
device (if any) is revoked `REPLACED`, the new key is stored with `pin_verified_at` = the session's
`auth_time`, and `platform:iam:field-unlock-key:registered` is emitted. The answer's `pinRequiredBy` is
`pin_verified_at + pinReauthDays` days, absent when the policy forces no periodic PIN. The attempt is recorded (method `UNLOCK_KEY_REGISTER`, success or
the refusal's reason).

### 4.2 Key proof and assertion verification

Both are verified with the unlock key, never with the device key: the JWS splits into three canonical
segments; the header is exactly the `typ` and `alg` above (and `kid` = the body's `unlockKeyId` for an
assertion); the signature verifies; every claim of the table is present and equal; `iat` is within 24
hours. The header holds exactly those members: any other, `crit`, `jwk` and `x5c` included, is refused. Any
failure is `401 INVALID_UNLOCK_PROOF` (the reason is in the attempt row only).

### 4.3 Challenge

The library asks for a challenge before it shows the biometric prompt, so the platform can say
"use your PIN" before the worker is asked for a finger. In this order:

1. The device is enrolled and PERSONAL (`401 UNKNOWN_DEVICE`, `403 SHARED_DEVICE`).
2. The device's challenge budget: `FC_FIELD_UNLOCK_CHALLENGES_PER_HOUR` (default 30) per device; and
   the device's and client's sign-in failure budgets (section 4.6). Full: `429 TOO_MANY_ATTEMPTS` +
   `Retry-After`.
3. The key exists, is live and is this device's (`401 UNLOCK_KEY_INVALID`).
4. The policy has `unlockKeysEnabled` (`403 UNLOCK_KEYS_DISABLED`).
5. The worker is ACTIVE (else `401 UNLOCK_KEY_INVALID`), not hard locked (`423 WORKER_LOCKED`), not soft
   locked (`429 WORKER_LOCKED_TEMPORARILY` + `Retry-After`).
6. The worker's PIN can be checked: not set under a retired app key, and set at all (else `403
   PIN_RESET_REQUIRED`, as the PIN sign-in answers: the unlock path is never less strict than the PIN login).
7. Under `REQUIRED`, the key's attestation was verified and no usable status list names its chain (6.3; else
   `401 UNLOCK_KEY_INVALID`, a listed one revoked `ATTESTATION_REVOKED`). With the list on and no usable
   copy, `503 ATTESTATION_STATUS_UNAVAILABLE` + `Retry-After` and the key stays registered (6.4).
8. No PIN change is pending and, when the policy sets a periodic PIN, its deadline has not passed (`401 PIN_REAUTH_REQUIRED`).

Then 32 random bytes, base64url, are stored as their SHA-256 with the key id, the device id and an
expiry of **5 minutes** (as the nonces: a biometric prompt can take a while), and answered with the
deadline (`pinRequiredBy`, absent when there is none). A challenge is single use and good only for that key on that device.

### 4.4 The forced PIN

**(D3) No periodic PIN by default.** The design says "a PIN is still forced every N days (policy, default
7)". The owner questioned it on 2026-11-26: the person has already passed the phone's own lock (its device
credential) to use it, so why ask for the worker's PIN again on a timer? This spec departs from the design's
default: `pinReauthDays` is **0 by default, meaning never**; a client may set 1 to 90 days. Reasoning: an
unlock key that needs a strong biometric for every use, held in the phone's secure hardware on an enrolled
device, is already two factors (the device's key, and the biometric that activates it), and Android itself
falls back to the phone's own PIN, pattern or password at least every 72 hours for a strong (Class 3)
biometric ("Up to 72 hours before fallback to primary authentication",
https://source.android.com/docs/security/features/biometric/measure) and keeps the app's credential-encrypted
storage locked after a restart until the device credential is entered ("only available after the user has
unlocked the device", https://source.android.com/docs/security/features/encryption/file-based). A biometric
enrolled after the key was made invalidates the key (the library sets `setInvalidatedByBiometricEnrollment`).

What a periodic worker PIN does and does not protect against, so a client can choose:

- *It does*: keep the worker practised at their PIN, so they still know it when they need it (a new phone, an
  invalidated key, a shared device, a PIN change); and bound how long a biometric that was **already enrolled on
  the phone before the key was made** (a family member's finger on a personal phone) can sign in as the worker
  without the worker's PIN.
- *It does not*: stop someone who holds the phone and an enrolled biometric between two PIN checks; add a factor
  beyond the two the key already needs; replace the revocations (a key revoked, unknown or invalidated always
  needs the PIN, as do a PIN change and a reset, whatever this setting says).

The PIN is still demanded, independently of `pinReauthDays`: whenever the key is invalidated, revoked or
unknown (`UNLOCK_KEY_INVALID`, or the library's own invalidation), after a PIN change on another device or a
reset, on every other revocation trigger of section 7, and while a PIN change is pending.

Each key carries `pin_verified_at`: the time the worker last proved their PIN on this device. It is set
at registration (the session's `auth_time`) and moved to now by every successful PIN sign-in of the worker
on this device (`login` with `amr` `pin` or `badge,pin`) and by the worker's own PIN change on this
device. When `pinReauthDays` is 1 to 90 the **PIN deadline** is `pin_verified_at + pinReauthDays` days, read
from the policy at the time of the check, so a change of `pinReauthDays` applies to every key at once; at 0
there is none (`pinRequiredBy` is absent on the wire). At or past the deadline, a challenge or an
unlock is `401 PIN_REAUTH_REQUIRED` and the key is kept: after the next PIN sign-in on the device it works
again, without a new key or a new biometric enrolment. "After any invalidated key" (design 3.4) is the
library's: a key the phone invalidated (a biometric enrolled, an OEM key loss) cannot sign, so the
library revokes it (`/field/auth/unlock-keys/revoke`), deletes it and asks for the PIN.

### 4.5 Unlock

In this order (the attempt is recorded with method `UNLOCK_KEY` for every outcome below):

1. The device is enrolled and PERSONAL (`401 UNKNOWN_DEVICE`, `403 SHARED_DEVICE`).
2. The device's and client's sign-in failure budgets (`429 TOO_MANY_ATTEMPTS` + `Retry-After`).
3. `unlockKeyId`, `appCode` and `assertion` are present (`400 INVALID_REQUEST`).
4. The key exists, is live and is this device's; else `401 UNLOCK_KEY_INVALID`, counted against the
   device and client budgets.
5. The policy has `unlockKeysEnabled` (`403 UNLOCK_KEYS_DISABLED`).
6. In one transaction that holds the worker's row and the key's row: the worker is ACTIVE (else
   `401 UNLOCK_KEY_INVALID`); the key is still live (a revocation that won the race: `401
   UNLOCK_KEY_INVALID`); not hard locked (`423 WORKER_LOCKED`) or soft locked (`429
   WORKER_LOCKED_TEMPORARILY` + `Retry-After`).
7. The PIN-reset and attestation rules of 4.3 (steps 6 and 7) (`403 PIN_RESET_REQUIRED`, `401 UNLOCK_KEY_INVALID`), checked before
   the signature and not counted.
8. The assertion verifies (section 4.2). A failure is one failure of the worker (section 4.6) and
   `401 INVALID_UNLOCK_PROOF`.
9. The challenge in the assertion's `nonce` is consumed. Unknown or already used (a replay), or issued
   for another key or device: one failure of the worker and `401 INVALID_UNLOCK_PROOF`. Expired:
   `401 CHALLENGE_EXPIRED`, not counted (a slow prompt is not an attack; the library asks for a new
   challenge and prompts again).
10. A pending PIN change or, when the policy sets a periodic PIN, a passed PIN deadline: `401 PIN_REAUTH_REQUIRED` (the challenge stays
   consumed).
11. As a login after the PIN: the application is active, the worker holds its grant, the device is
    enrolled for it, a device assigned to a worker is this worker's, the client is ACTIVE; any failure
    is `403 NO_APP_ACCESS` (the reason only in the attempt row).
12. Success: the worker's failure counter and soft lock are cleared and `lastLoginAt` stamped (as a
    login), a new login family is stored (section 2.2) with the key's id (`unlock_key_id`), the key's `last_used_at` is stamped, and the
    answer is the ordinary `FieldTokenResponse`. No domain event: a sign-in emits none today.

### 4.6 Failures and limits

- A failed unlock (rule 7) and a replayed or foreign challenge (rule 8) count exactly as a wrong PIN
  does: one more `failed_since_success` on the worker (soft lock at `softLockThreshold`, re-armed by
  each further failure, hard lock at `hardLockThreshold`), and one failure in the device bucket
  (`FC_FIELD_DEVICE_FAILURES_PER_HOUR`) and the client bucket (`FC_FIELD_CLIENT_FAILURES_PER_HOUR`), with
  the same `field-device:throttled` notices. The design's lockout rules speak of "failures since the
  last success", and a forged or replayed signature is the unlock's wrong guess.
- An unknown key id (rule 4) counts in the device and client buckets only (there is no worker).
- A successful unlock clears the soft counter, as a successful PIN does: it took the worker's biometric
  on the worker's phone. A hard lock is never cleared by an unlock (rule 6 refuses first).
- A worker reaching the hard lock (by PINs or by unlocks) has every unlock key revoked
  (`WORKER_LOCKED`, section 7) in the same transaction.
- The challenge route is limited per device (section 4.3) and refused while the device's or client's
  failure budget is full, so a throttled device cannot keep asking.

### 4.7 Revoke from the device

`/field/auth/unlock-keys/revoke {unlockKeyId}`: any holder of the device key may revoke a key on that
device (reason `DEVICE`, event `field-unlock-key:revoked`): revoking only removes access, and it is how
the library turns biometrics off, forgets a worker, or reports a key the phone invalidated, even when
no session is left. A key of another device, an unknown key and a revoked key answer `204` and change
nothing (no oracle; idempotent, as logout). It does not end the session the worker is in.

Because this route needs only the device key, whoever holds the phone can revoke the current worker's key and
then, after their own PIN sign-in on it, register theirs. That is the hand-over this spec intends for a phone
that changes hands; `409 UNLOCK_KEY_OTHER_WORKER` protects against silently replacing another worker's key, not
against a deliberate hand-over by someone who holds the phone and their own PIN. The revocation is recorded
(event and reason `DEVICE`).

## 5. Schema (migration 129)

```
field_unlock_keys (
  id                   VARCHAR(17) PRIMARY KEY,   -- fwk_
  client_id            VARCHAR(17) NOT NULL,
  identity_id          VARCHAR(17) NOT NULL,      -- the worker; no foreign key (docs/foreign-keys.md)
  device_id            VARCHAR(17) NOT NULL,
  jkt                  VARCHAR(64) NOT NULL,      -- RFC 7638 thumbprint, UNIQUE
  public_key_jwk       TEXT        NOT NULL,      -- canonical {"crv","kty","x","y"}
  attestation_chain    TEXT        NULL,          -- the chain as sent (JSON array of base64 DER), public data
  attestation          JSONB       NOT NULL,      -- the parsed summary (section 6.2)
  attestation_verified BOOLEAN     NOT NULL,      -- met the REQUIRED rule when registered (section 6.3)
  pin_verified_at      TIMESTAMPTZ NOT NULL,      -- section 4.4
  created_at           TIMESTAMPTZ NOT NULL,
  last_used_at         TIMESTAMPTZ NULL,
  revoked_at           TIMESTAMPTZ NULL,
  revoked_reason       VARCHAR(30) NULL CHECK (revoked_reason IN ('REPLACED','PIN_CHANGED','PIN_RESET',
                         'WORKER_SUSPENDED','WORKER_LOCKED','DEVICE_REVOKED','POLICY_DISABLED',
                         'POLICY_CHANGED','ADMIN','DEVICE')),
  revoked_by           VARCHAR(100) NULL          -- principal id (ADMIN), else the operation
)
-- revoked_reason also admits 'ATTESTATION_REVOKED'

field_sessions + unlock_key_id VARCHAR(17) NULL   -- the key that signed the family in; partial index while live
UNIQUE (jkt); UNIQUE (device_id) WHERE revoked_at IS NULL;
INDEX (identity_id); INDEX (client_id) WHERE revoked_at IS NULL

field_unlock_challenges (
  challenge_hash BYTEA PRIMARY KEY, unlock_key_id VARCHAR(17) NOT NULL, device_id VARCHAR(17) NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL
)

field_policies + unlock_keys_enabled BOOLEAN NOT NULL DEFAULT FALSE
               pin_reauth_days default 7 -> 0 (stored rows keep their value: a client that saved its
                 policy earlier has a real 7-day interval until it changes it; the owner, 2026-11-26)
               + unlock_key_attestation VARCHAR(10) NOT NULL DEFAULT 'NONE' CHECK (IN ('NONE','REQUIRED'))
```

TSID prefix `fwk` (the design's). Queries are sqlc (`internal/sqlc/queries/field_unlock_keys.sql`).
Deleting a worker or a device deletes its keys and challenges in the same transaction. The purger
deletes expired challenges and keys revoked more than 30 days ago (as sessions). The live-per-device
index is D2's guard against two registrations racing: the loser is `409 UNLOCK_KEY_OTHER_WORKER`.

## 6. Attestation

### 6.1 What each platform can prove

- **Android key attestation.** A key made in the Keystore with an attestation challenge comes with a
  certificate chain: the leaf certifies the key and carries the key description extension
  (`1.3.6.1.4.1.11129.2.1.17`), signed up to a Google attestation root. The description says where the
  key lives (`attestationSecurityLevel`, `keyMintSecurityLevel`: Software, TrustedEnvironment,
  StrongBox), the challenge, and two authorization lists (enforced in software, enforced in secure
  hardware) holding, among others, the purpose, the algorithm and curve, `noAuthRequired`,
  `userAuthType` (bit 1 password, bit 2 biometric) and `authTimeout`. A key made with
  `setUserAuthenticationParameters(0, AUTH_BIOMETRIC_STRONG)` lists `userAuthType = 2` and no
  `authTimeout` (or 0) in the hardware list. `setInvalidatedByBiometricEnrollment(true)` is **not**
  visible in the attestation (it is the key's binding to the current biometric set): the platform
  cannot prove it, and the library must set it.
- **iOS.** A Secure Enclave key with the `.biometryCurrentSet` access control has no per-key
  attestation. App Attest attests the app and an App Attest key, which cannot carry a biometric access
  control, so it proves nothing about the unlock key. An iOS registration sends no chain; its verdict is
  `NONE`. Under `REQUIRED` it is refused, and the console says so.
- **Browsers.** WebCrypto keys cannot require a biometric per use; `field-js` in a plain browser
  offers no unlock key (section 12).

### 6.2 What the platform checks and records (always)

Every chain is checked the same way whatever the policy; the policy decides only whether the result
refuses anything (6.3). The rules follow Google's own verifier, `KeyAttestationCertPathValidator` of
https://github.com/android/keyattestation, and Google's guidance at
https://developer.android.com/privacy-and-security/security-key-attestation, and were checked against the
real chains that library is tested on (its `testdata/`: factory chains of a Pixel 2 `walleye`, Pixel 3
`blueline` and Pixel 8 `akita` era devices and a Sony Xperia 10 III; Remote Key Provisioning chains of the
`caiman`, `tegu` and `frankel` devices; the software chain of `marlin`). All eight pass the rules below at a
time inside their validity (2026-11-26, this work).

| Rule | Real chains | Refusal |
|---|---|---|
| The leaf certifies the registered key | always | `MALFORMED` |
| Each certificate's issuer name is the next one's subject, each signature verifies under the next certificate, the last is self-issued and self-signed | always (Google checks name chaining and signatures) | `MALFORMED` |
| The key description (`1.3.6.1.4.1.11129.2.1.17`) is on the leaf and on no other certificate, except the app's own attestation key (hardware-enforced purpose exactly `ATTEST_KEY`, 7, and its own key description saying `TEE` or `STRONGBOX` for both the key and the attestation) directly above the leaf | the extension is on the leaf only; Google: "Only the first occurrence of the extension in the chain can be trusted. Any further instances of the extension have not been issued by the secure hardware and might have been issued by an attacker extending the chain" | `MALFORMED` |
| Every certificate above the leaf's issuer is a CA (`basicConstraints` CA true) that may sign certificates (`keyUsage` certSign, when key usage is present), and no stated path length is exceeded by the certificates under it | all eight; `Droid CA2` states path length 2 with two certificates under it, the software intermediate states 0 | `MALFORMED` |
| The leaf's issuer (the attestation key certificate) is **not** required to be a CA | the Pixel 2 and Sony factory batch certificates have CA false and `digitalSignature` only, so requiring it would refuse those phones (Google's verifier does not require it either: "Older Android devices produce Key Attestation certificate chains that do not fully conform to RFC 5280") | — |
| The root is one of `FC_FIELD_ATTESTATION_ROOTS` (compared by public key: Google's four RSA roots share one key) | Google publishes its roots at https://android.googleapis.com/attestation/root | `UNTRUSTED` |

The reviewer's forgery (an attested app key with no user authentication signing a fake leaf for a
software key) is refused by the third rule: the app key's own certificate, now above the leaf, carries a key
description and is not an attestation key (`TestEvaluate_ForgedLeafSignedByAnAttestedAppKeyIsRefused`).

**Dates, checked once, at registration** (never again for that key): Google's verifier "Do[es] not check
the validity of the final certificate in the path. The validity period of the final cert is set on the
device so could both be subject to tampering and could be impacted by clock skew"; it refuses a certificate
not yet valid; it ignores expiry "on factory-provisioned certificate chains because it is not possible to
safely rotate the keys", and the guidance says "Devices launched before 2021 have attestation keys with
expired certificates. These keys are still trustworthy unless they appear in the certificate revocation
list" while "It's critical, however, to ensure that Remote Key Provisioning (RKP) certificates continue to
have their validity period checked". So:

- the leaf's dates are not checked; the root's are not (it is the trust anchor, as in Google's verifier;
  the 2016 Google root at the top of the Pixel 2, Pixel 3 and Sony chains expired on 24 May 2026);
- every other certificate must have begun (`DatesInvalid`, "not yet valid", for any chain);
- it must not have expired, unless the chain is factory-provisioned. As Google's verifier does, a chain is
  factory-provisioned when the certificate under the root names a `SERIALNUMBER` in its subject (true of
  all four factory chains, false of the RKP chains, whose certificate under the root is `Droid CA2`).
- RKP certificates live days to weeks (in the samples, 9 to 70 days); a phone gets fresh ones from
  Google's provisioning service, so an RKP chain is always within its dates when the phone makes the key. A
  phone kept offline past its RKP certificates' expiry cannot make an attested key until it reconnects; it
  is refused under `REQUIRED` only, and its worker uses the PIN.

**Revocation** (6.4): every certificate above the leaf (the leaf's serial is always 1) is looked up in the
attestation key status list.

Recorded with the key (`attestation` JSONB) and shown on the admin API and in the console:

| Field | Meaning |
|---|---|
| `format` | `ANDROID_KEY` when a chain was sent, else `NONE` |
| `verdict` | `NONE`, `MALFORMED` (with `problem`), `UNTRUSTED`, `TRUSTED` (the rules above) |
| `securityLevel` / `attestationSecurityLevel` | where the key lives / where the attestation was made: `STRONGBOX`, `TEE`, `SOFTWARE` |
| `challengeMatches`, `authPerUse`, `biometricOnly`, `signP256` | as before: section 3's challenge; in the hardware list, authentication for every use, biometric only, a P-256 signing key |
| `provisioning` | `FACTORY` or `REMOTE` |
| `dates`, `datesProblem` | `VALID` or `INVALID` at registration |
| `revocation` | `CLEAR` (a usable list names nothing), `REVOKED` / `SUSPENDED` (it names a certificate), `NOT_CHECKED_OFF` (the platform's check is off), `NOT_CHECKED_UNAVAILABLE` (no usable list at registration) |
| `checkedAt` | the registration time the dates and the list were checked at |
| `rootFingerprint`, `attestationVersion` | as before |
| serials (stored, not on the wire) | the chain's certificate serials above the leaf, lowercase hex, so a list refreshed later can name the key's chain |

### 6.3 What the platform enforces (only what the policy says)

`unlockKeyAttestation` is `NONE` by default: **nothing is enforced**; every check above is made and
recorded, and the record says what was and was not checked (`revocation: NOT_CHECKED_...`). `REQUIRED`
refuses a registration (`422 ATTESTATION_REQUIRED`, the message naming the first shortfall) unless: the
verdict is `TRUSTED`; `securityLevel` **and** `attestationSecurityLevel` are `TEE` or `STRONGBOX`;
`challengeMatches`, `authPerUse`, `biometricOnly` and `signP256` hold; `dates` is `VALID`; and `revocation`
is `CLEAR`, or `NOT_CHECKED_OFF` when the platform's list check is switched off. With no usable list (never
fetched, or older than its maximum age) a registration that would otherwise pass is refused with
`503 ATTESTATION_STATUS_UNAVAILABLE` rather than accepted unchecked. The trusted roots are
`FC_FIELD_ATTESTATION_ROOTS`; without them `REQUIRED` cannot be set (`400 POLICY_INVALID`). The result at
registration is stored as `attestation_verified`.

`REQUIRED` is also checked at every **challenge and unlock**, so no policy-derived rule rests on the
revocation that runs when the policy changes alone: a key whose `attestation_verified` is false (registered
under `NONE`, or by a registration that raced the change) is refused as `401 UNLOCK_KEY_INVALID`; a key
whose recorded serials a usable, refreshed status list now names is revoked (`ATTESTATION_REVOKED`, its
sessions ended) and refused the same way. When the list is on and no usable copy exists at that moment, the
challenge and the unlock are refused with `503 ATTESTATION_STATUS_UNAVAILABLE` and `Retry-After`, and the key
is **not** revoked (section 6.4, "With no usable copy"). A registration and a policy change of one client also
take one lock (`pg_advisory_xact_lock(hashtext('field-policy:' || client id))`), so one runs before the
other.

**What `REQUIRED` still does not prove**, so a client can judge it:

- **Verified boot.** The key description's `rootOfTrust` (verified-boot state, locked bootloader) is not
  read: a key made by genuine hardware on a phone with an unlocked bootloader passes.
- **The attesting application.** `attestationApplicationId` (the package and its signing certificate) is not
  checked: a key made by another app on the same phone, with the same properties, passes the attestation
  (the registration still needs the worker's fresh PIN sign-in and the device key).
- **Revocation between refreshes.** The list is as fresh as its last fetch (`FC_FIELD_ATTESTATION_STATUS_REFRESH`,
  default 1 hour, usable for `FC_FIELD_ATTESTATION_STATUS_MAX_AGE`, default 48 hours); a key revoked by Google
  is revoked, and its sessions ended, by the check after the fetch that first lists it, not before.
- **Dates after registration.** A chain valid when the key was registered stays accepted for that key, by
  design; certificate dates are never checked again.
- **With the list switched off** (`FC_FIELD_ATTESTATION_STATUS_URL` empty) `REQUIRED` means a trusted, well
  formed chain within its dates, and nothing about revoked attestation keys.
- **Device-held facts.** `setInvalidatedByBiometricEnrollment` is not in the attestation (6.1).

### 6.4 The attestation key status list

Google publishes it at https://android.googleapis.com/attestation/status: JSON
`{"entries": {"<serial>": {"status": "REVOKED" | "SUSPENDED", "expires"?, "reason"?, "comment"?}}}`, the key
"the certificate's X.509 serialNumber integer formatted as a lowercase hexadecimal string with leading
zeros stripped" (developer.android.com, "Certificate revocation status list"). The platform:

- fetches it in the background, at start and every `FC_FIELD_ATTESTATION_STATUS_REFRESH` (default 1 hour;
  Google says the response's `Cache-Control` decides how often, and the published list is refreshed far less
  often than hourly), with a 30-second timeout; **no request ever waits on the network**;
- keeps the last good copy and when it was fetched. Only an answer that is not the published shape at all
  (not JSON, no `entries` object) is refused, so it never replaces a good copy. Inside a well-shaped list the
  reading leans to revoked: a serial is normalised (lower case, leading zeros dropped) and looked up in the
  same form; an entry whose status is not `SUSPENDED` (`REVOKED`, a status this version does not know, none,
  an entry that is not an object) counts as revoked; a key that is not hexadecimal is skipped, since no
  certificate can match it, and the fetch logs how many. A copy older than `FC_FIELD_ATTESTATION_STATUS_MAX_AGE`
  (default 48 hours) is not usable;
- after each good fetch, checks the list against every key it covers (`SweepAttestationStatus`): the keys of
  clients under `unlockKeysEnabled` and `REQUIRED` that are live or still have a live session, including keys
  the phone revoked or a replacement ended. A key whose chain it names is revoked (`ATTESTATION_REVOKED`, if
  it was still live) and its live sessions end, each in its own transaction. A key's sessions refresh without
  the key, so a check only at the next challenge would leave them running;
- treats `SUSPENDED` as `REVOKED`, deliberately and conservatively: in the list's own meaning a suspension is
  temporary, but a key revoked for it stays revoked when the entry is lifted. The platform cannot tell when a
  suspended attestation key becomes trustworthy again, and it keeps no "suspended" state to undo; once the entry
  is gone, the worker registers a new key after a PIN sign-in on the phone, as after any other revocation;
- logs a failed fetch as a warning while the copy it keeps is usable, and as an **error at every attempt**
  once no usable copy is left (never fetched, or too old), because from then on every unlock under `REQUIRED`
  is refused;
- fetches only from an `https` address, refused at start-up otherwise (the platform does not start), and
  follows at most three redirects, each to `https` on the same host; a redirect to another host or to plain
  `http` fails the fetch. The body is read up to 16 MiB; each fetch has 30 seconds;
- fetches only when attestation roots are configured, from `FC_FIELD_ATTESTATION_STATUS_URL` (default
  Google's; set empty to switch the check off, in which case `REQUIRED` means what 6.3 says). A platform with
  `REQUIRED` in use needs outbound HTTPS to that address;
- tests use a local TLS fixture server and never the network (no exception for plain `http`, not even on a
  loopback address, was needed).

**With no usable copy** (the list is on, and it was never fetched or the last good copy is older than its
maximum age), under `REQUIRED`: a registration is refused, and so are a challenge and an unlock, all with
`503 ATTESTATION_STATUS_UNAVAILABLE` (`Retry-After` at the challenge and the unlock). The key is not
revoked and nothing is counted against the worker; sessions the key already signed in go on; the mobile
library treats the answer as "sign in with your PIN for now" and tries the key again later. Under `NONE`
nothing changes. This is the coordinator's choice of 2026-11-26, reported to the owner as such: once
`REQUIRED` has been chosen, the platform does not admit a key it cannot check now (the copy it would trust
may be days old, which is exactly when a revocation could be missed), and it does not revoke one either, since
the failure is the platform's and not the key's; the PIN is always there. Before this, a key already
registered was admitted without a list.

The console's policy page says, beside the `REQUIRED` switch, when the list is switched off, has never been
fetched, or is too old (a warning while `REQUIRED` is on, with the last fetch time); the fetch failure is
logged.

## 7. Policy and revocation

Two new fields of the client's field policy, with `pinReauthDays`, which now has an effect:

| Field | Default | Bounds | A change takes effect |
|---|---|---|---|
| `unlockKeysEnabled` | `false` | | (the policy PUT replaces the whole policy, as for every other field: a PUT that omits it switches unlock keys off and revokes the client's keys; see section 14) on to off: every live key of the client is revoked (`POLICY_DISABLED`) in the policy's transaction; off to on: nothing (there are none) |
| `unlockKeyAttestation` | `NONE` | `NONE`, `REQUIRED` | `NONE` to `REQUIRED`: every live key with `attestation_verified` false is revoked (`POLICY_CHANGED`) in the same transaction; back to `NONE`: nothing |
| `pinReauthDays` | **0 (never)** | 0..90; 0 forces no periodic PIN (D3) | at the next challenge or unlock of every key (the deadline is computed, not stored). Migration 129 sets the column default to 0; stored rows keep their value (the old default 7 cannot be told from a client's choice), so a client that saved its policy before shows 7 until it changes it |

Every way a key ends, and where it is hooked:

| Trigger | Keys revoked | Reason | Where |
|---|---|---|---|
| The worker changes their PIN | the worker's keys on every OTHER device; the key on the device that changed it is kept and its `pin_verified_at` moved to now (from a key that is not an enrolled device, every key) | `PIN_CHANGED` | `ChangeOwnPin`, the rule its sessions already follow (other families revoked, this one kept). The owner confirmed it on 2026-11-26: changing the PIN needs the current PIN in a signed-in session on that phone, so the person has just proved the PIN there and the key never left that phone's hardware; a supervisor's reset (the worker does not know the PIN, or it is suspected) revokes everything |
| An administrator resets the PIN | all the worker's keys | `PIN_RESET` | `ResetWorkerPin` |
| The worker is suspended | all | `WORKER_SUSPENDED` | `SuspendWorker` |
| The worker reaches the hard lock | all | `WORKER_LOCKED` | `AuthTx.RecordFailure` (login, PIN change, act-as, unlock) |
| The worker is deleted | all (rows deleted) | | `IdentityRepo.Delete` |
| The device is revoked | the device's | `DEVICE_REVOKED` | `RevokeDevice` |
| The device is deleted | the device's (rows deleted) | | `DeviceRepo.Delete` |
| The device is re-enrolled | none needed: a re-enrolment is a new device row with a new key, and a key works only from the device it was registered on | | the binding of section 4.5 rule 4 |
| The policy switches unlock keys off / starts requiring attestation | the client's / those not verified, and their login families | `POLICY_DISABLED` / `POLICY_CHANGED` | `PutPolicy`; and, because a revocation at the moment of the change is not enough, every challenge and unlock refuses a key the policy no longer admits (unlock keys off: `UNLOCK_KEYS_DISABLED`; `REQUIRED` and not verified: `UNLOCK_KEY_INVALID`) |
| An administrator revokes | the one, **and the login families it signed in**; on a key already revoked (by the phone, a replacement, anything), its live sessions only, the key keeping its reason | `ADMIN` | `RevokeUnlockKey` (admin API, console: Revoke on a live key, End sessions on a revoked one with live sessions) |
| A refreshed status list names the key's chain (under `REQUIRED`) | the one, and its login families (also of a key revoked earlier) | `ATTESTATION_REVOKED` | the check after each fetch (`SweepAttestationStatus`), and the challenge and the unlock (`RevokeUnlockKeyForAttestation`) |
| The worker or library, from the device | the one | `DEVICE` | `RevokeOwnUnlockKey` (`/field/auth/unlock-keys/revoke`) |
| The same worker registers again on the device | the previous one | `REPLACED` | `RegisterUnlockKey` |

**Sessions.** Each login family an unlock signs in records its key (`field_sessions.unlock_key_id`, copied to
every rotated child). Ending them is **by key**, whoever revoked the key and whenever: an administrator's
revoke, the policy's revocations and the status list's end every live session whose `unlock_key_id` is a key
the rule covers, live or already revoked (`revoked_reason` `UNLOCK_KEY_REVOKED`, event
`field-session:revoked`): switching unlock keys off, every key of the client; starting to require attestation,
every key registered without a verified one; the status list, every key whose chain it names; the
administrator, the one key. The other triggers end the worker's or the device's sessions themselves (a
reset, a suspension, a revoked device, a PIN change on another device) or keep them on purpose:

- **The worker's own revoke from the phone** keeps the session they are in (they are switching the feature
  off, not signing out).
- **A replacement** keeps the old key's sessions. It is the same worker on the same phone, who has just
  proved their PIN there (a registration needs a PIN sign-in in the last 10 minutes); nothing about who holds
  the phone changed, and ending them would sign the worker out of the apps the old key signed in for no gain.
- **A hard lock** keeps them, as it keeps the worker's PIN sessions.

Such a key's sessions stay visible: the admin API's `liveSessions` and the console's "Open sessions" count
them, and the administrator's revoke (the console's End sessions) ends them. A refresh of a session an unlock
key signed in checks, under the worker's lock, that unlock keys are on, the key exists, it was not revoked for
a reason that ends its sessions (or one this version does not know), and under `REQUIRED` that it was
verified; otherwise `401 INVALID_GRANT`.

**Lock order.** Every transaction that changes a worker's sessions or keys takes the worker rows first, in
ascending id and in one statement, after only the client's policy lock (taken by a registration and a policy
change), and then the device, key and session rows; an operation that touches several workers (a policy
change: every worker with a key of the client; a device's revocation: every worker with a key or a live
session on it; a suspension, update or deletion: the worker and those whose sessions they hold open as an
actor) learns the set first and locks it at once; a console sign-in code's redemption locks the code's worker
before the code. An act-as never holds another worker's row while a PIN is hashed: the target is resolved
first, without a lock and only within the supervisor's client (another client's worker is never locked);
the supervisor's PIN is verified under the supervisor's lock alone; then the supervisor and the target are
locked in id order and the supervisor is checked again (still allowed to act, the same stored PIN). A refresh takes the
worker's row too, so a revocation runs wholly before or after it and a child session can never escape it.
The order is written once in `internal/platform/fieldidentity/lock_order.go`; a deadlock is a bug in it and
is not retried.

Not triggers, with the reason: **a device suspended** keeps its keys (suspension is resumable, and every
`/field/*` call from a suspended device is refused anyway); **a grant removed** keeps the key (a key is
the worker's on the device, for any application; every unlock checks the grant, as every login does, so
the removed application is `403 NO_APP_ACCESS`); **a client deactivated** keeps them (every unlock checks
the client); **a session ended** (logout, expiry) keeps them, since signing in again after that is what a
key is for.

## 8. Admin API

Permissions are the worker routes': reading needs `platform:iam:field-worker:view` (or manage),
revoking `platform:iam:field-worker:manage`; client-boxed and application-confined as the other worker
routes (another client's worker or key is `404`).

- `GET /api/field-workers/{id}/unlock-keys` → `{unlockKeys: FieldUnlockKey[]}`, newest first, live and
  revoked (revoked ones until the purger removes them).
- `POST /api/field-workers/{id}/unlock-keys/{unlockKeyId}/revoke` → `FieldUnlockKey`: revokes a live key
  (`ADMIN`) and ends the live sessions it signed in. On a revoked key it changes nothing on the key (which keeps
  its reason) and ends the sessions it left; with none left, nothing happens. Events
  `field-unlock-key:revoked` (reason `ADMIN`) when it revoked the key, `field-session:revoked` when it ended
  sessions.
- `GET /api/field-devices/{id}` and `GET /api/field-devices` gain `unlockKey: FieldUnlockKey | null`, the
  device's live key (device view permission; one batched query for a list).
- The policy (`GET|PUT /api/field-policies/{clientId}`) carries `unlockKeysEnabled` and
  `unlockKeyAttestation` (a PUT replaces the policy: omitted, they go back to `false` and `NONE`).

### 8.1 Shapes

```
FieldUnlockKey = {id, workerId, deviceId, status: "ACTIVE" | "REVOKED", createdAt, lastUsedAt?,
                  pinVerifiedAt, pinRequiredBy?, revokedAt?, revokedReason?, revokedBy?,
                  liveSessions, attestation: FieldUnlockKeyAttestation}
FieldUnlockKeyAttestation = {format: "ANDROID_KEY" | "NONE",
                  verdict: "NONE" | "MALFORMED" | "UNTRUSTED" | "TRUSTED",
                  securityLevel?: "STRONGBOX" | "TEE" | "SOFTWARE",
                  attestationSecurityLevel?, challengeMatches, authPerUse, biometricOnly, signP256,
                  rootFingerprint?, attestationVersion?, provisioning?: "FACTORY" | "REMOTE",
                  dates?: "VALID" | "INVALID", datesProblem?, revocation?: "CLEAR" | "REVOKED" | "SUSPENDED" |
                  "NOT_CHECKED_OFF" | "NOT_CHECKED_UNAVAILABLE", checkedAt?, meetsRequirement}
```

`liveSessions` is how many unexpired sessions the key signed in are still live (a revoked key can have
some: section 7). `meetsRequirement` is `attestation_verified`. Never the public key, the chain or the thumbprint: none is
secret, but none helps an administrator, and keeping them off the wire keeps them off logs.

## 9. Events and audit

- `platform:iam:field-unlock-key:registered` `{unlockKeyId, workerId, clientId, workerCode, deviceId,
  replacedUnlockKeyId?, attestationVerdict, securityLevel?}`, audited as the worker.
- `platform:iam:field-unlock-key:revoked` `{clientId, reason, count, unlockKeyId?, workerId?, deviceId?}`:
  one per revoking action (an explicit revoke names the key; a bulk revocation by another operation
  carries the count, as `field-session:revoked` does), emitted only when something was revoked. The
  explicit ones are audited as the administrator (`ADMIN`) or the worker (`DEVICE`). A hard lock reached
  on the device protocol emits it after its transaction (the failure is recorded first, as throttle
  notices are).
- Attempts (`field_login_attempts`, the sign-in history): `UNLOCK_KEY` for every unlock (success and each
  refusal with its reason) and `UNLOCK_KEY_REGISTER` for every registration. The challenge route records
  nothing (it is limited instead), nor does a device revoke beyond its event.
- Never key material: no public key, thumbprint, chain, challenge or signature in an event, an audit row,
  an attempt or a log line. Commands carrying them mark them `json:"-"`.

## 10. Errors (new and reused)

| Code | Status | Route | The library does |
|---|---|---|---|
| `SHARED_DEVICE` | 403 | register, challenge, unlock | never offer biometrics on this device |
| `UNLOCK_KEYS_DISABLED` | 403 | register, challenge, unlock | delete the local key; PIN |
| `PIN_SIGN_IN_REQUIRED` | 403 | register | offer the key after the next PIN sign-in |
| `WORKER_UNAVAILABLE` | 403 | register | as for a session that ended |
| `UNLOCK_KEY_OTHER_WORKER` | 409 | register | tell the worker the phone's unlock belongs to someone else |
| `UNLOCK_KEY_EXISTS` | 409 | register | make a new key |
| `INVALID_UNLOCK_PROOF` | 401 | register, unlock | PIN; delete the key if it repeats |
| `UNLOCK_KEY_INVALID` | 401 | challenge, unlock | delete the local key; PIN ("sign in with your PIN") |
| `CHALLENGE_EXPIRED` | 401 | unlock | new challenge, prompt again |
| `PIN_REAUTH_REQUIRED` | 401 | challenge, unlock | keep the key; PIN; the key works again after it |
| `ATTESTATION_REQUIRED` | 422 | register | this phone cannot have a key under the client's policy |
| `ATTESTATION_STATUS_UNAVAILABLE` | 503 | register, challenge, unlock | the platform has no usable status list under `REQUIRED`: sign in with the PIN for now and try the key again later (`Retry-After`); the key is not revoked |
| `PIN_RESET_REQUIRED` | 403 | challenge, unlock | as at the PIN sign-in: a supervisor must reset the PIN |
| reused: `UNKNOWN_DEVICE` 401, `IMPERSONATING` 403, `invalid_token` 401, `INVALID_REQUEST` / `INVALID_PUBLIC_KEY` 400, `WORKER_LOCKED` 423, `WORKER_LOCKED_TEMPORARILY` 429, `TOO_MANY_ATTEMPTS` 429, `NO_APP_ACCESS` 403, `DEVICE_SUSPENDED` / `DEVICE_REVOKED` 403 | | | as today |

## 11. Wire contract, fixtures, docs, tests

- `docs/wire-contract.md`: the four routes, the JWS formats, the challenge, the errors, `amr`
  `["hwk","uv"]`, the admin routes and shapes, the policy fields, the events, `FieldTokenResponse.unlockKeys`.
- `api/openapi.lock.json` and the SPA's generated types (`make api-bump`, `pnpm api:generate`).
- `api/field-fixtures/cases.json` gains `valid_unlocked` (`amr ["hwk","uv"]`, PERSONAL device) for the
  SDK verifiers; `api/field-fixtures/unlock.json` (new) holds a fixed unlock key, the claim values and
  the exact signing inputs of a key proof and an assertion with a valid signature, plus the variants the
  platform refuses, so the libraries can check their signer byte for byte.
- `docs/published/45-field-workers.md`: what an unlock key is, how to turn it on, what it records.
- `docs/environment-variables.md`: `FC_FIELD_UNLOCK_CHALLENGES_PER_HOUR`, `FC_FIELD_ATTESTATION_ROOTS`,
  `FC_FIELD_ATTESTATION_STATUS_URL`, `FC_FIELD_ATTESTATION_STATUS_REFRESH`, `FC_FIELD_ATTESTATION_STATUS_MAX_AGE`.
- Design doc: the phase table row P5 built, with what is and is not enforced.
- Tests: unit tests of the entity, the JWS verification (real P-256 signatures made in the test), the
  attestation parser (chains built in the test with the key description extension), the policy
  validation; integration tests (`-tags=integration`) of the whole path and of every refusal of
  sections 2.1, 4.3, 4.5 and every trigger of section 7.

## 12. What the libraries, SDKs and framework need (later pieces)

**Device libraries** (`field-android` first; `field-react-native` over it; `field-ios`; `field-js` only
through the native bridge):

1. Offer a key only when the session's latest `FieldTokenResponse` says `unlockKeys.allowed` and carries
   `unlockKeys.registerExpiresIn` (section 2.3), and only when the worker agrees; when
   `unlockKeys.attestationRequired` is true, offer it only on a phone that can attest the key (Android with an
   attestation chain). A library must not probe the platform for this (no empty or test registration, no
   request made to read the refusal): the token answer is the only source. Then make a key: Keystore EC P-256, alias per worker, `PURPOSE_SIGN`, `DIGEST_SHA256`,
   `setUserAuthenticationRequired(true)`, `setUserAuthenticationParameters(0, AUTH_BIOMETRIC_STRONG)`,
   `setInvalidatedByBiometricEnrollment(true)`, StrongBox first then TEE, `setAttestationChallenge`
   (section 3). iOS: Secure Enclave P-256 with `SecAccessControl` `.privateKeyUsage` and
   `.biometryCurrentSet`, no chain.
2. Within the session's `registerExpiresIn` (counted from when that answer arrived): sign the key proof through `BiometricPrompt.CryptoObject(Signature)`
   (iOS: `SecKeyCreateSignature` after `LAContext`), and call `/field/auth/unlock-keys`. Keep
   `{unlockKeyId, workerId, pinRequiredBy}` (not secret) in the store.
3. To sign in again: `/field/auth/unlock/challenge`; on `200`, prompt, sign the assertion, call
   `/field/auth/unlock` with the next nonce; map the errors of section 10 (`503 ATTESTATION_STATUS_UNAVAILABLE`:
   keep the key, sign in with the PIN for now, try the key at the next sign-in). Never prompt when the challenge
   was refused.
4. A `KeyPermanentlyInvalidatedException` / `UnrecoverableKeyException` (iOS: `errSecItemNotFound` after
   a biometric change) is "re-enrol, PIN required": call `/field/auth/unlock-keys/revoke`, delete the
   key, ask for the PIN, never crash. A sign-out that forgets the worker revokes and deletes too.
5. Never on a SHARED device, an unenrolled key, an impersonated session, a browser `WebCryptoKeyProvider`
   (it cannot require a biometric per use), or a session from an unlock. The existing local unlock
   (`unlock()` of the native session owner) stays as it is: it gates screens, not sign-in.
6. Check the signer against `api/field-fixtures/unlock.json`.

**Server SDKs**: no verifier change (`amr` is already exposed); fixture case `valid_unlocked` must pass.
Admin wrappers where the SDK has them: `fieldWorkers.listUnlockKeys(id)`,
`fieldWorkers.revokeUnlockKey(id, keyId)`, the device's `unlockKey`, the two policy fields. The generated
TS and Laravel clients pick them up when regenerated.

**Framework**: nothing is required. An application that wants "worker present now" before a high-value
action reads `amr` (`hwk` + `uv`) and `auth_time` from `FieldPrincipal`, as it would `pin`.

## 13. Not in scope, and why

- No Java or Rust hand-off note: the Java port is paused (the latest notes, 2026-11-20 and 2026-11-22,
  are the last; the features after them, worker photos and credential publishing, wrote none). If the port
  resumes, this document and the wire contract are its source.
- No enforcement beyond section 6.3; no Google attestation status list; no App Attest; no passkeys.
- No console flow for registering a key (only the phone can make one).

## 14. Open with the owner

Decided by the owner on 2026-11-26: D3 (no periodic PIN by default; stored policies that hold 7 keep it); a PIN
change keeps the changing device's key (section 7); `REQUIRED` checks certificate dates and the status list
(section 6). Still open:

1. D1 (10-minute window, no step-up PIN) and D2 (one live key per device; a second worker refused; the
   device's own revoke is the hand-over).
2. **The policy PUT and omitted fields.** The wire contract makes `PUT /api/field-policies/{clientId}` a full
   replacement: an omitted field goes back to its default (stated for the photo settings, and the SDKs say
   "Get the policy and send every field back"). So an integration written before this work that PUTs the
   policy switches unlock keys off (revoking every key and ending their sessions) and resets `pinReauthDays`.
   Making only the new fields keep their stored value when omitted would break that rule, so it is not built.
   Options: (a) keep full replacement and document the hazard (as built); (b) an exception for
   `unlockKeysEnabled`, `unlockKeyAttestation` and `pinReauthDays` (omitted keeps the stored value; the Go SDK's
   fields become pointers so `false` can still be sent); (c) PATCH semantics for the whole PUT (a contract
   change for every field and SDK); (d) a separate `PATCH` route, PUT unchanged.
3. Whether `REQUIRED` should also check verified boot (`rootOfTrust`) and the attesting app's identity
   (`attestationApplicationId`), which it does not (6.3).
4. **Told to the owner, the coordinator's choice of 2026-11-26:** under `REQUIRED` with no usable status
   list, a challenge and an unlock are refused with `503 ATTESTATION_STATUS_UNAVAILABLE` and the key is kept
   (6.4, "With no usable copy"); a replaced key keeps its sessions (section 7).

## 15. Build status

| Part | State |
|---|---|
| this document | written |
| migration 129, sqlc queries, entity, repository | built (`internal/migrate/sql/129_field_unlock_keys.sql`, `internal/sqlc/queries/field_unlock_keys.sql`, `internal/platform/fieldidentity/unlock_key*.go`) |
| attestation parser | built (`internal/platform/fieldidentity/attestation`); parsed and recorded always, enforced only under `REQUIRED` |
| the four `/field/*` routes, the JWS checks | built (`internal/platform/fieldauth/unlock.go`, `unlockproof.go`) |
| revocation hooks, lockout, limits, purge, events | built (section 7's table; `field_unlock_challenge_device` bucket; the purger) |
| sessions ended by key, the refresh's key check, the lock order | built (`unlock_key_revocations.go`, `fieldauth/session.go`, `fieldidentity/lock_order.go`) |
| `FieldTokenResponse.unlockKeys` (section 2.3) | built (`internal/platform/fieldauth/endpoints.go`, `fieldidentity.RegistrationDeadline`) |
| the status list's fetch (https only), parse, and check after each fetch | built (`attestation/status.go`, `unlock_key_attestation_sweep.go`) |
| admin API, OpenAPI lock, the Go SDK's types the lock check forces | built (`internal/platform/fieldidentity/api/unlockkeys.go`) |
| console (worker drawer with open sessions and End sessions, device drawer, policy page) | built (`internal/ui/pages/field_unlock_keys.go`) |
| fixtures (`valid_unlocked`, `unlock.json`) | built (`tools/field-fixtures`) |
| device libraries, SDK admin wrappers, framework | not built (section 12) |
