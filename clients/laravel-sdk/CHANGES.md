# Changes

## 0.23.0

- Additive: field-worker photos (`docs/field-worker-photo-spec.md`). `fieldWorkers()->setPhoto()` (the image
  as the request body), `getPhoto()` (bytes, ETag, and `notModified` for a 304 to `$ifNoneMatch`; the
  generated client drops an image answer, so it is read by hand and returned as
  `FlowCatalyst\DTOs\FieldWorkerPhotoImage`) and `removePhoto()`; `fieldPolicies()->deletePhotos()`;
  `FieldWorkerResponse::getPhoto()`; `FieldPolicyResponse` and `PutFieldPolicyRequest` gain `photosEnabled` and
  `workerMaySetOwnPhoto` (a put replaces the policy: a field left out takes its default). They need a
  platform that has the photo routes.

## 0.22.0

- Additive: `flowcatalyst.field.app_base_url` may be a list of origins (an array, or a comma-separated
  string, so `FLOWCATALYST_FIELD_APP_BASE_URL` can carry several), and an entry may have `*` as the whole
  leftmost label of its host (`https://*.example.com`), which matches exactly one label. For an
  application that serves each client on its own host. With several entries or a wildcard the `field`
  guard reads the request's host, takes the first entry it matches and expects the proof's `htu` to be
  that entry's scheme and port plus the request's host and path (the request's own scheme and port are
  never used). A host that matches no entry is `htu_mismatch`. A wildcard must cover a DNS zone you wholly
  control. An invalid entry (no scheme or host, a `*` anywhere but the whole leftmost label, a wildcard
  with fewer than two labels after it) throws a `LogicException`. New `FlowCatalyst\Auth\Field\FieldOrigins`.
  With several entries or a wildcard, a bound `FieldClientCheck` (next entry) is REQUIRED: with none, the
  guard throws a `LogicException` on the first field request, before the token is read. Nothing changes
  for an application that configures one origin and no wildcard: the host is still never read and no
  check is needed.
