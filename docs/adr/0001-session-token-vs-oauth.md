# ADR-0001: Session tokens are owned by `sessiontoken`, not fosite

**Status**: Accepted (2026-05-27). The fosite half is now moot — see
[Update (2026-05-28)](#update-2026-05-28).
**Date**: 2026-05-27

## Context

The platform issues JWTs in two places with overlapping but distinct
purposes:

1. **OAuth 2.0 access tokens** — returned by `POST /oauth/token`
   (client_credentials, etc.), accepted at `POST /oauth/introspect`,
   revoked at `POST /oauth/revoke`. RFC 6749 / 7009 / 7662 surface for
   first-party machine-to-machine clients and (eventually) OIDC RP
   delegations.
2. **Session-cookie tokens** — minted by `POST /auth/login` (and by
   the OIDC bridge callback after a successful SSO round-trip), set
   as the `fc_session` cookie, validated by the platform's auth
   middleware on every subsequent request.

Originally both paths went through [fosite][fosite]. The OAuth surface
fits fosite cleanly; the session-cookie path didn't. fosite's design
assumes every token flows through a *grant handler* that writes the
token to storage and then validates it on introspection from that
storage. Session cookies are stateless self-contained JWTs minted
outside any grant flow — there's nothing in storage to introspect.

Concretely, we hit:

- `MintSessionToken` had to construct a synthetic `AccessRequest`,
  populate `JWTSession.ExpiresAt` (which is separate from
  `JWTClaims.ExpiresAt`), grant a scope through the request because
  `claims.With()` unconditionally overwrites the JWT's `exp`, `scope`,
  and `aud` fields from the request / session.
- `fosite.IntrospectToken` runs `CoreValidator` first; on tokens that
  aren't in storage it returns `ErrRequestUnauthorized` (wrapped,
  not `ErrUnknownRequest`), which short-circuits the introspection
  loop before any stateless validator can run.
- Adding `OAuth2StatelessJWTIntrospectionFactory` next to
  `OAuth2TokenIntrospectionFactory` didn't help because of the
  short-circuit above.

These weren't bugs in fosite — they were a shape mismatch. The library
is built to be storage-backed; we wanted stateless.

## Decision

Split the JWT layer along the API boundary it actually maps to:

- **`internal/platform/auth/provider`** stays the OAuth surface. It
  composes fosite for `/oauth/*`, owns the OAuth client storage, the
  authorize/token/introspect/revoke endpoints, and the JWT strategy
  fosite signs OAuth-flow tokens with.
- **`internal/platform/auth/sessiontoken`** is a tiny standalone
  package — ~120 LOC built on `golang-jwt/jwt/v5` — that handles
  session-cookie tokens end-to-end (`Mint` + `Validate`). It depends
  on nothing else in the auth subdomain.

Both share the same RSA key pair so signatures verify across the line,
but the *shape* of each token is owned by exactly one place.

## Consequences

### Positive

- Session-cookie mint + validate are 1 LOC each at call sites: the
  package interface is honest about what it does.
- The auth middleware no longer reaches into fosite. `AuthConfig` only
  needs the public-key validator, not a `*fosite.Provider`.
- The `claims.With()` overwrite footgun is gone — `sessiontoken.Mint`
  writes the claims directly to a `jwt.MapClaims` and signs.
- Adding a new claim is editing one struct + two functions; no fosite
  session / extra-claim plumbing.
- `OAuth2StatelessJWTIntrospectionFactory` and the synthetic
  `AccessRequest` are removed from the codebase.

### Negative

- We now own session-cookie security (signature algorithm, claim
  validation, expiry checks). Mitigated by the small surface and the
  test coverage in `sessiontoken_test.go`.
- The two paths must keep their RSA key in sync. They already do —
  both pull it from `provider.SigningKey()`.

### Neutral

- Refresh-token rotation, login backoff, and `iam_login_attempts`
  recording remain deferred. Those gaps belong to the auth surface,
  not this layering call.

## Future

When the OAuth surface gets re-evaluated (see internal discussion
about option 2 — hand-rolling `/oauth/*` entirely and dropping
fosite), the line drawn here is the foothold. `sessiontoken` already
demonstrates the pattern; the OAuth endpoints would each move out one
at a time, with this package as the precedent.

## Update (2026-05-28)

The "option 2" anticipated above happened: every `/oauth/*` endpoint was
hand-rolled and **fosite was removed entirely** — no `ory/fosite` dependency
remains. OAuth/OIDC now lives in `internal/platform/auth/oauthapi`
(token/authorize/introspect/revoke/userinfo + `.well-known` + JWKS), backed
by `auth/authservice` (JWT mint/validate) and `auth/grantstore` (auth-code,
refresh-token, and pending-auth storage in `oauth_oidc_payloads`).

`internal/platform/auth/provider` no longer composes fosite — it now holds
only the principal→`Claims` projection plus the shared session-cookie
`Mint`/`Validate` helpers this ADR introduced. The layering line drawn here
became the seam the OAuth endpoints moved out along, one at a time, exactly
as predicted.

[fosite]: https://github.com/ory/fosite
