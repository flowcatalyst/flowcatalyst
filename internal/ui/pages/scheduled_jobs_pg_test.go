//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The scheduled jobs page end to end against Postgres (the API's use cases):
// create, edit, pause, resume, fire (a firing appears), archive, delete.
func TestScheduledJobsPageAgainstPostgres(t *testing.T) {
	pool := testpg.Pool(t)
	jobs := scheduledjob.NewRepository(pool)
	ins := scheduledjob.NewInstanceRepository(pool)
	page := &ScheduledJobs{
		Jobs: jobs, Instances: ins, Ops: ScheduledJobOps{Repo: jobs, Instances: ins, UoW: testpg.NewUoW(t)},
		Clients: client.NewRepository(pool), Apps: application.NewRepository(pool), Shell: shellForTest(),
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

	require.Contains(t, do(req{method: http.MethodPost, target: "/ui/scheduled-jobs/new", datastar: true, form: url.Values{
		"code": {"ui-pg-job"}, "name": {"UI PG job"}, "scope": {"platform"}, "timezone": {"UTC"}, "crons": {"0 0 2 * * *"},
		"targetUrl": {"https://example.com/process"}, "deliveryMaxAttempts": {"3"}, "payload": {`{"k":"v"}`},
	}}), "created")
	j, err := jobs.FindByCode(context.Background(), "ui-pg-job", nil)
	require.NoError(t, err)
	require.NotNil(t, j)
	assert.JSONEq(t, `{"k":"v"}`, string(j.Payload))
	base := "/ui/scheduled-jobs/" + j.ID

	do(req{method: http.MethodPost, target: base + "/edit", datastar: true, form: url.Values{"name": {"UI PG job 2"}, "crons": {"0 0 3 * * *\n0 0 4 * * *"}, "deliveryMaxAttempts": {"5"}}})
	got, _ := jobs.FindByID(context.Background(), j.ID)
	assert.Equal(t, "UI PG job 2", got.Name)
	assert.Equal(t, []string{"0 0 3 * * *", "0 0 4 * * *"}, got.Crons)
	assert.Equal(t, int32(5), got.DeliveryMaxAttempts)

	do(req{method: http.MethodPost, target: base + "/pause", datastar: true, form: url.Values{}})
	do(req{method: http.MethodPost, target: base + "/resume", datastar: true, form: url.Values{}})
	fired := do(req{method: http.MethodPost, target: base + "/fire", datastar: true, form: url.Values{}})
	assert.Contains(t, fired, "Instance ")
	assert.Contains(t, do(req{target: base + "/instances"}), "MANUAL")
	do(req{method: http.MethodPost, target: base + "/archive", datastar: true, form: url.Values{}})
	got, _ = jobs.FindByID(context.Background(), j.ID)
	assert.Equal(t, scheduledjob.StatusArchived, got.Status)
	do(req{method: http.MethodPost, target: base + "/delete", datastar: true, form: url.Values{}})
	gone, err := jobs.FindByID(context.Background(), j.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)
}