- Additive: `FlowCatalyst\Auth\Contracts\FieldClientCheck`, an interface an application may bind to say
  whether a verified field worker may use this request (for example, whether the worker's client is the
  one the request's host belongs to). The SDK binds none. When one is bound, the `field` guard calls it
  once per request after the token and proof verified; `false` gives no user and the new reason
  `FieldTokenException::REASON_WRONG_CLIENT` (`wrong_client`), which `fc.field` answers as
  `403 {"error":"WRONG_CLIENT"}` with no `WWW-Authenticate` header, so a device library does not end a
  good session. A `pcr` token is still answered `PIN_CHANGE_REQUIRED` first; an exception from the check
  is reported and answered `503`. `FieldTokenVerifier::verify()` does not run it. Nothing changes for an
  application that configures one origin and binds none.

## 0.21.0

- Behaviour change: a platform token or a DPoP proof verifies only in the spelling it was issued in
  (`docs/wire-contract.md`, "Token spelling"). `JwtSignature::verify()` (and so `AccessTokenValidator`,
  `IdTokenValidator`, the `fc-token` guard and `AuthenticateFc`) and `FieldTokenVerifier` re-padded the
  base64url, read `+` and `/`, skipped whitespace and line breaks and ignored the spare bits of a
  segment's last character, so one token had many spellings that all verified. Now every segment, and a
  proof key's `x` and `y`, must be canonical base64url (URL-safe alphabet, no `=`, no whitespace, spare
  bits zero; `JwtSignature::isCanonical()`): a re-spelled access or ID token is refused (null), a
  re-spelled field token is `malformed`, a re-spelled proof, or one with whitespace around it,
  `invalid_proof`. Whitespace around the `Authorization` header value is still trimmed (the header's
  framing). Nothing the platform issues is affected: its tokens (the platform's ID tokens included) and
  the field libraries' proofs are written by standard encoders and are always canonical, and the shared
  vectors in `api/field-fixtures/cases.json`, minted before the change, all still verify. What stops
  working is only a token or proof someone re-spelled after it was issued, or a proof from a signer of
  your own with a non-standard base64url encoder.
- Additive: `EnsureFieldWorkerRequest::setWorkerCode()` is now optional. Without a code the platform
  generates one (three letters, then the client policy's `workerCodeDigits` digits, e.g. `KTM482`) and
  `FieldWorkerResponse::getWorkerCode()` carries it. New `EnsureFieldWorkerRequest::setIdempotencyKey()`
  (only without a code): a repeat with the same key answers the worker the first call made. The
  field-worker wrappers go through the generated client, which resends nothing after a lost answer,
  so behaviour on errors is unchanged. New `workerCodeDigits` (3 to 6, default 3) on
  `PutFieldPolicyRequest` and `FieldPolicyResponse`. A request with a worker code is sent as before.
- Generated client (`src/Generated`) refreshed from the platform spec: app binaries and install links
  (ten operations, `putAppBinaryFile` … `removeAppInstallLink`, and their models),
  `PublishUpdateRequest::requireBinary`, `AppUpdateResponse::binariesOnBranch`. Additive.

## 0.20.0

- Breaking: `Applications::provisionServiceAccount()` (on `$client->admin()->applications()` and the
  deprecated `$client->applications()`) returns the new `FlowCatalyst\DTOs\ProvisionedServiceAccount`
  instead of `ServiceAccount`. It mapped the platform's answer
  (`{message, serviceAccount: {principalId, name, oauthClient, webhook}}`) into a `ServiceAccount`, which
  has none of those keys, so it returned empty fields and lost every credential. The new DTO carries
  `message`, `principalId`, `name`, `oauthClientId` (`oac_…`), `clientId`, `clientSecret`,
  `webhookAuthToken` and `webhookSigningSecret` (null from a platform that predates them); the secrets
  are redacted in `toArray()`, JSON, `print_r`/`var_dump` and `serialize()`, like `PlatformConfigSecret`.
  Adapt: `$sa->id` becomes `$p->principalId`, and read the client and webhook credentials from the
  new properties (`$p->webhookSigningSecret` is what `FLOWCATALYST_SIGNING_SECRET` must hold).
- Generated client (`src/Generated`) refreshed from the platform spec: the app-updates API (nine
  operations, `listAppUpdateBranches` … `putAppUpdateFile`, and their models), `BatchEventItem::time`,
  `ApplicationServiceAccountCredentials::webhook`. The models `RawEventResponse` and
  `RawDispatchJobResponse` and their normalizers are removed; no operation used them.

- Fix: `Applications::getServiceAccount()` (on `$client->admin()->applications()` and the deprecated
  `$client->applications()`) called `GET /api/applications/{id}/service-account`, which the platform did not
  have (only the `POST` that attaches an account is there), so it always failed (405). The platform now has
  the route; it answers the body of `GET /api/service-accounts/{id}`. `ServiceAccount` gains the fields that
  body carries, all optional trailing constructor arguments: `principalId` (the SERVICE principal, `prn_…`,
  which is what an application's `serviceAccountId` holds), `oauthClientId` (the public OAuth client id),
  `authType`, `clientIds`, `roles`, `scope`, `lastUsedAt`, `updatedAt`; `$id` is the account's own id
  (`sac_…`). Never a secret. An application without an account throws a 404 `FlowCatalystException`. Against
  a platform that predates the route it still fails, with 405.
- Breaking: `Applications::updateClientConfig()` and `FlowCatalyst\DTOs\Requests\ClientConfigRequest` are
  removed. The method sent `PUT /api/applications/{id}/clients/{clientId}`, which the platform has never had,
  so it could only fail. Adapt: `enableForClient()` / `disableForClient()` for `enabled`; the platform's API
  has no way to set `baseUrlOverride` or `config` (the Go and TypeScript SDKs dropped the method too).
- A test (`tests/Unit/Client/WrapperPathsTest.php`) calls every public method of every resource with
  placeholder arguments and fails when one sends a request whose method and path are not in the spec the
  package carries (`openapi/openapi.json` and its app and admin splits).

- Breaking (behaviour): a POST that may have been applied is no longer sent again. `request()` retried
  408, 429, 502, 503, 504 and every transport failure whatever the method, so a POST the platform had
  committed, whose answer a gateway lost, was sent again: `provisionServiceAccount()` was then refused
  `409 ALREADY_PROVISIONED` and the first answer's one-time secrets were gone; a create could make a
  duplicate. Now 408, 429 and a connection that was never made (cURL 5, 6, 7, 35, or nothing written) are
  retried for every request; 502, 503, 504 and a connection lost or timed out after sending only for GET,
  HEAD, OPTIONS, PUT, DELETE and the POSTs the SDK marks repeatable (the declarative syncs,
  `principals()->syncUsers()`, the push and OpenAPI syncs of `definitions()`, and a
  `dispatchJobs()->batch()` whose every job has an idempotency key). Any other request throws the new
  `FlowCatalyst\Exceptions\OutcomeUnknownException` at once (a `FlowCatalystException`: `$method`,
  `$endpoint`, `$status`, the gateway's body as `getContext()`, the transport failure as `getPrevious()`).
  A DELETE whose retry answers 404 after an unknown outcome returns `[]`. `retryAttempts` and
  `retryDelay` (and their config keys) keep their names, defaults and meaning. Adapt: where you create or
  change something, catch `OutcomeUnknownException` before `FlowCatalystException` and read the state
  (after a provisioning, `getServiceAccount()`) before sending again; code that caught the 502 or a
  "Request failed" transport `FlowCatalystException` from a POST still catches it, now as the subclass.
  `request()` takes `repeatable: true` for a custom POST that is safe to send twice (also on the facade).
  README, "Retries".

## 0.19.0 and earlier

These entries were written under "Unreleased" and shipped in the releases up to `laravel-sdk/v0.19.0`.

- Security (L14): a token whose `kid` is not in the cached JWKS now causes at most ONE forced discovery+JWKS
  refetch per issuer per 60 seconds (`JwksCache::keysAfterUnknownKid()`; interval and clock injectable). Inside
  the window an unknown `kid` is rejected with no network call, and a failed refetch keeps the previously
  cached key set until its normal expiry. Consequence: a token signed by a key rotated in within the last 60
  seconds is refused until the window passes.
- Security (L15): the OIDC callback regenerates the session id (data kept) immediately before the user handler
  runs, so a pre-sign-in session id is useless afterwards. New config `flowcatalyst.oidc.logout_methods`
  (default `['get', 'post']`, unchanged); set `['post']` to stop GET logouts (recommended).
- Security (L17): `OidcTokenManager` caches the client-credentials token under the configured key plus a short
  hash of base URL and client id (an explicit `$cacheKey` argument gets the suffix too, since a caller-chosen
  key shared by two managers could collide). Cost: one extra token fetch after upgrade.

- Security: the Bearer guard (`AccessTokenValidator`) now refuses an OIDC ID token presented as an API
  bearer. A token with no `token_use` that is ID-token-shaped (carries `nonce`, `auth_time` or `at_hash`, or
  has `azp` equal to its single `aud`) is rejected after the signature and issuer checks. Tokens with
  `token_use` `api` / `identity`, and marker-less tokens with none of those signs, behave as before. New
  config `flowcatalyst.oidc.accept_id_token_as_bearer` (env `FLOWCATALYST_OIDC_ACCEPT_ID_TOKEN_AS_BEARER`,
  default `false`) restores the old behaviour; `AccessTokenValidator` takes it as a trailing constructor
  argument `acceptIdTokenShapedTokens`. The login controller's ID-token validation is untouched.
- Fix: events written through `Outbox::createEvent()` / `createEvents()` now carry `deduplicationId` = the outbox row id unless `withDeduplicationId()` sets one, so a batch the outbox processor sends again no longer creates duplicate events (as the Go and Java SDKs do). `CreateEventDto::toPayload()` takes the row id.
- Push to clients (push gateway): `PUSH` outbox rows (`CreatePushDto`, `Outbox::createPush()` /
  `createPushes()`; validated at write, `PushException` with the contract's codes; message group = topic
  unless `withMessageGroup()` / `withUngrouped()`; `pushId` = row id). Passes the shared
  `outbox-push.json` vector. On PostgreSQL the `DatabaseDriver` writes the row with
  `pg_notify('fc_outbox_push')` in the same statement so the platform's outbox processor wakes at commit;
  MySQL and MongoDB fall back to the processor's poll. `OutboxManager` takes an optional `clientCode`
  (wired from `flowcatalyst.outbox.client_code`).
- `Plan::andPush(...)` (`PushesUnitOfWork`, implemented by `OutboxUnitOfWork`): pushes written in the plan's
  transaction, after the event; an invalid push fails the plan and rolls everything back.
- `flowcatalyst:sync` (and `SyncDefinitionSet::withPush()`, `SyncOptions::$syncPush` / `pushOnly()`,
  `SyncResult::$push`, `--push`) sends the topic rules and ticket keys declared in `flowcatalyst.push` to
  `POST /api/applications/{appCode}/push/sync` (a full replace).
- `TicketMinter` / `TicketKey` (ext-sodium): mint push tickets from a `kid:base64url-seed` Ed25519 key and
  export the JWKS for sync. Reproduces the framework's `conformance/push/tickets.json` tickets byte for byte.
- Generated client refreshed from the platform spec (push sync, push settings).

- Dispatch jobs: new `dispatchJobs()` resource (also on the `FlowCatalyst` facade) that calls
  `POST /api/dispatch-jobs/batch` directly: `batch(array $jobs)` returns one `DispatchJobResult`
  (`id`, `status`, `error`, `isSuccess()`) per job in order, and `create($job)` returns the job's id or
  throws `FlowCatalystException` on the item's error. It sends the same item as the outbox
  (`CreateDispatchJobDto::toBatchItem()`, which `toPayload()` now returns).
- `CreateDispatchJobDto` gains `clientCode` and `clientId` (trailing constructor arguments,
  `withClientCode()`, `withClientId()`), so a job can carry its client and its deliveries the
  `X-FlowCatalyst-Client` header.
- Fix: `metadata` is sent as the platform's list of `{key, value}`. An associative array (still
  accepted) used to be sent as a JSON object, which made the platform refuse the whole batch.
- `connectionId` and `idempotencyKey` were already sent and are now honoured by the platform: the
  connection's service account signs a subscription-less job, and a repeated key answers the first
  job's id instead of creating another (within the job's application and client; at most 100 characters).
- `headers`, `retryStrategy`, `scheduledFor` and `expiresAt` are no longer sent: the batch route never
  honoured them. `withHeaders()`, `scheduledFor()` and `expiresAt()` are deprecated no-ops kept for
  source compatibility. An empty `dispatchPoolId` is now omitted instead of sent as `""`.
- The DTO's withers now copy through one helper, so a wither can no longer drop a field.
- Field-worker identity: `FieldTokenVerifier` verifies field access tokens (RS256 only, `typ fc-field+jwt`,
  `token_use field`, `aud` = app code, `pcr` refused) and their DPoP proofs (ES256 only, RFC 7638 `jkt`,
  `htm`, `htu`, `ath`, `iat` +-5 min), returning a `FieldPrincipal`; refusals throw `FieldTokenException`
  with the shared reason names. New `field` auth guard and `fc.field` middleware (`pcr` -> 403
  `PIN_CHANGE_REQUIRED`, other refusals -> 401 with `WWW-Authenticate: DPoP error="invalid_token"`, JWKS
  failure -> 503). Config `flowcatalyst.field` (`app_code`, `app_base_url`, `proof_max_age_seconds`,
  `clock_skew_seconds`). New `fieldWorkers()`, `fieldDevices()`, `fieldPolicies()`, `fieldTokens()` and
  `introspectFieldToken($token)` over the generated client. Passes the shared `api/field-fixtures` vectors.
- Field-worker sign-in options: `FieldPrincipal::$deviceId` is `''` (and `$deviceMode` `UNENROLLED`) when the
  token's key is not an enrolled device. New trailing constructor arguments/properties `FieldPrincipal::$actor`
  (`FieldActor|null`: `id`, `type` = `field_worker` or `principal`, `name`, from the `act` claim) and
  `FieldPrincipal::$test`. Every other property is still the WORKER's: record the worker as the subject of an
  action and `$actor` as the operator. `fieldWorkers()` gains `issueBadge($id, ?$label)`, `listBadges($id)`,
  `revokeBadge($id, $badgeId)`, `bulkIssueBadges(array $workerIds)` and `createSignInCode($id, $appCode)`
  (badge and sign-in-code secrets are returned once); `list()` takes an `isTest` filter; `ensure`/`update`
  requests carry `canImpersonate` and `isTest`. Passes the four new shared fixture cases.
- Field-worker linked platform user: `FieldPrincipal::$linkedPrincipalId` (trailing constructor argument,
  `''` when the worker is not linked, like `$deviceId`) from the `linked_principal_id` claim; audit
  correlation only, it grants nothing. `fieldWorkers()` gains `link($id, $principalId)` (422 `INVALID_LINK`,
  409 `LINK_EXISTS`) and `unlink($id)` (idempotent); worker responses carry `linkedPrincipal`; `list()` takes a
  `linkedPrincipalId` filter. Passes the shared `valid_linked` fixture case; generated client regenerated.
- Generated client regenerated with jane-php 7.14 (it was built with 7.11): query parameters that are
  null are now left out instead of sent empty, header names match case-insensitively, and the runtime
  gains JSON payload and form encoders. `jane-php/open-api-runtime` and `jane-php/open-api-3` now require
  `^7.14`.
- `AccessTokenValidator` (the `fc-token` guard and `fc.auth` middleware) now treats `token_use` as an
  allowlist: `api`, `identity` (accepted as before) or an absent claim; `field` and any other class are refused.
- Ingestions: `Ingestion` gains `notices`, `latestNotice`, `latestNoticeAt`, `warnings` and `save`
  (new `IngestionSaveOutcome`: `outcome` SAVED | MADE_LIVE | DRAFT | DRAFT_EXISTS | UNCHANGED,
  `version`, `awaitingPromote`, `message`); `IngestionVersion` gains `draft`, `digest` and `save`.
  Without the promote permission a create or update saves a draft (`save->outcome === 'DRAFT'`).
  `failures()` takes `category` (payload | rejected) and `undismissed` (failure rows now carry
  `category`, `createdAt`, `dismissedAt`, `dismissedBy`); new `dismissFailure($id, $failureId)` and
  `secrets($id)` (bound names and each runtime's last fetch, never a value). sftp sources may set
  `acceptAnyHostKey`. All additive (new trailing optional parameters and constructor arguments).
- Platform config: `set()`, `delete()` and `revealSecret()` take a trailing `?string $onBehalfOf`, sent as
  `X-On-Behalf-Of` (`PlatformConfig::ON_BEHALF_OF_HEADER`) and recorded in the audit row.
  `PlatformConfigProperty` gains `clientIdentifier`. New constants `PlatformConfig::ERROR_SECRET_VALUE_MASKED`
  and `ERROR_SECRET_VALUE_REQUIRED` (writing `***` or an empty value to a SECRET is refused).
- Generated client regenerated from the refreshed spec (`make sdk-spec`, `prepare-openapi.php`, `jane-openapi
  generate`): adds the ingestion, ingestion-set and ingestion-runtime endpoints and models, the
  `X-On-Behalf-Of` header parameter on the platform-config endpoints, `ConfigResponse::clientIdentifier`, and
  the function-trace fields already in the spec.
- Docs: `platformConfig()->revealSecret()` needs a reveal permission; being the application's own
  service account is not enough. The account needs `platform:application-service:config:reveal`
  (carried by the `platform:application-service` role), and the platform confines it to its own
  applications and to the clients it is linked to (its explicit clients, else the clients its
  application is enabled for, plus the application-wide value). The entry below that says "or the
  application's own service account" is superseded by this one. Writing `***` back as a SECRET's
  value is refused by the platform (`400 SECRET_VALUE_MASKED`).
- New `ingestions()` and `ingestionSets()` resources for the ingestion API: `ingestions()->list/create/get/update/delete/pause/resume`,
  `versions/saveVersion/makeVersionLive`, `definition` (the YAML file as a string), `history`, `failures`, `sourceKinds`,
  `testConnection/getTest`; `ingestionSets()->list/apply/applies/getApply/reapply/release` (`dryRun` on apply and reapply).
  Typed `IngestionDefinition`, `IngestionSource` (any built-in kind, `IngestionSource::custom($type, $settings)` for a
  runtime-supplied one), `IngestionSplit`, `IngestionTarget`, `Ingestion`, `IngestionVersion`, `IngestionList` under
  `FlowCatalyst\DTOs\Ingestion`. Secrets in a definition are names, never values. Hand-written (the generated client is
  not regenerated for these); the ingestion-runtime routes are deliberately not exposed.
- `FlowCatalystClient::request()` returns a 2xx body that is not JSON whole under `FlowCatalystClient::RAW_BODY`
  (previously `[]`).

- New `platformConfig()->revealSecret($app, $section, $property, ?$clientId)` returning
  `PlatformConfigSecret` (`GET /api/config/{app}/{section}/{property}/secret`, audited; needs
  `platform:admin:config:reveal` or the application's own service account). `get`/`list` now
  always return `***` for SECRET values. The reveal is uncached and the DTO redacts `value` in
  var_dump/JSON/toArray()/serialize. Generated client regenerated (adds `RevealPlatformConfigSecret`
  plus the function-trace endpoints already in the spec).

Additive only; nothing removed, renamed or behaviour-changed (the only runtime difference is the
deprecation notice described below).

- New `platformConfig()` resource: `get`, `getRaw`, `set`, `delete`, `list`, `listRaw`
  (`/api/config/{app}/{section}/{property}`, `/api/platform-config/{app}`).
- New `emailDomainMappings()` resource: `getByDomain`, `getByDomainRaw`
  (`GET /api/email-domain-mappings/by-domain/{domain}`).
- `principals()->setClientAssociation($id, $clientId, ?$mode)`
  (`PUT /api/principals/{id}/client-association`, anchor-only).
- `clients()->getRaw($id)` and `getByIdentifierRaw($identifier)` return the platform's
  decoded body unchanged (the `Client` DTO defaults a missing `status` to `ACTIVE`).
- `roles()->sync()` failures from the atomic platform sync ("... failed at index N") now
  append the failing role name and the platform's error code/details to the message and
  add `failedIndex`/`failedRole` to the exception context. The original message is kept
  as the prefix, the code is unchanged and the original exception is chained as `previous`.
  Subclasses (e.g. `ValidationException`) are not touched.
- Tests pin `EventTypeDefinition::toArray()['code']` and `AddSchemaVersionRequest` always
  sending `version` (both already correct since 0.10.0; apps on older versions should upgrade
  rather than keep their workarounds).

### `admin()` facade and deprecation notices

- New `FlowCatalystClient::admin(): Admin` (also `FlowCatalyst::admin()`), exposing `principals()`,
  `clients()`, `applications()` and `router()`. They are the same resource classes as the old
  accessors and behave identically; the facade is just the quiet path.
- The old accessors keep working. These methods now raise one `E_USER_DEPRECATED` per method per
  process (suppressible via `error_reporting()`/`@`/handler; never raised through `admin()`), and
  carry `@deprecated` docblocks:
  - `principals()`: `list`, `get`, `findByEmail`, `createUser`, `resetPassword`, `update`, `activate`,
    `deactivate`, `getRoles`, `addRole`, `removeRole`, `setRoles`, `getClientAccessGrants`,
    `grantClientAccess`, `revokeClientAccess`.
  - `clients()`: `search`, `create`, `update`, `activate`, `suspend`, `deactivate`, `addNote`,
    `getApplications`, `updateApplications`, `enableApplication`, `disableApplication`.
  - `applications()`: `list`, `create`, `update`, `delete`, `activate`, `deactivate`,
    `provisionServiceAccount`, `getServiceAccount`, `listClients`, `updateClientConfig`,
    `enableForClient`, `disableForClient`.
  - `router()`: `inPipeline`, `inPipelineBatch`.
- Not deprecated: all `sync*` methods (`principals()->sync/syncUsers`, `roles()->sync`, ...),
  `clients()->list/get/getByIdentifier/getRaw/getByIdentifierRaw`,
  `applications()->get/getByCode/listRoles`, the new `principals()->setClientAssociation`,
  platform config, email-domain mappings, outbox, dispatch-job creation, auth, `request()` and the
  constructor options. Nothing is removed.
- Test doubles: a Mockery/subclass double that stubs `principals()` etc. does not intercept
  `admin()`; stub `admin()` (the `Admin` class is not final) when moving a call site.

Known issue, not fixed here: `Clients::update()` PUTs and then parses a body with
`Client::fromArray()`, but the platform answers `204 No Content`, so the decoded body is empty and
the call fails (missing `id`). Use `get($id)` after an update until it is fixed.

### Migrating to admin()

Pure search/replace on the accessor chain; arguments are unchanged. Paths are relative to
`/Users/andrewgraaff/Developer/inhance/InhanceMono`. Calls that stay as they are are listed so nobody
touches them by mistake. A test double that stubs the accessor (`shouldReceive('principals')` /
`applications`) must instead stub `admin()` returning an `Admin` whose `principals()` etc. return the
fake.

**apps/hr** (pinned `^0.8`, locked 0.8.19: `admin()` needs a constraint change and a 0.8 to 0.10 review first)

| File | Replace | With |
|---|---|---|
| `packages/Foundation/Domain/Services/PrincipalDirectory.php:151` | `$this->client->principals()->list(active: true)` | `$this->client->admin()->principals()->list(active: true)` |
| `packages/Register/Commands/ImportRolesRegisterCommand.php:275` | `resolve(FlowCatalystClient::class)->principals()->list(type: 'USER', active: true)` | `resolve(FlowCatalystClient::class)->admin()->principals()->list(type: 'USER', active: true)` |
| `packages/Foundation/Domain/Services/FeedbackNotifyRecipients.php:83-86` | `$applications = $this->client->applications(); $applications->getByCode(...)` then `$applications->listClients(...)` | keep `getByCode` on `$this->client->applications()`; change only `->listClients($application->id)` to `$this->client->admin()->applications()->listClients($application->id)` |
| `app/Console/Commands/SyncTestPrincipalsCommand.php:57` | `principals()->sync(` | unchanged (sync stays) |
| tests: `tests/Feature/Foundation/PrincipalDirectoryTest.php`, `FeedbackNotifyRecipientsTest.php` | `$client->shouldReceive('principals')` / `('applications')` for the moved calls | `shouldReceive('admin')` returning a mock `Admin` |

**apps/rfp**

| File | Replace | With |
|---|---|---|
| `packages/Tender/Infrastructure/Directory/PlatformPrincipalGateway.php:51` | `$this->client->principals()->findByEmail(` | `$this->client->admin()->principals()->findByEmail(` |
| same, `:60` | `$this->client->principals()->get(` | `$this->client->admin()->principals()->get(` |
| same, `:106` | `$this->client->principals()->createUser(` | `$this->client->admin()->principals()->createUser(` |
| same, `:128`, `:141` | `$this->client->principals()->addRole(` | `$this->client->admin()->principals()->addRole(` |
| same, `:135` | `$this->client->principals()->removeRole(` | `$this->client->admin()->principals()->removeRole(` |
| `packages/Tender/Infrastructure/Directory/PlatformPrincipalDirectory.php:223` | `$this->client->principals()->list(active: true)` | `$this->client->admin()->principals()->list(active: true)` |
| `app/Console/Commands/SyncTestPrincipalsCommand.php:57` | `principals()->sync(` | unchanged |
| `tests/Architecture/PrincipalWritersTest.php` | pattern that bans `->principals()` chains / `$p = $this->client->principals()` | extend to allow the single gateway to use `->admin()->principals()` and ban it elsewhere |
| `tests/Doubles/ScriptedFlowCatalystClient.php` | overrides at the `request()` level | no change expected (resources route through `request()`); verify |

**apps/integral**

| File | Replace | With |
|---|---|---|
| `app/Console/Commands/FlowCatalyst/CreateFlowCatalystUserCommand.php:103` | `$client->principals()->createUser(` | `$client->admin()->principals()->createUser(` |
| `app/Console/Commands/FlowCatalyst/SyncUsersToFlowCatalystCommand.php:381` | `$client->principals()->findByEmail(` | `$client->admin()->principals()->findByEmail(` |
| same, `:395` | `$client->principals()->update(` | `$client->admin()->principals()->update(` |
| same, `:440` | `$client->principals()->createUser(` | `$client->admin()->principals()->createUser(` |
| same, `:483` | `$client->principals()->getRoles(` | `$client->admin()->principals()->getRoles(` |
| same, `:490` | `$client->principals()->addRole(` | `$client->admin()->principals()->addRole(` |
| same, `:554` | `$client->principals()->grantClientAccess(` | `$client->admin()->principals()->grantClientAccess(` |
| same (raw `request('PUT', '/api/principals/{id}/client-association'...)`) | `$client->request(...)` | `$client->admin()->principals()->setClientAssociation($id, $clientId, $mode)` |
| same, `:163` | `$client->clients()->list()` | unchanged (not deprecated) |
| same, `:599` | `$client->principals()->syncUsers(` | unchanged |

**packages_root** (shared by integral)

| File | Replace | With |
|---|---|---|
| `packages/integral-service-v2/src/Admin/Http/Controllers/SetPasswordController.php:92` | `$this->flowCatalystClient->principals()->findByEmail(` | `...->admin()->principals()->findByEmail(` |
| same, `:98` | `...->principals()->resetPassword(` | `...->admin()->principals()->resetPassword(` |
| `.../Admin/UseCases/ResetUserPasswordUseCase.php:62`, `:68` | `->principals()->findByEmail(` / `->resetPassword(` | `->admin()->principals()->...` |
| `.../Admin/UseCases/UpdateUserUseCase.php:78`, `:84` | `->principals()->findByEmail(` / `->resetPassword(` | `->admin()->principals()->...` |
| `.../Admin/UseCases/CreateUserUseCase.php:122` | `->principals()->createUser(` | `->admin()->principals()->createUser(` |
| `.../Admin/Jobs/RunTenantMigrationsJob.php:137` | `$fc->clients()->create(` | `$fc->admin()->clients()->create(` |
| `.../FlowCatalyst/Commands/SyncFlowCatalystClientsCommand.php:95` | `$this->client->clients()->create(` | `$this->client->admin()->clients()->create(` |
| same, `:59` | `$this->client->clients()->list()` | unchanged (not deprecated) |
| `.../FlowCatalyst/Commands/SyncFlowCatalystDefinitionsCommand.php:600` | `$this->client->applications()->create(` | `$this->client->admin()->applications()->create(` |
| same, `:593` | `applications()->getByCode(` | unchanged |
| `.../ProcessRecord/ProcessRecordInProgressResetterJob.php:90` | `$client->router()->areInPipeline($messageIds)` | `$client->admin()->router()->inPipelineBatch($messageIds)` (see note) |
| `packages/integral-service-v1/src/Legacy/Services/FlowCatalystSyncService.php:100` | `$this->client->applications()->create([...array...])` | `$this->client->admin()->applications()->create(new CreateApplicationRequest(code:, name:, description:))` (see note) |
| same, `:95` | `applications()->getByCode(` | unchanged |

Notes found while checking call sites:

- `areInPipeline` does not exist in the SDK (and never did in this repo's history): the SDK's
  `Router` has `inPipeline(string)` and `inPipelineBatch(array)` (returns `messageId => bool`, which is
  what the Integral caller expects). The current v2 call would throw `Error: undefined method`; today
  the call is inside a `catch (Throwable)` that returns `[]`, and the surrounding code is partly
  commented out. The replacement above is the correct call, and the job should be re-enabled
  deliberately, not as part of the rename.
- The v1 `applications()->create([...])` passes an array to a method typed
  `CreateApplicationRequest`; it only works on an older SDK line. Moving to `admin()` also needs the
  request object.
- Every method the apps use has an `admin()` equivalent; none of the used methods is missing.
