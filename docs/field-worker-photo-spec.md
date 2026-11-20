# Field-worker photos: specification

Status: **accepted 2026-11-20 with the owner rulings of section 14.** Build order: section 12. Branch
`feature/field-worker-photos`. Nothing here is built except what "Build status" says.

The request: a field worker (a driver, a picker, a guard: an identity that signs in on a device with
a worker code and a PIN) can have a photograph of the person, set when the worker is set up, and
shown where it helps people recognise who they are dealing with: an office's list of workers, the
worker's own profile on the phone, a store handing goods to a driver. The same for ordinary users
(principals) is **later work** (section 13).

Parent documents: `docs/field-worker-identity-design.md`, `docs/field-worker-identity-p1-spec.md`
(the worker, the admin API, the policy), `docs/field-worker-sign-in-options-spec.md` (the policy's
other settings), `docs/field-mobile-libraries-spec.md` (the device libraries). Conventions:
`CONVENTIONS.md`, `docs/conventions.md`.

## Build status

| Step | What | State |
|---|---|---|
| 0 | this document | built |
| 1 | `internal/platform/imageproc`: the pure package that validates and re-encodes an upload | built |
| 2 | storage: migration, entities, repositories, use cases, events, deletion | built |
| 3 | the admin API, the OpenAPI lock, the wire contract | built |
| 4 | the console (worker drawer and list) | later |
| 5 | the device routes and the sign-in answer | later |
| 6 | the SDKs | later |
| 7 | the device libraries | later |

## 1. What exists today

- A worker (`internal/platform/fieldidentity/entity.go`, `Identity`) has no image field.
- `principal.UserIdentity.PictureURL` is declared and never set, stored, read or served: there is no
  column for it (`principal/repository.go` says the field is "dropped on write"), and it is on no API
  type. The console and the SPA draw initials.
- No token carries a picture. The field sign-in answer carries `worker {id, workerCode, name}`.
- The binaries store (`FC_APP_UPDATES_STORE`, migration 120) is content-addressed in one global
  digest namespace and its downloads are served without a login; it is **not** reused (section 2).

## 2. Storage

The bytes live in **Postgres** (owner ruling 1: "database"), in a table of their own, so that:

- a photo belongs to exactly one worker row and is read only through the worker's own gate, so one
  client's photos can never be reached through another's;
- deleting a worker, or switching a client's photos off, deletes the bytes in the same transaction;
- no object store, bucket, key or credential is needed, and the feature has no "not configured"
  state.

Migration `122_field_worker_photos.sql` (the next free number on the day; goose runs with
`AllowMissing`):

```
field_worker_photos (
  identity_id    VARCHAR(17) PRIMARY KEY,          -- the worker; no foreign key (docs/foreign-keys.md)
  client_id      VARCHAR(17) NOT NULL,
  version        CHAR(16)    NOT NULL,             -- hex of the first 8 bytes of SHA-256 of the full rendition
  content_type   VARCHAR(20) NOT NULL CHECK (content_type = 'image/jpeg'),
  full_bytes     BYTEA NOT NULL,  full_width INT NOT NULL,  full_height INT NOT NULL,
  thumb_bytes    BYTEA NOT NULL,  thumb_width INT NOT NULL, thumb_height INT NOT NULL,
  source_type    VARCHAR(10) NOT NULL CHECK (source_type IN ('jpeg','png','webp')),
  set_by_type    VARCHAR(20) NOT NULL CHECK (set_by_type IN ('ADMIN','SERVICE_ACCOUNT','WORKER')),
  set_by         VARCHAR(17) NOT NULL,             -- principal id, or the worker's own id
  created_at     TIMESTAMPTZ NOT NULL,
  updated_at     TIMESTAMPTZ NOT NULL
)
CREATE INDEX idx_field_worker_photos_client ON field_worker_photos (client_id);

ALTER TABLE field_identities ADD COLUMN photo_version CHAR(16), ADD COLUMN photo_updated_at TIMESTAMPTZ;
ALTER TABLE field_policies   ADD COLUMN photos_enabled BOOLEAN NOT NULL DEFAULT TRUE,
                             ADD COLUMN worker_may_set_own_photo BOOLEAN NOT NULL DEFAULT FALSE;
```

