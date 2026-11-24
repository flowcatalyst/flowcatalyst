# OIDC client credentials: certificate and federated credential

Status: approved 2026-11-23 (owner), being built.

## Why

An external OIDC identity provider (Entra in practice) registered as a
confidential ("Web") client needs proof of the client's identity at the token
endpoint. Until now the only proof the bridge could give was a client secret.
Entra secrets expire after at most 24 months. When one expires, every SSO login
through that provider fails at the code exchange (`AADSTS7000222`).

This adds the two other credentials Entra accepts. Both use RFC 7523
`private_key_jwt` client authentication. The token request carries
`client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer`
and a `client_assertion` JWT where it would otherwise carry `client_secret`.

## Owner rulings (2026-11-23)

1. **Federated credential: the platform is the issuer.** FlowCatalyst signs the
   assertion with its own RS256 signing key, the one already published at
   `/.well-known/jwks.json`. Entra is configured with a federated credential of
   type "Other issuer": issuer = the platform base URL, subject =
   `identity-provider:<idp id>`, audience = `api://AzureADTokenExchange`. No AWS
   STS and no token files.
2. **Certificate: the platform generates the key pair.** The private key never
   leaves the platform. It is stored encrypted with the app key. The admin
   downloads the public certificate and uploads it to Entra. No PEM upload.
3. **UI: the `/ui` console only.** The Vue SPA is unchanged. It keeps working
   for secret-based providers and does not show the new options.

## Model

### Auth method

New column `oauth_identity_providers.oidc_client_auth_method VARCHAR(30) NOT
NULL DEFAULT 'CLIENT_SECRET'`. Its values are:

| Value | Token request carries | Notes |
|---|---|---|
| `CLIENT_SECRET` | `client_secret` (sent in params) when a secret ref is set; nothing when unset, which makes it a public client | Today's behaviour, unchanged |
| `CERTIFICATE` | `client_assertion` signed by the provider's ACTIVE certificate key | Fails if there is no ACTIVE certificate |
| `FEDERATED` | `client_assertion` signed by the platform signing key | Needs a public HTTPS base URL that Entra can reach |

The entity gets `OIDCClientAuthMethod ClientAuthMethod` with a
`ParseClientAuthMethod(s) (ClientAuthMethod, bool)` in the style of `ParseType`.
A read error is loud and never coerced. Create and update accept it as
`oidcClientAuthMethod` and reject unknown values with validation code
`INVALID_CLIENT_AUTH_METHOD`. It is valid only for `type = OIDC`. Switching away
from `CLIENT_SECRET` does **not** clear a stored secret, so switching back works.

The migration only adds the column. The existing rows are a small configuration
table, so `docs/large-table-schema-changes.md` does not apply. Say so in the
migration header the way migration 126 does.

### Certificates

New table `oauth_identity_provider_certificates`:

```
id                    VARCHAR(17) PRIMARY KEY   -- TSID, new prefix e.g. "ipc"
identity_provider_id  VARCHAR(17) NOT NULL      -- no FK (repo convention: docs/foreign-keys.md); delete with the IdP in the same tx
status                VARCHAR(10) NOT NULL      -- ACTIVE | NEXT
private_key_ref       TEXT NOT NULL             -- PKCS#8 PEM, EncryptBound, aad = "oauth_identity_provider_certificates:" || id
certificate_pem       TEXT NOT NULL             -- public X.509, PEM
thumbprint_sha1       VARCHAR(40) NOT NULL      -- hex upper, what Entra's portal shows
thumbprint_sha256     VARCHAR(64) NOT NULL
not_before            TIMESTAMPTZ NOT NULL
not_after             TIMESTAMPTZ NOT NULL
created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
UNIQUE (identity_provider_id, status)           -- at most one ACTIVE and one NEXT
```

Check `docs/foreign-keys.md` for the no-FK rule before writing the migration.
Follow whatever it says.

**Generation:** RSA 3072. The certificate is self-signed, with subject
`CN=FlowCatalyst <idp code>`, a random serial, `not_before = now - 5m` and
`not_after = now + 24 months`. Key usage is digital signature. The code lives in
the identityprovider package or a small subpackage. It must not use the
platform's JWT signing key.

