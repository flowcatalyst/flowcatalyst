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
.package(url: "https://github.com/flowcatalyst/field-ios.git", from: "0.5.0"),
// target dependency:
.product(name: "FlowCatalystField", package: "field-ios")
```

In Xcode: File > Add Package Dependencies, and enter `https://github.com/flowcatalyst/field-ios`.

To work against a checkout of the source instead, use `.package(path: "../path/to/clients/field-ios")` (in Xcode:
Add Package Dependencies > Add Local...).

`Info.plist` keys the app needs:

| Key | When |
|---|---|
| `NSFaceIDUsageDescription` | Always, if you ever call `unlock` or use unlock keys (`enableUnlockKey`, `loginWithUnlockKey`) on a Face ID phone. Face ID needs it (what iOS does without it was not checked here). Touch ID and the passcode need no key. |

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

With no signal the session stays: a call that cannot reach the platform fails with `NETWORK` and ends nothing, and
no session is ended by the device's clock. It ends when the platform refuses a refresh after the signal returns
(`SESSION_EXPIRED` once the idle or absolute limit has passed), and sign-in itself needs the platform. The limits,
what `keepAlive` can and cannot do offline, and the rules for a queue on a shared device are in
`clients/field-react-native/README.md`, "No signal: what happens to a session"; they hold for this library too.

## API (Swift name, Kotlin name)

`FlowCatalystFieldCore` (all `public`):

| Swift | Kotlin |
|---|---|
| `actor FieldSession` | `FieldSession` |
| `init(config:storage:keys:transport:clock:unlockPrompter:unlockKeys:unlockKeyPrompter:)` | `FieldSession.create(config, Deps(...))` (internal in Kotlin) |
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
| `unlockKeyAvailability() throws -> UnlockKeyAvailability` | `unlockKeyAvailability()` |
| `unlockKey() throws -> UnlockKeyInfo?` | `unlockKey()` |
| `enableUnlockKey(prompt:)` / `enableUnlockKey(using:prompt:) -> UnlockKeyInfo` | `enableUnlockKey(activity, prompt)` / `enableUnlockKeyWith(prompter, prompt)` |
| `loginWithUnlockKey(prompt:)` / `loginWithUnlockKey(using:prompt:) -> Session` | `loginWithUnlockKey(activity, prompt)` / `loginWithUnlockKeyWith(prompter, prompt)` |
| `disableUnlockKey()` | `disableUnlockKey()` |
| `headersFor(method:url:)` / `headersForRetry(method:url:nonce:)` / `noteNonce(url:nonce:)` | same names |
| `perform(_ HTTPRequest, using:oneShot:)` | `FieldInterceptor.intercept` (`interceptor()`) |
| `FieldSession.normaliseBadgeToken(_:)` | `FieldSession.normaliseBadgeToken` |
| `FieldConfig`, `UnlockPolicy` (`.never`) | `FieldConfig`, `UnlockPolicy` (`NEVER`). Durations are `TimeInterval` seconds, not `Duration` |
| `DeviceInfo`, `DeviceRecord`, `DeviceMode` (`.personal/.shared/.unenrolled`) | same |
| `Session`, `Worker`, `Actor`, `LoginCredentials`, `ActAsTarget`, `LogoutScope` (`.session/.deviceAll`) | same (`DEVICE_ALL`) |
| `FieldState` (`.signedOut/.signedIn/.locked`) | `FieldState` (`SignedOut/SignedIn/Locked`) |
| `FieldEvent` (`.sessionChanged/.sessionEnded/.pinChangeRequired/.expiryWarning/.locked/.workerLocked/.unlockKeyChanged`) | `FieldEvent` |
| `UnlockKeyAvailability` (raw values are the Kotlin names), `UnlockKeyInfo`, `UnlockKeyPrompt`, `UnlockKeyMaterial`, `UnlockKeyCodes` | same |
| `protocol UnlockKeyProvider`, `protocol PreparedSignature`, `protocol UnlockKeyPrompter` | same |
| `UnlockKeyErrorMapping` | `BiometricErrors.forUnlockKey` |
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
| `SecureEnclaveUnlockKeyProvider` | `AndroidUnlockKeyProvider` |
| `LocalAuthenticationUnlockKeyPrompter` | `BiometricUnlockKeyPrompter` |
| `URLSessionTransport` | `defaultHttpClient()` |
| `FieldSession.authorize(_ URLRequest)`, `perform(_ URLRequest, session:)` | `interceptor()` |
| `DeviceInfo.current` | `DeviceInfo(model = Build.MODEL)` |

Not ported: `headersForBlocking` (no blocking form in Swift; use `await`).

## The client of the session

