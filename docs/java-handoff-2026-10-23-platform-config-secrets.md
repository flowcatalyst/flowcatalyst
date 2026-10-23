# Java platform hand-off: platform-config secrets (2026-10-23)

For `flowcatalyst-javalin`. The Go platform changed how SECRET platform-config values are stored and read;
the wire is the contract, `api/openapi.lock.json` is the reference (operation `revealPlatformConfigSecret`,
schema `SecretResponse`). Go commits: `4d4c6c8c` (platform), `2be7c7da` (admin page, published doc),
`05fd177b` (SDKs). Published doc: `docs/published/70-platform-configuration.md`.

## What to mirror

1. **Encrypt SECRET values at rest.** AES-256-GCM with the platform key (`FLOWCATALYST_APP_KEY`, previous keys
   for rotation), stored as `encrypted:` + the usual envelope, so a legacy plaintext row (no marker) is
   distinguishable. Convert legacy rows once at start-up (idempotent). A SECRET write with no key is refused,
   never stored in plaintext. Only SECRET rows are touched; PLAIN rows stay as they are.
2. **Mask everywhere.** `GET /api/platform-config/{app}`, `GET /api/config/{app}/{section}/{property}` and the
   `PUT` response return the literal `"***"` as `value` for a SECRET, for every caller, anchor included.
   (Previously only non-anchors were masked.)
3. **Reveal endpoint** `GET /api/config/{app}/{section}/{property}/secret?clientId=`:
   - response `SecretResponse {applicationCode, section, property, clientId?, value, updatedAt}` with
     `Cache-Control: no-store`; `updatedAt` is the value's version (it changes when the secret is set);
   - authorisation (amended 2026-10-23, always a permission): the application confinement that already exists;
     then the caller holds `platform:admin:config:reveal` (anchor required; not implied by `config:view`/`update`;
     in no seeded role except the new `platform:config-secrets-admin`, which also carries config view and update;
     super-admin's wildcard covers it) **or** `platform:application-service:config:reveal` (carried by the
     `platform:application-service` role), in which case the principal must be application-scoped (not
     all-applications) and bound to the requested application, and, below anchor tier, may reveal only
     **client-scoped** values of clients in its client list (the application-wide value is refused);
     a client-scoped value always needs access to that client; the client reference is resolved after these checks;
   - `404` for a missing property, `400 NOT_A_SECRET` for a PLAIN one;
   - **every reveal writes an audit row** (`entityType PLATFORM_CONFIG`, `entityId` the config id, operation
     `SECRET_REVEALED`, principal, client; never the value) and the reveal is **refused if the audit row cannot be
     written**.
4. The Go tests to port are in `internal/platform/platformconfig/{api,operations}` (authorisation table,
   encrypted-at-rest, fail-closed without a key, legacy conversion).

## Also, gate the other four calls (2026-10-23)

List, get, set and delete were open to any anchor-tier caller (no permission check; anchor scope was treated as
authority). They are now gated: view for list and get, update for set and delete, for an anchor-tier caller:
`platform:admin:config:view|update`, or, for an application-bound service account (not all-applications access),
`platform:application-service:config:view|update` (new, carried by the `platform:application-service` role).
Callers below anchor tier keep the existing per-role access grants. The gate runs before any client lookup.
Go: `auth.RequireConfigView` / `RequireConfigUpdate`; tests in `shared/auth/config_gate_test.go` and
`platformconfig/operations/ops_pg_test.go`. Only the SPA's admin pages call these routes today.

## Also

- The Java SDK (`java-sdk` v0.0.22) already has `client.platformConfig().revealSecret(..)`; its `toString` redacts.
- Framework side (secrets driver and admin pages) is specified in
  `../flowcatalyst-framework/docs/contracts/platform-config.md`.
- No schema change: the value column is unchanged (the marker is in the value).
