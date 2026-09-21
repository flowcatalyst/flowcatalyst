# Integral: defining FlowCatalyst subscriptions from module specifications

Status: **deferred, not started** (2026-09-22). This records the approach and
the open questions so the work can be picked up without re-deriving them.
Nothing in Integral has been changed.

Integral is the multi-tenant Laravel reference application
(`inhance/InhanceMono/apps/integral`, shared packages under
`InhanceMono/packages_root/packages`). The goal is for its FlowCatalyst
subscriptions to be defined in code, per tenant, and synced by
`flowcatalyst:sync` — not created by hand in the platform UI once per
environment.

## Is what exists enough to do it?

Yes, to build it and to pilot it. The platform and the Laravel SDK already
provide every mechanism the design below needs:

| Need | Provided by |
|---|---|
| A subscription defined in code, with a delivery target | `#[AsSubscription]` / `Sync\SubscriptionDefinition`, required `target` (path resolved at sync time) |
| A connection that is the same in every environment | `#[AsConnection]` / `Sync\ConnectionDefinition`, `connectionCode`, platform connection sync |
| One codebase, many tenants | `SyncDefinitionSet::forApplication(..)->forClient($identifier, targetBaseUrl:)` |
| The tenant list as runtime data | `ProvidesSyncDefinitionSets` + `flowcatalyst.definitions.set_providers` |
| Several sources feeding one scope safely | `syncGrouped()` merges per application; `flowcatalyst:sync` uses it |
| A tenant's sync never touching another tenant's rows | platform sync scoped to (application, client), `removeUnlisted` included |
| A failed tenant not blocking the others, but the run still failing | per-scope continue; `flowcatalyst:sync` exits FAILURE on any error |

