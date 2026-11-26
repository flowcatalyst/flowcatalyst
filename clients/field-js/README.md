# @flowcatalyst/field-js

Browser / WebView library for FlowCatalyst field workers: device enrolment, PIN sign-in, an authenticated `fetch` for application backends, and a reader for the push stream.

## Install

Not on npm. Install from a git tag of the published repository
([flowcatalyst/field-js](https://github.com/flowcatalyst/field-js)):

```bash
npm install github:flowcatalyst/field-js#v0.6.0
```

(or `"@flowcatalyst/field-js": "github:flowcatalyst/field-js#v0.6.0"` in `package.json`). See `docs/releasing.md`.

## On a web page

In a browser, `FieldAuth` calls the platform's `/field/*` from your page's origin (for example a page on `https://app.example.com` calling `https://platform.example.com/field/auth/login`), so the browser asks the platform first (a CORS preflight). The platform allows it only for an origin in its CORS origins table:

- A platform administrator adds your page's exact origin (scheme, host and port, no path, no wildcard: `https://app.example.com`) in the console under **CORS Origins** (`/ui/cors-origins`), or with `POST /api/platform/cors` `{"origin": "https://app.example.com", "description": "..."}`. It applies within 15 seconds. The same entry also lets the page read the push stream (`/api/push/*`).
- Until it is there, every call fails with `FieldAuthError` code `NETWORK` (the browser refuses the answer; its console names CORS). Do not work around this by starting a browser with web security disabled: add the origin, on a local platform too (`fcdev` serves the same console and API).
- No cookie is sent or needed: the platform never allows credentials on these routes, so leave `fetch` at its default `credentials`.
- The device key is a non-extractable WebCrypto key (`WebCryptoKeyProvider`) kept in the origin's IndexedDB; script on your origin can use it, so a page that signs field workers in needs the care of any sign-in page (a content security policy, trusted dependencies). See "Browser pages on other origins" in `docs/field-worker-identity-design.md`.

## Errors

Failures are `FieldAuthError`s with a `code` (the platform's error code, or the library's own), `status` and, for lockouts, `retryAfter`.

- `PIN_RESET_REQUIRED` (HTTP 403, from `login` and `actAs`): the worker's PIN was set under an app key that has since been removed. It is not a wrong PIN and retrying cannot help. Show "Ask your supervisor to reset your PIN". `isPinResetRequired(e)` tests for it. The library does not retry, does not emit `locked`, and does not clear the enrolment or end a session.
- `NETWORK`: the platform could not be reached. It ends nothing: the worker stays signed in, and no session is ended by the device's clock. A session ends when the platform refuses a refresh (`SESSION_EXPIRED` once its idle or absolute limit has passed, `INVALID_GRANT`, ...) or on logout, and sign-in itself needs the platform. The limits and the rules for work recorded with no signal are in `clients/field-react-native/README.md`, "No signal: what happens to a session"; they hold in a browser too.

## Unlock keys: not in a browser

Unlock keys (sign in again with a fingerprint or face, verified by the platform; `docs/field-unlock-keys-spec.md`)
need a key the phone's secure hardware uses only after a strong biometric. A browser's WebCrypto key cannot require a
biometric for each use, so this library offers none, and says so in a way you can test:

```ts
await auth.unlockKeyAvailability();      // always "UNSUPPORTED"
await auth.unlockKey();                  // always null
await auth.enableUnlockKey();            // rejects: FieldAuthError code UNLOCK_KEYS_UNSUPPORTED
await auth.loginWithUnlockKey();         // rejects: UNLOCK_KEYS_UNSUPPORTED
await auth.disableUnlockKey();           // resolves; nothing to do
```

The method names and the `UnlockKeyAvailability` values are those of field-android, field-ios and
`@flowcatalyst/field-react-native`, which have unlock keys. `auth.session.unlockKeys` keeps the platform's token
answer (`{allowed, attestationRequired, registerExpiresAt}`, null when it sends none) as they do; here it changes
nothing. `unlock()` (the local biometric unlock of the stored
refresh token) is unrelated and unchanged.

## The client of the session

The session says which client the worker belongs to, so an application that addresses its own API per client
(`/api/clients/{client}/...`) needs no "who am I" route to build its addresses:

```ts
const { clientId, clientIdentifier } = auth.session!;
// clientIdentifier: "acme" (the client's identifier, the access token's `client_identifier` claim)
// clientId: the platform's id for the client (the `client_id` claim)
```

- Both are read from the access token's claims at every sign-in, refresh, unlock, act-as and sign-in code (so an
  impersonated session has the worker's client), and kept with the session, so a session restored after the app
  restarts has them. The sign-in answer carries no client; the platform's wire shape is unchanged.
- Both are `string | null`: `null` when the token does not carry the claim (or is not a JWT), the same as
  `deviceId` on an unenrolled device. Every platform release since the field token's first release
  (`fcdev` 0.12.5) mints both claims, so `null` is met only on a session stored by a version of this library
  before it read them (until the next refresh, which is within the access token's life) or on a token from
  something other than the platform. Do not guess a client: when `clientIdentifier` is `null`, wait for the refresh or
  ask the worker to sign in again.
- **The identifier on the phone is for building addresses and showing a name.** It is read from a token the phone
  cannot verify and is not proof of anything. Your server must take the client from the verified token (the
  framework's `fw.Context`, or the SDK verifier's claims), never from the path or body the phone sends: check that
  the `{client}` in the address is the token's client, and refuse it otherwise.

## The worker's own photo

A worker can have a photograph (set by an administrator or your application, or, when the client's policy allows
it, by the worker themselves). The session says what the platform knows:

```ts
const w = auth.session!.worker;
w.photoVersion;    // null when there is none, or photos are off for the client or the platform
w.canSetOwnPhoto;  // true only when the client lets a worker set their own and this is the worker's own session
```

```ts
const photo = await auth.getPhoto({ size: "thumb" });           // or "full"; null when there is none
// { bytes: Uint8Array, contentType: "image/jpeg", version, notModified: false }
const same = await auth.getPhoto({ haveVersion: w.photoVersion }); // notModified: true, bytes: null while unchanged

if (w.canSetOwnPhoto) {
	await auth.setPhoto({ bytes: new Uint8Array(await file.arrayBuffer()), contentType: file.type }); // or a Blob
	await auth.removePhoto();
}
```

The platform decides the type from the bytes (JPEG, PNG or WebP; a `contentType` may only agree), scales the picture
to 512 px and 128 px and keeps nothing else of the upload. Each call is signed as the library's other platform calls
are. Failures are `FieldAuthError`s with the platform's code: `OWN_PHOTO_NOT_ALLOWED`, `PHOTOS_DISABLED`,
`PHOTOS_DISABLED_FOR_CLIENT`, `IMPERSONATING` (a supervisor or an admin acting as the worker may read the photo but
not change it), `PHOTO_EMPTY`, `PHOTO_TOO_LARGE`, `PHOTO_UNSUPPORTED_TYPE`, `PHOTO_TYPE_MISMATCH`,
`PHOTO_DIMENSIONS_TOO_LARGE`, `PHOTO_UNREADABLE`, `PHOTO_BUSY`, and `TOO_MANY_REQUESTS` with `retryAfter` (a worker
may read 120 and change 20 times an hour by default).

A photo that was changed or removed somewhere else shows as a different `photoVersion` at the next sign-in or
refresh: `sessionChanged` is raised each time, so compare `session.worker.photoVersion` with the version of the
bytes you hold. No separate event is needed. `setPhoto` and `removePhoto` raise `sessionChanged` too.

On a web page: the set call sends an image `Content-Type`, which a browser does not send without a preflight; the
platform's rule for `/field/*` already answers it (POST, `Content-Type`, `DPoP` and `Authorization`), so a page
needs only its origin in the CORS origins table, as for sign-in. The `ETag` of a read is exposed to the page.

## Push: job updates for field workers

A host that uses the framework's push module (contract: `docs/contracts/push.md`) pushes small signals to a worker's device. A worker's jobs arrive on the topic `{app}:field-worker:{principal}:jobs`. The push says what changed; read the state through your API.

`PushStream` subscribes with the credentials you give it, listens over server-sent events, remembers the last id, and reconnects and resumes on its own.

```ts
import { FieldAuth, PushStream } from "@flowcatalyst/field-js";

const principal = auth.session!.worker.id; // the worker's id (use the principal id your app puts in topics)
const stream = new PushStream({
	baseUrl: "https://app.example.com/api",
	topics: [`my-app:field-worker:${principal}:jobs`],
	fetch: auth.fetch.bind(auth), // DPoP + token refresh for the subscribe call
	lastEventId: await loadSavedId(), // optional: resume after an app restart
});

stream.on(`my-app:field-worker:${principal}:jobs`, (push) => {
	// push = { id, topic, type, data }
	refreshJobs();
});
stream.addEventListener("reset", () => refreshJobs()); // away too long: reload your state
stream.addEventListener("denied", (e) => console.warn((e as CustomEvent).detail));
stream.addEventListener("error", (e) => console.warn((e as CustomEvent).detail));
stream.start();

// Save stream.lastEventId (for example on each push) and pass it back next start.
// stream.setTopics([...]) subscribes again with a new set; stream.stop() ends it.
```

- `on(pattern, handler)` takes a topic, or a pattern where `*` is one segment and a final `**` is the rest. `onAny(handler)` takes every push. Both return a function that removes the handler.
- Pushes arrive once and in order. A push with an id at or below the last one delivered is dropped (resume overlap). Ids are 64-bit, so they are strings.
- Events (`EventTarget`, `detail` holds the value): `subscribed` (`{granted, latest}`), `reset` (`{reason, latest}`: reload your state), `denied` (`[{topic, code}]`), `error` (a `PushError`), `state` (`connecting`, `open`, `reconnecting`, `stopped`).
- Tickets: pass `getTickets(topics)` to fetch them from your own endpoint when a topic needs one. They are fetched again on every subscribe.
- When the server ends the stream with `auth-expired`, or answers 401, the reader subscribes again and resumes with the last id. Other drops reconnect after the server's `retry` (3 s) with jitter, and back off up to 60 s on repeated failures. `403 NO_TOPICS` stops the stream and reports it.
- The stream is read with `fetch` and a streaming body. Where bodies are not streamable (React Native's default `fetch`), the reader falls back to `EventSource` if there is one; give `eventTypes` (your push types) in that case, because `EventSource` only hears event types it is told about. Or pass a streaming `streamFetch`.
- On a web page, pass `fetch: globalThis.fetch.bind(globalThis)` for cookie sessions.
- The reader's logic is `src/push-core.ts`, with no DOM; `@flowcatalyst/field-react-native` carries a verbatim copy for Hermes (its `PushStream` uses `addListener` instead of `EventTarget`).
