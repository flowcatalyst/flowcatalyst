# Architecture

Internal architecture of FlowCatalyst. Audience: engineers who work on the platform.

## Read first

| Topic | Doc |
|---|---|
| C4 L1/L2 overview, domain model, ID format | [system-overview.md](system-overview.md) |
| Workspace structure: every crate, every binary | [shared-crates.md](shared-crates.md) |

## Per-component deep-dives

| Component | Doc |
|---|---|
| Message router (especially thorough) | [message-router.md](message-router.md) |
| Dispatch scheduler | [scheduler.md](scheduler.md) |
| Stream processor (projections + fan-out + partition manager) | [stream-processor.md](stream-processor.md) |
| Outbox processor (application-side) | [outbox-processor.md](outbox-processor.md) |
| Platform control plane (DDD, UoW, BFF vs API) | [platform-control-plane.md](platform-control-plane.md) |
| Auth, OIDC bridge, tokens, tenancy | [auth-and-oidc.md](auth-and-oidc.md) |

## Cross-cutting

| Topic | Doc |
|---|---|
| Monthly partitioning, in-Rust manager, retention | [partitioning.md](partitioning.md) |
| Adaptive concurrency design (Vegas) — not yet shipped | [adaptive-concurrency.md](adaptive-concurrency.md) |
| Long-term architecture direction (control-plane pattern, OIDC scope, scheduled tasks, distributed configs) | [architecture-direction.md](architecture-direction.md) |
| Release artefact signing (Linux, planned macOS/Windows) | [release-signing.md](release-signing.md) |

## Related

- [Operations](../operations/) — deployment, config, secrets, HA, runbooks.
- [Developers](../developers/) — building applications against the platform.
- `CLAUDE.md` (repo root) — development conventions; UoW seal; HTTP tier rules; permission check naming.
