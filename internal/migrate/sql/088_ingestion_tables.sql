-- +goose Up
-- Ingestion in the platform (docs/ingestion-design.md section 3).
--
-- An ingestion is (application, client, code) plus a source, a split and a
-- target. The row holds the LIVE definition; ing_versions keeps every saved
-- version, and ing_ingestions.version is the pointer to the live one
-- (latest_version is the highest number saved). No table holds a secret
-- value: a definition names secrets only.
--
-- All tables are new and empty, so plain CREATE statements are safe on a
-- production database (docs/large-table-schema-changes.md). The two that grow
-- with traffic, ing_exchange_metrics and ing_failures, carry no foreign key
-- (the function tables' convention, migration 059): a deleted ingestion's rows
-- are left for retention to sweep, and an insert pays for no parent lookup.

-- A set owns the ingestions it applied until released (WP 4 fills the logic).
CREATE TABLE IF NOT EXISTS ing_sets (
    id         VARCHAR(17)  PRIMARY KEY,
    name       VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL,
    updated_at TIMESTAMPTZ  NOT NULL,
    CONSTRAINT uq_ing_sets_name UNIQUE (name)
);

CREATE TABLE IF NOT EXISTS ing_ingestions (
    id                   VARCHAR(17)  PRIMARY KEY,
    application_id       VARCHAR(17)  NOT NULL REFERENCES app_applications(id),
    client_id            VARCHAR(17)  NOT NULL REFERENCES tnt_clients(id),
    code                 VARCHAR(64)  NOT NULL,
    name                 VARCHAR(200) NOT NULL,
    active               BOOLEAN      NOT NULL DEFAULT TRUE,
    -- The live version (0 = none live yet), and the highest version saved.
    -- Update is guarded on the live one (expectedVersion); latest_version only
    -- ever grows. The definition columns below always hold the live version's
    -- definition, or the latest saved one while nothing is live.
    version              INTEGER      NOT NULL,
    latest_version       INTEGER      NOT NULL,
    source               JSONB        NOT NULL,
    split                JSONB        NOT NULL,
    target               JSONB        NOT NULL,
    keep_failed_payloads BOOLEAN      NOT NULL DEFAULT FALSE,
    managed_by           VARCHAR(17)  REFERENCES ing_sets(id) ON DELETE SET NULL,
    created_at           TIMESTAMPTZ  NOT NULL,
    updated_at           TIMESTAMPTZ  NOT NULL,
    CONSTRAINT uq_ing_ingestions_code UNIQUE (application_id, client_id, code),
    CONSTRAINT chk_ing_ingestions_version CHECK (version >= 0 AND version <= latest_version)
);

-- What a runtime's poll reads: the live, running ingestions.
CREATE INDEX IF NOT EXISTS idx_ing_ingestions_active
    ON ing_ingestions (application_id, client_id) WHERE active AND version > 0;
-- Reach filter and the per-client list.
CREATE INDEX IF NOT EXISTS idx_ing_ingestions_client
    ON ing_ingestions (client_id, code);
CREATE INDEX IF NOT EXISTS idx_ing_ingestions_managed_by
    ON ing_ingestions (managed_by) WHERE managed_by IS NOT NULL;

-- One row per saved version. The definition is the editable part of the
-- ingestion (name, keepFailedPayloads, source, split, target).
CREATE TABLE IF NOT EXISTS ing_versions (
    id           VARCHAR(17) PRIMARY KEY,
    ingestion_id VARCHAR(17) NOT NULL REFERENCES ing_ingestions(id) ON DELETE CASCADE,
    version      INTEGER     NOT NULL,
    definition   JSONB       NOT NULL,
    note         TEXT,
    -- The set apply that produced it, when one did.
    applied_from VARCHAR(17),
    created_by   VARCHAR(17),
    created_at   TIMESTAMPTZ NOT NULL,
    CONSTRAINT uq_ing_versions_version UNIQUE (ingestion_id, version)
);

CREATE TABLE IF NOT EXISTS ing_runtimes (
    runtime_id       VARCHAR(200) PRIMARY KEY,
    kinds            JSONB        NOT NULL DEFAULT '[]'::jsonb,
    contract_version INTEGER      NOT NULL,
    -- Informational only: the platform never calls it.
    url              TEXT,
    first_seen_at    TIMESTAMPTZ  NOT NULL,
    last_seen_at     TIMESTAMPTZ  NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ing_runtimes_last_seen ON ing_runtimes (last_seen_at);

-- Polled sources run on one runtime at a time.
CREATE TABLE IF NOT EXISTS ing_leases (
    ingestion_id VARCHAR(17)  PRIMARY KEY REFERENCES ing_ingestions(id) ON DELETE CASCADE,
    runtime_id   VARCHAR(200) NOT NULL,
    acquired_at  TIMESTAMPTZ  NOT NULL,
    expires_at   TIMESTAMPTZ  NOT NULL,
    released     BOOLEAN      NOT NULL DEFAULT FALSE
);

-- Small values a polled source keeps (watermark, cursor); only the lease
-- holder writes.
CREATE TABLE IF NOT EXISTS ing_state (
    ingestion_id VARCHAR(17)  NOT NULL REFERENCES ing_ingestions(id) ON DELETE CASCADE,
    key          VARCHAR(100) NOT NULL,
    value        TEXT         NOT NULL,
    updated_at   TIMESTAMPTZ  NOT NULL,
    PRIMARY KEY (ingestion_id, key)
);

-- Per runtime: which version it runs and how it is doing.
CREATE TABLE IF NOT EXISTS ing_route_states (
    ingestion_id VARCHAR(17)  NOT NULL REFERENCES ing_ingestions(id) ON DELETE CASCADE,
    runtime_id   VARCHAR(200) NOT NULL,
    version      INTEGER      NOT NULL,
    status       VARCHAR(20)  NOT NULL,
    error        TEXT,
    updated_at   TIMESTAMPTZ  NOT NULL,
    PRIMARY KEY (ingestion_id, runtime_id)
);

-- High volume: one row per runtime window. The key makes a report idempotent
-- per window. Pruned after 30 days (the pruning job is WP 3).
CREATE TABLE IF NOT EXISTS ing_exchange_metrics (
    ingestion_id VARCHAR(17)  NOT NULL,
    runtime_id   VARCHAR(200) NOT NULL,
    window_start TIMESTAMPTZ  NOT NULL,
    window_secs  INTEGER      NOT NULL,
    completed    BIGINT       NOT NULL DEFAULT 0,
    failed       BIGINT       NOT NULL DEFAULT 0,
    records      BIGINT       NOT NULL DEFAULT 0,
    jobs         BIGINT       NOT NULL DEFAULT 0,
    PRIMARY KEY (ingestion_id, runtime_id, window_start)
);
-- Retention sweeps by window_start.
CREATE INDEX IF NOT EXISTS idx_ing_exchange_metrics_window
    ON ing_exchange_metrics (window_start);

-- High volume on a bad day. excerpt is the start of a payload and is only ever
-- set for an ingestion with keepFailedPayloads.
CREATE TABLE IF NOT EXISTS ing_failures (
    id            VARCHAR(17)  PRIMARY KEY,
    ingestion_id  VARCHAR(17)  NOT NULL,
    runtime_id    VARCHAR(200),
    version       INTEGER,
    kind          VARCHAR(50)  NOT NULL,
    message       TEXT         NOT NULL,
    after_records BIGINT       NOT NULL DEFAULT 0,
    excerpt       TEXT,
    occurred_at   TIMESTAMPTZ  NOT NULL,
    created_at    TIMESTAMPTZ  NOT NULL
);
-- The failures tab pages by id, newest first (TSIDs sort by time).
CREATE INDEX IF NOT EXISTS idx_ing_failures_ingestion
    ON ing_failures (ingestion_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_ing_failures_occurred
    ON ing_failures (occurred_at);

-- What each apply of a set did, with the archive of the files it was given
-- (archive_uri is file:// or s3://, as for function artifacts).
CREATE TABLE IF NOT EXISTS ing_set_applies (
    id          VARCHAR(17)  PRIMARY KEY,
    set_id      VARCHAR(17)  NOT NULL REFERENCES ing_sets(id) ON DELETE CASCADE,
    archive_uri TEXT,
    summary     JSONB        NOT NULL DEFAULT '{}'::jsonb,
    applied_by  VARCHAR(17),
    applied_at  TIMESTAMPTZ  NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ing_set_applies_set
    ON ing_set_applies (set_id, applied_at DESC);

-- Pending "test connection" requests, pulled by a runtime that carries the
-- kind. definition is the unsaved or saved definition to try.
CREATE TABLE IF NOT EXISTS ing_test_requests (
    id           VARCHAR(17)  PRIMARY KEY,
    ingestion_id   VARCHAR(17),
    application_id VARCHAR(17)  NOT NULL,
    client_id      VARCHAR(17)  NOT NULL,
    kind         VARCHAR(50)  NOT NULL,
    definition   JSONB        NOT NULL,
    status       VARCHAR(20)  NOT NULL,
    claimed_by   VARCHAR(200),
    result       JSONB,
    requested_by VARCHAR(17),
    created_at   TIMESTAMPTZ  NOT NULL,
    expires_at   TIMESTAMPTZ  NOT NULL,
    completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_ing_test_requests_pending
    ON ing_test_requests (kind, created_at) WHERE status = 'PENDING';
