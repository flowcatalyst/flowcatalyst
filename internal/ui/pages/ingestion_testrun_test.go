package pages

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

const uitPermRoute = auth.PermIngestionRoute

type uitQueuedTest struct {
	ingestionID *string
	app         ids.ApplicationID
	client      ids.ClientID
	def         ingestion.Definition
	by          string
}

// fakeUitTests stands in for the repository: the ingestions, the test rows (a
// test answers as the script says, one state per read) and the kinds' required
// permissions.
type fakeUitTests struct {
	mu     sync.Mutex
	ings   map[string]*ingestion.Ingestion
	queued []uitQueuedTest
	rows   map[string]*ingestion.TestRequest
	// script is the state each FindTest read returns after the first, in order;
	// the last one repeats.
	script  []ingestion.TestRequest
	reads   int
	carries bool
	perms   map[string]string
}

func (f *fakeUitTests) FindByID(_ context.Context, id string) (*ingestion.Ingestion, error) {
	return f.ings[id], nil
}

func (f *fakeUitTests) QueueTest(_ context.Context, ingestionID *string, app ids.ApplicationID, c ids.ClientID, def ingestion.Definition, by *string, now time.Time) (*ingestion.TestRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queued = append(f.queued, uitQueuedTest{ingestionID, app, c, def, *by})
	t := &ingestion.TestRequest{
		ID: "itr_one", IngestionID: ingestionID, ApplicationID: app, ClientID: c, Kind: ingestion.RunKind(def.Source), Definition: def,
		Status: ingestion.TestPending, CreatedAt: now, ExpiresAt: now.Add(ingestion.TestRequestTTL),
	}
	f.rows = map[string]*ingestion.TestRequest{t.ID: t}
	return t, nil
}

func (f *fakeUitTests) FindTest(_ context.Context, id string) (*ingestion.TestRequest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t := f.rows[id]
	if t == nil {
		return nil, nil
	}
	if f.reads < len(f.script) {
		*t = f.script[f.reads]
	} else if len(f.script) > 0 {
		*t = f.script[len(f.script)-1]
	}
	f.reads++
	cp := *t
	return &cp, nil
}

func (f *fakeUitTests) AnyLiveRuntimeCarries(context.Context) (func(string) bool, error) {
	return func(string) bool { return f.carries }, nil
}

func (f *fakeUitTests) RequiredPermission(_ context.Context, kind string) (string, error) {
	return f.perms[kind], nil
}

type fakeUitResolver struct{}

func (fakeUitResolver) Application(_ context.Context, code string) (*application.Application, error) {
	if code != "billing" {
		return nil, usecase.Validation("INGESTION_APPLICATION_INVALID", "no application with code "+code)
	}
	return &application.Application{ID: "app_a", Code: code}, nil
}

func (fakeUitResolver) ApplicationByID(context.Context, string) (*application.Application, error) {
	return nil, nil
}

func (fakeUitResolver) Client(_ context.Context, ref string) (*client.Client, error) {
	if ref != "acme" {
		return nil, usecase.Validation("INGESTION_CLIENT_INVALID", "no client "+ref)
	}
	return &client.Client{ID: "clt_a", Identifier: "acme"}, nil
}

func (fakeUitResolver) Target(context.Context, string, ids.ClientID, ingestion.Target) (ingestion.ResolvedTarget, error) {
	return ingestion.ResolvedTarget{}, nil
}

func uitTestDef(src ingestion.Source) ingestion.Definition {
	return ingestion.Definition{
		Name: "Orders", Source: src, Split: ingestion.None{},
		Target: ingestion.Target{Code: "billing:orders:order:received", TargetURL: "https://example.com/hook", Pool: "main", Mode: ingestion.ModeImmediate, BatchSize: 1},
	}
}

// uitTestHarness serves test connection with a form that names the subject:
// the definition's source kind is the form's "kind" field.
func uitTestHarness(store *fakeUitTests) *IngestionTests {
	return &IngestionTests{
		Store: store, Resolver: fakeUitResolver{}, Shell: shellForTest(),
		every: time.Millisecond, now: func() time.Time { return uitNow },
		Subject: func(r *http.Request) (IngestionTestSubject, error) {
			var src ingestion.Source = ingestion.Sqs{Region: "eu-west-1", Queue: "q"}
			if kind := r.PostFormValue("kind"); kind != "" {
				src = ingestion.Custom{Type: kind}
			}
			return IngestionTestSubject{Application: r.PostFormValue("application"), Client: r.PostFormValue("client"), Definition: uitTestDef(src)}, nil
		},
	}
}

