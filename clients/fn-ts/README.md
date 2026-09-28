# clients/fn-ts

The TypeScript/JavaScript guest SDK for FlowCatalyst functions
(`docs/function-runner-plan.md` §9). Package `@flowcatalyst/fn`, zero
runtime dependencies, TypeScript strict.

Unlike the Go and Rust SDKs, a JavaScript function does **not** compile to
Wasm: the artifact is a single bundled script that runs on the function
runner's shared QuickJS engine (`clients/fn-js-engine`). Bundling one script
per function keeps a JS function's cost to its own instances — the engine
itself (QuickJS, compiled once) is loaded by the runner, not shipped inside
every function.

```ts
import { fn } from "@flowcatalyst/fn";

const greeting = fn.config("GREETING");

fn.open("GET /healthz", () => fn.json(200, { status: "ok" }));

fn.platform("GET /hello/{name}", (req) =>
  fn.json(200, { message: `${greeting.get() ?? "Hello"}, ${req.params.name}`, caller: req.caller.principal?.id }),
);

fn.webhook(
  "POST /events/greeting",
  async (req) => {
    const ev = req.json() as { id?: string };
    const id = await fn.emit({
      type: "hello:greeting:greeting:sent",
      dedupId: `greeting-sent-${ev.id}`,
      data: { ok: true },
    });
    return fn.status(202);
  },
);

fn.emits("hello:greeting:greeting:sent");
```

## Package layout

- `src/errors.ts` — `FnError` (code, message, retryable) and `isRetryable`,
  the error-code vocabulary shared across every host op (plan §5.3).
- `src/ops.ts` — the ten host capability op numbers.
- `src/native.ts` — the seam to the engine's natives: `callHost` wraps
  `globalThis.__fc_call`, `utf8Encode`/`utf8Decode` wrap
  `globalThis.__fc_utf8_encode`/`__fc_utf8_decode` (falling back to
  `TextEncoder`/`TextDecoder` only so the SDK also runs under plain
  `node --test`, where the engine's natives don't exist — QuickJS itself has
  neither).
- `src/headers.ts` — `FnHeaders` (case-insensitive `get`/`getAll`/`has`) and
  `FnQuery` (parsed from `rawQuery`), hand-rolled since QuickJS has no
  `Headers`/`URLSearchParams`.
- `src/caller.ts` — `Principal`/`Caller`, with `hasPermission`'s `*`
  segment-wildcard matching mirrored exactly from
  `internal/platform/shared/auth`/`clients/fn-go`/`clients/fn-rust`.
- `src/route.ts` — a ServeMux-compatible routing subset: literal segments,
  `{name}`, a trailing `{name...}`, most-specific-wins precedence, the
  404/405(+Allow) split, and GET registrations also answering HEAD.
- `src/registry.ts` — the module-top-level declaration API
  (`open`/`platform`/`webhook`/`subscribe`/`schedule`), read lazily by
  describe.ts/dispatch.ts on every call (never a snapshot taken at import
  time).
- `src/config.ts`, `src/db.ts`, `src/event.ts`, `src/http.ts`, `src/log.ts`
  — the capability wrappers (config/secret, db, emit, fetch, log), each
  also exporting its declaration function (`config`/`secret`/`db`/
  `httpAllow`/`emits`).
- `src/respond.ts` — `json`/`text`/`status`/`retry`/`reject` response
  builders.
- `src/describe.ts` — builds the `fc_describe` JSON document (plan §5.4)
  from the registry; never touches a host import.
- `src/dispatch.ts` — turns a request frame (meta JSON string + body bytes)
  into a route match and a handler call, and the handler's return value
  back into a response frame. A malformed meta frame or a handler
  throw/rejection both become `500 {"error":"FUNCTION_PANIC"}`, logged via
  host op 1 — never propagated to the engine.
- `src/index.ts` — the `fn` namespace object, plus the module's own
  side effect: installs `globalThis.__fc = {describe, handle}` (the engine's
  binding protocol) and, when nothing has defined one yet, a
  `globalThis.console` routed through host op 1.
- `src/testing.ts` — `installFakeHost`/`resetRegistry`, the test-only
  fake-host seam (mirrors the Go SDK's `FakeHost`/`SetHost`, the Rust SDK's
  `FakeHost` closure). Not exported from `src/index.ts`; test files import
  it directly by relative path.
- `bin/fc-fn-build.js` — the `fc-fn-build` bundler (`esbuild --bundle
  --format=iife --platform=neutral --target=es2020`).

## Testing

```
npm test          # node --test over src/**/*.test.ts, via tsx
npm run typecheck # tsc --noEmit
```

68 tests across 6 files, run against a settable fake `__fc_call` on plain
Node — no QuickJS runtime needed: routing (precedence, 404, 405+Allow,
HEAD-falls-back-to-GET), the describe-JSON golden shape, request parsing
(params/query/headers/body/caller), every response shape (json/text/status/
object-body/raw-bytes), throw-and-rejection → 500 FUNCTION_PANIC, retry/
reject, config/secret/db/emit/fetch/log against the fake host (including
error codes, transaction commit/rollback, `Principal.hasPermission`
wildcards), and console routing.

## Building a function

```
npm run build:hello   # examples/hello/src/index.ts -> examples/hello/dist/function.js
```

