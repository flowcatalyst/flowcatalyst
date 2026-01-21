-- FlowCatalyst PostgreSQL Schema
-- V1: Initial schema migration from MongoDB

-- =============================================================================
-- Core Tables
-- =============================================================================

-- Clients (organizations/tenants)
CREATE TABLE clients (
    id VARCHAR(13) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    identifier VARCHAR(100) UNIQUE NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'ACTIVE',
    status_reason VARCHAR(255),
    status_changed_at TIMESTAMPTZ,
    notes JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_clients_identifier ON clients(identifier);
CREATE INDEX idx_clients_status ON clients(status);

-- Principals (Users & Service Accounts)
CREATE TABLE principals (
    id VARCHAR(13) PRIMARY KEY,
    type VARCHAR(20) NOT NULL,
    scope VARCHAR(20),
    client_id VARCHAR(13) REFERENCES clients(id) ON DELETE SET NULL,
    application_id VARCHAR(13),
    name VARCHAR(255) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    user_identity JSONB,
    service_account JSONB,
    roles JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_principals_type ON principals(type);
CREATE INDEX idx_principals_client_id ON principals(client_id);
CREATE INDEX idx_principals_active ON principals(active);
CREATE INDEX idx_principals_email ON principals((user_identity->>'email'));

-- Applications
CREATE TABLE applications (
    id VARCHAR(13) PRIMARY KEY,
    code VARCHAR(100) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    type VARCHAR(50) NOT NULL DEFAULT 'APPLICATION',
    default_base_url VARCHAR(500),
    service_account_id VARCHAR(13),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_applications_code ON applications(code);
CREATE INDEX idx_applications_active ON applications(active);

-- Service Accounts (standalone table)
CREATE TABLE service_accounts (
    id VARCHAR(13) PRIMARY KEY,
    code VARCHAR(100) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    client_ids TEXT[],
    application_id VARCHAR(13) REFERENCES applications(id) ON DELETE SET NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    webhook_credentials JSONB,
    roles JSONB DEFAULT '[]',
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_service_accounts_code ON service_accounts(code);
CREATE INDEX idx_service_accounts_application_id ON service_accounts(application_id);

-- =============================================================================
-- Authorization Tables
-- =============================================================================

-- Auth Roles
CREATE TABLE auth_roles (
    id VARCHAR(13) PRIMARY KEY,
    application_id VARCHAR(13),
    application_code VARCHAR(100),
    name VARCHAR(100) NOT NULL,
    display_name VARCHAR(255),
    description TEXT,
    permissions TEXT[],
    source VARCHAR(20) NOT NULL DEFAULT 'DATABASE',
    client_managed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(application_code, name)
);

CREATE INDEX idx_auth_roles_application_code ON auth_roles(application_code);

-- Auth Permissions
CREATE TABLE auth_permissions (
    id VARCHAR(13) PRIMARY KEY,
    application_id VARCHAR(13),
    name VARCHAR(100) NOT NULL,
    display_name VARCHAR(255),
    description TEXT,
    source VARCHAR(20) NOT NULL DEFAULT 'SDK',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_auth_permissions_application_id ON auth_permissions(application_id);

-- =============================================================================
-- Event/Messaging Tables
-- =============================================================================

-- Event Types
CREATE TABLE event_types (
    id VARCHAR(13) PRIMARY KEY,
    code VARCHAR(200) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    spec_versions JSONB DEFAULT '[]',
    status VARCHAR(20) NOT NULL DEFAULT 'CURRENT',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_event_types_code ON event_types(code);
CREATE INDEX idx_event_types_status ON event_types(status);

-- Events
CREATE TABLE events (
    id VARCHAR(13) PRIMARY KEY,
    type VARCHAR(200) NOT NULL,
    source VARCHAR(500) NOT NULL,
    subject VARCHAR(500),
    time TIMESTAMPTZ NOT NULL,
    data JSONB,
    correlation_id VARCHAR(100),
    causation_id VARCHAR(100),
    deduplication_id VARCHAR(100),
    message_group VARCHAR(200),
    client_id VARCHAR(13) REFERENCES clients(id) ON DELETE SET NULL,
    context_data JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_events_type ON events(type);
CREATE INDEX idx_events_client_id ON events(client_id);
CREATE INDEX idx_events_time ON events(time DESC);
CREATE INDEX idx_events_correlation_id ON events(correlation_id);
CREATE INDEX idx_events_deduplication_id ON events(deduplication_id);

-- Events Read (read-optimized projection populated by stream processor)
-- Note: id IS the event id (1:1 projection, no separate event_id needed)
CREATE TABLE events_read (
    id VARCHAR(13) PRIMARY KEY,
    spec_version VARCHAR(20),
    type VARCHAR(200) NOT NULL,
    application VARCHAR(100),
    subdomain VARCHAR(100),
    aggregate VARCHAR(100),
    source VARCHAR(500) NOT NULL,
    subject VARCHAR(500),
    time TIMESTAMPTZ NOT NULL,
    data TEXT,
    message_group VARCHAR(200),
    correlation_id VARCHAR(100),
    causation_id VARCHAR(100),
    deduplication_id VARCHAR(100),
    context_data JSONB DEFAULT '[]',
    client_id VARCHAR(13) REFERENCES clients(id) ON DELETE SET NULL,
    projected_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_events_read_type ON events_read(type);
CREATE INDEX idx_events_read_client_id ON events_read(client_id);
CREATE INDEX idx_events_read_time ON events_read(time DESC);
CREATE INDEX idx_events_read_application ON events_read(application);
CREATE INDEX idx_events_read_subdomain ON events_read(subdomain);
CREATE INDEX idx_events_read_aggregate ON events_read(aggregate);
CREATE INDEX idx_events_read_correlation_id ON events_read(correlation_id);

-- Subscriptions
CREATE TABLE subscriptions (
    id VARCHAR(13) PRIMARY KEY,
    code VARCHAR(100) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    client_id VARCHAR(13) REFERENCES clients(id) ON DELETE CASCADE,
    client_identifier VARCHAR(100),
    event_types JSONB DEFAULT '[]',
    target VARCHAR(500) NOT NULL,
    queue VARCHAR(200),
    custom_config JSONB DEFAULT '[]',
    source VARCHAR(20) NOT NULL DEFAULT 'API',
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
    max_age_seconds INT NOT NULL DEFAULT 86400,
    dispatch_pool_id VARCHAR(13),
    dispatch_pool_code VARCHAR(100),
    delay_seconds INT NOT NULL DEFAULT 0,
    sequence INT NOT NULL DEFAULT 99,
    mode VARCHAR(30) NOT NULL DEFAULT 'IMMEDIATE',
    timeout_seconds INT NOT NULL DEFAULT 30,
    max_retries INT NOT NULL DEFAULT 3,
    service_account_id VARCHAR(13),
    data_only BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(client_id, code)
);

CREATE INDEX idx_subscriptions_client_id ON subscriptions(client_id);
CREATE INDEX idx_subscriptions_status ON subscriptions(status);

-- Dispatch Pools
CREATE TABLE dispatch_pools (
    id VARCHAR(13) PRIMARY KEY,
    code VARCHAR(100) NOT NULL,
    rate_limit INT NOT NULL,
    concurrency INT NOT NULL DEFAULT 1,
    client_id VARCHAR(13) REFERENCES clients(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(client_id, code)
);

CREATE INDEX idx_dispatch_pools_client_id ON dispatch_pools(client_id);

-- Dispatch Jobs
CREATE TABLE dispatch_jobs (
    id VARCHAR(13) PRIMARY KEY,
    external_id VARCHAR(100),
    source VARCHAR(500),
    kind VARCHAR(20) NOT NULL DEFAULT 'EVENT',
    code VARCHAR(200) NOT NULL,
    subject VARCHAR(500),
    event_id VARCHAR(13),
    correlation_id VARCHAR(100),
    metadata JSONB DEFAULT '[]',
    target_url VARCHAR(500) NOT NULL,
    protocol VARCHAR(30) NOT NULL DEFAULT 'HTTP_WEBHOOK',
    payload TEXT,
    payload_content_type VARCHAR(100) DEFAULT 'application/json',
    data_only BOOLEAN NOT NULL DEFAULT TRUE,
    service_account_id VARCHAR(13),
    client_id VARCHAR(13) REFERENCES clients(id) ON DELETE SET NULL,
    subscription_id VARCHAR(13),
    mode VARCHAR(30) NOT NULL DEFAULT 'IMMEDIATE',
    dispatch_pool_id VARCHAR(13),
    message_group VARCHAR(200),
    sequence INT NOT NULL DEFAULT 99,
    timeout_seconds INT NOT NULL DEFAULT 30,
    schema_id VARCHAR(13),
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    max_retries INT NOT NULL DEFAULT 3,
    retry_strategy VARCHAR(50) DEFAULT 'exponential',
    scheduled_for TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    attempt_count INT NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    duration_millis BIGINT,
    last_error TEXT,
    idempotency_key VARCHAR(100),
    attempts JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_dispatch_jobs_status ON dispatch_jobs(status);
CREATE INDEX idx_dispatch_jobs_client_id ON dispatch_jobs(client_id);
CREATE INDEX idx_dispatch_jobs_message_group ON dispatch_jobs(message_group);
CREATE INDEX idx_dispatch_jobs_subscription_id ON dispatch_jobs(subscription_id);
CREATE INDEX idx_dispatch_jobs_created_at ON dispatch_jobs(created_at DESC);
CREATE INDEX idx_dispatch_jobs_scheduled_for ON dispatch_jobs(scheduled_for) WHERE scheduled_for IS NOT NULL;

-- Dispatch Jobs Read (read-optimized projection)
CREATE TABLE dispatch_jobs_read (
    id VARCHAR(13) PRIMARY KEY,
    external_id VARCHAR(100),
    source VARCHAR(500),
    kind VARCHAR(20) NOT NULL,
    code VARCHAR(200) NOT NULL,
    subject VARCHAR(500),
    event_id VARCHAR(13),
    correlation_id VARCHAR(100),
    target_url VARCHAR(500) NOT NULL,
    protocol VARCHAR(30) NOT NULL,
    service_account_id VARCHAR(13),
    client_id VARCHAR(13),
    subscription_id VARCHAR(13),
    mode VARCHAR(30) NOT NULL,
    dispatch_pool_id VARCHAR(13),
    message_group VARCHAR(200),
    status VARCHAR(20) NOT NULL,
    max_retries INT NOT NULL,
    attempt_count INT NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    duration_millis BIGINT,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    -- Parsed fields for efficient queries
    application VARCHAR(100),
    subdomain VARCHAR(100),
    aggregate VARCHAR(100)
);

CREATE INDEX idx_dispatch_jobs_read_status ON dispatch_jobs_read(status);
CREATE INDEX idx_dispatch_jobs_read_client_id ON dispatch_jobs_read(client_id);
CREATE INDEX idx_dispatch_jobs_read_application ON dispatch_jobs_read(application);

-- Schemas
CREATE TABLE schemas (
    id VARCHAR(13) PRIMARY KEY,
    schema_type VARCHAR(20) NOT NULL,
    content TEXT NOT NULL,
    event_type_id VARCHAR(13) REFERENCES event_types(id) ON DELETE CASCADE,
    version VARCHAR(50),
    mime_type VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_schemas_event_type_id ON schemas(event_type_id);

-- =============================================================================
-- Authentication Tables
-- =============================================================================

-- OAuth Clients
CREATE TABLE oauth_clients (
    id VARCHAR(13) PRIMARY KEY,
    client_id VARCHAR(100) UNIQUE NOT NULL,
    client_name VARCHAR(255) NOT NULL,
    client_type VARCHAR(20) NOT NULL DEFAULT 'PUBLIC',
    client_secret_ref VARCHAR(500),
    redirect_uris TEXT[],
    allowed_origins TEXT[],
    grant_types TEXT[],
    default_scopes VARCHAR(500),
    pkce_required BOOLEAN NOT NULL DEFAULT TRUE,
    application_ids TEXT[],
    service_account_principal_id VARCHAR(13),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_oauth_clients_client_id ON oauth_clients(client_id);
CREATE INDEX idx_oauth_clients_active ON oauth_clients(active);

-- Authorization Codes (short-lived)
CREATE TABLE authorization_codes (
    id VARCHAR(13) PRIMARY KEY,
    code VARCHAR(100) UNIQUE NOT NULL,
    client_id VARCHAR(100) NOT NULL,
    principal_id VARCHAR(13) NOT NULL,
    redirect_uri VARCHAR(500) NOT NULL,
    scope VARCHAR(500),
    code_challenge VARCHAR(200),
    code_challenge_method VARCHAR(20),
    nonce VARCHAR(100),
    state VARCHAR(100),
    context_client_id VARCHAR(13),
    used BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_auth_codes_code ON authorization_codes(code);
CREATE INDEX idx_auth_codes_expires ON authorization_codes(expires_at);

-- Refresh Tokens
CREATE TABLE refresh_tokens (
    token_hash VARCHAR(64) PRIMARY KEY,
    principal_id VARCHAR(13) NOT NULL,
    client_id VARCHAR(100) NOT NULL,
    context_client_id VARCHAR(13),
    scope VARCHAR(500),
    token_family VARCHAR(100),
    revoked BOOLEAN NOT NULL DEFAULT FALSE,
    revoked_at TIMESTAMPTZ,
    replaced_by VARCHAR(64),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_refresh_tokens_principal ON refresh_tokens(principal_id);
CREATE INDEX idx_refresh_tokens_expires ON refresh_tokens(expires_at);
CREATE INDEX idx_refresh_tokens_family ON refresh_tokens(token_family);

-- OIDC Login State
CREATE TABLE oidc_login_states (
    state VARCHAR(100) PRIMARY KEY,
    email_domain VARCHAR(255) NOT NULL,
    auth_config_id VARCHAR(13),
    nonce VARCHAR(100),
    code_verifier VARCHAR(200),
    return_url VARCHAR(500),
    oauth_client_id VARCHAR(100),
    oauth_redirect_uri VARCHAR(500),
    oauth_scope VARCHAR(500),
    oauth_state VARCHAR(100),
    oauth_nonce VARCHAR(100),
    oauth_code_challenge VARCHAR(200),
    oauth_code_challenge_method VARCHAR(20),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_oidc_login_states_expires ON oidc_login_states(expires_at);

-- IDP Role Mappings
CREATE TABLE idp_role_mappings (
    id VARCHAR(13) PRIMARY KEY,
    idp_role_name VARCHAR(200) UNIQUE NOT NULL,
    internal_role_name VARCHAR(200) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================================================
-- Configuration Tables
-- =============================================================================

-- Client Auth Config
CREATE TABLE client_auth_configs (
    id VARCHAR(13) PRIMARY KEY,
    email_domain VARCHAR(255) UNIQUE NOT NULL,
    config_type VARCHAR(20) NOT NULL DEFAULT 'CLIENT',
    client_id VARCHAR(13) REFERENCES clients(id) ON DELETE SET NULL,  -- deprecated, for backwards compat
    primary_client_id VARCHAR(13) REFERENCES clients(id) ON DELETE SET NULL,
    additional_client_ids TEXT[],
    granted_client_ids TEXT[],
    auth_provider VARCHAR(20) NOT NULL DEFAULT 'INTERNAL',
    oidc_issuer_url VARCHAR(500),
    oidc_client_id VARCHAR(200),
    oidc_client_secret_ref VARCHAR(500),
    oidc_multi_tenant BOOLEAN DEFAULT FALSE,
    oidc_issuer_pattern VARCHAR(500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_client_auth_configs_email_domain ON client_auth_configs(email_domain);
CREATE INDEX idx_client_auth_configs_config_type ON client_auth_configs(config_type);

-- Application Client Config
CREATE TABLE application_client_configs (
    id VARCHAR(13) PRIMARY KEY,
    application_id VARCHAR(13) NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    client_id VARCHAR(13) NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    base_url_override VARCHAR(500),
    config_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(application_id, client_id)
);

CREATE INDEX idx_app_client_configs_application ON application_client_configs(application_id);
CREATE INDEX idx_app_client_configs_client ON application_client_configs(client_id);

-- Client Access Grants
CREATE TABLE client_access_grants (
    id VARCHAR(13) PRIMARY KEY,
    principal_id VARCHAR(13) NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
    client_id VARCHAR(13) NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ,
    UNIQUE(principal_id, client_id)
);

CREATE INDEX idx_client_access_grants_principal ON client_access_grants(principal_id);
CREATE INDEX idx_client_access_grants_client ON client_access_grants(client_id);

-- Anchor Domains
CREATE TABLE anchor_domains (
    id VARCHAR(13) PRIMARY KEY,
    domain VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- CORS Allowed Origins
CREATE TABLE cors_allowed_origins (
    id VARCHAR(13) PRIMARY KEY,
    origin VARCHAR(500) UNIQUE NOT NULL,
    description VARCHAR(255),
    created_by VARCHAR(13),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============================================================================
-- Audit Tables
-- =============================================================================

-- Audit Logs
CREATE TABLE audit_logs (
    id VARCHAR(13) PRIMARY KEY,
    entity_type VARCHAR(100) NOT NULL,
    entity_id VARCHAR(13) NOT NULL,
    operation VARCHAR(100) NOT NULL,
    operation_json JSONB,
    principal_id VARCHAR(13),
    performed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_logs_entity ON audit_logs(entity_type, entity_id);
CREATE INDEX idx_audit_logs_performed ON audit_logs(performed_at DESC);
CREATE INDEX idx_audit_logs_principal ON audit_logs(principal_id);