`field_identities.photo_version` and `photo_updated_at` are NULL while the worker has no photo, so
the worker list and the sign-in answer need no join. `IdentityRepo.Persist` never writes them; only
the photo use cases do, in the same transaction as the photo row. One row per worker: a replace
overwrites it, and no earlier photo is kept. A row lives until it is removed, replaced, its worker is
deleted, or its client's photos are switched off (section 7).

## 3. The image

Done by `internal/platform/imageproc` (step 1). An upload is JPEG, PNG or WebP, decided from the
bytes; a declared `Content-Type` may only agree with them. Defaults, configurable (section 11):
at most 10 MiB, 25 million pixels and 10 000 px on a side, checked from the header before any
pixel is decoded; at most 2 decodes in flight per process. The platform re-encodes: two renditions,
**full** (fit within 512 px) and **thumb** (fit within 128 px), baseline JPEG quality 82, never
enlarged, never cropped, transparency flattened onto white, the JPEG EXIF orientation applied. Every
other byte of the upload (EXIF with its GPS position, XMP, IPTC, comments, ICC profiles, appended or
embedded payloads) is dropped, because the output is the encoder's own, made from pixels. HEIC, GIF
(still or animated), SVG, BMP and TIFF are refused.

Refusals and their codes on the wire (section 5):

| `imageproc` error | HTTP | code |
|---|---|---|
| `ErrEmpty` | 400 | `PHOTO_EMPTY` |
| `ErrTooLarge` (also a `Content-Length` over the limit, checked before the body is read) | 413 | `PHOTO_TOO_LARGE` |
| `ErrUnsupportedType` | 415 | `PHOTO_UNSUPPORTED_TYPE` |
| `ErrTypeMismatch` | 400 | `PHOTO_TYPE_MISMATCH` |
| `ErrTooManyPixels` | 400 | `PHOTO_DIMENSIONS_TOO_LARGE` |
| `ErrCorrupt` | 400 | `PHOTO_UNREADABLE` |
| `ErrBusy` | 503 | `PHOTO_BUSY` |

## 4. The two policy settings

Both are fields of the client's field policy (`field_policies`, `GET/PUT /api/field-policies/{clientId}`).
The policy is replaced whole by a PUT: **a value left out goes back to its default**, so omitting
`photosEnabled` means `true` and omitting `workerMaySetOwnPhoto` means `false`.

**`photosEnabled`** (default `true`; owner ruling 3, "Yes": a client can switch photos off). With it
off for a client:

- setting a photo (from the admin API or, later, the device) is refused: `409 PHOTOS_DISABLED_FOR_CLIENT`;
- reading answers as for a worker with no photo (`404 PHOTO_NOT_FOUND`), the worker JSON has
  `photo: null`, and the sign-in answer (step 5) has no photo version and no right to set one;
- **photos already stored are deleted**, in the same transaction as the policy change, since the
  client is saying it does not want them held. It is not reversible: switching photos on again does
  not bring them back. One `platform:iam:field-policy:updated` event records the count
  (`photosRemoved`); no per-worker event is written.
- The API guards the deletion: a PUT that would delete photos (`photosEnabled` false while the
  client holds any) must carry `confirmPhotoDeletion: true`, else `409 PHOTO_DELETION_NOT_CONFIRMED`
  with `details.count` the number of photos that would be deleted. The console asks first
  (step 4) and sends the confirmation. A PUT that leaves photos off on a client with none needs no
  confirmation.

**`workerMaySetOwnPhoto`** (default `false`). The owner's ruling 2, in his words: "I think that
should be opt in. Some do use that currently. I agree by default no." So it is a setting of the
client's field policy, off by default. When a client switches it on, a signed-in worker may set (and
replace) **and remove** their own photo from the device, through the same image processing and
limits (step 5); when off, the device has no upload and only an administrator or an application's
service account sets it. Switching it off leaves stored photos alone. It has no effect while
`photosEnabled` is false. The sign-in answer tells the device whether it may (`canSetOwnPhoto`) so
an application shows the control only then.

## 5. The admin API (step 3)

