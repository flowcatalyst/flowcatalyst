# dispatch-demo-rust

The Rust version of [`../dispatch-demo`](../dispatch-demo/README.md): the same two functions, the same names, endpoints,
status codes, permission and job shape, so everything in that README (the `dispatch` capability, the application,
connection, role and token setup, deploy and invoke steps, the checks) applies unchanged.

| Directory | Function |
|---|---|
| `dispatch-creator/` | `platform` endpoint `POST /dispatch`; requires `<app>:dispatch:job:create` (403 otherwise), takes a JSON object or array, creates a dispatch job for `<app>.payload-logger` `/deliver` through connection `dispatch-demo-conn`; 202 `{jobId}`, 400 bad body, 502 refused, 503 platform trouble |
| `payload-logger/` | `webhook` endpoint `POST /deliver` (signature verified by the runner), logs the payload, returns `{"ack":true}` |

## What differs from the TypeScript version

- `APP_CODE` is read with `option_env!` at compile time, exactly as the TypeScript build bakes it in:
  `APP_CODE=example fcdev fn deploy ...`. Unset, it is `dispatchdemo`. (Cargo reruns the build when it changes.)
- Tests are `cargo test` in each directory (`flowcatalyst_fn::testing` and the fake host).
- One-time: `rustup target add wasm32-unknown-unknown`. The artifact is
  `target/wasm32-unknown-unknown/release/<crate>.wasm` (about 130 KB); `fcdev fn build|dev|deploy <dir>` builds it.
- A Rust guest has no panic recovery: a panic traps the instance and the runner answers 500 `FUNCTION_FAILED`.

## Build, test, run locally

```
cd dispatch-demo-rust/dispatch-creator && cargo test && fcdev fn dev . --perms dispatchdemo:dispatch:job:create
curl -X POST 127.0.0.1:8099/fn/local.dispatch-creator/dispatch -H 'Authorization: Bearer dev' -d '{"order":42}'
cd ../payload-logger && cargo test && fcdev fn dev .
```

Deploy as in the TypeScript README, with `$D=examples/functions/dispatch-demo-rust`.
