#!/bin/sh
# Builds every benchmark artifact and host, then runs the whole suite serially.
# Run from the repo root:  bench/function-runtimes/run.sh > results.txt
# Needs: go, node/npm, cargo (+ wasm32-unknown-unknown), tinygo (optional).
set -eu
B=bench/function-runtimes
OUT=${OUT:-/tmp/fcbench}; mkdir -p "$OUT"
node $B/fixtures/gen.mjs >&2
(cd $B/js && npm install --silent && node build.mjs >&2 && node check-node.mjs >&2)
(cd $B/rust-guest && cargo build --release --target wasm32-unknown-unknown >&2
 CARGO_PROFILE_RELEASE_OPT_LEVEL=3 cargo build --release --target wasm32-unknown-unknown --target-dir "$OUT/rs-o3" >&2)
(cd $B/go-guest && GOOS=wasip1 GOARCH=wasm go build -trimpath -buildmode=c-shared -ldflags='-s -w' -o fcbench-go.wasm . >&2
 if command -v tinygo >/dev/null; then tinygo build -target=wasip1 -buildmode=c-shared -opt=2 -no-debug -scheduler=none -o fcbench-tinygo.wasm . >&2; fi)
(cd $B/rusthost && CARGO_TARGET_DIR="$OUT/rh" cargo build --release >&2)
go build -o "$OUT/wzb" ./$B/wazerobench
go build -o "$OUT/loopcfg" ./$B/wazerobench/loopcfg

JS=$B/js/dist/function.js
RS=$B/rust-guest/target/wasm32-unknown-unknown/release/fcbench.wasm
RS3=$OUT/rs-o3/wasm32-unknown-unknown/release/fcbench.wasm
GO=$B/go-guest/fcbench-go.wasm
TG=$B/go-guest/fcbench-tinygo.wasm
ARTS="qjs=js=$JS,rust=wasm=$RS,rust-o3=wasm=$RS3,go=wasm=$GO,tinygo=wasm=$TG"

echo "# machine: $(sysctl -n machdep.cpu.brand_string) ncpu=$(sysctl -n hw.ncpu) $(uname -sr) $(go version)"
echo "## cold"; "$OUT/wzb" -mode cold -a "$ARTS"
"$OUT/rh/release/v8bench" cold --script $JS
"$OUT/rh/release/wtbench" cold --wasm $RS --label wasmtime
echo "## mem"
for a in $(echo "$ARTS" | tr ',' ' '); do "$OUT/wzb" -mode mem -a "$a"; done
"$OUT/rh/release/v8bench" mem --script $JS
"$OUT/rh/release/wtbench" mem --wasm $RS --label wasmtime
echo "## lat"
for a in $(echo "$ARTS" | tr ',' ' '); do "$OUT/wzb" -mode lat -a "$a"; done
"$OUT/rh/release/v8bench" lat --script $JS
"$OUT/rh/release/wtbench" lat --wasm $RS --label wasmtime
echo "## deadline hook (CloseOnContextDone) on/off"
"$OUT/loopcfg" qjs=$JS rust=$RS rust-o3=$RS3 go=$GO tinygo=$TG

# Optional extras, each its own Go module (they pull third-party engines, so the platform's go.mod is untouched):
#   (cd $B/gojabench && go run . )                      # goja, pure-Go interpreter
#   (cd $B/v8gobench && CC=<clang>=21 CXX=<clang++>=21 CGO_CXXFLAGS=-nostdinc++ go run . -conc 1)   # V8 via cgo (tommie/v8go); Apple clang cannot build it
# Real-bundle equivalence smoke (describe + one request, QuickJS vs V8, must print identical text):
#   $OUT/wzb -mode smoke -a q=js=<function.js> -body f.xml -path /deliver -method POST -caller webhook
#   SMOKE=1 $OUT/rh/release/v8bench smoke --script <function.js> --body f.xml --path /deliver --method POST --caller webhook
# Global/feature probe: js/probe.js through the same two smoke commands.
