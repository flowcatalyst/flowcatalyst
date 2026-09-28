# clients/fn-go

The Go guest SDK for FlowCatalyst functions (plan `docs/function-runner-plan.md`
§9). Package `fn`, zero third-party dependencies, Go 1.24+.

Write handlers as plain `net/http`:

```go
package main

import (
	"net/http"

	fn "github.com/flowcatalyst/flowcatalyst-go/clients/fn-go"
)

var greeting = fn.Config("GREETING")

func init() {
	fn.Open("GET /healthz", health)
	fn.Platform("GET /hello/{name}", hello)
}

func main() {}

func health(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

func hello(w http.ResponseWriter, r *http.Request) {
	g, found, _ := greeting.Get(r.Context())
	if !found {
		g = "Hello"
	}
	w.Write([]byte(g + ", " + r.PathValue("name")))
}
```

## Package layout

- `frame.go` — the ABI v1 frame codec (`u32 LE metaLen | meta JSON | body`),
  portable, no build tag.
- `host.go` — the `host` interface that separates ABI transport from SDK
  logic, plus the shared `*Error`/error-code vocabulary.
- `abi_wasip1.go` (`//go:build wasip1`) — the entire ABI surface: the two
  `fc` host imports (`call`/`take`) and the four guest exports
  (`fc_abi_v1`, `fc_alloc`, `fc_handle`, `fc_describe`).
- `abi_other.go` (`//go:build !wasip1`) — a settable `FakeHost` so the rest
  of the package runs under a normal `go test` on the host OS.
- `registry.go`, `endpoint.go`, `subscribe.go`, `schedule.go`, `config.go`
  — the `init()`-time declaration API (`Webhook`/`Platform`/`Open`,
  `Subscribe`, `Schedule`, `Config`/`Secret`/`DB`, `HTTPAllow`, `Emits`).
- `describe.go` — builds the `fc_describe` JSON document (plan §5.4) from
  the registry; never touches a host import.
- `dispatch.go` — turns a request frame into an `*http.Request`, runs it
  through the registered `http.ServeMux`, and turns the response back into
  a frame. Recovers handler panics into `500 {"error":"FUNCTION_PANIC"}`.
- `caller.go`, `invocation.go` — `Caller`/`Principal` (with the platform's
  wildcard `HasPermission` semantics) and `Invocation` accessors, both read
  from the request context dispatch.go populates.
- `respond.go` — `Retry`/`Reject` helpers for the retry/terminal-outcome
  contract (plan §13.2).
- `log.go` — an `slog.Handler` over host op 1.
- `http.go` — `HTTPClient()`, an `*http.Client` whose `RoundTripper` runs
  over host op 4, forwarding the request context's deadline as `timeoutMs`.
- `event.go` — `Emit`, wrapping host op 5; `errors.Is(err, ErrRetryable)`
  distinguishes platform trouble (`UNAVAILABLE`) from a permanent refusal.
- `db.go` — a `database/sql` driver registered as `"fc"` over host ops
  6-10 (query/exec/begin/commit/rollback), with an order-preserving row
  decoder (`DBBinding.SQL()`).

## Testing

```
go test ./...
```

Every capability wrapper (config, secret, http, emit, db, log) and the
dispatch path are tested against `FakeHost` on the host OS — no wasm runtime
needed. `db.go`, `dispatch.go` and the declaration API each have dedicated
`_test.go` files; `describe_test.go` pins the exact `fc_describe` JSON shape
with a golden-style structural comparison.

## Building the example to wasm

```
make build-hello
```

This runs:

```
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o examples/hello/hello.wasm ./examples/hello
```

`examples/hello` declares an open health check, a platform-authenticated
greeting that reads a declared config value and the caller's principal, and
a webhook endpoint that emits an event. The built `.wasm` is not committed;
run `make build-hello` locally (or `make clean` to remove it).

As measured on this checkout (darwin/arm64, Go 1.27.1): `hello.wasm` is
**7,634,743 bytes** unstripped, **7,493,869 bytes** with `-ldflags="-s -w"`.
This is larger than the plan's §2.1 standard-Go reference point (4.4 MB) —
expected, since that guest's `.wasm` was presumably measured with symbols
stripped and/or a smaller Go/toolchain version; `-ldflags="-s -w"` only
narrows the gap here. `wasm-opt -Os` on this binary fails to validate
(reports "error validating input" partway through `memory.copy` lowering):
wasm-opt 133 does not yet fully understand every feature Go 1.27's wasip1
`c-shared` output emits. Not required by this work package's spec (only the
Rust build lists `wasm-opt` as optional) but noted here as a real gap for
whoever picks up further wasm-size work in phase 0/2.

## Decisions where the plan was silent

- `DB(name).SQL()` params: `[]byte` values are base64-encoded (standard
  encoding) into the JSON `params` array element; `time.Time` uses
  `RFC3339Nano` (an explicit task requirement). The host side (out of this
  work package's lane) must mirror the `[]byte` choice for bytea columns to
  round-trip, since the wire carries no per-parameter type tag.
- `db.query` rows: nested JSON/JSONB column values are re-marshaled to a
  JSON string `driver.Value` rather than exposed as a Go map/slice — the
  ABI's row JSON has no column-type metadata, so the SDK can't tell a
  JSON/JSONB column from any other nested value; callers who know a column
  is JSON/JSONB can `json.Unmarshal` the returned string themselves.
- Endpoint `method` is always emitted in `fc_describe` (every
  `Webhook`/`Platform`/`Open` registration requires an explicit "METHOD
  /path" pattern), even though the plan's §5.4 example shows a bare
  `/healthz` entry with no `method` key. The schema marks `method` optional,
  so always including it is valid; the SDK favours requiring an explicit
  method at every call site over supporting a method-less registration.
- `Reject(w, reason)` writes `reason` as a plain-text response body (no
  wrapping JSON envelope); `reason == ""` writes no body.
