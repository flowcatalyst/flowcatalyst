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
- State: `auth.session` (`{worker:{id, workerCode, name}, appCode, expiresAt, idleExpiresAt,
  refreshExpiresAt, pinChangeRequired}` or `null`), and events through `addEventListener`
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

Server-verified unlock keys (P5), attestation verification, offline login receipts (P6), passkeys
and the Kotlin-native library (P7), badges/NFC, release tooling for these packages (they stay
unreleased until the owner decides; no `make release-*` line yet).
