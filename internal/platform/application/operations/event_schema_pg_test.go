//go:build integration

package operations_test

import (
	"context"
	"encoding/json"
	"testing"

	jsonschema "github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// cataloguedSchema is the seeded schema of event type code, resolved.
func cataloguedSchema(t *testing.T, code string) *jsonschema.Resolved {
	t.Helper()
	for _, d := range seed.PlatformEventTypes() {
		if d.Code == code {
			var schema jsonschema.Schema
			require.NoError(t, json.Unmarshal(d.Schema, &schema))
			resolved, err := schema.Resolve(nil)
			require.NoError(t, err)
			return resolved
		}
	}
	t.Fatalf("event type %s is not catalogued", code)
	return nil
}

// Owner's decision D7 (2026-11-22): the data platform:iam:application:updated
// is stored with, as the update use case emits it (type always, previousType
// when the type changed), validates against its catalogued schema.
func TestApplicationUpdated_EmittedDataValidatesAgainstTheCatalogue(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := application.NewRepository(pool)
	uow := testpg.NewUoW(t)
	seeded := mustCreateApp(t, repo, uow, "appupdschema1", "Before")
	schema := cataloguedSchema(t, operations.ApplicationUpdatedType)

	stored := func(eventID string) map[string]any {
		t.Helper()
		var raw string
		require.NoError(t, pool.QueryRow(ctx, `SELECT data::text FROM msg_events WHERE id = $1`, eventID).Scan(&raw))
		var data map[string]any
		require.NoError(t, json.Unmarshal([]byte(raw), &data))
		return data
	}

	renamed, err := runAuthorized(uow, operations.UpdateApplication(repo, fakeIngestions(0)),
		operations.UpdateCommand{ID: seeded.ApplicationID, Name: new("After")})
	require.NoError(t, err)
	data := stored(renamed.EventID())
	assert.Equal(t, "APPLICATION", data["type"])
	assert.NotContains(t, data, "previousType", "the type did not change")
	assert.NoError(t, schema.Validate(data), "%v", data)

	retyped, err := runAuthorized(uow, operations.UpdateApplication(repo, fakeIngestions(0)),
		operations.UpdateCommand{ID: seeded.ApplicationID, Type: new("INTEGRATION")})
	require.NoError(t, err)
	data = stored(retyped.EventID())
	assert.Equal(t, "INTEGRATION", data["type"])
	assert.Equal(t, "APPLICATION", data["previousType"])
	assert.NoError(t, schema.Validate(data), "%v", data)

	data["unknown"] = "x"
	assert.Error(t, schema.Validate(data), "the schema stays closed")
}
