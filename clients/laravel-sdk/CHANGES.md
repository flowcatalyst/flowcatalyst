# Changes

## Unreleased

Additive only; nothing removed, renamed or behaviour-changed.

- New `platformConfig()` resource: `get`, `getRaw`, `set`, `delete`, `list`, `listRaw`
  (`/api/config/{app}/{section}/{property}`, `/api/platform-config/{app}`).
- New `emailDomainMappings()` resource: `getByDomain`, `getByDomainRaw`
  (`GET /api/email-domain-mappings/by-domain/{domain}`).
- `principals()->setClientAssociation($id, $clientId, ?$mode)`
  (`PUT /api/principals/{id}/client-association`, anchor-only).
- `clients()->getRaw($id)` and `getByIdentifierRaw($identifier)` return the platform's
  decoded body unchanged (the `Client` DTO defaults a missing `status` to `ACTIVE`).
- `roles()->sync()` failures from the atomic platform sync ("... failed at index N") now
  append the failing role name and the platform's error code/details to the message and
  add `failedIndex`/`failedRole` to the exception context. The original message is kept
  as the prefix, the code is unchanged and the original exception is chained as `previous`.
  Subclasses (e.g. `ValidationException`) are not touched.
- Tests pin `EventTypeDefinition::toArray()['code']` and `AddSchemaVersionRequest` always
  sending `version` (both already correct since 0.10.0; apps on older versions should upgrade
  rather than keep their workarounds).