func uitTestStore() *fakeUitTests {
	i := uitFixture(false)
	i.Definition = uitTestDef(ingestion.Sqs{Region: "eu-west-1", Queue: "q"})
	return &fakeUitTests{
		ings: map[string]*ingestion.Ingestion{"ing_one": i}, carries: true,
		perms: map[string]string{"camel-route": uitPermRoute},
	}
}

func uitTestPost(target string, form url.Values) req {
	if form == nil {
		form = url.Values{}
	}
	return req{method: http.MethodPost, target: target, datastar: true, multipart: true, form: form}
}

func TestIngestionTestPanelOffersTheButton(t *testing.T) {
	var b strings.Builder
	require.NoError(t, IngestionTestPanel("/ui/ingestions/ing_one/test", false).Render(context.Background(), &b))
	require.NoError(t, IngestionTestPanel("/ui/ingestions/test", true).Render(context.Background(), &b))
	mustContain(t, b.String(),
		`data-action="/ui/ingestions/ing_one/test"`, `data-on:click="@post(el.dataset.action, {retryMaxCount: 0})"`,
		`data-action="/ui/ingestions/test"`, `data-on:click="@post(el.dataset.action, {contentType: &#39;form&#39;, retryMaxCount: 0})"`,
		`id="ing-test-result"`, "Test connection", "nothing is dispatched or saved",
	)
}

func TestIngestionTestQueuesASavedIngestionAndFollowsIt(t *testing.T) {
	store := uitTestStore()
	rec := serveMounted(t, uitUser(uitPermTest), uitTestPost("/ui/ingestions/ing_one/test", nil), uitTestHarness(store))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	mustContain(t, rec.Body.String(),
		`id="ing-test-itr_one"`, `data-url="/ui/ingestion-tests/itr_one/stream"`, `data-init="@get(el.dataset.url)"`,
		"Waiting for a runtime to take the test", "selector #ing-test-result", "mode inner",
	)
	mustNotContain(t, rec.Body.String(), "No runtime carries this kind")
	require.Len(t, store.queued, 1)
	q := store.queued[0]
	require.NotNil(t, q.ingestionID)
	assert.Equal(t, "ing_one", *q.ingestionID)
	assert.Equal(t, ids.ApplicationID("app_a"), q.app)
	assert.Equal(t, "prn_ing", q.by)
}

func TestIngestionTestSaysAtOnceThatNoRuntimeCarriesTheKind(t *testing.T) {
	store := uitTestStore()
	store.carries = false
	rec := serveMounted(t, uitUser(uitPermTest), uitTestPost("/ui/ingestions/ing_one/test", nil), uitTestHarness(store))
	mustContain(t, rec.Body.String(), "No runtime carries this kind, so the test waits", `id="ing-test-notice"`)
}

func TestIngestionTestPermissionsAndReachOfASavedOne(t *testing.T) {
	post := uitTestPost("/ui/ingestions/ing_one/test", nil)
	for name, c := range map[string]struct {
		ac   *auth.AuthContext
		want string
	}{
		"view only":      {uitUser(uitPermView), "permission required"},
		"apply only":     {uitUser(uitPermApply), "permission required"},
		"nothing":        {uitUser(), "permission required"},
		"another client": {&auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_b"}, Roles: []string{"r"}, Permissions: []string{uitPermTest}, AllApplications: true, PrincipalType: auth.PrincipalTypeUser}, "no access to this resource"},
		"another app":    {&auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeAnchor, Roles: []string{"r"}, Permissions: []string{uitPermTest}, Applications: []string{"app_b"}, PrincipalType: auth.PrincipalTypeUser}, "no access to this ingestion"},
	} {
		store := uitTestStore()
		rec := serveMounted(t, c.ac, post, uitTestHarness(store))
		mustContain(t, rec.Body.String(), c.want)
		assert.Empty(t, store.queued, name)
	}
	// The ones that may.
	for name, ac := range map[string]*auth.AuthContext{"client": uitClientUser(uitPermTest), "application": uitAppUser(uitPermTest)} {
		store := uitTestStore()
		serveMounted(t, ac, post, uitTestHarness(store))
		assert.Len(t, store.queued, 1, name)
	}
	// Unknown and forged ids; a native post queues nothing.
	store := uitTestStore()
	for _, target := range []string{"/ui/ingestions/ing_none/test", "/ui/ingestions/x'y/test"} {
		rec := serveMounted(t, uitUser(uitPermTest), req{method: http.MethodPost, target: target, multipart: true, form: url.Values{}}, uitTestHarness(store))
		assert.Equal(t, http.StatusNotFound, rec.Code, target)
	}
	rec := serveMounted(t, uitUser(uitPermTest), req{method: http.MethodPost, target: "/ui/ingestions/ing_one/test", form: url.Values{}}, uitTestHarness(store))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Empty(t, store.queued)
}

