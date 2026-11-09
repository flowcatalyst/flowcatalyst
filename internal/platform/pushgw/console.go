package pushgw

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/framework/go/fw"
	"github.com/flowcatalyst/framework/go/push"
)

// What the console reads (docs/push-gateway-spec.md section 11): the applications that use the
// gateway with what they synced, the recent pushes, and the trace windows that let an operator
// see push data. Every read is of the platform database, so the console works whether or not the
// gateway runs in its own process.

// ConfiguredApp is an application that has push configuration: synced rules or keys, or
// operator settings.
type ConfiguredApp struct {
	ApplicationID string
	Code, Name    string
	Rules, Keys   int
	// SyncedAt is the newest sync of the application's rules or keys (nil: never synced).
	SyncedAt *time.Time
	Paused   bool
	// Overrides counts the limits the operator overrode.
	Overrides int
}

// ListConfigured returns the applications that have push rules, ticket keys or settings, by code.
func (r *ConfigRepo) ListConfigured(ctx context.Context) ([]ConfiguredApp, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT a.id, a.code, a.name,
		       (SELECT COUNT(*) FROM push_rules r WHERE r.application_id = a.id),
		       (SELECT COUNT(*) FROM push_ticket_keys k WHERE k.application_id = a.id),
		       (SELECT MAX(t) FROM (SELECT MAX(synced_at) AS t FROM push_rules WHERE application_id = a.id
		                            UNION ALL SELECT MAX(synced_at) FROM push_ticket_keys WHERE application_id = a.id) m),
		       COALESCE(s.paused, FALSE),
		       (CASE WHEN s.max_streams IS NULL THEN 0 ELSE 1 END + CASE WHEN s.publish_rate IS NULL THEN 0 ELSE 1 END
		        + CASE WHEN s.publish_burst IS NULL THEN 0 ELSE 1 END + CASE WHEN s.max_data_bytes IS NULL THEN 0 ELSE 1 END)
		  FROM app_applications a
		  LEFT JOIN push_app_settings s ON s.application_id = a.id
		 WHERE EXISTS (SELECT 1 FROM push_rules r WHERE r.application_id = a.id)
		    OR EXISTS (SELECT 1 FROM push_ticket_keys k WHERE k.application_id = a.id)
		    OR s.application_id IS NOT NULL
		 ORDER BY a.code`)
	if err != nil {
		return nil, fmt.Errorf("list push applications: %w", err)
	}
	defer rows.Close()
	var out []ConfiguredApp
	for rows.Next() {
		var c ConfiguredApp
		if err := rows.Scan(&c.ApplicationID, &c.Code, &c.Name, &c.Rules, &c.Keys, &c.SyncedAt, &c.Paused, &c.Overrides); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// RuleDetail is a synced rule with its sync time.
type RuleDetail struct {
	Pattern  string
	Requires []string
	Public   bool
	SyncedAt time.Time
}

// RuleDetails returns the application's rules with their sync times, by pattern.
func (r *ConfigRepo) RuleDetails(ctx context.Context, applicationID string) ([]RuleDetail, error) {
	rows, err := r.pool.Query(ctx, `SELECT pattern, requires, public, synced_at FROM push_rules WHERE application_id = $1 ORDER BY pattern`, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RuleDetail
	for rows.Next() {
		var d RuleDetail
		if err := rows.Scan(&d.Pattern, &d.Requires, &d.Public, &d.SyncedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// KeyDetail is a synced ticket key: the kid, its algorithm facts and its sync time. The key
// material itself is public but is not shown.
type KeyDetail struct {
	Kid      string
	Alg      string
	SyncedAt time.Time
}

// KeyDetails returns the application's ticket keys with their sync times, by kid.
func (r *ConfigRepo) KeyDetails(ctx context.Context, applicationID string) ([]KeyDetail, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT kid, COALESCE(jwk->>'crv', '') || CASE WHEN jwk->>'kty' IS NULL THEN '' ELSE ' ' || (jwk->>'kty') END, synced_at
		  FROM push_ticket_keys WHERE application_id = $1 ORDER BY kid`, applicationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []KeyDetail
	for rows.Next() {
		var d KeyDetail
		if err := rows.Scan(&d.Kid, &d.Alg, &d.SyncedAt); err != nil {
			return nil, err
		}
		d.Alg = strings.TrimSpace(d.Alg)
		out = append(out, d)
	}
	return out, rows.Err()
}

