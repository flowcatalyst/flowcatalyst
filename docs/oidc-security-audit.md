# OIDC Bridge — Security Audit (2026-05-31)

Scope: the **OIDC bridge** — FlowCatalyst acting as an OIDC *client* of an external
IdP (Entra, Keycloak, Google). Files: `internal/platform/auth/bridge/*`
(`login_endpoint.go`, `oidc.go`, `login_state.go`) + the wiring in
`internal/server/wire.go`.

The platform-as-OIDC-*provider* surface (`/oauth/authorize`, `/oauth/token`,
`jwks.json`, discovery) is **out of scope** here.

---

## 1. Deferred hardening — issuer pattern (layer 1)  ⚠️ ACTION REQUIRED

The multi-tenant flow accepts a token whose `iss` matches the IdP's
`oidc_issuer_pattern` (a regex). For a multi-tenant Entra app the IdP's shared
signing keys sign tokens from **any** tenant, so a broad pattern (e.g.
`^https://login\.microsoftonline\.com/[^/]+/v2\.0$`) lets *any* tenant's token
pass the issuer gate — leaving only layer 2 (the email-domain + `tid` checks) to
catch a foreign tenant.

**Recommendation (to apply later):** set `oidc_issuer_pattern` to the **specific
tenant**, so the tenant is pinned at the issuer level too:

```
^https://login\.microsoftonline\.com/<YOUR-TENANT-GUID>/v2\.0$
```

With all three controls (issuer-pattern + email-domain + `tid`) a token must come
from your tenant, carry your domain's email, and carry your tenant's `tid`.

> Status (2026-11-09): **enforced for new patterns.** Saving an IdP refuses an
> issuer pattern that does not pin the issuer (`ISSUER_PATTERN_NOT_PINNED`:
> it must be anchored `^…$` on every alternative and spell out its issuers —
> no wildcard, character range or repeat; `identityprovider.CheckIssuerPattern`).
> A pinned pattern is a whole-issuer match at sign-in, so another tenant's
> token fails the issuer check. A broad pattern saved before the rule keeps
> working (warned at startup, on first use, and in the console drawer) until
> `FC_OIDC_ISSUER_PATTERN_REQUIRE_PINNED=true`, which refuses its sign-ins.
> Existing rows still need the data change above.

---

## 2. Findings

Severity: **H** breaks auth or allows cross-tenant/forged login · **M** weakens a
control · **L** robustness/parity.

### Fixed in this pass

