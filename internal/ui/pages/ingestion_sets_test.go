package pages

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

type fakeUitSets struct {
	sets    []ingestion.Set
	applies map[string][]ingestion.SetApply // by set id, newest first
	files   map[string][]ingestion.SetFile  // by apply id
}

func (f *fakeUitSets) ListSets(context.Context) ([]ingestion.Set, error) { return f.sets, nil }

func (f *fakeUitSets) FindSetByName(_ context.Context, name string) (*ingestion.Set, error) {
	for _, s := range f.sets {
		if s.Name == name {
			return &s, nil
		}
	}
	return nil, nil
}

func (f *fakeUitSets) ListSetApplies(_ context.Context, setID string) ([]ingestion.SetApply, error) {
	return f.applies[setID], nil
}

func (f *fakeUitSets) FindSetApply(_ context.Context, setID, applyID string) (*ingestion.SetApply, error) {
	for _, a := range f.applies[setID] {
		if a.ID == applyID {
			return &a, nil
		}
	}
	return nil, nil
}

func (f *fakeUitSets) ApplyFiles(_ context.Context, a ingestion.SetApply) ([]ingestion.SetFile, error) {
	return f.files[a.ID], nil
}

type uitSetCall struct {
	op, name, apply string
	dryRun          bool
}

type fakeUitSetOps struct {
	calls []uitSetCall
	plan  IngestionSetPlan
	err   error
}

func (o *fakeUitSetOps) Reapply(_ context.Context, name, applyID string, dryRun bool) (IngestionSetPlan, error) {
	o.calls = append(o.calls, uitSetCall{"reapply", name, applyID, dryRun})
	return o.plan, o.err
}

func (o *fakeUitSetOps) Release(_ context.Context, name string) (int, error) {
	o.calls = append(o.calls, uitSetCall{op: "release", name: name})
	return 2, o.err
}

func uitSetsFixture() *fakeUitSets {
	id := "ing_one"
	apply := func(id, by string, goingBack *string, changes ...ingestion.SetChange) ingestion.SetApply {
		return ingestion.SetApply{
			ID: id, SetID: "ist_1", AppliedBy: &by, AppliedAt: uitNow.Add(-time.Hour),
			Summary: ingestion.SetApplySummary{Items: 2, Files: 1, GoingBackTo: goingBack, Changes: changes},
		}
	}
	back := "isa_2"
	return &fakeUitSets{
		sets: []ingestion.Set{
			{ID: "ist_1", Name: "billing", Ingestions: 2, UpdatedAt: uitNow},
			{ID: "ist_2", Name: "9-ops", Ingestions: 0, UpdatedAt: uitNow},
		},
		applies: map[string][]ingestion.SetApply{"ist_1": {
			apply("isa_3", "prn_ada", &back, ingestion.SetChange{Client: "acme", Code: "orders", Action: ingestion.SetChanged, IngestionID: &id}),
			apply("isa_2", "prn_bob",
				nil,
				ingestion.SetChange{Client: "acme", Code: "orders", Action: ingestion.SetUnchanged, IngestionID: &id},
				ingestion.SetChange{Client: "acme", Code: "<b>gone</b>", Action: ingestion.SetRemove, IngestionID: new("ing_gone")},
				ingestion.SetChange{Client: "acme", Code: "fresh", Action: ingestion.SetCreate, IngestionID: new("ing_fresh")}),
			apply("isa_1", "prn_bob", nil),
		}},
		files: map[string][]ingestion.SetFile{"isa_2": {{Path: "ingest/orders.yaml", Content: "name: <script>x</script>\nqueue: q"}}},
	}
}

func newUitSetsPage(f *fakeUitSets, ops *fakeUitSetOps) *IngestionSets {
	names := map[string]string{"prn_ada": "Ada Lovelace"}
	return &IngestionSets{
		Sets: f, Ops: ops, Shell: shellForTest(),
		Names: func(_ context.Context, id string) string { return names[id] },
	}
}

func TestIngestionSetsList(t *testing.T) {
	rec := serveMounted(t, uitUser(uitPermView), req{target: "/ui/ingestion-sets"}, newUitSetsPage(uitSetsFixture(), &fakeUitSetOps{}))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body := rec.Body.String()
	mustContain(t, body,
		`<title>Ingestion sets · FlowCatalyst</title>`, `<h1 class="fc-page-title">Ingestion sets</h1>`,
		// The read-only note: applying is done with fcdev.
		"fcdev ingest apply",
		`href="/ui/ingestion-sets/billing"`, `href="/ui/ingestion-sets/9-ops"`, `id="ing-sets"`,
	)
	mustNotContain(t, body, `id="ing-set-detail"><section`, "Go back to this", "Release the set")

	rec = serveMounted(t, uitUser(uitPermView), req{target: "/ui/ingestion-sets"}, newUitSetsPage(&fakeUitSets{}, &fakeUitSetOps{}))
	mustContain(t, rec.Body.String(), "No set manages an ingestion yet.")
}

