package router

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// TestRunImmediateAllocs pins the per-message allocation count of the
// IMMEDIATE success path (the retry closure must not force the message copy
// onto the heap for every message).
func TestRunImmediateAllocs(t *testing.T) {
	c := &grConsumer{}
	p := grPool(&grMediator{outcome: common.Success(200)}, c)
	ctx := context.Background()
	msg := grMsg("alloc-1", "http://127.0.0.1:1/x")
	run := func() {
		if err := p.sem.acquire(ctx); err != nil {
			t.Fatal(err)
		}
		p.runImmediate(ctx, msg)
	}
	run()
	got := testing.AllocsPerRun(500, run)
	t.Logf("runImmediate allocs/op = %v", got)
	if got > 40 && !raceEnabled {
		t.Fatalf("allocs/op = %v", got)
	}
}

func TestMarshalMediationPayloadMatchesJSON(t *testing.T) {
	for _, id := range []string{
		"", "0HZXY1234ABCD", "a-b_c.d:e", `q"uote`, `back\slash`, "tab\t", "<b>&", "ünï", " ", "bad\xffutf8", "del\x7f",
	} {
		want, err := json.Marshal(mediationPayload{MessageID: id})
		if err != nil {
			t.Fatal(err)
		}
		got, err := marshalMediationPayload(id)
		if err != nil || string(got) != string(want) {
			t.Errorf("%q: got %s want %s (%v)", id, got, want, err)
		}
	}
}

func TestMarshalMediationPayloadAllocs(t *testing.T) {
	var sink []byte
	fast := testing.AllocsPerRun(1000, func() { sink, _ = marshalMediationPayload("0HZXY1234ABCD") })
	std := testing.AllocsPerRun(1000, func() { sink, _ = json.Marshal(mediationPayload{MessageID: "0HZXY1234ABCD"}) })
	_ = sink
	t.Logf("payload allocs/op: json.Marshal=%v fast=%v", std, fast)
	if fast > 1 && !raceEnabled {
		t.Fatalf("fast path allocs = %v", fast)
	}
}
