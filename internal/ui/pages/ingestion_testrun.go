package pages

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	ingops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// IngestionTestStore is what test connection reads and writes;
// *ingestion.Repository satisfies it.
type IngestionTestStore interface {
	FindByID(ctx context.Context, id string) (*ingestion.Ingestion, error)
	QueueTest(ctx context.Context, ingestionID *string, applicationID ids.ApplicationID, clientID ids.ClientID, def ingestion.Definition, requestedBy *string, now time.Time) (*ingestion.TestRequest, error)
	FindTest(ctx context.Context, id string) (*ingestion.TestRequest, error)
	AnyLiveRuntimeCarries(ctx context.Context) (func(kind string) bool, error)
	// ApplicationCode and Resolve read the config entry a definition's source
	// names (docs/ingestion-design.md section 13): a runtime is given the
	// effective definition. CheckAuthor is the check a save makes on a
	// definition (the kind's permission, the runtime identity, the secrets it
	// points), which trying an unsaved one needs as well.
	ApplicationCode(ctx context.Context, id ids.ApplicationID) (string, error)
	Resolve(ctx context.Context, applicationCode string, clientID ids.ClientID, code string, def ingestion.Definition) (ingestion.Resolved, error)
	CheckAuthor(ctx context.Context, ac *auth.AuthContext, c ingestion.Change) error
}

// IngestionTestSubject is what a test of an unsaved definition is of: the
// definition the form holds, with the application and client it would belong
// to (POST /api/ingestion-tests with a definition).
type IngestionTestSubject struct {
	Application, Client string
	// Code is the ingestion's code when the form has one: the entry `config:
	// true` names is the code's.
	Code       string
	Definition ingestion.Definition
}

// IngestionTests is "test connection": a button queues a test, a runtime logs
// in and asks once with its own secrets and from its own network, and the
// outcome is shown as it arrives over a Datastar stream that polls the test
// row. Every route makes the API handlers' checks: the test permission (POST
// and GET /api/ingestion-tests), the ingestion's reach, and for a definition
// that is not saved the same checks a save makes (validity, the client and
// application reach, the permission a custom kind requires).
type IngestionTests struct {
	Store    IngestionTestStore
	Resolver ingestion.Resolver
	// Subject reads the form of an unsaved definition's test; nil leaves out
	// the route (only saved ingestions can be tested).
	Subject func(r *http.Request) (IngestionTestSubject, error)
	Shell   *shell.Shell

	// every is the poll interval of the stream and now its clock; zero values
	// are the defaults (1 s, the wall clock).
	every time.Duration
	now   func() time.Time
}

const (
	ingTestResultID = "ing-test-result"
	ingTestBase     = "/ui/ingestion-tests"

	ingTestDefaultEvery = time.Second
)

func (h *IngestionTests) clock() time.Time {
	if h.now != nil {
		return h.now()
	}
	return time.Now().UTC()
}

func (h *IngestionTests) interval() time.Duration {
	if h.every > 0 {
		return h.every
	}
	return ingTestDefaultEvery
}

// Mount registers the routes (POSTs behind parseForm). The unsaved route is
// registered beside {id} routes of the ingestion pages: a literal segment
// wins over a parameter in chi.
func (h *IngestionTests) Mount(r chi.Router) {
	r.Get("/ingestion-tests/{id}/stream", h.stream)
	post := r.With(parseForm)
	post.Post("/ingestions/{id}/test", h.testSaved)
	if h.Subject != nil {
		post.Post("/ingestions/test", h.testUnsaved)
	}
}

// ── queueing ───────────────────────────────────────────────────────────

// testSaved tests a saved ingestion's live definition (POST
// /api/ingestion-tests with ingestionId).
func (h *IngestionTests) testSaved(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanTestIngestions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		h.Shell.Fail(w, r, httperror.NotFound("Ingestion", id))
		return
	}
	i, err := h.Store.FindByID(ctx, id)
	if err == nil && i == nil {
		err = httperror.NotFound("Ingestion", id)
	} else if err != nil {
		err = usecase.Internal("REPO", "find_by_id failed", err)
	}
	if err == nil {
		err = ingestion.CheckReach(ac, i)
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// The runtime is given the effective definition: the settings of the config
	// entry the source names, when it names one.
	app, err := h.Store.ApplicationCode(ctx, i.ApplicationID)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "application lookup failed", err))
		return
	}
	def, err := h.testable(ctx, app, i.ClientID, i.Code, i.Definition)
	if err == nil {
		// A definition saved before the outbound guard existed is held to it
		// here: a test sends the runtime where the source points.
		err = ingestion.CheckSourceAddresses(def.Source).Err()
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.queue(w, r, ac, &i.ID, i.ApplicationID, i.ClientID, def)
}

