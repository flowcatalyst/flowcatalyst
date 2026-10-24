//go:build integration

package api_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

const permSetApply = "platform:ingestion:set:apply"

// asSetAdmin may do everything to ingestions and apply sets.
func (e *env) asSetAdmin() {
	e.as(anchor(append(append([]string{}, ingestionPerms...), permSetApply)...))
}

// item is a set item for application A and client A: a create body with the
// queue it reads.
func (e *env) item(code, queue string) map[string]any {
	b := e.body(e.appA, e.clientA.Identifier, code)
	b["source"] = map[string]any{"kind": "sqs", "region": "eu-west-1", "queue": queue}
	return b
}

func (e *env) putSet(name string, items ...map[string]any) response {
	return e.putSetQuery(name, "", items...)
}

func (e *env) putSetQuery(name, query string, items ...map[string]any) response {
	e.t.Helper()
	if items == nil {
		items = []map[string]any{}
	}
	path := "/api/ingestion-sets/" + name
	if query != "" {
		path += "?" + query
	}
	return e.do(http.MethodPut, path, map[string]any{"items": items})
}

func (e *env) setName() string { return "set-" + unique() }

// actions is the action of each change, by code.
func actions(r response) map[string]string {
	out := map[string]string{}
	changes, _ := r.json["changes"].([]any)
	for _, c := range changes {
		m := c.(map[string]any)
		out[m["code"].(string)], _ = m["action"].(string)
	}
	return out
}

func (e *env) byCode(app, clientID, code string) *ingestion.Ingestion {
	e.t.Helper()
	var appID string
	require.NoError(e.t, e.pool.QueryRow(context.Background(), `SELECT id FROM app_applications WHERE code = $1`, app).Scan(&appID))
	i, err := ingestion.NewRepository(e.pool).FindByCode(context.Background(), ids.ApplicationID(appID), ids.ClientID(clientID), code)
	require.NoError(e.t, err)
	return i
}

func (e *env) archiveHas(digest string) bool {
	ok, err := e.state.SetArchive.Exists(context.Background(), digest)
	require.NoError(e.t, err)
	return ok
}

// An apply creates what is new, leaves what is the same, saves a changed
// definition as the next live version, and deletes what it no longer lists.
func TestSetApplyCreateChangeUnchangedRemove(t *testing.T) {
	e := newEnv(t)
	e.asSetAdmin()
	name := e.setName()
	a, b, c := "a-"+unique(), "b-"+unique(), "c-"+unique()

	r := e.putSet(name, e.item(a, "q-a"), e.item(b, "q-b"), e.item(c, "q-c"))
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, map[string]string{a: "CREATE", b: "CREATE", c: "CREATE"}, actions(r))
	first := r.str("applyId")
	require.NotEmpty(t, first)
	assert.Equal(t, false, r.json["dryRun"])

	ing := e.byCode(e.appA.Code, string(e.clientA.ID), a)
	require.NotNil(t, ing)
	require.NotNil(t, ing.ManagedBy)
	assert.Equal(t, 1, ing.Version)
	versions := e.do(http.MethodGet, "/api/ingestions/"+ing.ID+"/versions", nil)
	vs := versions.json["versions"].([]any)
	require.Len(t, vs, 1)
	assert.Equal(t, first, vs[0].(map[string]any)["appliedFrom"], "a version records the apply that produced it")
	assert.Equal(t, 1, e.auditRows(ing.ID))

	// The same items: nothing is written, nothing is recorded.
	r = e.putSet(name, e.item(a, "q-a"), e.item(b, "q-b"), e.item(c, "q-c"))
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, map[string]string{a: "UNCHANGED", b: "UNCHANGED", c: "UNCHANGED"}, actions(r))
	assert.Nil(t, r.json["applyId"])
	assert.Equal(t, 1, e.auditRows(ing.ID))

	// a changes, b stays, c is no longer listed.
	r = e.putSet(name, e.item(a, "q-a2"), e.item(b, "q-b"))
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, map[string]string{a: "CHANGE", b: "UNCHANGED", c: "REMOVE"}, actions(r))
	second := r.str("applyId")
	require.NotEmpty(t, second)
	assert.NotEqual(t, first, second)

	ing = e.byCode(e.appA.Code, string(e.clientA.ID), a)
	assert.Equal(t, 2, ing.Version)
	assert.Equal(t, 2, ing.LatestVersion)
	require.NotNil(t, ing.ManagedBy)
	assert.Nil(t, e.byCode(e.appA.Code, string(e.clientA.ID), c), "an ingestion the set no longer lists is deleted")
	assert.Equal(t, 1, e.byCode(e.appA.Code, string(e.clientA.ID), b).Version, "an unchanged ingestion keeps its version")

	// The apply is in the history with what it did.
	applies := e.do(http.MethodGet, "/api/ingestion-sets/"+name+"/applies", nil)
	require.Equal(t, http.StatusOK, applies.code, applies.raw)
	list := applies.json["applies"].([]any)
	require.Len(t, list, 2)
	assert.Equal(t, second, list[0].(map[string]any)["id"], "newest first")
	assert.EqualValues(t, 2, list[0].(map[string]any)["items"])

	sets := e.do(http.MethodGet, "/api/ingestion-sets", nil)
	require.Equal(t, http.StatusOK, sets.code, sets.raw)
	var owned int
	for _, s := range sets.json["sets"].([]any) {
		if m := s.(map[string]any); m["name"] == name {
			owned = int(m["ingestions"].(float64))
		}
	}
	assert.Equal(t, 2, owned)
}

