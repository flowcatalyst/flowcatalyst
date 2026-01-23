-- V21: Change description columns from TEXT to VARCHAR(255)
--
-- TEXT columns don't have length limits. Using VARCHAR(255) enforces a reasonable
-- limit and is more consistent with other string columns in the schema.

-- Truncate any existing descriptions longer than 255 characters
UPDATE applications SET description = LEFT(description, 255) WHERE LENGTH(description) > 255;
UPDATE service_accounts SET description = LEFT(description, 255) WHERE LENGTH(description) > 255;
UPDATE auth_roles SET description = LEFT(description, 255) WHERE LENGTH(description) > 255;
UPDATE auth_permissions SET description = LEFT(description, 255) WHERE LENGTH(description) > 255;
UPDATE event_types SET description = LEFT(description, 255) WHERE LENGTH(description) > 255;
UPDATE subscriptions SET description = LEFT(description, 255) WHERE LENGTH(description) > 255;

-- Alter column types
ALTER TABLE applications ALTER COLUMN description TYPE VARCHAR(255);
ALTER TABLE service_accounts ALTER COLUMN description TYPE VARCHAR(255);
ALTER TABLE auth_roles ALTER COLUMN description TYPE VARCHAR(255);
ALTER TABLE auth_permissions ALTER COLUMN description TYPE VARCHAR(255);
ALTER TABLE event_types ALTER COLUMN description TYPE VARCHAR(255);
ALTER TABLE subscriptions ALTER COLUMN description TYPE VARCHAR(255);