// testable is the definition a runtime is given to test: def with the settings
// of its config entry. An entry that is missing or invalid stops the test (a
// test needs the settings to try anything), as the API's does.
func (h *IngestionTests) testable(ctx context.Context, applicationCode string, clientID ids.ClientID, code string, def ingestion.Definition) (ingestion.Definition, error) {
	c, byConfig := ingestion.ConfigOf(def)
	if !byConfig {
		return def, nil
	}
	if c.Config.Own && code == "" {
		return def, usecase.Validation("INGESTION_CONFIG_KEY_UNKNOWN",
			"the source says config: true, the ingestion's code, which this test was not given; name the key instead")
	}
	res, err := h.Store.Resolve(ctx, applicationCode, clientID, code, def)
	if err != nil {
		return def, usecase.Internal("REPO", "config entry lookup failed", err)
	}
	switch res.Status {
	case ingestion.ConfigMissing:
		return def, usecase.Validation("INGESTION_CONFIG_MISSING",
			"the config entry "+res.Key+" is not set for this client; set it on the ingestion (Settings, this environment) before testing")
	case ingestion.ConfigInvalid:
		return def, ingops.EntryProblems(res).Err()
	case ingestion.ConfigInline, ingestion.ConfigSet:
	}
	return res.Definition, nil
}

// testUnsaved tests the definition a form holds, without saving it.
func (h *IngestionTests) testUnsaved(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanTestIngestions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	subject, err := h.Subject(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	def := subject.Definition
	if err := def.Validate().Err(); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	c, err := h.Resolver.Client(ctx, ingClientRef(subject.Client))
	if err == nil {
		err = auth.CheckScopeAccess(ac, new(string(c.ID)))
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	app, err := h.Resolver.Application(ctx, strings.TrimSpace(subject.Application))
	if err == nil {
		err = ingestion.CheckApplicationReach(ac, string(app.ID))
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// The entry a source names is read now, and the runtime is given the
	// effective definition; an entry that is not there stops the test.
	if def, err = h.testable(ctx, app.Code, c.ID, strings.TrimSpace(subject.Code), def); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// Trying an unsaved definition runs it on a runtime, so it needs what
	// creating it would (the API's test makes the same check): the kind's
	// permission, the runtime identity, the secrets it points.
	if err := h.Store.CheckAuthor(ctx, ac, ingestion.Change{Next: def}); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.queue(w, r, ac, nil, app.ID, c.ID, def)
}

// ingClientRef is a client reference as the save path reads it: an id as it
// is, an identifier in lower case.
func ingClientRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if _, err := ids.ParseClientID(ref); err != nil {
		return strings.ToLower(ref)
	}
	return ref
}

// queue queues the test and answers with its first state, which asks for the
// stream. A native post has no panel to show a stream in: nothing is queued,
// the browser goes back to the ingestions.
func (h *IngestionTests) queue(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, ingestionID *string, app ids.ApplicationID, c ids.ClientID, def ingestion.Definition) {
	ctx := r.Context()
	if !ds.IsDatastar(r) {
		http.Redirect(w, r, ingTabBase, http.StatusSeeOther)
		return
	}
	by := ac.PrincipalID
	t, err := h.Store.QueueTest(ctx, ingestionID, app, c, def, &by, h.clock())
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "queue_test failed", err))
		return
	}
	v, err := h.view(ctx, t)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v.Follow = true
	if err := ds.NewSSE(w, r).Patch(ingTestBox(v), ds.Inner("#"+ingTestResultID)); err != nil {
		slog.DebugContext(ctx, "ui: ingestion test patch not delivered", "err", err)
	}
}

// ── the view ───────────────────────────────────────────────────────────

type ingTestView struct {
	ID        string
	StreamURL string
	// Follow makes the box ask for the stream when it is inserted; the
	// stream's own patches do not.
	Follow bool
	Status string
	// Runtime took the test.
	Runtime string
	// Notice is set while no live runtime carries the kind: the test waits.
	Notice  string
	Outcome string
	Label   string
	// Severity colours the outcome.
	Severity kit.TagSeverity
	Steps    []string
	Records  string
	Excerpt  string
}

// Terminal is a test that will not change any more.
func (v ingTestView) Terminal() bool {
	return v.Status == ingestion.TestDone || v.Status == ingestion.TestExpired
}

