-- +goose Up
-- Field-worker identity plane (docs/field-worker-identity-design.md section 4.4,
-- docs/field-worker-identity-p1-spec.md section 2): workers identified by
-- (client, worker_code) with a PIN, signing in from enrolled, key-bound
-- devices. Wholly separate from iam_principals and portal_identities.
-- Re-runnable: every statement is IF NOT EXISTS. All tables are new and empty,
-- so plain CREATEs are safe (docs/large-table-schema-changes.md). No foreign
-- keys to other modules' tables (application_id / client_id are plain
-- references, the convention of the newer tables); the module's own
-- grant/session rows reference their parents and are deleted by the use cases.

CREATE TABLE IF NOT EXISTS field_identities (
    id                   VARCHAR(17)  PRIMARY KEY,           -- fwu_
    client_id            VARCHAR(17)  NOT NULL,
    worker_code          VARCHAR(64)  NOT NULL,              -- normalised upper-case
    name                 VARCHAR(255) NOT NULL,
    email                VARCHAR(255),
    status               VARCHAR(20)  NOT NULL DEFAULT 'ACTIVE',   -- ACTIVE | DISABLED
    pin_hash             TEXT,                                -- NULL until a PIN is set
    pin_scheme           VARCHAR(20)  NOT NULL DEFAULT 'PEPPER_V1', -- PEPPER_V1 | LEGACY
    pepper_version       INTEGER      NOT NULL DEFAULT 1,
    pin_set_at           TIMESTAMPTZ,
    pin_change_required  BOOLEAN      NOT NULL DEFAULT FALSE,
    failed_since_success INTEGER      NOT NULL DEFAULT 0,
    locked_until         TIMESTAMPTZ,
    hard_locked          BOOLEAN      NOT NULL DEFAULT FALSE,
    last_login_at        TIMESTAMPTZ,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_field_identities_client_code UNIQUE (client_id, worker_code)
);
CREATE INDEX IF NOT EXISTS idx_field_identities_client_name
    ON field_identities (client_id, lower(name) text_pattern_ops);

CREATE TABLE IF NOT EXISTS field_identity_apps (
    identity_id    VARCHAR(17) NOT NULL REFERENCES field_identities (id) ON DELETE CASCADE,
    application_id VARCHAR(17) NOT NULL,
    source         VARCHAR(20) NOT NULL DEFAULT 'ADMIN',
    granted_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (identity_id, application_id)
);
CREATE INDEX IF NOT EXISTS idx_field_identity_apps_app ON field_identity_apps (application_id);

CREATE TABLE IF NOT EXISTS field_devices (
    id                   VARCHAR(17)  PRIMARY KEY,           -- fwd_
    client_id            VARCHAR(17)  NOT NULL,
    mode                 VARCHAR(20)  NOT NULL,              -- PERSONAL | SHARED
    label                VARCHAR(255),
    platform             VARCHAR(40),
    model                VARCHAR(120),
    jkt                  VARCHAR(64)  NOT NULL,              -- RFC 7638 thumbprint of the device key
    public_key_jwk       TEXT         NOT NULL,
    attestation_level    VARCHAR(20)  NOT NULL DEFAULT 'NONE', -- STRONGBOX | TEE | SOFTWARE | NONE
    attestation_chain    TEXT,                                 -- raw, as reported at enrolment; unverified in P1
    assigned_identity_id VARCHAR(17),
    status               VARCHAR(20)  NOT NULL DEFAULT 'ACTIVE', -- ACTIVE | SUSPENDED | REVOKED
    enrolled_by          VARCHAR(100),
    enrolled_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    last_seen_at         TIMESTAMPTZ,
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_field_devices_jkt UNIQUE (jkt)
);
CREATE INDEX IF NOT EXISTS idx_field_devices_client ON field_devices (client_id, status);
CREATE INDEX IF NOT EXISTS idx_field_devices_assigned ON field_devices (assigned_identity_id)
    WHERE assigned_identity_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS field_device_apps (
    device_id      VARCHAR(17) NOT NULL REFERENCES field_devices (id) ON DELETE CASCADE,
    application_id VARCHAR(17) NOT NULL,
    PRIMARY KEY (device_id, application_id)
);

CREATE TABLE IF NOT EXISTS field_enrolment_codes (
    id                   VARCHAR(17) PRIMARY KEY,            -- fwe_
    client_id            VARCHAR(17) NOT NULL,
    code_hash            VARCHAR(64) NOT NULL,               -- SHA-256 hex of the code
    mode                 VARCHAR(20) NOT NULL,
    app_ids              TEXT[]      NOT NULL DEFAULT '{}',
    assigned_identity_id VARCHAR(17),
    label                VARCHAR(255),
    expires_at           TIMESTAMPTZ NOT NULL,
    used_at              TIMESTAMPTZ,
    created_by           VARCHAR(100),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_field_enrolment_codes_hash UNIQUE (code_hash)
);
CREATE INDEX IF NOT EXISTS idx_field_enrolment_codes_expires ON field_enrolment_codes (expires_at);

