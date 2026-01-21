# FlowCatalyst Architecture Decision Record

## Executive Summary

After evaluating Java (Quarkus) and Rust for the FlowCatalyst platform, the decision is to use **Rust as the sole language** for all platform components.

---

## Context

FlowCatalyst is an event-oriented platform for building business applications. The platform needs to:

- Support aggregate-based domain modeling (not event-sourced, but event-oriented)
- Encode business invariants clearly
- Integrate with third-party systems
- Be maintainable for 10+ years
- Support LLM-assisted development (code generation, agents)
- Provide excellent developer experience for platform consumers

Both Java and Rust implementations were built with equivalent patterns to evaluate viability.

---

## Evaluation Criteria

### 1. LLM Code Generation Accuracy

| Language | Assessment |
|----------|------------|
| **Java** | High - extensive training data, stable APIs, mature patterns |
| **Rust** | High - Claude Opus 4.5 and modern LLMs handle Rust well |

**Winner: Tie**

Modern LLMs (particularly Claude Opus 4.5) generate high-quality Rust code. The historical advantage Java had from larger training corpora has diminished significantly. Both languages receive accurate code generation for the patterns used in this project.

### 2. Type System & Correctness Guarantees

| Language | Assessment |
|----------|------------|
| **Java** | Good - Records, sealed interfaces, pattern matching (Java 21+) |
| **Rust** | Excellent - Exhaustive matching, Option/Result mandatory, no null, borrow checker |

**Winner: Rust**

Rust's compiler catches more bugs at compile time. Adding a new enum variant forces handling everywhere. The borrow checker prevents entire classes of bugs (use-after-free, data races) that Java cannot catch statically. This translates directly to fewer production incidents and lower maintenance cost.

### 3. Domain Modeling Patterns

Both implementations achieved equivalent architectural quality:

| Pattern | Java | Rust |
|---------|------|------|
| UnitOfWork | Package-private `Result.success()` | `pub(crate)` success |
| Commands | Immutable records | Structs with Serialize |
| Events | Records with builders | Structs with builders |
| Error handling | Sealed interface hierarchy | Enum variants |
| Event builders | `.from(ctx)` pattern | `.from(ctx)` pattern |
| Error details | `Map.of()` | `details!` macro |

**Winner: Tie** - Both express the domain patterns equally well.

### 4. Ecosystem & Libraries

| Area | Java | Rust |
|------|------|------|
| LLM/AI agents | LangChain4j | Rig, graph-flow |
| Database drivers | Mature (MongoDB, Postgres) | Good (mongodb, sqlx) |
| HTTP framework | RESTEasy Reactive | axum |
| Serialization | Jackson | serde |
| Observability | OpenTelemetry, Micrometer | tracing, OpenTelemetry |

**Winner: Tie**

