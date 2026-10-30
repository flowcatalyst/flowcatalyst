//go:build integration

package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
)

// Secrets are entered as values with the settings, in one write: the platform
// names them, stores them as SECRET values at the ingestion's client, and no
// answer, audit row or event holds a value.

const (
	cfgKnownHost = "sftp.vendor.example ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample"
	cfgPassword  = "Zk9-never-in-a-response-Zk9"
)

// sftpConfigured creates an SFTP ingestion whose settings are the entry key.
func (e *env) sftpConfigured(key string) string {
	e.t.Helper()
	body := e.body(e.appA, e.clientA.Identifier, "sec-"+unique())
	body["source"] = map[string]any{"kind": "sftp", "config": key}
	body["split"] = map[string]any{"kind": "csv", "header": true}
	r := e.do(http.MethodPost, "/api/ingestions", body)
	require.Equal(e.t, http.StatusCreated, r.code, r.raw)
	return r.str("id")
}

func (e *env) sftpEntry(credential any) map[string]any {
	v := map[string]any{
		"host": "sftp.vendor.example", "port": 22, "directory": "/out", "username": "svc",
		"knownHosts": cfgKnownHost, "every": "PT5M",
	}
	if credential != nil {
		v["credential"] = credential
	}
	return v
}

// secretRow reads the stored secret of name at client A.
func (e *env) secretRow(name string) *platformconfig.Config {
	e.t.Helper()
	c, err := e.state.Configs.FindByCoordinate(context.Background(), e.appA.Code, "secrets", name, platformconfig.ScopeClient, new(string(e.clientA.ID)))
	require.NoError(e.t, err)
	return c
}

func (e *env) valueRows(value string) int {
	return e.count(`SELECT count(*) FROM msg_events e WHERE e::text LIKE $1`, "%"+value+"%") +
		e.count(`SELECT count(*) FROM aud_logs a WHERE a::text LIKE $1`, "%"+value+"%")
}

func TestConfigWritesTheEntryAndItsSecretsInOneStep(t *testing.T) {
	e := newEnv(t)
	key := "feed-" + unique()
	id := e.sftpConfigured(key)
	path := "/api/ingestions/" + id + "/config"

	// An author without bind-secrets: the value is theirs, so the name is too.
	e.as(anchor(permView, permUpdate))
	put := e.do(http.MethodPut, path, map[string]any{"value": e.sftpEntry(nil), "secretFields": map[string]any{"credential": cfgPassword}})
	require.Equal(t, http.StatusOK, put.code, put.raw)
	name := key + "-credential"

	// The entry names the derived secret; the secret is a SECRET at the client, and the runtime's fetch reads it.
	assert.Contains(t, put.raw, name)
	assert.NotContains(t, put.raw, cfgPassword)
	row := e.secretRow(name)
	require.NotNil(t, row)
	assert.Equal(t, platformconfig.ValueSecret, row.ValueType)
	assert.NotContains(t, row.Value, cfgPassword, "stored encrypted")
	svc := &platformconfig.Service{Repo: e.state.Configs}
	got, err := svc.FetchBoundSecretForIngestionRuntime(context.Background(), e.appA.Code, "secrets", name, string(e.clientA.ID))
	require.NoError(t, err)
	assert.Equal(t, cfgPassword, got.Value)

	// The state is told as names and set, never a value.
	secrets, _ := put.json["secrets"].([]any)
	require.Len(t, secrets, 1)
	assert.Equal(t, map[string]any{"field": "credential", "name": name, "set": true, "updatedAt": secrets[0].(map[string]any)["updatedAt"]}, secrets[0])
	get := e.do(http.MethodGet, path, nil)
	assert.NotContains(t, get.raw, cfgPassword)
	assert.Contains(t, get.raw, `"set":true`)

	// Neither the audit rows nor the events hold the value; the event names the secret.
	assert.Zero(t, e.valueRows(cfgPassword))
	assert.Positive(t, e.count(`SELECT count(*) FROM msg_events e WHERE e::text LIKE $1`, "%secretsSet%"+name+"%"))

	// Rotating: the name the entry has is kept (an existing entry's own name wins over a derived one).
	e.as(anchor(permView, permUpdate))
	again := e.do(http.MethodPut, path, map[string]any{"value": e.sftpEntry(map[string]any{"name": name}), "secrets": map[string]any{name: "rotated-" + cfgPassword}})
	require.Equal(t, http.StatusOK, again.code, again.raw)
	got, err = svc.FetchBoundSecretForIngestionRuntime(context.Background(), e.appA.Code, "secrets", name, string(e.clientA.ID))
	require.NoError(t, err)
	assert.Equal(t, "rotated-"+cfgPassword, got.Value)
	assert.Zero(t, e.valueRows("rotated-"))
}

