# Changelog

Notable changes to `@flowcatalyst/sdk`. Releases are tagged
`typescript-sdk/vX.Y.Z` in the platform repository.

## Unreleased

Additive: no signature breaks (new parameters are optional).

### Ingestions
- `Ingestion` gains `notices` (undismissed notices: failures the platform
  refused), `latestNotice`, `latestNoticeAt`, `warnings` (what the definition
  says that will not run as meant) and `save` (`IngestionSaveOutcome`: `outcome`
  `SAVED | MADE_LIVE | DRAFT | DRAFT_EXISTS | UNCHANGED`, `version`,
  `awaitingPromote`, `message`). Without the promote permission a create or
  update saves a draft (`save.outcome === "DRAFT"`), never a refusal.
- `IngestionVersion` gains `draft`, `digest` and `save` (on `saveVersion`'s
  answer).
- `IngestionFailure` gains `category` (`"payload" | "rejected"`; rejected is a
  notice), `createdAt`, `dismissedAt` and `dismissedBy`.
- `listFailures(id, filters?)` takes `FailureFilters` (`CursorPage` plus
  `category` and `undismissed`); existing `{ after, limit }` calls are unchanged.
- New `dismissFailure(id, failureId)` (`POST
  /api/ingestions/{id}/failures/{failureId}/dismiss`) and `listSecrets(id)`
  (`GET /api/ingestions/{id}/secrets`: the bound secret names and each runtime's
  last fetch; never a value).
- Typed `SftpIngestionSource`, with the new optional `acceptAnyHostKey`.

### Platform config
- `set`, `delete` and `revealSecret` take an optional `onBehalfOf`
  (`PlatformConfigActorOptions`), sent as `X-On-Behalf-Of`: who a service
  account acts for, recorded in the audit row (honoured only from a service
  account; at most 256 printable characters, else `400 INVALID_ON_BEHALF_OF`).
- Docs: reveal needs `platform:admin:config:reveal` or
  `platform:application-service:config:reveal`; an application's service
  account is confined to the clients it is linked to. Writing `***` or an empty
  value to a SECRET is refused (`400 SECRET_VALUE_MASKED` /
  `SECRET_VALUE_REQUIRED`).

### Generated client
- `openapi/` refreshed from the platform (`make sdk-spec`) and `src/generated/`
  regenerated: adds the ingestion, ingestion-runtime and function-trace
  operations and types that were missing from the snapshot.