All under `/api/field-workers/{id}/photo`, the worker addressed by its `fwu_` id like every other
worker route. **No new permission and no role migration**: setting and removing need
`platform:iam:field-worker:manage`, reading needs `view` or `manage`, with access to the worker's
client, exactly as the other worker routes (`loadWorker`: a worker of a client the caller cannot
reach, or, for an application-confined service account, a worker outside its applications, is
`404 FieldWorker_NOT_FOUND`; a missing permission is `403 PERMISSION_REQUIRED`).

| Route | Answer |
|---|---|
| `PUT /api/field-workers/{id}/photo` | Raw body (`image/jpeg`, `image/png`, `image/webp` or `application/octet-stream`), at most the limit. Sets or replaces. `200` with the worker. |
| `DELETE /api/field-workers/{id}/photo` | Removes it. `200` with the worker; removing a missing photo is a no-op. |
| `GET /api/field-workers/{id}/photo?size=thumb\|full` | `200 image/jpeg` (`thumb` by default); `404 PHOTO_NOT_FOUND` when there is none; `304` for a matching `If-None-Match`. |

Reading headers: `ETag: "<version>-<size>"`, `Cache-Control: private, no-cache` (the browser
revalidates, so a removal or a lost permission takes effect at once), `X-Content-Type-Options:
nosniff`, `Content-Disposition: inline`. A caller may append `?v=<version>` to cache hard.

The worker JSON gains `photo: {version, updatedAt}` or `null`, never a URL: the route is fixed.
`FieldPolicyResponse` and `PutFieldPolicyRequest` gain `photosEnabled` and `workerMaySetOwnPhoto`;
the request also gains `confirmPhotoDeletion`.

