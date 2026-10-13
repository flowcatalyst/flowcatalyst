package pages

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	sjops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

type fakeJobs struct {
	jobs    []scheduledjob.ScheduledJob
	filters []scheduledjob.ListFilters
}

func (f *fakeJobs) match(fl scheduledjob.ListFilters) []scheduledjob.ScheduledJob {
	var out []scheduledjob.ScheduledJob
	for _, j := range f.jobs {
		if len(fl.Statuses) > 0 && !slices.Contains(fl.Statuses, string(j.Status)) {
			continue
		}
		if len(fl.ClientIDs) > 0 {
			id := "platform"
			if j.ClientID != nil {
				id = string(*j.ClientID)
			}
			if !slices.Contains(fl.ClientIDs, id) {
				continue
			}
		}
		if fl.Search != nil && !strings.Contains(j.Code+j.Name, *fl.Search) {
			continue
		}
		out = append(out, j)
	}
	return out
}

func (f *fakeJobs) FindWithFilters(_ context.Context, fl scheduledjob.ListFilters) ([]scheduledjob.ScheduledJob, error) {
	f.filters = append(f.filters, fl)
	all := f.match(fl)
	lo := min(int(*fl.Offset), len(all))
	hi := min(lo+int(*fl.Limit), len(all))
	return all[lo:hi], nil
}

func (f *fakeJobs) CountWithFilters(_ context.Context, fl scheduledjob.ListFilters) (int64, error) {
	return int64(len(f.match(fl))), nil
}

func (f *fakeJobs) FindByID(_ context.Context, id string) (*scheduledjob.ScheduledJob, error) {
	for i := range f.jobs {
		if f.jobs[i].ID == id {
			j := f.jobs[i]
			return &j, nil
		}
	}
	return nil, nil
}

type fakeInstances struct {
	ins []scheduledjob.ScheduledJobInstance
}

func (f *fakeInstances) List(_ context.Context, fl scheduledjob.InstanceListFilters) ([]scheduledjob.ScheduledJobInstance, error) {
	var out []scheduledjob.ScheduledJobInstance
	for _, in := range f.ins {
		if (fl.ScheduledJobID == nil || in.ScheduledJobID == *fl.ScheduledJobID) && (fl.Status == nil || in.Status == *fl.Status) && (fl.TriggerKind == nil || in.TriggerKind == *fl.TriggerKind) {
			out = append(out, in)
		}
	}
	return out, nil
}

func (f *fakeInstances) Count(ctx context.Context, fl scheduledjob.InstanceListFilters) (int64, error) {
	l, _ := f.List(ctx, fl)
	return int64(len(l)), nil
}

func (f *fakeInstances) FindByID(_ context.Context, id string) (*scheduledjob.ScheduledJobInstance, error) {
	for i := range f.ins {
		if f.ins[i].ID == id {
			in := f.ins[i]
			return &in, nil
		}
	}
	return nil, nil
}

func (f *fakeInstances) HasActiveInstance(_ context.Context, jobID string, _ bool) (bool, error) {
	return jobID == "sj_1", nil
}

func (f *fakeInstances) ListLogs(_ context.Context, id string, _ int64) ([]scheduledjob.ScheduledJobInstanceLog, error) {
	return []scheduledjob.ScheduledJobInstanceLog{{ID: "log_1", InstanceID: id, Level: "ERROR", Message: "target <refused>", Metadata: json.RawMessage(`{"status":502}`)}}, nil
}

type fakeSjOps struct {
	created []sjops.CreateCommand
	updated []sjops.UpdateCommand
	actions []string
	err     error
}

func (o *fakeSjOps) Create(_ context.Context, cmd sjops.CreateCommand) (string, error) {
	o.created = append(o.created, cmd)
	return "sj_1", o.err
}

func (o *fakeSjOps) Update(_ context.Context, cmd sjops.UpdateCommand) error {
	o.updated = append(o.updated, cmd)
	return o.err
}

func (o *fakeSjOps) act(s string) error { o.actions = append(o.actions, s); return o.err }

func (o *fakeSjOps) Pause(_ context.Context, id string) error   { return o.act("pause:" + id) }
func (o *fakeSjOps) Resume(_ context.Context, id string) error  { return o.act("resume:" + id) }
func (o *fakeSjOps) Archive(_ context.Context, id string) error { return o.act("archive:" + id) }
func (o *fakeSjOps) Delete(_ context.Context, id string) error  { return o.act("delete:" + id) }
func (o *fakeSjOps) Fire(_ context.Context, id string) (string, error) {
	return "inst_42", o.act("fire:" + id)
}

func job(id, code, name string, status scheduledjob.Status, client string) scheduledjob.ScheduledJob {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	j := scheduledjob.ScheduledJob{
		ID: id, Code: code, Name: name, Status: status, Crons: []string{"0 0 * * * *", "0 30 * * * *"}, Timezone: "UTC",
		DeliveryMaxAttempts: 3, Payload: json.RawMessage(`{"a":1}`), CreatedAt: day, UpdatedAt: day,
	}
	if client != "" {
		cid := ids.ClientID(client)
		j.ClientID = &cid
	}
	return j
}

