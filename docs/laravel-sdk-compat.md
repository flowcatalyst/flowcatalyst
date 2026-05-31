# Laravel SDK Compatibility — Go-side Remediation Punch-list

**Goal:** the existing FlowCatalyst Laravel SDK (`flowcatalyst-rust/clients/laravel-sdk`)
must keep working when its traffic is pointed at the **Go** server, with **no SDK
release required**. Strategy chosen: **add Go-side compatibility aliases** (register
the Rust-named routes/fields alongside the Go ones). The SDK's public method
signatures and its wrapper stay untouched.

The SDK's `Generated/Client.php` was generated from the Rust OpenAPI spec
(`clients/laravel-sdk/openapi/openapi.json`), so that spec **is** the wire contract
the SDK speaks. Each item below is a place where the Go server does not currently
serve what that contract expects.

> **Note on `api/openapi.lock.json`:** it is incomplete (e.g. it omits the wired
> `sdksync` routes), so it was **not** used to judge parity. All "Go current"
> findings below come from actual route registrations / handler source.

Legend — Effort: **T** trivial route/field alias · **M** handler logic · Severity:
**P0** silent corruption · **P1** hard failure (404/4xx) · **P2** low-risk/admin.

---

## ✅ Implementation status (2026-05-31)

All wrapper-reachable items below are **DONE** (Go-side, build + tests green):

- **P0 `completeInstance`** — accepts `{status:SUCCESS/FAILURE, result}` and the
  SPA shape; unit-tested (`scheduledjob/api`: `resolveInstanceCompletion`).
- **P1 #1 `GET /api/clients/search?q=`** — added alongside POST.
- **P1 #2 `/api/dispatch-jobs/by-event/{eventId}`**, **#3 `/api/dispatch-jobs/raw`**,
  **#4 `/api/events/raw`** — aliases to the canonical handlers.
- **P1 #7 `GET /api/oauth-clients/by-client-id/{clientId}`** + **#8 `regenerate-secret`** (alias of rotate-secret).
- **P1 #9 `POST /api/principals/users`** — full create-user port with email-domain
  scope derivation (anchor-domain + mapping → ANCHOR/PARTNER/CLIENT, partner-merge);
  unit-tested (`principal/api`: `deriveUserScope`). **Verified the Inhance flow**:
  password reset already worked; user-create now works.
- **P1 #10 `/api/roles/{roleName}`** (GET/PUT/DELETE) — `{id}` handlers now resolve
  id-or-name (`resolveRole`). **#11 `POST /api/roles/{roleName}/permissions`** (body).
- **Laravel `WebhookValidator`** + the Rust/Go SDK validators — parse ISO8601-ms.
- **Lock fix** — `tools/dump-spec` now includes `sdksync` + `loginattempt`, with a
  drift-guard test; `api/openapi.lock.json` regenerated (now the source of truth again).

**Deferred (NOT reachable through the SDK's public wrapper, so they don't affect
existing users):**

- `POST /api/events` and `POST /api/dispatch-jobs` (single-item create). The SDK has
  no `events()`/`dispatchJobs()` resource — these live only in the raw `Generated/`
  layer; real ingestion goes through the outbox → `/api/events/batch` +
  `/api/dispatch-jobs/batch` (which Go already serves). To add later: wrap the batch
  ingest as a batch-of-1.
- **P2 monitoring** `/api/monitoring/*` (Go serves `/monitoring/*` in the router
  subsystem). No SDK wrapper exposes it; mounting under `/api` is a router-structure
  change. Left as-is.

> The "Note on `api/openapi.lock.json` is incomplete" caveat below is now **resolved**
> (lock fixed + drift-guarded).

---

## P0 — `completeInstance` silently corrupts state

**SDK call** (`Resources/ScheduledJobs.php:220`, `ScheduledJobRunner`):
`POST /api/scheduled-jobs/instances/{instanceId}/complete`
body `{ "status": "SUCCESS"|"FAILURE", "result"?: <mixed> }`
(Rust `InstanceCompleteRequest`: `required:[status]`, `status` = `CompletionStatusDto`, `result` nullable.)

**Go current** (`internal/platform/scheduledjob/api/api.go:594`, `dto.go:241`):
`CompleteInstanceRequest{ Status (instance-lifecycle, default COMPLETED), CompletionStatus (SUCCESS/FAILURE), CompletionResult }`.
With the SDK body, Go does `ParseInstanceStatus("SUCCESS")` → `default → QUEUED`
(`instance.go:41`), `completionStatus` stays empty (outcome lost), and `result` is
ignored (Go reads `completionResult`). **Returns success, marks the instance QUEUED,
drops the outcome and the result.**

