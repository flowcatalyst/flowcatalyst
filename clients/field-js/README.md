# @flowcatalyst/field-js

Browser / WebView library for FlowCatalyst field workers: device enrolment, PIN sign-in, an authenticated `fetch` for application backends, and a reader for the push stream.

## Install

Not on npm. Install from a git tag of the published repository
([flowcatalyst/field-js](https://github.com/flowcatalyst/field-js)):

```bash
npm install github:flowcatalyst/field-js#v0.2.0
```

(or `"@flowcatalyst/field-js": "github:flowcatalyst/field-js#v0.2.0"` in `package.json`). See `docs/releasing.md`.

## Errors

Failures are `FieldAuthError`s with a `code` (the platform's error code, or the library's own), `status` and, for lockouts, `retryAfter`.

- `PIN_RESET_REQUIRED` (HTTP 403, from `login` and `actAs`): the worker's PIN was set under an app key that has since been removed. It is not a wrong PIN and retrying cannot help. Show "Ask your supervisor to reset your PIN". `isPinResetRequired(e)` tests for it. The library does not retry, does not emit `locked`, and does not clear the enrolment or end a session.

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
