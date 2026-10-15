# dispatch-demo

Two TypeScript functions for the FlowCatalyst function runner.

```
 caller (bearer token holding <app>:dispatch:job:create)
   │ POST /fn/<app>.<creator>/dispatch   {"any":"json"}
   ▼
 dispatch-creator ──fn.dispatch──▶ DISPATCH JOB  (kind TASK, source function:<creator address>)
                                      │  platform scheduler/router, signed as the application
                                      ▼
 payload-logger  POST /deliver  → logs the payload, returns {"ack":true}
```

| Directory | Auth | Does |
|---|---|---|
| `dispatch-creator/` | `platform` endpoint `POST /dispatch`; handler requires the caller's verified `permissions` to include `<app>:dispatch:job:create` (403 JSON otherwise; the runner itself answers 401 without a valid token) | Creates one dispatch job whose payload is the posted JSON, addressed to `<app>.payload-logger` `/deliver`. |
| `payload-logger/` | `webhook` endpoint `POST /deliver` (signature verified by the runner); **no subscription** | `fn.log.info("payload received", ...)`, returns `{"ack":true}`. |

## How the job is created (the `dispatch` capability)

A function holds no platform credential (plan section 6.3), so it cannot call `POST /api/dispatch-jobs`.
It uses the `dispatch` capability instead, which mirrors `emit`:

- `dispatch-creator` **declares** the one target it may dispatch to: `fn.dispatches("<app>.payload-logger", "/deliver")`
  (it shows in the describe document as `dispatches`; `fcdev fn build` prints it).
- `fn.dispatch({ target, path, payload, ... })` makes the runner ask the platform to create the job on its behalf.
  The function supplies no URL and no credential.
- The platform checks the pair against this **version's** declaration; requires the target to be a function of
  **the same application** (and client) whose **live** version has a `webhook` endpoint at the path; builds the
  delivery URL itself; and signs the delivery as the application's service account (the same secret
  `payload-logger`'s `webhook` endpoint verifies). A refusal reaches the handler as a `FnError` (`isRetryable`
  is true only for platform trouble); the creator maps it to 502, or 503 when retryable.
- The job is recorded as kind `TASK`, source `function:<creator address>`, code
  `<app>:function:<creator name>:dispatch`, `dataOnly` (so `payload-logger` receives exactly the posted JSON),
  metadata `[{key,value}]` = the creator's `requestedBy` plus the platform's `fn.address`, `fn.version`,
  `fn.invocation`, and `correlationId` from the request's `X-Correlation-Id` when it has one.
- It is **not de-duplicated**: the platform has no dedup for dispatch jobs, so (unlike the old event version
  of this demo) there is no idempotency key; a retried request creates another job.

`payload-logger` used to subscribe to an event type so the platform would turn the creator's event into a job.
Nothing raises that event any more, so the subscription was removed (re-deploying `payload-logger` deletes the
old one at promote). Both functions must belong to the **same application**, which is also what makes the
signing work: the job's code starts with the application, and the application's service account signs it.

## Names

A function's address is `<application code>.<name>`. Its HTTP URL on a runner is
`<runner>/fn/<address>/<path>` (the runner's private entry, port 8095 on fcdev; the CLI's
`fcdev fn invoke <address> /dispatch` builds the same URL). On the owner's platform the application
code is `example`, the existing empty function is `example.test` (database id `fnc_6GKYYCGFWGPEJ`), and
the new function is `example.payload-logger`:

| | address | gets |
|---|---|---|
| existing function | `example.test` | `dispatch-creator` |
| new function | `example.payload-logger` | `payload-logger` |

Both are in application `example`: that is what lets the platform sign the job as the application and
lets `payload-logger`'s `webhook` endpoint verify it. (On another platform, replace `example` and `test`.)

## Build and test

```
cd examples/functions/dispatch-demo/dispatch-creator && npm install && npm test && npm run typecheck
cd ../payload-logger                                 && npm install && npm test && npm run typecheck
```

`APP_CODE` (default `dispatchdemo`) is baked in at build time and becomes the first segment of the
dispatch target address and of the required permission. **Set it to `example` for both functions.**

## Deploy to your fcdev (you run these; nothing here touches anything but your fcdev)

Run from the repository root. Credentials: `fcdev fn` reads the dev credential `fcdev start` wrote, so no
flags are needed against your own fcdev. **Rebuild fcdev first** (`make frontend && make go-build`, or
`go build ./cmd/fcdev`): the `dispatch` capability is new, so a runner built before it refuses the new
function's describe (`dispatches` is an unknown key) and cannot serve the call.

