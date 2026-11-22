package ingestion_test

import (
	"encoding/json"
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
	require.NotNil(t, f.Active)
	assert.True(t, *f.Active)
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
			f := ingestion.File{Application: "ingest", Client: "acme", Code: "x", Active: new(true), Definition: d}
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
	f := ingestion.File{Application: "ingest", Client: "acme", Code: "tick", Active: new(true), Definition: d}
	out, err := f.YAML()
	require.NoError(t, err)
	text := string(out)
	assert.Contains(t, text, "  kind: camel-route\n")
	assert.NotContains(t, text, "custom")
	assert.Contains(t, text, "  yaml: |\n    from:\n      uri: timer:tick\n", "a multi-line setting is a literal block")
}

// Owner's decision D6 (2026-11-22): an extension's source with no settings
// (nil, as the API stores one sent without settings, or empty) is written as
// its kind alone and reads back as the same definition, through the file and
// through the JSON a set item and the set archive carry.
func TestExtensionSourceWithoutSettingsIsItsKindAlone(t *testing.T) {
	for name, settings := range map[string]map[string]any{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			d := parse(t, definition(`{"kind":"sqs","region":"eu-west-1","queue":"q"}`, `{"kind":"none"}`))
			d.Source = ingestion.Custom{Type: "mix", Settings: settings}
			f := ingestion.File{Application: "ingest", Client: "acme", Code: "fleet", Active: new(true), Definition: d}
			out, err := f.YAML()
			require.NoError(t, err)
			assert.Contains(t, string(out), "source:\n  kind: mix\nsplit:\n", "the kind alone")
			assert.NotContains(t, string(out), "custom")

			files, err := ingestion.ParseFiles(out)
			require.NoError(t, err, string(out))
			require.Len(t, files, 1)
			assert.True(t, f.Equal(files[0].Definition), string(out))
			assert.Equal(t, ingestion.Custom{Type: "mix"}, files[0].Source, "read back with no settings")
			again, err := files[0].YAML()
			require.NoError(t, err)
			assert.Equal(t, string(out), string(again), "export writes the same file back")

			raw, err := json.Marshal(files[0])
			require.NoError(t, err)
			assert.Contains(t, string(raw), `"source":{"kind":"custom","type":"mix"}`)
			var back ingestion.File
			require.NoError(t, json.Unmarshal(raw, &back))
			assert.True(t, f.Equal(back.Definition), string(raw))
		})
	}
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
	assert.Nil(t, files[0].Active, "active is unsaid when the file does not say")
	require.NotNil(t, files[1].Active)
	assert.False(t, *files[1].Active)
	assert.Equal(t, "orders-two", files[1].Code)
}

func TestYAMLLeavesUnsaidActiveOut(t *testing.T) {
	unsaid := strings.Replace(sftpFile, "active: true\n", "", 1)
	files, err := ingestion.ParseFiles([]byte(unsaid))
	require.NoError(t, err)
	out, err := files[0].YAML()
	require.NoError(t, err)
	assert.Equal(t, unsaid, string(out), "a file that does not say active is written without it")
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

func TestClientsListGivesOneDefinitionPerClient(t *testing.T) {
	doc := strings.Replace(sftpFile, "client: acme\n", "clients:\n  - acme\n  - clt_0ABCDEFGHJKMN\n  - Globex\n", 1)
	files, err := ingestion.ParseFiles([]byte(doc))
	require.NoError(t, err)
	require.Len(t, files, 3)
	single, err := ingestion.ParseFiles([]byte(sftpFile))
	require.NoError(t, err)
	for n, client := range []string{"acme", "clt_0ABCDEFGHJKMN", "Globex"} {
		assert.Equal(t, client, files[n].Client, "in the order listed")
		want := single[0]
		want.Client = client
		assert.Equal(t, want, files[n], "everything but the client is the file's")
	}
}

func TestClientsListWithAnExtensionSource(t *testing.T) {
	doc := `application: ingest
clients: [acme, globex]
code: heartbeat
source:
  kind: camel-route
  yaml: "from: {uri: 'timer:x'}"
split:
  kind: json
target:
  code: acme:feed:beat:received
  targetUrl: https://hooks.example.com/beat
  pool: orders
`
	files, err := ingestion.ParseFiles([]byte(doc))
	require.NoError(t, err)
	require.Len(t, files, 2)
	for _, f := range files {
		c, ok := f.Source.(ingestion.Custom)
		require.True(t, ok, "%T", f.Source)
		assert.Equal(t, "camel-route", c.Type)
		assert.Equal(t, "from: {uri: 'timer:x'}", c.Settings["yaml"])
	}
	assert.Equal(t, []string{"acme", "globex"}, []string{files[0].Client, files[1].Client})
}

func TestClientsListBesideOtherDocuments(t *testing.T) {
	listed := strings.Replace(sftpFile, "client: acme\n", "clients: [acme, globex]\n", 1)
	doc := sftpFile + "---\n" + strings.Replace(listed, "orders-sftp", "orders-two", 1)
	files, err := ingestion.ParseFiles([]byte(doc))
	require.NoError(t, err)
	require.Len(t, files, 3)
	assert.Equal(t, "acme/orders-sftp", files[0].Client+"/"+files[0].Code)
	assert.Equal(t, "acme/orders-two", files[1].Client+"/"+files[1].Code)
	assert.Equal(t, "globex/orders-two", files[2].Client+"/"+files[2].Code)
}

func TestClientsListRefusals(t *testing.T) {
	cases := map[string]string{
		"client and clients": strings.Replace(sftpFile, "client: acme\n", "client: acme\nclients: [globex]\n", 1),
		"empty list":         strings.Replace(sftpFile, "client: acme\n", "clients: []\n", 1),
		"not a list":         strings.Replace(sftpFile, "client: acme\n", "clients: acme\n", 1),
		"blank entry":        strings.Replace(sftpFile, "client: acme\n", "clients: [acme, '']\n", 1),
		"not a string":       strings.Replace(sftpFile, "client: acme\n", "clients: [acme, {a: 1}]\n", 1),
		"listed twice":       strings.Replace(sftpFile, "client: acme\n", "clients: [acme, ACME]\n", 1),
		"star in a list":     strings.Replace(sftpFile, "client: acme\n", "clients: [acme, '*']\n", 1),
		"star and client":    strings.Replace(sftpFile, "client: acme\n", "client: acme\nclients: '*'\n", 1),
		"other text":         strings.Replace(sftpFile, "client: acme\n", "clients: all\n", 1),
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ingestion.ParseFiles([]byte(doc))
			assert.Error(t, err)
		})
	}
}

func TestClientsStarIsEveryClient(t *testing.T) {
	doc := strings.Replace(sftpFile, "client: acme\n", "clients: \"*\"\n", 1)
	files, err := ingestion.ParseFiles([]byte(doc))
	require.NoError(t, err)
	require.Len(t, files, 1, "the apply expands it, against the clients there are then")
	assert.Equal(t, ingestion.AllClients, files[0].Client)
	assert.Empty(t, files[0].Validate())
}
