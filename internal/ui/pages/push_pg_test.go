//go:build integration

package pages

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/pushgw"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/sdksync"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

type recordingPublisher struct {
	available bool
	sent      []string
}

func (p *recordingPublisher) Available() bool { return p.available }
func (p *recordingPublisher) PublishTest(_ context.Context, topic, clientID string) error {
	p.sent = append(p.sent, topic+"|"+clientID)
	return nil
}

func pushAuditOps(t *testing.T, appID string) []string {
	t.Helper()
	rows, err := testpg.Pool(t).Query(context.Background(), `SELECT operation FROM aud_logs WHERE entity_id = $1 ORDER BY performed_at, id`, appID)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var op string
		require.NoError(t, rows.Scan(&op))
		out = append(out, op)
	}
	return out
}

// The Push pages end to end against Postgres: the real settings operation (anchor + manage,
// validation, audit), the real tables for rules, keys, stats, the log and the trace window.
func TestPushConsoleAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	apps := application.NewRepository(pool)
	appID := string(ids.NewApplicationID())
	_, err := pool.Exec(ctx, `INSERT INTO app_applications (id, code, name) VALUES ($1, 'pushui', 'Push UI app')`, appID)
	require.NoError(t, err)

	cfg := pushgw.NewConfigRepo(pool)
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	jwk, _ := json.Marshal(map[string]string{"kty": "OKP", "crv": "Ed25519", "x": base64.RawURLEncoding.EncodeToString(pub), "kid": "kid1"})
	rules := []pushgw.SyncRule{{Pattern: "pushui:orders:{principal}"}, {Pattern: "pushui:news", Public: true}}
	require.NoError(t, pushgw.ValidateSync("pushui", rules, []pushgw.SyncKey{{Kid: "kid1", JWK: jwk}}))
	_, err = cfg.ReplaceRulesAndKeys(ctx, appID, rules, []pushgw.SyncKey{{Kid: "kid1", JWK: jwk}})
	require.NoError(t, err)

	auditRepo := audit.NewRepository(pool)
	publisher := &recordingPublisher{available: true}
	page := &PushConsole{
		Config: cfg, Settings: sdksync.PushSettingsOps{Push: cfg, Audit: auditRepo}, Apps: apps, Clients: client.NewRepository(pool),
		Stats: &pushgw.StatsRepo{Pool: pool}, Log: pushgw.PGLog{Pool: pool}, Traces: &pushgw.TraceRepo{Pool: pool}, Publisher: publisher,
		Audit: func(ctx context.Context, ac *auth.AuthContext, appID, op string, detail map[string]any) {
			sdksync.AuditPushChange(ctx, auditRepo, ac, appID, op, detail)
		},
		Shell: shellForTest(),
	}
	manager := connUser(pushViewPermTest, pushManagePermTest)
	do := func(ac *auth.AuthContext, q req) string {
		t.Helper()
		rec := serveMounted(t, ac, q, page)
		require.Contains(t, []int{http.StatusOK, http.StatusSeeOther}, rec.Code, "%s %s: %s", q.method, q.target, rec.Body.String())
		return rec.Body.String()
	}

	// the list and the detail read the synced rules and keys
	mustContain(t, do(manager, req{target: "/ui/push/applications"}), "Push UI app", "pushui", "Defaults", "Accepting")
	detail := do(manager, req{target: "/ui/push/applications/pushui"})
	mustContain(t, detail, "pushui:orders:{principal}", "pushui:news", "Public", "kid1", "Ed25519 OKP", "Effective 20,000")

	// limits: the settings operation writes the row, notifies and audits
	body := do(manager, post("/ui/push/applications/pushui/limits", url.Values{"maxStreams": {"750"}, "publishRate": {"50"}, "publishBurst": {"200"}, "maxDataBytes": {""}}))
	mustContain(t, body, "Push limits saved", `value="750"`, "Effective 750")
	st, err := cfg.Settings(ctx, appID)
	require.NoError(t, err)
	require.NotNil(t, st.MaxStreams)
	assert.Equal(t, 750, *st.MaxStreams)
	assert.Equal(t, 200, *st.PublishBurst)
	assert.Nil(t, st.MaxDataBytes)
	assert.False(t, st.Paused)
	assert.Equal(t, []string{"PUSH_SETTINGS_CHANGED"}, pushAuditOps(t, appID))

	// a refused edit stores and audits nothing
	bad := do(manager, post("/ui/push/applications/pushui/limits", url.Values{"publishRate": {"500"}, "publishBurst": {"10"}}))
	mustContain(t, bad, "must be at least")
	st, _ = cfg.Settings(ctx, appID)
	assert.Equal(t, 750, *st.MaxStreams, "unchanged")
	assert.Len(t, pushAuditOps(t, appID), 1)

	// the pause keeps the overrides and is audited through the same operation
	do(manager, post("/ui/push/applications/pushui/pause", url.Values{"paused": {"true"}}))
	st, _ = cfg.Settings(ctx, appID)
	assert.True(t, st.Paused)
	assert.Equal(t, 750, *st.MaxStreams)
	mustContain(t, do(manager, req{target: "/ui/push/applications"}), "Paused")
	do(manager, post("/ui/push/applications/pushui/pause", url.Values{"paused": {"false"}}))
	st, _ = cfg.Settings(ctx, appID)
	assert.False(t, st.Paused)
	assert.Equal(t, []string{"PUSH_SETTINGS_CHANGED", "PUSH_SETTINGS_CHANGED", "PUSH_SETTINGS_CHANGED"}, pushAuditOps(t, appID))

	// forged posts: a viewer, and a client-scoped user with the manage permission
	viewer := connUser(pushViewPermTest)
	clientScoped := &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_x"}, Roles: []string{"r"}, Permissions: []string{pushManagePermTest}, PrincipalType: auth.PrincipalTypeUser}
	for _, ac := range []*auth.AuthContext{viewer, clientScoped} {
		rec := serveMounted(t, ac, post("/ui/push/applications/pushui/pause", url.Values{"paused": {"true"}}), page)
		mustContain(t, rec.Body.String(), "fc-flash-error")
	}
	st, _ = cfg.Settings(ctx, appID)
	assert.False(t, st.Paused)
	assert.Len(t, pushAuditOps(t, appID), 3)

	// recent pushes: the data is behind the trace window, read from the log table
	_, err = pool.Exec(ctx, `INSERT INTO fw_push_log (id, topic, type, client, data, logged_at) VALUES ($1, 'pushui:orders:1', 'order.updated', NULL, $2, NOW())`,
		time.Now().UnixNano(), `{"secret":"pg-customer-data"}`)
	require.NoError(t, err)
	hidden := do(viewer, req{target: "/ui/push/recent?app=pushui"})
	mustContain(t, hidden, "order.updated", "pushui:orders:1", "hidden")
	mustNotContain(t, hidden, "pg-customer-data")
	do(manager, post("/ui/push/recent/trace/enable", url.Values{"app": {"pushui"}, "windowSecs": {"60"}}))
	win, err := (&pushgw.TraceRepo{Pool: pool}).Get(ctx, appID)
	require.NoError(t, err)
	require.NotNil(t, win)
	assert.True(t, win.Live(time.Now()))
	mustContain(t, do(viewer, req{target: "/ui/push/recent?app=pushui"}), "pg-customer-data")
	assert.Contains(t, pushAuditOps(t, appID), "PUSH_TRACE_ENABLED")
	do(manager, post("/ui/push/recent/trace/disable", url.Values{"app": {"pushui"}}))
	mustNotContain(t, do(viewer, req{target: "/ui/push/recent?app=pushui"}), "pg-customer-data")
	assert.Contains(t, pushAuditOps(t, appID), "PUSH_TRACE_DISABLED")

	// test send: the publisher gets it, the audit records it against the application
	ok := do(manager, post("/ui/push/test", url.Values{"topic": {"pushui:orders:1"}, "client": {"_platform"}}))
	mustContain(t, ok, "Test push sent")
	assert.Equal(t, []string{"pushui:orders:1|_platform"}, publisher.sent)
	assert.Contains(t, pushAuditOps(t, appID), "PUSH_TEST_SENT")
	mustContain(t, do(manager, post("/ui/push/test", url.Values{"topic": {"nopush:a:b"}, "client": {"_platform"}})), "No application has the code nopush.")
	assert.Len(t, publisher.sent, 1)

	// live: the stats rows the tasks wrote
	require.NoError(t, pushgw.WriteStats(ctx, pool, "ui-pg-task", pushgw.TaskStats{
		Driver: "postgres", Open: 3, ByApplication: map[string]int{"pushui": 3}, ByClient: map[string]int{"_platform": 3},
		Lag: pushgw.Histogram{Bounds: []float64{0.01, 0.1}, Counts: []uint64{4, 0, 0}},
	}, time.Now()))
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM push_gateway_stats WHERE task_id = 'ui-pg-task'`) })
	mustContain(t, do(viewer, req{target: "/ui/push/live"}), "pushui", "log driver: postgres", "Platform")
}
