# Java hand-off: the function `platform.token` capability

Go implements it in the commit that adds this file (2026-11-04). The Java platform's function host,
if/when it mirrors functions, needs the pieces below. The spec is `docs/function-platform-token.md`
(owner-approved 2026-11-04); behaviour as built is in `docs/function-runner-plan.md` §5.3, §5.4, §6.3,
§8.3 and §8.8; this is the checklist. The JVM function host in `ingestor/` is a separate piece of work
(spec §6, JVM block) and is not covered here.

## What it is

A function gets a FlowCatalyst access token from its host and sends it as a bearer to another first-party
application that accepts FlowCatalyst tokens (for example Integral's `auth:fc-token` routes). It holds no
client id or secret. The identity is the service account linked to the function's **application**, always:
no per-function account, no override.

## Mirror list

1. **Describe** gains `platformToken: boolean` (`omitempty`). Strict like the rest: any other JSON type is
   refused at publish like any malformed field. The JVM describe (`META-INF/flowcatalyst/function.json`)
   carries the same field.
2. **ABI op 15 `platform.token`** (additive; ABI stays v1). Call meta `{refresh?: boolean}` (may be empty or
   `{}`; no body). Result meta `{accessToken, expiresAt}` where `expiresAt` is unix milliseconds on the
   host's clock; no result body. The host refuses a version whose describe lacks `platformToken: true` with
   `NOT_DECLARED` before calling the platform. A host that predates the op answers `UNKNOWN_OP`.
3. **Control route** `POST /control/functions/token` (runner role, anchor scope), body
   `{functionId, version}` → `200 {accessToken, tokenType: "Bearer", expiresIn}`. Client error mapping (this
   one differs from `emit`/`dispatch`): 5xx / 429 / 408 / unreachable, and a 401 that outlives the one retry
   with a fresh runner token, are `UNAVAILABLE` (retryable); every other 4xx (400/403/404/409) is
   `NOT_ALLOWED` with the message carrying the platform code (`PLATFORM_TOKEN_NOT_DECLARED: ...`); context
   ended `DEADLINE`. The JVM host in `ingestor/` maps the same way.
4. **The platform's checks**, in order, with the Go codes: `functionId` empty 400 `FUNCTION_ID_REQUIRED`;
   function not found 404; version not found 404; the version's describe lacks `platformToken: true` 403
   `PLATFORM_TOKEN_NOT_DECLARED`; the application has no linked service account 409
   `APPLICATION_SERVICE_ACCOUNT_REQUIRED`; the account or its principal deactivated 400
   `SERVICE_ACCOUNT_INACTIVE`.
5. **The mint** shares its body with the admin token mint and is the account's `client_credentials` grant
   exactly: same claims, scope (the account's flattened permissions) and lifetime. **No role ceiling**: the
   caller is a runner, whose authority is the runner role plus the declaration. Stamp the account
   last-used. Audit every mint, best-effort: entity `SERVICE_ACCOUNT`, the account id, operation
   `TOKEN_MINTED_FOR_FUNCTION`, principal = the runner's principal, detail `{"function": "<address>"}`. Never
   the token.
6. **Host cache**: keyed by function id, one entry (token, expiry = fetch time + `expiresIn` on the host
   clock). Served while more than `max(60 s, 10% of the lifetime)` remains, otherwise refetched. Concurrent
   callers of one function share a single fetch, and that fetch must not be cancelled by the first caller's
   deadline (the others still need the result; a caller whose own deadline passes gets `DEADLINE`).
   `refresh: true` replaces the entry. A failed fetch is not cached. An entry is dropped when its function
   leaves the desired document, and a fetch still running for it must not be stored afterwards. The token
   is never logged, at any level.

## Surprises (read before wiring the account lookup)

- **`applications.service_account_id` holds the SERVICE principal's id**, not the service account's id: it is
  a foreign key to `iam_principals(id)` (migration 028; the provisioning and attach operations store
  `saPrincipal.ID`). The Go handler loads the principal and reads its `serviceAccountId`. A principal that is
  missing or has no service account counts as "no linked service account" (409).
- It is **not** the link webhook signing uses (`iam_service_accounts.application_id`, read by the outbound
  credential resolver), and not the function's own `functions.service_account_id` (webhook verifier). The
  three can name different accounts; the token is always the first.
- The token lifetime is the grant's (`AccessTokenTTLSecs`), not a constant: the Go admin mint used to
  hard-code 3600 and now reads the same setting.
- The OpenAPI schema names are `FunctionTokenRequest` / `FunctionTokenResponse` in Go because the schema
  registry names a type after its Go type and `oauthapi` already owns `TokenResponse`. The wire shape is
  unchanged. Pick whatever does not collide on the Java side.

## Wire

OpenAPI: the control route `postFunctionRunnerToken` and schemas `FunctionTokenRequest`,
`FunctionTokenResponse` (in `api/openapi.lock.json`); classified `internal` for the SDKs. The generated
SPA types are refreshed (`pnpm -C frontend api:generate`); the application SDKs need `make sdk-generate`
only if the contract tests ask for it (not run by this change).

## Decisions to keep (and why)

- **One identity, the application's.** A per-function identity would multiply credentials that need roles;
  the application's account is the one that already carries the application's permissions.
- **No role ceiling on the mint.** Runners are trusted infrastructure; they already receive every
  function's secrets in the desired document.
- **Nothing switches over.** `bevco-integration.yard-trip-processor` keeps fetching its own Integral token
  and Integral's `api/bevco/plan-visit` keeps Passport until the owner switches them.
- Not built: narrowing the token's scope or audience per target.