The session says which client the worker belongs to, so an application that addresses its own API per client
(`/api/clients/{client}/...`) needs no "who am I" route to build its addresses:

```swift
let me = try await session.login(LoginCredentials(workerCode: "W001", pin: "1234"))   // or any `Session` from the state
me.clientIdentifier // "acme": the client's identifier (the access token's `client_identifier` claim)
me.clientId         // the platform's id for the client (the `client_id` claim)
```

- Both are read from the access token's claims at every sign-in, refresh, act-as and sign-in code (so an
  impersonated session has the worker's client), and stored with the session, so a session restored after the
  app restarts has them (also while it is `.locked`). The sign-in answer carries no client; the platform's wire
  shape is unchanged.
- Both are `String?`: `nil` when the token does not carry the claim (or is not a JWT), the same as `deviceId`
  on an unenrolled device. Every platform release since the field token's first release (`fcdev` 0.12.5) mints
  both, so `nil` is met only on a session stored by a version of this library before it read them (until the
  next refresh) or on a token from something other than the platform. Do not guess a client: when
  `clientIdentifier` is `nil`, wait for the refresh or ask the worker to sign in again. They are the last two
  parameters of `Session.init` and default to `nil`, so code that builds a `Session` still compiles.
- **The identifier on the phone is for building addresses and showing a name.** It is read from a token the phone
  cannot verify and is not proof of anything. Your server must take the client from the verified token (the
  framework's `fw.Context`, or the SDK verifier's claims), never from the path or body the phone sends: check that
  the `{client}` in the address is the token's client, and refuse it otherwise.

## The worker's own photo

