# FlowCatalyst security scan — 2026-11-12

Scanned at `main` `109e855f` (the tree moved from `46813879` during the scan; other sessions were editing, so line numbers are as read). Read-only: nothing in the repo was changed, no server was started, no test was run.

## Remediation status (2026-11-12)

Every High finding (H1 to H9) and every Medium finding (M1 to M20) below has a fix on the
branch this report was committed with. Each fix has tests that fail when the fix is
removed. The Low findings are fixed on the same branch, except L22, which is open
(see below), and L5, which was fixed afterwards (see below); L8 was covered by the M3 work.

Where a fix differs from the remediation text below:

- H1: provider-direct sign-in was not restricted to principals that path created (that
  would break `/oauth/authorize?provider=` for ordinary users). Instead the token's email
  domain must be mapped to the same provider, and the mapping's tenant pin is applied.
- H2: a fourth route was closed as well: an API bearer can no longer register a passkey
  or change 2FA on the account.
- H8: verification is the default on the signed entry points whenever a signing secret is
  configured; with no secret configured, firings still run and a warning is logged once.
- H9: fcdev listens on loopback by default (`--bind` / `FC_API_BIND` widens it) and test
  headers are honoured from loopback peers only.
- M3: only the two password-reset buckets had settings; the introspect and revoke limits
  are opt-in (0 = not enforced).
- M4 and the developer-credential rule in M2 ship as warnings; `FC_OIDC_ROLE_SYNC_REQUIRE_ALLOWLIST`
  and `FC_DEVELOPER_CREDENTIAL_SSO_MAX_AGE_DAYS` turn enforcement on.
- M20: chi 5.3.0 deprecates `RealIP` without changing it, so logged remote addresses are
  still client-influenced; the platform's rate limits were never affected.
  Update 2026-11-12: the client address now follows one rule set by
  `FC_HTTP_TRUSTED_PROXY_HOPS` (default 1; 0 = no proxy, header ignored), shared by rate
  limits, login backoff and recorded sign-in attempts, and chi `RealIP` is no longer mounted.
  `RemoteAddr` is never rewritten; `True-Client-IP` and `X-Real-IP` are not read.
- L5: the session cookie is named `__Host-fc_session` whenever it is written with `Secure`
  (on HTTPS), and `fc_session` on plain HTTP, where a browser would refuse a `__Host-` cookie.
  Nobody is signed out by the upgrade. The old name is still read: a token minted after
  the upgrade carries a `host_cookie` claim, and on a process that issues `Secure` cookies
  such a token is refused when it arrives as `fc_session` (it can only be a planted cookie).
  A token without the claim was minted before the upgrade and is accepted under the old
  name until it expires (24 hours by default), which ends the transition with no flag.
  When both cookies are present the `__Host-` one is used and `fc_session` is ignored. One
  function (`middleware.ReadSessionCookie`) reads the cookie and one rule
  (`middleware.SessionCookieAccepted`) decides; `/oauth/authorize` and the push gateway go
  through them. Sign-out expires both names. During a rolling upgrade an instance still on
  the old version does not read the new cookie, so someone who signs in mid-upgrade may be
  asked to sign in again; a rollback signs out the sessions created after the upgrade.
- L16: an `http:` enrolment QR is also accepted when its origin is the configured platform
  URL or is listed in `allowedPlatformOrigins`, so a device can enrol against a development
  platform on a LAN address. iOS has no enrolment code in the repository and is not covered.
- L18: the embedded database's default password is unchanged (developers' applications
  connect with it); a different one is opt-in.
- L21: a server whose MCP listener is bound beyond loopback with no `FC_MCP_AUTH_TOKEN`
  refuses to start, and so does `fcdev mcp --http` (owner ruling 2026-11-12). The fcdev
  MCP account keeps super-admin: no seeded read-only role covers the MCP tools
  (`platform:viewer` lacks `platform:developer:application-openapi:view`).
- L22 (open): an opt-in client-reach field on a function's description spans the runner,
  the ingestor and the SDKs, and the current behaviour matches the written design.
- L25: `install.ps1` was edited but not executed (no PowerShell on the machine that made
  the change), and the cosign check was tested against a stand-in binary, not a real
  signature.

## How to read this

- **Verified** = I re-read the cited code myself after a reviewer reported it.
- **Reviewer-traced** = a reviewer read the full path (route → middleware → handler → repository); I did not re-read it.
- No finding was exercised against a running system. Each is a code-reading result.
- Every remediation is server-side or additive. Where a fix makes a call that succeeds today start failing, that is stated.

## Method

| Step | Tool | Result |
|---|---|---|
| Go dependency CVEs | `govulncheck` on root, `pkg/fcsdk`, `clients/fn-go` | 5 reachable in root, 3 in `pkg/fcsdk` |
| Go static analysis | `gosec` (pinned golangci-lint, no exclusions, non-test code) | 203 hits, all in the seven rule classes `.golangci.yml` excludes with a stated reason; 95 inline suppressions read, all justified |
| npm dependencies | `pnpm audit --prod` on `frontend`, `clients/typescript-sdk`, `clients/field-js` | frontend: 1 critical, 9 high, 7 moderate, 5 low; the two SDKs clean |
| PHP dependencies | `composer audit --locked` on `clients/laravel-sdk` | 14 packages with advisories, but the lockfile is untracked and local |
| Rust, Python, other npm, Java (direct) | OSV database query of third-party packages from lockfiles | see Dependencies |
| Secrets | pattern search of tracked files and all 1,628 commits | no real credentials; `.env` never committed |
| Dynamic SQL | search for formatted/concatenated SQL, each hit traced | identifiers are allow-listed or constants; values are bound |
| CI, Docker, installers | read all 12 workflows, `Dockerfile`, `docker-compose.yml`, installers | see Supply chain |
| Manual review | seven reviewers, one per trust boundary, ~250k lines | findings below |

