# User Impersonation — Implementation Plan

Status: **PLAN / not started** · Audit fidelity: **per-action attribution** · Author aid: investigation 2026-06-24

Lets a platform administrator (anchor) act as another user for support/debugging,
with every action attributable back to the real admin. This document is the
build spec; nothing here is implemented yet.

---

## 1. Why this is straightforward here

The session layer already does the hard part. A session cookie (`fc_session`)
carries **only** `sub` + `email`; the auth middleware resolves *all*
authorization data (scope, clients, applications, roles, permissions) **fresh
from the DB on every request, keyed off `sub`**:

- Mint: `internal/platform/auth/provider/provider.go:222-244` (`MintSessionToken` — cookie holds just subject/email).
- Resolve per request: `internal/platform/shared/middleware/middleware.go:154-168` (`introspect` → `ResolveClaims(c.Subject)`).

So **"act as user Y" reduces to "issue a session cookie whose `sub` is Y."** The
whole app then behaves as Y with zero changes to data-access, scoping, or
authorization logic. There is no server-side session store to mutate and no
per-request claim surgery.

All the work below is about doing this **safely, auditably, and reversibly** —
not about making it function.

### Confirmed facts (from investigation)

| Fact | Location |
| --- | --- |
| Session = self-contained RS256 JWT, cookie `fc_session`, 24h TTL | `auth/login/endpoint.go:470-489`, `middleware.go:106` |
| Cookie carries only `{sub, email, iat}`; authz re-resolved from `sub` per request | `provider.go:230-243`, `middleware.go:154-168` |
| In-request principal = `auth.AuthContext` (single principal, no actor/subject split today) | `shared/auth/auth.go:129-152` |
| Admin gate = `RequireAnchor` / `IsAdmin` (anchor scope OR `platform:*:*:*`) | `shared/auth/auth.go:281-324` |
| User-admin surface (reset-pw, reset-2fa, etc.) — natural home for an impersonate action | `principal/api/api.go:76-102` |
| Audit row written by sink from `event.PrincipalID()` → `aud_logs.principal_id` | `platformsink/sink.go:99-137` |
| `event.PrincipalID()` originates from `ExecutionContext.PrincipalID` | `usecase/domain_event.go:60-73`, `execution_context.go:31-39` |
| Handlers pass the **request ctx** (carrying `AuthContext`) straight into the use case → sink | `principal/api/api.go:306-307`, `usecasepgx/commit.go`, `run.go:83-100` |
| No impersonation exists anywhere (backend, SPA, TS SDK) | repo-wide search |
| SPA tracks user via Pinia `auth` store + `/auth/me`; `isPlatformAdmin` already present | `frontend/src/stores/auth.ts`, `frontend/src/api/auth.ts:80` |

---

## 2. Goals / non-goals

**Goals**
- An anchor can start impersonating a chosen user from the user-detail page.
- While impersonating, the app behaves exactly as that user (their scope, clients, apps, permissions).
- A persistent banner makes the state obvious and offers one-click "return to my account".
- **Every audited action during impersonation records the driving admin** (`impersonator_id`), in addition to the start/stop bracket events.
- Stop is stateless and clean (no orphaned admin session).

**Non-goals (explicit)**
- **No impersonation over bearer / `/oauth/token`.** Impersonation is a browser/SPA-only support feature. The OAuth surface is untouched.
- No "log in as" from outside the platform UI; no impersonation of SERVICE principals.
- No cross-anchor impersonation (an anchor cannot impersonate another anchor / super-admin).

---

## 3. Design: actor + subject token (RFC 8693 `act` style)

Do **not** simply overwrite `sub` and discard the admin — that loses the audit
trail and the way back. Embed the admin as an **actor claim** beside the
impersonated subject:

```
fc_session (impersonation):
    sub  = targetUserID
    act  = { sub: adminPrincipalID }
    tier/email/... = target's (so the SPA renders as the target)
    exp  = short (30–60 min, vs the normal 24h)
```

