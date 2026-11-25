package pages

import (
	"testing"

	"github.com/flowcatalyst/go-sdk/usecase"
)

func TestSecretSourcePosted(t *testing.T) {
	cases := []struct {
		name                      string
		form                      map[string]string
		source, ref, stored, want string
	}{
		{name: "no choice is a value", form: map[string]string{"v": "s3cret"}, source: "value", stored: "s3cret"},
		{name: "an unknown choice is a value", form: map[string]string{"vSource": "vault", "v": "s3cret", "vRefVault": "x"}, source: "value", stored: "s3cret"},
		{name: "parameter store", form: map[string]string{"vSource": "aws-ps", "v": "ignored", "vRefAwsPs": " /a/b ", "vRefAwsSm": "other"}, source: "aws-ps", ref: "/a/b", stored: "aws-ps:///a/b"},
		{name: "a path pasted with its scheme", form: map[string]string{"vSource": "aws-ps", "vRefAwsPs": "aws-ps:///a/b"}, source: "aws-ps", ref: "/a/b", stored: "aws-ps:///a/b"},
		{name: "secrets manager with a key", form: map[string]string{"vSource": "aws-sm", "vRefAwsSm": "prod/x#client_secret"}, source: "aws-sm", ref: "prod/x#client_secret", stored: "aws-sm://prod/x#client_secret"},
		{name: "an empty reference stores nothing", form: map[string]string{"vSource": "aws-sm", "v": "typed", "vRefAwsSm": "  "}, source: "aws-sm"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			source, ref, stored := secretSourcePosted(func(k string) string { return c.form[k] }, "v")
			if source != c.source || ref != c.ref || stored != c.stored {
				t.Errorf("got (%q, %q, %q), want (%q, %q, %q)", source, ref, stored, c.source, c.ref, c.stored)
			}
		})
	}
}

func TestSecretSourceOf(t *testing.T) {
	for stored, want := range map[string][2]string{
		"aws-ps:///a/b":   {"aws-ps", "/a/b"},
		"aws-sm://x#k":    {"aws-sm", "x#k"},
		"plain-secret":    {"value", ""},
		"encrypted:abcd=": {"value", ""},
	} {
		if s, r := secretSourceOf(stored); s != want[0] || r != want[1] {
			t.Errorf("%q: got (%q, %q), want %v", stored, s, r, want)
		}
	}
	if got := secretSourceLabel("aws-ps:///a"); got != "AWS Parameter Store" {
		t.Errorf("label %q", got)
	}
}

func TestSignalPart(t *testing.T) {
	if got := signalPart("sftp.credential"); got != "SftpCredential" {
		t.Errorf("got %q", got)
	}
	if got := kebab("ingsCfg" + signalPart("s3.secretAccessKey") + "Source"); got != "ings-cfg-s3-secret-access-key-source" {
		t.Errorf("got %q", got)
	}
}

func TestSecretReferenceOn(t *testing.T) {
	err := usecase.Validation("SECRET_REFERENCE_NOT_ALLOWED", "not allowed")
	e := formErrors{Form: "Not allowed."}
	e.secretReferenceOn(err, []string{"a"})
	if e.Form != "" || e.Fields["a"] != "Not allowed." {
		t.Errorf("one reference: %+v", e)
	}
	e = formErrors{Form: "Not allowed."}
	e.secretReferenceOn(err, []string{"a", "b"})
	if e.Form != "Not allowed." || len(e.Fields) != 0 {
		t.Errorf("two references: %+v", e)
	}
	e = formErrors{Form: "Other."}
	e.secretReferenceOn(usecase.Validation("INGESTION_INVALID", "x"), []string{"a"})
	if e.Form != "Other." {
		t.Errorf("another error: %+v", e)
	}
}
