# Platform identity: Go vs Rust (review, 2026-10-19)

Read-only review. Go = `flowcatalyst-go` at `b893efd4`; Rust = `flowcatalyst-rust` at `f7b5a67b`. Nothing was modified in
either repo; this file is the only output. Paths are relative to the repo named; `go:` = Go repo, `rs:` = Rust repo
(`crates/...`).

## 0. Verdict in one page

**Go is the better identity implementation today, narrowly, and it is the leader by construction.** Rust is a
line-for-line mirror of Go (its comments cite Go file:line throughout, e.g. `rs: fc-platform-iam/src/auth/password_service.rs:115`),
so almost every behaviour matches; the differences are where one side fixed something the other has not yet.

| Axis | Better | Why, in one line |
|---|---|---|
| Feature completeness | Go (small margin) | Same flow set. Go has F28, F33, F34, F35 that Rust lacks; Rust's PENDING ledger stops at F27. |
| Correctness / security | Mixed, Go ahead on exposure | Rust has one conditionally HIGH defect (redirect-URI wildcard bypass) and a login timing oracle; Go has one MEDIUM-HIGH (non-atomic refresh rotation). Each side is ahead on the other's bug. |
| Auditability for one reader | Go | ~9.2k lines on the critical path vs ~15.5k; no traits/macros/async_trait on the token path; stdlib-heavy. Rust is more disciplined mechanically (lint table, compile-checked SQL, route-auth scanner) but larger. |
| Supply chain | Go | ~28 non-std modules in the identity closure vs 510 crates; Go delegates JOSE/OIDC to maintained libs, Rust hand-rolls OIDC RP verification, TOTP, a JWS verifier and a CSRF/mime port. Rust's `cargo audit` is clean but carries a standing `rsa` Marvin ignore and a vendored OpenSSL build. |
| Operational | Even | Same argon2id parameters, same HMAC client-secret scheme. Rust caches bearer validation (unbounded). |

**What each should adopt.** Rust must adopt: Go's URL-aware redirect matcher, `EqualizeTiming` on the main login,
the F34 token_use allowlist, F28/F33/F35, a 0-second JWT leeway. Go must adopt: Rust's atomic refresh rotation (with
the 10 s replay leeway), the atomic TOTP step claim, the single session-cookie constructor with configurable SameSite,
and the route-auth convention test.

Top findings (ranked, with side):
1. **Rust, HIGH if any client registers a wildcard redirect URI**: `wildcard_matches` is a string matcher; `https://evil%2Ecom/.example.com/callback` matches `https://*.example.com/callback` (reproduced). Go is immune (reproduced).
2. **Go, MEDIUM-HIGH**: refresh-token rotation is not atomic; two concurrent presentations both rotate, forking the family and defeating reuse detection. Rust is atomic.
3. **Rust, MEDIUM**: no timing equalisation on the main password login (user-enumeration oracle). Go has it.
4. **Rust, LOW-MEDIUM (latent)**: the F34 class (denylist `token_use == identity`) is present in three places. Go fixed it in F34.
5. **Rust, LOW-MEDIUM**: `jsonwebtoken` default 60 s leeway on every access/session token; Go has 0.
6. **Go, LOW-MEDIUM**: TOTP replay guard is read-then-write (non-atomic); Rust claims the step atomically.
7. **Rust, LOW**: bearer-validation cache never evicts entries that are not re-presented (memory growth).
8. **Go, LOW**: `FC_AUTH_ALLOW_TEST_HEADERS` is an identity-spoof switch compiled into the production binary (default off).

## 1. Completeness and feature parity

Method: the Rust docs (`rs: docs/go-mirror/PENDING.md`, `docs/parity-plan-2026-10-18.md`) already do the spec diff
(309 Go ops vs 296/305 Rust, 28-35 Go-only, mostly function and dispatch routes, not identity). I did not re-run the
OpenAPI diff; I read the identity code on both sides and grepped Rust for each Go fix marker.

