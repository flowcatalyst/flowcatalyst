-- +goose Up
-- Push gateway: a field login family that ends closes its open push streams (push contract 7,
-- docs/push-gateway-spec.md section 5). Revocation reaches field_sessions through many paths
-- (logout, PIN change, worker suspend, device revoke, refresh reuse, admin revoke, deletes), some
-- inside use-case transactions; one trigger on the table is the single place that sees them all,
-- from any process, and NOTIFY delivers at commit. A gateway task LISTENs on the channel and ends
-- the family's streams on every task through the push log (Pushes.EndStreams). A rotation (the
-- refresh that replaces a session with its successor) keeps the family alive and is not an end.
-- The payload is the login family id (the stream's "session").
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION push_field_sessions_ended() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        PERFORM pg_notify('fc_push_session_end', ended.family_id)
        FROM (SELECT DISTINCT n.family_id
                FROM new_rows n JOIN old_rows o ON o.id = n.id
               WHERE o.revoked_at IS NULL AND n.revoked_at IS NOT NULL
                 AND n.revoked_reason IS DISTINCT FROM 'ROTATED') ended;
    ELSE
        PERFORM pg_notify('fc_push_session_end', ended.family_id)
        FROM (SELECT DISTINCT o.family_id FROM old_rows o WHERE o.revoked_at IS NULL) ended;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER push_field_sessions_revoked
    AFTER UPDATE ON field_sessions
    REFERENCING OLD TABLE AS old_rows NEW TABLE AS new_rows
    FOR EACH STATEMENT EXECUTE FUNCTION push_field_sessions_ended();

CREATE TRIGGER push_field_sessions_deleted
    AFTER DELETE ON field_sessions
    REFERENCING OLD TABLE AS old_rows
    FOR EACH STATEMENT EXECUTE FUNCTION push_field_sessions_ended();

-- +goose Down
DROP TRIGGER IF EXISTS push_field_sessions_deleted ON field_sessions;
DROP TRIGGER IF EXISTS push_field_sessions_revoked ON field_sessions;
DROP FUNCTION IF EXISTS push_field_sessions_ended();
