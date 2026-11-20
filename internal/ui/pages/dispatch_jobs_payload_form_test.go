package pages

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// The payload edit's form carries one payload, up to the largest the
// deployment accepts, so it takes the API edit's request cap
// (PayloadFormBytes) instead of the 1 MiB every other form has
// (docs/payload-offload-design.md, section 8). Unset, it keeps that 1 MiB.
func TestDispatchJobsPayloadFormCap(t *testing.T) {
	big := `"` + strings.Repeat("p", 2<<20) + `"` // 2 MiB: over the common form cap
	for _, c := range []struct {
		name      string
		cap       int64
		multipart bool
		saved     bool
	}{
		{"unset: the common cap refuses it", 0, true, false},
		{"the edit's cap takes it (Datastar's multipart)", 5 << 20, true, true},
		{"the edit's cap takes it (a native form)", 5 << 20, false, true},
		{"over the edit's cap", 1 << 20, true, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newDJHarness()
			page := h.wire(&DispatchJobs{
				Jobs: h.store, Ops: h.ops, Clients: newCliHarness().store, Shell: shellForTest(),
				ClientIdentifier: func(context.Context, string) (string, bool) { return "", false },
				PayloadFormBytes: c.cap,
			})
			rec := serveMounted(t, anchorAdmin(), req{
				method: http.MethodPost, target: "/ui/dispatch-jobs/djb_1/payload", datastar: true, multipart: c.multipart,
				form: url.Values{"payload": {big}, "contentType": {"application/json"}},
			}, page)
			if saved := len(h.ops.edits) == 1 && strings.HasPrefix(h.ops.edits[0], "djb_1|"+big+"|"); saved != c.saved {
				t.Fatalf("saved=%v, want %v (status %d, %d edits)", saved, c.saved, rec.Code, len(h.ops.edits))
			}
		})
	}
	// Every other form keeps the common cap.
	h := newDJHarness()
	page := h.wire(&DispatchJobs{Jobs: h.store, Ops: h.ops, Clients: newCliHarness().store, Shell: shellForTest(), PayloadFormBytes: 5 << 20})
	requeue := func(pad string) {
		serveMounted(t, anchorAdmin(), req{
			method: http.MethodPost, target: "/ui/dispatch-jobs/djb_1/requeue", datastar: true, multipart: true,
			form: url.Values{"pad": {pad}},
		}, page)
	}
	requeue("small")
	if len(h.ops.requeued) != 1 {
		t.Fatalf("a small requeue form was not read: %v", h.ops.requeued)
	}
	requeue(big)
	if len(h.ops.requeued) != 1 {
		t.Fatalf("a requeue form over 1 MiB was read: %d requeues", len(h.ops.requeued))
	}
}
