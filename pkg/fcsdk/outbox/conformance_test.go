package outbox

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

// The outbox conformance vectors of the application framework
// (flowcatalyst-framework conformance/outbox/outbox.json and
// conformance/sql/outbox/insert.sql), copied into testdata/conformance. The
// Go writers are held to them under the vectors' comparison rule: payloads
// compare as parsed JSON values, and a JSON document carried inside a string
// (an event's data, an audit row's operationData) compares as its parsed
// value. Go writes those as the value itself (never encoded), so its payload
// text and payload_size differ from the Java ones; Go's own exact bytes are
// pinned separately in testdata/conformance/go-payloads.json.
//
//	go test ./outbox -run Conformance -update   # rewrite go-payloads.json

var update = flag.Bool("update", false, "rewrite testdata/conformance/go-payloads.json")

const vectorDir = "testdata/conformance"

// frameworkConformanceDir is the sibling checkout's vector directory, when
// present (FC_FRAMEWORK_DIR overrides the default ../../../../flowcatalyst-framework).
func frameworkConformanceDir() string {
	root := os.Getenv("FC_FRAMEWORK_DIR")
	if root == "" {
		root = filepath.Join("..", "..", "..", "..", "flowcatalyst-framework")
	}
	return filepath.Join(root, "conformance")
}

func TestConformanceVectorsAreTheFrameworkCopy(t *testing.T) {
	src := frameworkConformanceDir()
	if _, err := os.Stat(src); err != nil {
		t.Skipf("framework checkout not found at %s (set FC_FRAMEWORK_DIR to compare the copy)", src)
	}
	for ours, theirs := range map[string]string{
		"outbox.json": filepath.Join(src, "outbox", "outbox.json"),
		"insert.sql":  filepath.Join(src, "sql", "outbox", "insert.sql"),
	} {
		a, err := os.ReadFile(filepath.Join(vectorDir, ours))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(theirs)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("testdata/conformance/%s differs from %s: copy the framework's file again and re-run", ours, theirs)
		}
	}
}

