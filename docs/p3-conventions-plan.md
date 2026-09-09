# P3 conventions epic — plan and SDK-compatibility verdict

_Covers owner rulings X-05, X-07, X-09, X-10 (items 24-26, 30 of
[`owner-rulings-todo.md`](./owner-rulings-todo.md); X-02, X-03 and X-08 were pulled out and
already shipped). Written 2026-09-03 against `9f7be62` after surveying the wire contract and
all three SDKs, not from the ruling text alone._

**The brief this answers:** implement these without SDK consumers having to change their app
code. We may ship updated SDKs; what must not move is the SDK's own public contract — the
function signatures app developers call — or it must stay source-compatible with existing
call sites.

## 1. Verdict at a glance

| Ruling | Breaks SDK consumers? | Can it be made invisible? |
|---|---|---|
| **X-10** branded types | No | **Yes** — wire-neutral, one hazard (below) |
| **X-05** one permission per use case | Only string-referencing consumers | **Yes, if additive-only** + new implication infra |
| **X-07** verb semantics | **Yes** — three distinct ways | **Partly.** One part is a bug fix worth taking |
| **X-09** no optional fields | **Yes** — unavoidably, on updates | **No**, as written. Yes, if reinterpreted |

Two live bugs were found while surveying, both worth fixing regardless of whether the
rulings proceed — see §6.

## 2. X-10 — branded types (safe; do first)

Ids, codes, names, URLs and secrets stop being bare `string` at domain boundaries.
No branded-type precedent exists in the tree today; entity factories like `process.New`
are the closest idiom.

**Wire impact: none.** A `type ClientID string` marshals as a JSON string and huma generates
`type: string` for it, so the contract and all three SDKs are untouched.

**The one hazard:** validate-on-construct can start *rejecting values that are accepted
today* — a code containing a space, a URL that does not parse, a name over some length.
That is a behaviour change wearing a typing change's clothes. Mitigation: each wrapper's
constructor must initially encode **exactly** the validation the current write path already
performs — no stricter. Tightening is a separate, deliberate change with its own
compatibility question.

Apply opportunistically, when a file is open for another reason. Not a standalone project.

## 3. X-05 — one permission per use case

**Prerequisite, and a latent bug in its own right:** the permission catalogue exists
**twice**. `internal/platform/seed/permissions.go` declares 151 strings (used to build
roles); `internal/platform/shared/auth/auth.go` independently re-declares **72 of the same
literals** (used to gate requests). They are kept identical by a comment
(`auth.go:14` — "byte-identical to seed/permissions.go") and nothing else. A typo in either
fails closed and silently. Deduplicate to one source of truth **before** any splitting —
it is also the precondition for the ruling's lint.

**Invisible to most consumers.** Splitting `edit` into verbs is a server-side authorization
change: same routes, same 200/403. A consumer that only calls endpoints sees nothing.

**Not invisible to string-referencing consumers**, and that pattern is SDK-documented, not
an edge case:
- `clients/laravel-sdk/src/Auth/FlowCatalystAuthenticatable.php:20-22` shows app code calling
  `$auth->can('platform:messaging:event:view')` against a literal platform string.
- `clients/typescript-sdk/docs/syncing-definitions.md:118` documents an app's role definition
  referencing `platform:iam:user:create`.
- `POST /api/applications/{appCode}/roles/sync` accepts **arbitrary unvalidated strings**
  (`role/operations/sync.go:191-199` validates only the app code and non-emptiness), so an
  unknown permission persists silently and simply never matches.

Permission matching is pure segment-wildcard equality (`auth.go:255-269`): holding
`…:process:manage` does **not** satisfy a check for `…:process:archive`.

**To keep it invisible:**
1. **Additive only.** Never rename or remove any of the 151 existing strings. New verbs get
   new codes; old codes keep working.
2. **Build the umbrella implication before relying on it.** The ruling promises
   `:write`/`:manage` implies every verb for one release. No aliasing or expansion table
   exists — today the pattern is hand-listing the umbrella in each gate's `requireAny(...)`
   (e.g. `CanSyncEventTypes`, `auth.go:450-453`). Doing that per new verb is 70+ manual edits
   with no compile-time check that one was missed. Build a real implication table instead,
   once, and derive the gates from it.