func uitState(status string, mod func(*ingestion.TestRequest)) ingestion.TestRequest {
	t := ingestion.TestRequest{
		ID: "itr_one", ApplicationID: "app_a", ClientID: "clt_a", Kind: "sqs", Status: status,
		CreatedAt: uitNow, ExpiresAt: uitNow.Add(ingestion.TestRequestTTL),
	}
	if mod != nil {
		mod(&t)
	}
	return t
}

func uitQueuedRow(store *fakeUitTests) {
	store.rows = map[string]*ingestion.TestRequest{"itr_one": new(uitState(ingestion.TestPending, nil))}
}

func TestIngestionTestStreamShowsTheStepsAsTheyArrive(t *testing.T) {
	store := uitTestStore()
	uitQueuedRow(store)
	records := int64(3)
	excerpt := "{\"order\": <b>1</b>}"
	rt := "rt-7"
	store.script = []ingestion.TestRequest{
		uitState(ingestion.TestPending, nil),
		uitState(ingestion.TestRunning, func(t *ingestion.TestRequest) { t.ClaimedBy = &rt }),
		uitState(ingestion.TestDone, func(t *ingestion.TestRequest) {
			t.ClaimedBy = &rt
			t.Result = &ingestion.TestResult{Outcome: "RESULTS", Steps: []string{"Logged in <b>ok</b>", "Read 3 messages"}, Records: &records, Excerpt: &excerpt}
		}),
	}
	rec := serveMounted(t, uitUser(uitPermTest), req{target: "/ui/ingestion-tests/itr_one/stream", datastar: true}, uitTestHarness(store))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body := rec.Body.String()
	mustContain(t, body,
		"Waiting for a runtime to take the test", "is trying the source", "rt-7",
		`fc-tag-success">Logged in, results`, "on rt-7", "Logged in &lt;b&gt;ok&lt;/b&gt;", "Read 3 messages",
		"The split makes 3 record(s) of the answer.", "{&#34;order&#34;: &lt;b&gt;1&lt;/b&gt;}",
	)
	mustNotContain(t, body, "<b>ok</b>", "<b>1</b>", "No runtime answered in time")
	// In the order they arrived, and the stream ended with the answer (no read after it).
	assert.Less(t, strings.Index(body, "Waiting for a runtime"), strings.Index(body, "is trying the source"))
	assert.Less(t, strings.Index(body, "is trying the source"), strings.Index(body, "Logged in, results"))
	assert.Equal(t, 3, store.reads)
}

func TestIngestionTestStreamEndsExpiredAfterTwoMinutes(t *testing.T) {
	store := uitTestStore()
	uitQueuedRow(store)
	now := uitNow
	var mu sync.Mutex
	h := uitTestHarness(store)
	h.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	// The second read happens after the test's two minutes are up.
	h.Store = &advancingUitStore{fakeUitTests: store, onRead: func(n int) {
		if n == 2 {
			mu.Lock()
			now = uitNow.Add(ingestion.TestRequestTTL)
			mu.Unlock()
		}
	}}
	rec := serveMounted(t, uitUser(uitPermTest), req{target: "/ui/ingestion-tests/itr_one/stream", datastar: true}, h)
	body := rec.Body.String()
	mustContain(t, body, "Waiting for a runtime to take the test", "No runtime answered in time.", "A test waits two minutes")
	mustNotContain(t, body, "Logged in")
}

type advancingUitStore struct {
	*fakeUitTests
	n      int
	onRead func(int)
}

func (a *advancingUitStore) FindTest(ctx context.Context, id string) (*ingestion.TestRequest, error) {
	a.n++
	a.onRead(a.n)
	return a.fakeUitTests.FindTest(ctx, id)
}

func TestIngestionTestStreamIsForWhoMayTestAndReachTheTest(t *testing.T) {
	for _, c := range map[string]struct {
		ac   *auth.AuthContext
		id   string
		want string
	}{
		"view only":      {uitUser(uitPermView), "itr_one", "permission required"},
		"another client": {&auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_b"}, Roles: []string{"r"}, Permissions: []string{uitPermTest}, AllApplications: true, PrincipalType: auth.PrincipalTypeUser}, "itr_one", "no access to this resource"},
		"another app":    {&auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeAnchor, Roles: []string{"r"}, Permissions: []string{uitPermTest}, Applications: []string{"app_b"}, PrincipalType: auth.PrincipalTypeUser}, "itr_one", "no access to this ingestion"},
		"unknown":        {uitUser(uitPermTest), "itr_none", "Ingestion test not found"},
	} {
		store := uitTestStore()
		uitQueuedRow(store)
		store.script = []ingestion.TestRequest{uitState(ingestion.TestDone, func(t *ingestion.TestRequest) {
			t.Result = &ingestion.TestResult{Outcome: "FAILED", Steps: []string{"SECRET-STEP"}}
		})}
		rec := serveMounted(t, c.ac, req{target: "/ui/ingestion-tests/" + c.id + "/stream", datastar: true}, uitTestHarness(store))
		mustContain(t, rec.Body.String(), c.want)
		mustNotContain(t, rec.Body.String(), "SECRET-STEP")
	}
	rec := serveMounted(t, uitUser(uitPermTest), req{target: "/ui/ingestion-tests/x'y/stream", datastar: true}, uitTestHarness(uitTestStore()))
	mustContain(t, rec.Body.String(), "Ingestion test not found")
}