`session.worker.photoVersion` (nil when there is none, or photos are off) and `session.worker.canSetOwnPhoto` (true only
when the client lets workers set their own and the session is the worker's own) are kept with the session across restarts.
A photo changed or removed elsewhere shows as a different `photoVersion` at the next sign-in or refresh, each time with a
`.sessionChanged`; there is no separate event.

```swift
let photo = try await session.getPhoto(size: .thumb, haveVersion: held)   // nil: no photo; photo.notModified: keep yours
if worker.canSetOwnPhoto {
    try await session.setPhoto(jpegData, contentType: "image/jpeg")       // JPEG, PNG or WebP; decided from the bytes
    try await session.removePhoto()
}
```

Each call is signed like the others (nonce, single refresher). Refusals are `FieldAuthError` codes: `OWN_PHOTO_NOT_ALLOWED`,
`PHOTOS_DISABLED`, `PHOTOS_DISABLED_FOR_CLIENT`, `IMPERSONATING` (reading is allowed while impersonating, changing is not),
`PHOTO_EMPTY`, `PHOTO_TOO_LARGE`, `PHOTO_UNSUPPORTED_TYPE`, `PHOTO_TYPE_MISMATCH`, `PHOTO_DIMENSIONS_TOO_LARGE`,
`PHOTO_UNREADABLE`, `PHOTO_BUSY`, and `TOO_MANY_REQUESTS` with `retryAfterSeconds`.

## Unlock keys (sign in again with Face ID or Touch ID)

`docs/field-unlock-keys-spec.md` section 12; the same API and behaviour as `clients/field-android` (its README,
"Unlock keys", has the full tables). After a PIN sign-in on an enrolled PERSONAL phone, the worker can agree to sign in
next time with Face ID or Touch ID: the library makes a Secure Enclave P-256 key that needs a biometric match for every
use, registers its public half with the platform, and later signs a fresh platform challenge with it to get a new
session without the PIN. Off unless the client's policy has `unlockKeysEnabled`.

```swift
let field = try FieldSession.get(config: config)
_ = try await field.login(LoginCredentials(workerCode: "W001", pin: pin))
if try await field.unlockKeyAvailability() == .available /* and the worker said yes */ {   // no network
    _ = try await field.enableUnlockKey(prompt: UnlockKeyPrompt(title: "Use Face ID to sign in next time", cancel: "Not now"))
}
// later, signed out (the session ended):
if let key = try await field.unlockKey() {            // whose key it is: key.workerCode, key.workerName
    do {
        _ = try await field.loginWithUnlockKey(prompt: UnlockKeyPrompt(title: "Sign in as \(key.workerName)", cancel: "Use PIN"))
    } catch let e as FieldAuthError {
        // every failure means: show the PIN pad; e.code says why (table below)
    }
}
try await field.disableUnlockKey()                    // "turn off Face ID sign-in" / forget the worker / hand the phone over
```

- **`unlockKeyAvailability()`** answers whether to offer it now, with no network: `.available`, or why not
  (`.enabled`, `.notEnrolled`, `.sharedDevice`, `.notSignedIn`, `.impersonating`, `.otherWorker`, `.policyOff`,
  `.pinSignInRequired`, `.noSecureHardware` (the simulator), `.biometricNotEnrolled`, `.biometricUnavailable`,
  `.attestationRequired`, `.unsupported`). The platform's part comes from the session's latest token answer,
  `unlockKeys {allowed, attestationRequired, registerExpiresIn?}` (spec 2.3), kept as `Session.unlockKeys` and replaced
  by every sign-in and refresh: `.policyOff` when it does not allow unlock keys (the client's policy is off, or the
  session is not the worker's own on their own enrolled phone) or the platform sent none (one older than unlock keys);
  `.pinSignInRequired` when it carries no `registerExpiresIn` (a badge alone, an unlock-key session, a pending PIN
  change) or that window has run out, counted from when the answer arrived on `FieldClock.monotonicMilliseconds()`
  (`SystemClock`: Darwin's `CLOCK_MONOTONIC`, which keeps counting while the phone sleeps). The window is not stored:
  after a restart the next refresh brings what is left of it. **`.attestationRequired` whenever the answer says the
  policy requires attestation: iOS cannot attest an unlock key.** The library never asks the platform for any of this
  (no empty or test registration).
- **`enableUnlockKey`** checks the same first and refuses before any key, request or prompt (`UNLOCK_KEYS_DISABLED`,
  `PIN_SIGN_IN_REQUIRED`, `ATTESTATION_REQUIRED`, ...); the answer is advisory and the platform checks again, so its
  refusals still come through as below. It shows one prompt (the key proof is signed by the new key). The proof signs the request's own single-use nonce, which the library takes
  before the prompt; a nonce that expired during a long prompt costs one more prompt, once, as does a
  `409 UNLOCK_KEY_EXISTS` (a new key is made). Nothing is kept when it fails.
- **`loginWithUnlockKey`** asks for the challenge first, so a refusal never shows a prompt; then the prompt, then the
  unlock, which gives a session exactly as `login` does (`amr` `["hwk","uv"]`, in `Session.amr`). A
  `CHALLENGE_EXPIRED` (a slow prompt) gets one new challenge and one more prompt.
- **`.unlockKeyChanged(UnlockKeyInfo?)`** is emitted when a key is registered (the key) or forgotten (nil).

| Code | Key | What to show |
|---|---|---|
| `UNLOCK_KEY_INVALID` (platform) | deleted, revoked | PIN; offer again after it |
| `UNLOCK_KEY_INVALIDATED` (library) | deleted, revoked | Face ID / Touch ID changed: PIN; offer again |
| `UNLOCK_KEYS_DISABLED` | deleted | PIN; the next refresh's answer says not allowed |
| `PIN_REAUTH_REQUIRED` | kept | PIN; the key works again after it |
| `PIN_RESET_REQUIRED` | kept | ask a supervisor to reset the PIN |
| `ATTESTATION_STATUS_UNAVAILABLE` (503, `retryAfterSeconds`) | kept | PIN for now; try the key next time |
| `INVALID_UNLOCK_PROOF` | kept, deleted on the second in a row | PIN |
| `WORKER_LOCKED`, `WORKER_LOCKED_TEMPORARILY`, `TOO_MANY_ATTEMPTS` | kept (`workerLocked` event) | as for a PIN sign-in |
| `BIOMETRIC_CANCELLED`, `BIOMETRIC_LOCKOUT`, `BIOMETRIC_NOT_ENROLLED`, `BIOMETRIC_UNAVAILABLE` | kept | PIN |
| `NO_UNLOCK_KEY`, `NOT_ENROLLED`, `SHARED_DEVICE`, `UNKNOWN_DEVICE`, `DEVICE_REVOKED` | none / deleted | PIN |
| `NO_APP_ACCESS`, `DEVICE_SUSPENDED`, `NETWORK` | kept | as for a PIN sign-in |

Registration refusals keep nothing: `UNLOCK_KEY_OTHER_WORKER`, `PIN_SIGN_IN_REQUIRED`, `WORKER_UNAVAILABLE`,
`INVALID_PUBLIC_KEY`, `ATTESTATION_REQUIRED`, `ATTESTATION_STATUS_UNAVAILABLE`, `UNLOCK_KEYS_UNSUPPORTED`, and the
prompt's codes.

**What iOS does differently** (spec 6.1, 12.1 to 12.4):

- **No attestation.** A Secure Enclave key with `.biometryCurrentSet` cannot be attested (App Attest attests an app key
  that cannot carry a biometric access control), so the registration sends no `attestationChain` and no attestation
  challenge is made; the platform records the verdict `NONE`. **A client whose policy has `unlockKeyAttestation:
  REQUIRED` cannot use unlock keys on iOS**: the token answer says `attestationRequired`, `unlockKeyAvailability`
  answers `.attestationRequired` and `enableUnlockKey` refuses `ATTESTATION_REQUIRED` with a message saying exactly that,
  before a key or a prompt. A platform's own `422 ATTESTATION_REQUIRED` gets the same message.
- **The key**: `SecureEnclaveUnlockKeyProvider`, a permanent P-256 key per worker (tag
  `io.flowcatalyst.field.unlock.<workerId>.<random>`), `kSecAttrTokenIDSecureEnclave`,
  `kSecAttrAccessibleWhenPasscodeSetThisDeviceOnly` with `.privateKeyUsage` and `.biometryCurrentSet`. No software
  fallback: the simulator answers `.noSecureHardware`.
- **The prompt**: `LocalAuthenticationUnlockKeyPrompter` evaluates `deviceOwnerAuthenticationWithBiometrics` (no
  passcode fallback: `localizedFallbackTitle` is empty; `title` is the `localizedReason`, `cancel` the cancel button),
  then the Secure Enclave signs under that evaluated `LAContext` (`kSecUseAuthenticationContext`), DER to raw r‖s.
  `subtitle` is not shown on iOS. The `LAError` table is `UnlockKeyErrorMapping` (lockout is `BIOMETRIC_LOCKOUT`, not
  the app unlock's `BIOMETRIC_UNAVAILABLE`).
- **Invalidation**: the key is unusable once Face ID or Touch ID enrolment changes (`.biometryCurrentSet`). The library
  finds that out before it asks the platform or shows a prompt: `prepare` refuses a key that is gone
  (`errSecItemNotFound`) or whose recorded biometric enrolment state (`LAContext.domainState.biometry.stateHash`, earlier
  `evaluatedPolicyDomainState`, kept in `UserDefaults` with the key; not secret) differs from the current one. A
  signature the Enclave still refuses with `errSecItemNotFound` is treated the same way. Then the key is revoked on the
  platform, deleted, and `UNLOCK_KEY_INVALIDATED` is thrown: sign in with the PIN.

**Unlock keys and the app unlock are different things.** `unlock(prompt:)` gates the screens of a session that is still
live (after a cold start or time in the background; Face ID, Touch ID or the passcode) and touches no token. An unlock
key signs the worker in again after the session has ended, verified by the platform, with a biometric only. An app may
use both: `unlock` while the session lives, `loginWithUnlockKey` when it has ended.

The state kept across restarts and sign-outs (nothing secret): `fc.field.unlockKey` (the key's id, worker, device,
tag), `fc.field.unlockKey.revoke` (revocations still to send: a disable without signal deletes the key at once and
revokes it with the next platform call), and `unlockKeys {allowed, attestationRequired}` in the stored session (the
register window is kept in memory only).

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
| `BIOMETRIC_CANCELLED`, `BIOMETRIC_UNAVAILABLE`, `NO_SECURE_LOCK_SCREEN` | From `unlock` (see the mapping in `UnlockErrorMapping`) and the unlock-key prompt (`UnlockKeyErrorMapping`). |
| `NO_UNLOCK_KEY`, `UNLOCK_KEY_INVALIDATED`, `UNLOCK_KEYS_UNSUPPORTED`, `UNLOCK_KEY_OTHER_WORKER`, `BIOMETRIC_NOT_ENROLLED`, `BIOMETRIC_LOCKOUT` | Unlock keys ("Unlock keys" above). |
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
`WireFlowTests`, `RefreshAndPerformTests`, `UnlockTests`, `TimersTests`, `UnlockKeyTests`, `UnlockJwsTests` (the
platform's byte vectors in `api/field-fixtures/unlock.json`; skipped where the package is checked out without them),
plus `DerSignatureTests` (the vectors the React Native
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
- Unlock keys on a device: the Secure Enclave key with `.biometryCurrentSet`, signing under the evaluated `LAContext`
  without a second prompt, the error a signature gives after an enrolment change, the recorded biometric state across
  an enrolment change and an OS update, and a registration and unlock against a real platform.
- Keychain access after a restart, before and after the first unlock, and the behaviour of a refresh while the phone is
  locked (the `WhenUnlocked` key).
- The simulator branch (`#if targetEnvironment(simulator)`) and the iOS-only app lifecycle observer (`#if os(iOS)`);
  the latter was type-checked against a stub of the two `UIApplication` notification names, not the real UIKit.
- `URLSession` behaviour against a real platform (redirects, header casing, `Authorization` handling).