// ── the log ───────────────────────────────────────────────────────────

// LogQuery selects recent pushes: those of one application (the first segment of the topic), at
// most Limit, newest first, optionally of one topic substring, type or client, and older than
// BeforeID (0: from the newest).
type LogQuery struct {
	Application string
	// Topic keeps the topics that contain it; Type keeps the exact type; Client the exact client
	// id ("" no filter, "_platform" the platform client).
	Topic, Type, Client string
	BeforeID            int64
	Limit               int
}

// LogRow is one logged push.
type LogRow struct {
	ID          int64
	Topic, Type string
	// Client is the client the push was written for; "" is the platform client.
	Client   string
	Data     string
	LoggedAt time.Time
}

// LogReader reads the push log for the console. The A1 log is a table of the platform database;
// the A2 log is a Redis stream.
type LogReader interface {
	Recent(ctx context.Context, q LogQuery) ([]LogRow, error)
}

// DefaultLogLimit and MaxLogLimit bound a page of recent pushes.
const (
	DefaultLogLimit = 50
	MaxLogLimit     = 200
)

func (q LogQuery) limit() int {
	switch {
	case q.Limit <= 0:
		return DefaultLogLimit
	case q.Limit > MaxLogLimit:
		return MaxLogLimit
	}
	return q.Limit
}

// PGLog reads fw_push_log (the A1 driver) by SQL.
type PGLog struct{ Pool *pgxpool.Pool }

