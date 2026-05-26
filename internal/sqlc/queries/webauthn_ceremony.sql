-- Queries for WebAuthn ceremony state in oauth_oidc_payloads.
-- The id column carries a type prefix ("WebauthnRegistration:{stateID}")
-- to keep ids unique across the shared store; the type column carries
-- the same discriminant. Mirrors the Rust impl.

-- name: WebauthnCeremonyUpsert :exec
INSERT INTO oauth_oidc_payloads (id, type, payload, expires_at, created_at)
VALUES ($1, $2, $3, $4, NOW())
ON CONFLICT (id) DO UPDATE SET
    payload = EXCLUDED.payload,
    expires_at = EXCLUDED.expires_at;

-- name: WebauthnCeremonyConsume :one
DELETE FROM oauth_oidc_payloads
WHERE id = $1 AND (expires_at IS NULL OR expires_at > NOW())
RETURNING payload;

-- name: WebauthnCeremonyPurgeExpired :execrows
DELETE FROM oauth_oidc_payloads
WHERE type IN ($1, $2)
  AND expires_at IS NOT NULL
  AND expires_at <= NOW();