**Go-side fix (M):**
- `dto.go`: add `Result json.RawMessage `json:"result,omitempty"`` as an alias for `CompletionResult`.
- `api.go` complete handler: if `Status ∈ {SUCCESS, FAILURE}` (a `CompletionStatusDto`,
  not an `InstanceStatus`), set `compStatus = Status` and `status = InstanceStatusCompleted`;
  if `CompletionResult` empty and `Result` present, use `Result`. Keep the existing
  `{status:"COMPLETED", completionStatus, completionResult}` path working (SPA/internal callers).

**Verify:** POST `{status:"SUCCESS", result:{...}}` → instance COMPLETED, `completionStatus=SUCCESS`, result persisted. Existing Go-shaped body still works.

---

## P1 — path / method aliases (these currently 404 / 405)

| # | SDK call (Rust contract) | Go current | Go-side fix | Files | Eff |
|---|---|---|---|---|---|
| 1 | `GET /api/clients/search?q=` | `POST /api/clients/search` (body) | register `GET /api/clients/search` reading `?q=`, delegate to existing search | `client/api/api.go:53` | M |
| 2 | `GET /api/dispatch-jobs/by-event/{eventId}` | `…/event/{eventId}` | alias route `by-event/{eventId}` → same handler | `dispatchjob/api/api.go:59` | T |
| 3 | `GET /api/dispatch-jobs/raw` | `…/list-raw` (+ `{id}/raw`) | alias `dispatch-jobs/raw` → `listDispatchJobsRaw` | `dispatchjob/api/api.go:41` | T |
| 4 | `GET /api/events/raw` | `…/list-raw` | alias `events/raw` → `listEventsRaw` | `event/api/api.go:46` | T |
| 5 | `POST /api/events` (`CreateEventRequest`: req `eventType,source,data`; opt `causationId,clientId,contextData,correlationId,deduplicationId,messageGroup,subject`) | only `POST /api/events/batch` | register `POST /api/events` accepting one `CreateEventRequest`, delegate to batch with a 1-element list | `event/api/api.go:28`, `shared/sdk/*` | M |
| 6 | `POST /api/dispatch-jobs` (`CreateDispatchJobRequest`: req `code,targetUrl,payload,serviceAccountId`; +many opt) | only `…/batch` (`shared/sdk/dispatch_jobs_batch.go:67`) | register `POST /api/dispatch-jobs` single → wrap into batch-of-1 | `dispatchjob/api/api.go`, `shared/sdk/dispatch_jobs_batch.go` | M |
| 7 | `GET /api/oauth-clients/by-client-id/{clientId}` | missing (only `/{id}` by TSID) | add lookup-by-client-id endpoint (repo `FindByClientID`) | `auth/api/api.go:36`, oauth-client repo | M |
| 8 | `POST /api/oauth-clients/{id}/regenerate-secret` | `…/rotate-secret` | alias `regenerate-secret` → rotate-secret handler | `auth/api/api.go:94` | T |
| 9 | `POST /api/principals/users` (`CreateUserRequest`: req `email,name`; opt `clientId,enforcePasswordComplexity,password`) | `POST /api/principals` | alias `POST /api/principals/users` → create handler (confirm create body == `CreateUserRequest`) | `principal/api/api.go:57` | T/M |
| 10 | `GET/PUT/DELETE /api/roles/{roleName}` (by **name**) | `/api/roles/{id}` (by **TSID**, `FindByID`) | make the `{id}` handlers fall back to `FindByName` when the param isn't a TSID (Go already has `by-code/{code}`) | `role/api/api.go:50-75,200` | M |
| 11 | `POST /api/roles/{roleName}/permissions` (`GrantPermissionRequest{permission}` in **body**) | `POST /api/roles/{roleName}/permissions/{permission}` (in **path**) | add `POST …/permissions` reading `{permission}` from body, delegate to grant | `role/api/api.go:115` | T |

---

## Real-world flow: Inhance `integral-service-v2` user management (verified 2026-05-31)

The Inhance app uses the Laravel SDK for user management when FlowCatalyst OIDC is
enabled (`src/Admin/UseCases/{ResetUserPassword,UpdateUser,CreateUser}UseCase.php`
via `FlowCatalystClient->principals()`). Exact SDK calls and Go status:

