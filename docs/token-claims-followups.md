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

## 4. Role assignment is not validated — the domain must own this

Not a claims issue, but it is what will produce the next surprise here.

`iam_principal_roles.role_name` is a plain `VARCHAR` and **no write path checks
that the role exists**. Console assignment picks from the definitions so it is
always canonical; SDK `sync_principals` stores whatever it is sent, lower-cased,
with no lookup. An assignment naming a role that does not exist resolves to no
permissions at token-mint time, silently — the principal simply has less access
than intended, with nothing logged and nothing to see in the row.

Both environments' data is currently clean, but **by accident**: the one app
doing principal sync exact-match-filters against the platform's qualified role
list before sending, so it cannot send anything else. Remove that app's filter,
or add a second syncing app, and the invariant is gone.

### Not a foreign key

Ruled 2026-08-27: **do not add one.** The invariant belongs to the domain, not
the schema.

The reasoning is not merely stylistic.

**It does not partition.** This platform already range-partitions five
messaging tables by `created_at` (migration 019, on every profile including
fc-dev, precisely so constraint shapes that do not survive partitioning fail in
dev rather than in production). MySQL — a documented future backend
(`envcfg.go`) — does not support foreign keys on partitioned tables at all.
Building on a mechanism the schema's own growth path cannot keep is borrowing
against a bill that comes due at the worst moment.

**It is the wrong boundary.** Role definitions and principal role assignments
are *different aggregates*, so this is cross-aggregate referential integrity —
the case that does not survive sharding and is first to break on a move to a
non-relational store. The constraint would encode a rule outside the boundary
that owns it, in the one place that cannot travel with it.

**It hides business rules in the schema.** `ON DELETE RESTRICT` is a domain
decision — "a role in use cannot be deleted" — written where no test covers it,
no reviewer reads it, and no error message explains it. The caller gets a
constraint violation instead of a reason.

**It relies on implicit cascading.** A mechanism that silently rewrites rows
the caller did not ask about is invisible in exactly the situations where you
most need to see it.

Reaching for the database to enforce a domain rule is a signal the domain code
is underbuilt. The fix is to build it.

### The fix, in three parts

**1. Reject at the write boundary.** `sync_principals` resolves each incoming
name against the application being synced and returns a validation error naming
the unknown role, rather than storing a string nobody checked. A caller learns
immediately instead of discovering it as missing permissions weeks later.

**2. Make an unvalidated write unrepresentable.** Today an assignment carries
`Role string`, so any writer can invent one. If it can only be constructed from
a *resolved* role — a `RoleName` type whose sole constructor is
`role.Role.CanonicalName()` — then holding one is proof the lookup happened.
This is stronger than the constraint it replaces: a foreign key rejects a bad
write at commit; a type stops the bad write being written.

It also collapses the two spellings for free. `hr-manager` and `hr:hr-manager`
can no longer both exist, because there is exactly one way to produce the
value.

**3. Handle rename and delete explicitly.** This is the half the constraint
would have covered that is not validation at all: integrity under operations in
the *other* aggregate. Today renaming a role orphans every assignment of it,
and deleting one strips access silently — no code covers either.

Deletion refuses while principals hold the role, with an error naming how many
do. Renaming is best **forbidden**: the canonical name is derived from the
application code and the short name, so a rename is really a new role plus a
migration of its holders, and modelling it as an in-place edit invites exactly
the silent multi-row rewrite that made the schema-level cascade a bad idea. If
it must exist, it is an explicit operation that emits an event, not a quiet
fan-out.

Better as explicit, testable behaviour than as a schema clause nobody reads —
and, unlike the clause, it ports.

### What this deliberately does not protect against

A type holds against code. It does not hold against `psql`, a migration script,
or a support query. A foreign key would. That trade is accepted: the domain is
the place invariants live, and a datastore that cannot express them must not be
the reason they go unexpressed.

Worth being clear-eyed that today the code is trusted more than it has earned —
the clean data is an accident of one app's filtering, not a property the
platform maintains. Parts 1 and 2 are what earn it.

### The same shape elsewhere

`iam_principal_application_access` and `iam_client_access_grants` hold
unvalidated ids on both columns, and `iam_role_permissions.permission` is a free
string with no check that the permission is defined (`iam_permissions` is
largely dormant) — which is why the double-prefixed
`logistics_portal:logistics_portal:…` survives. Same reasoning, same three-part
fix, no constraints.

Note the existing FK on `iam_principal_roles.principal_id` is a different case
and can stay: the junction *belongs to* the principal aggregate, so that one is
intra-aggregate identity rather than a cross-aggregate reference. The line is
between "this row's own identity" and "a pointer into another aggregate".
