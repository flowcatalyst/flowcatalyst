# Platform configuration

Platform configuration is a store of named values that an application keeps on
the platform: settings it reads at start-up, per-client options, and secrets
such as an API key or an SFTP password. Each value belongs to an **application**
and is addressed by **section** and **property** (`orders` / `sftp` / `password`).
A value is **global**, or it can be set **for one client**. The two are separate
rows: asking for a client's value never falls back to the global one.

A client is named by its id (`clt_…`) or by its identifier (`acme`); the platform
stores the id, so both mean the same scope. An unknown client is `404`, never a
new row.

A property is either **plain** (stored and shown as text) or a **secret**.

## Who may do what

Each call has two checks: a permission for the **application**, and a
**client confinement** that decides which of its values the caller reaches.

**The application.** Anchor scope (reaching every client) is reach, not
authority, so an anchor-tier caller needs one of these:

| Action | Platform administrator | An application's service account |
|---|---|---|
| List and read properties | `platform:admin:config:view` | `platform:application-service:config:view` |
| Set and delete properties | `platform:admin:config:update` | `platform:application-service:config:update` |
| Reveal a secret | `platform:admin:config:reveal` | `platform:application-service:config:reveal` |

`platform:admin` holds view and update; `platform:config-secrets-admin` holds all
three; super-admin holds everything through its wildcard; the
`platform:application-service` role holds the three application-service
permissions, which reach only the application(s) the account is bound to. A
caller below anchor tier is decided by the per-role **access grants** on the
application (read, or read and write) instead of a permission.

**The clients.** A caller reaches these values of the application:

| Caller | Client values | The application-wide (global) value |
|---|---|---|
| Platform administrator (anchor tier) | every client's | read and write |
| An application's service account | only the clients it is **linked to**: its own client list if it has one, otherwise the clients its application is enabled for | read and write |
| Below anchor tier (access grants) | only its own clients' | read; not set or deleted |

A value outside the caller's reach is `403`, and is left out of the list. The
access grants have no client axis, so a grant to a role lets its holders act only
for the clients they belong to, never for another client.

The application and the permission are checked before a client is looked up, so
a caller who may not write the application gets the same `403` for a client that
exists and one that does not.

**Acting for a person.** An application that changes or reveals its configuration
for one of its own signed-in users (for example from its admin pages) can send
`X-On-Behalf-Of: <who>` (at most 256 printable characters) on the set, delete and
reveal calls. The platform records it in the audit row beside the service account.
It is honoured only from a service account and ignored from anyone else.

## Secrets

A secret is **encrypted at rest** (AES-256-GCM, the platform key), bound to the
property it belongs to (its application, section, property and client), so a
stored value copied to another property does not decrypt there. It is **never
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
  "clientId": "clt_…", "value": "…", "updatedAt": "2026-10-23T10:00:00Z" }
```

`clientId` is the stored client id, also when the request named the client by its
identifier. `updatedAt` changes whenever the secret is set, so a caller that
caches the value can tell when it was rotated. The response carries
`Cache-Control: no-store`; do not cache the value beyond a short, bounded time in
your own process, and never log it.

**Who may reveal.** Always a permission, never implied by being an application's
service account:

- A **platform administrator** holds `platform:admin:config:reveal` (anchor tier).
  Super-admin has it through its wildcard, and the role
  `platform:config-secrets-admin` (config view, edit and reveal) is the deliberate
  grant for the few people who may; `platform:admin` can edit configuration but
  cannot reveal a secret.
- An **application's service account** holds `platform:application-service:config:reveal`,
  which the `platform:application-service` role carries. It reaches only the
  applications the account is bound to, never another's, and only the clients it is
  linked to (see the table above) and the application-wide value; an account below
  anchor tier only the **values of its own clients** (not the application-wide
  value).

Viewing or editing configuration does not imply reveal.

**Every reveal is audited** (who, which property, when, and on whose behalf when
the application said so; never the value), and a reveal that cannot be recorded is
refused.

Errors: `403` not authorised, `404` no such property (`Config_NOT_FOUND`) or no
such client (`Client_NOT_FOUND`), `400 NOT_A_SECRET` when the property is plain
(read it with the ordinary call).

**Writing a secret.** A secret's value is required, and the mask is not a value:
setting a secret to `***` is refused (`400 SECRET_VALUE_MASKED`), as is an empty
secret (`400 SECRET_VALUE_REQUIRED`), so a value read with the ordinary calls and
written back cannot replace the secret with its mask. To leave a secret unchanged,
do not set it.

## Changes are recorded

Setting and deleting a property are recorded in the audit log (the caller, the
property and its client, and on whose behalf; never a secret's value) and emit
an event: `platform:admin:platform-config:property-set` and
`platform:admin:platform-config:property-deleted`. Deleting a property that is not
there is not an error.

## When an application is deleted

Configuration belongs to an application. Deleting the application deletes its
values (secrets included) and its access grants in the same step, and the
`platform:iam:application:deleted` event says how many
(`platformConfigPropertiesDeleted`, `platformConfigGrantsDeleted`). A new
application created later with the same code starts with no configuration.
Configuration can only be set, and access granted, for an application that
exists (`404 Application_NOT_FOUND` otherwise).

## Administration

Platform → Settings → **Configuration** lists an application's properties, adds,
edits and deletes them, and reveals a secret after a confirmation (the reveal is
audited). The rules are the API's: the list shows only the values the viewer
reaches, and edit and delete are offered only for values the viewer may write. A
secret's value is write-only in the form: it is never filled in when you edit, and
saving requires entering a new value. The page needs
`platform:admin:config:view`; editing needs `platform:admin:config:update`;
revealing needs `platform:admin:config:reveal`.

## From code

Every SDK has a reveal call (`revealSecret` / `reveal_secret`) beside the other
platform-configuration calls, and reads of secrets through the ordinary get/list
calls return `***`. Applications built on the FlowCatalyst framework read
their secrets through the framework's secret resolver, whose `platform` driver
uses this endpoint, and their admin pages send `X-On-Behalf-Of` with the signed-in
user.

## Notes for operators

- The platform key (`FLOWCATALYST_APP_KEY`) must be set for secrets. Without it a
  secret cannot be written or revealed (secrets stored before encryption at rest
  included): the platform refuses rather than store or return plaintext. The
  platform still starts, and logs how many secrets it cannot read.
- Secrets written before encryption at rest was introduced, and secrets encrypted
  before the ciphertext was bound to its property, are converted when the platform
  starts, and when they are next revealed. This is automatic and repeatable, and a
  conversion never overwrites a value that was set while it ran.
- After rotating `FLOWCATALYST_APP_KEY`, keep the old key as
  `FLOWCATALYST_APP_KEY_PREVIOUS`: existing secrets stay readable with it, and a
  secret is re-encrypted under the new key the next time it is set. (There is no
  bulk re-encryption of platform configuration under a new key yet.)
- Configuration left behind by applications deleted before this release is removed
  by a database migration; values whose application code is ambiguous (the
  platform's own `platform`, or a code that matches an application only when case
  and spaces are ignored) are kept.