| SDK call (Principals.php) | HTTP | Go status |
|---|---|---|
| `findByEmail($email)` | `GET /api/principals?email=…` then **client-side filter** | ✅ works — Go `list` ignores the `email` query param but returns the full unpaginated set (`FindAll`, api.go:232), so the SDK's own filter finds the user. (Scale caveat: Go returns all principals; fine functionally.) |
| `resetPassword($id,$pw,$enforce=true)` | `POST /api/principals/{id}/reset-password` body `{newPassword, enforcePasswordComplexity}` | ✅ works — Go `ResetPasswordRequest.newPassword` matches. ⚠️ Go silently ignores `enforcePasswordComplexity` (huma drops the unknown field). |
| `createUser(CreateUserRequest{email,name,password,clientId,enforcePasswordComplexity})` | `POST /api/principals/users` | ❌ **breaks** — see below |

**Result: the password reset/update flow WORKS against Go.** Only user *creation* breaks.

**`POST /api/principals/users` is NOT a trivial alias (supersedes P1 #9).** Two issues:
1. Path: Go has no `/api/principals/users`; it has `POST /api/principals`.
2. Body + scope: the SDK sends `{email,name,password,clientId,enforcePasswordComplexity}`
   with **no `scope`**, but Go's `CreatePrincipalRequest` (dto.go:15) **requires** `scope`
   (ANCHOR/PARTNER/CLIENT) and has no `enforcePasswordComplexity`. Rust's `create_user`
   (`fc-platform/src/principal/api.rs:477`) **derives** scope from the email domain
   (anchor-domain check + email-domain-mapping `scope_type`), not from the request.

   So closing this means porting Rust `create_user`'s scope-derivation into a new Go
   `POST /api/principals/users` handler (resolve anchor-domain + email-domain-mapping →
   scope + client association; accept `enforcePasswordComplexity`), delegating to the
   existing principal create operation. This is a feature port, bigger than a route alias —
   call it out separately on the punch-list.

## P2 — low-risk (SDKs rarely call these)

| SDK call | Go current | Note |
|---|---|---|
| `GET /api/monitoring/{circuit-breakers,dashboard,in-flight-messages,pool-stats,standby-status}` | served at `/monitoring/*` (router subsystem, no `/api` prefix; possibly separate mount/port) | If any consumer uses these, either mount the monitoring router under `/api/monitoring` or add `/api/monitoring/*` aliases. `internal/router/api/handlers_dashboard.go`, `handlers_misc.go` |

---

## Pre-existing SDK bug — informational, NOT a switch regression

`Webhook/WebhookValidator.php:84` does `(int)$timestamp` expecting **Unix seconds**,
but both Rust (`fc-router/src/mediator/signing.rs:33`) **and** Go
(`internal/router/mediator.go:211`) send `X-FLOWCATALYST-TIMESTAMP` as
**ISO8601-millisecond** (`%Y-%m-%dT%H:%M:%S%.3fZ`). `(int)"2026-…"` → `2026` → always
"expired." The HMAC itself matches (it concatenates the received timestamp string
verbatim) — only the replay-window check fails. **Go ≡ Rust here, so switching changes
nothing.** Fix belongs in the SDK (parse ISO8601). Optional Go accommodation: also emit
a Unix-seconds header — but not required for parity and not recommended (would diverge
from Rust).

---

## Already compatible — no action

- 8 `/sync` self-registration endpoints (`sdksync`, wired `wire.go:408`) — paths,
  request field names, and `SyncResultResponse {applicationCode,created,updated,deleted,syncedCodes}` match.
- `POST /oauth/token` (client_credentials) — params + response `{access_token,token_type,expires_in,…}`, `Cache-Control: no-store`.
- `/.well-known/jwks.json`.
- Webhook HMAC **signing** — byte-identical to Rust.
- Scheduled-job CRUD + `POST …/instances/{id}/log` (`{message,level,metadata}`) — exact match.
- Batch ingest `/api/events/batch`, `/api/dispatch-jobs/batch` (the outbox processor path).
- TSID format; error envelope `{error,message}`.

---

## Suggested order

1. **P0 #completeInstance** (data-integrity — do first).
2. Trivial aliases: P1 #2,#3,#4,#8,#11 (pure route adds).
3. Handler-logic aliases: P1 #1,#5,#6,#7,#9,#10.
4. Decide on P2 monitoring + the SDK-side webhook fix.

After landing the aliases, regenerate `api/openapi.lock.json` (and fix `tools/dump-spec`
so it includes `sdksync` + the new aliases), then a one-shot contract check: boot Go,
point a Laravel SDK integration test (sync → fire scheduled job → complete → webhook) at it.
