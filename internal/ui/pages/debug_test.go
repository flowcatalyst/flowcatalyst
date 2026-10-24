package pages

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

const (
	permEventViewRaw = "platform:messaging:event:view-raw"
	permEventView    = "platform:messaging:event:view"
	permDjViewRaw    = "platform:messaging:dispatch-job:view-raw"
	permDjView       = "platform:messaging:dispatch-job:view"
)

type fakeRawEvents struct {
	events []event.Event
	limits []int
}

func (f *fakeRawEvents) FindRecentRaw(_ context.Context, limit int) ([]event.Event, error) {
	f.limits = append(f.limits, limit)
	return f.events, nil
}

func (f *fakeRawEvents) FindRawByID(_ context.Context, id string) (*event.Event, error) {
	for i := range f.events {
		if f.events[i].ID == id {
			return &f.events[i], nil
		}
	}
	return nil, nil
}

func newRawEvents() *fakeRawEvents {
	corr := "corr-1"
	first := ev("0EVENTID0000001", "orders:sales:order:created", "clt_1", 1)
	first.Data = json.RawMessage(`{"z":1,"note":"<b>raw</b>"}`)
	first.CorrelationID = &corr
	first.Context = []event.ContextEntry{{Key: "tenant", Value: "acme <x>"}}
	return &fakeRawEvents{events: []event.Event{first, ev("0EVENTID0000002", "crm:contacts:person:updated", "", 2)}}
}

func rawEventsRec(t *testing.T, store *fakeRawEvents, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, &DebugEvents{Events: store, Shell: shellForTest()})
}

func TestRawEventsList(t *testing.T) {
	store := newRawEvents()
	ac := cfgUser(permEventViewRaw)
	body := rawEventsRec(t, store, ac, req{target: "/ui/debug/events"}).Body.String()
	mustContain(t, body, "Raw Events", "Debug view of the transactional event store", "Showing the 2 most recent raw events",
		"0EVENTID00...", `href="/ui/debug/events/0EVENTID0000001"`, "orders:sales:order:created", "dedup-0EVE...", `value="200" selected`)
	if got := store.limits[len(store.limits)-1]; got != 200 {
		t.Errorf("default size %d", got)
	}
	t.Run("the result size is one of the offered sizes", func(t *testing.T) {
		rawEventsRec(t, store, ac, req{target: "/ui/debug/events?size=50"})
		rawEventsRec(t, store, ac, req{target: "/ui/debug/events?size=100000"})
		rawEventsRec(t, store, ac, req{target: "/ui/debug/events?size=-3"})
		if got := store.limits[len(store.limits)-3:]; got[0] != 50 || got[1] != 200 || got[2] != 200 {
			t.Errorf("limits %v", got)
		}
	})
	t.Run("a live request patches the table, summary and toolbar and keeps the size in the address", func(t *testing.T) {
		body := rawEventsRec(t, store, ac, req{target: "/ui/debug/events?size=500", datastar: true}).Body.String()
		mustContain(t, body, `id="dbg-ev-table"`, `id="dbg-ev-summary"`, "/ui/debug/events?size=500")
	})
}

func TestRawEventDetail(t *testing.T) {
	store := newRawEvents()
	ac := cfgUser(permEventViewRaw)
	body := rawEventsRec(t, store, ac, req{target: "/ui/debug/events/0EVENTID0000001", datastar: true}).Body.String()
	mustContain(t, body, "orders:sales:order:created", "0EVENTID0000001", "corr-1", "clt_1", "&lt;b&gt;raw&lt;/b&gt;", "acme &lt;x&gt;", `"selDbgEv":"0EVENTID0000001"`)
	mustNotContain(t, body, "<b>raw</b>", "acme <x>")
	t.Run("the full page opens the drawer", func(t *testing.T) {
		mustContain(t, rawEventsRec(t, store, ac, req{target: "/ui/debug/events/0EVENTID0000001"}).Body.String(), `data-signals="{selDbgEv: &#39;0EVENTID0000001&#39;}"`, "Showing the 2 most recent raw events")
	})
	t.Run("an event that is not there, or an id that is not an id", func(t *testing.T) {
		for _, id := range []string{"0EVENTID0000999", "a'b", "x%20y"} {
			if rec := rawEventsRec(t, store, ac, req{target: "/ui/debug/events/" + id}); rec.Code != http.StatusNotFound {
				t.Errorf("%s: status %d", id, rec.Code)
			}
		}
	})
}

func TestRawEventsPermission(t *testing.T) {
	store := newRawEvents()
	// event:view is the Events page's permission, not this one's.
	for name, ac := range map[string]*auth.AuthContext{"nothing": cfgUser(), "the regular view": cfgUser(permEventView), "the raw dispatch permission": cfgUser(permDjViewRaw)} {
		for _, target := range []string{"/ui/debug/events", "/ui/debug/events/0EVENTID0000001"} {
			for _, datastar := range []bool{false, true} {
				rec := rawEventsRec(t, store, ac, req{target: target, datastar: datastar})
				body := rec.Body.String()
				if datastar {
					mustContain(t, body, "fc-flash-error")
				} else if rec.Code != http.StatusForbidden {
					t.Errorf("%s %s: status %d", name, target, rec.Code)
				}
				mustNotContain(t, body, "orders:sales:order:created", "raw</b>", "corr-1")
			}
		}
	}
	if len(store.limits) != 0 {
		t.Error("the store was read for a caller without the permission")
	}
	t.Run("not scoped to a client, as the API's", func(t *testing.T) {
		body := rawEventsRec(t, store, cfgUser(permEventViewRaw), req{target: "/ui/debug/events"}).Body.String()
		mustContain(t, body, "orders:sales:order:created", "crm:contacts:person:updated")
	})
}

