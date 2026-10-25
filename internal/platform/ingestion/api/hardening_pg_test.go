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
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

const permTest = "platform:ingestion:ingestion:test"

// Making a version live needs promote, a create's (live by default) and an
// update's (which makes its version live) as much as a make-live's (owner
// ruling R3). Without it a create must say live false and an update is
// refused, pointing at saving a version that is not live.
func TestPromoteIsNeededToMakeAVersionLive(t *testing.T) {
	e := newEnv(t)
	author := without(ingestionPerms, permPromote)
	e.as(anchor(author...))

	r := e.do(http.MethodPost, "/api/ingestions", e.body(e.appA, e.clientA.Identifier, "live-"+unique()))
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Equal(t, "INGESTION_PROMOTE_REQUIRED", r.errCode())
	assert.Contains(t, r.str("message"), "live: false")
	explicit := e.body(e.appA, e.clientA.Identifier, "live-"+unique())
	explicit["live"] = true
	assert.Equal(t, "INGESTION_PROMOTE_REQUIRED", e.do(http.MethodPost, "/api/ingestions", explicit).errCode())

	draft := e.body(e.appA, e.clientA.Identifier, "draft-"+unique())
	draft["live"] = false
	r = e.do(http.MethodPost, "/api/ingestions", draft)
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	assert.Equal(t, 0, r.num("version"), "a draft")
	id := r.str("id")

	upd := e.definitionOf("c", "q-upd")
	upd["expectedVersion"] = 0
	r = e.do(http.MethodPut, "/api/ingestions/"+id, upd)
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Equal(t, "INGESTION_PROMOTE_REQUIRED", r.errCode())
	assert.Contains(t, r.str("message"), "/versions")

	saved := e.definitionOf("c", "q-saved")
	r = e.do(http.MethodPost, "/api/ingestions/"+id+"/versions", saved)
	require.Equal(t, http.StatusCreated, r.code, "a version that is not live needs no promote: %s", r.raw)
	saved["live"] = true
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodPost, "/api/ingestions/"+id+"/versions", saved).code)

	e.as(anchor(ingestionPerms...))
	assert.Equal(t, http.StatusOK, e.do(http.MethodPut, "/api/ingestions/"+id, upd).code, "with promote")
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
