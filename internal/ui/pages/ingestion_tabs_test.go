package pages

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

const (
	uitPermView   = "platform:ingestion:ingestion:view"
	uitPermTest   = "platform:ingestion:ingestion:test"
	uitPermApply  = "platform:ingestion:set:apply"
	uitSecretNote = "SECRET-DEFINITION-hunter2"
)

// uitUser is a platform user (all applications) holding perms.
func uitUser(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{
		PrincipalID: "prn_ing", Email: "ing@x.y", Scope: auth.ScopeAnchor, Roles: []string{"r"}, Permissions: perms,
		AllApplications: true, PrincipalType: auth.PrincipalTypeUser,
	}
}

// uitClientUser reaches clt_a only.
func uitClientUser(perms ...string) *auth.AuthContext {
	ac := uitUser(perms...)
	ac.Scope, ac.Clients = auth.ScopeClient, []string{"clt_a"}
	return ac
}

// uitAppUser reaches app_a only.
func uitAppUser(perms ...string) *auth.AuthContext {
	ac := uitUser(perms...)
	ac.AllApplications, ac.Applications = false, []string{"app_a"}
	return ac
}

type fakeUitTabs struct {
	ings     map[string]*ingestion.Ingestion
	routes   []ingestion.RouteState
	activity *ingestion.Activity
	failures []ingestion.Failure
	afters   []string
}

func (f *fakeUitTabs) FindByID(_ context.Context, id string) (*ingestion.Ingestion, error) {
	return f.ings[id], nil
}

func (f *fakeUitTabs) ListRouteStates(context.Context, string) ([]ingestion.RouteState, error) {
	return f.routes, nil
}

func (f *fakeUitTabs) ActivityOf(context.Context, string, time.Time) (*ingestion.Activity, error) {
	return f.activity, nil
}

// ListFailures pages the way the repository does: by id, newest first, the
// cursor being the last id of the previous page.
func (f *fakeUitTabs) ListFailures(_ context.Context, _, after string, limit int) (*ingestion.FailurePage, error) {
	f.afters = append(f.afters, after)
	start := 0
	if after != "" {
		for n, x := range f.failures {
			if x.ID == after {
				start = n + 1
			}
		}
	}
	rest := f.failures[start:]
	page := &ingestion.FailurePage{}
	if len(rest) > limit {
		page.Items, page.Next = rest[:limit], rest[limit-1].ID
	} else {
		page.Items = rest
	}
	return page, nil
}

type fakeUitAudit struct{ logs []audit.Log }

func (f fakeUitAudit) FindWithFilters(_ context.Context, p audit.FilterParams) ([]audit.Log, error) {
	if p.Offset >= len(f.logs) {
		return nil, nil
	}
	end := min(p.Offset+p.Limit, len(f.logs))
	return f.logs[p.Offset:end], nil
}

var uitNow = time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

func uitFixture(keep bool) *ingestion.Ingestion {
	return &ingestion.Ingestion{
		ID: "ing_one", ApplicationID: "app_a", ClientID: "clt_a", Code: "orders", Active: true, Version: 3, LatestVersion: 3,
		Definition: ingestion.Definition{Name: "Orders", KeepFailedPayloads: keep},
	}
}

func uitTabsStore(keep bool) *fakeUitTabs {
	last := uitNow.Add(-time.Minute)
	failed := uitNow.Add(-time.Hour)
	errText := "connection refused <b>by</b> the source"
	return &fakeUitTabs{
		ings: map[string]*ingestion.Ingestion{"ing_one": uitFixture(keep)},
		routes: []ingestion.RouteState{
			{RuntimeID: "rt-1", Version: 3, Status: "STARTED", UpdatedAt: uitNow.Add(-2 * time.Minute)},
			{RuntimeID: "rt-2", Version: 2, Status: "FAILING", Error: &errText, UpdatedAt: uitNow.Add(-3 * time.Minute)},
		},
		activity: &ingestion.Activity{
			Day: ingestion.Exchanges{Completed: 12, Failed: 1, Records: 340, Jobs: 12}, Week: ingestion.Exchanges{Completed: 80, Failed: 4, Records: 2100, Jobs: 80},
			LastExchange: &last, LastFailure: &failed,
		},
	}
}

