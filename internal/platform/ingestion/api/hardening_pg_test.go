//go:build integration

package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

const permTest = "platform:ingestion:ingestion:test"

// A save without promote is a draft, never a refusal (owner ruling
// 2026-10-25): a create without promote makes an ingestion whose first
// version is not live, whatever live says, and an update without it saves a
// version that is not live. Making a version live (make live, a version saved
// live) still needs promote.
func TestWithoutPromoteASaveIsADraft(t *testing.T) {
	e := newEnv(t)
	author := without(ingestionPerms, permPromote)
	e.as(anchor(author...))

	r := e.do(http.MethodPost, "/api/ingestions", e.body(e.appA, e.clientA.Identifier, "live-"+unique()))
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	assert.Equal(t, 0, r.num("version"), "a draft: nothing live")
	assert.Equal(t, 1, r.num("latestVersion"))
	save := r.json["save"].(map[string]any)
	assert.Equal(t, "DRAFT", save["outcome"])
	assert.Equal(t, true, save["awaitingPromote"])
	assert.Contains(t, save["message"], "someone with promote makes it live")
	explicit := e.body(e.appA, e.clientA.Identifier, "live-"+unique())
	explicit["live"] = true
	r = e.do(http.MethodPost, "/api/ingestions", explicit)
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	assert.Equal(t, 0, r.num("version"), "live: true without promote is a draft too")
	id := r.str("id")
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM aud_logs WHERE entity_id = $1 AND operation = 'CreateCommand'
		AND (operation_json->>'live')::boolean = false`, id), "the audit row records the draft")

	upd := e.definitionOf("c", "q-upd")
	upd["expectedVersion"] = 0
	r = e.do(http.MethodPut, "/api/ingestions/"+id, upd)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, 0, r.num("version"))
	assert.Equal(t, 2, r.num("latestVersion"))
	assert.Equal(t, "DRAFT", r.json["save"].(map[string]any)["outcome"])

	saved := e.definitionOf("c", "q-saved")
	r = e.do(http.MethodPost, "/api/ingestions/"+id+"/versions", saved)
	require.Equal(t, http.StatusCreated, r.code, "a version that is not live needs no promote: %s", r.raw)
	assert.Equal(t, true, r.json["draft"])
	saved["live"] = true
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodPost, "/api/ingestions/"+id+"/versions", saved).code)
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodPost, "/api/ingestions/"+id+"/versions/2/live", nil).code)

	e.as(anchor(ingestionPerms...))
	r = e.do(http.MethodPut, "/api/ingestions/"+id, upd)
	require.Equal(t, http.StatusOK, r.code, "with promote: %s", r.raw)
	assert.Equal(t, "MADE_LIVE", r.json["save"].(map[string]any)["outcome"], "version 2 already said it")
	assert.Equal(t, 2, r.num("version"))
}

// A save is deduplicated by the digest of its definition (owner ruling
// 2026-10-25), with promote and without: the live version's definition
// changes nothing; a draft's is not saved again (without promote it is
// answered, with promote that draft is made live); a new one is saved, as a
// draft without promote and live with it.
func TestSavesAreDeduplicatedByDigest(t *testing.T) {
	e := newEnv(t)
	code := "dedupe-" + unique()
	id := e.create(code) // version 1, live
	versions := func() int { return e.count(`SELECT count(*) FROM ing_versions WHERE ingestion_id = $1`, id) }
	update := func(queue string, expected int) response {
		e.t.Helper()
		b := e.definitionOf(code, queue)
		b["expectedVersion"] = expected
		return e.do(http.MethodPut, "/api/ingestions/"+id, b)
	}
	outcome := func(r response) string {
		s, _ := r.json["save"].(map[string]any)
		o, _ := s["outcome"].(string)
		return o
	}
	liveQueue := "q-" + code

	t.Run("without promote", func(t *testing.T) {
		e.as(anchor(without(ingestionPerms, permPromote)...))
		audits := e.auditRows(id)
		r := update(liveQueue, 1)
		require.Equal(t, http.StatusOK, r.code, r.raw)
		assert.Equal(t, "UNCHANGED", outcome(r), "the live version's definition")
		assert.Equal(t, 1, versions())
		assert.Equal(t, audits, e.auditRows(id), "nothing written")

		r = update("q-draft", 1)
		require.Equal(t, http.StatusOK, r.code, r.raw)
		assert.Equal(t, "DRAFT", outcome(r))
		assert.Equal(t, 2.0, r.json["save"].(map[string]any)["version"])
		assert.Equal(t, 1, r.num("version"), "the live version stays")
		assert.Equal(t, 2, versions())
		assert.Equal(t, 1, e.count(`SELECT count(*) FROM aud_logs WHERE entity_id = $1 AND operation = 'SaveVersionCommand'`, id),
			"the update is recorded as the draft it saved")

		audits = e.auditRows(id)
		r = update("q-draft", 1)
		require.Equal(t, http.StatusOK, r.code, r.raw)
		assert.Equal(t, "DRAFT_EXISTS", outcome(r), "a draft already says it")
		assert.Equal(t, 2.0, r.json["save"].(map[string]any)["version"])
		assert.Equal(t, 2, versions(), "no duplicate")
		assert.Equal(t, audits, e.auditRows(id))

		// The digest is stored and the same for the same definition.
		assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_versions WHERE ingestion_id = $1 AND (definition_digest IS NULL OR length(definition_digest) <> 64)`, id))
		r = e.do(http.MethodGet, "/api/ingestions/"+id+"/versions", nil)
		require.Equal(t, http.StatusOK, r.code, r.raw)
		vs := r.json["versions"].([]any)
		newest := vs[0].(map[string]any)
		assert.Equal(t, true, newest["draft"])
		assert.Equal(t, false, newest["live"])
		assert.Len(t, newest["digest"], 64)
		assert.Equal(t, false, vs[1].(map[string]any)["draft"], "the live version is no draft")
	})

	t.Run("with promote", func(t *testing.T) {
		e.as(anchor(ingestionPerms...))
		r := update(liveQueue, 1)
		require.Equal(t, http.StatusOK, r.code, r.raw)
		assert.Equal(t, "UNCHANGED", outcome(r))

		r = update("q-draft", 1)
		require.Equal(t, http.StatusOK, r.code, r.raw)
		assert.Equal(t, "MADE_LIVE", outcome(r), "the draft that says it is made live")
		assert.Equal(t, 2, r.num("version"))
		assert.Equal(t, 2, versions(), "no duplicate")
		assert.Equal(t, 1, e.count(`SELECT count(*) FROM aud_logs WHERE entity_id = $1 AND operation = 'MakeVersionLiveCommand'`, id))

		r = update("q-new", 2)
		require.Equal(t, http.StatusOK, r.code, r.raw)
		assert.Equal(t, "SAVED", outcome(r))
		assert.Equal(t, 3, r.num("version"))
		assert.Equal(t, 3, versions())

		// Going back to version 1's definition makes version 1 live again.
		r = update(liveQueue, 3)
		require.Equal(t, http.StatusOK, r.code, r.raw)
		assert.Equal(t, "MADE_LIVE", outcome(r))
		assert.Equal(t, 1, r.num("version"))
		assert.Equal(t, 3, versions())

		// POST .../versions dedupes the same way.
		b := e.definitionOf(code, "q-new")
		r = e.do(http.MethodPost, "/api/ingestions/"+id+"/versions", b)
		require.Equal(t, http.StatusCreated, r.code, r.raw)
		assert.Equal(t, 3.0, r.json["version"])
		assert.Equal(t, "DRAFT_EXISTS", r.json["save"].(map[string]any)["outcome"])
		assert.Equal(t, 3, versions())
	})
}

