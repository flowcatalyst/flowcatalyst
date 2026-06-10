-- +goose Up
-- Throttle the factor-gated password-reset confirm: a requires_factor token
-- presents a 6-digit TOTP keyspace against a 15-minute token TTL, and confirm
-- deliberately does not consume the token on a wrong code (so a legitimate
-- user can retry). Without a wrong-guess ceiling that retry allowance is a
-- brute-force window. factor_attempts counts failed factor proofs per token;
-- the confirm handler burns the principal's token set once the ceiling
-- (5, matching the MFA email-PIN cap) is hit.
ALTER TABLE iam_password_reset_tokens
    ADD COLUMN IF NOT EXISTS factor_attempts INT NOT NULL DEFAULT 0;
