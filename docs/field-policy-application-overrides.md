# Field policy: per-application overrides

Status: **built** (2026-11-26; migration 130). The owner: "this is a client policy not an app client policy. I think
an app should be able to override parts of the client wide policy." The client-wide policy and its PUT are as
released in 0.31.0 (an application-confined caller is refused `403 APPLICATION_FORBIDDEN`); an application changes
its own override instead.

## 0. Decided by the owner (2026-11-26)

| # | Decision | His words, or the answer he accepted |
|---|---|---|
| D1 | An application's override may loosen as well as tighten: for an overridable member the client-wide value is a default, not a ceiling | "yes" |
| D2 | The client's policy can forbid overrides of a member, so a client that wants one rule everywhere has it; only a platform administrator sets this lock | "yes that makes good sense" |
| D3 | An application shows three states per member: its own value, the other value, and follow the client's rule | "I think 3" |
| D4 | Who sets an override: the application's own service account for its own application, with no new grant; client and platform administrators for any application of the client | "okay go ahead please" (to the recommendation) |
| D5 | Who locks a member: platform administrators only | as D4 |
| D6 | The unlock challenge's `appCode` is optional now, required at the next major version of the device protocol | as D4 |
| R1 | **Who may loosen which member** (the reviewer's rule, adopted as the default, awaiting the owner's confirmation; D1 stands as the model): an application's own account may only tighten `requireEnrolledDevice`, `badgeOnlyLogin` and `photosEnabled` for itself; loosening them for an application is for a client or platform administrator (section 5) | not yet confirmed |
| D7 | Unlock keys stay one per worker per device enrolment, not per application: the key is scoped to the worker and the device as a passkey is scoped to its identity provider, and each application decides whether it accepts it; separate native apps already have separate device enrolments and so separate keys | as D4 |

## 1. The model

The effective policy of a session is the platform default, overlaid by the client's policy (`field_policies`),
overlaid by the override of the session's application for that client (`field_policy_overrides`, sparse), except
for a member the client has locked (`field_policy_locks`), where the override is kept and does not apply.

