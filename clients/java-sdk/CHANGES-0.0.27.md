# Java SDK 0.0.27 (unreleased)

Changes since `java-sdk/v0.0.26`. Nothing is tagged or released: `make release-java-sdk BUMP=patch`
cuts 0.0.27 when the owner decides.

Sources of truth: `docs/field-native-session-spec.md` (section 5), `docs/wire-contract.md` (Field identity),
`api/field-fixtures` (the `valid_linked` case), `api/openapi.lock.json`.

## Field worker linked platform user

- `FieldPrincipal` gains `linkedPrincipalId`, the platform user the worker is linked to (the
  `linked_principal_id` claim); `""` when the worker is not linked, like `deviceId`. Audit correlation only:
  it grants nothing. The 14- and 16-argument constructors stay (`linkedPrincipalId` is `""`).
- `client.fieldWorkers()` gains `link(id, principalId)` (422 `INVALID_LINK` when the user cannot be linked,
  409 `LINK_EXISTS` when another worker of the client holds the link) and `unlink(id)` (idempotent); both
  return the refreshed worker, whose `linkedPrincipal` is the linked user. `Filters` gains
  `linkedPrincipalId` (the six-argument constructor stays).
- Introspection and worker models carry the new fields (regenerated from `openapi/openapi.json`).
