# The Rust SDK (`clients/rust-sdk`)

`flowcatalyst-sdk` is the Rust member of the SDK family (Go `pkg/fcsdk`, Java,
TypeScript, Laravel). Created 2026-10-12 by owner decision 11 of the Rust
framework design (`flowcatalyst-framework/docs/rust/design.md`): a crate in this
repository, seeded from the unpublished `flowcatalyst-rust/crates/fc-sdk`
(Apache-2.0) with the wire shapes fixed, released by this repository's tooling
like the other SDKs. The framework's `fw-flowcatalyst` crate is meant to be its
only framework dependant.

The crate's README is the detailed record: the feature set, every change from
fc-sdk, and the wire-contract ambiguities found. This page covers how it fits
the repository.

## Layout and checks

| Path | What |
|---|---|
| `clients/rust-sdk/src` | the crate (edition 2024, MSRV 1.98, toolchain pinned 1.98.1 by its `rust-toolchain.toml`) |
| `clients/rust-sdk/migrations` | the Java SDK's `outbox_messages` DDL, byte for byte |
| `clients/rust-sdk/tests/fixtures/wire` | golden payloads (event, dispatch job, audit), signature and TSID vectors |
| `clients/rust-sdk/conformance/wire_test.go` | Go test in this module: decodes the golden payloads with the platform's own types (`sdk.BatchItem` strictly, `eventapi.BatchEventItem`, `sdk.AuditBatchItem`), checks the vectors with the Go SDK, and checks the DDL and redaction vectors against the Java and Go SDK files |
| `clients/rust-sdk/Makefile` | `make ci` (fmt, clippy `-D warnings`, tests, the Go conformance test, `cargo deny`), `make test-db-up` / `test-db` / `test-db-down` (throwaway Postgres and MySQL containers on 55433/55434 for the sqlx drivers) |
| `clients/rust-sdk/deny.toml` | the platform's `deny-sdk.toml` licence policy (no AGPL/GPL/LGPL), plus advisories, bans, sources |

Because the conformance test is an ordinary Go test, `go test ./...` (and so
`make ci`) fails when a platform change breaks a payload the Rust SDK writes,
and the Rust tests fail when the SDK stops writing the golden bytes.

## Releasing (not wired yet)

Nothing publishes the crate yet (`publish = false`). To add the release line
the way `fn-rust` has one:

1. `scripts/release.sh`: a `rust` kind with `prefix="rust-sdk"`,
   `version_file="clients/rust-sdk/VERSION"`,
   `manifest_toml="clients/rust-sdk/Cargo.toml"`,
   `lock_cargo="clients/rust-sdk/Cargo.lock"`, `lock_cargo_crate="flowcatalyst-sdk"`
   and a split workflow name.
2. `Makefile`: `release-rust-sdk: ## … @scripts/release.sh rust "$(BUMP)"` and a
   `build-rust-sdk` target running `make -C clients/rust-sdk ci`.
3. A `split-rust-sdk.yml` workflow (copy `split-fn-rust-sdk.yml`) mirroring to a
   standalone repository and, behind a repository variable, `cargo publish`.
4. Remove `publish = false` and settle the crate name on crates.io (framework
   decision 19).

## Open questions for the owner

1. ~~Event `data` and audit `operationData` embedded as JSON strings~~:
   resolved by unwrapping at ingest (owner ruling 2026-10-12,
   `docs/platform-bug-fixes.md` F13; the rule is in `docs/wire-contract.md`).
2. ~~The Go SDK's `outboxpgx` audit rows are snake_case~~: fixed (F13).
3. Principal sync: ~~Go applies `passwordHash` to existing users and never
   sends `passwordHashIgnored`~~ (fixed, F13); all SDK docs claimed the
   platform prefixes bare role names, which Go does not (this SDK now
   prefixes them itself).
4. The dispatch-jobs batch does not resolve `clientCode` (design question
   for the owner, F13 in `docs/platform-bug-fixes.md`).
5. `docs/wire-contract.md` names `tests/golden/webhook/`, which does not exist.
6. `event-minimal.json` has no `source`, which `/api/events/batch` requires
   (that item is refused `BAD_REQUEST` "source is required").

## The Rust platform

`flowcatalyst-rust` (paused except for its bug-fix list) should later depend on
this crate instead of its own `fc-sdk` for everything an application links:
`fc-sdk` still carries the outbox DDL, dispatch-job and webhook divergences
fixed here, and two Rust SDKs would drift. What stays platform-side is what
this crate dropped on purpose (use cases, cache, locks, axum session layer,
scheduled-job runner); those belong in `fc-sdk` (or the framework) on top of
this crate. Nothing in that repository was changed.
