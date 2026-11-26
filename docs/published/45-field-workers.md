# Field Workers

*Drivers, pickers and gate staff sign in on a phone or handheld with a worker code and a PIN.*

Field workers are a separate identity population from platform users and portal users. A field
worker belongs to one client and is identified by a **worker code**, unique within that client.
On a device the worker enters the client's code, their worker code and their PIN; the PIN policy,
lock-outs, devices, badges, unlock keys and sessions are the platform's, so applications never see a PIN. The
console pages are Identity & Access → Field Workers, Field Devices and Field Policy; the admin API is
`/api/field-workers`, `/api/field-devices` and `/api/field-policies` (`docs/wire-contract.md`,
"Field identity").

## Worker codes

A worker code cannot be changed after the worker is created, so leave it to the platform unless the
client already has one (a payroll or badge number) that workers know:

- **Generated (the default).** Create the worker without a code and the platform draws one: three
  random letters, then digits, for example `KTM482`. The letters leave out I, O and L (they look like
  1 and 0) and every vowel (A, E, I, O, U, Y), so a code never spells a word; the digits are 2 to 9.
  The code says nothing about the site or the kind of worker, because workers move and change roles.
  The number of digits is the client's field policy setting **Digits in a generated worker code**
  (`workerCodeDigits`): 3 by default, up to 6 for a client with very many workers. Changing it applies
  to codes generated afterwards; existing codes never change.
- **Your own.** Give a code and it is used as typed (stored in upper case, at most 64 characters).
  Creating a worker with a code that already exists in the client only adds the applications to that
  worker.

Entry is not case-sensitive: `ktm482` and `KTM482` are the same worker. A code is not a secret (the
PIN is), so the console shows it on the worker's page and in the list.

| Digits | Codes per client |
|---|---|
| 3 | 3 502 592 |
| 4 | 28 020 736 |
| 5 | 224 165 888 |
| 6 | 1 793 327 104 |

## Creating workers from an application

`POST /api/field-workers` (the SDKs' `FieldWorkers().Ensure` / `fieldWorkers().ensure`) with the
client, the name and the applications the worker may sign in to. Omit `workerCode` and read the
generated code from the response's `workerCode`; give it to the worker with their first PIN (a PIN
reset returns a temporary one, once).

A creation without a code is safe to repeat only with an `idempotencyKey`: send your own record's id
(an employee number, a row id), and a repeat after a lost answer returns the worker the first request
made instead of making a second one. Without a key every request is a new worker, so the SDKs do not
resend it when its answer is lost; look the worker up before trying again.

## Signing in again with a fingerprint or face

On the worker's own phone (a **personal**, enrolled device) a worker can set up an **unlock key**: after
signing in there with their PIN, the app asks the phone to make a key that only a strong biometric (a
fingerprint or face the phone recognises) can use, and registers its public half with the platform. Next
time the worker's session has ended, the phone signs a fresh challenge from the platform with that key and
the worker is signed in again, without the PIN. The PIN is never stored, on the phone or anywhere else.

- **Off by default.** The client's field policy setting **Sign in again with a fingerprint or face**
  (`unlockKeysEnabled`) turns it on. Switching it off revokes every unlock key of the client.
- **The PIN still matters.** The phone asks for the worker's PIN whenever a key is revoked, unknown or invalidated
  (a new fingerprint enrolled, the phone reset), after a PIN change on another phone or a reset, and while a
  temporary PIN is waiting to be changed. By default there is no PIN on a timer: the key already needs the phone
  and a strong biometric, and Android itself asks for the phone's own lock code at least every 72 hours. A client that
  wants the worker's PIN regularly too sets **PIN re-entry on a personal device** (`pinReauthDays`, 1 to 90 days;
  0, the default, is never): after that many days the app asks for the PIN once and the key works again. It keeps
  workers practised at their PIN and limits how long a fingerprint that was already on the phone can stand in for
  the worker; it adds no factor the key does not already need.
- **Never on a shared device**, never from a phone that is not enrolled, and never for someone signed in as
  the worker (a supervisor or a console sign-in code). One phone holds one worker's key: a phone's
  biometrics cannot tell its people apart.
- **The policy is saved whole.** A save of the field policy (the console's, or `PUT /api/field-policies`) sets
  every setting: one that leaves out **Sign in again with a fingerprint or face** switches it off, which ends every
  unlock key and the sessions they started. A client that saved its policy before 2026-11-26 has **PIN re-entry**
  at 7 days until it changes it. The Go, TypeScript, Laravel and Java SDKs list a worker's keys
  (`ListUnlockKeys` / `listUnlockKeys`), revoke one (`RevokeUnlockKey` / `revokeUnlockKey`) and carry the
  device's `unlockKey`; each has a policy `Update` / `update` that reads the policy, changes what you name and
  writes the rest back, so a caller that does not know a setting does not switch it off.
- **Ended by** a PIN reset, a suspension, the worker locking themselves out, a PIN change on another phone,
  revoking the device, the policy, or an administrator (the worker's page and the device's page in the
  console, **Revoke unlock key**, which also ends any session the key started). A failed or replayed unlock counts towards the worker's lock-out as a
  wrong PIN does.
- **Sessions a key started.** Turning biometrics off on the phone, or setting up a new key there, leaves the
  worker signed in; the console shows each key's **Open sessions**, and **End sessions** on a revoked key that
  still has some signs them out. Switching unlock keys off, starting to require attestation and Google's list
  naming the phone's attestation key end every session the keys concerned started, whenever the keys were
  revoked.
- **When the platform cannot check.** With attestation required and Google's list unavailable to the platform
  for longer than `FC_FIELD_ATTESTATION_STATUS_MAX_AGE`, workers sign in with their PIN until it is back; no key
  is revoked for it.
- **What the console shows**: when the key was made and last used, when the PIN is next needed, and what the
  phone's attestation says (an Android phone can prove the key is in secure hardware and needs a biometric
  for every use; an iPhone cannot). Requiring that proof is the policy setting **Require a trusted hardware
  attestation** (off by default), which needs the platform's `FC_FIELD_ATTESTATION_ROOTS` and checks the chain's
  shape and dates and Google's list of revoked attestation keys (the platform fetches it in the background and needs
  outbound access to it); it does not check the phone's verified boot or which app made the key.

Sessions signed in this way say so in the token (`amr` `hwk`, `uv`). The protocol is in
`docs/field-unlock-keys-spec.md` and `docs/wire-contract.md`, "Unlock keys".

## Where the code lives

`internal/platform/fieldidentity` (the worker code generator is `workercode.go`, the creation is
`Ops.RunEnsureWorker`; unlock keys are `unlock_key*.go` and the `attestation` package), the admin API in
`internal/platform/fieldidentity/api`, the device protocol in `internal/platform/fieldauth`, and the console
pages in `internal/ui/pages/field_*`.
