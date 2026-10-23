# Platform configuration

Platform configuration is a store of named values that an application keeps on
the platform: settings it reads at start-up, per-client options, and secrets
such as an API key or an SFTP password. Each value belongs to an **application**
and is addressed by **section** and **property** (`orders` / `sftp` / `password`).
A value is **global**, or it can be set **for one client**, which overrides the
global value for that client only.

A property is either **plain** (stored and shown as text) or a **secret**.

## Secrets

A secret is **encrypted at rest** (AES-256-GCM, the platform key) and is **never
returned** by the ordinary calls. Listing an application's configuration, reading
a property and setting one all return the literal `***` for a secret, for every
caller, administrators included. The only way to read a secret's value is the
**reveal** call:

```
GET /api/config/{app}/{section}/{property}/secret            (global value)
GET /api/config/{app}/{section}/{property}/secret?clientId=  (a client's value)
```

```json
{ "applicationCode": "orders", "section": "sftp", "property": "password",
  "value": "…", "updatedAt": "2026-10-23T10:00:00Z" }
```

`updatedAt` changes whenever the secret is set, so a caller that caches the value
can tell when it was rotated. The response carries `Cache-Control: no-store`; do
not cache the value beyond a short, bounded time in your own process, and never
log it.

**Who may reveal.** An application's own service account may reveal the secrets
of **its own application** (never another's). Any other caller needs the
permission `platform:admin:config:reveal`, which no built-in role carries except
super-admin (its `platform:*:*:*` covers it); grant it through a role. Viewing or
editing configuration does not imply it. A client-scoped secret also requires
access to that client.

**Every reveal is audited** (who, which property, when; never the value), and a
reveal that cannot be recorded is refused.

Errors: `403` not authorised, `404` no such property, `400 NOT_A_SECRET` when the
property is plain (read it with the ordinary call).

## Administration

Platform → Settings → **Configuration** lists an application's properties, adds,
edits and deletes them, and reveals a secret after a confirmation (the reveal is
audited). A secret's value is write-only in the form: it is never filled in when
you edit, and saving requires entering a new value. The page needs
`platform:admin:config:view`; editing needs `platform:admin:config:update`;
revealing needs `platform:admin:config:reveal`.

## From code

Every SDK has a reveal call (`revealSecret` / `reveal_secret`) beside the other
platform-configuration calls, and reads of secrets through the ordinary get/list
calls now return `***`. Applications built on the FlowCatalyst framework read
their secrets through the framework's secret resolver, whose `platform` driver
uses this endpoint.

## Notes for operators

- The platform key (`FLOWCATALYST_APP_KEY`) must be set. Without it a secret
  cannot be written or revealed: the platform refuses rather than store plaintext.
- Secrets written before encryption at rest was introduced are converted when the
  platform starts; this is automatic and repeatable.
- After rotating `FLOWCATALYST_APP_KEY`, keep the old key as
  `FLOWCATALYST_APP_KEY_PREVIOUS`: existing secrets stay readable with it, and a
  secret is re-encrypted under the new key the next time it is set. (There is no
  bulk re-encryption of platform configuration yet.)