**Key rotation:** register the column in
`internal/platform/keyrotation/reencrypt.go` `Targets()` as a bound target. Copy
how `app_platform_configs.value` is registered, including the AAD. Re-encryption
must keep the AAD.

### Operations (use cases, each one domain operation with its own event)

Each operation is a `usecaseop.TxOperation`, written like the existing
create/update/delete. Authorization is enforced at the controller, like update:
anchor-only, "may write identity providers".

- `GenerateIdentityProviderCertificate {identityProviderId}`. Refuses non-OIDC
  providers. Refuses when there is no encryption service, with code
  `ENCRYPTION_NOT_CONFIGURED`. If there is no ACTIVE certificate, the new one
  becomes ACTIVE. Otherwise it becomes NEXT, replacing any existing NEXT.
  Event `platform:admin:identity-provider:certificate-generated` carries data
  `{identityProviderId, code, certificateId, status, thumbprintSha1, notAfter}`.
- `PromoteIdentityProviderCertificate {identityProviderId}`. NEXT becomes
  ACTIVE and the old ACTIVE row is deleted. Refuses with `NO_NEXT_CERTIFICATE`
  when there is no NEXT. Event `…:certificate-promoted` carries
  `{identityProviderId, code, certificateId, retiredCertificateId}`.
- `DiscardIdentityProviderCertificate {identityProviderId}`. Deletes the NEXT
  certificate. Refuses with `NO_NEXT_CERTIFICATE`. Event
  `…:certificate-discarded`.
- `DeleteIdentityProvider` also deletes the provider's certificates.

Seed a schema for each new event type in
`internal/platform/seed/event_schemas.go`. Schemas are
`additionalProperties:false`, so every field emitted must be listed. Check
whether seeded event types also need a migration for existing databases. Look
at how migration 126 and the seeder handle schemas that are new, as opposed to
changed. Do the same.

`IdentityProviderUpdated` data is unchanged.

## Bridge (`internal/platform/auth/bridge/oidc.go`)

- `resolved` gains a `clientAuth` strategy, built in `buildResolved` from the
  method:
  - `CLIENT_SECRET`: as today.
  - `CERTIFICATE`: loads the ACTIVE certificate through a repository method and
    decrypts the key with `DecryptBound` and the same AAD. An error names the
    provider. It never logs key material.
  - `FEDERATED`: holds a signer, an interface the bridge receives (see wiring).
- For both assertion methods, `oauth2.Config.ClientSecret` is empty and
  `Endpoint.AuthStyle = oauth2.AuthStyleInParams`, so `client_id` is in the form
  and no Basic header is sent. `Exchange` mints a **fresh** assertion per call.
  Never cache a minted assertion. Append
  `oauth2.SetAuthURLParam("client_assertion_type", …)` and `("client_assertion",
  jwt)`.
