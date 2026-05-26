-- Queries for webauthn_credentials.
-- Column is passkey_data (JSONB), not "credential" — the entity field
-- name and the column name differ.

-- name: WebauthnCredentialFindByID :one
SELECT id, principal_id, credential_id, passkey_data, name, created_at, last_used_at
FROM webauthn_credentials
WHERE id = $1;

-- name: WebauthnCredentialFindByCredentialID :one
SELECT id, principal_id, credential_id, passkey_data, name, created_at, last_used_at
FROM webauthn_credentials
WHERE credential_id = $1;

-- name: WebauthnCredentialFindByPrincipal :many
SELECT id, principal_id, credential_id, passkey_data, name, created_at, last_used_at
FROM webauthn_credentials
WHERE principal_id = $1
ORDER BY created_at;

-- name: WebauthnCredentialUpsert :exec
INSERT INTO webauthn_credentials
    (id, principal_id, credential_id, passkey_data, name, created_at, last_used_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE SET
    passkey_data = EXCLUDED.passkey_data,
    name = EXCLUDED.name,
    last_used_at = EXCLUDED.last_used_at;

-- name: WebauthnCredentialDelete :exec
DELETE FROM webauthn_credentials WHERE id = $1;