// A dry run does every check and writes nothing: no ingestion, no set, no
// apply, no archive, no event.
func TestSetDryRunWritesNothing(t *testing.T) {
	e := newEnv(t)
	e.asSetAdmin()
	name := e.setName()
	code := "dry-" + unique()

	events := e.count(`SELECT count(*) FROM msg_events WHERE type LIKE 'platform:ingestion:%'`)
	r := e.putSetQuery(name, "dryRun=true", e.item(code, "q-dry"))
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, true, r.json["dryRun"])
	assert.Equal(t, map[string]string{code: "CREATE"}, actions(r))
	assert.Nil(t, r.json["applyId"])
	assert.Nil(t, e.byCode(e.appA.Code, string(e.clientA.ID), code))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_sets WHERE name = $1`, name))
	assert.Equal(t, events, e.count(`SELECT count(*) FROM msg_events WHERE type LIKE 'platform:ingestion:%'`))
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, "/api/ingestion-sets/"+name+"/applies", nil).code)

	// Over a real set: it says what it would change and changes none of it.
	require.Equal(t, http.StatusOK, e.putSet(name, e.item(code, "q-dry")).code)
	other := "dry2-" + unique()
	r = e.putSetQuery(name, "dryRun=true", e.item(other, "q-other"))
	assert.Equal(t, map[string]string{other: "CREATE", code: "REMOVE"}, actions(r))
	assert.NotNil(t, e.byCode(e.appA.Code, string(e.clientA.ID), code), "the removal was not done")
	assert.Nil(t, e.byCode(e.appA.Code, string(e.clientA.ID), other))
	assert.Len(t, e.do(http.MethodGet, "/api/ingestion-sets/"+name+"/applies", nil).json["applies"], 1)

	// A dry run fails on a bad reference, as the apply would.
	bad := e.item("bad-"+unique(), "q-bad")
	bad["target"].(map[string]any)["pool"] = "no-such-pool"
	assert.Equal(t, http.StatusBadRequest, e.putSetQuery(name, "dryRun=true", bad).code)
}

// One ingestion owned by another set stops the whole apply.
func TestSetOwnedByAnotherSet(t *testing.T) {
	e := newEnv(t)
	e.asSetAdmin()
	one, two := e.setName(), e.setName()
	shared, fresh := "shared-"+unique(), "fresh-"+unique()

	require.Equal(t, http.StatusOK, e.putSet(one, e.item(shared, "q-1")).code)
	r := e.putSet(two, e.item(fresh, "q-f"), e.item(shared, "q-1"))
	assert.Equal(t, http.StatusConflict, r.code, r.raw)
	assert.Equal(t, "INGESTION_SET_OWNED", r.errCode())
	assert.Contains(t, r.raw, one, "the message names the owning set")
	assert.Nil(t, e.byCode(e.appA.Code, string(e.clientA.ID), fresh), "nothing of the refused apply was written")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_sets WHERE name = $1`, two))

	// After the first set lets go, the second may take it in.
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/ingestion-sets/"+one+"/release", nil).code)
	r = e.putSet(two, e.item(fresh, "q-f"), e.item(shared, "q-1"))
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, map[string]string{fresh: "CREATE", shared: "ADOPT"}, actions(r))
}