- **Authz** resolves from `sub` → app behaves as the target. (Existing per-request resolution; nothing to change in handlers.)
- **`act`** carries the real admin → drives audit attribution and the banner.
- **Stop is stateless**: the stop endpoint reads `act.sub`, re-verifies that admin is still an active anchor, and mints them a fresh *normal* session. No second cookie, no DB stash, no fixation risk.

`created_by`-style columns and the audit `principal_id` stay = the impersonated
user (`sub`) — the action genuinely happened "as" them. The admin is recorded
*additionally* via the new `impersonator_id`. This keeps every existing
`NewExecutionContext(ac.PrincipalID)` call site correct and untouched.

---

## 4. Backend changes

### 4.1 Session token — carry the actor
**`internal/platform/auth/sessiontoken/sessiontoken.go`**
- Add `Actor string` to `Claims` (the impersonator's principal id).
- `Mint`: when `c.Actor != ""`, emit `act` claim as `{"sub": c.Actor}` (nested object, RFC 8693 shape).
- `Validate`: parse `act.sub` back into `Claims.Actor`.
- Keep it minimal — no actor email/roles in the token; the SPA fetches the impersonator's display fields from `/auth/me`.

### 4.2 Provider — mint/resolve impersonation sessions
**`internal/platform/auth/provider/provider.go`**
- `MintImpersonationToken(ctx, targetID, adminID string, ttl) (string, error)`: build the **target's** identity claims (so cookie carries the target's `sub`/`email`), set `Actor = adminID`, sign with the short impersonation TTL.
- Expose the actor from validation: `ValidateSessionToken` already returns `*sessiontoken.Claims`, which will now include `Actor`. No signature change needed.
- Add `ImpersonationTTL` (e.g. 45 min) to `provider.Config` or a package const.

### 4.3 AuthContext — surface the impersonator
**`internal/platform/shared/auth/auth.go`**
- Add field `ImpersonatorID string` to `AuthContext` (struct at `:129-152`).
- Add helper `func (a *AuthContext) IsImpersonating() bool { return a != nil && a.ImpersonatorID != "" }`.

**`internal/platform/shared/middleware/middleware.go`** (`introspect`, cookie branch `:154-168`)
- After `ResolveClaims(c.Subject)`, set `ImpersonatorID: c.Actor` on the returned `AuthContext`.
- Bearer branch (`:171-192`): leave `ImpersonatorID` empty — impersonation never rides bearer tokens.

### 4.4 Per-action audit attribution (the per-action requirement)
Chosen approach: **context-carried**, because the request `ctx` (which holds the
`AuthContext`) threads unchanged all the way into `WriteAudit(ctx, …)` (verified:
`principal/api/api.go:306-307` → `operations.*` → `usecasepgx/commit.go`/`run.go`
→ `sink.WriteAudit(ctx, …)`). This avoids touching the ~hundreds of inline
`NewExecutionContext(ac.PrincipalID)` call sites and every concrete event struct.

- **Migration `037_audit_impersonator.sql`** (goose): `ALTER TABLE aud_logs ADD COLUMN impersonator_id VARCHAR(100);` + `CREATE INDEX idx_aud_logs_impersonator ON aud_logs (impersonator_id);`. (Schema today: `migrate/sql/006_audit_tables.sql:8-21`; column-add precedent: `009_p0_alignment.sql`.)
- **`internal/platform/shared/platformsink/sink.go`** (`WriteAudit` `:99-137`): read `ac := auth.FromContext(ctx)`; bind `impersonator_id = nullIfEmpty(ac.ImpersonatorID)` (nil when not impersonating). Add the column to the INSERT. `principal_id` stays `event.PrincipalID()` (= the impersonated user).
  - Import note: `platformsink` is `internal/platform/shared/platformsink`; importing `internal/platform/shared/auth` is in-tree and fine (no `pkg/fcsdk` boundary crossing).
  - sqlc: this INSERT is hand-written `tx.Inner().Exec`, not generated — no sqlc regen needed for the write. Regen only if the audit **read** query (`sqlc/queries/audit.sql`) is extended to surface the column.
