package seed

import (
	"encoding/json"
	"testing"

	jsonschema "github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	djops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/operations"
)

// The payload edit's audit event validates against its seeded schema with the
// payload ids present (both, or only one) and absent, as each switch state
// and job shape emits it; an unknown field still does not.
func TestPayloadEditedDataValidatesAgainstTheSeededSchema(t *testing.T) {
	var rawSchema json.RawMessage
	for _, d := range PlatformEventTypes() {
		if d.Code == djops.DispatchJobPayloadEditedType {
			rawSchema = d.Schema
		}
	}
	require.NotNil(t, rawSchema)
	var schema jsonschema.Schema
	require.NoError(t, json.Unmarshal(rawSchema, &schema))
	resolved, err := schema.Resolve(nil)
	require.NoError(t, err)

	base := djops.DispatchJobPayloadEdited{
		Status: "FAILED", PreviousSize: 10, PreviousSHA256: "a", Size: 12, SHA256: "b",
		PreviousContentType: "application/json", ContentType: "application/json",
	}
	for name, ids := range map[string][2]string{
		"legacy job, switch off":    {"", ""},
		"legacy job, switch on":     {"", "0PAYLOADNEW01"},
		"new-shape job, switch on":  {"0PAYLOADOLD01", "0PAYLOADNEW01"},
		"new-shape job, switch off": {"0PAYLOADOLD01", ""},
	} {
		e := base
		e.PreviousPayloadID, e.PayloadID = ids[0], ids[1]
		raw, err := e.ToDataJSON()
		require.NoError(t, err)
		var data map[string]any
		require.NoError(t, json.Unmarshal(raw, &data))
		assert.NoError(t, resolved.Validate(data), "%s: %s", name, raw)
		_, hasPrev := data["previousPayloadId"]
		_, hasNew := data["payloadId"]
		assert.Equal(t, ids[0] != "", hasPrev, "%s: previousPayloadId present only when set", name)
		assert.Equal(t, ids[1] != "", hasNew, "%s: payloadId present only when set", name)
	}

	raw, err := base.ToDataJSON()
	require.NoError(t, err)
	var data map[string]any
	require.NoError(t, json.Unmarshal(raw, &data))
	data["payload"] = "{}"
	assert.Error(t, resolved.Validate(data), "the schema stays closed: no payload text")
}