func uitTabsPage(store *fakeUitTabs, logs ...audit.Log) *IngestionTabs {
	return &IngestionTabs{Store: store, Audit: fakeUitAudit{logs: logs}, Shell: shellForTest(), now: func() time.Time { return uitNow }}
}

func uitGet(t *testing.T, page *IngestionTabs, ac *auth.AuthContext, target string, datastar bool) (int, string) {
	t.Helper()
	rec := serveMounted(t, ac, req{target: target, datastar: datastar}, page)
	return rec.Code, rec.Body.String()
}

func TestIngestionTabPlaceholdersAskForTheirPanels(t *testing.T) {
	var b strings.Builder
	require.NoError(t, IngestionActivityTab("ing_one").Render(context.Background(), &b))
	require.NoError(t, IngestionFailuresTab("ing_one").Render(context.Background(), &b))
	require.NoError(t, IngestionHistoryTab("ing_one").Render(context.Background(), &b))
	mustContain(t, b.String(),
		`id="ing-activity" data-url="/ui/ingestions/ing_one/activity" data-init="@get(el.dataset.url)"`,
		`id="ing-failures" data-url="/ui/ingestions/ing_one/failures" data-init="@get(el.dataset.url)"`,
		`id="ing-history" data-url="/ui/ingestions/ing_one/history" data-init="@get(el.dataset.url)"`,
	)
}

func TestIngestionActivityPanel(t *testing.T) {
	code, body := uitGet(t, uitTabsPage(uitTabsStore(false)), uitUser(uitPermView), "/ui/ingestions/ing_one/activity", true)
	require.Equal(t, http.StatusOK, code, body)
	mustContain(t, body,
		`id="ing-activity"`, "rt-1", "rt-2", `fc-tag fc-tag-success">STARTED`, `fc-tag fc-tag-warn">FAILING`,
		// rt-2 runs version 2, not the live 3.
		`class="text-warning" title="Not the live version">2<`,
		"connection refused &lt;b&gt;by&lt;/b&gt; the source",
		"12 completed, 1 failed, 340 records, 12 jobs", "80 completed, 4 failed, 2100 records, 80 jobs",
		"Last exchange", "Last failure", `datetime="2026-10-24T11:59:00Z"`,
	)
	mustNotContain(t, body, "<b>by</b>", "None in 7 days")

	// No window and no failure: said so.
	s := uitTabsStore(false)
	s.activity, s.routes = &ingestion.Activity{}, nil
	_, body = uitGet(t, uitTabsPage(s), uitUser(uitPermView), "/ui/ingestions/ing_one/activity", true)
	mustContain(t, body, "No runtime has reported it yet.", "None in 7 days", "None kept", "0 completed, 0 failed, 0 records, 0 jobs")
}

