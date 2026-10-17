# `fcdev fn dev`: the local development loop

One command for a function project (TypeScript, Go or Rust) with no platform:

```
fcdev fn dev [dir] [--send sample.xml] [--path /events/trip] [--secret K=V] [--config K=V]
```

It builds the project exactly as `fcdev fn build` does, serves the artifact on the
real function runner at `127.0.0.1:8099`, then watches the source tree. Every
change rebuilds and hot-reloads; a successful reload prints one line:

```
ok v3  js in 840ms  size=212KB digest=9f2c41ab07de  routes=2 subscriptions=1 schedules=0 dispatches=2
```

A failed build prints the toolchain's error and the loop carries on: the previous
good build keeps serving until a build succeeds.

## Recommended workflow

1. Unit tests while writing handlers (`npm test`, `go test`, `cargo test`).
2. `fcdev fn dev --send sample.xml --path /events/trip --secret API_KEY=...`
   in a second terminal: edit, save, read the response. The sample file is
   re-read on every send, so editing it resends without a rebuild.
3. On a terminal: `r`+Enter resends the last request, `q`+Enter quits.
4. `fcdev fn deploy` to a non-production platform before production.

## Request replay

`--send FILE` sends the file after every successful (re)load. `--method` (POST),
`--path` (`/`), repeatable `--header 'K: V'`, `--content-type` (default by
extension: `.xml` application/xml, `.json` application/json, else
application/octet-stream). Auth is added from the endpoint the path lands on: a
`webhook` endpoint is signed with `--webhook-secret`, a `platform` endpoint gets a
dev bearer token (any token is accepted locally). The output shows status,
selected headers, the body (JSON pretty-printed, truncated at 8 KiB with a note)
and, grouped under the request, the events emitted and jobs the call would
create. Without `--send` it prints a `curl` line per endpoint.

## Dispatches

A function's `dispatch` call is checked against the version's declared
`dispatches` (`{target, path, connection, pool?}`) with the real route's shape
rules (declared target and path, valid JSON for JSON content types, metadata
limits). A match prints:

```
would create job: target=yard.logger path=/deliver pool=yard-process-pool connection=yard-conn group=trip-42 payload={"id":42}
```

and returns a fake job id; a refusal returns the ABI error to the function and is
printed as `dispatch refused: CODE: message`. (An undeclared target is refused by
the runner itself, as `NOT_DECLARED`, before the control plane is asked, exactly as
on a platform.)

## Flags

`--address` (default `local.<directory name>`), `--port`, `--config K=V`,
`--secret K=V` (values are masked in everything `fn dev` prints), `--db NAME=DSN`,
`--perms` (permissions of the dev principal), `--memory` (MB), `--runtime wasm|js`,
`--webhook-secret`, `--no-watch` (build and serve once).

## Limits

- No platform: events are not ingested and jobs are not created, only printed.
- Pools, connections and target functions are **not resolved**; their existence
  and status, application ownership and the target's webhook endpoint are only
  checked on a real platform.
- No delivery signing by a connection's service account; subscriptions and
  schedules are not fired (call their paths with `--send`).
- Listens on 127.0.0.1 only. Watching is by polling (300ms, mtime and size), and
  ignores `node_modules`, `dist`, `target`, `bin`, `vendor`, `.git`, hidden paths
  and build outputs.
