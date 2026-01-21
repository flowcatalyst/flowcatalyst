-- Platform configuration storage
-- Supports hierarchical keys (application.section.property) with global and client-scoped values

CREATE TABLE platform_configs (
    id VARCHAR(17) PRIMARY KEY,
    application_code VARCHAR(100) NOT NULL,
    section VARCHAR(100) NOT NULL,
    property VARCHAR(100) NOT NULL,
    scope VARCHAR(20) NOT NULL CHECK (scope IN ('GLOBAL', 'CLIENT')),
    client_id VARCHAR(17),
    value_type VARCHAR(20) NOT NULL CHECK (value_type IN ('PLAIN', 'SECRET')),
    value TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_platform_config_key UNIQUE (application_code, section, property, scope, client_id),
    CONSTRAINT chk_client_scope CHECK (
        (scope = 'GLOBAL' AND client_id IS NULL) OR
        (scope = 'CLIENT' AND client_id IS NOT NULL)
    )
);

-- Index for efficient lookups by application/section with scope filtering
CREATE INDEX idx_platform_configs_lookup ON platform_configs (application_code, section, scope, client_id);
CREATE INDEX idx_platform_configs_app_section ON platform_configs (application_code, section);

-- Role-based access control for application configs
CREATE TABLE platform_config_access (
    id VARCHAR(17) PRIMARY KEY,
    application_code VARCHAR(100) NOT NULL,
    role_code VARCHAR(200) NOT NULL,
    can_read BOOLEAN NOT NULL DEFAULT true,
    can_write BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_config_access_role UNIQUE (application_code, role_code)
);

CREATE INDEX idx_config_access_app ON platform_config_access (application_code);
CREATE INDEX idx_config_access_role ON platform_config_access (role_code);