- **Certificate assertion** (Entra's format, also standard RFC 7523):
  - Header: `alg: RS256`, `typ: JWT`, `x5t: base64url(SHA-1(DER))`,
    `x5t#S256: base64url(SHA-256(DER))`.
  - Claims: `iss = sub = client_id`, `aud = provider.Endpoint().TokenURL`,
    `jti` random, `iat`, `nbf = now`, `exp = now + 5m`.
- **Federated assertion**: the signer produces `iss = platform issuer`
  (`cfg.JWTIssuer`, which equals the discovery document's `issuer`),
  `sub = "identity-provider:" + idp.ID`, `aud = "api://AzureADTokenExchange"`,
  `jti`, `iat`, `nbf`, `exp = now + 5m`, plus `token_use = "client_assertion"`.
  The header carries the current `kid`, so Entra can find the key in our JWKS.
- `cacheKey` adds the auth method and, for `CERTIFICATE`, the ACTIVE
  certificate's id. Promoting a certificate then takes effect on the next login
  without a restart. Keep the cacheKey comment's history and add one sentence.
  `cacheKey` currently takes only the IdP. If adding the certificate id needs a
  lookup, do the lookup in `resolve` before computing the key. Keep `mu` free of
  network calls, as the existing comment requires. A DB read is fine outside the
  lock.
- The "no client secret configured" info log fires only for `CLIENT_SECRET`.
- `NewBridge` signature: add an option setter
  `SetAssertionSigner(s AssertionSigner)` instead of changing the constructor,
  so the existing tests are untouched. When `FEDERATED` is set without a signer,
  fail with a clear error.

### Signer (`internal/platform/auth/authservice`)

Add `func (s *AuthService) SignClientAssertion(sub, aud string, ttl
time.Duration) (string, error)`. It returns an error unless the service signs
RS256, because Entra cannot verify HS256 against a JWKS. It uses `s.sign`, so
the `kid` matches the JWKS. Wire it in `internal/server/wire_routes.go` next to
`bridge.NewBridge`.

**Security requirement (must have tests):** a client assertion must never be
accepted as a platform credential. Add tests showing that
`AuthService.ValidateToken`, the session-token validator and the field-token
validator reject a token minted by `SignClientAssertion`. The audience differs,
and `token_use` gives a second guard. If any validator does not check the
audience, make it reject `token_use = client_assertion` explicitly.

### Boot warning

`bridge.WarnFederatedWithoutPublicIssuer(ctx, repo, issuer)`, in the style of
`WarnBroadIssuerPatterns`. When any provider uses `FEDERATED` and the issuer is
not `https://` or is a loopback or localhost host, log a warning naming each
such provider. It is best-effort and never fails boot.

## `/ui` console (`internal/ui/pages/identity_providers*`)

- The create and edit form gains "Client authentication": Client secret
  (default) / Certificate / Federated credential. The client-secret input shows
  only for Client secret, using Datastar signals as the form already does for
  other conditional fields.
- The detail page gains a "Client credential" section:
  - **Client secret:** what it shows today, i.e. whether a secret is set.
  - **Certificate:** a table of ACTIVE and NEXT certificates with SHA-1
    thumbprint, expiry date and a "Download .cer" link. The link serves
    DER-encoded X.509 with `Content-Disposition: attachment;
    filename="<code>-<thumbprint>.cer"` and needs the same read permission as
    the page. The buttons are "Generate certificate" (when there is none),
    "Generate next" (when ACTIVE exists and there is no NEXT), and "Make active"
    and "Discard" (when NEXT exists). Show a warning when ACTIVE expires within
    60 days. Under the buttons, add a help line: "Upload the certificate in Entra
    → App registrations → your app → Certificates & secrets → Certificates
    before making it active."
  - **Federated credential:** read-only, copyable values for Issuer (the
    platform issuer), Subject (`identity-provider:<id>`) and Audience
    (`api://AzureADTokenExchange`). Add a help line pointing to Entra →
    Certificates & secrets → Federated credentials → Add credential → "Other
    issuer". Show a warning if the issuer is not public HTTPS.
- The certificate buttons call the operations through the console's existing
  ops plumbing (`identity_providers_ops.go`). Follow how that file reaches the
  use cases. No JSON API endpoints are needed for certificates in this round.
  `oidcClientAuthMethod` does go on the existing create/update JSON DTOs and the
  response DTO, so the API and console agree. Then refresh the OpenAPI lock
  (`api/openapi.lock.json`) with the repo's make target. Run `make ui-generate`
  after editing `.templ` files.

## Docs

Add a short section to `docs/published/30-identity-and-access.md` covering the
three methods. Include the Entra steps for each, and say that secrets expire
while federated credentials do not.

## Tests

- Unit tests for certificate generation: parses, thumbprints match, validity
  window. Unit tests for the certificate assertion's header and claims.
- Bridge test with an httptest token endpoint. For each method, assert the form
  the endpoint receives: `client_secret` vs `client_assertion` +
  `client_assertion_type`, no Basic header for assertion methods, and an
  assertion that verifies against the cert or JWKS public key with the right
  claims.
- `cacheKey` test: changing the method changes the key, and promoting a
  certificate changes the key.
- Integration tests (`-tags=integration`, `*_pg_test.go`) for the
  generate/promote/discard operations and the delete cascade. They must also
  check that the private key ref decrypts only with its own AAD.
- The client-assertion-rejection tests above.
- Run `make sqlc`, `make ui-generate`, `go build ./...`, `go vet ./...`, `go
  test ./...`, then `go test -tags=integration` on the touched packages.

## Out of scope

- The Vue SPA.
- AWS STS / token-file federation sources.
- Uploading an externally issued certificate.
- Client-auth-config (`tnt_client_auth_configs`) OIDC secrets.
- A "test credential" button that does a client_credentials grant. Possible
  follow-up.
