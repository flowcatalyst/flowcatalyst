# Platform UI: finishing the move off PrimeVue (templ + Datastar)

Status: plan, 2026-10-24. Companion to `docs/ui/README.md` (what is built and how pages are made) and
`flowcatalyst-framework/docs/ui-dogfood-plan.md` (why).

## 1. Decisions (owner, 2026-10-24)

1. The platform's admin UI is **templ + Datastar** on the framework's UI kit, served from the Go binary under `/ui`.
   This confirms the 2026-10-14 ruling; a move of the platform to Nuxt UI was considered and **not** taken.
   Nuxt UI (`@flowcatalyst/ui`) remains the primary UI for **business applications built on the framework**.
2. **The PrimeVue SPA is frozen**: bug fixes and security fixes only. **No new page is written in it.** (Two pages written
   on 2026-10-21 and 10-23, the function trace pages and the platform Configuration page, are in the SPA and are ported
   in wave 2.)
3. New features are built in templ from the start (the ingestion pages, wave 3).
4. As today, `/ui` replaces an SPA area only when the owner approves that area; the SPA stays the default until then.
   When every area is switched, PrimeVue, Node and `frontend/` are removed from the build.
5. **Islands** are allowed where a page needs real client code, as web components or a script on a server-rendered page
   (never a second SPA inside `/ui`): code, JSON and YAML editing; diagrams; WebAuthn; the Swagger viewer.

## 2. Inventory (SPA lines are the Vue single-file components per area)

| Area | SPA lines | In `/ui` today | Remaining work |
|---|---|---|---|
| dispatch pools, connections, events, processes, clients, event types, subscriptions, applications, scheduled jobs | 10,781 | at parity as of 2026-10-10 | none known; **re-check applications** (one SPA commit since) |
| dispatch jobs | 2,370 | ported 2026-10-10; the SPA gained operator tooling on 2026-10-19 (time window, filters, quick lenses, edit payload, requeue by filter, redrive confirmation, split write permissions) | **catch up**, then re-verify |
| functions (incl. trace switches and log) | 2,917 | no | wave 2 |
| platform: Configuration, audit log, CORS, login attempts, docs, theme, names, debug | 3,212 | no | waves 2 and 4 |
| service accounts | 2,578 | no | wave 4 |
| users | 3,594 | no | wave 4 |
| authorization (roles, permissions) | 2,114 | no (the nav links to the SPA) | wave 4 |
| developer portal | 2,103 | no | wave 4 |
| portal (portal users and apps) | 1,104 | no | wave 4 |
| authentication (identity providers, OAuth clients, email domains, reset approvals) | 5,725 | no | wave 5 |
| auth (login, callback, forgot and reset password, logout, passkeys, 2FA) | 2,356 | no | wave 5, last |
| **Total** | **40,210** | about a third, the simpler third | |

## 3. Waves

**Wave 0: kit gaps and islands (before the large areas).** From the kit's gap list and what the remaining areas need:
multi-select with search; password field with strength meter; secret-reference input (`SecretRefInput`); the
permission-denied modal; a **code editor island** (one CodeMirror 6 web component for JSON, YAML and payload editing,
used by dispatch jobs, schemas, ingestion definitions, config values); a **Markdown renderer** server-side (goldmark with a
sanitising policy) for the docs pages; the **diagram island** decision (processes show the source today; vendor Mermaid
as an island, or keep the link; owner decision recorded in `docs/ui/README.md` as open); a **tabs** part (drawer tabs).
Each new part ships with its spec, fixtures and goldens in the kit.

