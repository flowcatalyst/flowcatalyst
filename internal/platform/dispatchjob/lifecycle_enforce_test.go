package dispatchjob_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// TestOnlyTheLifecycleWritesDispatchJobs fails if any production source file
// other than the lifecycle writes msg_dispatch_jobs: an INSERT / UPDATE /
// DELETE / TRUNCATE / DROP / ALTER statement against it, or a call to a
// generated query that does. It scans every non-test .go and .sql file in the
// repository (the generated dbq code and the sqlc query files included), so a
// new writer cannot appear anywhere without either living in lifecycle.go or
// being named in lifecycleWriteExceptions below, with its reason.
//
// This is the guard behind the rule "one place makes a job PENDING": a second
// UPDATE msg_dispatch_jobs SET status = ... is not a style problem, it is the
// bug this module exists to prevent.
func TestOnlyTheLifecycleWritesDispatchJobs(t *testing.T) {
	root := repoRoot(t)

	// The only files allowed to write the table, repo-relative.
	allowed := map[string]string{
		"internal/platform/dispatchjob/lifecycle.go": "the lifecycle: the one owner of the table's writes",
		// Deliberate exceptions, each named with its reason.
		"internal/stream/dispatch_jobs.go":     "the projector stamps projected_at (not status) after copying a row into the read model",
		"internal/stream/partition_manager.go": "partition DDL: DROP TABLE of expired partitions (report: this also drops any PENDING rows in them)",
		"cmd/fcdev/fresh.go":                   "fc-dev fresh: TRUNCATE of the dev database",
	}

	// msg_dispatch_queue (one row per PENDING job) has the same single owner,
	// with two named exceptions.
	allowedQueue := map[string]string{
		"internal/platform/dispatchjob/lifecycle.go": "the lifecycle: the one owner of the queue's writes",
		"internal/stream/partition_manager.go":       "dropping a msg_dispatch_jobs partition removes the queue rows of the jobs it took with it (no foreign key can)",
		"cmd/fcdev/fresh.go":                         "fc-dev fresh: TRUNCATE of the dev database",
		"internal/testpg/testpg.go":                  "integration-test support (build tag integration): SyncDispatchQueue makes the queue agree with a job a test wrote directly",
	}

	write := regexp.MustCompile(`(?is)\b(insert\s+into|update|delete\s+from|truncate(\s+table)?|drop\s+table(\s+if\s+exists)?|alter\s+table)\s+(only\s+)?msg_dispatch_jobs\b`)
	writeQueue := regexp.MustCompile(`(?is)\b(insert\s+into|update|delete\s+from|truncate(\s+table)?|drop\s+table(\s+if\s+exists)?|alter\s+table)\s+(only\s+)?msg_dispatch_queue\b`)
	// sqlc accessors for the table's writers, were one ever generated again.
	accessor := regexp.MustCompile(`\bDispatchJob(Insert|Persist|Delete|Update\w*|Mark\w+|ScheduleRetry|ClaimForDelivery|ReclaimStaleDelivery|SettleAcked|SweepStrandedSiblings)\b`)

	var violations []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "frontend", "testdata":
				return filepath.SkipDir
			}
			// Migrations create and alter the table by definition.
			if rel == "internal/migrate/sql" {
				return filepath.SkipDir
			}
			return nil
		}
		if !(strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".sql")) || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(b)
		if _, ok := allowedQueue[rel]; !ok {
			if m := writeQueue.FindString(src); m != "" {
				violations = append(violations, rel+": "+strings.Join(strings.Fields(m), " "))
			}
		}
		if _, ok := allowed[rel]; ok {
			return nil
		}
		if m := write.FindString(src); m != "" {
			violations = append(violations, rel+": "+strings.Join(strings.Fields(m), " "))
		}
		if m := accessor.FindString(src); m != "" {
			violations = append(violations, rel+": generated writer "+m)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(violations)
	if len(violations) > 0 {
		t.Fatalf("msg_dispatch_jobs is written outside the dispatch-job lifecycle "+
			"(internal/platform/dispatchjob/lifecycle.go). Route the write through it, or, if it truly is "+
			"not a status/lifecycle write, add the file to the exceptions with a reason:\n  %s",
			strings.Join(violations, "\n  "))
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above " + file)
		}
		dir = parent
	}
}
