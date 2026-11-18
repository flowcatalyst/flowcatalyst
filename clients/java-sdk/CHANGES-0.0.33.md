# Java SDK 0.0.33 (unreleased)

Changes since `java-sdk/v0.0.32`. Nothing is tagged or released: `make release-java-sdk BUMP=patch`
cuts 0.0.33 when the owner decides.

Sources of truth: `api/openapi.lock.json`, `docs/wire-contract.md` ("Application service-account
provisioning", "Reading an application's service account").

## `applications().getServiceAccount(id)` works

It calls `GET /api/applications/{id}/service-account`, which the platform did not have until now (it
answered 405); the platform answers it with the `ServiceAccountResponse` of
`GET /api/service-accounts/{id}` (the account's own id, `code`, `name`, `principalId`, `oauthClientId`;
never a secret), 404 for an application without one. The method is unchanged; it needs a platform with
the route. `openapi/openapi.json` carries the new operation (`getApplicationServiceAccount`).

## Retries: a request that may have been applied is not sent again

Until 0.0.32 the transport retried 408, 429, 502, 503 and 504 whatever the method, and never retried
a transport failure. A POST the platform had committed, whose answer was lost at the gateway, was
then sent again: `POST /api/applications/{id}/provision-service-account` was refused 409
`ALREADY_PROVISIONED` on the retry and the one-time secrets of the first answer were lost, and
other POSTs could create duplicates. The rule now (README, "Retries"):

- A request is repeatable when its method is GET, HEAD, OPTIONS, PUT or DELETE, or when it is a
  POST the SDK marks repeatable for that call: the declarative sync endpoints
  (`/api/applications/{app}/{roles,event-types,subscriptions,connections,dispatch-pools,principals,
  processes,scheduled-jobs,openapi,push,docs}/sync` and `/api/principals/sync`), the read-only
  `POST /api/field-tokens/introspect` and the router's
  `POST /monitoring/in-flight-messages/check-batch`, and `POST /api/dispatch-jobs/batch` when every
  job carries an `idempotencyKey`. Every other POST is not repeatable.
- 408 and 429 are retried for every request, as before. 502, 503 and 504 are retried for a
  repeatable request; for any other the call fails at once with the new
  `SdkError.OutcomeUnknown`.
- New: a transport failure before anything was sent (DNS, connection refused, connect timeout,
  TLS handshake) is retried for every request; one after the request may have been sent (reset,
  EOF, the request timeout) is retried for a repeatable request and is `OutcomeUnknown` for any
  other. An interrupt is never retried. A repeatable request whose retries run out ends with the
  same `Timeout`, `Network` or `HttpStatus` as before.
- New: a DELETE that answers 404 on a retry after an attempt with an unknown outcome succeeds with
  no body. A 404 to the first attempt is still `NotFound`.
- `retryAttempts` and `retryDelay` keep their names, defaults and meaning (retries after the first
  request; 0 turns them off). There is no switch that retries every POST.

New in the API:

- `SdkError.OutcomeUnknown(String message, String method, String url, Integer status, String body,
  Throwable cause)`: the request may have been applied and was not repeated. `status` is 502, 503
  or 504 when the gateway answered and null for a transport failure; `cause` is the transport
  failure (also the `FlowCatalystException`'s `getCause()`), null for a status.
- `Transport.postRepeatable(path, body, type)` and `postRepeatable(path, query, body, type)`, and
  overloads of `Transport.exchange(...)` and `Transport.rawAuthenticated(...)` taking a final
  `boolean repeatable`, for a custom call that knows its POST is safe to repeat. The existing
  signatures are unchanged and mark nothing.

How a caller adapts:

- `SdkError` is sealed and gains a variant: an exhaustive `switch` over it with no `default` no
  longer compiles until it adds `case SdkError.OutcomeUnknown u -> ...`.
- A non-repeatable POST answered 502/503/504 was `SdkError.HttpStatus` (after the retries) and is
  now `OutcomeUnknown`; one that failed after sending was `Network` or `Timeout` and is now
  `OutcomeUnknown`. Code that matched those for a POST matches `OutcomeUnknown` as well.
- On `OutcomeUnknown`, read the state before sending again: for `provisionServiceAccount`, get the
  application's service account (`applications().getServiceAccount(id)`); if it exists the first
  attempt provisioned it and its secrets were lost, so regenerate them
  (`POST /api/service-accounts/{id}/regenerate-secret` and the related routes) rather than
  provisioning again.
  For a create, look the record up by its code or identifier.
