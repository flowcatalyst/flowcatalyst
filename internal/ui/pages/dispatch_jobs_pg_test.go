//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The dispatch jobs page end to end against Postgres (the API's reads and
// use cases): the filtered list, the drawer, requeue of ticked rows, and
// requeue-by-filter's preview, changed-count refusal and run.
func TestDispatchJobsPageAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)
	page := &DispatchJobs{
		Jobs: repo, Ops: DispatchJobOps{Repo: repo, UoW: testpg.NewUoW(t)},
		Clients: client.NewRepository(pool), Shell: shellForTest(),
	}
	var seedAged func(id, code, status string, minutesAgo int)
	// The write table and the read projection (no projector runs here).
	seed := func(id, code, status string) { seedAged(id, code, status, 0) }
	seedAged = func(id, code, status string, minutesAgo int) {
		t.Helper()
		_, err := pool.Exec(ctx, `INSERT INTO msg_dispatch_jobs (id, code, target_url, status, attempt_count, last_error, payload, created_at, updated_at)
			VALUES ($1, $2, 'http://example.invalid/hook', $3, 3, 'HTTP 500', '{"n":1,"a":"<x>"}', NOW() - make_interval(mins => $4), NOW())`, id, code, status, minutesAgo)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO msg_dispatch_jobs_read
			(id, code, application, subdomain, aggregate, target_url, kind, protocol, mode, status, max_retries, attempt_count, created_at, updated_at)
			VALUES ($1, $2::text, split_part($2::text, ':', 1), split_part($2::text, ':', 2), split_part($2::text, ':', 3), 'http://example.invalid/hook',
			        'EVENT', 'HTTP_WEBHOOK', 'IMMEDIATE', $3, 3, 3, NOW() - make_interval(mins => $4), NOW())`, id, code, status, minutesAgo)
		require.NoError(t, err)
	}
	seed("djbuipg000001", "uipg:sales:order:created", "FAILED")
	seed("djbuipg000002", "uipg:sales:order:paid", "FAILED")
	seed("djbuipg000003", "uipg:sales:order:paid", "COMPLETED")
	seed("djbuipg000004", "uipgx:sales:order:paid", "FAILED")
	status := func(id string) common.DispatchStatus {
		t.Helper()
		j, err := repo.FindByID(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, j)
		return j.Status
	}
	do := func(q req) string {
		t.Helper()
		rec := serveMounted(t, anchorAdmin(), q, page)
		if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
			t.Fatalf("%s %s: status %d: %s", q.method, q.target, rec.Code, rec.Body.String())
		}
		if q.datastar {
			require.NotContains(t, rec.Body.String(), "fc-flash-error", "%s %s", q.method, q.target)
		}
		return rec.Body.String()
	}

	list := do(req{target: "/ui/dispatch-jobs?applications=uipg&statuses=FAILED"})
	assert.Contains(t, list, "djbuipg000001")
	assert.Contains(t, list, "djbuipg000002")
	assert.NotContains(t, list, "djbuipg000003")
	assert.Contains(t, list, `value="uipg" checked`)

	drawer := do(req{target: "/ui/dispatch-jobs/djbuipg000001", datastar: true})
	assert.Contains(t, drawer, "HTTP 500")
	assert.Contains(t, drawer, "&#34;a&#34;: &#34;&lt;x&gt;&#34;")

	// Ticked rows including a COMPLETED one: the caller may redrive, so the dialog
	// asks first and nothing is reset.
	ticked := url.Values{"ids": {"djbuipg000003", "djbuipg000004"}}
	asked := do(req{method: http.MethodPost, target: "/ui/dispatch-jobs/requeue", datastar: true, form: ticked})
	assert.Contains(t, asked, "of them COMPLETED")
	assert.Equal(t, common.DispatchFailed, status("djbuipg000004"))
	// "Requeue 1 other job": the FAILED one is reset; the COMPLETED one is left alone.
	do(req{method: http.MethodPost, target: "/ui/dispatch-jobs/requeue", datastar: true, form: url.Values{"ids": ticked["ids"], "step": {"plain"}}})
	assert.Equal(t, common.DispatchCompleted, status("djbuipg000003"))
	assert.Equal(t, common.DispatchPending, status("djbuipg000004"))
	// A wrong confirmation redrives nothing; the typed one redrives the COMPLETED job.
	do(req{method: http.MethodPost, target: "/ui/dispatch-jobs/requeue", datastar: true, form: url.Values{"ids": {"djbuipg000003"}, "step": {"redrive"}, "word": {"REDRIVE"}, "count": {"2"}}})
	assert.Equal(t, common.DispatchCompleted, status("djbuipg000003"))
	do(req{method: http.MethodPost, target: "/ui/dispatch-jobs/requeue", datastar: true, form: url.Values{"ids": {"djbuipg000003"}, "step": {"redrive"}, "word": {"REDRIVE"}, "count": {"1"}}})
	assert.Equal(t, common.DispatchPending, status("djbuipg000003"))

	// The Stuck lens: only PENDING and QUEUED jobs older than five minutes.
	seedAged("djbuipg000010", "uipgs:sales:order:paid", "PENDING", 60)
	seedAged("djbuipg000011", "uipgs:sales:order:paid", "PENDING", 1)
	stuck := do(req{target: "/ui/dispatch-jobs?applications=uipgs&statuses=PENDING&statuses=QUEUED&lens=stuck"})
	assert.Contains(t, stuck, "djbuipg000010")
	assert.NotContains(t, stuck, "djbuipg000011")
	assert.Contains(t, do(req{target: "/ui/dispatch-jobs?applications=uipgs&statuses=PENDING&statuses=QUEUED"}), "djbuipg000011")

	// Requeue by filter with a redrive: the typed count covers the eligible and the COMPLETED jobs.
	seed("djbuipg000020", "uipgr:sales:order:paid", "FAILED")
	seed("djbuipg000021", "uipgr:sales:order:paid", "COMPLETED")
	redrive := do(req{target: "/ui/dispatch-jobs/requeue-by-filter?applications=uipgr&step=redrive", datastar: true})
	assert.Contains(t, redrive, `name="confirmCount" value="2"`)
	do(req{method: http.MethodPost, target: "/ui/dispatch-jobs/requeue-by-filter?applications=uipgr", datastar: true, form: url.Values{"confirmCount": {"2"}, "redrive": {"1"}, "word": {"REDRIVE"}, "count": {"2"}}})
	assert.Equal(t, common.DispatchPending, status("djbuipg000020"))
	assert.Equal(t, common.DispatchPending, status("djbuipg000021"))

	preview := do(req{target: "/ui/dispatch-jobs/requeue-by-filter?applications=uipg&statuses=FAILED", datastar: true})
	assert.Contains(t, preview, "2 jobs</strong> match")
	assert.Contains(t, preview, `name="confirmCount" value="2"`)

	changed := do(req{method: http.MethodPost, target: "/ui/dispatch-jobs/requeue-by-filter?applications=uipg&statuses=FAILED", datastar: true, form: url.Values{"confirmCount": {"1"}}})
	assert.Contains(t, changed, "More jobs match than when you looked")
	assert.Equal(t, common.DispatchFailed, status("djbuipg000001"), "a changed count resets nothing")

	run := do(req{method: http.MethodPost, target: "/ui/dispatch-jobs/requeue-by-filter?applications=uipg&statuses=FAILED", datastar: true, form: url.Values{"confirmCount": {"2"}}})
	assert.Contains(t, run, "Requeued 2 jobs")
	assert.Equal(t, common.DispatchPending, status("djbuipg000001"))
	assert.Equal(t, common.DispatchPending, status("djbuipg000002"))

	// Ignore selected: a PENDING, a FAILED and a QUEUED job (in flight) become
	// CANCELLED, a COMPLETED one is left as it is. The drawer of a PENDING job
	// offers it.
	seed("djbuipg000030", "uipgi:sales:order:paid", "PENDING")
	seed("djbuipg000031", "uipgi:sales:order:paid", "FAILED")
	seed("djbuipg000032", "uipgi:sales:order:paid", "QUEUED")
	seed("djbuipg000033", "uipgi:sales:order:paid", "COMPLETED")
	assert.Contains(t, do(req{target: "/ui/dispatch-jobs/djbuipg000030", datastar: true}), "Cancel job")
	ignore := url.Values{"ids": {"djbuipg000030", "djbuipg000031", "djbuipg000032", "djbuipg000033"}}
	asked = do(req{method: http.MethodPost, target: "/ui/dispatch-jobs/cancel", datastar: true, form: ignore})
	assert.Contains(t, asked, "Ignore 3 jobs?")
	assert.Contains(t, asked, "1 of them is in flight")
	assert.Equal(t, common.DispatchPending, status("djbuipg000030"), "asking ignores nothing")
	ignore.Set("step", "confirm")
	done := do(req{method: http.MethodPost, target: "/ui/dispatch-jobs/cancel", datastar: true, form: ignore})
	assert.Contains(t, done, "3 jobs ignored")
	assert.Equal(t, common.DispatchCancelled, status("djbuipg000030"))
	assert.Equal(t, common.DispatchCancelled, status("djbuipg000031"))
	assert.Equal(t, common.DispatchCancelled, status("djbuipg000032"), "a QUEUED job may be ignored")
	assert.Equal(t, common.DispatchCompleted, status("djbuipg000033"))

	// Requeue of a job in flight (owner ruling 2026-10-30): the drawer offers it
	// behind its own confirmation, and only inflight=1 resets it.
	seed("djbuipg000040", "uipgf:sales:order:paid", "PROCESSING")
	drawerIF := do(req{target: "/ui/dispatch-jobs/djbuipg000040", datastar: true})
	assert.Contains(t, drawerIF, `data-action="/ui/dispatch-jobs/djbuipg000040/requeue"`)
	assert.Contains(t, drawerIF, "Cancel job")
	do(req{method: http.MethodPost, target: "/ui/dispatch-jobs/djbuipg000040/requeue", datastar: true, form: url.Values{}})
	assert.Equal(t, common.DispatchProcessing, status("djbuipg000040"), "no consent, no reset")
	flash := do(req{method: http.MethodPost, target: "/ui/dispatch-jobs/djbuipg000040/requeue", datastar: true, form: url.Values{"inflight": {"1"}}})
	assert.Equal(t, common.DispatchPending, status("djbuipg000040"))
	assert.Contains(t, flash, "1 of them was in flight.")
}
