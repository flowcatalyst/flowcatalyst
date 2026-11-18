# Java SDK 0.0.32 (unreleased)

Changes since `java-sdk/v0.0.31`. Nothing is tagged or released: `make release-java-sdk BUMP=patch`
cuts 0.0.32 when the owner decides.

Sources of truth: `api/openapi.lock.json`, `docs/wire-contract.md` ("Application service-account
provisioning").

## Generated models (regenerated from `openapi/openapi.json`)

- `ApplicationServiceAccountCredentials` gains `webhook` (`ServiceAccountWebhookSecrets`: `authToken`,
  `signingSecret`), so `client.applications().provisionServiceAccount(id)` now returns the provisioned
  account's webhook credentials, shown once, beside its OAuth client. Null from a platform that predates
  them. The method's return type (`ApplicationProvisionServiceAccountResponse`) is unchanged.
- `BatchEventItem` gains the optional `time` (when the event occurred; absent means the ingest time).
- New models for the app-updates API (`AppUpdateResponse`, `PublishUpdateRequest`, `RollbackRequest`,
  `SetRolloutRequest` and the rest); the SDK has no hand-written resource for it.
- Removed: the models `RawEventResponse` and `RawDispatchJobResponse`. No operation used them (they
  belonged to routes the spec does not publish); code that named them no longer compiles.