| Flow | Go | Rust | Evidence |
|---|---|---|---|
| Authorization code + PKCE (S256 only at authorize) | yes | yes | go: `auth/oauthapi/authorize.go:111-119`; rs: `fc-platform-auth/src/auth/oauth_api.rs:397-427` |
| Client credentials, developer-credential grant, app-scoped SAs | yes | yes | go: `token.go:507-654`; rs: `oauth_api.rs:1742,2043` |
| Refresh rotation + reuse detection | yes, non-atomic | yes, atomic + replay leeway | go: `grantstore/rotate.go:34-86`; rs: `refresh_rotation.rs:98-125` |
| Introspection / revocation | yes | yes (same shape) | go: `oauthapi/introspect_revoke.go`; rs: `oauth_api.rs:2534,2623` |
| Userinfo, discovery, JWKS, previous-key rotation | yes | yes | go: `authservice.go:363,402,687`; rs: `auth_service.rs:1310-1380` |
| Session cookie + CSRF (F16) | yes (stdlib `CrossOriginProtection`) | yes (hand port) | go: `shared/middleware/crossorigin.go:60-140`; rs: `fc-platform/src/shared/session_csrf.rs:1-330` |
| Password hash (argon2id 64 MiB/3/4, argon2i + bcrypt verify, lazy rehash) | yes | yes | go: `passwordhash.go:44-150`; rs: `password_service.rs:210-330` |
| Password policy (NIST: length, 10k blocklist, not own identity) | yes | yes at all write sites (`portal/policy.rs`); the old composition policy survives in `PasswordService::validate_password` | go: `passwordpolicy.go`; rs: `principal/operations/create.rs:220` |
| Login throttling / backoff | yes | yes | go: `loginbackoff.go`, `login/endpoint.go:258,483`; rs: `login_backoff.rs`, `auth_api.rs:303-311` |
| 2FA: TOTP, email PIN, recovery codes, trusted device, MFA step-up token | yes | yes | go: `mfa/*.go`, `mfatoken.go`; rs: `mfa/*.rs` |
| Passkeys / WebAuthn | go-webauthn v0.17.4 | webauthn-rs 0.5.5 | go: `webauthn/api/api.go`; rs: `webauthn/api.rs` (not read in depth) |
| IdP federation (OIDC upstream, multi-tenant, role mapping, domain scope) | go-oidc | hand-rolled on `jsonwebtoken` + own JWKS cache | go: `bridge/oidc.go:150-260`; rs: `oidc_login_api.rs:1214-1410`, `jwks_cache.rs` |
| Invitations, app-managed users, portal identity plane | yes | yes | go: `appusers`, `portalidentity`; rs: `appusers/api.rs`, `portal/*` (not read in depth) |
| Route-level authz | per-handler `Can*`/`Require*` | per-handler + structural scanner test | rs: `fc-platform/tests/it/route_auth_convention_test.rs:1-40` |

**Fixes one side has and the other lacks.**

| Item | Go | Rust |
|---|---|---|
| F28 `SCOPE_REACH_MISMATCH` (SA scope vs reach) | done (`platform-bug-fixes.md` F28) | absent: no match for the code anywhere under `crates/` |
| F33 stale IdP role mapping skipped, not fatal; mapping create checks role exists | done | absent (no "stale mapping" handling found) |
| F34 `token_use` allowlist | done (`sessiontoken.go:86`, `middleware.go:168`) | absent (see R3) |
| F35 per-function webhook service account | done (migration 084) | absent |
| F29, F30, F31, F32 | done | not checked individually; Rust PENDING (`PENDING.md` table) stops at F27, so F28-F35 are untracked there, which the protocol in that file calls a protocol failure |
| F17-F21, F25, F26 | done | done (batches A-D per PENDING) |
| Atomic refresh rotation + 10 s replay leeway | **no** | yes (Rust ahead) |
| Atomic TOTP step claim | **no** | yes (Rust ahead) |
| `EqualizeTiming` on login | yes | **no** (portal login only: `portal/login_api.rs:396,454`) |
| URL-aware redirect matcher | yes | **no** |
| Single session-cookie constructor with `FC_SESSION_COOKIE_SAME_SITE` | no (Lax hard-coded at 8+ sites) | yes (`auth/session_cookie.rs:13-37`) |

