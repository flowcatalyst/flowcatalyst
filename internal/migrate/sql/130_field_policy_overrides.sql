-- +goose Up
-- Per-application overrides of a client's field policy
-- (docs/field-policy-application-overrides.md).
--
-- field_policy_overrides holds, per (client, application), the members an
-- application overrides: NULL is "follow the client". Only the overridable
-- members have a column, so a member that may not be overridden can never be
-- stored, whatever a caller sends. The ranges are the client-wide policy's.
--
-- field_policy_locks names the overridable members a client does not let its
-- applications override (set by platform administrators only). An override of
-- a locked member is kept and does not apply.
--
-- No foreign keys (docs/foreign-keys.md). Cost: two new, small tables, empty
-- when created.
SET LOCAL lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS field_policy_overrides (
    client_id               VARCHAR(17)  NOT NULL,
    application_id          VARCHAR(17)  NOT NULL,
    unlock_keys_enabled     BOOLEAN,
    require_enrolled_device BOOLEAN,
    badge_only_login        BOOLEAN,
    shared_idle_secs        INTEGER CHECK (shared_idle_secs BETWEEN 60 AND 86400),
    personal_idle_secs      INTEGER CHECK (personal_idle_secs BETWEEN 60 AND 86400),
    shift_max_secs          INTEGER CHECK (shift_max_secs BETWEEN 3600 AND 86400),
    unenrolled_idle_secs    INTEGER CHECK (unenrolled_idle_secs BETWEEN 60 AND 86400),
    photos_enabled          BOOLEAN,
    updated_at              TIMESTAMPTZ  NOT NULL,
    updated_by              VARCHAR(100),
    PRIMARY KEY (client_id, application_id)
);

CREATE TABLE IF NOT EXISTS field_policy_locks (
    client_id  VARCHAR(17)  NOT NULL,
    member     VARCHAR(40)  NOT NULL CHECK (member IN ('unlockKeysEnabled', 'requireEnrolledDevice',
                   'badgeOnlyLogin', 'sharedIdleSecs', 'personalIdleSecs', 'shiftMaxSecs',
                   'unenrolledIdleSecs', 'photosEnabled')),
    locked_at  TIMESTAMPTZ  NOT NULL,
    locked_by  VARCHAR(100),
    PRIMARY KEY (client_id, member)
);

-- The lock permission is a platform administrator's: the seeder never rewrites
-- an existing role, so the Platform Admin role of an existing deployment gets
-- it here, once (super-admin holds it through its wildcard).
INSERT INTO iam_role_permissions (role_id, permission)
SELECT r.id, 'platform:iam:field-policy:lock'
  FROM iam_roles r
 WHERE r.name = 'platform:admin'
ON CONFLICT DO NOTHING;

-- +goose Down
SET LOCAL lock_timeout = '5s';
DELETE FROM iam_role_permissions
 WHERE permission = 'platform:iam:field-policy:lock'
   AND role_id IN (SELECT id FROM iam_roles WHERE name = 'platform:admin');
DROP TABLE IF EXISTS field_policy_locks;
DROP TABLE IF EXISTS field_policy_overrides;