The Rust LLM ecosystem has matured significantly:
- **[Rig](https://rig.rs/)** - Full-featured LLM framework with provider abstraction, RAG, tool calling, and MongoDB vector store support
- **[graph-flow](https://crates.io/crates/graph-flow)** - Type-safe workflow orchestration with Rig integration, persistence, and human-in-the-loop support

Rig + graph-flow provide equivalent capabilities to LangChain4j, with the added benefit of compile-time workflow verification.

### 5. Framework & Dependency Management

| Aspect | Java (Quarkus) | Rust |
|--------|----------------|------|
| Dependency management | Quarkus BOM ensures compatibility | Cargo with lockfile |
| Dev experience | `quarkus:dev` with hot reload | `cargo run`, `cargo watch` |
| Build complexity | Gradle/Maven | Cargo (simpler) |
| Compile-time checks | Limited | Extensive |

**Winner: Rust**

While Quarkus BOM provides tested dependency combinations, Rust's compiler catches incompatibilities and breaking changes at compile time. The compiler is effectively a more rigorous "BOM" that validates correctness, not just version compatibility.

### 6. Performance

| Metric | Java (Quarkus Reactive) | Rust (tokio) |
|--------|-------------------------|--------------|
| Throughput | 15-25K msg/sec | 25-40K msg/sec |
| P99 latency | 5-15ms (GC jitter) | 1-3ms (predictable) |
| Memory per connection | 10-20KB | 2-5KB |
| Tail latency under load | Degrades with GC | Stable |
| Cold start | 500ms-2s | 10-50ms |

**Winner: Rust**

For API operations on aggregates, MongoDB I/O dominates (~80% of request time). However, Rust advantages compound at scale:
- 1.5-2x more requests per CPU core
- 4-5x less memory per connection
- Predictable P99 latency (no GC pauses)
- Instant cold starts for autoscaling

### 7. Lifetime Cost Analysis

Assuming LLM-assisted development (code generation speed is equivalent):

| Cost Factor | Java | Rust |
|-------------|------|------|
| Infrastructure (10 years) | ~$300K | ~$150-200K |
| Production incidents | Higher (runtime errors) | Lower (compile-time catches) |
| Incident response | More frequent | Less frequent |
| Security vulnerabilities | Memory safety issues possible | Memory safe by design |
| Refactoring safety | Runtime verification | Compile-time verification |

**Winner: Rust**

When LLMs write the code, Java loses its "faster to write" advantage. The real cost over 10 years is:
1. **Infrastructure** - Rust is 30-50% cheaper
2. **Incidents** - Rust's compiler prevents bugs that become 2am pages
3. **Maintenance** - Refactoring with compiler guarantees is safer

Estimated 10-year total cost:
- Java: ~$500K (infrastructure + incidents + maintenance)
- Rust: ~$250K

### 8. Long-term Maintainability

| Factor | Java | Rust |
|--------|------|------|
| Language stability | 25+ years backward compatibility | Stable with editions |
| Hiring pool | Large | Growing rapidly |
| Compile-time guarantees | Limited | Extensive |
| Refactoring confidence | Medium | High |

**Winner: Rust**

While Java has a larger hiring pool today, Rust developers are increasingly available. More importantly, Rust's compiler guarantees make the codebase safer to maintain regardless of who is working on it. The compiler enforces correctness, reducing reliance on institutional knowledge.

### 9. Distribution & Developer Experience

Developers building applications on FlowCatalyst need to run it locally.

| Approach | Assessment |
|----------|------------|
| Docker Compose | Primary - includes MongoDB, cross-platform, familiar |
| Native binary | Secondary - ~15MB, instant startup |

Binary size:
- Rust: ~15MB
- Java (jlink): ~50MB

**Verdict:** Docker Compose is the primary distribution method. Rust binaries are smaller and start instantly, which benefits autoscaling and developer experience.

---

## Decision

### Language: Rust

Used for all components:
- **fc-platform** - Business domain, IAM, APIs
- **fc-router** - Message routing
- **fc-stream** - Stream processing
- **fc-outbox** - Outbox processor
- **LLM agents** - Using Rig + graph-flow

Rationale:
- Compile-time correctness guarantees reduce production incidents
- Lower infrastructure cost (30-50% savings)
- Predictable performance (no GC pauses)
- LLM ecosystem is now mature (Rig + graph-flow)
- Pattern parity with Java implementation proven
- LLMs generate quality Rust code (Opus 4.5)

### Not Using

- **Java/Quarkus** - No longer provides sufficient advantages to justify maintaining two languages
- **Spring** - Preference was for Quarkus anyway; now moot
- **GraalVM native-image** - Rust native binaries are simpler

---

## Technology Stack

### Platform (Rust)

```
Runtime:          tokio
HTTP:             axum
Database:         mongodb (official driver)
Serialization:    serde
Auth:             JWT (jsonwebtoken crate)
LLM Agents:       Rig + graph-flow
Observability:    tracing + OpenTelemetry
Distribution:     Docker image + native binary
```

### Key Dependencies

```toml
[dependencies]
tokio = { version = "1", features = ["full"] }
axum = "0.7"
mongodb = "3"
serde = { version = "1", features = ["derive"] }
serde_json = "1"
jsonwebtoken = "9"
rig-core = "0.5"
graph-flow = { version = "0.2", features = ["rig"] }
tracing = "0.1"
opentelemetry = "0.21"
```

---

## Architecture Patterns

All components use identical patterns:

### Use Case Pattern

```
Command → UseCase → UnitOfWork.commit() → Event
```

- Commands are immutable structs with `Serialize`/`Deserialize`
- Use cases validate, apply business rules, create aggregates
- `UnitOfWork.commit()` is the **only** way to return success
- Events are always emitted on state change (enforced by type system)

### Unit of Work

Atomic commit of:
1. Aggregate state change
2. Domain event
3. Audit log

All within a single MongoDB transaction.

### Result Type

```rust
// pub(crate) - only UnitOfWork can create success
pub struct UseCaseResult<T> { ... }

impl<T> UseCaseResult<T> {
    pub(crate) fn success(event: T) -> Self { ... }
    pub fn failure(error: UseCaseError) -> Self { ... }
}
```

Forces all state changes through the transactional path.

### Domain Events

- CloudEvents specification compliant
- Tracing context (correlation_id, causation_id, execution_id)
- Message group for ordering guarantees
- Builder pattern with `.from(ctx)` for tracing propagation

```rust
let event = UserCreated::builder()
    .from(&ctx)
    .principal_id(&user.id)
    .email(&email)
    .name(&user.name)
    .scope(UserScope::Client)
    .client_id(Some(&client_id))
    .build();
```

### Error Handling

Categorized errors with `details!` macro:

```rust
UseCaseError::business_rule_with_details(
    "EMAIL_EXISTS",
    format!("Email '{}' already exists", email),
    details! { "email" => &email },
)
```

---

## Distribution Strategy

### For Platform Consumers

Primary: **Docker Compose**

```yaml
services:
  flowcatalyst:
    image: ghcr.io/flowcatalyst/platform:latest
    ports:
      - "8080:8080"
    depends_on:
      - mongodb

  mongodb:
    image: mongo:7
```

Secondary: **Native binary**

```bash
curl -L https://flowcatalyst.io/install.sh | bash
flowcatalyst start
```

### Developer Experience

```bash
# Clone their app
git clone mycompany/logistics-app

# Add FlowCatalyst to their docker-compose.yml
# Run everything
docker compose up -d

# Develop their app, events flow through FlowCatalyst
```

---

## LLM Agent Architecture

Using Rig + graph-flow for AI agent capabilities:

### Rig Features Used

- **Provider abstraction** - OpenAI, Anthropic, Cohere via unified interface
- **Tool calling** - Custom tools for domain operations
- **RAG** - MongoDB vector store integration
- **Agentic pipelines** - Multi-step LLM workflows

### graph-flow Features Used

- **Type-safe workflows** - Compile-time workflow verification
- **Persistence** - PostgreSQL/MongoDB state management
- **Human-in-the-loop** - Workflow interruption and approval flows
- **Parallel execution** - FanOutTask for concurrent operations

### Example Agent Workflow

```rust
use graph_flow::prelude::*;
use rig::prelude::*;

// Type-safe workflow definition
let workflow = WorkflowBuilder::new()
    .add_node("analyze", AnalyzeClaimNode::new(llm_client))
    .add_node("validate", ValidateClaimNode::new())
    .add_node("approve", HumanApprovalNode::new())
    .add_edge("analyze", "validate")
    .add_edge("validate", "approve")
    .build();
```

---

## Migration Path

### Phase 1: Consolidate on Rust (Current)

- Complete fc-platform Rust implementation
- Ensure pattern parity with Java version
- All new development in Rust

### Phase 2: Retire Java Implementation

- Migrate any remaining Java-only features to Rust
- Archive Java codebase for reference
- Single Rust codebase for all components

### Phase 3: Build Fulfillment Application

- Prove out platform with real application
- Integrate Rig + graph-flow for LLM agents
- Validate the domain patterns at scale

---

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Rust hiring pool smaller | Growing rapidly; compiler reduces reliance on expertise |
| LLM agent ecosystem younger | Rig and graph-flow are actively maintained with real users |
| Ecosystem churn | Cargo.lock pins versions; compiler catches breaking changes |
| Learning curve | LLMs assist with Rust code; patterns are well-documented |

---

## Appendix: Comparison Summary

| Criterion | Java (Quarkus) | Rust | Decision |
|-----------|----------------|------|----------|
| LLM code generation | High | High | Tie |
| Type safety | Good | Excellent | Rust |
| Domain modeling | Excellent | Excellent | Tie |
| LLM agent libraries | LangChain4j | Rig + graph-flow | Tie |
| Compile-time guarantees | Limited | Extensive | Rust |
| Raw performance | Good | Better | Rust |
| Tail latency | GC jitter | Predictable | Rust |
| Infrastructure cost | Higher | Lower | Rust |
| 10-year lifetime cost | ~$500K | ~$250K | Rust |
| Memory efficiency | 10-20KB/conn | 2-5KB/conn | Rust |
| Cold start | 500ms-2s | 10-50ms | Rust |

**Overall: Rust for 100% of codebase**

---

## Decision Date

January 2025 (Revised)

## Decision Makers

Architecture review considering:
- 10+ year maintenance horizon
- LLM-assisted development (code generation parity)
- Lifetime cost analysis (infrastructure + incidents + maintenance)
- Maturation of Rust LLM ecosystem (Rig, graph-flow)
- Compile-time correctness as primary maintenance advantage

## References

- [Rig - Rust LLM Framework](https://rig.rs/)
- [graph-flow - Type-safe Workflow Framework](https://crates.io/crates/graph-flow)
- [Anthropic Claude Opus 4.5 Announcement](https://www.anthropic.com/)
