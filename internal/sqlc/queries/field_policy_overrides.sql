-- Per-application overrides of a client's field policy and the client's
-- locks (migration 130, docs/field-policy-application-overrides.md).

-- name: FieldPolicyOverrideGet :one
SELECT * FROM field_policy_overrides WHERE client_id = $1 AND application_id = $2;

-- name: FieldPolicyOverrideList :many
SELECT * FROM field_policy_overrides WHERE client_id = $1 ORDER BY application_id;

-- name: FieldPolicyOverrideUpsert :exec
INSERT INTO field_policy_overrides (client_id, application_id, unlock_keys_enabled, require_enrolled_device,
    badge_only_login, shared_idle_secs, personal_idle_secs, shift_max_secs, unenrolled_idle_secs, photos_enabled,
    updated_at, updated_by)
VALUES (@client_id, @application_id, @unlock_keys_enabled, @require_enrolled_device, @badge_only_login,
    @shared_idle_secs, @personal_idle_secs, @shift_max_secs, @unenrolled_idle_secs, @photos_enabled,
    @updated_at, @updated_by)
ON CONFLICT (client_id, application_id) DO UPDATE SET
    unlock_keys_enabled = EXCLUDED.unlock_keys_enabled, require_enrolled_device = EXCLUDED.require_enrolled_device,
    badge_only_login = EXCLUDED.badge_only_login, shared_idle_secs = EXCLUDED.shared_idle_secs,
    personal_idle_secs = EXCLUDED.personal_idle_secs, shift_max_secs = EXCLUDED.shift_max_secs,
    unenrolled_idle_secs = EXCLUDED.unenrolled_idle_secs, photos_enabled = EXCLUDED.photos_enabled,
    updated_at = EXCLUDED.updated_at, updated_by = EXCLUDED.updated_by;

-- name: FieldPolicyOverrideDelete :execrows
DELETE FROM field_policy_overrides WHERE client_id = $1 AND application_id = $2;

-- name: FieldPolicyLockList :many
SELECT member FROM field_policy_locks WHERE client_id = $1 ORDER BY member;

-- name: FieldPolicyLockClear :exec
DELETE FROM field_policy_locks WHERE client_id = $1;

-- name: FieldPolicyLockAdd :exec
INSERT INTO field_policy_locks (client_id, member, locked_at, locked_by) VALUES ($1, $2, $3, $4)
ON CONFLICT (client_id, member) DO NOTHING;

-- name: FieldSessionEndUnlockKeysOfApplications :execrows
-- Ends the live sessions an unlock key of the client signed in to the given
-- applications (an application whose unlock keys are now off).
UPDATE field_sessions SET revoked_at = @at, revoked_reason = 'UNLOCK_KEY_REVOKED'
WHERE revoked_at IS NULL AND application_id = ANY(@application_ids::text[])
  AND unlock_key_id IN (SELECT k.id FROM field_unlock_keys k WHERE k.client_id = @client_id);

-- name: FieldSessionEndUnlockKeysOutsideApplications :execrows
-- Ends the live sessions an unlock key of the client signed in to any
-- application but the given ones (the applications that keep unlock keys on).
UPDATE field_sessions SET revoked_at = @at, revoked_reason = 'UNLOCK_KEY_REVOKED'
WHERE revoked_at IS NULL AND NOT (application_id = ANY(@application_ids::text[]))
  AND unlock_key_id IN (SELECT k.id FROM field_unlock_keys k WHERE k.client_id = @client_id);

-- name: FieldApplicationServesClient :one
-- Whether an application's override may apply for the client: the application
-- is active and enabled for the client.
SELECT EXISTS (
    SELECT 1 FROM app_applications a JOIN app_client_configs c ON c.application_id = a.id
    WHERE a.id = @application_id AND c.client_id = @client_id AND c.enabled AND a.active
)::boolean AS serves;

-- name: FieldApplicationsServingClient :many
-- The applications active and enabled for the client.
SELECT a.id::text AS id FROM app_applications a JOIN app_client_configs c ON c.application_id = a.id
WHERE c.client_id = @client_id AND c.enabled AND a.active;

-- name: FieldPolicyOverrideDeleteByApplication :exec
-- An application deleted: its overrides go with it.
DELETE FROM field_policy_overrides WHERE application_id = $1;

-- name: FieldPolicyOverrideDeleteForApplicationAndClient :exec
-- An application disabled for a client: its override for that client goes.
DELETE FROM field_policy_overrides WHERE application_id = $1 AND client_id = $2;
