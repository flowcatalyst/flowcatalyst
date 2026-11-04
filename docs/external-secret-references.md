# External secret references (`aws-sm://`, `aws-ps://`)

Status: owner asked 2026-11-04 ("our secrets should be able to take a pointer to AWS Secrets Manager or
Parameter Store so we don't have to set and update the same parameters multiple times"); owner ruling the
same day: **resolved by the platform, at read time.** Build spec.

A secret value may be a reference instead of the secret. The platform stores the reference (never the
secret), and resolves it each time it hands the secret to a consumer, with a short cache. Rotating the
secret in AWS reaches every consumer without touching the platform.

## 1. Reference syntax

| Form | Resolves to |
|---|---|
| `aws-sm://<name-or-arn>` | the secret's `SecretString` (stage AWSCURRENT) |
| `aws-sm://<name-or-arn>#<key>` | field `<key>` of the `SecretString` parsed as a JSON object; a string field as is, a number/bool as its JSON text; a missing key, null, object or array is an error |
| `aws-ps://<parameter-name>` | the parameter's value, `WithDecryption: true` (String and SecureString). The name is everything after `aws-ps://`, so a path parameter is written `aws-ps:///inhance/np/x` |

- `encryption.EncryptSecretRef`'s scheme list (`internal/platform/shared/encryption/secretref.go`) stays the
  write-side authority for the closed list of schemes. Only `aws-sm` and `aws-ps` are **resolvable** in this
  build. On the paths in §3, a write of any other external scheme (`gcp-sm://`, `vault://`, `env://`) is
  refused with 400 `SECRET_REFERENCE_UNSUPPORTED` naming the two that work. `env://` especially must never
  resolve on these paths: it would hand the platform's own environment to a caller. `literal:` keeps its
  current meaning.
- A reference is not a secret: it may be shown, logged and audited. The resolved value never is.

## 2. Allow-list (required)

Without one, anyone who may set a function secret could point it at any secret the platform's task role can
read (for example the database credentials) and have it delivered to a function. So:

- `FC_SECRET_REFERENCE_ALLOW`: comma-separated prefixes of whole references, for example
  `aws-sm://prod/data_engineering/,aws-ps:///inhance/np/shared/`. A reference must start with one of them.
  Empty or unset: the feature is off (writes of a reference are refused with 400
  `SECRET_REFERENCE_NOT_ALLOWED` and the message says the platform allows none; reads of a stored one fail as
  in §4).
- An ARN reference matches on the ARN text, so allowing by name does not allow by ARN; documented, not worked
  around.
- Checked at write (400 `SECRET_REFERENCE_NOT_ALLOWED`, naming the allowed prefixes) **and** at resolve (an
  allow-list narrowed after the write stops the old reference too).
- Document it in `docs/environment-variables.md`; fcdev's local platform leaves it unset (off) unless the
  developer sets it.

## 3. Where references are resolved

The three places a secret leaves the platform:

1. **Function SECRET and DB settings → the runner's desired document.**
   `internal/platform/function/repository_control.go` `ResolveSettingsForControl` (via
   `decryptSettingValue`). The write path already stores `aws-sm://`/`aws-ps://` verbatim
   (`UpsertSetting`/`UpsertSettingTx` → `EncryptSecretRef`); add the allow-list and resolvable-scheme checks on
   that write (the `fn set` use case / API), not in the repository.
   The desired document is re-rendered every 15 s per held poll and its ETag covers the plaintext
   (`control/desired.go` `etagOf`): resolution MUST come from the cache (§4), and a failed refresh must not
   make the document flip between values (serve the last good value, §4).
2. **Ingestion secrets → the ingestor runtime.**
   `internal/platform/platformconfig/service.go` `FetchBoundSecretForIngestionRuntime` (route
   `GET /api/ingestion-runtime/v1/ingestions/{id}/secrets/{name}`, `runtimeapi/secrets.go`).
3. **Config secret reveal → SDKs and apps.** `Service.RevealAuthorised` (`GET /api/config/{app}/{section}/{property}/secret`).
   Resolves, so an application that stores a reference reads the secret as before.

Config secrets are encrypted at rest whatever their value (`platformconfig/repository.go` `storedValue`): a
reference is stored encrypted and resolution happens **after** decryption, on the plaintext. Add the scheme and
allow-list checks to the config-secret write (`operations/set_property.go`, beside `ValidSecretValue`).

A shared resolver is called at those three points; nothing else changes shape on the wire.

**Display (console, list APIs):** a stored reference is not masked: where a secret shows as `***`
(`platformconfig/entity.go` `MaskedValue`, the function settings list that blanks SECRET/DB), show the reference
itself. The console's function settings and the platform config page show it as such ("reference:
aws-sm://..."). The UI "reveal" of a reference shows the reference, not the resolved value (the admin reveal
is for seeing what is stored).

## 4. Resolver

New package `internal/secretref` (or extend `internal/secrets` if that stays clean; it is unused today):