func TestIngestionSetsPermissionMatrix(t *testing.T) {
	page := newUitSetsPage(uitSetsFixture(), &fakeUitSetOps{})
	for _, target := range []string{"/ui/ingestion-sets", "/ui/ingestion-sets/billing"} {
		// Reading needs the view permission and a caller that reaches every
		// client and application.
		assert.Equal(t, http.StatusOK, serveMounted(t, uitUser(uitPermView), req{target: target}, page).Code, target)
		assert.Equal(t, http.StatusForbidden, serveMounted(t, uitUser(uitPermApply), req{target: target}, page).Code, target+" without view")
		assert.Equal(t, http.StatusForbidden, serveMounted(t, uitClientUser(uitPermView), req{target: target}, page).Code, target+" client tier")
		assert.Equal(t, http.StatusForbidden, serveMounted(t, uitAppUser(uitPermView), req{target: target}, page).Code, target+" application scoped")
	}
	rec := serveMounted(t, uitClientUser(uitPermView), req{target: "/ui/ingestion-sets"}, page)
	mustContain(t, rec.Body.String(), "sets span clients and applications")
	mustNotContain(t, rec.Body.String(), "billing")
}

func TestIngestionSetAppliesPage(t *testing.T) {
	page := newUitSetsPage(uitSetsFixture(), &fakeUitSetOps{})
	rec := serveMounted(t, uitUser(uitPermView), req{target: "/ui/ingestion-sets/billing"}, page)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	body := rec.Body.String()
	mustContain(t, body,
		"Applies of billing", `id="ing-apply-isa_3"`, `id="ing-apply-isa_2"`, `id="ing-apply-isa_1"`,
		"Newest", "going back to apply isa_2", "by Ada Lovelace", "by prn_bob", "2 definition(s), 1 file(s)",
		// Only what an apply did is listed, an unchanged item is not; a change
		// opens its ingestion, a removed one has nothing to open.
		`fc-tag-info">CHANGE`, `fc-tag-success">CREATE`, `fc-tag-danger">REMOVE`,
		`href="/ui/ingestions/ing_one"`, `href="/ui/ingestions/ing_fresh"`, "acme/fresh", "acme/&lt;b&gt;gone&lt;/b&gt;",
		`data-url="/ui/ingestion-sets/billing/applies/isa_2/files"`, "Show files",
	)
	mustNotContain(t, body, `href="/ui/ingestions/ing_gone"`, "<b>gone</b>", "UNCHANGED",
		// A viewer may not go back or release.
		"Go back to this", "Release the set", `id="ing-set-release"`)

	// A set that applied nothing is said so; an unknown set is not found.
	rec = serveMounted(t, uitUser(uitPermView), req{target: "/ui/ingestion-sets/9-ops"}, page)
	mustContain(t, rec.Body.String(), "No apply of this set is kept.")
	assert.Equal(t, http.StatusNotFound, serveMounted(t, uitUser(uitPermView), req{target: "/ui/ingestion-sets/nothing"}, page).Code)
	assert.Equal(t, http.StatusNotFound, serveMounted(t, uitUser(uitPermView), req{target: "/ui/ingestion-sets/Not_A_Name"}, page).Code)
}

func TestIngestionSetGoBackIsOfferedToWhoMayApply(t *testing.T) {
	page := newUitSetsPage(uitSetsFixture(), &fakeUitSetOps{})
	rec := serveMounted(t, uitUser(uitPermView, uitPermApply), req{target: "/ui/ingestion-sets/billing"}, page)
	body := rec.Body.String()
	// On every apply but the newest.
	assert.Equal(t, 2, strings.Count(body, "Go back to this"))
	mustContain(t, body,
		`href="/ui/ingestion-sets/billing/applies/isa_2/reapply"`, `href="/ui/ingestion-sets/billing/applies/isa_1/reapply"`,
		"Release the set", `data-action="/ui/ingestion-sets/billing/release"`, `id="ing-set-release"`,
	)
	mustNotContain(t, body, `applies/isa_3/reapply`)
	// A set that owns nothing has nothing to release.
	rec = serveMounted(t, uitUser(uitPermView, uitPermApply), req{target: "/ui/ingestion-sets/9-ops"}, page)
	mustNotContain(t, rec.Body.String(), "Release the set")
}

