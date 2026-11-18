# App updates: a self-hosted Expo Updates server

Status: first slice, built. The platform serves over-the-air updates to phone apps built with
Expo, over the open Expo Updates protocol (version 1), to the open-source `expo-updates` client
library. No hosted Expo service is involved. Console pages, adoption metrics, bundle diffing and
SDK methods are later slices (section 12).

## 1. What it is

An application registered on the platform (identified by its **code**) owns one or more phone
apps. Each phone app is named by an **app key** (`driver-app`); the platform keeps no registry of
them in this slice, the key is a validated path segment, so a second phone app is a second key, not
a schema change. A phone app's binary is built with `expo-updates`, the server's manifest address,
a **channel** (a request header), a **runtime version** and an embedded code-signing certificate.

An **update** belongs to exactly one *branch*: (application, app key, platform, runtime version,
channel). A phone asks for its branch's update; the server answers with the newest live update
(subject to the staged rollout), a signed `noUpdateAvailable`, or a signed `rollBackToEmbedded`.

## 2. The protocol as the client implements it

Verified against the client library source, not written from memory:

| Source | Version | What was read |
|---|---|---|
| `expo` (the SDK the first app is on) | 58.0.23 | `bundledNativeModules.json`: `expo-updates ~58.0.21`, `expo-manifests ~58.0.1`, `expo-eas-client ~58.0.2` |
| `expo-updates` | 58.0.21 (58.0.22, the newest match, differs only in the state machine) | Android `loader/FileDownloader.kt`, `loader/RemoteUpdate.kt`, `loader/CompressionInterceptor.kt`, `codesigning/*.kt`, `manifest/ExpoUpdatesUpdate.kt`, `manifest/ResponseHeaderData.kt`, `manifest/UpdateFactory.kt`, `UpdatesUtils.kt`, `UpdatesConfiguration.kt`, `selectionpolicy/*FilterAware.kt`, `db/entity/AssetEntity.kt`; iOS `AppLoader/FileDownloader.swift`, `AppLoader/UpdateResponse.swift`, `Multipart/UpdatesMultipartStreamReader.swift`, `CodeSigning/*.swift`, `Update/ExpoUpdatesUpdate.swift`; `cli/src/generateCodeSigningAsync.ts`, `configureCodeSigningAsync.ts`, `resolveRuntimeVersion.ts` |
| `expo-manifests` | 58.0.1 | `ExpoUpdatesManifest` (Android and iOS): which manifest fields are required |
| `expo-eas-client` | 58.0.2 | `EASClientID`: the installation id is a UUID kept in shared preferences |
| `@expo/code-signing-certificates` | 0.0.7 | `generateSelfSignedCodeSigningCertificate`: the certificate the tooling makes |
| `@expo/cli` | 58.1.21 | `export/createMetadataJson.js`, `export/exportApp.js`: the `metadata.json` of `expo export` |
| Specification | `docs/pages/technical-specs/expo-updates-1.mdx`, expo/expo branch `sdk-58` (modified 12 Aug 2026) | the whole page |

The packages were fetched with `npm pack` into a scratch directory outside the repository.

**Manifest request** (`GET` the configured update URL). The client sends `Accept:
multipart/mixed,application/expo+json,application/json`, `Expo-Platform: android|ios`,
`Expo-Protocol-Version: 1`, `Expo-API-Version: 1`, `Expo-Updates-Environment: BARE`,
`Expo-JSON-Error: true`, `EAS-Client-ID: <uuid>`, `Expo-Runtime-Version`, every header of the app
config's `updates.requestHeaders` (where the channel travels, `expo-channel-name` by convention),
`Expo-Current-Update-ID`, `Expo-Embedded-Update-ID`, `Expo-Recent-Failed-Update-IDs`,
`Expo-Fatal-Error` (at most 1024 characters) and, when code signing is configured,
`expo-expect-signature: sig, keyid="<keyid>", alg="rsa-v1_5-sha256"` (an SFV dictionary).

