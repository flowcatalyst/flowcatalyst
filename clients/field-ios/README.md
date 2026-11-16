# field-ios (`FlowCatalystField`)

Swift package that owns a field worker's FlowCatalyst session on iOS: device key (Secure Enclave), DPoP proofs, tokens,
refresh, nonces, request signing with retry. No UI. It is the iOS counterpart of `clients/field-android` and speaks to
the platform in exactly the same way: the same requests, DPoP proofs, state machine, timers and error codes (the Kotlin
tests define the wire behaviour; they are ported one for one, see "Tests"). The wire behaviour is a port of
`clients/field-js/src/auth.ts`; the design is `docs/field-worker-identity-design.md`.

iOS 15+, macOS 12+ (macOS only so the package builds and its Apple code compiles on a development machine). Swift tools
5.9. One dependency: `apple/swift-crypto`, used only off Apple platforms (Linux CI); on Apple platforms CryptoKit.

Two library targets:

| Target | Contents | Platforms |
|---|---|---|
| `FlowCatalystFieldCore` | Everything that is not Apple-specific: `FieldSession` (the state machine), DPoP proofs, DER signature conversion, models, errors, timers, request authorisation and retry. Written against protocols (`SecureStorage`, `DeviceKeyProvider`, `HTTPTransport`, `FieldClock`, `UnlockPrompter`). Foundation only (plus CryptoKit or swift-crypto for SHA-256). | iOS, macOS, Linux |
| `FlowCatalystField` | The real implementations: `SecureEnclaveKeyProvider`, `KeychainStorage`, `LocalAuthenticationUnlocker`, `URLSessionTransport`, `FieldSession.get(config:)`, the `URLRequest` helpers. Re-exports the core: an app only does `import FlowCatalystField`. | iOS, macOS |

## Setup

Swift Package Manager. Each release is published to `github.com/flowcatalyst/field-ios` (this package at the root of
its own repository, one commit and one `vX.Y.Z` tag per release), and an app depends on a version:

```swift
.package(url: "https://github.com/flowcatalyst/field-ios.git", from: "0.2.0"),
// target dependency:
.product(name: "FlowCatalystField", package: "field-ios")
```

In Xcode: File > Add Package Dependencies, and enter `https://github.com/flowcatalyst/field-ios`.

To work against a checkout of the source instead, use `.package(path: "../path/to/clients/field-ios")` (in Xcode:
Add Package Dependencies > Add Local...).

`Info.plist` keys the app needs:

| Key | When |
|---|---|
| `NSFaceIDUsageDescription` | Always, if you ever call `unlock` (Face ID). Face ID needs it (what iOS does without it was not checked here). Touch ID and the passcode need no key. |

