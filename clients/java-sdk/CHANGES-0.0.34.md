# Java SDK 0.0.34 (unreleased)

Changes since `java-sdk/v0.0.33`. Nothing is tagged or released: `make release-java-sdk BUMP=patch`
cuts 0.0.34 when the owner decides.

Sources of truth: `api/openapi.lock.json`, `docs/wire-contract.md` ("Field identity", the admin
endpoints; Authentication, "Token spelling"), the shared vectors `api/field-fixtures/noncanonical.json`.

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

## Generated models: app binaries and install links

The models generated from the platform spec now include those of the app binaries and install links
API (`AppBinaryResponse`, `AppBinaryFactsResponse`, `AppInstallLinkResponse`, `PublishBinaryRequest`,
`CreateInstallLinkRequest` and the rest), `requireBinary` on `PublishUpdateRequest` and
`binariesOnBranch` on `AppUpdateResponse`. Additive; there is no hand-written resource for these
routes.

## A field token or proof verifies only in the spelling it was issued in

Behaviour change. `FieldTokenVerifier` decoded the token and the DPoP proof with
`Base64.getUrlDecoder()`, which accepts `=` padding and ignores the spare low bits of a segment's
last character, and it trimmed the proof. So one token had several spellings that all verified: the
15 other last characters of a 2048-bit RS256 signature, the same for an ES256 proof signature and 3
for the proof key's `x` or `y`, any segment padded, and a proof with whitespace around it. Now every
segment of the token and the proof, and the key's `x` and `y`, must be canonical base64url (the
URL-safe alphabet, no padding, no whitespace, spare bits zero), checked on the text before decoding
(`CompactJws`): a re-spelled token is `Invalid(malformed)`, a re-spelled proof
`Invalid(invalid_proof)`. The standard alphabet and line breaks were already refused. Whitespace
around the token in the `Authorization` header is still trimmed (it is the header's framing).

What stops working: only a token or proof someone re-spelled after it was issued, or a proof from a
signer of your own with a non-standard base64url encoder. Nothing the platform issues is affected:
its tokens and the field libraries' proofs are written by standard encoders and are always
canonical, and the shared vectors in `api/field-fixtures/cases.json`, minted before the change, all
still verify. `FieldTokenVerifierTest` and the new `CanonicalSpellingTest` run both files.

## Tests

`FieldResourcesTest.ensureWorkerWithoutCodeSendsNoCode`, the policy put with `workerCodeDigits`, and
`RetryRuleTest.fieldWorkerEnsureWithoutCodeIsRepeatableOnlyWithAKey`.