// The route permission counts only at anchor tier (owner ruling R2): a caller
// below it is refused a camel-route ingestion even holding the permission.
func TestRoutePermissionCountsOnlyAtAnchorTier(t *testing.T) {
	e := newEnv(t)
	route := func(code string) map[string]any {
		b := e.body(e.appA, e.clientA.Identifier, code)
		b["source"] = map[string]any{"kind": "custom", "type": "camel-route", "settings": map[string]any{"yaml": "- from: direct:x"}}
		return b
	}
	e.as(clientTier([]string{string(e.clientA.ID)}, append([]string{permRoute}, ingestionPerms...)...))
	r := e.do(http.MethodPost, "/api/ingestions", route("route-"+unique()))
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Equal(t, "INGESTION_ROUTE_ANCHOR_REQUIRED", r.errCode())

	e.as(anchor(append([]string{permRoute}, ingestionPerms...)...))
	r = e.do(http.MethodPost, "/api/ingestions", route("route-"+unique()))
	require.Equal(t, http.StatusCreated, r.code, r.raw)
}

// Camel routes are written by a pipeline or an ingestion administrator (owner
// ruling 2026-10-25). The route-author role (view, create, update, route,
// bind-secrets) writes routes only from a SERVICE principal, and as drafts
// (it holds no promote); a user holding it is refused; an administrator (who
// holds route and delete) writes them live; an operator holds no route.
func TestRoutesAreWrittenByAPipelineOrAnAdministrator(t *testing.T) {
	e := newEnv(t)
	route := func(code string) map[string]any {
		b := e.body(e.appA, e.clientA.Identifier, code)
		b["source"] = map[string]any{"kind": "custom", "type": "camel-route", "settings": map[string]any{"yaml": "- from: direct:x"}}
		return b
	}
	routeAuthor := []string{permView, permCreate, permUpdate, permRoute, permSecrets}

	user := anchor(routeAuthor...)
	user.PrincipalType = auth.PrincipalTypeUser
	e.as(user)
	r := e.do(http.MethodPost, "/api/ingestions", route("route-"+unique()))
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Equal(t, "INGESTION_ROUTE_PIPELINE_REQUIRED", r.errCode())

	pipeline := anchor(routeAuthor...)
	pipeline.PrincipalType = auth.PrincipalTypeService
	e.as(pipeline)
	r = e.do(http.MethodPost, "/api/ingestions", route("route-"+unique()))
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	assert.Equal(t, 0, r.num("version"), "a pipeline's route is a draft an administrator makes live")
	id := r.str("id")
	upd := route("ignored")
	delete(upd, "application")
	delete(upd, "client")
	delete(upd, "code")
	upd["source"] = map[string]any{"kind": "custom", "type": "camel-route", "settings": map[string]any{"yaml": "- from: direct:y"}}
	upd["expectedVersion"] = 0
	r = e.do(http.MethodPut, "/api/ingestions/"+id, upd)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, "DRAFT", r.json["save"].(map[string]any)["outcome"])

	operator := anchor(permView, permUpdate, permTest)
	operator.PrincipalType = auth.PrincipalTypeUser
	e.as(operator)
	r = e.do(http.MethodPut, "/api/ingestions/"+id, upd)
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Equal(t, "INGESTION_KIND_PERMISSION_REQUIRED", r.errCode(), "an operator cannot change a route")

	admin := anchor(append([]string{permRoute}, ingestionPerms...)...)
	admin.PrincipalType = auth.PrincipalTypeUser
	e.as(admin)
	r = e.do(http.MethodPut, "/api/ingestions/"+id, upd)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, "MADE_LIVE", r.json["save"].(map[string]any)["outcome"], "the administrator makes the pipeline's draft live")
}

