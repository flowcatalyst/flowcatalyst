package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestCommittedFixturesAreCurrent: the generator is deterministic and the
// committed api/field-fixtures files are exactly its output. Regenerate with
// `go run ./tools/field-fixtures` after changing it.
func TestCommittedFixturesAreCurrent(t *testing.T) {
	first, second := generate(), generate()
	for name, data := range first {
		if !bytes.Equal(data, second[name]) {
			t.Fatalf("%s: generator output differs between runs", name)
		}
		committed, err := os.ReadFile(filepath.Join("..", "..", "api", "field-fixtures", name))
		if err != nil {
			t.Fatalf("read committed %s: %v", name, err)
		}
		if !bytes.Equal(committed, data) {
			t.Errorf("api/field-fixtures/%s is stale; run `go run ./tools/field-fixtures`", name)
		}
	}
}

// TestLinkedFixtureCarriesTheClaim: valid_linked is the one case whose token
// carries linked_principal_id, and it lists the same id as the principal a
// verifier must expose (docs/field-native-session-spec.md section 5).
func TestLinkedFixtureCarriesTheClaim(t *testing.T) {
	var f struct {
		Cases []fixtureCase `json:"cases"`
	}
	if err := json.Unmarshal(generate()["cases.json"], &f); err != nil {
		t.Fatal(err)
	}
	linked := 0
	for _, c := range f.Cases {
		payload, err := base64.RawURLEncoding.DecodeString(splitToken(c.AccessToken)[1])
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		var claims map[string]any
		if err := json.Unmarshal(payload, &claims); err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		id, has := claims["linked_principal_id"]
		if c.Name != "valid_linked" {
			if has {
				t.Errorf("%s carries linked_principal_id", c.Name)
			}
			continue
		}
		linked++
		if id != "prn_0FIXTUREPRINCIPAL" || c.Principal == nil || c.Principal.LinkedPrincipalID != id || c.Expect != "valid" {
			t.Errorf("valid_linked: claim %v, principal %+v, expect %s", id, c.Principal, c.Expect)
		}
	}
	if linked != 1 {
		t.Errorf("valid_linked appears %d times, want 1", linked)
	}
}
