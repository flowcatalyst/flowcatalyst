# Auth hardening + client-administrator (Phase 8)

Follow-on to the 2FA work (docs/2fa-implementation-plan.md). Status: **complete
(8a–8d), uncommitted**. Owner: andrew@belac.io. Started 2026-06-06.

Verified: `go build ./...`, `go vet`, `make api-diff` (reset-approval endpoints in
the lockfile), embedded-PG migrate test (→ v32), frontend `vue-tsc -b` +
`npm run build`. Simplification: admin-notification finds client-admins by home
client (PARTNER-grant fan-out is a follow-up).

2026-11-09 (fix-identity): the queue rule is wired as an opt-in env var
(`FC_PASSWORD_RESET_REQUIRE_STRONG_FACTOR`, default `false` — owner ruling: a
user without a strong factor, including an email-PIN-only user, keeps the
emailed reset; 16b63d0f had made it opt-in with nothing able to set it, so the
queue could never run). Goal 2 below therefore holds only with the flag on;
with it off, users WITH a strong factor still confirm the link with it. A passkey
now counts as a strong factor: the reset is confirmed with a passkey assertion
(`POST /auth/password-reset/passkey/begin`, then confirm with
`passkeyStateId` + `passkeyCredential`). `platform:iam:user:reset-2fa` is
seeded (iam-admin, client-admin; migration 113 grants it to every role that
held `user:update`) and gates reset-2fa, a send-password-reset with
`reset2fa`, and approving a queued reset.

2026-11-12 (token authority): changing how an account signs in now needs a
recent sign-in and an SSO-managed account has no platform passkey. (1) Passkey
register / delete and the 2FA self-service writes are refused
`403 REAUTH_REQUIRED` for a browser session whose sign-in is older than
`FC_AUTH_STEPUP_MAX_AGE_SECS` (default 900; `0` off), in the SPA and `/ui`.
(2) A user whose email domain is mapped to an OIDC provider cannot register a
passkey (`400 SSO_MANAGED`, as change-password) and cannot sign in with one
(`authenticate/begin` answers as for an unknown email, `complete` answers
`INVALID_CREDENTIALS`), so disabling them at the identity provider ends their
access. The reset-with-a-passkey path above is unchanged: SSO-federated users
get no self-service reset link in the first place. (3) `FC_DEVELOPER_CREDENTIAL_SSO_MAX_AGE_DAYS`
(default `0`, off) can refuse a developer `client_credentials` token for an
SSO-managed user with no recent interactive sign-in. (4) A password change, a
confirmed password reset, an administrator's password or 2FA reset and a
deactivation also end the user's existing browser sessions
(`iam_principals.sessions_valid_after`, migration 116; the user who changes their
own password is re-issued a session; sign-out clears only that browser).

An administrator's credential-takeover actions are bounded by the role ceiling:
admin reset-password, reset-2fa, send-password-reset with `reset2fa`, and
setting another principal's developer credential are refused with 403
`ROLE_ABOVE_CALLER` when the target holds a role with a platform permission the
administrator lacks (a super-admin passes; acting on oneself is not an
escalation). The `/api/principals/{id}` update, activate, deactivate and delete
routes act on USER principals only (a service principal answers 409
`NOT_A_USER`), and only an anchor-tier administrator may set
`allApplications: true`; a service principal's application access needs the
service-account gate (anchor reach and the account's application).

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
