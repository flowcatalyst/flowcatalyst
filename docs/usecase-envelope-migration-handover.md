# Use-Case Envelope Migration — Handover

**Status as of 2026-06-25:** tree is fully green and **uncommitted**. **The entire Go side is
done** — every platform write-module, all 8 `Sync*` ops, the scheduledjob CRUD (§5.1+§5.2),
**and the §5.4 cleanup** (dead `commit` pkg deleted, vestigial `usecase.UseCase`/`Run` +
order-service example deleted, `internal/sealed` relocated under `pkg/fcsdk/internal/`,
analyzer confirmed wired into CI). The ONLY remaining work is the TS/PHP SDK ports (§5.3).
This doc is self-contained — a fresh session should be able to resume from it + the referenced files.

Related memory: `usecase-envelope-refactor.md`, `feedback-usecase-is-domain-operation.md`.

---

## 1. What this is

We replaced the original Rust-mirrored `usecase.UseCase`/`Run` + `usecase.Result` machinery
(and the bare `commit.Save` plain-function style) with a single **enforced use-case envelope**
so that **every command (write) operation** goes through one uniform pipeline:

```
Validate  → command shape (pure, no DB)
Authorize → resource-level authorization (see the authz model below)
Execute   → invariant checks (loads, business rules) → returns a Plan describing the change
────────── the Run driver then: persist aggregate(s) + domain event(s) + audit log, atomically
```

A use case models **one domain/business operation** — NOT "one write". It may persist several
aggregates and emit several events in one transaction. "Write" is infrastructure vocabulary;
keep it out of domain-layer docs/naming. (Owner feedback — see the feedback memory.)

### Core package: `pkg/fcsdk/usecaseop/`
- `doc.go` — the contract + rationale.
- `operation.go` — `Operation[C,E]{Name, Validate, Authorize, Execute}` + `Run(ctx, uow, op, cmd, ec)` + `Public[C]`.
- `plan.go` — `Plan[E]` (sealed interface; unexported `apply` method = the seal) + constructors `Save`/`Delete`/`Emit`/`SaveAll`/`Sync`.
- `orchestration.go` — `TxOperation[C,R]{...Execute(ctx, s *usecasepgx.TxScopedUnitOfWork, cmd, ec) (R, error)}` + `RunTx(...)` for **multi-aggregate** ops that emit several events / return a custom result.

Supporting:
- `pkg/fcsdk/usecasepgx/run.go` — added `RunErr` (commit-on-nil-error tx) used by `RunTx`.
- `tools/analyzer/uowseal/analyzer/analyzer.go` — `go vet` analyzer; type-resolved check that **every `Operation`/`TxOperation` composite literal sets `Authorize`** (`Public` counts). Run via `make analyze` (currently NOT wired into `make lint`/CI — a cleanup item).

The seal: `Execute` returns a `Plan` (sealed); only `Run`/`RunTx` can apply it. So an operation
cannot reach the DB except through the driver, which always writes event+audit atomically.

---

## 2. THE LOCKED AUTHORIZATION MODEL (most important — owner-confirmed)

- **Controller (HTTP handler / entry point)** does the **coarse permission** check
  (`auth.CanCreateX`/`CanWriteX`/`CanDeleteX`, which resolve to `HasPermission`; or
  `auth.RequireAnchor` for platform-owned resources). It builds the command DTO and calls
  `usecaseop.Run`/`RunTx`. Reads do NOT go through use cases.
- **Use case (`Authorize` phase, or post-load in `Execute`)** does the **resource-level**
  check — "may this principal act on THIS resource?" via `auth.CheckScopeAccess(ac, resource.ClientID)`
  (nil ClientID ⇒ cross-client ⇒ anchor required; non-nil ⇒ must access that client), ownership,
  or state-based rules. `usecaseop.Public[C]` when the resource has no client/per-instance dimension.

Placement detail (you can't authorize a resource you haven't loaded):
- **create** → resource check in `Authorize` against `cmd.<ClientID/AppID>`.
- **update/delete/status** → resource check in `Execute`, right after `repo.FindByID` + not-found.

Invariant (grep-enforced): **no coarse `auth.Can*`/`RequireAnchor` may appear in any
`internal/platform/*/operations/*.go` file** — only `CheckScopeAccess`/`RequireUserAdmin`/`Public`.

