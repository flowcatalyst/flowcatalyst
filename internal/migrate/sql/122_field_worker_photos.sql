-- +goose Up
-- Field-worker photos (docs/field-worker-photo-spec.md section 2).
--
-- field_worker_photos holds one worker's photograph as the two renditions the
-- platform made from the upload (internal/platform/imageproc): never the
-- upload itself, so no metadata of it survives. One row per worker; a replace
-- overwrites it. No foreign key (docs/foreign-keys.md): the worker's delete and
-- a client's switching photos off remove the rows in their own transaction.
--
-- field_identities.photo_version / photo_updated_at are NULL while the worker
-- has no photo, so a list or a sign-in needs no join; only the photo use cases
-- write them, in the same transaction as the row.
--
-- field_policies.photos_enabled (default true) is the client's switch for the
-- whole feature; worker_may_set_own_photo (default false) lets a signed-in
-- worker set and remove their own photo from the device.
CREATE TABLE IF NOT EXISTS field_worker_photos (
    identity_id  VARCHAR(17) PRIMARY KEY,
    client_id    VARCHAR(17) NOT NULL,
    version      CHAR(16)    NOT NULL,
    content_type VARCHAR(20) NOT NULL CHECK (content_type = 'image/jpeg'),
    full_bytes   BYTEA       NOT NULL,
    full_width   INT         NOT NULL,
    full_height  INT         NOT NULL,
    thumb_bytes  BYTEA       NOT NULL,
    thumb_width  INT         NOT NULL,
    thumb_height INT         NOT NULL,
    source_type  VARCHAR(10) NOT NULL CHECK (source_type IN ('jpeg', 'png', 'webp')),
    set_by_type  VARCHAR(20) NOT NULL CHECK (set_by_type IN ('ADMIN', 'SERVICE_ACCOUNT', 'WORKER')),
    set_by       VARCHAR(17) NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_field_worker_photos_client ON field_worker_photos (client_id);

ALTER TABLE field_identities ADD COLUMN IF NOT EXISTS photo_version    CHAR(16);
ALTER TABLE field_identities ADD COLUMN IF NOT EXISTS photo_updated_at TIMESTAMPTZ;

ALTER TABLE field_policies ADD COLUMN IF NOT EXISTS photos_enabled           BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE field_policies ADD COLUMN IF NOT EXISTS worker_may_set_own_photo BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE field_policies DROP COLUMN IF EXISTS worker_may_set_own_photo;
ALTER TABLE field_policies DROP COLUMN IF EXISTS photos_enabled;
ALTER TABLE field_identities DROP COLUMN IF EXISTS photo_updated_at;
ALTER TABLE field_identities DROP COLUMN IF EXISTS photo_version;
DROP TABLE IF EXISTS field_worker_photos;
