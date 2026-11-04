# Function platform token

Status: owner approved 2026-11-04. Build spec for the capability; nothing switches over yet.

A function gets a FlowCatalyst access token from its host and sends it as a bearer to another
first-party application that accepts FlowCatalyst tokens (for example Integral's `auth:fc-token`
routes). The function holds no client id or secret.

## 1. Rulings (owner, 2026-11-04)

- **Identity: the service account linked to the function's application** (`applications.service_account_id`).
  Always that one. There is no per-function service account and no per-function override: the
  function's optional webhook-verifier account (`functions.service_account_id`) is not used here.
- **The token is cached while it is valid**, by the host, and renewed shortly before it expires.
- **Nothing switches over now.** `bevco-integration.yard-trip-processor` keeps fetching its own
  Integral token (INTEGRAL_CLIENT_ID/SECRET) and Integral's `api/bevco/plan-visit` keeps Passport until
  the owner makes the switch. This build adds the capability only.

## 2. Declaration

A function declares it in its describe document:

```json
{ "abi": 1, "platformToken": true, ... }
```

- Go `abi.Describe` gains `PlatformToken bool \`json:"platformToken,omitempty"\``. Any other JSON
  type than a boolean is refused at publish like other describe errors.
- The JVM describe (`META-INF/flowcatalyst/function.json`) carries the same field.
- The console's function drawer lists it under **What It Declares** ("Platform token: calls other
  applications as <application's service account>"); the SPA and the /ui page both, if both list
  declarations.
- Undeclared use is refused by the host (`NOT_DECLARED`) and again by the platform.

## 3. Wire: platform control endpoint

`POST /control/functions/token` (constant `control.PathToken` in `internal/functions/control`).
Same caller rules as the other control routes: bearer token, `auth.RequireAnchor`,
`auth.CanControlFunctionRunner`.

Request (`control.TokenRequest`):

```json
{ "functionId": "fnc_...", "version": 3 }
```

Response 200 (`control.TokenResponse`):

```json
{ "accessToken": "eyJ...", "tokenType": "Bearer", "expiresIn": 3600 }
```

Checks, in order (each a `usecase` error with this code):

| Check | Status | Code |
|---|---|---|
| functionId empty | 400 | `FUNCTION_ID_REQUIRED` |
| function not found | 404 | `FUNCTION_NOT_FOUND` (or the existing not-found helper) |
| version not found for that function | 404 | as dispatch does |
| that version's describe lacks `platformToken: true` | 403 | `PLATFORM_TOKEN_NOT_DECLARED` |
| the application has no linked service account | 409 | `APPLICATION_SERVICE_ACCOUNT_REQUIRED` |
| the account or its principal is deactivated | 400 | `SERVICE_ACCOUNT_INACTIVE` (existing code) |

The mint:

