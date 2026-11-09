package outbox

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func fixedCfg() Config {
	return Config{ClientID: "acme", ClientCode: "acme", Now: func() time.Time { return time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC) }}
}

func TestPushRowShape(t *testing.T) {
	row, err := PushRow(fixedCfg(), NewPush("shop:orders:ord-1", "order.updated", map[string]any{"b": 2, "a": 1}).WithID("0PQRSTVWXYZ01"))
	if err != nil {
		t.Fatal(err)
	}
	if row.ID != "0PQRSTVWXYZ01" || row.Type != TypePush || row.MessageGroup != "shop:orders:ord-1" || row.ClientID != "acme" {
		t.Fatalf("row = %+v", row)
	}
	// members in the framework's order; data is the object itself, compact, never an encoded string
	want := `{"topic":"shop:orders:ord-1","type":"order.updated","data":{"a":1,"b":2},"clientCode":"acme","pushId":"0PQRSTVWXYZ01"}`
	if string(row.Payload) != want {
		t.Fatalf("payload\n got %s\nwant %s", row.Payload, want)
	}
	if args := row.Args(); args[8] != len(row.Payload) {
		t.Fatalf("payload_size = %v", args[8])
	}
}

func TestPushRowGroupAndClientRules(t *testing.T) {
	cfg := fixedCfg()
	row, _ := PushRow(cfg, NewPush("shop:a", "x.y", map[string]any{}).WithUngrouped().WithMessageGroup("ignored"))
	if row.MessageGroup != "" {
		t.Errorf("an ungrouped push has no group: %q", row.MessageGroup)
	}
	row, _ = PushRow(cfg, NewPush("shop:a", "x.y", map[string]any{}).WithMessageGroup("custom"))
	if row.MessageGroup != "custom" {
		t.Errorf("group = %q", row.MessageGroup)
	}
	// the push's own client wins; the platform's context leaves the client out
	row, _ = PushRow(cfg, NewPush("shop:a", "x.y", map[string]any{}).WithClientCode("globex"))
	if !strings.Contains(string(row.Payload), `"clientCode":"globex"`) {
		t.Errorf("payload %s", row.Payload)
	}
	row, _ = PushRow(Config{ClientCode: PlatformClientCode}, NewPush("shop:a", "x.y", map[string]any{}))
	if strings.Contains(string(row.Payload), "clientCode") {
		t.Errorf("platform context: %s", row.Payload)
	}
	row, _ = PushRow(Config{}, NewPush("shop:a", "x.y", map[string]any{}))
	if strings.Contains(string(row.Payload), "clientCode") {
		t.Errorf("no client configured: %s", row.Payload)
	}
	// a generated id is a TSID and the pushId
	var p struct{ PushID string }
	_ = json.Unmarshal(row.Payload, &p)
	if !tsidPattern.MatchString(row.ID) || p.PushID != row.ID {
		t.Errorf("id %q pushId %q", row.ID, p.PushID)
	}
}

func TestPushDataForms(t *testing.T) {
	type order struct {
		ID    string `json:"id"`
		Lines int    `json:"lines"`
	}
	for name, data := range map[string]any{
		"a struct": order{ID: "o1", Lines: 2}, "a map": map[string]any{"id": "o1", "lines": 2},
		"json text": json.RawMessage(` { "id" : "o1", "lines": 2 } `), "bytes": []byte(`{"id":"o1","lines":2}`),
	} {
		row, err := PushRow(Config{}, NewPush("shop:a", "x.y", data))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(string(row.Payload), `"data":{"id":"o1","lines":2}`) {
			t.Errorf("%s: %s", name, row.Payload)
		}
	}
	// html is not escaped, as for the other rows
	row, _ := PushRow(Config{}, NewPush("shop:a", "x.y", map[string]string{"t": "<b>&"}))
	if !strings.Contains(string(row.Payload), `"t":"<b>&"`) {
		t.Errorf("escaped: %s", row.Payload)
	}
}