- **Verification task (must do during impl):** add one integration test asserting the ctx carrying `AuthContext` reaches `WriteAudit` (i.e. `auth.FromContext(ctx)` is non-nil there). If any commit path swaps in a fresh `context.Background()`, fall back to **Plan B** below for that path.
  - **Plan B (fallback):** add `ImpersonatorID` to `ExecutionContext` (`usecase/execution_context.go`) + `EventMetadata` (`domain_event.go:43-55`, populated by `NewEventMetadata` from `ec`), expose `ImpersonatorID()` via a small optional interface `type Impersonated interface { ImpersonatorID() string }`, and have `WriteAudit` type-assert it. Heavier (every event type that should carry it must implement the method), so only used where ctx-carry proves insufficient.

### 4.5 Audit read + UI surfacing (optional but recommended for per-action value)
- `internal/sqlc/queries/audit.sql` + `internal/platform/audit/audit.go` (`Log` struct `:25-36`): add `ImpersonatorID *string` and a `?impersonatorId=` filter, so the platform Audit Log viewer can show / filter "driven by admin X".
- Frontend `AuditLogListPage.vue`: render an "impersonated by" indicator when present.

### 4.6 Endpoints
Place alongside the existing user-admin actions (`principal/api/api.go`) for the
start action, and in the auth/login package for the stop action (it operates on
the current session, not a principal id).

**`POST /api/principals/{id}/impersonate`** — start
- Gate: `platform:iam:user:impersonate` (new dedicated permission; seed it onto the anchor/super-admin role). Quick interim: `auth.RequireAnchor(ac)`.
- Guardrails (all enforced here — see §5).
- Effect: `Set-Cookie: fc_session=<impersonation token>` via the established huma Set-Cookie pattern (`header:"Set-Cookie"` output field — see memory `huma-set-cookie-pattern`). Emit a **start** audit/domain event ("admin X started impersonating Y").
- Response body: the same `loginResponse` shape as the target would get (so the SPA can hydrate the store), plus `impersonating: true` and `impersonator: {id,name,email}`.

**`POST /auth/impersonate/stop`** — stop
- Requires the current session to be an impersonation session (`ac.IsImpersonating()`); else 400.
- Re-verify `act.sub` (the admin) is still an **active anchor**; if not, hard-logout instead.
- Mint a fresh **normal** session for the admin, `Set-Cookie` it. Emit a **stop** audit event.
- Response: the admin's `loginResponse`.

