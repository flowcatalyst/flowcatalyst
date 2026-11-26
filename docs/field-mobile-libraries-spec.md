# Field-worker mobile libraries (P4): build spec

Written 2026-11-03 by the main session. Parent design: `docs/field-worker-identity-design.md`
(§2 "client library owns", §3.2–§3.5, §5 client matrix, §6 "Mobile libraries"). Wire reference:
`docs/wire-contract.md` "Field identity (`/field/*`)". Platform code: `internal/platform/fieldauth`.
The design's rulings apply: the library is the only code that touches a PIN or a device key; there is
no offline login (continuation only); no biometrics on SHARED devices.

Two packages, built in parallel, which meet only at the **bridge protocol** (§3) and the
**`KeyProvider` / `SecureStore` interfaces** (§2). Both copy those definitions exactly as written
here.

## 1. `clients/field-js` (`@flowcatalyst/field-js`)

TypeScript, ESM, no runtime dependencies beyond the platform's Web APIs (`crypto.subtle`, `fetch`,
`indexedDB`, `customElements`). Runs in a browser, in a React Native WebView, and in Node 22+ for
tests. Built with `tsc`; tests with the Node test runner or vitest (pick one; match
`clients/typescript-sdk` if practical).

### 1.1 `FieldAuth`

```ts
const auth = new FieldAuth({
  platformUrl: "https://fc.example.com",  // or taken from an enrolment QR payload
  appCode: "yard-gate",
  keys: KeyProvider,                       // §2
  store: SecureStore,                      // §2
  fetch?: typeof fetch,
  clock?: () => number,                    // ms, for tests
});
```

- `parseEnrolmentQr(payload)` → `{platformUrl, code}` for `fcfield:1:<platformBaseUrl>:<code>`
  (the URL contains `:`; the code is the last segment).
- `enrol(code, {label?})`: `keys.getOrCreateDeviceKey()`, then `POST /field/devices/enrol`
  `{code, publicKeyJwk, attestationChain?, platform, model, label?}` with a proof signed by that
  key. Stores `{deviceId, mode, clientId, appCodes, platformUrl}` in `store` under
  `fc.field.device`. `platform`/`model` come from an optional `deviceInfo` option (default
  `"web"` / `navigator.userAgent`).
- `login({workerCode, pin})` → `Session` (`POST /field/auth/login`). The PIN is passed through,
  never stored, never logged, never put in an exception message.
- `refresh()` (single-flight: concurrent callers share one request), `changePin({currentPin,
  newPin})`, `logout({scope: "session" | "device-all"})`, `unlock()` (§1.3).
- `fetch(input, init)`: for **application backends**. Adds `Authorization: DPoP <access>` and a
  `DPoP` proof (`htm`, `htu` = the request URL without query/fragment, `ath`, `jti`, `iat`, and
  the cached nonce for that origin). Refreshes first when the access token expires within 30 s. On
  `401` with `WWW-Authenticate: DPoP error="use_dpop_nonce"`, takes `DPoP-Nonce`, retries once.
  On `401 invalid_token` after a refresh, emits `sessionEnded`. Every response's `DPoP-Nonce` is
  cached per origin.
- Platform calls use the same nonce handling (`/field/auth/nonce` to prime; `DPoP-Nonce` on every
  response; one retry on `use_dpop_nonce`).
- State: `auth.session` (`{worker:{id, workerCode, name, photoVersion, canSetOwnPhoto}, appCode, expiresAt, idleExpiresAt,
  refreshExpiresAt, pinChangeRequired, clientId, clientIdentifier}` or `null`; the last two are the access
  token's `client_id` and `client_identifier` claims, `null` when absent: `docs/wire-contract.md`, "The client on the
  phone"), and events through `addEventListener`
  (`EventTarget`): `sessionChanged`, `pinChangeRequired`, `sessionEnded` (`reason`), `locked`
  (`{retryAfter?}` or hard).
- Errors: `FieldAuthError` with `code` = the platform's error code (`INVALID_CREDENTIALS`,
  `WORKER_LOCKED`, `WORKER_LOCKED_TEMPORARILY`, `TOO_MANY_ATTEMPTS` + `retryAfter`, `NO_APP_ACCESS`,
  `DEVICE_REVOKED`, `DEVICE_SUSPENDED`, `UNKNOWN_DEVICE`, `INVALID_GRANT`, `SESSION_EXPIRED`,
  `PIN_POLICY` + `violations`, `INVALID_ENROLMENT_CODE`, `FIELD_IDENTITY_UNCONFIGURED`,
  `NETWORK`). Read the exact codes and shapes from `internal/platform/fieldauth/endpoints.go`.
