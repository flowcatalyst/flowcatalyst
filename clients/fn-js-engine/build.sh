#!/bin/sh
# Builds the shared JS engine and installs it where the runner embeds it.
# Needs Rust's wasm32-wasip1 target and a wasi-sdk (for QuickJS's C sources):
#   WASI_SDK_PATH=/path/to/wasi-sdk ./build.sh
set -eu
: "${WASI_SDK_PATH:?set WASI_SDK_PATH to a wasi-sdk install (github.com/WebAssembly/wasi-sdk)}"
cd "$(dirname "$0")"
export CC_wasm32_wasip1="$WASI_SDK_PATH/bin/clang"
export AR_wasm32_wasip1="$WASI_SDK_PATH/bin/llvm-ar"
# The paths the compilers bake into the binary (QuickJS's assert strings, Rust
# panic locations) are remapped, so the engine is the same bytes wherever it
# is built and names nobody's home directory.
here=$(pwd)
export CFLAGS_wasm32_wasip1="--sysroot=$WASI_SDK_PATH/share/wasi-sysroot -ffile-prefix-map=$here=."
export RUSTFLAGS="--remap-path-prefix=$here=. --remap-path-prefix=${CARGO_HOME:-$HOME/.cargo}=/cargo --remap-path-prefix=${RUSTUP_HOME:-$HOME/.rustup}=/rustup"
# rquickjs-sys's build script reads WASI_SDK, not the variables above; without
# it, it downloads its own wasi-sdk (24) from GitHub on every fresh build. The
# compile uses the CC_/CFLAGS_ ones either way.
export WASI_SDK="$WASI_SDK_PATH"
cargo build --release --target wasm32-wasip1
out=../../internal/functions/jsengine/engine.wasm
cp target/wasm32-wasip1/release/fc_js_engine.wasm "$out"
if command -v wasm-opt >/dev/null 2>&1; then
  wasm-opt -Oz --enable-bulk-memory --enable-sign-ext --enable-nontrapping-float-to-int "$out" -o "$out"
fi
ls -l "$out"