Shared/unauthenticated entry points: when one op is reached by multiple entry points with
different gating (e.g. `principal.CreateUser` from admin API **and** login-JIT; `ResetPassword`
from admin **and** the unauthenticated password-reset flow), the op's `Authorize` is `Public`
and each entry point keeps its own gating. Baking an admin check into such an op would break
the system/unauthenticated flow.

---

## 3. Reference implementations (copy these patterns)

| Pattern | Files |
|---|---|
| **CRUD, client-scoped** (canonical) | `internal/platform/connection/operations/{create,update,delete}.go`, `internal/platform/connection/api/api.go`, `internal/platform/connection/operations/ops_pg_test.go` (note `TestCreateConnection_ResourceScope`) |
| **Sync** | `internal/platform/process/operations/sync.go`, the `runProcessSync` handler in `internal/platform/sdksync/api.go`, sync tests in `internal/platform/process/operations/ops_pg_test.go` |
| **Orchestration (TxOperation/RunTx)** | `internal/platform/application/operations/provision_service_account.go`, `internal/platform/serviceaccount/operations/create_credentials.go` |
| **Global, no client dimension** (`Public`) | `internal/platform/role/`, `internal/platform/process/` (CRUD) |
| **Platform-owned (anchor)** | `internal/platform/cors/`, `internal/platform/identityprovider/` |
| **Self-service `Public` + ownership in handler** | `internal/platform/webauthn/` |

Test/auth helpers (do not duplicate): `internal/testpg/uow.go` (`AnchorCtx()`, `WithAuth(ctx, *auth.AuthContext)`, `TestEC()`, `NewUoW()`); `internal/platform/shared/auth/auth.go` (`NewExecutionContext(ctx)`, `CheckScopeAccess`, `CanAccessApplication`, the `Can*` helpers).

---

## 4. What's DONE (verified green)

All on the envelope, controllers do coarse / use cases do resource-level, integration tests pass:

- Wave 1: connection (ref), client, cors, emaildomainmapping, identityprovider, platformconfig, webauthn.
- Cluster A (CRUD): eventtype, role, subscription, dispatchpool, process.
- Cluster B: application (+ `ProvisionServiceAccount` on `RunTx`), serviceaccount (+ `CreateServiceAccountWithCredentials` on `RunTx`), auth, principal (security-critical; hand-reviewed).
- **All 8 `Sync*` ops (§5.1, done 2026-06-25):** process (the reference), eventtype
  (`SyncEventTypes`), role (`SyncRoles` + `SyncPlatformRoles`), subscription
  (`SyncSubscriptions`), dispatchpool (`SyncDispatchPools`), scheduledjob (`SyncScheduledJobs`),
  principal (`SyncPrincipals`), openapispecs (`SyncOpenApiSpec`). See the authz table below.
- **scheduledjob CRUD (§5.2, done 2026-06-25):** Create/Update/Pause/Resume/Archive/Delete +
  FireNow on the envelope; create authorizes against `cmd.ClientID`, the by-id ops do
  post-load `CheckScopeAccess`, FireNow keeps its two-phase shape (direct instance insert +
  `usecaseop.Emit`). The old `requireScopeByID` handler helper is gone (the check is in the
  use cases now). openapispecs is sync-only — no separate CRUD.
- Cross-module callers rewired: `auth/bridge/login_endpoint.go`, `passwordreset/api`, `client/api`,
  `sdksync/api.go` (all 8 sync handlers), `bff/event_types.go` + `bff/roles.go` + `bff/developer.go`
  (the bff sync handlers), `principal/api/sync.go` (platform user-sync), + test seeds in
  `openapispecs`/`sdksync`/`scheduledjob/scheduler`.