// ingTestOutcomes are the outcomes, for people (the Nuxt control plane's
// LABELS and COLORS).
var ingTestOutcomes = map[string]struct {
	Label    string
	Severity kit.TagSeverity
}{
	"RESULTS":       {"Logged in, results", kit.TagSuccess},
	"NO_RESULTS":    {"Logged in, no results", kit.TagInfo},
	"REFUSED":       {"Login refused", kit.TagDanger},
	"FAILED":        {"Failed", kit.TagDanger},
	"NOT_SUPPORTED": {"Not offered", kit.TagSecondary},
}

const ingNoRuntimeNotice = "No runtime carries this kind, so the test waits: nothing has failed, nobody is there to run it yet."

// view is the test as the API's testResponse describes it: its effective
// status (expired once the two minutes are up), the notice while no runtime
// carries the kind, and the result once a runtime has answered.
func (h *IngestionTests) view(ctx context.Context, t *ingestion.TestRequest) (ingTestView, error) {
	carries, err := h.Store.AnyLiveRuntimeCarries(ctx)
	if err != nil {
		return ingTestView{}, usecase.Internal("REPO", "live runtimes failed", err)
	}
	v := ingTestView{
		ID: t.ID, StreamURL: ingTestBase + "/" + t.ID + "/stream", Status: t.EffectiveStatus(h.clock()), Runtime: deref(t.ClaimedBy),
	}
	if v.Status == ingestion.TestPending && !carries(t.Kind) {
		v.Notice = ingNoRuntimeNotice
	}
	if t.Result != nil {
		v.Outcome, v.Steps, v.Excerpt = t.Result.Outcome, t.Result.Steps, deref(t.Result.Excerpt)
		o := ingTestOutcomes[t.Result.Outcome]
		v.Label, v.Severity = o.Label, o.Severity
		if v.Label == "" {
			v.Label, v.Severity = t.Result.Outcome, kit.TagSecondary
		}
		if t.Result.Records != nil {
			v.Records = strconv.FormatInt(*t.Result.Records, 10)
		}
	}
	return v, nil
}

// ── the stream ─────────────────────────────────────────────────────────

// stream follows a test: every interval it reads the row again and patches
// the box when something changed, and it ends when the test is done or has
// expired (a test row expires two minutes after it was queued, so the stream
// is bounded by that) or the viewer goes away (GET /api/ingestion-tests/{id}).
func (h *IngestionTests) stream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id := chi.URLParam(r, "id")
	err := auth.CanTestIngestions(ac)
	if err == nil && !kit.ValidID(id) {
		err = usecase.NotFound("INGESTION_TEST_NOT_FOUND", "Ingestion test not found: "+id)
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	t, err := h.find(ctx, ac, id)
	var v ingTestView
	if err == nil {
		v, err = h.view(ctx, t)
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		http.Redirect(w, r, ingTabBase, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.Patch(ingTestBox(v)); err != nil {
		slog.DebugContext(ctx, "ui: ingestion test patch not delivered", "err", err)
		return
	}
	tick := time.NewTicker(h.interval())
	defer tick.Stop()
	for !v.Terminal() {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		t, err := h.find(ctx, ac, id)
		if err != nil {
			h.Shell.FailSSE(sse, r, err)
			return
		}
		next, err := h.view(ctx, t)
		if err != nil {
			h.Shell.FailSSE(sse, r, err)
			return
		}
		if next.sameAs(v) {
			continue
		}
		v = next
		if err := sse.Patch(ingTestBox(v)); err != nil {
			return
		}
	}
}

// sameAs reports whether nothing the box shows differs.
func (v ingTestView) sameAs(o ingTestView) bool {
	return v.Status == o.Status && v.Runtime == o.Runtime && v.Notice == o.Notice && v.Outcome == o.Outcome
}

// find reads a test with the API's checks: the client's and the application's
// reach.
func (h *IngestionTests) find(ctx context.Context, ac *auth.AuthContext, id string) (*ingestion.TestRequest, error) {
	t, err := h.Store.FindTest(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_test failed", err)
	}
	if t == nil {
		return nil, usecase.NotFound("INGESTION_TEST_NOT_FOUND", "Ingestion test not found: "+id)
	}
	if err := auth.CheckScopeAccess(ac, new(string(t.ClientID))); err != nil {
		return nil, err
	}
	if err := ingestion.CheckApplicationReach(ac, string(t.ApplicationID)); err != nil {
		return nil, err
	}
	return t, nil
}

// ingTestExpr is the button's handler.
func ingTestExpr(fromForm bool) string {
	if fromForm {
		return ingTestFormExpr
	}
	return ingTestSavedExpr
}