**When an override applies at all.** Only when the application is **active**, **enabled for the client**
(`app_client_configs.enabled`, the platform's existing notion, `application/client_config.go`), and, for a session,
the **worker holds the application**. Otherwise the client-wide policy decides. A path decides this **before any
credential is checked**, so an application's override never changes what can be done to, or learnt about, a worker
who does not hold the application: no new counted failure, no new oracle, relative to 0.31.0.
`Ops.EffectivePolicy(client, application, worker)` computes it (a nil worker is the provisional policy of a path that
has not identified the worker yet and decides again once it has); `PolicyRepo.Effective(client)` stays for the paths
with no application.

| Thing | Belongs to | Where |
|---|---|---|
| PIN, failure counter, soft and hard lock | the worker (client-wide) | `field_identities`, `AuthTx.RecordFailure` |
| A device and its enrolment | the client; enrolled for a set of applications | `field_devices`, `field_device_apps` |
| A session (login family) | the worker, one application, one device or unenrolled key | `field_sessions.application_id` |
| An unlock key | the worker on one device enrolment, for every application that accepts it (D7) | `field_unlock_keys` (no application column) |
| A photo | the worker, shown in every application with photos on | `field_worker_photos` |
| A generated worker code | the client | `EnsureWorker` |

## 2. Which members can be overridden

Every overridable member may be overridden in both directions (D1), within the client-wide policy's ranges; who may
set which direction is R1 (section 5).

| Member | Override | Reason (from the code) |
|---|---|---|
| `unlockKeysEnabled` | **yes** | the unlock names its `appCode` and gives a session of that application; registration and `unlockKeys` in the token answer belong to a session, which has an application |
| `requireEnrolledDevice` | yes | checked at login, refresh, act-as, sign-in code and the device photo routes, each of which knows the application |
| `badgeOnlyLogin` | yes | a badge login names its application (only ever on an enrolled device) |
| `sharedIdleSecs`, `personalIdleSecs` (60 to 86400), `shiftMaxSecs` (3600 to 86400), `unenrolledIdleSecs` (60 to 86400) | yes | `SessionLimits` applies to a session, which is one application's |
| `photosEnabled` | yes, as "photos in this application" | `worker.photoVersion` in the token answer and the device photo routes are per session; the photo is the worker's |
| `impersonationMaxSecs` | no | acting as another worker is a client-level control, and D1 would let an application lengthen it |
| `workerMaySetOwnPhoto` | no | the photo a worker sets from one application is the one every application shows |
| `pinMinLength` | no | the PIN is the worker's in every application |
| `softLockThreshold`, `softLockSecs`, `hardLockThreshold` | no | the counters are the worker's |
| `pinReauthDays` | no | counted from the key's `pin_verified_at`, which is per device |
| `unlockKeyAttestation` | no | a key serves every application on the device and is verified once, at registration |
| `requireAttestation` | no | read when a device is enrolled, and a device serves several applications |
| `workerCodeDigits` | no | a worker code is the client's |

`field_policy_overrides` has a column for each overridable member and nothing else, with the ranges as CHECKs, so a
member that may not be overridden can never be stored or applied, whatever a caller sends: the API's body schema
refuses unknown members (`additionalProperties: false`, 422), and the use case refuses a member outside the set
(`422 POLICY_INVALID` naming it).

## 3. Locks (D2, D5)

`field_policy_locks` names the overridable members a client does not let its applications override. Only a platform
administrator (the permission `platform:iam:field-policy:lock`, section 5) sets them, with `PUT
/api/field-policies/{clientId}/locks`, never through the client-wide PUT (a full replacement written by client
administrators and integrations, which must not clear them). The client-wide GET shows them as `lockedMembers`.

An override of a member that becomes locked is **kept and stops applying**; the reads show it as `overruled`, and
unlocking makes it apply again. An application cannot set a locked member (`409 POLICY_MEMBER_LOCKED`, `details.field`
names it) and may clear it. A lock change is settled as any other change (section 4): locking `unlockKeysEnabled`
over an application's "on" override, when the client has them off, revokes the keys if no other application has them
on; unlocking it later does not bring revoked keys back (a worker registers a new one).

## 4. Unlock keys under overrides

Before this, a key registered in application A's session already signed the worker in to application B on the same
device: the unlock takes `appCode` and checks the worker's and the device's grants; the key has no application. That
stays (D7). The rule now:

| Event | What happens |
|---|---|
| Registration | allowed when the registering session's application has unlock keys on (section 2.1 of the unlock-keys spec reads its effective value) |
| Challenge with `appCode` | `403 UNLOCK_KEYS_DISABLED` when that application has them off, before any biometric prompt |
| Challenge without `appCode` (D6) | refused `403 UNLOCK_KEYS_DISABLED` only when no application of the client has them on; it applies no application's own rule |
| Unlock (names `appCode`) | `403 UNLOCK_KEYS_DISABLED` when that application has them off for the worker (not counted as a failure), decided again under the worker's lock, so an override change that lands between the first read and the session cannot leave a key-signed session of an application that is now off |
| `unlockKeys` in a token answer; refresh of a key-signed session | the session's application's effective value |
| An application goes from on to off (its override, the client's switch under "follow", or a lock or unlock) | the live sessions keys of the client signed in to it end (`UNLOCK_KEY_REVOKED`); the keys are kept while any application of the client has them on |
| No application of the client has them on any more | every live key of the client is revoked `POLICY_DISABLED` and every session a key of the client signed in ends, as switching off did in 0.31.0 |

**The one change to 0.31.0:** switching unlock keys off client-wide revoked every key. Now:

| Client-wide off, and | Keys | Key-signed sessions |
|---|---|---|
| no application overrides it on | every key revoked `POLICY_DISABLED` (as 0.31.0) | all end (as 0.31.0) |
| application A overrides it on | kept | A's go on; every other application's end |

Every change that moves where unlock keys are on takes the client's policy lock, then the rows of every worker with
a key of the client, in the order of `internal/platform/fieldidentity/lock_order.go`. Only overrides of applications
active and enabled for the client count: an override of an inactive, deleted or disabled application keeps no key
alive (its key-signed sessions stop refreshing at once; the keys are revoked at the next change of where unlock keys
are on).

## 5. Who sets what, and the routes

| Who | What | Gate |
|---|---|---|
| The application, through its service account (D4) | its own application's override, **only for clients the application is active and enabled for** (an application's service account is anchor-scoped, so the client scope alone would reach every client), within R1 | `platform:iam:field-worker:manage` or `field-device:manage`, and application confinement (`403 APPLICATION_FORBIDDEN` for another application or a client it does not serve) |
| A client administrator | any application's override, for their client, either direction | the same permissions, client-scoped |
| A platform administrator (D5) | any override, and the locks | the locks need **`platform:iam:field-policy:lock`**, held only by `platform:admin` (and `platform:super-admin` through its wildcard; migration 130 grants it to existing deployments); scope is reach, not authority, so anchor scope with field permissions is not enough (`403 PERMISSION_REQUIRED`) |

**R1, what an application's own account may set** (an application-confined caller; `403 POLICY_LOOSENING_FORBIDDEN`
naming the member otherwise, "a client or platform administrator can loosen it"):

