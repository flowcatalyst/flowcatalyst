# Error Handling

The Go SDK follows two stdlib patterns: **`errors.Is(err, Sentinel)`**
when you want to branch on a known failure kind, and
**`errors.As(err, &Typed{})`** when you need the wrapped detail. This
page lists every exported sentinel and typed error, by package, so you
can copy the right idiom without spelunking through godoc.

## Quick rules

- Anything declared as `var ErrXxx = errors.New(...)` → use
  `errors.Is`.
- Anything declared as a type (`type FooError struct { ... }`) →
  use `errors.As(err, &target)` to pull it out for inspection.
- Backend / driver failures across `cache`, `lock`, and `auth` wrap
  a single `ErrBackend` sentinel — so one `errors.Is(err, pkg.ErrBackend)`
  check covers every transient infra fault.

## `client`

The platform HTTP client returns a typed `*APIError` for every
non-2xx response. There are no sentinels; all branching is by
status code on the typed value. A success status never produces an error:
routes that answer `204 No Content` return only an `error` (nil on success), so
there is no body to decode.

```go
import "github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/client"

err := c.Principals().Update(ctx, id, &req)
var apiErr *client.APIError
if errors.As(err, &apiErr) {
    switch apiErr.StatusCode {
    case 404:
        // not found
    case 409:
        // conflict
    default:
        // log apiErr.Body for the platform's structured response
    }
}
```

Convenience methods on `*APIError`: `IsNotFound()`, `IsConflict()`,
`IsUnauthorized()`, `IsForbidden()`, `Retryable()`, `Code()`,
`Message()`.

## `usecase`

Domain operations return `*usecase.Error`. The `Kind` field drives
HTTP status mapping via `(*Error).HTTPStatus()`. Use the constructor
functions to build them in an operation's `Validate` / `Authorize` /
`Execute`: `Validation`, `BusinessRule`, `Authorization`, `NotFound`,
`Conflict`, `Internal`. `usecaseop.Run` returns them unchanged, and wraps
a failed commit (repository, event or audit write) as an `Internal` one.

```go
import (
    "github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
    "github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

event, err := usecaseop.Run(ctx, uow, PlaceOrder(repo), cmd, ec)
if err != nil {
    if e := usecase.AsError(err); e != nil {
        http.Error(w, e.Message, e.HTTPStatus())
        return
    }
    http.Error(w, "internal", 500)
    return
}
```

The lower-level `usecasepgx.Commit*` functions return a sealed
`usecase.Result[E]`; `usecase.Into(result)` turns it into `(E, error)`.

## `auth`

A single typed `*auth.Error` for everything, branched by `Kind`. The
sentinel values exist so you can `errors.Is` against a *kind* without
caring about the message.

| Sentinel | Meaning |
|---|---|
| `auth.ErrTokenExpired` | JWT exp has passed. Refresh and retry. |
| `auth.ErrInvalidToken` | Bad signature / issuer / audience / format. Reject. |
| `auth.ErrDiscovery` | OIDC discovery or JWKS fetch failed. Network or DNS. |
| `auth.ErrTokenExchange` | `/oauth/token` or related endpoint failed. |
| `auth.ErrConfig` | Bad config (missing required field, etc.). |
| `auth.ErrCrypto` | Crypto operation failed. |

```go
ctx, err := validator.ValidateBearer(r.Context(), r.Header.Get("Authorization"))
switch {
case errors.Is(err, auth.ErrTokenExpired):
    http.Error(w, "token expired", 401)
case errors.Is(err, auth.ErrInvalidToken):
    http.Error(w, "invalid token", 401)
case err != nil:
    http.Error(w, "auth error", 500)
}
```

## `webhook`

Two parallel APIs exist:

- `Verifier` (existing) — matches THIS Go platform's router (uppercase
  headers, ISO8601 millisecond timestamps). Sentinels:
  `ErrMissingSignature`, `ErrMissingTimestamp`, `ErrBadTimestamp`,
  `ErrStaleTimestamp`, `ErrBadSignature`.
- `Validator` (the legacy platform-SDK shape) — mixed-case headers, Unix-second
  timestamps. Use when receiving webhooks from a peer FlowCatalyst SDK.

`Validator` sentinels — `errors.Is` branching maps to HTTP status codes:

| Sentinel | Suggested HTTP status |
|---|---|
| `webhook.ErrValidatorMissingSignature` | 400 |
| `webhook.ErrValidatorMissingTimestamp` | 400 |
| `webhook.ErrValidatorInvalidTimestamp` | 400 |
| `webhook.ErrTimestampExpired` | 401 |
| `webhook.ErrTimestampInFuture` | 401 |
| `webhook.ErrValidatorInvalidSignature` | 403 |
| `webhook.ErrMissingSecret` | 500 (caller misconfiguration) |

See `examples/webhook-receiver/main.go` for the full switch.

## `cache`

| Sentinel | Meaning |
|---|---|
| `cache.ErrInvalidTTL` | Caller passed a zero / negative TTL to `SetBytes`. |
| `cache.ErrBackend` | Any driver-level I/O failure. Wrap → `errors.Is` works. |

Plus two typed errors for JSON conversion:

| Type | When |
|---|---|
| `*cache.SerializeError` | `Set` / `GetOrSet` failed to marshal the value. |
| `*cache.DeserializeError` | `Get` / `GetOrSet` failed to unmarshal stored bytes into `T`. |

```go
v, ok, err := cache.Get[User](ctx, c, "u:1")
var de *cache.DeserializeError
switch {
case errors.As(err, &de):
    // stored bytes don't match T — schema migration?
case errors.Is(err, cache.ErrBackend):
    // network / DB hiccup; usually retryable
}
```

## `lock`

| Sentinel | Meaning |
|---|---|
| `lock.ErrInvalidTTL` | Caller passed a zero / negative TTL to `Acquire`. |
| `lock.ErrBackend` | Driver-level I/O failure. |

`Acquire` returns `(nil, nil)` on contention — that's **not** an
error. The error path is reserved for backend faults.

```go
h, err := lp.Acquire(ctx, key, 30*time.Second)
switch {
case err != nil:
    // ErrInvalidTTL or wrapped ErrBackend
case h == nil:
    // contended; another holder owns the key
default:
    defer h.Release(ctx)
    // critical section
}
```

## `scheduledjobs`

The `Runner.Process` entry point never returns an error — it returns
a `Result{Kind: ...}` value the caller maps to HTTP status. Background
failures (handler panics, lock-acquire failures, callback failures)
are surfaced via the `OnError` hook, which receives a `*RunnerError`
carrying an `ErrorKind`:

| Kind | Meaning |
|---|---|
| `scheduledjobs.ErrCallbackFailed` | `/log` or `/complete` API call failed. |
| `scheduledjobs.ErrLockFailed` | Lock acquire or release failed. |
| `scheduledjobs.ErrHandlerPanicked` | User handler returned an error or panicked. |

```go
runner := scheduledjobs.NewBuilder(api, lp).
    OnError(func(err error, env *scheduledjobs.Envelope) {
        var re *scheduledjobs.RunnerError
        if errors.As(err, &re) && re.Kind == scheduledjobs.ErrLockFailed {
            metrics.LockFailures.Inc()
        }
        log.Printf("runner: %s instance=%s: %v", env.JobCode, env.InstanceID, err)
    }).
    Build()
```

## `outbox`, `outboxpgx`, `outboxsql`

No sentinels. The writers return:

- a plain `error` from building the row, before anything is written: a
  dispatch job that breaks the batch endpoint's rules
  (`outbox.DispatchJob.Validate`; the messages are the framework's, e.g.
  `dispatch job targetUrl must be an absolute http(s) URL with a host and no
  user info`), an event with no id or type, an event whose `ToDataJSON` fails
  or returns invalid JSON, a command that cannot be serialised for its audit
  row, an invalid table name;
- the driver's error from the `INSERT` (`pgx` / `database/sql`), unwrapped.

Inside `usecaseop.Run` / `usecasepgx.Commit*` both surface as a
`usecase.Internal` error (`EVENT_WRITE`, `AUDIT_WRITE`) wrapping the cause,
and the transaction rolls back.

## `sync`

`Synchronizer.Sync` never returns an error — per-category failures are
captured on the returned `*Result`. Subscriptions are not sent when the
connections sync failed (their `Error` says so). `SkippedRoles` and
`PasswordHashIgnored` on the principals result are **not** errors: the
sync applied, those names/hashes were left out.

```go
r := sync.NewSynchronizer(c).Sync(ctx, set, sync.DefaultOptions())
if r.HasErrors() {
    for cat, msg := range r.Errors() {
        log.Printf("sync category %s failed: %s", cat, msg)
    }
}
```