### Sync-op authorization (the §2 model applied per entry-point shape)
| Op | `Authorize` | Why |
|---|---|---|
| `SyncEventTypes` | `Public` | Two entry points, different gating: sdksync (app-scoped) + bff platform-catalogue (anchor-only, `"platform"` pseudo-code). Each controller keeps its gate. |
| `SyncRoles` | `CanAccessApplication(cmd.ApplicationID)` | Single sdksync entry; `requireAppAccess` moved into the op. |
| `SyncPlatformRoles` | `Public` | Single bff entry, anchor-only, static CODE catalogue — no client/app dimension. Catalogue passed to the constructor (command stays empty for the audit type name). |
| `SyncSubscriptions` | `CanAccessApplication(cmd.ApplicationID)` | Single sdksync entry; added `ApplicationID` to the command. |
| `SyncDispatchPools` | `CanAccessApplication(cmd.ApplicationID)` | Single sdksync entry; added `ApplicationID` (pools are global but the caller must act for the app). |
| `SyncScheduledJobs` | client-scope (`CanAccessClient` / anchor-for-platform) on `cmd.ClientID` | Single sdksync entry; gating is client- not app-scoped. |
| `SyncPrincipals` | `Public` | Two entry points: sdksync (app-scoped, keeps `requireAppAccess`) + platform `POST /api/principals/sync` (CanSyncPrincipals only). Users are global (matched by email). |
| `SyncOpenApiSpec` | `Public` | Two entry points: bff platform-spec (anchor) + sdksync (app-scoped). Uses `Emit` (direct repo archive+insert preserved). |

Last full check: `go build ./...`, `go vet -tags=integration ./...`, `make analyze`, `gofmt`,
and the **full integration suite** (`make test-integration`) — all green.

---

## 5. REMAINING WORK (priority order)

### 5.1 — DONE (2026-06-25). All 8 `Sync*` ops migrated, handlers rewired, tests green.
See §4 for the per-op authorization table. Pattern notes for future reference:
- App-scoped single-entry syncs (`SyncRoles`/`SyncSubscriptions`/`SyncDispatchPools`) moved
  `requireAppAccess` into the op as `CanAccessApplication(cmd.ApplicationID)`; the sdksync
  handler now only does `CanSync*` + `resolveApp` + sets `ApplicationID`.
- Multi-entry-point syncs (`SyncEventTypes`/`SyncPrincipals`/`SyncOpenApiSpec`) are `Public`
  per the §2 shared-entry-point rule — each controller keeps its own gate (sdksync keeps its
  `requireAppAccess`; bff keeps `RequireAnchor`).
- `AnchorCtx` does NOT grant `CanAccessApplication`; the app-scoped sync tests use a local
  `appAccessCtx()` helper (`WithAuth` + `Scope: Anchor, AllApplications: true`).

### 5.2 — DONE (2026-06-25). scheduledjob CRUD migrated; openapispecs is sync-only (covered by 5.1).
scheduledjob CRUD (Create/Update/Pause/Resume/Archive/Delete + FireNow) is on the envelope per
the connection ref. The shared status-flip body is `statusFlip[E]` (load → post-load
`CheckScopeAccess` → mutate → `Save`). FireNow keeps the two-phase shape (direct instance insert
then `usecaseop.Emit`). The `requireScopeByID` api helper was deleted.

### 5.3 TS / PHP SDK ports — IMPLEMENTED 2026-06-25. **Full plan + status: `docs/sdk-envelope-port-plan.md`.**
TS done + verified (37/37 tests, incl. wire-parity + rollback). PHP done + verified (6/6) — run with
`XDEBUG_MODE=off vendor/bin/phpunit` (Homebrew Xdebug hangs CLI PHP otherwise; cf. Makefile `sdk-generate`).
Remaining: cut the breaking major-version SDK releases (`make release-ts-sdk BUMP=minor` →
typescript-sdk/v0.8.0, `make release-laravel-sdk BUMP=minor` → laravel-sdk/v0.7.0; needs a clean tree + push).
Port `clients/typescript-sdk` and `clients/laravel-sdk` to the same `Operation`/`Plan`/`Run`
contract. Survey done 2026-06-25: both SDKs have a bare `execute()` (no Validate/Authorize/Execute
split), a *soft* Result seal (TS exports the token; PHP `internal()` is public), a caller-owned-tx
UoW (optional `persist` callback), and **no tests or consumers** of the machinery (so it's a
framework rewrite with nothing to migrate). TS: standardize on the Promise tree, lift the airtight
`unique symbol` brand from `src/effect/usecase/seal.ts` for the Plan seal, make the UoW own the tx
(drop `persist`); defer the unused Effect variant. PHP: make `DB::transaction` ownership mandatory
in the UoW; the seal stays convention-level (no module privacy) but the structure carries the
invariant. Hard non-goal: keep the outbox-row wire output byte-identical (parity gate). Both are
breaking → major SDK bumps via the split workflows. See the plan doc for phasing + open questions.