**Manifest response.** `multipart/mixed` with parts named (by `content-disposition` `name=`)
`manifest`, `extensions`, `directive`; outer headers `expo-protocol-version: 1` (required: without
it the client throws "Legacy manifests are no longer supported"), `expo-sfv-version: 0`,
`cache-control: private, max-age=0`. The platform sends no `expo-manifest-filters` and no
`expo-server-defined-headers` (both optional; without filters every update matches). A part carries
`expo-signature: sig="<base64>", keyid="<id>", alg="rsa-v1_5-sha256"` over the part's exact body
bytes.

**Manifest body.** `{id (UUID), createdAt, runtimeVersion, launchAsset, assets[], metadata {},
extra}`; an asset is `{hash, key, contentType, fileExtension, url}`, `hash` the base64url SHA-256
(no padding), `key` the asset's MD5 (the file name `expo export` gives it, which `expo-asset`
looks it up by), `url` relative to the manifest URL by default (both clients resolve it against the
update URL: Android `ExpoUpdatesUpdate.resolveManifestAssetUrls`, iOS the same name). `extra` holds
`expoClient` (the public app config) when the publisher supplies it: it becomes
`Constants.expoConfig` in the running update.

**Directives.** `{"type":"noUpdateAvailable"}` and `{"type":"rollBackToEmbedded","parameters":
{"commitTime":"<time>"}}`.

**Assets.** `GET` the asset URL with `Accept: */*`, the same `Expo-*` and `EAS-Client-ID` headers,
`Accept-Encoding: zstd, br, gzip` (Android's interceptor). The client checks the base64url SHA-256
of the body against `hash`. The platform serves uncompressed (the specification allows it).

### Where the client and the specification differ

The client wins. Each is something the server does because of it:

1. **`fileExtension` is required** on every non-launch asset by both clients (iOS `requiredValue`,
   which raises; Android `getString`, which skips the asset with an error). The specification
   makes it optional. The server always sends it, with its leading dot.
2. **`launchAsset.key` is required** by Android (`getString("key")`). The server always sends it.
3. **`createdAt` and `commitTime`** must be `yyyy-MM-dd'T'HH:mm:ss.SSS'Z'` exactly: Android parses
   only that shape (`UpdatesUtils.parseDateString`) and otherwise **substitutes the current time**,
   which would break ordering. The specification says "ISO 8601".
4. **The iOS part-header lookup is case-sensitive** (`optionalValue(forKey: "expo-signature")` on
   a plain dictionary). The server writes every part header name in lower case.
5. **The iOS multipart reader needs the closing delimiter followed by CRLF**
   (`UpdatesMultipartStreamReader`: `\r\n--boundary--\r\n`). Go's `mime/multipart` writes it.
6. **A 204 is accepted only with `expo-protocol-version` > 0.** The server never answers 204; "no
   update" is a signed `noUpdateAvailable` directive.
7. **`noUpdateAvailable`** is implemented by both clients but not mentioned by the specification;
   **`rollBackToEmbedded`'s `parameters.commitTime`** is required by both clients and not defined
   by the specification.
8. **Ordering.** The client loads a new manifest only if its `createdAt` is later than the
   launched update's (`LoaderSelectionPolicyFilterAware.shouldLoadNewUpdate`) and launches the
   newest stored one (`LauncherSelectionPolicyFilterAware`). So "make the previous update live
   again" cannot re-serve the old manifest: it is a **republish**, a new update (new UUID, new
   `createdAt`) with the old update's files. A `rollBackToEmbedded` only acts if its `commitTime` is
   later than the launched update's, so it carries the time of the rollback.
9. **The signature encoding** is standard base64 (Android `Base64.DEFAULT`, iOS
   `Data(base64Encoded:)`), carried as an SFV string. The specification does not say which base64.
10. **Channels are not in the specification.** They are whatever request header the app config
    sends. The platform reads `expo-channel-name` (the convention `expo-updates` documents).