or, for your own entry point:

```
node bin/fc-fn-build.js <entry.ts> --outfile dist/function.js
```

`examples/hello` mirrors `clients/fn-go/examples/hello` and
`clients/fn-rust/examples/hello` exactly: an open health check, a
platform-authenticated greeting that reads a declared config value and the
caller's principal, and a webhook endpoint that emits an event
(`hello:greeting:greeting:sent`, deduped from the inbound event's id).
Measured on this checkout: `function.js` is **27,254 bytes** (bundled IIFE,
`--target=es2020`).

The bundle was verified end to end through the real runner and shared JS
engine:

```
FN_EXAMPLE_WASM=$PWD/clients/fn-ts/examples/hello/dist/function.js \
  go test ./internal/functions/runner -run TestSDKExamples -count=1 -v
```

## Decisions where the plan/task spec was silent

- **Own routing, not the host's `route`/`pathParams`.** The guest re-derives
  its match from `method`+`path` rather than trusting the request frame's
  `route`/`pathParams` fields — the same "let the router own matching"
  choice `clients/fn-rust` documents. The precedence rule implemented
  (literal beats param beats a trailing `{...}` rest wildcard at each
  segment position, then longer beats shorter, then registration order) is
  a faithful subset of Go's `http.ServeMux` precedence, not full
  conflict-detection-at-registration parity.
- **Sync vs async capability surface.** `config()`/`secret()`'s `.get()` is
  synchronous (`string | undefined`, throws `FnError`) since it's a plain
  read from the runner's already-fetched desired state, with nothing to
  await — mirroring `clients/fn-go`'s synchronous `ConfigValue.Get`/
  `SecretValue.Get`. `fetch`, `emit` and every `db` operation are `async`
  (real I/O over the host boundary), even though the underlying
  `__fc_call` native is itself synchronous — `await fn.fetch(...)` is what
  the task's own API sketch shows, and the same convention is extended to
  `db`/`emit` for a consistent async-for-I/O surface.
- **`db` row/param encoding.** Mirrors `clients/fn-go/db.go` exactly:
  `Uint8Array` params are base64-encoded into the JSON `params` array (no
  per-parameter type tag exists on the wire, so the host side must mirror
  this for `bytea` columns); `Date` uses `toISOString()` (millisecond
  precision — the JS analogue of the Go SDK's `RFC3339Nano`). Unlike the Go
  SDK (a hand-rolled streaming decoder, since `encoding/json`'s map mode
  doesn't preserve key order) and the Rust SDK (`serde_json`'s
  `preserve_order` feature), row decoding here is a bare `JSON.parse`: a
  JS object's own string keys already iterate in insertion order per
  ECMA-262, so no special decoder is needed. One acknowledged edge case: a
  column literally named as an unsigned integer (e.g. `"0"`) would sort
  first under JS's integer-key-ordering rule, same as any other JS object —
  not worth working around for an unusual column name.
- **`reject(reason)` writes `reason` as a plain-text body** (no JSON
  envelope), and writes no body when `reason` is omitted/empty — matching
  `clients/fn-go/respond.go`'s documented choice exactly.
- **Response body/content-type inference.** A handler may return
  `{status, headers?, body?}` directly (not just via `fn.json`/`fn.text`):
  a `string` body is UTF-8 encoded as given (no header is injected, so
  `fn.text()` remains the one that sets `Content-Type: text/plain`); a
  plain object/array body is `JSON.stringify`'d and gets
  `Content-Type: application/json` unless the handler already set one; a
  `Uint8Array` body passes through untouched; `undefined`/`null` is an
  empty body. A handler that returns nothing (`void`) is treated as
  `{status: 200}`.
- **404/405 bodies are empty.** Neither status is specified by the plan
  beyond "404 when nothing matches... 405 (+Allow header) when the path
  matches but not the method" (plan §9); this SDK sends no body for either,
  keeping the contract to status + (for 405) the `Allow` header.
- **HEAD-via-GET suppresses the response body** but keeps status/headers —
  the standard HTTP semantics for HEAD, not just a routing nicety.
- **`console` is installed only when nothing has already defined one**
  (`typeof globalThis.console === "undefined"`), so importing this package
  under plain Node (which already has a real `console`) never clobbers it;
  in the QuickJS engine, where no `console` exists at all, the SDK's own
  console is always installed. `installConsole()` is exported and accepts
  an explicit target object, so tests can verify its routing without
  touching `globalThis`.
- **`emit()`'s `data`** accepts `string | Uint8Array | object`; a plain
  object/array is JSON-encoded and defaults `contentType` to
  `application/json` unless the caller set one explicitly — the same
  convention as response bodies.

## Conflicts found against clients/fn-js-engine/src/lib.rs (verified, none blocking)

None. The engine's binding protocol — `globalThis.__fc = {describe(): string,
handle(meta: string, body: Uint8Array): {meta, body} | Promise<{meta, body}>}`,
plus the `__fc_call(op, meta, body?) -> {code, meta, body}` /
`__fc_utf8_encode` / `__fc_utf8_decode` natives — matches this SDK's
implementation exactly, confirmed by the passing end-to-end run through the
real engine and runner (`TestSDKExamples`), not just by reading the Rust
source.
