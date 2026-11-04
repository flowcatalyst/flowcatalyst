//go:build integration

package runtimeapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/secretref"
)

// refsFake resolves a reference to a value, or to the failure "ERR:<code>".
type refsFake map[string]string

func (m refsFake) Resolve(_ context.Context, ref string) (string, error) {
	v, ok := m[ref]
	switch {
	case !ok:
		return "", &secretref.Error{Code: secretref.CodeNotFound, Ref: ref, Msg: "no such secret"}
	case len(v) > 4 && v[:4] == "ERR:":
		return "", &secretref.Error{Code: v[4:], Ref: ref, Msg: "fake failure"}
	}
	return v, nil
}

// A secret that is a reference is resolved when the runtime fetches it. One
// that does not resolve is answered 409 with its reference code (the ingestor
// backs off on it and says why) or, when the secret store cannot be reached,
// 503 SECRET_REFERENCE_UNAVAILABLE (retryable); the refusals are audited and
// logged as any refused fetch, and no value is in a response or a log.
func TestSecrets_ReferenceResolvedAndFailuresMapped(t *testing.T) {
	// Not parallel: it swaps the process's resolver (the write check).
	restore := secretref.SetDefault(secretref.New(secretref.Config{Allow: []string{"aws-sm://prod/"}}, nil))
	defer restore()
	e := newEnv(t)
	tg := e.makeTargets()
	i := e.saveTarget("secretref", sftpSource, tg.viaAccount(), true)
	cid := string(e.client.ID)
	const resolved = "pw-from-aws-do-not-log"
	e.config.Refs = refsFake{"aws-sm://prod/sftp#password": resolved, "aws-sm://prod/down": "ERR:" + secretref.CodeUnavailable}
	e.setSecret("sftp-password", "aws-sm://prod/sftp#password", &cid)
	holder := "rt-" + unique()
	require.Len(t, e.poll(holder, ingestion.ContractVersion).list("ingestions"), 1)
	logs := captureLogs(t)

	rec := e.api.Get(secretPath(i.ID, "sftp-password", holder))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var got struct{ Value string }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, resolved, got.Value)

	// Not found in AWS: a refusal.
	e.setSecret("sftp-password", "aws-sm://prod/gone", &cid)
	r := e.do(http.MethodGet, secretPath(i.ID, "sftp-password", holder), nil)
	assert.Equal(t, http.StatusConflict, r.code, r.raw)
	assert.Contains(t, r.raw, secretref.CodeNotFound)
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM aud_logs WHERE operation = 'SECRET_FETCH_REFUSED' AND entity_id = $1`, i.ID))

	// The store cannot be reached: 503, retryable.
	e.setSecret("sftp-password", "aws-sm://prod/down", &cid)
	r = e.do(http.MethodGet, secretPath(i.ID, "sftp-password", holder), nil)
	assert.Equal(t, http.StatusServiceUnavailable, r.code, r.raw)
	assert.Contains(t, r.raw, secretref.CodeUnavailable)

	raw, _ := json.Marshal(logs())
	assert.NotContains(t, string(raw), resolved, "the resolved value is never logged")
	assert.NotContains(t, r.raw, resolved)
}