func TestIngestionTabsPermissionsAndReach(t *testing.T) {
	cases := map[string]struct {
		ac   *auth.AuthContext
		want int
	}{
		"viewer":                    {uitUser(uitPermView), http.StatusOK},
		"no permission":             {uitUser(uitPermTest), http.StatusForbidden},
		"client of the ingestion":   {uitClientUser(uitPermView), http.StatusOK},
		"another client":            {&auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_b"}, Roles: []string{"r"}, Permissions: []string{uitPermView}, AllApplications: true, PrincipalType: auth.PrincipalTypeUser}, http.StatusForbidden},
		"application of it":         {uitAppUser(uitPermView), http.StatusOK},
		"another application":       {&auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeAnchor, Roles: []string{"r"}, Permissions: []string{uitPermView}, Applications: []string{"app_b"}, PrincipalType: auth.PrincipalTypeUser}, http.StatusForbidden},
		"anchor app-scoped to none": {&auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeAnchor, Roles: []string{"r"}, Permissions: []string{uitPermView}, PrincipalType: auth.PrincipalTypeUser}, http.StatusForbidden},
	}
	for name, c := range cases {
		for _, tab := range []string{"activity", "failures", "history"} {
			// A Datastar request answers 200 with an error flash (the error
			// contract); the page request carries the real status.
			code, body := uitGet(t, uitTabsPage(uitTabsStore(true), uitLog("CreateCommand", `{}`, 0)), c.ac, "/ui/ingestions/ing_one/"+tab, false)
			if c.want == http.StatusOK {
				assert.Equal(t, http.StatusSeeOther, code, "%s %s: a navigation to a panel goes to the ingestion", name, tab)
				continue
			}
			assert.Equal(t, c.want, code, "%s %s", name, tab)
			_, ds := uitGet(t, uitTabsPage(uitTabsStore(true), uitLog("CreateCommand", `{}`, 0)), c.ac, "/ui/ingestions/ing_one/"+tab, true)
			mustNotContain(t, ds+body, "rt-1", "Created by")
			mustContain(t, ds, "flash")
		}
	}
	// Reach did not stop the allowed ones from reading their panel.
	for name, c := range cases {
		if c.want != http.StatusOK {
			continue
		}
		code, body := uitGet(t, uitTabsPage(uitTabsStore(true)), c.ac, "/ui/ingestions/ing_one/activity", true)
		assert.Equal(t, http.StatusOK, code, name)
		mustContain(t, body, "rt-1")
	}
}

func TestIngestionTabsUnknownAndForgedIDs(t *testing.T) {
	page := uitTabsPage(uitTabsStore(true))
	for _, id := range []string{"ing_none", "x'y", "a b"} {
		for _, tab := range []string{"activity", "failures", "history"} {
			code, _ := uitGet(t, page, uitUser(uitPermView), "/ui/ingestions/"+strings.ReplaceAll(id, " ", "%20")+"/"+tab, false)
			assert.Equal(t, http.StatusNotFound, code, "%s %s", id, tab)
		}
	}
	// A cursor that is not an id never reaches the store.
	s := uitTabsStore(true)
	_, body := uitGet(t, uitTabsPage(s), uitUser(uitPermView), "/ui/ingestions/ing_one/failures?after=x%27%3Bdrop", true)
	assert.Empty(t, s.afters)
	mustContain(t, body, "after is not a cursor this list gave")
}

func uitFailure(id, kind, msg string, excerpt *string, after int64) ingestion.Failure {
	rt, v := "rt-1", 3
	return ingestion.Failure{ID: id, RuntimeID: &rt, Version: &v, Kind: kind, Message: msg, AfterRecords: after, Excerpt: excerpt, OccurredAt: uitNow.Add(-time.Minute)}
}

func TestIngestionFailuresPanel(t *testing.T) {
	start := "{\"order\": <script>alert(1)</script>"
	s := uitTabsStore(true)
	s.failures = []ingestion.Failure{
		uitFailure("ifl_3", "REJECTED", "bad <i>payload</i>", &start, 1),
		uitFailure("ifl_2", "SOURCE", "timeout", nil, 0),
		uitFailure("ifl_1", "AUTH", "login refused", nil, 5),
	}
	code, body := uitGet(t, uitTabsPage(s), uitUser(uitPermView), "/ui/ingestions/ing_one/failures", true)
	require.Equal(t, http.StatusOK, code, body)
	mustContain(t, body,
		"with the start of the payload each one failed on", "Payload rejected", "Source error", "Login refused",
		"bad &lt;i&gt;payload&lt;/i&gt;", "After 1 record of the payload.", "After 5 records of the payload.",
		"rt-1", ", version 3", "The start of the payload", "&lt;script&gt;alert(1)&lt;/script&gt;",
	)
	mustNotContain(t, body, "<script>alert(1)</script>", `id="ing-failures-more"><a`)
	// Newest first, as given.
	assert.Less(t, strings.Index(body, "ifl_3"), strings.Index(body, "ifl_2"))
	assert.Less(t, strings.Index(body, "ifl_2"), strings.Index(body, "ifl_1"))
}

