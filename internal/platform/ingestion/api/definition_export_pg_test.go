//go:build integration

package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

// Owner's decision D6 (2026-11-22): exporting an ingestion whose extension
// source has no settings answers its definition file (it answered 500
// MARSHAL), the source written as its kind alone; the file reads back as the
// same definition, and applied as a set file (the JSON fcdev sends) it takes
// the ingestion in unchanged, through the apply's archive and a reapply too.
func TestDefinitionExportOfAnExtensionSourceWithoutSettings(t *testing.T) {
	e := newEnv(t)
	e.asSetAdmin()
	code, kind := "fleet-"+unique(), "mix-"+unique()
	body := e.body(e.appA, e.clientA.Identifier, code)
	body["source"] = map[string]any{"kind": "custom", "type": kind}
	r := e.do(http.MethodPost, "/api/ingestions", body)
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	id := r.str("id")

	rec := e.api.Get("/api/ingestions/" + id + "/definition")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	exported := rec.Body.String()
	assert.Contains(t, exported, "source:\n  kind: "+kind+"\nsplit:\n", "the kind alone")
	files, err := ingestion.ParseFiles(rec.Body.Bytes())
	require.NoError(t, err, exported)
	require.Len(t, files, 1)
	stored := e.byCode(e.appA.Code, string(e.clientA.ID), code)
	require.NotNil(t, stored)
	assert.True(t, stored.Definition.Equal(files[0].Definition), exported)
	assert.Equal(t, ingestion.Custom{Type: kind}, files[0].Source)

	// The file as a set item: adopted, nothing changed, the version kept.
	raw, err := json.Marshal(files[0])
	require.NoError(t, err)
	var item map[string]any
	require.NoError(t, json.Unmarshal(raw, &item))
	name := e.setName()
	r = e.putSet(name, item)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, map[string]string{code: "ADOPT"}, actions(r))
	changes := r.json["changes"].([]any)
	require.Len(t, changes, 1)
	assert.Empty(t, changes[0].(map[string]any)["changed"])
	assert.Equal(t, 1, e.byCode(e.appA.Code, string(e.clientA.ID), code).Version)

	// The apply's archive holds the item, and going back to it changes nothing.
	applyID := r.str("applyId")
	require.NotEmpty(t, applyID)
	got := e.do(http.MethodGet, "/api/ingestion-sets/"+name+"/applies/"+applyID, nil)
	require.Equal(t, http.StatusOK, got.code, got.raw)
	again := e.do(http.MethodPost, "/api/ingestion-sets/"+name+"/applies/"+applyID+"/reapply", nil)
	require.Equal(t, http.StatusOK, again.code, again.raw)
	assert.Equal(t, map[string]string{code: "UNCHANGED"}, actions(again))

	rec = e.api.Get("/api/ingestions/" + id + "/definition")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, exported, rec.Body.String(), "the export is the same after the apply")
}
