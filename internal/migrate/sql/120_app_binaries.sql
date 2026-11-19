-- +goose Up
-- App binaries and install links (docs/app-updates-design.md section 14).
--
-- app_binary_files is the per-application registry of the binaries the
-- content-addressed store holds for it, with what the platform read from each
-- file when it was uploaded (package, version, runtime version, signing
-- certificate): a row is written only after the store accepted the bytes, their
-- SHA-256 matched and the file parsed as a signed APK. app_binaries is one row
-- per published binary of a phone app (application, app key, platform, channel,
-- variant), immutable, the file's facts copied onto it. app_install_links is the
-- one link per application, app key and channel through which a person
-- downloads the newest binary of each variant; its token is kept only as a
-- SHA-256. No foreign keys (docs/foreign-keys.md), like the update tables.
CREATE TABLE IF NOT EXISTS app_binary_files (
    application_id       VARCHAR(17)  NOT NULL,
    sha256               CHAR(64)     NOT NULL,
    size_bytes           BIGINT       NOT NULL,
    platform             VARCHAR(10)  NOT NULL CHECK (platform IN ('android', 'ios')),
    package_name         VARCHAR(255) NOT NULL,
    version_code         BIGINT       NOT NULL,
    version_name         VARCHAR(255),
    min_sdk              INT          NOT NULL DEFAULT 0,
    debuggable           BOOLEAN      NOT NULL,
    runtime_version      VARCHAR(128),
    update_url           TEXT,
    update_channel       VARCHAR(255),
    update_key_id        VARCHAR(255),
    signing_cert_sha256  CHAR(64)     NOT NULL,
    signing_cert_subject TEXT         NOT NULL,
    debug_certificate    BOOLEAN      NOT NULL,
    signing_schemes      INT[]        NOT NULL,
    created_by           VARCHAR(17),
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (application_id, sha256)
);

CREATE TABLE IF NOT EXISTS app_binaries (
    id                   VARCHAR(17)  PRIMARY KEY,
    application_id       VARCHAR(17)  NOT NULL,
    app_key              VARCHAR(40)  NOT NULL,
    platform             VARCHAR(10)  NOT NULL CHECK (platform IN ('android', 'ios')),
    channel              VARCHAR(64)  NOT NULL,
    variant              VARCHAR(20)  NOT NULL,
    sha256               CHAR(64)     NOT NULL,
    size_bytes           BIGINT       NOT NULL,
    package_name         VARCHAR(255) NOT NULL,
    version_code         BIGINT       NOT NULL,
    version_name         VARCHAR(255),
    min_sdk              INT          NOT NULL DEFAULT 0,
    debuggable           BOOLEAN      NOT NULL,
    runtime_version      VARCHAR(128),
    update_url           TEXT,
    update_channel       VARCHAR(255),
    update_key_id        VARCHAR(255),
    signing_cert_sha256  CHAR(64)     NOT NULL,
    signing_cert_subject TEXT         NOT NULL,
    debug_certificate    BOOLEAN      NOT NULL,
    signing_schemes      INT[]        NOT NULL,
    message              TEXT,
    source_commit        VARCHAR(100),
    published_by         VARCHAR(17)  NOT NULL,
    created_at           TIMESTAMPTZ  NOT NULL,
    -- Re-publishing the same file to the same place is a no-op.
    CONSTRAINT app_binaries_file_once UNIQUE (application_id, app_key, platform, channel, variant, sha256),
    -- A version code names one file per place, so "the newest" is well defined;
    -- also the read of the newest binary of a variant.
    CONSTRAINT app_binaries_version_once UNIQUE (application_id, app_key, platform, channel, variant, version_code)
);

-- Is there a binary of this runtime version on a channel (an update pipeline's question).
CREATE INDEX IF NOT EXISTS idx_app_binaries_runtime
    ON app_binaries (application_id, app_key, platform, channel, runtime_version);

CREATE TABLE IF NOT EXISTS app_install_links (
    id             VARCHAR(17)  PRIMARY KEY,
    application_id VARCHAR(17)  NOT NULL,
    app_key        VARCHAR(40)  NOT NULL,
    channel        VARCHAR(64)  NOT NULL,
    token_hash     CHAR(64)     NOT NULL,
    generation     INT          NOT NULL DEFAULT 1,
    created_by     VARCHAR(17)  NOT NULL,
    created_at     TIMESTAMPTZ  NOT NULL,
    updated_by     VARCHAR(17)  NOT NULL,
    updated_at     TIMESTAMPTZ  NOT NULL,
    CONSTRAINT app_install_links_one_per_channel UNIQUE (application_id, app_key, channel),
    CONSTRAINT app_install_links_token_once UNIQUE (token_hash)
);

-- +goose Down
DROP TABLE IF EXISTS app_install_links;
DROP TABLE IF EXISTS app_binaries;
DROP TABLE IF EXISTS app_binary_files;
