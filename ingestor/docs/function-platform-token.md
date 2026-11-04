# Platform token for JVM functions

A function can ask its host for a FlowCatalyst access token and send it as a bearer to another first-party
application that accepts FlowCatalyst tokens. The function holds no client id or secret: the token is the one the
service account linked to the function's application would get from its `client_credentials` grant. (The wire and
the platform side are in the repository's `docs/function-platform-token.md`.)

## Declare it

In `META-INF/flowcatalyst/function.json`, beside `config`, `secrets`, `emits` and `dispatches`:

```json
{ "abi": 1, "platformToken": true, "jvm": { "entrypoint": "com.acme.MyFunction" }, "endpoints": [ ... ] }
```

Only a boolean is accepted. A version that does not declare it gets `NOT_DECLARED` from the host before any call
is made to the platform, and the platform refuses it again.

## Call it

```java
String token = ctx.platformToken();          // cached by the host while valid
// send "Authorization: Bearer " + token to the other application ...
// the target answered 401 (the account's roles changed): fetch a new one, once
String fresh = ctx.platformToken(true);
```

Call it for each outbound request and do not keep the value: the host caches the token and renews it shortly
before it expires, so a call is cheap. The token is a credential; never log it.

`PlatformTokenException` (unchecked) carries a `code()` and `retryable()`:

| `code` | Meaning | Retryable |
|---|---|---|
| `UNAVAILABLE` | the platform could not be reached, answered 5xx, 429 or 408, or did not accept the host's own credential | yes |
| `NOT_DECLARED` | the version's describe does not declare `platformToken` | no |
| `NOT_ALLOWED` | the platform refused (403, 409, 400, 404): not declared there, the application has no linked service account, the account is inactive; the message names the platform's code | no |
| `DEADLINE` | the invocation's deadline had passed | no |

## What the host does

- One cache entry per function id, shared by every version of the function. A token is served while more than
  `max(60 s, 10% of its lifetime)` remains on the host's clock, and fetched again otherwise.
- Concurrent callers for one function share one fetch. `platformToken(true)` skips the cache and replaces the entry.
- A failed fetch is not cached; the function gets the exception and the next call tries again.
- An entry is dropped when its function leaves the desired document.
- The call is `POST /control/functions/token` on the host's own control client, so it has the same fresh-token retry
  on 401 as the host's other control calls.
- The token is never logged or put in a `toString` or exception message by the host.

## Testing

`FakeFunctionContext.builder(address).platformToken("fake-token")` answers that string;
`.platformTokenFailure(new PlatformTokenException("UNAVAILABLE", "down"))` makes every call throw. With neither, a call
throws `NOT_DECLARED`. `platformTokenCalls()` and `platformTokenRefreshes()` count the calls.
