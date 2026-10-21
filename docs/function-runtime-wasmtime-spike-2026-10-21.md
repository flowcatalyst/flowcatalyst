# wasmtime for function runtimes: spike results (2026-10-21)

*Apple M4 Pro, macOS arm64, wasmtime 49.0.1. The machine was heavily loaded (load average 12-17), so
absolute times are pessimistic and the wazero comparison (measured earlier at lower load) is
approximate: trust the direction, not the last digit. Source: `bench/function-runtimes/wtspike`
(`cargo run --release -- runaway|verify|engine`). Functions are first-party code (owner ruling
2026-10-21).*

## Results

**A runaway compiled guest is killed, cheaply, and nothing is left behind.** A guest in an
unconditional loop with a 250 ms epoch budget (1 ms ticker thread) stopped after about 312 ms with
trap `Interrupt` (the ticker runs slightly slow). The next instance ran normally, three rounds in a
row, and the process went back to idle (CPU 2%, which is the ticker). Compare wazero without its
deadline hook: the call cannot be stopped, it is abandoned, a core burns until restart, and four of
them make the runner unhealthy (`docs/function-runtime-deadlines.md`).

**The QuickJS engine we ship (`engine.wasm`, unchanged) on wasmtime**, same bundle and fixtures as the
other benchmarks, output byte-identical to the reference for W2, W3 and W4:

| Workload | wazero, cooperative (earlier run) | wasmtime, epochs off | wasmtime, epochs on |
|---|---|---|---|
| W2 JSON 2 KB | 89 us | 44 us | 57 us |
| W3 XML 15 KB | 18 ms | 8.9 ms | 10.6 ms |
| W4 5M-iteration loop | 1.0 s | 0.51 s | 0.57 s |

So wasmtime is about 2x faster than wazero for the same QuickJS engine, and epoch interruption costs
15-20% (wazero's per-loop hook cost 3.7-10x). The engine's own in-guest deadline handler was armed
as in production and is unaffected.

## What it means

- Wasmtime gives compiled guests the two things wazero cannot give together: speed (no 8-10x hook)
  and a reliable kill at the deadline with no leaked thread.
- It is also about 2x faster for JS functions on the same engine. It does not close the gap to V8
  (XML 10 ms against 0.35 ms on workerd).
- Not tested: a real ABI-v1 host with host calls, instance pooling, Linux and amd64, the Rust guests
  under a real runner, the monthly upgrade cost.
