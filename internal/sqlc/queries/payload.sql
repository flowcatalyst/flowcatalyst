-- Queries for oauth_oidc_payloads — the OIDC artifact store
-- (access/refresh tokens, authorization codes, PKCE sessions, etc.).
-- Webauthn ceremonies use this same table; their queries live in
-- webauthn_ceremony.sql so each subsystem owns its own slice.

-- name: OAuthPayloadInsert :exec
INSERT INTO oauth_oidc_payloads
    (id, type, payload, grant_id, user_code, uid, expires_at, consumed_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: OAuthPayloadFindByID :one
SELECT id, type, payload, grant_id, user_code, uid, expires_at, consumed_at, created_at
FROM oauth_oidc_payloads
WHERE id = $1 AND type = $2;

-- name: OAuthPayloadMarkConsumed :exec
UPDATE oauth_oidc_payloads SET consumed_at = NOW() WHERE id = $1;

-- name: OAuthPayloadDelete :exec
DELETE FROM oauth_oidc_payloads WHERE id = $1;

-- name: OAuthPayloadDeleteByGrant :exec
DELETE FROM oauth_oidc_payloads WHERE grant_id = $1;

-- name: OAuthPayloadPurgeExpired :execrows
DELETE FROM oauth_oidc_payloads
WHERE expires_at IS NOT NULL AND expires_at < NOW();
