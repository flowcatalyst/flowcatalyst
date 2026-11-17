# wasmtime-go spike (phase 1 of docs/function-runtime-decision-2026-10-25.md)

Results: `docs/function-runtime-wasmtime-go-spike-2026-11-17.md`.

A separate Go module, so the main module's `go.mod`/`go.sum` do not change. Its module path is
under the main module's (`github.com/flowcatalyst/flowcatalyst-go/bench/...`), so it may import
`internal/functions/...` (Go's `internal` rule is by import path), and `replace` points the main
module at `../../..`.

| Path | What |
|---|---|
| `wtengine/` | the ABI v1 call path on wasmtime-go: module check, fc.call/fc.take, WASI preview1, `_initialize`, JS preload, epoch deadline, store memory limit, outcome errors (the production engine's own `engine.Err*`). Build tag `wasmtime`; `stub.go` is what a build without it gets. |
| `cmd/spike/` | the measurements; each mode runs against the production wazero engine (`internal/functions/engine` through `runtimes.Loader`, untouched) and against `wtengine`, with the same host object. |
| `guests/gospike/` | a raw ABI v1 Go guest for shapes the benchmark guest lacks: an unconditional loop, allocation past the cap, N host calls. |
| `cmd/sizeprobe/` | the smallest program that loads the JS engine: wazero without the tag, wasmtime with it (binary size). |
| `Dockerfile` | Linux build and run (amd64, arm64), with the static-library fix below. |
| `run-linux.sh`, `run-linux-amd64-emulated.sh` | every measurement in the image; output in `results/`. |
| `build-isolation.sh` | question 6 on a copy of HEAD in a scratch directory: `build-isolation.sh <scratch dir> <worktree>`. |
| `results/` | the raw output the results document quotes, per mode and platform. |

## Go 1.27.4 refuses wasmtime-go's libraries

Go 1.27.4's `cmd/go` scans the libraries in every `#cgo LDFLAGS -L` directory and rejects objects
that carry LLVM bitcode (`disallowed LTO section ".llvmbc"`; on macOS `__LLVM,__bitcode`). Every
prebuilt `libwasmtime.a` in wasmtime-go v50.0.0 has such sections, so `go build -tags wasmtime`
fails with the repository's toolchain. Go 1.27.2 (and 1.26.3) has no such check.

- Locally (macOS), the spike is built with `GOTOOLCHAIN=go1.27.2` (in the module cache already).
- In Docker it is built with Go 1.27.4 after stripping the sections from a copy of the module
  (`objcopy --remove-section=.llvmbc --remove-section=.llvmcmd`) and pointing a `-modfile` copy of
  `go.mod` at that copy. Nothing in the worktree changes.

## Build and run locally (macOS)

From this directory:

```sh
(cd ../go-guest && GOOS=wasip1 GOARCH=wasm go build -trimpath -buildmode=c-shared -ldflags='-s -w' -o fcbench-go.wasm .)
(cd guests/gospike && GOOS=wasip1 GOARCH=wasm go build -trimpath -buildmode=c-shared -ldflags='-s -w' -o ../../build/gospike.wasm .)
# ../js/dist/function.js: `npm install && node build.mjs` in ../js (or a copy of a built bundle)
GOTOOLCHAIN=go1.27.2 go build -tags wasmtime -o build/spike ./cmd/spike

./build/spike -mode equiv -engines wazero,wasmtime,wt-armed,wt-noep   # question 1
./build/spike -mode runaway -engines wazero,wasmtime,wt-armed           # question 2 (wazero js-indexof: ~20 s, use -rounds 1 -watch 23s)
./build/spike -mode gcpause -engines wazero,wasmtime                    # question 3 (same note)
./build/spike -mode mem -engines wazero,wasmtime -cap 67108864          # question 4
./build/spike -mode lat -engines wazero,wasmtime,wt-noep -passes 2 -runs 3 -dur 1s   # question 5
./build/spike -mode hostcost -engines wazero,wasmtime                   # question 5, per host call
./build/spike -mode conc -engines wazero,wasmtime -guests qjs -conc 1,14,64          # question 7
./build/spike -mode migrate -engines wazero,wasmtime                    # question 7
./build/spike -mode cold                                                # compile, cache, instantiate (question 8)
./build/spike -mode idle -engines wazero,wasmtime,wt-noep               # idle CPU and epoch tick rate
```

`-cpuprofile` / `-mutexprofile` write pprof profiles (how the wasmtime-go store lock was found).

Engines: `wazero` (production engine), `wasmtime` (epoch deadline only), `wt-armed` (epoch at the
deadline + `-grace`, and the JS engine's own `fc_js_deadline` armed as on wazero), `wt-noep`
(epochs off: prices the checks, stops nothing), `wt-map` (as `wasmtime`, but the host functions
find their state through a `sync.Map` instead of `Caller.Data`). `-tick` is the epoch period (default 1 ms).

Purity check (question 6): `CGO_ENABLED=0 go build ./...` here builds `cmd/spike` as a stub with no
wasmtime in it (`go list -deps ./cmd/spike | grep wasmtime` is empty).

## Docker (Linux)

From the repository root (the build context is the whole repository, read-only):

```sh
docker build -f bench/function-runtimes/wasmtime-go-spike/Dockerfile --platform linux/arm64 -t fc-wtspike:arm64 .
docker build -f bench/function-runtimes/wasmtime-go-spike/Dockerfile --platform linux/amd64 -t fc-wtspike:amd64 .   # emulated on Apple silicon
docker run --rm fc-wtspike:arm64 -mode equiv -engines wazero,wasmtime,wt-armed,wt-noep
docker run --rm fc-wtspike:arm64 -mode runaway -engines wasmtime -rounds 2
./run-linux.sh arm64                  # from this directory: the whole set into results/
./run-linux-amd64-emulated.sh         # the subset that means something under emulation
```

The image's entrypoint is the spike binary, working directory `/spike` with `../js`, `../go-guest`
and `../fixtures` laid out as in the repository. Timings from the amd64 image on an arm64 host
are emulated and not to be trusted; equivalence and deadline behaviour are.