| Member | The application itself | A client or platform administrator |
|---|---|---|
| `requireEnrolledDevice` | `true`; clear only when that does not loosen what is in force | `true`, `false` or clear |
| `badgeOnlyLogin` | `false`; clear only when that does not loosen what is in force | either, or clear |
| `photosEnabled` | `false`; clear only when that does not loosen what is in force | either, or clear |
| `unlockKeysEnabled` | either, or clear | either, or clear |
| `sharedIdleSecs`, `personalIdleSecs`, `shiftMaxSecs`, `unenrolledIdleSecs` | any value in range, or clear | the same |

**Clearing** one of those three, by `clear` or by the DELETE, is refused for the application when its stored override
is stricter than the client's value (clearing would loosen what is in force: an administrator's tightening the
application cannot undo); it is allowed when it does not loosen (the override equals the client's value, or the
client's lock already overrules it). It is judged at the time of the call, against the client's policy as it is then. The override does not record who
set a member, so this holds for a tightening the application set itself too: once in force, an administrator lifts
it.
A call that would loosen any of them is refused **whole**, naming every such member (`details.fields`), so nothing
half-happens: a DELETE by the application either clears everything or nothing.

The reviewer's reasons: `requireEnrolledDevice` and `badgeOnlyLogin` gate the credential check itself, and what follows
from it (the PIN, the failure counter, lockout, the hard lock's revocation of keys) is the worker's across applications;
`photosEnabled` loosened re-shows photos the client chose to hide.

An override PATCH of an application that is not active and enabled for the client is `409 APPLICATION_NOT_ENABLED`
for an administrator.

Reads need `field-worker:view|manage` or `field-device:view|manage`, with the same confinement: an application sees
only its own override, a client administrator only their client's.

| Route | Answer |
|---|---|
| `GET /api/field-policies/{clientId}/applications` | `{applications: [{applicationId, appCode, overrides, overruled, updatedAt, updatedBy?}]}`, only the caller's applications |
| `GET /api/field-policies/{clientId}/applications/{appCode}` | `FieldPolicyApplication` |
| `PATCH /api/field-policies/{clientId}/applications/{appCode}` | body `{set?: FieldPolicyOverrideValues, clear?: [member]}`; answers `FieldPolicyApplication` |
| `DELETE /api/field-policies/{clientId}/applications/{appCode}` | clears every member; answers `FieldPolicyApplication` |
| `PUT /api/field-policies/{clientId}/locks` | body `{lockedMembers: [member]}`; answers the client's `FieldPolicy` |

```
FieldPolicyApplication = {clientId, applicationId, appCode, members: FieldPolicyMembers,
                          effective: FieldPolicy, updatedAt?, updatedBy?}
FieldPolicyMembers = {unlockKeysEnabled, requireEnrolledDevice, badgeOnlyLogin, photosEnabled: FieldPolicyBoolMember,
                      sharedIdleSecs, personalIdleSecs, shiftMaxSecs, unenrolledIdleSecs: FieldPolicyIntMember}
FieldPolicyBoolMember = {effective: boolean, source: "DEFAULT" | "CLIENT" | "APPLICATION",
                         locked: boolean, override: boolean | null, overruled: boolean}
FieldPolicyIntMember  = {effective: integer, source, locked, override: integer | null, overruled}
FieldPolicyOverrideValues = {unlockKeysEnabled?, requireEnrolledDevice?, badgeOnlyLogin?, photosEnabled?: boolean,
                             sharedIdleSecs?, personalIdleSecs?, shiftMaxSecs?, unenrolledIdleSecs?: integer}
```

`source` is `DEFAULT` when the client has no policy of its own, `CLIENT` when the client's value applies (also for a
locked member with an override), `APPLICATION` when the application's override applies. `override` is the
application's own value, `null` when it follows the client; `overruled` is true when it has one and the member is
locked. A settings page with three states per member reads `override` (null: "Follow the client (now {effective})";
a value: that value) and `locked` (shown and not editable when true), and writes `set` for a value and `clear` for
"follow". A member in both `set` and `clear` is `422`.

**Console:** the Field Policy page has "Application overrides": each application's override with its members (an
overruled one marked "locked by the client, not applied") and Clear override behind a confirmation; a form that sets
one member of an application's override to on, off, a number of seconds, or follow the client; and, for a platform
administrator (the lock permission), "Locked for applications" (a checkbox per overridable member, Save locks); a lock
that would leave no application with unlock keys on is not saved at once: the form comes back as submitted with a
warning that saving revokes every unlock key of the client, and "Lock and revoke keys" confirms. Others see which
members are locked. An override of an application that is not active and enabled for the client is marked "not
applied".

