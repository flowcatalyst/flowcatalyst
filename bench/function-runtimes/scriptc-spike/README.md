# scriptc spike (2026-10-21)

Sources for `docs/function-runtime-scriptc-spike-2026-10-21.md`.

- `w3.ts`: the BEVCO XML mapping with `fast-xml-parser`. Builds with scriptc 0.4.0 but fails at startup (see the doc).
- `w3b.ts`: the same mapping with a hand-written 100-line XML parser; runs natively under scriptc and Node.
- `fcmap.ts` + `fcmap.json`: the hand-written version as a scriptc wasm32-wasi library (profile format 1).

Reproduce: `npm i scriptc@0.4.0 @scriptc/types @scriptc/runtime-wasm32-wasi@0.4.0 typescript @types/node` in a scratch
directory with `"type": "module"` in package.json; zig 0.16.0 on PATH (the version scriptc's CI pins);
`SCRIPTC_TARGET=wasm32-wasi scriptc build --lib --profile fcmap.json -o fcmap.wasm`; then
`WASM=fcmap.wasm cargo run --release -- scriptc` in `../wtspike`.