## 2. Correctness and security findings

Severity is my judgement of exploitability in this platform's deployment model, not a CVSS score. "Reproduced" means I
ran the logic; otherwise it is by reading.

### Rust

**R1. HIGH (conditional): wildcard redirect-URI matcher is not URL-aware.** `matches_redirect_uri` / `wildcard_matches`
(`rs: fc-platform-auth/src/auth/oauth_api.rs:2252-2318`) treats the URI as a string: split on `*`, require each
wildcard segment to contain no literal `.` and be non-empty. A percent-encoded dot defeats that, and a `/` or `#` can
end the host inside the "segment". I copied the function verbatim into a scratch program and ran it against
`https://*.example.com/callback`:

```
https://evil%2Ecom/.example.com/callback   -> true   (browser host = evil.com)
https://evil%2Ecom#.example.com/callback   -> true   (browser host = evil.com)
https://a.example.com/callback             -> true
https://evil.com/.example.com/callback     -> false
```

`/oauth/authorize` calls it at `oauth_api.rs:355` and sends the user agent (with the authorization code) to the supplied
URI; the OIDC `post_logout_redirect_uri` check reuses it (comment at `oauth_api.rs:2249`). An attacker who can get a
signed-in victim to open a crafted authorize link against any client that registers a wildcard pattern steals the code;
for a public/PKCE client the attacker supplied their own challenge and redeems it. Requires: a client with a `*` pattern
(Go's own matcher doc names `*.inhanceapps.com` as the intended use). Exact-match-only clients are unaffected.
Go is immune: `go: auth/oauthapi/redirecturi.go:30-110` parses with `net/url`, refuses userinfo, compares scheme/port,
confines `*` to the leftmost label of a >=2-label base; I ran `url.Parse` on both payloads and it fails with
`invalid URL escape "%2E"`. Rust's tests (`oauth_api.rs:2667-2760`) cover only dotted/empty segments, not encoded
characters, `/`, `?`, `#`, `@` or backslash.

**R2. MEDIUM: user-enumeration timing oracle on the main password login.** `rs: fc-platform-auth/src/auth/auth_api.rs:348-368`
returns "Invalid credentials" immediately when the principal is missing, inactive or has no hash, and only runs
Argon2id when the account is real. Go closes this: `go: auth/login/endpoint.go:512-523` calls
`passwordhash.EqualizeTiming` (`passwordhash.go:70-96`) on that path. Rust ported `equalize_timing` for the portal login
only (`portal/login_api.rs:396,454`; grep for "equaliz" finds nothing else). Argon2id at 64 MiB/3/4 is tens of ms, easily
measurable. Mitigated, not removed, by the per-(email, IP) backoff.

**R3. LOW-MEDIUM (latent): the F34 class exists in Rust.** Three denylist checks on `token_use == "identity"`:
`rs: fc-platform-iam/src/shared/authorization_service.rs:162` (via `is_identity_only`, `auth_service.rs:395`),
`fc-platform-jwks/src/bearer.rs:230` (function endpoints). Any future token class (the field-worker `token_use: "field"`
in the roadmap) would be accepted as an API credential. `fc-router/src/api/platform_auth.rs:135` is already an
allowlist (`is_api_token`, `bearer.rs:57-59`: exactly `api`), so the router is fine; note it also refuses cookie tokens,
which differs from Go's "api or absent". No token class that would exploit it is issued today, hence latent.

**R4. LOW-MEDIUM: 60 s clock leeway on every JWT.** `Validation::new(...)` in `auth_service.rs:1066-1069,1346-1349`
never sets `leeway`; `jsonwebtoken` defaults to 60 s, and Rust's own test says so ("past default 60s leeway",
`fc-platform/tests/it/oauth_api_tests.rs:79`). A 1-hour access token is honoured for 61 minutes; session cookies and the
upstream-IdP id_token check (`oidc_login_api.rs:1273`) too. Go's `golang-jwt` v5 has none and the repo sets none
(`grep WithLeeway` is empty). Fix: `validation.leeway = 0` (or a deliberate 5-10 s).