// A source the runtime fleet reads with its own AWS identity needs
// runtime-identity (owner ruling R4a); one that names its own key does not
// (it names a secret, which bind-secrets covers).
func TestRuntimeIdentityPermission(t *testing.T) {
	e := newEnv(t)
	e.as(anchor(without(ingestionPerms, permIdentity)...))
	r := e.do(http.MethodPost, "/api/ingestions", e.body(e.appA, e.clientA.Identifier, "own-identity-"+unique()))
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Equal(t, "INGESTION_RUNTIME_IDENTITY_PERMISSION_REQUIRED", r.errCode())

	keyed := e.body(e.appA, e.clientA.Identifier, "keyed-"+unique())
	keyed["source"] = map[string]any{
		"kind": "sqs", "region": "eu-west-1", "queue": "q-keyed",
		"credential": map[string]any{"accessKeyId": "AKIAORDERS", "secretAccessKey": map[string]any{"name": "aws-key"}},
	}
	require.Equal(t, http.StatusCreated, e.do(http.MethodPost, "/api/ingestions", keyed).code)

	// The admin made one; renaming it changes nothing the identity reads.
	id := e.create("ident-" + unique())
	e.as(anchor(without(ingestionPerms, permIdentity)...))
	same := e.definitionOf("c", "q-"+"ident")
	got := e.do(http.MethodGet, "/api/ingestions/"+id, nil)
	same["source"] = got.json["source"]
	same["name"] = "Renamed"
	same["expectedVersion"] = 1
	assert.Equal(t, http.StatusOK, e.do(http.MethodPut, "/api/ingestions/"+id, same).code, "an unchanged source needs no more")
	other := e.definitionOf("c", "q-elsewhere")
	other["expectedVersion"] = 2
	assert.Equal(t, "INGESTION_RUNTIME_IDENTITY_PERMISSION_REQUIRED", e.do(http.MethodPut, "/api/ingestions/"+id, other).errCode())
}

