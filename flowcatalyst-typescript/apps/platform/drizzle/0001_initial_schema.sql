-- FlowCatalyst Platform Initial Schema
-- Creates tables for principals, clients, and anchor domains

-- Enums
DO $$ BEGIN
    CREATE TYPE principal_type AS ENUM ('USER', 'SERVICE');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    CREATE TYPE user_scope AS ENUM ('ANCHOR', 'PARTNER', 'CLIENT');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    CREATE TYPE idp_type AS ENUM ('INTERNAL', 'OIDC');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    CREATE TYPE client_status AS ENUM ('ACTIVE', 'INACTIVE', 'SUSPENDED');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- Principals table
CREATE TABLE IF NOT EXISTS principals (
    id VARCHAR(13) PRIMARY KEY,
    type principal_type NOT NULL,
    scope user_scope,
    client_id VARCHAR(13),
    application_id VARCHAR(13),
    name VARCHAR(255) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS principals_type_idx ON principals(type);
CREATE INDEX IF NOT EXISTS principals_client_id_idx ON principals(client_id);
CREATE INDEX IF NOT EXISTS principals_active_idx ON principals(active);

-- User identities table
CREATE TABLE IF NOT EXISTS user_identities (
    principal_id VARCHAR(13) PRIMARY KEY REFERENCES principals(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL UNIQUE,
    email_domain VARCHAR(255) NOT NULL,
    idp_type idp_type NOT NULL,
    external_idp_id VARCHAR(255),
    password_hash VARCHAR(255),
    last_login_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS user_identities_email_idx ON user_identities(email);
CREATE INDEX IF NOT EXISTS user_identities_email_domain_idx ON user_identities(email_domain);

-- Role assignments table
CREATE TABLE IF NOT EXISTS role_assignments (
    principal_id VARCHAR(13) NOT NULL REFERENCES principals(id) ON DELETE CASCADE,
    role_name VARCHAR(100) NOT NULL,
    assignment_source VARCHAR(100) NOT NULL,
    assigned_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (principal_id, role_name)
);

CREATE INDEX IF NOT EXISTS role_assignments_principal_id_idx ON role_assignments(principal_id);
CREATE INDEX IF NOT EXISTS role_assignments_role_name_idx ON role_assignments(role_name);

-- Clients table
CREATE TABLE IF NOT EXISTS clients (
    id VARCHAR(13) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    identifier VARCHAR(60) NOT NULL UNIQUE,
    status client_status NOT NULL DEFAULT 'ACTIVE',
    status_reason VARCHAR(100),
    status_changed_at TIMESTAMP WITH TIME ZONE,
    notes JSONB DEFAULT '[]'::jsonb,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS clients_identifier_idx ON clients(identifier);
CREATE INDEX IF NOT EXISTS clients_status_idx ON clients(status);

-- Anchor domains table
CREATE TABLE IF NOT EXISTS anchor_domains (
    id VARCHAR(13) PRIMARY KEY,
    domain VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS anchor_domains_domain_idx ON anchor_domains(domain);

-- Events table (from persistence package, included here for completeness)
CREATE TABLE IF NOT EXISTS events (
    id VARCHAR(13) PRIMARY KEY,
    spec_version VARCHAR(10) NOT NULL,
    type VARCHAR(255) NOT NULL,
    source VARCHAR(255) NOT NULL,
    subject VARCHAR(255) NOT NULL,
    time TIMESTAMP WITH TIME ZONE NOT NULL,
    data JSONB NOT NULL,
    correlation_id VARCHAR(36) NOT NULL,
    causation_id VARCHAR(13),
    deduplication_id VARCHAR(255) NOT NULL UNIQUE,
    message_group VARCHAR(255) NOT NULL,
    client_id VARCHAR(13),
    context_data JSONB
);

CREATE INDEX IF NOT EXISTS events_type_idx ON events(type);
CREATE INDEX IF NOT EXISTS events_subject_idx ON events(subject);
CREATE INDEX IF NOT EXISTS events_time_idx ON events(time);
CREATE INDEX IF NOT EXISTS events_correlation_id_idx ON events(correlation_id);
CREATE INDEX IF NOT EXISTS events_client_id_idx ON events(client_id);

-- Audit logs table (from persistence package)
CREATE TABLE IF NOT EXISTS audit_logs (
    id VARCHAR(13) PRIMARY KEY,
    entity_type VARCHAR(100) NOT NULL,
    entity_id VARCHAR(13) NOT NULL,
    operation VARCHAR(100) NOT NULL,
    operation_json JSONB,
    principal_id VARCHAR(13) NOT NULL,
    performed_at TIMESTAMP WITH TIME ZONE NOT NULL
);

CREATE INDEX IF NOT EXISTS audit_logs_entity_type_idx ON audit_logs(entity_type);
CREATE INDEX IF NOT EXISTS audit_logs_entity_id_idx ON audit_logs(entity_id);
CREATE INDEX IF NOT EXISTS audit_logs_principal_id_idx ON audit_logs(principal_id);
CREATE INDEX IF NOT EXISTS audit_logs_performed_at_idx ON audit_logs(performed_at);