// Recent implements LogReader.
func (l PGLog) Recent(ctx context.Context, q LogQuery) ([]LogRow, error) {
	args := []any{q.Application + ":%"}
	where := []string{`topic LIKE $1`}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if q.Topic != "" {
		add(`position($%d in topic) > 0`, q.Topic)
	}
	if q.Type != "" {
		add(`type = $%d`, q.Type)
	}
	if q.Client != "" {
		if q.Client == fw.PlatformClient.String() {
			where = append(where, `client IS NULL`)
		} else {
			add(`client = $%d`, q.Client)
		}
	}
	if q.BeforeID > 0 {
		add(`id < $%d`, q.BeforeID)
	}
	args = append(args, q.limit())
	rows, err := l.Pool.Query(ctx, `SELECT id, topic, type, client, data, logged_at FROM fw_push_log WHERE `+
		strings.Join(where, " AND ")+fmt.Sprintf(` ORDER BY id DESC LIMIT $%d`, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("read fw_push_log: %w", err)
	}
	defer rows.Close()
	var out []LogRow
	for rows.Next() {
		var r LogRow
		var client *string
		if err := rows.Scan(&r.ID, &r.Topic, &r.Type, &client, &r.Data, &r.LoggedAt); err != nil {
			return nil, err
		}
		if client != nil {
			r.Client = *client
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ModuleLog reads a module log (push.Log): it works for either driver, at the cost of reading the
// newest Scan entries and filtering them in memory (the Redis stream has no query).
type ModuleLog struct {
	Log push.Log
	// Scan is how many of the newest entries a read looks through (default 2,000).
	Scan int
}

// Recent implements LogReader.
func (l ModuleLog) Recent(ctx context.Context, q LogQuery) ([]LogRow, error) {
	scan := l.Scan
	if scan <= 0 {
		scan = 2000
	}
	entries, err := l.Log.Newest(ctx, scan)
	if err != nil {
		return nil, err
	}
	var out []LogRow
	for i := len(entries) - 1; i >= 0 && len(out) < q.limit(); i-- { // newest first
		e := entries[i]
		app, _, _ := strings.Cut(e.Topic, ":")
		switch {
		case app != q.Application,
			q.BeforeID > 0 && e.ID >= q.BeforeID,
			q.Topic != "" && !strings.Contains(e.Topic, q.Topic),
			q.Type != "" && e.Type != q.Type,
			q.Client != "" && !sameClient(e.Client, q.Client):
			continue
		}
		out = append(out, LogRow{ID: e.ID, Topic: e.Topic, Type: e.Type, Client: e.Client, Data: e.Data, LoggedAt: e.LoggedAt})
	}
	return out, nil
}

func sameClient(entry, want string) bool {
	if want == fw.PlatformClient.String() {
		return entry == ""
	}
	return entry == want
}

// ── trace windows ─────────────────────────────────────────────────────

// Trace window lengths an operator may choose: 1, 5 and 15 minutes (as for ingestion traces).
const (
	TraceWindow1m  = 60
	TraceWindow5m  = 300
	TraceWindow15m = 900
	// DefaultTraceWindowSecs applies when none is named.
	DefaultTraceWindowSecs = TraceWindow1m
)

// ValidTraceWindowSecs reports whether n is one of the window lengths.
func ValidTraceWindowSecs(n int) bool {
	return n == TraceWindow1m || n == TraceWindow5m || n == TraceWindow15m
}

// TraceWindow is the window an operator last switched on for an application.
type TraceWindow struct {
	ApplicationID string
	HardStop      time.Time
	WindowSecs    int
	EnabledBy     *string
	EnabledAt     time.Time
}

// Live reports whether the window is open at now.
func (w *TraceWindow) Live(now time.Time) bool { return w != nil && w.HardStop.After(now) }

// TraceRepo stores the trace windows.
type TraceRepo struct{ Pool *pgxpool.Pool }

// Get returns the application's window, nil when it was never switched on.
func (r *TraceRepo) Get(ctx context.Context, applicationID string) (*TraceWindow, error) {
	var w TraceWindow
	err := r.Pool.QueryRow(ctx, `SELECT application_id, hard_stop, window_secs, enabled_by, enabled_at FROM push_trace_windows WHERE application_id = $1`, applicationID).
		Scan(&w.ApplicationID, &w.HardStop, &w.WindowSecs, &w.EnabledBy, &w.EnabledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &w, err
}

// Enable opens a window of secs seconds from now, replacing the application's earlier one.
func (r *TraceRepo) Enable(ctx context.Context, applicationID string, secs int, by string, now time.Time) (*TraceWindow, error) {
	if !ValidTraceWindowSecs(secs) {
		return nil, refuse("TRACE_WINDOW_INVALID", "the window is 60, 300 or 900 seconds")
	}
	w := TraceWindow{ApplicationID: applicationID, HardStop: now.Add(time.Duration(secs) * time.Second).UTC(), WindowSecs: secs, EnabledBy: &by, EnabledAt: now.UTC()}
	_, err := r.Pool.Exec(ctx, `
		INSERT INTO push_trace_windows (application_id, hard_stop, window_secs, enabled_by, enabled_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (application_id) DO UPDATE SET hard_stop = EXCLUDED.hard_stop, window_secs = EXCLUDED.window_secs,
		       enabled_by = EXCLUDED.enabled_by, enabled_at = EXCLUDED.enabled_at`,
		w.ApplicationID, w.HardStop, w.WindowSecs, w.EnabledBy, w.EnabledAt)
	return &w, err
}

// Disable closes the application's window at now; it reports whether one was open.
func (r *TraceRepo) Disable(ctx context.Context, applicationID string, now time.Time) (bool, error) {
	tag, err := r.Pool.Exec(ctx, `UPDATE push_trace_windows SET hard_stop = $2 WHERE application_id = $1 AND hard_stop > $2`, applicationID, now.UTC())
	return tag.RowsAffected() > 0, err
}

// ── test send ─────────────────────────────────────────────────────────

// TestType is the type of the console's test push: fw-test, not fw, which the contract reserves.
const TestType = "fw-test.ping"

// PublishTest publishes {"test":true} of type [TestType] to topic for the client (a client id;
// "" or "_platform" is the platform client) through the module's publish path: one transaction,
// one NOTIFY, the relay and log as for any push. The caller has already checked the operator's
// rights and that the topic's application exists.
func (g *Gateway) PublishTest(ctx context.Context, topic, clientID string) error {
	pv, err := push.New(topic, TestType, []byte(`{"test":true}`))
	if err != nil {
		return err
	}
	cx := fw.ContextFor(fw.PlatformClient)
	if clientID != "" && clientID != fw.PlatformClient.String() {
		cid, err := fw.ParseClientID(clientID)
		if err != nil {
			return fmt.Errorf("the client id is not usable for a push: %w", err)
		}
		cx = fw.ContextFor(cid)
	}
	return g.pushes.PublishBatchAlone(ctx, push.Publication{Context: cx, Push: pv})
}
