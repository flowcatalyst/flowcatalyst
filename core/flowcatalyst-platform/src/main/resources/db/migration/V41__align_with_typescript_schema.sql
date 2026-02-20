-- Widen audit_logs.principal_id to support system identifiers (e.g., "system:oidc-federation")
ALTER TABLE audit_logs ALTER COLUMN principal_id TYPE varchar(100);

-- Add interaction_uid to oidc_login_states for OIDC provider session resumption
ALTER TABLE oidc_login_states ADD COLUMN IF NOT EXISTS interaction_uid varchar(256);