### 4.7 `/auth/me` + login response — expose impersonation state
**`internal/platform/auth/login/endpoint.go`** (`handleMe` `:534-571`, `loginResponse`)
- Add `Impersonating bool` and `Impersonator *{ID, Name, Email}` to `loginResponse`.
- In `handleMe`, populate from `ac.ImpersonatorID` (resolve the admin's name/email via `Principals.FindByID`).

---

## 5. Security guardrails (enforced in the start handler)

- **Anchor-only**, ideally behind a dedicated `platform:iam:user:impersonate` permission (grantable deliberately; reads clearly in audit). Interim: `RequireAnchor`.
- **Refuse to target another anchor or super-admin** — eliminates any privilege-escalation path. Impersonating a lower-privilege user is only ever a de-escalation.
- **Refuse inactive targets, SERVICE principals, and self.**
- **No nesting** — reject if the caller's session is already an impersonation session (`act` present / `ac.IsImpersonating()`).
- **Short TTL** on the impersonation token (30–60 min) vs the normal 24h. Re-resolution from `sub` means a target deactivated mid-session is rejected on the next request automatically.
- **Block sensitive self-service while impersonating** — password change, 2FA enroll/disable, account deletion. The admin already has dedicated reset endpoints for those. Implement as an `ac.IsImpersonating()` check in those specific handlers (e.g. 2FA enroll, change-password).
- **Bearer untouched** — no impersonation claim is ever honored on the bearer branch of `introspect`.

---

## 6. Frontend changes (Vue SPA)

- **`stores/auth.ts`**: add `impersonating: boolean` and `impersonator: {id,name,email} | null` state; set them in `setUser`/from `/auth/me`; clear in `clearAuth`.
- **`api/auth.ts`**: parse the new `/auth/me` + login fields; add `startImpersonation(userId)` and `stopImpersonation()` calls.
- **Banner**: a persistent top bar ("You are viewing FlowCatalyst as **{name}** — Return to your account") wired to `stopImpersonation()`. Mount in the app layout so it shows on every page while `impersonating`.
- **"Impersonate" button** on `UserDetailPage.vue`, visible only to anchors (`auth.isPlatformAdmin` / a stricter anchor check) and hidden for anchor/inactive/service targets. On success, refresh the store via `checkSession()` and redirect to the app home.
- Optional: visually distinct chrome (e.g. colored border) while impersonating, to prevent "who am I" confusion.

---

## 7. Spec / SDK

- Add the two endpoints to the OpenAPI spec; `make dump-spec` drift guard will require they're registered. Run `make frontend-types-verify` if SPA types are generated from the spec.
- **Public TS SDK (`clients/typescript-sdk`) is NOT updated** — impersonation is an internal platform-admin/BFF feature, not part of the consumer API surface.

---

## 8. Testing

- **Unit**: `sessiontoken` mint/validate round-trips `act`; `introspect` sets `ImpersonatorID` on the cookie path and never on the bearer path.
- **Guardrails**: start handler rejects anchor target, inactive target, service target, self, and nested impersonation; non-anchor caller is forbidden.
- **Attribution**: integration test — impersonate, perform an audited mutation, assert the `aud_logs` row has `principal_id = target` **and** `impersonator_id = admin`. (Also the ctx-reaches-sink assertion from §4.4.)
- **Stop**: stop restores a normal admin session; stop on a non-impersonation session 400s; stop when the admin lost anchor → hard logout.
- **Self-service block**: change-password / 2FA-enroll rejected while impersonating.
- **E2E (optional)**: full SPA loop — start from user detail, banner shows, action audited, return to account.

---

## 9. Phased rollout

1. **Token + context plumbing** — `sessiontoken.Actor`, `provider.MintImpersonationToken`, `AuthContext.ImpersonatorID`, `introspect` wiring. (No behavior change yet; nothing calls it.)
2. **Audit attribution** — migration 037, `WriteAudit` impersonator column, ctx-reaches-sink test.
3. **Endpoints + guardrails** — start/stop, `/auth/me` fields, start/stop bracket events, self-service blocks.
4. **Frontend** — store, banner, button, api wiring.
5. **Audit viewer surfacing** — read query + filter + UI indicator (optional polish).

---

## 10. Open questions

- Permission model: dedicated `platform:iam:user:impersonate` (recommended) vs plain `RequireAnchor`?
- Impersonation TTL: 30 vs 60 min?
- Should a banner/notification also fire to the impersonated user's email or a webhook on start (some compliance regimes require notifying the user)? Out of scope unless required.
- Audit viewer: surface `impersonator_id` now (phase 5) or defer?

---

## 11. Effort estimate

| Phase | Scope | Est. |
| --- | --- | --- |
| 1 | Token + context plumbing | ~0.5 day |
| 2 | Audit attribution + migration + tests | ~0.5 day |
| 3 | Endpoints + guardrails + me fields | ~0.5–1 day |
| 4 | Frontend (store, banner, button) | ~0.5 day |
| 5 | Audit viewer surfacing (optional) | ~0.5 day |
| | **Total** | **~2.5–3 days** |

Mirrors existing patterns (2FA-reset / password-reset admin actions, huma
Set-Cookie). No changes to the OAuth surface, no consumer-SDK churn, and — via
the context-carried audit approach — no sweep of the inline `NewExecutionContext`
call sites.