- Storage keys: `fc.field.device` (plain), `fc.field.refresh` (the refresh token; stored with
  `requireBiometric: true` only when the device mode is PERSONAL and the caller turned
  `biometricUnlock` on), `fc.field.session` (non-secret session metadata).
- Never send the refresh token anywhere but `POST /field/auth/refresh|logout` on `platformUrl`.

### 1.2 Key providers and stores shipped in field-js

- `WebCryptoKeyProvider`: a **non-extractable** ECDSA P-256 key, kept in IndexedDB (a
  `CryptoKey` survives structured clone), attestation `{level: "SOFTWARE"}`. `sign` returns the raw
  64-byte r‖s (WebCrypto already does).
- `IndexedDbSecureStore` (browser) and `MemorySecureStore` (tests): no biometric support
  (`set(..., {requireBiometric: true})` throws `BIOMETRIC_UNAVAILABLE`).
- `BridgeKeyProvider` and `BridgeSecureStore`: talk to the React Native host over §3. `bridgeAvailable()`
  reports whether the page runs inside a host that installed the bridge.
- `defaultProviders()`: the bridge ones when available, else WebCrypto + IndexedDB.

### 1.3 Biometric local unlock (design §3.4, local unlock only)

`unlock()` reads `fc.field.refresh` from the store (the native side shows the biometric prompt),
then refreshes. Allowed only when the enrolled device mode is PERSONAL and the store supports
biometrics; otherwise `BIOMETRIC_UNAVAILABLE`. A store answer `KEY_INVALIDATED` (new fingerprint
enrolled, OEM key loss) clears the stored token and raises `BIOMETRIC_INVALIDATED`: the caller
asks for the PIN. Never a crash.

### 1.4 `<fc-pin-pad>`

A custom element, no framework: a 3×4 keypad with large targets (gloves; at least 64 px),
dots instead of digits, attributes `length` (default 6), `scrambled` (shuffle the digits each
time it is shown), `disabled`, `label`. Emits `pin` (`detail.pin`) when `length` digits are in
and then clears itself; `clear()` method. Keyboard input works (digits, Backspace, Enter). It
never writes the PIN to an attribute, the DOM text, storage or the console. Works with no CSS
framework (styles in a shadow root, CSS custom properties for colours).

### 1.5 Tests

- Unit: proof construction (verify proofs with an independent ES256 check in the test, jkt
  per RFC 7638), nonce retry, single-flight refresh, error mapping, QR parsing, pin pad behaviour
  (with a DOM shim, or happy-dom/jsdom as a dev dependency), "PIN never appears" (in thrown
  errors, stored values, `console.*` output during a login).
- **End-to-end against a real platform**: start an isolated platform the way
  `internal/platform/fieldauth/*_pg_test.go` does, or better, a Go test helper. Preferred: a Go
  program under `clients/field-js/e2e/server` (or a test in `internal/platform/fieldauth`) that
  boots the real `fieldauth` + admin ops on `internal/testpg`, seeds a client, app, worker (PIN)
  and an enrolment code, prints `{baseUrl, code, workerCode, pin, appCode}` as JSON on stdout,
  and serves until killed; the field-js e2e test spawns it, then: enrol → login → `fetch`
  against a tiny app server that verifies with the Go SDK `FieldTokenVerifier` (also in the
  helper) → refresh → changePin → logout. The test kills only the child process it spawned.

## 2. Interfaces shared by both packages (copy exactly)

```ts
export type AttestationLevel = "STRONGBOX" | "TEE" | "SOFTWARE" | "NONE";

export interface PublicEcJwk { kty: "EC"; crv: "P-256"; x: string; y: string }

export interface DeviceKey {
  jwk: PublicEcJwk;
  attestation: { level: AttestationLevel; chain?: string[] }; // chain: base64 DER certs, leaf first
}

export interface KeyProvider {
  getOrCreateDeviceKey(): Promise<DeviceKey>;
  /** ES256 over SHA-256(data); returns raw r‖s (64 bytes). */
  sign(data: Uint8Array): Promise<Uint8Array>;
  deleteDeviceKey(): Promise<void>;
}

export interface SecureStore {
  get(key: string, opts?: { prompt?: string }): Promise<string | null>;
  set(key: string, value: string, opts?: { requireBiometric?: boolean }): Promise<void>;
  delete(key: string): Promise<void>;
  biometricAvailable(): Promise<boolean>;
}
```

`set` never prompts the user, even with `requireBiometric`; only `get` of a biometric entry prompts.