**Wave 1: catch-up and switch-over of the done areas.** Dispatch jobs catches up to the SPA's operator tooling; the nine
other done areas get a final parity pass against the current SPA (diff the SPA's commits since 2026-10-10). Then ask the
owner, area by area, to switch the default. Each switch removes that area's PrimeVue code from the build only at the end
(wave 6), not at the switch.

**Wave 2: Configuration and Functions.** Our newest pages and the ones we know best: the platform Configuration page
(properties table, write-only secrets, audited reveal) and the Functions area including the trace switches (cards) and the
trace log page (filters, auto-refresh over a Datastar stream, expandable request and response records).

**Wave 3: Ingestion (new, built in templ only).** After the backend work packages 1 to 4 of `docs/ingestion-design.md`.
The current Nuxt UI pages (`inhance/mono-repo/apps/ingest-control-plane/web`, about 7,400 lines) are the specification,
not a source to translate: list and filters; the form drawn from the source kinds the runtimes report (a server-side
renderer, which is easier in templ than the SPA's schema-driven form); drawer tabs (activity, failures, history, versions,
definition); test connection as a streamed result; sets and their applies.

**Wave 4: administration.** Service accounts, users, authorization (roles and permissions), developer portal, portal,
and the rest of the platform area (audit log, CORS, login attempts, docs, theme and names, debug). Mostly tables, drawers
and forms; the role and permission editors keep the "cannot grant above your own" rules, which are already enforced on
the server.

**Wave 5: authentication and sign-in (last, highest risk).** Identity providers, OAuth clients, email domains, reset
approvals, then the sign-in pages: password, SSO redirect, passkeys (the WebAuthn island), 2FA set-up and verify, reset
and forgot password. Done last because a defect here locks people out, and with the SPA still available as the fallback
until the owner approves.

**Wave 6: removal.** When every area is switched and approved: delete `frontend/`, PrimeVue, the generated client, and
the Node steps of `make frontend` and the Dockerfile; the SPA's embed goes; `/ui` becomes `/`.

## 4. Per-area acceptance (the existing conventions, made a checklist)

List state in the URL (search, facets, sort, paging); drawer with deep link; create and edit dialogs with field errors on the
field; state actions and delete behind confirmations; every server-side permission gate reflected (controls hidden, never
the only guard); the Datastar error contract; flash messages; phone width; dark mode; compression and CSRF as
documented; fixtures and goldens for new kit parts; and the area **opened in a real browser** (light, dark, narrow) against an
isolated platform before the owner is asked to switch it. The SPA's behaviours that were bugs are recorded in the area's
verification section, as for the earlier areas.

## 5. Rules while it runs

- A change to a switched area's behaviour is made in `/ui`; the SPA copy gets only the same fix if the area is not yet
  switched (parity drift is the main risk of a long migration, so waves 1 and 4 start with a diff of SPA commits).
- Work packages are sized for one agent, one area each, with the area's verification section as the deliverable.
- Datastar is pinned (currently 1.0.4); upgrading it is its own change with the kit's conformance run.

## 6. Risks

| Risk | Mitigation |
|---|---|
| Datastar is young and has a small ecosystem | Pinned version, our own error contract, the kit's fixtures, islands for anything the library cannot do |
| The remaining two thirds contain the hardest areas | They come last; the SPA remains the fallback until each area is approved |
| Parity drift while the SPA is frozen but the platform changes | Backend changes that alter an SPA area's behaviour are reviewed against the `/ui` page in the same change |
| Passkey and 2FA flows need client code | One small WebAuthn island, tested with a virtual authenticator |
| Two component families (kit and `@flowcatalyst/ui`) | Shared tokens; the boundary is platform admin (kit) versus business applications (Nuxt UI) |

## 7. Rulings (owner, 2026-10-24)

1. Wave order as written, functions and configuration before ingestion, sign-in last: **yes**.
2. **Diagram island: vendor Mermaid.** Its published single-file bundle is committed under `internal/ui/assets/` with its
   licence and loaded by a script tag **only on the pages that draw a diagram**, served compressed; no Vite or Node in the
   product build. Diagrams render in Mermaid's strict security mode from the stored source.
3. **Code editor island: CodeMirror 6, vendored.** It has no official single-file bundle, so a small pinned script
   (`tools/`, esbuild) builds one custom bundle (JSON, YAML, basic editing) that is committed with its licence. The script runs
   only when upgrading, never in the normal build.
