package ingestion_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

// The JSON form of a file reads back as the file: the archive depends on it.
func TestFileJSONRoundTrip(t *testing.T) {
	files, err := ingestion.ParseFiles([]byte(sftpFile))
	require.NoError(t, err)
	require.Len(t, files, 1)
	files[0].Active = false

	raw, err := json.Marshal(files[0])
	require.NoError(t, err)
	var back ingestion.File
	require.NoError(t, json.Unmarshal(raw, &back))

	assert.Equal(t, "ingest", back.Application)
	assert.Equal(t, "acme", back.Client)
	assert.Equal(t, "orders-sftp", back.Code)
	assert.False(t, back.Active)
	assert.True(t, files[0].Equal(back.Definition))
}

func TestSetArchiveEncodeIsContentAddressed(t *testing.T) {
	a := ingestion.SetArchive{Set: "acme", ApplyID: "isa_1", Items: []ingestion.File{}, Files: []ingestion.SetFile{}}
	d1, data, err := a.Encode()
	require.NoError(t, err)
	assert.Len(t, d1, 64)
	assert.NotEmpty(t, data)
	a.ApplyID = "isa_2"
	d2, _, err := a.Encode()
	require.NoError(t, err)
	assert.NotEqual(t, d1, d2)
}

func TestChangedFields(t *testing.T) {
	files, err := ingestion.ParseFiles([]byte(sftpFile))
	require.NoError(t, err)
	def := files[0].Definition
	assert.Empty(t, def.ChangedFields(def))

	other := def
	other.Name = "Renamed"
	other.KeepFailedPayloads = true
	other.Split = ingestion.None{}
	assert.Equal(t, []string{"name", "keepFailedPayloads", "split"}, def.ChangedFields(other))
}

func TestValidSetName(t *testing.T) {
	for _, ok := range []string{"acme", "a", "acme-orders-2"} {
		assert.True(t, ingestion.ValidSetName(ok), ok)
	}
	for _, bad := range []string{"", "Acme", "-a", "a-", "a_b", "a b"} {
		assert.False(t, ingestion.ValidSetName(bad), bad)
	}
}
