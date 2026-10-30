package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/go-sdk/outbox"
	"github.com/flowcatalyst/go-sdk/outboxpgx"
	"github.com/flowcatalyst/go-sdk/tests/internal/pgtest"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// The outbox conformance vector (the SDK's outbox/testdata/conformance), run
// the way an application writes it: a use case whose plan is the case's event
// plus its dispatch jobs, run by usecaseop.Run through outboxpgx into
// embedded Postgres. The vector helpers are copies of the SDK's own
// outbox/conformance_test.go (which holds the non-database half); the
// vector files are read from the SDK tree.

const vectorDir = "../outbox/testdata/conformance"

func TestMain(m *testing.M) { pgtest.Main(m) }

// marshal encodes v without HTML escaping or the trailing newline, as the
// outbox writers do.
func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

type vectorFile struct {
	Cases               []vectorCase     `json:"cases"`
	InvalidDispatchJobs []map[string]any `json:"invalidDispatchJobs"`
}

type vectorCase struct {
	Name    string `json:"name"`
	Now     string `json:"now"`
	Context struct {
		Client, Principal, Application, Correlation, Causation, Execution string
	} `json:"context"`
	Events []struct {
		Kind         string          `json:"kind"`
		EventID      string          `json:"eventId"`
		Type         string          `json:"type"`
		SubjectID    string          `json:"subjectId"`
		MessageGroup string          `json:"messageGroup"`
		Fields       json.RawMessage `json:"fields"`
	} `json:"events"`
	DispatchJobs []map[string]any `json:"dispatchJobs"`
	Audit        struct {
		EntityType string         `json:"entityType"`
		EntityID   string         `json:"entityId"`
		Operation  string         `json:"operation"`
		Data       map[string]any `json:"data"`
		Source     string         `json:"source"`
	} `json:"audit"`
	Rows []vectorRow `json:"rows"`
}

type vectorRow struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	MessageGroup *string         `json:"message_group"`
	Status       int             `json:"status"`
	RetryCount   int             `json:"retry_count"`
	ClientID     *string         `json:"client_id"`
	PayloadSize  int             `json:"payload_size"`
	Headers      json.RawMessage `json:"headers"`
	ErrorMessage *string         `json:"error_message"`
	CreatedAt    string          `json:"created_at"`
	UpdatedAt    string          `json:"updated_at"`
	Payload      json.RawMessage `json:"payload"`
}

// The vector's event kinds, as records serialise them (Jackson, nulls left
// out, members in declaration order).
type placedLine struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

type placedData struct {
	OrderID    string       `json:"orderId"`
	Customer   string       `json:"customer"`
	Lines      []placedLine `json:"lines"`
	TotalCents int          `json:"totalCents"`
	Note       *string      `json:"note,omitempty"`
	PlacedAt   string       `json:"placedAt"`
}

type cancelledData struct {
	OrderID string `json:"orderId"`
	Reason  string `json:"reason"`
}

// vectorEvent is a domain event as the framework builds one: source
// {application}:{domain}, subject {domain}.{aggregate}.{id}, message group
// {domain}:{aggregate}:{id} unless overridden.
type vectorEvent struct {
	id, typ, subjectID, group                    string
	principal, correlation, causation, execution string
	at                                           time.Time
	data                                         []byte
}

func (e vectorEvent) seg(i int) string      { return strings.Split(e.typ, ":")[i] }
func (e vectorEvent) EventID() string       { return e.id }
func (e vectorEvent) EventType() string     { return e.typ }
func (e vectorEvent) SpecVersion() string   { return "1.0" }
func (e vectorEvent) Source() string        { return e.seg(0) + ":" + e.seg(1) }
func (e vectorEvent) Subject() string       { return e.seg(1) + "." + e.seg(2) + "." + e.subjectID }
func (e vectorEvent) Time() time.Time       { return e.at }
func (e vectorEvent) PrincipalID() string   { return e.principal }
func (e vectorEvent) CorrelationID() string { return e.correlation }
func (e vectorEvent) CausationID() string   { return e.causation }
func (e vectorEvent) ExecutionID() string   { return e.execution }
func (e vectorEvent) ToDataJSON() ([]byte, error) {
	return e.data, nil
}