func TestPushRefusals(t *testing.T) {
	big := map[string]string{"s": strings.Repeat("a", DefaultPushMaxDataBytes)}
	for name, c := range map[string]struct {
		p    Push
		code string
	}{
		"upper-case application": {NewPush("Shop:a", "x.y", map[string]any{}), PushTopicInvalid},
		"one segment":            {NewPush("shop", "x.y", map[string]any{}), PushTopicInvalid},
		"empty segment":          {NewPush("shop:a:", "x.y", map[string]any{}), PushTopicInvalid},
		"long topic":             {NewPush("shop:"+strings.Repeat("a", 251), "x.y", map[string]any{}), PushTopicInvalid},
		"type with capitals":     {NewPush("shop:a", "Order.updated", map[string]any{}), PushTypeInvalid},
		"empty type":             {NewPush("shop:a", "", map[string]any{}), PushTypeInvalid},
		"long type":              {NewPush("shop:a", strings.Repeat("a", 101), map[string]any{}), PushTypeInvalid},
		"reserved reset":         {NewPush("shop:a", "reset", map[string]any{}), PushTypeReserved},
		"reserved prefix":        {NewPush("shop:a", "fw.x", map[string]any{}), PushTypeReserved},
		"an array":               {NewPush("shop:a", "x.y", []int{1}), PushDataNotObject},
		"null":                   {NewPush("shop:a", "x.y", nil), PushDataNotObject},
		"a string":               {NewPush("shop:a", "x.y", "text"), PushDataNotObject},
		"bad json text":          {NewPush("shop:a", "x.y", json.RawMessage(`{"a":`)), PushDataInvalid},
		"no data":                {NewPush("shop:a", "x.y", json.RawMessage(nil)), PushDataInvalid},
		"unmarshalable":          {NewPush("shop:a", "x.y", make(chan int)), PushDataInvalid},
		"too large":              {NewPush("shop:a", "x.y", big), PushDataTooLarge},
		"bad id":                 {NewPush("shop:a", "x.y", map[string]any{}).WithID("has space"), PushIDInvalid},
		"long id":                {NewPush("shop:a", "x.y", map[string]any{}).WithID(strings.Repeat("i", 27)), PushIDInvalid},
		"blank client":           {NewPush("shop:a", "x.y", map[string]any{}).WithClientCode("  "), PushClientInvalid},
	} {
		err := c.p.Validate()
		var pe *PushError
		if !errors.As(err, &pe) || pe.Code != c.code {
			t.Errorf("%s: %v, want %s", name, err, c.code)
		}
		if _, err := PushRow(fixedCfg(), c.p); err == nil {
			t.Errorf("%s: PushRow wrote it", name)
		}
	}
	// the limit is the application's push.max-data-bytes, at most 65,536
	if _, err := PushRow(Config{}, NewPush("shop:a", "x.y", big).WithMaxDataBytes(MaxPushDataBytes)); err != nil {
		t.Errorf("a raised limit: %v", err)
	}
	if err := NewPush("shop:a", "x.y", map[string]string{"s": strings.Repeat("a", MaxPushDataBytes)}).WithMaxDataBytes(1 << 20).Validate(); err == nil {
		t.Error("the limit is capped at 65536")
	}
}

func TestPushInsertStatement(t *testing.T) {
	stmt, err := PushInsertStatement("")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stmt, "pg_notify('"+PushNotifyChannel+"'") || !strings.Contains(stmt, "INSERT INTO outbox_messages") {
		t.Fatalf("%s", stmt)
	}
	if strings.Count(stmt, "$") != len(Row{}.Args()) {
		t.Errorf("%d placeholders for %d arguments", strings.Count(stmt, "$"), len(Row{}.Args()))
	}
	if s, err := PushInsertStatement("app.outbox_2"); err != nil || !strings.Contains(s, "INSERT INTO app.outbox_2") {
		t.Errorf("%q %v", s, err)
	}
	if _, err := PushInsertStatement("x; DROP TABLE y"); err == nil {
		t.Error("an invalid table name was accepted")
	}
}