**R5. LOW: unbounded bearer cache.** `token_cache: DashMap<String, CachedClaims>` (`auth_service.rs:745`) is keyed by the
full token string; entries are removed only when the same token is presented again after 30 s (`:1312-1340`). Distinct
tokens that are never re-presented (every `client_credentials` mint that is used once) stay forever. Slow memory growth
on a long-lived instance. Go has no cache (RSA verify per request).

**R6. LOW: upstream OIDC verifier edges.** `oidc_login_api.rs:1236-1242`: a kid-less token matches the first JWKS key
(`is_none_or`); RSA only (`:1243-1260`), so an IdP signing ES256/EdDSA fails; `jwks_cache.rs:77-140` refreshes on TTL only
(no refetch on unknown kid), so an IdP key rotation breaks logins until the TTL; `decode_jwt_payload_unsafe`
(`:910,963`) reads the IdP access-token payload unverified into an event. Go's go-oidc `RemoteKeySet` refetches on an
unknown kid and enforces strict kid matching.

**R7. LOW (latent foot-gun): `AuthService::new` silently downgrades to HS256 on a bad RSA key and, with the default empty
`secret_key`, to an empty-secret HS256** (`auth_service.rs:844-863`, `:477`). Production boot avoids it by calling
`new_with_rsa` and failing closed (`fc-platform/src/shared/server_setup/auth_init.rs:171-184`, with the comment that this
mirrors Go). Only tests and `middleware.rs:73` reach `new`. Remove the public fallback.

**R8. LOW: key bootstrap.** With no key configured Rust generates a keypair into `./.jwt-keys/` relative to the CWD
(`auth_service.rs:580-640`); Go generates an ephemeral in-memory key and logs a warning (`go: internal/server/signing_key.go:23-45`).
Neither refuses to start in production; per-instance keys silently break multi-replica deployments in both.

### Go

**G1. MEDIUM-HIGH: refresh-token rotation is not atomic.** `go: auth/grantstore/rotate.go:34-86` does
`FindValidByHash` -> `RevokeByHash` -> `Insert` -> `MarkAsReplaced` as four separate statements. `RevokeByHash`
(`grantstore.go:441-455`) is `UPDATE ... WHERE type=$1 AND payload->>'tokenHash'=$2` with no `consumed_at IS NULL`
guard, and `Rotate` ignores its rows-affected (`rotate.go:62`). Two concurrent presentations of one token both pass the
find, both "revoke", both insert a replacement: two live descendants in one family. A thief and the legitimate client
can then each keep a chain, and the replay that reuse detection exists to catch (`rotate.go:42-47`) never happens.
`MarkAsReplaced` is explicitly best-effort (`rotate.go:82-83`), so a failure there also disables detection for that
token. Rust fixes all of this: one `UPDATE ... WHERE consumed_at IS NULL ... RETURNING` plus the replacement insert in
one transaction (`rs: fc-platform-auth/src/auth/refresh_token_repository.rs:194-235`), a 10 s replay leeway for an
SDK racing itself with the family still intact (`refresh_rotation.rs:98-170`), and reuse revokes the family. This is
the largest correctness gap between the two. Fix in Go: guard the revoke on `consumed_at IS NULL`, check rows-affected,
do revoke + insert + link in one tx; adopt the leeway because the Laravel/TS SDKs refresh per request.

**G2. LOW-MEDIUM: TOTP replay guard is read-then-write.** `go: mfa/service.go:258-285` reads `LastUsedAt`, compares,
then `TouchMethodUsed` (`repository.go:42-46`, unconditional). Two concurrent submissions of the same valid code both
succeed. It is a second factor behind a verified password, hence not higher. Rust: `claim_totp_step` is atomic
(`rs: mfa/service.rs:258-263`).

