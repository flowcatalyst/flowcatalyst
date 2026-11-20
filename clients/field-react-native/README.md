# @flowcatalyst/field-react-native

React Native bridge for field-worker devices. **New Architecture only** (TurboModule `FcField`, codegen spec
`src/NativeFcField.ts`, React Native >= 0.76). Spec: `docs/field-native-session-spec.md` section 2; WebView
bridge: `docs/field-mobile-libraries-spec.md`.

Installed from git, not a registry: `"@flowcatalyst/field-react-native": "github:flowcatalyst/field-react-native#v0.4.1"`
(the published repository holds the built package; see `docs/releasing.md`). Metro reads the TypeScript source
(`src/`) through the `react-native` entry; `lib/` carries the JS and `.d.ts` types for everything else.

- **Android:** a thin bridge over the Kotlin `io.flowcatalyst.field.FieldSession` (`clients/field-android`), the one
  owner of keys, tokens, refresh and nonces in the process. JS screens and a native foreground service share it.
- **iOS:** the same module, spec and JS API over the Swift `FieldSession` of `clients/field-ios`, the process-wide
  owner of keys, tokens, refresh and nonces. The Swift sources are carried inside this package (`ios/FieldIos`, a
  verbatim copy made by `pnpm sync:field-ios`) and compile together with the bridge into one Swift module named
  `FlowcatalystFieldReactNative`. Every `FieldClient` method, event and error code behaves as on Android (differences:
  see "iOS notes").
- **WebView pages** keep using `@flowcatalyst/field-js` through the bridge host (protocol v1). On Android its key
  operations use the same Keystore key as `FieldSession` (alias `fc_field_device`): one device key per app.

## Setup

### Expo (prebuild / dev client; not Expo Go)

```json
{ "expo": { "plugins": [["@flowcatalyst/field-react-native", {
  "faceIdPermission": "Unlock the app with Face ID",
  "fieldAndroidVersion": "0.4.0"
}]] } }
```

The plugin (`app.plugin.js`) adds `USE_BIOMETRIC`, raises `android.minSdkVersion` to at least 26 (option
`minSdkVersion` to go higher), adds the GitHub Packages Maven repository
(`https://maven.pkg.github.com/flowcatalyst/flowcatalyst`, credentials from gradle properties `gpr.user` / `gpr.key`
or env `GITHUB_ACTOR` / `GITHUB_TOKEN`), adds `implementation "io.flowcatalyst:field-android:<version>"` to the app
module, and sets `NSFaceIDUsageDescription` (`faceIdPermission: false` to skip; an existing value is kept). It is
idempotent. Needs a Groovy `android/build.gradle` (the Expo default).

### Bare React Native

1. `minSdkVersion 26`; the host `Activity` must be a `FragmentActivity` (`ReactActivity` is).
2. Add the Maven repository above to `allprojects.repositories` (with credentials). `io.flowcatalyst:field-android`
   is on GitHub Packages; this bridge needs v0.4.0 or later, which has the worker's own photo (its default is the version it was built against). For local work on the library, a composite build in the app's
   `settings.gradle` also works: `includeBuild("../path/to/clients/field-android")` (it replaces the published
   coordinates).
3. Autolinking registers `FcFieldPackage`. Override the library version with `ext.fieldAndroidVersion` in the root
   `build.gradle`.
4. iOS: iOS 15.1 or later (the pod needs 15.0; React Native 0.76 itself needs 15.1). `cd ios && pod install`, and
   `NSFaceIDUsageDescription` in Info.plist (Face ID `unlock` needs it). The pod `FlowcatalystFieldReactNative`
   compiles Swift and Objective-C++ (no `use_frameworks!` setting is required), links `Security`,
   `LocalAuthentication`, `CryptoKit` and `UIKit`, and is registered as the TurboModule `FcField` through
   `RCT_EXPORT_MODULE` and the codegen spec. The device key and the Keychain items need no entitlement and no
   capability. An app served over plain `http` (a local platform in development) needs an ATS exception in Info.plist
   (`NSAllowsLocalNetworking` covers `localhost`); the library itself accepts `http:` only for loopback hosts and
   origins named in the config.

## Use

```ts
import { FieldClient } from "@flowcatalyst/field-react-native";

const field = new FieldClient({
  platformUrl: "https://platform.example.com",
  appCode: "my-app",
  clientIdentifier: "acme",                                   // for worker-code sign-in on an unenrolled device
  requireUnlock: { afterColdStart: true, afterBackgroundMs: 5 * 60_000 },
  expiryWarningsMs: [30 * 60_000, 10 * 60_000],              // defaults
  keepAliveLeadMs: 2 * 60_000,                                // default
});
```