- A new operation in `internal/platform/serviceaccount/operations` (for example
  `MintForFunction(ctx, MintDeps, serviceAccountID, functionAddress)`), sharing the body of
  `MintAdminToken` (find account, find principal, flatten roles, `GenerateAccessTokenWithScope`,
  `TouchLastUsed`) through a private helper. It does **not** run `requireMintWithinCaller`: the caller is
  a runner, whose authority is the runner role plus the function's declaration. Runners are trusted
  infrastructure already (they receive every function's secrets in the desired document).
- The token is exactly what the account's `client_credentials` grant would give: same claims, same
  scope (the account's flattened permissions), same TTL.
- Audit every mint (best effort, like the admin mint): entity `SERVICE_ACCOUNT`, the account id,
  operation `TOKEN_MINTED_FOR_FUNCTION`, principal = the runner's principal, and the function address
  wherever the audit row has room for detail. Never the token.
- Never log the token, at any level, anywhere (platform, runner, JVM host, fcdev).
- Registered in `internal/server/wire_routes.go` beside the dispatch route; `api/openapi.lock.json`
  regenerated the way the repo does it.

## 4. ABI op

`OpPlatformToken Op = 15`, `String()` = `"platform.token"`. Additive like `OpDispatch`: a runner that
predates it answers `UNKNOWN_OP`.

Call meta (JSON, may be empty or `{}`):

```json
{ "refresh": false }
```

`refresh: true` skips the cache and fetches a new token (for a function whose target answered 401
because the account's roles changed). Body: none.

Result meta:

```json
{ "accessToken": "eyJ...", "expiresAt": 1793736000000 }
```

`expiresAt` is unix milliseconds on the host's clock. Result body: none.

Errors to the guest (`abi.Error` codes, existing ones only):

| Situation | Code |
|---|---|
| the version does not declare `platformToken` | `NOT_DECLARED` |
| platform answered any other 4xx: 400 / 403 / 404 / 409 (not declared there, no linked account, inactive, function or version not found) | `NOT_ALLOWED`, message carrying the platform code |
| platform unreachable, 5xx, 429, 408, or a 401 that outlived the retry with a fresh runner token | `UNAVAILABLE` (retryable) |
| the invocation's deadline passed | `DEADLINE` |

## 5. Host cache (Go runner, JVM host)

- Keyed by function id. One entry: token, expiry (host clock: fetch time + `expiresIn`).
- Served from cache while more than `max(60 s, 10% of the lifetime)` remains; otherwise refetched.
- Concurrent callers for the same function share one fetch (singleflight).
- `refresh: true` replaces the entry.
- An entry is dropped when its function leaves the desired document.
- A failed fetch is not cached; the guest gets the error.
- The runner's own credential for the control call is the runner's existing control client (same 401 →
  fresh runner token retry the other control calls have).

## 6. Where it is built

**Go repo (platform, runner, fcdev):**

- `internal/functions/abi`: op 15, meta types `PlatformTokenCall{Refresh bool}`,
  `PlatformTokenResult{AccessToken string; ExpiresAt int64}`, describe field and validation, tests.
- `internal/functions/control`: `PathToken`, `TokenRequest`, `TokenResponse`, client method with the
  same error mapping the dispatch client method has; client tests.
- `internal/functions/runner`: `capabilities.Call` case, the cache (section 5), a `TokenSource`
  interface beside `Emitter`/`Dispatcher`, tests including: not declared, cache hit, renewal near expiry,
  singleflight (N concurrent calls → 1 fetch), refresh, failure not cached, entry dropped when the
  function leaves desired state, token never in logs.
- `internal/platform/function/control`: handler `token.go` + `token_test.go` covering every row of the
  section 3 table, the audit row, and the token being the application's account's (not the function's
  webhook account when that differs).
- `internal/platform/serviceaccount/operations`: `MintForFunction` + test.
- `cmd/fcdev`: wherever `fn run` / `fn dev` serve op 11 (`fn_run.go`, `fn_dev_cp.go`), serve op 15 the
  same way (against the local platform when there is one; otherwise the same "not available" answer
  op 11 gives offline).
- Console: the declaration in the function drawer (SPA `FunctionDetailDrawer.vue` / `functionHelpers.ts`
  and the /ui page if it lists declarations).
- Docs: `docs/function-runner-plan.md` (op table and control routes), `docs/published/60-functions.md`,
  `docs/wire-contract.md` if it lists control routes, and a Java-port hand-off note
  `docs/java-handoff-2026-11-04-function-platform-token.md` in the shape of
  `docs/java-handoff-2026-10-15-function-dispatch.md`.

**Function SDKs (`clients/fn-go`, `clients/fn-rust`, `clients/fn-ts`):** declare + call, each in its own
idiom, following how that SDK does `dispatches`/`dispatch`:

- declare at registration (module scope), which sets `platformToken: true` in the describe;
- call: returns the access token string; an option to force a refresh; errors surface as the SDK's
  existing host-error type with the code;
- an older runner's `UNKNOWN_OP` surfaces as a clear "this runner does not support platform tokens"
  error, not a crash;
- README section, example in the hello example, unit tests with the SDK's existing host fake.

**JVM (`ingestor/function-api`, `ingestor/ingestor-runtime`):**

- `FunctionContext.platformToken()` returning the access token `String`, and
  `platformToken(boolean refresh)`; throws a new `PlatformTokenException` (code + retryable flag,
  mirroring `DispatchException`).
- `FakeFunctionContext` lets a test set the token (or a failure) and counts calls.
- Runtime: `HttpControlPlane` gains the token call (`ControlPlane` interface method); a cache per
  section 5 in the functions host; refuses undeclared use before calling the platform; the describe
  reader reads `platformToken`.
- Tests: cache, renewal, concurrency (one fetch), refresh, failure not cached, undeclared refusal, the
  token never logged.
- `ingestor/docs` function docs updated.

## 7. Not in this build

- Switching `bevco-integration.yard-trip-processor` to the token, and moving Integral's
  `api/bevco/plan-visit` to `auth:fc-token` (owner switches later).
- Narrowing the token's scope or audience per target.
- SDK releases (cut by the owner via make).