**G3. LOW: identity-spoof switch in the prod binary.** `AllowTestHeaders` (`go: shared/middleware/middleware.go:102`)
lets `X-FC-Test-Principal` + `X-FC-Test-Permissions` build an AuthContext from request headers; it is controlled by
`FC_AUTH_ALLOW_TEST_HEADERS` (`internal/server/envcfg.go:401`, default false) and, tellingly, also flips
`CookieSecure` (`wire_ui.go:37`). One misconfigured env var is total auth bypass. Rust has no such path outside tests.
Fix: build-tag it out of release binaries or refuse to start when set with a non-loopback bind.

**G4. LOW: error-text leakage to unauthenticated callers.** `go: auth/bridge/login_endpoint.go:497` returns
`"id_token verification failed: "+err.Error()` (go-oidc/jwt internals).

**G5. LOW: cookie construction is duplicated** at 8+ sites with `SameSite=Lax` hard-coded (`login/endpoint.go:596,659`,
`bridge/login_endpoint.go:185`, `passwordreset/api/api.go:907`, `webauthn/api/api.go:377`, `server/wire_routes.go:282`,
...), so the documented `FC_SESSION_COOKIE_SAME_SITE` knob that Rust honours (`rs: session_cookie.rs:13-37`) is not
available and a flag change must be made in many places.

**G6. LOW: thin tests on passkeys.** `webauthn` is 1,299 source lines against 1 test function (44 lines), and
`grantstore`, `clientselection`, `payload`, `webauthn/api`, `webauthn/operations` have no unit tests (`go test` reports
"[no test files]"; `grantstore` has integration-tagged PG tests only).

### Both sides (same finding)

- **B1. Introspect/revoke authorise any caller.** Any valid bearer (an identity token included) or any authenticated
  client may introspect any token and revoke any refresh token it can name; there is no client-ownership check
  (`go: introspect_revoke.go:21-35,100-120`; `rs: oauth_api.rs:892-920,2623`). Needs the token value, so LOW.
- **B2. Upstream OIDC trusts an unverified email claim.** Neither checks `email_verified` (Go `login_endpoint.go:511-537`;
  Rust `oidc_login_api.rs:1325-1331`). Both pin the email domain to the login domain and optional `tid`, which bounds it.
- **B3. Multi-tenant issuer pattern is an unanchored regex** (`go: bridge/oidc.go:~292-300`; `rs: oidc_login_api.rs:1393-1401`):
  safe only if operators anchor with `^...$`.
- **B4. Distinct "Unknown client" / "Invalid client credentials" responses** enumerate client ids (both `authenticateClient`).
- **B5. Client secrets are HMAC-SHA256 under the same app key used for AES-GCM** (`go: shared/encryption/encryption.go:19-21`;
  `rs: encryption_service.rs:26`). Fine for generated high-entropy secrets, key reuse across primitives is a design smell.
- **B6. PKCE is enforced only when the client's `pkce_required` flag is set** (`authorize.go:111`; `oauth_api.rs:397`).
- **B7. Backoff check fails open on a repository error** in the 2FA step (`go: login/twofactor.go:173-174`; `rs: mfa/login_api.rs:590`).
  Go also fails open on the password step (`endpoint.go:258,483`); Rust's password step propagates the error (`?`, `auth_api.rs:308`).

### Checked and found sound (both unless stated)

Algorithm pinned to RS256 on session/access tokens; HS256 MFA step-up tokens are separate and rejected by the RS256
verifier (`mfatoken.go:1-20`; `rs: mfa/token.rs`); issuer and audience checked (access tokens must carry the platform
audience, ID tokens are rejected as bearers, `sessiontoken.go:~128-160`); constant-time compares for PKCE, TOTP,
recovery codes, client secrets, password hash; code single-use via atomic consume (`FindAndConsume`); code bound to the
authenticated client and exact redirect URI; `state` mandatory; Basic client id vs body id conflict refused; inactive
principal refused at code and refresh grants; `no-store` on OAuth errors; argon2id parameters identical.

## 3. Code quality and auditability (single owner-reader)

