//go:build integration

package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformconfigops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// setSecret stores a SECRET in the "secrets" section of application A at
// client A, where a runtime fetching a bound secret looks.
func (e *env) setSecret(name string) {
	e.t.Helper()
	secret := "SECRET"
	admin := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_optestrunner1", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:*:*:*"},
	})
	clientID := string(e.clientA.ID)
	_, err := usecaseop.Run(admin, testpg.NewUoW(e.t), platformconfigops.SetProperty(e.state.Configs), platformconfigops.SetPropertyCommand{
		ApplicationCode: e.appA.Code, Section: "secrets", Property: name, Value: "s3cret-value", ValueType: &secret, ClientID: &clientID,
	}, testpg.TestEC())
	require.NoError(e.t, err)
}

// configured is a set item whose source names the entry key (no active said).
func (e *env) configured(code, key string) map[string]any {
	b := e.item(code, "unused")
	b["source"] = map[string]any{"kind": "sqs", "config": key}
	return b
}

func (e *env) status(id string) string {
	e.t.Helper()
	return e.do(http.MethodGet, "/api/ingestions/"+id, nil).str("status")
}

// Resuming checks the environment is set up: the entry the source names, then
// the secrets it binds, at the ingestion's client.
func TestResumeChecksReadiness(t *testing.T) {
	e := newEnv(t)
	key, code := "feed-"+unique(), "rdy-"+unique()
	body := e.body(e.appA, e.clientA.Identifier, code)
	body["source"] = map[string]any{"kind": "sqs", "config": key}
	r := e.do(http.MethodPost, "/api/ingestions", body)
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	id := r.str("id")
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/ingestions/"+id+"/pause", nil).code)

	// The entry is not set.
	r = e.do(http.MethodPost, "/api/ingestions/"+id+"/resume", nil)
	assert.Equal(t, http.StatusConflict, r.code, r.raw)
	assert.Equal(t, "INGESTION_CONFIG_NOT_SET", r.errCode())
	assert.Contains(t, r.raw, key)
	assert.Equal(t, "PAUSED", e.status(id))

	// The entry is set but binds a secret that has no value.
	put := e.do(http.MethodPut, "/api/ingestions/"+id+"/config", map[string]any{"value": map[string]any{
		"region": "eu-west-1", "queue": "q-ready",
		"credential": map[string]any{"accessKeyId": "AKIAEXAMPLE", "secretAccessKey": map[string]any{"name": "feed-sqs-secret"}},
	}})
	require.Equal(t, http.StatusOK, put.code, put.raw)
	r = e.do(http.MethodPost, "/api/ingestions/"+id+"/resume", nil)
	assert.Equal(t, http.StatusConflict, r.code, r.raw)
	assert.Equal(t, "INGESTION_SECRET_MISSING", r.errCode())
	assert.Contains(t, r.raw, "feed-sqs-secret")
	assert.Equal(t, "PAUSED", e.status(id))

	// Both are there.
	e.setSecret("feed-sqs-secret")
	r = e.do(http.MethodPost, "/api/ingestions/"+id+"/resume", nil)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, "ACTIVE", r.str("status"))
	assert.NotContains(t, r.raw, "s3cret-value")

	// Pausing is not checked.
	assert.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/ingestions/"+id+"/pause", nil).code)
}

// A source with inline settings is not held to the secret check: it was saved
// with everything it needs.
func TestResumeInlineSourceIsNotChecked(t *testing.T) {
	e := newEnv(t)
	body := e.body(e.appA, e.clientA.Identifier, "inl-"+unique())
	body["source"] = map[string]any{"kind": "sqs", "region": "eu-west-1", "queue": "q", "credential": map[string]any{
		"accessKeyId": "AKIAEXAMPLE", "secretAccessKey": map[string]any{"name": "inline-secret"},
	}}
	r := e.do(http.MethodPost, "/api/ingestions", body)
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	id := r.str("id")
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/ingestions/"+id+"/pause", nil).code)
	assert.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/ingestions/"+id+"/resume", nil).code)
}

// A set item that does not say active is created paused when its source names
// a config entry and active otherwise; later applies leave the state alone and
// do not call it a change. A stated active is enforced every time.
func TestSetApplyActiveIsTriState(t *testing.T) {
	e := newEnv(t)
	e.asSetAdmin()
	name := e.setName()
	cfg, inline := "cfg-"+unique(), "inl-"+unique()
	key := "feed-" + unique()

	r := e.putSet(name, e.configured(cfg, key), e.item(inline, "q-inline"))
	require.Equal(t, http.StatusOK, r.code, r.raw)
	cfgID := e.byCode(e.appA.Code, string(e.clientA.ID), cfg).ID
	inlineID := e.byCode(e.appA.Code, string(e.clientA.ID), inline).ID
	assert.Equal(t, "PAUSED", e.status(cfgID), "a source naming an entry is created paused")
	assert.Equal(t, "ACTIVE", e.status(inlineID))

	// Set the entry, resume it, and a pause made on the other: an apply that
	// does not say active leaves both as they are.
	put := e.do(http.MethodPut, "/api/ingestions/"+cfgID+"/config", map[string]any{"value": map[string]any{"region": "eu-west-1", "queue": "q"}})
	require.Equal(t, http.StatusOK, put.code, put.raw)
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/ingestions/"+cfgID+"/resume", nil).code)
	require.Equal(t, http.StatusOK, e.do(http.MethodPost, "/api/ingestions/"+inlineID+"/pause", nil).code)

	r = e.putSet(name, e.configured(cfg, key), e.item(inline, "q-inline"))
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, map[string]string{cfg: "UNCHANGED", inline: "UNCHANGED"}, actions(r))
	assert.Equal(t, "ACTIVE", e.status(cfgID))
	assert.Equal(t, "PAUSED", e.status(inlineID))

	// A stated active is enforced and is a change.
	on, off := e.configured(cfg, key), e.item(inline, "q-inline")
	off["active"] = false
	on["active"] = true
	r = e.putSet(name, on, off)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, map[string]string{cfg: "UNCHANGED", inline: "UNCHANGED"}, actions(r), "both already are what the file says")
	off["active"], on["active"] = true, false
	r = e.putSet(name, on, off)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, map[string]string{cfg: "CHANGE", inline: "CHANGE"}, actions(r))
	assert.Equal(t, "PAUSED", e.status(cfgID))
	assert.Equal(t, "ACTIVE", e.status(inlineID))

	// A stated active on a create is enforced, a configured source included.
	other := "own-" + unique()
	item := e.configured(other, key)
	item["active"] = true
	r = e.putSet(e.setName(), item)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, "ACTIVE", e.status(e.byCode(e.appA.Code, string(e.clientA.ID), other).ID))
}
