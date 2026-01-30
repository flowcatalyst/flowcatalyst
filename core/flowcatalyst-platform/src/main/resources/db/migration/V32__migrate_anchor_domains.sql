-- V32: Migrate anchor_domains to email_domain_mappings with scopeType=ANCHOR
-- This migration completes the consolidation of anchor domain configuration into EmailDomainMapping

-- Step 1: Ensure internal identity provider exists
-- Uses a generated TSID-style ID (edm prefix + 13 chars = 17 total for entity IDs, but idp prefix)
INSERT INTO identity_providers (id, code, name, type, created_at, updated_at)
SELECT 'idp_internal001', 'internal', 'Internal Authentication', 'INTERNAL', NOW(), NOW()
WHERE NOT EXISTS (SELECT 1 FROM identity_providers WHERE code = 'internal');

-- Step 2: Migrate anchor_domains to email_domain_mappings
-- For each anchor domain, create an email_domain_mapping with:
--   - scopeType = 'ANCHOR'
--   - identity_provider_id = the internal IDP
--   - No primary client (anchor users have global access)
INSERT INTO email_domain_mappings (
    id,
    email_domain,
    identity_provider_id,
    scope_type,
    primary_client_id,
    additional_client_ids,
    granted_client_ids,
    created_at,
    updated_at
)
SELECT
    'edm' || SUBSTRING(ad.id FROM 4),  -- Convert id prefix from old to 'edm'
    LOWER(ad.domain),
    (SELECT id FROM identity_providers WHERE code = 'internal'),
    'ANCHOR',
    NULL,  -- No primary client for anchor users
    NULL,  -- No additional clients (they have global access)
    NULL,  -- No granted clients (they have global access)
    ad.created_at,
    NOW()
FROM anchor_domains ad
WHERE NOT EXISTS (
    SELECT 1 FROM email_domain_mappings edm
    WHERE LOWER(edm.email_domain) = LOWER(ad.domain)
);

-- Step 3: Log migration summary (will show in migration output)
DO $$
DECLARE
    migrated_count INTEGER;
    skipped_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO migrated_count
    FROM email_domain_mappings
    WHERE scope_type = 'ANCHOR';

    SELECT COUNT(*) INTO skipped_count
    FROM anchor_domains ad
    WHERE EXISTS (
        SELECT 1 FROM email_domain_mappings edm
        WHERE LOWER(edm.email_domain) = LOWER(ad.domain)
        AND edm.scope_type != 'ANCHOR'
    );

    RAISE NOTICE 'Anchor domain migration: % domains migrated, % skipped (already mapped with different scope)',
        migrated_count, skipped_count;
END $$;

-- Note: The anchor_domains table is kept for now for rollback safety.
-- It can be dropped in a future migration (V33+) after verification.
-- DROP TABLE anchor_domains; -- Uncomment in future migration
