package outbox

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubEvent struct {
	id, typ, source, subject, group, principal, execution string
	data                                                  string
	dataErr                                               error
	at                                                    time.Time
}

func (e stubEvent) EventID() string       { return e.id }
func (e stubEvent) EventType() string     { return e.typ }
func (e stubEvent) SpecVersion() string   { return "1.0" }
func (e stubEvent) Source() string        { return e.source }
func (e stubEvent) Subject() string       { return e.subject }
func (e stubEvent) Time() time.Time       { return e.at }
func (e stubEvent) PrincipalID() string   { return e.principal }
func (e stubEvent) CorrelationID() string { return "" }
func (e stubEvent) CausationID() string   { return "" }
func (e stubEvent) ExecutionID() string   { return e.execution }
func (e stubEvent) MessageGroup() string  { return e.group }
func (e stubEvent) ToDataJSON() ([]byte, error) {
	return []byte(e.data), e.dataErr
}

type clientCodedEvent struct{ stubEvent }

func (clientCodedEvent) ClientCode() string { return "tenant-b" }

func base() stubEvent {
	return stubEvent{id: "0ABCDEFGHJKMN", typ: "shop:sales:order:placed", source: "shop:sales", subject: "sales.order.ord_1", data: `{"a":1}`}
}

func members(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

// F13: the SDK never writes double-encoded data. The event's value is
// written as itself; a string holding one encoded object or array (the other
// SDKs' form) is unwrapped once; any other string is kept; empty and null
// become {}; invalid JSON is an error, not a silent {}.
func TestEventPayloadDataIsTheValueItself(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`{"a":1}`, `{"a":1}`},
		{` { "a" : [1, 2] } `, `{"a":[1,2]}`},
		{`[1,2]`, `[1,2]`},
		{`"{\"a\":1}"`, `{"a":1}`},
		{`"[{\"line\":1}]"`, `[{"line":1}]`},
		{`"\"{\\\"a\\\":1}\""`, `"\"{\\\"a\\\":1}\""`}, // encoded twice: one level only, stays a string
		{`"hello"`, `"hello"`},
		{`"{not json"`, `"{not json"`},
		{`42`, `42`},
		{``, `{}`},
		{`null`, `{}`},
	} {
		e := base()
		e.data = c.in
		raw, err := EventPayload(e, "")
		require.NoError(t, err, c.in)
		assert.Equal(t, c.want, string(members(t, raw)["data"]), c.in)
	}
	e := base()
	e.data = `{"a":`
	_, err := EventPayload(e, "")
	require.Error(t, err)
}