## Root causes

Most of the High findings come from four patterns:

1. **Anchor tier is treated as authority.** Several gates pass any ANCHOR-tier caller with no permission check, and every application-provisioned service account is ANCHOR tier (`internal/platform/serviceaccount/operations/client_reach.go:23-31`). This contradicts the rule stated at `internal/platform/shared/auth/auth.go:555-563` (scope is reach, roles are authority). → H5.
2. **The application axis is not enforced everywhere.** `CanAccessApplication` exists and most sync routes use it; scheduled-job sync, the function API, and delivery-credential binding do not. → H3, H4, H6, M1.
3. **Authority is re-derived from the database instead of from the presented credential.** Three routes hand a narrowed or stale token the principal's full current authority. → H2.
4. **Email is the federated identity key.** A provider-direct SSO login resolves to whatever principal has the asserted email. → H1.

---

## High

### H1. Provider-direct SSO signs in to an existing account by asserted email
**Verified.** Found independently by two reviewers.

- **Where:** `internal/platform/auth/bridge/login_endpoint.go:303-336` (start accepts `provider_id`), `:452` (`providerDirect`), `:578-584` (binding skipped when the IdP's domain list is empty), `:620` (`FindByEmail`), `:698` (`SessionWriter`); `internal/platform/auth/bridge/oidc.go:132` (only multi-tenant IdPs must have a domain list); portal sink `login_endpoint.go:1139-1197`.
- **Who:** the administrator of a registered OIDC IdP, or any of its users who can set their own `email`/`preferred_username`. Unauthenticated to the platform.
- **Precondition:** an OIDC IdP row that is either (a) single-tenant with zero mapped email domains — legal at create, and the state an IdP is left in when its last domain is moved away — or (b) multi-tenant with a legacy broad issuer pattern, because this branch never applies the mapping's `required_oidc_tenant_id`.
- **What happens:** `GET /auth/oidc/login?provider_id=<idp>`, authenticate at the IdP with `email` set to a platform user's address. The callback finds that principal and sets `fc_session` for it. No password, no 2FA. This includes ANCHOR administrators. The same assertion reaches the portal sink and signs in as an existing portal identity of any client.
- **Check today:** list OIDC identity providers that have no email-domain mapping. If any exists, this is exploitable now by whoever controls that IdP.
- **Fix:**
  - On the platform plane, let a provider-direct login resolve only to a principal that this path provisioned (inert, OIDC, no client; better, record IdP id + `sub` at first login and require a match). Otherwise refuse with the Authorization error the handler already uses.
  - Treat an empty domain list as "no emails", for every IdP type.
  - Apply the matching mapping's `required_oidc_tenant_id` on this branch.
  - No response shape or claim changes.

### H2. Any API bearer can obtain full, un-narrowed, renewable authority — even after deactivation
**Verified.** Found independently by two reviewers.

Three routes, one root cause:

1. `POST /auth/client/switch` (`internal/platform/auth/clientselection/clientselection.go:69-78, 167-212`): loads the principal with no `Active` check and returns `GenerateAccessToken(p)` — all roles, no scope narrowing, all applications, fresh expiry. The bearer path of the authenticator never touches the database, so a deactivated principal can repeat this indefinitely. The `fc_session` JWT itself is accepted as a Bearer here (it has no `aud` and no `token_use`; `internal/platform/auth/sessiontoken/sessiontoken.go:88-90, 198`).
2. Cookie transport (`internal/platform/shared/middleware/middleware.go:224-244`): any API access token sent as `Cookie: fc_session=…` gets the principal's full current roles from the database, ignoring the token's narrowed `scope`/`applications`.
3. `/oauth/authorize` accepts `Authorization: Bearer` as an SSO session (`internal/platform/auth/oauthapi/authorize.go:289-294`, `internal/server/wire_services.go:194-200`). With the public `fcdev-cli` client, which is re-seeded active on every server boot (`internal/platform/seed/seed.go:49`, `fcdev_client.go:23-53`), a bearer turns into a code, then a full-authority access token plus a rotating refresh family.

- **Who:** holder of any `token_use=api` token: an app-scoped OAuth client holding a signed-in admin's narrowed token; anyone with a leaked service-account or developer token; a user or service account deactivated while holding a live token.
- **Impact:** requested-scope narrowing and per-application narrowing are not boundaries. A leaked 1-hour token becomes durable and survives secret rotation; via route 1 it survives deactivation.
- **Fix:**
  - `/auth/client/switch`: refuse inactive principals; require a cookie session (the `WithSessionCookieAuth` marker) or stop returning a token to bearer callers. Reviewer-traced: the SPA is the only consumer and discards the returned token (`frontend/src/api/auth.ts:400-409`).
  - Cookie transport: accept only session-class tokens in `fc_session` (today exactly "no `aud`, no `token_use`").
  - `/oauth/authorize`: apply the same session-class test in `ValidateSession`, or drop the `Authorization` fallback.
  - Optional: have the bearer path consult a cached active flag so deactivation takes effect before `exp`.
  - New failures are 401/403 for the abuse cases only. Bearer callers of `/auth/client/switch` (none known) would be refused.

### H3. Scheduled-job sync has no application confinement: another application's webhook credentials are sent to a caller-chosen URL
**Verified.**

- **Where:** `internal/platform/sdksync/api.go:784-840` (no `requireAppAccess`; the other syncs call it at `:194, :535, :610, :888`); `internal/platform/scheduledjob/operations/sync.go:88-105` (client-only check), `:116` (existing jobs matched by client and code only), `:191-195` (job re-stamped to the caller's application); `internal/platform/scheduledjob/scheduler/dispatcher.go:271-292`.
- **Who:** any application service account. The built-in application-service role holds the sync permission.
- **What happens:** app A's account calls `POST /api/applications/B/scheduled-jobs/sync` with a job whose `targetUrl` is attacker-controlled. On each tick the dispatcher posts there with `Authorization: Bearer <B's webhook token>` and a signature under B's signing secret, over an attacker-chosen payload. Variant: syncing under its own app with a code another application already uses overwrites that job.
- **Fix:** call `requireAppAccess` in the handler and `CanAccessApplication(cmd.ApplicationID)` in `Authorize`. Do not adopt a job whose `application_id` is a different application (answer 409); keep the NULL backfill. Cross-application calls that succeed today get 403/409.

### H4. A subscription, connection or scheduled job can name someone else's signing account
**Verified** (create operations, resolver, sign endpoint). Found independently by two reviewers.

- **Where:** `internal/platform/subscription/operations/create.go:89-91, 116-121` and `update.go:97-101, 129-131`; `internal/platform/connection/operations/create.go:36` (`TODO(wave-3c): validate that ServiceAccountID exists`) and `update.go:86-102`; `internal/platform/scheduledjob/operations/ops.go:80-92, 105`; `internal/server/delivery_creds.go:70-85`; `internal/platform/dispatchjob/api/api.go:163-186` (sign); `internal/platform/dispatchjob/operations/edit_payload.go:72-107`.
- **Who:** a principal with messaging write permissions for its own client (the seeded messaging-admin role has them), or an anchor operator confined to some applications.
- **What happens:** create a subscription with `endpoint` = attacker URL and `serviceAccountId` = any account id. Deliveries carry that account's bearer token and HMAC signature. No endpoint is needed for the signature: edit a job's payload, then call `POST /api/dispatch-jobs/{id}/sign`, which returns a valid signature over the chosen body — a signing oracle. Scheduled-job variant: `applicationId` = any application.
- **Fix:** at create/update, require `serviceAccountId`, `connectionId` and scheduled-job `applicationId` to be within the caller's reach, reusing `connection.ScopeProblem` (already applied on the sync path, `subscription/operations/sync.go:238`) and the ingestion resolver's signer rule (`ingestion/resolver.go:212-241`). As defence in depth, have the credential resolver re-check scope and decline through the existing "delivered unsigned: reason" path. Sweep existing rows whose signer is out of scope. Bodies accepted today would get 400/403.

### H5. Anchor tier alone authorises portal administration and application↔client enablement
**Verified.** Found independently by three reviewers.

- **Portal plane:** `internal/platform/shared/auth/auth.go:1263-1289` — `CanReadPortalUsers` and `CanManagePortalUsers` return nil for any anchor. The portal operations are all `usecaseop.Public`, so this gate is the only check. `POST /api/portal-users` with `returnInviteLink: true` returns a live set-password link (`internal/platform/portalidentity/api/api.go:216-245`).
  - Any application service account can list any tenant's portal identities, mint an invite link for an identity that has no password and take that portal account over, delete and re-create one that has, or create a portal OAuth client under a victim client.
- **Application routes:** only `RequireAnchor` / `IsAnchor()` on `internal/platform/application/api/api.go:218-221` (attach service account), `:253-256` and `:266-269` (enable/disable for client), `:323-327` (provision login client); `internal/platform/client/api/api.go:306-309, 330-333, 349-352`; and the `/ui` twins `internal/ui/pages/clients_page.go:469-474`, `applications_form.go:385-391`.
  - Any application service account, or a read-only anchor user, can enable its application for any client (enablement is the tenant boundary for app-user invites and config reach), disable every application for a tenant, or create an OAuth login client under another application with its own redirect URIs.
  - The permissions already exist and are never checked: `internal/platform/seed/permissions.go:37-38` (`enable-client` / `disable-client`).
- **Fix:** make each gate "permission AND reach", as `fieldClientGate` (`auth.go:1297-1308`) already does. Use the seeded enable/disable permissions; `CanCreateOAuthClients` for provision-login-client; add `CanAccessApplication` for application-scoped callers.
- **Behaviour change to plan for:** portal backends' service accounts probably rely on the anchor pass today. Grant them the portal-administrator role first, and roll out log-then-enforce.

### H6. The function API checks the client but not the application
**Verified.**

- **Where:** `internal/platform/function/operations/create.go:151-160` and the other function operations (client-only `CheckScopeAccess`); token mint `internal/platform/function/control/token.go:87-101`. `CanAccessApplication` appears once in the whole package (`create.go:115`, an unrelated check).
- **Who:** an application-confined principal holding a function role — the CI roles `function-deployer` / `function-publisher` are meant for exactly such accounts.
- **What happens:** create or publish a function under another application, declare `platformToken: true`, promote and invoke it. The platform mints that application's service-account token for the guest. The same gap allows replacing another application's live code, reading its traces and editing its function secrets.
- **Fix:** one `function.CheckReach` = client scope plus `CanAccessApplication`, used at create and every existing check site; confine the list by `ac.Applications`. Users with all-applications access (the default) see no change. A CI account that deploys several applications needs each in its application access.

### H7. TypeScript SDK (Fastify): the OAuth state cookie is a valid session cookie
**Verified.**

- **Where:** `clients/typescript-sdk/src/fastify/plugin.ts:155-160` (session store), `:193-197` (state store, same secret), `:228-258` (principal built from any decrypted payload), `:485-507` (state payload shaped as a session); `crypto.ts:38-40` (AES-GCM, no associated data); `session/cookie-store.ts:42-52`; `guards.ts:28`.
- **Who:** unauthenticated, against any consuming app using the default cookie session store.
- **What happens:** `GET /auth/login` sets `fc_oauth_state`. Resend its value as `fc_session`. It decrypts under the same key, `expiresAt` is ten minutes ahead, and `req.principal` becomes a role-less user. `requireSession`, `requireAuth` and `requireBearer` only test that a principal exists, so every route gated on "is signed in" opens. Role and client checks still deny.
- **Fix:** immediately, drop a session payload whose `principal.id` is `_oauth_state` or whose access token is empty — this rejects nothing legitimate. Properly, bind the cookie purpose into the AEAD as associated data under a `v2.` envelope and keep reading `v1.` for session cookies only. No API change. Needs an SDK release.

### H8. Scheduled-job firings are accepted unsigned by default (Go and TypeScript SDK runners)
**Verified.**

- **Where:** `pkg/fcsdk/scheduledjobs/runner.go:258-263` ("With no secret configured (the default) no verification happens"), `:301-320`; `clients/typescript-sdk/src/runner/scheduled-job-runner.ts:128-155` (no signature option at all). Laravel is fail-closed. Reviewer-traced: Go puts the caller-supplied `instanceId` unescaped into an authenticated platform call (`pkg/fcsdk/client/scheduled_jobs.go:380, 387`).
- **Who:** anyone who can reach the application's scheduled-job URL.
- **What happens:** post a firing envelope; the handler runs with the attacker's payload. `concurrent: true` skips the overlap lock. In Go with `tracksCompletion: true`, the runner then calls the platform with the app's own token to mark an attacker-chosen instance complete.
- **Fix:** `url.PathEscape(instanceID)` in the Go client. Add an additive `RequireSignature()` (Go) and `signingSecret` + `processSigned` (TS), and use them in the examples and READMEs. Making fail-closed the default would reject unsigned firings accepted today — that needs your ruling.

### H9. `fcdev` serves a header-authenticated super-admin API on every network interface
**Verified.**

- **Where:** `internal/server/run.go:210` (`Addr: ":<port>"`); `cmd/fcdev/envcfg.go:31` (`AuthAllowTestHeaders = true`); `internal/platform/shared/middleware/middleware.go:103-106`.
- **Who:** any host that can open TCP 8080 on a developer's machine (same Wi-Fi, LAN or VPN).
- **What happens:** `X-FC-Test-Principal`, `X-FC-Test-Scope: ANCHOR`, `X-FC-Test-Permissions: platform:*:*:*` give full read/write on the dev platform. fcdev also allows loopback and private delivery targets, so the caller can make it send requests into the developer's localhost and LAN.
- **Fix:** default fcdev's API and metrics listeners to `127.0.0.1` with a flag to opt out, or honour the test headers only when the TCP peer is loopback (captured before chi `RealIP` rewrites it). The bind change alters fcdev behaviour for containers and other machines that reach it over the host IP.
- **Related:** fc-server refuses the test headers unless `APP_ENV` is `local` or `test` (`internal/server/envcfg.go:701-709`). An environment literally named `test` passes.

---

## Medium

### M1. `/api/principals/{id}` routes act on SERVICE principals under user-admin permissions
**Verified** (the `allApplications` check and the type acceptance).
- **Where:** `internal/platform/principal/api/api.go:1259-1334` (`:1280`); `internal/platform/principal/operations/assign_application_access.go:59`.
- **What:** a client administrator can set `allApplications: true` on a service account homed at their client — the only bar is the caller's own flag, which is true by default for users — and can deactivate or delete it. This bypasses the service-account gate (anchor + `service-account:update` + application reach).
- **Fix:** apply the service-account write gate for SERVICE targets; refuse non-USER targets in update/activate/deactivate/delete; require anchor to set `allApplications: true`.

### M2. Passkey enrolment and 2FA changes need no re-authentication; passkey sign-in ignores SSO enforcement
**Verified** (register gate; no SSO check anywhere in `internal/platform/webauthn`).
- **Where:** `internal/platform/webauthn/api/api.go:103-145` (register: any AuthContext), `:221-389` (authenticate); the SSO rule lives only on the password path (`internal/platform/auth/login/endpoint.go:527-544`); developer credentials `internal/platform/auth/oauthapi/token.go:616-654`.
- **What:** anyone holding a session or API bearer for a user can bind their own passkey to that account and sign in indefinitely. An SSO-managed user who registered a passkey (or a developer secret) keeps platform access after being disabled at the IdP.
- **Fix:** register passkeys and change factors only from a recent cookie session (a new refusal code the SPA and `/ui` must handle). Apply the SSO-managed rule to passkey registration and sign-in using existing codes. Developer credentials for SSO-managed users need your ruling.

### M3. Password-reset requests are unthrottled; five configured rate-limit buckets are never enforced
**Verified.**
- **Where:** `internal/platform/passwordreset/api/api.go:493-584` (`:558-566`: each request deletes the old token and mints one with the factor-attempt counter at zero). `BucketPasswordResetIP`, `BucketPasswordResetEmail`, `BucketCheckDomainIP`, `BucketOAuthIntrospectIP` and `BucketOAuthRevokeIP` are defined in `internal/platform/shared/ratelimit/ratelimit.go` and referenced nowhere else.
- **What:** a mailbox holder can loop request → 5 TOTP guesses, defeating the "inbox alone cannot reset" rule; anyone can send unlimited reset mail to a user, keep invalidating a pending 72-hour invite, and distinguish real accounts by response time (the SMTP send is inline).
- **Fix:** enforce the two reset buckets (silently drop over the per-email limit so the generic 200 is unchanged); count failed factor proofs per principal; send mail off the request goroutine.

### M4. IdP role sync uses one global role-name table for every IdP
**Verified.**
- **Where:** `internal/platform/auth/bridge/login_endpoint.go:884-966` (`:901-907`); `internal/platform/auth/repository.go:717-721`.
- **What:** a tenant IdP with role sync on and an empty allow-list can put any role name mapped for another IdP in its `roles` claim and have that platform role assigned to its users.
- **Fix:** at sync time drop platform roles unless the login's mapping scope is ANCHOR; require a non-empty allow-list to enable sync on a non-anchor IdP; optionally add an `identity_provider_id` column to the mapping.

### M5. The network guard exempts the platform's own endpoints per host:port, including for customer URLs
**Verified.**
- **Where:** `internal/netguard/netguard.go:167-169, 213-216`; `internal/server/envcfg.go:673-679`; response body readable via `internal/platform/dispatchjob/api/api.go:512-541`.
- **What:** a tenant webhook target may be `http://localhost:<api-port>/…` or the function runner's private address, and up to 64 KiB of the response is returned through the attempts API. With the router co-hosted and unauthenticated, this publishes router messages to arbitrary targets using only tenant credentials.
- **Check:** how production enforces login on `/router`. If it is at the load balancer rather than `FC_ROUTER_AUTH_USER`, this path goes around it.
- **Fix:** stop exempting the dispatch-processing endpoint in the customer-facing policy (only the unguarded router dials it), and use a separate policy instance for platform-written function targets. URLs to those hosts accepted today would get the existing 400.

### M6. External secret references: the allow-list is process-wide
**Verified** (prefix match with no caller, application or client).
- **Where:** `internal/secretref/resolver.go:178-190`; `internal/secretref/write.go:28-43`.
- **Precondition:** `FC_SECRET_REFERENCE_ALLOW` is set (off by default).
- **What:** any application service account (it holds config update and reveal) can point a config secret at any AWS secret under an allowed prefix and reveal it. `HasPrefix` without a segment boundary also admits sibling names.
- **Fix:** allow entries that carry scope (`{app}` / `{client}` placeholder or an application→prefix map), checked at write and resolve; require a segment boundary.

### M7. OIDC discovery runs under a process-wide mutex with no timeout
**Verified** (lock held across `oidc.NewProvider`).
- **Where:** `internal/platform/auth/bridge/oidc.go:145-173`.
- **What:** one IdP whose discovery endpoint stalls blocks every tenant's SSO login, cached providers included, because the lock is taken before the cache lookup.
- **Fix:** per-key singleflight outside the lock; a bounded HTTP client through `oidc.ClientContext`; brief negative caching.

### M8. Reads gated by permission only, with no tenant scope
**Verified** for audit logs; the rest reviewer-traced.
- **Where:** audit logs `internal/platform/audit/api/api.go:75-248`; raw events `/bff/debug/events` (`internal/platform/event/api/api.go:465-504`); raw dispatch jobs (`internal/platform/dispatchjob/api/api.go:447-466`); scheduled-job instances and logs (`internal/platform/scheduledjob/api/api.go:293-322, 351-365`; the BFF twins do check); service-account list and get; platform control-plane events written with `client_id` NULL are visible to every non-anchor event viewer.
- **Precondition:** a client- or partner-tier principal holding a platform read role.
- **Fix:** anchor-gate the audit and debug routes or apply the accessible-clients filter; copy the BFF checks into the two scheduled-job routes; scope the service-account reads; exclude or stamp control-plane events.

### M9. Unauthenticated `GET /api/email-domain-mappings/lookup?domain=`
**Verified.**
- **Where:** `internal/platform/emaildomainmapping/api/api.go:45, 209-221`. The authenticator passes credential-less requests through and the handler checks nothing.
- **What:** returns the IdP id, required tenant id, primary and granted client ids, scope type and 2FA policy for any domain. It also supplies the IdP id H1 needs.
- **Fix:** add `CanReadEmailDomainMappings`. The SPA login uses `/auth/check-domain`, not this route.

### M10. The role ceiling does not cover credential-takeover actions
**Reviewer-traced** (the final sign-in step was not read).
- **Where:** `internal/platform/principal/api/api.go:1153-1171` (reset-password), `:1577-1621` (reset-2fa), `:1471-1487` (developer credential, secret returned).
- **What:** an administrator with `user:update` can reset the password and 2FA of a more privileged principal in reach, or obtain a developer's client secret. Role assignment refuses the equivalent with `ROLE_ABOVE_CALLER`.
- **Fix:** apply `role.RequireRolesWithinCaller` against the target's current roles.

### M11. Unauthenticated caller can block all field sign-ins for a tenant
**Verified** (failure accounting on the unenrolled path).
- **Where:** `internal/platform/fieldauth/session.go:118, 176-243`; `endpoints.go:552-621`.
- **Precondition:** the client allows unenrolled device keys (the default).
- **What:** 300 bad logins an hour with random worker codes fill the per-client failure bucket; every login, PIN change and act-as for that client then answers 429, enrolled devices included.
- **Fix:** count unenrolled-key failures in a separate bucket that throttles only unenrolled sign-ins.

### M12. `field-worker:manage` can sign in as a real worker by toggling `isTest`
**Verified.**
- **Where:** `internal/platform/fieldidentity/operations.go:415-417`; `api/badges.go:198-204`; `impersonation_ops.go:190-287`.
- **What:** set `isTest: true`, mint a sign-in code (allowed with manage for test identities), optionally flip it back, redeem. This bypasses the `impersonate` permission and the human-only rule, silently for the worker (it is audited).
- **Fix:** require `impersonate` and a USER principal to set `isTest` on an existing worker; record the rule a code was minted under and refuse redemption if the worker is no longer a test identity.

### M13. SPA Documentation page keeps forms and styles from tenant-authored Markdown
**Verified** (DOMPurify defaults, SPA CSP, `/ui` CSRF rule). Not exercised in a browser.
- **Where:** `frontend/src/pages/platform/DocsPage.vue:74-75` (`DOMPurify.sanitize(raw)` with no config); `frontend/handler.go:108` (no `form-action`); `internal/ui/guard/guard.go:70-87` (same-origin posts accepted).
- **What:** an application's synced docs page can carry a `<style>` that stretches a `<button>` over the page and a `<form method=post action="/ui/users/{id}/roles">`. One click by an admin viewing the page in the SPA runs that `/ui` action with their session.
- **Scope:** the SPA only. The default `/ui` console sanitises with bluemonday's UGC policy, which drops forms and styles (checked in the framework checkout). The SPA is served with `FC_CONSOLE=spa`, and may still be reachable otherwise (L12).
- **Fix:** give DOMPurify a Markdown profile (`FORBID_TAGS` style/form/input/button/select/textarea, `FORBID_ATTR` style/action/formaction/form); add `form-action 'self'` to the SPA CSP.

### M14. `/swagger-ui` runs an unpinned third-party script on the platform origin
**Verified.**
- **Where:** `internal/server/http.go:20-32`; `internal/server/wire_spec.go:47-50`.
- **What:** `unpkg.com/swagger-ui-dist` at floating latest, no integrity attribute, no CSP on the response. A package or CDN compromise runs script same-origin with an admin's session ambient.
- **Fix:** serve the bundle from embedded assets (it is already an SPA dependency) or pin a version with `integrity`. The URL stays the same.

### M15. TypeScript and Laravel SDK bearer validators accept OIDC ID tokens as API credentials
**Verified** for TypeScript; Laravel reviewer-traced.
- **Where:** `clients/typescript-sdk/src/fastify/oidc/discovery.ts:32-37, 122-127`; `clients/laravel-sdk/src/Auth/Support/AccessTokenValidator.php:61-82`. Audience is checked only when configured; `token_use` absent is accepted.
- **What:** another relying party on the same issuer can present a victim's ID token (which carries roles, clients and tier) as a bearer to an app using default SDK configuration.
- **Fix:** refuse a marker-less token that looks like an ID token (`nonce` or `auth_time` present, or `azp === aud`), or require `aud` to be the issuer, the configured audience or legacy `flowcatalyst`. Add an opt-in for anyone doing this deliberately.

### M16. Go SDK webhook verifier does not fail closed on an empty secret
**Verified.**
- **Where:** `pkg/fcsdk/webhook/webhook.go:34-36, 71`; `validator.go:74-84`. TypeScript, Java, Rust and Laravel refuse an empty secret.
- **What:** an app that builds the verifier from an unset env var accepts any request signed with the empty key.
- **Fix:** return the existing `ErrMissingSecret` when the secret is empty.

### M17. TypeScript SDK (Fastify): open redirect after login via `returnTo`
**Verified** (the sanitiser).
- **Where:** `clients/typescript-sdk/src/fastify/plugin.ts:468-480`. `/\evil.example` and `/<tab>/evil.example` pass the "starts with `/`, not `//`" test.
- **Fix:** reject backslashes and control characters, and require `new URL(v, "http://x").origin === "http://x"`. Apply the same to logout `redirectTo`, with an allow-list option for absolute targets.

### M18. Stored ciphertexts are not context-bound, and caller-supplied `encrypted:` values are stored as-is
**Reviewer-traced.**
- **Where:** `internal/platform/shared/encryption/encryption.go:208-274`; `internal/platform/serviceaccount/repository.go:159-191`; `internal/platform/serviceaccount/operations/update.go:88-90`; `internal/platform/identityprovider/api/api.go:47-63`.
- **What:** someone with database ciphertexts and service-account or IdP admin rights, but not the app key, can paste a ciphertext into a webhook token or IdP client-secret field and have the platform decrypt it and send the plaintext to a URL they control. Platform-config secrets are not affected (they are sealed with context).
- **Fix:** refuse `encrypted:` on API and UI input for these fields; longer term move them to the context-bound form.

### M19. Sessions and access tokens are never invalidated server-side
**Reviewer-traced.**
- **Where:** `internal/platform/auth/login/endpoint.go:673-679`; `change_password.go:89-104`; `internal/platform/auth/provider/provider.go:286-308`.
- **What:** a captured 24-hour session JWT keeps working after logout, password change or reset, and 2FA reset. Only deactivating the principal ends it.
- **Fix:** a per-principal `sessions_valid_after` timestamp compared with the cookie's `iat`. The cookie path already loads the principal on every request, so no extra query and no wire change.

### M20. Dependencies with known vulnerabilities
See the Dependencies section.

---

## Low

| # | Finding | Where | Fix |
|---|---|---|---|
| L1 | Per-client OAuth throttles are charged to an unvalidated `client_id`; one IP can 429 a client | `internal/platform/auth/oauthapi/authorize.go:61-65`, `token.go:270-281` | count after the client resolves, or key on (client, IP) |
| L2 | `/auth/login` has no per-IP limiter ahead of a 64 MiB Argon2 verify; per-email ceiling is a remote lockout | `internal/platform/auth/login/endpoint.go:546-559` | per-IP governor on `/auth/*`; semaphore around Argon2 |
| L3 | Email-PIN enrolment via enroll token can be brute-forced (no resend interval, no backoff) | `internal/platform/mfa/service.go:227-241, 444-489` | apply resend interval and backoff on enrol |
| L4 | Passkey sign-in reveals which emails have passkeys (random decoy ids) | `internal/platform/webauthn/api/api.go:239-260, 488-507` | derive the decoy id from an HMAC of the email |
| L5 | `fc_session` has no `__Host-` prefix (suspected; depends on sibling subdomains) | `internal/platform/shared/middleware/cookie.go:86-92` | set and accept both names. Fixed: `__Host-fc_session` on HTTPS, old name read for tokens minted before the change (see the L5 note above) |
| L6 | `POST /api/principals` skips the scope rules `POST /api/principals/users` enforces | `internal/platform/principal/api/api.go:468-494` | same checks when scope is not CLIENT |
| L7 | Unauthenticated client-identifier oracle on field admin routes (404 vs 401) | `internal/platform/fieldidentity/api/api.go:371-377` and siblings | authenticate before resolving the client |
| L8 | Portal forgot-password is silent in body but not in time (inline SMTP) | `internal/platform/portalauth/endpoints.go:345-354` | send in the background |
| L9 | Scheduled-job dispatcher follows redirects; signature headers are re-sent on 307/308 | `internal/platform/scheduledjob/scheduler/cron.go:117-121` | `CheckRedirect: http.ErrUseLastResponse` (a 3xx→2xx target would then fail) |
| L10 | HTML injection into the "new device remembered" email via User-Agent | `internal/platform/notify/notify.go:127-133` | `html.EscapeString(label)` |
| L11 | A service-account credential that fails to decrypt is sent as if it were the plaintext | `internal/platform/serviceaccount/repository.go:179-191` | return no credential; use the existing "delivered unsigned" path |
| L12 | The SPA shell is still served for GET on POST-only `/ui` routes (suspected; one `curl -i /ui/logout` confirms) | `internal/ui/ui.go:242, 363`, `internal/consoleredirect/consoleredirect.go:72-74` | explicit 405 on the `/ui` sub-router |
| L13 | Secret reveal is a cookie-authenticated GET reachable by cross-site navigation (forged audit rows) | `internal/platform/platformconfig/api/api.go:246-271` | require `Sec-Fetch-Site: same-origin` for cookie callers |
| L14 | Unbounded JWKS refetch on unknown `kid` before signature check (Laravel, Rust SDKs) | `clients/laravel-sdk/src/Auth/Support/JwtSignature.php:51-59`, `clients/rust-sdk/src/auth/jwks.rs:249-255` | one forced refetch per issuer per minute |
| L15 | Laravel default OIDC handler keeps the pre-login session id; logout reachable by GET | `clients/laravel-sdk/src/Auth/DefaultOidcUserHandler.php:59-78` | `session()->regenerate()` in the callback; config key for logout methods |
| L16 | Field libraries: an enrolment QR silently replaces the configured platform URL; `http://` accepted | `clients/field-js/src/auth.ts:486-490`, `clients/field-android/.../FieldSession.kt:244-247` | require the QR origin to match the configured one |
| L17 | Laravel `OidcTokenManager` caches the client-credentials token under a fixed key (suspected impact) | `clients/laravel-sdk/src/Client/Auth/OidcTokenManager.php:49` | add a hash of base URL and client id to the key |
| L18 | fcdev embedded Postgres runs as superuser `postgres`/`postgres` | `cmd/fcdev/embedded.go:42-43` | random password for new clusters |
| L19 | fcdev runs an ingestor jar found by walking up from the working directory | `cmd/fcdev/ingestor_jar.go:112-138` | accept only user-owned, non-writable-by-others paths |
| L20 | fcdev pointed at an external database seeds `admin@flowcatalyst.local` with the fixed dev password when no anchor exists | `cmd/fcdev/start.go:155-208` | refuse or warn on a non-embedded database |
| L21 | MCP HTTP endpoint has no caller authentication (loopback by default; fcdev's account is super-admin) | `internal/mcp/server.go:70-72` | refuse non-loopback bind without a bearer; read-only role |
| L22 | `auth: platform` function endpoints accept any tenant's API token (matches the written design) | `internal/functions/runner/invoke.go:141-202` | opt-in `reach: "client"` describe field |
| L23 | Go function runner does not mask declared secret values in trace records (the JVM host does) | `internal/functions/runner/trace.go:346-421` | mask declared secrets |
| L24 | `dump.rdb` is tracked (a stray Redis dump holding one principal id) | repo root, added `6a747a48` | remove and ignore |
| L25 | Installers and `fcdev upgrade`: checksum comes from the same origin, a missing sidecar only warns, no signature check | `install.sh`, `install.ps1`, `cmd/fcdev/upgrade.go:115-126` | fail without a checksum; verify the cosign signature where present |

---

## Dependencies

| Component | Package | Installed → fixed | Note |
|---|---|---|---|
| Platform (Go) | Go standard library `net/http` | 1.27.3 → 1.27.4 | HTTP/1 server crash (GO-2026-6859); reachable. `Dockerfile` uses `golang:1.27-alpine3.24`, so a rebuild picks it up |
| Platform (Go) | `github.com/go-chi/chi/v5` | 5.2.5 → 5.3.0 | `RealIP` trusts client `X-Forwarded-For` (GO-2026-5774/5775/5777). Already noted at `internal/server/run.go:110-116`; platform rate limits use the rightmost hop, but logged and audited addresses can be spoofed |
| Platform (Go) | `github.com/klauspost/compress` | 1.18.6 → 1.20.2 | panic on crafted frames (GO-2026-6865); reachable |
| Platform (Go) | `github.com/andybalholm/brotli` | 1.2.1 → 1.2.7 | memory exhaustion on decompress; imported, not called |
| Go SDK | `golang.org/x/text` | 0.37.0 → 0.41.0 | two advisories, reachable |
| Frontend | `mermaid` | 11.15.0 → 11.16.1 | CSS injection, prototype pollution, DoS. Renders tenant-authored docs and process diagrams. Within the existing `^11.4.0` range |
| Frontend | `dompurify` | 3.4.14 → 3.4.17 | the critical advisory needs `SAFE_FOR_TEMPLATES`, which is not used here; bump anyway. Within `^3.4.14` |
| Frontend | `zod`, `fflate` | → 4.6.6, 0.8.3 | client-side DoS only |
| Frontend | `postcss`, `nanoid`, `source-map-js`, `@vue/server-renderer` | — | build-time or unused at runtime in a client-only SPA |
| Java SDK | `jackson-databind` | 3.1.5 → 3.1.7 | three high DoS advisories. Published to consumers. The ingestor is on 3.2.3 (fixed) |
| Rust SDK | `rustls-webpki`, `webpki-roots`, `rustls`, `tokio` | → 0.103.16, 1.0.10, 0.23.46, 1.53.3 | certificate-verification and distrusted-root advisories; `cargo update` |
| Python SDK | `certifi` | 2026.7.22 → 2026.10.18 | distrusted roots; refresh `uv.lock` |
| Laravel SDK | several (guzzle, laravel/framework, symfony/*) | — | the lockfile is untracked and local; consumers resolve their own. Consider raising lower bounds |

Not scanned: the ingestor's full transitive Maven tree (offline resolution failed; only its direct coordinates were checked, clean), Cargo and Gradle trees of the examples and field-android.

## Supply chain and CI

- All actions are pinned by mutable tag (`@v4`, `@v3`), not commit SHA, including third-party actions in jobs with `contents: write`, `packages: write` or `id-token: write`.
- Four workflows interpolate `${{ github.event.inputs.tag }}` / `${{ github.ref_name }}` straight into `run:` scripts (`release-fcdev.yml:39`, `release-field-android.yml:32`, `split-laravel-sdk.yml:48-52`, `split-typescript-sdk.yml:59-62`). The others pass them through `env:`. Exploiting it needs tag-push or dispatch rights.
- `ci.yml`, `go-sdk.yml`, `split-laravel-sdk.yml` and `split-typescript-sdk.yml` have no `permissions:` block.
- The cosign verification command in the fcdev release notes names `flowcatalyst/flowcatalyst-go` (`release-fcdev.yml`, release notes block); the remote is `flowcatalyst/flowcatalyst`. Not tested: if the certificate identity carries the current repository name, the documented command fails.
- `Dockerfile`: non-root runtime user, multi-stage, tag-pinned (not digest-pinned) base images.
- `docker-compose.yml` (example stack): default database password and Postgres/Redis published on all interfaces.

## Checked and found sound

- No real credentials in tracked files or history. JWTs and one RSA key in `api/field-fixtures`, `tools/field-fixtures` and the SDK test fixtures are published test vectors.
- No `InsecureSkipVerify`; no weak hashing or non-cryptographic randomness in a security role.
- Authorization-code flow: redirect URI matching, single-use codes bound to client, redirect URI and PKCE.
- Token endpoint: constant-time client-secret comparison, refresh rotation with reuse detection.
- JWT verification: RS256 pinned, issuer and audience checked, `token_use` allow-list on the API middleware.
- Password hashing (Argon2id), TOTP replay protection, recovery codes, remembered-device tokens.
- CSRF: `/ui` and cookie-authenticated `/api` and `/bff` mutations are guarded; login CSRF guard on credential routes.
- `/ui` XSS: no user data reaches `templ.Raw` or a Datastar expression unescaped; `/ui` authorization matches `/api` on the 16 action families compared.
- Dispatch callbacks: per-job HMAC token, constant-time, fail-closed without an app key; the caller supplies only the message id.
- Field plane: DPoP proof validation, PIN storage, per-worker lockout, enrolment codes, refresh rotation.
- Encryption at rest: AES-256-GCM with random nonces; platform-config secrets are context-bound and masked; reveal is audited.
- Debug and pprof endpoints: off by default, always authenticated, cross-origin guarded.
- Function artifacts: digest-verified at upload, download and cache read; no archive extraction.
- Java ingestor: no native deserialization, safe YAML and XML parsing, digest-pinned function and extension jars.

## Not covered

- Anything requiring a running system: timing measurements, real rate-limit thresholds, load-balancer header behaviour, proxy-level headers (HSTS).
- Deployed configuration and data: which IdP rows have no domains (H1), which roles the portal backends' service accounts hold (H5), how `/router` is protected in production (M5).
- Code outside this repo: the push hub (`framework/go/push`) — topic isolation and replay across identities are unanswered; the Java framework; third-party library internals.
- Parts of the tree read only by search: `internal/queue`, most of `internal/stream` and `internal/outbox`, `internal/platform/keyrotation`, the ingestion authoring handlers, several `/ui` pages (identity providers, OAuth clients, roles, audit log, scheduled jobs, functions, ingestion), most of the SPA beyond its HTML sinks, and the generated API clients.
- The other worktrees (`../flowcatalyst-go-*`).
