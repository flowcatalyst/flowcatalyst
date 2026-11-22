# The Wire Contract

The HTTP API is a stable wire contract. Its consumers — the embedded Vue
frontend, the TypeScript SDK, the Laravel SDK, and every webhook subscriber —
are built against the exact shapes documented here. Changing any of them is a
breaking change and requires deliberate, versioned evolution, not a refactor.

---

## What counts as a breaking change

These changes look harmless and are **not** allowed:

- **Renaming a JSON field.** `eventTypeId` → `event_type_id`: no.
- **Changing an enum's string value.** `"CURRENT"` → `"current"`: no.
- **Changing nullable to required (or vice versa).**
- **Adding or removing a field from a response**, even when the value is null,
  even when "logically equivalent".
- **Changing the status code for an error case.** 422 → 400: no.
- **Returning the updated entity from PUT where the contract is 204 No
  Content.** The frontend treats `Promise<void>` differently from
  `Promise<Entity>`. See [conventions §7](./conventions.md#7-frontend-api-response-handling).
- **Changing an OpenAPI operationId.** The frontend's generated TypeScript
  client uses operationIds as method names (`postApiEventTypes`,
  `getApiEventTypesById`, …). Renaming an operationId renames a frontend
  method, breaking the build.
- **Changing a pagination shape.** Cursor → offset, or vice versa, per endpoint.

## How it's enforced

- **`api/openapi.lock.json`** — the committed spec baseline, generated from the
  huma-registered routes. `make api-diff` (part of `make ci`) fails on any
  drift; `make api-bump` records an intentional change. Every operation also
  carries its SDK class as `x-sdk: app | admin` (absent = internal), and the
  routes the server mounts as plain handlers (`POST /api/dispatch-jobs`,
  `/api/dispatch-jobs/batch`, `/api/audit-logs/batch`, `/api/me*`, the OAuth/OIDC
  endpoints) are described by spec-only `RegisterSpec` stubs; see
  `docs/sdk-audit-and-python-sdk.md` ("Spec tagging as built"). The
  application-facing surface is exactly the `x-sdk: app` operations.
- **Per-feature contract tests** — the Go tests next to each handler assert
  status, headers and body shape for representative requests (there is no
  separate replay harness); the shared SDK vectors under
  `clients/*/tests/fixtures/wire/` pin the cross-language pieces.
- **Frontend codegen** — the frontend's TypeScript client is generated from
  the spec; regenerating must not produce a diff unless the contract
  deliberately changed.

---

## Timestamps

API timestamps serialize as RFC3339 with **fixed 6-digit microsecond**
precision: `2026-05-24T08:30:00.123456Z`. Not milliseconds, not nanoseconds.

Implemented by `internal/platform/shared/jsontime.Time` (aliased as
`httpcompat.Time`) — use it for every timestamp field in a wire DTO, never a
bare `time.Time` (whose `RFC3339Nano` default emits variable-width nanosecond
precision).

Exception: the webhook delivery path uses **millisecond** precision in its
signed timestamp header — see [Webhook signatures](#webhook-signatures).

## Nullable fields

Where the contract omits a field when it's unset, use `*T` + `omitempty` —
omits when nil. Avoid `T` + `omitempty` for values where zero is legitimate
(a count of 0 would disappear). For optional booleans where `false` is a real
value, `*bool` + `omitempty` is required.

The `null`-vs-missing posture is part of the contract per-field: do not switch
one for the other.

## Empty collections

`omitempty` on a slice omits when nil **or** empty. Where the contract always
emits `[]` (e.g. list endpoints), drop `omitempty`. The posture is per-field;
match what the lock file records.

## Enum values

Enums are `SCREAMING_SNAKE_CASE` strings on the wire (`"CURRENT"`,
`"ARCHIVED"`). In Go they are string typedefs with constants, plus a `Valid()`
method. Parsing is lenient — unknown strings default to a designated value
rather than erroring.

## Error envelope

Errors serialize as:

```json
{
  "error": "VALIDATION_ERROR",
  "message": "Event type code is required"
}
```

The `error` field carries the error **code** string. The dominant path has
**no** `details` field; only the auth/rate-limit middleware envelope carries an
optional `details` that is omitted when absent.

Centralized in `internal/platform/shared/httperror/` (chi path) and
`internal/platform/shared/httpcompat/` (huma path):

```go
type Envelope struct {
    Code    string                 `json:"error"` // the error CODE string
    Message string                 `json:"message"`
    Details map[string]interface{} `json:"details,omitempty"` // middleware only
}
```

**Bearer-token failures are the exception.** A missing, invalid or expired
bearer token on `/api/*` is answered by the auth middleware
(`internal/platform/shared/middleware/middleware.go`, `writeInvalidTokenError`)
with an OAuth-style body and a challenge header:

```http
HTTP/1.1 401 Unauthorized
WWW-Authenticate: Bearer error="invalid_token"
Content-Type: application/json

{"error":"invalid_token","error_description":"<reason>"}
```

`error_description` replaces `message` here. Clients that branch on `error`
see `invalid_token` rather than a platform code such as `UNAUTHORIZED`.

Status-code mapping: Validation→400,
Unauthorized/InvalidCredentials/TokenExpired/InvalidToken→401, Forbidden→403,
NotFound→404, Duplicate/BusinessRule/Concurrency→409, TooManyRequests→429,
catch-all→500. **There is no 422 in the mapping.** Routes that change how a
principal signs in (passkey register / delete, the 2FA self-service writes) answer
403 `SESSION_REQUIRED` to an API bearer and 403 `REAUTH_REQUIRED` ("Sign in again to
change your sign-in methods") to a browser session older than
`FC_AUTH_STEPUP_MAX_AGE_SECS`; passkey registration for an SSO-managed user is 400
`SSO_MANAGED`. The session-cookie guard
and the sign-in (login-CSRF) guard add 403 `CROSS_ORIGIN_REQUEST` and 415
`UNSUPPORTED_MEDIA_TYPE` (see Authentication); the debug endpoints add 403
`CROSS_ORIGIN_REQUEST`; the SSO callback adds 400 `LOGIN_NOT_STARTED_HERE`; an
unknown path under `/api`, `/bff`, `/oauth`,
`/.well-known`, `/control` or `/q` is 404 `NOT_FOUND` (it used to be the SPA's
`index.html` with 200).

## Pagination shapes

Three postures exist across the API; each list endpoint uses one specific
posture, and which one is part of that endpoint's contract:

```json
// Cursor (high-volume firehose tables — events, dispatch jobs):
{
  "items": [...],
  "nextCursor": "abc123",
  "hasMore": true
}

// Offset (most list endpoints):
{
  "items": [...],
  "totalCount": 1234,
  "page": 1,
  "pageSize": 50,
  "totalPages": 25
}

// Size only (some firehose tables):
{
  "items": [...]
}
```

## Authentication

JWT shape:

- Header: `alg=RS256`, `kid=<key-id>`, `typ=JWT`.
- Claims include `sub`, `iat`, `exp`, `iss`, `scope`, `clients` (array),
  `roles` (array), `applications` (array), `email`. Exact field names — no
  `client_ids` vs `clients` substitutions.

**Token spelling.** Every signed token the platform verifies (session cookies and bearer access
tokens, field access tokens, MFA and enrolment tokens, DPoP proofs, and the access tokens the function
runner checks) must be spelled exactly as RFC 7515 §2 base64url: three segments of the URL-safe
alphabet only, no `=` padding, no `+` or `/`, no whitespace or line break, and the spare low bits of
each segment's last character zero (RFC 4648 §3.5). Anything else is refused as an invalid token
(`invalid_token`, or `invalid_dpop_proof` for a proof) even when it decodes to the bytes of a valid
one, so a token has one spelling only. The platform's own tokens are written by a standard encoder
and are always canonical; a token issued before this rule verifies unchanged. A client must send a
token exactly as it received it, and a DPoP signer must encode with a standard base64url encoder
(the field libraries do). The public JWK in a proof header and at device enrolment follows the same
rule for `x` and `y`. ID tokens from external identity providers are not covered (bug-fix ledger
F53). The SDKs' verifiers apply the same rule, and `api/field-fixtures/noncanonical.json` holds the
re-spellings every field verifier must refuse (a token as `malformed`, a proof as `invalid_proof`).

JWKS is served at `/.well-known/jwks.json`. The `client_credentials` grant at
`/oauth/token` returns `Cache-Control: no-store` and
`Content-Type: application/json`.

A token response that carries a `refresh_token` also carries `refresh_expires_in` (seconds until that
refresh family's ABSOLUTE deadline; rotation never extends it). Additive and optional. The built-in public
client `fcdev-cli` (`authorization_code` + `refresh_token`, PKCE `S256` required) may use any port on its
registered `http://127.0.0.1/callback` and `http://[::1]/callback` redirects (RFC 8252); no other client
may.

**Chi-mounted auth/session endpoints (intentionally absent from
`api/openapi.lock.json`).** The lock is generated from the huma-registered
platform routes; the auth/session surface is served by plain `chi` handlers,
so it does **not** appear in that lock — by design. The inventory:

| Method | Path | Purpose |
|---|---|---|
| POST | `/auth/login` | password → session cookie + principal |
| POST | `/auth/refresh` | rotate a refresh token without an existing session |
| GET | `/auth/check-domain` | resolve auth method for an email's domain |
| GET | `/auth/me` | read the current session cookie's principal |
| GET | `/auth/oidc/login`, `/auth/oidc/callback` | OIDC SSO (email-domain path) |
| POST | `/auth/password-reset/request`, `/auth/password-reset/confirm`, `/auth/password-setup/request` | unauthenticated password reset (hex SHA-256 tokens) |
| POST | `/auth/2fa/verify`, `/auth/2fa/challenge/email`, `/auth/2fa/enroll/*` | second factor that finishes a password login |
| POST | `/auth/logout`, `/auth/check-domain` | clear the session cookie; resolve auth method (SPA shape) |
| GET | `/auth/password-reset/validate` | check a reset token |
| GET | `/api/me`, `/api/me/applications` | caller identity + accessible applications |
| GET/POST | `/oauth/authorize`, `/oauth/token` | OAuth/OIDC provider surface |
| GET | `/.well-known/openid-configuration`, `/.well-known/jwks.json` | OIDC discovery + JWKS |

**Session cookie name.** The browser session cookie is `__Host-fc_session` when it is
written with `Secure` (HTTPS) and `fc_session` on plain HTTP (development). A `__Host-`
cookie cannot be set by a sibling subdomain. The old name `fc_session` is still read: a
session token minted before this change has no `host_cookie` claim and is accepted under
the old name until it expires; a token minted since carries the claim and, on a process
that issues `Secure` cookies, is refused when it arrives as `fc_session` (the request is
treated as signed out, like a stale cookie). When both cookies are present, `__Host-fc_session`
is used. Sign-out expires both names. During a rolling upgrade an instance still on the
old version does not read the new cookie, so someone who signs in mid-upgrade may be asked
to sign in again; a rollback signs out the sessions created after the upgrade.

**Session-cookie requests (since 2026-10-13, docs/platform-bug-fixes.md F16).**
A POST, PUT, PATCH or DELETE under the authenticated group (`/api/*`,
`/bff/*`, `/auth/*` session routes) that the `fc_session` cookie authenticates
must come from the platform's own origin: `Sec-Fetch-Site: same-origin` or
`none`, or, without that header, an `Origin` naming the request's host or the
public base URL (`FC_JWT_ISSUER`). Otherwise, and when neither header is
present, the answer is **403 `CROSS_ORIGIN_REQUEST`**. Its body, when it has
one, must be `application/json` (or `application/*+json`), else **415
`UNSUPPORTED_MEDIA_TYPE`**. Requests authenticated by `Authorization: Bearer`
(SDKs, service accounts, the router), the OAuth/OIDC endpoints and the
dispatch callbacks are not subject to either rule. A script that replays a
browser's cookie must send a matching `Origin` (or use a bearer token).

**Sign-in and credential requests (login CSRF, since 2026-10-13, F16
follow-up).** The public routes that sign a browser in or set its credentials
take only a JSON body, from any caller: `POST /auth/login`, the public
`/auth/2fa/*` steps (`verify`, `challenge/email`, `enroll/{totp,email}/{begin,
confirm}`), `POST /auth/webauthn/authenticate/{begin,complete}`,
`POST /auth/password-reset/{request,confirm}`, `POST /auth/password-setup/request`,
and `POST /portal/auth/{check-domain,login,password-reset}`. A body that is not
`application/json` (or `application/*+json`), a body without `Content-Type`
included, is **415 `UNSUPPORTED_MEDIA_TYPE`**. A request a browser marks as
coming from elsewhere (`Sec-Fetch-Site` other than `same-origin`/`none`, or,
without it, an `Origin` naming neither the request's host nor the public base
URL, `Origin: null` included) is **403 `CROSS_ORIGIN_REQUEST`**. Unlike the
session-cookie rule, a request with **neither** header is accepted: a
non-browser caller (curl, a script) that sends JSON keeps working. Not guarded:
`POST /auth/check-domain` (read-only), `POST /auth/logout` (issues nothing;
`GET /auth/oidc/session/end` clears the cookie by design), `POST /auth/refresh`
(tokens in the JSON body, no cookie), and the form-encoded OAuth endpoints
(`/oauth/token`, `/oauth/introspect`, `/oauth/revoke`: form-encoded by the
spec, client-authenticated, PKCE; no cookie).

**Revocation, SSO and passkey additions (2026-10-19, F36 to F45).**
`POST /oauth/revoke` still answers 200 for every token, but a client authenticated
by credentials revokes only refresh tokens issued to it (a token bound to another
client is left live); Bearer callers and unbound tokens are unchanged
(`FC_OAUTH_REVOKE_ENFORCE_CLIENT=false` restores the old behaviour).
`POST /oauth/introspect` is unchanged unless `FC_OAUTH_INTROSPECT_CLIENT_AUTH_ONLY=true`
(Bearer then gets 401 `invalid_client`). With `FC_OIDC_REJECT_UNVERIFIED_EMAIL=true` an
SSO callback whose id_token says `email_verified: false` is **403 `EMAIL_NOT_VERIFIED`**.
A passkey assertion whose signature counter did not advance is
**401 `INVALID_CREDENTIALS`** (and the counter is now stored). Refresh rotation:
of concurrent presentations of one refresh token exactly one succeeds; the others
get `invalid_grant` and the family is revoked as for any replay.

**SSO callback binding (since 2026-10-13, F16 follow-up).**
`GET /auth/oidc/login` and `GET /portal/auth/oidc/login` set a per-login cookie
(`__Host-fc_sso_<digest>` over HTTPS, `fc_sso_<digest>` over HTTP; HttpOnly,
SameSite=Lax, Path=/, 10 minutes, the login state's lifetime) in the browser
that starts the sign-in. `GET /auth/oidc/callback` requires it: a callback the
browser did not start is **400 `LOGIN_NOT_STARTED_HERE`** (and its state is
spent); the callback clears the cookie. A sign-in must therefore start and
finish in the same browser, on the host of the public base URL (the callback's
host).

**Debug endpoints (`<router prefix>/debug/*`, `/debug/*`, since 2026-10-13).**
A request whose `Sec-Fetch-Site` is present and neither `same-origin` nor
`none`, and a request the `fc_session` cookie authenticates without
`Sec-Fetch-Site`, are **403 `CROSS_ORIGIN_REQUEST`**. Tools with a bearer token
or the router's Basic credentials (no `Sec-Fetch-Site`) are not affected.

**`$schema` in huma responses.** Responses of the huma routes carry a
`"$schema"` field (and a `Link: <…>; rel="describedBy"` header) pointing at
`<scheme>://<host>/<Type>.json`, a path the platform does not serve; the spec
declares the field (read-only) on every response schema and the generated SDK
types carry it. Clients ignore it. It is not removed because that changes the
lock and every generated SDK.

**`/oauth/authorize?provider=<idp-id>` (direct-IdP entry, since 2026-07-22):**
`?provider=` chains into the OIDC bridge (`/auth/oidc/login?provider_id=` +
the `oauth_*` param set), which resolves the IdP by id
(`bridge.ResolveByProviderID`), completes the code flow on the callback, and
resumes `/oauth/authorize` so the downstream app receives its code. This path
JIT-provisions null-client principals (no role sync). The IdP must own the token's
email domain (a mapping points at it) and the mapping's tenant pin applies; an
IdP with no mapped domain answers `400 IDP_NO_EMAIL_DOMAINS` at the start. NOTE: portals no longer use this path — the portal
identity plane (docs/portal-identity-plan.md Phase 2.5 v2) has its own
`/portal/*` entry points and identity population; provider-direct JIT of
principals remains as Phase-1 behaviour for non-portal apps. Portal-flagged
OAuth clients are refused at `/oauth/authorize` outright.

## Encoded `data` and `operationData` at batch ingest

The SDK outbox writers (Java, TypeScript, Laravel, Rust, the framework) put an
event's `data` and an audit entry's `operationData` into the batch item as a
JSON **string** holding the serialised value (`"data":"{\"a\":1}"`); the Go
SDK sends the value itself. `POST /api/events/batch` (`data`) and
`POST /api/audit-logs/batch` (`operationData`) unwrap it at ingest (owner
ruling 2026-10-12, `sdk.UnwrapEncodedJSON`), so the stored event, the
dispatch job fanned out from it, the delivery envelope's `data`, a `dataOnly`
body and the audit view all carry the object.

The rule, applied once per member:

- A member that is not a JSON string (object, array, number, boolean, null)
  is stored as sent.
- A string whose text, less surrounding JSON whitespace, starts with `{` or
  `[` and is exactly one valid JSON value is replaced by that value.
- Every other string is stored as sent: `"hello"`, `""`, `"42"`, `"true"`,
  `"null"` (Java's encoding of a null map), an encoded string, and text that
  only looks like JSON (`"{not json"`, `"{} trailing"`).
- At most one level is removed; the result is never a string, so nothing is
  unwrapped twice, and a value encoded twice is kept as sent.
- Audit redaction runs after the unwrap, so it sees inside the value.

Edge case, accepted: a writer that means a string whose text is a JSON object
or array gets the object; to send such text as text, wrap it in an object.
Not changed: the singular `POST /api/events` (it receives the value itself),
dispatch-job `payload` (a string by contract, with `payloadContentType`), and
function-emitted events and function-created dispatch jobs
(`docs/function-runner-plan.md` §8.7: their metadata is always an array of
`{key,value}`, with the platform's `fn.*` provenance entries appended).

**Go SDK outbox rows.** Since 2026-10-13 the Go SDK (`pkg/fcsdk/outbox`, used
by `outboxpgx` and `outboxsql`) writes the batch endpoints' camelCase items,
held to the framework's outbox conformance vector (copied in
`pkg/fcsdk/outbox/testdata/conformance`): the row id and `deduplicationId`
are the event id, `clientCode` comes from the writer's configuration (or the
event), `source` is left out when the event has none, `contextData` carries
`principalId` and `executionId`, and `data`/`operationData` are the value
itself (never encoded). It also writes `DISPATCH_JOB` rows (`clientCode`
optional, F14); a use case declares them on its plan
(`usecaseop.Plan.Dispatch`), which writes them after the event and before the
audit row in the same transaction, with the event's `correlationId` when the
job has none (framework write-path 7a). Rows the Go SDK wrote before then are snake_case
(`event_type`, `spec_version`, `correlation_id`, `deduplication_id` =
`<type>-<id>`, `message_group`, `context_data`, ...) and may still be pending
in applications' outbox tables: `POST /api/events/batch` keeps reading those
aliases (`BatchEventItem.UnmarshalJSON`), pinned by
`internal/platform/event/api/go_sdk_wire_shapes_pg_test.go`. Do not drop the
aliases while such rows can exist.

## SDK ingest: client and source

Owner rulings 2026-10-12 (`docs/platform-bug-fixes.md` F14;
`internal/platform/shared/sdk/client_code.go`). A `clientId` or `clientCode`
that is absent, `null`, empty or whitespace counts as not given.

**Event client** (`POST /api/events/batch`, `/bff/events/batch`,
`POST /api/events`):

- A `clientId` the caller cannot reach: `403 {"error":"FORBIDDEN","message":"No access to client: <clientId>"}`.
- Batch only, no `clientId`: a `clientCode` naming a client the caller
  reaches resolves to it; naming another client, or no client, from a
  non-anchor: `403 FORBIDDEN "No access to client: <clientCode>"`; an
  anchor's unknown code leaves the event platform-scoped.
- No client at all (singular: a non-anchor first defaults to its first
  client): an anchor writes a platform-scoped event; a caller whose `clients`
  is exactly one id writes under it; anyone else:
  `403 FORBIDDEN "clientId is required: a client-scoped caller cannot write platform-scoped rows"`.
- In the batch these refusals reject the whole request (nothing written);
  type/source/data problems and application confinement stay per item
  (`BAD_REQUEST` in that item's `results[]` slot) and are checked first.

**Audit client** (`POST /api/audit-logs/batch`, 2026-11-18; `AuditClient` in
`internal/platform/shared/sdk/audit_batch.go`). Items carry `clientCode` only
(no `clientId`). Every refusal is per item, in that item's `results[]` slot;
the rest of the batch lands:

- A `clientCode` naming a client the caller reaches resolves to it.
- A `clientCode` naming no client, from an anchor: the row is stored
  platform-scoped (no client), as an anchor's event with an unknown code is.
- A `clientCode` naming a client the caller does not reach, or naming no
  client, from a non-anchor: `FORBIDDEN` "No access to client: <clientCode>".
- No `clientCode`: no client (unchanged).
- An `applicationCode` naming no application: `BAD_REQUEST`
  "Application not found: <applicationCode>".
- `SKIPPED` is no longer answered. Before 2026-11-18 an unknown client code
  (from any caller), an unreachable client and an unknown application were
  each `SKIPPED` and not stored, and the outbox processor cleared a `SKIPPED`
  row as delivered, so the entry was lost without a trace. The processor now
  keeps a `SKIPPED` row as a refusal (below), for a platform that still
  answers it.

**Outbox processor: per-item `SKIPPED`** (2026-11-18,
`internal/outbox/http_dispatcher.go`). A `SKIPPED` result is a refusal, not a
delivery: the row stays in the outbox table with status `2` (`BAD_REQUEST`)
and the message `SKIPPED: the platform did not store this item` (the
platform's own text after it, when it sent one; behind the processor's
`[blocking] ` marker, as every refusal), so an operator sees it and can return
it to pending (status `0`) once the cause is fixed. It is not retried. Audit rows are written without a message group by every writer,
so such a row blocks no group; a grouped row answered `SKIPPED` would block
its group under block-on-error, as any refusal does.

**Dispatch-job client** (`POST /api/dispatch-jobs/batch`,
`POST /api/dispatch-jobs`; both accept `clientCode`, never stored):

- A `clientId` wins (any `clientCode` is ignored) and must be reachable:
  `403 FORBIDDEN "No access to client: <clientId>"`.
- Else a `clientCode` naming a client the caller reaches resolves to it.
- A code naming another client or no client, from a non-anchor:
  `403 FORBIDDEN "No access to client: <clientCode>"`.
- A code naming no client, from an anchor:
  `400 {"error":"CLIENT_NOT_FOUND","message":"Client not found: <clientCode>"}`.
- Any refusal rejects the whole batch. Neither given: no client.

**Dispatch-job connection** (`connectionId` on a batch item and on
`POST /api/dispatch-jobs`, 2026-11-05; `internal/platform/shared/sdk/job_connection.go`).
Optional; absent, `null` or blank is none. It is stored on the job
(`connection_id`, the column a function's `fn.dispatch` job sets): pausing the
connection holds the job PENDING, and for a job with no subscription the
connection's service account signs the delivery (delivery credentials step 2b).
Checked after the client and application rules, so the connection is one the
caller reaches:

- unknown: `400 CONNECTION_NOT_FOUND "Connection not found: <id>"`;
- scoped to another client (or to a client when the job has none), or owned by
  another application than the job code's first segment:
  `400 CONNECTION_SCOPE_MISMATCH` (the rule a subscription sync applies to a
  `connectionId`, `connection.ScopeProblem`);
- with a `subscriptionId` whose connection is not this one:
  `400 CONNECTION_MISMATCH` (a subscription's job is signed through its
  subscription, so the id would otherwise be ignored);
- status is not checked: a PAUSED connection's job is created and waits.

Any refusal rejects the whole batch.

**Dispatch-job idempotency** (`idempotencyKey` on a batch item and on
`POST /api/dispatch-jobs`, 2026-11-05; migration 099,
`dispatchjob.Repository.InsertBatchKeyed`). Optional; trimmed; blank is none;
over 100 characters is `400 INVALID_IDEMPOTENCY_KEY`. A key is unique within
the job's application (the first segment of its `code`) and client (none
counts as its own scope). The first job to name it creates the job and holds
the key (`msg_dispatch_job_keys`, written in the job's transaction); a later
item with the same key in the same scope creates nothing and answers the
holding job's id: in its `results[]` slot with status `SUCCESS` on the batch,
as `{id}` with 201 on the singular route. Within one batch the earlier item
wins. Keys are kept 7 days (`dispatchjob.JobKeysRetention`). An SDK-supplied
`id` is not an idempotency key: `msg_dispatch_jobs` is partitioned by
`created_at`, so a repeated id is a second row.

**Dispatch-job batch fields not honoured.** A batch item's `metadata` is a list
of `{key, value}` (an object fails the JSON decode: `400 INVALID_JSON`, whole
batch). `headers`, `retryStrategy`, `scheduledFor` and `expiresAt` are not batch
fields and are ignored (`retryStrategy` is taken by the singular route only;
see "Dispatch delivery" below for what each strategy does).
The outbox processor (`internal/outbox`) forwards a `DISPATCH_JOB` row's
payload as written except that an object `metadata` (written by Laravel SDK
and TypeScript SDK releases before 2026-11-06) is turned into the
key-sorted list, non-string values as their JSON text.

**Event time** (`POST /api/events/batch`, since 2026-11-09). A batch item may carry
`time`, the instant the event occurred as the producer recorded it (RFC 3339, e.g.
`2026-11-09T10:15:02.481Z`); it is stored as the event's `time`. Absent, `null` or blank:
the ingest time, as before. `createdAt` is always the ingest time (it partitions the
table and clocks every ordering, delay and expiry). A `time` that does not parse, or lies
more than 5 minutes after the platform's clock, is that item's `BAD_REQUEST` result; the
rest of the batch lands. No SDK sends `time` yet: the Go, TypeScript, Laravel, Java and
Rust outbox payloads carry no time member (the Go SDK's comment says "the platform
stamps it at ingest"); the Python SDK has no outbox.

**Event source** (both event routes): an absent, `null` or blank `source` is
stored as the trimmed type's `{application}:{subdomain}`, its first two `:`
segments (`orders:sales:order:created` → `orders:sales`), or the first alone
when there is no non-blank second (`orders` → `orders`). A type whose first
segment is blank still answers `source is required` (batch item
`BAD_REQUEST`, singular `400 VALIDATION`). A given source is stored as sent.
`source` is optional on `CreateEventRequest`.

## SDK sync: roles

Owner rulings 2026-10-13 (`docs/platform-bug-fixes.md` F15).

**Role sync** (`POST /api/applications/{appCode}/roles/sync`): a platform
permission (first segment `platform`) that a synced role gains or loses, every
permission of a new role and of a role `removeUnlisted` deletes, must be one
the caller holds, as on the role-edit routes:
`403 {"error":"PERMISSION_ABOVE_CALLER","message":"You may only add or remove permissions you hold yourself: <permissions>"}`.
Only the change is checked; an empty `permissions` keeps the stored list and
checks nothing. The sync is all-or-nothing: a refusal rejects it whole,
nothing written.

**Application principal sync**
(`POST /api/applications/{appCode}/principals/sync`): role names are matched
lower-cased and as given (the platform adds no `app:` prefix). A name that is
not a role, whatever its prefix, is not assigned to anyone and creates
nothing; the rest of the sync applies, and the response lists it in
`skippedRoles` (lower-cased, each name once, in payload order; omitted when
none). Another application's role still refuses the whole sync,
`403 ROLE_APP_FORBIDDEN`. The platform route `POST /api/principals/sync` is
unchanged and has no `skippedRoles`.

## Application admin: scoped reads, users and invitations

F20 (`docs/platform-bug-fixes.md`). An application's own admin pages call the
platform as the application's service account. Everything below is confined to
the applications the credential is bound to (the `applications` claim); a
credential reaching every application (`*`) is not narrowed. Permissions are in
the built-in `platform:application-service` role (migration 080).

**Confinement, all routes.**
`{appCode}` routes: a bound application passes. Another application's code,
an unbound credential, and a code that names no application all answer the
same `404 {"error":"Application_NOT_FOUND","message":"Application not found: <code>"}`
(F32), so the answer does not say which applications exist. (The SDK sync
routes `/api/applications/{appCode}/*/sync` keep their own
`403 FORBIDDEN "Not authorised for application '<code>'"`.)
Records outside the application answer the same `404` a missing id would
(`Event_NOT_FOUND`, `DispatchJob_NOT_FOUND`, `User_NOT_FOUND`,
`Invitation_NOT_FOUND`).

**Events and dispatch jobs** (existing routes, new admission).
`GET /api/events`, `GET /api/events/{id}`, `GET /api/dispatch-jobs`,
`GET /api/dispatch-jobs/{id}`, `GET /api/dispatch-jobs/{id}/attempts`,
`GET /api/dispatch-jobs/event/{eventId}` additionally admit
`platform:application-service:event:view` / `...:dispatch-job:view`. Shapes,
filters and paging (`size`/`limit` up to 1000, `offset`) are unchanged. A caller
admitted only by the application-service permission sees only rows whose
application (the first segment of the event type or job code, the read
projection's `application` column) is one it is bound to; its own `applications`,
`types`, `code`, `codes` filters can only narrow within that, never widen. A
credential bound to no application (or to ids that resolve to none) is
`403 FORBIDDEN` "this credential is not bound to an application", not an empty
page. Callers holding the console's `platform:messaging:event:view` /
`dispatch-job:view` are unchanged and unconfined by application; the client
scoping of the console stays on top for both.

**Users** (`platform:application-service:user:view`, or `platform:iam:user:view`).
An application's users are the CLIENT-scope users homed at a client the
application is enabled for (`app_client_configs.enabled`) who hold at least one
of its roles. A client-scoped caller also keeps its own client reach.

- `GET /api/applications/{appCode}/users`: query `q`, `active`, `clientId`,
  `roles` (the application's roles; a foreign role matches nothing), `page`
  (0-based), `pageSize` (default 50, max 200), `sortField`
  (`name`|`email`|`createdAt`), `sortOrder`. Response
  `{users:[ApplicationUser], total, page, pageSize}`.
- `GET /api/applications/{appCode}/users/{id}`: one `ApplicationUser`.
- `ApplicationUser`: `id`, `email`, `name`, `active`, `clientId`, `idpType`
  (`INTERNAL`|`OIDC`), `roles` (only the application's own), `invitation`
  (`PENDING`|`EXPIRED`, absent unless awaiting an invitation), `lastLoginAt?`,
  `createdAt`. Never a password hash, token, link or second-factor state.
- `POST /api/applications/{appCode}/users` (`...:user:create` or
  `platform:iam:user:create`), `201`: body `{email, name, clientId, roles[1..],
  sendInvitation?, returnInviteLink?, inviteRedirectUri?}`. `clientId` is a `clt_`
  id or the identifier; the application must be enabled for it and the caller
  must reach it, otherwise `403` "client is not available to this application"
  (unknown, not enabled and out-of-reach are one answer). `roles` are the
  application's (`app:role` or the short name): another application's is
  `403 ROLE_APP_FORBIDDEN`, a name that is not a role is `400 UNKNOWN_ROLE`
  (never silently dropped), none is `400 ROLES_REQUIRED`; the F8 ceiling
  applies. The user is CLIENT scope, roles tagged `ADMIN_ASSIGNED`.
  `409 EMAIL_EXISTS` as for create-user. Response `{id, inviteLink?,
  invitationSent}`: the same `sendInvitation` / `returnInviteLink` /
  `inviteRedirectUri` rules as create-user; `invitationSent` is true only when
  the platform emailed it (false for a federated user, a returned link, or a
  failed delivery, which is logged; resend it).

**Invitations.** An invitation is an active, internal-password user, visible
to the application, who has not set a password. `PENDING` while an unexpired
invite token exists, `EXPIRED` otherwise (the token lapsed or was purged).

- `GET /api/applications/{appCode}/invitations`
  (`...:invitation:view`, `...:invitation:manage` or `platform:iam:user:view`):
  query `status` (`PENDING`|`EXPIRED`), `page`, `pageSize`. Response
  `{invitations:[{userId, email, name, clientId, roles, status, createdAt,
  expiresAt?}], total, page, pageSize}`. Never the link or token.
- `POST /api/applications/{appCode}/invitations/{id}/resend`
  (`...:invitation:manage` or `platform:iam:user:create`), `200`: optional body
  as above; mints a fresh invite (the earlier link stops working) and emails it
  or, with `returnInviteLink`, returns it. Response `{inviteLink?, expiresAt?}`.
  Nothing to deliver (`sendInvitation:false` without the link) is
  `400 INVITATION_DELIVERY_REQUIRED`; a failed delivery is `500
  INVITE_DELIVERY_FAILED`, never a silent success.
- `DELETE /api/applications/{appCode}/invitations/{id}`, `204`: revokes. The
  user is deactivated (a passwordless active user can ask for a fresh link at
  the login page, so deleting the link alone would revoke nothing) and the
  invite tokens are deleted. The user record stays; reactivating it is a console
  action. Not an invitee in the application: `404`.

## Application service-account provisioning

`POST /api/applications/{id}/provision-service-account`, `201`, creates the
application's service account (code `app:<application code>`), its SERVICE
principal and a confidential OAuth client, in one transaction, and answers
every secret once, in plaintext:

```json
{
  "message": "Service account provisioned",
  "serviceAccount": {
    "principalId": "prn_…",
    "name": "<application name> Service Account",
    "oauthClient": { "id": "oac_…", "clientId": "…", "clientSecret": "…" },
    "webhook": { "authToken": "fc_…", "signingSecret": "…" }
  }
}
```

- `serviceAccount.webhook` (added 2026-11-18, optional in the schema so the
  change was additive; provisioning always sends it) has the shape of
  `webhook` in `POST /api/service-accounts`' answer: `authToken` is the bearer
  the platform sends on deliveries to the application (`fc_` + 32 lowercase
  alphanumerics), `signingSecret` the HMAC-SHA256 key it signs them with
  (see [Webhook signatures](#webhook-signatures)), which the application's SDK
  verifies with. Before it, an operator had to call
  `POST /api/service-accounts/{id}/regenerate-signing-secret` to learn a secret
  the provisioning had already minted.
- Storage as at create: the client secret as a keyed hash, the token and
  signing secret encrypted. No secret is in the domain events
  (`ServiceAccountCreated`, `ApplicationServiceAccountProvisioned`,
  `OAuthClientCreated`), the audit rows or the logs; none can be read back.
- An application that already has a service account is refused,
  `409 ALREADY_PROVISIONED`, and nothing changes: the running application's
  signing secret is never replaced by a second provisioning. Rotation is the
  service account's own `regenerate-auth-token` / `regenerate-signing-secret`.
  Deleting the account's principal clears the application's link
  (`ON DELETE SET NULL`), after which it may be provisioned again.
- The answer is not repeatable: a provisioning whose answer is lost (a
  gateway's 502/503/504, a timeout) cannot be asked for again, and a second
  POST is refused `409`. The SDKs therefore never resend it automatically and
  report the outcome as unknown; read the account (below) to learn whether it
  was made, and rotate its secrets if it was.

### Reading an application's service account

`GET /api/applications/{id}/service-account` (added 2026-11-18), `200`,
answers the service account attached to the application with the body of
`GET /api/service-accounts/{id}` (`ServiceAccountResponse`): the account's own
id (`sac_…`), `code`, `name`, `principalId`, `oauthClientId` (the public OAuth
`client_id`) and the rest; never a secret. The application's own
`serviceAccountId` holds the account's SERVICE principal id (`prn_…`), not the
account's id, so `GET /api/service-accounts/{serviceAccountId}` with it answers
`404`; this route resolves the principal to its account. It needs read access
to the application (`platform:admin:application:view`, or the application
service's own `application:view` confined to its applications) and
`platform:iam:service-account:view`. An unknown application is
`404 Application_NOT_FOUND`; an application without an account, or with one
the caller may not view (the reach rules of `GET /api/service-accounts/{id}`),
is `404 ServiceAccount_NOT_FOUND`. `POST` on the same path attaches an existing
account (`204`).

## Scheduled jobs: cron, firing and delivery semantics

- `crons` are 6-field, seconds-first (`sec min hour dom mon dow`). Create, update and
  SDK sync reject any other field count with `CRON_INVALID_SHAPE` (5-field and 7-field
  expressions were previously accepted but never fired).
- Each (job, scheduled tick) fires at most once, enforced in the database
  (`msg_scheduled_job_ticks`), not only by the leader gate. Missed ticks are skipped to
  the latest.
- The firing envelope carries `concurrent` and `timeoutSeconds`. `concurrent` is NOT
  enforced by the platform; the receiver enforces concurrency. `timeoutSeconds` is also
  the platform's HTTP timeout for the call (default 10 s, capped at 300 s).
- An instance left `IN_FLIGHT` by a crash is marked `DELIVERY_FAILED` with an
  `ABANDONED` `deliveryError` after the call timeout plus a margin, and is never re-sent.

## Scheduled jobs: an application looks up and fires its own jobs

`platform:application-service:scheduled-job:fire` (seeded; held by the built-in
`platform:application-service` role; migration 100 grants it to existing
deployments) lets an application's service account look up and fire the
scheduled jobs of its own application(s), and nothing broader.

- `POST /api/scheduled-jobs/{id}/fire`: admitted by `platform:messaging:scheduled-job:fire`
  (any job, as before) or by the application-service fire permission for a job
  whose `applicationId` the caller may access (`applications` / `all_applications`
  claims). Another application's job, or a job with no application, is
  `403 APPLICATION_ACCESS_REQUIRED`. The per-client scope check still applies.
- `GET /api/scheduled-jobs/{id}` and `GET /api/scheduled-jobs/by-code/{code}`:
  admitted by `platform:messaging:scheduled-job:view` (unchanged), or by the
  application-service fire permission or `...:scheduled-job:sync` for a job of an
  application the caller may access. Any other job is `404` (not revealed).
- The list, instances and logs reads stay `platform:messaging:scheduled-job:view`.
- The audit event `platform:admin:scheduled-job:fired-manually` is unchanged on
  the wire; its principal is the application's service account.

## Principals by id: platform-level principals follow the list rule

Owner ruling 2026-11-06 (completes PR-3/PR-4). `GET /api/principals/{id}` and its
sub-routes `/version`, `/roles`, `/application-access` and `/available-applications`
apply the list's visibility rule (`PrincipalVisible`) to every principal: an
anchor sees all; a non-anchor sees principals homed at a client it can access
and partners granted one of its clients. A principal with no home client that
the caller may not see (an anchor user, a global service account, a partner of
other clients) answers the same `404 {"code":"Principal_NOT_FOUND","message":"Principal not found: <id>"}`
a missing id does. Before, these routes only checked access when the principal
had a home client, so a client administrator could read an anchor user's name,
email and roles by id. Reading yourself (`/{id}` and `/{id}/version` with your
own id) is unchanged. The SDKs' revocation check reads its own principal, and
fails open on any non-200, so it is unaffected.

## Dispatch jobs: time window, requeue guard, write permissions

**Time window.** The list-shaped reads are held to a `created_at` window:
`GET /api/dispatch-jobs` (and `/bff/dispatch-jobs`), `GET /api/dispatch-jobs/list-raw`
(alias `/raw`), `GET /api/dispatch-jobs/filter-options`,
`POST /api/dispatch-jobs/requeue-by-filter/preview` and
`POST /api/dispatch-jobs/requeue-by-filter`. The bounds are the existing RFC 3339
`since` / `until` (query on the GETs, body fields on the POSTs). Rules:

- neither given: `since` = now minus 24 hours (`defaulted: true`);
- `since` only: `since` .. now; `until` only: `until` minus 24 hours .. `until`
  (`defaulted: true`); both: as given, `until` before `since` is
  `400 INVALID_WINDOW`;
- a window wider than `FC_DISPATCH_JOB_MAX_WINDOW_DAYS` (default 31) is
  `400 WINDOW_TOO_LARGE` (`kind` validation);
- an unparseable `since` / `until` on the list is ignored as before (the default
  then applies); on filter-options and the POSTs it is `400 INVALID_TIMESTAMP`.

The applied window is echoed. The two list endpoints keep their bare-array body and
answer headers `X-Window-From`, `X-Window-To` (RFC 3339 UTC; "to" is now when no
`until` was given) and `X-Window-Defaulted` (`true`/`false`). `filter-options`,
the preview and the run carry `window: {from, to, defaulted}` in the body. Not
windowed (a single job or event already bounds them): `GET /api/dispatch-jobs/{id}`,
`/{id}/raw`, `/{id}/attempts`, `/event/{eventId}` (`/by-event/{eventId}`),
`POST /{id}/sign`, and the write actions on one job.

**Event type, subscription and connection filters (migration 086).** New filter
fields on the list (`GET /api/dispatch-jobs`, `/bff/dispatch-jobs`, `/list-raw`)
and, under the same names, in the bodies of `requeue-by-filter/preview` and
`requeue-by-filter`:

| field | meaning |
|---|---|
| `eventType` | exact event type; filters `code` (a job's `code` is its event type for an event-raised job and the caller's job code for an SDK or `fn.dispatch` job; no separate column exists) |
| `eventTypes` | CSV, at most 10 (`400 TOO_MANY_EVENT_TYPES`); with `eventType` and `codes` all must hold |
| `subscriptionIds` | CSV, at most 50 (`400 TOO_MANY_SUBSCRIPTION_IDS`); with `subscriptionId` both must hold |
| `subscriptionCode`, `subscriptionName` | exact; trailing `*` = prefix; name case-insensitive. Resolved to subscription ids first (within the caller's tenants), then `subscription_id = ANY(ids)`; no match = empty list; over 200 matches = `400 TOO_MANY_MATCHES` |
| `connectionId`, `connectionCode` | jobs of the connection's subscriptions plus jobs whose own `connection_id` is the connection (function dispatch). Code is resolved like the names (over 50 connections = `400 TOO_MANY_MATCHES`) |

`GET /api/dispatch-jobs/filter-options` gains `eventTypes` (same list as `codes`),
is cached per (tenant scope, window bucket) for 60 seconds, and still reads
distinct values from the window rather than the event-type catalogue (events may
be emitted unregistered; a direct job's code is not an event type).

Schema (all implementations sharing the database): `msg_dispatch_jobs_read`
gains `connection_id VARCHAR(17)` (projected verbatim from `msg_dispatch_jobs.connection_id`,
set in the projector's insert and its `ON CONFLICT DO UPDATE`; NULL for event-raised
jobs). Indexes on the read table: `(code, created_at DESC)` replaces the
single-column `code` index, `(subscription_id, created_at DESC)` replaces the
single-column `subscription_id` index, and a new partial
`(connection_id, created_at DESC) WHERE connection_id IS NOT NULL`.
Rows projected before the migration keep `connection_id` NULL until their next
projection (or the optional backfill in `scripts/ops/086-dispatch-read-indexes-online.sql`).

**Requeue guard.** `POST /api/dispatch-jobs/requeue` and
`POST /api/dispatch-jobs/requeue-by-filter` reset `PENDING`, `FAILED` (`ERROR`),
`CANCELLED`, `EXPIRED` jobs. `QUEUED`, `PROCESSING` (`IN_PROGRESS`) are never
reset. `COMPLETED` is reset only when the body has `redriveCompleted: true` and
`confirmRedriveCompleted: "REDRIVE"`. Skipped jobs are reported, never a batch
error. New request fields (both endpoints): `redriveCompleted` (bool),
`confirmRedriveCompleted` (string). New response fields: `redriven`,
`skippedInFlight`, `skippedCompleted` on both; `skipped: [{id, status, reason}]`
(`IN_FLIGHT` | `COMPLETED_REDRIVE_NOT_CONFIRMED`) on the by-ids response;
`window` on the by-filter response. The preview adds `completedCount`,
`inFlightCount`, `redriveEligibleCount` and `window`; `eligibleCount` now excludes
COMPLETED and in-flight jobs (send it as `confirmCount`; a redrive confirms
`redriveEligibleCount`). A run that would reset more than `confirmCount` is
`409 FILTER_MATCH_CHANGED`, whose `details` also carry `completedCount`,
`inFlightCount`, `redriveEligibleCount`.

The requeue emits one `platform:messaging:dispatch-jobs:resent` event whose data is
only `{ids: string[]}`. The redrive mark lives in the audit row's operation JSON
(`redriveCompleted`, `confirmRedriveCompleted`); the skipped counts are in the HTTP
response.

**Cancel guard.** `POST /api/dispatch-jobs/{id}/cancel` cancels an unfinished
(`PENDING`, `QUEUED`, `PROCESSING`/`IN_PROGRESS`) or `FAILED` (`ERROR`) job;
`COMPLETED`, `CANCELLED`, `EXPIRED` are `409 NOT_CANCELLABLE` (it was
`NOT_FAILED`, FAILED only, before 2026-10-30). A CANCELLED job is never claimed
for delivery, and a delivery already under way cannot move it on (complete and
fail move only live jobs), though that delivery may still land. `POST /{id}/complete` stays FAILED-only (`409
NOT_FAILED`). The admin UI's "Ignore selected" cancels many in one use case
(`CancelDispatchJobs`, no HTTP route), emitting one
`platform:messaging:dispatch-jobs:cancelled` event whose data is `{ids: string[]}`.
The admin UI's confirmed requeue of selected jobs may also reset in-flight
(`QUEUED`, `PROCESSING`) ones (`ResendCommand.requeueInFlight`, recorded in the
audit row's operation JSON; not on the HTTP API, and never by filter).

**Permissions.** `view` is read-only. `platform:messaging:dispatch-job:requeue`
gates both requeue endpoints (the preview also admits `view`);
`...:redrive-completed` is additionally required when `redriveCompleted` is true
(`403 PERMISSION_REQUIRED` otherwise); `...:cancel`, `...:complete`,
`...:edit-payload` gate `POST /{id}/cancel`, `POST /{id}/complete` and
`PUT /{id}/payload`. `/sign` and the raw reads stay `...:view-raw`; `/sign` also
requires anchor tier (`403 SCOPE_FORBIDDEN` otherwise, because it returns a real
signature of the job's body) and, for a caller confined to some applications, a job
of one of those applications (otherwise `404`). Migration 085
grants requeue, cancel, complete and edit-payload to every role holding `view`
except `platform:viewer`.

## Webhook signatures

Webhook delivery (router → subscriber) signs
`"{timestamp}{payload}"` with HMAC-SHA256 and emits:

- `X-FLOWCATALYST-SIGNATURE: <hex>` (lowercase hex, 64 chars)
- `X-FLOWCATALYST-TIMESTAMP: <ISO8601 with millisecond precision>` — format
  `%Y-%m-%dT%H:%M:%S%.3fZ` (3 fractional-second digits). Note this is
  **different** from the platform's general timestamp format (microseconds) —
  the webhook path specifically uses milliseconds here.

The router signs the payload **bytes it receives — it never re-serializes**
before signing, so JSON library choice cannot affect the signature. The one
place a serialized struct is signed is the mediation payload
(`{"messageId":"<tsid>"}` — a single-field object with no serializer
ambiguity).

Shared test vectors (`clients/rust-sdk/tests/fixtures/wire/webhook-signatures.json`)
pin the expected signature for fixed inputs: a shared SDK vector, a router
mediation payload with a millisecond ISO-8601 timestamp, and a Unix-seconds
timestamp with a UTF-8 body. The Go SDK's `pkg/fcsdk/webhook/vectors_test.go`
and the Rust and Java SDK tests assert against them. If a vector ever changes,
every side changes in lockstep.

Other signing sites are HMAC-over-plain-strings (dispatch-job auth token signs
the raw job id) or not HMAC at all (JWT RS256, Argon2id, AES-GCM) — none sign
serialized JSON.

### Dispatch-processing delivery request

`POST /api/dispatch/process` (`internal/platform/dispatchjob/processing`) is
the *other* subscriber-facing delivery path — the platform, not the router,
POSTs directly to the subscription's `target_url` (the router only invokes
this endpoint; it never sees the real webhook). The request:

- **Headers:** `Content-Type` (`application/json`; a `dataOnly` delivery sends
  the job's own, see below), `X-Dispatch-Job-Id`,
  `X-Event-Type` (the job's code), `Authorization: Bearer <token>` (when the
  resolved service account has one), and the same
  `X-FlowCatalyst-Signature`/`X-FlowCatalyst-Timestamp` pair described above
  — same byte format (HMAC-SHA256 over `timestamp+body`, millisecond ISO8601
  UTC timestamp), computed over the **body only**.
- **Which service account signs** (owner ruling 2026-09-22, reversing
  2026-09-19's application-first rule; `internal/server/delivery_creds.go`):
  the first of these that *names* an account decides, with no fall-through
  past it — (1) the subscription's own `serviceAccountId`, (2) the
  subscription's **connection's** `serviceAccountId` (the normal case: it is
  the account the connection form requires), (3) the application's oldest
  active service account, via the subscription's `applicationCode` or a
  direct job's `app:` code prefix. A named account that is inactive, has no
  webhook credentials, or holds one that cannot be decrypted (a wrong or
  rotated encryption key; the ciphertext is never sent) makes the delivery go out **unsigned with a reason**
  (never silently signed by another account); the reason is logged and, on a
  failed attempt, appended to the attempt's `errorMessage` as
  `(delivered unsigned: …)`.
- **Who may name the signer** (security fix, 2026-11): the credentials a
  delivery carries belong to the account that signs it, so whoever sets or
  changes the URL, or the credentials, of signed work must be allowed to use
  them. `POST`/`PUT /api/subscriptions` (a `serviceAccountId` or `connectionId`,
  or a changed `endpoint`), `POST`/`PUT /api/connections` (`serviceAccountId`)
  and `POST /api/scheduled-jobs` / `PUT` (`applicationId`, changed `targetUrl`)
  answer `403` when they are not: `SCOPE_FORBIDDEN` for a service account the
  caller may not use (an anchor may use any account; anyone else only an account
  linked to clients they can all access, and to no application they cannot
  access; an account linked to no client, such as an application's own, is
  anchor-only; a caller confined to some applications only an account bound to
  one of them), `APPLICATION_ACCESS_REQUIRED` when the application's own
  account would sign (a subscription with an `applicationCode` and neither an
  account nor a connection, or an application-stamped scheduled job) and the
  caller is not an anchor with access to that application. A named account that
  does not exist is `400 SERVICE_ACCOUNT_NOT_FOUND` (inactive:
  `SERVICE_ACCOUNT_INACTIVE`); a `connectionId` that does not exist is
  `404 CONNECTION_NOT_FOUND`, and one scoped to another client
  `400 CONNECTION_SCOPE_MISMATCH`. A value an update leaves unchanged is not
  re-checked. Rows that already exist keep delivering as before.
- **`X-FlowCatalyst-Client: {clientId}:{clientCode}`** (owner ruling
  2026-09-18, `docs/spec/webhook-client-code.md`) — names the tenant a
  multi-tenant subscriber's endpoint is receiving a delivery for. Sent for
  **every** delivery whose job carries a `client_id` that resolves to a
  `tnt_clients.identifier`, including `dataOnly` deliveries (whose raw body
  has no envelope to carry a `clientCode` field). **Never a half pair**: a
  platform-scoped job (no `client_id`) or a client that does not resolve
  omits the header entirely. Adding this header does not change what the
  signature covers — it is still the body alone.
- **`Content-Type`:** `application/json` for the envelope. A `dataOnly`
  delivery sends the job's own `payloadContentType` (`application/json` when
  it has none), since its body is the raw payload.
- **Body:** the raw payload byte-for-byte in `dataOnly` mode; otherwise a
  CloudEvents-style envelope — `id`, `type`, `attemptNumber`, `time` (the
  job's creation time, UTC, RFC 3339 with milliseconds), and, when
  present, `dataContentType` (the job's payload content type), `source`, `subject`, `correlationId`, `messageGroup` (the event's own group, not the per-subscription key the job is ordered by), `clientId`,
  `clientCode` (identifier slug, alongside `clientId`, omitted — the key
  absent, not null — when the job has no `clientId` or the client cannot be
  resolved), and `data` (the parsed or raw payload).

Client-identifier resolution is cached in memory
(`client.NewCachedIdentifierResolver`): a hit is permanent for the process's
life (identifiers are immutable after create), a miss is cached for a
bounded window (1 minute, same as the delivery-credentials cache below) so a
client created after an initial miss still resolves on a later delivery, and
a repository failure resolves to "unknown" rather than failing the delivery.

### Mediation response

The target answers the mediation POST with 2xx and an optional JSON body.
An absent/unparseable body on a 2xx means plain success.

| Field | Type | Meaning |
|---|---|---|
| `ack` | bool | `false` = don't treat as delivered; requeue with backoff. Default `true`. |
| `delaySeconds` | uint32 | With `ack:false`, the minimum requeue delay. With `flushGroup`, the suppression window. |
| `notBefore` | string (RFC 3339) | With `ack:false`, WHEN to redeliver: no earlier than this instant. Takes precedence over `delaySeconds` when it parses (the router waits until it, rounded up to a whole second, at least 1 s); an unparseable value is ignored. Added 2026-11-09; a router that predates it reads `delaySeconds`, so a target should send both. |
| `flushGroup` | bool | `true` = delivered, and stop delivering this message's group. Default `false`. |

A named delay (`notBefore` or `delaySeconds`) is honoured exactly: on SQS and the
Postgres queue the message is handed back to the broker with that visibility delay on
its first occurrence (SQS caps a message's total invisibility at 12 hours from its
receipt; a longer wait comes back early and is asked again). On NATS, which cannot hold
a message back without reordering its group, the router waits in memory for the named
delay, at most 60 s at a time. Without a named delay the router's deferred curve
(5 s, doubling, 60 s cap) applies. The platform's own callback answers
`{"ack": false, "notBefore": "...", "delaySeconds": n}` for a job whose subscription's
`delaySeconds` has not elapsed (see "Dispatch delivery" below). The same field works for
a subscriber's own deferral on the platform's delivery path: a 2xx
`{"ack": false, "notBefore": "..."}` reschedules the job to that instant
(`delaySeconds` otherwise, else 30 s), and a 429's `Retry-After` is read as
delay-seconds or an HTTP-date (else 30 s), without spending a retry.

`flushGroup` semantics: the router ACKs the group's remaining messages
**without delivering them**, spending no rate-limit token and no concurrency
slot. It is therefore only safe for targets that already own the records the
messages point at (the message-pointer pattern) and re-drive them themselves —
a target whose message body carries the only copy of the payload must never
set it. Suppression is TTL-bounded (`delaySeconds`, defaulting to 60s and
capped at 5 minutes), so it self-heals: once the window lapses the next
message probes the target, which either flushes again or resumes. `ack:false`
takes precedence — a target that wants the message back cannot also discard
its group.

## Dispatch delivery: groups, delay, age, pools, retries (2026-11-09)

Owner rulings 2026-11-09; enforced by the event fan-out (`internal/stream/fan_out.go`),
the scheduler and `POST /api/dispatch/process`. Router side: `docs/router-architecture.md` §4.

- **Message group per subscription.** A job the event fan-out raises is ordered in
  `<subscription id>:<event message group>` (stored in `message_group`; when that would
  exceed 200 characters, `<subscription id>:sha256:<hex digest of the event group>`). One
  subscriber's `BLOCK_ON_ERROR` failure blocks only that subscriber's later jobs, until
  resolved; other subscribers of the same event group keep flowing. An event with no
  group raises unordered jobs, as before. The delivery envelope's `messageGroup` is still
  the event's own group. A job created through the SDK routes, a function or an
  ingestion keeps the group it was given. Visible on the wire as the job's
  `messageGroup` in the dispatch-job API (and its `messageGroup` filter, exact match).
- **`delaySeconds` and `maxAgeSeconds`** (subscription fields, stored but ignored until
  now) are copied onto each job at fan-out, made absolute against the event's
  `createdAt`: `not_before = createdAt + delaySeconds` (column added by migration 109; not
  on the API), `expiresAt = createdAt + maxAgeSeconds`. Zero means none. A later edit of
  the subscription does not move jobs already raised. The callback answers a job reached
  before `not_before` with `200 {"ack": false, "notBefore", "delaySeconds"}` (at most 5
  minutes per answer; the job stays `QUEUED`), and settles a job reached at or after
  `expiresAt` as **`EXPIRED`** (last error `expired: not delivered by <time>`, no attempt
  counted) and acks it. `expiresAt` set any other way is enforced the same way. Note the
  column default `maxAgeSeconds` = 86400: an event-raised job not delivered within 24
  hours of its event — for example one held behind a `BLOCK_ON_ERROR` failure for longer —
  now expires instead of delivering. A requeue (`EXPIRED` is requeueable) clears
  `expiresAt`: an operator's requeue is a delivery now.
- **Dispatch pool status.** `SUSPENDED`: the pool's jobs wait — the scheduler does not
  claim them, and the callback returns any already queued to `PENDING` (no attempt) and
  acks them; resuming the pool releases them in order. `ARCHIVED`: no new job may name the
  pool — `POST /api/dispatch-jobs` and `/batch` answer `409 POOL_ARCHIVED` (a batch is
  refused whole), subscription create and update, and an SDK sync newly binding a
  subscription to it, answer `409 POOL_ARCHIVED` (functions and ingestion already refused
  any non-`ACTIVE` pool); jobs that already name it wait like a suspended pool's. The
  router-config document still lists every pool.
- **`retryStrategy`** (singular `POST /api/dispatch-jobs` only; every other job is
  `exponential`): the wait before the next attempt after a retryable failure.
  `exponential` 5 s, 15 s, 30 s, 60 s, then 120 s; `fixed` 30 s every time; `immediate`
  none (the next scheduler poll). `IMMEDIATE` and `FIXED_DELAY` stay accepted aliases.
- **Event-type binding `filter`** is refused unless absent or blank:
  `400 FILTER_NOT_SUPPORTED` on subscription create and update and on the SDK
  subscription sync. No filter syntax was ever defined, no column stores it, and the
  fan-out never applied it, so a filter that was sent was dropped and every event of the
  type delivered. SDKs that can send one: TypeScript, Laravel, Java, Rust, Go.
- **`dataOnly` default** for a new subscription is `false` (the envelope), the documented
  default, on every path: the create API and console now match the SDK sync. Existing
  subscriptions keep their value; the column default is `false` from migration 110.

## Delivery URLs: what the outbound guard refuses (2026-11-22)

A subscription endpoint, a dispatch job's or scheduled job's `targetUrl` and an
ingestion's `target.targetUrl` must be an absolute `http` or `https` URL with a
host and no embedded credentials. Its host may not be `localhost` (or a
`.localhost` name) or a literal address that is loopback, private or reserved
(RFC 1918, `fc00::/7`, `100.64.0.0/10`), link-local (including
`169.254.169.254`), the IPv6 metadata address `fd00:ec2::254`, unspecified or
multicast; an IPv4-mapped IPv6 address is judged as its IPv4 address. **An IPv6
address with a zone** (`http://[fe80::1%25eth0]/`, `http://[::%25lo0]:8080/`) is
refused whatever the address (F63): the rules judge the address without its
zone, and the zone, which names a network interface, is refused on its own. The
write answers `400` (`INVALID_ENDPOINT` for a subscription, `INVALID_TARGET_URL`
for a dispatch or scheduled job; an ingestion's problem names
`target.targetUrl`). A host name is not resolved at the write; the address a
delivery actually dials, after DNS, is checked by the same rules, and a refused
dial fails the attempt as a connection error. The operator's
`FC_DELIVERY_ALLOW_LOOPBACK` and `FC_DELIVERY_ALLOW_PRIVATE` open loopback and
private addresses but never a zone, link-local, metadata, unspecified or
multicast address; a `host:port` exempted by `FC_DELIVERY_ALLOW_HOSTS` skips
the checks (`docs/environment-variables.md` §5a).

## Payloads in the payload table: what a reader returns (2026-11-20)

`docs/payload-offload-design.md`, sections 7, 10 (step 1) and 11. From migration 123 an
event row (`msg_events`, `msg_events_read`) or a dispatch job row (`msg_dispatch_jobs`)
may carry a `payload_id`; its payload is then the `msg_payloads` row with that id under
the row's own `created_at`: the producer's bytes (`data`), or a reference into the
payload store (`ref`), with its `size`. A row without one keeps its payload where it was
(`msg_events.data`, `msg_events_read.data`, `msg_dispatch_jobs.payload`). Every reader
accepts both; until `FC_PAYLOAD_TABLE_WRITES` is turned on (the next section) no row has
a `payload_id` and nothing below differs from before except the new `dataSize`. All new fields are
optional and omitted when they say nothing.

- **Lists never read the payload store.** A list joins each row of its page to its
  payload row by primary key; a payload held in the database is returned as today, an
  offloaded one is left out. `GET /api/events` (and `/list-raw`, `/raw`,
  `/bff/events`): each item gains `dataSize` (bytes; for a legacy row the length of its
  stored data) and `dataOffloaded: true` when the data is in the payload store. The
  debug list `GET /bff/debug/events` returns `data` as before and leaves an offloaded
  one out with `dataOffloaded: true`, beside `dataSize`. `GET /bff/debug/dispatch-jobs`'
  `payloadLength` is the payload row's `size` (the payload is not returned there).
- **A read of one record returns the payload resolved.** `GET /api/events/{id}`
  (`/bff/events/{id}`), `GET /bff/debug/events/{id}`, `GET /api/dispatch-jobs/{id}`,
  `/{id}/raw`, and the job returned by `POST /{id}/cancel`, `/{id}/complete` and
  `PUT /{id}/payload` read an offloaded payload from the store. When it cannot be read
  (the store failed, or the payload is gone) the record is still returned, without its
  payload: `dataUnavailable: true` on an event (with `dataSize`), `payloadUnavailable:
  true` on a job. Never a 500. Data held in the payload table is the producer's bytes;
  the JSON response embeds an event's `data` as a JSON value (compacted, key order and
  repeated keys as sent), a job's `payload` as a string, byte for byte.
- **`POST /api/dispatch-jobs/{id}/sign`** builds the plan over the resolved payload;
  when it cannot be read: `409 PAYLOAD_MISSING` (gone) or `500 PAYLOAD_UNAVAILABLE`
  (try again).
- **`PUT /api/dispatch-jobs/{id}/payload`** reads the payload it replaces for the audit
  event's `previousSize` / `previousSha256`. A store that cannot be read refuses the
  edit with `500 PAYLOAD_STORE_UNAVAILABLE` (try again); a payload that is gone does not
  block the edit (`previousSize` is the payload row's size, `previousSha256` empty).
- **Delivery** (`POST /api/dispatch/process`). The body of a job with a `payload_id` is
  built from its payload row: in `dataOnly` mode exactly the payload row's bytes,
  otherwise the envelope, whose `data` is parsed from them. An offloaded payload is read
  before the job is claimed. When the store cannot be read, or is not configured, it is
  a platform fault: no attempt is recorded or counted, the job is not moved, and the
  callback answers `503 {"ack": false, "message": "payload store unavailable"}` (or
  `"payload store not configured"`) like a failed load, so the queue redelivers. When
  the payload is gone (its row or object missing, or not matching its reference) the
  job is settled **`FAILED`** with a last error starting `PAYLOAD_MISSING`, no attempt
  counted and nothing sent, and the message is acked (`{"ack": true, "message":
  "payload missing"}`); it is not retried. Like any failure it holds a `BLOCK_ON_ERROR`
  group until an operator resolves it (edit the payload and requeue, or cancel).

## Payloads in the payload table: what a writer answers (2026-11-20)

`docs/payload-offload-design.md`, sections 3, 7, 8 and 10 (step 2). Behind
`FC_PAYLOAD_TABLE_WRITES` (default `false`). With it off every route answers as
before except for the payload edit's cap (last item) and the request caps and
per-item limit of the next section. With it
on, event ingest, the platform's own events, directly created dispatch jobs and the
payload edit store each payload as a `msg_payloads` row (offloaded above
`FC_PAYLOAD_OFFLOAD_MIN_BYTES` when a store is set), and the rows that point at it
carry `payload_id` with their own payload column NULL. Response shapes do not change;
these are the new outcomes:

- **Batch routes** (`POST /api/events/batch`, `/bff/events/batch`,
  `POST /api/dispatch-jobs/batch`). Results stay positional, one per item, and the
  items that can be stored are. An item whose payload is refused answers in its own
  slot: `BAD_REQUEST` with an error starting `PAYLOAD_TOO_LARGE` (larger than
  `FC_PAYLOAD_MAX_BYTES` with a store, `FC_PAYLOAD_INLINE_MAX_BYTES` without) or
  `PAYLOAD_NOT_TEXT` (not UTF-8, or a NUL byte), both terminal for the outbox
  processor; or `INTERNAL_ERROR` with an error starting `PAYLOAD_STORE_UNAVAILABLE`
  (the store could not be written and the payload is larger than the database takes),
  which the processor retries. Before, a payload the database refused failed the
  whole request.
- **Single routes.** `POST /api/events` and `POST /api/dispatch-jobs`: `400
  PAYLOAD_TOO_LARGE`, `400 PAYLOAD_NOT_TEXT`, `503 PAYLOAD_STORE_UNAVAILABLE` (try
  again). `POST /control/functions/events` and `/control/functions/dispatch-jobs`
  answer the same codes (the 256 KiB cap of a function's dispatch stays).
- **Ingestion runtime** (`POST /api/ingestion-runtime/v1/ingestions/{id}/jobs`): a refused record
  answers in its `results[].error`, starting with the same three codes, and makes no
  job or event; the others are made. A record refused `PAYLOAD_STORE_UNAVAILABLE`
  should be sent again.
- **An event with no data** is stored with the data `{}`, as before, so what its
  subscribers receive is unchanged.
- **Data-only deliveries** of a payload written this way carry the producer's bytes
  (section 3.2), not the JSONB rendering.
- **`PUT /api/dispatch-jobs/{id}/payload`** (switch on or off): the cap is the largest
  payload the deployment accepts (`FC_PAYLOAD_MAX_BYTES` with a store,
  `FC_PAYLOAD_INLINE_MAX_BYTES`, 1 MiB by default, without; with the switch off it is
  always the inline maximum), up from 256 KiB; the route's request body cap is the
  next section's. With the switch on the edited job gets a
  payload row of its own and the event and its other jobs keep the original; with it
  off the payload is written to the job's payload column and its `payload_id`
  cleared. A store that cannot be written refuses the edit with `500
  PAYLOAD_STORE_UNAVAILABLE` (try again). The audit event
  `platform:messaging:dispatch-job:payload-edited` gains two optional fields
  (migration 124 adds them to the seeded 1.0 schema, which stays 1.0):
  `previousPayloadId`, the payload row the job pointed at before the edit (absent when
  its payload was the job's payload column), and `payloadId`, the row the edit gave it
  (absent with the switch off, when the payload column is written).

## Request body caps and the largest payload (2026-11-20)

`docs/payload-offload-design.md`, section 8. Before, every route that takes a
payload had huma's default request cap of 1 MiB (the dispatch-job routes, plain chi
handlers, had none), so no payload could be larger and a batch of several was
refused whole once it passed 1 MiB. Now:

- **The largest payload** is `FC_PAYLOAD_MAX_BYTES` (5 MB) while payloads are
  offloaded, which needs a payload store, a threshold that is not `0` and
  `FC_PAYLOAD_TABLE_WRITES` on; otherwise `FC_PAYLOAD_INLINE_MAX_BYTES` (1 MiB),
  because every payload is kept in the database. It holds whatever the switch says:
  a larger payload is refused `PAYLOAD_TOO_LARGE` (per item on a batch route,
  `BAD_REQUEST` in its own slot with the rest of the batch stored; `400` on a
  single route), before either shape is written.
- **Batch routes** (`POST /api/events/batch`, `/bff/events/batch`,
  `POST /api/dispatch-jobs/batch`): request cap `FC_INGEST_BATCH_MAX_BYTES`
  (16 MiB), at least twice the largest payload.
- **Routes that carry one payload** (`POST /api/events`, `POST /api/dispatch-jobs`,
  `PUT /api/dispatch-jobs/{id}/payload` and `/bff/dispatch-jobs/{id}/payload`):
  request cap twice the largest payload plus 64 KiB (a payload sent as a JSON string
  can nearly double when escaped): 2 162 688 bytes by default, 10 065 536 while
  offloading at the defaults.
- **A body over its route's cap** is refused `413` with
  `{"error":"BODY_TOO_LARGE","message":"request body is too large limit=<n> bytes"}`,
  on every huma route (it was a `400 VALIDATION` with the same message) and on the
  two dispatch-job routes.
- Unchanged: the ingestion runtime's caps (`FC_INGESTION_JOBS_MAX_BYTES`, 10 MiB;
  `FC_INGESTION_REPORT_MAX_BYTES`, 4 MiB; traces 1 MiB), the 256 KiB cap of a
  function's `dispatch.create`, and huma's 1 MiB on every other route
  (`POST /control/functions/events` included).
- **The outbox processor** sends at most `FC_OUTBOX_MAX_REQUEST_BYTES` (8 MiB) a
  request, cut by rows and bytes; a `413` on several rows is split in two and the
  halves sent, a `413` on one row is terminal (`PAYLOAD_TOO_LARGE`). A grouped row
  is always sent alone, so neither changes a message group's order.

## The payload store's health (2026-11-20)

`docs/payload-offload-design.md`, sections 7 and 13. Not in the OpenAPI document
(plain routes, like `/health`).

- **`GET /health/payload-store`** on the API listener of a process that runs the
  platform API, without credentials, always `200`:
  `{"status", "configured", "kind"?, "reachable"?, "offloading", "lastSuccessAt"?,
  "lastFailureAt"?, "lastFailure"?, "windowSeconds"}`. `status` is `NOT_CONFIGURED`
  (no payload store: a healthy state), `DEGRADED` (a write of the store failed, so its
  payloads were kept in the database or refused, or a read failed, within the last
  `windowSeconds`, 300) or `UP`. `reachable` is the outcome of the last store
  operation (a missing object counts as reachable), absent until there is one;
  `lastFailure` is `put`, `get` or `startup`; `kind` is `s3`, `file` or `memory`,
  never the bucket or directory. It is derived from the outcomes of real operations
  and one check at startup (the file store writes and removes a temporary file; the S3
  store records only a credential chain that did not resolve), never a store call per
  request.
- **The metrics listener's `/ready`** gains `"payload_store": "<status>"` in such a
  process. It stays `"status": "ready"` and `200` whatever the store's status: a store
  that is down never makes an instance unready or fails liveness, because ingest falls
  back to the database.

## Function-runner control plane: runtimes and capabilities

The runner-facing routes under `/control/functions/` (`desired`, `heartbeat`,
`artifacts/{digest}`, `events`, `dispatch-jobs`, `token`, `trace-log`) are the Go v1 control
protocol. Additions for runners of runtimes beyond the built-in `wasm` and
`js` (all additive; a runner that predates them keeps working unchanged):

- `GET /control/functions/desired?pool=&runtimes=a,b` — `runtimes` lists the
  runtimes the runner can run (tags: lower-case, a letter first, 2-16
  characters). Only versions of those runtimes are returned, and a function
  whose versions are all filtered out (with its routes) is omitted. Absent or
  empty means `wasm,js`, so a runner that never sends it never receives a
  version it cannot run. A malformed list is `400 RUNTIMES_INVALID`. Each
  runtime set has its own `ETag`.
- `POST /control/functions/heartbeat` — optional `capabilities`
  (`{protocol, runtimes[], runnerKind, version, extra{}}`), stored with the
  heartbeat report. A report without it counts as a `wasm,js` runner.
  `capabilities.runtimes` must be non-empty valid tags (`400
  CAPABILITIES_INVALID`).
- Promoting (`PUT .../aliases/{name}`) a version whose runtime no live runner
  of the function's pool advertises (heartbeat within 60s) is refused
  `409 POOL_LACKS_RUNTIME`. For the built-in runtimes the guard applies only
  once the pool has at least one live runner.
- `POST /control/functions/token` — `{functionId, version}` →
  `200 {accessToken, tokenType: "Bearer", expiresIn}`: the access token of the
  service account linked to the function's application (the same bearer its
  `client_credentials` grant gives), for a version whose describe declares
  `platformToken: true`. Runner role and anchor scope. Refusals: `400
  FUNCTION_ID_REQUIRED`, `404` function or version, `403
  PLATFORM_TOKEN_NOT_DECLARED`, `409 APPLICATION_SERVICE_ACCOUNT_REQUIRED`, `400
  SERVICE_ACCOUNT_INACTIVE`. Every mint is audited (`TOKEN_MINTED_FOR_FUNCTION`);
  the token is never logged. `docs/function-platform-token.md`.
- `POST /control/functions/trace-log` — a runner ships trace records
  (`{runnerId, pool, records[]}`; `docs/function-trace-design.md` section 5):
  `202 {accepted, dropped}`. At most 500 records and 1 MiB per batch, per
  function 200 records/second per platform process (an in-memory bucket each,
  so N replicas take up to N times that), a record is taken only while its
  `(function, context)` window is live (plus 10 s); switching off sets the
  window's end to the switch-off time, so records captured just before are
  still taken. A record's `at` more than a minute ahead of arrival is clamped to
  arrival, one more than an hour behind is dropped; retention counts from the
  platform's `created_at`. A record may carry `id` (the runner's UUID, the same
  on every retry): the platform stores it once (`ftl_r<id>`); without one
  (an older runner) it assigns an id. Unknown fields are accepted at every
  depth (a newer runner may add them). The desired document's function entry
  carries `trace` (the live windows). Host op 14 `trace.reference`
  (`{reference}`) sets the invocation's business key on the runner.
- Function trace API: `PUT /api/functions/{id}/trace/{context}` with
  `indefinite: true` needs `platform:function:trace:indefinite`, is refused
  `400 TRACE_INDEFINITE_NOT_ALLOWED` for `http`, and opens a window ending
  `indefiniteMaxDays` (default 7) days later; context state carries
  `indefiniteMaxDays` (0 for `http`). `DELETE` leaves `enabledUntil` at the
  switch-off time. `GET .../trace-log` without
  `platform:function:trace:view-bodies` returns records with `redacted: true`:
  an http record's data keeps only method, path, status, durationMs, outcome
  and body sizes (bodies null, `bodiesRedacted: true`), a log record has an
  empty message and null data, a trace record null data. The
  `platform:function:trace:enabled` event always carries `indefinite`.
- Delivery URLs for subscriptions, scheduled jobs and function dispatch use
  `FC_FUNCTIONS_RUNNER_URL_<RT>` when set for the version's runtime (see
  `docs/environment-variables.md`).
- The `jvm` runtime (owner ruling 2026-10-31, `docs/jvm-functions-in-ingestor.md`):
  an artifact that is a jar (zip magic) is published with runtime `jvm`; its
  describe document is read, without executing anything, from the entry
  `META-INF/flowcatalyst/function.json` (at most 1 MiB uncompressed, at most
  50,000 entries): the ordinary describe document plus `jvm: {entrypoint}`, the
  class implementing the function interface. A jar without the entry, or whose
  document is invalid or has no `jvm.entrypoint`, is refused like a bad Wasm
  describe, and a `jvm` block in a non-jar artifact is refused. Whether a
  function is JVM is fixed by its first version: publishing a `jvm` artifact for
  a function whose versions are not `jvm` (or the reverse) is `409
  RUNTIME_MISMATCH`; `wasm` and `js` may still follow each other. The runner is
  the ingestor runtime's function host, which sends `runtimes=jvm` and
  `capabilities.runnerKind` `ingestor-jvm`.
- Invocation, both runners: a `webhook` endpoint of a function whose desired
  document carries no webhook secret yet (the signing account came after the
  runner's last document) answers `503 WEBHOOK_SECRET_PENDING` with
  `Retry-After: 5`, not `401`: a `401` is never retried and the delivery would be
  lost, a `503` is retried once the next document has the secret.

## Ingestion: the HTTP listener and the runtime jobs route

`POST /api/ingestion-runtime/v1/ingestions/{id}/jobs` takes the records
(`payload`, `payloadContentType`, `messageGroup`, `externalId`, `correlationId`,
`subject`) and answers `{kind, created, results[]}` with `kind` `DISPATCH_JOB` or
`EVENT` (the ingestion target's kind). `results[]` is positional: `{index, id,
existing?}` or `{index, error}`; for `EVENT`, `id` is an event's id. An event is
stored through the `POST /api/events/batch` insert path with source
`ingestion:<application>:<code>`, the ingestion's client and, when the record has an
`externalId`, the deduplication id `ing:{ingestionId}:{externalId}`; a repeat
answers the stored event's id with `existing: true`. Event data is the record when
its content type is JSON and it parses; else `{"contentType","body"}` when the
target has `wrapNonJson`, else that record's result is an error.

The runtime's HTTP listener (`POST /ingest/{client}/{code}`, or
`/ingest/{application}/{client}/{code}`) answers `202 {"accepted":true,"jobId"}`
(split `none`) or `"jobIds"` in record order, `eventId` / `eventIds` for an event
target, `503 UNAVAILABLE` (send it again), `422 REFUSED` / `UNPROCESSABLE`, `429
RATE_LIMITED` with `Retry-After`, and the refusals `401`, `403 FORBIDDEN`, `404`,
`409 AMBIGUOUS`, `415 UNSUPPORTED_MEDIA_TYPE`, `400 EMPTY_BODY | INVALID_JSON |
PAYLOAD_MUST_BE_A_NON_EMPTY_JSON_OBJECT | INVALID_XML | INVALID_MESSAGE_GROUP |
INVALID_CORRELATION_ID`, each as `{"error": CODE, "message"?}`. The request headers
`Idempotency-Key`, `X-Message-Group` (the records' group becomes
`{ingestionId}:{value}`) and `X-Correlation-Id` are honoured. See "HTTP
ingestions" in `docs/published/80-ingestion.md`.

## Ingestion: the runtime's instance and lease release

Additive within contract version 2 (`docs/ingestion-design.md` section 7, "Robustness"):

- `POST /api/ingestion-runtime/v1/poll` takes an optional `instanceId` (8 to 64 of
  `[A-Za-z0-9-]`, a random UUID per runtime process). While another instance that
  polled as the same `runtimeId` is live (polled within one lease and has not said
  goodbye since), the poll answers `409 {"error":"RUNTIME_ID_IN_USE","message":"runtime
  id R is in use by another live process"}` and changes nothing. A poll without it is
  not checked.
- `POST /api/ingestion-runtime/v1/leases/release` (operation
  `ingestionRuntimeReleaseLeases`, the poll's auth) takes `{runtimeId, instanceId?,
  ingestionIds: [string] (0..1000), goodbye: boolean}` and answers `200 {"released":
  int}`: the leases among `ingestionIds` the runtime holds end now (others are ignored
  and not counted), and `goodbye` marks the runtime gone (no longer live). An
  `instanceId` other than the stored one is `409 RUNTIME_ID_IN_USE`, changing nothing.
  An older platform answers `404`; the runtime then stops calling it until restart.
- `runtimeNotice` on `GET /api/ingestions` and `GET /api/ingestions/{id}` is either
  `no runtime carries this kind` or, for a running polled ingestion no runtime has held
  for one lease and two minutes, `not running: no runtime has held it since <RFC 3339
  time>`.

## Field identity: the admin API (`/api/field-*`)

The field-worker identity plane (workers with a PIN, enrolled devices) is a separate
population from principals and portal users. Design: `docs/field-worker-identity-design.md`;
build spec: `docs/field-worker-identity-p1-spec.md`; sign-in options (unenrolled devices, badges,
impersonation, test identities): `docs/field-worker-sign-in-options-spec.md`. This section is the admin surface;
the device-facing `/field/*` protocol is the next section.

**Authorization.** Every route needs a permission AND access to the target client:
`platform:iam:field-worker:view|manage` (workers), `platform:iam:field-device:view|manage`
(devices, enrolment codes, policy), `platform:iam:field-token:introspect`, and
`platform:iam:field-worker:impersonate` (granting `canImpersonate`, console sign-in codes for real
workers). Scope alone grants nothing (an anchor needs the permission too). The role
`platform:field-administrator` holds all but introspect. By-id routes answer `404` (`FieldWorker_NOT_FOUND` / `FieldDevice_NOT_FOUND`)
for a resource of a client the caller cannot reach, and `403 PERMISSION_REQUIRED` without the
permission; routes that take a `clientId` answer `403 SCOPE_FORBIDDEN`. An application-confined
service account (an explicit application list) may only name its own applications
(`403 APPLICATION_FORBIDDEN` otherwise), sees and acts on workers/devices that share one of
its applications (others are `404`), must name its application in `appCodes` when ensuring a
worker, may grant its own application to a worker it cannot yet see, and may not change the
client's policy. Application codes resolve to active applications, else `400 UNKNOWN_APPLICATION`.

**Workers.** `POST /api/field-workers {clientId, workerCode?, idempotencyKey?, name, email?, phone?, appCodes?[], pin?}`
is an idempotent ensure on `(clientId, workerCode)` (the code is stored upper-case): `201` with
`created:true` for a new worker, `200` with `created:false` for an existing one, whose fields
(including any PIN) are not overwritten; `appCodes` grants are added.

*Generated codes.* Without a `workerCode` (absent or blank) the platform generates one and the
response's `workerCode` carries it: three letters from `BCDFGHJKMNPQRSTVWXZ` (no I, O, L, no vowels
A, E, U, Y; a short list of offensive triples is never drawn), then `workerCodeDigits` digits from
`23456789` (the client's policy, 3 to 6, default 3), for example `KTM482`; drawn at random. The
unique `(client_id, worker_code)` key decides: a drawn code already held (typed or generated, in any
case) is drawn again, at most 10 times, then `409 WORKER_CODES_EXHAUSTED` (raise
`workerCodeDigits`, or give a code). The `platform:iam:field-worker:created` event carries
`workerCodeGenerated: true`. Codes per client: 3 502 592 with 3 digits, 28 020 736 with 4,
224 165 888 with 5, 1 793 327 104 with 6.

*Repeating a creation without a code.* With a code the request is keyed and safe to repeat. Without
one it is keyed only by `idempotencyKey` (at most 100 characters, `400 INVALID_IDEMPOTENCY_KEY`
otherwise; unique within the client): a repeat with the same key answers the worker the first one
made (`200`, `created:false`, grants added as for an ensure), also when the two race. Without a key
every request is a new worker, so the SDKs do not resend it after an unknown outcome (A.9 of
`docs/sdk-audit-and-python-sdk.md`); with a key they mark it repeatable. A key with a `workerCode` is
`400 IDEMPOTENCY_KEY_WITH_WORKER_CODE`. The key is stored on the worker and never returned. A supplied `pin` must pass
the client's PIN policy (`400 PIN_POLICY`, `details.violations[]`) and is stored with
`pinChangeRequired:true`; without one the worker has no PIN and cannot sign in until a reset.
Responses carry `phone?` (optional contact number, see the 2026-11-08 hand-off), `hasPin`, `pinChangeRequired`, `locked`, `hardLocked`, `lockedUntil?`,
`failedSinceSuccess`, `apps[]`, `canImpersonate`, `isTest`, `linkedPrincipal` (`{id, name, email}` of the
linked platform user, else `null`; see "Link to a platform user"), `badge` (`{id, status, createdAt, lastUsedAt?}`
for the ACTIVE badge, else `null`) and never the hash. Ensure (new worker only) and PUT accept
`canImpersonate?` and `isTest?`: setting or changing `canImpersonate` needs
`platform:iam:field-worker:impersonate` in addition to manage (`403 PERMISSION_REQUIRED`), and clearing
it revokes the sessions the worker holds open as an actor (`ACTOR_SUSPENDED`). Suspending or deleting a
worker does the same. Changing `isTest` from false to true on an existing worker needs the rule for signing in
as a real worker (`impersonate` and a human caller, else `403`); true to false expires the worker's unredeemed
sign-in codes in the same transaction. `GET /api/field-workers` also takes `isTest=true|false` and `linkedPrincipalId`. `GET /api/field-workers?clientId&q&appCode&status&page&pageSize`
(`page` 0-based, `pageSize` default 100 max 1000; `q` is a case-insensitive prefix of name or
worker code) answers `{workers, total, page, size}`; `GET|PUT|DELETE /api/field-workers/{id}`
(PUT takes `{name, email?, phone?}`; DELETE revokes the worker's sessions and removes the row).
`POST /{id}/pin-reset {pin?}` answers `{temporaryPin?, pinChangeRequired:true}`: with no `pin`
the platform generates a policy-valid PIN, returned this once and recorded nowhere readable
(not in events or audit); every session of the worker is revoked (`PIN_CHANGED`) and the lock
state cleared. `POST /{id}/unlock` clears both locks and the failure counter. `POST /{id}/suspend`
sets `DISABLED` and revokes all sessions; `POST /{id}/activate` reverses the status.
`POST /{id}/apps {appCode}` grants; `DELETE /{id}/apps/{appCode}` revokes and revokes that
application's sessions for the worker; both idempotent. `GET /{id}/sign-ins?page&pageSize` lists
sign-in attempts newest first (`{signIns, total, page, size}`; each has `authMethod` (`PIN`,
`BADGE_PIN`, `BADGE`, `ACT_AS`, `SIGN_IN_CODE`, `PIN_CHANGE`), `deviceId?` (absent for an unenrolled key)
and `actorId?`, the supervisor or principal behind an impersonated sign-in).

**Link to a platform user** (`docs/field-native-session-spec.md` section 5). Audit correlation only: it says
"this dispatcher on the web is that driver on the phone" and grants nothing on either side (a field token
stays a field token; the platform user gains nothing on field routes). `PUT /api/field-workers/{id}/link
{principalId}` and `DELETE /api/field-workers/{id}/link` need `platform:iam:field-worker:manage` for the
worker's client (client-boxed like the other worker routes: another client's worker is `404`) and answer the
refreshed worker. The principal must exist, be a USER (not a service account), be ACTIVE and be able to access the
worker's client (anchor scope, the client itself as home client, or a partner's granted client); otherwise
`422 INVALID_LINK`. A user already linked to another worker of the same client is `409 LINK_EXISTS` (a person
has at most one field identity per client; one platform user may be linked in several clients). Linking a
different user replaces the link; linking the same user, and unlinking an unlinked worker, are no-ops. The
link has no foreign key: deleting the principal leaves it, and `linkedPrincipal` then carries the id with an
empty `name` and `email`. Schema: `field_identities.linked_principal_id VARCHAR(17) NULL`,
`UNIQUE (client_id, linked_principal_id)` (migration 102).

**Badges** (permission `platform:iam:field-worker:manage`, client-boxed like the worker routes).
`POST /api/field-workers/{id}/badges {label?}` answers `201 {badge, token}`: `token` is `FWB1` plus 26
Crockford base32 characters (the QR content, compared case-insensitively), shown once and stored only
as its SHA-256; issuing revokes the worker's earlier ACTIVE badges in the same transaction (one active
badge per worker). `GET .../badges` answers `{badges[]}` (all statuses, newest first, never the token);
`POST .../badges/{badgeId}/revoke` answers the badge (idempotent). `POST /api/field-workers/badges
{workerIds[1..200]}` issues one per worker in one transaction, all or nothing (a worker the caller
cannot reach is `404`/`403` and nothing is issued): `201 {issued: [{workerId, workerCode, name, badge,
token}]}`. `FieldBadge = {id, workerId, label?, status: ACTIVE|REVOKED, createdAt, createdBy?, revokedAt?,
lastUsedAt?}`. Deleting a worker deletes its badges.

**Console sign-in codes.** `POST /api/field-workers/{id}/sign-in-codes {appCode}` answers
`201 {code, expiresAt, workerId, appCode}`: `code` is `FWS1` plus 26 characters, valid 10 minutes,
single use, shown once, stored hashed. Needs `platform:iam:field-worker:impersonate`, or manage when
the target is a test identity. The caller must be a human principal, except for a test identity
(service accounts may drive QA with test identities); `403 PRINCIPAL_NOT_HUMAN` otherwise. The worker
must be ACTIVE (`409 WORKER_UNAVAILABLE`) and granted the application (`409 NO_APP_ACCESS`). The
device redeems it at `POST /field/auth/sign-in-code`.

**Photos** (`docs/field-worker-photo-spec.md`). A worker's photograph is set, replaced, read and removed at
`/api/field-workers/{id}/photo`, with the worker routes' own permissions and reach (`manage` to set or remove,
`view` or `manage` to read; no new permission) and the same `404 FieldWorker_NOT_FOUND` for a worker the caller
cannot reach. `PUT` takes the image as the raw body (`image/jpeg`, `image/png`, `image/webp`, or
`application/octet-stream`; the platform decides the type from the bytes and a declared type may only agree):
at most `FC_FIELD_PHOTO_MAX_BYTES` (10 MiB), 25 million pixels and 10 000 px a side. The platform keeps two
JPEG renditions, full (fit within 512 px) and thumb (128 px), made from the pixels, so none of the upload's
metadata survives; a `Content-Length` over the limit is refused before the body is read. It answers the
worker (`200`). `DELETE` answers the worker and is idempotent. `GET ?size=thumb|full` (default `thumb`)
answers `200 image/jpeg` with `ETag: "<version>-<size>"`, `Cache-Control: private, no-cache`,
`X-Content-Type-Options: nosniff` and `Content-Disposition: inline`, `304` for a matching `If-None-Match`
(the gate still applies), `404 PHOTO_NOT_FOUND` when there is none. The worker JSON has
`photo: {version, updatedAt}` or `null` (never a URL). Refusals: `400 PHOTO_EMPTY`, `413 PHOTO_TOO_LARGE`,
`415 PHOTO_UNSUPPORTED_TYPE` (not JPEG, PNG or WebP: SVG, GIF and HEIC included), `400 PHOTO_TYPE_MISMATCH`,
`400 PHOTO_DIMENSIONS_TOO_LARGE`, `400 PHOTO_UNREADABLE`, `503 PHOTO_BUSY`. Two switches, with distinct codes:
`FC_FIELD_PHOTOS=off` answers `404 PHOTOS_DISABLED` on all three routes (and the worker JSON has no photo);
the client's policy `photosEnabled` false answers `PUT` `409 PHOTOS_DISABLED_FOR_CLIENT`, `GET`
`404 PHOTO_NOT_FOUND` and `DELETE` `200`. Events `platform:iam:field-worker:photo-set` and `photo-removed`
carry the worker, the version and who (`ADMIN`, `SERVICE_ACCOUNT` or `WORKER`), never image data.

**Without `FLOWCATALYST_APP_KEY`** every operation that hashes a PIN (a supplied `pin`, a
pin-reset) answers `503 FIELD_IDENTITY_UNCONFIGURED`; the field plane never hashes without the
pepper.

**Devices.** `POST /api/field-devices/enrolment-codes {clientId, mode, appCodes[], assignedWorkerId?, label?}`
answers `201 {code, qrPayload, expiresAt}`. `mode` is `PERSONAL | SHARED`; a `SHARED` code with
`assignedWorkerId` is `400 SHARED_DEVICE_ASSIGNED`; `appCodes` must be non-empty. The code is 10
characters from `23456789ABCDEFGHJKMNPQRSTUVWXYZ`, valid 10 minutes, single use, stored only as
its SHA-256 and shown once; `qrPayload` is `fcfield:1:<platform base URL>:<code>`.
`GET /api/field-devices?clientId&status&mode&page&pageSize` answers `{devices, total, page, size}`;
`GET /api/field-devices/{id}` includes `activeSessions` and `lastSeenAt`. `POST /{id}/suspend`
revokes the device's sessions and is resumable by `POST /{id}/activate`; `POST /{id}/revoke` is
terminal (sessions revoked; revoking twice is a no-op; suspend or activate on a revoked device is
`409 DEVICE_REVOKED`).

**Policy.** `GET /api/field-policies/{clientId}` answers the effective policy with `isDefault`
(true when the client has no row). Defaults: `pinMinLength` 6, `softLockThreshold` 5,
`softLockSecs` 900, `hardLockThreshold` 10, `sharedIdleSecs` 900, `personalIdleSecs` 7200,
`shiftMaxSecs` 50400, `requireAttestation` false, `pinReauthDays` 7, `requireEnrolledDevice` false,
`unenrolledIdleSecs` 900, `badgeOnlyLogin` false, `impersonationMaxSecs` 14400, `workerCodeDigits` 3. `PUT` takes the same fields
(numeric ones optional, defaults applied; ranges: `pinMinLength` 4..12, `softLockThreshold` 1..20,
`softLockSecs` 30..86400, `hardLockThreshold` softLockThreshold..50, idle 60..86400, `shiftMaxSecs`
3600..86400, `pinReauthDays` 1..90, `unenrolledIdleSecs` 60..86400, `impersonationMaxSecs` 300..43200,
`workerCodeDigits` 3..6, which applies to codes generated afterwards and never changes an existing one;
`requireEnrolledDevice` and `badgeOnlyLogin` default to false when omitted; a `pinMinLength` below 6 defaults `hardLockThreshold` to 5 and
caps it at 5) or `400 POLICY_INVALID`. The policy also carries `photosEnabled` (default `true`) and
`workerMaySetOwnPhoto` (default `false`): a PUT replaces the policy, so a value left out goes back to its default
(omitted `photosEnabled` is `true`, omitted `workerMaySetOwnPhoto` is `false`). Switching `photosEnabled` off only hides the photos: none can be set (`409 PHOTOS_DISABLED_FOR_CLIENT`),
`GET` answers `404 PHOTO_NOT_FOUND` and the worker JSON has `photo: null`, but the stored photos are kept and
come back when it is switched on. `DELETE /api/field-policies/{clientId}/photos` deletes every stored photo of
the client's workers in one transaction, whatever the setting says; it needs `platform:iam:field-worker:manage`
for the client, refuses an application-confined account (`403 APPLICATION_FORBIDDEN`), answers `200 {removed}`
and is safe to repeat (`removed: 0`, no event); a run that removed photos writes one
`platform:iam:field-policy:photos-deleted` `{clientId, removed}`.
`workerMaySetOwnPhoto` is read by the device
protocol (the worker's own photo, below). PINs are digits only, at most 12, and may not be all one
digit, a straight run (including wrap-around, `123456`, `890123`, `654321`), on the short common
list, or the worker's own code.

**Introspection.** `POST /api/field-tokens/introspect {token}` answers `{active, sub?, clientId?,
workerCode?, deviceId?, appCode?, amr?, authTime?, exp?, act?: {sub, typ}, test?, linkedPrincipalId?}` (`deviceId` is absent
for a token on an unenrolled key; `act` when someone is signed in as the worker; `linkedPrincipalId` is the token's
`linked_principal_id` claim, absent when the worker was not linked when the token was minted). A token that fails any check, belongs to
a client or application the caller cannot reach, or is unknown answers `{"active": false}`, never
an error. `active` is true only while the token is unexpired, its login family (`sid`) has a
non-revoked unexpired session, the worker is `ACTIVE` and, when the token names a device, the device is `ACTIVE`.

**Events** (catalogued, `platform:iam:...`): `field-worker:created | ensured | updated | deleted |
suspended | activated | pin-reset | pin-changed | unlocked | app-granted | app-revoked | badge-issued
| badge-revoked | impersonated | sign-in-code-created | linked | unlinked`, `field-device:enrolment-code-created
| enrolled | suspended | activated | revoked | throttled`, `field-session:revoked` (admin, reuse and worker
revocations, not routine rotation), `field-policy:updated`. Payloads never carry a PIN, code or badge
token. `created | ensured | updated` carry `isTest` (`updated` also `canImpersonate`); `badge-issued`
`{workerId, clientId, badgeId, revokedBadgeIds[]}`; `badge-revoked {badgeId}`; `linked | unlinked`
`{workerId, clientId, workerCode, principalId}` (audited as the calling admin; `principalId` of `unlinked` is the
user that was linked, empty when there was none); `impersonated`
`{workerId, clientId, appCode, actorType, actorId, method: ACT_AS|SIGN_IN_CODE, deviceId?, sessionId}`
(audited as the ACTOR impersonating the worker); `sign-in-code-created {appCode, createdBy, expiresAt}`;
`field-policy:updated` the four sign-in-option fields; `field-device:throttled` scope `DEVICE | CLIENT | IP`
(`ipAddress` for IP).

## Field identity (`/field/*`)

The device-facing half of the field-worker plane. Public routes, mounted outside the platform
API middleware (the device proof and the PIN authenticate), behind a per-IP governor
(`FC_FIELD_RATE_PER_MIN` / `FC_FIELD_BURST`). All bodies are JSON, camelCase; every error is
`{"error": CODE, "message": "..."}`. Without `FLOWCATALYST_APP_KEY` every route answers
`503 FIELD_IDENTITY_UNCONFIGURED` (the plane fails closed). Design:
`docs/field-worker-identity-design.md`; spec: `docs/field-worker-identity-p1-spec.md`.

**Proof (every request).** Header `DPoP: <JWS>`: JOSE header `typ: dpop+jwt`, `alg: ES256` (only),
`jwk` = the device's PUBLIC EC P-256 key (a private member is refused). Claims: `htm` (the method),
`htu` (the platform's configured public base URL + the request path; never the Host header, no
query), `iat` (must be present; accepted within +-24 h, devices have bad clocks), `jti` (present),
`nonce` (required on every route but `/field/auth/nonce`), `ath` (base64url SHA-256 of the access
token, required on bearer routes). The key's thumbprint is the RFC 7638 `jkt`
(`SHA-256({"crv","kty","x","y"})`, base64url, no padding). Every segment and the key's `x` and `y`
must be canonical base64url (see Authentication, "Token spelling"). A bad proof is `401 invalid_dpop_proof`
with `WWW-Authenticate: DPoP error="invalid_dpop_proof", algs="ES256"`.

**Nonces.** 32 random bytes, base64url, valid 5 minutes and **single use** (consumed atomically; a
nonce cannot be replayed, so the platform keeps no `jti` cache). EVERY response, success or error,
carries a fresh `DPoP-Nonce` header. A missing, used or expired nonce is `401 use_dpop_nonce` with
`WWW-Authenticate: DPoP error="use_dpop_nonce"` and the fresh nonce; the client retries once.
`POST /field/auth/nonce` (proof without a nonce) answers `{nonce, expiresAt}`, the same value as
the header.

**Browsers (CORS).** A page on another origin (an application's browser build using
`clients/field-js`) may call these routes when its origin is in the platform's CORS origins table
(`tnt_cors_allowed_origins`, the console's CORS Origins page, `POST /api/platform/cors`): the same
table and the same rule as the push gateway's `/api/push/*` (`internal/platform/cors.Middleware`),
read every 15 seconds. For such an origin every answer carries
`Access-Control-Allow-Origin: <the origin>` (never `*`), `Access-Control-Expose-Headers: DPoP-Nonce,
Retry-After, WWW-Authenticate` and `Vary: Origin`; a preflight (`OPTIONS` with
`Access-Control-Request-Method`) is `204` with `Access-Control-Allow-Methods: POST`,
`Access-Control-Allow-Headers: Authorization, Content-Type, DPoP` and `Access-Control-Max-Age: 600`.
`Access-Control-Allow-Credentials` is never sent: the routes use no cookie, and a page cannot send
the platform's session cookie through them. An origin not in the table gets no CORS headers (only
`Vary: Origin`) and its preflight is `403`, so a browser never sends its page's request. A request
without an `Origin` header (a phone) is answered exactly as before. The rule covers only `/field/*`
(and `/api/push/*`): the platform's other routes, `/oauth/*` and `/.well-known/*` included, answer no
CORS. What allowing an origin means: `docs/field-worker-identity-design.md`, "Browser pages on other
origins".

**Devices and unenrolled keys.** Enrolment is opt-in. The proof is required on every route, and a
key that is not an enrolled device's key is an *unenrolled* request: it may sign in, refresh and
change the PIN, and its tokens stay bound to the key (`cnf.jkt`). When the client's policy has
`requireEnrolledDevice` an unknown key gets `401 UNKNOWN_DEVICE` (login, PIN change, act-as,
sign-in code, and refresh of an unenrolled session). An enrolled key that is `SUSPENDED` / `REVOKED`
is `403 DEVICE_SUSPENDED` / `403 DEVICE_REVOKED` and never falls back to unenrolled. `lastSeenAt` is
touched at most once a minute. What an unenrolled key loses: badge-only sign-in, biometric unlock,
device revoke/suspend and the per-device failure budget, `scope: "device-all"` logout, the device's
app restriction.

| Route | Request | Success |
|---|---|---|
| `POST /field/devices/enrol` | `{code, publicKeyJwk, attestationChain?, platform, model, label?}`; the proof key must be `publicKeyJwk` | `201 {deviceId, mode, clientId, appCodes[]}` |
| `POST /field/auth/nonce` | `{}` | `{nonce, expiresAt}` |
| `POST /field/auth/login` | `{appCode, clientIdentifier?, workerCode \| badgeToken, pin?}` | `FieldTokenResponse` |
| `POST /field/auth/refresh` | `{refreshToken}` | `FieldTokenResponse` |
| `POST /field/auth/pin` | `{currentPin, newPin}`, `Authorization: DPoP <access>` | `204` |
| `POST /field/auth/logout` | `{refreshToken, scope: "session" or "device-all"}` | `204` |
| `POST /field/auth/act-as` | `{workerCode \| workerId, pin}`, `Authorization: DPoP <supervisor access>` | `FieldTokenResponse` (for the worker, with `act`) |
| `POST /field/auth/sign-in-code` | `{code}` | `FieldTokenResponse` (for the code's worker, with `act`) |
| `POST /field/worker/photo` | `{size?: "thumb" \| "full", version?}`, `Authorization: DPoP <access>` | `200 image/jpeg` + `ETag`, `304` when `version` is current, `404 PHOTO_NOT_FOUND` |
| `POST /field/worker/photo/set` | the image as the body, its `Content-Type`; `Authorization: DPoP <access>` | `200 {photoVersion}` |
| `POST /field/worker/photo/remove` | `{}`, `Authorization: DPoP <access>` | `200 {photoVersion: null}` |

`FieldTokenResponse = {accessToken, tokenType: "DPoP", expiresIn, refreshToken, refreshExpiresAt,
idleExpiresAt, worker: {id, workerCode, name, photoVersion, canSetOwnPhoto}, pinChangeRequired}`.
`refreshExpiresAt` is the login's absolute deadline. `photoVersion` is the version of the worker's photo, `null`
when there is none or photos are off for the client or the platform; `canSetOwnPhoto` is true only when photos
are on for the platform and the client, the client's policy has `workerMaySetOwnPhoto`, and the session is the
worker's own (a supervisor's act-as session and a console sign-in code session, which someone else holds as the
worker, say false). Every answer that carries a session (login, refresh, act-as, sign-in code, badge) says the same.

**The worker's own photo** (`docs/field-worker-photo-spec.md` section 10). The three photo routes sit behind the
proof, the nonce and the access token, act on the token's own worker (nothing in a request names a worker: a body
that does is ignored) and are limited per worker per hour (`FC_FIELD_PHOTO_READS_PER_HOUR` 120,
`FC_FIELD_PHOTO_WRITES_PER_HOUR` 20: `429` + `Retry-After`) besides the per-IP governor. `read` answers the
`thumb` (128 px, default) or `full` (512 px) JPEG with `ETag: "<version>-<size>"` and `Cache-Control: private,
no-cache`; a `version` that is still current is a `304` with no body. `set` takes the raw image (JPEG, PNG or
WebP; the bytes decide and a `Content-Type` may only agree) up to `FC_FIELD_PHOTO_MAX_BYTES`: the protocol's
64 KiB body cap is raised for this route only, a `Content-Length` over the limit is refused before the body is
read, and the image refusals are the admin API's (`PHOTO_EMPTY`, `PHOTO_TOO_LARGE`, `PHOTO_UNSUPPORTED_TYPE`,
`PHOTO_TYPE_MISMATCH`, `PHOTO_DIMENSIONS_TOO_LARGE`, `PHOTO_UNREADABLE`, `PHOTO_BUSY`). Setting and removing
need the client's `workerMaySetOwnPhoto` (`403 OWN_PHOTO_NOT_ALLOWED`) and photos on for the client
(`409 PHOTOS_DISABLED_FOR_CLIENT`; `read` answers `404 PHOTO_NOT_FOUND`); `FC_FIELD_PHOTOS=off` answers
`404 PHOTOS_DISABLED`. A PIN-change-only token is `401 invalid_token`; an impersonating session (a supervisor
acting as the worker, or a console sign-in code) may read but not set or remove: `403 IMPERSONATING`; an
unenrolled key is as for the PIN change (`401 UNKNOWN_DEVICE` only when the client requires an enrolled device);
a suspended or revoked device is `403 DEVICE_SUSPENDED` / `DEVICE_REVOKED`; a suspended worker's sessions are
ended (`401 invalid_token`). The event `field-worker:photo-set` / `photo-removed` has `setByType` / `removedByType`
`WORKER`, and the audit row's operation is `SetOwnPhotoCommand` / `RemoveOwnPhotoCommand` with the worker as the
principal, never image data. For a browser page the CORS rule is unchanged: POST only, `Content-Type`, `DPoP` and
`Authorization` allowed (an image `Content-Type` is not safelisted, so the browser preflights the set route, and
the rule allows any value), and `ETag` is exposed besides `DPoP-Nonce`, `Retry-After` and `WWW-Authenticate`.

**Enrol.** The code (10 characters, see the admin API) is consumed atomically with the device
insert; a failure leaves it unused. `401 INVALID_ENROLMENT_CODE` (unknown, used or expired),
`409 DEVICE_KEY_EXISTS` (the key is already enrolled), `422 ATTESTATION_REQUIRED` (the client's
policy requires attestation, which P1 cannot verify), `403 CLIENT_INACTIVE`. `attestationChain` is
stored raw; the level is recorded as reported-but-unverified `SOFTWARE`/`NONE`.

**Login.** Exactly one of `workerCode` / `badgeToken` (else `400 INVALID_REQUEST`). The client is
resolved first: an enrolled device's client (a `clientIdentifier` naming another is `400 CLIENT_MISMATCH`);
a badge's client (a `clientIdentifier` that does not match it is `401 INVALID_CREDENTIALS`); for an
unenrolled key with a `workerCode`, `clientIdentifier` is required (`400 INVALID_REQUEST`) and an unknown
or inactive client is `401 INVALID_CREDENTIALS` after a dummy verify (no client enumeration).
`requireEnrolledDevice` is checked as soon as the client is known (`401 UNKNOWN_DEVICE`). Then, in this
order: device ACTIVE (when enrolled); subject and client PIN-failure buckets (an enrolled device's device
bucket; an unenrolled key's per-IP bucket `FC_FIELD_IP_FAILURES_PER_HOUR`, 100, scope `IP` with
`ipAddress` in the throttle event; `429 TOO_MANY_ATTEMPTS` + `Retry-After`); worker exists (else a dummy PIN verify is spent and the
failure counted, `401 INVALID_CREDENTIALS`); worker ACTIVE and has a PIN (else the same 401 after a
dummy verify); hard lock (`423 WORKER_LOCKED`); soft lock (`429 WORKER_LOCKED_TEMPORARILY` +
`Retry-After`); PIN verify. A wrong PIN is `401 INVALID_CREDENTIALS`. **Badge:** the token is
upper-cased and hashed; unknown or revoked is `401 INVALID_CREDENTIALS` after a dummy verify and counts
as a failure (reasons `UNKNOWN_BADGE` / `BADGE_REVOKED`). With a badge the `pin` is required (`400
PIN_REQUIRED`, after the worker and lock checks) unless the client's policy has `badgeOnlyLogin` AND the
device is enrolled; an unenrolled key always needs the PIN. `amr` is `["badge","pin"]` or `["badge"]`.
Only after the PIN (or badge) verifies:
the worker holds a grant for the application, the application is active, the device (when enrolled) is
enrolled for it, a PERSONAL device assigned to a worker is that worker's (enrolled only), and the client
is ACTIVE; any failure is
`403 NO_APP_ACCESS` (the reason is in the sign-in log, never in the response). A success resets the
failure counter, stamps `lastLoginAt` and rehashes a `LEGACY` PIN hash to `PEPPER_V1`. Locks: the
`soft_lock_threshold`-th consecutive failure soft-locks for `soft_lock_secs`, each further failure
re-arms it, the `hard_lock_threshold`-th hard-locks until an administrator unlocks or resets. Device
bucket `FC_FIELD_DEVICE_FAILURES_PER_HOUR` (20), per-IP bucket `FC_FIELD_IP_FAILURES_PER_HOUR` (100, an
unenrolled key's stand-in for the device bucket; enrolled devices do not count against it), client bucket
`FC_FIELD_CLIENT_FAILURES_PER_HOUR` (300), each emitting one `field-device.throttled` event per window.

**Sessions.** A login is a *family*; each refresh token is one row. The refresh token is 32 random
bytes, base64url, prefixed `fwr_`; only its SHA-256 is stored. `POST /field/auth/refresh` rotates:
the presented token is revoked `ROTATED` and a child with the same family, `amr` and absolute
deadline is issued with `idle = now + idle`. Idle limit: `personal_idle_secs` (PERSONAL) or
`shared_idle_secs` (SHARED) or `unenrolled_idle_secs` (an unenrolled key); absolute limit:
`shift_max_secs` (PERSONAL) or 43200 (SHARED and unenrolled). Past either: `401 SESSION_EXPIRED`. The
worker, grant and (enrolled sessions) device are re-checked. A session is bound to the proof key (`jkt`)
it was created with, with no device for an unenrolled key; a key enrolled later keeps the session's
unenrolled limits until the next sign-in. The refresh token must be presented with that key: any other
key (or, for an enrolled session, device) is theft and revokes the family. **Reuse:** a `ROTATED`
token presented again with a valid proof from the session's own device key within **60 s** of its
rotation is a lost-response retry: the family's live child is rotated and a fresh one issued.
Any other presentation of a rotated token (later, or from another device) revokes the whole family
(`REUSE`), emits `field-session.revoked` and answers `401 INVALID_GRANT`; an unknown, logged-out
or otherwise revoked token is `401 INVALID_GRANT`. `POST /field/auth/logout`: `session` ends the
token's family, `device-all` every session on the proof's device (a SHARED hand-over; needs an enrolled
device, else `400 INVALID_REQUEST`); idempotent, a token not signed in with the proof key is a quiet `204`.

**PIN change.** `POST /field/auth/pin` takes `Authorization: DPoP <access token>` plus the proof
(`ath`); a `pcr` token is accepted here and nowhere else. The current PIN goes through the same
lockout accounting as a login. The new PIN must pass the client's policy (`422 PIN_POLICY`
`{violations[]}`) and differ from the current one (`SAME_AS_CURRENT`). The worker's other families
are revoked (`PIN_CHANGED`); the current one is kept, and the client refreshes to drop `pcr`. It works
from an unenrolled key; an impersonated session gets `403 IMPERSONATING`.

**Impersonation** (`docs/field-worker-sign-in-options-spec.md` section 4). Both paths produce a session
for the WORKER (`sub`, `worker_code`, `name`, `client_*` are the worker's) with an `act` claim for whoever
is acting and `amr` ending `"imp"`; `pcr` is never set; the absolute limit is the lesser of the normal
limit, the policy's `impersonation_max_secs` and (act-as) the supervisor's own session expiry; the idle
limit is normal. `field_sessions.actor_type` (`FIELD_WORKER | PRINCIPAL`) / `actor_id` mark it and are
copied to every rotated child; refresh re-checks that the actor may still act. `POST /field/auth/act-as`
(bearer = the supervisor's own token): `403 IMPERSONATING` if that session is itself impersonated; the
supervisor must be ACTIVE, `canImpersonate` and not locked (`403 IMPERSONATION_FORBIDDEN`, no PIN check);
`pin` is the SUPERVISOR's PIN, verified with the full lockout accounting (`401 INVALID_CREDENTIALS`);
then the target must be in the same client, ACTIVE (a locked target is allowed), not a supervisor, not
the supervisor, and granted the token's application, else `403 NO_APP_ACCESS` (reason only in the
sign-in log); device checks as login (a PERSONAL device assigned to the supervisor is acceptable).
`amr` is `["pin","imp"]`; the supervisor's own session is untouched. `POST /field/auth/sign-in-code`:
atomically consumes the code (`401 INVALID_CODE`: unknown, used, expired, or its creator no longer
active; counts against the subject bucket), re-checks worker ACTIVE, grant, application, client,
`requireEnrolledDevice` (`401 UNKNOWN_DEVICE`) and the device's app restriction (`403 NO_APP_ACCESS`; a
failed redemption leaves the code unused); `act = {sub: principal id, typ: "principal", name}`, `amr`
`["code","imp"]`. Both emit `field-worker:impersonated` and an audit row whose principal is the actor and
entity the worker; attempts are logged with `auth_method` `ACT_AS` / `SIGN_IN_CODE` and `actor_id`.

**Access token.** A JWT signed with the platform key (same `kid` and JWKS as every platform token),
JOSE header `typ: fc-field+jwt`, lifetime `FC_FIELD_ACCESS_TOKEN_TTL_SECS` (default 600). Claims:
`iss`, `sub` (the worker, `fwu_...`), `aud` (the application CODE, a bare string), `exp`, `iat`,
`nbf`, `jti`, `token_use: "field"`, `client_id`, `client_identifier`, `worker_code`, `name`,
`device_id` (omitted on an unenrolled key, where `device_mode` is `"UNENROLLED"`), `device_mode`, `sid`
(the family id), `amr` (`["pin"]`, `["badge","pin"]`, `["badge"]`, `["pin","imp"]`, `["code","imp"]`),
`auth_time`, `cnf: {jkt}`, `pcr: true` only while a PIN change is required, `act: {sub, typ:
"field_worker" | "principal", name}` only on an impersonated token, `test: true` only for a test
identity, and `linked_principal_id` (the platform user the worker is linked to) only when linked: audit
correlation, it grants nothing. It is copied at login and at every refresh (a link added or removed after
login shows on the next refresh, within one access-token lifetime) and, on an impersonated session, is the
TARGET worker's link, never the actor's. No roles, scope, applications or clients
claims. The platform API middleware refuses `token_use: "field"` (and `pkg/fcsdk/auth`
`TokenValidator` accepts only `token_use` `api`, `identity` or absent; the platform-token validators
of every SDK share that allowlist), so a field token is never an API credential. The
application verifies it with the SDK's `FieldTokenVerifier` (token: `typ`, signature, `iss`, `exp`
(REQUIRED: a token without `exp` is `invalid_token`, vector `missing_exp`),
`token_use`, `aud`; refuses `pcr`; proof: `cnf.jkt`, `htm`, `htu`, `ath`, `iat` within +-5 min) or
asks `POST /api/field-tokens/introspect` (`active:false` for a `pcr` token). Nonce checking at the
application is out of scope for the SDK (use the framework's stateless nonce for replay protection).
The verifier compares `htu` with the application's configured public base URL + the request path; the
Host header and `X-Forwarded-*` headers are never used (the Go middleware requires a base URL).
Access tokens are signed RS256 only (the platform signer; a verifier refuses any other header `alg` as `invalid_token`, vector `alg_not_rs256`); DPoP proofs are ES256 only.
Shared test vectors: `api/field-fixtures/` (`go run ./tools/field-fixtures`; access tokens RS256, proofs ES256).
Valid cases `valid_unenrolled`, `valid_impersonated_supervisor`, `valid_impersonated_principal` and
`valid_test` and `valid_linked` carry a `principal` object (`deviceId` empty on an unenrolled key, `deviceMode`,
`amr`, `actor?`, `test`, `linkedPrincipalId`, empty when absent) that a verifier must expose
(`FieldPrincipal.LinkedPrincipalID` in the Go SDK).

**The client on the phone.** The sign-in answer carries the worker and no client, and it does not need to:
the access token's `client_id` and `client_identifier` claims (minted with every field token since the
first release of the field-token protocol, `fcdev` 0.12.5, impersonated sessions included) are the client's
platform id and identifier, and the four device libraries (`clients/field-js`, `field-android`, `field-ios`,
`field-react-native`) read them into the signed-in session as `clientId` and `clientIdentifier` (`null` when the
token does not carry the claim) at every sign-in, refresh, unlock and impersonation, and keep them with the
stored session. An application that addresses its API per client (`/api/clients/{client}/...`) builds the
address from `clientIdentifier` and needs no "who am I" route. The identifier on the phone is for building
addresses and showing a name: the phone cannot verify its own token, so the application's server takes the
client from the verified token (the framework context's client, or the SDK verifier's `client_identifier` claim) and refuses a
request whose address names another client.


**Errors in one place.** `use_dpop_nonce` 401, `invalid_dpop_proof` 401, `invalid_token` 401
(bearer routes), `UNKNOWN_DEVICE` 401, `DEVICE_SUSPENDED` 403, `DEVICE_REVOKED` 403,
`INVALID_CREDENTIALS` 401, `CLIENT_MISMATCH` 400, `PIN_REQUIRED` 400, `IMPERSONATING` 403,
`IMPERSONATION_FORBIDDEN` 403, `INVALID_LINK` 422 and `LINK_EXISTS` 409 (admin link), `INVALID_CODE` 401, `WORKER_LOCKED` 423, `WORKER_LOCKED_TEMPORARILY` 429,
`PIN_RESET_REQUIRED` 403 (the PIN was set under a retired app key; a supervisor must reset it; not counted as a wrong PIN),
`TOO_MANY_ATTEMPTS` 429, `NO_APP_ACCESS` 403, `INVALID_GRANT` 401, `SESSION_EXPIRED` 401,
`PIN_POLICY` 422, `INVALID_ENROLMENT_CODE` 401, `DEVICE_KEY_EXISTS` 409, `ATTESTATION_REQUIRED`
422, `CLIENT_INACTIVE` 403, `INVALID_REQUEST` / `INVALID_PUBLIC_KEY` 400 (the login `clientIdentifier` is required from an unenrolled key),
`FIELD_IDENTITY_UNCONFIGURED` 503.

Sign-in attempts (success and failure, with device, method and the failure reason) are recorded in
`field_login_attempts`, not `iam_login_attempts` (a field worker is not a principal). The history is
retained for 90 days by default (`FC_FIELD_LOGIN_ATTEMPT_RETENTION_DAYS`; `0` keeps it forever) and purged in
batches, so `GET /api/field-workers/{id}/sign-ins` only reaches back that far.

## App updates (`/app-updates/*`, `/api/app-updates/*`)

Over-the-air updates of phone apps built with Expo (design: `docs/app-updates-design.md`, which
records what was verified against the `expo-updates` 58 client source). The public half is the Expo
Updates protocol, version 1, as that client implements it; the API half is ordinary huma routes in
the OpenAPI lock (tag `app-updates`, `x-sdk: admin`).

**Manifest** `GET /app-updates/{applicationCode}/{appKey}/manifest`, unauthenticated, per-IP governor
(`FC_APP_UPDATES_MANIFEST_RATE_PER_MIN` 300 / `_BURST` 100). Request headers: `expo-protocol-version: 1`
(else `406 UNSUPPORTED_PROTOCOL_VERSION`), `accept` admitting `multipart/mixed` (else `406
NOT_ACCEPTABLE`), `expo-platform` `android|ios`, `expo-runtime-version`, `expo-channel-name` (the app
config's `updates.requestHeaders`), `eas-client-id` (a UUID, for the staged rollout), and
`expo-expect-signature` (`sig, keyid="<id>", alg="rsa-v1_5-sha256"`; an unknown key id is `400
UNKNOWN_KEY_ID`, another algorithm `400 UNSUPPORTED_SIGNATURE_ALGORITHM`). A malformed path or header is
`400 INVALID_REQUEST`. Without a signing key every request is `503 APP_UPDATES_UNCONFIGURED`. Errors are
`{"error": CODE, "message": "..."}` with `cache-control: no-store`.

The answer is `200`, `content-type: multipart/mixed; boundary=...`, `expo-protocol-version: 1`,
`expo-sfv-version: 0`, `cache-control: private, max-age=0` (no `expo-manifest-filters`, no
`expo-server-defined-headers`), with either a `manifest` part and an `extensions` part
(`{"assetRequestHeaders":{}}`), or a `directive` part: `{"type":"noUpdateAvailable"}` or
`{"type":"rollBackToEmbedded","parameters":{"commitTime":"<time>"}}`. Part headers are written in lower
case (`content-disposition: form-data; name="manifest"`, `content-type: application/json`,
`expo-signature: sig="<base64>", keyid="<id>", alg="rsa-v1_5-sha256"`); the manifest or directive part is
always signed (RSASSA-PKCS1-v1_5, SHA-256, standard base64, over the part's exact bytes) with the key the
client named, or the default key. The manifest is `{id (UUID), createdAt, runtimeVersion, launchAsset,
assets[], metadata: {}, extra: {expoClient?}}`, an asset `{hash (base64url SHA-256, no padding), key
(MD5 hex), contentType, fileExtension (with its dot; never on the launch asset), url}`, `url` relative to
the manifest URL (`assets/<sha256>`) unless `FC_APP_UPDATES_ASSET_BASE_URL` makes it absolute. Times are
`yyyy-MM-ddTHH:mm:ss.SSSZ`. An unknown or inactive application, app key, platform, runtime version or
channel gets the same signed `noUpdateAvailable` as a branch with no updates.

**Selection.** The branch's two newest rows: the newest when the installation is in its rollout
(`bucket = uint64(first 8 bytes of SHA-256("<updateId>:<lower-case client UUID>")) mod 100 <
rolloutPercent`; without a client id only at 100), else the one before it, else `noUpdateAvailable`; a
newest row that is a rollback answers `rollBackToEmbedded`.

**Asset** `GET /app-updates/{applicationCode}/{appKey}/assets/{sha256}`, unauthenticated, per-IP governor
(`FC_APP_UPDATES_ASSET_RATE_PER_MIN` 3000 / `_BURST` 1000): the file when it belongs to a published update
of that phone app of an active application, uncompressed, with its manifest content type,
`content-length`, `cache-control: public, max-age=31536000, immutable`, `etag: "<sha256>"` (`304` on
`if-none-match`); anything else is `404 NOT_FOUND`. Only `GET` (`405 METHOD_NOT_ALLOWED`); an unknown path
under `/app-updates/` is `404 NOT_FOUND`. No CORS.

**API.** Every route needs a permission (`platform:app-update:update:view`, `...:update:publish`,
`platform:app-update:rollout:manage`; `403 PERMISSION_REQUIRED`), anchor scope and reach to the
application (`403 SCOPE_FORBIDDEN`; an application-scoped caller naming an application it does not reach,
or an unknown one, gets the same 403; by-id routes answer `404 AppUpdate_NOT_FOUND` for another
application's update).

| Route | Success | Errors |
|---|---|---|
| `POST /api/app-updates/files/{applicationCode}/missing {sha256[≤5000]}` | `200 {missing[]}` | `400 INVALID_SHA256`, `TOO_MANY_FILES` |
| `PUT /api/app-updates/files/{applicationCode}/{sha256}` (raw body ≤ `FC_APP_UPDATES_MAX_FILE_BYTES`) | `200 {sha256, sizeBytes, created}` | `400 DIGEST_MISMATCH`, `INVALID_SHA256`, `EMPTY_FILE` |
| `POST /api/app-updates/updates {applicationCode, appKey, platform, runtimeVersion, channel, rolloutPercent?, message?, sourceCommit?, launchAsset{sha256}, assets[{sha256, fileExtension}], expoClient?}` | `201 AppUpdateResponse` | `400 INVALID_*`, `TOO_MANY_ASSETS`; `409 FILES_MISSING` (`details.missing`), `ROLLOUT_IN_PROGRESS`, `APPLICATION_INACTIVE` |
| `GET /api/app-updates/updates?applicationCode&appKey&platform&runtimeVersion&channel&page&size` | `200 {data, page, size, total, total_pages}` | |
| `GET /api/app-updates/updates/{id}` | `200 {update, assets[], totalBytes, hasExpoClient}` | |
| `GET /api/app-updates/branches?applicationCode&appKey` | `200 {branches[{appKey, platform, runtimeVersion, channel, current, previous?, updateCount}]}` | |
| `PUT /api/app-updates/updates/{id}/rollout {rolloutPercent}` | `200 AppUpdateResponse` | `409 NOT_LATEST_UPDATE`, `NOT_AN_UPDATE` |
| `POST /api/app-updates/updates/{id}/republish {message?}` | `201 AppUpdateResponse` (a new update) | `409 ALREADY_LIVE`, `NOT_AN_UPDATE` |
| `POST /api/app-updates/rollback {applicationCode, appKey, platform, runtimeVersion, channel, to: PREVIOUS\|EMBEDDED, message?}` | `201 {update, created: true}`; `200 {created: false}` when already rolled back to the embedded bundle | `409 NOTHING_TO_ROLL_BACK`, `NO_PREVIOUS_UPDATE` |

`AppUpdateResponse = {id, applicationId, applicationCode, appKey, platform, runtimeVersion, channel, kind:
UPDATE|ROLLBACK_TO_EMBEDDED, manifestId?, createdAt, rolloutPercent, servedPercent, live, message?,
sourceCommit?, republishedFrom?, publishedBy, publishedByName?, updatedAt, binariesOnBranch?}`
(`binariesOnBranch` only in a publish's answer: how many binaries of the runtime version are on the
channel). A publish with `requireBinary: true` is `409 NO_BINARY_FOR_RUNTIME_VERSION` when there is
none. Events (catalogued, audited): `platform:app-update:update:published | rollout-changed |
republished | rolled-back-to-embedded`.

### App binaries and install links

Design: `docs/app-updates-design.md` section 14. Android binaries (APKs) of a phone app, published to a
channel in a variant (`release`, `debug`, or another `^[a-z][a-z0-9-]{0,19}$`), and the install link a
person downloads them from.

**Install page** `GET /app-updates/{applicationCode}/{appKey}/install/{token}` and **download** `GET
/app-updates/{applicationCode}/{appKey}/install/{token}/{variant}.apk`, unauthenticated, per-IP governors
(`FC_APP_UPDATES_INSTALL_RATE_PER_MIN` 300 / `_BURST` 100, `FC_APP_UPDATES_DOWNLOAD_RATE_PER_MIN` 120 /
`_BURST` 100). The page is `200 text/html`, `cache-control: no-store`. The download is `200` or `206`
(one range; `content-range`), `416` past the end (`content-range: bytes */<size>`), `412` for an
`if-match` that is not its ETag, `304` for an `if-none-match` that is; `content-type:
application/vnd.android.package-archive`, `content-disposition: attachment; filename="<appKey>-<versionName>-<variant>.apk"`,
`accept-ranges: bytes`, `etag: "<sha256>"`, `cache-control: private, no-cache`; an `if-range` that is not
the ETag gets the whole file. Every other outcome (a wrong, replaced or malformed token, an unknown or
inactive application, another application or app key, no binary, an unknown variant) is `404
text/html` with one fixed body. Every answer carries `referrer-policy: no-referrer`, `x-robots-tag:
noindex, nofollow`, `x-frame-options: DENY`, `x-content-type-options: nosniff` and
`content-security-policy: default-src 'none'; style-src 'sha256-<the page stylesheet>'; img-src 'none';
base-uri 'none'; form-action 'none'; frame-ancestors 'none'`. A stored file that does not match its hash
is cut off short of its `content-length`.

**API** (tag `app-updates`, `x-sdk: admin`; anchor scope and reach to the application as above):

| Route | Permission | Success | Errors |
|---|---|---|---|
| `GET /api/app-updates/binary-files/{applicationCode}/{sha256}` | publish | `200 AppBinaryFileResponse` | `404 BINARY_FILE_NOT_FOUND` |
| `PUT /api/app-updates/binary-files/{applicationCode}/{sha256}` (raw body ≤ `FC_APP_UPDATES_MAX_BINARY_BYTES`) | publish | `200 {sha256, sizeBytes, created, facts}` | `400 DIGEST_MISMATCH`, `EMPTY_FILE`, `FILE_TOO_LARGE`, `UPLOAD_INCOMPLETE`, `INVALID_RUNTIME_VERSION`, `NOT_AN_APK`, `MALFORMED_MANIFEST`, `UNSIGNED`, `SIGNATURE_INVALID`, `UNSUPPORTED_SIGNATURE`, `MALFORMED_RUNTIME_VERSION` |
| `POST /api/app-updates/binaries {applicationCode, appKey, platform: android, channel, variant, sha256, message?, sourceCommit?}` | publish | `201 {binary, created: true}`; `200 {binary, created: false}` for the same file again | `409 FILE_MISSING`, `APPLICATION_INACTIVE`, `PLATFORM_MISMATCH`, `RELEASE_IS_DEBUGGABLE`, `RELEASE_SIGNED_WITH_DEBUG_KEY`, `CHANNEL_MISMATCH`, `UPDATE_URL_MISMATCH`, `PACKAGE_MISMATCH`, `SIGNING_CERTIFICATE_CHANGED`, `VERSION_CODE_TAKEN` |
| `GET /api/app-updates/binaries?applicationCode&appKey&platform&channel&variant&runtimeVersion&page&size` | view | `200 {data: AppBinaryResponse[], page, size, total, total_pages}` | |
| `GET /api/app-updates/binaries/{id}` | view | `200 AppBinaryResponse` | `404 AppBinary_NOT_FOUND` |
| `GET /api/app-updates/binary-check?applicationCode&appKey&platform&channel&runtimeVersion` | view | `200 {published, count, newest?}` | |
| `GET /api/app-updates/install-links?applicationCode&appKey` | view | `200 {links: AppInstallLinkResponse[]}` | |
| `POST /api/app-updates/install-links {applicationCode, appKey, channel}` | install-link | `201 {link, url, token}` | `409 INSTALL_LINK_EXISTS` |
| `POST /api/app-updates/install-links/{id}/replace` | install-link | `200 {link, url, token}` | `404 AppInstallLink_NOT_FOUND` |
| `DELETE /api/app-updates/install-links/{id}` | install-link | `204` | `404 AppInstallLink_NOT_FOUND` |

Permissions: publish `platform:app-update:update:publish`, view `platform:app-update:update:view`,
install-link `platform:app-update:install-link:manage`. `AppBinaryResponse = {id, applicationId,
applicationCode, appKey, channel, variant, sha256, sizeBytes, fileName, facts: {platform, packageName,
versionCode, versionName?, minSdk, debuggable, runtimeVersion?, updateUrl?, updateChannel?, updateKeyId?,
signingCertificateSha256, signingCertificateSubject, debugCertificate, signingSchemes[]}, newest,
message?, sourceCommit?, publishedBy, publishedByName?, createdAt}`. `AppInstallLinkResponse = {id,
applicationId, applicationCode, appKey, channel, generation, createdBy, createdAt, updatedBy,
updatedAt}`; the token (43 characters, base64url of 32 random bytes) appears only in the create and
replace answers. Events: `platform:app-update:binary:published`, `platform:app-update:install-link:created
| replaced | removed`.

## Non-contractual surface

These may vary freely between builds/deployments:

- HTTP/2 vs HTTP/1.1 negotiation defaults, TLS cipher ordering.
- `Server` and `Date` response headers.
- Response gzip behavior (gzip when the client asks).
- Internal SQL query plans; connection pool sizing.
- Log line format (stdout structure isn't part of the API contract).
