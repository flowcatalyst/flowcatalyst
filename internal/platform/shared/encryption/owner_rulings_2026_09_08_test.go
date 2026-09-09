package encryption

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// Owner rulings of 2026-09-08, mirrored from the Java port
// (docs/spec/encryption.md §1, §2, §3, §4).

func newService(t *testing.T) *Service {
	t.Helper()
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	enc, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	return enc
}

// ── §3 the external-scheme list is closed, and rejects ───────────────────────

func TestUnknownSchemeIsRejectedNotSealedAsThoughItWereTheSecret(t *testing.T) {
	enc := newService(t)
	for _, ref := range []string{"aws-smm://prod/db-password", "vaultt://secret/x", "gcp://x"} {
		out, err := EncryptSecretRef(enc, ptr(ref))
		if err == nil {
			t.Fatalf("%q: want rejection, got stored %q", ref, *out)
		}
		if !errors.Is(err, ErrUnsupportedScheme) {
			t.Fatalf("%q: want ErrUnsupportedScheme, got %v", ref, err)
		}
		if out != nil {
			t.Fatalf("%q: a rejected write must store nothing, got %q", ref, *out)
		}
		// The message has to be actionable: what was wrong, what is allowed,
		// and the way out.
		for _, want := range []string{"aws-sm://", "encrypt:"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("%q: message %q does not mention %q", ref, err.Error(), want)
			}
		}
	}
}

func TestEverySupportedSchemeStillPassesThroughVerbatim(t *testing.T) {
	enc := newService(t)
	for _, scheme := range []string{"aws-sm", "aws-ps", "gcp-sm", "vault", "env"} {
		ref := scheme + "://prod/db-password"
		out, err := EncryptSecretRef(enc, ptr(ref))
		if err != nil {
			t.Fatalf("%s: %v", scheme, err)
		}
		if *out != ref {
			t.Fatalf("%s: want %q stored verbatim, got %q", scheme, ref, *out)
		}
	}
}

func TestEncryptDirectiveIsTheOverrideForASecretShapedLikeAUrl(t *testing.T) {
	enc := newService(t)
	out, err := EncryptSecretRef(enc, ptr("encrypt:foo://bar"))
	if err != nil {
		t.Fatalf("the explicit directive must win: %v", err)
	}
	if !strings.HasPrefix(*out, "encrypted:") {
		t.Fatalf("want an envelope, got %q", *out)
	}
	pt, err := enc.Decrypt(*out)
	if err != nil || pt != "foo://bar" {
		t.Fatalf("want round-trip to foo://bar, got (%q,%v)", pt, err)
	}
}

func TestASecretThatMerelyContainsTheSeparatorIsNotASchemeAndIsEncrypted(t *testing.T) {
	enc := newService(t)
	// "p@ss" is not an RFC 3986 scheme token, so this is a password.
	out, err := EncryptSecretRef(enc, ptr("p@ss://word"))
	if err != nil {
		t.Fatalf("a password containing :// must still be encrypted, got %v", err)
	}
	if !strings.HasPrefix(*out, "encrypted:") {
		t.Fatalf("want an envelope, got %q", *out)
	}
	pt, err := enc.Decrypt(*out)
	if err != nil || pt != "p@ss://word" {
		t.Fatalf("want round-trip, got (%q,%v)", pt, err)
	}
}

func TestSchemeRejectionIsWriteSideOnlySoStoredRowsKeepReading(t *testing.T) {
	enc := newService(t)
	// Decrypt is untouched: an unknown scheme reads exactly as it did before
	// the ruling, so a row already stored under one is not orphaned.
	if _, err := enc.Decrypt("foo://bar"); err == nil {
		t.Fatal("want the pre-existing read failure, got success")
	} else if !strings.Contains(err.Error(), "invalid base64") {
		t.Fatalf("read behaviour changed: %v", err)
	}
}

// ── §4 literal: means the value IS the plaintext ─────────────────────────────

func TestDecryptHonoursLiteral(t *testing.T) {
	enc := newService(t)
	pt, err := enc.Decrypt("literal:hunter2")
	if err != nil || pt != "hunter2" {
		t.Fatalf("want (hunter2,nil), got (%q,%v)", pt, err)
	}
	// An empty literal is still a literal, not an "empty ciphertext" error.
	if pt, err := enc.Decrypt("literal:"); err != nil || pt != "" {
		t.Fatalf("want empty plaintext, got (%q,%v)", pt, err)
	}
}

// ── §2 a v0 nonce may begin with the v1 version byte ─────────────────────────

func TestV0EnvelopeWhoseNonceBeginsWithTheVersionByteStillDecrypts(t *testing.T) {
	enc := newService(t)
	const secret = "legacy v0 secret"
	nonce := make([]byte, enc.current.NonceSize())
	nonce[0] = currentVersion // the 1-in-256 collision
	ct := enc.current.Seal(nil, nonce, []byte(secret), nil)
	v0 := base64.StdEncoding.EncodeToString(append(append([]byte{}, nonce...), ct...))

	pt, err := enc.Decrypt(v0)
	if err != nil {
		t.Fatalf("a v0 row is unreadable whenever its nonce starts 0x01: %v", err)
	}
	if pt != secret {
		t.Fatalf("want %q, got %q", secret, pt)
	}
}

func TestGarbageIsStillRejectedAfterTheV0Retry(t *testing.T) {
	enc := newService(t)
	// The fallback must not turn "cannot decrypt" into a false positive.
	junk := make([]byte, 40)
	junk[0] = currentVersion
	if _, err := enc.Decrypt(base64.StdEncoding.EncodeToString(junk)); err == nil {
		t.Fatal("want failure for undecryptable data, got success")
	}
}

// ── §1 a malformed key is fatal, an absent one is not ────────────────────────

func TestMustFromEnvPanicsOnAMalformedKey(t *testing.T) {
	t.Setenv("FLOWCATALYST_APP_KEY", "not-a-key")
	defer func() {
		if recover() == nil {
			t.Fatal("a malformed key must be fatal, not silently encryption-disabled")
		}
	}()
	MustFromEnv()
}

func TestMustFromEnvReturnsNilWhenTheKeyIsUnset(t *testing.T) {
	t.Setenv("FLOWCATALYST_APP_KEY", "")
	if svc := MustFromEnv(); svc != nil {
		t.Fatalf("an unset key is the documented disabled state, got %v", svc)
	}
}

func ptr(s string) *string { return &s }
