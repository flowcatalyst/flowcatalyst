# clients/fn-rust

The Rust guest SDK for FlowCatalyst functions (plan
`docs/function-runner-plan.md` §9). Crate `flowcatalyst-fn`, edition 2021,
deps `serde` + `serde_json` only (the latter with its `preserve_order`
feature enabled — see "Decisions" below), target `wasm32-unknown-unknown`
(no WASI).

```rust
use flowcatalyst_fn::{App, Context, Error, Request, Response};

flowcatalyst_fn::export!(|app: &mut App| {
    app.webhook("POST /events/x", handler)
       .platform("GET /hello/{name}", hello)
       .open("GET /healthz", health)
       .subscribe("t:y:z:w", "/events/x")
       .schedule("*/5 * * * *", "/events/x")
       .config("GREETING")
       .secret("K")
       .db("main")
       .http_allow("api.x.com")
       .emits("a:b:c:d");
});

fn hello(req: &Request, ctx: &Context) -> Result<Response, Error> {
    Ok(Response::new(200).with_body(format!("hi {}", req.path_param("name").unwrap_or("")).into_bytes()))
}
```

## Crate layout

- `src/frame.rs` — the ABI v1 frame codec, no `cfg` gate.
- `src/host.rs` — the `fc` host import boundary: `host_call` is real ABI
  glue behind `cfg(target_arch = "wasm32")` (`#[link(wasm_import_module =
  "fc")]` extern "C" `call`/`take`), and a settable `FakeHost` closure
  behind `cfg(not(target_arch = "wasm32"))` for `cargo test` on the host
  target. Also the shared `HostError`/error-code vocabulary.
- `src/route.rs` — pattern compiling (literal segments, `{param}`, trailing
  `{rest...}`) and matching, independent of the ABI.
- `src/types.rs` — `Request`/`Response`/`Caller`/`Principal`/`Error`, plus
  the request/response meta wire shapes (`serde::Deserialize`/`Serialize`).
  `Principal::has_permission` mirrors the platform's wildcard semantics
  exactly (same `permissionMatches` logic as the Go SDK).
- `src/app.rs` — the `App` builder (`webhook`/`platform`/`open`/
  `subscribe`/`schedule`/`config`/`secret`/`db`/`http_allow`/`emits`),
  route dispatch (distinguishing 404 from 405) and the `fc_describe` JSON
  builder — never touches a host import.
- `src/context.rs` — `Context`: `config_get`/`secret_get`/`http_fetch`/
  `emit`/`db_query`/`db_exec`/`db_begin`/`db_commit`/`db_rollback`/`log*`,
  all going through `host_call`.
- `src/dispatch.rs` — turns a request frame into `Request`+`Context`,
  dispatches through `App::route`, turns the handler's
  `Result<Response, Error>` back into a response frame; implements the
  `alloc`/`handle`/`describe` functions the `export!` macro's generated
  `#[no_mangle]` exports call.
