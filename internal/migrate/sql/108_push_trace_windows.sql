-- +goose Up
-- Push gateway recent-pushes trace (docs/push-gateway-spec.md section 11, console page 3). The
-- list of recent pushes shows their ids, times, clients and types; their data is shown only while
-- an operator has a trace window open for the application. One row per application: the window
-- the operator last switched on (hard_stop is when it ends; a switch-off sets it to the moment).
CREATE TABLE push_trace_windows (
    application_id VARCHAR(17) PRIMARY KEY REFERENCES app_applications(id) ON DELETE CASCADE,
    hard_stop      TIMESTAMPTZ NOT NULL,
    window_secs    INTEGER     NOT NULL,
    enabled_by     VARCHAR(17),
    enabled_at     TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS push_trace_windows;
