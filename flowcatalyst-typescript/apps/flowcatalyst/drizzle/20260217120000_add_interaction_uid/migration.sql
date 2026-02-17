-- Add interaction_uid column to oauth_oidc_login_states
-- This column stores the oidc-provider interaction UID when login is triggered from an interaction flow
ALTER TABLE "oauth_oidc_login_states" ADD COLUMN IF NOT EXISTS "interaction_uid" varchar(200);
