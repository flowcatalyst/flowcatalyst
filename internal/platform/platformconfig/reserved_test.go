package platformconfig_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

func TestRequireWritableSection(t *testing.T) {
	require.NoError(t, platformconfig.RequireWritableSection("secrets"))
	require.NoError(t, platformconfig.RequireWritableSection("ingestion"))
	require.NoError(t, platformconfig.RequireWritableSection(""))
	for _, section := range []string{"ingest", "INGEST", "  ingest "} {
		ue := usecase.AsError(platformconfig.RequireWritableSection(section))
		require.NotNil(t, ue, section)
		assert.Equal(t, "CONFIG_SECTION_RESERVED", ue.Code)
		assert.Equal(t, usecase.KindValidation, ue.Kind)
	}
}
