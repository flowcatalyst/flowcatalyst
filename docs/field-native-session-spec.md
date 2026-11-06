# Field sessions in native code: Kotlin session owner, React Native bridge, framework resolver

Status: SPEC, 2026-11-06. Answers `mono-repo/apps/fulfil-go/docs/field-login-requirements.md`
(R1–R10). Builds on `docs/field-worker-identity-design.md`, `docs/field-mobile-libraries-spec.md`
and `docs/field-worker-sign-in-options-spec.md`; where they disagree, this spec wins. Rulings are
the main session's, 2026-11-06, and the owner can override them.

## 0. Rulings on R1–R10

| Req | Ruling |
|---|---|
| R1 Kotlin library | **Accepted.** New `clients/field-android` (AAR, no React Native dependency): device key, proofs, tokens, refresh, nonce cache, OkHttp interceptor, `headersFor`. No UI; usable from a `Service`; never needs the main thread. |
| R2 One session owner | **Accepted.** On Android the Kotlin `FieldSession` (one per process) owns keys, tokens, refresh and nonces. `@flowcatalyst/field-react-native` becomes a thin bridge to it for JS screens. field-js stays the implementation for browsers and WebViews. The tracking service MUST run in the app's process (no `android:process`), so JS and the service share the one owner. Swift follows the same shape later. |
| R3 Background refresh never prompts | **Accepted, with a change to the design's local-unlock rule (§3.4).** On the native path the refresh token is stored under a Keystore AES key **without** user authentication (StrongBox when available, else TEE), so the owner can refresh in the background and after a reboot. Biometrics become an **app unlock** (`FieldSession.unlock(activity)` / JS `unlock()`), gating the screens after a cold start or a configurable time in the background, not the token. Reason: the token is already useless off the device (DPoP key, non-exportable); what a biometric adds is "someone picked up the unlocked phone", which is a screen concern. field-js (browsers/WebViews) keeps today's behaviour. Still PERSONAL-enrolled only, never for impersonated sessions. |
| R4 Shift-long sessions | **(a) keepAlive.** `FieldSession.keepAlive(true)` makes the owner refresh before the idle deadline (at `idleExpiresAt − 2 min`, or half the idle window if shorter) for as long as it is on. No server change: idle still ends sessions nobody declared busy. The app turns it on only while a trip/tracking is active. Plus an `ExpiryWarning` event at 30 and 10 minutes before the absolute expiry (configurable), so the app can ask for a new sign-in at a stop. (b) and (c) rejected: a heartbeat or idle override would weaken idle for every request rather than for the declared-busy period. |
| R5 Proofs for other code | **Accepted.** `headersFor(method, url)` refreshes first if needed and uses the origin's cached nonce; `noteNonce(url, nonce)` feeds back a `DPoP-Nonce` seen elsewhere; `headersForRetry(method, url, nonce)` signs again after a `401 use_dpop_nonce`. **Offline queue rule confirmed:** store requests unsigned and sign when sent. A stored proof would fail anyway: platform `/field/*` nonces are single-use; framework application nonces are valid for the current and previous 5-minute bucket; `jti` is remembered per device key for 11 minutes; `iat` may be ±24 h on both (device clocks), so freshness comes from the nonce; access tokens last 10 minutes. |
| R6 RN + Expo | **Accepted.** The RN package becomes a New-Architecture TurboModule (codegen spec) over field-android on Android; it does not use field-js. An Expo config plugin (`app.plugin.js`) sets `USE_BIOMETRIC`, `minSdkVersion` (26), the Maven repository for the AAR, and `NSFaceIDUsageDescription`. iOS keeps its current key/store implementation behind the same TurboModule interface until the Swift session owner is scheduled; on iOS the JS API runs field-js in JS with the native key/store, as today. |
| R7 Revocation | **Confirmed.** Suspending a worker revokes all their sessions in the same transaction (`WORKER_SUSPENDED`), so the next refresh is `401 INVALID_GRANT`. Revoking or suspending a device revokes its sessions, and every `/field/*` call from its key gets `403 DEVICE_REVOKED`/`DEVICE_SUSPENDED`. Already-issued access tokens stay valid at application backends until they expire (≤ 10 min) unless the app calls introspection or handles `OnFieldRevocation`. The owner maps `INVALID_GRANT`, `SESSION_EXPIRED`, `DEVICE_REVOKED`, `DEVICE_SUSPENDED` and an `invalid_token` that survives a refresh to `SessionEnded(reason)`. With uploads every 30 s, the refresh that fails comes within one access-token lifetime. |
| R8 App-held roles | **Accepted.** Framework Go: `auth.FieldPermissionResolver func(ctx context.Context, p FieldPrincipal) ([]string, error)` beside the existing mapper (option `auth.FieldPermissionsFrom(resolver)`); an error answers `503` (the `Unavailable` result), never "no permissions". Java mirrors it (`FieldPermissionResolver`, checked exception → 503). Contract §15.5 amended. |
| R9 Both planes | **Confirmed:** an application may grant any of its own permissions to a field worker inside the app; field tokens never carry platform authority. **Optional link (owner approved 2026-11-06):** an admin may link a field identity to one platform user, for audit correlation only ("this dispatcher on the web is that driver on the phone"). It grants nothing on either side. §5. |
| R10 Act-as audit | Covered by the work merged 2026-11-06: framework `operatorId`/`operatorType`, contract §15 and the guides updated for optional `device_id`; go-sdk released with it. |

