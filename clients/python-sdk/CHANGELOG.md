# Changelog

Notable changes to the `flowcatalyst` Python SDK.

## Unreleased

Additive: new parameters are keyword-only with defaults, so existing calls are unchanged.

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

### Platform config
- `set`, `delete` and `reveal_secret` take `on_behalf_of`, sent as `X-On-Behalf-Of`
  (`ON_BEHALF_OF_HEADER`); `Transport.request` takes extra `headers`.
- `ConfigProperty.client_identifier` (the client's identifier beside the stored `client_id`).
- Docs: writing `***` or an empty value to a SECRET is refused (`SECRET_VALUE_MASKED` /
  `SECRET_VALUE_REQUIRED`).