3. Only after a full release on the implication table may an umbrella grant be withdrawn.

## 4. X-07 — verb semantics

Currently **four** behaviours coexist. Confirmed inventory of what happens when an aggregate
is already in the target state:

- **409 with no event** — eventtype `archive` only, and it is BFF/UI-only, not on `/api/`,
  so no SDK caller sees it.
- **Documented idempotent-and-still-emits** — subscription `pause`, application
  `enable-for-client` / `disable-for-client` (the latter's comment explicitly says it emits
  "for audit trail", which X-07 directly contradicts).
- **Unguarded flip-and-emit** — dispatchpool archive/suspend/activate, process archive,
  client suspend/activate, application activate/deactivate, principal activate/deactivate,
  serviceaccount deactivate, scheduledjob pause/archive.
- **No from-state check at all** — scheduledjob `resume`, which un-archives. See §6.

**Three independently observable changes for existing callers:**

1. **Status flips on transition verbs.** `resume` on an already-active subscription or job
   returns 204 today and must return 409 under the ruling. Breaking for the common
   idempotent-retry / poll-and-resume pattern, which today silently succeeds. All three SDKs
   wrap every one of these endpoints.
2. **The un-archive fix** (§6) — a genuine behaviour change, and a bug fix.
3. **Silent webhook loss — the non-obvious one.** Suppressing the event on a no-op is not
   merely an audit decision. `platformsink` writes domain events into `msg_events`, which is
   the same table the fan-out reads (`stream/fan_out.go`) to create dispatch jobs. Matching is
   wildcard-based with no catalogue gate (`subscription/entity.go:72-83`), so a subscriber
   with a broad pattern (`platform:admin:*:*`, `platform:*:*:*`) currently receives a webhook
   for **every redundant** pause/archive/suspend call. Suppressing it stops a delivery they
   get today — with no error, just a webhook that stops arriving.

**Recommended split:**
- **Take now:** the target-state verbs (archive/activate/suspend). They keep their 200/204,
  so the HTTP contract is unchanged; only the redundant event disappears. Gate on the §7
  check first.
- **Take now:** the `resume` un-archive fix — it is a bug.
- **Defer or negotiate:** the 200/204 → 409 flips on transition verbs. This is the only part
  that breaks a well-behaved caller doing nothing wrong, and idempotent retry is a legitimate
  pattern. If it proceeds, it needs an SDK major and a release note; it cannot be hidden,
  because the whole point is that the caller learns the precondition failed.

## 5. X-09 — no optional fields

**The blocker is not the SDK's shape — partial update is a documented contract.** The Laravel
SDK's own DTO states: *"Payload for `PUT /api/subscriptions/{id}`. Only provided fields are
updated."* All three SDKs omit unset fields by construction: TypeScript via `JSON.stringify`
dropping `undefined`, Laravel via a normalizer that skips uninitialised fields, Java via
`Include.NON_NULL` on the shared mapper.

Scale:
- **215 optional business properties across 46 request-body schemas** would become mandatory.
- **13 of 22 `PUT` endpoints have zero required fields** — PATCH-shaped in all but name. There
  are no `/api/` `PATCH` routes to fall back to.
- **~156 nil-guarded "don't change this field" sites** across 16 operation files.
- The existing `RelaxRequestBodies` shim relaxes `additionalProperties`, **not** `required` —
  it solves the opposite problem (callers sending a superset) and is not a usable hook.

**Why an SDK shim cannot hide it.** For a *create*, defaulting an omitted field to `""` is
harmless. For an *update* it is data loss: the SDK does not know the current value, so filling
`""` wipes fields the caller never touched. The only signature-preserving alternative is a
read-modify-write inside the SDK, which adds a round trip and introduces a lost-update race
between the GET and the PUT — a worse bug than the one being fixed.

