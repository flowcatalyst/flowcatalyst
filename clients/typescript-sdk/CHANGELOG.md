# Changelog

Notable changes to `@flowcatalyst/sdk`. Releases are tagged
`typescript-sdk/vX.Y.Z` in the platform repository.

## Unreleased (after 0.22.0)

Breaking: the return type of `applications().provisionServiceAccount()`, and
`applications().updateClientConfig()` is removed. Needs a platform with
`GET /api/applications/{id}/service-account` for `getServiceAccount()`.

### Changed (breaking)
- `applications().provisionServiceAccount(id)` now resolves to the generated
  `ApplicationProvisionServiceAccountResponse`, the shape the platform sends:
  `{ message, serviceAccount: { principalId, name, oauthClient: { id, clientId, clientSecret }, webhook?: { authToken, signingSecret } } }`.
  It was typed as `{ serviceAccountId, clientId, clientSecret }`, which the
  platform never sent, so those fields were `undefined` at run time. Adapt:
  `res.clientId` becomes `res.serviceAccount.oauthClient.clientId`,
  `res.clientSecret` becomes `res.serviceAccount.oauthClient.clientSecret`,
  and `res.serviceAccountId` becomes `res.serviceAccount.principalId` (the
  SERVICE principal, `prn_…`). New: `res.serviceAccount.webhook.signingSecret`
  (what `verifyDeliverySignature` and `ScheduledJobRunner` verify with) and
  `.authToken`, shown this once.
- Generated client refreshed from the platform spec: the app-updates API
  (`listAppUpdateBranches`, `listAppUpdates`, `getAppUpdate`, `publishAppUpdate`,
  `republishAppUpdate`, `setAppUpdateRollout`, `rollbackAppUpdates`,
  `findMissingAppUpdateFiles`, `putAppUpdateFile` and their types), the optional
  `time` on `BatchEventItem`, `webhook` on `ApplicationServiceAccountCredentials`.
  The types `RawEventResponse`, `RawEventResponseWritable` and
  `RawDispatchJobResponse` are gone: no operation used them (they belonged to
  routes the spec does not publish).

