-- Add iam_password_reset_tokens table for self-service password reset

CREATE TABLE IF NOT EXISTS "iam_password_reset_tokens" (
    "id"           varchar(17)  PRIMARY KEY,
    "principal_id" varchar(17)  NOT NULL,
    "token_hash"   varchar(64)  NOT NULL,
    "expires_at"   timestamptz  NOT NULL,
    "created_at"   timestamptz  NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS "idx_prt_token_hash"    ON "iam_password_reset_tokens" ("token_hash");
CREATE        INDEX IF NOT EXISTS "idx_prt_principal_id"  ON "iam_password_reset_tokens" ("principal_id");
