# Java hand-off: external secret references (`aws-sm://`, `aws-ps://`)

Go implements it in the commit that adds this file (2026-11-04). The spec is
`docs/external-secret-references.md` (owner-approved 2026-11-04: resolved by the platform, at read time);
this is the checklist for the Java platform. The Java ingestor needs no change (see the last section).

## What it is

A secret value may be a reference to AWS instead of the secret. The platform stores the reference, never
the secret, and resolves it each time it hands the secret to a consumer, through a short cache. Rotating the
secret in AWS reaches every consumer with nothing to change on the platform.

| Form | Resolves to |
|---|---|
| `aws-sm://<name-or-arn>` | the secret's `SecretString` (AWSCURRENT) |
| `aws-sm://<name-or-arn>#<key>` | field `<key>` of the `SecretString` parsed as a JSON object: a string as is, a number or bool as its JSON text; a missing key, null, object or array is `KEY_MISSING` |
| `aws-ps://<parameter>` | the parameter value, `WithDecryption: true`. The name is everything after `aws-ps://`, so a path parameter is `aws-ps:///inhance/np/x` |

## Mirror list

1. **Resolver** (Go: `internal/secretref`). `Resolve(ref)`, `IsReference(v)`, `Allowed(ref)`, `CheckWrite(v)`.
   AWS clients built lazily from the default credential chain, one per region; the region comes from the
   ARN when the reference names one, else the default chain's. Interfaces over both clients so tests use
   fakes.
2. **Allow-list**, `FC_SECRET_REFERENCE_ALLOW`: comma-separated prefixes of whole reference text. Empty or
   unset means the feature is off. An ARN reference matches on the ARN text (allowing by name does not
   allow by ARN). Checked at write **and** at every resolve.
3. **Cache**, keyed by the AWS object (for `#key` references the whole secret is cached once per name and
   the key picked from it): fresh for `FC_SECRET_REFERENCE_TTL` (default 5m); concurrent misses share one
   AWS call, detached from the first caller's deadline (30 s bound); **stale on error**: when a refresh
   fails and a value is cached, serve it and log WARN (reference, code, age) for up to
   `FC_SECRET_REFERENCE_MAX_STALE` (default 1h), after that the error; a failed refresh (and a failure with
   nothing cached) is remembered for 30 s so a 15 s re-render loop does not hammer AWS. Stale is served for
   any error, not only outages.
4. **Error codes**: `SECRET_REFERENCE_NOT_FOUND` (`ResourceNotFoundException`, `ParameterNotFound`, a secret
   with no string value), `_DENIED` (`AccessDenied*`, `DecryptionFailure`, KMS access), `_KEY_MISSING`,
   `_UNAVAILABLE` (everything else: network, throttling, 5xx, credentials), `_NOT_ALLOWED`; and on the
   write only `SECRET_REFERENCE_UNSUPPORTED`. Messages and logs name the reference, never a value.
