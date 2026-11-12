# Rotating FLOWCATALYST_APP_KEY

`FLOWCATALYST_APP_KEY` (base64, 32 bytes) is the platform's own secret. This is the
procedure for replacing it, and what each use of it does during the change.

## What uses the key

| Use | Where | During a rotation |
|---|---|---|
| Encrypted columns (AES-256-GCM): SECRET platform-config values, service-account webhook token + signing secret, IdP and client-auth OIDC client secrets, TOTP secrets, function SECRET/DB settings, legacy encrypted OAuth-client and developer secrets | `internal/platform/shared/encryption` | New writes use the current key. Reads try the current key, then every previous one. `fc-server reencrypt` moves stored values to the current key. |
| Verify-only secrets (`hashed:v1:` HMAC): OAuth client secrets, service-account client secrets, developer credentials | `encryption.Service.Hash` / `VerifySecret` | Verified under the current key or any previous one. A secret that verifies only under a previous key is re-hashed under the current key on that request. They cannot be re-hashed in bulk: the platform never holds the plaintext. |
| Dispatch-job auth tokens (HKDF info `fc-dispatch-auth`) | `internal/server/subsystems.go`, `scheduler.DispatchAuthService` | The scheduler signs with the current key. `/api/dispatch/process` and `/api/dispatch/settled` accept a token signed under the current key or any previous one. Nothing is stored: a job on the queue carries its token until it is delivered, or until stale recovery puts it back to PENDING (15 minutes) and it is signed again. |
| Field-worker PIN pepper (HKDF info `fc-field-pin`) | `internal/platform/fieldidentity/pin` | New hashes use the current pepper and store its id in `field_identities.pin_pepper_id`. Verification picks the pepper by that id, or tries the current pepper and then every previous one for hashes stored before ids existed. The first successful sign-in after a rotation re-hashes the PIN under the current pepper. |

These do not derive from the app key and are not covered here: the JWT signing key
(`FC_JWT_SIGNING_KEY_PATH`, with `FLOWCATALYST_JWT_PREVIOUS_PUBLIC_KEY` for its own
rotation), `FC_PUSH_STREAM_SECRET`, the database credentials, and secrets held in a
secret manager (`aws-sm://`, `aws-ps://`, …).

`FLOWCATALYST_APP_KEY_PREVIOUS` is a comma-separated list, so several keys can be
retired at once (`new-old,older`). A single value is a list of one. A malformed
entry, like a malformed current key, stops every tier at boot.

## Procedure

Generate the new key with `openssl rand -base64 32`.

### 1. Make every process accept the new key

Every process with `FLOWCATALYST_APP_KEY` in its environment needs the change: every
fc-server tier (API, scheduler, stream, outbox, push) and any one-off task.

During a rolling deploy, processes with the old key and processes with the new key
run at the same time. A process still on the old key cannot read a value written with
the new key, and an API tier on the old key rejects a dispatch token signed by a
scheduler already on the new key. To avoid any errors, deploy twice:

1. `FLOWCATALYST_APP_KEY=<old>`, `FLOWCATALYST_APP_KEY_PREVIOUS=<new>`. Nothing is
   written with the new key yet, but every process can now read it.
2. `FLOWCATALYST_APP_KEY=<new>`, `FLOWCATALYST_APP_KEY_PREVIOUS=<old>`.

If a few minutes of errors during the rollout are acceptable, step 2 alone is enough.

Watch `fc_dispatch_job_auth_rejected_total` while the deploy runs. It should stay at
zero. If it climbs, two tiers disagree about the key: a rejected token gets a 401, the
router does not ACK it, and the job comes back only through stale recovery, again and
again.

### 2. Re-encrypt the stored values

Once every process runs with the new key as current, run this with the same
environment as the API tier. It is safe while the platform is serving traffic:

```
fc-server reencrypt --dry-run   # counts only
fc-server reencrypt
```

How it works:

- It reads each encrypted column in primary-key order, in batches (`--batch-size`,
  default 500).
- It rewrites each value that is not yet on the current key in its own statement. The
  update is a compare-and-set on the exact ciphertext it read, so it never overwrites
  a value written in the meantime. Those rows show up in `CHANGED_MEANWHILE`.
- Values already on the current key are skipped, so if it stops, run it again.

Exit status:

- `0`: done.
- `1`: error. Run it again to resume.
- `3`: done, but some values could not be opened with any configured key. The log
  lists their rows, never their values. These were already unreadable before the
  rotation. Fix or re-enter them.

`HASHED` counts the verify-only secrets, and `EXTERNAL` counts secret-manager
references. Neither is touched. `NOT_ENCRYPTED` counts legacy plaintext in columns
that encrypt lazily on the next write.

A dry run after the real run should show `WOULD_REENCRYPT` 0 for every target.

### 3. Let the lazy migrations catch up

- **PINs** are re-hashed when each worker next signs in. Until then they keep working,
  as long as the old key stays in `FLOWCATALYST_APP_KEY_PREVIOUS`. To see progress:
  `SELECT pin_pepper_id, count(*) FROM field_identities WHERE pin_hash IS NOT NULL AND pin_scheme = 'PEPPER_V1' GROUP BY 1`.
  The id that PINs set after the rotation carry is the current one. A NULL id is a
  hash stored before pepper ids existed. It may be under either key.
- **Verify-only client secrets** are re-hashed the next time each client
  authenticates. There is no way to tell which key an existing hash was made under,
  so a client that does not authenticate while the old key is still listed will stop
  working when the old key is dropped. Its secret then has to be rotated.
  Before dropping the key, list the secrets not re-hashed since the rotation went out
  (a re-hash rewrites the row, so its update time moves), with `$1` the time the new
  key was deployed:

  ```sql
  SELECT 'oauth client' AS kind, id, client_id AS name, updated_at AS last_change
    FROM oauth_clients
   WHERE client_secret_ref LIKE 'hashed:v1:%' AND updated_at < $1
  UNION ALL
  SELECT 'developer credential', id, name, dev_client_secret_updated_at
    FROM iam_principals
   WHERE dev_client_secret_ref LIKE 'hashed:v1:%' AND dev_client_secret_updated_at < $1;
  ```

  Each row is a client that has not authenticated since the rotation: ask its owner
  to use it, or plan to reissue its secret once the key is dropped. The list can miss
  a client whose row changed for another reason after the rotation (an edit in the
  console), so treat it as the likely casualties, not a proof that no other will fail.
  A client's previous secret during a secret rotation (`previous_secret_ref`) follows
  the same rule and expires with its grace period.

### 4. Drop the old key

Remove the old key from `FLOWCATALYST_APP_KEY_PREVIOUS` and deploy, once all of these
hold:

- A `reencrypt --dry-run` reports `WOULD_REENCRYPT` 0 and `UNREADABLE` 0.
- `fc_dispatch_job_auth_previous_key_total` has stayed flat for longer than a job can
  sit on the queue. That is at least the 15-minute stale-recovery window, plus any
  subscription `delaySeconds`.
- You accept what happens to anything not yet migrated:
  - A worker who has not signed in since the rotation cannot sign in. The sign-in
    is refused with 403 `PIN_RESET_REQUIRED` ("this PIN was set under a retired key
    and must be reset by a supervisor") and does not count as a wrong PIN. The same
    answer comes from changing a PIN and from acting as a worker. A supervisor
    resetting the PIN fixes it.
  - A client whose verify-only secret was not used since the rotation needs a new
    secret.

Keep a copy of the old key until you are sure. Putting it back in
`FLOWCATALYST_APP_KEY_PREVIOUS` makes everything above readable again.
