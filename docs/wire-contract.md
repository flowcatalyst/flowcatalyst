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
- **`tests/parity/`** + `tools/parityharness/` — replay-based contract tests
  that assert status, headers, and body shape for representative requests.
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

Status-code mapping: Validation→400,
Unauthorized/InvalidCredentials/TokenExpired/InvalidToken→401, Forbidden→403,
NotFound→404, Duplicate/BusinessRule/Concurrency→409, TooManyRequests→429,
catch-all→500. **There is no 422 in the mapping.** The session-cookie guard
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
JIT-provisions null-client principals (no role sync, trust bound to the IdP's
`allowed_email_domains`). NOTE: portals no longer use this path — the portal
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

A committed test vector (`tests/golden/webhook/`, mirrored in
`pkg/fcsdk/webhook/testdata/`) pins the expected signature for a fixed input;
the router's tests and the consumer SDKs' verifier tests (TypeScript, Laravel)
all assert against it. If the vector ever changes, every side changes in
lockstep.

Other signing sites are HMAC-over-plain-strings (dispatch-job auth token signs
the raw job id) or not HMAC at all (JWT RS256, Argon2id, AES-GCM) — none sign
serialized JSON.

### Dispatch-processing delivery request

`POST /api/dispatch/process` (`internal/platform/dispatchjob/processing`) is
the *other* subscriber-facing delivery path — the platform, not the router,
POSTs directly to the subscription's `target_url` (the router only invokes
this endpoint; it never sees the real webhook). The request:

- **Headers:** `Content-Type: application/json`, `X-Dispatch-Job-Id`,
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
  direct job's `app:` code prefix. A named account that is inactive or has no
  webhook credentials makes the delivery go out **unsigned with a reason**
  (never silently signed by another account); the reason is logged and, on a
  failed attempt, appended to the attempt's `errorMessage` as
  `(delivered unsigned: …)`.
- **`X-FlowCatalyst-Client: {clientId}:{clientCode}`** (owner ruling
  2026-09-18, `docs/spec/webhook-client-code.md`) — names the tenant a
  multi-tenant subscriber's endpoint is receiving a delivery for. Sent for
  **every** delivery whose job carries a `client_id` that resolves to a
  `tnt_clients.identifier`, including `dataOnly` deliveries (whose raw body
  has no envelope to carry a `clientCode` field). **Never a half pair**: a
  platform-scoped job (no `client_id`) or a client that does not resolve
  omits the header entirely. Adding this header does not change what the
  signature covers — it is still the body alone.
- **Body:** the raw payload byte-for-byte in `dataOnly` mode; otherwise a
  CloudEvents-style envelope — `id`, `type`, `attemptNumber`, and, when
  present, `source`, `subject`, `correlationId`, `messageGroup`, `clientId`,
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
| `flushGroup` | bool | `true` = delivered, and stop delivering this message's group. Default `false`. |

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

## Function-runner control plane: runtimes and capabilities

The runner-facing routes under `/control/functions/` (`desired`, `heartbeat`,
`artifacts/{digest}`, `events`, `dispatch-jobs`) are the Go v1 control
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
- Delivery URLs for subscriptions, scheduled jobs and function dispatch use
  `FC_FUNCTIONS_RUNNER_URL_<RT>` when set for the version's runtime (see
  `docs/environment-variables.md`).

## Non-contractual surface

These may vary freely between builds/deployments:

- HTTP/2 vs HTTP/1.1 negotiation defaults, TLS cipher ordering.
- `Server` and `Date` response headers.
- Response gzip behavior (gzip when the client asks).
- Internal SQL query plans; connection pool sizing.
- Log line format (stdout structure isn't part of the API contract).
