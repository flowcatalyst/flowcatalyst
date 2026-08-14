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
  drift; `make api-bump` records an intentional change.
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
catch-all→500. **There is no 422 in the mapping.**

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
| POST | `/auth/password-reset/request`, `/auth/password-reset/confirm` | unauthenticated password reset (hex SHA-256 tokens) |
| GET | `/auth/password-reset/validate` | check a reset token |
| GET | `/api/me`, `/api/me/applications` | caller identity + accessible applications |
| GET/POST | `/oauth/authorize`, `/oauth/token` | OAuth/OIDC provider surface |
| GET | `/.well-known/openid-configuration`, `/.well-known/jwks.json` | OIDC discovery + JWKS |

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

## Non-contractual surface

These may vary freely between builds/deployments:

- HTTP/2 vs HTTP/1.1 negotiation defaults, TLS cipher ordering.
- `Server` and `Date` response headers.
- Response gzip behavior (gzip when the client asks).
- Internal SQL query plans; connection pool sizing.
- Log line format (stdout structure isn't part of the API contract).