func TestIngestionSetFilesLoadLazily(t *testing.T) {
	page := newUitSetsPage(uitSetsFixture(), &fakeUitSetOps{})
	base := "/ui/ingestion-sets/billing/applies/"
	rec := serveMounted(t, uitUser(uitPermView), req{target: base + "isa_2/files", datastar: true}, page)
	require.Equal(t, http.StatusOK, rec.Code)
	mustContain(t, rec.Body.String(), `id="apply-files-isa_2"`, "ingest/orders.yaml", "name: &lt;script&gt;x&lt;/script&gt;")
	mustNotContain(t, rec.Body.String(), "<script>x</script>")

	rec = serveMounted(t, uitUser(uitPermView), req{target: base + "isa_1/files", datastar: true}, page)
	mustContain(t, rec.Body.String(), "No files were sent with this apply")

	// A navigation goes to the set; an apply the set does not keep is not found;
	// the permission matrix is the page's.
	rec = serveMounted(t, uitUser(uitPermView), req{target: base + "isa_2/files"}, page)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/ui/ingestion-sets/billing", rec.Header().Get("Location"))
	assert.Equal(t, http.StatusNotFound, serveMounted(t, uitUser(uitPermView), req{target: base + "x'y/files"}, page).Code)
	rec = serveMounted(t, uitUser(uitPermView), req{target: base + "isa_9/files", datastar: true}, page)
	mustContain(t, rec.Body.String(), "the set does not keep apply isa_9")
	assert.Equal(t, http.StatusForbidden, serveMounted(t, uitClientUser(uitPermView), req{target: base + "isa_2/files"}, page).Code)
	assert.Equal(t, http.StatusForbidden, serveMounted(t, uitUser(), req{target: base + "isa_2/files"}, page).Code)
}

func TestIngestionSetGoBackShowsThePlanFirst(t *testing.T) {
	ops := &fakeUitSetOps{plan: IngestionSetPlan{Changes: []ingestion.SetChange{
		{Client: "acme", Code: "orders", Action: ingestion.SetChanged},
		{Client: "acme", Code: "same", Action: ingestion.SetUnchanged},
		{Client: "acme", Code: "old", Action: ingestion.SetRemove},
	}}}
	page := newUitSetsPage(uitSetsFixture(), ops)
	target := "/ui/ingestion-sets/billing/applies/isa_2/reapply"
	rec := serveMounted(t, uitUser(uitPermView, uitPermApply), req{target: target, datastar: true}, page)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	mustContain(t, rec.Body.String(),
		`id="ing-set-dialog"`, "Go back to apply isa_2 of billing?", `action="`+target+`"`,
		"acme/orders", "acme/old", `fc-tag-danger">REMOVE`,
		"This lasts until the set is applied again; to keep it, change the set", "s files back too.", ">Go back</button>", "mode replace",
	)
	mustNotContain(t, rec.Body.String(), "acme/same")
	// The plan is a dry run: nothing was applied by showing it.
	assert.Equal(t, []uitSetCall{{op: "reapply", name: "billing", apply: "isa_2", dryRun: true}}, ops.calls)

	// Nothing to do: only Close.
	ops = &fakeUitSetOps{}
	rec = serveMounted(t, uitUser(uitPermApply), req{target: target, datastar: true}, newUitSetsPage(uitSetsFixture(), ops))
	mustContain(t, rec.Body.String(), "already is as apply isa_2 left it")
	mustNotContain(t, rec.Body.String(), ">Go back</button>")

	// A navigation to it goes to the set, without reading anything.
	ops = &fakeUitSetOps{}
	rec = serveMounted(t, uitUser(uitPermApply), req{target: target}, newUitSetsPage(uitSetsFixture(), ops))
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Empty(t, ops.calls)
}

