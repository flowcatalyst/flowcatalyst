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

## Build and test

JDK 17 and the Android SDK (`local.properties` `sdk.dir=` or `ANDROID_HOME`):

```
./gradlew assembleRelease testDebugUnitTest
```

Tests are JVM unit tests (JUnit, MockWebServer, kotlinx-coroutines-test) with a software EC key and an in-memory
storage; the Keystore classes (`AndroidKeystoreStorage`, `AndroidKeyProvider`, `BiometricUnlocker`) are not covered
by them (instrumented tests are not part of this module yet).