**CORRECTION (2026-09-03):** an earlier draft of this plan called the create path "free and
invisible". That is wrong for reference-shaped fields, and one case is security-relevant.
`CreateSubscriptionRequest.clientId` is a `*string` consumed as an **authorization-branch
selector**, not as data: `auth.CheckScopeAccess` (`auth.go:384-397`) treats `nil` as
"platform-wide resource, anchor required" and **any** non-nil pointer — including one to `""`
— as "tenant-scoped, check access to that client". It also feeds the uniqueness lookup
(`create.go:74`, `FindByCode(ctx, code, cmd.ClientID)`). An SDK defaulting an omitted
`clientId` to `""` to satisfy a newly-required field would silently reroute every
intentionally platform-wide subscription through the tenant-scoped check. The same caution
applies unverified to `connectionId`, `dispatchPoolId` and `serviceAccountId`, which the
domain represents as nil pointers rather than empty-string sentinels. Free-text fields
(`description`, `iconUrl`) and already-unified fields (`mode`, where
`ParseDispatchMode("")` returns the default) genuinely are safe. **Creates need a per-field
audit, not a blanket default.**

**Language asymmetry, which decides where breakage surfaces:**
- **TypeScript breaks at compile time.** If the generated types mirror the server's new
  required-ness, every existing partial call site (`update(id, { name: "x" })`) fails to
  compile. So the TS SDK's exported types must stay optional *regardless* of the wire — the
  strictness cannot be passed through, it must be absorbed internally.
- **Java does not break at compile time**, which is worse. OpenAPI Generator marks required
  fields `@Nonnull` but no null-checker is configured, and the model keeps a no-arg
  constructor with independent fluent setters. Old caller code compiles unchanged and then
  starts 400ing at runtime, with no advance signal.
- **PHP has no compile-time check either**, and its hand-rolled DTO's documented promise
  ("only provided fields are updated") would be violated silently — existing
  `new UpdateSubscriptionRequest(name: 'x')` calls would begin wiping every other field.

**Recommendation — split by cost:**
- **Nearly free:** `NOT NULL DEFAULT ''` columns and the NULL-normalising migration, plus the
  create path for free-text fields only, after the per-field audit above.
- **Reinterpret the update path:** resolve "absent" to the *current stored value* once, at the
  API boundary, and hand the domain a total command with every field populated. The ~156
  nil-guards collapse into one place and the domain gets exactly the no-tri-state property the
  ruling is after — while the wire stays compatible and no app code moves.
  **This departs from the ruling as written** (which requires absent ⇒ 400 `FIELD_REQUIRED`)
  and keeps optional fields on the wire. It is offered as the way to get the ruling's intent
  at zero consumer cost; taking the literal ruling instead means a coordinated breaking major
  across all three SDKs plus clearing operations for every nil-means-unchanged field, and app
  code that does partial updates must change.

## 6. Two live bugs found while surveying

1. **`resume` un-archives an archived scheduled job.** `ScheduledJob.Resume()`
   (`scheduledjob/entity.go:96-100`) sets `StatusActive` unconditionally, and the operation
   layer adds no from-state guard. `POST /api/scheduled-jobs/{id}/resume` on an ARCHIVED job
   returns 204 and silently reactivates it — it will then be picked up and fired by the cron
   poller. This is the exact failure X-07 names. Worth fixing on its own merits.
2. **`subscription.connectionId` can never be cleared.** `update.go:77-79` documents
   set-if-provided: the binding can be re-pointed but not removed, and there is no clearing
   operation. This is the ruling's own cited example of a truly-nullable field needing one.

## 7. Before X-07 suppresses any event — one check to run

Query production for subscriptions whose event-type binding would match the platform-admin
lifecycle types (`platform:admin:scheduled-job:*`, `platform:iam:*`, and any `*` wildcard).
If none exist, §4's webhook concern is theoretical and target-state verbs can be changed
freely. If any exist, those consumers are receiving webhooks today that would stop, and they
need telling first. Same shape of gate as the X-06 pre-scan, and for the same reason: the
answer lives in production data, not in the code.

## 8. Suggested order

1. Deduplicate the permission catalogue (§3 prerequisite; fixes a latent silent-failure bug).
2. The two bug fixes in §6.
3. X-10 opportunistically; X-09's create/column half.
4. Run §7's check, then X-07's target-state verbs.
5. X-05 additively, with the implication table built first.
6. Decide X-09's update path and X-07's transition verbs — the only two genuinely breaking
   items, both needing an explicit owner call rather than a default.
