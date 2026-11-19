# Java SDK 0.0.34 (unreleased)

Changes since `java-sdk/v0.0.33`. Nothing is tagged or released: `make release-java-sdk BUMP=patch`
cuts 0.0.34 when the owner decides.

Sources of truth: `api/openapi.lock.json`, `docs/wire-contract.md` ("Field identity", the admin
endpoints).

## A field worker can be created without a code

`EnsureFieldWorkerRequest.workerCode` is optional. Without it the platform generates the code (three
letters, then the client policy's `workerCodeDigits` digits, for example `KTM482`) and
`FieldWorkerResponse.getWorkerCode()` carries it. Additive: a request with a `workerCode` is sent and
answered exactly as before.

New `EnsureFieldWorkerRequest.idempotencyKey` (only without a code, at most 100 characters, unique
within the client): a repeat with the same key answers the worker the first call made
(`created` false). `fieldWorkers().ensure(...)` sends such a request as repeatable, so it is retried
after a 502, 503, 504 or a lost connection; a creation without a code and without a key is not resent
and ends with `SdkError.OutcomeUnknown`, because a second send could make a second worker.

## `workerCodeDigits` on the field policy

`PutFieldPolicyRequest.workerCodeDigits` and `FieldPolicyResponse.getWorkerCodeDigits()`: the digits
of a generated worker code, 3 to 6 (default 3). Codes generated afterwards only.

## Tests

`FieldResourcesTest.ensureWorkerWithoutCodeSendsNoCode`, the policy put with `workerCodeDigits`, and
`RetryRuleTest.fieldWorkerEnsureWithoutCodeIsRepeatableOnlyWithAKey`.
