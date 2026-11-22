//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	jsonschema "github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
)

// Owner's decision D7 (2026-11-22): the data platform:ingestion:ingestion:
// config-set is stored with, as a config write emits it (secretsSet the names
// of the secrets the write set, null when it set none), validates against its
// catalogued schema.
func TestConfigSet_EmittedDataValidatesAgainstTheCatalogue(t *testing.T) {
	e := newEnv(t)
	var schema *jsonschema.Resolved
	for _, d := range seed.PlatformEventTypes() {
		if d.Code == operations.IngestionConfigSetType {
			var s jsonschema.Schema
			require.NoError(t, json.Unmarshal(d.Schema, &s))
			var err error
			schema, err = s.Resolve(nil)
			require.NoError(t, err)
		}
	}
	require.NotNil(t, schema, "config-set is catalogued")

	key := "feed-" + unique()
	id := e.sftpConfigured(key)
	path := "/api/ingestions/" + id + "/config"
	name := key + "-credential"
	emitted := func() []map[string]any {
		t.Helper()
		rows, err := e.pool.Query(context.Background(),
			`SELECT data::text FROM msg_events WHERE subject = $1 AND type = $2 ORDER BY id`,
			"platform.ingestion."+id, operations.IngestionConfigSetType)
		require.NoError(t, err)
		defer rows.Close()
		var out []map[string]any
		for rows.Next() {
			var raw string
			require.NoError(t, rows.Scan(&raw))
			var data map[string]any
			require.NoError(t, json.Unmarshal([]byte(raw), &data))
			out = append(out, data)
		}
		require.NoError(t, rows.Err())
		return out
	}

	// A write that sets a secret, then one that sets none.
	put := e.do(http.MethodPut, path, map[string]any{"value": e.sftpEntry(nil), "secretFields": map[string]any{"credential": cfgPassword}})
	require.Equal(t, http.StatusOK, put.code, put.raw)
	put = e.do(http.MethodPut, path, map[string]any{"value": e.sftpEntry(map[string]any{"name": name})})
	require.Equal(t, http.StatusOK, put.code, put.raw)

	events := emitted()
	require.Len(t, events, 2)
	assert.Equal(t, []any{name}, events[0]["secretsSet"])
	assert.Contains(t, events[1], "secretsSet")
	assert.Nil(t, events[1]["secretsSet"], "null when the write set no secret")
	for i, data := range events {
		assert.NoError(t, schema.Validate(data), "event %d: %v", i, data)
	}

	events[1]["unknown"] = "x"
	assert.Error(t, schema.Validate(events[1]), "the schema stays closed")
}
