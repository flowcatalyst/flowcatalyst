-- 0006: Dispatch jobs and outbox infrastructure
--
-- Creates tables for:
-- 1. Dispatch jobs (webhook/event delivery tasks)
-- 2. Dispatch job attempts (normalized from JSONB)
-- 3. Read model projections (events_read, dispatch_jobs_read)
-- 4. Outbox tables for CQRS change capture (event_outbox, dispatch_job_outbox)
--
-- Note: metadata is stored as JSONB on dispatch_jobs table
-- Note: headers are NOT stored (calculated at dispatch time)

-- =============================================================================
-- 1. Dispatch Jobs Table
-- =============================================================================

CREATE TABLE IF NOT EXISTS dispatch_jobs (
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
    data_only BOOLEAN NOT NULL DEFAULT true,
    service_account_id VARCHAR(17),
    client_id VARCHAR(17),
    subscription_id VARCHAR(17),
    mode VARCHAR(30) NOT NULL DEFAULT 'IMMEDIATE',
    dispatch_pool_id VARCHAR(17),
    message_group VARCHAR(200),
    sequence INTEGER NOT NULL DEFAULT 99,
    timeout_seconds INTEGER NOT NULL DEFAULT 30,
    schema_id VARCHAR(17),
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    max_retries INTEGER NOT NULL DEFAULT 3,
    retry_strategy VARCHAR(50) DEFAULT 'exponential',
    scheduled_for TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    duration_millis BIGINT,
    last_error TEXT,
    idempotency_key VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_status ON dispatch_jobs(status);
CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_client_id ON dispatch_jobs(client_id);
CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_message_group ON dispatch_jobs(message_group);
CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_subscription_id ON dispatch_jobs(subscription_id);
CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_created_at ON dispatch_jobs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_scheduled_for ON dispatch_jobs(scheduled_for) WHERE scheduled_for IS NOT NULL;

-- =============================================================================
-- 2. Dispatch Job Attempts Table (normalized from JSONB)
-- =============================================================================

CREATE TABLE IF NOT EXISTS dispatch_job_attempts (
    id VARCHAR(17) PRIMARY KEY,
    dispatch_job_id VARCHAR(13) NOT NULL,
    attempt_number INTEGER,
    status VARCHAR(20),
    response_code INTEGER,
    response_body TEXT,
    error_message TEXT,
    error_stack_trace TEXT,
    error_type VARCHAR(20),
    duration_millis BIGINT,
    attempted_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_dispatch_job_attempts_job_number ON dispatch_job_attempts(dispatch_job_id, attempt_number);
CREATE INDEX IF NOT EXISTS idx_dispatch_job_attempts_job ON dispatch_job_attempts(dispatch_job_id);

-- =============================================================================
-- 3. Events Read Table (CQRS projection)
-- =============================================================================
-- Note: id IS the event id (1:1 projection, no separate event_id needed)

CREATE TABLE IF NOT EXISTS events_read (
    id VARCHAR(13) PRIMARY KEY,
    spec_version VARCHAR(20),
    type VARCHAR(200) NOT NULL,
    source VARCHAR(500) NOT NULL,
    subject VARCHAR(500),
    time TIMESTAMP WITH TIME ZONE NOT NULL,
    data TEXT,
    correlation_id VARCHAR(100),
    causation_id VARCHAR(100),
    deduplication_id VARCHAR(200),
    message_group VARCHAR(200),
    client_id VARCHAR(17),
    application VARCHAR(100),
    subdomain VARCHAR(100),
    aggregate VARCHAR(100),
    projected_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_events_read_type ON events_read(type);
CREATE INDEX IF NOT EXISTS idx_events_read_client_id ON events_read(client_id);
CREATE INDEX IF NOT EXISTS idx_events_read_time ON events_read(time DESC);
CREATE INDEX IF NOT EXISTS idx_events_read_application ON events_read(application);
CREATE INDEX IF NOT EXISTS idx_events_read_subdomain ON events_read(subdomain);
CREATE INDEX IF NOT EXISTS idx_events_read_aggregate ON events_read(aggregate);
CREATE INDEX IF NOT EXISTS idx_events_read_correlation_id ON events_read(correlation_id);

-- =============================================================================
-- 4. Dispatch Jobs Read Table (CQRS projection)
-- =============================================================================

CREATE TABLE IF NOT EXISTS dispatch_jobs_read (
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
    service_account_id VARCHAR(17),
    client_id VARCHAR(17),
    subscription_id VARCHAR(17),
    dispatch_pool_id VARCHAR(17),
    mode VARCHAR(30) NOT NULL,
    message_group VARCHAR(200),
    sequence INTEGER DEFAULT 99,
    timeout_seconds INTEGER DEFAULT 30,
    status VARCHAR(20) NOT NULL,
    max_retries INTEGER NOT NULL,
    retry_strategy VARCHAR(50),
    scheduled_for TIMESTAMP WITH TIME ZONE,
    expires_at TIMESTAMP WITH TIME ZONE,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    last_attempt_at TIMESTAMP WITH TIME ZONE,
    completed_at TIMESTAMP WITH TIME ZONE,
    duration_millis BIGINT,
    last_error TEXT,
    idempotency_key VARCHAR(100),
    is_completed BOOLEAN,
    is_terminal BOOLEAN,
    application VARCHAR(100),
    subdomain VARCHAR(100),
    aggregate VARCHAR(100),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    projected_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_read_status ON dispatch_jobs_read(status);
CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_read_client_id ON dispatch_jobs_read(client_id);
CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_read_application ON dispatch_jobs_read(application);
CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_read_subscription_id ON dispatch_jobs_read(subscription_id);
CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_read_message_group ON dispatch_jobs_read(message_group);
CREATE INDEX IF NOT EXISTS idx_dispatch_jobs_read_created_at ON dispatch_jobs_read(created_at DESC);

-- =============================================================================
-- 5. Event Outbox Table (CQRS change capture)
-- =============================================================================
-- Processed values (following postbox-processor pattern):
--   0 = pending
--   1 = success
--   2 = bad request (permanent failure)
--   3 = server error (retriable)
--   9 = in-progress (crash recovery marker)

CREATE TABLE IF NOT EXISTS event_outbox (
    id BIGSERIAL PRIMARY KEY,
    event_id VARCHAR(13) NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    processed SMALLINT NOT NULL DEFAULT 0,
    processed_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_event_outbox_unprocessed ON event_outbox(id) WHERE processed = 0;
CREATE INDEX IF NOT EXISTS idx_event_outbox_in_progress ON event_outbox(id) WHERE processed = 9;

-- =============================================================================
-- 6. Dispatch Job Outbox Table (CQRS change capture)
-- =============================================================================
-- Uses dispatch_job_id as message group for sequencing
-- Operation: INSERT=full payload, UPDATE=patch, DELETE=tombstone

CREATE TABLE IF NOT EXISTS dispatch_job_outbox (
    id BIGSERIAL PRIMARY KEY,
    dispatch_job_id VARCHAR(13) NOT NULL,
    operation VARCHAR(10) NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    processed SMALLINT NOT NULL DEFAULT 0,
    processed_at TIMESTAMP WITH TIME ZONE,
    error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_dispatch_job_outbox_unprocessed ON dispatch_job_outbox(dispatch_job_id, id) WHERE processed = 0;
CREATE INDEX IF NOT EXISTS idx_dispatch_job_outbox_in_progress ON dispatch_job_outbox(id) WHERE processed = 9;
CREATE INDEX IF NOT EXISTS idx_dispatch_job_outbox_processed_at ON dispatch_job_outbox(processed_at) WHERE processed = 1;

-- =============================================================================
-- 7. Comments for documentation
-- =============================================================================

COMMENT ON TABLE dispatch_jobs IS 'Main dispatch job storage for webhook/event delivery tasks. Metadata stored as JSONB, headers calculated at dispatch time.';
COMMENT ON COLUMN dispatch_jobs.metadata IS 'Key-value metadata as JSON array: [{"key": "...", "value": "..."}, ...]';
COMMENT ON TABLE dispatch_job_attempts IS 'Delivery attempt history for dispatch jobs';
COMMENT ON TABLE events_read IS 'Read-optimized projection of events, populated from event_outbox. id IS the event id (1:1 projection).';
COMMENT ON TABLE dispatch_jobs_read IS 'Read-optimized projection of dispatch_jobs, populated from dispatch_job_outbox';
COMMENT ON TABLE event_outbox IS 'Outbox for CQRS projection of events to events_read';
COMMENT ON COLUMN event_outbox.processed IS '0=pending, 1=success, 2=bad_request, 3=server_error, 9=in_progress';
COMMENT ON TABLE dispatch_job_outbox IS 'Outbox for CQRS projection of dispatch_jobs to dispatch_jobs_read. Uses dispatch_job_id as message group.';
COMMENT ON COLUMN dispatch_job_outbox.processed IS '0=pending, 1=success, 2=bad_request, 3=server_error, 9=in_progress';
COMMENT ON COLUMN dispatch_job_outbox.operation IS 'INSERT=full payload, UPDATE=patch with changed fields, DELETE=tombstone';
