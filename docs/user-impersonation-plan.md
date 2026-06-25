# User Impersonation & Acting-User Attribution — Implementation Plan

Status: **PLAN / not started** · Audit fidelity: **per-action (subject + operator)** · `created_by` = operator · Investigation 2026-06-24

Two related capabilities sharing one identity model:

1. **UI impersonation** — a platform admin (anchor) acts *as* another user, with that
   user's full privileges, from the platform UI. Browser/cookie only.
2. **SDK / API on-behalf-of** — a service-account caller attributes an action to an
   end-user **for audit only**, keeping its own privileges. No privilege grant.

Both record, on every audited row, **who it was done as (subject)** and **who actually
did it (operator)**; ownership columns (`created_by`, `granted_by`, …) and event
`principalId` record the **operator**. Nothing here is implemented yet.

---

## 1. Why this is feasible

The session layer already does the hard part of (1). A session cookie (`fc_session`)
carries **only** `sub` + `email`; the auth middleware resolves *all* authorization data
(scope, clients, applications, roles, permissions) **fresh from the DB on every request,
keyed off `sub`** (`provider.go:222-244` mint; `middleware.go:154-168` resolve). So
**"act as user Y" = "issue a session cookie whose `sub` is Y."** No server-side session
store to mutate, no per-request claim surgery.

The attribution plumbing is equally tractable: the request `ctx` (carrying the
`AuthContext`) threads unchanged into both the operation layer (where `created_by` is
written) and `WriteAudit(ctx, …)` (verified: `principal/api/api.go:306-307` →
`operations.*` → `usecasepgx/commit.go`/`run.go:83-100` → `sink.WriteAudit`).

### Confirmed facts

| Fact | Location |
| --- | --- |
| Session = self-contained RS256 JWT, cookie `fc_session`, 24h TTL | `auth/login/endpoint.go:470-489`, `middleware.go:106` |
| Cookie carries only `{sub, email, iat}`; authz re-resolved from `sub` per request | `provider.go:230-243`, `middleware.go:154-168` |
| In-request principal = `auth.AuthContext` (single principal today) | `shared/auth/auth.go:129-152` |
| Admin gate = `RequireAnchor` / `IsAdmin` (anchor scope OR `platform:*:*:*`) | `shared/auth/auth.go:281-324` |
| User-admin surface (reset-pw, reset-2fa, …) — home for an impersonate action | `principal/api/api.go:76-102` |
| `created_by`/`granted_by` written from `ec.PrincipalID` | e.g. `operations/set_client_association.go:124` |
| `event.PrincipalID()` = `ec.PrincipalID`, documented as "the actor" | `operations/events.go:30,45`, `usecase/domain_event.go:60-73` |
| Audit row written by sink from `event.PrincipalID()` → `aud_logs.principal_id` | `platformsink/sink.go:99-137` |
| **129** `NewExecutionContext(ac.PrincipalID)` sites across **25** files (the sweep) | repo-wide; no `WithCorrelation`/`FromParentEvent` in non-test code |
| SDK audit-batch ingest exists (`POST /api/audit-logs/batch`); `AuditBatchItem.principalId` | `shared/sdk/audit_batch.go:30-44`, `audit/audit.go:55-72` |
| TS SDK audit resource read-only (`list/get/recent/forEntity/forPrincipal`) | `clients/typescript-sdk/src/resources/audit-logs.ts` |
| No impersonation / on-behalf-of exists anywhere | repo-wide search |
| SPA tracks user via Pinia `auth` store + `/auth/me`; `isPlatformAdmin` present | `frontend/src/stores/auth.ts`, `frontend/src/api/auth.ts:80` |

---

## 2. Identity & audit model (the core design)

Three identities can be in play on one request:

| Identity | Meaning | Normal | UI impersonation | SDK on-behalf-of |
| --- | --- | --- | --- | --- |
| **Authz identity** | whose permissions apply | caller | **target** (cookie `sub`) | **service account** |
| **Operator** | who actually did it | caller | admin (`act` claim) | service account |
| **Subject** | who it's recorded as done *as* | caller | target | named end-user |

