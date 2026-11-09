-- +goose Up
-- Push gateway: replay protection for the DPoP proofs of field workers' subscribes
-- (docs/push-gateway-spec.md section 5). A proof is accepted once: its (key thumbprint, jti) is
-- recorded here with the instant after which the proof is no longer acceptable by its iat (the
-- field plane's clock-skew window), and a second subscribe with the same proof is refused. The
-- table is shared by every gateway task. Rows past expires_at are pruned by the gateway.
CREATE TABLE push_dpop_proofs (
    proof_hash VARCHAR(64)  PRIMARY KEY,   -- hex sha256 of jkt + NUL + jti
    expires_at TIMESTAMPTZ  NOT NULL
);
CREATE INDEX push_dpop_proofs_expires_at ON push_dpop_proofs (expires_at);

-- +goose Down
DROP TABLE IF EXISTS push_dpop_proofs;
