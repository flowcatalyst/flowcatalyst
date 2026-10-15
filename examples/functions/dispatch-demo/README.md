# dispatch-demo

Two TypeScript functions for the FlowCatalyst function runner.

```
 caller (bearer token holding <app>:dispatch:job:create)
   │ POST /fn/<app>.<creator>/dispatch   {"any":"json"}
   ▼
 dispatch-creator ──fn.emit──▶ event  <app>:demo:dispatch:requested
                                  │  subscription owned by payload-logger (made at promote)
                                  ▼
                        dispatch job (platform scheduler/router, signed)
                                  ▼
 payload-logger  POST /deliver  → logs the payload, returns {"ack":true}
```

| Directory | Auth | Does |
|---|---|---|
| `dispatch-creator/` | `platform` endpoint `POST /dispatch`; handler requires the caller's verified `permissions` to include `<app>:dispatch:job:create` (403 JSON otherwise; the runner itself answers 401 without a valid token) | Emits one event whose data is the posted JSON. |
| `payload-logger/` | `webhook` endpoint `POST /deliver` (signature verified by the runner); subscribed to the event, `dataOnly`, `IMMEDIATE` | `fn.log.info("payload received", ...)`, returns `{"ack":true}`. |

## Why emit, not the dispatch-job API

A function holds no platform credential (plan section 6.3). Its only write path into the platform
is the `event.emit` capability, performed by the runner on the function's behalf and checked
against the version's declared `emits` (type must be owned by the function's application). Calling
`POST /api/dispatch-jobs` through the `http` capability would need a stored token, a loopback/
private-address exemption and `httpAllow`, and would make the function re-implement the
dispatch-job rules (metadata as an array of `{key,value}`, `clientCode` resolution and the source
default, F14 in `docs/platform-bug-fixes.md`). Emitting an event and letting a subscription
deliver it is the platform's normal path: the platform creates the dispatch job (kind `EVENT`,
source `function:<creator address>`), signs the delivery with the application's service account and
applies those rules itself. With `dataOnly` the delivery body is exactly the JSON that was posted.

## Names

A function's address is `<application code>.<name>`. Its HTTP URL on a runner is
`<runner>/fn/<address>/<path>` (the runner's private entry, port 8095 on fcdev; the CLI's
`fcdev fn invoke <address> /dispatch` builds the same URL). The id `fnc_6GKYYCGFWGPEJ` is the
function's database id; find its address with an admin session or token:

```
curl -s -b jar http://localhost:8080/api/functions/fnc_6GKYYCGFWGPEJ | jq -r .address
```

(it is also on the Functions page of the admin UI). Below, `$APP` is the part before the dot and
`$NAME` the part after.

**Recommendation:** put `dispatch-creator` into the existing empty function `fnc_6GKYYCGFWGPEJ`
and deploy `payload-logger` as a new function `$APP.payload-logger` in the same application
(one application means one signing service account and one owner for the event type).

## Build and test

```
cd examples/functions/dispatch-demo/dispatch-creator && npm install && npm test && npm run typecheck
cd ../payload-logger                                 && npm install && npm test && npm run typecheck
```

`APP_CODE` (default `dispatchdemo`) is baked in at build time and becomes the first segment of the
event type and of the required permission. **Set it to `$APP` for both functions.**

## Deploy to your fcdev (you run these; nothing here touches 8080)

Credentials: `fcdev fn` reads the dev credential `fcdev start` wrote, so no flags are needed
against your own fcdev.

```
export APP=<application code from the address above>
export NAME=<function name from the address above>
D=examples/functions/dispatch-demo

# 0. once: the application must exist and have a service account (it signs deliveries; promote
#    of payload-logger fails with NO_SIGNING_ACCOUNT otherwise). Skip if already provisioned.
curl -s -c jar -X POST http://localhost:8080/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"admin@flowcatalyst.local","password":"DevPassword123!"}'
APPID=$(curl -s -b jar http://localhost:8080/api/applications/by-code/$APP | jq -r .id)
curl -s -b jar -X POST -H 'Origin: http://localhost:8080' \
  http://localhost:8080/api/applications/$APPID/provision-service-account   # refused if it already has one: fine

# 1. payload-logger first (its subscription needs nothing from the creator), as a NEW function
APP_CODE=$APP fcdev fn deploy $APP.payload-logger $D/payload-logger --create

# 2. dispatch-creator into the EXISTING function (no --create)
APP_CODE=$APP fcdev fn deploy $APP.$NAME $D/dispatch-creator
```

`deploy` = build + publish + wait READY + promote `live`. The separate steps are
`fcdev fn build <dir>`, `fcdev fn publish $APP.$NAME <dir>/dist/function.js`,
`fcdev fn promote $APP.$NAME <version>`, and `fcdev fn status $APP.$NAME`. Promoting
payload-logger creates its subscription (`wiring: subscriptionsCreated 1`).

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
# -> 202 {"eventId":"..."}
```

Expected: no token 401; a token without the permission 403
`{"error":"FORBIDDEN","message":"requires permission ..."}`; with it 202. A moment later (the
scheduler polls) the dispatch job appears under Dispatch Jobs (kind EVENT, code
`$APP:demo:dispatch:requested`, target `.../fn/$APP.payload-logger/deliver`, COMPLETED), and
fcdev's log has `payload received` with `guest.payload` set to the JSON. Add an
`Idempotency-Key` header to make a retried request create one event.

`fcdev fn invoke $APP.$NAME /dispatch -d '{...}'` also works but signs in as the CLI's own dev
credential, not the demo role; use curl to demonstrate the permission check.
