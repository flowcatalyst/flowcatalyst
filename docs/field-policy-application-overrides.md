# Field policy: per-application overrides (proposal)

Status: **design only, nothing built** (2026-11-26), for the owner to read before any code. The owner: "this is a
client policy not an app client policy. I think an app should be able to override parts of the client wide policy."
The client-wide policy and its PUT stay as released in 0.31.0 (an application-confined caller is refused
`403 APPLICATION_FORBIDDEN`).

## 0. Decided by the owner (2026-11-26)

| # | Decision | His words |
|---|---|---|
| D1 | An application's override may loosen as well as tighten: for an overridable member the client-wide value is a default, not a ceiling | "yes" |
| D2 | The client's policy can forbid overrides of a member, so a client that wants one rule everywhere has it; only a platform administrator sets this lock | "yes that makes good sense" |
| D3 | An application shows three states per member: on (or its value), off, and follow the client's rule | "I think 3" |

## 1. The model

The effective policy of a session is the platform default, overlaid by the client's policy (`field_policies`, one
row per client), overlaid by the override of the session's application for that client (new, sparse: only the
members it sets), except for a member the client has locked, where the override does not apply. Every path that knows
the session's application uses the effective policy for that application; the paths that do not (the worker's PIN,
the failure counters, device enrolment) keep the client's.

What the code makes per worker, device and application today:

| Thing | Belongs to | Where |
|---|---|---|
| PIN, failure counter, soft and hard lock | the worker (client-wide) | `field_identities`, `AuthTx.RecordFailure` |
| A device and its enrolment | the client; enrolled for a set of applications | `field_devices`, `field_device_apps`; `EnrolDevice` reads `requireAttestation` |
| A session (login family) | the worker, one application, one device or unenrolled key | `field_sessions.application_id` |
| An unlock key | the worker on one device, for every application | `field_unlock_keys` (no application column) |
| A photo | the worker, shown in every application | `field_worker_photos` |
| A generated worker code | the client | `EnsureWorker` reads `workerCodeDigits` |

## 2. Which members can be overridden

Every overridable member may be overridden in both directions (D1), within the ranges the client-wide policy allows.

| Member | Override? | Reason (from the code) |
|---|---|---|
| `unlockKeysEnabled` | **yes** | the unlock names its `appCode` and gives a session of that application; registration and `unlockKeys` in the token answer belong to a session, which has an application. The case that prompted this: client off (the default), one application on |
| `requireEnrolledDevice` | yes | checked at login, refresh and act-as, each of which knows the application |
| `badgeOnlyLogin` | yes | a badge login names its application; on removes the PIN for that application's sign-ins (only ever on an enrolled device) |
| `sharedIdleSecs`, `personalIdleSecs`, `shiftMaxSecs`, `unenrolledIdleSecs` | yes | `SessionLimits` applies to a session, which is one application's |
| `photosEnabled` | yes, as "photos in this application" | `worker.photoVersion` in the token answer and the device photo routes are per session; the photo itself stays the worker's (an application with photos on can set one that applications with photos off do not show) |
| `impersonationMaxSecs` | no | acting as another worker is a client-level control, and D1 would let an application lengthen it |
| `workerMaySetOwnPhoto` | no | the photo a worker sets from one application is the one every application shows |
| `pinMinLength` | no | the PIN is the worker's in every application |
| `softLockThreshold`, `softLockSecs`, `hardLockThreshold` | no | the counters are the worker's; per-application thresholds would let the loosest application decide the worker's lock |
| `pinReauthDays` | no | counted from the key's `pin_verified_at`, which is per device, not per application |
| `unlockKeyAttestation` | no | a key serves every application on the device and is verified once, at registration, against one rule |
| `requireAttestation` | no | read when a device is enrolled, and a device serves several applications |
| `workerCodeDigits` | no | a worker code is the client's |

## 3. Locks (D2)

The client's policy gains `lockedMembers`: the overridable members applications may not override. Only a platform
administrator (anchor scope, `platform:iam:field-device:manage`) sets it, through its own route (section 5), never
through the client-wide PUT: that PUT is a full replacement written by client administrators and integrations, and
a body without the list must not clear it.

