package operations_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func TestUpdateApplication_ValidateType(t *testing.T) {
	t.Parallel()
	op := operations.UpdateApplication(nil, nil)
	for _, ok := range []*string{nil, new("APPLICATION"), new("INTEGRATION")} {
		assert.NoError(t, op.Validate(context.Background(), operations.UpdateCommand{ID: "app_1", Type: ok}))
	}
	for _, bad := range []string{"", "integration", "OTHER"} {
		err := op.Validate(context.Background(), operations.UpdateCommand{ID: "app_1", Type: &bad})
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "type %q", bad)
		assert.Equal(t, usecase.KindValidation, ue.Kind)
		assert.Equal(t, "APPLICATION_TYPE_INVALID", ue.Code)
	}
}

func TestApplicationUpdated_DataCarriesTypes(t *testing.T) {
	t.Parallel()
	data := func(e operations.ApplicationUpdated) map[string]any {
		b, err := e.ToDataJSON()
		require.NoError(t, err)
		m := map[string]any{}
		require.NoError(t, json.Unmarshal(b, &m))
		return m
	}
	m := data(operations.ApplicationUpdated{ApplicationID: "app_1", Name: "N", Type: "INTEGRATION", PreviousType: "APPLICATION"})
	assert.Equal(t, "INTEGRATION", m["type"])
	assert.Equal(t, "APPLICATION", m["previousType"])
	m = data(operations.ApplicationUpdated{ApplicationID: "app_1", Name: "N", Type: "INTEGRATION"})
	assert.Equal(t, "INTEGRATION", m["type"])
	assert.NotContains(t, m, "previousType")
}