- `src/lib.rs` — public re-exports and the `export!` `macro_rules!` macro,
  which expands to the four Guest ABI v1 exports (`fc_abi_v1`, `fc_alloc`,
  `fc_handle`, `fc_describe`) at the call site (the guest crate's root). The
  `App` is rebuilt fresh on every `fc_handle`/`fc_describe` call rather than
  cached behind global state — cheap, since registration is just pattern
  strings and `fn` pointers.

## Testing

```
cargo test --manifest-path clients/fn-rust/Cargo.toml
```

65 tests, all on the host target: the frame codec (including a
little-endian pin), route compiling/matching, the `App` builder and its
describe-JSON output, dispatch (404/405/handler-`Err`/malformed-frame
panic), every `Context` capability wrapper against `FakeHost`, and
`Principal::has_permission`'s wildcard matching. `cargo clippy` is clean.

Two dispatch tests (`describe`/`alloc`/`pack`) deliberately avoid
dereferencing the packed `ptr<<32|len` `u64` the ABI export functions
return: that packing is only a valid pointer inside an actual wasm32
address space. Doing that cast+deref against a real (64-bit) host pointer
segfaults — confirmed while writing the first version of those tests, which
is why they now assert on the packing arithmetic and the underlying
thread-local buffer's contents instead.

## Building the example to wasm

```
cargo build --release --target wasm32-unknown-unknown --manifest-path clients/fn-rust/examples/hello/Cargo.toml
```

`examples/hello` declares an open health check, a platform-authenticated
greeting that reads a declared config value and the caller's principal, and
a webhook endpoint that emits an event.

Measured on this checkout (darwin/arm64 host, `wasm32-unknown-unknown`
target, `opt-level = "s"`, `lto = true`, `panic = "abort"`, `strip = true`):
`hello.wasm` is **137,055 bytes**. `wasm-opt -Os --enable-bulk-memory`
(the `--enable-bulk-memory` flag is required — Rust's wasm32-unknown-unknown
codegen emits `memory.copy`, which wasm-opt refuses without it) brings it to
**120,466 bytes**. Both exported the required `fc_abi_v1`/`fc_alloc`/
`fc_handle`/`fc_describe`/`memory` set, confirmed with `wasm-opt --print`.
This is much larger than the plan's §2.1 own-ABI Rust reference point
(85 KB), which measured a guest doing only `serde_json` parse+response with
no describe/routing/capability machinery; `examples/hello` links the full
SDK (routing, describe-JSON building, every `Context` capability wrapper)
whether or not a given build uses all of it.

## Decisions where the plan was silent

- **`serde_json` `preserve_order` feature.** `Context::db_query`'s `Row` is
  `serde_json::Map<String, Value>`; with `preserve_order` enabled that type
  iterates in insertion order, so a row's key order matches the host's SQL
  column order (plan §5.3 op 6: "the host will emit keys in select order").
  This is a feature flag on `serde_json`, not a new top-level dependency, so
  it stays within "deps: serde, serde_json only". The Go SDK solves the same
  problem with a hand-rolled streaming decoder instead, since
  `encoding/json` has no order-preserving map mode.
- **No automatic HTTP timeout from a clock.** The Go SDK forwards its
  request context's deadline as `http.fetch`'s `timeoutMs` because it runs
  under WASI and can read a real clock. A `wasm32-unknown-unknown` guest has
  *no* clock capability at all (no WASI, and the ABI has no clock op), so
  `Context::http_fetch` cannot derive "time remaining" from "now" the way
  the Go SDK does; `HttpRequest::timeout_ms` must be set explicitly by the
  caller (`0` means "let the host apply its default"). This is a real
  platform-target difference, not an oversight.
- **No guest-side panic recovery.** The plan's Rust section (§9) only
  specifies "Handler `Err` -> 500 with fixed body, logged" — unlike the Go
  section, it does not ask for panic recovery. `examples/hello` (and any
  real guest) builds with `panic = "abort"`, the wasm norm, under which an
  actual Rust panic traps the instance rather than unwinding; that trap is
  exactly the runner's own "Trap ... -> 500 FUNCTION_FAILED" outcome (plan
  §6.2), handled uniformly by the host for any guest language. A malformed
  request frame is treated the same way (a real `panic!`, not a
  manufactured response) since it indicates a host/guest ABI mismatch that
  should never happen.
- **No per-endpoint `maxBodyBytes`/`timeoutMs`/CORS options, no
  subscription/schedule modifiers (mode, maxRetries, dataOnly,
  timeoutSeconds, timezone, payload).** The task's own Rust API example
  shows only the two positional arguments for `subscribe`/`schedule` and no
  endpoint-option calls at all (contrast the Go section, which explicitly
  lists `fn.Timeout`/`fn.MaxBody`/`fn.CORS`/`fn.Mode`/etc.). `App`'s builder
  therefore emits describe entries with those fields omitted; declaring
  them is a gap versus the Go SDK, not a bug, and the describe JSON degrades
  gracefully (the fields are all optional per plan §5.4).
- **Handler type is a plain `fn` pointer**, not `Fn`/`FnMut`/a boxed
  closure: matches the task's example (`fn hello(req: &Request, ctx:
  &Context) -> Result<Response, Error> { ... }` used directly as a value)
  and keeps `export!`'s per-call `App` rebuild cheap and alloc-free for the
  registration step itself.
