# field-android (`io.flowcatalyst:field-android`)

Kotlin library (AAR) that owns a field worker's FlowCatalyst session on Android: device key, DPoP proofs,
tokens, refresh, nonces, an OkHttp interceptor. No UI, no React Native dependency, never needs the main thread.
Spec: `docs/field-native-session-spec.md` section 1. Wire behaviour is a port of `clients/field-js/src/auth.ts`.

minSdk 26, compileSdk 35. Dependencies: OkHttp 4, kotlinx-coroutines, androidx.biometric (unlock only).

## Setup

Released builds (v0.1.0 and later, the `VERSION` file) come from GitHub Packages
(`https://maven.pkg.github.com/flowcatalyst/flowcatalyst`, coordinates
`io.flowcatalyst:field-android:<version>`; GitHub Packages needs credentials,
`GITHUB_ACTOR`/`GITHUB_TOKEN` or gradle properties `gpr.user`/`gpr.key`), then
`implementation("io.flowcatalyst:field-android:<version>")`. They are published
by the `release-field-android.yml` workflow (run by `workflow_dispatch`); see
`docs/releasing.md`. For local work on the library itself, use a composite
build instead: in the app's `settings.gradle`
`includeBuild("../path/to/clients/field-android")`, then
`implementation("io.flowcatalyst:field-android")`. To publish by hand:
`./gradlew publishReleasePublicationToGitHubPackagesRepository`.

The library manifest declares `INTERNET` and `USE_BIOMETRIC`.

## Use

```kotlin
val session = FieldSession.get(applicationContext, FieldConfig(
    platformUrl = "https://platform.example.com",
    appCode = "my-app",
    clientIdentifier = "acme",                       // needed for worker-code sign-in on an unenrolled device
    requireUnlock = UnlockPolicy(afterColdStart = true, afterBackground = 5.minutes),
))
session.restore()                                    // optional: waits for the stored session to load
session.enrolledDevice()                             // the stored DeviceRecord, or null when not enrolled
session.enrol("fcfield:1:https://platform.example.com:ABCDE12345", DeviceInfo(model = Build.MODEL))
session.login(LoginCredentials(workerCode = "W001", pin = "1234"))   // or badgeToken = scanned
session.redeemSignInCode(code); session.actAs(ActAsTarget(workerCode = "W002", pin = supervisorPin))
session.endImpersonation(); session.changePin(old, new); session.logout()
```

`FieldSession.get` is the process-wide owner; call it from anywhere with the same config (a different config throws).
`state` is a `StateFlow<FieldState>` (`SignedOut`, `SignedIn(session)`, `Locked(session)`); `addListener` delivers
`FieldEvent`s (`SessionChanged`, `SessionEnded(reason)`, `PinChangeRequired`, `ExpiryWarning(remaining)`, `Locked`,
`WorkerLocked`). Failures are `FieldAuthException(code, message)` (an `IOException`) with the field-js codes.

### From a foreground Service

```kotlin
class TrackingService : Service() {
    private val http by lazy {
        OkHttpClient.Builder().addInterceptor(FieldSession.get(this, config).interceptor()).build()
    }
    override fun onCreate() { FieldSession.get(this, config).keepAlive(true) }   // while a trip is active
    override fun onDestroy() { FieldSession.get(this, config).keepAlive(false) }
}
```

### Same-process rule

The service MUST run in the app's process: do not set `android:process` on it. The session (keys in memory, nonce
cache, single-flight refresh) is one object per process; a second process would have a second owner and refresh tokens
rotate, so two owners would fight (and a reused rotated token revokes the family).

### Errors

Failures are `FieldAuthException`s with a `code` (the platform's error code, or the library's own), `status` and, for lockouts, `retryAfterSeconds`.

`PIN_RESET_REQUIRED` (HTTP 403, from `login` and `actAs`): the worker's PIN was set under an app key that has since
been removed. It is not a wrong PIN and retrying cannot help. Show "Ask your supervisor to reset your PIN".
`e.isPinResetRequired` tests for it. The library does not retry, does not emit `WorkerLocked`, and does not clear the
enrolment or end a session.

### OkHttp interceptor

