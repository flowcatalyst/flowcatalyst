#!/bin/sh
# Builds the shared JS engine and installs it where the runner embeds it.
# Needs Rust's wasm32-wasip1 target and a wasi-sdk (for QuickJS's C sources):
#   WASI_SDK_PATH=/path/to/wasi-sdk ./build.sh
set -eu
: "${WASI_SDK_PATH:?set WASI_SDK_PATH to a wasi-sdk install (github.com/WebAssembly/wasi-sdk)}"
cd "$(dirname "$0")"
export CC_wasm32_wasip1="$WASI_SDK_PATH/bin/clang"
export AR_wasm32_wasip1="$WASI_SDK_PATH/bin/llvm-ar"
export CFLAGS_wasm32_wasip1="--sysroot=$WASI_SDK_PATH/share/wasi-sysroot"
cargo build --release --target wasm32-wasip1
out=../../internal/functions/jsengine/engine.wasm
cp target/wasm32-wasip1/release/fc_js_engine.wasm "$out"
if command -v wasm-opt >/dev/null 2>&1; then
  wasm-opt -Oz --enable-bulk-memory --enable-sign-ext --enable-nontrapping-float-to-int "$out" -o "$out"
fi
ls -l "$out"