// An ingestion made by hand is taken into the set: with its definition kept
// when it already says the same, changed when it does not.
func TestSetAdoptsHandMadeIngestions(t *testing.T) {
	e := newEnv(t)
	e.asSetAdmin()
	same, differs := "same-"+unique(), "differs-"+unique()
	for _, code := range []string{same, differs} {
		r := e.do(http.MethodPost, "/api/ingestions", e.item(code, "q-"+code))
		require.Equal(t, http.StatusCreated, r.code, r.raw)
	}

	r := e.putSet(e.setName(), e.item(same, "q-"+same), e.item(differs, "q-new"))
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, map[string]string{same: "ADOPT", differs: "ADOPT"}, actions(r))
	for _, c := range r.json["changes"].([]any) {
		m := c.(map[string]any)
		if m["code"] == same {
			assert.Empty(t, m["changed"])
		} else {
			assert.Equal(t, []any{"source"}, m["changed"])
		}
	}
	a := e.byCode(e.appA.Code, string(e.clientA.ID), same)
	assert.NotNil(t, a.ManagedBy)
	assert.Equal(t, 1, a.Version, "an adopted ingestion that is the same keeps its version, so runtimes keep the route")
	b := e.byCode(e.appA.Code, string(e.clientA.ID), differs)
	assert.NotNil(t, b.ManagedBy)
	assert.Equal(t, 2, b.Version)
}

// Going back applies the items an earlier apply was given, as a new apply.
func TestSetReapply(t *testing.T) {
	e := newEnv(t)
	e.asSetAdmin()
	name := e.setName()
	a, b := "a-"+unique(), "b-"+unique()

	first := e.putSet(name, e.item(a, "q-1"), e.item(b, "q-b"))
	require.Equal(t, http.StatusOK, first.code, first.raw)
	second := e.putSet(name, e.item(a, "q-2"))
	require.Equal(t, http.StatusOK, second.code, second.raw)
	assert.Nil(t, e.byCode(e.appA.Code, string(e.clientA.ID), b))
	assert.Equal(t, 2, e.byCode(e.appA.Code, string(e.clientA.ID), a).Version)

	path := "/api/ingestion-sets/" + name + "/applies/" + first.str("applyId") + "/reapply"
	dry := e.do(http.MethodPost, path+"?dryRun=true", nil)
	require.Equal(t, http.StatusOK, dry.code, dry.raw)
	assert.Equal(t, map[string]string{a: "CHANGE", b: "CREATE"}, actions(dry))
	assert.Nil(t, e.byCode(e.appA.Code, string(e.clientA.ID), b), "a dry run reapplies nothing")

	r := e.do(http.MethodPost, path, nil)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, map[string]string{a: "CHANGE", b: "CREATE"}, actions(r))
	third := r.str("applyId")
	require.NotEmpty(t, third)
	got := e.byCode(e.appA.Code, string(e.clientA.ID), a)
	assert.Equal(t, 3, got.Version, "going back is a new version, not a rewrite of history")
	assert.Equal(t, "q-1", got.Definition.Source.(ingestion.Sqs).Queue)
	assert.NotNil(t, e.byCode(e.appA.Code, string(e.clientA.ID), b))

	applies := e.do(http.MethodGet, "/api/ingestion-sets/"+name+"/applies", nil).json["applies"].([]any)
	require.Len(t, applies, 3)
	assert.Equal(t, third, applies[0].(map[string]any)["id"])
	assert.Equal(t, first.str("applyId"), applies[0].(map[string]any)["goingBackTo"])

	// An apply the set does not keep.
	gone := e.do(http.MethodPost, "/api/ingestion-sets/"+name+"/applies/isa_missing0000/reapply", nil)
	assert.Equal(t, http.StatusNotFound, gone.code, gone.raw)
	assert.Equal(t, "INGESTION_SET_APPLY_NOT_FOUND", gone.errCode())
}

