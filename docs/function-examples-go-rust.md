# Go and Rust function examples; standard Go or TinyGo

*Measured 2026-10-18 on darwin/arm64 (M-series), Go 1.27.2, TinyGo 0.43.0 (LLVM 22.1.4), Rust 1.98.1,
wazero as embedded in this repo's function engine. Treat the numbers as relative; the plan (§2) already says
phase 0 re-measures on Linux in a container.*

## 1. What was built

Each TypeScript example now has a Go and a Rust twin with the same endpoints, status codes and job shape. Every
one has unit tests that run on the host (`go test` / `cargo test`) and builds and serves under `fcdev fn dev`.

| Example | Go | Rust | Mirrors |
|---|---|---|---|
| hello | `examples/functions/hello-go` | `examples/functions/hello-rust` | `clients/fn-ts/examples/hello` |
| dispatch-demo (creator + payload-logger) | `examples/functions/dispatch-demo-go/` | `examples/functions/dispatch-demo-rust/` | `examples/functions/dispatch-demo` |
| ingest-webhook | `examples/functions/ingest-webhook-go` | `examples/functions/ingest-webhook-rust` | the `function-ingest` template, with a third-party signature check added |

`ingest-webhook` is a public (`none`-auth) endpoint: it verifies `X-Signature: sha256=<hex HMAC-SHA256 of the raw
body>` against the declared secret `INGEST_SIGNING_SECRET` (constant-time compare), requires a JSON object or array,
then `fn.Dispatch`es the body unchanged to `changeme.changeme-processor` `/deliver` through the declared connection
`changeme-connection` and pool `changeme-pool`. Bad signature 401, unset secret 500 (never an open door), refusal 502,
platform trouble 429 with `Retry-After`. Placeholder names live in `names.go` / `src/names.rs`.

Two small SDK additions made the tests possible, because neither SDK exposed a way to run a request through its router:

- `clients/fn-go/fntest.go`: `fn.InvokeForTest(fn.TestRequest{...})` and `fn.DescribeForTest()` (not compiled for wasip1).
- `clients/fn-rust/src/testing.rs`: `testing::invoke`, `testing::describe`, `TestRequest`, `host_ok`, `host_error`
  (not compiled for wasm32). The registration must be a plain `fn register(app: &mut App)` shared with `export!(register)`.

Also `internal/functions/engine/measure_test.go` (opt-in, `FN_MEASURE`), the harness behind §3.

The Go examples `replace` the SDK with the in-repo path and the Rust examples use a `path` dependency, as
`fcdev fn init --sdk` does. The Go dispatch-demo takes its application code from a constant (`names.go`), because
`fcdev fn build` passes no linker flags; the Rust one uses `option_env!("APP_CODE")`, matching the TypeScript build.

All eight Go and Rust example projects: tests pass; `fcdev fn dev` builds, serves and answers (ingest: 202 with
`would create job: ... pool=changeme-pool connection=changeme-connection group=trip-1`; creators: 202 with a
permission, 401 without a token, 400 on a scalar body). `TestSDKExamples` passes for `hello-go` and `hello-rust`.

## 2. Method

Harness: `TestMeasureGuests` instantiates each artifact on the real engine (`engine.New`, `Compile`, `Instantiate`,
`Handle`), with a fake host answering `secret.get` and `dispatch.create`. It asserts the expected status, so a guest
that misbehaves fails rather than just measuring slow. Per row: compile with an empty cache (median of 3), compile
from the disk cache (median of 3), 200 instantiations (`_initialize` included), the first call on a fresh instance,
500 warm-up calls then 5000 timed calls, and memory charged to the budget (minimum, and after the 5000 calls).
The table below is the second of two identical runs, on a quiet machine; the first agreed within ~10%.

Scenarios: **health** `GET /healthz` (the floor: routing plus a tiny response); **ingest** HMAC-SHA256 over a ~80 B
JSON body, JSON validation, one dispatch host call; **creator** permission check, JSON validation, one dispatch host call.

