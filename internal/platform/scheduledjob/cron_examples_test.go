package scheduledjob

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Documentation examples must be accepted by function publish, which rejects
// anything ValidateCronFires does (6-field, seconds first). A 5-field example
// in the docs or an SDK README looks valid and fails at publish.
func TestDocumentedScheduleExamplesFire(t *testing.T) {
	re := regexp.MustCompile(`[Ss]chedule\("([^"]+)"`)
	var files []string
	for _, g := range []string{
		"../../../docs/published/*.md",
		"../../../clients/fn-*/README.md",
	} {
		m, err := filepath.Glob(g)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	if len(files) == 0 {
		t.Fatal("no documentation files found")
	}
	found := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			found++
			if err := ValidateCronFires(m[1]); err != nil {
				t.Errorf("%s: schedule example %q is not publishable: %v", f, m[1], err)
			}
		}
	}
	if found == 0 {
		t.Fatal("expected at least one documented Schedule(...) example")
	}
}
