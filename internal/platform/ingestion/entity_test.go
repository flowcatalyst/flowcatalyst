package ingestion_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

func twoDefs(t *testing.T) (ingestion.Definition, ingestion.Definition) {
	t.Helper()
	const source = `{"kind":"sqs","region":"eu-west-1","queue":"orders"}`
	return parse(t, definition(source, `{"kind":"none"}`)), parse(t, definition(source, `{"kind":"csv","header":true}`))
}

func TestNewSavesAndMakesTheFirstVersionLive(t *testing.T) {
	a, _ := twoDefs(t)
	i := ingestion.New(ids.NewApplicationID(), ids.NewClientID(), "orders", a, true, nil, nil)
	assert.Contains(t, i.ID, "ing_")
	assert.Equal(t, 1, i.Version)
	assert.Equal(t, 1, i.LatestVersion)
	assert.True(t, i.Active)
	assert.True(t, i.Live())
}

func TestNewNotLiveHasNothingLiveYet(t *testing.T) {
	a, _ := twoDefs(t)
	i := ingestion.New(ids.NewApplicationID(), ids.NewClientID(), "orders", a, false, nil, nil)
	assert.Equal(t, 0, i.Version)
	assert.Equal(t, 1, i.LatestVersion)
	assert.False(t, i.Live())
	assert.Equal(t, a, i.Definition, "the row shows the draft while nothing is live")
}

func TestSavedVersionIsNotRunUntilMadeLive(t *testing.T) {
	a, b := twoDefs(t)
	i := ingestion.New(ids.NewApplicationID(), ids.NewClientID(), "orders", a, true, nil, nil)

	v2 := i.SaveVersion(b, false, nil, nil)
	assert.Equal(t, 2, v2.Number)
	assert.Equal(t, 1, i.Version, "the live pointer did not move")
	assert.Equal(t, 2, i.LatestVersion)
	assert.Equal(t, a, i.Definition, "the row still holds the live definition")

	require.NoError(t, i.MakeLive(v2))
	assert.Equal(t, 2, i.Version)
	assert.Equal(t, b, i.Definition)

	// Rolling back is making an older saved version live again.
	v1 := ingestion.Version{IngestionID: i.ID, Number: 1, Definition: a}
	require.NoError(t, i.MakeLive(v1))
	assert.Equal(t, 1, i.Version)
	assert.Equal(t, 2, i.LatestVersion, "latest only grows")
}

func TestMakeLiveRefusesWhatWasNeverSaved(t *testing.T) {
	a, _ := twoDefs(t)
	i := ingestion.New(ids.NewApplicationID(), ids.NewClientID(), "orders", a, true, nil, nil)
	assert.ErrorIs(t, i.MakeLive(ingestion.Version{IngestionID: i.ID, Number: 2, Definition: a}), ingestion.ErrNotSaved)
	assert.ErrorIs(t, i.MakeLive(ingestion.Version{IngestionID: "ing_other", Number: 1, Definition: a}), ingestion.ErrNotSaved)
	assert.Equal(t, 1, i.Version)
}

func TestPauseKeepsTheDefinition(t *testing.T) {
	a, _ := twoDefs(t)
	i := ingestion.New(ids.NewApplicationID(), ids.NewClientID(), "orders", a, true, nil, nil)
	i.Pause()
	assert.False(t, i.Live())
	assert.Equal(t, 1, i.Version)
	i.Resume()
	assert.True(t, i.Live())
}
