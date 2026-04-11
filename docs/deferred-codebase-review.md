# Deferred: Codebase Duplication & Abstraction Review

## Context
During the session on 2026-04-11, a full codebase review for duplicated code,
wrong abstractions, and consolidation opportunities was requested but deferred
to first fix the stream processor / projection pipeline.

## Scope
Review the Rust FlowCatalyst codebase for:

1. **Duplicated code** — repeated patterns across repositories, handlers, config
   parsing, entity/DTO conversions, and query patterns that could be consolidated.

2. **Wrong abstractions** — traits that are too generic or too specific, forced
   patterns that don't fit (e.g., UseCase trait where it doesn't belong), layers
   that just pass through, EntityType enum containing non-entity types.

3. **Inconsistencies** — mixed SeaORM/SQLx usage, different error handling
   approaches, inconsistent naming, mixed approaches to the same problem.

Focus crates: fc-platform, fc-common, fc-sdk, fc-router, fc-outbox, fc-stream.

## Resume
Pick this up after the stream processor simplification is complete.
