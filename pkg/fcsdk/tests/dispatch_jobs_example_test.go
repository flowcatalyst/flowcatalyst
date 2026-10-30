package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flowcatalyst/go-sdk/tests/internal/pgtest"
)

// The dispatch-jobs example run as a program against embedded Postgres (an
// example is a main package, so it is built and executed, not imported):
// the invoice, its event, both jobs (in plan order, with the plan's
// correlation id) and the audit row land in one schema. The example's
// refusal of a bad notify URL needs no database and is tested in the SDK
// (examples/dispatch-jobs/main_test.go).
func TestDispatchJobsExampleWritesTheJobsWithTheEvent(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "dispatch-jobs")
	build := exec.Command("go", "build", "-o", bin, "github.com/flowcatalyst/go-sdk/examples/dispatch-jobs")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the example: %v\n%s", err, out)
	}

	pool, url := pgtest.SchemaURL(t)
	run := exec.Command(bin)
	run.Env = append(os.Environ(), "FC_DATABASE_URL="+url)
	out, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("run the example: %v\n%s", err, out)
	}
	if !strings.HasPrefix(string(out), "issued ") {
		t.Errorf("unexpected output:\n%s", out)
	}

	rows := pgtest.OutboxRows(t, pool)
	want := []string{"EVENT", "DISPATCH_JOB", "DISPATCH_JOB", "AUDIT_LOG"}
	if len(rows) != len(want) {
		t.Fatalf("%d rows, want %v", len(rows), want)
	}
	codes := []string{"", "billing:ledger:export", "billing:invoice:notify", ""}
	var invoiceID string
	for i, r := range rows {
		if r.Type != want[i] {
			t.Fatalf("row %d is %s, want %s", i, r.Type, want[i])
		}
		if r.Type == "EVENT" {
			var p struct{ Subject string }
			_ = json.Unmarshal([]byte(r.Payload), &p)
			invoiceID = p.Subject[strings.LastIndex(p.Subject, ".")+1:]
		}
		if r.Type != "DISPATCH_JOB" {
			continue
		}
		var p map[string]any
		if err := json.Unmarshal([]byte(r.Payload), &p); err != nil {
			t.Fatal(err)
		}
		if p["code"] != codes[i] || p["correlationId"] == nil || p["correlationId"] == "" || p["id"] != r.ID {
			t.Errorf("job %d: %s", i, r.Payload)
		}
	}
	if !json.Valid([]byte(rows[1].Payload)) || rows[2].MessageGroup == nil || *rows[2].MessageGroup != "invoice:"+invoiceID {
		t.Errorf("notify job row: %+v (invoice %q)", rows[2], invoiceID)
	}
}
