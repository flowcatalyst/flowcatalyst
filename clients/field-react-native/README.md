# @flowcatalyst/field-react-native

React Native native module for field-worker devices (Android Kotlin, iOS Swift). It holds the
device's hardware-backed signing key and a secure store, and hosts the WebView bridge for
`@flowcatalyst/field-js`. Spec: `docs/field-mobile-libraries-spec.md` §2-§4; design:
`docs/field-worker-identity-design.md`. Unreleased (no release line yet).

Bare React Native only (no Expo dependency). Peer dependencies: `react`, `react-native`,
`react-native-webview`.

## Use

From plain RN screens:

```ts
import { NativeKeyProvider, NativeSecureStore } from "@flowcatalyst/field-react-native";
const keys = new NativeKeyProvider();   // implements KeyProvider
const store = new NativeSecureStore();  // implements SecureStore
```

Errors are `FieldError` with `code` in `KEY_INVALIDATED | BIOMETRIC_UNAVAILABLE |
BIOMETRIC_CANCELLED | NO_SECURE_LOCK_SCREEN | INTERNAL`. A lost or invalidated key never crashes
the app; it surfaces as `KEY_INVALIDATED` (or a fresh key from `getOrCreateDeviceKey`).

Hosting `field-js` in a WebView (the one-line install):

```tsx
const bridge = useFieldBridge({ allowedOrigins: ["https://field.example.com"], onMessage: myHandler });
<WebView source={{ uri: "https://field.example.com" }} {...bridge} />
```

The host answers only pages whose origin (from `nativeEvent.url`) is in `allowedOrigins`; others
get no answer and a warning naming the origin only. Messages without `fcField: 1` go to your
`onMessage`. Unknown ops answer `INTERNAL`. Values, data and signatures are never logged.

## Android setup

- `minSdkVersion 26` (the library declares 26; compile/target 35).
- Dependencies pulled in: `androidx.biometric:biometric`, `androidx.fragment:fragment`.
- Autolinking registers `FcFieldPackage`. The host `Activity` must be a `FragmentActivity`
  (`ReactActivity` is) for the biometric prompt.
- Manifest permission `USE_BIOMETRIC` is merged from the library.
- Key: Keystore EC P-256 alias `fc_field_device`, StrongBox first then TEE; attestation chain sent when
  the device provides one. Store keys: `fc_field_store` and `fc_field_store_bio`
  (a Keystore RSA-2048 pair, decrypt gated by biometric-strong, invalidated by enrolment change).
- `set` never prompts, even with `requireBiometric`; only `get` of a biometric entry prompts (a rotating
  refresh token is rewritten every few minutes without bothering the worker). Biometric values use a
  hybrid envelope: a fresh AES-256-GCM data key encrypts the value and is wrapped with the RSA public key;
  `get` unwraps it through `BiometricPrompt` + `CryptoObject`. On iOS, `set` is `SecItemDelete` +
  `SecItemAdd` with `.biometryCurrentSet` (adding does not prompt; never `SecItemUpdate`).

## iOS setup

- `cd ios && pod install`. Needs a Swift-enabled app target (`use_frameworks!` not required).
- Info.plist: `NSFaceIDUsageDescription` (for example "Unlock your session with Face ID"). Without
  it Face ID devices fail the biometric prompt.
- Key: Secure Enclave P-256 (level `TEE`); simulator falls back to a Keychain software key (`SOFTWARE`).
  Store: Keychain generic passwords; biometric entries use `.biometryCurrentSet`.

## SHARED devices

Biometrics belong to the device, not to a worker. On SHARED devices **do not use biometric
unlock** (`requireBiometric` / `bio.available` must not drive worker identity); the platform
refuses biometrics and unlock keys for SHARED devices (design §9 D5). Use the PIN path only.

## Verification status

Verified (2026-11-03, on a dev machine):

- TypeScript: `pnpm lint` (tsc, strict) clean; `pnpm test` 14 tests (bridge host: origin allowlist,
  non-bridge pass-through, unknown op, every op, error codes, escaping of quotes / `</script>` /
  U+2028 / U+2029, no logging of values; adapters with the native module mocked).
- Android: `./gradlew assembleRelease testDebugUnitTest` (AGP 8.7.3, Gradle 8.10.2, JDK 17,
  compileSdk 35, react-android 0.76.9): library compiles; 12 JVM unit tests pass (DER<->raw, and the hybrid envelope: format, round-trip with a software RSA key, wrong key, tamper).
- iOS (partial): `DerSignature.swift` tests ran on macOS with `swiftc` and a tiny XCTest shim
  (6 tests incl. 200 real Keychain-signed round trips); `FcField.swift` type-checks against the macOS SDK
  with a stubbed `React` module.

NOT verified:

- No Xcode / iOS SDK on the build machine: the Swift module and the Obj-C bridge file (`FcField.m`) were
  never compiled for iOS, the podspec was never run through `pod lib lint`, and the Secure Enclave and
  simulator-fallback branches were not compiled (`#if targetEnvironment(simulator)`).
- Nothing was run on a device or emulator: Keystore/StrongBox key generation, attestation chains,
  `BiometricPrompt` unwrap with a `CryptoObject` (and that a Keystore-generated RSA key accepts the OAEP SHA-256/MGF1-SHA-1 parameters on real devices), Keychain biometric items, and enrolment-change invalidation are
  untested. `ios/Tests` (XCTest via `swift test`) needs full Xcode.
- The React Native glue (`useFieldBridge` hook, native module registration) was not exercised in a real app.

A run on a real rugged device (enrol, login, background, unlock, revoke) is still required
(design §7 P4).

## Develop

```
CI=true pnpm install && pnpm lint && pnpm test
cd android && ANDROID_HOME=~/Library/Android/sdk ./gradlew assembleRelease testDebugUnitTest   # JDK 17
```