Two gaps do **not** block starting, but each must be closed before the part of
the migration it affects — see [Gaps to close first](#gaps-to-close-first).

## What Integral has today

- A **module** is the unit of functionality — think of it as an application or
  a separate service; a tenant can have its own module for custom code. Each
  module has a specification factory
  (`ModuleSpecificationFactoryInterface::make(): ModuleSpecificationDto`)
  returning event types, subscriptions, permissions, roles, pollers, configs,
  process pools and an `isApplication` flag. The Epod module alone declares
  about 25 subscriptions.
- Those subscriptions are in Integral's own shape
  (`Inhance\IntegralContract\...\SubscriptionDto`): `runnerType`,
  `runnerExecuteUri`, `processPool`, `moduleCode`, `initialDelay`, `sequence`,
  `maxAttempts`, `maxDelaySeconds`, `queueType`, `transformer`, `configData`.
- A **tenant** (a FlowCatalyst *client*) has a specification factory
  (`TenantSpecificationDtoFactory::make(): TenantSpecificationDto`) listing the
  modules it gets, in code. The factories are a static list in
  `config/integral-service.php`.
- There is **no tenant↔module mapping in the database**. The owner wants one:
  at least the ability to set a tenant's modules in the DB, overriding or
  replacing the factory.
- `ModuleSpecificationDto` and `SubscriptionDto` live in shared packages
  (`module-contract`, `integral-contract`), not in the Integral app — a change
  to them reaches every consumer of those packages.
- Nothing in Integral's deployment runs `flowcatalyst:scan` or
  `flowcatalyst:sync`. `app/FlowCatalyst/` holds two scheduled-job classes and
  a permission resolver; no subscriptions, no connections.

## Approach

1. **FlowCatalyst subscriptions live in the module specification**, in NEW
   fields (e.g. `flowCatalystSubscriptions`, `flowCatalystConnections`) added
   as trailing optional constructor arguments, so every existing factory keeps
   compiling. Do **not** reuse the existing `subscriptions` list: a subscription
   present in both would be delivered twice, once by each system. Migrating one
   is then a deliberate move from one list to the other, reviewable in a diff.

2. **No custom sync command.** Integral implements one
   `ProvidesSyncDefinitionSets` class and lists it in
   `flowcatalyst.definitions.set_providers`. For each tenant, for each of its
   modules, it yields a `SyncDefinitionSet` for the module's FlowCatalyst
   application, bound to the tenant with `forClient($tenantIdentifier,
   targetBaseUrl: $tenantHost)`. `flowcatalyst:sync` does the rest: merges sets
   per application, syncs connections before subscriptions, issues one call per
   tenant, and fails the run if any scope failed. "Build the subscription and
   add it to the repository on the fly" is exactly what the provider is.

3. **One tenant-module resolver.** A single class answers "which modules does
   this tenant have": the DB mapping when present, falling back to the tenant
   factory. Both the existing Integral machinery and the FlowCatalyst provider
   call it, so the two can never disagree about a tenant's modules. **Build
   this first** — it is independent of FlowCatalyst and everything else reads
   through it.

4. **One global connection per application**, defined in the application's
   global set. A client-scoped subscription may use a global connection, and a
   synced connection always signs with the application's own service account,
   so tenants do not need a connection each.

5. **Pilot one subscription end to end** (define → sync → deliver → signature
   verified by the `fc-signature` middleware) on a non-production tenant before
   moving any others. Then migrate module by module.

6. **Deploy wiring**: a step that runs `flowcatalyst:scan` then
   `flowcatalyst:sync` per environment. A sync failure should fail the deploy —
   the command's exit status already reflects it.

## Open questions (owner decisions)

1. **One FlowCatalyst application for all of Integral, or one per module?**
   Codes are unique per (application, client, code). Separate applications
   (natural for modules with `isApplication`, and for tenant custom-code
   modules) keep a tenant's module from colliding with a core one and let a
   module be synced and permissioned independently. The cost: each application
   has its own provisioned service account and signing secret, while a Laravel
   app has a single `FLOWCATALYST_SIGNING_SECRET` — one codebase serving
   several applications cannot validate all their signatures without a
   per-application secret lookup, which the SDK does not have today.
   One application avoids that entirely, at the price of a shared code
   namespace (prefix codes with the module code).

2. **Do FlowCatalyst client identifiers equal Integral tenant codes** (`poc`,
   `upington`, …)? The sync names a client by identifier, never by id (ids
   differ per environment). If they are not the same, the tenant needs a column
   holding its FlowCatalyst client identifier, and provisioning must set it.

3. **May `module-contract` depend on the FlowCatalyst Laravel SDK**, so a module
   specification holds `FlowCatalyst\Sync\SubscriptionDefinition` /
   `ConnectionDefinition` directly? The alternative is an Integral-side DTO plus
   a mapper — one more layer that can drift from the SDK, in exchange for
   keeping the contract package free of the dependency.

4. **Where does the tenant↔module mapping live, and what does it mean?** A
   table in the tenant package, presumably — but does a DB row *add to* the
   factory's modules, *replace* them, or can it also *remove* one? And is the
   factory kept as the seed for new tenants, or retired once the DB is the
   authority?

5. **What is a tenant's delivery host?** `forClient(.., targetBaseUrl:)` needs
   the URL the platform should call for that tenant. If every tenant is served
   from one host, a single `flowcatalyst.subscriptions.target_base_url` (or
   `APP_URL`) is enough and the per-set override is unnecessary.

6. **Event types.** FlowCatalyst event-type codes are
   `application:subdomain:aggregate:event`. Do Integral's existing event types
   map onto that one-to-one, and are they synced from the same module
   specifications (a `flowCatalystEventTypes` field) or defined separately?

7. **Who owns the shared-package change?** `packages_root` is worked in by
   other sessions. The `ModuleSpecificationDto` change should be made by
   whoever owns that lane.

## Gaps to close first

Neither blocks the resolver, the provider or the pilot.

- **A tenant that loses its last module is never cleaned up.** With
  `removeUnlisted`, a tenant losing ONE module works: its subscriptions drop out
  of that tenant's list and the platform removes them. A tenant losing EVERY
  module yields no set, a scope with no definitions produces no call, and its
  subscriptions stay live and keep delivering. Tolerable while modules are fixed
  in code; a real hole once they can be changed in the DB. Fix in the SDK
  (all three, for parity): let a set explicitly sync an EMPTY list for a scope,
  and have the provider yield such a set for every known tenant. Must land
  before the DB-driven module mapping goes live.

- **The subscription sync drops fields Integral uses.** `#[AsSubscription]`
  accepts `delaySeconds`, `queue`, `sequence`, `maxAgeSeconds` and
  `customConfig`, but neither the SDK's sync payload nor the platform's sync
  input carries them (`internal/platform/subscription/operations/sync.go`
  `SyncSubscriptionInput`). Integral subscriptions use `initialDelay`,
  `sequence`, `maxDelaySeconds` and `queueType`. A migrated subscription that
  relies on any of them would silently behave differently — e.g. a 10-minute
  `initialDelay` becoming immediate delivery. Needs the platform sync input,
  the three SDKs and the Java platform mirror. Must land before migrating any
  subscription that sets one of those fields; the pilot should pick one that
  does not.

Smaller, not blocking: `runnerType`, `transformer` and `processPool` have no
direct FlowCatalyst equivalent field — `processPool` maps to a dispatch pool
(`dispatchPoolCode`, itself syncable), the other two need a decision per
subscription.

## Suggested order

1. Owner answers questions 1–5 (6 and 7 can follow).
2. Tenant-module resolver + DB mapping in Integral (no FlowCatalyst involved).
3. `ModuleSpecificationDto` gains the FlowCatalyst fields (shared package).
4. The provider, registered in config; deploy step running scan + sync.
5. Pilot one subscription on a non-production tenant.
6. Close the two gaps above.
7. Migrate module by module, moving each subscription from the old list to the
   new one.

## References

- Platform contract and rollout:
  `docs/java-handoff-2026-09-21-code-first-connections.md`
- SDK usage, including the multi-tenant provider example:
  `clients/laravel-sdk/docs/syncing-definitions.md`
- Platform deploy order: run `scripts/ops/056-application-scope-prescan.sql` →
  deploy → anchor `POST /bff/roles/sync-platform`. The SDK releases carrying
  this are held until the platform is deployed.