func TestIngestionFailuresShowAnExcerptOnlyWhilePayloadsAreKept(t *testing.T) {
	start := "PAYLOAD-START-hunter2"
	for _, keep := range []bool{true, false} {
		s := uitTabsStore(keep)
		s.failures = []ingestion.Failure{uitFailure("ifl_1", "REJECTED", "bad", &start, 0)}
		_, body := uitGet(t, uitTabsPage(s), uitUser(uitPermView), "/ui/ingestions/ing_one/failures", true)
		if keep {
			mustContain(t, body, start, "with the start of the payload each one failed on")
		} else {
			// Kept before keeping was switched off: not shown now.
			mustNotContain(t, body, start)
			mustContain(t, body, "without payloads", "Keep failed payloads")
		}
	}
}

func TestIngestionFailuresPageByCursor(t *testing.T) {
	s := uitTabsStore(false)
	for n := 25; n >= 1; n-- {
		s.failures = append(s.failures, uitFailure("ifl_"+uitPad2(n), "SOURCE", "failure number "+uitPad2(n), nil, 0))
	}
	_, body := uitGet(t, uitTabsPage(s), uitUser(uitPermView), "/ui/ingestions/ing_one/failures", true)
	mustContain(t, body, "failure number 25", "failure number 06", `data-more="/ui/ingestions/ing_one/failures?after=ifl_06"`, ">Older<")
	mustNotContain(t, body, "failure number 05")

	// The next page appends below, and the last one has no button.
	_, body = uitGet(t, uitTabsPage(s), uitUser(uitPermView), "/ui/ingestions/ing_one/failures?after=ifl_06", true)
	mustContain(t, body, "failure number 05", "failure number 01", "mode append", "#"+ingFailuresList, `id="ing-failures-more"`)
	mustNotContain(t, body, "failure number 06", "data-more=")
	assert.Equal(t, []string{"", "ifl_06"}, s.afters)
}