The first method call (or `on`) configures the native owner (`FieldSession.get`); a second `FieldClient` with a
different config rejects with `CONFIG_MISMATCH`.

### Sign-in flows

Same names and `Session` shape as field-js `FieldAuth` (`worker, actor, test, deviceMode, deviceId, appCode,
expiresAt, idleExpiresAt, refreshExpiresAt, pinChangeRequired`; times are ms since the epoch).

```ts
const { state, session } = await field.restore();            // SignedOut | SignedIn | Locked
await field.enrol(qrPayloadOrCode, { label: "Truck 7" });
await field.login({ workerCode: "W001", pin: "1234" });      // or { badgeToken, pin? }
await field.redeemSignInCode(code);
await field.actAs({ workerCode: "W002", pin: supervisorPin }); await field.endImpersonation();
await field.changePin({ currentPin, newPin });               // changes, then refreshes
await field.refresh(); await field.logout({ scope: "session" | "device-all" });
```

Failures reject with `FieldAuthError` (`code`, `status?`, `retryAfterSeconds?`, `violations`): the field-js codes
(`INVALID_CREDENTIALS`, `INVALID_GRANT`, `SESSION_EXPIRED`, `DEVICE_REVOKED`, ...) plus `NETWORK`, `NOT_ENROLLED`,
`NOT_CONFIGURED` (a session method before the first `configure`), `CONFIG_MISMATCH` (a second config that differs from
the first), `KEY_INVALIDATED`, `BIOMETRIC_UNAVAILABLE`, `BIOMETRIC_CANCELLED`, `NO_SECURE_LOCK_SCREEN`,
`INVALID_REQUEST`, `PLATFORM_MISMATCH`, `INTERNAL`. On both platforms the native rejection also carries `status`,
`retryAfterSeconds` and `violations` when the platform sent them (the NSError user-info on iOS).

`PIN_RESET_REQUIRED` (HTTP 403, from `login` and `actAs`): the worker's PIN was set under an app key that has since
been removed. It is not a wrong PIN and retrying cannot help. Show "Ask your supervisor to reset your PIN".
`isPinResetRequired(e)` tests for it. The library does not retry, does not emit `WorkerLocked`, and does not clear
the enrolment or end a session.

### Events

```ts
const off = field.on("SessionEnded", ({ reason }) => navigateToSignIn(reason));
```

`SessionChanged {session|null}`, `SessionEnded {reason}`, `PinChangeRequired {session}`, `ExpiryWarning
{remainingMs}`, `Locked {session}`, `WorkerLocked {hard, retryAfterSeconds?}`. Plain callbacks (no `EventTarget`, no
DOM; works on Hermes). `off()` unsubscribes; `field.dispose()` drops everything.

### Unlock

Tokens are stored without user authentication so refresh works in the background and after a reboot. The biometric
is an app unlock: when `state` is `Locked` (policy `requireUnlock`; enrolled PERSONAL, non-impersonated sessions
only) show your lock screen and call `await field.unlock("Unlock")`. Never use it on SHARED devices (PIN only).

### Requests to your backend, SSE, uploads

```ts
const res = await field.fetch("https://api.example.com/trips", { method: "POST", body });  // signs, retries once on 401 use_dpop_nonce
const h = await field.headersFor("GET", url);        // { Authorization, DPoP }: refreshes first when needed
// on 401 use_dpop_nonce: field.headersForRetry("GET", url, response.headers.get("DPoP-Nonce"))
// any DPoP-Nonce you see elsewhere: field.noteNonce(url, nonce)
```

Use `headersFor` for things that are not `fetch` (SSE/EventSource polyfills, upload libraries). A proof is bound to
one method + URL and one nonce.

**Offline queue rule:** store requests unsigned and sign when sending. A stored proof fails anyway (single-use or
5-minute nonces, `jti` replay memory, access tokens last 10 minutes).

### Push: job updates for field workers

A host that uses the framework's push module (contract: `docs/contracts/push.md`) pushes small signals to a worker's
device; a worker's jobs arrive on `{app}:field-worker:{principal}:jobs`. The push says what changed, so read the state
through your API. `PushStream` is the same reader as in `@flowcatalyst/field-js`, with no DOM (it runs on Hermes, with
or without `TextDecoder`) and `addListener` for events.

