-- +goose Up
-- Field-worker sign-in options (docs/field-worker-sign-in-options-spec.md):
-- unenrolled devices, printed QR badges, impersonation (supervisor act-as and
-- console sign-in codes) and test identities.
-- Re-runnable: every statement is IF NOT EXISTS / idempotent. The field_*
-- tables are small and the new columns are nullable or have constant defaults
-- (metadata-only), so none of this rewrites a large table
-- (docs/large-table-schema-changes.md).

-- A session on an unenrolled key has no device row. It is bound to the proof
-- key instead (jkt); existing sessions take the jkt of their device. The actor
-- columns mark an impersonated session (FIELD_WORKER | PRINCIPAL) and are
-- copied to every rotated child.
ALTER TABLE field_sessions ALTER COLUMN device_id DROP NOT NULL;
ALTER TABLE field_sessions ADD COLUMN IF NOT EXISTS jkt VARCHAR(64);
ALTER TABLE field_sessions ADD COLUMN IF NOT EXISTS actor_type VARCHAR(20);
ALTER TABLE field_sessions ADD COLUMN IF NOT EXISTS actor_id VARCHAR(17);
UPDATE field_sessions s SET jkt = d.jkt
  FROM field_devices d
 WHERE s.device_id = d.id AND s.jkt IS NULL;
CREATE INDEX IF NOT EXISTS idx_field_sessions_actor ON field_sessions (actor_id)
    WHERE revoked_at IS NULL AND actor_id IS NOT NULL;

ALTER TABLE field_identities ADD COLUMN IF NOT EXISTS can_impersonate BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE field_identities ADD COLUMN IF NOT EXISTS is_test BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE field_policies ADD COLUMN IF NOT EXISTS require_enrolled_device BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE field_policies ADD COLUMN IF NOT EXISTS unenrolled_idle_secs INTEGER NOT NULL DEFAULT 900;
ALTER TABLE field_policies ADD COLUMN IF NOT EXISTS badge_only_login BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE field_policies ADD COLUMN IF NOT EXISTS impersonation_max_secs INTEGER NOT NULL DEFAULT 14400;

-- The principal behind an impersonated sign-in (ACT_AS / SIGN_IN_CODE).
ALTER TABLE field_login_attempts ADD COLUMN IF NOT EXISTS actor_id VARCHAR(17);