`session.interceptor()` (add as an application interceptor) signs every request, retries once on
`401 WWW-Authenticate: DPoP error="use_dpop_nonce"` with the new nonce, and once on `401 invalid_token` after a
refresh (exactly one refresh request however many callers need one). If the refresh ends the session
(`INVALID_GRANT`, `SESSION_EXPIRED`, `DEVICE_REVOKED`, `DEVICE_SUSPENDED`, `UNKNOWN_DEVICE`, `NO_APP_ACCESS`) the
session emits `SessionEnded(code)` and the request fails with `FieldAuthException`. An `invalid_token` that survives a
refresh emits `SessionEnded("invalid_token")` and the 401 is returned. Requests with one-shot bodies are not retried.
It blocks the calling thread (OkHttp's), so do not call enqueue/execute from the main thread.

### Other uploaders

```kotlin
val h = session.headersFor("POST", url)                       // Authorization + DPoP (refreshes first if needed)
// on 401 use_dpop_nonce: session.headersForRetry("POST", url, response.header("DPoP-Nonce")!!)
// any DPoP-Nonce seen elsewhere: session.noteNonce(url, nonce)
// Java / non-coroutine, off the main thread: session.headersForBlocking("POST", url)
```

Offline queues: store requests unsigned and sign when sending. Platform `/field/` nonces are single-use, application
nonces last about 5-10 minutes, and `iat` may be off by +-24 h, so a stored proof is useless.

With no signal the session stays: a call that cannot reach the platform fails with `NETWORK` and ends nothing, and
no session is ended by the device's clock. It ends when the platform refuses a refresh after the signal returns
(`SESSION_EXPIRED` once the idle or absolute limit has passed), and sign-in itself needs the platform. The limits,
what `keepAlive` can and cannot do offline, and the rules for a queue on a shared device are in
`clients/field-react-native/README.md`, "No signal: what happens to a session"; they hold for this library too.

### keepAlive (long shifts)

`keepAlive(true)` refreshes at `min(idleExpiresAt - keepAliveLead, idle midpoint)` (lead defaults to 2 minutes) and
re-plans after every refresh. It uses a coroutine, not an Android alarm: the app's foreground service keeps the
process alive. Turn it on only while a trip/tracking is active. It stops when the session ends and resumes on the
next sign-in while still on. `ExpiryWarning` fires once per threshold per login (default 30 and 10 minutes before the
absolute expiry, `FieldConfig.expiryWarnings`) so the app can ask for a new sign-in at a stop.

### Unlock policy

Tokens are stored under a Keystore AES-GCM key without user authentication (StrongBox first, then TEE), so refresh
works in the background and after a reboot. A biometric is an app unlock, not a token gate. With
`UnlockPolicy(afterColdStart, afterBackground)` the state becomes `Locked(session)` when due, for enrolled PERSONAL,
non-impersonated sessions only. `Locked` never blocks `headersFor`, the interceptor or keepAlive. The UI calls
`unlock(activity, "Unlock")` (BiometricPrompt, BIOMETRIC_STRONG or DEVICE_CREDENTIAL on API 30+; WEAK or credential
below). "Background" means no started activity (lifecycle callbacks are registered by `get`); hosts can also call
`noteBackgrounded()`/`noteForegrounded()`.

## The client of the session

The session says which client the worker belongs to, so an application that addresses its own API per client
(`/api/clients/{client}/...`) needs no "who am I" route to build its addresses:

```kotlin
val session = (field.state.value as FieldState.SignedIn).session
session.clientIdentifier // "acme": the client's identifier (the access token's `client_identifier` claim)
session.clientId         // the platform's id for the client (the `client_id` claim)
```

- Both are read from the access token's claims at every sign-in, refresh, act-as and sign-in code (so an
  impersonated session has the worker's client), and stored with the session, so a session restored after the
  app restarts has them (also while it is `Locked`). The sign-in answer carries no client; the platform's wire
  shape is unchanged.
- Both are `String?`: `null` when the token does not carry the claim (or is not a JWT), the same as `deviceId`
  on an unenrolled device. Every platform release since the field token's first release (`fcdev` 0.12.5) mints
  both, so `null` is met only on a session stored by a version of this library before it read them (until the
  next refresh) or on a token from something other than the platform. Do not guess a client: when
  `clientIdentifier` is `null`, wait for the refresh or ask the worker to sign in again. They are the last two
  members of `Session` and default to `null`, so code that builds a `Session` still compiles.
- **The identifier on the phone is for building addresses and showing a name.** It is read from a token the phone
  cannot verify and is not proof of anything. Your server must take the client from the verified token (the
  framework's `fw.Context`, or the SDK verifier's claims), never from the path or body the phone sends: check that
  the `{client}` in the address is the token's client, and refuse it otherwise.

## The worker's own photo

`session.worker.photoVersion` (null when there is none, or photos are off) and `session.worker.canSetOwnPhoto` (true only when
the client lets workers set their own and the session is the worker's own) are kept with the session across restarts. A
photo changed or removed elsewhere shows as a different `photoVersion` at the next sign-in or refresh, each time with a
`FieldEvent.SessionChanged`; no separate event.

```kotlin
val photo = session.getPhoto(PhotoSize.THUMB, haveVersion = held)   // null: no photo; photo.notModified: keep yours
if (state.worker.canSetOwnPhoto) {
    session.setPhoto(bytes, "image/jpeg")   // JPEG, PNG or WebP; the platform decides from the bytes
    session.removePhoto()
}
```

Each call is signed like the others (nonce, single refresher). Refusals are `FieldAuthException` codes: `OWN_PHOTO_NOT_ALLOWED`,
`PHOTOS_DISABLED`, `PHOTOS_DISABLED_FOR_CLIENT`, `IMPERSONATING` (reading is allowed while impersonating, changing is not),
`PHOTO_EMPTY`, `PHOTO_TOO_LARGE`, `PHOTO_UNSUPPORTED_TYPE`, `PHOTO_TYPE_MISMATCH`, `PHOTO_DIMENSIONS_TOO_LARGE`,
`PHOTO_UNREADABLE`, `PHOTO_BUSY`, and `TOO_MANY_REQUESTS` with `retryAfterSeconds`.

## Unlock keys: sign in again with a fingerprint or face

An unlock key (`docs/field-unlock-keys-spec.md`) lets a worker whose session has ended (the shift limit, the idle
limit, a sign-out) sign in again on their own phone with a fingerprint or face instead of the PIN. The phone holds an
ECDSA P-256 key in the Keystore (StrongBox first, then the TEE) that signs only after a strong (Class 3) biometric
match and is invalidated when a biometric is enrolled; the platform holds its public half, sends a fresh challenge,
verifies the signature and issues a new session (`amr` `["hwk","uv"]`). The PIN is never stored. Android 11 (API 30)
and later; enrolled PERSONAL devices only; never for a shared phone, an unenrolled key, an impersonated session or a
session that itself came from an unlock key. The client's policy must have `unlockKeysEnabled`.

```kotlin
// Right after a PIN sign-in on this phone (the platform accepts a key for 10 minutes after it):
when (session.unlockKeyAvailability(askPlatform = true)) {          // askPlatform: is the client's policy on? (no prompt)
    UnlockKeyAvailability.AVAILABLE -> if (workerSaysYes()) {
        session.enableUnlockKey(activity, UnlockKeyPrompt(title = "Use your fingerprint to sign in", cancel = "Not now"))
    }
    else -> {}                                                        // ENABLED, PIN_SIGN_IN_REQUIRED, POLICY_OFF, ...
}

// On the sign-in screen, when session.unlockKey() names a worker ("Sign in as Ann"):
try {
    session.loginWithUnlockKey(activity, UnlockKeyPrompt(title = "Sign in", cancel = "Use PIN"))
} catch (e: FieldAuthException) {
    showPinPad(reason = e.code)                                       // every failure means "sign in with the PIN"
}

session.disableUnlockKey()                                            // switch it off on this phone (no session needed)
```

| Call | What it does |
|---|---|
| `suspend fun unlockKeyAvailability(askPlatform: Boolean = false): UnlockKeyAvailability` | Whether to offer it now. No network unless `askPlatform`. |
| `suspend fun unlockKey(): UnlockKeyInfo?` | The key on this phone (worker id, code, name, `pinRequiredBy`), whoever's it is. No network. |
| `suspend fun enableUnlockKey(activity: FragmentActivity, prompt: UnlockKeyPrompt): UnlockKeyInfo` | Makes the key, signs the key proof (one prompt), registers it. Replaces the worker's own earlier key. |
| `suspend fun loginWithUnlockKey(activity: FragmentActivity, prompt: UnlockKeyPrompt): Session` | Challenge, then prompt, then a new session exactly as `login` gives. |
| `suspend fun disableUnlockKey()` | Deletes the key and revokes it on the platform with the device key alone. |

`FieldEvent.UnlockKeyChanged(key)` fires when a key is registered (`key`) or forgotten (`null`). `Session.amr` says
how the family was signed in.

**Availability** (`UnlockKeyAvailability`): `AVAILABLE` (offer it), `ENABLED` (this worker has one here),
`NOT_ENROLLED`, `SHARED_DEVICE`, `NOT_SIGNED_IN`, `IMPERSONATING`, `PIN_SIGN_IN_REQUIRED` (not a PIN sign-in of this
worker on this phone in the last 10 minutes, or a PIN change pending), `OTHER_WORKER` (one key per phone: another
worker's is here), `OS_TOO_OLD`, `NO_SECURE_HARDWARE`, `BIOMETRIC_NOT_ENROLLED`, `BIOMETRIC_UNAVAILABLE`, `POLICY_OFF`,
`ATTESTATION_REQUIRED`, `UNSUPPORTED`. The sign-in answer does not say whether the client's policy allows unlock keys,
so `askPlatform = true` sends an empty registration, which the platform refuses before reading any key (`403
UNLOCK_KEYS_DISABLED` when off, `400 INVALID_REQUEST` when on; spec 2.1, rules 1 to 7). `enableUnlockKey` asks the
same way before it makes a key, so a client with the policy off never sees a prompt. A `POLICY_OFF` answer is
remembered for a day and an `ATTESTATION_REQUIRED` one for a week (each try of the latter costs a prompt), per client.

**Enabling.** The key is made with `PURPOSE_SIGN`, `DIGEST_SHA256`, `setUserAuthenticationRequired(true)`,
`setUserAuthenticationParameters(0, AUTH_BIOMETRIC_STRONG)`, `setInvalidatedByBiometricEnrollment(true)` and
`setAttestationChallenge(SHA-256("fc-unlock-key-attestation:v1\n" + sid + "\n" + workerId + "\n" + deviceId))`
from the access token's claims; its chain is sent when the Keystore gives one (a key the TEE cannot attest is sent
without, which the platform refuses only under `unlockKeyAttestation: REQUIRED`). A key that is not in secure hardware
is deleted before any prompt (`UNLOCK_KEYS_UNSUPPORTED`). The key proof signs the registration's own DPoP nonce: the
library takes the nonce, shows the prompt without holding the platform lock, then sends. A nonce that expired during a
long prompt costs one more prompt, once; `409 UNLOCK_KEY_EXISTS` makes a new key, once. Nothing is kept on failure.

**Signing in.** The library asks for the challenge first, so a refusal answers without a prompt; a key the phone
invalidated is found before even that (`Signature.initSign` fails). Then `BiometricPrompt` (strong biometrics only, a
`CryptoObject` over the signature; the prompt's button is your `cancel` text), the assertion, and `/field/auth/unlock`.

| Code | What happened | The key |
|---|---|---|
| `NO_UNLOCK_KEY` | none on this phone | |
| `UNLOCK_KEY_INVALIDATED` | the phone's biometrics changed (or the keystore lost it) | revoked and deleted |
| `UNLOCK_KEY_INVALID` | unknown or revoked on the platform (a PIN reset, suspension, admin, ...) | revoked and deleted |
| `UNLOCK_KEYS_DISABLED` | the client switched unlock keys off | deleted (the platform revoked it) |
| `SHARED_DEVICE`, `UNKNOWN_DEVICE`, `DEVICE_REVOKED` | the device can no longer hold one | deleted |
| `INVALID_UNLOCK_PROOF` | the platform refused the signature (counts as a wrong PIN) | kept once; revoked and deleted when it repeats |
| `PIN_REAUTH_REQUIRED` | the policy's periodic PIN is due, or a PIN change is pending | kept: works again after the next PIN sign-in |
| `ATTESTATION_STATUS_UNAVAILABLE` (503, `retryAfterSeconds`) | the platform cannot check attestation now | kept: try it again later |
| `PIN_RESET_REQUIRED`, `NO_APP_ACCESS`, `DEVICE_SUSPENDED`, `NETWORK` | as for a PIN sign-in | kept |
| `WORKER_LOCKED`, `WORKER_LOCKED_TEMPORARILY`, `TOO_MANY_ATTEMPTS` | locked out (`FieldEvent.WorkerLocked` too) | kept |
| `CHALLENGE_EXPIRED` | the prompt took over 5 minutes twice (the first time a new challenge is asked for and the prompt shown again) | kept |
| `BIOMETRIC_CANCELLED`, `BIOMETRIC_LOCKOUT`, `BIOMETRIC_NOT_ENROLLED`, `BIOMETRIC_UNAVAILABLE` | the prompt | kept |

The enabling refusals are the availability codes (`NOT_ENROLLED`, `SHARED_DEVICE`, `SESSION_EXPIRED`,
`IMPERSONATING`, `PIN_SIGN_IN_REQUIRED`, `UNLOCK_KEY_OTHER_WORKER`, `UNLOCK_KEYS_UNSUPPORTED`, `BIOMETRIC_*`) and the
platform's (`UNLOCK_KEYS_DISABLED`, `ATTESTATION_REQUIRED`, `ATTESTATION_STATUS_UNAVAILABLE`, `UNLOCK_KEY_OTHER_WORKER`,
`WORKER_UNAVAILABLE`, `INVALID_UNLOCK_PROOF`, `WORKER_LOCKED`, ...).

**Disabling and revocations.** `disableUnlockKey()` needs only the device (it works signed out) and keeps the current
session. When the platform cannot be reached the key is deleted at once and its revocation is sent with the next
platform call (a sign-in, an unlock, enabling, `askPlatform`), so a live key nobody can use does not keep another
worker off the phone. A phone changing hands: the holder disables the key (`OTHER_WORKER` until then), then the new
worker signs in with their PIN and enables theirs. "Forget me" on a phone is `disableUnlockKey()` then `logout()`.
A new enrolment forgets the key of the old device row.

**State.** `fc.field.unlockKey` (`{unlockKeyId, workerId, workerCode, workerName, deviceId, createdAt, pinRequiredBy,
attestationVerdict, alias}`), `fc.field.unlockKey.revoke` (ids still to revoke), `fc.field.unlockKey.refusal` and
`fc.field.pinSignIn` (`{sid, workerId, at}`: the family last signed in with the PIN here) are kept in the library's
storage like the device record and survive a restart and a sign-out. None is secret; nothing about the key, the
challenge or a signature is logged.

**The app unlock is different.** `unlock(activity, prompt)` (`UnlockPolicy`) gates the screens of a session that is
still live (it shows a prompt and touches no token); an unlock key starts a new session after the old one ended and
the platform verifies it. They can be used together: a phone that locks its screens after a break with `unlock`, and
signs the worker in with `loginWithUnlockKey` at the start of the next shift.

Not run on a phone by these tests: the Keystore key, the attestation chain and the prompt
(`AndroidUnlockKeyProvider`, `BiometricUnlockKeyPrompter`). The JVM tests check the signer byte for byte against
`api/field-fixtures/unlock.json` and every flow above against a fake platform that verifies the key proof and the
assertion.

## Build and test

JDK 17 and the Android SDK (`local.properties` `sdk.dir=` or `ANDROID_HOME`):

```
./gradlew assembleRelease testDebugUnitTest
```

Tests are JVM unit tests (JUnit, MockWebServer, kotlinx-coroutines-test) with a software EC key and an in-memory
storage; the Keystore classes (`AndroidKeystoreStorage`, `AndroidKeyProvider`, `BiometricUnlocker`) are not covered
by them (instrumented tests are not part of this module yet).
