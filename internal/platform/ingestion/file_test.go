package ingestion_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

const sftpFile = `application: ingest
client: acme
code: orders-sftp
name: Orders over SFTP
active: true
source:
  kind: sftp
  host: sftp.example.com
  port: 22
  directory: /in/orders
  username: feed
  credential:
    name: sftp-password
  include: '*.csv'
  every: PT5M
split:
  kind: csv
  header: true
target:
  code: acme:orders:order:received
  targetUrl: https://hooks.example.com/orders
  pool: orders
  connection: orders-conn
  mode: NEXT_ON_ERROR
  batchSize: 100
`

func TestYAMLRoundTripKeepsEveryFile(t *testing.T) {
	files, err := ingestion.ParseFiles([]byte(sftpFile))
	require.NoError(t, err)
	require.Len(t, files, 1)
	f := files[0]
	assert.Equal(t, "ingest", f.Application)
	assert.Equal(t, "acme", f.Client)
	assert.Equal(t, "orders-sftp", f.Code)
	assert.True(t, f.Active)
	assert.Empty(t, f.Validate())

	out, err := f.YAML()
	require.NoError(t, err)
	assert.Equal(t, sftpFile, string(out), "export writes the same file back")

	again, err := ingestion.ParseFiles(out)
	require.NoError(t, err)
	assert.Equal(t, files, again)
}

func TestYAMLRoundTripOfEverySourceKind(t *testing.T) {
	for _, c := range sourceCases {
		t.Run(c.name, func(t *testing.T) {
			d := parse(t, definition(c.source, c.split))
			f := ingestion.File{Application: "ingest", Client: "acme", Code: "x", Active: true, Definition: d}
			out, err := f.YAML()
			require.NoError(t, err)

			files, err := ingestion.ParseFiles(out)
			require.NoError(t, err, string(out))
			require.Len(t, files, 1)
			assert.True(t, f.Equal(files[0].Definition), string(out))
			assert.Equal(t, f, files[0])
		})
	}
}

func TestExtensionSourceIsWrittenFlat(t *testing.T) {
	d := parse(t, definition(`{"kind":"custom","type":"camel-route","settings":{"yaml":"from:\n  uri: timer:tick\n"}}`, `{"kind":"none"}`))
	f := ingestion.File{Application: "ingest", Client: "acme", Code: "tick", Active: true, Definition: d}
	out, err := f.YAML()
	require.NoError(t, err)
	text := string(out)
	assert.Contains(t, text, "  kind: camel-route\n")
	assert.NotContains(t, text, "custom")
	assert.Contains(t, text, "  yaml: |\n    from:\n      uri: timer:tick\n", "a multi-line setting is a literal block")
}

func TestKindCustomIsReadAsItIs(t *testing.T) {
	const doc = `application: ingest
client: acme
code: mix
name: Mix
source:
  kind: custom
  type: mix
  settings:
    apiUrl: https://api.mix.example
split:
  kind: none
target:
  code: a:b:c:d
  targetUrl: https://x.example.com
  pool: p
  mode: IMMEDIATE
  batchSize: 1
`
	files, err := ingestion.ParseFiles([]byte(doc))
	require.NoError(t, err)
	assert.Equal(t, ingestion.Custom{Type: "mix", Settings: map[string]any{"apiUrl": "https://api.mix.example"}}, files[0].Source)
}

func TestFileDefaultsAndMultipleDocuments(t *testing.T) {
	doc := "---\n" + strings.Replace(sftpFile, "active: true\n", "", 1) + "---\n" +
		strings.Replace(strings.Replace(sftpFile, "orders-sftp", "orders-two", 1), "active: true", "active: false", 1)
	files, err := ingestion.ParseFiles([]byte(doc))
	require.NoError(t, err)
	require.Len(t, files, 2)
	assert.True(t, files[0].Active, "active is true when the file does not say")
	assert.False(t, files[1].Active)
	assert.Equal(t, "orders-two", files[1].Code)
}

func TestKeepFailedPayloadsIsWrittenOnlyWhenOn(t *testing.T) {
	files, err := ingestion.ParseFiles([]byte(sftpFile))
	require.NoError(t, err)
	out, _ := files[0].YAML()
	assert.NotContains(t, string(out), "keepFailedPayloads")

	files[0].KeepFailedPayloads = true
	out, _ = files[0].YAML()
	assert.Contains(t, string(out), "keepFailedPayloads: true\n")
	again, err := ingestion.ParseFiles(out)
	require.NoError(t, err)
	assert.True(t, again[0].KeepFailedPayloads)
}

func TestFileRejectsWhatItDoesNotKnow(t *testing.T) {
	cases := map[string]string{
		"unknown top-level field": sftpFile + "bogus: 1\n",
		"unknown source field":    strings.Replace(sftpFile, "  port: 22\n", "  port: 22\n  portt: 23\n", 1),
		"duration as a number":    strings.Replace(sftpFile, "every: PT5M", "every: 300", 1),
		"unknown split kind":      strings.Replace(sftpFile, "kind: csv", "kind: tsv", 1),
		"not an object":           "- a\n- b\n",
		"broken yaml":             "application: [ingest\n",
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ingestion.ParseFiles([]byte(doc))
			assert.Error(t, err)
		})
	}
}

func TestSecretsStayNamesInTheFile(t *testing.T) {
	files, err := ingestion.ParseFiles([]byte(sftpFile))
	require.NoError(t, err)
	out, _ := files[0].YAML()
	assert.Contains(t, string(out), "name: sftp-password")
}