func TestEventPayloadMembers(t *testing.T) {
	e := base()
	e.principal, e.execution, e.group = "prn_1", "run-1", "sales:order:ord_1"
	raw, err := EventPayload(e, " acme ")
	require.NoError(t, err)
	assert.JSONEq(t, `{"specVersion":"1.0","type":"shop:sales:order:placed","source":"shop:sales","subject":"sales.order.ord_1",
		"data":{"a":1},"deduplicationId":"0ABCDEFGHJKMN","messageGroup":"sales:order:ord_1","clientCode":"acme",
		"contextData":[{"key":"principalId","value":"prn_1"},{"key":"executionId","value":"run-1"}]}`, string(raw))
	for k := range members(t, raw) {
		assert.NotContains(t, k, "_", "camelCase members only")
	}

	// No source: left out, the platform defaults it (F14). No client code:
	// left out, the platform scopes by the credential.
	e = base()
	e.source = ""
	raw, err = EventPayload(e, "")
	require.NoError(t, err)
	m := members(t, raw)
	assert.NotContains(t, m, "source")
	assert.NotContains(t, m, "clientCode")
	assert.NotContains(t, m, "contextData")

	// An event's own client code wins over the writer's.
	raw, err = EventPayload(clientCodedEvent{base()}, "acme")
	require.NoError(t, err)
	assert.JSONEq(t, `"tenant-b"`, string(members(t, raw)["clientCode"]))

	// <, > and & are not HTML-escaped (the other SDKs' bytes).
	e = base()
	e.data = `{"html":"<a & b>"}`
	raw, err = EventPayload(e, "")
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"<a & b>"`)

	_, err = EventPayload(stubEvent{typ: "shop:x:y:z"}, "")
	require.ErrorContains(t, err, "event id")
	_, err = EventPayload(stubEvent{id: "x"}, "")
	require.ErrorContains(t, err, "event type")
}

type plainCommand struct {
	OrderID  string `json:"orderId"`
	Password string `json:"password"`
}

type namedCommand struct{ plainCommand }

func (namedCommand) AuditOperation() string { return "place-order" }

type dataCommand struct{ plainCommand }

func (dataCommand) AuditData() any { return map[string]any{"token": "t", "lines": 2} }

type noDataCommand struct{ plainCommand }

func (noDataCommand) AuditData() any { return nil }

func TestAuditPayload(t *testing.T) {
	e := base()
	e.principal = "prn_1"
	e.at = time.Date(2026, 10, 8, 10, 15, 31, 500_000_000, time.UTC)

	raw, err := AuditPayload(e, plainCommand{OrderID: "ord_1", Password: "p"}, "shop", "acme")
	require.NoError(t, err)
	assert.JSONEq(t, `{"entityType":"Order","entityId":"ord_1","operation":"plainCommand",
		"operationData":{"orderId":"ord_1","password":"***"},"principalId":"prn_1",
		"performedAt":"2026-10-08T10:15:31.500Z","source":"shop:sales","applicationCode":"shop","clientCode":"acme"}`, string(raw))

	raw, err = AuditPayload(e, &namedCommand{}, "", "")
	require.NoError(t, err)
	assert.JSONEq(t, `"place-order"`, string(members(t, raw)["operation"]))

	raw, err = AuditPayload(e, dataCommand{}, "", "")
	require.NoError(t, err)
	assert.JSONEq(t, `{"token":"***","lines":2}`, string(members(t, raw)["operationData"]))

	for _, cmd := range []any{nil, noDataCommand{}} {
		raw, err = AuditPayload(e, cmd, "", "")
		require.NoError(t, err)
		assert.NotContains(t, members(t, raw), "operationData")
	}

	e.principal = ""
	raw, err = AuditPayload(e, nil, "", "")
	require.NoError(t, err)
	assert.NotContains(t, members(t, raw), "principalId")
}

func TestFormatInstant(t *testing.T) {
	for ns, want := range map[int]string{
		0:           "2026-10-08T10:15:30Z",
		123_000_000: "2026-10-08T10:15:30.123Z",
		500_000_000: "2026-10-08T10:15:30.500Z",
		123_000:     "2026-10-08T10:15:30.000123Z",
		123_456_789: "2026-10-08T10:15:30.123456789Z",
	} {
		assert.Equal(t, want, FormatInstant(time.Date(2026, 10, 8, 12, 15, 30, ns, time.FixedZone("x", 2*3600))))
	}
}

// F14: a dispatch job may name its client by code; the platform resolves it.
func TestDispatchJobClientCode(t *testing.T) {
	j := NewDispatchJob("shop:billing:export", "https://billing.example.com/x").WithClientCode("acme").WithID("job-1")
	_, raw, err := DispatchJobPayload(j)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"job-1","kind":"TASK","code":"shop:billing:export","source":"shop","targetUrl":"https://billing.example.com/x",
		"payload":"{}","payloadContentType":"application/json","dataOnly":false,"clientCode":"acme"}`, string(raw))

	err = j.WithPlatformClientID("0CLIENT0000001").Validate()
	require.ErrorContains(t, err, "not both")

	resolved, err := NewDispatchJob("shop:x", "https://a.example.com").Resolved()
	require.NoError(t, err)
	assert.Len(t, resolved.ID, 13)
	assert.Equal(t, "shop", resolved.Source)
}

func TestDispatchJobMetadataIsTheKeyValueArray(t *testing.T) {
	_, raw, err := DispatchJobPayload(NewDispatchJob("shop:x", "https://a.example.com").WithID("j").
		WithMetadata("b", "2").WithMetadata("a", "1"))
	require.NoError(t, err)
	assert.JSONEq(t, `[{"key":"b","value":"2"},{"key":"a","value":"1"}]`, string(members(t, raw)["metadata"]))
}

func TestInsertStatementTable(t *testing.T) {
	stmt, err := InsertStatement("app.outbox", PlaceholderDollar)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(stmt, "INSERT INTO app.outbox (\n"))
	_, err = InsertStatement("outbox; DROP TABLE x", PlaceholderDollar)
	require.Error(t, err)
}

func TestRows(t *testing.T) {
	now := time.Date(2026, 10, 8, 10, 0, 0, 0, time.UTC)
	cfg := Config{ClientID: "clt_1", ClientCode: "acme", ApplicationCode: "shop", Now: func() time.Time { return now }}
	e := base()
	e.group = "sales:order:ord_1"

	row, err := EventRow(cfg, e)
	require.NoError(t, err)
	assert.Equal(t, "0ABCDEFGHJKMN", row.ID)
	assert.Equal(t, TypeEvent, row.Type)
	args := row.Args()
	assert.Equal(t, []any{"0ABCDEFGHJKMN", "EVENT", "sales:order:ord_1", string(row.Payload), 0, now, now, "clt_1", len(row.Payload), nil}, args)

	row, err = AuditRow(cfg, e, nil)
	require.NoError(t, err)
	assert.Equal(t, TypeAuditLog, row.Type)
	assert.Nil(t, row.Args()[2], "audit rows carry no message group")
	assert.Len(t, row.ID, 13)

	row, err = DispatchJobRow(Config{}, NewDispatchJob("shop:x", "https://a.example.com").WithMessageGroup("g").WithID("job-9"))
	require.NoError(t, err)
	assert.Equal(t, "job-9", row.ID)
	assert.Equal(t, "g", row.Args()[2])
	assert.Nil(t, row.Args()[7], "empty client id is NULL")

	_, err = DispatchJobRow(Config{}, NewDispatchJob("Shop:x", "https://a.example.com"))
	require.Error(t, err)
}
