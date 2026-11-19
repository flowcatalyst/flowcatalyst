# Field Workers

*Drivers, pickers and gate staff sign in on a phone or handheld with a worker code and a PIN.*

Field workers are a separate identity population from platform users and portal users. A field
worker belongs to one client and is identified by a **worker code**, unique within that client.
On a device the worker enters the client's code, their worker code and their PIN; the PIN policy,
lock-outs, devices, badges and sessions are the platform's, so applications never see a PIN. The
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

## Where the code lives

`internal/platform/fieldidentity` (the worker code generator is `workercode.go`, the creation is
`Ops.RunEnsureWorker`), the admin API in `internal/platform/fieldidentity/api`, the device protocol in
`internal/platform/fieldauth`, and the console pages in `internal/ui/pages/field_*`.