11. **`EAS-Client-ID`** is a UUID, upper case from iOS, lower case from Android; the server
    normalises it before bucketing.
12. **The certificate's validity is checked at every verification** (`checkValidity()` on Android,
    `X509Certificate.checkValidity()` on iOS): an expired certificate stops every update, and a
    phone whose clock is before `notBefore` refuses it.

### Seen on an Android emulator

A release build of an Expo SDK 58 app (expo-updates 58.0.22, Android 16 emulator) against `fcdev`,
every request logged by a proxy in between: first launch with nothing published, three updates in
turn, a staged rollout held back at the phone's bucket and taken one percent higher, rollback to the
previous update, rollback to the embedded bundle, a publish after it, a changed stored file, a
changed manifest and a changed rollback directive (refused: "Code signing verification failed for
manifest" / "for directive"), updates on another runtime version and another channel (never
served), and the server stopped (the app starts on what it has). Each update was downloaded in the
background at one start and ran from the next; no start relaunched the app.

- **Manifest requests** carry the headers listed above, and also `If-Modified-Since` (OkHttp's
  cache revalidating the previous response; the server answers 200 every time) and
  `User-Agent: okhttp/4.12.0`. `Expo-Recent-Failed-Update-IDs` and `Expo-Fatal-Error` were never
  sent: a download that fails (a hash mismatch, a cut-off file, no server) is not a failed launch.