// What an apply was given is kept in the artifact store and read back.
func TestSetArchive(t *testing.T) {
	e := newEnv(t)
	e.asSetAdmin()
	name := e.setName()
	code := "arch-" + unique()
	body := map[string]any{
		"items": []map[string]any{e.item(code, "q-arch")},
		"files": []map[string]any{{"path": "acme/ingest/" + code + ".yaml", "content": "code: " + code + "\n"}},
	}
	r := e.do(http.MethodPut, "/api/ingestion-sets/"+name, body)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	applyID := r.str("applyId")

	got := e.do(http.MethodGet, "/api/ingestion-sets/"+name+"/applies/"+applyID, nil)
	require.Equal(t, http.StatusOK, got.code, got.raw)
	assert.Equal(t, name, got.str("set"))
	items := got.json["items"].([]any)
	require.Len(t, items, 1)
	assert.Equal(t, code, items[0].(map[string]any)["code"])
	assert.Equal(t, e.clientA.Identifier, items[0].(map[string]any)["client"], "the client is kept as it was given")
	files := got.json["files"].([]any)
	require.Len(t, files, 1)
	assert.Equal(t, "code: "+code+"\n", files[0].(map[string]any)["content"])

	set, err := ingestion.NewRepository(e.pool).FindSetByName(context.Background(), name)
	require.NoError(t, err)
	apply, err := ingestion.NewRepository(e.pool).FindSetApply(context.Background(), set.ID, applyID)
	require.NoError(t, err)
	require.NotNil(t, apply)
	assert.True(t, e.archiveHas(apply.ArchiveDigest))

	// Lost from the store, the apply cannot be read or gone back to.
	require.NoError(t, e.state.SetArchive.Delete(context.Background(), apply.ArchiveDigest))
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, "/api/ingestion-sets/"+name+"/applies/"+applyID, nil).code)
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPost, "/api/ingestion-sets/"+name+"/applies/"+applyID+"/reapply", nil).code)
}

// failingStore keeps nothing: an apply that cannot keep its archive is not
// applied.
type failingStore struct{ artifact.Store }

func (failingStore) Put(context.Context, string, io.Reader) error { return fmt.Errorf("store is down") }