Builds: standard Go as `fcdev fn build` does it (`-trimpath -buildmode=c-shared -ldflags='-s -w'`); TinyGo
`tinygo build -target=wasip1 -buildmode=c-shared -no-debug` with `-opt=z` and `-opt=2` (see §4: **the TinyGo rows
needed a patched SDK to work at all**); Rust `cargo build --release --target wasm32-unknown-unknown`
(`opt-level=s`, LTO, `panic=abort`, strip) with and without `wasm-opt -Os`. TinyGo came from the release tarball into
a scratch directory (no install, no sudo); the Rust target was already present. Nothing touched a running fcdev or
database: `fcdev fn dev` was run from a scratch-built binary on ports 18511-18518 with
`FC_FN_CLI_CREDENTIALS_PATH` in scratch, and each process was stopped by PID.

## 3. Measurements

| guest | scenario | size | compile cold | compile cached | instantiate p50 / p99 | first call | warm p50 / p99 | min mem | mem after 5000 calls |
|---|---|---|---|---|---|---|---|---|---|
| go-std | health | 7.51 MB | 1201 ms | 42 ms | 3.0 ms / 4.2 ms | 2.1 ms | 94 µs / 145 µs | 4.3 MB | 9.8 MB |
| tinygo -opt=z | health | 1.77 MB | 525 ms | 13 ms | 361 µs / 656 µs | 1.1 ms | 698 µs / 883 µs | 0.2 MB | 0.5 MB |
| tinygo -opt=2 | health | 2.37 MB | 786 ms | 18 ms | 318 µs / 425 µs | 1.1 ms | 151 µs / 3.8 ms | 0.2 MB | 1.0 MB |
| rust | health | 0.15 MB | 28 ms | 1.2 ms | 8 µs / 60 µs | 155 µs | 17 µs / 28 µs | 1.1 MB | 1.1 MB |
| rust + wasm-opt | health | 0.13 MB | 28 ms | 1.3 ms | 9 µs / 86 µs | 144 µs | 17 µs / 26 µs | 1.1 MB | 1.1 MB |
| go-std | ingest | 7.51 MB | 1213 ms | 44 ms | 3.7 ms / 4.3 ms | 3.9 ms | 324 µs / 867 µs | 4.3 MB | 10.1 MB |
| tinygo -opt=z | ingest | 1.77 MB | 538 ms | 14 ms | 378 µs / 625 µs | 2.4 ms | 945 µs / 1.2 ms | 0.2 MB | 0.5 MB |
| tinygo -opt=2 | ingest | 2.37 MB | 796 ms | 18 ms | 329 µs / 680 µs | 2.2 ms | 249 µs / 4.1 ms | 0.2 MB | 1.0 MB |
| rust | ingest | 0.15 MB | 29 ms | 1.1 ms | 9 µs / 94 µs | 218 µs | 42 µs / 58 µs | 1.1 MB | 1.1 MB |
| rust + wasm-opt | ingest | 0.13 MB | 29 ms | 1.1 ms | 9 µs / 77 µs | 200 µs | 45 µs / 58 µs | 1.1 MB | 1.1 MB |
| go-std | creator | 7.49 MB | 1205 ms | 42 ms | 2.9 ms / 3.7 ms | 3.0 ms | 235 µs / 510 µs | 4.3 MB | 10.1 MB |
| tinygo -opt=z | creator | 1.76 MB | 528 ms | 14 ms | 390 µs / 705 µs | 2.2 ms | 915 µs / 1.2 ms | 0.2 MB | 0.5 MB |
| tinygo -opt=2 | creator | 2.36 MB | 799 ms | 19 ms | 315 µs / 579 µs | 1.9 ms | 220 µs / 4.0 ms | 0.2 MB | 1.0 MB |
| rust | creator | 0.14 MB | 28 ms | 1.3 ms | 10 µs / 100 µs | 225 µs | 31 µs / 42 µs | 1.1 MB | 1.1 MB |
| rust + wasm-opt | creator | 0.13 MB | 28 ms | 1.0 ms | 11 µs / 133 µs | 391 µs | 31 µs / 43 µs | 1.1 MB | 1.1 MB |

