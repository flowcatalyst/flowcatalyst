package function

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTraceLimitsDefaultsAndIndefinite(t *testing.T) {
	l := TraceLimits{}.Normalised()
	if l.IndefiniteMaxDays != 7 || l.PruneMaxRows != 5_000_000 || l.PruneInterval != time.Hour {
		t.Fatalf("defaults %+v", l)
	}
	if l.IndefiniteMaxDaysFor(TraceHTTPContext) != 0 || l.IndefiniteMaxDaysFor("orders") != 7 {
		t.Fatal("http never takes an indefinite window")
	}
	if l.IndefiniteCeiling() != 7*24*time.Hour {
		t.Fatalf("ceiling %v", l.IndefiniteCeiling())
	}
	if n := (TraceLimits{PruneMaxRows: 12_001}).PruneChunks(); n != 3 {
		t.Fatalf("chunks %d", n)
	}
}

func TestWithholdTraceDetail(t *testing.T) {
	httpRec := TraceLogRecord{Kind: TraceKindHTTP, Message: "POST /orders -> 200", Data: []byte(`{
		"request":{"method":"POST","path":"/orders","rawQuery":"token=x","headers":{"cookie":["a"]},"body":"secret","bodyEncoding":"utf8","bodyBytes":6,"bodyTruncated":false,"caller":{"kind":"principal","id":"prn_1"}},
		"response":{"status":200,"headers":{"set-cookie":["b"]},"body":"ok","bodyEncoding":"utf8","bodyBytes":2,"bodyTruncated":true},
		"durationMs":12,"outcome":"ok"}`)}
	if !WithholdTraceDetail(&httpRec) {
		t.Fatal("an http record always has something withheld")
	}
	want := `{"request":{"method":"POST","path":"/orders","body":null,"bodyBytes":6,"bodyTruncated":false},` +
		`"response":{"status":200,"body":null,"bodyBytes":2,"bodyTruncated":true},"durationMs":12,"outcome":"ok"}`
	var got, exp any
	_ = json.Unmarshal(httpRec.Data, &got)
	_ = json.Unmarshal([]byte(want), &exp)
	if b1, b2 := mustJSON(got), mustJSON(exp); b1 != b2 {
		t.Fatalf("http summary\n got %s\nwant %s", b1, b2)
	}
	if httpRec.Message != "POST /orders -> 200" {
		t.Fatal("the summary line stays")
	}

	logRec := TraceLogRecord{Kind: TraceKindLog, Message: "card 4111", Data: []byte(`{"k":1}`)}
	if !WithholdTraceDetail(&logRec) || logRec.Message != "" || logRec.Data != nil {
		t.Fatalf("log %+v", logRec)
	}
	traceRec := TraceLogRecord{Kind: TraceKindTrace, Message: "mapped", Data: []byte(`{"k":1}`)}
	if !WithholdTraceDetail(&traceRec) || traceRec.Message != "mapped" || traceRec.Data != nil {
		t.Fatalf("trace %+v", traceRec)
	}
	bare := TraceLogRecord{Kind: TraceKindTrace, Message: "plain"}
	if WithholdTraceDetail(&bare) {
		t.Fatal("nothing to withhold")
	}
	broken := TraceLogRecord{Kind: TraceKindHTTP, Data: []byte(`not json`)}
	if WithholdTraceDetail(&broken); broken.Data != nil {
		t.Fatal("unparseable http data is dropped, never passed through")
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
