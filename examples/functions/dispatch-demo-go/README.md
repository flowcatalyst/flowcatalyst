# dispatch-demo-go

The Go version of [`../dispatch-demo`](../dispatch-demo/README.md): the same two functions, the same names, endpoints,
status codes, permission and job shape, so everything in that README (what the `dispatch` capability is, the application,
connection, role and token setup, deploy and invoke steps, the checks) applies unchanged. Only the language-specific
parts differ:

| Directory | Function |
|---|---|
| `dispatch-creator/` | `platform` endpoint `POST /dispatch`; requires `<app>:dispatch:job:create` (403 otherwise), takes a JSON object or array, creates a dispatch job for `<app>.payload-logger` `/deliver` through connection `dispatch-demo-conn`; 202 `{jobId}`, 400 bad body, 502 refused, 503 platform trouble |
| `payload-logger/` | `webhook` endpoint `POST /deliver` (signature verified by the runner), logs the payload, returns `{"ack":true}` |

## What differs from the TypeScript version

- The application code is the constant `appCode` in `dispatch-creator/names.go` (default `dispatchdemo`), not the
  `APP_CODE` environment variable: `fcdev fn build` passes no linker flags, so edit it to `example` (or your code)
  before building, and update `TestNames`, which pins the default.
- Tests are `go test ./...` in each directory (`fn.InvokeForTest` and `fn.FakeHost`, no wasm runtime).
- The artifact is `function.wasm` built with standard Go; `fcdev fn build|dev|deploy <dir>` does it for you.

## Build, test, run locally

```
cd dispatch-demo-go/dispatch-creator && go test ./... && fcdev fn dev . --perms dispatchdemo:dispatch:job:create
curl -X POST 127.0.0.1:8099/fn/local.dispatch-creator/dispatch -H 'Authorization: Bearer dev' -d '{"order":42}'
#   -> 202 {"jobId":"djob_local_1"}; fcdev prints: would create job: target=dispatchdemo.payload-logger path=/deliver ...
cd ../payload-logger && go test ./... && fcdev fn dev .
```

Deploy as in the TypeScript README, with `$D=examples/functions/dispatch-demo-go` (payload-logger first, with `--create`).
