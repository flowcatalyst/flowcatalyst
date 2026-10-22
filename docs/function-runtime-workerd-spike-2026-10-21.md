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

## Part 2: can it run as a function runtime? (same day)

The questions: load versions at runtime, host calls, concurrency, and recovery from a runaway. Files:
`bench/function-runtimes/workerd-spike`. Same machine and caveats as above.

| Question | Result |
|---|---|
| Load code at runtime | **Works.** The `workerLoader` binding (needs `compatibilityFlags = ["experimental"]` and `--experimental`; module names must end in `.js`) loads a Worker from code text presented at runtime, caches it by name, and unloads unused ones. Two versions ran side by side, output byte-identical to the reference. A loaded worker gets only the bindings we hand it (`globalOutbound: null` blocks everything else). |
| Host calls | Must be async `fetch` calls to the runner (a service binding to an external server). **75 us per call** over loopback HTTP to a Node stub, 1.5 ms for 20 calls. Direct calls on QuickJS cost about 1.75 us each. Fine next to a real db or http call; noticeable for chatty logging. |
| Request overhead | A keep-alive round trip through the loader for a tiny request: 138 us p50. |
| Concurrency | **One process uses one core.** Throughput topped out at about 2.2k W3 requests/s with 4 or 8 clients while latency doubled, and the process sat at 99-115% CPU. CPU parallelism needs one process per core. A 20 ms request blocks every other function in that process while it runs. |
| Runaway recovery | Detect (client deadline), SIGKILL, restart, reload one version, first good answer: **29 ms** after the kill. The loss is every in-flight request in that process and its loaded versions (they must be reloaded from the runner's desired state). |

### Reading it

Everything we feared is workable for first-party code: versions load dynamically, recovery is quick, the
bridge is slow only relative to in-process calls. The structural costs are that a runner for it is a
supervisor of several single-threaded processes (one per core at least), that a stuck function takes
its process-mates with it, and that host calls become network calls. Memory per process and the
runner-side supervisor design are the open items.

## Part 3: Linux and amd64 (2026-10-22)

*`workerd` 2026-10-22, `node:24-slim` (glibc) in Docker Desktop on the same M4 Pro, so a Linux VM on
macOS, not bare metal. Script: `bench/function-runtimes/workerd-spike/run-linux.sh` + `test.mjs`.*

| | macOS arm64 (part 1/2) | Linux arm64 (container) | Linux amd64 (emulated) |
|---|---|---|---|
| W2, W3, W4 output vs reference | identical | identical | identical |
| W2 / W3 / W4 per call | 2.8 us / 0.35 ms / 22 ms | 3.1 us / 0.39 ms / 21.6 ms | 8.6 us / 0.77 ms / 27.5 ms (emulation: not a speed result) |
| Idle RSS, one worker | about 85 MB | 92 MB | 147 MB (emulated) |
| W3 requests/s, c=1 / 4 / 8 | about 2.2k peak | 1.7k / 2.0k / 1.9k | not measured |
| Runaway `for(;;){}` | process stuck, no recovery | same: a second request got no answer within 1 s | not measured |
| SIGKILL to first good answer (reload one version) | 29 ms | 74 ms (includes a 50 ms wait) | not measured |

Findings:
- **Linux behaves like macOS.** Same output, same speed, same one-core ceiling, same absence of a CPU
  limit. The decision in the recommendation above is unchanged.
- **Linux needs `ca-certificates`.** On a slim image workerd exits at start with
  `TLS configuration: unexpected error: No CA certificates were loaded from the system`, even for a
  config with no TLS. A workerd image or the fcdev-managed install must carry a CA bundle.
- **amd64 was only run to check that the build and the output are right** (QEMU emulation on an arm64
  host). Its timings say nothing about native amd64; that needs a real amd64 Linux box.
- **Windows is not tested.** The npm package ships `@cloudflare/workerd-windows-64` (same release);
  per the owner, Windows matters only for `fcdev`, which is not code-signed today.
- **Still not measured:** the real host-call bridge (all host calls are stubs), and throughput with one
  process per core.