| Measure | Go | Rust |
|---|---|---|
| Critical-path lines (token/authorize/redirect, authservice, session, grant store, password, backoff, login, 2FA token, OIDC bridge, middleware+CSRF, encryption, mfa) | 9,197 (23 files) | 15,456 (24 files) |
| Largest single file | `oauthapi/token.go` 1,221 | `oauth_api.rs` 3,038; `oidc_login_api.rs` 2,078 |
| Whole identity surface (src / test lines) | auth 13.6k/7.6k, principal 6.7k/5.4k, SA 2.6k/2.2k, idp 1.4k/0.75k | `fc-platform-auth` 21.6k, `fc-platform-iam` 59.8k (includes roles/apps/clients), `-core` 14.7k |
| Layers to follow a token request | chi handler -> `State` -> repos (pgx/sqlc) | axum handler -> `OAuthState` -> repo traits (`RefreshTokenStore`, `TokenAuthenticator` as `dyn`, `async_trait`) -> sqlx macros |
| Hand-ported stdlib behaviour | none (uses `net/http.CrossOriginProtection`, `mime`, `net/url`) | `session_csrf.rs` (952 lines) re-implements `CrossOriginProtection` and `mime.ParseMediaType` |
| Unit/integration tests | identity packages: ~216 (auth) + 136 (principal) + 58 (SA) + 33 (encryption) test funcs; 220 files carry the `integration` build tag, 207 `*_pg_test.go` | 606 `#[test]`-family in the three auth crates; 123 files under `fc-platform/tests/it` (e.g. `auth_security_test.rs`, `two_factor_test.rs`, `oauth_api_tests.rs`, `fcdev_login_test.rs`) |
| Negative / spec-conformance tests | redirect matcher matrix (`redirecturi_test.go`), token scope, PKCE; no conformance suite | RFC TOTP vectors (`mfa/crypto.rs:244`), golden HMAC vector shared with Go (`encryption_service.rs:589`), Go-contract OpenAPI ratchet, route-auth scanner |
| Fuzz | none (`func Fuzz` count 0) | none (no proptest/quickcheck/fuzz in the identity crates) |
| Lint | `go vet` clean on auth, middleware, mfa, webauthn (I ran it). `go test` on those packages passes (cached). `golangci-lint` could not run: the installed binary is built against an older Go than 1.27.2 (export-data error); config is strict (`errorlint`, `exhaustive`, `gosec`, `modernize`) | Workspace lint table denies `unsafe`, warns `unwrap/expect/panic`, `wildcard_enum_match_arm`, `let_underscore_must_use`, `absolute_paths` (`Cargo.toml:55-90`, `clippy.toml`). I did not run clippy or the tests (cost) |

Reading: Go's critical path fits in one sitting and uses well-known libraries, so a security reviewer sees the
decisions. Rust spreads the same decisions over more code, trait objects and macros, but its mechanical guardrails are
stronger (denied `unsafe`, exhaustive enums by lint, compile-time-checked SQL such as the rotation `UPDATE` at
`refresh_token_repository.rs:202`, and a structural scan that every `/api` and `/bff` route authenticates and checks a
permission). Go has no equivalent route-coverage scanner (my grep found none), which matters because its authz is
per-handler.

## 4. Supply chain

Measured with `go list -deps` over the identity packages and `cargo tree --offline -e normal` on the lockfile.