5. **Write checks** (400): on function SECRET and DB setting writes (`PutSetting` Validate), on platform
   config SECRET writes (`ValidSecretValue`, which the ingestion config write also calls for `secrets` and
   `secretFields`): `gcp-sm://`, `vault://`, `env://` are `SECRET_REFERENCE_UNSUPPORTED` naming the two that
   work (`env://` must never resolve: it would hand the platform's environment to a caller); an `aws-sm://`
   or `aws-ps://` reference outside the allow-list is `SECRET_REFERENCE_NOT_ALLOWED` (message names the
   allowed prefixes, or says the platform allows none). `literal:` and `encrypted:` keep their meaning. The
   checks are in the use cases, not the repository.
6. **The three read points**:
   - **Function settings -> the runner's desired document** (`ResolveSettingsForControl`). A stored
     SECRET/DB value that `IsReference` is resolved through the cache; it never goes through decryption
     (references are stored verbatim). A reference that does not resolve is **left out** of the function's
     secrets/DB map (the guest's `secret(...)` answers not-set and fails at the point of use), the function
     stays in the document, and the platform logs ERROR once per (function, key, code) per TTL with the
     address, key, reference and code. Never the reference text in place of the value. The document's ETag
     covers the plaintext and the document re-renders every 15 s per held poll, so resolution must come from
     the cache and a failed refresh must serve the last value rather than flip the document.
   - **Ingestion secrets -> the ingestor runtime** (`FetchBoundSecretForIngestionRuntime`, route
     `GET /api/ingestion-runtime/v1/ingestions/{id}/secrets/{name}`).
   - **Config secret reveal -> SDKs and apps** (`RevealAuthorised`, `GET /api/config/{app}/{section}/{property}/secret`).
   Config secrets are encrypted at rest whatever their value; resolution happens after decryption, on the
   plaintext.
7. **Status mapping** for the last two: `NOT_FOUND`/`DENIED`/`KEY_MISSING`/`NOT_ALLOWED` are `409` with the
   code; `UNAVAILABLE` is `503 SECRET_REFERENCE_UNAVAILABLE` (retryable). The huma error model derives a
   status from the use case kind, so the two handlers map the unavailable code to an explicit 503 error
   (`httpcompat.NewStatusError`). A reveal that did not resolve revealed nothing, so it writes no
   `SECRET_REVEALED` audit row; the ingestion fetch audits and logs it as any refused fetch.
8. **Display**: a stored reference is not masked. Where a SECRET shows `***` (platform-config list/get,
   `Config.MaskedValue`; the console's config table; the function settings list that blanks SECRET/DB) the
   reference itself is shown. The console's reveal of a config secret shows the **reference**, not the
   resolved value (the admin reveal is for seeing what is stored); the SDK/API reveal resolves. In Go the
   function settings list returns the reference in `value` for a SECRET/DB setting that holds one, and
   `""` otherwise.
9. **Metrics** (own registry, mounted on the metrics listener): `fc_secret_reference_resolve_total{outcome}`
   (`hit`, `miss`, `stale`) and `fc_secret_reference_errors_total{code}`.

## Deferred (also in Go)

- The per-key `referenceError` read-model field on a function's status/settings (so the console can show a
  failed reference on the settings row) is **not built**: the log + leave-out is. `ResolvedSettings.Unresolved`
  already carries the data (kind, key, reference, code) for it.

## Surprises

- The function settings write path stores `aws-sm://` / `aws-ps://` verbatim already (`IsStoredForm`), and
  the old read path returned the raw reference to the runner when decryption failed. The new read must test
  `IsReference` **before** decrypting. A legacy row under another external scheme (`gcp-sm://`...) still
  returns its stored text unchanged (nothing resolves it); new writes of those are refused.
- Config secret references are stored as ordinary encrypted SECRET values, so the list/get read must decrypt
  to know whether a secret is a reference (Go does it in the repository's finders; a SECRET that cannot be
  opened stays masked).
- No test calls AWS; the Go tests use fake clients and a fake resolver, and assert a resolved value is absent
  from captured `slog` output.

## The Java ingestor

No change. A `409` from the secrets call is not a permanent refusal in `PlatformApi.permanent` and not a
platform-unavailable status either, so it surfaces as an `IOException` -> `SecretStoreException`
("cannot be read from the platform: ... answered 409: SECRET_REFERENCE_NOT_FOUND ...", with the platform's
code in the message), and `Secrets.reveal` remembers a failed reveal for 10 s so it does not retry hot.
A `503` is `PlatformUnavailableException`, the same path. `HttpIngest` answers its caller 503 and records the
failure through the failure pacing. Both are visible failures, not swallowed.

## Not in this change

- IAM on the platform's ECS task role (`secretsmanager:GetSecretValue`, `ssm:GetParameter`, `kms:Decrypt`
  scoped to the allowed prefixes): the iac repository, on the owner's go-ahead.
- Other schemes; references in other secret holders (IdP client secrets, service accounts).
- fcdev: `fn set --secret KEY=aws-sm://...` help text; `ingest config set --secrets-file refs.json` with
  `{"<secret name>": "<reference>"}`, references only (a plain value is refused client-side).
