package encryption

import (
	"strings"
	"testing"
)

func ptr(s string) *string { return &s }

func TestEncryptSecretRef(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	enc, err := New(key)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("nil passes through (field omitted on update)", func(t *testing.T) {
		out, err := EncryptSecretRef(enc, nil)
		if err != nil || out != nil {
			t.Fatalf("want (nil,nil), got (%v,%v)", out, err)
		}
	})

	t.Run("empty preserved (clears secret)", func(t *testing.T) {
		out, err := EncryptSecretRef(enc, ptr(""))
		if err != nil || out == nil || *out != "" {
			t.Fatalf("want empty preserved, got (%v,%v)", out, err)
		}
	})

	t.Run("plaintext is encrypted and round-trips", func(t *testing.T) {
		const secret = "oIC8Q~263EHzIfRlOtV8MQTZnLdHdrb4I~~Jydv2" // Azure-style, contains ~
		out, err := EncryptSecretRef(enc, ptr(secret))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(*out, "encrypted:") {
			t.Fatalf("want encrypted: prefix, got %q", *out)
		}
		pt, err := enc.Decrypt(*out)
		if err != nil {
			t.Fatalf("stored value must decrypt: %v", err)
		}
		if pt != secret {
			t.Fatalf("round-trip mismatch: %q != %q", pt, secret)
		}
	})

	t.Run("encrypt: directive is stripped before encrypting", func(t *testing.T) {
		out, err := EncryptSecretRef(enc, ptr("encrypt:mysecret"))
		if err != nil {
			t.Fatal(err)
		}
		pt, err := enc.Decrypt(*out)
		if err != nil || pt != "mysecret" {
			t.Fatalf("want plaintext 'mysecret', got %q (%v)", pt, err)
		}
	})

	t.Run("already-encrypted is idempotent", func(t *testing.T) {
		blob, _ := enc.Encrypt("x")
		in := "encrypted:" + blob
		out, err := EncryptSecretRef(enc, ptr(in))
		if err != nil || *out != in {
			t.Fatalf("already-encrypted must pass through unchanged, got %q (%v)", *out, err)
		}
	})

	t.Run("external refs pass through unchanged", func(t *testing.T) {
		for _, ref := range []string{"aws-sm://name", "aws-ps://p", "gcp-sm://s", "vault://path#k", "env://VAR"} {
			out, err := EncryptSecretRef(enc, ptr(ref))
			if err != nil || *out != ref {
				t.Fatalf("external ref %q must pass through, got %q (%v)", ref, *out, err)
			}
		}
	})

	t.Run("plaintext with nil service is rejected, never stored raw", func(t *testing.T) {
		out, err := EncryptSecretRef(nil, ptr("plaintext-secret"))
		if err == nil || out != nil {
			t.Fatalf("want ErrNotConfigured, got (%v,%v)", out, err)
		}
	})
}
