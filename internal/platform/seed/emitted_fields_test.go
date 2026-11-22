package seed

import (
	"encoding/json"
	"testing"

	jsonschema "github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	ingops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
)

// seededSchema is the catalogued schema of code, resolved.
func seededSchema(t *testing.T, code string) *jsonschema.Resolved {
	t.Helper()
	var schema jsonschema.Schema
	require.NoError(t, json.Unmarshal(platformEventSchemas()[code], &schema))
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)
	return resolved
}

// dataOf is e's data as a subscriber decodes it.
func dataOf(t *testing.T, e interface{ ToDataJSON() ([]byte, error) }) map[string]any {
	t.Helper()
	raw, err := e.ToDataJSON()
	require.NoError(t, err)
	var data map[string]any
	require.NoError(t, json.Unmarshal(raw, &data))
	return data
}

// Owner's decision D7 (2026-11-22): the two events whose data their seeded
// schemas refused validate in every shape their use cases emit, and in the
// shape an older instance emitted without the new fields; the schemas stay
// closed. (The Postgres-backed tests of the two use cases validate the data
// they store: application/operations event_schema_pg_test.go and
// ingestion/api config_event_schema_pg_test.go.)
func TestEmittedFieldsValidateAgainstTheSeededSchemas(t *testing.T) {
	t.Run("application updated", func(t *testing.T) {
		schema := seededSchema(t, appops.ApplicationUpdatedType)
		for name, e := range map[string]appops.ApplicationUpdated{
			"the type unchanged": {ApplicationID: "app_1", Name: "A", Type: "APPLICATION"},
			"the type changed":   {ApplicationID: "app_1", Name: "A", Type: "INTEGRATION", PreviousType: "APPLICATION"},
			"an older instance":  {ApplicationID: "app_1", Name: "A"},
		} {
			data := dataOf(t, e)
			assert.NoError(t, schema.Validate(data), "%s: %v", name, data)
		}
		data := dataOf(t, appops.ApplicationUpdated{ApplicationID: "app_1", Name: "A", Type: "APPLICATION"})
		data["description"] = "x"
		assert.Error(t, schema.Validate(data), "the schema stays closed")
	})
	t.Run("ingestion config set", func(t *testing.T) {
		schema := seededSchema(t, ingops.IngestionConfigSetType)
		for name, e := range map[string]ingops.IngestionConfigSet{
			"no secret set": {Code: "c", Key: "k", Settings: []string{"host"}, Ingestions: []string{"c"}},
			"secrets set":   {Code: "c", Key: "k", Settings: []string{"host"}, SecretsSet: []string{"k-credential"}, Ingestions: []string{"c"}},
		} {
			data := dataOf(t, e)
			assert.Contains(t, data, "secretsSet", name)
			assert.NoError(t, schema.Validate(data), "%s: %v", name, data)
		}
		data := dataOf(t, ingops.IngestionConfigSet{Code: "c", Key: "k", Settings: []string{"host"}, Ingestions: []string{"c"}})
		delete(data, "secretsSet")
		assert.NoError(t, schema.Validate(data), "an older instance's data without secretsSet")
		data["secretsSet"] = "k-credential"
		assert.Error(t, schema.Validate(data), "secretsSet is a list")
		data["secretsSet"] = nil
		data["warnings"] = []any{"SCHEMA_UNCHECKED"}
		assert.Error(t, schema.Validate(data), "the schema stays closed")
	})
}