When a member becomes locked, an existing override of it is **kept and stops applying**: the reads show it as
overruled, and it applies again if the lock is lifted. It is not deleted because the application chose it and the
lock may be temporary; an application cannot set or change an override of a locked member (`409
POLICY_MEMBER_LOCKED`), but may clear it. Locking `unlockKeysEnabled` takes effect like the override changes of
section 4 (sessions of applications that are now off end; keys are revoked only when no application has it on).

## 4. Unlock keys under overrides (the hard part)

**Today** a key registered in application A's session signs the worker in to application B on the same device: the
unlock takes `appCode` and checks only that the worker and the device are granted B (`fieldauth/unlock.go`); the key
has no application, and the challenge (`{unlockKeyId}`) names none.

**Rule:** a key stays the worker's on the device, for every application whose effective `unlockKeysEnabled` is on
and which the worker and the device are granted.

| Event | What happens |
|---|---|
| Registration | allowed when the session's application has it on (rule 2.1 of the unlock-keys spec reads the effective value) |
| Challenge | gains an optional `appCode`; when given and that application has it off, `403 UNLOCK_KEYS_DISABLED` before any biometric prompt |
| Unlock | `403 UNLOCK_KEYS_DISABLED` when `appCode`'s application has it off |
| `unlockKeys.allowed` in a token answer, and refresh of a key-signed session | the session's application's effective value (today: the client's) |
| An application goes from on to off (its override, the client's default under "follow", or a lock) | its key-signed sessions end (`application_id` = it and `unlock_key_id` set); keys are **kept** while any application of the client still has it on |
| No application of the client has it on any more | every key of the client is revoked `POLICY_DISABLED` and its sessions end, as switching off does today |

Under D1 the client-wide switch is the default for applications that follow it, so switching it off revokes keys
only when no application overrides it on. **This is the one change to 0.31.0's behaviour**, and only for clients
that have an "on" override. Every change here takes the client's policy lock, then the workers' rows, in the order of
`fieldidentity/lock_order.go`.

## 5. Who sets what, and the routes

| Who | What | Gate |
|---|---|---|
| The application, through its service account | its own application's override | application confinement (`CanAccessApplication`) and the client scope, with the permission applications already hold for field work (`platform:iam:field-worker:manage`, or `field-device:manage`); no new grant |
| A client or platform administrator (console or API) | any application's override for a client they manage | `field-device:manage` and the client scope |
| A platform administrator | the client's `lockedMembers` | anchor scope and `field-device:manage` |

| Route | Body / answer |
|---|---|
| `GET /api/field-policies/{clientId}/applications/{appCode}` | the three states for every overridable member (below) and the effective policy |
| `PATCH /api/field-policies/{clientId}/applications/{appCode}` | per member: a value sets the override, `null` clears it ("follow the client"), absent leaves it |
| `DELETE /api/field-policies/{clientId}/applications/{appCode}` | clears every member |
| `GET /api/field-policies/{clientId}/applications` | every application of the client with an override |
| `PUT /api/field-policies/{clientId}/locks` | `{lockedMembers: [...]}`, platform administrators only |

The read, per overridable member (D3):

```
FieldPolicyMember = {effective: value, source: "DEFAULT" | "CLIENT" | "APPLICATION",
                     locked: boolean, override: value | null, overruled: boolean}
```

`override` is the application's own value or `null` (follow); `overruled` is true when it has one and the member
is locked. An application shows on, off and "follow the client (now on/off)" from these. A member that cannot be
overridden is refused `422 POLICY_INVALID` naming it.

**Console:** the client's Field Policy page gains "Application overrides": one row per application with an override,
the members it sets with their values (an overruled one marked so), Edit (a drawer with the overridable members,
each "Follow the client" or a value) and Clear (with a confirmation). Beside each overridable member of the
client-wide form, a platform administrator sees "Applications may override" (the lock), and everyone sees how many
applications override it.

## 6. Schema, events, compatibility

```sql
ALTER TABLE field_policies ADD COLUMN locked_members TEXT[] NOT NULL DEFAULT '{}';

CREATE TABLE field_policy_overrides (
    client_id                VARCHAR(17) NOT NULL,
    application_id           VARCHAR(17) NOT NULL,
    unlock_keys_enabled      BOOLEAN,      -- NULL: follow the client
    require_enrolled_device  BOOLEAN,
    badge_only_login         BOOLEAN,
    shared_idle_secs         INTEGER,
    personal_idle_secs       INTEGER,
    shift_max_secs           INTEGER,
    unenrolled_idle_secs     INTEGER,
    photos_enabled           BOOLEAN,
    updated_at               TIMESTAMPTZ NOT NULL,
    updated_by               VARCHAR(100),
    PRIMARY KEY (client_id, application_id)
);
```

No foreign keys (`docs/foreign-keys.md`): deleting an application or a client deletes its overrides in its own
transaction. `PolicyRepo.Effective(client, application)` reads both rows in one query; `Effective(client)` stays for
the places with no application.

- Events: `platform:iam:field-policy:application-overridden {clientId, applicationId, appCode, set, cleared}` and
  `platform:iam:field-policy:locks-changed {clientId, lockedMembers}`, audited as the caller (an application's service
  account or a person).
- Compatibility: the client-wide `GET` gains `lockedMembers` (read-only, additive); the client-wide `PUT` is
  unchanged and still writes the client's values only. An older application that reads or writes the client-wide
  policy keeps working; where an override exists, that application's sessions follow it. The token answer's shape
  does not change; its values become per application.

## 7. Where the application must be taken into account

Today every caller reads `Policies.Effective(clientID)` (about 30 call sites). Those that know the session's
application change to the effective value for it:

| Path | File | Members read |
|---|---|---|
| PIN or badge login | `fieldauth/session.go` (login) | `requireEnrolledDevice`, `badgeOnlyLogin`, session limits; thresholds stay the client's |
| Refresh | `fieldauth/session.go` (refresh) | `requireEnrolledDevice`, session limits, `unlockKeysEnabled` for a key-signed session |
| Act-as, console sign-in code | `fieldauth/impersonate.go`, `fieldidentity/impersonation_ops.go` | `requireEnrolledDevice`, session limits |
| Unlock challenge and unlock | `fieldauth/unlock.go` | `unlockKeysEnabled` (the challenge only with the new optional `appCode`) |
| Key registration | `fieldidentity/unlock_key_register.go` | `unlockKeysEnabled` of the registering session's application |
| Token answer (`unlockKeys`, `worker.photoVersion`) | `fieldauth/endpoints.go`, `fieldauth/photo.go` | `unlockKeysEnabled`, `photosEnabled` |
| Device photo routes | `fieldauth/photo.go` | `photosEnabled` |
| Client-wide PUT, override PATCH and DELETE, locks PUT | `fieldidentity/operations.go` and new operations | the revocations of section 4 |

Unchanged (client-wide): setting and checking the PIN, `RecordFailure`, device enrolment, worker creation, the admin
photo routes, the attestation rule and the status list.

## 8. Cost

About 1 800 lines with tests: a migration, the repository and `Effective(client, application)`, five routes and their
OpenAPI, about ten call sites, the revocation rule of section 4, locks, the console section, two events, docs (wire
contract, published page, this file, section 7 of the unlock-keys spec). The OpenAPI changes are additive and the
token answer's shape is unchanged; the one behaviour change is section 4's for clients with an "on" override. It can
be a **minor release**.

## 9. Questions for the owner

1. **Who may set an override?** Recommendation: the application's own service account for itself (no new grant),
   and client and platform administrators for any application of the client.
2. **May a client administrator lock members too, or only a platform administrator?** Recommendation: only a
   platform administrator, as decided, until a client asks.
3. **Should the challenge's `appCode` be required** (so a library never prompts for a biometric an application then
   refuses)? Recommendation: optional now, required in the next major version of the device protocol.
