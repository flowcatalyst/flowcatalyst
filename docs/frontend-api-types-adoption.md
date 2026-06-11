# Frontend: adopting the generated OpenAPI types

Status: **DONE** (2026-06-11). All 17 migratable (apiFetch) modules alias
the generated response types; the `Time` schema is patched to `string` at
generate time (openapi-ts.config.ts — backend Time types carry no JSON
schema); `make frontend-types-verify` (in `make ci`) regenerates and fails
on diff, mirroring sqlc-verify. The BFF set (dashboard, filter-options,
developer, event-types, roles, permissions, processes — all on bffFetch,
stripped from the spec) and the auth surface (auth, twofactor,
changePassword — chi-mounted; webauthn — ceremony-shaped) stay hand-rolled,
each with a header comment saying why. Note: the original priority table
below listed event-types/roles before discovering they are BFF modules.

Migration surfaced and fixed real drift: creates returning {id} not full
entities, 204 no-body mutations typed as message envelopes, phantom fields
(EDM identityProviderType, dispatch-job filter-option shapes — empty
dropdowns at runtime), a cors create toasting undefined, a nonexistent
identity-provider by-code route, a phantom scope=GLOBAL config param, and
one OPEN issue: OAuthClientCreatePage never sends the contract-required
clientId, so SPA oauth-client creation is rejected today (needs a product
decision: SPA generates one vs backend derives).

## Why

Every SPA↔Go wire break we've debugged this year (huma
`additionalProperties:false` 400s, casing mismatches, list-shape drift) had
the same structural cause: the SPA's request/response types are hand-typed
from memory, so nothing fails at build time when the backend contract moves.
The types in `src/api/generated/types.gen.ts` are generated from
`api/openapi.lock.json` — the exact contract `make api-diff` gates in CI —
so once a module consumes them, `vue-tsc -b` (part of `pnpm build`) becomes
a compile-time canary for backend drift.

## What's already in place

- `frontend/openapi-ts.config.ts` reads `../api/openapi.lock.json` by
  default (`OPENAPI_LIVE=true pnpm api:generate:live` targets a running
  server instead).
- Output is **types only** (`@hey-api/typescript`); the previously-generated
  fetch client/SDK and the interceptor layer attached to them were dead code
  and have been removed. The app's transport is `src/api/client.ts`
  (`apiFetch` / `bffFetch` / `authFetch`).
- Regenerate with `cd frontend && pnpm api:generate` after any backend
  change that runs `make api-bump`. **The two belong in the same commit**:
  lockfile bump + regenerated `types.gen.ts`.

## The friction to expect (read before starting)

The generated and hand-rolled types disagree on conventions, not just
content. Reconciling them is the work:

| Hand-rolled (`users.ts`) | Generated (`PrincipalResponse`) |
|---|---|
| `email: string \| null` | `email?: string` |
| `scope: PrincipalScope \| null` (string union) | `scope: string` |
| `idpType: IdpType \| null` | `idpType?: string` |
| `createdAt: string` | `createdAt: Time` (alias) |
| — | `readonly $schema?: string` (huma extra) |

Consequences:

1. **`null` vs `undefined`**: pages written against `x === null` checks need
   `x == null` (or `?? null` normalisation at the module boundary). With
   `strict` + `noUncheckedIndexedAccess` this surfaces as real compile
   errors — that's the point, but budget for it.
2. **Lost string unions**: the spec emits plain `string` where the SPA wants
   `"USER" | "SERVICE"`. Two options: tighten the backend spec with enums
   (preferred — fixes every consumer including the published SDKs), or
   narrow locally with `as` + a runtime guard at the module boundary.

   > ⚠️ **SDK coordination required for the enum option.** Adding enums
   > changes `api/openapi.lock.json`, and the split-and-push pipeline
   > regenerates the published TS/Laravel SDKs from it — their generated
   > types change signature (`string` → enum union), which is
   > compile-time breaking for typed SDK consumers. Enum tightening must
   > ship as a deliberate, release-noted SDK version bump via
   > `make api-bump` + the SDK release pipeline — never as a side effect
   > of a frontend type migration. The local-narrowing option has zero
   > SDK impact and is the right default while migrating.