"Cold start of one function in a fresh runner" is compile (cached once the runner has seen the bytes) plus
instantiate plus first call: std Go about 50 ms (1.3 s the very first time), TinyGo about 15-20 ms (0.5-0.8 s first),
Rust about 2 ms (30 ms first). Instances are reused, so instantiate matters when a pool grows, not per request.

Reading the table:

- **Rust** is 50x smaller than std Go, compiles 40x faster, instantiates in ~10 µs and serves a signed ingest in
  ~45 µs at 1.1 MB per instance. It is the right answer when latency or density matter.
- **Standard Go** is the heaviest: 7.5 MB (mostly `net/http`, `database/sql`, `httptest` linked by the SDK), 1.2 s to
  compile once, ~3 ms and 4.3 MB per new instance, ~10 MB resident once warm. Calls are still sub-millisecond with a
  tight p99 (0.15-0.9 ms).
- **TinyGo** is 3-4x smaller and starts 10x faster than std Go, and one instance is 0.5-1 MB instead of 10 MB (about
  10x the density). But `-opt=z` serves calls 3-7x *slower* than std Go (0.7-0.95 ms), and `-opt=2` matches std Go's
  median (150-250 µs) with a p99 of ~4 ms, 5-25x std Go's tail. The tail was identical with `-gc=conservative`,
  `-gc=precise` and the default, so it is TinyGo's collector or asyncify scheduler under this workload; I did not
  isolate it further. `-opt=1` was worse (1.3 ms median).

## 4. TinyGo: what broke or needed care

