package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/keyrotation"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
)

func TestReencryptRefusesBadInvocationsBeforeTouchingTheDatabase(t *testing.T) {
	key, _ := encryption.GenerateKey()
	cases := []struct {
		name, cur, prev string
		args            []string
		want            int
		stderr          string
	}{
		{"unknown flag", key, "", []string{"--force"}, reencryptUsage, "usage"},
		{"stray argument", key, "", []string{"now"}, reencryptUsage, "usage"},
		{"bad batch size", key, "", []string{"--batch-size", "0"}, reencryptUsage, "usage"},
		{"no key", "", "", nil, reencryptFailed, "FLOWCATALYST_APP_KEY is not set"},
		{"malformed previous key", key, key[:10] + "," + "bad", nil, reencryptFailed, "previous key"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv(encryption.EnvAppKey, c.cur)
			t.Setenv(encryption.EnvAppKeyPrevious, c.prev)
			// An unreachable database: reaching it would fail differently.
			t.Setenv("FC_DATABASE_URL", "postgres://nobody@127.0.0.1:1/none?connect_timeout=1")
			var out, errOut bytes.Buffer
			if got := runReencrypt(c.args, &out, &errOut); got != c.want {
				t.Fatalf("exit %d, want %d (stderr %q)", got, c.want, errOut.String())
			}
			if !strings.Contains(errOut.String(), c.stderr) {
				t.Fatalf("stderr %q, want it to mention %q", errOut.String(), c.stderr)
			}
		})
	}
}

func TestPrintReencryptResults(t *testing.T) {
	var out bytes.Buffer
	n := printReencryptResults(&out, []keyrotation.Result{
		{Target: "a.b", Scanned: 3, Reencrypted: 2, Unreadable: 1},
		{Target: "c.d", Scanned: 1, Hashed: 1},
	}, true)
	if n != 1 {
		t.Fatalf("unreadable total %d, want 1", n)
	}
	s := out.String()
	for _, want := range []string{"WOULD_REENCRYPT", "a.b", "c.d"} {
		if !strings.Contains(s, want) {
			t.Fatalf("output %q lacks %q", s, want)
		}
	}
}