## 6. Events, audit, compatibility

- `platform:iam:field-policy:application-overridden {clientId, applicationId, appCode, overrides, set, cleared}`
  (`overrides`: the members it sets after the change) and `platform:iam:field-policy:locks-changed {clientId,
  lockedMembers}`, both with the caller as the event's principal (an application's service account or a person) and
  audited as the use cases `PatchFieldPolicyOverride` and `SetFieldPolicyLocks`. Sessions they end emit
  `field-session:revoked`; keys they revoke, `field-unlock-key:revoked`.
- The client-wide `GET` gains `lockedMembers` (read-only); its `PUT` is unchanged and leaves overrides and locks
  alone. An older application that reads or writes the client-wide policy keeps working; where an override exists,
  that application's sessions follow it. The token answer's shape is unchanged; its values are per application. The
  device protocol gains the challenge's optional `appCode` (D6).
- No foreign keys (`docs/foreign-keys.md`). An application **disabled for a client**, or its configuration row for the
  client **deleted**, takes its override for that client with it (`application.ClientConfigRepo.Persist` and `Delete`,
  in the calling use case's transaction, under the client's field-policy lock, so an override change racing them runs
  wholly before, and its row goes, or after, and is refused); an application **deleted** takes all its overrides
  (`application.Repository.Delete`, in `DeleteApplication`'s transaction). Neither revokes keys
  at once: an override of an application that does not serve the client counts nowhere (section 4).
- **A badge alone never resets the PIN's failure counter.** A sign-in that did not verify the PIN (badge-only, whether
  the client allows it or an application's override does) records the sign-in and leaves the counter and the soft lock
  as they were, so the holder of a copied badge cannot guess the PIN between resets. In 0.31.0 a client-wide
  badge-only sign-in reset them; this is the same rule for both, and a change to 0.31.0. Only a PIN sign-in resets
  them.
- **An unlock no longer resets the PIN's failure counter either.** An unlock proves the key and a biometric, not the
  PIN, and the unlock-keys spec accepts that a biometric already enrolled on a personal phone can unlock: whoever holds
  the phone with such a finger could otherwise alternate an unlock with PIN guesses and stay clear of the hard lock
  indefinitely, and the PIN they learn works everywhere. An unlock stamps the sign-in and leaves the counter and the
  soft lock as they were; everything else about it is unchanged.

**Behaviour changes to 0.31.0, for the release notes:**

1. Switching unlock keys off client-wide no longer revokes every key while an application's override keeps them on
   (section 4).
2. A badge-only sign-in no longer resets the worker's PIN failure counter or soft lock.
3. An unlock no longer resets them either.
4. A custom role an operator wrote with a wildcard that matches `platform:iam:field-policy:lock` (for example
   `platform:iam:*:*`) now carries the new lock permission, and with it the power to lock members of every client it
   reaches.

**Known limits:** through an application's override that loosens the enrolled-device rule, five wrong PINs (the soft
lock) still tell the caller whether the named worker holds the application: a worker who does not is answered as an
unknown worker and never soft-locks. It costs the guesser a lockout of a real worker, as telling known from unknown
workers does in 0.31.0.