TinyGo 0.43.0 does compile the unmodified SDK and examples to a valid ABI v1 module (`-target=wasip1
-buildmode=c-shared` exports `_initialize` and the four `fc_*` exports; `//go:wasmimport`/`//go:wasmexport` and the
`wasip1` build tag work; the engine's import check accepts it). Running it showed:

1. **`http.ServeMux` method patterns never match.** `mux.HandleFunc("GET /healthz", ...)` registers, then every
   request is a 404, so *every endpoint the SDK declares (all patterns are `METHOD /path`) returns 404 and the
   function is dead on arrival*, even though `fc_describe` looks right. Plain `/path` and `/path/{name}` patterns
   work; so does `r.PathValue`. A 12-line program reproduces it under `tinygo run` (host), and `tinygo test` on the
   example fails the same way. Also broken: the `{rest...}` multi-segment wildcard, with or without a method.
   Workaround that I verified (kept out of the repo): register path-only patterns with the mux and dispatch on
   `r.Method` in a wrapper (about 20 lines in `registry.go`: a `map[path]map[method]HandlerFunc`, 405 otherwise). With
   it, all three examples pass `tinygo test`, and `hello` passes `TestSDKExamples`. `{rest...}` stays broken, so
   it would need SDK-side routing (as the Rust SDK has).
2. **Reflection is partial.** `encoding/json` was fine in everything I tried (structs, embedded structs, tags,
   `omitempty`, `map[string]any`, `json.RawMessage`, custom `MarshalJSON`, `time.Time`), as were `regexp`,
   `crypto/aes`+`cipher`, `crypto/rand`, `sort`, `slog`. `text/template` panicked (recovered by the SDK as a 500
   `FUNCTION_PANIC`; std Go ran it correctly). Anything leaning on `reflect` method calls is a risk; there is no way to
   know without running it on the wasm.
3. **Goroutines work, `-scheduler=none` does not.** `WaitGroup`, `Mutex`, buffered channels, `time.Sleep` and
   `select` with `time.After` behaved like std Go (via the asyncify scheduler, which costs size and speed). Disabling
   the scheduler fails to build because the SDK's `context.WithDeadline` starts a timer goroutine.
4. **GC.** Heap churn of 100 MB in one call completed with correct results (default collector), and one instance
   stays at 0.5-1 MB. The cost is the p99 tail above.
5. **Panics** are recovered like std Go (`FUNCTION_PANIC` 500), including a nil-map write.
6. **Time zones.** `time.LoadLocation` fails without `import _ "time/tzdata"` on both compilers (std Go: "Bad file
   number"; TinyGo: "unknown time zone"); with the import both work (+0.4 MB).
7. **Tooling.** `fcdev fn build|dev|deploy` hard-codes `go build`; a TinyGo artifact would have to be built by hand
   and published with `fcdev fn publish`, and the dev loop (hot reload) would not use it. The authoring tests run
   under std Go, so they would not catch any of the above for the artifact that actually ships. TinyGo must track Go
   releases (0.43 supports Go 1.27 today, but a new Go release can outrun it) and `-opt` flags change
   behaviour materially (`-opt=z` quadruples call time).
8. **No `net/http` client.** Not hit, because `fn.HTTPClient()` is a `RoundTripper` over the host op, but a function that
   imports a library which dials a socket will not work on either compiler under the ABI.

Standard Go did everything above correctly; its only caveat is size, and that `wasm-opt` cannot validate its output
(already noted in `clients/fn-go/README.md`).

## 5. Recommendation

**Examples and the supported path: standard Go (`GOOS=wasip1`).** It is what `fcdev fn build|dev|deploy` runs, the SDK
and every example work unmodified, the unit tests run the same language semantics as the shipped artifact, and its
costs (7.5 MB, ~3 ms and ~10 MB per instance, sub-ms calls with a tight tail) are acceptable for glue code where the
runner keeps instances warm and caches compiled code on disk. The Go examples therefore carry no TinyGo build target.

**TinyGo: not now.** It would give a real saving (about 4x smaller, 10x less memory per instance, faster start), but
today it needs an SDK change just to route a request, has a 4 ms p99 at the only optimisation level that is fast, cannot
run `text/template`, is not wired into fcdev, and splits the toolchain from the one the tests use. Revisit if instance
density in a runner becomes the constraint *and* the SDK is changed to route without `ServeMux` method patterns
(which would also help std Go: dropping `net/http`/`httptest` from the SDK is the largest lever on its 7.5 MB).

**When a function author needs small or fast, the answer is Rust, not TinyGo:** 0.14 MB, 30 ms to compile, 10 µs to
instantiate, 30-45 µs per call, 1.1 MB per instance, a p99 under 0.15 ms, and no toolchain surprises. Its costs are the
language, no clock or panic recovery in the guest (a panic traps the instance and the runner answers
`FUNCTION_FAILED`), and fewer endpoint options than the Go SDK.

## 6. Caveats

- One machine, darwin/arm64, in-process engine with a fake host: no HTTP, auth or real host capabilities. Linux and
  1-2 vCPU containers (plan phase 0) may shift the ratios, in particular the TinyGo p99.
- TinyGo rows use the patched `registry.go` described in §4.1; the shipped examples do not, and a TinyGo build of
  them today returns 404 for every endpoint.
- `compile cold` is wazero's compiler on an empty cache; `compile cached` is `NewCompilationCacheWithDir` after one
  prior compile in the same directory.
- Memory columns are what the engine's budget charges (linear memory), not process RSS.

## 7. Reproducing

```
# tests
go test ./clients/fn-go/... && cargo test --manifest-path clients/fn-rust/Cargo.toml
for d in hello-go ingest-webhook-go dispatch-demo-go/dispatch-creator dispatch-demo-go/payload-logger; do (cd examples/functions/$d && go test ./...); done
for d in hello-rust ingest-webhook-rust dispatch-demo-rust/dispatch-creator dispatch-demo-rust/payload-logger; do (cd examples/functions/$d && cargo test); done

# build the artifacts, then measure (label=scenario=path, comma-separated; scenarios health|ingest|creator)
(cd examples/functions/ingest-webhook-go && fcdev fn build .)       # function.wasm
(cd examples/functions/ingest-webhook-rust && fcdev fn build .)
FN_MEASURE="go=ingest=$PWD/examples/functions/ingest-webhook-go/function.wasm,rust=ingest=$PWD/examples/functions/ingest-webhook-rust/target/wasm32-unknown-unknown/release/ingest_webhook.wasm" \
  go test ./internal/functions/engine -run TestMeasureGuests -v

# the runner's own check of an SDK example
FN_EXAMPLE_WASM=$PWD/examples/functions/hello-go/function.wasm,$PWD/examples/functions/hello-rust/target/wasm32-unknown-unknown/release/hello.wasm \
  go test ./internal/functions/runner -run TestSDKExamples -v
```