func TestSetArchiveUnavailableAppliesNothing(t *testing.T) {
	e := newEnv(t)
	e.asSetAdmin()
	e.state.SetArchive = failingStore{e.state.SetArchive}
	name, code := e.setName(), "noarch-"+unique()

	r := e.putSet(name, e.item(code, "q-x"))
	assert.Equal(t, http.StatusInternalServerError, r.code, r.raw)
	assert.Nil(t, e.byCode(e.appA.Code, string(e.clientA.ID), code), "the ingestion was rolled back with the apply")
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_sets WHERE name = $1`, name))
}

// Only the newest ten applies are kept, and an older one's archive goes.
func TestSetKeepsTenApplies(t *testing.T) {
	e := newEnv(t)
	e.asSetAdmin()
	name, code := e.setName(), "ten-"+unique()
	var ids []string
	for n := range ingestion.MaxSetApplies + 2 {
		r := e.putSet(name, e.item(code, fmt.Sprintf("q-%d", n)))
		require.Equal(t, http.StatusOK, r.code, r.raw)
		ids = append(ids, r.str("applyId"))
	}
	applies := e.do(http.MethodGet, "/api/ingestion-sets/"+name+"/applies", nil).json["applies"].([]any)
	assert.Len(t, applies, ingestion.MaxSetApplies)
	assert.Equal(t, ids[len(ids)-1], applies[0].(map[string]any)["id"])
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, "/api/ingestion-sets/"+name+"/applies/"+ids[0], nil).code)
	assert.Equal(t, http.StatusOK, e.do(http.MethodGet, "/api/ingestion-sets/"+name+"/applies/"+ids[2], nil).code)
}

// Releasing leaves the ingestions as they are and editable by hand.
func TestSetRelease(t *testing.T) {
	e := newEnv(t)
	e.asSetAdmin()
	name, code := e.setName(), "rel-"+unique()
	require.Equal(t, http.StatusOK, e.putSet(name, e.item(code, "q-r")).code)
	ing := e.byCode(e.appA.Code, string(e.clientA.ID), code)
	require.NotNil(t, ing.ManagedBy)

	upd := e.definitionOf(code, "q-edit")
	upd["expectedVersion"] = 1
	r := e.do(http.MethodPut, "/api/ingestions/"+ing.ID, upd)
	assert.Equal(t, "INGESTION_SET_OWNED", r.errCode(), "owned: read-only")

	r = e.do(http.MethodPost, "/api/ingestion-sets/"+name+"/release", nil)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, name, r.str("name"))
	assert.Equal(t, 1, r.num("released"))
	after := e.byCode(e.appA.Code, string(e.clientA.ID), code)
	assert.Nil(t, after.ManagedBy)
	assert.Equal(t, 1, after.Version, "release changes the owner and nothing else")
	assert.True(t, after.Definition.Equal(ing.Definition))

	r = e.do(http.MethodPut, "/api/ingestions/"+ing.ID, upd)
	assert.Equal(t, http.StatusOK, r.code, "released: editable by hand again: %s", r.raw)

	// Releasing a set that was never applied, or twice.
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPost, "/api/ingestion-sets/"+e.setName()+"/release", nil).code)
	r = e.do(http.MethodPost, "/api/ingestion-sets/"+name+"/release", nil)
	assert.Equal(t, 0, r.num("released"))
}

// A set's ingestions are read-only whichever way they are changed; pausing
// and resuming stay.
func TestSetOwnedIngestionsAreReadOnly(t *testing.T) {
	e := newEnv(t)
	e.asSetAdmin()
	code := "ro-" + unique()
	require.Equal(t, http.StatusOK, e.putSet(e.setName(), e.item(code, "q-ro")).code)
	id := e.byCode(e.appA.Code, string(e.clientA.ID), code).ID

	upd := e.definitionOf(code, "q-new")
	upd["expectedVersion"] = 1
	for _, c := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPut, "/api/ingestions/" + id, upd},
		{http.MethodPost, "/api/ingestions/" + id + "/versions", e.definitionOf(code, "q-v")},
		{http.MethodPost, "/api/ingestions/" + id + "/versions/1/live", nil},
		{http.MethodDelete, "/api/ingestions/" + id, nil},
	} {
		r := e.do(c.method, c.path, c.body)
		assert.Equal(t, http.StatusConflict, r.code, "%s %s: %s", c.method, c.path, r.raw)
		assert.Equal(t, "INGESTION_SET_OWNED", r.errCode())
	}
	assert.Equal(t, "PAUSED", e.do(http.MethodPost, "/api/ingestions/"+id+"/pause", nil).str("status"))
	assert.Equal(t, "ACTIVE", e.do(http.MethodPost, "/api/ingestions/"+id+"/resume", nil).str("status"))
	assert.NotNil(t, e.byCode(e.appA.Code, string(e.clientA.ID), code), "still there")
}

// Two hundred items go in one transaction.
func TestSetLargeApply(t *testing.T) {
	e := newEnv(t)
	e.asSetAdmin()
	name := e.setName()
	items := make([]map[string]any, 0, 200)
	for n := range 200 {
		items = append(items, e.item(fmt.Sprintf("big-%03d-%s", n, unique()), fmt.Sprintf("q-%d", n)))
	}
	r := e.putSet(name, items...)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Len(t, r.json["changes"], 200)
	assert.Equal(t, 200, e.count(`SELECT count(*) FROM ing_ingestions i JOIN ing_sets s ON s.id = i.managed_by WHERE s.name = $1`, name))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_set_applies a JOIN ing_sets s ON s.id = a.set_id WHERE s.name = $1`, name))
	assert.Equal(t, 200, e.count(`SELECT count(*) FROM ing_versions v JOIN ing_set_applies a ON a.id = v.applied_from JOIN ing_sets s ON s.id = a.set_id WHERE s.name = $1`, name))
}

