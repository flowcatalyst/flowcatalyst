package main

import (
	"bytes"
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