- `Resolve(ctx, ref) (string, error)`; `IsReference(v) bool`; `Allowed(ref) error`.
- AWS clients built once, lazily, from the default credential chain (`awsconfig.LoadDefaultConfig`); region
  from an ARN when the reference is one, else the default chain (reuse/move `internal/server/dbsecret.go`'s
  helper). Add `github.com/aws/aws-sdk-go-v2/service/ssm` to go.mod. Interfaces for both clients so tests use
  fakes; no test calls AWS.
- Cache keyed by the reference text (for `#key` refs, cache the whole secret once per name and pick the key):
  - fresh for `FC_SECRET_REFERENCE_TTL` (default 5m);
  - concurrent misses for one key share one AWS call (singleflight, as `internal/functions/runner/platformtoken.go` does);
  - **stale on error**: when a refresh fails and a value is cached, serve it and log WARN (reference, error
    code, age), for up to `FC_SECRET_REFERENCE_MAX_STALE` (default 1h); after that, the error;
  - a failure with nothing cached is cached negatively for 30 s (no hammering AWS from a 15 s re-render loop);
  - never cache an `AccessDenied`/`ResourceNotFound` as stale-servable after the value is gone? No: serve stale
    for any error (rotation-safe, outage-safe); deletion shows after max-stale. Keep it simple.
- Errors carry a code: `SECRET_REFERENCE_NOT_FOUND`, `SECRET_REFERENCE_DENIED`, `SECRET_REFERENCE_KEY_MISSING`,
  `SECRET_REFERENCE_UNAVAILABLE` (network/throttle/5xx), `SECRET_REFERENCE_NOT_ALLOWED`.
- Logs and errors name the reference, never a value. Metrics: hits, misses, errors by code, stale serves (use
  the repo's existing metrics pattern if there is one; otherwise skip and say so).

Failure behaviour per path ("never skip silently"):

1. Desired document: the function stays in the document; the unresolved key is **left out** of its
   secrets/DB map (so the guest's `secret(...)` answers not-set and the function fails loudly at the point of
   use), and the platform logs ERROR once per (function, key, error code) per TTL: address, key, reference,
   code. Never put the reference text in place of the value (the old behaviour) for a resolvable scheme.
   Also expose it: the function's status/settings read model gains, per key, `referenceError` (code) when the
   last resolve failed (console shows it on the settings row). If that read-model change is large, do the log
   + leave-out now and list the status field as follow-up in the report.
2. Ingestion runtime fetch: 503 `SECRET_REFERENCE_UNAVAILABLE` (retryable) for UNAVAILABLE; 409 with the code
   for NOT_FOUND / DENIED / KEY_MISSING / NOT_ALLOWED (the ingestor already backs off on refusals; check
   `ingestor/.../runtime/PlatformApi.java` and `Secrets.java` so a 409 is a visible failure there, and adjust
   the Java side only if it would retry hot or swallow it).
3. Reveal: same status mapping as 2.

## 5. fcdev

- `fcdev fn set --secret KEY=aws-sm://...` already sends the value; the platform now validates it. Help text:
  mention references.
- `fcdev ingest config set ... --secrets-file <file.json>`: non-interactive. The file is `{ "<secret name>":
  "<reference>" }`; every value must be an `aws-sm://` or `aws-ps://` reference (a plain value is refused by
  fcdev before sending: "a secrets file holds references only; use --secrets to type a value"). Sent with the
  entry in the same write `--secrets` uses. `--secrets` and `--secrets-file` together is an error. Tests.

## 6. Tests

- Resolver: sm whole, sm `#key` (string, number, missing, non-object), ps, ARN region, cache hit, TTL expiry,
  singleflight (N concurrent → 1 call), stale-on-error within max-stale, error after max-stale, negative cache
  30 s, allow-list match/miss (including ARN vs name), value never in logs (capture slog output).
- Function path: desired document carries the resolved value; ETag stable across re-renders with a fresh cache;
  unresolved key left out + ERROR logged; write refused for a non-allowed / unsupported scheme reference (API
  test). Integration test (`-tags=integration`) for `ResolveSettingsForControl` with a fake resolver.
- Config path: write validation; ingestion-runtime fetch resolves; 503/409 mapping; reveal resolves; masked
  display shows the reference.
- fcdev `--secrets-file`.

## 7. Docs

`docs/environment-variables.md` (the three variables), `docs/published/` (the functions page's settings
section and the platform-config/ingestion secrets docs: how to write a reference, the allow-list, rotation and
caching behaviour, the IAM the platform's role needs), `docs/wire-contract.md` if the settings/config read
models change, and a Java-port hand-off note `docs/java-handoff-2026-11-04-external-secret-references.md` in
the shape of `docs/java-handoff-2026-11-04-function-platform-token.md`.

## 8. Not in this build

- IAM on the platform's ECS task role (`secretsmanager:GetSecretValue`, `ssm:GetParameter`, `kms:Decrypt`
  scoped to the allowed prefixes): the iac repository, done separately on the owner's go-ahead.
- Other schemes (`gcp-sm`, `vault`). Resolving references in other secret holders (IdP client secrets,
  service accounts), which keep their current behaviour.
- Ingestion settings values that are not secrets (a MiX `clientId` or `username` stays a plain value in the
  settings file).