// A reference that does not resolve fails the whole apply: nothing before it
// is kept either.
func TestSetUnknownReferenceFailsAtomically(t *testing.T) {
	e := newEnv(t)
	e.asSetAdmin()
	good, bad := "good-"+unique(), "bad-"+unique()

	for name, mutate := range map[string]func(map[string]any){
		"pool":        func(b map[string]any) { b["target"].(map[string]any)["pool"] = "no-such-pool" },
		"connection":  func(b map[string]any) { b["target"].(map[string]any)["connection"] = "no-such-connection" },
		"client":      func(b map[string]any) { b["client"] = "no-such-client" },
		"application": func(b map[string]any) { b["application"] = "no-such-app" },
	} {
		set := e.setName()
		broken := e.item(bad, "q-bad")
		mutate(broken)
		r := e.putSet(set, e.item(good, "q-good"), broken)
		assert.Equal(t, http.StatusBadRequest, r.code, "%s: %s", name, r.raw)
		assert.Nil(t, e.byCode(e.appA.Code, string(e.clientA.ID), good), "%s: the good item was not kept", name)
		assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_sets WHERE name = $1`, set), name)
		assert.Contains(t, r.raw, "items[1]", "%s: the error says which item", name)
	}
}

// An item's client is an id or an identifier; the same ingestion written both
// ways is a duplicate.
func TestSetClientIDOrIdentifier(t *testing.T) {
	e := newEnv(t)
	e.asSetAdmin()
	byID, byIdent := "byid-"+unique(), "byident-"+unique()

	a := e.body(e.appA, string(e.clientA.ID), byID)
	b := e.body(e.appA, e.clientB.Identifier, byIdent)
	r := e.putSet(e.setName(), a, b)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.NotNil(t, e.byCode(e.appA.Code, string(e.clientA.ID), byID))
	assert.NotNil(t, e.byCode(e.appA.Code, string(e.clientB.ID), byIdent))

	dup1 := e.body(e.appA, string(e.clientA.ID), "dup-"+unique())
	dup2 := e.body(e.appA, e.clientA.Identifier, dup1["code"].(string))
	r = e.putSet(e.setName(), dup1, dup2)
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)
	assert.Contains(t, r.raw, "items[1].code")
}

// Applying and releasing need the set permission; reading needs the view
// permission and a caller that reaches everything.
func TestSetPermissions(t *testing.T) {
	e := newEnv(t)
	name, code := e.setName(), "perm-"+unique()
	e.asSetAdmin()
	applied := e.putSet(name, e.item(code, "q-p"))
	require.Equal(t, http.StatusOK, applied.code, applied.raw)
	applyID := applied.str("applyId")
	base := "/api/ingestion-sets/" + name

	e.as(anchor(ingestionPerms...))
	for _, c := range []struct{ method, path string }{
		{http.MethodPut, base},
		{http.MethodPost, base + "/release"},
		{http.MethodPost, base + "/applies/" + applyID + "/reapply"},
	} {
		var body any
		if c.method == http.MethodPut {
			body = map[string]any{"items": []any{}}
		}
		r := e.do(c.method, c.path, body)
		assert.Equal(t, http.StatusForbidden, r.code, "%s %s: %s", c.method, c.path, r.raw)
		assert.Equal(t, "PERMISSION_REQUIRED", r.errCode())
	}
	assert.NotNil(t, e.byCode(e.appA.Code, string(e.clientA.ID), code), "a refused apply removed nothing")

	e.as(anchor(permSetApply))
	for _, p := range []string{"/api/ingestion-sets", base + "/applies", base + "/applies/" + applyID} {
		assert.Equal(t, http.StatusForbidden, e.do(http.MethodGet, p, nil).code, "reading %s needs view", p)
	}
	assert.Equal(t, http.StatusOK, e.putSetQuery(name, "dryRun=true", e.item(code, "q-p")).code, "apply alone may dry-run")

	e.as(anchor(permView))
	for _, p := range []string{"/api/ingestion-sets", base + "/applies", base + "/applies/" + applyID} {
		assert.Equal(t, http.StatusOK, e.do(http.MethodGet, p, nil).code, p)
	}
}

// Below anchor tier a caller applies only items of its own clients, and
// cannot read sets, which span clients.
func TestSetClientTierReach(t *testing.T) {
	e := newEnv(t)
	name := e.setName()
	e.as(clientTier([]string{string(e.clientA.ID)}, append(append([]string{}, ingestionPerms...), permSetApply)...))

	foreign := "foreign-" + unique()
	r := e.putSet(name, e.item("own-"+unique(), "q-o"), e.body(e.appA, e.clientB.Identifier, foreign))
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Nil(t, e.byCode(e.appA.Code, string(e.clientB.ID), foreign))
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_sets WHERE name = $1`, name), "nothing of the refused apply was kept")

	own := "own-" + unique()
	require.Equal(t, http.StatusOK, e.putSet(name, e.item(own, "q-o")).code)
	r = e.do(http.MethodGet, "/api/ingestion-sets", nil)
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Equal(t, "INGESTION_SET_ANCHOR_ONLY", r.errCode())

	// It cannot remove, by omission, what it does not reach.
	e.asSetAdmin()
	foreignOwned := "fo-" + unique()
	require.Equal(t, http.StatusOK, e.putSet(name, e.item(own, "q-o"), e.body(e.appA, e.clientB.Identifier, foreignOwned)).code)
	e.as(clientTier([]string{string(e.clientA.ID)}, permSetApply))
	r = e.putSet(name, e.item(own, "q-o"))
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.NotNil(t, e.byCode(e.appA.Code, string(e.clientB.ID), foreignOwned))
}

// Shape errors name the item and the field; a bad set name is refused.
func TestSetValidation(t *testing.T) {
	e := newEnv(t)
	e.asSetAdmin()
	bad := e.item("Bad_Code", "q-v")
	r := e.putSet(e.setName(), bad)
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)
	assert.Contains(t, r.raw, "items[0].code")

	r = e.putSet("Bad_Name", e.item("ok-"+unique(), "q"))
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)

	// An empty set is valid: it removes what the set owns.
	name, code := e.setName(), "empty-"+unique()
	require.Equal(t, http.StatusOK, e.putSet(name, e.item(code, "q-e")).code)
	r = e.putSet(name)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, map[string]string{code: "REMOVE"}, actions(r))
	assert.Nil(t, e.byCode(e.appA.Code, string(e.clientA.ID), code))
}
