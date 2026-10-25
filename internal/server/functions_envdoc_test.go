package server

import (
	"os"
	"regexp"
	"testing"
)

// Every FC_FUNCTIONS_* variable the runner reads must be documented in
// docs/environment-variables.md (and the doc must not name ones the code
// does not read).
func TestFunctionsEnvVarsDocumented(t *testing.T) {
	src, err := os.ReadFile("functions.go")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := os.ReadFile("../../docs/environment-variables.md")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile("`(FC_FUNCTIONS_[A-Z_]+)`")
	documented := map[string]bool{}
	for _, m := range re.FindAllStringSubmatch(string(doc), -1) {
		documented[m[1]] = true
	}
	read := map[string]bool{}
	for _, m := range regexp.MustCompile(`"(FC_FUNCTIONS_[A-Z_]+)"`).FindAllStringSubmatch(string(src), -1) {
		read[m[1]] = true
		if !documented[m[1]] {
			t.Errorf("%s is read by functions.go but not documented", m[1])
		}
	}
	// Variables read elsewhere (envcfg.go) are documented too.
	read["FC_FUNCTIONS_ARTIFACT_STORE"], read["FC_FUNCTIONS_RUNNER_URL"] = true, true
	read["FC_FUNCTIONS_KEEP_VERSIONS"] = true
	read["FC_FUNCTIONS_TRACE_MAX_MINUTES"] = true
	read["FC_FUNCTIONS_TRACE_HTTP_MAX_MINUTES"] = true
	read["FC_FUNCTIONS_TRACE_RETENTION_DAYS"] = true
	read["FC_FUNCTIONS_TRACE_INDEFINITE_MAX_DAYS"] = true
	read["FC_FUNCTIONS_TRACE_PRUNE_MAX_ROWS"] = true
	read["FC_FUNCTIONS_TRACE_PRUNE_INTERVAL_MINUTES"] = true
	for name := range documented {
		if !read[name] {
			t.Errorf("%s is documented but not read by the code", name)
		}
	}
}
