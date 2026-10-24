//go:build integration

package pages

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The raw views against Postgres: they read the write-side tables (a row that
// exists only there is shown, one that exists only in the read projection is
// not), the detail carries what the list does not, and the view-raw
// permissions gate every route.
func TestDebugPagesAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	events := &DebugEvents{Events: event.NewRepository(pool), Shell: shellForTest()}
	jobs := &DebugDispatchJobs{Jobs: dispatchjob.NewRepository(pool), Shell: shellForTest()}

	_, err := pool.Exec(ctx, `INSERT INTO msg_events (id, type, source, subject, time, data, deduplication_id, correlation_id, context_data)
		VALUES ('uidbgev000001', 'uidbg:raw:event:written', 'svc', 'order/1', NOW(), '{"k":"<v>"}', 'uidbg-dedup-1', 'uidbg-corr', '[{"key":"tenant","value":"acme <x>"}]')`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO msg_events_read (id, type, source, subject, time, data, application, subdomain, aggregate, created_at)
		VALUES ('uidbgev000002', 'uidbg:read:event:only', 'svc', 'order/2', NOW(), '{}', 'uidbg', 'read', 'event', NOW())`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO msg_dispatch_jobs (id, code, target_url, status, attempt_count, last_error, payload, created_at, updated_at)
		VALUES ('uidbgdj000001', 'uidbg:raw:job:written', 'http://example.invalid/hook', 'FAILED', 2, 'HTTP 500 <b>', '{"a":"<x>"}', NOW(), NOW())`)
	require.NoError(t, err)

	rawAdmin := cfgUser(permEventViewRaw, permDjViewRaw)
	do := func(page mounter, q req) string {
		t.Helper()
		rec := serveMounted(t, rawAdmin, q, page)
		require.Equal(t, http.StatusOK, rec.Code, "%s: %s", q.target, rec.Body.String())
		return rec.Body.String()
	}

	list := do(events, req{target: "/ui/debug/events?size=1000"})
	assert.Contains(t, list, "uidbgev000001")
	assert.Contains(t, list, "uidbg:raw:event:written")
	assert.NotContains(t, list, "uidbg:read:event:only", "the read projection is the Events page's")
	drawer := do(events, req{target: "/ui/debug/events/uidbgev000001", datastar: true})
	assert.Contains(t, drawer, "uidbg-corr")
	assert.Contains(t, drawer, "&#34;k&#34;: &#34;&lt;v&gt;&#34;")
	assert.Contains(t, drawer, "acme &lt;x&gt;", "the context data is on the raw row")
	assert.Equal(t, http.StatusNotFound, serveMounted(t, rawAdmin, req{target: "/ui/debug/events/uidbgev000002"}, events).Code, "not in the write table")

	jlist := do(jobs, req{target: "/ui/debug/dispatch-jobs?size=1000"})
	assert.Contains(t, jlist, "uidbgdj000001")
	assert.Contains(t, jlist, "uidbg:raw:job:written")
	assert.Contains(t, jlist, "FAILED")
	jdrawer := do(jobs, req{target: "/ui/debug/dispatch-jobs/uidbgdj000001", datastar: true})
	assert.Contains(t, jdrawer, "HTTP 500 &lt;b&gt;")
	assert.Contains(t, jdrawer, "http://example.invalid/hook")

	for _, denied := range []struct {
		page mounter
		path string
	}{
		{events, "/ui/debug/events"},
		{events, "/ui/debug/events/uidbgev000001"},
		{jobs, "/ui/debug/dispatch-jobs"},
		{jobs, "/ui/debug/dispatch-jobs/uidbgdj000001"},
	} {
		// The regular view permissions are the Events and Dispatch Jobs pages'.
		rec := serveMounted(t, cfgUser(permEventView, permDjView), req{target: denied.path}, denied.page)
		assert.Equal(t, http.StatusForbidden, rec.Code, denied.path)
		assert.NotContains(t, rec.Body.String(), "uidbg")
	}
}