type fakeRawJobs struct {
	jobs   []dispatchjob.DispatchJob
	limits []int
}

func (f *fakeRawJobs) FindRecentRaw(_ context.Context, limit int) ([]dispatchjob.DispatchJob, error) {
	f.limits = append(f.limits, limit)
	return f.jobs, nil
}

func (f *fakeRawJobs) FindByID(_ context.Context, id string) (*dispatchjob.DispatchJob, error) {
	for i := range f.jobs {
		if f.jobs[i].ID == id {
			return &f.jobs[i], nil
		}
	}
	return nil, nil
}

func newRawJobs() *fakeRawJobs {
	payload, key, errText := `{"p":"<b>"}`, "idem-0123456789abcdef", "boom <script>"
	first := djJob("djb_000000001", "orders:sales:order:created", common.DispatchFailed, "clt_1", 1)
	first.Payload, first.IdempotencyKey, first.LastError, first.PayloadContentType = &payload, &key, &errText, "application/json"
	first.Attempts = []dispatchjob.Attempt{{}, {}}
	return &fakeRawJobs{jobs: []dispatchjob.DispatchJob{first, djJob("djb_000000002", "crm:contacts:person:updated", common.DispatchCompleted, "clt_2", 2)}}
}

func rawJobsRec(t *testing.T, store *fakeRawJobs, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, &DebugDispatchJobs{Jobs: store, Shell: shellForTest()})
}

func TestRawDispatchJobsList(t *testing.T) {
	store := newRawJobs()
	ac := cfgUser(permDjViewRaw)
	body := rawJobsRec(t, store, ac, req{target: "/ui/debug/dispatch-jobs"}).Body.String()
	mustContain(t, body, "Raw Dispatch Jobs", "Debug view of the transactional dispatch job store", "Showing the 2 most recent raw dispatch jobs",
		"djb_000000...", `href="/ui/debug/dispatch-jobs/djb_000000001"`, "FAILED", "COMPLETED", "1/3", "11 bytes", "idem-01234...")
	if got := store.limits[0]; got != 200 {
		t.Errorf("default size %d", got)
	}
	t.Run("the severity of a status", func(t *testing.T) {
		for status, want := range map[string]string{"COMPLETED": "success", "PENDING": "info", "QUEUED": "info", "PROCESSING": "warn", "FAILED": "danger", "CANCELLED": "secondary", "EXPIRED": "secondary", "WHATEVER": "secondary"} {
			if got := string(dbgDjSeverity(status)); got != want {
				t.Errorf("%s: %s, want %s", status, got, want)
			}
		}
	})
	t.Run("a live request patches the table", func(t *testing.T) {
		mustContain(t, rawJobsRec(t, store, ac, req{target: "/ui/debug/dispatch-jobs?size=1000", datastar: true}).Body.String(), `id="dbg-dj-table"`, "/ui/debug/dispatch-jobs?size=1000")
	})
}

func TestRawDispatchJobDetail(t *testing.T) {
	store := newRawJobs()
	ac := cfgUser(permDjViewRaw)
	ac.Scope = auth.ScopeAnchor
	body := rawJobsRec(t, store, ac, req{target: "/ui/debug/dispatch-jobs/djb_000000001", datastar: true}).Body.String()
	mustContain(t, body, "djb_000000001", "https://hooks.example.com/in", "clt_1", "1/3 (2 in history)", "boom &lt;script&gt;", "11 bytes (application/json)", `"selDbgDj":"djb_000000001"`)
	mustNotContain(t, body, "boom <script>")
	mustNotContain(t, body, `{"p":"<b>"}`)
	t.Run("a client-scoped viewer is checked against the job's client", func(t *testing.T) {
		scoped := &auth.AuthContext{PrincipalID: "prn_c", Scope: auth.ScopeClient, Roles: []string{"r"}, Permissions: []string{permDjViewRaw}, Clients: []string{"clt_2"}, PrincipalType: auth.PrincipalTypeUser}
		if rec := rawJobsRec(t, store, scoped, req{target: "/ui/debug/dispatch-jobs/djb_000000001"}); rec.Code != http.StatusForbidden {
			t.Errorf("another client's job: status %d", rec.Code)
		}
		if rec := rawJobsRec(t, store, scoped, req{target: "/ui/debug/dispatch-jobs/djb_000000002"}); rec.Code != http.StatusOK {
			t.Errorf("its own client's job: status %d", rec.Code)
		}
	})
	t.Run("not found", func(t *testing.T) {
		for _, id := range []string{"djb_999", "a'b"} {
			if rec := rawJobsRec(t, store, ac, req{target: "/ui/debug/dispatch-jobs/" + id}); rec.Code != http.StatusNotFound {
				t.Errorf("%s: status %d", id, rec.Code)
			}
		}
	})
}

func TestRawDispatchJobsPermission(t *testing.T) {
	store := newRawJobs()
	// dispatch-job:view is the Dispatch Jobs page's, not this one's.
	for name, ac := range map[string]*auth.AuthContext{"nothing": cfgUser(), "the regular view": cfgUser(permDjView), "the raw event permission": cfgUser(permEventViewRaw)} {
		for _, target := range []string{"/ui/debug/dispatch-jobs", "/ui/debug/dispatch-jobs/djb_000000001"} {
			rec := rawJobsRec(t, store, ac, req{target: target})
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s %s: status %d", name, target, rec.Code)
			}
			mustNotContain(t, rec.Body.String(), "orders:sales:order:created", "hooks.example.com")
			mustContain(t, rawJobsRec(t, store, ac, req{target: target, datastar: true}).Body.String(), "fc-flash-error")
		}
	}
	if len(store.limits) != 0 {
		t.Error("the store was read for a caller without the permission")
	}
}