func TestConfigKeepsTheNameAnEntryAlreadyHas(t *testing.T) {
	e := newEnv(t)
	key := "feed-" + unique()
	id := e.sftpConfigured(key)
	path := "/api/ingestions/" + id + "/config"
	kept := "old-vendor-pass-" + unique()
	put := e.do(http.MethodPut, path, map[string]any{"value": e.sftpEntry(map[string]any{"name": kept})})
	require.Equal(t, http.StatusOK, put.code, put.raw)

	// secretFields on an entry that names a secret writes the value under that name.
	put = e.do(http.MethodPut, path, map[string]any{"value": e.sftpEntry(nil), "secretFields": map[string]any{"credential": cfgPassword}})
	require.Equal(t, http.StatusOK, put.code, put.raw)
	require.NotNil(t, e.secretRow(kept))
	assert.Nil(t, e.secretRow(key+"-credential"))
}

func TestConfigRefusesWhatItCannotWriteAtomically(t *testing.T) {
	e := newEnv(t)
	key := "feed-" + unique()
	id := e.sftpConfigured(key)
	path := "/api/ingestions/" + id + "/config"

	// A name the value does not bind is refused, and nothing is written.
	bad := e.do(http.MethodPut, path, map[string]any{"value": e.sftpEntry(map[string]any{"name": "bound-" + key}), "secrets": map[string]any{"unbound-" + key: cfgPassword}})
	require.Equal(t, http.StatusBadRequest, bad.code, bad.raw)
	assert.Contains(t, bad.raw, "not a secret this entry uses")
	assert.NotContains(t, bad.raw, cfgPassword)
	assert.Nil(t, e.secretRow("unbound-"+key))
	assert.Equal(t, "MISSING", e.do(http.MethodGet, path, nil).str("status"), "the entry was not written either")

	// An entry that fails validation writes no secret.
	invalid := e.sftpEntry(nil)
	delete(invalid, "host")
	bad = e.do(http.MethodPut, path, map[string]any{"value": invalid, "secretFields": map[string]any{"credential": cfgPassword}})
	require.Equal(t, http.StatusBadRequest, bad.code, bad.raw)
	assert.Nil(t, e.secretRow(key+"-credential"))

	// The mask is not a value.
	bad = e.do(http.MethodPut, path, map[string]any{"value": e.sftpEntry(nil), "secretFields": map[string]any{"credential": "***"}})
	require.Equal(t, http.StatusBadRequest, bad.code, bad.raw)
	assert.Equal(t, "SECRET_VALUE_MASKED", bad.errCode())
}

// A value supplied with the name is the author's own; a name without a value, or
// one that already has someone else's value, still needs bind-secrets.
func TestConfigBindSecretsRuleWithSuppliedValues(t *testing.T) {
	e := newEnv(t)
	key := "feed-" + unique()
	id := e.sftpConfigured(key)
	path := "/api/ingestions/" + id + "/config"
	second := e.sftpConfigured("feed-" + unique())
	e.as(anchor(permView, permUpdate))

	named := e.do(http.MethodPut, path, map[string]any{"value": e.sftpEntry(map[string]any{"name": "mine-" + key})})
	require.Equal(t, http.StatusForbidden, named.code, "a name without its value: "+named.raw)
	assert.Equal(t, "INGESTION_BIND_SECRETS_PERMISSION_REQUIRED", named.errCode())

	ok := e.do(http.MethodPut, path, map[string]any{"value": e.sftpEntry(map[string]any{"name": "mine-" + key}), "secrets": map[string]any{"mine-" + key: cfgPassword}})
	require.Equal(t, http.StatusOK, ok.code, ok.raw)

	// A secret that already has a value, bound by nothing here: replacing it is not the author's to do.
	other := "someone-elses-" + unique()
	e.setSecret(other)
	steal := e.do(http.MethodPut, "/api/ingestions/"+second+"/config",
		map[string]any{"value": e.sftpEntry(map[string]any{"name": other}), "secrets": map[string]any{other: cfgPassword}})
	require.Equal(t, http.StatusForbidden, steal.code, steal.raw)
	assert.Equal(t, "INGESTION_BIND_SECRETS_PERMISSION_REQUIRED", steal.errCode())
}
