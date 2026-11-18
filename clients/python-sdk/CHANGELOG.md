# Changelog

Notable changes to the `flowcatalyst` Python SDK.

## Unreleased

Additive: new parameters are keyword-only with defaults, so existing calls are unchanged.

### Retries
- Fix: a request that is not idempotent (a POST not marked `idempotent=True`) is no longer sent
  again after a 502, 503 or 504. Before, every status in `RetryPolicy.retry_statuses` was retried
  for any method, so a POST the platform had committed but whose answer the gateway lost was sent
  twice (a create could duplicate; a provisioning is refused 409 on the repeat and its one-time
  secrets are lost). 408, 425 and 429 are still retried for every request, and 502/503/504 for
  idempotent ones; a status of 500 or above that a policy adds to `retry_statuses` is retried
  only for idempotent requests. `RetryPolicy`'s names and defaults are unchanged.
- New `OutcomeUnknownError` (exported from `flowcatalyst`), raised instead of retrying when a
  request that is not idempotent got 502/503/504, or failed in transport after it may have been
  sent (reset, read or write timeout, the overall deadline while it was on the wire). It carries
  `method`, `path`, `status`, `body` and `cause`, and says the request may have been applied and
  was not repeated. The instance raised is also a `ServerError` (gateway status), a
  `RequestTimeoutError` (a timeout) or a `NetworkError` (another transport failure), so the
  `except` clauses that caught those before still catch it. How to adapt: catch
  `OutcomeUnknownError` before those, read the state (look the record up), then decide whether to
  send the request again.
- A DELETE answered 404 on a retry after an unknown outcome (a 502/503/504 or a failure after
  sending) now succeeds: `Transport.request` returns an empty 204 response, and the resources'
  `delete` methods return `None` as before. A 404 on the first attempt is still `NotFoundError`.
- `ingestions.pause`, `resume`, `make_version_live` and `ingestion_sets.release` are no longer
  marked idempotent: after a gateway status or a failure after sending they raise
  `OutcomeUnknownError` instead of being sent again. The roles sync and the token request stay
  marked.
- `httpx.UnsupportedProtocol` (a base URL with a scheme httpx cannot send) counts as nothing
  sent.

### Ingestions
- `Ingestion` gains `notices` (undismissed notices; 0 when an older platform omits it),
  `latest_notice`, `latest_notice_at`, `warnings` and `save` (`IngestionSaveOutcome`: `outcome`
  `SAVED | MADE_LIVE | DRAFT | DRAFT_EXISTS | UNCHANGED`, `version`, `awaiting_promote`,
  `message`). Without the promote permission a create or update saves a draft
  (`save.outcome == "DRAFT"`).
- `IngestionVersion` gains `draft`, `digest` and `save`; `IngestionFailure` gains `category`
  (`payload | rejected`), `created_at`, `dismissed_at` and `dismissed_by`.
- `failures()` / `iter_failures()` take `category` and `undismissed`; new `dismiss_failure(id,
  failure_id)` and `secrets(id)` (`IngestionSecretList`: bound names and each runtime's last
  fetch, never a value). The blocking facade has the same.
- `SftpSource.accept_any_host_key`.
- `Target` gains `kind` (`TargetKind`: `DISPATCH_JOB` | `EVENT`), `data_only` and `wrap_non_json`;
  `target_url`, `pool` and `mode` are now optional (an `EVENT` target has none of them).
  `HttpSource` gains `accept` (`Accept`: `content_types`, `body`).

### Platform config
- `set`, `delete` and `reveal_secret` take `on_behalf_of`, sent as `X-On-Behalf-Of`
  (`ON_BEHALF_OF_HEADER`); `Transport.request` takes extra `headers`.
- `ConfigProperty.client_identifier` (the client's identifier beside the stored `client_id`).
- Docs: writing `***` or an empty value to a SECRET is refused (`SECRET_VALUE_MASKED` /
  `SECRET_VALUE_REQUIRED`).
