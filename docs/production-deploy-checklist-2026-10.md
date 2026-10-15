# Production deploy checklist: platform release after 2026-10-15

Scope: deploy the Go platform to production WITHOUT updating any SDK. The main worry is login and
token validation. Nothing here has been run against production; the data queries are read-only.
Items marked (unverified) are assumptions from reading the code, not tested facts.

## 0. What is in the release (so you know what to watch)

| Change | Ledger | Risk |
|---|---|---|
| Migration 075: connections' service account `prn_...` -> `sac_...` | F17 | data change, idempotent |
| Migration 076: whitespace role assignments renamed (bad row dropped if the real one exists) | F18 | data change, idempotent |
| Migration 077: `iam_roles.application_id` filled from `application_code` where NULL | F18 | data change, idempotent |
| Role names/permissions validated at every write (400 `INVALID_ROLE_NAME`, `INVALID_PERMISSION`, `ROLE_NOT_FOUND`) | F18 | behaviour: stricter writes |
| Code-first connection sync fails with `APPLICATION_SERVICE_ACCOUNT_REQUIRED` if the application has no service account | F17 | behaviour: stricter sync |
| Login hardening: JSON-only sign-in, cookie-session CSRF rule, SSO login-state cookie `fc_sso_*`, SPA headers | F16 (13 Oct) | login (check whether prod already has it) |
| Function runner: `fn.dispatch`, runner identity fix, `fcdev fn delete` | n/a | only if you run the function runner |

Also check your production's last applied migration number (`goose_db_version`) and list everything after it:
this release may include 071-074 as well (event-type catalogue cleanup; they remove 16 never-emitted event types
and reshape function event type codes).

Token signing and validation code did not change in this release (unverified beyond the F18 agent's isolated tests).

## 1. Before the window

- [ ] Note the current production image/tag so you can redeploy it.
- [ ] **Backup the production database** and record where it is. Confirm the restore works somewhere (see section 5).
- [ ] Restore that backup into a scratch/staging database and run the new platform's migrations against the copy. Compare
      row counts the migrations touch (section 2 queries before and after).
- [ ] Run the read-only checks (section 2) on production data. Fix or understand every non-empty result.
- [ ] Confirm non-production ran this exact build, with the apps on their CURRENT SDK versions, and that these passed there:
  - [ ] password login (with 2FA, and a passkey if used)
  - [ ] federated (SSO) login through the real proxy (done)
  - [ ] OAuth code + PKCE flow and refresh for each app type (Laravel apps, Java app, AgentPlanner)
  - [ ] each app's own authenticated call after login (this exercises the SDK's issuer and id_token checks)
  - [ ] a client-credentials token used against an API by each service-account consumer
  - [ ] each app's definition sync (`flowcatalyst:sync`, Node/Java equivalents)
  - [ ] console writes through the proxy (create/edit something, rotate a secret)
- [ ] Pick an off-peak window and tell the people who log in.

## 2. Read-only data checks (run on production data first, again after the deploy)

Column and table names come from the code notes; confirm them against your schema.

```sql
-- F17: connections still holding an application principal id (expect 0 after 075 or a sync)
SELECT count(*) FROM msg_connections WHERE service_account_id LIKE 'prn_%';

-- F18: role assignments with whitespace (expect 0 after 076)
SELECT principal_id, role_name FROM iam_principal_roles WHERE role_name ~ '\s';

-- F18: role names the new validator would refuse (expect 0 rows; each row is a write that would start failing)
SELECT name FROM iam_roles WHERE name !~ '^[A-Za-z0-9_.-]+(:[A-Za-z0-9_.-]+)+$';
SELECT DISTINCT role_name FROM iam_principal_roles
 WHERE role_name !~ '^[A-Za-z0-9_.-]+(:[A-Za-z0-9_.-]+)+$';

-- F18: roles with no application id (077 leaves rows it cannot match)
SELECT id, name, application_code FROM iam_roles WHERE application_id IS NULL;

-- F17: applications without a service account (their connection sync would be refused)
SELECT id, code FROM app_applications WHERE service_account_id IS NULL;
```

Record the results. Anything unexpected: stop and look before deploying.

## 3. Deploy

- [ ] Take the final backup immediately before the deploy (note the time).
- [ ] Deploy the platform build. Migrations run on start; watch the log for migration errors and for `ERROR`.
- [ ] Do NOT update any SDK or app in the same window.
- [ ] Re-run the section 2 queries and compare.

## 4. Verify after deploy (in this order, stop on the first failure)

1. [ ] Platform health endpoint and the console loads.
2. [ ] Log in as yourself (password, then 2FA).
3. [ ] SSO login through each IdP you use (the fc_sso binding cookie: a failure shows `LOGIN_NOT_STARTED_HERE`).
4. [ ] One login per app type; make one authenticated call in each app.
5. [ ] Mint or use a service-account token; call an API with it; confirm each SDK/app that validates tokens still accepts it.
6. [ ] A console write and a secret rotation (the confirmation modal appears).
7. [ ] Run one app's definition sync (`--dry-run` first). Expect pinpoint's dispatch-pool step to still fail
       (`platform:messaging:dispatch-pool:sync` is admin-tier by design); that is not new.
8. [ ] Watch dispatch jobs, the router and the logs for 15 to 30 minutes: 400s with `INVALID_ROLE_NAME`, `ROLE_NOT_FOUND`
       or `APPLICATION_SERVICE_ACCOUNT_REQUIRED` mean an app or SDK is sending something the platform now refuses.
9. [ ] If you use functions: deploy/invoke one function and check a dispatch job is created and delivered.

## 5. Backup and restore plan (fill in for your hosting)

- Backup type and location: ______________________
- How to restore into a new database: ______________________ (time it)
- How to point the platform at the restored database: ______________________
- Who can do this at the time of the window: ______________________

## 6. Rollback decision

- **Migrations are forward-only.** If data is wrong or logins fail and cannot be fixed forward, restore the database
  from the backup and redeploy the previous platform build together. Expect to lose writes made after the backup.
- **Code-only rollback after 077 is not clean (unverified):** the old code treated "no application id" as "platform
  role" in the client-admin assignability checks. After 077 platform roles carry an application id, so the old code
  could mis-handle them (a client admin might be able to assign a platform role). If you must roll the code back
  without a restore, treat client-admin role assignment as unsafe until the new code is back.
- Decide the rollback trigger before the window (for example: any failed login for more than N minutes, or any
  data check regressing).

## 7. After production is stable

- Update SDKs/apps one at a time, separately: rfp, integral, packages_root, then hr (see `docs/sdk-audit-and-python-sdk.md`,
  Part C). The Laravel mirror only has tags up to v0.10.26 (the split workflow is disabled).
- Give the pinpoint deploy a way to sync dispatch pools (a narrow `platform:messaging:dispatch-pool:sync` role, or an
  admin credential for that step). Not part of this window.
