-- +goose Up
-- Publishing a service account's or OAuth client's credentials to an external
-- store (docs/credential-publishing.md). A copy, never a replacement: the
-- credentials stay where they are; the platform also writes each one to a
-- Parameter Store parameter so a receiving application reads it from there.
--
--   * iam_credential_publications: one row per owner that has publishing
--     switched on (owner_type SERVICE_ACCOUNT | OAUTH_CLIENT, owner_id the
--     sac_/oac_ row id). base_path is the part of the parameter name before the
--     credential name, fixed once publishing is on.
--   * iam_credential_publication_items: one row per credential of such an owner,
--     the work list of the background publisher. state is PENDING_PUT,
--     PUBLISHED, PENDING_DELETE or NOT_PUBLISHABLE (a client secret that is not
--     known: its plaintext exists only in the operation that mints it).
--     pending_value holds that plaintext, encrypted with the app key, until the
--     parameter is written. generation is bumped on every queueing, so the
--     publisher can tell that an item was re-queued while it was calling AWS and
--     must not record its older outcome over the newer request.
--
-- No foreign keys (docs/foreign-keys.md: new tables get none by default), and
-- on purpose: a deleted owner's items must outlive it until their parameters
-- have been deleted; the publisher then removes the rows.
--
-- Cost (docs/large-table-schema-changes.md does not apply): two new small
-- tables, one row per owner that opts in and at most four per owner.
SET LOCAL lock_timeout = '5s';

CREATE TABLE IF NOT EXISTS iam_credential_publications (
    owner_type VARCHAR(20)  NOT NULL,
    owner_id   VARCHAR(17)  NOT NULL,
    store      VARCHAR(20)  NOT NULL,
    base_path  TEXT         NOT NULL,
    enabled_at TIMESTAMPTZ  NOT NULL,
    enabled_by TEXT,
    PRIMARY KEY (owner_type, owner_id)
);

CREATE TABLE IF NOT EXISTS iam_credential_publication_items (
    owner_type      VARCHAR(20) NOT NULL,
    owner_id        VARCHAR(17) NOT NULL,
    credential      VARCHAR(30) NOT NULL,
    path            TEXT        NOT NULL,
    state           VARCHAR(20) NOT NULL,
    pending_value   TEXT,
    attempts        INT         NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    last_error      TEXT,
    published_at    TIMESTAMPTZ,
    generation      BIGINT      NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (owner_type, owner_id, credential)
);

CREATE INDEX IF NOT EXISTS iam_credential_publication_items_due
    ON iam_credential_publication_items (next_attempt_at)
    WHERE state IN ('PENDING_PUT', 'PENDING_DELETE');

-- +goose Down
DROP TABLE IF EXISTS iam_credential_publication_items;
DROP TABLE IF EXISTS iam_credential_publications;