| # | Sev | Finding | Fix | Commit |
|---|-----|---------|-----|--------|
| 1 | H | **OIDC resolved from the wrong table** (`tnt_client_auth_configs`) → every configured domain returned `OIDC_NOT_CONFIGURED`. The config lives in email-domain-mapping → identity-provider. | Resolve via mapping → IdP. | `94b8ecf` |
| 2 | H | **PKCE verifier never sent on token exchange** — the login redirect sends a `code_challenge`, so Entra rejects the exchange (`code exchange failed`). Also defeated PKCE's interception protection. | Send `code_verifier` on `Exchange`. | `eb71cb1` |
| 3 | H | **Multi-tenant tokens rejected** — go-oidc's default verifier checks `iss`/`aud` which are tenant-specific. | `InsecureIssuerURLContext` + `SkipIssuerCheck`/`SkipClientIDCheck` + post-verify issuer-pattern regex. | `f6bbf7e` |
| 4 | H | **No tenant binding** — the callback trusted the email claim with no tenant/domain check, so a token from any tenant passing signature+issuer could auto-provision & log in. | (a) token email domain must equal the login domain; (b) explicit `tid` must equal `required_oidc_tenant_id` when set; (c) reject `#EXT#` guest accounts. | `58a42c8`, `eb71cb1` |
| 5 | M | **State replay / race** — non-atomic `FindByState` + best-effort `Delete` only on the success path; the row survived every error path. | Atomic single-use `Consume` (`DELETE … WHERE expires_at > NOW() RETURNING …`), consumed up-front. | `67b1134` |
| 6 | M | **Open redirect** via `return_url` (attacker-controllable, redirected verbatim post-login). | Sanitise to a same-site relative path (reject absolute, `//host`, `/\`). | `67b1134` |
| 7 | L | Login failed when the IdP omitted the `email` claim. | Fall back to `preferred_username`. | `eb71cb1` |
| 8 | H | A stale `fc_session` cookie hard-401'd the public login routes. | Treat a bad cookie as a graceful logout; only reject explicit bad bearer tokens. | `5892bae` |

### Confirmed correct (no change needed)

- **State / nonce / PKCE generation** — 32-byte CSPRNG, base64url-no-pad, S256 challenge.
- **State TTL** — 10 minutes; periodic purge poller.
- **Nonce** — token `nonce` compared to the stored value.
- **ID-token verification** — signature via JWKS (go-oidc discovery), `exp`, `nbf`;
  `iss`+`aud` for single-tenant; issuer-pattern for multi-tenant.
- **IdP resolution** — domain → `email_domain_mappings` → `iam_identity_providers`.
- **Auto-provisioning** — new-user **scope + primary client** come from the
  EmailDomainMapping, never from the token (no privilege escalation via token edit).
- **IDP role sync** — a role is applied only if it is in `oauth_idp_role_mappings`
  **and** (when set) the provider's `allowed_role_ids`; unmapped roles are
  rejected and logged. The role-mapping table is shared by every provider, so
  the allow-list is what bounds one provider: a provider that serves a client or
  partner domain must have one (see the 2026-11-12 note in §3), and with it a
  compromised IdP cannot inject arbitrary platform roles.
  `source=IDP_SYNC`, so admin-assigned roles are preserved.
- **Session establishment** — real SessionWriter (wire.go) mints the session JWT
  and sets `fc_session` (HttpOnly, `Secure` in prod, SameSite=Lax, Path=/).

### Deliberately strict (kept)

- **EC keys** — go-oidc verifies RSA *and* EC ID tokens.
- **Client-secret decryption failure is fatal** — no fallback to a raw
  (plaintext) value.

### Addressed after the first pass

| Sev | Item | Resolution |
|-----|------|-----------|
| M | **Rate limiting on the OIDC bridge** | Per-IP token-bucket governor (`FC_OIDC_RATE_PER_MIN`=60 / `FC_OIDC_BURST`=30) on `/auth/oidc/*` via `ratelimit.GovernorMiddleware` — blunts auth-code probing / DoS. |
| L | **Empty `return_url`** | Now defaults to `/dashboard` and always redirects, instead of emitting JSON-200. |
| — | **Chained OAuth flow** | NOT a gap — the flow is SPA-mediated, not server-mediated. `/oauth/authorize` stashes the request (`PendingAuth`) and bounces to `/auth/login?oauth=true&…`; the SPA drives login (incl. SSO via `/auth/oidc/login` with the rebuilt authorize URL as `return_url`); the callback redirects back to that relative `/oauth/authorize?…` URL (which passes the open-redirect sanitiser). No server change needed. The `login_state.oauth_*` columns are vestigial (a schema carry-over) and unused in this design. |

### Open / lower-priority (not addressed)

| Sev | Item | Note |
|-----|------|------|
| — | **Dead code** | `LoginStateRepo.FindByState` / `OIDCLoginState.IsExpired` are unused after the `Consume` switch; safe to remove later. |

---

## 3. Net result

The OIDC login flow is, for a multi-tenant IdP, bound on three independent
axes: **issuer pattern → email domain → `tid`**. The high-severity gaps that blocked login (config table, PKCE,
multi-tenant verify) and the cross-tenant / replay / open-redirect security gaps
are closed. The remaining items are lower-severity hardening + one deferred
config change (the tenant-specific issuer pattern, §1).

**2026-11-12 — provider-direct login binds to the IdP's own domains.** Provider-direct
(`provider_id=`) login now requires the IdP to own the token's email domain for every
IdP type (an IdP with no mapped domain is refused with `IDP_NO_EMAIL_DOMAINS`, previously
only multi-tenant ones were), and applies the owning mapping's `required_oidc_tenant_id`
pin to the token's `tid`, so an IdP can no longer assert an email outside its domains.

**2026-11-12 — role sync needs an allow-list for client and partner domains.** The
`oauth_idp_role_mappings` table is global, so an IdP with role sync on and an empty
`allowed_role_ids` could put any role name mapped for another provider in its `roles` claim
and have that platform role assigned to its users. Saving a provider with role sync on, no
allow-list and at least one client- or partner-scope email domain (also mapping such a domain
to, or moving it onto, one) is now refused with `ROLE_ALLOWLIST_REQUIRED`; a provider serving
only anchor domains is unchanged. A provider stored before the rule keeps syncing, is logged
at startup and once at first sign-in, and is flagged in the console drawer. With
`FC_OIDC_ROLE_SYNC_REQUIRE_ALLOWLIST=true` its sign-ins map to no roles (the user's IdP-synced
roles are dropped); turn it on once nothing is flagged.