type sjHarness struct {
	jobs *fakeJobs
	ins  *fakeInstances
	ops  *fakeSjOps
}

func newSjHarness() *sjHarness {
	day := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	errText := "connection <refused>"
	return &sjHarness{
		jobs: &fakeJobs{jobs: []scheduledjob.ScheduledJob{
			job("sj_1", "daily-cleanup", "Daily <cleanup>", scheduledjob.StatusActive, ""),
			job("sj_2", "acme-report", "Acme report", scheduledjob.StatusPaused, "clt_1"),
			job("sj_3", "old-job", "Old job", scheduledjob.StatusArchived, "clt_2"),
		}},
		ins: &fakeInstances{ins: []scheduledjob.ScheduledJobInstance{
			{ID: "inst_1", ScheduledJobID: "sj_1", JobCode: "daily-cleanup", TriggerKind: scheduledjob.TriggerCron, Status: scheduledjob.InstanceStatusDeliveryFailed, FiredAt: day, DeliveryError: &errText},
			{ID: "inst_2", ScheduledJobID: "sj_1", JobCode: "daily-cleanup", TriggerKind: scheduledjob.TriggerManual, Status: scheduledjob.InstanceStatusCompleted, FiredAt: day},
		}},
		ops: &fakeSjOps{},
	}
}

func (h *sjHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	page := &ScheduledJobs{Jobs: h.jobs, Instances: h.ins, Ops: h.ops, Clients: newCliHarness().store, Apps: newAppHarness().store, Shell: shellForTest()}
	return serveMounted(t, ac, q, page)
}

func (h *sjHarness) do(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

const sjViewPerm = "platform:messaging:scheduled-job:view"

func TestScheduledJobsList(t *testing.T) {
	h := newSjHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/scheduled-jobs"})
	mustContain(t, body, `<title>Scheduled Jobs · FlowCatalyst</title>`, "New Scheduled Job", "Daily &lt;cleanup&gt;", "running",
		"0 0 * * * * (+1)", ">Platform<", ">Acme &lt;Corp&gt;<", `value="platform"`, "Showing 1 to 3 of 3 scheduled jobs")
	filtered := h.do(t, anchorAdmin(), req{target: "/ui/scheduled-jobs?statuses=PAUSED&clients=clt_1", datastar: true})
	mustContain(t, filtered, "Acme report", `id="sj-filter-statuses-summary" class="fc-multiselect-summary">Paused<`)
	mustNotContain(t, filtered, "Daily &lt;cleanup&gt;")
	paged := h.do(t, anchorAdmin(), req{target: "/ui/scheduled-jobs?rows=10&page=1&q=report"})
	mustContain(t, paged, "Showing 1 to 1 of 1 scheduled jobs")

	scoped := &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Roles: []string{"r"}, Permissions: []string{sjViewPerm}, PrincipalType: auth.PrincipalTypeUser}
	own := h.do(t, scoped, req{target: "/ui/scheduled-jobs"})
	mustContain(t, own, "Acme report")
	mustNotContain(t, own, "Daily &lt;cleanup&gt;", `value="platform"`)
	if rec := h.rec(t, scoped, req{target: "/ui/scheduled-jobs/sj_1"}); rec.Code != http.StatusNotFound {
		t.Errorf("a platform job for a client user: %d", rec.Code)
	}
}

