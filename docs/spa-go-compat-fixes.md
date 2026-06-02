# SPA ↔ Go API compatibility fixes — handoff (fixes 1–4)

Source: the 2026-06-02 **SPA↔Go compatibility audit** (per-module workflow). Of 154 SPA-called
endpoints, 137 work and 17 are broken. This doc is the executable punch-list for the **safe,
high-impact batch (fixes 1–4)**. Fixes 5–7 are deferred (need entity/persistence/design
decisions) and summarised at the bottom. Background on the root-cause class is in the auto-memory
`spa-go-compat-huma-strictness.md`.

## Working agreement / environment
- Repo: `/Users/andrewgraaff/Developer/flowcatalyst-go`.
- Build: `go build ./...`  · Test: `go test ./internal/platform/<domain>/...`  · Frontend typecheck: `cd frontend && npx vue-tsc -b`.
- **Two API layers** — match the one the SPA actually calls:
  - huma `/api/*` handlers under `internal/platform/<domain>/api/` (request bodies are
    `additionalProperties:false` — unknown fields → 400).
  - BFF chi `/bff/*` handlers under `internal/platform/shared/bff/` (plain `encoding/json`).
    The SPA's `bffFetch(...)` calls hit these; `apiFetch(...)` calls hit `/api/*`.
- **⚠ Concurrent editor:** another process has been editing this repo (frontend styling,
  `principal/*`) and switching branches / moving `main` out from under commits. Before committing:
  `git branch --show-current` + `git status`, stage **only your files by explicit pathspec**
  (`git commit -F - -- <files>`), and confirm with the user if branch state is unclear.
- Commit trailer: `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`.
- Each Go fix needs a redeploy; each **frontend** fix needs the SPA rebuilt + re-embedded to land.

---

## Fix 1 — `event-types` edit → 405 (SPA sends PATCH, Go only registers PUT)
- **SPA:** `frontend/src/api/event-types.ts:73-77` → `bffFetch('/bff/event-types/{id}', {method:'PATCH'})`. Body fields `{name, description}` already match.
- **Go (BFF):** `internal/platform/shared/bff/event_types.go` registers only `r.Put("/{id}", s.update)` → chi returns **405** for PATCH before any handler runs.
- **Fix:** add `r.Patch("/{id}", s.update)` next to the existing `r.Put` (safer than changing the SPA).
- **Verify:** `PATCH /bff/event-types/{id}` with `{name}` returns 200, not 405; the detail-page "Edit name/description" Save works.

## Fix 2 — `PUT /api/identity-providers/{id}` → 204 blanks the detail page
- **SPA:** DetailPage does `provider.value = updated` after PUT; the view is gated by `v-else-if="provider"`, so an undefined result collapses the card.
- **Go:** `internal/platform/identityprovider/api/api.go` — route at ~`:54-63` has `DefaultStatus: http.StatusNoContent`; `update(...) (*emptyOutput, error)` at `:149`. The `get` handler already returns `IdentityProviderResponse` (output Body at `:103`).
- **Fix:** make update return the updated provider with **200**: change the route `DefaultStatus` to `http.StatusOK`, change the update output Body type to `IdentityProviderResponse`, and have `update` re-fetch (or map the committed entity) and return it — mirror `get`.
- **Verify:** PUT returns 200 + full provider JSON; the detail card stays after save.

## Fix 3 — response shape / wrapper-key mismatches

### 3a. scheduled-job instance logs — `{data:[…]}` vs bare array
- **SPA:** `listInstanceLogs` typed `Promise<ScheduledJobInstanceLog[]>` and consumed as a bare array (`ScheduledJobInstanceDetailPage.vue:25`, `logs.length`, `<DataTable :value="logs">`).
- **Go (BFF):** `internal/platform/shared/bff/scheduled_jobs.go` (~`:349`) returns `writeJSON(w, 200, map[string]any{"data": out})`. (There is also a huma `/api/...instances/{instanceId}/logs` at `scheduledjob/api/api.go:144` — the SPA uses the **BFF** path via `bffFetch`; confirm in `frontend/src/api/scheduled-jobs.ts`.)
- **Fix:** return the bare array `out` from the BFF logs handler. Do **not** touch the other `/bff/scheduled-jobs` list handlers — they intentionally return `{data,…}` and are typed `Paginated*`.
- **Verify:** logs render; empty-state guard (`logs.length === 0`) works.

### 3b. event-types applications filter — `{applications:[{value,label}]}` vs `{options:[string]}`
- **SPA:** `useEventTypes.loadApplications()` reads `response.options` as `string[]`.
- **Go (BFF):** `internal/platform/shared/bff/filter_options.go` (~`:115`, `eventTypeApplications`) returns `{"applications":[{value,label}]}`.
- **Fix:** return `{"options": [<app code string>, …]}` to match the sibling filter endpoints in the same file.
- **Verify:** the Applications MultiSelect populates.