### 5.4 Cleanup — DONE (2026-06-25). All four items complete; tree green.
- ✅ **Deleted `pkg/fcsdk/commit`** (was fully dead — 0 production + 0 cross-package test callers).
- ✅ **Deleted `usecase.UseCase` + `usecase.Run`** (`use_case.go`, 0 callers) **+ the `order-service`
  example** (its only consumer). Reworked `usecase/seal_test.go` to drop the Run-pipeline tests.
  **Kept** `usecase.Result`/`Success`/`Failure`/`Into` (load-bearing carrier for `usecasepgx`↔`usecaseop`)
  and `IsSuccess`/`IsFailure` (still used by the seal test; harmless public helpers).
- ✅ **Relocated `internal/sealed` → `pkg/fcsdk/internal/sealed`.** It is NOT deletable — it still
  seals `usecase.Success`. Moving it under `pkg/fcsdk/internal/` makes the seal real: only packages
  under `pkg/fcsdk/` can mint a `Token` now (at repo-root `internal/` any platform package could).
  Updated the 5 importers (`usecase/result.go`, `usecasepgx/{commit,run}.go`, `usecasesql/{commit,run}.go`).
- ✅ **`make analyze` is wired into CI** — it was already there: `.github/workflows/ci.yml` runs it as a
  dedicated "Custom UoW seal analyzer" step, and the Makefile `ci:` target chains `analyze`.
- Stale `commit.*` doc comments + `pkg/fcsdk/doc.go` (mental-model diagram, package map) updated to
  reference `usecaseop`. **Not done (separate, deferred):** `usecasesql` (1 drifted user) is still
  present — the roadmap flags it for deletion but it was out of scope for this cleanup.

---

## 6. How to verify (run after every change)

```
go build ./...                              # production compiles
go vet -tags=integration ./...             # ALL test files compile — CRITICAL: catches stale
                                           # cross-module TEST seeds (plain build does NOT)
make analyze                               # uowseal: every Operation/TxOperation sets Authorize
gofmt -l internal pkg cmd tools            # empty = clean
make test-integration                      # full embedded-Postgres suite (~10-15 min, -p 1)
# or per-module: go test -p 1 -tags=integration ./internal/platform/<mod>/...
```

Sanity grep (must return nothing): coarse checks must not live in use cases —
`grep -rnE "auth\.(Can[A-Za-z]+|RequireAnchor)\(" internal/platform/*/operations/*.go | grep -v _test`

---

## 7. Gotchas / non-obvious

- **`go vet -tags=integration ./...`** is the only check that compiles test files repo-wide — run
  it after ANY op-signature change to catch cross-module test seeds (e.g. `openapispecs`/`sdksync`
  tests seed via `appops.CreateApplication`).
- **Never rename/resignature the `*Event` constructors** (e.g. `authops.NewOAuthClientCreatedEvent`,
  `saops.NewServiceAccountCreatedEvent`) — orchestration ops in other modules call them directly.
- **`AnchorCtx` does not grant `CanAccessApplication`** (it sets Scope=Anchor, not AllApplications) —
  app-scoped sync tests need `WithAuth` + `AllApplications:true` (or `Applications:[id]`).
- **`send_password_reset`** is intentionally a plain function (no domain event) — not an envelope op.
- **The scheduledjob instance endpoints** (`writeInstanceLog`, `completeInstance`) are NOT envelope
  ops — they write the instance *projection* directly (no domain event), so they keep their inline
  `auth.CheckScopeAccess(ac, inst.ClientID)` in the api handler. Left as-is by design.
- **`SyncPlatformRoles`** takes its catalogue as a constructor arg (`SyncPlatformRoles(repo, codeRoles)`),
  NOT in the command — the command stays empty so the audit log records `SyncPlatformRolesCommand`
  (not the whole serialized catalogue). The bff passes `seed.PlatformRoles()`.
- **`go vet -tags=integration ./...`** is the only check that compiles test files repo-wide — run
  it after ANY op-signature change to catch cross-module test seeds.

---

## 8. Resume prompt suggestion

> The Go use-case envelope migration is COMPLETE per `docs/usecase-envelope-migration-handover.md`
> (§5.1 sync ops + §5.2 scheduledjob CRUD + §5.4 cleanup all done 2026-06-25; tree green,
> uncommitted). The only remaining work is §5.3 — porting the TS (`clients/typescript-sdk`) and
> PHP (`clients/laravel-sdk`) SDKs to the same `Operation`/`Plan`/`Run` contract. Verify Go with
> the §6 commands.
