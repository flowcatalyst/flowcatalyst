-- +goose Up
-- Scrub SECRET and DB function-setting values from aud_logs.
--
-- PutSettingCommand did not mask `value`, so every SECRET and DB setting
-- written through PUT …/secrets/{key} or …/db/{name} was stored in plaintext in
-- aud_logs.operation_json. The command now masks it (AuditMaskedFields); this
-- migration removes what was already written. CONFIG rows are left alone.
--
-- Scrubbing the audit log does not un-leak a value: every secret and DSN ever
-- written through these endpoints must still be rotated.
--
-- Read-only impact check, to run before deploying:
--   SELECT COUNT(*) FROM aud_logs
--    WHERE operation = 'PutSettingCommand'
--      AND operation_json ? 'functionId'
--      AND operation_json->>'kind' IN ('SECRET', 'DB')
--      AND operation_json->>'value' IS DISTINCT FROM '***';
UPDATE aud_logs
   SET operation_json = jsonb_set(operation_json, '{value}', '"***"'::jsonb)
 WHERE operation = 'PutSettingCommand'
   AND operation_json ? 'functionId'
   AND operation_json->>'kind' IN ('SECRET', 'DB')
   AND operation_json ? 'value'
   AND operation_json->>'value' IS DISTINCT FROM '***';

-- +goose Down
-- Irreversible by design: the scrubbed values are not recoverable.
SELECT 1;
