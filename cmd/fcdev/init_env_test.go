package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeEnvForTest(t *testing.T, path string, updates [][2]string) string {
	t.Helper()
	var out bytes.Buffer
	require.NoError(t, writeEnvUpdates(&out, path, updates))
	return out.String()
}

func readFileForTest(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

func TestWriteEnvUpdates_NewFileAppendsSortedUnderHeader(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".env")

	out := writeEnvForTest(t, path, [][2]string{
		{"FLOWCATALYST_CLIENT_ID", "cid"},
		{envSigningSecret, "sig"},
		{envWebhookAuthToken, "fc_tok"},
		{"FLOWCATALYST_BASE_URL", "http://localhost:1"},
	})

	assert.Equal(t, "# FlowCatalyst (added by `fcdev init`)\n"+
		"FLOWCATALYST_BASE_URL=http://localhost:1\n"+
		"FLOWCATALYST_CLIENT_ID=cid\n"+
		"FLOWCATALYST_SIGNING_SECRET=sig\n"+
		"FLOWCATALYST_WEBHOOK_AUTH_TOKEN=fc_tok\n", readFileForTest(t, path))
	assert.Contains(t, out, "created")
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

// A secret the developer set is replaced in place, as every key init writes
// is, but its old value stays in a comment above it; other keys are untouched.
func TestWriteEnvUpdates_ReplacedSecretKeepsOldValueInComment(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".env")
	require.NoError(t, os.WriteFile(path, []byte("APP_NAME=shop\nFLOWCATALYST_SIGNING_SECRET=mine\nFLOWCATALYST_CLIENT_ID=old-id\n"), 0o600))

	out := writeEnvForTest(t, path, [][2]string{
		{"FLOWCATALYST_CLIENT_ID", "new-id"},
		{envSigningSecret, "minted"},
		{envWebhookAuthToken, "fc_tok"},
	})

	assert.Equal(t, "APP_NAME=shop\n"+
		"# replaced by fcdev init: FLOWCATALYST_SIGNING_SECRET=mine\n"+
		"FLOWCATALYST_SIGNING_SECRET=minted\n"+
		"FLOWCATALYST_CLIENT_ID=new-id\n"+
		"\n"+
		"# FlowCatalyst (added by `fcdev init`)\n"+
		"FLOWCATALYST_WEBHOOK_AUTH_TOKEN=fc_tok\n", readFileForTest(t, path),
		"a non-secret key (the client id) is rewritten without a comment")
	assert.Contains(t, out, "FLOWCATALYST_SIGNING_SECRET replaced")
	assert.NotContains(t, out, "minted", "a secret is never printed")
	assert.NotContains(t, out, "mine", "a secret is never printed")
}

// Running init again (fcdev fresh, then init) keeps one comment per secret,
// holding the value just replaced, so the file does not grow per run.
func TestWriteEnvUpdates_SecondReplacementSupersedesComment(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".env")
	require.NoError(t, os.WriteFile(path, []byte("FLOWCATALYST_SIGNING_SECRET=mine\n"), 0o600))

	writeEnvForTest(t, path, [][2]string{{envSigningSecret, "first"}})
	writeEnvForTest(t, path, [][2]string{{envSigningSecret, "second"}})

	got := readFileForTest(t, path)
	assert.Equal(t, "# replaced by fcdev init: FLOWCATALYST_SIGNING_SECRET=first\n"+
		"FLOWCATALYST_SIGNING_SECRET=second\n", got)
	assert.Equal(t, 1, strings.Count(got, envReplacedPrefix))
}

func TestWriteEnvUpdates_EmptyOrSameValueKeepsNoComment(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".env")
	require.NoError(t, os.WriteFile(path, []byte("FLOWCATALYST_SIGNING_SECRET=''\nFLOWCATALYST_WEBHOOK_AUTH_TOKEN=fc_same\n"), 0o600))

	writeEnvForTest(t, path, [][2]string{{envSigningSecret, "minted"}, {envWebhookAuthToken, "fc_same"}})
	assert.Equal(t, "FLOWCATALYST_SIGNING_SECRET=minted\nFLOWCATALYST_WEBHOOK_AUTH_TOKEN=fc_same\n", readFileForTest(t, path))

	out := writeEnvForTest(t, path, [][2]string{{envSigningSecret, "minted"}})
	assert.Contains(t, out, "already current")
}

// A replaced-comment of a key that is not being changed stays as it is.
func TestWriteEnvUpdates_UnrelatedReplacedCommentKept(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), ".env")
	start := "# replaced by fcdev init: FLOWCATALYST_CLIENT_SECRET=older\nFLOWCATALYST_CLIENT_SECRET=current\nFLOWCATALYST_SIGNING_SECRET=s1\n"
	require.NoError(t, os.WriteFile(path, []byte(start), 0o600))

	writeEnvForTest(t, path, [][2]string{{envSigningSecret, "s2"}})

	assert.Equal(t, "# replaced by fcdev init: FLOWCATALYST_CLIENT_SECRET=older\n"+
		"FLOWCATALYST_CLIENT_SECRET=current\n"+
		"# replaced by fcdev init: FLOWCATALYST_SIGNING_SECRET=s1\n"+
		"FLOWCATALYST_SIGNING_SECRET=s2\n", readFileForTest(t, path))
}

func TestUnquoteEnvValue(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"": "", "''": "", `""`: "", "'a b'": "a b", "x": "x", " y ": "y"} {
		assert.Equal(t, want, unquoteEnvValue(in), in)
	}
}