// Naming secrets is a read path (owner ruling R4d): saving a definition that
// names secrets and is new, or sends them to a new address, or names other
// ones, needs bind-secrets; a change elsewhere does not.
func TestBindSecretsPermission(t *testing.T) {
	e := newEnv(t)
	sftp := func(code, host, secret string) map[string]any {
		b := e.body(e.appA, e.clientA.Identifier, code)
		b["source"] = map[string]any{
			"kind": "sftp", "host": host, "port": 22, "directory": "/in", "username": "feed",
			"credential": map[string]any{"name": secret}, "every": "PT5M",
		}
		return b
	}
	e.as(anchor(without(ingestionPerms, permSecrets)...))
	r := e.do(http.MethodPost, "/api/ingestions", sftp("s-"+unique(), "sftp.example.com", "sftp-password"))
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Equal(t, "INGESTION_BIND_SECRETS_PERMISSION_REQUIRED", r.errCode())

	e.as(anchor(ingestionPerms...))
	code := "s-" + unique()
	r = e.do(http.MethodPost, "/api/ingestions", sftp(code, "sftp.example.com", "sftp-password"))
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	id := r.str("id")

	e.as(anchor(without(ingestionPerms, permSecrets)...))
	change := func(host, secret, dir string, version int) response {
		b := sftp(code, host, secret)
		b["source"].(map[string]any)["directory"] = dir
		delete(b, "application")
		delete(b, "client")
		delete(b, "code")
		b["expectedVersion"] = version
		return e.do(http.MethodPut, "/api/ingestions/"+id, b)
	}
	assert.Equal(t, http.StatusOK, change("sftp.example.com", "sftp-password", "/in/v2", 1).code, "same host and secret")
	assert.Equal(t, "INGESTION_BIND_SECRETS_PERMISSION_REQUIRED", change("files.evil.example", "sftp-password", "/in/v2", 2).errCode(), "another host")
	assert.Equal(t, "INGESTION_BIND_SECRETS_PERMISSION_REQUIRED", change("sftp.example.com", "payroll-password", "/in/v2", 2).errCode(), "another secret")
}

// The outbound guard (owner ruling R4b) refuses a loopback, link-local or
// metadata address at save and when a test is queued, a saved definition
// included.
func TestSourceAddressGuard(t *testing.T) {
	e := newEnv(t)
	loop := e.body(e.appA, e.clientA.Identifier, "loop-"+unique())
	loop["source"] = map[string]any{
		"kind": "sftp", "host": "169.254.169.254", "port": 22, "directory": "/in", "username": "feed",
		"credential": map[string]any{"name": "pw"}, "every": "PT5M",
	}
	r := e.do(http.MethodPost, "/api/ingestions", loop)
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)
	assert.Equal(t, "INGESTION_INVALID", r.errCode())
	assert.Contains(t, r.raw, "source.host")

	e.as(anchor(append([]string{permTest}, ingestionPerms...)...))
	def := e.definitionOf("c", "q")
	def["source"] = loop["source"]
	r = e.do(http.MethodPost, "/api/ingestion-tests", map[string]any{"application": e.appA.Code, "client": e.clientA.Identifier, "definition": def})
	assert.Equal(t, http.StatusBadRequest, r.code, "an unsaved definition: %s", r.raw)

	// A definition saved before the guard existed is held to it when tested.
	id := e.create("old-" + unique())
	_, err := e.pool.Exec(context.Background(), `UPDATE ing_ingestions SET source = $2 WHERE id = $1`, id,
		`{"kind":"sftp","host":"localhost","port":22,"directory":"/in","username":"feed","credential":{"name":"pw"},"every":"PT5M"}`)
	require.NoError(t, err)
	r = e.do(http.MethodPost, "/api/ingestion-tests", map[string]any{"ingestionId": id})
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)
	assert.Contains(t, r.raw, "source.host")
}

