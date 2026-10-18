# hello-rust

The same function as `hello-go` and `clients/fn-ts/examples/hello` in Rust: open `GET /healthz`, platform
`GET /hello/{name}` (declared config `GREETING` and the caller's principal), webhook `POST /events/greeting`
(emits, deduplicated on the delivered event's id) and open `POST /dispatch` (a job for `hello.logger`
`/deliver` through connection `hello-conn`, pool `hello-pool`).

```
cargo test                                     # handlers against a fake host, on the host target
fcdev fn dev . --config GREETING=Kia           # build, serve on the real runner at 127.0.0.1:8099, hot-reload
curl 127.0.0.1:8099/fn/local.hello-rust/hello/Aroha -H 'Authorization: Bearer dev'
fcdev fn build .                               # target/wasm32-unknown-unknown/release/hello.wasm
```

One-time: `rustup target add wasm32-unknown-unknown`. Tests use `flowcatalyst_fn::testing` (`invoke`, `describe`,
`TestRequest`, `host_ok`/`host_error`) with `set_fake_host`. The registration is a plain `fn register(app: &mut App)`
shared by `export!(register)` and the tests. `wasm-opt -Os --enable-bulk-memory` takes a further ~12% off.
