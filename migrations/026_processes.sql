-- Process documentation — free-form Mermaid diagrams describing workflows
-- inside an application. Mirrors msg_event_types: code is application:subdomain:process,
-- platform-owned aggregate, status CURRENT/ARCHIVED, source CODE/API/UI.

CREATE TABLE IF NOT EXISTS msg_processes (
    id VARCHAR(17) PRIMARY KEY,
    code VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'CURRENT',
    source VARCHAR(20) NOT NULL DEFAULT 'UI',
    application VARCHAR(100) NOT NULL,
    subdomain VARCHAR(100) NOT NULL,
    process_name VARCHAR(100) NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    diagram_type VARCHAR(20) NOT NULL DEFAULT 'mermaid',
    tags TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_msg_processes_status ON msg_processes (status);
CREATE INDEX IF NOT EXISTS idx_msg_processes_source ON msg_processes (source);
CREATE INDEX IF NOT EXISTS idx_msg_processes_application ON msg_processes (application);
CREATE INDEX IF NOT EXISTS idx_msg_processes_subdomain ON msg_processes (subdomain);
