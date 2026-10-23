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
   - authorisation, in order: the application confinement that already exists (an application-scoped principal
     only reaches its own application); then either the caller is an application-scoped anchor principal for that
     application (an application's own service account) **or** holds the new permission
     `platform:admin:config:reveal` (anchor required; not implied by `config:view`/`config:update`; add it to the
     permission catalogue, in no seeded role, super-admin's wildcard covers it); a client-scoped value also needs
     access to that client;
   - `404` for a missing property, `400 NOT_A_SECRET` for a PLAIN one;
   - **every reveal writes an audit row** (`entityType PLATFORM_CONFIG`, `entityId` the config id, operation
     `SECRET_REVEALED`, principal, client; never the value) and the reveal is **refused if the audit row cannot be
     written**.
4. The Go tests to port are in `internal/platform/platformconfig/{api,operations}` (authorisation table,
   encrypted-at-rest, fail-closed without a key, legacy conversion).

## Also

- The Java SDK (`java-sdk` v0.0.22) already has `client.platformConfig().revealSecret(..)`; its `toString` redacts.
- Framework side (secrets driver and admin pages) is specified in
  `../flowcatalyst-framework/docs/contracts/platform-config.md`.
- No schema change: the value column is unchanged (the marker is in the value).
