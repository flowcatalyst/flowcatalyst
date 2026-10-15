# Java hand-off: the function `dispatch` capability

Go implements it in the commit that adds this file (2026-10-15). The Java platform's function host,
if/when it mirrors functions, needs the pieces below. Behaviour is specified in the Go repo's
`docs/function-runner-plan.md` §5.3, §5.4, §6.3, §8.3 and §8.7; this is the checklist. Nothing here is
implemented on the Java side.

## What it is

A function creates a **dispatch job** directly (`fn.dispatch`), without holding a platform credential,
mirroring `event.emit`. The function names a declared target function and a path; the platform decides
everything else.

## Mirror list

1. **Describe** gains `dispatches: [{target, path}]`. Strict like the rest (unknown keys rejected):
   `target` matches `^[A-Za-z0-9][A-Za-z0-9_-]{0,99}\.[a-z][a-z0-9-]{0,62}$`; `path` is a literal absolute path
   (none of `{ } * ? #`, backslash, whitespace, `%`, `//`, no `.`/`..` segments); a `(target, path)` pair may
   not repeat. Publish additionally refuses (`DISPATCH_NOT_OWNED`, 400) a target whose first label is not the
   function's application code.
2. **ABI op 11 `dispatch.create`** (additive; ABI stays v1). Request meta
   `{target, path, code?, subject?, messageGroup?, mode?, correlationId?, causationId?, contentType?,
   descriptor?, maxRetries?, timeoutSeconds?, dataOnly?, metadata?: [{key,value}]}`, body = the payload.
   Result meta `{jobId}`. The host rejects `(target, path)` not in the invoking version's `dispatches`
   with `NOT_DECLARED` and a payload over 256 KiB with `TOO_LARGE`, then calls the control plane.
3. **Control-plane route** `POST /control/functions/dispatch-jobs` (runner role, anchor scope), body
   `{functionId, version, invocationId, requestCorrelationId, dispatch, payload(base64)}` → `201 {jobId}`.
   `requestCorrelationId` is the inbound request's `X-Correlation-Id`. Client error mapping as `emit`:
   5xx/unreachable `UNAVAILABLE` (the only retryable class), 403 `NOT_ALLOWED`, other 4xx `BAD_REQUEST`,
   context ended `DEADLINE`.
4. **The platform's per-call checks** (all must hold, none trusts the guest), in order, with the Go codes:
   - `(target, path)` is in **this version's** describe (`DISPATCH_NOT_DECLARED`, 403);
   - the target address starts with the function's application code (`DISPATCH_TARGET_NOT_OWNED`, 403 even
     if the function does not exist), the target function has the same application id (same code), and the
     same client id, null equal to null only (`DISPATCH_TARGET_CLIENT_MISMATCH`, 403);
   - the target exists (`DISPATCH_TARGET_NOT_FOUND`, 404) and its `live` alias's version has a `webhook`
     endpoint accepting POST at the path (`DISPATCH_TARGET_UNAVAILABLE`, 409);
   - fields (400): `code` starts with the application code (default
     `{app}:function:{name}:dispatch`), no whitespace, at most 255; payload UTF-8, at most 256 KiB, valid JSON
     for a JSON content type; `mode` one of IMMEDIATE / NEXT_ON_ERROR / BLOCK_ON_ERROR; `maxRetries >= 0`;
     `timeoutSeconds > 0`; metadata at most 50 entries, non-empty unique keys (key at most 100, value at most
     1000), **no key starting with `fn.`** (case-insensitive).
5. **The job written** (through the same mapping as `POST /api/dispatch-jobs`): `kind` TASK, `source`
   `function:{address}`, `code` as above, `targetUrl` = runner URL template with the **target's** pool
   substituted + `/fn/{target}{path}` (the same builder as promote wiring), `payloadContentType` default
   `application/json`, `dataOnly` default **true**, `clientId` = the function's client (no `clientCode`),
   `serviceAccountId` and `dispatchPoolId` unset, `correlationId` = guest's else the request's, `metadata` =
   the guest's entries, then `fn.address`, `fn.version`, `fn.invocation` (when known), plus a `causationId`
   entry if given. `maxRetries` explicit 0 means no retries (the ingest mapping reads 0 as "default"; the
   function route overrides that). **Metadata is an array of `{key,value}` on the wire and in storage.**
6. **Credential resolution needs nothing new**: a job with no subscription resolves the application's
   signing account from the code's first segment, which is the same secret the target function's
   `webhookSecret` carries (delivery-credential rule of 2026-09-22).
7. **Not idempotent**: dispatch jobs have no de-duplication in Go (nor in `POST /api/dispatch-jobs`), so there
   is no `dedupId`. Do not invent one for parity; if Java ever adds job dedup it is a separate decision.

## Wire

OpenAPI: the control route `postFunctionRunnerDispatchJob` and schemas `Dispatch`, `DispatchRequest`,
`DispatchResponse`, `KV` (in `api/openapi.lock.json`). The generated SDKs need `make sdk-generate` (not
run by this change). The control routes are not part of the application SDKs' contract.

## Decisions to keep (and why)

- **Targets are function addresses, never URLs.** A job is signed by the application's account; a URL would
  send that credential wherever the function pointed.
- **Same application and client only.** The signing identity is one application's.
- **Not live is a refusal at call time**, not at publish: a creator may be deployed before its target.
- Deferred: the target's own dispatch pool, `clientCode` for platform-owned functions, URL targets.
