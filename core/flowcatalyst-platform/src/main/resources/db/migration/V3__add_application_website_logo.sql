-- V3: Add website, icon, and logo fields to applications
-- Adds website URL, icon URL, and embedded logo support for applications and integrations

-- Add icon_url, website and logo columns to applications table
ALTER TABLE applications ADD COLUMN IF NOT EXISTS icon_url VARCHAR(500);
ALTER TABLE applications ADD COLUMN IF NOT EXISTS website VARCHAR(500);
ALTER TABLE applications ADD COLUMN IF NOT EXISTS logo TEXT;
ALTER TABLE applications ADD COLUMN IF NOT EXISTS logo_mime_type VARCHAR(100);

-- Add website override column to application_client_configs table
ALTER TABLE application_client_configs ADD COLUMN IF NOT EXISTS website_override VARCHAR(500);

-- Add comment documentation
COMMENT ON COLUMN applications.icon_url IS 'URL to application icon image';
COMMENT ON COLUMN applications.website IS 'Public website URL for the application (e.g., https://www.yardmanagement.com)';
COMMENT ON COLUMN applications.logo IS 'Embedded logo content (SVG/vector format stored as text)';
COMMENT ON COLUMN applications.logo_mime_type IS 'MIME type of the logo content (e.g., image/svg+xml)';
COMMENT ON COLUMN application_client_configs.website_override IS 'Client-specific website URL override';
