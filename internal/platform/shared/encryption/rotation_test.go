package encryption_test

import (
	"crypto/hkdf"
	"crypto/sha256"
	"slices"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
)

func TestParsePreviousKeys(t *testing.T) {
	cases := []struct {
		in, current string
		want        []string
	}{
		{"", "cur", nil},
		{"old", "cur", []string{"old"}},
		{" old1 , old2 ,, old1, cur ,", "cur", []string{"old1", "old2"}},
		{"cur", "cur", nil},
	}
	for _, c := range cases {
		if got := encryption.ParsePreviousKeys(c.in, c.current); !slices.Equal(got, c.want) {
			t.Errorf("ParsePreviousKeys(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Every key in a comma-separated FLOWCATALYST_APP_KEY_PREVIOUS decrypts: a
// value sealed two rotations ago still reads.
func TestFromEnvAcceptsAListOfPreviousKeys(t *testing.T) {
	k1, _ := encryption.GenerateKey()
	k2, _ := encryption.GenerateKey()
	k3, _ := encryption.GenerateKey()
	s1, _ := encryption.New(k1)
	s2, _ := encryption.New(k2)
	ct1, _ := s1.Encrypt("one")
	ct2, _ := s2.Encrypt("two")

	t.Setenv(encryption.EnvAppKey, k3)
	t.Setenv(encryption.EnvAppKeyPrevious, k2+", "+k1)
	svc, err := encryption.FromEnv()
	if err != nil || svc == nil {
		t.Fatalf("FromEnv: %v %v", svc, err)
	}
	for ct, want := range map[string]string{ct1: "one", ct2: "two"} {
		if got, err := svc.Decrypt(ct); err != nil || got != want {
			t.Fatalf("Decrypt = %q, %v; want %q", got, err, want)
		}
	}
	// A single value keeps working.
	t.Setenv(encryption.EnvAppKeyPrevious, k1)
	svc, err = encryption.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv single previous: %v", err)
	}
	if got, err := svc.Decrypt(ct1); err != nil || got != "one" {
		t.Fatalf("Decrypt single previous = %q, %v", got, err)
	}
}

func TestFromEnvRejectsAMalformedPreviousEntry(t *testing.T) {
	k1, _ := encryption.GenerateKey()
	k2, _ := encryption.GenerateKey()
	t.Setenv(encryption.EnvAppKey, k1)
	t.Setenv(encryption.EnvAppKeyPrevious, k2+",not-a-key")
	if _, err := encryption.FromEnv(); err == nil || !strings.Contains(err.Error(), "previous key 1") {
		t.Fatalf("FromEnv with a malformed second previous key: err = %v, want one naming previous key 1", err)
	}
}

func TestDecryptCurrent(t *testing.T) {
	oldKey, _ := encryption.GenerateKey()
	newKey, _ := encryption.GenerateKey()
	oldSvc, _ := encryption.New(oldKey)
	svc, _ := encryption.WithPreviousKeys(newKey, []string{oldKey})

	oldCT, _ := oldSvc.Encrypt("v")
	newCT, _ := svc.Encrypt("v")

	if pt, cur, err := svc.DecryptCurrent(oldCT); err != nil || pt != "v" || cur {
		t.Fatalf("old-key value: %q current=%v err=%v; want readable, not current", pt, cur, err)
	}
	if pt, cur, err := svc.DecryptCurrent("encrypted:" + newCT); err != nil || pt != "v" || !cur {
		t.Fatalf("current-key value: %q current=%v err=%v; want readable and current", pt, cur, err)
	}
	if _, _, err := svc.DecryptCurrent("garbage!"); err == nil {
		t.Fatalf("garbage must not decrypt")
	}
}

func TestDecryptBoundCurrent(t *testing.T) {
	oldKey, _ := encryption.GenerateKey()
	newKey, _ := encryption.GenerateKey()
	oldSvc, _ := encryption.New(oldKey)
	svc, _ := encryption.WithPreviousKeys(newKey, []string{oldKey})
	aad := []byte("row-1")

	oldBound, _ := oldSvc.EncryptBound("v", aad)
	newBound, _ := svc.EncryptBound("v", aad)
	newUnbound, _ := svc.Encrypt("v")

	for name, c := range map[string]struct {
		ct   string
		want bool
	}{
		"bound under the previous key":  {oldBound, false},
		"bound under the current key":   {newBound, true},
		"unbound under the current key": {newUnbound, false},
	} {
		pt, cur, err := svc.DecryptBoundCurrent(c.ct, aad)
		if err != nil || pt != "v" || cur != c.want {
			t.Errorf("%s: %q current=%v err=%v; want current=%v", name, pt, cur, err, c.want)
		}
	}
	if _, _, err := svc.DecryptBoundCurrent(newBound, []byte("row-2")); err == nil {
		t.Fatalf("a bound value must not open under another row's aad")
	}
}

// DeriveKeys must keep producing exactly what the dispatch-auth and PIN-pepper
// derivations always produced (HKDF over the key's text), or every in-flight
// token and stored PIN breaks on upgrade.
func TestDeriveKeysMatchesTheHistoricDerivation(t *testing.T) {
	cur, prev, err := encryption.DeriveKeys("current-key-text", []string{"old-key-text"}, "fc-dispatch-auth")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := hkdf.Key(sha256.New, []byte("current-key-text"), nil, "fc-dispatch-auth", 32)
	wantPrev, _ := hkdf.Key(sha256.New, []byte("old-key-text"), nil, "fc-dispatch-auth", 32)
	if string(cur) != string(want) || len(prev) != 1 || string(prev[0]) != string(wantPrev) {
		t.Fatalf("DeriveKeys drifted from HKDF-SHA256(key text, info)")
	}
}
