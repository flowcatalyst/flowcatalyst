# Java SDK handover: application-managed invitations

Audience: the agent adding the "no invitation" user-creation surface to the
Java SDK at `clients/java-sdk`. The platform, the TypeScript SDK and the
Laravel SDK already ship this (2026-09-14). Your job is the Java mirror.

Read first: `clients/java-sdk/README.md` (build + codegen model) and
`docs/java-sdk-plan.md` (decisions). The Java SDK's models are **generated at
build time** from the vendored `clients/java-sdk/openapi/openapi.json`; only
the resource layer under `src/main/java/io/flowcatalyst/sdk/resources/` is
hand-written.

## What the platform now does

### 1. Create-user flags (the part the Java SDK exposes)

`POST /api/principals/users` (`createUser`) and `POST /api/principals`
(`createPrincipal`) accept two new optional booleans:

| Field | Default | Meaning |
|---|---|---|
| `sendInvitation` | `true` | `false` suppresses **all** platform email for the new user: neither the "set your password" invite nor the "account created" welcome is sent. The application takes over inviting the user. Ignored for service accounts and OIDC/federated users (they never get email anyway). |
| `returnInviteLink` | `false` | `true` mints the 72-hour "set your password" token and returns the link in the response as `inviteLink`. Only applies to a passwordless INTERNAL user; absent when a password was supplied or the user is OIDC/federated. |
| `inviteRedirectUri` | absent | Where the invitee goes after setting their password (and any 2FA enrolment), with their platform session already established. Rides on the invite token, so it applies to both the platform-sent email and `returnInviteLink`. Must match (the `/oauth/authorize` rule, wildcards included) a redirect URI of an active, non-portal `authorization_code` OAuth client serving an application the caller can access; otherwise the request fails `INVITE_REDIRECT_URI_INVALID` and no user is created. Added 2026-09-16. |

Precedence: `returnInviteLink:true` **always** suppresses the platform's own
invite email, even when `sendInvitation` is true or omitted. The token can
only be minted once per delivery (a second mint invalidates the first link),
so asking for the link back means "I am sending my own email with it".

Response changes:

- `PrincipalResponse` gained optional `inviteLink` (string). Populated only
  on a create-user response that asked for it. Absent on every other read.
- `createPrincipal` now returns `CreatePrincipalResponse { id, inviteLink? }`
  instead of the generic created response. Additive; `id` is unchanged.

The invite link is a live 72-hour bearer credential. Treat it like a
password: never log it, never put it in an audit trail, never return it to a
browser you do not trust.

Source of truth: `internal/platform/principal/api/dto.go` (`CreateUserRequest`,
`CreatePrincipalRequest`, `CreatePrincipalResponse`, `PrincipalResponse`) and
`api/openapi.lock.json`.

### 2. First-login password creation (platform side, no SDK work)

When an application creates a user with `sendInvitation:false`, sends its own
email linking to the application, and the application then redirects to
`/oauth/authorize` as usual, the hosted login page detects that the account
is an internal password account that has never had a password set. Instead of
a password field it shows "Create your password" and emails the user a
set-password link (proof of mailbox ownership; the login page never accepts a
first password inline). After the user sets the password, the platform signs
them in and follows the stored OAuth redirect back to the calling
application, so the application receives its authorization code as normal.
If the email domain requires 2FA, enrolment runs first and then the same
redirect is followed.

Endpoints involved (chi-mounted, not in the OpenAPI spec, no SDK surface):
`POST /auth/check-domain` (gained `passwordSetupRequired`),
`POST /auth/password-setup/request`, `POST /auth/password-reset/confirm`
(gained `sessionEstablished`). Nothing for the Java SDK to do here.

## Two integration patterns the SDK docs must describe

1. **Login-detected (recommended).** Create the user with
   `sendInvitation:false`. Send your own email that links to *your*
   application. The platform handles password creation at first login and
   returns the user to you. No link handling on your side.
2. **Embedded link.** Create the user with `returnInviteLink:true`, read
   `inviteLink` from the response, and embed it in your own email. The user
   sets their password on the platform and is signed in. Pass
   `inviteRedirectUri` (one of your login client's redirect URIs) to send
   them back to your application afterwards; without it they land on the
   platform's own landing page.

## Java SDK work

### Regenerate models

`make sdk-spec` has already refreshed `clients/java-sdk/openapi/openapi.json`
(verify with `grep -n sendInvitation clients/java-sdk/openapi/openapi.json`).
Run `make build-java-sdk` from the repo root. The generated
`io.flowcatalyst.sdk.generated.model.CreateUserRequest` and
`CreatePrincipalRequest` gain `sendInvitation` / `returnInviteLink`,
`PrincipalResponse` gains `inviteLink`, and a new `CreatePrincipalResponse`
model appears. Build failures in the hand-written layer are the wire-drift
signal the README describes; fix them.

### Resource layer (`resources/PrincipalsResource.java`)

- `createUser(CreateUserRequest)` already posts the generated model and
  returns `PrincipalResponse`, so it picks the fields up automatically. Add
  Javadoc on the method describing `sendInvitation`, `returnInviteLink`,
  `inviteRedirectUri`, the precedence rule, and the never-log warning on `inviteLink`. Mirror the
  wording in `clients/typescript-sdk/src/resources/principals.ts` (the
  `createUser` JSDoc) and
  `clients/laravel-sdk/src/Client/Resources/Principals.php`.
- If a `create(CreatePrincipalRequest)` method exists and is typed to the
  generic created response, retype it to the generated
  `CreatePrincipalResponse`. If none exists, do not add one.
- Do not add convenience overloads that hide the flags; the generated model's
  setters are the API, consistent with the rest of the resource layer.

### Tests

Add a `PrincipalsResourceTest` (or extend `ClientCoreTest`) using the
existing `StubServer` pattern (`src/test/java/io/flowcatalyst/sdk/StubServer.java`):

1. `createUser` with `sendInvitation=false` serialises
   `"sendInvitation":false` in the request body and omits
   `returnInviteLink`.
2. `createUser` with `returnInviteLink=true` deserialises an `inviteLink`
   from the stub response into `PrincipalResponse.getInviteLink()`.
3. `createUser` with neither flag omits both keys from the body (Jackson
   must not emit `null` for them; check the generated model's inclusion
   settings).

### Docs

Add a short "Creating users and invitations" section to
`clients/java-sdk/README.md` covering the two integration patterns above with
a Java snippet for each, plus the never-log warning.

### Verification

```
make build-java-sdk
```

must be green. Do not commit `target/`. Do not edit
`clients/java-sdk/openapi/openapi.json` by hand; it is refreshed only by
`make sdk-spec`.

## Reference implementations to mirror

- TypeScript: `clients/typescript-sdk/src/resources/principals.ts`
  (`createUser` JSDoc), generated types in
  `clients/typescript-sdk/src/generated/types.gen.ts`.
- Laravel: `clients/laravel-sdk/src/DTOs/Requests/CreateUserRequest.php`,
  `clients/laravel-sdk/src/DTOs/Principal.php` (`inviteLink`),
  `clients/laravel-sdk/tests/Unit/CreateUserRequestTest.php`.
