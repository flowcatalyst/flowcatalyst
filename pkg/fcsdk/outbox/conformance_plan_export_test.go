//go:build integration

package outbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// The conformance vector's cases as inputs for a writer that builds the rows
// itself and stores them (usecaseop's Plan through outboxpgx, in package
// outbox_test: conformance_plan_pg_test.go), and the check of what it stored.
// Exported only to the test binary.

// StoredRow is one outbox_messages row as read back from the database.
type StoredRow struct {
	ID, Type, Payload      string
	MessageGroup, ClientID *string
	ErrorMessage, Headers  *string
	Status, RetryCount     int
	PayloadSize            *int
	CreatedAt, UpdatedAt   time.Time
}

// PlanCase is a vector case a single-event plan can express.
type PlanCase struct {
	Name   string
	Config Config
	Event  usecase.DomainEvent
	// Jobs carry no correlation id: the plan gives them its event's, which is
	// the vector context's.
	Jobs    []DispatchJob
	Command any
	want    []vectorRow
	golden  map[int]string
}

// PlanCases returns the cases with exactly one event (a plan has one event;
// fcsdk's Sync writes an audit row per event, which the vector's
// multi-event cases do not), and the names of the others.
func PlanCases(t *testing.T) (cases []PlanCase, skipped []string) {
	t.Helper()
	vectors := loadVectors(t)
	rawJobs := rawDispatchJobs(t)
	golden := goldenPayloads(t)
	for _, c := range vectors.Cases {
		if len(c.Events) != 1 {
			skipped = append(skipped, c.Name)
			continue
		}
		now, err := time.Parse(time.RFC3339Nano, c.Now)
		if err != nil {
			t.Fatal(err)
		}
		pc := PlanCase{
			Name: c.Name,
			Config: Config{
				ClientID: c.Context.Client, ClientCode: c.Context.Client,
				ApplicationCode: c.Context.Application, Now: func() time.Time { return now },
			},
			Event:   buildEvent(t, c, 0, now),
			Command: vectorCommand{operation: c.Audit.Operation, data: c.Audit.Data},
			want:    c.Rows,
			golden:  golden[c.Name],
		}
		for i, in := range c.DispatchJobs {
			pc.Jobs = append(pc.Jobs, buildDispatchJob(t, in, rawJobs[c.Name][i], ""))
		}
		cases = append(cases, pc)
	}
	return cases, skipped
}

// Check compares the stored rows with the vector's, column by column, under
// the vector's comparison rule, and each payload's text with the bytes the
// row builders produce for the same case (go-payloads.json): a plan writes
// exactly what the direct writers write.
func (c PlanCase) Check(t *testing.T, rows []StoredRow) {
	t.Helper()
	if len(rows) != len(c.want) {
		t.Fatalf("%d rows stored, vector has %d", len(rows), len(c.want))
	}
	for i, want := range c.want {
		got := rows[i]
		if want.ID == "<tsid>" {
			if !tsidPattern.MatchString(got.ID) {
				t.Errorf("row %d: id %q is not a 13-character TSID", i, got.ID)
			}
		} else if got.ID != want.ID {
			t.Errorf("row %d: id %q, vector %q", i, got.ID, want.ID)
		}
		if got.Type != want.Type {
			t.Errorf("row %d: type %q, vector %q", i, got.Type, want.Type)
		}
		if ptrAny(got.MessageGroup) != ptrAny(want.MessageGroup) {
			t.Errorf("row %d: message_group %v, vector %v", i, ptrAny(got.MessageGroup), ptrAny(want.MessageGroup))
		}
		if got.Status != want.Status || got.RetryCount != want.RetryCount {
			t.Errorf("row %d: status/retry_count %d/%d, vector %d/%d", i, got.Status, got.RetryCount, want.Status, want.RetryCount)
		}
		if ptrAny(got.ClientID) != ptrAny(want.ClientID) {
			t.Errorf("row %d: client_id %v, vector %v", i, ptrAny(got.ClientID), ptrAny(want.ClientID))
		}
		if got.Headers != nil || got.ErrorMessage != nil || string(want.Headers) != "null" || want.ErrorMessage != nil {
			t.Errorf("row %d: headers/error_message must be null", i)
		}
		created := got.CreatedAt.UTC().Format("2006-01-02T15:04:05.000000Z")
		if created != want.CreatedAt || !got.UpdatedAt.Equal(got.CreatedAt) {
			t.Errorf("row %d: created_at %s (updated_at %s), vector %s", i, created, got.UpdatedAt, want.CreatedAt)
		}
		if got.PayloadSize == nil || *got.PayloadSize != len(got.Payload) {
			t.Errorf("row %d: payload_size %v is not the payload's byte length %d", i, got.PayloadSize, len(got.Payload))
		}
		if g, w := canonical(t, []byte(got.Payload)), canonical(t, want.Payload); !reflect.DeepEqual(g, w) {
			t.Errorf("row %d (%s) payload differs from the vector:\n got  %s\n want %s", i, got.Type, got.Payload, want.Payload)
		}
		if text, ok := c.golden[i]; !ok || got.Payload != text {
			t.Errorf("row %d (%s) payload differs from the row builders' bytes:\n got  %s\n want %s", i, got.Type, got.Payload, text)
		}
	}
}

func goldenPayloads(t *testing.T) map[string]map[int]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(vectorDir, "go-payloads.json"))
	if err != nil {
		t.Fatal(err)
	}
	var all []goPayload
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatal(err)
	}
	out := map[string]map[int]string{}
	for _, p := range all {
		if out[p.Case] == nil {
			out[p.Case] = map[int]string{}
		}
		out[p.Case][p.Row] = p.PayloadText
	}
	return out
}
