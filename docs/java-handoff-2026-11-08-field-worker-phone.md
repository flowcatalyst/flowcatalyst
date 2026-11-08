# Java hand-off: field-worker contact phone

For the Java platform port. Small delta: an optional contact phone number on a field worker, beside the
optional `email`. Go packages: `internal/platform/fieldidentity` (`entity.go`, `repository.go`,
`operations.go` with `validPhone`), `internal/platform/fieldidentity/api/api.go`. Wire reference:
`docs/wire-contract.md` ("Field identity: the admin API").

## 1. Schema (goose migration 103; take the next free number at merge)

`ALTER TABLE field_identities ADD COLUMN IF NOT EXISTS phone VARCHAR(32);` Re-runnable, nullable,
metadata-only; `field_identities` is small. Empty is stored as NULL (as `email`).

## 2. Validation

One rule for both operations, error `400` with code `PHONE_INVALID` ("phone is not a valid phone number"):
after trimming, empty is allowed; otherwise only digits, spaces, `-`, `.`, `(`, `)` and a single leading
`+`; 7 to 15 digits (E.164 maximum); at most 32 characters in total. The value is stored trimmed, exactly
as entered: no reformatting, there is no country to normalise against.

## 3. Semantics

- Ensure (`POST /api/field-workers`): `phone` applies to a NEW worker only; an existing worker keeps every
  field, as with `email`. It is validated even when the worker exists.
- Update (`PUT /api/field-workers/{id}`): `phone` omitted leaves it unchanged; `""` clears it.

## 4. Wire

`phone` (optional string) on the ensure body, the update body and the worker response (omitted when empty).
The console edit/create forms and the worker detail show it next to the email. The linked platform user's
email and `PrincipalByEmail` are untouched.

## 5. Not in events

Phone is contact PII and is not in any event data (email is not either); `field-worker` events are
unchanged.
