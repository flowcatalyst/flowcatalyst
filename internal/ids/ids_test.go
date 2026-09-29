package ids

import (
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

func TestParse(t *testing.T) {
	if id, err := ParseClientID(tsid.Generate(tsid.Client)); err != nil || id == "" {
		t.Fatalf("generated client id rejected: %v", err)
	}
	if _, err := ParseClientID(tsid.Generate(tsid.Principal)); err == nil {
		t.Fatal("principal id accepted as client id")
	}
	for _, bad := range []string{"", "clt_", "clt", "CLT_abc"} {
		if _, err := ParseClientID(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
	if _, err := ParsePrincipalID(tsid.Generate(tsid.Principal)); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseApplicationID(tsid.Generate(tsid.Application)); err != nil {
		t.Fatal(err)
	}
}

func TestStrings(t *testing.T) {
	got := Strings([]ClientID{"a", "b"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("got %v", got)
	}
}