### Fixed
- `applications().getServiceAccount(id)` called
  `GET /api/applications/{id}/service-account`, which the platform did not
  have (only the `POST` that attaches an account is there), so it always failed
  (405). The platform now has the route; the method calls it through the
  generated client and resolves to the generated `ServiceAccountResponse`, the
  body of `GET /api/service-accounts/{id}`: the account's own `id` (`sac_…`),
  `code`, `name`, `principalId` (the SERVICE principal, which is what the
  application's `serviceAccountId` holds), `oauthClientId` and the rest; never
  a secret. Its hand-written type had only `id`, `code`, `name`,
  `description`, `active`, `applicationId`, `createdAt`, so code using them
  compiles unchanged. An application without an account is `not_found`.
  Against a platform that predates the route it still fails, with 405.
- A test (`tests/wrapper-paths.test.ts`) now reads every hand-written wrapper
  and fails when one calls a path or method that is not in the spec the
  package carries.

### Removed (breaking)
- `applications().updateClientConfig(id, clientId, data)` and its
  `ClientConfigRequest` type: it sent `PUT /api/applications/{id}/clients/{clientId}`,
  which the platform has never had, so it could only fail. Adapt:
  `enableForClient(id, clientId)` / `disableForClient(id, clientId)` for
  `enabled`; the platform's API has no way to set `baseUrlOverride` or `config`
  (the Go SDK dropped the method for the same reason).

## 0.22.0 and earlier

These entries were written under "Unreleased" and shipped in the releases up
to `typescript-sdk/v0.22.0`.

Additive: no signature breaks (new parameters are optional).

### Security (behaviour changes)
- Fastify plugin: the `fc_oauth_state` cookie could be replayed as the session cookie and authenticated an anonymous caller. Cookie payloads now carry `purpose` and `cookie` (name) inside the encrypted JSON (envelope unchanged, so old and new versions interoperate during a rolling deploy); `CookieSessionStore` rejects a mismatched purpose or cookie name, and legacy payloads that are state-shaped (`principal.id === "_oauth_state"`) or have an empty access token. Two registrations sharing one secret no longer accept each other's newly written cookies. New optional `CookieSessionStoreOptions.purpose` (default `"session"`).
- `returnTo` and relative logout `redirectTo` are now validated as same-origin paths (single leading `/`, no backslash, no control characters, stays on origin); otherwise `/`. New option `allowedLogoutRedirectOrigins` (default unset = absolute logout targets still honoured). Exported `sanitizeReturnTo`/`isSafeLocalPath`/`resolveLogoutRedirect` from the plugin module.
- Bearer path: a token with no `token_use` that is ID-token-shaped (`nonce`, `auth_time`, `at_hash`, or `azp` equal to its single `aud`) is now rejected. New option `acceptIdTokenAsBearer` (default `false`) restores the old behaviour. The `expectedAudience` doc now states the real default (none: audience unchecked).
- `ScheduledJobRunner`: new `processSigned(rawBody, headers)` verifies the platform's HMAC signature (reusing `verifyDeliverySignature`). New options `signingSecret` and `requireSignature` (default `false`). With `signingSecret` set, `process(envelope)` returns 401 (use `processSigned`). With no secret, both run as before and log one warning per runner. `RunResult` status now includes `401`.

### Fixed
- Events written through `OutboxManager` now carry `deduplicationId` = the outbox row id unless `withDeduplicationId` sets one, so a batch the outbox processor sends again no longer creates duplicate events (as the Go and Java SDKs do). `CreateEventDto.toPayload(rowId?)` takes the row id.

### Push to clients (push gateway)
- `PUSH` outbox rows: `CreatePushDto` and `OutboxManager.createPush` /
  `createPushes`. Validated at write (`PushError`: `TOPIC_INVALID`,
  `TYPE_INVALID`, `TYPE_RESERVED`, `DATA_INVALID`, `DATA_NOT_OBJECT`,
  `DATA_TOO_LARGE`, ...), message group = topic unless `withMessageGroup` /
  `withUngrouped`, `pushId` = row id. `OutboxManager` takes an optional
  `{ clientCode }` (the default client of a push). Passes the shared
  `outbox-push.json` vector.
- `PgOutboxDriver` writes `PUSH` rows with `pg_notify('fc_outbox_push')` in the
  same statement, so the platform's outbox processor wakes at commit. Other
  drivers and databases fall back to the processor's poll.
- `usecase.Plan.andPush(plan, ...pushes)`: pushes written in the plan's
  transaction, after the event.
- `defineApplication(...).withPush({ rules, keys })`, `DefinitionSet.push`,
  `SyncOptions.skipPush`, `SyncResult.push` and
  `client.definitions().syncPush(appCode, push)`: topic rules and ticket keys,
  `POST /api/applications/{appCode}/push/sync` (a full replace).
- `TicketMinter` / `TicketKey`: mint push tickets from a `kid:base64url-seed`
  Ed25519 key and export the JWKS for sync. Reproduces the framework's
  `conformance/push/tickets.json` tickets byte for byte.
- Generated client refreshed from the platform spec (push sync, push settings).

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

### Field workers: sign-in options
- `FieldPrincipal.deviceId` is `""` (and `deviceMode` `"UNENROLLED"`) when the
  token's key is not an enrolled device. New `FieldPrincipal.actor`
  (`FieldActor | null`: `{ id, type, name }` from the `act` claim; `type` is
  `field_worker` for a supervisor or `principal` for a platform user) and
  `FieldPrincipal.test`. Every other field is still the WORKER's, so an app
  records the worker as the subject of an action and `actor` as the operator.
  Passes the four new shared `api/field-fixtures` cases.
- `fieldWorkers()`: `issueBadge(id, { label? })`, `listBadges(id)`,
  `revokeBadge(id, badgeId)`, `bulkIssueBadges(workerIds)` (1 to 200, all or
  nothing) and `createSignInCode(id, appCode)`. Badge and sign-in-code secrets
  are returned once. `ensure`/`update` take `canImpersonate` and `isTest`;
  `list` filters on `isTest`.
- Regenerated client: policy `requireEnrolledDevice`, `unenrolledIdleSecs`,
  `badgeOnlyLogin`, `impersonationMaxSecs`; introspection `act` and `test`.

### Field workers: linked platform user
- `FieldPrincipal.linkedPrincipalId` (`string`, `""` when the worker is not
  linked, like `deviceId`): the platform user the worker is linked to, from the
  `linked_principal_id` claim. Audit correlation only: it grants nothing.
  Passes the shared `valid_linked` fixture case.
- `fieldWorkers()`: `link(id, principalId)` (422 `INVALID_LINK`, 409
  `LINK_EXISTS`) and `unlink(id)` (idempotent); worker responses carry
  `linkedPrincipal`; `list` filters on `linkedPrincipalId`.
- Regenerated client: link/unlink operations, `linkedPrincipal`, introspection
  `linkedPrincipalId`.

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
