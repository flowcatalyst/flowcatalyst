# Removing the foreign keys

Ruled 2026-08-27: invariants belong in the domain, not the schema. The reasoning
is in `token-claims-followups.md` §4 — foreign keys do not partition, they sit
at the wrong boundary for cross-aggregate references, they hide business rules
where no test covers them, and they rely on implicit cascading. This is the plan
to remove the 16 we have.

**The columns stay. The indexes stay. Only the constraints go**, and only after
the domain does the work they were doing.

---

## What they are actually doing

Almost none of them are validating anything. Fifteen of the sixteen exist to
**delete or mutate rows in response to a parent's deletion** — cleanup, not
integrity. That matters, because "remove the foreign key" is really "make the
delete path do its own work".

### Group A — child cleanup (13 constraints)

`ON DELETE CASCADE` from a child table to its owning aggregate root.

| Child table | Parent |
|---|---|
| `iam_principal_roles` | `iam_principals` |
| `iam_role_permissions` | `iam_roles` |
| `iam_permissions` | `iam_principals` |
| `iam_user_mfa_methods` | `iam_principals` |
| `iam_user_mfa_recovery_codes` | `iam_principals` |
| `iam_mfa_email_pins` | `iam_principals` |
| `iam_mfa_trusted_devices` | `iam_principals` |
| `iam_reset_approval_requests` | `iam_principals` |
| `webauthn_credentials` | `iam_principals` |
| `oauth_client_redirect_uris` | `oauth_clients` |
| `oauth_client_post_logout_redirect_uris` | `oauth_clients` |
| `oauth_client_allowed_origins` | `oauth_clients` |
| `oauth_client_grant_types` | `oauth_clients` |
| `oauth_client_application_ids` | `oauth_clients` |
| `app_application_openapi_specs` | `app_applications` |

These are the easy ones, and the codebase is already most of the way there.
`principal.Repository.Delete` **already** clears `iam_principal_application_access`
and `iam_client_access_grants` explicitly — the two junctions that have no
foreign key — while leaning on the constraint for the ones that do. The instinct
is right; it just stopped where the schema took over.

### Group B — cross-aggregate side effects (2 constraints) ⚠️

These are the ones worth being alarmed about, because they are not cleanup at
all. They mutate *other aggregates* on delete, and nothing in the code says so.

**`oauth_clients.service_account_principal_id` → `iam_principals` ON DELETE
CASCADE** (migration 027). Deleting a SERVICE principal **deletes the OAuth
client row outright**. An operator removing a principal silently destroys a
client's credentials and every relying party configured against it.

**`app_applications.service_account_id` → `iam_principals` ON DELETE SET NULL**
(migration 028). Deleting a principal **nulls an application's service-account
pointer**, silently unlinking the application from the account that syncs it.

Neither is expressed anywhere in Go. Neither has a test. Neither produces an
audit record. Whatever we decide these operations *should* do, "an ALTER TABLE
from a migration a year ago decides it" is not the answer — this is the hidden-
business-rule objection in its strongest form, and it is the reason to do this
work rather than merely tidy the schema.

---

## Plan

Constraints come off **last**. Every phase before that leaves the database
exactly as strict as it is today, so a mistake in the new code is still caught.

### Phase 1 — make Group A explicit (no schema change)

For each parent aggregate, extend its `Delete` to remove its children in the
same transaction, following the pattern `principal.Repository.Delete` already
uses for the unconstrained junctions.

- `principal.Repository.Delete` — add roles, permissions, the four MFA tables,
  webauthn credentials, reset-approval requests.
- `auth` OAuth client delete — add the five `oauth_client_*` child tables.
- `application.Repository.Delete` — add OpenAPI specs.
- `role.Repository.Delete` — add role permissions.

Each gets a test that deletes a parent with children present and asserts zero
orphans remain, queried directly. Those tests are the real deliverable: they
pass identically before and after the constraints are dropped, which is what
makes the drop safe.

### Phase 2 — decide Group B deliberately

Not a mechanical port. Each needs a product decision first, then code:

- **Deleting a SERVICE principal that owns an OAuth client.** Options: refuse
  while a client references it (probably right — it is a destructive surprise
  otherwise), or delete the client as an explicit, audited step. Either way the
  caller learns what will happen before it happens.
- **Deleting a principal linked to an application.** Options: refuse, or unlink
  explicitly with an audit record. The silent `SET NULL` is the one behaviour
  that should not survive: an application quietly losing its sync account is
  invisible until a sync fails.

Whatever is chosen, it emits an event and is covered by a test.

### Phase 3 — drop the constraints

One migration, `ALTER TABLE … DROP CONSTRAINT` for all 16. Columns and indexes
untouched — the indexes were created for the cascade lookups and are equally
useful to the explicit deletes that replace them.

The down migration re-adds them, which will fail if orphans have accumulated in
the meantime. That is correct and worth stating: it is a real rollback
constraint, not an oversight.

### Phase 4 — replace the safety net

Removing a constraint removes a guarantee. Do not pretend otherwise: add a
reconciliation check that counts orphans per junction and reports them, run on
a schedule and exposed as a metric so a regression is visible rather than
discovered. This is the standard shape wherever foreign keys are given up at
scale — the integrity check moves from the write path to an asynchronous
auditor.

Start it in Phase 1, before anything is dropped, so it has a clean baseline to
alarm against.

### Parallel workstream — typed identifiers

`token-claims-followups.md` §4 part 2 covers the other half: a `RoleName` type
whose only constructor resolves against `iam_roles`, so an unvalidated
assignment cannot be constructed. That closes the one case in this set that was
never about deletion — `iam_principal_roles.role_name` has no constraint today
and is the reference most likely to dangle.

Independent of the phases above, and worth doing first: it is the change that
prevents new bad data, where the rest prevents leftovers.

---

## Sequencing note

Do not batch this with the removal of `iam_reset_approval_requests` rows (see
`token-claims-followups.md`) — that table has no cleanup sweep at all today,
which makes it the one child table where a Phase 1 bug would be least visible.
Give it its expiry sweep first, then include it here.