func (e vectorEvent) MessageGroup() string {
	if e.group != "" {
		return e.group
	}
	return e.seg(1) + ":" + e.seg(2) + ":" + e.subjectID
}

// vectorCommand names its audit operation and chooses its audit data, as a
// framework use case does.
type vectorCommand struct {
	operation string
	data      map[string]any
}

func (c vectorCommand) AuditOperation() string { return c.operation }
func (c vectorCommand) AuditData() any {
	if c.data == nil {
		return nil
	}
	return c.data
}

func loadVectors(t *testing.T) vectorFile {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(vectorDir, "outbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v vectorFile
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Cases) == 0 {
		t.Fatal("no cases")
	}
	return v
}

func buildEvent(t *testing.T, c vectorCase, i int, now time.Time) vectorEvent {
	t.Helper()
	ev := c.Events[i]
	var data []byte
	var err error
	switch ev.Kind {
	case "placed":
		var d placedData
		if err := json.Unmarshal(ev.Fields, &d); err != nil {
			t.Fatal(err)
		}
		data, err = marshal(d)
	case "cancelled":
		var d cancelledData
		if err := json.Unmarshal(ev.Fields, &d); err != nil {
			t.Fatal(err)
		}
		data, err = marshal(d)
	default:
		t.Fatalf("unknown event kind %q", ev.Kind)
	}
	if err != nil {
		t.Fatal(err)
	}
	return vectorEvent{
		id: ev.EventID, typ: ev.Type, subjectID: ev.SubjectID, group: ev.MessageGroup,
		principal: c.Context.Principal, correlation: c.Context.Correlation,
		causation: c.Context.Causation, execution: c.Context.Execution, at: now, data: data,
	}
}

// buildDispatchJob builds a job from a vector input the way an application
// calls the builder. raw holds the input's members as written (so a
// jsonPayload keeps its member order; Go's encoder would sort a map's keys).
func buildDispatchJob(t *testing.T, in map[string]any, raw map[string]json.RawMessage, correlation string) outbox.DispatchJob {
	t.Helper()
	str := func(k string) string { s, _ := in[k].(string); return s }
	j := outbox.NewDispatchJob(str("code"), str("targetUrl"))
	if correlation != "" {
		j = j.WithCorrelationID(correlation)
	}
	for k, v := range in {
		switch k {
		case "id":
			j = j.WithID(str(k))
		case "kind":
			j = j.WithKind(str(k))
		case "source":
			j = j.WithSource(str(k))
		case "subject":
			j = j.WithSubject(str(k))
		case "externalId":
			j = j.WithExternalID(str(k))
		case "eventId":
			j = j.WithEventID(str(k))
		case "platformClientId":
			j = j.WithPlatformClientID(str(k))
		case "subscriptionId":
			j = j.WithSubscriptionID(str(k))
		case "serviceAccountId":
			j = j.WithServiceAccountID(str(k))
		case "dispatchPoolId":
			j = j.WithDispatchPoolID(str(k))
		case "messageGroup":
			j = j.WithMessageGroup(str(k))
		case "mode":
			j = j.WithMode(str(k))
		case "queue":
			j = j.WithQueue(str(k))
		case "descriptor":
			j = j.WithDescriptor(str(k))
		case "dataOnly":
			if v == true {
				j = j.DataOnlyEnabled()
			}
		case "sequence":
			j = j.WithSequence(int(v.(float64)))
		case "timeoutSeconds":
			j = j.WithTimeoutSeconds(int(v.(float64)))
		case "maxRetries":
			j = j.WithMaxRetries(int(v.(float64)))
		case "payloadText":
			j = j.WithPayload(str(k), str("contentType"))
		case "jsonPayload":
			var err error
			if j, err = j.WithJSONPayload(raw[k]); err != nil {
				t.Fatal(err)
			}
		case "code", "targetUrl", "contentType", "error", "metadata":
		default:
			t.Fatalf("unknown dispatch-job vector member %q", k)
		}
	}
	// Metadata in vector order (map iteration above is unordered).
	if md, ok := in["metadata"].([]any); ok {
		for _, pair := range md {
			p := pair.([]any)
			j = j.WithMetadata(p[0].(string), p[1].(string))
		}
	}
	return j
}