Rules:
- **UI impersonation:** authz identity = subject = target; operator = admin. (The admin assumes the target's privileges — the point of impersonation.)
- **SDK on-behalf-of:** authz identity = operator = service account (privileges unchanged); subject = the named end-user. **The header grants nothing** — attribution only.

### Where each lands

| Sink column / value | = which identity |
| --- | --- |
| `aud_logs.principal_id` (exists) | **subject** |
| `aud_logs.actor_id` (new, migration 037) | **operator**, NULL when operator == subject |
| `created_by` / `granted_by` / ownership cols | **operator** |
| `msg_events.context_data.principalId` / `event.PrincipalID()` | **operator** |

### AuthContext representation
`shared/auth/auth.go` `AuthContext` (`:129-152`) keeps `PrincipalID` = the **authz
identity** (handlers keep using it for authorization, unchanged) and gains two optional
fields plus two helpers:

```go
type AuthContext struct {
    // ... existing fields (PrincipalID is the authz identity) ...
    ActorID    string // operator, when it differs from PrincipalID (UI impersonation: the admin)
    OnBehalfOf string // audit subject override (SDK on-behalf-of: the named end-user)
}

func (a *AuthContext) OperatorID() string { // "who did it"
    if a == nil { return "" }
    if a.ActorID != "" { return a.ActorID }
    return a.PrincipalID
}
func (a *AuthContext) SubjectID() string { // "done as"
    if a == nil { return "" }
    if a.OnBehalfOf != "" { return a.OnBehalfOf }
    return a.PrincipalID
}
func (a *AuthContext) IsImpersonating() bool { return a != nil && a.ActorID != "" }
```

### Derivations (verify against the three columns)
- `ExecutionContext.PrincipalID = OperatorID()` → `created_by` & `event.PrincipalID()` = operator.
- `aud_logs.principal_id = SubjectID()`.
- `aud_logs.actor_id = OperatorID()` when `OperatorID() != SubjectID()`, else NULL.

| Scenario | Operator (`created_by`, `actor_id`) | Subject (`principal_id`) | `actor_id` NULL? |
| --- | --- | --- | --- |
| Normal | caller | caller | yes |
| UI impersonation | admin | target | no |
| SDK on-behalf-of | service account | end-user | no |

For normal requests `OperatorID() == PrincipalID == ac.PrincipalID`, so behaviour is
byte-for-byte unchanged — only impersonation/on-behalf-of rows differ.

---

## 3. Goals / non-goals

**Goals**
- An anchor can impersonate a chosen user from the user-detail page; the app then behaves exactly as that user.
- A persistent banner makes the state obvious and offers one-click "return to my account".
- SDK/API callers can attribute actions to an acting end-user **without** gaining that user's privileges.
- Every audited action records both subject and operator; ownership columns record the operator.
- Stop (UI) is stateless and clean (no orphaned admin session).

**Non-goals**
- **No privilege-granting impersonation over bearer / `/oauth/token`.** Bearer callers may *attribute* (on-behalf-of) but never *assume* permissions. Full identity-swap stays cookie-only.
- No UI impersonation of SERVICE principals; no cross-anchor impersonation.
- No automatic name resolution for external on-behalf-of users (opaque id; `principal_name` null).

---

## 4. Security guardrails

**UI impersonation (start handler):**
- Anchor-only, behind `platform:iam:user:impersonate` (interim: `RequireAnchor`).
- Refuse to target another anchor / super-admin (no escalation; lower-privilege target = de-escalation).
- Refuse inactive targets, SERVICE principals, self.
- No nesting (reject if caller's session is already an impersonation session).
- Short TTL (30–60 min). Re-resolution from `sub` means a target deactivated mid-session is rejected next request.
- Block sensitive self-service while impersonating: password change, 2FA enroll/disable, account deletion (`ac.IsImpersonating()` guard in those handlers).

**SDK on-behalf-of:**
- Attribution-only, **never authorization** — `OnBehalfOf` must not touch `Scope/Clients/Applications/Permissions`. Covered by an explicit "no privilege gain" test.
- Optionally gate which service accounts may set the header (a flag/grant).

---

# Phases

Build order: Phase 1 establishes the shared identity model both features depend on;
Phases 2–4 deliver UI impersonation; Phase 5 delivers SDK on-behalf-of; Phase 6 is
audit-viewer polish. Each phase is independently shippable and green.

---

## Phase 1 — Shared identity & audit plumbing

**Objective:** introduce subject/operator/authz everywhere attribution is written, with
zero behaviour change for normal requests.

**1.1 Migration** — `internal/migrate/sql/037_audit_actor.sql` (goose):
```sql
-- +goose Up
ALTER TABLE aud_logs ADD COLUMN IF NOT EXISTS actor_id VARCHAR(100);
CREATE INDEX IF NOT EXISTS idx_aud_logs_actor ON aud_logs (actor_id);
-- +goose Down
DROP INDEX IF EXISTS idx_aud_logs_actor;
ALTER TABLE aud_logs DROP COLUMN IF EXISTS actor_id;
```
Follow the goose `-- +goose Up`/`Down` + column-order conventions (precedent:
`009_p0_alignment.sql`). `aud_logs` base schema: `006_audit_tables.sql:8-21`.

**1.2 AuthContext fields + helpers** — `shared/auth/auth.go`:
- Add `ActorID string`, `OnBehalfOf string` to `AuthContext` (`:129-152`).
- Add `OperatorID()`, `SubjectID()`, `IsImpersonating()` (§2).
- `auth.go` already imports `pkg/fcsdk/usecase` (used for `usecase.Authorization`), so the next item adds no new dependency.

**1.3 Central EC helper + sweep** — the `created_by = operator` lever:
- Add `func ExecContext(ctx context.Context) usecase.ExecutionContext { return usecase.NewExecutionContext(FromContext(ctx).OperatorID()) }` in `shared/auth/auth.go` (nil-safe via `OperatorID()`). Keeps the existing fresh-correlation behaviour of `NewExecutionContext` (no inbound-correlation change — out of scope).
- **Sweep** the **129** `usecase.NewExecutionContext(ac.PrincipalID)` call sites (25 files) → `auth.ExecContext(ctx)`. Mechanical; `ctx` is in scope at every site. Leave the 5 non-`ac.PrincipalID` constructions (e.g. `bridge/login_endpoint.go` `NewExecutionContext("")`) untouched — not impersonation paths.
- Net effect: `ec.PrincipalID = operator`. For normal requests `operator == ac.PrincipalID`, so `created_by`, `granted_by`, and `event.PrincipalID()` are unchanged; they diverge only under impersonation/on-behalf-of.

**1.4 Sink: write subject + operator** — `shared/platformsink/sink.go` `WriteAudit` (`:99-137`):
```go
ac := auth.FromContext(ctx)
operator := event.PrincipalID()        // == ec.PrincipalID == OperatorID()
subject := operator
if ac != nil { subject = ac.SubjectID() }
var actorCol any = nil
if subject != operator { actorCol = operator }
// add actor_id to the INSERT column list + VALUES ($N)
```
Set `principal_id = subject`, `actor_id = actorCol`. INSERT is hand-written
`tx.Inner().Exec` — no sqlc regen for the write. `platformsink` importing
`shared/auth` is in-tree (no `pkg/fcsdk` boundary crossing). Mirror the same two values
into `WriteEvent`'s `context_data` if you want the subject there too (optional;
default keep `principalId = operator`).

**1.5 Verification** — integration test asserting `auth.FromContext(ctx)` is non-nil
inside `WriteAudit` (i.e. the request `ctx` reaches the sink). If any commit path
substitutes `context.Background()`, fall back to **Plan B**: thread subject via
`ExecutionContext` + `EventMetadata` (`domain_event.go:43-55`) + a small optional
`Impersonated` interface `WriteAudit` type-asserts. (Expected: ctx does reach the sink — call sites pass it straight through.)

**Tests**
- Normal request: `aud_logs.principal_id = caller`, `actor_id IS NULL`, `created_by = caller` — unchanged from today (regression guard).
- Unit: `OperatorID()`/`SubjectID()` truth table (all three scenarios).
- Migration up/down clean on a fresh embedded PG.

**Acceptance:** all existing tests green; new audit columns present; normal-path audit/ownership byte-identical to pre-change.

---

## Phase 2 — UI impersonation token layer

**Objective:** mint and read an impersonation session that carries the admin as actor.

**2.1 sessiontoken** — `auth/sessiontoken/sessiontoken.go`:
- Add `Actor string` to `Claims` (`:45-60`).
- `Mint` (`:66-110`): when `c.Actor != ""`, set `mc["act"] = map[string]any{"sub": c.Actor}` (RFC 8693 nested shape).
- `Validate` (`:139-189`): parse `act.sub` → `out.Actor` (tolerate `act` as `map[string]any`).

**2.2 provider** — `auth/provider/provider.go`:
- `MintImpersonationToken(ctx, targetID, adminID string, ttl time.Duration) (string, error)`: `BuildClaims` for **targetID** (so the cookie identity is the target), then `sessiontoken.Mint(sessiontoken.Claims{Subject: c.Subject, Email: c.Email, Actor: adminID}, …, ttl)`.
- Add `ImpersonationTTL` const (e.g. 45m) — used by the start endpoint.
- `ValidateSessionToken` already returns `*sessiontoken.Claims` (now incl. `Actor`) — no signature change.

**2.3 middleware** — `shared/middleware/middleware.go` `introspect` (`:139-193`):
- Cookie branch (`:154-168`): after `ResolveClaims(c.Subject)`, set `ActorID: c.Actor` on the returned `AuthContext`.
- Bearer branch (`:171-192`): never set `ActorID` from a token (full impersonation never rides bearer).

**Tests**
- `Mint`→`Validate` round-trips `Actor` (and omits `act` when empty).
- Cookie `introspect` sets `ActorID`; bearer `introspect` never does.
- An impersonation token resolves the **target's** scope/permissions (authz = target) while exposing `ActorID = admin`.

**Acceptance:** nothing calls `MintImpersonationToken` yet (no behaviour change); token layer unit-green.

---

## Phase 3 — UI impersonation endpoints, guardrails & `/auth/me`

**Objective:** the start/stop flow with full guardrails and SPA-visible state.

**3.1 Permission seed** — add `platform:iam:user:impersonate` to `seed/permissions.go`; grant to the anchor/super-admin role. (Interim gate `RequireAnchor` if deferring the seed.)

**3.2 Start** — `POST /api/principals/{id}/impersonate` in `principal/api/api.go`:
- Authorize: caller holds `platform:iam:user:impersonate` (or `RequireAnchor`).
- Load target via `Principals.FindByID`. Guardrails (§4): reject if target is anchor/super-admin, inactive, SERVICE, self, or if `ac.IsImpersonating()` (no nesting).
- `token := MintImpersonationToken(ctx, targetID, ac.PrincipalID, ImpersonationTTL)`.
- `Set-Cookie: fc_session=<token>` via the huma Set-Cookie output-field pattern (memory `huma-set-cookie-pattern`): output struct field `SetCookie string \`header:"Set-Cookie"\`` carrying a marshalled `http.Cookie` (HttpOnly, Secure=cfg, SameSite=Lax, MaxAge=ImpersonationTTL).
- Emit a domain/audit event `ImpersonationStarted` (subject = target, operator = admin → audit row records both automatically via Phase 1).
- Response body: target's `loginResponse` shape + `impersonating: true` + `impersonator: {id,name,email}`.

**3.3 Stop** — `POST /auth/impersonate/stop` in `auth/login` package:
- Require `ac.IsImpersonating()`; else 400 `NOT_IMPERSONATING`.
- Re-verify the admin (`ac.ActorID`) is still an active anchor via `Principals.FindByID`; if not → clear cookie (hard logout) instead.
- Mint a fresh **normal** session for the admin (`MintSessionToken(ctx, adminID, SessionTTL)`); `Set-Cookie` it.
- Emit `ImpersonationStopped`. Return the admin's `loginResponse`.

**3.4 `/auth/me` + login response** — `auth/login/endpoint.go` (`handleMe` `:534-571`, `loginResponse`):
- Add `Impersonating bool` and `Impersonator *struct{ ID, Name, Email string }`.
- Populate from `ac.ActorID` (resolve admin name/email via `Principals.FindByID`).

**3.5 Self-service blocks** — add `if ac.IsImpersonating() { return Forbidden(...) }` to: change-password, 2FA enroll/disable, account self-delete handlers.

**Tests**
- Guardrails: start rejects anchor/inactive/service/self/nested; non-anchor forbidden.
- Start sets an impersonation cookie whose `introspect` yields authz = target, `ActorID = admin`.
- An audited mutation while impersonating → `principal_id = target`, `actor_id = admin`, `created_by = admin`.
- Stop restores a normal admin session; 400 on non-impersonation session; hard-logout if admin lost anchor.
- `/auth/me` reports impersonation state.
- Self-service endpoints rejected while impersonating.

**Acceptance:** end-to-end UI impersonation works server-side (drive via the test headers / curl); audit + ownership attribution correct.

---

## Phase 4 — Frontend (Vue SPA)

**Objective:** start from the UI, an unmistakable banner, one-click return.

**4.1 Store** — `frontend/src/stores/auth.ts`: add `impersonating: boolean` and `impersonator: {id,name,email} | null`; set in `setUser`/from `/auth/me`; clear in `clearAuth`.

**4.2 API** — `frontend/src/api/auth.ts`: parse the new `/auth/me`/login fields; add `startImpersonation(userId)` (`POST /api/principals/{id}/impersonate`) and `stopImpersonation()` (`POST /auth/impersonate/stop`).

**4.3 Banner** — a persistent top bar ("You are viewing FlowCatalyst as **{name}** — Return to your account") wired to `stopImpersonation()` → `checkSession()` → redirect home. Mount in the app layout so it shows on every page while `impersonating`. Optional: distinct chrome (coloured border) to prevent "who am I" confusion.

**4.4 Button** — "Impersonate" on `frontend/src/pages/users/UserDetailPage.vue`, visible only to anchors, hidden for anchor/inactive/service targets. On success → `checkSession()` + redirect to app home.

**Tests:** component/E2E (optional) — start from user detail, banner shows, return restores admin.

**Acceptance:** full SPA loop works against a running instance (see `/run`, `make frontend && make go-build` — embedded SPA, memory `embedded-frontend-staleness`).

---

## Phase 5 — SDK / API on-behalf-of (attribution only)

**Objective:** let bearer callers attribute to an end-user without privilege change.

**5.1 Live header** — `shared/middleware/middleware.go`, bearer branch only:
- Read `FC-On-Behalf-Of: <userId>`; set `AuthContext.OnBehalfOf`. **Do not touch** `Scope/Clients/Applications/Permissions` — authorization stays the service account's.
- Ignored on the cookie transport (UI uses real impersonation).
- Phase 1's sink derivation then records `principal_id = end-user`, `actor_id = SA`, and ownership cols = SA (operator).

**5.2 Batch ingest** — `shared/sdk/audit_batch.go` `AuditBatchItem` (`:30-39`): add optional `ActorID *string \`json:"actorId,omitempty"\`` (operator). `audit.Insert` + the INSERT (`audit/audit.go:55-72`) gain `actor_id`; `audit.Log` (`:25-36`) gains `ActorID *string`. `principalId` stays the subject. Lets apps running their own impersonation forward both.

**5.3 SDK surfaces** — TS (`clients/typescript-sdk`), Laravel, Go:
- A per-call option and/or client-level default to set `FC-On-Behalf-Of` (e.g. `client.withActingUser(userId)` / request option).
- Optionally a batch-audit ingest method carrying `actorId` (TS audit resource is read-only today — new write surface).
- Regenerate from spec where applicable; coordinate an SDK release (memory `release-tooling-port`).

**5.4 Spec** — add `FC-On-Behalf-Of` (documented header) + the batch-ingest `actorId` field to the OpenAPI spec; `make dump-spec` drift guard requires registration; `make frontend-types-verify` if SPA types regen.

**Tests**
- **No privilege gain (critical):** a service account with limited perms + `FC-On-Behalf-Of` naming an admin gains **no** admin permission.
- Live header → `principal_id = end-user`, `actor_id = SA`.
- Batch ingest with `actorId` persists both; without it behaves as today.
- External id stored opaquely; `principal_name` null (read JOIN, `audit.go:137`).

**Acceptance:** API on-behalf-of attribution lands in audit; SDKs expose it; security test green.

---

## Phase 6 — Audit viewer surfacing (polish)

**Objective:** make subject/operator visible and filterable.

- `internal/sqlc/queries/audit.sql` + `audit.Log`/`FindWithCursor`/`FindWithFilters`/`FindByID` (`audit/audit.go`): select `a.actor_id`, optionally a second `LEFT JOIN iam_principals` to resolve actor name; add `?actorId=` filter (mirror the `principal_id` filter at `:122`).
- `frontend/src/pages/platform/AuditLogListPage.vue`: render "performed by **{actor}** as **{subject}**" when `actor_id` present; add an actor filter.

**Tests:** read endpoint returns/filters `actor_id`; UI shows the pair.

**Acceptance:** investigators can answer "who really did this, and as whom" from the UI.

---

## 5. Open questions

- Permission: dedicated `platform:iam:user:impersonate` (recommended) vs plain `RequireAnchor`?
- Impersonation TTL: 30 vs 60 min?
- Restrict which service accounts may use `FC-On-Behalf-Of`, or allow any authenticated SA?
- Notify the impersonated/named user (email/webhook) on start? (Some compliance regimes require it.)

*(Resolved: `created_by` = operator — Phase 1.3 / §2.)*

---

## 6. Effort estimate

| Phase | Scope | Est. |
| --- | --- | --- |
| 1 | Shared identity & audit plumbing (migration, AuthContext, EC helper + 129-site sweep, sink, tests) | ~1 day |
| 2 | UI token layer (sessiontoken, provider, middleware) | ~0.5 day |
| 3 | UI endpoints + guardrails + `/auth/me` + self-service blocks | ~0.5–1 day |
| 4 | Frontend (store, banner, button) | ~0.5 day |
| 5 | SDK on-behalf-of (header, batch field, SDK options, release) | ~1 day |
| 6 | Audit viewer surfacing (optional) | ~0.5 day |
| | **Total** | **~4–4.5 days** |

The 129-site EC sweep is the main mechanical cost; it's centralising work the codebase
already favours (memory `crud-consolidation`). No privilege changes to the OAuth surface;
no event-struct changes; normal-path behaviour byte-identical.