Why not signed URLs: a capability URL for a photograph of a person leaks through logs, referrers
and chat, and needs a signing-key story. Browsers send the console's session cookie to the
same-origin `/api` route (an `<img>` works), services send a bearer token, the device signs its
requests with its key (step 5), and an application's backend proxies a photo to its own pages and
to its other workers, deciding by its own rule who may see whom (the platform cannot know that a
store's staff may see the driver collecting).

Switches and their codes (distinct on purpose):

| Condition | Answer |
|---|---|
| platform-wide `FC_FIELD_PHOTOS=off` | `404 PHOTOS_DISABLED` on all three routes; the worker JSON has `photo: null` |
| the client's `photosEnabled` is false | `PUT`: `409 PHOTOS_DISABLED_FOR_CLIENT`; `GET`: `404 PHOTO_NOT_FOUND`; `DELETE`: `200` (nothing to remove) |

## 6. Who may see a photo

The administrators who manage or view workers; an application's backend through its service account
(workers of its own applications only); the worker themselves on their device (step 5). It is in no
token claim. Another worker's photo is never served to a device: if a store's staff must see the
driver collecting, the application's backend fetches it with its service account.

## 7. Events, audit, deletion, retention

- New events `platform:iam:field-worker:photo-set` (`workerId`, `clientId`, `workerCode`, `version`,
  `setByType`, `source`) and `platform:iam:field-worker:photo-removed` (`workerId`, `clientId`,
  `workerCode`, `removedByType`). `platform:iam:field-policy:updated` gains `photosEnabled`,
  `workerMaySetOwnPhoto` and `photosRemoved`. All are catalogued and have seeded schemas. **No event
  and no audit row ever holds image bytes**: the commands carry the processed image outside their
  JSON (`json:"-"`), and a test reads the audit rows and the outbox to prove it.
- Audit operations: `SetFieldWorkerPhoto`, `RemoveFieldWorkerPhoto` (and `PutFieldPolicy`).
- Actor: `ADMIN` (a user), `SERVICE_ACCOUNT`, or `WORKER` (the worker themselves, step 5).
- Worker deleted: the photo row is deleted in the same transaction (`IdentityRepo.Delete`); the
  worker's deletion event covers it. Worker suspended or activated: the photo is untouched. No
  operation moves a worker between clients.
- A client switches photos off: section 4.
- Photos stay in database backups until those rotate.

## 8. Privacy position

It is a photograph of a person, kept for identification. It is set by an administrator, an
application's service account or (opt-in, section 4) the worker, and is removable at any time. It is
deleted with the worker, and with the client's photos when the client switches them off. It is never
logged (logs carry the worker id, the version, the sizes and the outcome; never bytes, file names or
anything read from the upload's metadata), never in a token, and never in an event or an audit row
beyond "photo set" or "photo removed". The platform does no recognition or matching.

## 9. The console (step 4)

The worker drawer shows the thumb (`<img src="/api/field-workers/{id}/photo?size=thumb&v=…">`, the
session cookie authenticates it; the page policy already allows same-origin images) or initials, with
"Change photo" (a multipart form to a `/ui` route that runs the same image processing and use case)
and "Remove photo" (a confirmation). The worker list gets a small avatar. The policy page gets the
two settings; switching `photosEnabled` off asks first, saying how many photos will be deleted and
that it cannot be undone. The kit has no avatar or file component, so the page uses its own markup
(the definitions import dialog is the precedent).

## 10. The device (step 5)

- `worker` in the sign-in answer gains `photoVersion` (null when none, or when photos are off for the
  client) and `canSetOwnPhoto`.
- `POST /field/worker/photo` (DPoP proof and access token) returns the signed-in worker's own
  `thumb` or `full`; only the token's own worker, rate limited.
- `POST /field/worker/photo/set` and `POST /field/worker/photo/remove`, same authentication, only
  the token's own worker, rate limited, only when the client's policy has `workerMaySetOwnPhoto`
  (else `403 OWN_PHOTO_NOT_ALLOWED`) and `photosEnabled` (else `409 PHOTOS_DISABLED_FOR_CLIENT`).
  The protocol's 64 KiB body cap is raised for the set route only, to the photo limit.
- The event and the audit row say the worker themselves set it (`setByType` `WORKER`).

## 11. Configuration

| Variable | Default | |
|---|---|---|
| `FC_FIELD_PHOTOS` | on | `off` (or `false`, `0`) turns the feature off platform-wide: `404 PHOTOS_DISABLED` |
| `FC_FIELD_PHOTO_MAX_BYTES` | 10485760 | upload limit |
| `FC_FIELD_PHOTO_MAX_PIXELS` | 25000000 | pixel limit |
| `FC_FIELD_PHOTO_CONCURRENCY` | 2 | decodes in flight per process |

A decode of a 25 MP image holds about 100 MB, so two at once is the memory budget; lower the pixel
limit before raising the concurrency.

## 12. Order of work, releases

1. `imageproc` (built). 0. this document. 2. Storage (migration, entities, repositories, use cases,
   events, seeds, deletion). 3. The admin API, `make api-bump`, the SDK inputs, the SPA's generated
   types, `docs/wire-contract.md`, `docs/environment-variables.md`. 4. Console. 5. Device routes and
   the sign-in answer. 6. SDKs (generated TS, Laravel, Java; the Go SDK by hand). 7. Device libraries
   (`field-js`, `field-react-native`, `field-android`, `field-ios`).

Every step passes `make ci`. Releases (`docs/releasing.md`, `scripts/release.sh`): the platform first
(`release-dev`) after steps 3 and again after 5, then `go-sdk`, `ts`, `laravel`, `java` after 6, then
the four device libraries after 7. An application needs the platform release of step 3 and an SDK
released after it to set a photo, and the platform release of step 5 and a device library after it
for the phone. Everything added to a response is optional, so older SDKs and libraries keep working.

## 13. Not in this piece: users

Ordinary users' photos are later, a separate piece (owner ruling 4, "Yes leave it."): the same
image package, a `principal_photos` table, `/api/principals/{id}/photo`, a self-service upload on
`/ui/profile`, the user-management permissions. An uploaded picture will win over any identity
provider's, and a sign-in will never overwrite it. `PictureURL` is dead today (section 1).

## 14. Owner rulings (2026-11-20)

1. Storage: "database".
2. A worker setting their own photo from the device: "I think that should be opt in. Some do use that
   currently. I agree by default no." (section 4).
3. A client being able to switch photos off: "Yes" (section 4).
4. Users' photos: "Yes leave it." (section 13).