// rawDispatchJobs re-reads each case's dispatchJobs with raw members, so a
// jsonPayload keeps its member order.
func rawDispatchJobs(t *testing.T) map[string][]map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(vectorDir, "outbox.json"))
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Cases []struct {
			Name         string                       `json:"name"`
			DispatchJobs []map[string]json.RawMessage `json:"dispatchJobs"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	out := map[string][]map[string]json.RawMessage{}
	for _, c := range v.Cases {
		out[c.Name] = c.DispatchJobs
	}
	return out
}

// canonical parses a payload for comparison: numbers exact, and an event's
// data / an audit row's operationData unwrapped when it is a string holding
// a JSON document (the vectors' comparison rule).
func canonical(t *testing.T, raw []byte) any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("payload is not a JSON object: %v\n%s", err, raw)
	}
	for _, k := range []string{"data", "operationData"} {
		if s, ok := m[k].(string); ok {
			d := json.NewDecoder(strings.NewReader(s))
			d.UseNumber()
			var inner any
			if err := d.Decode(&inner); err == nil {
				m[k] = inner
			}
		}
	}
	return m
}

type goPayload struct {
	Case        string `json:"case"`
	Row         int    `json:"row"`
	Type        string `json:"type"`
	PayloadSize int    `json:"payloadSize"`
	PayloadText string `json:"payloadText"`
}

func ptrAny(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// StoredRow is one outbox_messages row as read back from the database.
type StoredRow = pgtest.Row

// PlanCase is a vector case a single-event plan can express.
type PlanCase struct {
	Name   string
	Config outbox.Config
	Event  usecase.DomainEvent
	// Jobs carry no correlation id: the plan gives them its event's, which is
	// the vector context's.
	Jobs    []outbox.DispatchJob
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
			Config: outbox.Config{
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

// The conformance vector, written the way an application writes it: a use
// case whose plan is the case's event plus its dispatch jobs
// (usecaseop.Emit(...).Dispatch(...)), run by usecaseop.Run through
// outboxpgx into embedded Postgres, then read back in insertion order. The
// stored rows equal the vector's (event, jobs, audit row; every column), and
// each payload is byte for byte what the row builders write. The jobs are
// built without a correlation id: the plan gives them the event's.
func TestConformancePlanBuiltRows(t *testing.T) {
	cases, skipped := PlanCases(t)
	if len(cases) == 0 {
		t.Fatal("no single-event cases")
	}
	t.Logf("not expressible as one plan (more than one event): %q", skipped)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			pool := pgtest.Schema(t)
			sink := outboxpgx.NewSink(outboxpgx.Config{
				ClientID: c.Config.ClientID, ClientCode: c.Config.ClientCode,
				ApplicationCode: c.Config.ApplicationCode, Now: c.Config.Now, AuditEnabled: true,
			})
			op := usecaseop.Operation[any, usecase.DomainEvent]{
				Name:      "conformance",
				Authorize: usecaseop.Public[any],
				Execute: func(context.Context, any, usecase.ExecutionContext) (usecaseop.Plan[usecase.DomainEvent], error) {
					return usecaseop.Emit(c.Event).Dispatch(c.Jobs...), nil
				},
			}
			if _, err := usecaseop.Run(context.Background(), usecasepgx.New(pool, sink), op, c.Command, usecase.ExecutionContext{}); err != nil {
				t.Fatal(err)
			}
			stored := pgtest.OutboxRows(t, pool)
			rows := make([]StoredRow, len(stored))
			for i, r := range stored {
				rows[i] = StoredRow(r)
			}
			c.Check(t, rows)
		})
	}
}