func uitPad2(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// uitLog is an audit row of ingestion ing_one, ago minutes before uitNow.
func uitLog(op, command string, ago int) audit.Log {
	who := "Ada"
	return audit.Log{
		ID: "aud_" + op, EntityType: "Ingestion", EntityID: "ing_one", Operation: op, OperationJSON: json.RawMessage(command),
		PrincipalID: new("prn_ada"), PrincipalName: &who, PerformedAt: uitNow.Add(-time.Duration(ago) * time.Minute),
	}
}

// uitCmd is the audit command of an operation that carried a definition.
func uitCmd(t *testing.T, def ingestion.Definition, note string) string {
	t.Helper()
	cmd := map[string]any{"id": "ing_one", "definition": def}
	if note != "" {
		cmd["note"] = note
	}
	b, err := json.Marshal(cmd)
	require.NoError(t, err)
	return string(b)
}

func uitDef(name, queue, target string) ingestion.Definition {
	return ingestion.Definition{
		Name: name, Source: ingestion.Sqs{Region: "eu-west-1", Queue: queue}, Split: ingestion.None{}, Target: ingestion.Target{Code: target},
	}
}

func TestIngestionHistoryPanel(t *testing.T) {
	// Newest first: an update that changed the name and the target, a pause,
	// and the create.
	page := uitTabsPage(uitTabsStore(false),
		uitLog("UpdateCommand", uitCmd(t, uitDef("Orders v2", "q", "t2"), "rename <b>it</b>"), 1),
		uitLog("PauseCommand", `{"id":"ing_one"}`, 5),
		uitLog("CreateCommand", uitCmd(t, uitDef("Orders", "q", "t1"), uitSecretNote), 60),
	)
	code, body := uitGet(t, page, uitUser(uitPermView), "/ui/ingestions/ing_one/history", true)
	require.Equal(t, http.StatusOK, code, body)
	mustContain(t, body,
		`id="ing-history"`, "Changed", "Paused", "Created", "by Ada",
		"Changed: name, target", "Note: rename &lt;b&gt;it&lt;/b&gt;", "Note: "+uitSecretNote,
	)
	mustNotContain(t, body, "<b>it</b>", "No changes recorded.")
	// The create has no changed list; only field names are ever shown.
	assert.Equal(t, 1, strings.Count(body, "Changed: "))
	mustNotContain(t, body, "Orders v2", "eu-west-1", "keepFailedPayloads")
	assert.Less(t, strings.Index(body, "Changed"), strings.Index(body, "Paused"))
	assert.Less(t, strings.Index(body, "Paused"), strings.Index(body, "Created"))

	_, body = uitGet(t, uitTabsPage(uitTabsStore(false)), uitUser(uitPermView), "/ui/ingestions/ing_one/history", true)
	mustContain(t, body, "No changes recorded.")
}

func TestIngestionHistoryNeverPrintsADefinition(t *testing.T) {
	// A saved version's command carries the whole definition; the panel names
	// the parts that differ and nothing of their content.
	page := uitTabsPage(uitTabsStore(false),
		uitLog("SaveVersionCommand", uitCmd(t, uitDef("A", uitSecretNote+"-2", "t"), ""), 1),
		uitLog("SaveVersionCommand", uitCmd(t, uitDef("A", uitSecretNote+"-1", "t"), ""), 2),
	)
	_, body := uitGet(t, page, uitUser(uitPermView), "/ui/ingestions/ing_one/history", true)
	mustContain(t, body, "Saved as a draft, not live,", "Changed: source")
	mustNotContain(t, body, uitSecretNote)
}

// Drafts read as drafts in the history, a version saved live as live, and a
// refused secret fetch names the secret, the runtime and the reason.
func TestIngestionHistoryShowsDraftsAndRefusedFetches(t *testing.T) {
	page := uitTabsPage(uitTabsStore(false),
		uitLog("SECRET_FETCH_REFUSED", `{"secretName":"sftp-password","runtimeId":"rt-1","reason":"LEASE_NOT_HELD"}`, 1),
		uitLog("SaveVersionCommand", `{"id":"ing_one","live":true}`, 2),
		uitLog("CreateCommand", `{"id":"ing_one","live":false}`, 3),
	)
	_, body := uitGet(t, page, uitUser(uitPermView), "/ui/ingestions/ing_one/history", true)
	mustContain(t, body, "Secret fetch refused", "Secret sftp-password, runtime rt-1: LEASE_NOT_HELD",
		"Version saved and made live", "Created as a draft, not live,")
}

func TestIngestionHistoryPagesByOffset(t *testing.T) {
	var logs []audit.Log
	for n := range 25 {
		logs = append(logs, uitLog("ResumeCommand", `{"id":"ing_one"}`, n))
	}
	page := uitTabsPage(uitTabsStore(false), logs...)
	_, body := uitGet(t, page, uitUser(uitPermView), "/ui/ingestions/ing_one/history", true)
	assert.Equal(t, 20, strings.Count(body, "Resumed"))
	mustContain(t, body, `data-more="/ui/ingestions/ing_one/history?offset=20"`)
	_, body = uitGet(t, page, uitUser(uitPermView), "/ui/ingestions/ing_one/history?offset=20", true)
	assert.Equal(t, 5, strings.Count(body, "Resumed"))
	mustContain(t, body, "mode append")
	mustNotContain(t, body, "data-more=")
}