CREATE TABLE IF NOT EXISTS field_sessions (
    id                  VARCHAR(17) PRIMARY KEY,             -- fws_, one per issued refresh token
    family_id           VARCHAR(17) NOT NULL,                -- one login = one family (= the access token's sid)
    identity_id         VARCHAR(17) NOT NULL,
    device_id           VARCHAR(17) NOT NULL,
    application_id      VARCHAR(17) NOT NULL,
    refresh_hash        VARCHAR(64) NOT NULL,                -- SHA-256 hex of the fwr_ token
    amr                 TEXT[]      NOT NULL DEFAULT '{}',
    auth_time           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    issued_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    idle_expires_at     TIMESTAMPTZ NOT NULL,
    absolute_expires_at TIMESTAMPTZ NOT NULL,
    rotated_at          TIMESTAMPTZ,
    revoked_at          TIMESTAMPTZ,
    revoked_reason      VARCHAR(30), -- ROTATED | REUSE | LOGOUT | DEVICE_REVOKED | WORKER_SUSPENDED | ADMIN | PIN_CHANGED
    pin_change_required BOOLEAN     NOT NULL DEFAULT FALSE,
    CONSTRAINT uq_field_sessions_refresh UNIQUE (refresh_hash)
);
CREATE INDEX IF NOT EXISTS idx_field_sessions_identity ON field_sessions (identity_id)
    WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_field_sessions_device ON field_sessions (device_id)
    WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_field_sessions_family ON field_sessions (family_id);

CREATE TABLE IF NOT EXISTS field_policies (
    client_id             VARCHAR(17) PRIMARY KEY,
    pin_min_length        INTEGER     NOT NULL DEFAULT 6,
    soft_lock_threshold   INTEGER     NOT NULL DEFAULT 5,
    soft_lock_secs        INTEGER     NOT NULL DEFAULT 900,
    hard_lock_threshold   INTEGER     NOT NULL DEFAULT 10,
    shared_idle_secs      INTEGER     NOT NULL DEFAULT 900,
    personal_idle_secs    INTEGER     NOT NULL DEFAULT 7200,
    shift_max_secs        INTEGER     NOT NULL DEFAULT 50400,
    require_attestation   BOOLEAN     NOT NULL DEFAULT FALSE,
    pin_reauth_days       INTEGER     NOT NULL DEFAULT 7,
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Single-use DPoP-style server nonces (SHA-256 of the nonce), consumed with
-- DELETE ... RETURNING; the purger removes the expired.
CREATE TABLE IF NOT EXISTS field_nonces (
    nonce_hash BYTEA       PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_field_nonces_expires ON field_nonces (expires_at);

-- Field sign-in attempts (success and failure) with the device and method.
-- A dedicated table rather than iam_login_attempts: that table keys on a
-- principal and an email-shaped identifier and feeds the employee login
-- throttle, neither of which fits a (client, worker_code, device) subject.
CREATE TABLE IF NOT EXISTS field_login_attempts (
    id             VARCHAR(17) PRIMARY KEY,
    client_id      VARCHAR(17),
    identity_id    VARCHAR(17),                              -- NULL when the worker code is unknown
    worker_code    VARCHAR(64),
    device_id      VARCHAR(17),
    application_id VARCHAR(17),
    auth_method    VARCHAR(20) NOT NULL DEFAULT 'PIN',
    outcome        VARCHAR(20) NOT NULL,                     -- SUCCESS | FAILURE
    failure_reason VARCHAR(60),
    ip_address     VARCHAR(45),
    attempted_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_field_login_attempts_identity
    ON field_login_attempts (identity_id, attempted_at DESC);
CREATE INDEX IF NOT EXISTS idx_field_login_attempts_at ON field_login_attempts (attempted_at);

-- +goose Down
DROP TABLE IF EXISTS field_login_attempts;
DROP TABLE IF EXISTS field_nonces;
DROP TABLE IF EXISTS field_policies;
DROP TABLE IF EXISTS field_sessions;
DROP TABLE IF EXISTS field_enrolment_codes;
DROP TABLE IF EXISTS field_device_apps;
DROP TABLE IF EXISTS field_devices;
DROP TABLE IF EXISTS field_identity_apps;
DROP TABLE IF EXISTS field_identities;