Errors from providers and stores carry a `code`: `KEY_INVALIDATED`, `BIOMETRIC_UNAVAILABLE`,
`BIOMETRIC_CANCELLED`, `NO_SECURE_LOCK_SCREEN`, `INTERNAL`.

## 3. The WebView bridge protocol (version 1, copy exactly)

The page (field-js) posts to the host with `window.ReactNativeWebView.postMessage(string)`; the
host answers by calling `window.__fcFieldBridge.receive(string)` through
`webViewRef.injectJavaScript(...)`. The host installs `window.__fcFieldBridge = {version: 1,
receive}` before content loads (`injectedJavaScriptBeforeContentLoaded`), which is what
`bridgeAvailable()` checks.

**Host stub and listener (ruling, 2026-11-03).** Before content loads the host injects
`window.__fcFieldBridge = {version: 1, receive: function (m) { if (this._l) this._l(m); else (this._q = this._q || []).push(m); }}`.
`_q` (the queue) and `_l` (the page's listener) may be absent initially. Every response is
delivered as `window.__fcFieldBridge.receive(<JSON string literal>)`, where `m` is the JSON
**string** of the response object. The page never replaces `receive`: on first use it sets
`window.__fcFieldBridge._l = handler`, then drains the queue (`const q = bridge._q || [];
bridge._q = [];`) and handles each entry. Responses whose `id` is unknown are ignored.
`bridgeAvailable()` is `typeof window.__fcFieldBridge === "object" && window.__fcFieldBridge.version === 1
&& typeof window.ReactNativeWebView?.postMessage === "function"`.

Request (page → host), JSON:
```json
{"fcField": 1, "id": "<unique per request>", "op": "<op>", "args": { }}
```
Response (host → page), JSON:
```json
{"fcField": 1, "id": "<same id>", "ok": true, "result": { }}
{"fcField": 1, "id": "<same id>", "ok": false, "error": {"code": "KEY_INVALIDATED", "message": "..."}}
```

| `op` | `args` | `result` |
|---|---|---|
| `key.getOrCreate` | `{}` | `DeviceKey` |
| `key.sign` | `{data: <base64url>}` | `{signature: <base64url of 64 bytes r‖s>}` |
| `key.delete` | `{}` | `{}` |
| `store.get` | `{key, prompt?}` | `{value: string \| null}` |
| `store.set` | `{key, value, requireBiometric?}` | `{}` |
| `store.delete` | `{key}` | `{}` |
| `bio.available` | `{}` | `{available: boolean}` |

Rules: the host answers **only** for pages whose origin is in its `allowedOrigins` (the
WebView's `onMessage` event gives `nativeEvent.url`); any other origin gets no answer and a warning
log without the payload. Messages without `fcField: 1` are ignored (the app's own messages pass
through to its handler). Unknown `op` → `{ok:false, error:{code:"INTERNAL"}}`. The page times out a
request after 60 s (a biometric prompt can take a while). Store values cross the bridge as
strings; the host never logs `value` or `data`.

## 4. `clients/field-react-native` (`@flowcatalyst/field-react-native`)

A bare React Native native module (no Expo dependency), usable from plain RN screens
(`NativeKeyProvider`, `NativeSecureStore`, implementing §2) and as the WebView bridge host.

- **JS/TS**: `NativeKeyProvider`, `NativeSecureStore`; `useFieldBridge({allowedOrigins})` →
  `{ref, onMessage, injectedJavaScriptBeforeContentLoaded}` to spread on a `react-native-webview`
  `<WebView>` (the "one line" install; `onMessage` chains to an app handler passed in options);
  peer dependencies `react`, `react-native`, `react-native-webview`.
- **Android (Kotlin, `android/`)**:
  - Device key: Keystore EC P-256, alias `fc_field_device`, `PURPOSE_SIGN`, `DIGEST_SHA256`,
    `setIsStrongBoxBacked(true)` first, falling back without it on
    `StrongBoxUnavailableException`; attestation challenge = 32 random bytes (the platform does
    not verify it in P1; send the chain); level from `KeyInfo.securityLevel` on API 31+
    (`STRONGBOX`/`TRUSTED_ENVIRONMENT`→`TEE`/`SOFTWARE`), else `isInsideSecureHardware`. `sign`
    uses `SHA256withECDSA` and converts the DER signature to raw r‖s (left-pad to 32 bytes each).
  - Secure store: plain entries are AES-256-GCM under the Keystore key `fc_field_store` (no user auth),
    kept in `SharedPreferences` (ciphertext + IV, base64). Biometric entries use a hybrid envelope: the
    Keystore RSA-2048 pair `fc_field_store_bio` (DECRYPT, OAEP SHA-256/MGF1-SHA-1,
    `setUserAuthenticationRequired(true)`, `setUserAuthenticationParameters(0, AUTH_BIOMETRIC_STRONG)` on
    API 30+, `setInvalidatedByBiometricEnrollment(true)`); `set` wraps a fresh software AES-256-GCM data key
    with the public key (no prompt); only `get` runs `BiometricPrompt` with a `CryptoObject(Cipher)` to unwrap. `KeyPermanentlyInvalidatedException` /
    `UnrecoverableKeyException` → delete the entry and the key, answer `KEY_INVALIDATED`. No
    secure lock screen → `NO_SECURE_LOCK_SCREEN`.
  - `minSdkVersion` 26, `androidx.biometric`. Never log values or signatures.
- **iOS (Swift, `ios/`)**: device key in the Secure Enclave (`kSecAttrTokenIDSecureEnclave`,
  P-256, `.privateKeyUsage`), level `TEE`; falls back to a Keychain software key on the simulator
  (level `SOFTWARE`). `SecKeyCreateSignature(.ecdsaSignatureMessageX962SHA256)` → DER → raw r‖s.
  Store: Keychain generic passwords; biometric entries with `SecAccessControl`
  `.biometryCurrentSet` (invalidated by enrolment change → `KEY_INVALIDATED`). `NSFaceIDUsageDescription`
  documented for the app's Info.plist.
- **Tests**: Jest (or the package's runner) for the bridge host (origin allowlist, unknown op,
  pass-through of non-bridge messages, no logging of values) with the native module mocked; the
  DER↔raw conversion unit-tested on both platforms where a toolchain allows (Kotlin JVM unit test;
  Swift via `swift test` on a small package target if practical).
- **Compile**: an `example/` app is optional. Build the Android library if Gradle and the Android SDK
  at `~/Library/Android/sdk` let you (a Gradle wrapper may need downloading); compile the Swift with
  `xcodebuild`/`swiftc` against the iOS simulator SDK if practical. Report exactly what was compiled
  and what was not. Running on a real device is the owner's gate (design §7 P4).

## 5. Not in P4

Server-verified unlock keys (P5: built later, section 6), attestation verification, offline login receipts (P6), passkeys
and the Kotlin-native library (P7), badges/NFC, release tooling for these packages (they stay
unreleased until the owner decides; no `make release-*` line yet).

## 6. Unlock keys (P5, built in the libraries 2026-11-26)

The libraries' side of `docs/field-unlock-keys-spec.md` (section 12 there is the requirement; this section says what
was built). The same five operations, the same availability values, the same codes and the same stored state in
every library; field-android is the reference and its README ("Unlock keys") has the full tables.

| Operation | field-android (Kotlin) | field-ios (Swift) | field-react-native (`FieldClient`) | field-js |
|---|---|---|---|---|
| may it be offered now | `unlockKeyAvailability()` | `unlockKeyAvailability()` | `unlockKeyAvailability()` | always `"UNSUPPORTED"` |
| the key on this phone | `unlockKey()` | `unlockKey()` | `unlockKey()` | `null` |
| make and register it | `enableUnlockKey(activity, prompt)` | `enableUnlockKey(prompt:)` | `enableUnlockKey(prompt)` | rejects `UNLOCK_KEYS_UNSUPPORTED` |
| sign in with it | `loginWithUnlockKey(activity, prompt)` | `loginWithUnlockKey(prompt:)` | `loginWithUnlockKey(prompt)` | rejects `UNLOCK_KEYS_UNSUPPORTED` |
| switch it off | `disableUnlockKey()` | `disableUnlockKey()` | `disableUnlockKey()` | resolves, no request |

- **What the platform says.** Every token answer (login, refresh, unlock, act-as, sign-in code) carries `unlockKeys =
  {allowed, attestationRequired, registerExpiresIn?}` (`docs/field-unlock-keys-spec.md` section 2.3). Each library
  keeps it with the session, replaced by every answer and cleared with the session, as `Session.unlockKeys` (`allowed`,
  `attestationRequired`, `registerExpiresAt`; null when the platform sent none). The register window is counted from
  when the answer arrived on the monotonic clock (Android `SystemClock.elapsedRealtime`; iOS: see field-ios's README)
  and kept in memory only: after a restart it is closed until the session's next refresh, which carries what is left
  of it. The stored session keeps `allowed` and `attestationRequired`. **No library asks the platform** (no empty or
  test registration): the answer is the only source, and it is advisory (the registration is checked again).
- **Availability** (`UnlockKeyAvailability`), computed on the phone with no request, in this order: `UNSUPPORTED` /
  `OS_TOO_OLD`; `NOT_ENROLLED`, `SHARED_DEVICE`, `NOT_SIGNED_IN`, `IMPERSONATING` (the library's own, more specific
  reasons first); `PIN_SIGN_IN_REQUIRED` (the session is not this device's); `OTHER_WORKER`; `ENABLED`; `POLICY_OFF`
  (`allowed: false`, or no `unlockKeys` at all: a platform too old to say reads as off); `PIN_SIGN_IN_REQUIRED` (no
  `registerExpiresIn`, a PIN change pending, the window run out, or a restart since); `BIOMETRIC_NOT_ENROLLED`,
  `BIOMETRIC_UNAVAILABLE`, `NO_SECURE_HARDWARE`; `ATTESTATION_REQUIRED` (`attestationRequired` on a phone that cannot
  attest: always iOS; on Android when the device key, made by the same Keystore with an attestation challenge, has no
  chain); else `AVAILABLE`. `enableUnlockKey` checks the same first and refuses without a key, a request or a prompt
  (`POLICY_OFF` as `UNLOCK_KEYS_DISABLED`); a new Android key made without a chain under `attestationRequired` is
  deleted before its prompt. The platform's refusals at registration are mapped as before.
- **Registration.** Android: Keystore P-256 per worker (alias `fc_field_unlock.<workerId>.<random>`), the parameters of
  spec 12.1 including the attestation challenge of spec 3, StrongBox, then the TEE with attestation, then the TEE
  without; a key outside secure hardware is deleted before any prompt. iOS: Secure Enclave P-256 with
  `.privateKeyUsage` and `.biometryCurrentSet`, no chain. The key proof signs the registration's own DPoP nonce: the
  nonce is reserved, the prompt shown without holding the platform-call lock, the request sent with it. One more
  prompt, once, after `use_dpop_nonce` (the prompt outlived the nonce) or `409 UNLOCK_KEY_EXISTS` (a new key).
  Stored: `fc.field.unlockKey` `{unlockKeyId, workerId, workerCode, workerName, deviceId, createdAt, pinRequiredBy,
  attestationVerdict, alias}`; the worker's earlier key is deleted after the platform replaced it.
- **Sign-in.** A key the phone invalidated is detected before anything is sent; then the challenge; only on `200`
  the prompt (Android `BiometricPrompt` with a `CryptoObject(Signature)`, strong biometrics only; iOS
  `SecKeyCreateSignature` after `LAContext`); then `/field/auth/unlock` with the next nonce, applied as a login.
  `CHALLENGE_EXPIRED` asks for one new challenge and prompts again, once.
- **What each refusal does to the key** (spec section 10): deleted and revoked for `UNLOCK_KEY_INVALID`, a key the
  phone invalidated (`UNLOCK_KEY_INVALIDATED`) and a second `INVALID_UNLOCK_PROOF` in a row; deleted for
  `UNLOCK_KEYS_DISABLED`, `SHARED_DEVICE`, `UNKNOWN_DEVICE`, `DEVICE_REVOKED`; kept for everything else
  (`PIN_REAUTH_REQUIRED`, `ATTESTATION_STATUS_UNAVAILABLE`, `PIN_RESET_REQUIRED`, `NO_APP_ACCESS`, lockouts, the
  prompt's codes, `NETWORK`).
- **Revocation from the phone** (`/field/auth/unlock-keys/revoke`, the device key alone) for `disableUnlockKey`, an
  invalidated key and an unknown one. Offline, the key is deleted at once and the revocation queued
  (`fc.field.unlockKey.revoke`), sent with the next platform call and dropped only when the platform answers or no
  longer knows the device, so a live key nobody can use does not keep another worker off the phone (spec D2).
- **New library codes:** `NO_UNLOCK_KEY`, `UNLOCK_KEY_INVALIDATED`, `UNLOCK_KEYS_UNSUPPORTED`,
  `BIOMETRIC_NOT_ENROLLED`, `BIOMETRIC_LOCKOUT`, `UNLOCK_KEY_OTHER_WORKER`. New event `UnlockKeyChanged {key|null}`;
  `Session.amr` (native libraries).
- **The local unlock is unchanged.** field-js's `unlock()` (the refresh token behind a biometric store entry, §1.3)
  and the native `unlock(...)` app lock gate a live session; an unlock key makes a new one after a session ended.
- **Tests:** each library checks its signing inputs byte for byte, and the vectors' signatures, against
  `api/field-fixtures/unlock.json`, and every flow against its fake platform. Not run on a phone: the Keystore and
  Secure Enclave keys, the prompts, a real attestation chain.

