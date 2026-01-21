# FlowCatalyst Rust Architecture Proposal

## 1. Executive Summary

FlowCatalyst's current Quarkus Native implementation results in a **233MB binary**. While functional, this footprint is suboptimal for distributed agents, sidecars, and cross-platform CLI tools. 

By migrating the core services to **Rust**, we aim to:
*   **Reduce Binary Size:** Target < 20MB for production binaries (a 90%+ reduction).
*   **Improve Efficiency:** Achieve near-instant startup (< 5ms) and ultra-low idle memory (< 10MB).
*   **Simplify Cross-Compilation:** Enable single-machine builds for Mac (ARM/X64), Linux (ARM/X64), and Windows (ARM/X64) using the Rust toolchain.
*   **Modularize Distribution:** Use a Cargo Workspace to produce a "Dev Monolith" for local development and "Specialized Binaries" for production.

---

## 2. Workspace Structure

We will utilize a **Cargo Workspace** to maximize code reuse while allowing distinct binary entry points.

```text
flowcatalyst-rust/
├── Cargo.toml                  # Workspace & Shared Dependencies
├── crates/                     # SHARED LIBRARIES (Crates)
│   ├── fc-common/              # Shared Types (Message, PoolConfig), Logging, Metrics
│   ├── fc-queue/               # Queue Traits & Impls (SQS, SQLite, NATS)
│   ├── fc-router/              # Core Routing Engine (Process Pools, Mediators)
│   ├── fc-api/                 # Axum Handlers & Middleware
│   ├── fc-outbox/              # Outbox Polling Logic
│   └── fc-stream/              # Change Stream Processor Logic
└── bin/                        # EXECUTABLES
    ├── fc-dev/                 # The Developer Monolith (Monolith of all modules)
    ├── fc-router/              # Production Message Router
    ├── fc-outbox/              # Production Outbox Poller
    └── fc-stream/              # Production Stream Processor
```

---

## 3. Core Engine Design (Message Router)

### Actor-Based Concurrency
The Java implementation uses `ConcurrentHashMap` and virtual threads. The Rust implementation will move to an **Actor-like model** using **Tokio Channels (MPSC)**:

1.  **Consumer Actors:** Tasks that poll SQS or SQLite and push `QueuedMessage` into a central `IngestionChannel`.
2.  **Manager Actor:** A single lightweight task that reads from `IngestionChannel`, identifies the target `PoolCode`, and routes the message to the appropriate `PoolChannel`.
3.  **Process Pool Actors:** Each pool manages a set of workers. Workers use a `tokio::sync::Semaphore` to enforce concurrency limits and `tokio::time::sleep` for rate limiting.

### Queue Abstraction (The "Dev" Mode Strategy)
To support both SQS and SQLite (Embedded) seamlessly, we use an async trait:

```rust
#[async_trait]
pub trait QueueConsumer: Send + Sync {
    async fn poll(&self, max_messages: u32) -> Result<Vec<QueuedMessage>>;
    async fn ack(&self, message_id: &str, handle: &str) -> Result<()>;
    async fn nack(&self, message_id: &str, handle: &str) -> Result<()>;
}
```

*   **Production:** The `SqsConsumer` uses the `aws-sdk-sqs` crate.
*   **Development:** The `SqliteConsumer` uses `sqlx` to execute SQL queries that mimic SQS FIFO and Visibility Timeout logic.

---

## 4. The Developer Monolith (`fc-dev`)

The `fc-dev` binary is designed for zero-dependency local development. It will statically link all functional modules:

*   **Embedded Database:** Uses `sqlx` with an in-memory or local file SQLite database.
*   **Integrated API:** Runs the Axum web server providing the Platform API.
*   **Internal Routing:** The Outbox Poller, Stream Processor, and Message Router all run as concurrent Tokio tasks within the same process.
*   **Zero-Config:** Automatically initializes database schemas on startup.

---

## 5. Technology Stack

| Component | Technology | Reason |
| :--- | :--- | :--- |
| **Runtime** | `Tokio` | The industry-standard async executor for Rust. |
| **Web / API** | `Axum` | High performance, uses `Tower` middleware ecosystem. |
| **Serialization** | `Serde` / `Serde_json` | Type-safe, high-speed JSON handling. |
| **Database** | `sqlx` | Async, type-safe SQL (supports SQLite, Postgres, MySQL). |
| **AWS SDK** | `aws-sdk-sqs` | Official AWS SDK for Rust. |
| **Observability** | `metrics` / `tracing` | Standard facades for Prometheus and structured logging. |
| **Security** | `jsonwebtoken` / `rustls` | Native Rust implementations for JWT and TLS (no OpenSSL dependency). |

---

## 6. Cross-Platform Build Strategy

Rust makes targeting multiple architectures straightforward:

*   **Tools:** Use `cargo-zigbuild` or `cross` to cross-compile from a single OS.
*   **Linking:** Statically link `musl` on Linux to ensure binaries run on any distribution (Alpine, Ubuntu, etc.) without shared library issues.
*   **Architectures:**
    *   `aarch64-apple-darwin` (Mac ARM)
    *   `x86_64-apple-darwin` (Mac Intel)
    *   `aarch64-unknown-linux-musl` (Linux ARM)
    *   `x86_64-unknown-linux-musl` (Linux X64)
    *   `aarch64-pc-windows-msvc` (Windows ARM)
    *   `x86_64-pc-windows-msvc` (Windows X64)

---

## 7. Implementation Phasing

1.  **Phase 1 (Scaffold):** Initialize Workspace, define shared data models (`fc-common`), and implement the `QueueConsumer` trait.
2.  **Phase 2 (Embedded Mode):** Build the `SqliteConsumer` and the basic `fc-router` engine logic.
3.  **Phase 3 (Dev Monolith):** Create `fc-dev` binary combining the Router and a mock API layer.
4.  **Phase 4 (Production Ready):** Implement `SqsConsumer`, Prometheus metrics, and structured logging.
5.  **Phase 5 (Specialization):** Extract Outbox and Stream processing logic into their own crates/binaries.