// A failure is dismissable (C3): the update permission and the ingestion's
// reach, once, with an event and an audit row in the ingestion's history. An
// undismissed rejected failure is a notice the list and the ingestion show.
func TestDismissFailure(t *testing.T) {
	e := newEnv(t)
	id := e.create("dis-" + unique())
	insert := func(category, message string) string {
		fid := tsid.Generate(tsid.IngestionFailure)
		_, err := e.pool.Exec(context.Background(),
			`INSERT INTO ing_failures (id, ingestion_id, kind, category, message, occurred_at, created_at) VALUES ($1, $2, 'NOT_DISPATCHED', $3, $4, now(), now())`,
			fid, id, category, message)
		require.NoError(t, err)
		time.Sleep(2 * time.Millisecond)
		return fid
	}
	insert("payload", "one record")
	insert("rejected", "first refusal")
	latest := insert("rejected", "target pool suspended")

	got := e.do(http.MethodGet, "/api/ingestions/"+id, nil)
	assert.Equal(t, 2, got.num("notices"))
	assert.Equal(t, "target pool suspended", got.str("latestNotice"))
	list := e.do(http.MethodGet, "/api/ingestions?applicationCode="+e.appA.Code, nil)
	require.Equal(t, http.StatusOK, list.code)
	found := false
	for _, it := range list.json["ingestions"].([]any) {
		if m := it.(map[string]any); m["id"] == id {
			found = true
			assert.EqualValues(t, 2, m["notices"])
		}
	}
	assert.True(t, found)

	path := "/api/ingestions/" + id + "/failures/" + latest + "/dismiss"
	e.as(anchor(permView))
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodPost, path, nil).code, "update is needed")
	e.as(clientTier([]string{string(e.clientB.ID)}, ingestionPerms...))
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodPost, path, nil).code, "reach is needed")

	e.as(anchor(ingestionPerms...))
	audits, events := e.auditRows(id), e.eventRows(id)
	r := e.do(http.MethodPost, path, nil)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.NotEmpty(t, r.str("dismissedAt"))
	assert.Equal(t, "prn_ingtester0001", r.str("dismissedBy"))
	assert.Equal(t, "rejected", r.str("category"))
	assert.Equal(t, audits+1, e.auditRows(id), "in the ingestion's history")
	assert.Equal(t, events+1, e.eventRows(id))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM msg_events WHERE subject = $1 AND type = 'platform:ingestion:ingestion:failure-dismissed'`, "platform.ingestion."+id))

	r = e.do(http.MethodPost, path, nil)
	assert.Equal(t, http.StatusOK, r.code, "dismissing again answers it as it is")
	assert.Equal(t, audits+1, e.auditRows(id), "and writes nothing")

	got = e.do(http.MethodGet, "/api/ingestions/"+id, nil)
	assert.Equal(t, 1, got.num("notices"))
	assert.Equal(t, "first refusal", got.str("latestNotice"))
	open := e.do(http.MethodGet, "/api/ingestions/"+id+"/failures?undismissed=true&category=rejected", nil)
	require.Len(t, open.json["failures"], 1)
	assert.Equal(t, "first refusal", open.json["failures"].([]any)[0].(map[string]any)["message"])
	all := e.do(http.MethodGet, "/api/ingestions/"+id+"/failures", nil)
	assert.Len(t, all.json["failures"], 3, "a dismissed failure is still listed")

	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPost, "/api/ingestions/"+id+"/failures/"+tsid.Generate(tsid.IngestionFailure)+"/dismiss", nil).code)
	other := e.create("other-" + unique())
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodPost, "/api/ingestions/"+other+"/failures/"+latest+"/dismiss", nil).code,
		"a failure is dismissed through its own ingestion")
	_ = ingestion.FailureRejected
}