The Secure Enclave key and the Keychain items need no entitlement and no capability. They use the app's default
Keychain access group (the first group in the app's entitlements, else the app id), the same group the React Native
module uses; a host that sets a custom `keychain-access-groups` must keep both on the same one.

## Use

```swift
import FlowCatalystField

let config = FieldConfig(
    platformUrl: "https://platform.example.com",
    appCode: "my-app",
    clientIdentifier: "acme",                                              // worker-code sign-in on an unenrolled device
    requireUnlock: UnlockPolicy(afterColdStart: true, afterBackground: 5 * 60)
)
let session = try FieldSession.get(config: config)       // process-wide owner; same config every time
_ = try await session.restore()                          // optional: waits for the stored session to load

// 1. enrol this device (QR payload or the 10-character code)
let device = try await session.enrol("fcfield:1:https://platform.example.com:ABCDE12345", info: .current)

// 2. sign in (or `badgeToken:`), then watch the state
let me = try await session.login(LoginCredentials(workerCode: "W001", pin: "1234"))
for await state in session.stateUpdates() { /* .signedOut, .signedIn(session), .locked(session) */ }
session.addListener { event in /* .sessionEnded(reason), .expiryWarning, .pinChangeRequired, ... */ }

// 3. an authorised request to your backend (signs, retries after a nonce challenge or a refresh)
var req = URLRequest(url: URL(string: "https://api.example.com/trips")!)
req.httpMethod = "POST"; req.httpBody = body
let (data, response) = try await session.perform(req)

// 4. sign out
try await session.logout()                               // .deviceAll: every session on this device
```

Other flows: `redeemSignInCode(_:)`, `actAs(_:)` / `endImpersonation()`, `changePin(current:new:)`, `refresh()`,
`keepAlive(_:)`, `unlock(prompt:)`.

`FieldSession` is an `actor`: call its methods with `await`. `state` and `addListener` are synchronous and usable from
anywhere; `stateUpdates()` is an `AsyncStream<FieldState>` (the current state, then every change). Listener callbacks
run synchronously where the event happens (on the session's executor): do not block in them and do not call the
session synchronously from one.

### Authorising other requests

```swift
let signed = try await session.authorize(urlRequest)                      // a URLRequest with Authorization + DPoP
let h = try await session.headersFor(method: "POST", url: url)            // [String: String] for any other client
// on 401 use_dpop_nonce: session.headersForRetry(method:url:nonce:) with the response's DPoP-Nonce header
// any DPoP-Nonce seen elsewhere: await session.noteNonce(url:nonce:)
```

`perform(_ request: URLRequest, session:)` is the interceptor: it signs, retries once on `401 use_dpop_nonce` (with the
new nonce) and once on `401 invalid_token` after a refresh (exactly one refresh request however many callers need one).
A refresh that ends the session (`INVALID_GRANT`, `SESSION_EXPIRED`, `DEVICE_REVOKED`, `DEVICE_SUSPENDED`,
`UNKNOWN_DEVICE`, `NO_APP_ACCESS`) emits `sessionEnded(code)` and throws `FieldAuthError`; an `invalid_token` that
survives a refresh emits `sessionEnded("invalid_token")` and the 401 is returned. A body given as `httpBodyStream` is
read into memory and the request is never retried (Kotlin: one-shot bodies). For uploads from a file use `authorize`.

Offline queues: store requests unsigned and sign when sending. Platform `/field/` nonces are single-use, application
nonces last about 5-10 minutes and `iat` may be off by +-24 h, so a stored proof is useless.

## API (Swift name, Kotlin name)

`FlowCatalystFieldCore` (all `public`):

| Swift | Kotlin |
|---|---|
| `actor FieldSession` | `FieldSession` |
| `init(config:storage:keys:transport:clock:unlockPrompter:)` | `FieldSession.create(config, Deps(...))` (internal in Kotlin) |
| `state: FieldState` (sync) / `stateUpdates() -> AsyncStream<FieldState>` | `state: StateFlow<FieldState>` |
| `addListener(_:) -> FieldListenerToken` (`cancel()`) | `addListener(FieldListener): Closeable` |
| `restore() async throws -> FieldState` | `restore()` |
| `enrolledDevice() throws -> DeviceRecord?` (stored record, no network; kept after logout) | none (Android bridge reports no device) |
| `enrol(_:info:) async throws -> DeviceRecord` | `enrol(qrOrCode, info)` |
| `login(_:)` / `redeemSignInCode(_:)` | `login(creds)` / `redeemSignInCode(code)` |
| `actAs(_:)` / `endImpersonation() -> Session?` | `actAs(target)` / `endImpersonation()` |
| `changePin(current:new:)` | `changePin(current, new)` |
| `logout(scope:)` | `logout(scope)` |
| `refresh()` | `refresh()` (single-flight) |
| `unlock(prompt:)` / `unlock(using:prompt:)` | `unlock(activity, prompt)` / `unlockWith(prompter, prompt)` |
| `noteBackgrounded()` / `noteForegrounded()` | `noteBackgrounded()` / `noteForegrounded()` |
| `keepAlive(_:)` | `keepAlive(on)` |
| `headersFor(method:url:)` / `headersForRetry(method:url:nonce:)` / `noteNonce(url:nonce:)` | same names |
| `perform(_ HTTPRequest, using:oneShot:)` | `FieldInterceptor.intercept` (`interceptor()`) |
| `FieldSession.normaliseBadgeToken(_:)` | `FieldSession.normaliseBadgeToken` |
| `FieldConfig`, `UnlockPolicy` (`.never`) | `FieldConfig`, `UnlockPolicy` (`NEVER`). Durations are `TimeInterval` seconds, not `Duration` |
| `DeviceInfo`, `DeviceRecord`, `DeviceMode` (`.personal/.shared/.unenrolled`) | same |
| `Session`, `Worker`, `Actor`, `LoginCredentials`, `ActAsTarget`, `LogoutScope` (`.session/.deviceAll`) | same (`DEVICE_ALL`) |
| `FieldState` (`.signedOut/.signedIn/.locked`) | `FieldState` (`SignedOut/SignedIn/Locked`) |
| `FieldEvent` (`.sessionChanged/.sessionEnded/.pinChangeRequired/.expiryWarning/.locked/.workerLocked`) | `FieldEvent` |
| `FieldAuthError` (`code, message, status, retryAfterSeconds, violations, isPinResetRequired`) | `FieldAuthException` |
| `FcFieldError` (`code, message`) | `FcFieldException` |
| `protocol SecureStorage`, `InMemoryStorage` | `SecureStorage`, `InMemoryStorage` |
| `protocol DeviceKeyProvider`, `DeviceKey` | `DeviceKeyProvider`, `DeviceKey` |
| `protocol UnlockPrompter` | `UnlockPrompter` |
| `protocol HTTPTransport`, `HTTPRequest`, `HTTPResponse` | the OkHttp client |
| `protocol FieldClock`, `SystemClock` | the `clock` lambda and coroutine `delay` |
| `DerSignature`, `Base64URL`, `UnlockErrorMapping` | `DerSignature` (internal), `Base64`, `BiometricUnlocker.mapError` |

`FlowCatalystField` (Apple):

| Swift | Kotlin |
|---|---|
| `FieldSession.get(config:)` (throws `CONFIG_MISMATCH` on a different config) | `FieldSession.get(context, config)` |
| `SecureEnclaveKeyProvider` | `AndroidKeyProvider` / `DeviceKeyManager` |
| `KeychainStorage` | `AndroidKeystoreStorage` |
| `LocalAuthenticationUnlocker` | `BiometricUnlocker` |
| `URLSessionTransport` | `defaultHttpClient()` |
| `FieldSession.authorize(_ URLRequest)`, `perform(_ URLRequest, session:)` | `interceptor()` |
| `DeviceInfo.current` | `DeviceInfo(model = Build.MODEL)` |

Not ported: `headersForBlocking` (no blocking form in Swift; use `await`).

## Errors

Failures are `FieldAuthError` (`code`, `message`, `status`, `retryAfterSeconds`, `violations`), with the field-js codes:
the platform's (`INVALID_CREDENTIALS`, `INVALID_GRANT`, `SESSION_EXPIRED`, `DEVICE_REVOKED`, `DEVICE_SUSPENDED`,
`UNKNOWN_DEVICE`, `NO_APP_ACCESS`, `WORKER_LOCKED`, `WORKER_LOCKED_TEMPORARILY`, `TOO_MANY_ATTEMPTS`, `PIN_POLICY`,
`PIN_RESET_REQUIRED`, `INVALID_ENROLMENT_CODE`, ...), the DPoP codes (`invalid_token`, `use_dpop_nonce`,
`invalid_dpop_proof`), a plain `HTTP_<status>` when the body has none, and the library's own:

| Code | Meaning |
|---|---|
| `NETWORK` | The platform could not be reached (the transport error is dropped on purpose: it may quote the request). |
| `NOT_ENROLLED` | No platform URL: configure `platformUrl` or enrol first. |
| `INVALID_REQUEST` | Bad arguments (both or neither of code and badge, no `clientIdentifier` on an unenrolled device, empty code, bad URL, `device-all` without enrolment). |
| `INVALID_ENROLMENT_CODE` | Not a `fcfield:1:` QR, or `http:` for a host that is not loopback or named in the config. |
| `PLATFORM_MISMATCH` | The QR names an origin that is not the configured one or in `allowedPlatformOrigins`. Nothing is persisted. |
| `IMPERSONATING` / `NOT_IMPERSONATING` | `actAs` or `changePin` while impersonating; `endImpersonation` when not. |
| `KEY_INVALIDATED` | The device key is gone or no longer usable: the platform will reject the old identity; re-enrol. |
| `BIOMETRIC_CANCELLED`, `BIOMETRIC_UNAVAILABLE`, `NO_SECURE_LOCK_SCREEN` | From `unlock` (see the mapping in `UnlockErrorMapping`). |
| `CONFIG_MISMATCH` | `FieldSession.get(config:)` called with a different config (Kotlin throws `IllegalStateException`; the React Native module already uses this code). |
| `INTERNAL` | Malformed token response, storage or signing failure, unexpected status. |

`PIN_RESET_REQUIRED` (HTTP 403, from `login` and `actAs`): the worker's PIN was set under an app key that has since been
removed. It is not a wrong PIN and retrying cannot help. Show "Ask your supervisor to reset your PIN".
`error.isPinResetRequired` tests for it. The library does not retry, does not emit `workerLocked`, and does not clear
the enrolment or end a session. Messages never contain a PIN or a refresh token.

## Shared with the React Native module

`clients/field-react-native/ios` keeps values for the WebView bridge in the same Keychain service and uses this
library's key provider. The session owner and the module therefore use the same key and the same stored items, the way `FieldSession` on Android shares the Keystore key
(alias `fc_field_device`) with the React Native module. The React Native module uses this library's `SecureEnclaveKeyProvider` for the same key.

| Item | Value |
|---|---|
| Device key | Permanent EC P-256 (`kSecAttrKeyTypeECSECPrimeRandom`, 256 bits), `kSecAttrApplicationTag` = UTF-8 `io.flowcatalyst.field.device`. On a device: `kSecAttrTokenIDSecureEnclave`, access control `kSecAttrAccessibleWhenUnlockedThisDeviceOnly` + `.privateKeyUsage`. In the simulator (no Secure Enclave): a software Keychain key with the same tag. Looked up by tag and key type. |
| Signature | `SecKeyCreateSignature` with `.ecdsaSignatureMessageX962SHA256` (DER), converted to raw r‖s by the same `DerSignature` code. |
| `DeviceKey.level` | `TEE` on a device, `SOFTWARE` in the simulator (as the module reports). No attestation chain. |
| Stored values | Generic passwords: `kSecAttrService` = `io.flowcatalyst.field.store`, `kSecAttrAccount` = the key, value = UTF-8, `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`, no access control. The biometric variant of the module's store (`biometryCurrentSet`) is never written here; its marker service `io.flowcatalyst.field.store.bio-marker` is honoured on read. |
| Store keys | `fc.field.device`, `fc.field.refresh`, `fc.field.session`, `fc.field.supervisor`, `fc.field.supervisor.refresh` (the field-js and Android names). |
| Stored JSON | The same shapes field-js and the Android library write (`DeviceRecord`, `Session`), so a session started by one is restored by another. |

So an app can switch between the module's WebView/field-js path and this library on the same device without
re-enrolling, as long as only one of them owns the refresh token at a time (tokens rotate; two owners fight and a reused
rotated token revokes the family).

The React Native package (`clients/field-react-native`) is built on this library. Its pod compiles a verbatim copy of
these sources (`Sources/FlowCatalystFieldCore` and `Sources/FlowCatalystField`, plus `VERSION`) together with the
bridge into one Swift module named `FlowcatalystFieldReactNative`; `scripts/sync-field-ios.mjs` in that package makes
the copy and a test checks it. That is why every `import FlowCatalystFieldCore` in the Apple target is wrapped in
`#if canImport(FlowCatalystFieldCore)`: the same bytes compile as two modules here and as one there. After changing
these sources, run `pnpm sync:field-ios` in `clients/field-react-native`. The bridge's `FcField` module holds the
process-wide `FieldSession.get(config:)` and forwards; the React Native key/store methods use `SecureEnclaveKeyProvider`
(one key per app). Native code in the same app (for example a background location module) reaches this library with
`import FlowcatalystFieldReactNative` and rebuilds the JS config with `FieldBridgeConfig.fieldConfig(from:)`.

## Behaviour (same as Android)

- **Enrolment**: `fcfield:1:<platformUrl>:<code>`; the code is the last segment. With a configured `platformUrl` a QR for
  another origin is refused (`PLATFORM_MISMATCH`) unless listed in `allowedPlatformOrigins`; `http:` only for
  `localhost`, `127.0.0.1`, `::1`, `10.0.2.2`, or an origin the config names. With no configured URL the first QR is
  adopted.
- **Platform calls** (`/field/...`) are serialised, the nonce is primed with `POST /field/auth/nonce` when none is
  cached, nonces on `/field/` routes are single-use, and one `use_dpop_nonce` is retried.
- **Proof**: `typ dpop+jwt`, `alg ES256`, `jwk` (kty, crv, x, y); claims `htm` (upper-case), `htu` (scheme, host, port
  unless default, path; no query, no fragment), `iat` (device seconds), `jti` (16 random bytes), `nonce` when known,
  `ath` = base64url SHA-256 of the access token when there is one.
- **Refresh** is single-flight and happens first when the access token expires within 30 s. Failure codes
  `INVALID_GRANT`, `SESSION_EXPIRED`, `DEVICE_REVOKED`, `DEVICE_SUSPENDED`, `UNKNOWN_DEVICE`, `NO_APP_ACCESS` end the
  session (storage cleared, `sessionEnded` then `sessionChanged(nil)`); others keep it. An impersonation that ends on
  its own hands back to the supervisor.
- **keepAlive** refreshes at `min(idleExpiresAt - keepAliveLead, idle midpoint)` and re-plans after each refresh;
  retries 30 s after a transient failure, never faster than 1 s. **Expiry warnings** fire once per threshold per
  absolute-expiry family (defaults 30 and 10 minutes).
- **Unlock policy**: `locked` applies to enrolled PERSONAL, non-impersonated sessions only; a cold start (restore) or
  `afterBackground` of time in the background locks. `locked` never blocks `headersFor`, `perform` or `keepAlive`.

## What differs from Android, and why

- **Concurrency**: `FieldSession` is an actor and uses `async`/`throws` and value types; explicit async mutexes replace
  Kotlin's `Mutex` because actors are reentrant at every `await`. No `headersForBlocking`.
- **No interceptor chain**: `URLSession` has none, so `perform(_:)` / `authorize(_:)` are functions you call.
- **No `Context`/`Activity`**: `FieldSession.get(config:)`. App visibility comes from `UIApplication` background and
  foreground notifications (iOS only); on macOS call `noteBackgrounded()` / `noteForegrounded()` yourself.
- **The process is suspended in the background.** Android keeps the process alive with a foreground service; iOS
  suspends it. `keepAlive` and the expiry-warning timers are in-process timers that only run while the process runs
  (foreground, or a background mode the app declares such as location). They resume on foreground. A refresh is also
  done lazily on the first use, so a session that outlived its idle window while suspended ends with a
  session-ending code from the platform on the next call, exactly as on Android after a process kill.
- **Key and storage accessibility is weaker for background work.** The key is created by `SecureEnclaveKeyProvider` (also when the React Native module is the caller) with
  `kSecAttrAccessibleWhenUnlockedThisDeviceOnly`, so it cannot sign while the phone is locked; Android's key can. A
  refresh or proof attempted while the device is locked fails with `INTERNAL` ("device key is unavailable while the
  device is locked"), not `KEY_INVALIDATED` (the React Native module used to map every signing failure to `KEY_INVALIDATED`,
  which would tell an app to re-enrol; it now uses this provider, so it behaves the same). Stored values use `AfterFirstUnlock`, so they are
  readable after the first unlock following a restart; before that, reads fail with `INTERNAL`, and `restore()` can be
  called again later. A tracking service that must run with the phone locked needs the key re-created with
  `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`, which this provider does not do today (the key cannot
  be changed in place; it would be a new key and a re-enrolment).
- **Key invalidation** is not deleted-and-recreated on a failure the way `DeviceKeyManager` does on Android. A missing
  key is `KEY_INVALIDATED`; the next `getOrCreate` after a restart creates a new key (a different public key, so the
  platform rejects the old identity and the app re-enrols), as on Android.
- **No attestation.** Android sends the Keystore attestation chain at enrolment; the Swift module has none (App
  Attest would be a different protocol and is not done). `attestationChain` is omitted (the React Native module sends none either).
- **Unlock** uses `LAPolicy.deviceOwnerAuthentication` (Face ID or Touch ID, then the passcode), the counterpart of
  `BIOMETRIC_STRONG or DEVICE_CREDENTIAL`. There is no strong/weak distinction on iOS. No Keychain item is gated on it.
- **Platform string**: `DeviceInfo.platform` defaults to `ios`; `DeviceInfo.current` fills `model` with the machine
  identifier (for example `iPhone15,2`) and no label (the device name is personal data).
- **First-run provisioning survives a restart.** With an empty `platformUrl`, the Kotlin library forgets the platform URL
  adopted from a QR when the process restarts (it only reads `config.platformUrl`); field-js restores it from the device
  record and this library does too. See the report.
- **Timeouts**: OkHttp's 15 s connect / 30 s read are a `URLSession` idle timeout of 30 s and a 120 s total.
- **Durations** in the config are seconds (`TimeInterval`); `Duration` needs iOS 16.

## Tests

`Tests/FlowCatalystFieldCoreTests` ports the Kotlin tests one for one against the core with fakes (an in-memory
platform and application server, a software P-256 key, an in-memory store, a virtual clock): `DpopProofTests`,
`WireFlowTests`, `RefreshAndPerformTests`, `UnlockTests`, `TimersTests`, plus `DerSignatureTests` (the vectors the React Native
module's DER tests used before it adopted this library). Each Swift test names the Kotlin test it ports.

```
# Linux (the core), no Xcode needed:
docker run --rm -v "$PWD/clients/field-ios":/work -w /work swift:6.2 swift test --scratch-path .build-linux
# macOS: compile both targets (with Xcode installed, `swift test` also runs the tests):
swift build --package-path clients/field-ios
# iOS: both library products for a device
xcodebuild build -scheme FlowCatalystField -destination 'generic/platform=iOS' CODE_SIGNING_ALLOWED=NO
```

`.github/workflows/field-ios.yml` does all three on a macOS runner and in a Linux container.

## Not verified on a device

Everything under `Sources/FlowCatalystField` that talks to the system was compiled (macOS) but never run on an iPhone,
and the core tests do not exercise it. Only a real device proves:

- Secure Enclave key creation, signing, and what happens to it after a passcode removal, a biometry change, a restore
  from backup onto another device, or an app reinstall (Keychain items normally survive an uninstall on iOS, so a reinstalled
  app may find a stored session and key; the key is `ThisDeviceOnly`).
- Sharing the key and the Keychain items with the React Native module in one app on a device (verified in the simulator only: see the React Native package's README).
- The Face ID, Touch ID and passcode prompts, their cancel/lockout/not-enrolled error codes, and the
  `NSFaceIDUsageDescription` behaviour.
- Keychain access after a restart, before and after the first unlock, and the behaviour of a refresh while the phone is
  locked (the `WhenUnlocked` key).
- The simulator branch (`#if targetEnvironment(simulator)`) and the iOS-only app lifecycle observer (`#if os(iOS)`);
  the latter was type-checked against a stub of the two `UIApplication` notification names, not the real UIKit.
- `URLSession` behaviour against a real platform (redirects, header casing, `Authorization` handling).
