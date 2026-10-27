//go:build integration

package api_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// The value of the entry an ingestion's source names is shown only to a caller
// who can update ingestions; a viewer gets the status and valueHidden.
func TestConfigValueIsShownOnlyToWhoCanUpdate(t *testing.T) {
	e := newEnv(t)
	key := "feed-" + unique()
	body := e.body(e.appA, e.clientA.Identifier, "cfg-"+unique())
	body["source"] = map[string]any{"kind": "sqs", "config": key}
	r := e.do(http.MethodPost, "/api/ingestions", body)
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	path := "/api/ingestions/" + r.str("id") + "/config"

	put := e.do(http.MethodPut, path, map[string]any{"value": map[string]any{"region": "eu-west-1", "queue": "q-secret-name"}})
	require.Equal(t, http.StatusOK, put.code, put.raw)
	require.NotNil(t, put.json["value"], "the writer is answered with the value")

	editor := e.do(http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, editor.code, editor.raw)
	require.Equal(t, "SET", editor.json["status"])
	require.Contains(t, editor.raw, "q-secret-name")
	require.Nil(t, editor.json["valueHidden"])

	e.as(anchor(permView))
	viewer := e.do(http.MethodGet, path, nil)
	require.Equal(t, http.StatusOK, viewer.code, viewer.raw)
	require.Equal(t, "SET", viewer.json["status"])
	require.Equal(t, key, viewer.json["key"])
	require.Equal(t, true, viewer.json["valueHidden"])
	require.Nil(t, viewer.json["value"])
	require.NotContains(t, viewer.raw, "q-secret-name")
}

// Nothing else a viewer reads (the ingestion, the list, its secrets) quotes
// the entry either: not its settings, its delivery URL, nor the secret names it
// binds. The editor reads them all.
func TestEntryContentIsNotInAnyOtherViewersAnswer(t *testing.T) {
	e := newEnv(t)
	key := "feed-" + unique()
	code := "cfg-" + unique()
	body := e.body(e.appA, e.clientA.Identifier, code)
	body["source"] = map[string]any{"kind": "sqs", "config": key}
	delete(body["target"].(map[string]any), "targetUrl")
	r := e.do(http.MethodPost, "/api/ingestions", body)
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	id := r.str("id")

	entry := map[string]any{
		"region": "eu-west-1", "queue": "q-vendor-queue",
		"credential": map[string]any{"accessKeyId": "AKIAVENDORKEY", "secretAccessKey": map[string]any{"name": "vendor-sqs-secret"}},
		"targetUrl":  "https://vendor-hook.example.com/in",
	}
	put := e.do(http.MethodPut, "/api/ingestions/"+id+"/config", map[string]any{"value": entry})
	require.Equal(t, http.StatusOK, put.code, put.raw)
	withheld := []string{"q-vendor-queue", "AKIAVENDORKEY", "vendor-sqs-secret", "vendor-hook.example.com"}

	paths := []string{"/api/ingestions/" + id, "/api/ingestions/" + id + "/secrets", "/api/ingestions?applicationCode=" + e.appA.Code}
	for _, path := range paths {
		got := e.do(http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, got.code, got.raw)
		if path == paths[1] {
			require.Contains(t, got.raw, "vendor-sqs-secret", "the editor reads the names")
		}
	}

	e.as(anchor(permView))
	for _, path := range paths {
		got := e.do(http.MethodGet, path, nil)
		require.Equal(t, http.StatusOK, got.code, got.raw)
		for _, w := range withheld {
			require.NotContains(t, got.raw, w, path)
		}
	}
	secrets := e.do(http.MethodGet, paths[1], nil)
	require.Equal(t, true, secrets.json["namesHidden"])
	require.Empty(t, secrets.json["secrets"])
	one := e.do(http.MethodGet, paths[0], nil)
	require.Equal(t, "SET", one.json["config"].(map[string]any)["status"], "the status is still told")
}
