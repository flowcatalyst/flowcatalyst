-- 025_application_openapi_specs.sql
--
-- Per-application OpenAPI document storage with versioning. At most one row
-- per application has status='CURRENT' (enforced by a partial unique index);
-- previous syncs are flipped to status='ARCHIVED' with computed change_notes
-- so the lineage is auditable. The platform itself is one of the applications
-- (seeded row with code='platform') so its spec is stored the same way.

CREATE TABLE IF NOT EXISTS app_application_openapi_specs (
    id                  VARCHAR(17) PRIMARY KEY,
    application_id      VARCHAR(17) NOT NULL REFERENCES app_applications(id) ON DELETE CASCADE,
    version             VARCHAR(64) NOT NULL,
    status              VARCHAR(20) NOT NULL,
    spec                JSONB       NOT NULL,
    spec_hash           VARCHAR(64) NOT NULL,
    change_notes        JSONB,
    change_notes_text   TEXT,
    synced_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    synced_by           VARCHAR(17),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (application_id, version)
);

-- Enforce at-most-one CURRENT per application. Archive operations must demote
-- the prior row before inserting the new one (the sync use case does this).
CREATE UNIQUE INDEX IF NOT EXISTS idx_app_openapi_one_current
    ON app_application_openapi_specs (application_id)
    WHERE status = 'CURRENT';

CREATE INDEX IF NOT EXISTS idx_app_openapi_app
    ON app_application_openapi_specs (application_id, synced_at DESC);
