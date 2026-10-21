# workerd as a JavaScript function runtime: spike results (2026-10-21)

*One dev machine (Apple M4 Pro, macOS arm64, load average 6-8 during the runs, so noisy: trust ratios to
about +-1.5x). `workerd` 2026-10-21 from the npm package, Apache-2.0. The same bundle as
`docs/function-runtime-performance-2026-10-19.md` (`bench/function-runtimes/js/dist/function.js`, real
`fast-xml-parser`), wrapped in a small module that defines the three natives and calls `__fc.handle`.
Nothing in the platform was changed. Scratch files were not committed.*

Context: functions are first-party code (owner ruling 2026-10-21), so the question is speed and
operability, not hostile-code hardening.

## Results

**Equivalence.** W2, W3 and W4 produce byte-identical output to the reference files. W1 and W5 were not
compared (W5's host calls were stubbed to return empty results, so its output differs by design).

**Speed**, per call, steady state, measured by running the handler n times inside one request and
dividing, which cancels HTTP overhead:

| Workload | QuickJS today (after the deadline change) | workerd | Factor |
|---|---|---|---|
| W2 JSON, 2 KB | 89 us | 2.8 us | 32x |
| W3 XML, 15 KB (the BEVCO shape) | 18 ms | 0.35 ms | 51x |
| W4 5M-iteration loop | 1.0 s | 22 ms | 46x |

W4 matches Node on the same machine (22 ms), so workerd is running stock V8 speed. The earlier
`deno_core` run measured W4 at 6.9 ms and I cannot reproduce that figure under either workerd or Node;
treat that earlier number as suspect until someone re-checks it. W1 and W5 are not reported: V8 optimised
the echo away and W5's calls were stubs.

**Operational facts**

| | Result |
|---|---|
| Start | 23 ms from launch to the first 200 |
| Binary | 136 MB (npm darwin-arm64); about 85 MB RSS idle with one worker |
| Per-call CPU limit | **None.** An infinite loop was never stopped. CPU stayed at 100% for 10+ s after the client disconnected, and the worker answered no other request meanwhile. SIGTERM did not stop it promptly. |
| Heap cap | `v8Flags = ["--max-old-space-size=256"]` works only as a process-wide setting. Exceeding it **aborts the whole workerd process** (V8 fatal error), not a catchable error in one function. |
| Off-heap memory | Typed-array allocation is not bounded by that flag: it grew to 2.9 GB until a guard I had set killed the process. |
| Config | The schema has no CPU or time limit option. It has a `workerLoader` binding for loading workers from code at runtime (not tried). |

## What it means

- It delivers the speed. XML is 50x faster than QuickJS and about 4x faster than the same work as a
  Rust guest on wazero with the deadline hook (1.5 ms).
- It does **not** deliver graceful limits. QuickJS in our runner stops a runaway at the deadline by
  itself and treats memory as a budget; workerd does neither. A runner on workerd would need external
  safety nets: a watchdog that kills the process when a call overruns, a container memory limit, and a
  restart on failure. The blast radius of any limit breach is the whole workerd process, so functions
  would be grouped into processes by pool or application and restarted as a unit.
- Process count matters: about 85 MB idle each, so one process per function is expensive and one per
  pool shares fate.
- Adopting it means a second process the runner supervises (and `fcdev` must start and stop), a
  136 MB platform binary, and an SDK adapter from `__fc_call` to bindings. It does not change the
  authoring model.

## Not tested

Throughput under concurrency; the `workerLoader` binding for hot load and unload of versions; how
host calls would be bridged (W5 was stubbed, and a real bridge costs far more than the stub); Linux and
amd64; Windows; whether per-request CPU limits exist in a build other than the npm one.

## Recommendation

Keep QuickJS-in-wazero as the default JS runtime. Offer workerd as an opt-in `runtime` for the few
CPU-heavy first-party functions, only with process-level supervision, or send those functions to Rust.
Before building either, try the cheaper idea: host-side `xml.parse` for QuickJS, which keeps the
graceful limits and might reach the 1-2 ms range.