```
export APP=example
export NAME=test
D=examples/functions/dispatch-demo

# 0. once: the application must exist and have a service account (it signs deliveries; promote
#    of payload-logger fails with NO_SIGNING_ACCOUNT otherwise). Skip if already provisioned.
curl -s -c jar -X POST http://localhost:8080/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"admin@flowcatalyst.local","password":"DevPassword123!"}'
APPID=$(curl -s -b jar http://localhost:8080/api/applications/by-code/$APP | jq -r .id)
curl -s -b jar -X POST -H 'Origin: http://localhost:8080' \
  http://localhost:8080/api/applications/$APPID/provision-service-account   # refused if it already has one: fine

# 1. payload-logger first, as a NEW function (the creator's dispatch needs its live version)
APP_CODE=$APP fcdev fn deploy $APP.payload-logger $D/payload-logger --create

# 2. dispatch-creator into the EXISTING function example.test (no --create)
APP_CODE=$APP fcdev fn deploy $APP.$NAME $D/dispatch-creator
```

`deploy` = build + publish + wait READY + promote `live`. The separate steps are
`fcdev fn build <dir>`, `fcdev fn publish $APP.$NAME <dir>/dist/function.js`,
`fcdev fn promote $APP.$NAME <version>`, and `fcdev fn status $APP.$NAME`. If you ran the earlier
(event-based) version of this demo, re-deploying `payload-logger` removes its old subscription at
promote; nothing else needs cleaning up.

Publishing `dispatch-creator` is refused with `DISPATCH_NOT_OWNED` if `APP_CODE` was left at its default
(the target would be `dispatchdemo.payload-logger`, not a function of `example`). Dispatching before
`payload-logger` is live is refused at call time (`DISPATCH_TARGET_UNAVAILABLE`), so deploy it first.

## Permission, role, service account, token

The permission `$APP:dispatch:job:create` is a normal four-segment code
(`application:context:resource:action`). Permissions are defined by the roles that carry them
(role sync); a principal's token `scope` and the function's `req.caller.principal.permissions` are
the permissions of its roles. Session shown with the admin cookie `jar` from step 0.

```
H='Origin: http://localhost:8080'; B=http://localhost:8080
# a role carrying it
curl -s -b jar -H "$H" -H 'Content-Type: application/json' -X POST $B/api/applications/$APP/roles/sync -d \
 '{"roles":[{"name":"dispatcher","displayName":"Dispatch caller","description":"May call dispatch-creator","permissions":["'$APP':dispatch:job:create"]}]}'
# a service account of that application holding the role
SA=$(curl -s -b jar -H "$H" -H 'Content-Type: application/json' -X POST $B/api/service-accounts \
 -d '{"code":"dispatch-demo-caller","name":"dispatch-demo-caller","applicationId":"'$APPID'"}' | jq -r .serviceAccount.id)
curl -s -b jar -H "$H" -H 'Content-Type: application/json' -X PUT $B/api/service-accounts/$SA/roles -d '{"roles":["'$APP':dispatcher"]}'
# its bearer token (anchor-only mint endpoint; 1 hour)
TOKEN=$(curl -s -b jar -H "$H" -X POST $B/api/service-accounts/$SA/token | jq -r .accessToken)
```

(Or in the UI: Applications > roles; Service accounts > create, assign role, "API access".)

## Invoke

```
R=http://127.0.0.1:8095      # fcdev's function runner private entry
curl -s -X POST $R/fn/$APP.$NAME/dispatch -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"order":42,"note":"hello"}'
# -> 202 {"jobId":"..."}
```

Expected: no token 401; a token without the permission 403
`{"error":"FORBIDDEN","message":"requires permission ..."}`; with it 202. A moment later (the
scheduler polls) the dispatch job appears under Dispatch Jobs (kind TASK, source
`function:$APP.$NAME`, code `$APP:function:$NAME:dispatch`, target
`.../fn/$APP.payload-logger/deliver`, metadata `requestedBy`, `fn.address`, `fn.version`,
`fn.invocation`, COMPLETED), and fcdev's log has `payload received` with `guest.payload` set to the
JSON. Send an `X-Correlation-Id` header to have the job carry it as its correlation id.

`fcdev fn invoke $APP.$NAME /dispatch -d '{...}'` also works but signs in as the CLI's own dev
credential, not the demo role; use curl to demonstrate the permission check.

## Checked end to end

The same sequence was run on an isolated platform (`scripts/e2e-platform.sh` with `E2E_FUNCTIONS=1`,
`E2E_DB_MODE=own`): no token 401; a token without the permission 403; a permitted token 202 and the job
COMPLETED with `payload received` in the runner log; and a creator that dispatches to a path it did not
declare, or to a function with no webhook endpoint there, got 502 and created no job.