```ts
import { PushStream } from "@flowcatalyst/field-react-native";
import { fetch as streamFetch } from "expo/fetch"; // streams response bodies; RN's built-in fetch does not

const topic = `my-app:field-worker:${principal}:jobs`;
const stream = new PushStream({
	baseUrl: "https://app.example.com/api",
	topics: [topic],
	fetch: field.fetch.bind(field), // signs the subscribe call (DPoP + refresh)
	streamFetch, // the stream GET carries its token in the URL, so it needs no signature
	lastEventId: await loadSavedId(), // optional: resume after an app restart
});

stream.on(topic, (push) => refreshJobs()); // push = { id, topic, type, data }
stream.addListener("reset", () => refreshJobs()); // away too long: reload your state
stream.addListener("denied", (topics) => console.warn(topics));
stream.addListener("error", (err) => console.warn(err.code));
stream.start();

// Foreground and background: the OS drops the connection, so end it and start it again.
AppState.addEventListener("change", (s) => {
	if (s === "active") { stream.stop(); stream.start(); } else stream.stop();
});
```

- `addListener(event, fn)` returns `{ remove() }`. Events: `subscribed` (`{granted, latest}`), `reset` (`{reason, latest}`),
  `denied` (`[{topic, code}]`), `error` (a `PushError`), `state` (`connecting`, `open`, `reconnecting`, `stopped`).
- `on(pattern, handler)` takes a topic or a pattern (`*` one segment, a final `**` the rest); `onAny(handler)` takes every
  push. Both return a function that removes the handler. Ids are 64-bit strings; pushes arrive once and in order.
- Save `stream.lastEventId` (for example on each push) and pass it back next start. `stream.setTopics([...])` subscribes
  again with a new set. `getTickets(topics)` fetches tickets from your own endpoint when a topic needs one.
- After `stop(); start()` the reader resumes from the last id; `stop()` alone does not forget it.
- The full behaviour (reconnect, backoff, `auth-expired`, resume) is described in the `@flowcatalyst/field-js` README.
  The two packages share one source file, `push-core.ts`.

### Long shifts: keepAlive

```ts
await field.keepAlive(true);   // while a trip / tracking is active
await field.keepAlive(false);
```

While on, the owner refreshes before the idle deadline. Turn it on only while the worker is genuinely busy. Use
`ExpiryWarning` to ask for a new sign-in at a stop.

### No signal: what happens to a session

**A session is never ended by the device's clock.** `idleExpiresAt` and `refreshExpiresAt` only plan `keepAlive` and
the expiry warnings. A session ends on the device when the platform refuses a refresh (`SessionEnded` with
`SESSION_EXPIRED`, `INVALID_GRANT`, `DEVICE_REVOKED`, ...), when an `invalid_token` survives a refresh, or on logout.
A call that cannot reach the platform fails with `NETWORK` and changes nothing: the worker stays signed in. This is
the same on Android, iOS and in the browser (`@flowcatalyst/field-js`).

So, for a device that works where there is no signal:

- **The worker stays signed in for as long as the signal is away**, whatever the idle limit, and the app can go on
  recording work into its own offline queue. A shared device never locks (Unlock is for PERSONAL sessions), and
  unlock is local to the device.
- **`keepAlive` does not help offline.** Its refresh needs the platform, so it fails with `NETWORK` and is tried
  again; the session's deadline passes on the platform meanwhile.
- **When the signal returns, the first request refreshes.** If the session's idle or absolute limit passed while the
  device was away, the platform answers `SESSION_EXPIRED`, the session ends and the worker signs in again. Nothing
  recorded offline is lost by that: keep the queue across the sign-in and send it afterwards (unsigned in the queue,
  signed when sent, as above).
- **Send each queued item only as the worker who recorded it.** On a shared device another worker may sign in first;
  the first worker's items wait until that worker signs in on the device again. Keep the worker's id with each item.
- **Sign-in needs the platform.** There is no offline sign-in: a worker who is signed out, or whose session has ended,
  cannot sign in until there is signal. A hand-over between workers at a place with no signal is therefore not
  possible on one device; plan the hand-over for where there is signal, or give each worker a device.

The limits, each counted on the platform from the last refresh (idle) or the sign-in (absolute):

