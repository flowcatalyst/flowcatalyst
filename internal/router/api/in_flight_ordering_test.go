package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
	routerapi "github.com/flowcatalyst/flowcatalyst-go/internal/router/api"
)

// TestInFlightMessages_OrderedByElapsedDesc is the regression guard for the
// operator-facing "which ids are being mediated, longest first" view. It pins
// two behaviours the handler previously lacked:
//   - rows come back sorted by elapsed DESCENDING (longest in flight first);
//   - the longest-running entries survive a small limit — the old handler
//     truncated in map order BEFORE sorting, so orphaned/stuck messages could
//     be dropped from the view entirely when the backlog exceeded the limit.
// It also asserts the additive messageGroup + attempts fields are populated.
func TestInFlightMessages_OrderedByElapsedDesc(t *testing.T) {
	now := time.Now()
	// Deliberately inserted OUT of elapsed order; entries carry distinct ages.
	entries := []common.InFlightMessage{
		{MessageID: "young", PoolCode: "p", QueueIdentifier: "q", MessageGroupID: "g1", StartedAt: now.Add(-2 * time.Second)},
		{MessageID: "orphan", PoolCode: "p", QueueIdentifier: "q", MessageGroupID: "g2", Attempts: 0, StartedAt: now.Add(-9 * time.Minute)},
		{MessageID: "mid", PoolCode: "p", QueueIdentifier: "q", MessageGroupID: "g1", Attempts: 3, StartedAt: now.Add(-90 * time.Second)},
	}

	ws := router.NewWarningService(router.WarningServiceConfig{})
	hs := router.NewHealthService(router.DefaultHealthServiceConfig(), ws)
	_, api := humatest.New(t)
	routerapi.Register(api, &routerapi.State{
		Warnings: ws, Health: hs, Mocks: routerapi.NewMockState(),
		InFlight: stubInFlightProvider{entries: entries},
	})

	// limit=2 must return the TWO longest-running, in elapsed-desc order —
	// never the arbitrary map-order first two.
	resp := api.Get("/monitoring/in-flight-messages?limit=2")
	if resp.Code != http.StatusOK {
		t.Fatalf("status %d", resp.Code)
	}
	var body []routerapi.InFlightMessageInfo
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 2 {
		t.Fatalf("want 2 rows (limited), got %d", len(body))
	}
	if body[0].MessageID != "orphan" || body[1].MessageID != "mid" {
		t.Fatalf("want [orphan, mid] (longest first), got [%s, %s]", body[0].MessageID, body[1].MessageID)
	}
	if body[0].ElapsedTimeMs < body[1].ElapsedTimeMs {
		t.Errorf("not sorted desc: %d < %d", body[0].ElapsedTimeMs, body[1].ElapsedTimeMs)
	}
	// The "young" message must NOT appear — it's the shortest and beyond the limit.
	for _, r := range body {
		if r.MessageID == "young" {
			t.Errorf("young message leaked into the top-2 longest view")
		}
	}
	// Additive fields carried through.
	if body[0].MessageGroup != "g2" {
		t.Errorf("orphan messageGroup = %q, want g2", body[0].MessageGroup)
	}
	if body[1].Attempts != 3 {
		t.Errorf("mid attempts = %d, want 3", body[1].Attempts)
	}
}
