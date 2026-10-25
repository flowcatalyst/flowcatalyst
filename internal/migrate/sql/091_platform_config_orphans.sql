-- +goose Up
-- Platform configuration (app_platform_configs) and its access grants
-- (app_platform_config_access) are keyed by application code without a
-- foreign key, and until now survived the application's deletion: a new
-- application created with the same code inherited the old one's values,
-- secrets included. Deleting an application now deletes them in the same
-- transaction; this removes the rows left behind by earlier deletions.
--
-- Only rows whose code names no application at all are removed. Kept, and
-- counted as ambiguous: rows of the platform's own code ('platform', whose
-- application row the seeder writes after the migrations on a new database),
-- and rows whose code matches an existing application's code only when case
-- and surrounding spaces are ignored (a typo or a hand edit, not clearly an
-- orphan). The counts are raised as a NOTICE. Idempotent.
--
-- Also: the seeded 1.0 schema of platform:iam:application:deleted gains the
-- two counts the event now carries (platformConfigPropertiesDeleted,
-- platformConfigGrantsDeleted), replaced only while it is still exactly what a
-- seed wrote (migration 072's rule).
-- +goose StatementBegin
DO $$
DECLARE
    removed_configs bigint;
    removed_grants  bigint;
    kept_configs    bigint;
    kept_grants     bigint;
BEGIN
    SELECT count(*) INTO kept_configs
      FROM app_platform_configs pc
     WHERE NOT EXISTS (SELECT 1 FROM app_applications a WHERE a.code = pc.application_code)
       AND (pc.application_code = 'platform'
            OR EXISTS (SELECT 1 FROM app_applications a
                        WHERE lower(btrim(a.code)) = lower(btrim(pc.application_code))));
    SELECT count(*) INTO kept_grants
      FROM app_platform_config_access pca
     WHERE NOT EXISTS (SELECT 1 FROM app_applications a WHERE a.code = pca.application_code)
       AND (pca.application_code = 'platform'
            OR EXISTS (SELECT 1 FROM app_applications a
                        WHERE lower(btrim(a.code)) = lower(btrim(pca.application_code))));

    DELETE FROM app_platform_configs pc
     WHERE pc.application_code <> 'platform'
       AND NOT EXISTS (SELECT 1 FROM app_applications a
                        WHERE lower(btrim(a.code)) = lower(btrim(pc.application_code)));
    GET DIAGNOSTICS removed_configs = ROW_COUNT;

    DELETE FROM app_platform_config_access pca
     WHERE pca.application_code <> 'platform'
       AND NOT EXISTS (SELECT 1 FROM app_applications a
                        WHERE lower(btrim(a.code)) = lower(btrim(pca.application_code)));
    GET DIAGNOSTICS removed_grants = ROW_COUNT;

    RAISE NOTICE 'Migration 091: removed % platform config value(s) and % access grant(s) of applications that no longer exist; kept % value(s) and % grant(s) whose application is ambiguous',
        removed_configs, removed_grants, kept_configs, kept_grants;
END $$;
-- +goose StatementEnd

UPDATE msg_event_type_spec_versions sv
   SET schema_content = '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"applicationId":{"type":"string"},"code":{"type":"string"},"platformConfigGrantsDeleted":{"minimum":0,"type":"integer"},"platformConfigPropertiesDeleted":{"minimum":0,"type":"integer"}},"required":["applicationId","code","platformConfigPropertiesDeleted","platformConfigGrantsDeleted"],"type":"object"}'::jsonb,
       updated_at = NOW()
  FROM msg_event_types et
 WHERE et.id = sv.event_type_id
   AND et.code = 'platform:iam:application:deleted'
   AND sv.version = '1.0'
   AND sv.schema_content = '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"applicationId":{"type":"string"},"code":{"type":"string"}},"required":["applicationId","code"],"type":"object"}'::jsonb;

-- +goose Down
-- The removed rows are not restored (they belonged to no application).
UPDATE msg_event_type_spec_versions sv
   SET schema_content = '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"applicationId":{"type":"string"},"code":{"type":"string"}},"required":["applicationId","code"],"type":"object"}'::jsonb,
       updated_at = NOW()
  FROM msg_event_types et
 WHERE et.id = sv.event_type_id
   AND et.code = 'platform:iam:application:deleted'
   AND sv.version = '1.0'
   AND sv.schema_content = '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"applicationId":{"type":"string"},"code":{"type":"string"},"platformConfigGrantsDeleted":{"minimum":0,"type":"integer"},"platformConfigPropertiesDeleted":{"minimum":0,"type":"integer"}},"required":["applicationId","code","platformConfigPropertiesDeleted","platformConfigGrantsDeleted"],"type":"object"}'::jsonb;
