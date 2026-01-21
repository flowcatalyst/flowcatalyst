# Architecture Decision: Java (Quarkus) vs. Go for FlowCatalyst

**Date:** 29 December 2025
**Context:** Choosing the primary language ecosystem for a dispatch platform capable of handling 10,000 jobs/sec, with requirements for a "Great Developer Experience" (All-in-One distribution) and complex platform features (RBAC, Multi-tenancy).

## 1. Executive Summary

**Recommendation: Use the Java (Quarkus) Ecosystem.**

While Go offers a smaller binary size (37MB vs ~223MB), the **Quarkus + Virtual Threads** architecture provides an equivalent high-concurrency model while significantly accelerating the development of complex "Control Plane" features (Users, Roles, Permissions). The ability to seamlessly switch between an embedded SQLite broker (for DevEx) and SQS (for Production) is more robustly handled in the Java ecosystem.

---

## 2. Architecture Comparison

Both implementations currently use a modern "Thread-per-Group" concurrency model.

### Java (Quarkus + Virtual Threads)
*   **Mechanism:** Uses Java 21 Virtual Threads (`Executors.newVirtualThreadPerTaskExecutor()`).
*   **Concurrency:** Creates a dedicated Virtual Thread per message group. This scales to 100k+ concurrent groups efficiently.
*   **I/O Handling:** Uses standard blocking I/O which the JVM automatically handles by parking virtual threads, effectively providing "reactive" throughput with imperative code simplicity.
*   **Verdict:** Cutting-edge. It eliminates the historical "heavy thread" bottleneck and matches Go's throughput for I/O-bound workloads.

### Go (Goroutines)
*   **Mechanism:** Uses Goroutines and Channels.
*   **Concurrency:** Identical architecture—a goroutine per message group limits concurrency via a semaphore.
*   **Verdict:** Idiomatic and performant, with a smaller memory footprint and faster startup, but lacks the rich ecosystem for complex domain modeling.

---

## 3. Developer Experience (DevEx) & Distribution

### The Size Trade-off (37MB vs 223MB)
**Conclusion:** The size difference should **not** drive the decision.

1.  **Docker Normalization:** In a containerized environment (the standard for platform deployment), the difference between pulling a 20MB layer and a 200MB layer is negligible (seconds).
2.  **Complexity vs. Size:** Optimizing solely for binary size is appropriate for CLI tools, but less so for a server-side Platform.
3.  **GraalVM Native Image:** Provides the "Single Binary" experience (no JVM required on host) similar to Go, validating the DevEx requirement.

### The "All-in-One" Strategy
The platform requires an "It just works" mode (Embedded) and a "Production" mode (SQS).

*   **Embedded Mode (SQLite):**
    *   **Java Strength:** SQLite via JDBC is extremely stable and portable. Virtual Threads handle the blocking SQLite I/O effortlessly.
    *   **Go Weakness:** Embedding SQLite requires CGO (C bindings), which complicates cross-compilation and build pipelines.
*   **Production Mode (SQS):**
    *   **Java Strength (Dev Services):** Quarkus can automatically spin up LocalStack containers during development if no config is found, providing zero-config setup.
    *   **Abstraction:** Java's strict interface patterns (CDI) make supporting dual implementations (SQLite vs SQS) cleaner and less prone to abstraction leaks.

---

## 4. Strategic Advantages of Quarkus

1.  **Platform Velocity:**
    *   Implementing RBAC, OIDC integration, and complex Database interactions (Hibernate/Panache) is significantly faster in Quarkus than writing boilerplate "glue" code in Go.
    *   This is critical for the "Platform" aspect (Users, Apps, Subscriptions).

2.  **Concurrency Safety:**
    *   The use of `java.util.concurrent` (Semaphores, ConcurrentHashMap) is battle-tested.
    *   Implementing complex logic like "Cascading Nacks" and "Batch+Group FIFO" is safer in Java's structured environment than managing complex channel signaling in Go.

3.  **Hybrid Broker Support:**
    *   The architecture supports shipping a binary that defaults to **Embedded SQLite** (instant start, no infra) but switches to **SQS** via environment variables. This satisfies the "Great Developer Experience" goal without sacrificing production scalability.

## 5. Final Verdict

**Stick with Java (Quarkus).**

The architecture is sound, the performance with Virtual Threads matches the requirements, and the ecosystem allows for faster delivery of the complex platform features. The binary size is a negligible cost for these benefits.
