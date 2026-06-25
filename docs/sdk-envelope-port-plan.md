# SDK Envelope Port Plan — TypeScript + PHP (§5.3)

Port `clients/typescript-sdk` and `clients/laravel-sdk` to the same **use-case envelope**
the Go SDK/platform now use (`Operation` / `Plan` / `Run`). This is the last remaining piece
of the use-case envelope migration — the Go side is complete (see
`docs/usecase-envelope-migration-handover.md`).

**Scope reality check (good news):** neither SDK's use-case machinery has any in-repo
consumer or any test. Grep confirms: the `UseCase` / `UnitOfWork` / `Result` layer is a
*provided framework* with zero callers and zero tests in either SDK. So this is a
**framework-layer rewrite with nothing to migrate** — lower risk than the Go work, but it
also means we add the tests that don't exist yet, and there's no reference consumer to mirror.

---

## 1. The target contract (what we're porting TO)

The Go envelope, restated language-neutrally:

```
Operation{ name, validate(cmd), authorize(ctx, cmd), execute(ctx, cmd, ec) -> Plan }
Run(uow, op, cmd, ec) -> (event, error)     // Validate → Authorize → Execute → apply(Plan) in ONE tx
Plan = Save(agg, repo, event) | Delete(agg, repo, event) | Emit(event) | SaveAll(...) | Sync(...)
```

Four invariants the port must reproduce:

1. **Named phases.** `validate` (pure shape), `authorize` (resource-level; `Public` = explicitly open), `execute` (loads + invariants → returns a Plan). Today both SDKs collapse all three into one `execute()`.
2. **Execute returns a `Plan`, not a committed `Result`.** The Plan describes the change + event but is NOT applied. This is what makes "aggregate written ⇒ event + audit written, atomically" hold *by construction* — Execute literally cannot reach the DB.
3. **`Run` is the only thing that applies a Plan, and it owns the transaction.** Persist(s) + event + audit commit together. Today the UoW does NOT own the tx — the caller passes an optional `persist` callback and must wrap the call in their own transaction (PHP doesn't even have the opt-in tx path TS has).
4. **The Plan is sealed** — only the SDK package can construct one, so the only path to a committed event is through `Run`. Today the `Result` seal is *soft* in both SDKs (see §2).

> **Non-goal / hard invariant:** the envelope is an *authoring* structure. It must NOT change
> the wire output — the `outbox_messages` EVENT/AUDIT_LOG row shape, the `CreateEventDto` /
> `CreateAuditLogDto` field mapping, dedup IDs, TSIDs — stays byte-identical across all four
> SDKs. The parity gate (§5) is how we prove that.

---

## 2. Current state per SDK (from the source survey)

| Aspect | TypeScript (`src/usecase/`) | PHP (`src/UseCase/`) |
|---|---|---|
| UseCase shape | `execute(cmd, ctx): Promise<Result<E>>` (one method). `SecuredUseCase` adds deny-by-default `authorizeResource()` → `doExecute()`. | `execute(Command, ExecutionContext): Result` (one method). `SecuredUseCase` same deny-by-default `authorizeResource()` → `doExecute()`. |
| Result seal | **Soft.** `RESULT_SUCCESS_TOKEN` (a `unique symbol`) is re-exported from `./usecase` and `.` — a consumer can pass it and mint a Success. Guard only stops *accidental* misuse. | **Soft.** `ResultSuccessToken::internal()` is **`public`** (PHP has no module privacy); the identity check passes trivially. Convention-level only. |
| UoW owns tx? | **No (default).** `commit(event, cmd, persist?)` runs `persist()` then writes the event — caller owns atomicity. *Opt-in* `OutboxUnitOfWork.run(cb)` + `OutboxDriver.withTransaction?` DOES own a tx (on the class, not the interface), modeled on Rust's `OutboxUnitOfWork::run`. | **No.** `commit(event, cmd, persist?)` runs `persist()` then writes — never opens a tx. Only enforcement is `DatabaseDriver`'s `strictTransactions` flag (default OFF → warn-and-proceed). |
| Commit family | `commit` / `commitAggregate` / `commitDelete` / `emitEvent`. No `CommitAll` / `Sync`. `Aggregate` arg is parity-only (ignored). | `commit` / `commitAggregate` / `commitDelete` / `emitEvent`. Same — no `CommitAll`/`Sync`; `Aggregate` ignored. |
| Sink | `OutboxManager.createEvent/createAuditLog(dto, tx?)` → `OutboxDriver.insert`. Audit off by default. | `OutboxManager::createEvent/createAuditLog` → `OutboxDriver::insert`. Audit off by default. |
| Tests for the machinery | **None.** | **None.** |
| In-repo consumers | **None** (only JSDoc examples). | **None** (only the service-provider binding). |
| Effect variant | `src/effect/usecase/` — complete, shipped via `./effect` subpath, **zero internal imports**, `effect` is an *optional* peer dep. Has the one **airtight** seal: a compile-time `Sealed<E>` brand keyed by a module-private `unique symbol` not in the `exports` map. | — |
| Build / test | `tsc` build; `node --import tsx --test` runner; lint = `tsc --noEmit`. | PHPUnit via Orchestra Testbench; **no** `scripts`, phpstan, or Pint. |

---

## 3. Gap → work, per SDK

### 3.1 TypeScript (`clients/typescript-sdk`)

Per the handover: *standardize on the Promise tree, upgrade the seal to the `unique symbol`
brand, make the UoW own the tx (drop the optional `persist` callback). Defer the Effect variant.*

1. **`Plan` type + constructors** (`src/usecase/plan.ts`, new). A sealed `Plan<E>` whose seal is a module-private `unique symbol` brand — lift the exact mechanism from `src/effect/usecase/seal.ts` (it's the airtight one) into the Promise surface. Constructors `save(aggregate, repo, event)`, `delete(aggregate, repo, event)`, `emit(event)`, optionally `saveAll` / `sync`. **Do not** add a `./usecase/plan` subpath to the `exports` map, and do not export the brand symbol — that's what keeps it unforgeable (the lesson from the leaky `RESULT_SUCCESS_TOKEN`).
2. **`Operation` interface + `run` driver** (`src/usecase/operation.ts`, new). `Operation<C,E>` with optional `validate`, **required** `authorize` (+ a `Public` helper), and `execute(cmd, ctx) -> Promise<Plan<E>>`. `run(uow, op, cmd, ec): Promise<Result<E>>` runs the phases, then applies the Plan inside a UoW-owned tx. Keep the existing `Result<E>` as `run`'s *return* type (it's the handler-facing outcome) — only the Plan is new.
3. **`Repo` abstraction** (`src/usecase/repo.ts`, new): `interface Repo<A> { persist(agg, tx): Promise<void>; delete(agg, tx): Promise<void> }`. The `persist` callback that today lives on `commit()` moves INTO the Plan (`save(agg, repo, event)`), and `run` calls `repo.persist(agg, tx)` inside the tx it owns. This is the Go `usecasepgx.Persist[A]` analogue.
4. **UoW owns the tx.** Promote the opt-in `OutboxUnitOfWork.run` + `OutboxDriver.withTransaction` to *the* path `run()` uses, and make `withTransaction` **required** on the driver (it's currently optional → `DRIVER_NOT_TX_AWARE` failure). Remove the `persist?` parameter from the public `commit*` family (those become internal Plan-apply primitives, or are deleted in favour of the Plan constructors). Keep `TxScopedOutboxUnitOfWork` as the in-tx applier.
5. **Defer Effect.** Leave `src/effect/usecase/` untouched for now, but add a one-line deprecation note in its index pointing at the Promise envelope as the supported surface. (Decision needed — see §6 Q3.)
6. **Tests** (new — none exist): `run` phase ordering + short-circuit; Plan-seal cannot be forged externally (compile-time proof comment + a runtime "no public constructor" check); UoW-owned tx rolls back the persist when the event write throws; `save`/`delete`/`emit` produce identical outbox rows to today (snapshot).

### 3.2 PHP (`clients/laravel-sdk`)

Per the handover: *make `DB::transaction` ownership mandatory in the UoW.*

1. **`Plan` type + constructors** (`src/UseCase/Plan.php`, new). `final class Plan` (sealed-by-convention as today — PHP has no module privacy, so this stays a convention + a private constructor reachable only via the static `save`/`delete`/`emit` factories on `Plan` itself). Carries aggregate + persist closure + event + kind. This is structurally still strong: `execute()` returns a `Plan`, and only `Run` applies it, so the "can't reach the DB except via Run" property holds even though the token isn't language-enforced.
2. **`Operation` + `Run`** (`src/UseCase/Operation.php` + `src/UseCase/Run.php` or a `UseCaseRunner`, new). `Operation` interface with `validate` / `authorize` / `execute(...): Plan`; `Run::execute(UnitOfWork, Operation, Command, ExecutionContext): Result` runs the phases and applies the Plan. `SecuredUseCase` folds into the `authorize` phase; keep a `PublicOperation` trait / `Authorize::public()` helper.
3. **UoW owns `DB::transaction`.** `OutboxUnitOfWork` (or the new `Run`) wraps the whole apply in `DB::transaction(fn () => ...)` on the outbox connection: run the Plan's persist closure, write the event, write audit — all in one tx. Drop the caller-passed `?callable $persist` from the public surface (it moves into the Plan). The `DatabaseDriver::strictTransactions` flag becomes redundant for SDK-authored ops (the UoW now always owns the tx) — keep it only for direct `OutboxManager` use, or retire it.
4. **Tests** (new — none exist; there's no `phpunit.xml` either): add a `phpunit.xml.dist`, then test phase ordering, Plan-only-applied-by-Run, `DB::transaction` rollback atomicity (persist + outbox roll back together), and outbox-row parity snapshots.

---

## 4. Phasing & sequencing

Do TS first (it has the reference seal mechanism to lift, and a faster test loop), then PHP.

- **Phase A — shared design lock (small).** Confirm the §6 open questions, write the language-neutral `Operation`/`Plan`/`Run` shape into each SDK's `docs/`.
- **Phase B — TS port.** §3.1 items 1–4 + tests (item 6). Gate: `tsc --noEmit` clean, `node --test` green, outbox-row snapshots unchanged.
- **Phase C — PHP port.** §3.2 items 1–3 + tests (item 4). Gate: PHPUnit green, outbox-row snapshots unchanged.
- **Phase D — parity + release.** Run the cross-SDK parity check (§5); cut SDK releases via the existing split workflows (`split-typescript-sdk.yml`, `split-laravel-sdk.yml`) so downstreams pick up the envelope. Note this is a **breaking API change** for any external consumer of the SDK use-case layer (there are none in-repo) → major-version bump for both SDKs.

Rough size: TS ≈ 1 focused session (new plan.ts/operation.ts/repo.ts + UoW rewire + ~4 test files); PHP ≈ 1 focused session (parallel structure + phpunit bootstrap). Parity/release ≈ small.

---

## 5. Verification strategy

- **TS:** `pnpm build` (`tsc`), `pnpm test` (`node --test`), `pnpm lint` (`tsc --noEmit`).
- **PHP:** `vendor/bin/phpunit` under Testbench (add `phpunit.xml.dist` first).
- **Cross-SDK wire parity (the critical gate):** the EVENT/AUDIT_LOG outbox-row JSON must stay byte-identical to the Go SDK's (`pkg/fcsdk/outboxpgx` `buildEventPayload`/`buildAuditPayload`) and to each other. Add a fixture test in each SDK that builds a known `DomainEvent` + command through the envelope and asserts the serialized outbox payload against a committed golden file shared in spirit with the Go `outboxpgx` output. This is what guarantees the refactor didn't change the wire.
- **Seal proof:** TS — a `// @ts-expect-error` block proving `Plan` can't be constructed externally; PHP — a docblock + a test asserting `execute` results only flow through `Run`.

---

## 6. Open questions (decide in Phase A)

1. **Keep the `commit*` UoW family as public API, or fully replace with Plan constructors?** Go kept the `usecasepgx.Commit*` primitives *internal* and exposed only `Plan`+`Run`. Recommend the same: `commit/commitAggregate/commitDelete/emitEvent` become internal apply primitives (or are deleted), and consumers author via `save/delete/emit` + `run`. Confirm we're willing to break that surface (no in-repo consumers, so only external downstreams — and the SDKs aren't released to the new contract yet).
2. **PHP seal:** accept that it stays convention-level (language limit), with the structural guarantee coming from "Execute returns a Plan; only Run applies it"? Recommend yes — matches the existing PHP `ResultSuccessToken` reality, and the structure (not the token) is what carries the invariant.
3. **Effect TS variant:** (a) leave as-is and deprecate, (b) port it to the envelope too, or (c) delete it. It has zero internal users and `effect` is optional. Recommend **(a) deprecate now, delete in a later major** — porting it doubles the TS work for an unused surface; deleting it now is a separate breaking change better batched with the major bump.
4. **`Sync` Plan in the SDKs?** The Go `Sync` Plan exists for the platform's bulk SDK-registration endpoints. Consumer SDKs don't do bulk-upsert-with-rollup, so **omit `Sync`/`SaveAll` from the SDK Plan** unless a consumer need surfaces. Confirm.

---

## 7. Resume prompt

> Execute the SDK envelope port per `docs/sdk-envelope-port-plan.md`. Start Phase A (lock the
> §6 open questions), then Phase B (TypeScript: add `plan.ts`/`operation.ts`/`repo.ts`, lift the
> `unique symbol` brand from `src/effect/usecase/seal.ts`, make the UoW own the tx, drop the
> `persist` callback, add the missing tests), then Phase C (PHP: parallel structure, UoW owns
> `DB::transaction`, add `phpunit.xml.dist` + tests). Keep the outbox-row wire output byte-identical
> (Phase D parity gate). Both ports are breaking → major-version SDK bumps via the split workflows.
