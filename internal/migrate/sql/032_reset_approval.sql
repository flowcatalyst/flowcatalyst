-- +goose Up
-- Auth hardening (Phase 8): factor-gated password reset + admin approval queue.
-- See docs/auth-hardening-plan.md.

-- A reset token flagged requires_factor can only be confirmed by additionally
-- proving an authenticator (TOTP) code — email alone (incl. email PIN) never
-- authorizes the reset.
ALTER TABLE iam_password_reset_tokens
    ADD COLUMN IF NOT EXISTS requires_factor BOOLEAN NOT NULL DEFAULT FALSE;

-- =============================================================================
-- iam_reset_approval_requests - lost-device reset requests awaiting a client
-- administrator's approval (the user has no strong factor: no TOTP, no passkey).
-- =============================================================================
CREATE TABLE IF NOT EXISTS iam_reset_approval_requests (
    id           VARCHAR(17) PRIMARY KEY,
    principal_id VARCHAR(17) NOT NULL REFERENCES iam_principals(id) ON DELETE CASCADE,
    client_id    VARCHAR(17),
    status       VARCHAR(20) NOT NULL DEFAULT 'PENDING', -- PENDING|APPROVED|DENIED|EXPIRED
    reset_2fa    BOOLEAN NOT NULL DEFAULT TRUE,
    note         VARCHAR(255),
    decided_by   VARCHAR(17),
    decided_at   TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_iam_reset_approval_client_status
    ON iam_reset_approval_requests (client_id, status);
CREATE INDEX IF NOT EXISTS idx_iam_reset_approval_principal
    ON iam_reset_approval_requests (principal_id);
