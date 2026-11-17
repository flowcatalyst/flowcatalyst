-- +goose Up
-- Over-the-air updates for phone apps built with Expo (docs/app-updates-design.md).
--
-- app_update_files is the per-application registry of the files the
-- content-addressed store holds for it: a row is written only after the store
-- accepted the bytes and verified their SHA-256. app_updates is one row per
-- update (or rollback to the embedded bundle) of a branch (application, app
-- key, platform, runtime version, channel); app_update_assets is an update's
-- manifest asset list in order (ordinal 0 is the launch asset). No foreign
-- keys (docs/foreign-keys.md): the rows are written per publish and will be
-- pruned by retention.
CREATE TABLE IF NOT EXISTS app_update_files (
    application_id VARCHAR(17)  NOT NULL,
    sha256         CHAR(64)     NOT NULL,
    md5            CHAR(32)     NOT NULL,
    size_bytes     BIGINT       NOT NULL,
    created_by     VARCHAR(17),
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (application_id, sha256)
);

CREATE TABLE IF NOT EXISTS app_updates (
    id               VARCHAR(17)  PRIMARY KEY,
    application_id   VARCHAR(17)  NOT NULL,
    app_key          VARCHAR(40)  NOT NULL,
    platform         VARCHAR(10)  NOT NULL CHECK (platform IN ('android', 'ios')),
    runtime_version  VARCHAR(128) NOT NULL,
    channel          VARCHAR(64)  NOT NULL,
    kind             VARCHAR(30)  NOT NULL CHECK (kind IN ('UPDATE', 'ROLLBACK_TO_EMBEDDED')),
    manifest_id      UUID,
    created_at       TIMESTAMPTZ  NOT NULL,
    rollout_percent  SMALLINT     NOT NULL DEFAULT 100 CHECK (rollout_percent BETWEEN 0 AND 100),
    message          TEXT,
    source_commit    VARCHAR(100),
    republished_from VARCHAR(17),
    expo_client      JSONB,
    published_by     VARCHAR(17)  NOT NULL,
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT app_updates_manifest_by_kind CHECK ((kind = 'UPDATE') = (manifest_id IS NOT NULL))
);

-- The selection read: the newest rows of one branch.
CREATE INDEX IF NOT EXISTS idx_app_updates_branch
    ON app_updates (application_id, app_key, platform, runtime_version, channel, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS app_update_assets (
    update_id       VARCHAR(17)  NOT NULL,
    ordinal         INT          NOT NULL,
    sha256          CHAR(64)     NOT NULL,
    md5             CHAR(32)     NOT NULL,
    file_extension  VARCHAR(20)  NOT NULL,
    content_type    VARCHAR(100) NOT NULL,
    is_launch_asset BOOLEAN      NOT NULL,
    PRIMARY KEY (update_id, ordinal)
);

-- The asset route: is this file part of a published update of this phone app.
CREATE INDEX IF NOT EXISTS idx_app_update_assets_sha256 ON app_update_assets (sha256);

-- +goose Down
DROP TABLE IF EXISTS app_update_assets;
DROP TABLE IF EXISTS app_updates;
DROP TABLE IF EXISTS app_update_files;
