# field-android (`io.flowcatalyst:field-android`)

Kotlin library (AAR) that owns a field worker's FlowCatalyst session on Android: device key, DPoP proofs,
tokens, refresh, nonces, an OkHttp interceptor. No UI, no React Native dependency, never needs the main thread.
Spec: `docs/field-native-session-spec.md` section 1. Wire behaviour is a port of `clients/field-js/src/auth.ts`.

minSdk 26, compileSdk 35. Dependencies: OkHttp 4, kotlinx-coroutines, androidx.biometric (unlock only).

## Setup

Until released, use a composite build: in the app's `settings.gradle`
`includeBuild("../path/to/clients/field-android")`, then `implementation("io.flowcatalyst:field-android")`.
Released builds come from GitHub Packages (`https://maven.pkg.github.com/flowcatalyst/flowcatalyst`); the version is the
`VERSION` file. Publishing is configured (`./gradlew publishReleasePublicationToGitHubPackagesRepository`, needs
`GITHUB_ACTOR`/`GITHUB_TOKEN`) but is not run from this repo yet.

The library manifest declares `INTERNET` and `USE_BIOMETRIC`.

## Use

```kotlin
val session = FieldSession.get(applicationContext, FieldConfig(
    platformUrl = "https://platform.example.com",
    appCode = "fulfil",
    clientIdentifier = "acme",                       // needed for worker-code sign-in on an unenrolled device
    requireUnlock = UnlockPolicy(afterColdStart = true, afterBackground = 5.minutes),
))
session.restore()                                    // optional: waits for the stored session to load
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

## Build and test

JDK 17 and the Android SDK (`local.properties` `sdk.dir=` or `ANDROID_HOME`):

```
./gradlew assembleRelease testDebugUnitTest
```

Tests are JVM unit tests (JUnit, MockWebServer, kotlinx-coroutines-test) with a software EC key and an in-memory
storage; the Keystore classes (`AndroidKeystoreStorage`, `AndroidKeyProvider`, `BiometricUnlocker`) are not covered
by them (instrumented tests are not part of this module yet).
