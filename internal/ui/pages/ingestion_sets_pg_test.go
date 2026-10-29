//go:build integration

package pages

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// apply applies the set with these queues, one item each, through the use
// case the API runs.
func (e *uitPG) apply(set string, codes map[string]string) operations.ApplySetResult {
	e.t.Helper()
	ctx := e.ctx()
	var items []ingestion.File
	for code, queue := range codes {
		items = append(items, ingestion.File{Application: e.app.Code, Client: e.client.Identifier, Code: code, Active: new(true), Definition: e.def(queue)})
	}
	files := []ingestion.SetFile{{Path: "ingest/" + set + ".yaml", Content: "queues: <" + set + ">"}}
	res, err := usecaseop.RunTx(ctx, e.uow, operations.ApplySet(e.repo, e.resolver, e.archive),
		operations.ApplySetCommand{Name: set, Items: items, Files: files}, testpg.TestEC())
	require.NoError(e.t, err)
	return res
}

// The sets area against Postgres and the real set archive: what an apply did,
// the files it was given, going back (a dry run first, then a new apply that
// puts the definitions back) and releasing; and the permission matrix on forged
// posts.
func TestIngestionSetsAgainstPostgres(t *testing.T) {
	e := newUitPG(t)
	set := "ui-set-" + uitUnique()
	code := "item-" + uitUnique()
	first := e.apply(set, map[string]string{code: "q-first"})
	require.NotNil(t, first.ApplyID)
	second := e.apply(set, map[string]string{code: "q-second"})
	require.NotNil(t, second.ApplyID)
	require.NotEqual(t, *first.ApplyID, *second.ApplyID)

	page := &IngestionSets{
		Sets:  IngestionSetReader{Repository: e.repo, Archive: e.archive},
		Ops:   IngestionSetOps{Repo: e.repo, Resolver: e.resolver, Archive: e.archive, UoW: e.uow},
		Shell: shellForTest(),
	}
	admin := e.admin()
	get := func(ac *auth.AuthContext, target string, datastar bool) (int, string) {
		rec := serveMounted(t, ac, req{target: target, datastar: datastar}, page)
		return rec.Code, rec.Body.String()
	}
	post := func(ac *auth.AuthContext, target string) (int, string) {
		rec := serveMounted(t, ac, req{method: http.MethodPost, target: target, datastar: true, multipart: true, form: url.Values{}}, page)
		return rec.Code, rec.Body.String()
	}
	live := func() *ingestion.Ingestion {
		i, err := e.repo.FindByCode(t.Context(), e.app.ID, e.client.ID, code)
		require.NoError(t, err)
		return i
	}
	queueOf := func(i *ingestion.Ingestion) string { return i.Definition.Source.(ingestion.Sqs).Queue }
	require.Equal(t, "q-second", queueOf(live()))
	base := "/ui/ingestion-sets/" + set

	// The list and the set's applies, newest first, with the files lazily.
	_, body := get(admin, "/ui/ingestion-sets", false)
	mustContain(t, body, `href="`+base+`"`, "fcdev ingest apply")
	code1, body := get(admin, base, false)
	require.Equal(t, http.StatusOK, code1, body)
	mustContain(t, body, "Applies of "+set, *first.ApplyID, *second.ApplyID, "Newest", "Go back to this", "Release the set", "1 definition(s), 1 file(s)", "by prn_",
		`fc-tag-info">CHANGE`, `fc-tag-success">CREATE`)
	assert.Less(t, strings.Index(body, *second.ApplyID), strings.Index(body, *first.ApplyID), "newest first")
	mustContain(t, body, `applies/`+*first.ApplyID+`/reapply"`)
	mustNotContain(t, body, `applies/`+*second.ApplyID+`/reapply"`)
	_, body = get(admin, base+"/applies/"+*first.ApplyID+"/files", true)
	mustContain(t, body, "ingest/"+set+".yaml", "queues: &lt;"+set+"&gt;")

	// Going back shows the plan and writes nothing.
	versions := live().LatestVersion
	_, body = get(admin, base+"/applies/"+*first.ApplyID+"/reapply", true)
	mustContain(t, body, "Go back to apply "+*first.ApplyID+" of "+set+"?", `fc-tag-info">CHANGE`, e.client.Identifier+"/"+code)
	assert.Equal(t, versions, live().LatestVersion, "a dry run saves no version")
	applies, err := e.repo.ListSetApplies(t.Context(), uitMustSet(t, e, set).ID)
	require.NoError(t, err)
	assert.Len(t, applies, 2)

	// Forged posts by callers without the set-apply permission change nothing.
	for name, ac := range map[string]*auth.AuthContext{
		"view only": uitUser(uitPermView), "test only": uitUser(uitPermTest), "client tier": uitClientUser(uitPermView),
	} {
		_, body = post(ac, base+"/applies/"+*first.ApplyID+"/reapply")
		mustContain(t, body, "permission required")
		_, body = post(ac, base+"/release")
		mustContain(t, body, "permission required")
		i := live()
		assert.Equal(t, "q-second", queueOf(i), name)
		require.NotNil(t, i.ManagedBy, name)
	}

	// Going back: a third apply that puts the first definitions back.
	code2, body := post(admin, base+"/applies/"+*first.ApplyID+"/reapply")
	require.Equal(t, http.StatusOK, code2, body)
	mustContain(t, body, "Set "+set+" is back as apply "+*first.ApplyID+" left it.", "going back to apply "+*first.ApplyID)
	assert.Equal(t, "q-first", queueOf(live()))
	applies, err = e.repo.ListSetApplies(t.Context(), uitMustSet(t, e, set).ID)
	require.NoError(t, err)
	require.Len(t, applies, 3)
	require.NotNil(t, applies[0].Summary.GoingBackTo)
	assert.Equal(t, *first.ApplyID, *applies[0].Summary.GoingBackTo)

	// An apply the set does not keep, and a set that does not exist.
	code3, _ := get(admin, base+"/applies/isa_none/files", false)
	assert.Equal(t, http.StatusSeeOther, code3, "a navigation goes to the set")
	_, body = get(admin, base+"/applies/isa_none/files", true)
	mustContain(t, body, "the set does not keep apply isa_none")
	code4, _ := get(admin, "/ui/ingestion-sets/no-such-set-"+uitUnique(), false)
	assert.Equal(t, http.StatusNotFound, code4)

	// Releasing lets go of the ingestion: it is edited one at a time again.
	_, body = post(admin, base+"/release")
	mustContain(t, body, "Set "+set+" released: 1 ingestion can be edited one at a time again.")
	assert.Nil(t, live().ManagedBy)
	_, body = get(admin, base, false)
	mustNotContain(t, body, "Release the set")
}

func uitMustSet(t *testing.T, e *uitPG, name string) *ingestion.Set {
	t.Helper()
	s, err := e.repo.FindSetByName(t.Context(), name)
	require.NoError(t, err)
	require.NotNil(t, s)
	return s
}
