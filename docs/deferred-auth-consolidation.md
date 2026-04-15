# Auth Layer: Two-Level Authorization (Intentional Design)

## Not Duplication — Separate Concerns

### Layer 1: Handler — Role/Permission Authorization
"Does this user have the role/permission to call this endpoint?"

### Layer 2: UseCase::authorize() — Resource-Level Authorization
"Does this user have access to this specific resource?"

Empty `authorize()` stubs force every operation to consider resource-level auth
even when there's nothing to check yet. Do not remove them.