func TestConformanceInsertStatementIsTheFrameworkStatement(t *testing.T) {
	want, err := os.ReadFile(filepath.Join(vectorDir, "insert.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if InsertSQL != string(want) {
		t.Fatalf("InsertSQL differs from the framework statement:\n got %q\nwant %q", InsertSQL, want)
	}
	q, err := InsertStatement("", PlaceholderQuestion)
	if err != nil || q != string(want) {
		t.Fatalf("question style: %q %v", q, err)
	}
	d, err := InsertStatement("", PlaceholderDollar)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(d, "VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)") || strings.Contains(d, "?") {
		t.Fatalf("dollar style: %q", d)
	}
	if got := strings.Count(string(want), "?"); got != len(Row{}.Args()) {
		t.Fatalf("statement has %d placeholders, Row.Args gives %d", got, len(Row{}.Args()))
	}
}

// ── the vector's shape ──────────────────────────────────────────────

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
func buildDispatchJob(t *testing.T, in map[string]any, raw map[string]json.RawMessage, correlation string) DispatchJob {
	t.Helper()
	str := func(k string) string { s, _ := in[k].(string); return s }
	j := NewDispatchJob(str("code"), str("targetUrl"))
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

var tsidPattern = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{13}$`)

type goPayload struct {
	Case        string `json:"case"`
	Row         int    `json:"row"`
	Type        string `json:"type"`
	PayloadSize int    `json:"payloadSize"`
	PayloadText string `json:"payloadText"`
}

func TestConformanceOutboxRows(t *testing.T) {
	vectors := loadVectors(t)
	rawJobs := rawDispatchJobs(t)
	var produced []goPayload

	for _, c := range vectors.Cases {
		t.Run(c.Name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339Nano, c.Now)
			if err != nil {
				t.Fatal(err)
			}
			cfg := Config{
				ClientID: c.Context.Client, ClientCode: c.Context.Client,
				ApplicationCode: c.Context.Application, Now: func() time.Time { return now },
			}

			// The framework's write order: the events, the dispatch jobs, the
			// audit row (of the first event).
			var rows []Row
			var first vectorEvent
			for i := range c.Events {
				ev := buildEvent(t, c, i, now)
				if i == 0 {
					first = ev
				}
				row, err := EventRow(cfg, ev)
				if err != nil {
					t.Fatal(err)
				}
				rows = append(rows, row)
			}
			for i, in := range c.DispatchJobs {
				row, err := DispatchJobRow(cfg, buildDispatchJob(t, in, rawJobs[c.Name][i], c.Context.Correlation))
				if err != nil {
					t.Fatal(err)
				}
				rows = append(rows, row)
			}
			if first.source() != c.Audit.Source || first.entityType() != c.Audit.EntityType || first.entityID() != c.Audit.EntityID {
				t.Fatalf("audit input %+v is not the first event's (%s, %s, %s)", c.Audit, first.Source(), first.entityType(), first.entityID())
			}
			row, err := AuditRow(cfg, first, vectorCommand{operation: c.Audit.Operation, data: c.Audit.Data})
			if err != nil {
				t.Fatal(err)
			}
			rows = append(rows, row)

			if len(rows) != len(c.Rows) {
				t.Fatalf("%d rows, vector has %d", len(rows), len(c.Rows))
			}
			for i, want := range c.Rows {
				got := rows[i]
				args := got.Args()
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
				if !reflect.DeepEqual(args[2], ptrAny(want.MessageGroup)) {
					t.Errorf("row %d: message_group %v, vector %v", i, args[2], ptrAny(want.MessageGroup))
				}
				if args[4] != want.Status || want.RetryCount != 0 {
					t.Errorf("row %d: status %v (retry_count is the column default 0), vector %d/%d", i, args[4], want.Status, want.RetryCount)
				}
				if !reflect.DeepEqual(args[7], ptrAny(want.ClientID)) {
					t.Errorf("row %d: client_id %v, vector %v", i, args[7], ptrAny(want.ClientID))
				}
				if args[9] != nil || string(want.Headers) != "null" || want.ErrorMessage != nil {
					t.Errorf("row %d: headers/error_message must be null", i)
				}
				created := args[5].(time.Time).Format("2006-01-02T15:04:05.000000Z")
				if created != want.CreatedAt || args[6] != args[5] || want.UpdatedAt != want.CreatedAt {
					t.Errorf("row %d: created_at %s, vector %s", i, created, want.CreatedAt)
				}
				if args[8] != len(got.Payload) {
					t.Errorf("row %d: payload_size %v is not the payload's length %d", i, args[8], len(got.Payload))
				}
				if g, w := canonical(t, got.Payload), canonical(t, want.Payload); !reflect.DeepEqual(g, w) {
					t.Errorf("row %d (%s) payload differs from the vector:\n got  %s\n want %s", i, got.Type, got.Payload, want.Payload)
				}
				if bytes.Contains(got.Payload, []byte(`"data":"`)) || bytes.Contains(got.Payload, []byte(`"operationData":"`)) {
					t.Errorf("row %d: data/operationData written as a string: %s", i, got.Payload)
				}
				produced = append(produced, goPayload{Case: c.Name, Row: i, Type: got.Type, PayloadSize: len(got.Payload), PayloadText: string(got.Payload)})
			}
		})
	}

	golden := filepath.Join(vectorDir, "go-payloads.json")
	out, err := json.MarshalIndent(produced, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	out = append(out, '\n')
	if *update {
		if err := os.WriteFile(golden, out, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if !bytes.Equal(out, want) {
		t.Fatalf("Go's outbox payload bytes changed; review and run `go test ./outbox -run Conformance -update`:\n got  %s", out)
	}
}

func TestConformanceInvalidDispatchJobs(t *testing.T) {
	vectors := loadVectors(t)
	if len(vectors.InvalidDispatchJobs) == 0 {
		t.Fatal("no invalid dispatch-job vectors")
	}
	for _, in := range vectors.InvalidDispatchJobs {
		want, _ := in["error"].(string)
		err := buildDispatchJob(t, in, nil, "").Validate()
		if err == nil {
			t.Errorf("%v: accepted, vector refuses %q", in, want)
			continue
		}
		if err.Error() != want {
			t.Errorf("%v:\n got  %q\n want %q", in, err.Error(), want)
		}
		if _, _, perr := DispatchJobPayload(buildDispatchJob(t, in, nil, "")); perr == nil {
			t.Errorf("%v: DispatchJobPayload wrote an invalid job", in)
		}
	}
}

func ptrAny(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func (e vectorEvent) source() string     { return e.Source() }
func (e vectorEvent) entityType() string { return strings.ToUpper(e.seg(2)[:1]) + e.seg(2)[1:] }
func (e vectorEvent) entityID() string   { return e.subjectID }