3. **One module ripples**: changing `users.ts`'s `User` touches every page
   that imports it. Migrate **one module per PR**, run `pnpm build`, fix the
   fallout, stop.

## Migration recipe (per module)

1. Find the module's response types in `types.gen.ts` (search the entity
   name; the operation map at the bottom of the file links paths →
   request/response types).
2. Re-export under the existing names so pages keep their imports:
   ```ts
   import type { PrincipalResponse, PrincipalListResponse } from "./generated";
   export type User = PrincipalResponse;
   export type UserListResponse = PrincipalListResponse;
   ```
3. Where the generated shape is too loose (plain `string` for a known
   union), prefer fixing the spec (backend enum) over local widening.
4. Keep hand-rolled **request** types only where the generated input type is
   awkward (path/query params are flattened into hey-api's `*Data` types);
   response types are the drift-critical half.
5. `pnpm build` — fix page fallout, normalising `undefined`→`null` at the
   module boundary if the page logic genuinely wants `null`.
6. Delete the now-shadowed hand-rolled interface.

## What NOT to migrate

- **BFF modules** (`dashboard.ts`, `filter-options.ts`, `developer.ts`,
  anything on `bffFetch`): the BFF paths are deliberately stripped from the
  OpenAPI spec (`StripBFFPaths`), so there are no generated types for them.
  They stay hand-rolled by design.
- **Auth-surface modules** (`auth.ts`, `twofactor.ts`, `webauthn.ts`,
  `changePassword.ts`): mounted as chi routes outside huma, also not in the
  spec. Stay hand-rolled until/unless those routes converge on huma
  (tracked separately in the backend backlog).

## Suggested order (drift-risk × page blast-radius)

| Priority | Module | Generated counterpart | Notes |
|---|---|---|---|
| 1 | `users.ts` | `PrincipalResponse`, `PrincipalListResponse`, `PrincipalRoleAssignmentDto`… | Most past drift; biggest page surface (UserDetail/UserList/ClientUsers) — do it first while patience is fresh |
| 2 | `event-types.ts` | `EventTypeResponse`… | Schema-bearing payloads; SDK-synced entity |
| 3 | `scheduled-jobs.ts` | `ScheduledJobResponse`, `OffsetPage…` | Already shipped one envelope bug |
| 4 | `subscriptions.ts` | `SubscriptionResponse`… | Junction-heavy (eventTypes/pools) |
| 5 | `service-accounts.ts` | `ServiceAccountResponse`… | Credential bundle shapes |
| 6 | `applications.ts` | `ApplicationResponse`… | |
| 7 | `clients.ts` | `ClientResponse`… | |
| 8 | `roles.ts` / `permissions.ts` | `RoleResponse`… | Coordinate with the permissions work |
| 9 | `oauth-clients.ts` | `OAuthClientResponse`… | |
| 10 | `events.ts` / `dispatch-jobs.ts` | `EventRead`, `DispatchJobRead` | Bare-array list responses; read-only pages |
| 11 | remainder | `connections`, `dispatch-pools`, `identity-providers`, `email-domain-mappings`, `processes`, `cors`, `audit-logs`, `login-attempts`, `config`, `reset-approvals` | Mechanical once the pattern is set |

## Guardrail worth adding (cheap, do early)

A CI step that mirrors `sqlc-verify`: regenerate and fail on diff, so the
committed `types.gen.ts` can never lag the lockfile:

```make
frontend-types-verify:
	cd frontend && $(PNPM) api:generate
	git diff --exit-code frontend/src/api/generated/ || \
		(echo "generated API types out of date; run 'pnpm api:generate' and commit" && exit 1)
```

Add it to `make ci` once the first module has migrated (before that it
guards nothing the build doesn't already).

## Definition of done

Every non-BFF, non-auth module's **response types** alias `types.gen.ts`;
`pnpm build` fails on any backend wire change that isn't reflected in the
lockfile + regen; the hand-rolled interfaces that remain are exactly the
BFF/auth set, each with a comment saying why.
