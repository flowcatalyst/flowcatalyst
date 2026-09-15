# Java hand-off: explicit mapping scope on identity-provider create/update

Owner ruling (2026-09-15): when an identity provider is created or updated
with email domains, the scope of any NEW email-domain mapping must be an
explicit choice, ANCHOR or a specific CLIENT. It must never fall through to
ANCHOR because no client was given. Go implements it in the commit that adds
this file; Java must mirror the wire contract so the parity corpus agrees.

## Wire contract

`POST /api/identity-providers` and `PUT /api/identity-providers/{id}` gain

```
mappingScope?: "ANCHOR" | "CLIENT"
```

Validation (400, `usecase.Validation` codes):

| Code | When |
|---|---|
| `MAPPING_SCOPE_REQUIRED` | the request would create a mapping for a domain that has none yet and `mappingScope` is absent; also `primaryClientId` set without `mappingScope` |
| `INVALID_MAPPING_SCOPE` | any value other than ANCHOR or CLIENT (PARTNER is managed on the email-domain page) |
| `PRIMARY_CLIENT_REQUIRED` | `mappingScope: CLIENT` without a non-blank `primaryClientId` |
| `PRIMARY_CLIENT_NOT_ALLOWED` | `mappingScope: ANCHOR` with a `primaryClientId` |

Behaviour:

- New mapping: created with the chosen scope; `primaryClientId` set when CLIENT.
- Existing mapping routed elsewhere (claim): scope untouched; with CLIENT the
  client is linked only when the mapping has none; then the usual move.
- Existing mapping already routed to this provider: scope untouched; with
  CLIENT the client is linked when the mapping has none (this is the "edit
  later" fix: a provider created with ANCHOR domains can have a client linked
  afterwards from the provider's edit form). Persisted with an
  `email-domain-mapping:updated` event.
- An existing primary client is never overwritten.
- The required-scope check runs before any row is written, so a failing
  request leaves nothing behind.
- Removal-only updates (`allowedEmailDomains` shrinks) need no scope.
- Operation results gain `domainsLinked: string[]` next to
  `domainsCreated` / `domainsClaimed` / `domainsReleased`. The HTTP handlers
  return the reloaded `IdentityProviderResponse`, so this is not on the wire.

Source of truth: `internal/platform/identityprovider/operations/create.go`
(`validateMappingScope`, `requireScopeForNewDomains`, `mapDomainTx`),
`update.go`, `api/dto.go`, and `api/openapi.lock.json`.

## Parity corpus

`parity/scenarios/identity-providers/identity-providers.json` creates a
provider with `allowedEmailDomains` and no client. Add
`"mappingScope": "ANCHOR"` to that create body (and to any other scenario
that creates a provider with a fresh domain), or pin the new
`MAPPING_SCOPE_REQUIRED` outcome. Two Go test fixtures needed the same
addition (`portalauth/portal_flow_pg_test.go`,
`portalidentity/api/ensure_sso_pg_test.go`).

## SPA

Both identity-provider drawers now show a "Domain scope" choice (Anchor or
Client, nothing preselected) whenever at least one domain is listed, with
the Primary Client autocomplete shown and required only for Client. On
edit it is required only when a domain new to the provider is added;
otherwise it is optional and links the client to unlinked domains. The
four error codes render inline under the scope field. The frontend trees
are shared, so take the two drawers and `api/identity-providers.ts` verbatim.