## 1. `clients/field-android` (`io.flowcatalyst:field-android`)

Kotlin, Android library (AAR), minSdk 26, compileSdk 35, no React Native dependency.
Dependencies: OkHttp 4, kotlinx-coroutines, androidx.biometric (for `unlock` only). Package
`io.flowcatalyst.field`. Move `DeviceKeyManager`, `DerSignature` and the Keystore parts of
`SecureStoreManager` here from field-react-native (that package then depends on this one).

### 1.1 API (freeze this; the RN bridge builds against it)

```kotlin
class FieldSession private constructor(...) {
  companion object {
    /** The process-wide owner. First call configures it; later calls must pass equal config. */
    fun get(context: Context, config: FieldConfig): FieldSession
  }
  val state: StateFlow<FieldState>            // SignedOut | SignedIn(session) | Locked(session)
  fun addListener(l: FieldListener): Closeable // for Java/RN callers; events below

  suspend fun enrol(qrOrCode: String, info: DeviceInfo): DeviceRecord
  suspend fun login(creds: LoginCredentials): Session   // workerCode|badgeToken, pin?, clientIdentifier?
  suspend fun redeemSignInCode(code: String): Session
  suspend fun actAs(target: ActAsTarget): Session
  suspend fun endImpersonation(): Session?
  suspend fun changePin(current: String, new: String)
  suspend fun logout(scope: LogoutScope = LogoutScope.SESSION)
  suspend fun unlock(activity: FragmentActivity, prompt: String): Session  // R3 app unlock
  fun keepAlive(on: Boolean)                                                // R4

  /** Single-flight: concurrent callers share one refresh. */
  suspend fun refresh(): Session
  suspend fun headersFor(method: String, url: String): Map<String, String>  // Authorization + DPoP
  suspend fun headersForRetry(method: String, url: String, nonce: String): Map<String, String>
  fun noteNonce(url: String, nonce: String)
  fun interceptor(): okhttp3.Interceptor
  /** Blocking forms for Java and non-coroutine callers (never on the main thread). */
  fun headersForBlocking(method: String, url: String): Map<String, String>
}
```

`FieldConfig(platformUrl, appCode, clientIdentifier?, requireUnlock: UnlockPolicy, expiryWarnings:
List<Duration> = [30.min, 10.min], keepAliveLead: Duration = 2.min)`. `Session` mirrors field-js
(`worker`, `actor`, `test`, `deviceMode`, `deviceId`, `appCode`, `expiresAt`, `idleExpiresAt`,
`refreshExpiresAt`, `pinChangeRequired`). Events: `SessionChanged`, `SessionEnded(reason)`,
`PinChangeRequired`, `ExpiryWarning(remaining)`, `Locked`, `WorkerLocked`. Errors:
`FieldAuthException(code, message)` with the same codes as field-js.

### 1.2 Behaviour

- Wire behaviour identical to field-js (`auth.ts`), including: unenrolled login, badges, sign-in
  codes, act-as with the supervisor kept aside, the platform nonce on `/field/*` (retry once on
  `use_dpop_nonce`), refresh retry grace semantics. Port, don't reinvent: read field-js and its
  tests first.
- **Single refresher:** a `Mutex` around refresh; the refresh token is read from and written to the
  store inside it; every path (interceptor, `headersFor`, keepAlive timer, JS) goes through
  `refresh()`. Refresh when the access token expires within 30 s.
- **Storage:** device record, session metadata, refresh token and the kept-aside supervisor in
  `EncryptedSharedPreferences`-style storage under a Keystore AES-GCM key with no user
  authentication (StrongBox first). Survives reboot. Cleared on logout/session end.
- **Nonce cache:** per origin (`scheme://host:port`), from every `DPoP-Nonce` response header the
  owner sees, plus `noteNonce`. Platform `/field/*` nonces are single-use: drop after use.
