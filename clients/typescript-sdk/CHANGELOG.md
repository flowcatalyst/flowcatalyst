# Changelog

Notable changes to `@flowcatalyst/sdk`. Releases are tagged
`typescript-sdk/vX.Y.Z` in the platform repository.

## Unreleased

Additive: no signature breaks (new parameters are optional).

### Dispatch jobs
- Fix: the outbox `DISPATCH_JOB` payload sends `metadata` as the platform's list
  of `{key, value}` (insertion order). It was sent as a JSON object, which the
  platform had to convert on forward.
- `connectionId` and `idempotencyKey` were already sent and are now honoured by
  the platform: the connection's service account signs a subscription-less job,
  and a repeated key answers the first job's id instead of creating another
  (within the job's application and client; at most 100 characters).
- Metadata keeps the order pairs were added in, integer-like keys included (an
  object reorders those first). New `withMetadata(key, value)` overload; an
  existing key keeps its place and takes the new value. `metadata` is now a
  read-only object view of the new `metadataPairs`.
- `headers`, `retryStrategy`, `scheduledFor` and `expiresAt` (and their withers)
  are deprecated and no longer sent: the batch route never honoured them.

### Field workers
- `FieldTokenVerifier` / `verifyFieldToken` and `FieldPrincipal`: offline
  verification of field-worker access tokens (RS256, `typ fc-field+jwt`,
  `token_use field`, `aud` = app code) and their DPoP proofs (ES256, RFC 7638
  `jkt` binding, `htm`/`htu`/`ath`, `iat` window). `FieldError.reason` is one of
  the shared `FieldReason` strings; `pcr` tokens are refused. Key discovery and
  caching use `jose`.
- Fastify: `fieldAuth` plugin (`@flowcatalyst/sdk/fastify`) decorates
  `request.fieldPrincipal` on routes using `app.fcField.require()`. It never
  writes `request.principal`. `pcr` -> `403 PIN_CHANGE_REQUIRED`, other
  refusals -> `401` + `WWW-Authenticate: DPoP error="invalid_token"`.
- Admin resources `fieldWorkers()`, `fieldDevices()`, `fieldPolicies()`,
  `fieldTokens()` and `introspectFieldToken(token)`.
- The Fastify bearer validator now enforces a `token_use` allowlist (`api`,
  `identity`, or absent): a field token, or any other plane's token, no longer
  authenticates as a platform principal even when no `expectedAudience` is set.

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
- `ConfigResponse` (list rows and get/set answers) gains optional
  `clientIdentifier`, the client's identifier beside the stored `clientId`, so
  a caller holding either can match a row.
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
