# Definition export and import (both UIs)

Copying a record that was set up by hand, not by an SDK, from one environment to
another (QA to production): the detail drawer **exports** the record's safe values as
JSON, and the create form **imports** that JSON to fill itself in. Owner ruling
2026-10-29: this is a UI convenience only.

- **No writes.** Export reads what the drawer already shows. Import only fills the
  create form; nothing is saved until the user presses Create as usual, and the usual
  create checks (permissions, validation, `CODE_EXISTS`) apply. No `/api` endpoint is
  added or changed.
- **No secrets, no ids.** Ids differ per environment, so every reference is by **code**
  (or client identifier) and is matched against this environment's records on import.
  Credentials (signing secrets, auth tokens, client secrets) are never exported.
- **Never skip silently.** Anything the import can't apply is listed to the user (see
  Import rules), never dropped quietly.
- **One format for both UIs.** A file exported by the SPA imports into `/ui` and the
  reverse. The examples in `internal/ui/pages/testdata/definitions/*.json` are the
  contract; both UIs' tests read them.

Kinds: `application`, `service-account`, `connection`, `dispatch-pool`, `subscription`.
Not event types (owner ruling 2026-10-29; they come from SDK sync).

## Envelope

```json
{
  "flowcatalyst": "definition",
  "kind": "connection",
  "version": 1,
  "definition": { ... }
}
```

- `flowcatalyst` is always `"definition"`; `kind` is one of the five above; `version`
  is `1`. Import refuses anything else (wrong kind for this form, unknown version, not
  JSON) with a message and leaves the form untouched.
- Export writes the keys in the order listed below, standard two-space pretty-printing
  (every array element and object on its own line, as `JSON.stringify(v, null, 2)` and
  Go's `json.Encoder` with `SetIndent("", "  ")` and `SetEscapeHTML(false)` both
  produce), a trailing newline. Keys whose value is not set (`null`, `""`, an empty
  array) are **omitted**; `0` and `false` are values and are kept. Numbers are JSON
  numbers, booleans JSON booleans.
- Download file name: `<kind>-<code>.json`, e.g. `connection-billing-hooks.json`.

## Definitions (key order = export order)

| Kind | Keys | Notes |
|---|---|---|
| `application` | `code`, `name`, `description`, `type`, `defaultBaseUrl`, `iconUrl`, `website`, `logo`, `logoMimeType` | `type` is `APPLICATION` or `INTEGRATION`. `logo` is the stored logo content (e.g. SVG text). |
| `service-account` | `code`, `name`, `description`, `scope`, `clientIdentifiers`, `applicationCode`, `roles` | `clientIdentifiers` replaces the client ids; `applicationCode` the application id. `roles` are role names (`app:role`), portable as they are. No credentials, no OAuth client id, no principal id. |
| `connection` | `code`, `name`, `description`, `externalId`, `serviceAccountCode`, `clientIdentifier` | `serviceAccountCode` replaces `serviceAccountId` (owner ruling: never the id; match on code). |
| `dispatch-pool` | `code`, `name`, `description`, `rateLimit`, `concurrency`, `clientIdentifier` | `rateLimit` omitted for a concurrency-only pool. |
| `subscription` | `code`, `applicationCode`, `name`, `description`, `endpoint`, `clientScoped`, `clientIdentifier`, `eventTypes`, `connectionCode`, `queue`, `dispatchPoolCode`, `mode`, `maxAgeSeconds`, `timeoutSeconds`, `delaySeconds`, `sequence` | `eventTypes` is `[{ "eventTypeCode", "specVersion" }]` (`specVersion` omitted when unset). `connectionCode` replaces `connectionId`, `dispatchPoolCode` the pool id. `customConfig` is not exported: neither create form has it. |

Never exported, for any kind: `id`, `status`, `source`, `createdAt`, `updatedAt`,
`createdBy`, any credential, any other id.

## Import rules

1. **Where:** an "Import from JSON" control on the **create** form only (not edit).
   Shown to whoever may open the create form. Paste the JSON or choose a file.
2. **Fill, never submit.** The form's fields are set from `definition`; the user
   reviews and presses Create. Fields the JSON doesn't have keep their defaults.
3. **References by code** (`applicationCode`, `serviceAccountCode`, `connectionCode`,
   `dispatchPoolCode`, `clientIdentifier(s)`, `eventTypes[].eventTypeCode`) are matched
   against the choices the form already offers. A match selects it. **No match**:
   the field is left empty and the notice lists it, e.g. "Connection `billing-hooks`
   was not found here. Import or create it first, then choose it."
4. **Values the create form can't take** (e.g. a service account's `roles`, or a key the
   form has no field for) are listed in the notice as "not applied by this form", with
   the values, so the user can do them after creating (e.g. "Assign roles after
   creating: `orders:dispatcher`").
5. **Unknown keys** in `definition` are listed in the notice as ignored.
6. The notice stays visible on the form until the dialog/drawer closes; an import with
   nothing to report says so briefly ("Imported. Review and create.").
7. Importing a code that already exists here is not pre-checked: the normal create
   error (`CODE_EXISTS`) shows on submit.

## Export rules

- **Where:** the detail drawer, view mode, for whoever can open the drawer: an
  "Export JSON" action that shows the JSON (read-only, monospace) with **Copy** and
  **Download**.
- References are written as codes. If a referenced record can't be read by the viewer
  (no permission, or gone), its key is **omitted** and the export view says which, e.g.
  "Connection not included: you can't read connections." Never writes an id instead.

## Per UI

- **SPA** (`frontend/`): pure client code. Export builds the JSON from the drawer's
  loaded record plus reads it already does (or the same `GET`s for the referenced
  records' codes). Import resolves codes against the lists the create drawer loads.
  Shared helpers in `frontend/src/utils/definition-transfer.ts`.
- **`/ui`** (Datastar, server-rendered): the same behaviour rendered by the Go page
  handlers, read-only. Export: `GET /ui/<page>/{id}/export` renders the JSON view
  (Datastar) and `GET /ui/<page>/{id}/export.json` downloads it
  (`Content-Disposition: attachment`). Import: `POST /ui/<page>/new/import` (form field
  `definition`, or a file) parses the JSON and re-renders the create dialog filled in,
  with the notice. Neither touches the use cases or writes anything.
