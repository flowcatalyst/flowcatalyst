# Java SDK 0.0.36 (unreleased)

Changes since `java-sdk/v0.0.35`. Nothing is tagged or released: `make release-java-sdk BUMP=patch`
cuts 0.0.36 when the owner decides.

Sources of truth: `api/openapi.lock.json`, `docs/field-unlock-keys-spec.md`, `docs/wire-contract.md`
("Unlock keys").

## Field-worker unlock keys

Additive. `fieldWorkers().listUnlockKeys(id)` (`FieldUnlockKeyListResponse`) and
`revokeUnlockKey(id, unlockKeyId)` (`FieldUnlockKeyResponse`; on an already revoked key it ends the
sessions the key left); `FieldDeviceResponse.getUnlockKey()` (null when none); `FieldPolicyResponse` and
`PutFieldPolicyRequest` gain `unlockKeysEnabled` (default false) and `unlockKeyAttestation` (`NONE` or
`REQUIRED`); `pinReauthDays` accepts 0 (never) and defaults to 0. They need a platform that has unlock
keys. The vendored spec was refreshed from the platform's current API document, so the generated models
also carry what was added since 0.0.35 (credential publication, payload offload members).

`fieldPolicies().update(clientId, change)` reads the policy, lets `change` alter the request that would
store it unchanged and writes the result; `FieldPoliciesResource.toRequest(policy)` is that request.

A policy PUT replaces the policy. A request that leaves `unlockKeysEnabled` unset now switches unlock keys
off for the client, revokes all its keys and signs those workers out; `put` is unchanged and sends only
what is set. Use `update`.