- **Asset requests** carry `Expo-Requested-Update-ID` (the manifest's id) and `A-IM: bsdiff` (the
  binary's `ENABLE_BSDIFF_PATCH_SUPPORT` is true by default) besides the `Expo-*` and
  `EAS-Client-ID` headers; not `Expo-Expect-Signature`. The server ignores `A-IM` and sends the whole
  file with 200, which the client accepts. Assets the binary already embeds (same `key`) are not
  downloaded: an update that changes only the bundle costs one request.
- **`rollBackToEmbedded` while the embedded bundle already runs** (the question this section left
  open): nothing happens. The client reports the check as "update available" at every start
  (`CheckCompleteAvailable`) but finishes with "No update available", starts once per launch and
  never relaunches; a phone on an update gets the directive at one start and runs the embedded
  bundle from the next.
- **The client's HTTP cache keeps assets**: see section 8, "A file is verified as it is served".

## 3. Model and tables

Migration `117_app_updates.sql`, three tables, no foreign keys (`docs/foreign-keys.md`: these are
written per publish and will be pruned):

- `app_update_files (application_id, sha256, md5, size_bytes, created_at, created_by)`, primary key
  `(application_id, sha256)`: the **file registry**, which files an application has stored. A row is
  written only after the store accepted the bytes and verified their SHA-256. The store itself is
  content-addressed across applications; the registry is per application, so asking which files
  are missing tells an application nothing about another's.
- `app_updates`: one row per update or rollback, id `upd_…`: `application_id`, `app_key`,
  `platform` (`android|ios`), `runtime_version`, `channel`, `kind` (`UPDATE |
  ROLLBACK_TO_EMBEDDED`), `manifest_id` (UUID, updates only), `created_at` (the manifest's
  `createdAt` or the directive's `commitTime`, strictly increasing per branch), `rollout_percent`
  (0..100), `message`, `source_commit`, `republished_from`, `expo_client` (JSON), `published_by`,
  `updated_at`. Index on the branch plus `created_at DESC, id DESC`.
- `app_update_assets (update_id, ordinal, sha256, md5, file_extension, content_type,
  is_launch_asset)`: the manifest's asset list in order (ordinal 0 is the launch asset). Index on
  `sha256` for the asset route.

An update's rows are immutable after publish except `rollout_percent` (and `updated_at`).

## 4. Addresses

Public (a phone asks before anyone signs in; section 8):

| Route | Purpose |
|---|---|
| `GET /app-updates/{applicationCode}/{appKey}/manifest` | the manifest request |
| `GET /app-updates/{applicationCode}/{appKey}/assets/{sha256}` | one file of a published update of that phone app |

The app's config: `updates.url = https://updates.example.com/app-updates/acme/driver-app/manifest`,
`updates.requestHeaders = {"expo-channel-name": "production"}`.

Authenticated API (bearer, `/api/app-updates/*`, huma, in the OpenAPI lock; tag `app-updates`):

| Route | Permission | Purpose |
|---|---|---|
| `POST /api/app-updates/files/{applicationCode}/missing` `{sha256[]}` | publish | which of these files the application has not stored |
| `PUT /api/app-updates/files/{applicationCode}/{sha256}` raw body | publish | store one file (verified against its SHA-256; idempotent) |
| `POST /api/app-updates/updates` | publish | register an update: it is live when this returns |
| `GET /api/app-updates/updates?applicationCode&appKey&platform&runtimeVersion&channel` | view | updates newest first, paged, with what is live |
| `GET /api/app-updates/updates/{id}` | view | one update with its assets |
| `GET /api/app-updates/branches?applicationCode&appKey` | view | each branch with its live update and rollout |
| `PUT /api/app-updates/updates/{id}/rollout` `{rolloutPercent}` | rollout | change a staged rollout |
| `POST /api/app-updates/updates/{id}/republish` | rollout | make an earlier update live again (as a new update) |
| `POST /api/app-updates/rollback` `{…branch, to: PREVIOUS|EMBEDDED}` | rollout | roll back to the previous update, or to the embedded bundle |

## 5. Publishing

1. The publisher (normally `fcdev updates publish` in a pipeline) reads the `expo export` output:
   `metadata.json` (`fileMetadata.<platform>.bundle` and `.assets[] {path, ext}`), hashes every file,
   asks `…/missing`, and uploads only the missing ones. Content addressing makes a retry or a
   re-publish upload nothing twice.
2. It registers the update per platform: application code, app key, platform, runtime version,
   channel, rollout, the launch asset and asset list (SHA-256 and extension each), the public app
   config (`expoClient`), a message and a source commit.
3. Registration, in one transaction (a `usecaseop.TxOperation`): takes a transaction advisory lock on
   the branch, checks that **every** file is in the application's registry (else `409
   FILES_MISSING` listing them), refuses while the branch's newest update is a partial rollout
   (`409 ROLLOUT_IN_PROGRESS`), sets `createdAt` to the later of now and the branch's newest
   `created_at` plus one millisecond, writes the update and its assets, and records the domain event
   and the audit row. A phone reads only committed rows, so it is never told about an update whose
   files are not all stored: atomic from its point of view.

The runtime version is **not** in an `expo export` (verified: `metadata.json` has none). The
command takes it from `--runtime-version` (or per platform), else from the exported public app
config (`expoConfig.json`, `npx expo config --type public --json`) when its `runtimeVersion` is a
plain string; a policy object (`{"policy": "fingerprint"}`) must be resolved first with `npx
expo-updates runtimeversion:resolve --platform <p>`. Without one it fails before uploading.

## 6. Selection and rollout

For a request, the server validates the headers, then reads at most the **two newest rows** of the
branch (one indexed query joined to the application, which must exist and be active):

- no rows: `noUpdateAvailable`;
- newest row `R0` is a rollback: `rollBackToEmbedded` with `R0`'s time;
- `R0` is an update and the installation is in its rollout: `R0`'s manifest;
- otherwise the next row `R1` (always 100%, see below) as above, or `noUpdateAvailable`.

**Bucketing:** `bucket = uint64(first 8 bytes of SHA-256("<updateId>:<lower-case client UUID>")) mod
100`; in the rollout when `bucket < rolloutPercent`. Per update, so each rollout samples different
phones; stable for a phone, and monotone, so raising a percentage keeps every phone already in. A
request without a valid `EAS-Client-ID` is in only at 100%. **Only the newest row of a branch may be
partial**: a publish onto a partial rollout is refused, and only the newest update's percentage can
change. Lowering a percentage does not take an update off a phone that has it (the client never goes
back to an older `createdAt`); that is what rollback is for.

**Rollback** (both are new rows, both always 100%, both events and audit rows):

- *to the previous update* (`to: PREVIOUS`, or `republish` of any earlier update of the branch by
  id): a new update with a new UUID and `createdAt`, the source's files, `republished_from` set.
  `PREVIOUS` picks the newest update older than the branch's current one; when the branch's
  newest row is already a rollback to embedded, it is refused (`409 NOTHING_TO_ROLL_BACK`):
  republish by id instead.
- *to the embedded bundle* (`to: EMBEDDED`): a `ROLLBACK_TO_EMBEDDED` row; phones get the signed
  directive until something newer is published.

## 7. Signing and keys

The server signs **every** manifest and directive part (RSASSA-PKCS1-v1_5 with SHA-256, standard
base64), with the key whose id the client names in `expo-expect-signature` (`keyid`, default
`root` in the client when unnamed in its config) or, when the client asks for none, the default key.
A request naming an unknown key id is `400 UNKNOWN_KEY_ID`; one naming another algorithm is
`400 UNSUPPORTED_SIGNATURE_ALGORITHM`. Without any key configured the public routes answer
`503 APP_UPDATES_UNCONFIGURED` (fail closed, like the field plane).

**Where the private key lives.** `FC_APP_UPDATES_SIGNING_KEYS` is a comma-separated list of
`keyid=source`: `aws-sm://…` or `aws-ps://…` (a secret reference, resolved at use through the
platform's resolver `internal/secretref`, so it must be admitted by `FC_SECRET_REFERENCE_ALLOW`, and
cached as the resolver caches), `env:NAME` (an environment variable holding the PEM, as a task
definition injects a secret), or `file:/path` (development). `FC_APP_UPDATES_DEFAULT_KEY_ID` names
the default (else the first). Keys are RSA, at least 2048 bits, PKCS#1 or PKCS#8 PEM. The private
key is never stored in the database, logged or returned.

**Creating the key and the certificate:** `fcdev updates keygen --key-id main --common-name "acme
driver-app updates" --years 10` writes `private-key.pem` (mode 0600) and `certificate.pem`. The
certificate is what `@expo/code-signing-certificates` makes and both clients require: self-signed,
SHA-256 with RSA, key usage **digital signature** (critical), extended key usage **code signing**
(critical), no Expo project-information extension (so the client does not compare an EAS project id
or scope key), `notBefore` one day back for phones with slow clocks. The private key goes into the
secret store; the certificate into the app (`updates.codeSigningCertificate`,
`updates.codeSigningMetadata: {"keyid": "main", "alg": "rsa-v1_5-sha256"}`).

**Rotation** needs a new binary: the client trusts only its embedded certificate and requires the
signature's key id to equal its configured one. So: create a new key with a new id, add it to
`FC_APP_UPDATES_SIGNING_KEYS` beside the old one, ship a binary embedding the new certificate with
the new key id; old binaries keep asking for the old id and are signed with the old key until they
are gone, then remove it. A certificate must be replaced the same way before it expires (item 12 of
section 2). `fcdev` generates a development key and certificate on first start.

Signed parts are cached in memory (bounded) by update id and key id, so a burst of phones costs one
RSA signature per update, not one per request.

## 8. The public routes are hostile-facing

- **Validation before any work**: path segments by pattern (application code `^[a-z][a-z0-9_-]{0,49}$`,
  app key `^[a-z0-9][a-z0-9-]{0,39}$`, SHA-256 64 lower-case hex), headers by pattern (platform
  `android|ios`, runtime version `^[A-Za-z0-9][A-Za-z0-9._:+-]{0,127}$`, channel
  `^[a-z0-9][a-z0-9._-]{0,63}$`, protocol version `1`, `Accept` must allow `multipart/mixed`, else
  `406`). Anything malformed is `400 INVALID_REQUEST`; unknown paths under `/app-updates` are
  `404 NOT_FOUND`; only `GET`.
- **Constant shape**: every error is `{"error": CODE, "message": "…"}` with fixed messages. An
  unknown application, an inactive one, an unknown app key or channel all get the same signed
  `noUpdateAvailable` as a branch with no updates; an asset that is unknown, unpublished or another
  application's is the same `404`.
- **Bounded work**: one indexed query per manifest request (two rows), one per asset, signatures
  cached; asset bodies are streamed from the store; per-IP governors (the platform's
  `ratelimit.Governor`, like `/field/*`): `FC_APP_UPDATES_MANIFEST_RATE_PER_MIN` 300 / burst 100 and
  `FC_APP_UPDATES_ASSET_RATE_PER_MIN` 3000 / burst 1000 (a depot of phones shares one address).
- **Caching**: manifests `cache-control: private, max-age=0`; assets `public, max-age=31536000,
  immutable` with an `ETag` of the SHA-256.
- **A file is verified as it is served.** The asset route hashes the stored file while streaming
  it and holds back its last 32 KiB until the SHA-256 (and the size) are those of the address; a
  file that does not match is cut off short of its `Content-Length` (the handler aborts) and
  logged as an error. Because the response is immutable, a complete but wrong one would be kept:
  expo-updates downloads through an OkHttp cache (50 MiB), and on an Android emulator a phone
  that once received a corrupted bundle refused that update on every launch afterwards without
  asking the server again, even after the stored file was repaired. A broken download is retried
  instead.
- **No CORS**: the routes are for the native clients. (A browser build of an Expo app does not use
  `expo-updates`.)

**Streaming, not redirecting.** Both clients follow redirects (OkHttp's default client and the
default `URLSession` do), so a time-limited S3 address would work, but the platform streams: the
file store and S3 behave the same, the per-application check on every asset stays at the platform,
a phone needs to reach one host only (and an emulator talking to `fcdev` needs nothing else), and
integrity does not depend on the transport (the hash is in the signed manifest). A CDN in front of
`/app-updates/*/assets/*` caches for ever because the address is content-addressed.

## 9. Storage

The function-artifact store (`internal/platform/function/artifact`: content-addressed by SHA-256,
`sha256/ab/abcdef…`, atomic writes, an S3 and a file implementation behind one interface) is
reused, at its own location: `FC_APP_UPDATES_STORE` (`s3://bucket/prefix` deployed, `file:///path`,
empty for `/var/lib/flowcatalyst/app-updates`; `fcdev` uses its data directory). Files are capped at
`FC_APP_UPDATES_MAX_FILE_BYTES` (64 MiB); an update at 2000 assets; `expoClient` at 256 KiB.

The file registry is written by the upload handler directly, not through a use case: it is the
index of a content-addressed store (as the function-artifact upload writes to its store), carries no
business state, and an event per file would bury the publish event. Every publish, rollout change and
rollback goes through `usecaseop` with its event and audit row.

## 10. Permissions and roles

| Permission | Allows |
|---|---|
| `platform:app-update:update:view` | read updates, branches, live state |
| `platform:app-update:update:publish` | upload files, register updates |
| `platform:app-update:rollout:manage` | change a rollout, republish, roll back (either kind) |

Every route also requires **anchor scope and reach to the application** (`CanAccessApplication`):
an application's service account (anchor, confined to its application) acts on its own
application only; an operator (anchor, all applications) on any. A client-scoped principal is
refused whatever its permissions (an update reaches every client's phones).

Roles: the seeded `platform:application-service` role (every application's service account) gets all
three (migration 118 grants them to existing deployments); a new `platform:app-update-admin`
("App Update Administrator") holds all three for operators; `platform:viewer` gets view.
`platform:super-admin`'s wildcard covers them.

Events (`platform:app-update:update:*`, catalogued and audited): `published`, `rollout-changed`,
`republished`, `rolled-back-to-embedded`.

## 11. fcdev

`fcdev start` creates (once) a development key and certificate in its data directory
(`app-updates/signing-key.pem`, `app-updates/certificate.pem`, key id `main`), uses the file store
there, and serves the same routes. `fcdev updates certificate` prints the certificate for the app.

| Command | |
|---|---|
| `fcdev updates keygen` | create a signing key and its code-signing certificate |
| `fcdev updates certificate` | the local fcdev's development certificate |
| `fcdev updates publish <export-dir> --app <code> --app-key <key> --channel <c>` | upload and register (`--os android\|ios\|all`, `--runtime-version`, `--rollout`, `--message`, `--commit`, `--expo-config`, `--json`) |
| `fcdev updates list` / `branches` | read |
| `fcdev updates rollout <updateId> <percent>` | change a rollout |
| `fcdev updates republish <updateId>` | make an earlier update live again |
| `fcdev updates rollback --app … --app-key … --os … --runtime-version … --channel … [--to-embedded]` | roll back |

Connection and credentials are the `fcdev fn` ones (`--profile`, or the local fcdev's credential;
`--platform` is the platform URL there, so the phone platform is `--os`).
`docs/published/65-app-updates.md` is the operator's and developer's page.

## 12. Not in this slice

Console pages (the read API is here); adoption metrics (which update phones run; the request
headers `Expo-Current-Update-ID` and `Expo-Recent-Failed-Update-IDs` are received and not
recorded); bundle diffing (`A-IM: bsdiff`, never offered: the server always sends the whole file);
compressed asset responses; asset redirects to S3; `expo-manifest-filters` and server-defined
headers; manifest `metadata`; certificate chains served with the manifest
(`codeSigningIncludeManifestResponseCertificateChain`); SDK methods in the client libraries;
anything in an application's repository; retention and pruning of old updates and files; per-
application signing keys; a registry of phone apps.

## 13. For the owner

Choices made for now, each easy to change:

1. **Names.** Permissions `platform:app-update:update:view|publish` and
   `platform:app-update:rollout:manage`; role `platform:app-update-admin`; route prefix
   `/app-updates/` and `/api/app-updates/`; event codes `platform:app-update:update:*`.
2. **Who may act.** Anchor scope only: an application's own service account and platform
   operators. A client's administrators can neither see nor act on updates. If they should (for
   example to see which update is live), it is a new permission scoped by application.
3. **The application's service account may roll back** as well as publish (the same pipeline that
   publishes is the first to need a rollback). The alternative is publish only, with rollback
   reserved for operators.
4. **Limits.** 64 MiB a file, 2000 assets an update, 256 KiB of app config, rollouts 0 to 100 in whole
   percent; per-IP rates 300/min (manifest) and 3000/min (assets).
5. **Retention.** Nothing is pruned: every update and file is kept. A keep-N-per-branch rule
   (files referenced by no kept update deleted from the store) is the natural next step.
6. **One platform-wide keyring**, not a key per application: a leaked key affects every app on the
   platform, and a rotation means a new binary of every app that embeds that certificate.
   Per-application key ids are possible later without a protocol change.
7. **Deleting an application** leaves its update rows (no key, no cascade); they are never served
   again (the public routes resolve the application by its code to its current id). They go when
   retention arrives.
8. **The file registry write is outside `usecaseop`** (section 9), which adds a case to the closed
   list of `docs/conventions.md` section 3.
9. **Only the newest update of a branch may be partially rolled out** (EAS's rule as well).
10. **The channel header is required.** A request without `expo-channel-name` is `400`; there is no
    default channel. An app must set `updates.requestHeaders`.
11. **Migration numbers 117 and 118** are the next free ones on `main` today; a branch merged first that
    takes them means renumbering these two files before merging.