func TestScheduledJobDrawerAndActions(t *testing.T) {
	h := newSjHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/scheduled-jobs/sj_1", datastar: true})
	mustContain(t, body, "Definition", "Recent Firings", "DELIVERY_FAILED", `href="/ui/scheduled-jobs/instances/inst_1"`, ">Running<",
		"not configured (firings will fail)", "&#34;a&#34;: 1", `action="/ui/scheduled-jobs/sj_1/pause"`, `action="/ui/scheduled-jobs/sj_1/fire"`,
		`data-action="/ui/scheduled-jobs/sj_1/archive"`, `href="/ui/scheduled-jobs/sj_1/instances"`)
	archived := h.do(t, anchorAdmin(), req{target: "/ui/scheduled-jobs/sj_3", datastar: true})
	mustNotContain(t, archived, `/archive"`, ">Edit<", `action="/ui/scheduled-jobs/sj_3/fire"`)

	fire := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/scheduled-jobs/sj_1/fire", datastar: true, form: url.Values{"drawer": {"sj_1"}}})
	mustContain(t, fire, "Instance inst_42 created", "data: selector #sj-drawer\n")
	for _, a := range []string{"sj_1/pause", "sj_2/resume", "sj_1/archive", "sj_2/delete"} {
		mustContain(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/scheduled-jobs/" + a, datastar: true, form: url.Values{}}), "fc-flash-success")
	}
	if got := strings.Join(h.ops.actions, ","); got != "fire:sj_1,pause:sj_1,resume:sj_2,archive:sj_1,delete:sj_2" {
		t.Errorf("actions = %s", got)
	}

	firer := connUser(sjViewPerm, "platform:messaging:scheduled-job:fire")
	only := h.do(t, firer, req{target: "/ui/scheduled-jobs/sj_1", datastar: true})
	mustContain(t, only, "Fire Now")
	mustNotContain(t, only, "/pause\"", "Archive", ">Delete<")
	h.do(t, firer, req{method: http.MethodPost, target: "/ui/scheduled-jobs/sj_1/archive", datastar: true, form: url.Values{}})
	if len(h.ops.actions) != 5 {
		t.Fatal("archived without the update permission")
	}
}

func TestScheduledJobForms(t *testing.T) {
	h := newSjHarness()
	dlg := h.do(t, anchorAdmin(), req{target: "/ui/scheduled-jobs/new", datastar: true})
	mustContain(t, dlg, "data: mode replace\n", `data: elements <div id="sj-dialog-slot">`, "New Scheduled Job", "Platform-scoped (anchor only)", `value="UTC"`, `name="deliveryMaxAttempts" value="3"`, `name="crons"`)
	body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/scheduled-jobs/new", datastar: true, form: url.Values{
		"code": {"nightly"}, "name": {"Nightly"}, "scope": {"platform"}, "applicationId": {""}, "timezone": {"Europe/London"},
		"crons": {" 0 0 2 * * *\n\n0 0 3 * * * "}, "targetUrl": {""}, "deliveryMaxAttempts": {"5"}, "timeoutSeconds": {""}, "payload": {`{"x":true}`}, "concurrent": {"true"},
	}})
	c := h.ops.created[0]
	if c.Code != "nightly" || c.ClientID != nil || strings.Join(c.Crons, "|") != "0 0 2 * * *|0 0 3 * * *" || *c.DeliveryMaxAttempts != 5 || c.TimeoutSeconds != nil ||
		string(c.Payload) != `{"x":true}` || !c.Concurrent || c.TracksCompletion || c.TargetURL != nil || c.Timezone != "Europe/London" {
		t.Errorf("create = %+v", c)
	}
	mustContain(t, body, "Scheduled job &#39;nightly&#39; created")
	bad := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/scheduled-jobs/new", datastar: true, form: url.Values{"code": {"x"}, "name": {"X"}, "scope": {""}, "crons": {""}, "payload": {"{nope"}}})
	mustContain(t, bad, "Choose Platform or a client", "At least one cron expression is required.", "Payload must be valid JSON or empty.")
	h.ops.err = usecase.Validation("CRON_INVALID_SHAPE", "cron must have 6 fields")
	shape := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/scheduled-jobs/new", datastar: true, form: url.Values{"code": {"x"}, "name": {"X"}, "scope": {"platform"}, "crons": {"* *"}}})
	mustContain(t, shape, `id="sj-form-crons-err"`, "Cron must have 6 fields")

	h.ops.err = nil
	edit := h.do(t, anchorAdmin(), req{target: "/ui/scheduled-jobs/sj_1/edit", datastar: true})
	mustContain(t, edit, "Edit Scheduled Job", "0 0 * * * *", "0 30 * * * *", "&#34;a&#34;: 1")
	h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/scheduled-jobs/sj_1/edit", datastar: true, form: url.Values{
		"name": {" Daily "}, "description": {""}, "timezone": {""}, "crons": {"0 0 1 * * *"}, "payload": {""}, "deliveryMaxAttempts": {"4"}, "timeoutSeconds": {""}, "targetUrl": {"https://t.example.com"},
	}})
	u := h.ops.updated[0]
	if *u.Name != "Daily" || u.Description != nil || u.Timezone != nil || u.Payload != nil || *u.DeliveryMaxAttempts != 4 || deref(u.TargetURL) != "https://t.example.com" || *u.Concurrent || *u.TracksCompletion {
		t.Errorf("update = %+v", u)
	}
}

func TestScheduledJobFirings(t *testing.T) {
	h := newSjHarness()
	list := h.do(t, anchorAdmin(), req{target: "/ui/scheduled-jobs/sj_1/instances"})
	mustContain(t, list, "Firings: Daily &lt;cleanup&gt;", "connection &lt;refused&gt;", "Showing 1 to 2 of 2 firings")
	cron := h.do(t, anchorAdmin(), req{target: "/ui/scheduled-jobs/sj_1/instances?trigger=CRON", datastar: true})
	mustContain(t, cron, "DELIVERY_FAILED", "Showing 1 to 1 of 1 firings")
	mustNotContain(t, cron, "COMPLETED")
	one := h.do(t, anchorAdmin(), req{target: "/ui/scheduled-jobs/instances/inst_1"})
	mustContain(t, one, "Firing inst_1", "connection &lt;refused&gt;", "target &lt;refused&gt;", "{&#34;status&#34;:502}")
	scoped := &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Roles: []string{"r"}, Permissions: []string{sjViewPerm}, PrincipalType: auth.PrincipalTypeUser}
	if rec := h.rec(t, scoped, req{target: "/ui/scheduled-jobs/instances/inst_1"}); rec.Code != http.StatusNotFound {
		t.Errorf("a platform firing for a client user: %d", rec.Code)
	}
}