- **Interceptor:** adds headers; on `401` with `WWW-Authenticate: DPoP error="use_dpop_nonce"`
  stores the nonce and retries once; on `401 invalid_token` refreshes once and retries once; a
  refresh that fails with a session-ending code emits `SessionEnded` and the request fails with
  `FieldAuthException`. Never on the main thread (interceptors run on OkHttp's threads).
- **keepAlive:** a coroutine on a background scope schedules `refresh()` at
  `min(idleExpiresAt − keepAliveLead, idle midpoint)`; reschedules after each refresh; stops on
  `keepAlive(false)` or session end. Uses no Android alarm: the app's foreground service keeps the
  process alive. Document that.
- **Expiry warnings:** fire once per threshold per session family.
- **Unlock (R3):** `UnlockPolicy(afterColdStart: Boolean, afterBackground: Duration?)`. When due,
  `state` is `Locked`; `headersFor`/interceptor still work (the service is not blocked); only the
  JS/UI checks `Locked` and calls `unlock(activity)`, which shows `BiometricPrompt`
  (`BIOMETRIC_STRONG or DEVICE_CREDENTIAL`). PERSONAL enrolled, non-impersonated sessions only;
  other sessions never lock.

### 1.3 Tests

JVM unit tests with a fake platform (MockWebServer) for every wire flow and: single-flight
refresh under concurrency (two coroutines + the interceptor → one refresh request); nonce retry;
session-ended mapping; keepAlive scheduling with a test clock; expiry warnings; supervisor
restore; storage round-trip through an in-memory store implementation (the Keystore parts behind
an interface; instrumented tests are optional and not run here). Mutation checks on the
single-flight lock, the nonce retry and the session-ended mapping.

### 1.4 Build and publishing

Gradle wrapper inside `clients/field-android`. Publishing: `maven-publish` to GitHub Packages
(`io.flowcatalyst:field-android`), with a `make release-field-android BUMP=…` line in the style of
the other release lines (tag `field-android/vX.Y.Z`, workflow publishes). Until released, consumers
can use a Gradle composite build (`includeBuild`) of the directory.

## 2. `@flowcatalyst/field-react-native` (rework)

- TurboModule (`NativeFcField` codegen spec in `src/NativeFcField.ts`), New Architecture only.
- Android: delegates everything to `FieldSession` (enrol, login, redeemSignInCode, actAs,
  endImpersonation, changePin, logout, unlock, keepAlive, refresh, headersFor, headersForRetry,
  noteNonce, session state, events via `RCTDeviceEventEmitter` / codegen events).
- JS API: a `FieldClient` class with the field-js `FieldAuth` method names and `Session` shape plus
  `headersFor`, `fetch(url, init)` (built on `headersFor` + nonce retry), `keepAlive`, event
  listeners (plain `on(event, cb)`; no `EventTarget`).
- iOS: unchanged native key/store behind the same module; `FieldClient` on iOS runs field-js core
  with the native key provider and store (as today). Note for later: Swift session owner.
- The WebView bridge stays (protocol v1) for WebView pages.
- Expo config plugin `app.plugin.js`: `USE_BIOMETRIC` permission, `minSdkVersion` ≥ 26, the Maven
  repository (GitHub Packages) and dependency for field-android, `NSFaceIDUsageDescription`
  (configurable text). Tests for the plugin's mods with `@expo/config-plugins` test helpers.

## 3. Framework: permission resolver (R8)

Go `auth.FieldPermissionResolver` + option; Java `FieldPermissionResolver`; error → 503 via the
existing `Unavailable` result; contract §15.5; tests incl. error → 503 and a ctx value reaching
the resolver.

## 4. Not in this spec

Swift session owner; passkeys; offline login; device testing (the owner runs the §4 acceptance script of the requirements
on a real device).

## 5. Linking a field identity to a platform user (R9)

Audit correlation only. The link grants nothing: a field token stays a field token, and the
platform user gains nothing on field routes.

- Schema (next free migration): `field_identities.linked_principal_id VARCHAR(17) NULL`;
  `UNIQUE (client_id, linked_principal_id)` (a person has at most one field identity per client;
  one platform user may be linked in several clients).
- Admin API: `PUT /api/field-workers/{id}/link {principalId}` and `DELETE
  /api/field-workers/{id}/link`, permission `platform:iam:field-worker:manage`, client-boxed like
  the other worker routes. The principal must exist, be a USER (not a service account), be ACTIVE
  and be able to access the worker's client (anchor, the client itself, or a partner/granted
  client); otherwise 422 `INVALID_LINK`. Linking an already-linked principal in the same client →
  409 `LINK_EXISTS`. `GET /api/field-workers/{id}` and the list carry `linkedPrincipal: {id, name,
  email} | null`; the list accepts `linkedPrincipalId` as a filter.
- Events `platform:iam:field-worker:linked` / `:unlinked` `{workerId, clientId, principalId}`,
  audited as the calling admin.
- Token: claim `linked_principal_id` when set (omitted otherwise), copied at login and refresh
  (a refresh picks up a changed link). Introspection returns it. Fixture case `valid_linked`
  with the principal field.
- SDK verifiers (Go, TS, Laravel, Java) and frameworks (Go, Java):
  `FieldPrincipal.LinkedPrincipalID` (empty/null when absent).
- Console: worker drawer "Linked platform user" with link (search users who can access the
  client) and unlink.
- Deleting a principal does not touch the link (no foreign key, as elsewhere in the module); the
  console shows an unresolvable id as such.