func TestIngestionTestOfAnUnsavedDefinition(t *testing.T) {
	post := func(ac *auth.AuthContext, form url.Values) (*fakeUitTests, string) {
		store := uitTestStore()
		rec := serveMounted(t, ac, uitTestPost("/ui/ingestions/test", form), uitTestHarness(store))
		return store, rec.Body.String()
	}
	valid := url.Values{"application": {"billing"}, "client": {"ACME"}}

	store, body := post(uitUser(uitPermTest), valid)
	mustContain(t, body, `id="ing-test-itr_one"`)
	require.Len(t, store.queued, 1)
	assert.Nil(t, store.queued[0].ingestionID)
	assert.Equal(t, ids.ClientID("clt_a"), store.queued[0].client)

	// No test permission: nothing queued.
	store, body = post(uitUser(uitPermView), valid)
	mustContain(t, body, "permission required")
	assert.Empty(t, store.queued)

	// The client is another tenant's, the application is not the caller's, or
	// either does not exist.
	store, body = post(&auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_b"}, Roles: []string{"r"}, Permissions: []string{uitPermTest}, AllApplications: true, PrincipalType: auth.PrincipalTypeUser}, valid)
	mustContain(t, body, "no access to this resource")
	assert.Empty(t, store.queued)
	store, _ = post(uitAppUser(uitPermTest), url.Values{"application": {"billing"}, "client": {"acme"}})
	assert.Len(t, store.queued, 1, "billing is app_a")
	other := uitAppUser(uitPermTest)
	other.Applications = []string{"app_b"}
	store, body = post(other, valid)
	mustContain(t, body, "no access to this ingestion")
	assert.Empty(t, store.queued)
	store, body = post(uitUser(uitPermTest), url.Values{"application": {"billing"}, "client": {"nobody"}})
	mustContain(t, body, "no client nobody")
	assert.Empty(t, store.queued)
	store, body = post(uitUser(uitPermTest), url.Values{"application": {"nope"}, "client": {"acme"}})
	mustContain(t, body, "no application with code nope")
	assert.Empty(t, store.queued)
}

func TestIngestionTestOfAKindThatNeedsAPermission(t *testing.T) {
	form := url.Values{"application": {"billing"}, "client": {"acme"}, "kind": {"camel-route"}}
	for name, c := range map[string]struct {
		ac     *auth.AuthContext
		queued int
	}{
		"test only":       {uitUser(uitPermTest), 0},
		"route only":      {uitUser(uitPermRoute), 0},
		"test and route":  {uitUser(uitPermTest, uitPermRoute), 1},
		"super admin":     {uitAnchorAdmin(), 1},
		"view, test, set": {uitUser(uitPermView, uitPermTest, uitPermApply), 0},
	} {
		store := uitTestStore()
		rec := serveMounted(t, c.ac, uitTestPost("/ui/ingestions/test", form), uitTestHarness(store))
		assert.Len(t, store.queued, c.queued, name)
		if c.queued == 0 && c.ac.HasPermission(uitPermTest) {
			mustContain(t, rec.Body.String(), "permission required for source kind camel-route: "+uitPermRoute)
		}
	}
}

func uitAnchorAdmin() *auth.AuthContext {
	ac := anchorAdmin()
	ac.AllApplications = true
	return ac
}

func TestIngestionTestOfAnInvalidDefinitionQueuesNothing(t *testing.T) {
	store := uitTestStore()
	h := uitTestHarness(store)
	h.Subject = func(*http.Request) (IngestionTestSubject, error) {
		return IngestionTestSubject{Application: "billing", Client: "acme", Definition: ingestion.Definition{}}, nil
	}
	rec := serveMounted(t, uitUser(uitPermTest), uitTestPost("/ui/ingestions/test", nil), h)
	assert.Empty(t, store.queued)
	mustContain(t, rec.Body.String(), "name")

	// Without a way to read the form, the route is not there.
	h = uitTestHarness(store)
	h.Subject = nil
	rec = serveMounted(t, uitUser(uitPermTest), uitTestPost("/ui/ingestions/test", nil), h)
	assert.NotEqual(t, http.StatusOK, rec.Code)
	assert.Empty(t, store.queued)
}
