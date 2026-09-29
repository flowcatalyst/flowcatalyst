# Function runner: Java, Go and Rust side by side

Status: 2026-09-30. A reading aid, not a new measurement. Every number below was taken from one of the
three sources listed at the end; nothing here was re-run.

## The table

| Metric | Java (JVM, Endive) | Go (wazero) | Rust (wasmtime) | Notes |
|---|---|---|---|---|
| **Wasm guest (Rust code)** | | | | |
| Compile from `.wasm` | 506 ms | 22 ms | 15–44 ms | |
| Instantiate p50 | 214 µs | 20 µs | — | Rust not reported in this form |
| Warm call p50 | 18.7 µs | 8.2 µs (own ABI) | 5.9 µs | Rust and Go each run their own guest |
| Throughput | 341k/s (8 threads) | 283k/s (8 goroutines) | 753k/s (c=64, 14 cores) | Rust used more cores |
| Memory per function | 13.6 MB | 2.75 MB (compiled + 1 instance) | 0.64 MB; 0.05–0.15 MB from `.cwasm` | |
| Reload precompiled | — | 0.74 ms | 0.22 ms | |
| **JS guest** | | | | |
| Warm call p50 | 5.8 ms | 13.9 µs (shared engine); 153 µs (Extism build) | 7.8 µs | Java's JS runs partly interpreted |
| Throughput | 279 calls/s | 421k/s (c=64, shared engine); 35k/s (Extism build) | 739k/s (c=64) | Different concurrency |
| Memory per function | about 50 MB (about 60 fit in a 3 GB heap) | 0.95 MB (engine shared) | 5.7 MB compiled; 0.63 MB from `.cwasm` | |
| First call, fresh instance | — | about 2.5 ms | about 0.8 ms | |
| Functions loadable | about 60 in 3 GB | 100 in 812 MB | 2,000 in about 1.5 GB (Wasm component, idle) | |
| **Java's own HTTP tests (Docker on Mac)** | | | | |
| First call (lean / typical) | 3.2 ms / 58 ms p50 | — | — | |
| I/O throughput (lean, c=1000) | 50.7k req/s | — | — | Directional, not isolated cores |
| Noisy neighbour, p99 | +167% | — | — | Others not measured |
| **Platform qualities (judgement, not measured)** | | | | |
| Compile speed | fast-ish | fast | slow | owner's assessment |
| Disk use and binary size | medium | small | large | owner's assessment |
| Type system for domain modelling | good | weak (guard rails added) | strongest | |
| Safety and reliability | good | good | strongest | |
| Supply chain | good ecosystem | best | good, heavy dependency tree | owner's assessment |
| Readability and hiring | good | best | steep | |
| Function story | weakest | strong | strongest | from the numbers above |

## Read this before comparing rows

- **Different methods.** The Go and Rust rows are in-process spikes on an M-series Mac, with no HTTP or auth
  layer. The Java HTTP rows are Docker Desktop for Mac, and the Java report itself calls its throughput and
  noisy-neighbour numbers directional: the load generator is not cgroup-isolated from the container's CPU quota.
- **Different guests.** The guests differ between runtimes. In the JS rows Go's figure is the shared-engine build
  with its own ABI, and Rust's is per-function QuickJS.
- **Memory is not one quantity.** Rust's 0.63 MB is anonymous footprint when loaded from `.cwasm`; its resident
  size is larger because the mapped code counts there. Java's per-function JS figure is derived from how many
  functions fit in a 3 GB heap.
- **Throughput and cores.** The Rust throughput rows used more cores than the Go run, and the concurrency levels
  differ.
- **Linux.** All three sets were measured on macOS. The Go and Rust plans both schedule a Linux re-run.
- **Judgement rows.** The qualities at the bottom are assessments, not measurements.

## What the numbers say, and what they do not

- A JS function's warm call is small next to the user code it runs. Java's 5.8 ms is the JVM interpreting the JS
  engine; it is rarely the bottleneck for an I/O-bound function.
- Density, meaning how many functions one host holds in memory, is the constraint that actually differs: about
  60 JS functions in a 3 GB JVM heap, against 100 in 812 MB for Go's Extism build and less again with the shared
  engine.
- Rust's wasmtime is faster per call and per reload, especially across the host boundary (the same Extism-ABI
  guest costs 5.9 µs there and 39.6 µs on wazero). Go's own bulk-frame ABI keeps the real paths within about
  1.4–1.8× of Rust.

## Sources

- Go and its comparisons with the JVM and Rust hosts: `docs/function-runner-plan.md` §2 (spikes of 2026-09-28).
- Rust density study: `docs/function-runner-density.md` in the Rust repository; harness under
  `spikes/fnhost-density/`.
- Java function host B1–B5: `docs/function-runner-report.md` ("Performance") in the Java repository; scripts,
  fixtures and raw results in `bench/function-host/`; plan in `docs/spec/function-host-benchmark.md`.