### 3c. dispatch-jobs filter-options — ⚠ bigger reshape, may defer
- **SPA:** `DispatchJobFilterOptions` expects `{applications, subdomains, aggregates, codes, statuses}`, each `{value,label}[]` (`DispatchJobListPage` binds `optionLabel="label" optionValue="value"`).
- **Go:** `internal/platform/dispatchjob/api/dto.go:344` `DispatchJobFilterOptionsResponse` emits `{statuses, codes, clientIds, dispatchPoolIds, subscriptionIds, kinds}` as `[]string` (handler `api.go:463-475`).
- **Problem:** Go emits **different facets** and as `[]string`; it does not compute `applications/subdomains/aggregates`. Full alignment needs Go to source those facets — **not trivial**.
- **Action:** confirm with the user before investing. Minimum: change element type to `{value,label}` for the keys the SPA reads; document any facets Go can't cheaply produce.

## Fix 4 — cosmetic (cheap)

### 4a. `POST /api/scheduled-jobs/{id}/fire` toast shows "Instance undefined"
- **Go:** `internal/platform/scheduledjob/api/dto.go:147-150` `FireNowResponse{ScheduledJobID, InstanceID}` (no `id`); SPA reads `result.id`.
- **Fix:** add `ID string \`json:"id"\`` to `FireNowResponse` and set it to the instance id in the fire handler (keep the existing fields). (Server-side add is safer than changing the SPA.)
- **Verify:** fire toast shows the real instance id.

### 4b. `GET /api/events/{id}` Context-Data subsection blank — `context` vs `contextData`
- **Go:** `internal/platform/event/api/dto.go:29` `Context []ContextEntryDTO \`json:"context,omitempty"\`` (a sibling struct at `:139` already uses `contextData`). SPA reads `selectedEvent.contextData`.
- **Fix:** confirm `GET /api/events/{id}` returns `EventResponse` (the `:29` struct), then change its json tag `context` → `contextData`. Check no other consumer relies on `context`.
- **Verify:** the Context Data section renders on the event detail dialog.

### 4c. `POST /api/connections` and `POST /api/identity-providers` return `{id}` only
- **Go:** `connection/api/api.go:156-171` and `identityprovider/api/api.go:126-139` return `apicommon.CreatedResponse{ID}`.
- **Symptom (minor):** SubscriptionCreatePage pushes the returned connection into a `Select` (blank label until reload); IdP create toast shows name `"undefined"`. Navigation by id works.
- **Fix:** return the full `ConnectionResponse` / `IdentityProviderResponse` (fetch/map the created entity, 201) — mirror each `get` handler's Body.
- **Verify:** create returns the full object; dropdown label / toast name correct.

---

## After each fix
`go build ./...` → `go test ./internal/platform/<domain>/...`. Any frontend edits: `cd frontend && npx vue-tsc -b` + `npx oxlint <changed files>`.

## Deferred — 5–7 (do with the user AFTER 1–4)
5. **subscriptions** create/update 400 (huma): SPA sends `clientScoped,queue,sequence,source` (create) / `queue,sequence,connectionId` (update); none on the Go DTOs. Add them and **persist** the ones the entity supports (`connectionId` must). Hard block — can't create/edit subscriptions today.
6. **applications** create/update 400 (huma) when a logo is set: SPA sends `logo,logoMimeType`; Go DTOs lack them. Also `POST /api/applications` must provision a `serviceAccount` + return credentials (the SPA shows a credentials dialog) — a **missing feature**, not just a shape fix.
7. **`GET /principals/check-email-domain`** is a dual-consumer endpoint: the login page wants the slim `{authMethod,loginUrl,idpIssuer}` Go returns; UserCreatePage wants the rich `{requiresClientId,allowedClientIds,emailExists,derivedScope,…}`. Return the **superset** so both work (without breaking the login page).
- **Systemic safety net:** make huma request bodies lenient (accept/ignore unknown fields, matching serde) so future SPA-superset fields stop 400-ing — but fields that must persist still need modeling (#5/#6).

## Already done (do not redo)
- `PUT /api/oauth-clients/{id}` accepts `defaultScopes`/`pkceRequired` (committed to main, `5f2980b`).
- `pkceRequired` response no longer hardcoded `false`; create+update thread it (uncommitted at handoff time — verify with `git log`/`grep "c.PKCERequired" internal/platform/auth/api/dto.go`; if absent, it's in the working tree or needs re-applying).
- Validation errors now include per-field `details.errors` (`httpcompat.go`), and the SPA folds them into the message (`frontend/src/api/client.ts`); OAuthClientDetailPage keeps the form on save error.
