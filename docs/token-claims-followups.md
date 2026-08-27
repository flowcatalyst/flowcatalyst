# Token claims — known gaps and follow-ups

Scope: the platform as OIDC **provider** — what the id_token, access token and
session cookie assert, and the endpoints that answer the same questions
(`/api/me`, `/oauth/userinfo`). Written 2026-08-27 after a coherence pass that
closed most of the list; what remains is here so it isn't rediscovered from a
production symptom.

The model these are measured against: an **id_token is an assertion to the
application** about its own user — what that user holds *inside that
application's walls*. An **access token is a credential**, authority-free by
default (`token_use: identity`, rejected by the platform API middleware) so a
relying party cannot relay a user's token to act as them; `apiAccess` is the
explicit per-client grant that makes it authority-bearing, narrowed to that
client's applications.

---

## 1. `email_verified` is hardcoded `true` — ACTION REQUIRED

`authservice.generateIDToken` sets `email_verified` whenever the principal has
an email at all. It reflects nothing about actual verification.

A relying party is entitled to read this as "the platform verified this
address", and some will branch on it — skipping their own verification step, or
matching an account by email on the strength of it. Today that trust is
unearned.

Fixing it needs a real verification state on the principal first (there is
none), then either emitting it truthfully or omitting the claim. Omitting is the
safer interim: an absent claim means "not asserted", which is accurate, while a
hardcoded `true` is a false assertion. Left as-is deliberately for now — noted
here rather than fixed because it needs the identity model extended, not a claim
tweak.

## 2. `all_applications` is deprecated, not yet removed

The `applications` claim carries `"*"` for a principal that reaches every
application, mirroring how `clients` has always signalled an anchor. The
`all_applications` boolean is still emitted alongside for consumers that predate
the sentinel.

Both SDKs parse the sentinel and honour the boolean, and both mark the boolean
`@deprecated`. Remove the claim once deployed consumers have moved — the Go
middleware ORs the two (`auth.ParseApplicationsClaim`), so the boolean can be
dropped from the mint side without a coordinated release.

## 3. Portal id_tokens carry `tier: ""` — correct, but not the right shape

`oauthapi.redeemPortalCode` builds a synthetic principal to mint from
(`portal_token.go`), setting only id/type/name/email. `Scope` is left at its zero
`UserScope`, so the `tier` claim serialises as `""`.

**Do not "fix" this by setting a tier.** A portal identity is not a platform
principal — it lives on its own plane, per (client, email), with no tenancy
tier at all. Stamping it `CLIENT` would assert it *is* a CLIENT-tier platform
principal, which is false and is precisely the kind of claim a relying party
would branch on. The portal token exists to prove identity and nothing else.

The empty string is not the defect; it is the visible edge of a claim that does
not apply to this token. `Tier` is `json:"tier"` with no `omitempty`, so
"absent" and "empty" render the same. If this is ever tidied, the change is to
omit the claim for portal tokens and let consumers default when it is missing —
not to fill it in.

Worth knowing that both SDKs resolve tier with `??`, so an empty string is not
nullish and passes through instead of falling back to their default. That is
the SDKs' half to handle if the claim starts being omitted. Currently working
as-is, and any change here is a wire change for portal relying parties.

## 4. Role assignment has no write-path validation

Not a claims issue, but it is what will produce the next surprise here.

`iam_principal_roles.role_name` is an unconstrained `VARCHAR` with no foreign
key to `iam_roles.name`, and no write path checks that the role exists. Console
assignment picks from the definitions so it is always canonical; SDK
`sync_principals` stores whatever it is sent, lower-cased, without a lookup. An
assignment naming a role that does not exist resolves to no permissions at
mint time, silently.

Both environments' data is currently clean — but by accident: the one app doing
principal sync exact-match-filters against the platform's qualified role list
before sending, so it cannot send anything else.

The constraint applies cleanly to that data:

```sql
ALTER TABLE iam_principal_roles
  ADD CONSTRAINT fk_iam_principal_roles_role_name
  FOREIGN KEY (role_name) REFERENCES iam_roles (name)
  ON UPDATE CASCADE ON DELETE RESTRICT;
```

`ON UPDATE CASCADE` because renaming a role currently orphans every assignment
of it; `ON DELETE RESTRICT` because deleting one currently strips access
silently.

It is not free: adopting it fails ~15 test packages, which assign undefined role
names freely — the codebase depends on the missing constraint. Doing it properly
means validating in the write paths (so a caller gets a 400, not a constraint
500), then seeding role definitions in those fixtures. Sequenced separately for
that reason.

`iam_principal_application_access` and `iam_client_access_grants` have no
foreign keys on any column either; check for orphans before adding them.
`iam_role_permissions.permission` cannot take one while `iam_permissions`
remains dormant.