-- Printed badges. Only the SHA-256 of the token is stored (shown once, at
-- issue). One ACTIVE badge per worker, enforced by the issue use case.
CREATE TABLE IF NOT EXISTS field_badges (
    id          VARCHAR(17)  PRIMARY KEY,                    -- fwb_
    client_id   VARCHAR(17)  NOT NULL,
    identity_id VARCHAR(17)  NOT NULL REFERENCES field_identities (id) ON DELETE CASCADE,
    token_hash  VARCHAR(64)  NOT NULL,                       -- SHA-256 hex of the normalised token
    label       VARCHAR(255),
    status      VARCHAR(20)  NOT NULL DEFAULT 'ACTIVE',      -- ACTIVE | REVOKED
    created_by  VARCHAR(100),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    revoked_at  TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    CONSTRAINT uq_field_badges_hash UNIQUE (token_hash)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_field_badges_identity_active ON field_badges (identity_id)
    WHERE status = 'ACTIVE';
CREATE INDEX IF NOT EXISTS idx_field_badges_identity ON field_badges (identity_id, created_at DESC);

-- Console sign-in codes: an admin mints one for a worker and an application,
-- the target app redeems it (single use, 10 minutes).
CREATE TABLE IF NOT EXISTS field_sign_in_codes (
    id              VARCHAR(17)  PRIMARY KEY,                -- fwi_
    client_id       VARCHAR(17)  NOT NULL,
    identity_id     VARCHAR(17)  NOT NULL REFERENCES field_identities (id) ON DELETE CASCADE,
    application_id  VARCHAR(17)  NOT NULL,
    code_hash       VARCHAR(64)  NOT NULL,
    created_by      VARCHAR(17)  NOT NULL,                   -- the principal id
    created_by_name VARCHAR(255),
    expires_at      TIMESTAMPTZ  NOT NULL,
    used_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_field_sign_in_codes_hash UNIQUE (code_hash)
);
CREATE INDEX IF NOT EXISTS idx_field_sign_in_codes_expires ON field_sign_in_codes (expires_at);

-- The seeder only inserts a built-in role when it is missing (it never
-- rewrites an existing role's permissions), so existing deployments pick the
-- new permission up here, once.
INSERT INTO iam_role_permissions (role_id, permission)
SELECT r.id, 'platform:iam:field-worker:impersonate'
  FROM iam_roles r
 WHERE r.name = 'platform:field-administrator'
ON CONFLICT DO NOTHING;

-- The seeded 1.0 schemas of the events this feature extends (isTest and
-- canImpersonate on the worker events, the policy fields on policy-updated, the
-- IP scope on throttled). The seeder never replaces an attached 1.0, so this
-- does it for existing databases, only while the 1.0 is still exactly what a
-- seed wrote (migration 072's rule). New event types are catalogued by the
-- seeder. Idempotent.
UPDATE msg_event_type_spec_versions sv
   SET schema_content = s.new_schema::jsonb, updated_at = NOW()
  FROM msg_event_types et,
       (VALUES
        ('platform:iam:field-worker:created',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"applicationIds":{"items":{"type":"string"},"type":"array"},"clientId":{"type":"string"},"name":{"type":"string"},"workerCode":{"type":"string"},"workerId":{"type":"string"}},"required":["workerId","clientId","workerCode","name"],"type":"object"}',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"applicationIds":{"items":{"type":"string"},"type":"array"},"clientId":{"type":"string"},"isTest":{"type":"boolean"},"name":{"type":"string"},"workerCode":{"type":"string"},"workerId":{"type":"string"}},"required":["workerId","clientId","workerCode","name","isTest"],"type":"object"}'),
        ('platform:iam:field-worker:ensured',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"applicationIds":{"items":{"type":"string"},"type":"array"},"clientId":{"type":"string"},"workerCode":{"type":"string"},"workerId":{"type":"string"}},"required":["workerId","clientId","workerCode"],"type":"object"}',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"applicationIds":{"items":{"type":"string"},"type":"array"},"clientId":{"type":"string"},"isTest":{"type":"boolean"},"workerCode":{"type":"string"},"workerId":{"type":"string"}},"required":["workerId","clientId","workerCode","isTest"],"type":"object"}'),
        ('platform:iam:field-worker:updated',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"clientId":{"type":"string"},"name":{"type":"string"},"workerCode":{"type":"string"},"workerId":{"type":"string"}},"required":["workerId","clientId","workerCode","name"],"type":"object"}',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"canImpersonate":{"type":"boolean"},"clientId":{"type":"string"},"isTest":{"type":"boolean"},"name":{"type":"string"},"workerCode":{"type":"string"},"workerId":{"type":"string"}},"required":["workerId","clientId","workerCode","name","isTest","canImpersonate"],"type":"object"}'),
        ('platform:iam:field-device:throttled',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"clientId":{"type":"string"},"deviceId":{"type":["string","null"]},"scope":{"enum":["DEVICE","CLIENT"],"type":"string"}},"required":["scope","clientId"],"type":"object"}',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"clientId":{"type":"string"},"deviceId":{"type":["string","null"]},"ipAddress":{"type":["string","null"]},"scope":{"enum":["DEVICE","CLIENT","IP"],"type":"string"}},"required":["scope","clientId"],"type":"object"}'),
        ('platform:iam:field-policy:updated',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"clientId":{"type":"string"}},"required":["clientId"],"type":"object"}',
         '{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"badgeOnlyLogin":{"type":"boolean"},"clientId":{"type":"string"},"impersonationMaxSecs":{"minimum":0,"type":"integer"},"requireEnrolledDevice":{"type":"boolean"},"unenrolledIdleSecs":{"minimum":0,"type":"integer"}},"required":["clientId","requireEnrolledDevice","unenrolledIdleSecs","badgeOnlyLogin","impersonationMaxSecs"],"type":"object"}')
       ) AS s(code, old_schema, new_schema)
 WHERE et.code = s.code
   AND sv.event_type_id = et.id
   AND sv.version = '1.0'
   AND sv.schema_content = s.old_schema::jsonb;

-- +goose Down
DELETE FROM iam_role_permissions
 WHERE permission = 'platform:iam:field-worker:impersonate'
   AND role_id IN (SELECT id FROM iam_roles WHERE name = 'platform:field-administrator');
DROP TABLE IF EXISTS field_sign_in_codes;
DROP TABLE IF EXISTS field_badges;
ALTER TABLE field_login_attempts DROP COLUMN IF EXISTS actor_id;
ALTER TABLE field_policies DROP COLUMN IF EXISTS impersonation_max_secs;
ALTER TABLE field_policies DROP COLUMN IF EXISTS badge_only_login;
ALTER TABLE field_policies DROP COLUMN IF EXISTS unenrolled_idle_secs;
ALTER TABLE field_policies DROP COLUMN IF EXISTS require_enrolled_device;
ALTER TABLE field_identities DROP COLUMN IF EXISTS is_test;
ALTER TABLE field_identities DROP COLUMN IF EXISTS can_impersonate;
DROP INDEX IF EXISTS idx_field_sessions_actor;
ALTER TABLE field_sessions DROP COLUMN IF EXISTS actor_id;
ALTER TABLE field_sessions DROP COLUMN IF EXISTS actor_type;
ALTER TABLE field_sessions DROP COLUMN IF EXISTS jkt;
-- The event schemas above are descriptive and stay as updated.
-- device_id stays nullable: unenrolled sessions written since Up have no device.
