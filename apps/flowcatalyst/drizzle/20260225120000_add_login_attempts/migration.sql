-- Add iam_login_attempts table for structured auth attempt logging

CREATE TABLE IF NOT EXISTS "iam_login_attempts" (
    "id"             varchar(17)  PRIMARY KEY,
    "attempt_type"   varchar(20)  NOT NULL,
    "outcome"        varchar(20)  NOT NULL,
    "failure_reason" varchar(100),
    "identifier"     varchar(255),
    "principal_id"   varchar(17),
    "ip_address"     varchar(45),
    "user_agent"     text,
    "attempted_at"   timestamptz  NOT NULL
);

CREATE INDEX IF NOT EXISTS "idx_iam_login_attempts_attempted_at" ON "iam_login_attempts" ("attempted_at");
CREATE INDEX IF NOT EXISTS "idx_iam_login_attempts_outcome"       ON "iam_login_attempts" ("outcome");
CREATE INDEX IF NOT EXISTS "idx_iam_login_attempts_identifier"    ON "iam_login_attempts" ("identifier");
CREATE INDEX IF NOT EXISTS "idx_iam_login_attempts_principal_id"  ON "iam_login_attempts" ("principal_id");