| Session | Idle limit (default) | Absolute limit (default) |
|---|---|---|
| Shared device | `sharedIdleSecs` (900, 15 minutes) | 12 hours, fixed |
| Personal device | `personalIdleSecs` (7200, 2 hours) | `shiftMaxSecs` (50400, 14 hours) |
| Device not enrolled | `unenrolledIdleSecs` (900, 15 minutes) | 12 hours, fixed |

The idle limits and `shiftMaxSecs` are the client's field policy (`PUT /api/field-policies/{clientId}`; each idle
limit 60 to 86400 seconds, `shiftMaxSecs` 3600 to 86400). The policy belongs to the client, not to one application:
a longer idle limit applies to every application's sessions for that client, and a caller confined to one application
may not change it. A PUT replaces the whole policy (a value left out goes back to its default). Where a shared device is out of signal for longer than the idle limit and signing in again on its
return is not acceptable, raise `sharedIdleSecs` for that client; the 12 hours stay.

### Foreground service

A tracking service is plain Kotlin and uses `FieldSession` directly (`FieldSession.get(ctx, config).interceptor()`
for OkHttp, or `headersForBlocking`). It MUST run in the app's process (no `android:process`) so JS and the service
share the one owner. The config must equal the JS config (`CONFIG_MISMATCH` otherwise). The owner uses no alarm:
the foreground service keeps the process alive. See `clients/field-android/README.md`.

### Native code on iOS (background location and similar)

Native code in the same app process reaches the same owner through the pod's Swift module:

```swift
import FlowcatalystFieldReactNative            // the other pod depends on "FlowcatalystFieldReactNative"

let config = try FieldBridgeConfig.fieldConfig(fromJSON: storedConfigJson)   // or fieldConfig(from: [String: Any])
let session = try FieldSession.get(config: config)       // the same object JS configured; CONFIG_MISMATCH otherwise
_ = try await session.restore()
let signed = try await session.authorize(urlRequest)     // Authorization: DPoP + DPoP proof
```

`FieldSession.get(config:)` refuses a config that differs from the first one it received. `FieldBridgeConfig` is the
mapping `configure` applies to the JS config object (`platformUrl`, `appCode`, `clientIdentifier`, `requireUnlock`,
`expiryWarningsMs`, `keepAliveLeadMs`, `allowedPlatformOrigins`; milliseconds in, seconds in `FieldConfig`), so a
module that stores the JSON JS passed to `configure` rebuilds an equal `FieldConfig` after a relaunch, before or
without JS. The whole public API of `clients/field-ios` (`FieldSession`, `FieldConfig`, `FieldAuthError`, the models,
`URLRequest` helpers) is visible through that one import. Keep the Objective-C++ part of your own pod out of public
headers, as this pod does, if it imports this module from Swift.

### iOS notes

- The key is created by `SecureEnclaveKeyProvider` (Secure Enclave on a device, a software Keychain key in the
  simulator, attestation level `SOFTWARE` there and `TEE` on a device; no attestation chain). The platform treats a
  device enrolled without a chain as attestation level `NONE`.
- A signing failure while the phone is locked (the key is `WhenUnlocked`) rejects with `INTERNAL`, not
  `KEY_INVALIDATED` (the module used to report `KEY_INVALIDATED` for every signing failure).
- `restore` and `getState` include the enrolled `device` record, from `FieldSession.enrolledDevice()` (the same on
  Android, from field-android's accessor of that name).
- `unlock` shows the system prompt (Face ID / Touch ID, then the passcode: `LAPolicy.deviceOwnerAuthentication`);
  there is no foreground `Activity` to require. The process is suspended in the background, so `keepAlive` and the
  expiry-warning timers only run while the app runs; a refresh is also done on first use.
- Events are delivered through `NativeEventEmitter` once a listener is registered (`FieldClient.on` does that); events
  fired before that are not replayed.
- `keepAlive` and `noteNonce` return at once and are applied in call order (one ordered task chain, `ios/Ordering`).
- The first `configure` also starts loading the stored session in the background, as on Android.

## The worker's own photo

`session.worker.photoVersion` (null when there is none, or photos are off) and `session.worker.canSetOwnPhoto` come with the
session and survive restarts. A photo changed or removed elsewhere shows as a different `photoVersion` at the next sign-in
or refresh, each time with a `SessionChanged` event; no separate event is raised.

```ts
const photo = await field.getPhoto({ size: "thumb", haveVersion: held }); // null: none; photo.notModified: keep yours
if (field.session?.worker.canSetOwnPhoto) {
	await field.setPhoto({ bytes, contentType: "image/jpeg" }); // JPEG, PNG or WebP, decided from the bytes
	await field.removePhoto();
}
```

Failures are `FieldAuthError`s carrying the platform's code (`OWN_PHOTO_NOT_ALLOWED`, `PHOTOS_DISABLED`,
`PHOTOS_DISABLED_FOR_CLIENT`, `IMPERSONATING`, `PHOTO_EMPTY`, `PHOTO_TOO_LARGE`, `PHOTO_UNSUPPORTED_TYPE`,
`PHOTO_TYPE_MISMATCH`, `PHOTO_DIMENSIONS_TOO_LARGE`, `PHOTO_UNREADABLE`, `PHOTO_BUSY`, `TOO_MANY_REQUESTS` with `retryAfterSeconds`).

