# @flowcatalyst/field-react-native

React Native bridge for field-worker devices. **New Architecture only** (TurboModule `FcField`, codegen spec
`src/NativeFcField.ts`, React Native >= 0.76). Spec: `docs/field-native-session-spec.md` section 2; WebView
bridge: `docs/field-mobile-libraries-spec.md`.

Installed from git, not a registry: `"@flowcatalyst/field-react-native": "github:flowcatalyst/field-react-native#v0.2.0"`
(the published repository holds the built package; see `docs/releasing.md`). Metro reads the TypeScript source
(`src/`) through the `react-native` entry; `lib/` carries the JS and `.d.ts` types for everything else.

- **Android:** a thin bridge over the Kotlin `io.flowcatalyst.field.FieldSession` (`clients/field-android`), the one
  owner of keys, tokens, refresh and nonces in the process. JS screens and a native foreground service share it.
- **iOS:** only the key/store module exists (Secure Enclave key, Keychain store) for the WebView bridge.
  `FieldClient` session methods reject with `NOT_SUPPORTED_ON_IOS_YET` until the Swift session owner is built.
- **WebView pages** keep using `@flowcatalyst/field-js` through the bridge host (protocol v1). On Android its key
  operations use the same Keystore key as `FieldSession` (alias `fc_field_device`): one device key per app.

## Setup

### Expo (prebuild / dev client; not Expo Go)

```json
{ "expo": { "plugins": [["@flowcatalyst/field-react-native", {
  "faceIdPermission": "Unlock the app with Face ID",
  "fieldAndroidVersion": "0.1.0"
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
   v0.1.0 and later are on GitHub Packages. For local work on the library, a composite build in the app's
   `settings.gradle` also works: `includeBuild("../path/to/clients/field-android")` (it replaces the published
   coordinates).
3. Autolinking registers `FcFieldPackage`. Override the library version with `ext.fieldAndroidVersion` in the root
   `build.gradle`.
4. iOS: `cd ios && pod install`, and `NSFaceIDUsageDescription` in Info.plist.

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
`NOT_CONFIGURED`, `CONFIG_MISMATCH`, `KEY_INVALIDATED`, `BIOMETRIC_*`, `NOT_SUPPORTED_ON_IOS_YET`, `INTERNAL`.

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

### Foreground service

A tracking service is plain Kotlin and uses `FieldSession` directly (`FieldSession.get(ctx, config).interceptor()`
for OkHttp, or `headersForBlocking`). It MUST run in the app's process (no `android:process`) so JS and the service
share the one owner. The config must equal the JS config (`CONFIG_MISMATCH` otherwise). The owner uses no alarm:
the foreground service keeps the process alive. See `clients/field-android/README.md`.

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
pnpm codegen                                                  # TurboModule spec -> android/build/codegen
JAVA_HOME=<jdk17> ANDROID_HOME=~/Library/Android/sdk ./android/gradlew -p android \
  -PfcCodegenJava=build/codegen/android/app/build/generated/source/codegen/java assembleRelease testDebugUnitTest
```

Verified (2026-11-06): `tsc --noEmit` clean; 44 JS/plugin tests pass; Kotlin compiles (release and debug) against
react-android 0.76.9, the codegen-generated `NativeFcFieldSpec` and `field-android` (composite build), 11 JVM tests
pass. Not verified: a real app build with the React Native Gradle plugin and autolinking, a device or emulator run,
the C++/JNI codegen half, and everything iOS (no Xcode; the Swift module is not New-Architecture-registered yet).
