# Java SDK 0.0.25 (unreleased)

Changes since `java-sdk/v0.0.24`. Nothing is tagged or released: `VERSION` and `pom.xml` stay at
0.0.24 in git; `make release-java-sdk BUMP=patch` cuts 0.0.25 when the owner decides.

Sources of truth: `docs/wire-contract.md` (Field identity), `docs/field-worker-identity-design.md`,
`api/openapi.lock.json`, `api/field-fixtures`; reference implementation is the Go SDK's
`pkg/fcsdk/auth/field.go`.

## Field-worker identity

- `io.flowcatalyst.sdk.field.FieldTokenVerifier`: verifies a field access token
  (`Authorization: DPoP <token>`, RS256, `typ fc-field+jwt`, `token_use field`, `aud` = app code,
  device-bound via `cnf.jkt`) and its `DPoP` proof (ES256, RFC 7638 thumbprint, `htm`/`htu`/`ath`,
  `iat` within 5 minutes by default) from `verify(authorization, dpop, method, fullUrl)`. Returns
  a sealed `FieldVerification`: `Valid(FieldPrincipal)`, `Invalid(reason, detail)` (reasons in
  `FieldReason`, identical to the Go SDK's and the shared fixtures'), or `Unavailable` when the
  JWKS cannot be fetched. JDK only: no JOSE library and no new dependency. No web-framework glue.
- Admin resources, on the generated models: `client.fieldWorkers()` (ensure, list, get, update,
  delete, resetPin, unlock, suspend, activate, grantApp, revokeApp, signIns),
  `client.fieldDevices()` (createEnrolmentCode, list, get, suspend, activate, revoke),
  `client.fieldPolicies()` (get, put), `client.fieldTokens().introspect(token)` and the shorthand
  `client.introspectFieldToken(token)`.

Differences from the Go verifier: a token with no `exp` is refused (`invalid_token`); the JWKS is
refetched when a token names an unknown `kid`, at most once a minute (Go refreshes hourly only).

Tests: all 16 shared fixture cases, alg pinning (HS256/none/ES256/RS512 tokens, non-ES256
proofs), JWKS cache/refetch/unavailable/discovery, ES256 raw-to-DER edge cases (leading zeros),
and the admin resources against a local stub. 198 tests in the module.
