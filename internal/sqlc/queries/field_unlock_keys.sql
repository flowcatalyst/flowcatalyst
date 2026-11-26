-- Queries for field_unlock_keys and field_unlock_challenges
-- (docs/field-unlock-keys-spec.md section 5). The rest of the field plane
-- predates sqlc and keeps its hand-written SQL in its repository files.

-- name: FieldUnlockKeyInsert :exec
INSERT INTO field_unlock_keys
    (id, client_id, identity_id, device_id, jkt, public_key_jwk, attestation_chain, attestation,
     attestation_verified, pin_verified_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: FieldUnlockKeyFindByID :one
SELECT * FROM field_unlock_keys WHERE id = $1;

-- name: FieldUnlockKeyLockByID :one
SELECT * FROM field_unlock_keys WHERE id = $1 FOR UPDATE;

-- name: FieldUnlockKeyExistsByJKT :one
SELECT EXISTS (SELECT 1 FROM field_unlock_keys WHERE jkt = $1);

-- name: FieldUnlockKeyLiveByDevice :one
SELECT * FROM field_unlock_keys WHERE device_id = $1 AND revoked_at IS NULL;

-- name: FieldUnlockKeyLiveByDevices :many
SELECT * FROM field_unlock_keys
WHERE device_id = ANY(@device_ids::text[]) AND revoked_at IS NULL;

-- name: FieldUnlockKeyListByIdentity :many
SELECT * FROM field_unlock_keys WHERE identity_id = $1 ORDER BY created_at DESC, id DESC;

-- name: FieldUnlockKeyRevokeByID :many
UPDATE field_unlock_keys SET revoked_at = @at, revoked_reason = @reason, revoked_by = @revoked_by
WHERE id = @id AND revoked_at IS NULL
RETURNING id;

-- name: FieldUnlockKeyRevokeByIdentity :many
UPDATE field_unlock_keys SET revoked_at = @at, revoked_reason = @reason, revoked_by = @revoked_by
WHERE identity_id = @identity_id AND revoked_at IS NULL
RETURNING id;

-- name: FieldUnlockKeyRevokeByIdentityExceptDevice :many
UPDATE field_unlock_keys SET revoked_at = @at, revoked_reason = @reason, revoked_by = @revoked_by
WHERE identity_id = @identity_id AND device_id <> @device_id AND revoked_at IS NULL
RETURNING id;

-- name: FieldUnlockKeyRevokeByDevice :many
UPDATE field_unlock_keys SET revoked_at = @at, revoked_reason = @reason, revoked_by = @revoked_by
WHERE device_id = @device_id AND revoked_at IS NULL
RETURNING id;

-- name: FieldUnlockKeyRevokeByClient :many
UPDATE field_unlock_keys SET revoked_at = @at, revoked_reason = @reason, revoked_by = @revoked_by
WHERE client_id = @client_id AND revoked_at IS NULL
RETURNING id;

-- name: FieldUnlockKeyRevokeUnverifiedByClient :many
UPDATE field_unlock_keys SET revoked_at = @at, revoked_reason = @reason, revoked_by = @revoked_by
WHERE client_id = @client_id AND revoked_at IS NULL AND NOT attestation_verified
RETURNING id;

-- name: FieldUnlockKeyTouchPinVerified :execrows
UPDATE field_unlock_keys SET pin_verified_at = @at
WHERE device_id = @device_id AND identity_id = @identity_id AND revoked_at IS NULL;

-- name: FieldUnlockKeyTouchUsed :exec
UPDATE field_unlock_keys SET last_used_at = @at WHERE id = @id;

-- name: FieldUnlockKeyDeleteByIdentity :exec
DELETE FROM field_unlock_keys WHERE identity_id = $1;

-- name: FieldUnlockKeyDeleteByDevice :exec
DELETE FROM field_unlock_keys WHERE device_id = $1;

-- name: FieldUnlockKeyPurgeRevoked :execrows
DELETE FROM field_unlock_keys WHERE revoked_at < @before;

-- name: FieldUnlockChallengeInsert :exec
INSERT INTO field_unlock_challenges (challenge_hash, unlock_key_id, device_id, expires_at)
VALUES ($1, $2, $3, $4);

-- name: FieldUnlockChallengeConsume :one
DELETE FROM field_unlock_challenges WHERE challenge_hash = $1
RETURNING unlock_key_id, device_id, expires_at;

-- name: FieldUnlockChallengeDeleteByIdentity :exec
DELETE FROM field_unlock_challenges
WHERE unlock_key_id IN (SELECT k.id FROM field_unlock_keys k WHERE k.identity_id = $1);

-- name: FieldUnlockChallengeDeleteByDevice :exec
DELETE FROM field_unlock_challenges WHERE device_id = $1;

-- name: FieldUnlockChallengePurgeExpired :execrows
DELETE FROM field_unlock_challenges WHERE expires_at < @before;

-- name: FieldUnlockKeyDeviceLabels :many
SELECT id, label FROM field_devices WHERE id = ANY(@device_ids::text[]);

-- name: FieldPolicyLock :exec
-- Serialises a policy change with unlock-key registrations of the client, so a
-- registration never commits against a policy that changed under it.
SELECT pg_advisory_xact_lock(hashtext('field-policy:' || @client_id::text));

-- name: FieldSessionEndForUnlockKeys :execrows
UPDATE field_sessions SET revoked_at = @at, revoked_reason = 'UNLOCK_KEY_REVOKED'
WHERE unlock_key_id = ANY(@unlock_key_ids::text[]) AND revoked_at IS NULL;

-- name: FieldWorkersLock :many
-- Row-locks the workers in id order: the first row lock of every transaction
-- that changes a worker's unlock keys or sessions (lock_order.go).
SELECT id FROM field_identities WHERE id = ANY(@ids::text[]) ORDER BY id FOR UPDATE;

-- name: FieldUnlockKeyWorkersOfClient :many
SELECT DISTINCT identity_id FROM field_unlock_keys WHERE client_id = $1 ORDER BY identity_id;

-- name: FieldWorkersOnDevice :many
-- The workers a device's revocation or deletion touches: those with an unlock
-- key on it (live or not) and those with a live session on it.
SELECT k.identity_id FROM field_unlock_keys k WHERE k.device_id = $1
UNION
SELECT s.identity_id FROM field_sessions s WHERE s.device_id = $1 AND s.revoked_at IS NULL
ORDER BY 1;

-- name: FieldWorkersActedAsBy :many
-- The workers whose sessions a worker holds open as an actor.
SELECT DISTINCT identity_id FROM field_sessions
WHERE actor_id = $1 AND actor_type = 'FIELD_WORKER' AND revoked_at IS NULL ORDER BY identity_id;

-- name: FieldSessionEndForClientUnlockKeys :execrows
-- Ends every live session signed in by an unlock key of the client, live or
-- already revoked (unverified_only: only keys registered without a verified
-- attestation).
UPDATE field_sessions SET revoked_at = @at, revoked_reason = 'UNLOCK_KEY_REVOKED'
WHERE revoked_at IS NULL AND unlock_key_id IN (
    SELECT k.id FROM field_unlock_keys k
    WHERE k.client_id = @client_id AND (NOT @unverified_only::boolean OR NOT k.attestation_verified));

-- name: FieldUnlockKeyLiveSessionCounts :many
-- How many unexpired live sessions each key signed in.
SELECT unlock_key_id::text AS unlock_key_id, COUNT(*)::bigint AS sessions FROM field_sessions
WHERE unlock_key_id = ANY(@unlock_key_ids::text[]) AND revoked_at IS NULL
  AND idle_expires_at > @now AND absolute_expires_at > @now
GROUP BY unlock_key_id;

-- name: FieldUnlockKeysUnderRequiredAttestation :many
-- The keys the attestation status list is checked against after each fetch:
-- of clients whose policy has unlock keys on and REQUIRED, that are live or
-- still have a live session.
SELECT k.id, k.attestation FROM field_unlock_keys k
JOIN field_policies p ON p.client_id = k.client_id
WHERE p.unlock_keys_enabled AND p.unlock_key_attestation = 'REQUIRED'
  AND (k.revoked_at IS NULL OR EXISTS (
      SELECT 1 FROM field_sessions s WHERE s.unlock_key_id = k.id AND s.revoked_at IS NULL))
ORDER BY k.id;

-- name: FieldSignInCodeWorker :many
-- The worker a console sign-in code names, read without a lock so the
-- redemption can lock the worker before it consumes the code (lock_order.go).
SELECT identity_id FROM field_sign_in_codes WHERE code_hash = $1;
