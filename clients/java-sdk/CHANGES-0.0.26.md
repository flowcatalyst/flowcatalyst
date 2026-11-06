# Java SDK 0.0.26

Changes since `java-sdk/v0.0.25`. Tagged `java-sdk/v0.0.26` on 2026-11-06.

Sources of truth: `docs/wire-contract.md` (dispatch jobs), `api/openapi.lock.json`.

## Dispatch jobs

- `CreateDispatchJobDto.withConnectionId(...)` and `withIdempotencyKey(...)` are no longer
  deprecated no-ops: both are sent on the outbox payload and by `DispatchJobsResource` (they are
  members of the same batch item). The connection's service account signs a subscription-less
  job; a repeated key answers the first job's id instead of creating another (within the job's
  application and client, kept 7 days). `validate()` refuses a key over 100 characters or a
  malformed connection id.

## Field-worker sign-in options

Sources of truth: `docs/field-worker-sign-in-options-spec.md`, `docs/wire-contract.md` (Field identity),
`api/field-fixtures` (the four `principal` cases), `api/openapi.lock.json`.

- `FieldPrincipal` gains `actor` (`FieldActor(id, type, name)` from the token's `act` claim; `type` is
  `field_worker` for a supervisor or `principal` for a platform user; null when nobody is signed in as the
  worker) and `test` (test identity). Every other component is still the WORKER's: record the worker as the
  subject of an action and `actor` as the operator. `deviceId` is `""` (and `deviceMode` `UNENROLLED`) when the
  token's key is not an enrolled device. The 14-argument constructor stays (no actor, not a test identity).
- `client.fieldWorkers()` gains `issueBadge(id, label)`, `listBadges(id)`, `revokeBadge(id, badgeId)`,
  `bulkIssueBadges(workerIds)` (1 to 200, all or nothing) and `createSignInCode(id, appCode)`; badge and
  sign-in-code secrets are returned once. `Filters` gains `isTest` (the five-argument constructor stays).
  `ensure`/`update` requests carry `canImpersonate` and `isTest`; the policy and introspection models carry the
  new fields (regenerated from `openapi/openapi.json`).