func TestIngestionSetForgedPostsChangeNothing(t *testing.T) {
	post := func(target string) req {
		return req{method: http.MethodPost, target: target, datastar: true, multipart: true, form: url.Values{}}
	}
	reapply := "/ui/ingestion-sets/billing/applies/isa_2/reapply"
	release := "/ui/ingestion-sets/billing/release"
	for name, q := range map[string]req{
		"reapply": post(reapply), "reapply dry run": {target: reapply, datastar: true}, "release": post(release),
	} {
		ops := &fakeUitSetOps{}
		page := newUitSetsPage(uitSetsFixture(), ops)
		for who, perms := range map[string][]string{"view only": {uitPermView}, "test only": {uitPermTest}, "nothing": nil} {
			rec := serveMounted(t, uitUser(perms...), q, page)
			assert.Equal(t, http.StatusOK, rec.Code, name+" "+who+": the error contract answers 200 with a flash")
			mustContain(t, rec.Body.String(), "permission required")
		}
		assert.Empty(t, ops.calls, name)
	}

	// A path that names no set or apply is not found and runs nothing.
	ops := &fakeUitSetOps{}
	page := newUitSetsPage(uitSetsFixture(), ops)
	for _, target := range []string{"/ui/ingestion-sets/Bad_Name/release", "/ui/ingestion-sets/billing/applies/x'y/reapply"} {
		rec := serveMounted(t, uitUser(uitPermApply), req{method: http.MethodPost, target: target, multipart: true, form: url.Values{}}, page)
		assert.Equal(t, http.StatusNotFound, rec.Code, target)
	}
	assert.Empty(t, ops.calls)
}

func TestIngestionSetReapplyAndRelease(t *testing.T) {
	ops := &fakeUitSetOps{plan: IngestionSetPlan{ApplyID: new("isa_4")}}
	page := newUitSetsPage(uitSetsFixture(), ops)
	ac := uitUser(uitPermView, uitPermApply)
	post := func(target string, datastar bool) req {
		return req{method: http.MethodPost, target: target, datastar: datastar, multipart: datastar, form: url.Values{}}
	}

	rec := serveMounted(t, ac, post("/ui/ingestion-sets/billing/applies/isa_2/reapply", true), page)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	mustContain(t, rec.Body.String(), "Set billing is back as apply isa_2 left it.", `id="ing-sets"`, `id="ing-set-detail"`, "Applies of billing")
	assert.Equal(t, []uitSetCall{{op: "reapply", name: "billing", apply: "isa_2"}}, ops.calls)

	// An apply that changed nothing says so.
	ops.plan = IngestionSetPlan{}
	rec = serveMounted(t, ac, post("/ui/ingestion-sets/billing/applies/isa_2/reapply", true), page)
	mustContain(t, rec.Body.String(), "Set billing already is as apply isa_2 left it.")

	rec = serveMounted(t, ac, post("/ui/ingestion-sets/billing/release", true), page)
	require.Equal(t, http.StatusOK, rec.Code)
	mustContain(t, rec.Body.String(), "Set billing released: 2 ingestions can be edited one at a time again.", `id="ing-sets"`)
	assert.Equal(t, uitSetCall{op: "release", name: "billing"}, ops.calls[len(ops.calls)-1])

	// A native post goes back to the set with a fixed flash key.
	rec = serveMounted(t, ac, post("/ui/ingestion-sets/billing/release", false), page)
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/ui/ingestion-sets/billing", rec.Header().Get("Location"))
	assert.Contains(t, rec.Header().Get("Set-Cookie"), "fc_ui_flash="+flashIngSetReleased)

	// A refusal of the use case is shown, not swallowed.
	ops.err = usecase.Conflict("INGESTION_SET_OWNED", "ingestion x is owned by set y")
	rec = serveMounted(t, ac, post("/ui/ingestion-sets/billing/applies/isa_2/reapply", true), page)
	mustContain(t, rec.Body.String(), "ingestion x is owned by set y")
	mustNotContain(t, rec.Body.String(), "is back as apply")
}

func TestIngestionSetPostsDoNotReadTheSetsForAnyoneBelowTheListsRule(t *testing.T) {
	// The API lets a client-tier caller with the apply permission release a set
	// whose ingestions it reaches; the answer is the message only, never the
	// sets it may not list.
	ops := &fakeUitSetOps{}
	page := newUitSetsPage(uitSetsFixture(), ops)
	rec := serveMounted(t, uitClientUser(uitPermApply), req{method: http.MethodPost, target: "/ui/ingestion-sets/billing/release", datastar: true, multipart: true, form: url.Values{}}, page)
	require.Equal(t, http.StatusOK, rec.Code)
	mustContain(t, rec.Body.String(), "Set billing released")
	mustNotContain(t, rec.Body.String(), `id="ing-sets"`, "9-ops", "isa_3")
	assert.Len(t, ops.calls, 1)
}
