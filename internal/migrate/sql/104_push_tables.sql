-- +goose Up
-- Push gateway (docs/push-gateway-spec.md). Part 1 is the framework push module's own
-- migration (go/push/migrations/V0001__create_push_tables.sql of github.com/flowcatalyst/framework),
-- copied byte for byte: the module is hosted with push.ExternalMigrations(), so this file creates
-- its tables. Keep part 1 identical to the framework file when the module changes.
-- ---- part 1: framework V0001__create_push_tables.sql, verbatim ----
-- Push to clients (push contract sections 2 and 4). fw_push_outbox holds the pushes written in
-- application transactions; the relay moves them, in id order, into fw_push_log, which every
-- instance follows. The log's id is max(id) + 1, assigned by the one relay in its transaction.
CREATE TABLE fw_push_outbox (
    id         BIGSERIAL PRIMARY KEY,
    topic      VARCHAR(255) NOT NULL,
    type       VARCHAR(100) NOT NULL,
    client     VARCHAR(100),             -- NULL: the platform client (section 5)
    data       TEXT NOT NULL,            -- the JSON object as written
    created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE fw_push_log (
    id         BIGINT PRIMARY KEY,
    topic      VARCHAR(255) NOT NULL,
    type       VARCHAR(100) NOT NULL,
    client     VARCHAR(100),
    data       TEXT NOT NULL,
    logged_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX fw_push_log_logged_at ON fw_push_log (logged_at);

-- ---- part 2: the relay lock (framework locks-database V0001__create_lock_table.sql, verbatim) ----
-- The push relay is gated by a lock in the platform database (svc:push-relay).
CREATE SEQUENCE fw_lock_fence;
CREATE TABLE fw_lock (
    lock_key      text        PRIMARY KEY,
    owner_token   text,
    fencing_token bigint      NOT NULL,
    expires_at    timestamptz NOT NULL
);

-- ---- part 3: the gateway's own tables (spec sections 6.2, 8, 10, 11) ----

-- Topic rules an application synced (spec 8): who may subscribe without a ticket.
CREATE TABLE push_rules (
    application_id VARCHAR(17)  NOT NULL REFERENCES app_applications(id) ON DELETE CASCADE,
    pattern        VARCHAR(255) NOT NULL,
    requires       TEXT[]       NOT NULL DEFAULT '{}',
    public         BOOLEAN      NOT NULL DEFAULT FALSE,
    synced_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (application_id, pattern)
);

-- Ed25519 public keys (JWK) an application signs its tickets with (spec 8, 9).
CREATE TABLE push_ticket_keys (
    application_id VARCHAR(17) NOT NULL REFERENCES app_applications(id) ON DELETE CASCADE,
    kid            VARCHAR(64) NOT NULL,
    jwk            JSONB       NOT NULL,
    synced_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (application_id, kid)
);

-- Per-application limit overrides and the pause (spec 10). A NULL limit takes the gateway default.
CREATE TABLE push_app_settings (
    application_id VARCHAR(17) PRIMARY KEY REFERENCES app_applications(id) ON DELETE CASCADE,
    max_streams    INTEGER,
    publish_rate   INTEGER,
    publish_burst  INTEGER,
    max_data_bytes INTEGER,
    paused         BOOLEAN     NOT NULL DEFAULT FALSE,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_by     VARCHAR(17)
);

-- Push ids ingest has accepted, so a retried batch writes nothing twice (spec 6.2). Pruned past
-- push.retention.
CREATE TABLE push_ingest_ids (
    application_id VARCHAR(17) NOT NULL REFERENCES app_applications(id) ON DELETE CASCADE,
    push_id        VARCHAR(64) NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (application_id, push_id)
);
CREATE INDEX push_ingest_ids_created_at ON push_ingest_ids (created_at);

-- One row per gateway task, rewritten every 10 s; the console sums the fresh rows (spec 11).
CREATE TABLE push_gateway_stats (
    task_id VARCHAR(100) PRIMARY KEY,
    at      TIMESTAMPTZ  NOT NULL,
    stats   JSONB        NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS push_gateway_stats;
DROP TABLE IF EXISTS push_ingest_ids;
DROP TABLE IF EXISTS push_app_settings;
DROP TABLE IF EXISTS push_ticket_keys;
DROP TABLE IF EXISTS push_rules;
DROP TABLE IF EXISTS fw_lock;
DROP SEQUENCE IF EXISTS fw_lock_fence;
DROP TABLE IF EXISTS fw_push_log;
DROP TABLE IF EXISTS fw_push_outbox;
