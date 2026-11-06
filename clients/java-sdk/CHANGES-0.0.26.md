# Java SDK 0.0.26 (unreleased)

Changes since `java-sdk/v0.0.25`. Nothing is tagged or released: `make release-java-sdk BUMP=patch`
cuts 0.0.26 when the owner decides.

Sources of truth: `docs/wire-contract.md` (dispatch jobs), `api/openapi.lock.json`.

## Dispatch jobs

- `CreateDispatchJobDto.withConnectionId(...)` and `withIdempotencyKey(...)` are no longer
  deprecated no-ops: both are sent on the outbox payload and by `DispatchJobsResource` (they are
  members of the same batch item). The connection's service account signs a subscription-less
  job; a repeated key answers the first job's id instead of creating another (within the job's
  application and client, kept 7 days). `validate()` refuses a key over 100 characters or a
  malformed connection id.
