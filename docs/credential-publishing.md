# Publishing service account and OAuth client credentials to Parameter Store

Status: owner asked 2026-11-25 ("an auto option to publish to a configured provider e.g. parameter store and
secret manager, vault etc. … a config which decides if it is enabled by default and maybe the path rules").
Owner rulings the same day:

- **Additional, not instead.** Every credential stays exactly where it is today: the webhook token and
  signing secret stay encrypted on the service account, and the OAuth client secret stays as its keyed hash.
  Delivery, signing, token checks and rotation work as before. Publishing *also* writes a copy to the store,
  so a receiving application (or a function or ingestion, through an `aws-ps://` reference) reads it from
  there and nobody has to copy it by hand.
- **Per owner, opt in.** Credentials handed to a client are not ours to store elsewhere, so publishing is a
  choice made on each service account and OAuth client. The option starts unticked.
- **Parameter Store, one parameter per credential**, is the shape. Secrets Manager and Vault come later
  behind the same seam (§9).
- **Available in np and prod.**
- **Deleting the owner deletes its published copies**; so does turning publishing off.
- **Covers OAuth clients as well as service accounts.**
- No new grace period: the service account token and signing secret change at once on regenerate, as today.
  The OAuth client secret already overlaps for 24 hours on rotation (`DefaultSecretGrace`), which is what
  lets readers pick up the new one.

Status of this document: build spec, for review before building.

## 1. What is published

| Owner | Credential | Parameter name (`{credential}`) | When it exists |
|---|---|---|---|
| Service account | OAuth client id | `client-id` | its linked OAuth client exists |
| Service account | OAuth client secret | `client-secret` | see §3: only when minted |
| Service account | webhook token | `auth-token` | `BEARER_TOKEN` or `API_KEY` with a token |
| Service account | signing secret | `signing-secret` | a signing secret is set |
| OAuth client (confidential, not linked to a service account) | client id | `client-id` | always |
| OAuth client (confidential, not linked to a service account) | client secret | `client-secret` | see §3 |

- Every parameter is a `SecureString`. The client id is not a secret, but one type for every parameter keeps
  readers simple.
- An OAuth client whose principal is a service account's principal (the client made with the account, or
  by an application's provision flow) is published **with its service account**, under the account's path
  and setting. Its own page shows "Published with service account <name>" and links there. Only
  stand-alone confidential clients have their own setting. Public clients have nothing secret and are not
  offered it.
- A credential that stops existing (for example a token removed by a webhook credential update) has
  its parameter deleted. Basic-auth user and password are not published: the platform does not store them
  (only the token and signing secret have columns, `Repository.encryptCreds`).

## 2. Configuration (deployment settings)

Deployment settings, not console settings, because the platform's IAM role has to match them.

| Variable | Meaning | Default |
|---|---|---|
| `FC_CREDENTIAL_PUBLISH` | The store: `aws-ps`. Empty or unset: publishing is not offered anywhere, and no control is shown. | unset |
| `FC_CREDENTIAL_PUBLISH_PREFIX` | Every published parameter must lie under it, for example `/flowcatalyst/np/`. Required when publishing is on; it must start and end with `/`. A path outside it is refused, whatever the template or a custom path says. | none |
| `FC_CREDENTIAL_PUBLISH_SERVICE_ACCOUNT_PATH` | Template of a service account's parameters | `{prefix}service-accounts/{account}/{credential}` |
| `FC_CREDENTIAL_PUBLISH_OAUTH_CLIENT_PATH` | Template of a stand-alone OAuth client's parameters | `{prefix}oauth-clients/{oauthClient}/{credential}` |
| `FC_CREDENTIAL_PUBLISH_DEFAULT` | Whether the option starts ticked on the create forms: `off` or `on` | `off` (opt in) |
| `FC_CREDENTIAL_PUBLISH_KMS_KEY` | KMS key id or alias for the `SecureString`s | empty (the account's `aws/ssm` key) |

Placeholders: `{prefix}`, `{account}` (service account code, with the `:` of an application account's `app:<code>` written `.`, as `app.orders`: Parameter Store names allow no `:`, and ordinary codes have no `.`, so the two cannot collide), `{app}` (the service account's application
code), `{oauthClient}` (the OAuth `client_id`) and `{credential}` (§1). Each must be a whole path segment, as in
`FC_SECRET_REFERENCE_ALLOW`. The template must end in `{credential}`. An invalid template is a startup error
when publishing is on, not a silent fallback. A template with `{app}` cannot be used for an account without an
application. Enabling one then needs a custom path (§4).

Region: the default chain's, as for reading references.

**Reading it back.** For a function or ingestion to use a published credential through the "Stored as"
selector, `FC_SECRET_REFERENCE_ALLOW` must admit it, for example `aws-ps:///flowcatalyst/np/service-accounts/`.
The two lists are separate on purpose: publishing a credential does not, by itself, let anyone point a
function at it.

## 3. When the platform can publish a credential

- The **webhook token and signing secret** are stored encrypted, so the platform can
  publish them at any time. That includes an existing account that has publishing switched on later.
- The **client id** is not secret and can be published at any time.
- The **OAuth client secret** is stored only as a keyed hash (`hashed:v1:`). Its plaintext exists only in the
  operation that mints it: create, or rotate. So it is published from that operation or never.
  - Publishing switched on for an existing owner publishes everything else. The client secret shows "Not
    published: rotate the client secret to publish it", with a **Rotate and publish** action. That action is
    the existing rotation, with its 24-hour overlap, so nothing using the old secret breaks.
  - The plaintext waiting to be published is kept encrypted with the platform key (as webhook credentials
    are) in the publication row (§5). It is cleared as soon as the parameter is written. It is never kept
    for a credential the platform can read back.

## 4. The choice, per owner

- **Created with it.** The create forms have a "Publish credentials to Parameter Store" checkbox. It starts as
  `FC_CREDENTIAL_PUBLISH_DEFAULT` says (unticked). These forms are:
  - the service account create dialog;
  - an application's "provision service account" flow;
  - the OAuth client create dialog, for confidential clients.

  Beneath the checkbox is the path the parameters will get, which updates as the code is typed.
- **Custom path (optional).** It replaces the part of the path before `{credential}`, and it must lie under
  `FC_CREDENTIAL_PUBLISH_PREFIX`. It is fixed once publishing is on. To move it, turn publishing off and on
  again; the client secret then needs a rotation, which the page says.
- **Switched on later**, from the owner's page: §3.
- **Switched off**: the published parameters are deleted (§6), after a confirmation that names them. The
  credentials on the platform are untouched.
- **API**: the create commands (`CreateCommand` for service accounts, `CreateOAuthClientCommand`, the
  application provision command) gain `publishCredentials *bool`. nil takes `FC_CREDENTIAL_PUBLISH_DEFAULT`.
  true when publishing is not configured is refused with 400 `CREDENTIAL_PUBLISHING_UNAVAILABLE`. Endpoints to
  switch it on or off and read the status follow the same operations (§7). They are in scope, but the console
  comes first.

## 5. Data

Migration `128_credential_publications.sql` (two small new tables, no change to existing ones):

```sql
CREATE TABLE iam_credential_publications (
    owner_type   TEXT NOT NULL,          -- SERVICE_ACCOUNT | OAUTH_CLIENT
    owner_id     TEXT NOT NULL,          -- sac_… / oac_… row id
    store        TEXT NOT NULL,          -- aws-ps
    base_path    TEXT NOT NULL,          -- the path before {credential}, ending in '/'
    enabled_at   TIMESTAMPTZ NOT NULL,
    enabled_by   TEXT,
    PRIMARY KEY (owner_type, owner_id)
);

CREATE TABLE iam_credential_publication_items (
    owner_type      TEXT NOT NULL,
    owner_id        TEXT NOT NULL,
    credential      TEXT NOT NULL,       -- client-id | client-secret | auth-token | signing-secret
    path            TEXT NOT NULL,       -- full parameter name
    state           TEXT NOT NULL,       -- PENDING_PUT | PUBLISHED | PENDING_DELETE | NOT_PUBLISHABLE
    pending_value   TEXT,                -- encrypted; only for a client secret awaiting its put
    attempts        INT  NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    last_error      TEXT,
    published_at    TIMESTAMPTZ,
    generation      BIGINT NOT NULL DEFAULT 0, -- bumped by every queueing; an outcome applies only to the generation it claimed
    updated_at      TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (owner_type, owner_id, credential)
);
CREATE INDEX iam_credential_publication_items_due
    ON iam_credential_publication_items (next_attempt_at)
    WHERE state IN ('PENDING_PUT', 'PENDING_DELETE');
```

- Neither table has a foreign key to its owner. A deleted owner's items must outlive it until their deletes
  have gone through, and then the worker removes the rows.
- `NOT_PUBLISHABLE` is the client secret of §3 that is not known. The row exists so the page can say so.
- **Same transaction.** Each operation that changes a published credential marks its item `PENDING_PUT` (or
  `PENDING_DELETE`) in the same transaction as the change. A credential is never changed without its publish
  being queued, and a publish is never queued for a change that rolled back. The aggregate carries the
  transient "to publish" set, and its repository's persist writes the items. That is the same pattern as
  `encryptCreds`, so no operation talks to AWS. For the client secret the set carries the plaintext, which
  the persist encrypts into `pending_value`. For everything else the worker reads the current value when it
  publishes, so two quick regenerates publish only the latest.

## 6. The publisher (background worker)

- Leader-gated (`newLeaderGate(ctx, cfg, "credential-publish")`), started only when `FC_CREDENTIAL_PUBLISH`
  is set. It claims due items with `FOR UPDATE SKIP LOCKED` and handles them one at a time. Parameter Store's
  write limit is low, and a `ThrottlingException` is retried like any other failure.
- **Put**:
  1. `PutParameter{Name, Value, Type: SecureString, KeyId, Overwrite: false, Tags: [flowcatalyst:managed=<owner_type>/<owner_id>]}`.
  2. On `ParameterAlreadyExists`, it reads the tags (`ListTagsForResource`). When the parameter is tagged as
     this owner's, it puts again with `Overwrite: true` (AWS refuses tags together with overwrite).
     Otherwise it fails with "a parameter FlowCatalyst does not manage already exists at …". **The platform
     never overwrites a parameter it did not create.**
  3. On success the item is `PUBLISHED`, `published_at` is set, and `pending_value` and `last_error` are
     cleared.
- **Delete**: it checks the tag the same way, then calls `DeleteParameter`. `ParameterNotFound` counts as
  done. A parameter not tagged as ours is left in place, the item is dropped, and a warning is logged. Once
  an owner that is switched off or deleted has no items left, its `iam_credential_publications` row goes.
- **Failure**: `attempts`++, `last_error` is the AWS error code and message (never the value), and
  `next_attempt_at` backs off from 30 s, doubling, capped at 1 h. It keeps retrying while the item is
  pending, and the page shows it (§7). A failed publish never blocks or undoes the create or rotate that
  queued it. AWS being down must not stop a rotation.
- Logs: each put or delete with owner, credential and path at info, failures at warn. Metrics: counters for
  puts, deletes and failures by error code, and a gauge of pending items.
- Undecryptable stored value (key rotated badly): the item fails with "the stored value cannot be decrypted".
  The platform does not publish an empty value.

## 7. Operations and events

New operations, in `internal/platform/credentialpublish/operations` (each is one domain operation with its
event, so it is audited):

| Operation | Event | Effect |
|---|---|---|
| `EnableCredentialPublishing{OwnerType, OwnerID, BasePath?}` | `CredentialPublishingEnabled` (path) | the row, plus items as §3 |
| `DisableCredentialPublishing{OwnerType, OwnerID}` | `CredentialPublishingDisabled` (paths) | every item `PENDING_DELETE` |
| `RetryCredentialPublishing{OwnerType, OwnerID}` | none | due items' `next_attempt_at` = now |

Existing operations queue items when the owner publishes:

| Operation | Items |
|---|---|
| `CreateServiceAccountWithCredentials`, application `ProvisionServiceAccount` (`publishCredentials`) | enable, then all of §1 with the client secret's plaintext |
| `RegenerateAuthToken`, `RegenerateSigningSecret` | `auth-token` / `signing-secret` |
| webhook credential update (`credential_input.go`) | changed credentials `PENDING_PUT`, removed ones `PENDING_DELETE` |
| `CreateOAuthClient` (confidential, `publishCredentials`) | enable, `client-id`, `client-secret` with plaintext |
| `RotateOAuthClientSecret` | `client-secret` with plaintext, under the linked service account's row when it has one |
| `DeleteServiceAccount`, `DeleteOAuthClient` | every item `PENDING_DELETE` (the linked OAuth client's are the account's) |
| deactivate (either) | nothing: the copy stays, as the credential does |

The publish outcome is item state, not an event. An event per put would be noise, and the enable and
disable events already record who chose what.

## 8. Console

Shown only when publishing is configured.

- **Create dialogs** (service account, application provision, confidential OAuth client): the checkbox,
  the live path preview, and "Custom path" behind a disclosure.
- **One-time reveal** after create, regenerate or rotate: under each value, "Also published to
  `/flowcatalyst/np/service-accounts/orders/signing-secret`" when it was queued.
- **Service account page**, credentials section, a "Parameter Store" panel:
  - **Off**: "Not published", with **Publish credentials** (a dialog with the path and custom path).
  - **On**: one row per credential, showing:
    - the parameter name and its `aws-ps://` reference, each with copy, for pasting into a function's
      "Stored as" selector;
    - the state: "Published 12:01", "Publishing…", "Failing: AccessDeniedException …, retrying at
      12:31", or for the client secret "Not published: rotate the client secret to publish it"
      [**Rotate and publish**].

    The panel has **Retry now** when anything is failing, and **Stop publishing**, which asks for
    confirmation: "Deletes these N parameters from Parameter Store. The credentials in FlowCatalyst are
    not changed."
- **OAuth client page**: the same panel for a stand-alone confidential client; for a linked one, "Published
  with service account <name>".

## 9. Seam for other stores

`credentialpublish.Store` is an interface with `Put(ctx, path, value, ownerTag) error`,
`Delete(ctx, path, ownerTag) error` and `Reference(path) string`. Only `aws-ps` is built. The store choice
lives in the publication row, so a later Secrets Manager or Vault store does not move existing rows.
Vault needs its own auth config and client; that is a separate piece of work when asked for.

## 10. IAM (task role), per environment

```json
{
  "Effect": "Allow",
  "Action": ["ssm:PutParameter", "ssm:DeleteParameter", "ssm:AddTagsToResource", "ssm:ListTagsForResource"],
  "Resource": "arn:aws:ssm:<region>:<account>:parameter/flowcatalyst/<env>/*"
}
```

With a customer managed key in `FC_CREDENTIAL_PUBLISH_KMS_KEY`, the role also needs `kms:Encrypt` on that
key. Reading back through references uses the existing `ssm:GetParameter` and `kms:Decrypt` grants of
docs/external-secret-references.md. The receiving application's role needs `ssm:GetParameter` on its own
account's path only, for example `parameter/flowcatalyst/np/service-accounts/orders/*`.

## 11. Tests

- **Unit**:
  - template parsing and validation, including the whole-segment placeholder rule, the `{app}`-without-
    application case, and a custom path outside the prefix;
  - the store against the fake (`secretreftest` gains `PutParameter`, `DeleteParameter`, `AddTagsToResource`,
    `ListTagsForResource`), covering create, overwrite of our own, refusal to overwrite a foreign one,
    delete of a foreign one left alone, and not-found on delete.
- **Integration** (`-tags=integration`):
  - each operation in §7 queues the right items in its transaction, and a rolled-back operation queues
    none;
  - the client secret's `pending_value` is encrypted and cleared after the put;
  - switching publishing on later marks the client secret `NOT_PUBLISHABLE`, and rotation publishes it;
  - deleting the owner leaves its items, and the worker deletes the parameters, then the rows;
  - backoff and `last_error`.
- **Console**: the pages render each state; the checkbox follows `FC_CREDENTIAL_PUBLISH_DEFAULT`; nothing is
  shown when publishing is off. Browser check of create, reveal, the panel, Stop publishing, and Rotate and
  publish, on a scratch fcdev with the fake store.

## 12. Build order

1. Config, `Store` (aws-ps) and the fake, the migration, the worker.
2. Service accounts: the operations of §7, the create checkbox, the reveal note, the panel.
3. OAuth clients: create, rotate, delete, and the linked-client rule.
4. The application provision flow, the API flag and endpoints, and docs/published.

Not in this build: a Secrets Manager or Vault store; a grace period for the service account token or signing
secret; Java and Rust mirrors.
