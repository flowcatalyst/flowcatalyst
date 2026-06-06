# Auth hardening + client-administrator (Phase 8)

Follow-on to the 2FA work (docs/2fa-implementation-plan.md). Status: **complete
(8a–8d), uncommitted**. Owner: andrew@belac.io. Started 2026-06-06.

Verified: `go build ./...`, `go vet`, `make api-diff` (reset-approval endpoints in
the lockfile), embedded-PG migrate test (→ v32), frontend `vue-tsc -b` +
`npm run build`. Simplifications: reset-confirm strong factor = TOTP only (passkey
users sign in with the passkey); admin-notification finds client-admins by home
client (PARTNER-grant fan-out is a follow-up).

## Goals (confirmed with owner)

1. **Login model** (already built): internal users choose passkey (passwordless,
   no 2FA) OR password + 2FA (authenticator app / email PIN). No SMS.
2. **Password reset can't be email-only.** Self-service reset requires a STRONG
   factor — **authenticator app (TOTP) or passkey**. Email PIN does NOT authorize
   a reset (code + link both arrive in the same inbox). No strong factor → admin
   approval.
3. **Reset-approval queue.** A user with no strong factor (email-PIN-only / lost
   device) files a reset request; the **client-administrator(s) of the user's
   client** get an email linking to the queue entry; they approve **in the
   dashboard**; approval emails the user an authorised reset link that also clears
   2FA so they re-onboard.
4. **Anchor / platform users** are expected to be **OIDC-federated** (no password
   to reset). Internal anchor users get no approval path — recovery is manual/ops.
   The approval queue is effectively a client-user feature.
5. **client-administrator role** — scoped to the clients the admin can access
   (one for CLIENT scope, several for PARTNER). Full user management for those
   clients (create, reset password, approve resets, reset 2FA, deactivate, edit
   roles). **Guardrails:** can only attach roles/applications the client already
   has access to, and can NEVER assign platform roles/permissions or grant/revoke
   client access (no privilege escalation, no cross-client).

## How it fits the existing authz model

(Refs: internal/platform/shared/auth/auth.go, seed/roles.go, seed/permissions.go,
principal/api/api.go, principal/operations/.)

- Permissions are 4-segment `platform:<ctx>:<resource>:<action>` strings matched
  with `*` wildcards. Client-scope is enforced **separately** via
  `CanAccessClient` / `CheckScopeAccess`.
- So the client-admin holds the **same** user-management permissions as a platform
  iam-admin; the difference is purely **scope**: a CLIENT/PARTNER-scoped admin only
  passes `CanAccessClient` for their own client(s). Today the user-management
  endpoints are gated `RequireAnchor` (which ignores permissions) — replacing that
  with "anchor OR (has perm AND CanAccessClient(target))" yields client-admin
  behaviour for free, scoped by `CanAccessClient`.
- **Platform roles** = `ApplicationID == nil` and name starts `platform:`.
  Client-admins must be blocked from assigning these.

## Build stages

### 8a — client-administrator role + scoped authz (backend)
- seed/permissions.go: add `platform:iam:user:reset-2fa` (and reuse existing
  user:create/update/delete/view + assign-roles).
- seed/roles.go: add seeded role `platform:client-admin` holding user
  view/create/update/delete + assign-roles + role:view + reset-2fa. (Name kept in
  the `platform:` namespace for the role registry, but it grants nothing
  platform-wide on its own — scope gates every action.)
- auth.go: new helpers
  - `RequireUserAdmin(a, targetClientID *string) error` — nil if anchor, else
    requires a user-write permission AND `CanAccessClient(targetClientID)`
    (targetClientID nil → anchor only; platform/no-client users are anchor-managed).
  - `CanResetUserTwoFactor`, and a role-assignment guard
    `AssertAssignableRoles(a, roles []role.Role, clientApps []string) error` that
    rejects platform roles and roles whose ApplicationID isn't in the client's
    accessible applications (anchors bypass).
- principal/api/api.go: swap `RequireAnchor` → `RequireUserAdmin(ac, targetClient)`
  on create, createUser (force the new user's client to one the admin can access),
  resetPassword, sendPasswordReset, resetTwoFactor, assignRoles (+ role bounding).
  Leave grant/revoke client-access and set-client-association anchor-only.

### 8b — reset-flow hardening (backend)
- `/auth/password-reset/request`: look up the user's strong factors (confirmed
  TOTP, or a registered passkey). Has one → issue a reset token as today but flag
  it `requires_factor`. None → create a reset-approval request (8c) and notify
  admins; do NOT issue a self-service token. Response stays generic
  (anti-enumeration): "we've started the process / an administrator may need to
  approve."
- `/auth/password-reset/confirm`: when the token is `requires_factor`, require a
  TOTP code or passkey assertion in the same call before `ResetPassword`. Email
  PIN is not accepted here.

### 8c — reset-approval queue (backend + dashboard)
- New table `iam_reset_approval_requests` (id, principal_id, client_id, status
  [pending|approved|denied|expired], requested_at, decided_by, decided_at,
  note). Repo + ops.
- Endpoints (client-admin gated, scoped):
  `GET /api/reset-approvals` (pending for the admin's clients),
  `POST /api/reset-approvals/{id}/approve` (→ mints an authorised, reset_2fa reset
  token + emails the user the link), `POST /api/reset-approvals/{id}/deny`.
- Email to client-admins: subject "A password reset needs your approval", deep
  link to the dashboard queue entry. Best-effort, anti-enumeration-safe.

### 8d — frontend
- Reset page: factor step (TOTP/passkey) when `requires_factor`; "an admin has
  been notified" terminal state when queued.
- Client-admin user management screens (create/list/reset/reset-2FA/deactivate,
  role editor bounded to client roles/apps).
- Approval queue screen (list pending, approve/deny).

## Open defaults (assumed unless corrected)
- Reset-with-passkey in practice = log in with the passkey + change password in
  Profile; the explicit factor-reset path targets the "forgot password, still
  have the authenticator app" case.
- Client-admin authority spans exactly the clients they can access (CLIENT → one,
  PARTNER → their granted set), via `CanAccessClient`.
- Approval requests expire (e.g. 72h) and are single-decision.