| | Go | Rust |
|---|---|---|
| Closure size | 380 packages incl. std, **28 non-std modules** (incl. huma, chi, redis, pgx used by the same packages) | `fc-platform-auth` 510 unique crate lines, `fc-platform-iam` 457, `fc-platform-jwks` 192; whole lockfile 944 crates |
| JWT/JOSE | `golang-jwt/jwt/v5` 5.3.1, `go-jose/v4` 4.1.4 (via go-oidc) | `jsonwebtoken` 10.3 (rust_crypto backend -> `rsa` 0.9.10); **hand-rolled JWS verifier** in `fc-platform-jwks/src/bearer.rs:100-190` (RS256 PKCS#1 with manual DigestInfo) for functions/router |
| OIDC | `coreos/go-oidc` 3.18.0 (3.21.0 available) | **hand-rolled** RP verification + JWKS cache (`openidconnect` is a workspace dependency but is not used by the auth crates) |
| WebAuthn | `go-webauthn/webauthn` 0.17.4 (0.18.2 available) + `go-webauthn/x`, `fxamacker/cbor`, `google/go-tpm` | `webauthn-rs` 0.5.5 with `danger-allow-state-serialisation` (`Cargo.toml:199`), pulls `openssl` **vendored** (OpenSSL built from source, `fc-platform-auth/Cargo.toml:59`) |
| Password | `x/crypto` argon2 + bcrypt | `argon2` 0.5.3, `bcrypt` 0.19.3 |
| TOTP | `pquerna/otp` 1.5.0 (last release 2024-12) + `boombuler/barcode` pinned to a **2019 pseudo-version** (stale, transitive) | **hand-rolled TOTP** on `hmac`+`sha1` (`mfa/crypto.rs:99-135`, RFC vectors tested) and a hand-rolled QR-to-PNG writer (`crc32fast`, `flate2`, `qrcodegen`) |
| Crypto backends in the tree | stdlib only | RustCrypto + `ring` + `aws-lc-rs` + `openssl` (four backends) |
| Known advisories | not run (`govulncheck` not installed) | `cargo audit`: 0 vulnerabilities, 1 yanked warning (`yoke-derive` 0.8.3); 5 ignores in `deny.toml:32-61`, including **RUSTSEC-2023-0071 (`rsa` Marvin, no fix)**, justified as sign/verify only |
| Hand-rolled crypto | none (HMAC/AES-GCM/SHA via stdlib) | none at the primitive level; protocol-level hand-rolling in OIDC RP, JWS verify, TOTP, mime/CSRF |

Reading: Go's identity path is small and delegates protocol code to maintained libraries (two of them a minor version
behind). Rust's is an order of magnitude larger in crates and owns more protocol code itself; its supply-chain hygiene
(`cargo deny` in CI, documented ignores in `docs/operations/supply-chain.md`) is good, and the single serious standing
advisory (`rsa` Marvin) does not apply to verification, but signing the platform's own JWTs with RS256 does use the
private-key path, which is exactly where Marvin lives; the ignore reasoning ("signing/verifying only") understates that.

## 5. Performance and operations

- **Hashing cost.** Identical: argon2id m=64 MiB, t=3, p=4, 32-byte key, 16-byte salt (`go: passwordhash.go:44-49`;
  `rs: password_service.rs:176-183`). Both rehash lazily on login when parameters or algorithm differ. Neither bounds
  concurrent hashes, so a burst of logins costs 64 MiB each; the per-IP and per-(email, IP) limits are the only brake.
- **OAuth client secrets and service-account tokens** use a keyed HMAC, not argon2 (`encryption.go:281-312`;
  `encryption_service.rs:369-420`), so `client_credentials` is cheap in both.
- **Bearer validation.** Go verifies RSA on every request, no cache; Rust caches validated claims 30 s keyed by token
  string (R5). Session-cookie requests resolve claims from the database on every request in both
  (`go: middleware.go:123-150`; `rs: fc-platform-core/src/shared/middleware.rs:305+`), which is a per-request round trip
  by design (immediate revocation of roles).
- **Key and JWKS caching.** Platform JWKS is built from the in-memory current + previous keys in both; neither selects
  verify key by `kid` (both try current then previous). Rust's upstream-IdP JWKS cache is TTL-only (R6); go-oidc's is
  refetch-on-miss.
- **DB round trips per token request.** Not measured. By reading the authorization-code grant: Go does consume code,
  principal lookup, role/permission flattening, refresh insert (`token.go:699-800`); Rust the same plus the rate-limit
  store call per client (`oauth_api.rs:1208-1400`, `:1010`). No benchmark was run.
- **Rate limits.** Both: per-IP middleware, per-client-id in-memory governor + distributed store on `/oauth/token` and
  `/oauth/authorize`.

## 6. Verdict and prioritised fixes

