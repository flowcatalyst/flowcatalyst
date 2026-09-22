# Java hand-off: the connection's service account signs dispatch deliveries

Owner ruling 2026-09-22, **reversing** `docs/spec/dispatch-delivery-credentials.md`
(2026-09-19) §2 "Credentials belong to the application". Go implements it in the
commit that adds this file (`internal/server/delivery_creds.go`,
`internal/platform/serviceaccount/secretresolver.go`,
`internal/platform/dispatchjob/processing/processing.go`). No schema or wire
change; `docs/wire-contract.md` "Dispatch-processing delivery request" carries
the new rule.

## Why

The 2026-09-19 rule keyed signing on the *application's oldest active service
account*, and never read `msg_connections.service_account_id` or
`msg_subscriptions.service_account_id`. But the connection create form
**requires** a service account, and the subscription form asks for **no
application** — so both UIs described a credential nothing used. A Laravel
subscriber configured with the connection account's signing secret rejected
every delivery with 401, the job went `FAILED: HTTP 401 Unauthorized` after
three retries, and nothing recorded why. Owner: "What is the point of having
the connection setup asking for a service account."

## Resolution order (replaces spec §2)

The first that **names** an account decides. **No fall-through past a named
account**: an account someone configured must be the one used, or declined
with a reason — silently signing with a different account is how a rotated or
deactivated credential keeps working by accident.

1. `subscription.serviceAccountId` — an explicit override.
2. `subscription.connectionId → connection.serviceAccountId` — the normal case.
3. The application's oldest active service account, found through
   `subscription.applicationCode` or, for a direct job with no subscription,
   the job code's first segment (`billing:invoice:created` → `billing`). This is
   the old rule, kept only as the fallback for a job with no connection to
   name an account.
4. Nothing → bare delivery, **with a reason** (below).

Steps 1–2 resolve the account **by id** and require `active = TRUE`; an
inactive account, a missing account, or one with no webhook credentials yields
*no credentials* and a reason naming what configured it —
`connection cnn-value: service account sa-conn is inactive`. Step 3 is
unchanged (`OutboundCredentials.resolve` by application, oldest active). Both
lookups stay behind the one-minute-per-key cache; add a by-id cache beside the
by-application one (Go: `NewCachedOutboundCredsByIDResolver`, sharing the memo
with `NewCachedOutboundCredsResolver`).

## Never unsigned silently

`OutboundCreds` (Java `DeliveryCredentials.Resolved`) gains a `reason` —
non-empty exactly when both credentials are empty. The processing handler:

- logs `dispatch process: delivering unsigned` at WARN with `job_id`,
  `subscription_id`, `reason` (a resolver that throws still degrades to bare
  delivery with its WARN, as spec §3 says — unchanged);
- on a **failed** attempt appends `" (delivered unsigned: <reason>)"` to the
  attempt's `errorMessage`, so the job page says why the 401 happened
  instead of just that it happened. Success and deferral are untouched.

Reasons, for parity of wording:

| Situation | Reason |
|---|---|
| named account inactive | `<who>: service account <code> is inactive` |
| named account missing | `<who>: service account <id> does not exist` |
| named account without credentials | `<who>: service account <code> has no webhook credentials` |
| application has no active account | `application <code> has no active service account` |
| application unknown | `application <code> does not exist` |
| nothing names anything | `no subscription, connection or application names a service account` |

`<who>` is `subscription <code>` or `connection <code>`.

**No secret in any log field, message, exception or `toString`** — unchanged.

## Tests to mirror (Go `internal/server/delivery_creds_test.go`, fakes, no DB)

Two accounts with **different** secrets — the connection's and the
application's — so which one signs is observable:

- connection account signs; the application lookup is **not consulted**
  (mutant: the old application-first rule);
- subscription account overrides the connection's;
- a named account that is inactive / has no credentials / does not exist →
  bare, reason names `connection <code>`, application **not consulted**
  (mutant: fall through to the application — Go's mutant of this was caught);
- no connection account, or no connection → application fallback;
- direct job: `value:invoice:created` → application `value`; `legacy` → bare
  with the "nothing names" reason; `nosuchapp:x` → "does not exist";
- a lookup error propagates (the caller degrades it, not the resolver).

Spec §5's S1–S8 stay valid with S2/S4 re-read under the new order.

## Still open

- The SPA connection form's "Service Account" field now does what it says; the
  subscription form still has no application field, which only matters for the
  step-3 fallback.
- Retry policy for a config-caused 401 (three attempts cannot fix a missing
  credential) — not ruled; deliveries still retry to `max_retries`.
