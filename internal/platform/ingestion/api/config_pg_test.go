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