**Overall: Go.** It has the more recent fix set (F28, F33, F34, F35), the immune redirect matcher, the login timing
equalisation, a far smaller dependency and code surface to audit, and it leans on stdlib/maintained libraries. Rust is
a faithful, well-guarded follower whose strengths are concurrency-correct rotation and TOTP, mechanical lint/route
discipline, and a cleaner cookie/SameSite design. Neither is fuzzed. The two worst defects are on opposite sides, so the
"better" call is about exposure and effort: Go's worst (G1) is a contained SQL/transaction change; Rust's worst (R1)
needs a matcher rewrite and is remotely exploitable wherever wildcards are registered.

### Go: fix in this order
1. **G1** Make refresh rotation atomic (guarded `UPDATE ... WHERE consumed_at IS NULL RETURNING`, revoke + insert + replaced-by link in one tx), check rows affected, add the 10 s replay leeway; port Rust's concurrency test (`rs: tests/it/auth_security_test.rs:721-771`).
2. **G3** Remove the test-header bypass from release builds or fail startup when set on a non-loopback listener.
3. **G2** Make the TOTP step claim a single guarded `UPDATE` (copy Rust `claim_totp_step`).
4. **G5** One `sessioncookie` constructor honouring `FC_SESSION_COOKIE_SAME_SITE`; delete the 8 inline `http.Cookie` literals.
5. **G4** Do not return `err.Error()` from id_token verification to the browser; log it.
6. Add unit tests for `webauthn` (G6), `grantstore` (non-integration), and a route-auth coverage test modelled on Rust's scanner.
7. B1-B6 hygiene: client-scoped revoke/introspect, `email_verified` policy, require anchored issuer patterns, uniform `invalid_client` text, bump `go-webauthn`/`go-oidc`, replace the 2019 `barcode` pin.

### Rust: fix in this order
1. **R1** Replace `wildcard_matches` with Go's `MatchRedirectURI` semantics (parse, no userinfo, scheme/port equality, leftmost-label wildcard, >=2-label base); port Go's `redirecturi_test.go` matrix plus encoded-dot, `/`, `#`, `?`, `@`, backslash cases. Until then, audit production clients for wildcard `redirect_uris`.
2. **R2** Call an `equalize_timing` on the not-found / inactive / no-hash path of `password_login` (`auth_api.rs:353`).
3. **R4** Set `validation.leeway = 0` (or a documented small value) on all `Validation::new` sites.
4. **R3** Switch the three denylists to `token_use in {api, absent}` (Go F34) before the field-worker token class lands.
5. Mirror **F28, F33, F35** and add F28-F35 rows to `PENDING.md`.
6. **R5** Bound or time-evict the bearer cache (e.g. `moka` or a periodic `retain`).
7. **R6** Refetch JWKS on unknown `kid`, require `kid` when more than one key, support ES256; **R7** drop the HS256 fallback from `AuthService::new`.
8. Reconsider hand-rolled OIDC verification vs the already-vendored `openidconnect`; document that RS256 signing is on the `rsa` Marvin path.

## 7. What I did not read or verify

- Did not read in depth: WebAuthn/passkey code on either side, invitations and app-managed users, the portal identity
  plane, service-account and webhook-credential modules, `loginbackoff`/`login_backoff` internals, `authorization_service`
  permission ceilings, password-reset flows, `userinfo` and `discovery` bodies, `clientselection`, and `shared/bff`.
- Did not run: the Rust test suite or clippy (cost), `golangci-lint` (toolchain mismatch with Go 1.27.2), `govulncheck`
  (not installed), the OpenAPI spec diff (relied on Rust's own 2026-10-19 PENDING log), any benchmark. Go `go vet` was run
  clean and `go test` on the auth/middleware/mfa/webauthn packages passed from cache; I ran `cargo audit` against the
  local advisory DB only.
- R1 and the Go counter-case were reproduced with scratch programs (outside both repos). Every other finding is from
  reading the cited lines; R3/R4/R5 and G1/G2 are static conclusions, not exercised.
- F29-F32 parity in Rust was not checked individually.
- Numbers for "critical path" lines are the 23/24 files I chose; a different file set would shift them, not the ratio.