How the bytes cross the bridge: as base64url strings, the way `sign` already passes binary (the library has no other binary
path, and the TurboModule codegen has no byte-array type). That costs one third more bytes in each direction plus one copy
on each side: a 128 px thumb is about 5 KB (7 KB over the bridge), a 512 px picture about 60 KB (80 KB), and an upload is
whatever the app picked (a 3 MB camera photo crosses as 4 MB; the platform accepts up to 10 MiB by default, so
resize before uploading if you can). Both native sides decode and hand the bytes to field-android / field-ios, which sign and send them.
The Android bridge needs the field-android release that has `getPhoto`/`setPhoto`/`removePhoto` (`ext.fieldAndroidVersion`).

## WebView bridge and key/store (protocol v1)

```tsx
const bridge = useFieldBridge({ allowedOrigins: ["https://field.example.com"], onMessage: myHandler });
<WebView source={{ uri: "https://field.example.com" }} {...bridge} />
```

`NativeKeyProvider` / `NativeSecureStore` implement the field-js `KeyProvider` / `SecureStore` over the same module
(Keystore key shared with `FieldSession` on Android; Secure Enclave on iOS). Origin allow-listing, error codes and
the no-logging rule are unchanged. On SHARED devices do not use biometrics for worker identity.

## Develop and verify

```
CI=true pnpm install && pnpm lint && pnpm test               # tsc; JS + Expo plugin tests
pnpm sync:field-ios                                           # after changing clients/field-ios: refresh ios/FieldIos
swift test --package-path ios                                 # the pod's Swift files as one module + helper tests (macOS)
pnpm codegen                                                  # TurboModule spec -> android/build/codegen
JAVA_HOME=<jdk17> ANDROID_HOME=~/Library/Android/sdk ./android/gradlew -p android \
  -PfcCodegenJava=build/codegen/android/app/build/generated/source/codegen/java assembleRelease testDebugUnitTest
```

Verified (2026-11-06, Android): `tsc --noEmit` clean; Kotlin compiles (release and debug) against react-android
0.76.9, the codegen-generated `NativeFcFieldSpec` and `field-android` (composite build), 11 JVM tests pass. Not
verified on Android: a real app build with the React Native Gradle plugin and autolinking, a device or emulator run, the
C++/JNI codegen half.

iOS (2026-11-16, updated the same day): built as a Release app (Expo SDK 58, React Native 0.88.1, React 19.3, New Architecture) with
`expo prebuild` and `pod install`, run on the iOS 26.3 simulator against a local platform. A scripted flow
(configure, restore, enrol, login, changePin, refresh, `headersFor`, authenticated HTTP calls, wrong PIN,
`CONFIG_MISMATCH`, `NOT_CONFIGURED`, logout, events, relaunch and restore, key and store methods, `unlock` with simulated
Face ID) passed three times from clean installs; a second pod doing `import FlowcatalystFieldReactNative` signed a
request on the same session. Not verified: a physical device (Secure Enclave key, real Face ID / Touch ID, Keychain
after a restart or restore, background behaviour), `use_frameworks!` builds, React Native versions other than 0.88.1
(the registration is the standard TurboModule form and the spec parses with 0.76.9's codegen, but only 0.88.1 was
built and run), the biometric Keychain prompt (on the simulator `storeSet` with `requireBiometric` and `storeGet` round-trip the value after Face ID
is enrolled, but the simulator showed no prompt and returned the value without one for both a matching and a
non-matching Face ID, so the access control, the cancel path and `BIOMETRIC_CANCELLED` from `storeGet` are unproven),
an Expo SDK other than 58.
