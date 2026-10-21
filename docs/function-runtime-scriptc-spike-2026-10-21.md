# scriptc (TypeScript to WebAssembly): spike results (2026-10-21)

*scriptc 0.4.0 (Vercel Labs, Apache-2.0, "experimental"), zig 0.16.0 (the version scriptc's CI pins),
wasmtime 49.0.1, Apple M4 Pro, machine loaded (load average 4-17), so trust ratios, not last digits.
Sources: `bench/function-runtimes/scriptc-spike`, harness `bench/function-runtimes/wtspike`. Functions are
first-party code (owner ruling 2026-10-21).*

## The question

Can TypeScript functions be compiled to fast Wasm instead of running on QuickJS? Test case: the BEVCO
15 KB XML mapping, same input and expected output as every other benchmark.

## Results

**1. The real mapping (`fast-xml-parser`) does not run yet.** `scriptc coverage`: 93% of statements static,
44 sites deferred to runtime. It builds, but the program fails at startup on the first deferred site that
executes. Each dependency problem I patched locally (three in `xml-naming`, one in
`path-expression-matcher`) uncovered another in `is-unsafe`; I stopped there. The failures are scriptc
limits on constructs in the packages (optional destructuring parameters, default exports of classes,
assignment to non-variables). This matches the brief's finding for `pg`: builds, then fails at startup.

**2. A purpose-built parser compiles fully and is fast.** A 100-line strict-TypeScript XML parser plus the same
mapping: 100% of statements static, output byte-identical to the reference.

| Where it runs | Per call, 15 KB XML | Notes |
|---|---|---|
| Node (V8), hand parser | 60 us | |
| scriptc native binary, hand parser | 83 us | 346 KB binary |
| **scriptc Wasm on wasmtime**, hand parser | **160 us** (195 us with epoch deadline on) | 302 KB module, 17 MB initial memory, byte-identical |
| QuickJS engine on wasmtime, hand parser | 2.2 ms (2.8 ms epochs on) | same TypeScript, bundled with esbuild |
| QuickJS engine on wasmtime, `fast-xml-parser` | 8.9 ms (10.6 ms epochs on) | |
| QuickJS engine on wazero (cooperative), `fast-xml-parser` | 18 ms | what ships today |
| workerd (V8), `fast-xml-parser` | 0.35 ms | see the workerd spike |

## What it means

- The library is a big part of the slowness: the same QuickJS engine is 4x faster with a leaner parser
  (8.9 to 2.2 ms on wasmtime). Part of "QuickJS is slow" is "fast-xml-parser is heavy".
- scriptc Wasm is another 14x faster than QuickJS with the same parser (2.2 ms to 0.16 ms) and within
  about 2.5x of Node's JIT, in a 302 KB module that wasmtime can stop at a deadline.
- It is not usable as a general TypeScript path today: npm packages are the weak spot. A function that
  depends on a package scriptc cannot run needs that package replaced with code we write. For first-party
  adapters with a handful of dependencies that may be acceptable; it is a real constraint to put in front
  of authors.
- Calling convention: scriptc exports use `(ptr,len)` inputs with output slots and its own
  `scriptc_alloc`, plus `wasi_snapshot_preview1` imports and `scriptc.panic`. ABI v1 uses
  `fc_alloc`/`fc_handle`/`fc.call`. A small adapter module (or a runner that understands both) is needed.
  Not built.

## Not tested

wazero with this module (the vendor reports it works); host calls through scriptc callbacks (they are
synchronous scalars only, so our `fc.call` frames would need an adapter); instance start-up time;
deadline behaviour of a runaway scriptc module (epochs would stop it like any Wasm); other npm packages;
the `--dynamic` tier's effect on speed (1,253 statements of the `fast-xml-parser` build were in it).