## 7. Where the application is taken into account

| Path | File | Members read from the effective policy |
|---|---|---|
| PIN or badge login | `fieldauth/session.go` | `requireEnrolledDevice`, `badgeOnlyLogin`, session limits. Provisional before the worker is known (the application active and enabled); decided for the worker once they are locked, before the PIN. When only an override of an application the worker does not hold let an unenrolled key through, the request is answered as an unknown worker is (`401 INVALID_CREDENTIALS`, recorded `OVERRIDE_NOT_APPLICABLE`, nothing counted against the worker); a badge alone for such a worker is `400 PIN_REQUIRED`, the client's rule |
| Refresh | `fieldauth/session.go` | `requireEnrolledDevice`, session limits, `unlockKeysEnabled` of a key-signed session |
| Act-as | `fieldauth/impersonate.go` | `requireEnrolledDevice`, session limits |
| Console sign-in code | `fieldidentity/impersonation_ops.go` | the same |
| Unlock challenge, unlock | `fieldauth/unlock.go` | `unlockKeysEnabled` (section 4) |
| Key registration | `fieldauth/unlock.go`, `fieldidentity/unlock_key_register.go` | `unlockKeysEnabled` of the registering session's application |
| Token answer | `fieldauth/endpoints.go`, `photo.go` | `unlockKeys`, `worker.photoVersion`, `worker.canSetOwnPhoto` |
| Device photo routes | `fieldauth/photo.go` | `photosEnabled`, `requireEnrolledDevice` |
| Client-wide PUT, override PATCH and DELETE, locks PUT | `fieldidentity/operations.go`, `policy_override_ops.go`, `policy_locks_ops.go`, `policy_override_settle.go` | the revocations of section 4 |

Client-wide, unchanged: the PIN and its checks, `RecordFailure`, device enrolment, worker creation, the admin photo
routes, the attestation rule and the status list, and the console's and admin API's key views (`pinReauthDays`).

## 8. Size and release

About 2 000 lines with tests: migration 130, the repository, `EffectiveFor`, five routes, nine call sites, the
settling rule, the console section, two events, docs. The OpenAPI changes are additive, the token answer's shape is
unchanged and the challenge's `appCode` is optional; the one behaviour change is section 4's, for clients with an
"on" override. A **minor release** (0.32.0).

## 9. Tests

`fieldidentity/policy_override_test.go` (the overlay, the three states, where keys stay on, the ranges),
`fieldidentity/api/policy_overrides_pg_test.go` (three states, limits and members, locks both ways, tenancy),
`fieldauth/policy_overrides_pg_test.go` (the 0.31.0 change from both sides, one application off, an application on
for itself, locks over `unlockKeysEnabled` both ways, the sign-in rules and limits per application),
`fieldauth/lock_order_pg_test.go` (an override change and a lock change take the workers first),
`fieldauth/policy_override_scope_pg_test.go` (the review's reproduction: an override never reaches a worker outside the
application, an application that does not serve the client, badge-only and the failure counter, an override change
racing an unlock and a registration, an inactive application keeps no key alive),
`fieldidentity/api/policy_override_account_pg_test.go` (the application's account in its real shape, R1, the lock
permission, overrides going with their application), `seed/roles_test.go` (who holds the lock),
`migrate/field_policy_overrides_pg_test.go` (the columns and CHECKs), `ui/pages/field_policy_overrides_test.go`.
