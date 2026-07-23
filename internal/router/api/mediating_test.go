package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
	routerapi "github.com/flowcatalyst/flowcatalyst-go/internal/router/api"
)

type stubMediatingProvider struct{ entries []router.MediatingEntry }

func (s stubMediatingProvider) MediatingSnapshot() []router.MediatingEntry { return s.entries }

func nowMinus(sec int) time.Time { return time.Now().Add(-time.Duration(sec) * time.Second) }

// TestMediating_OrderedByElapsedAndFiltered covers the live "what is being
// mediated right now" endpoint: rows sorted longest-mediating first, the
// poolCode filter applied, and the fields (group/target/attempts) carried
// through. Elapsed is derived from MediatedAt at request time, so the fixture
// stamps distinct ages via time offsets that the handler converts to ms.
func TestMediating_OrderedByElapsedAndFiltered(t *testing.T) {
	// Use a fixed clock offset by constructing entries whose MediatedAt is in
	// the past; the handler computes elapsed = now - MediatedAt.
	entries := []router.MediatingEntry{
		{MessageID: "short", PoolCode: "p1", Group: "g", Target: "http://t", MediatedAt: nowMinus(2)},
		{MessageID: "stuck", PoolCode: "p1", Group: "g", Target: "http://t", Attempts: 0, MediatedAt: nowMinus(600)},
		{MessageID: "mid", PoolCode: "p1", Group: "g", Target: "http://t", Attempts: 2, MediatedAt: nowMinus(90)},
		{MessageID: "other-pool", PoolCode: "p2", Group: "g", Target: "http://t", MediatedAt: nowMinus(999)},
	}

	ws := router.NewWarningService(router.WarningServiceConfig{})
	hs := router.NewHealthService(router.DefaultHealthServiceConfig(), ws)
	_, api := humatest.New(t)
	routerapi.Register(api, &routerapi.State{
		Warnings: ws, Health: hs, Mocks: routerapi.NewMockState(),
		Mediating: stubMediatingProvider{entries: entries},
	})

	// Filter to p1 → the p2 entry (even though it's the oldest) must be excluded,
	// and the rest returned longest-mediating first.
	resp := api.Get("/monitoring/mediating?poolCode=p1")
	if resp.Code != http.StatusOK {
		t.Fatalf("status %d", resp.Code)
	}
	var body []routerapi.MediatingInfo
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body) != 3 {
		t.Fatalf("want 3 p1 rows, got %d", len(body))
	}
	gotOrder := []string{body[0].MessageID, body[1].MessageID, body[2].MessageID}
	wantOrder := []string{"stuck", "mid", "short"}
	for i := range wantOrder {
		if gotOrder[i] != wantOrder[i] {
			t.Fatalf("order = %v, want %v (longest mediating first)", gotOrder, wantOrder)
		}
	}
	if body[0].ElapsedTimeMs < body[1].ElapsedTimeMs {
		t.Errorf("not sorted desc")
	}
	for _, r := range body {
		if r.PoolCode == "p2" {
			t.Errorf("poolCode filter leaked a p2 row")
		}
	}
	if body[1].Attempts != 2 {
		t.Errorf("mid attempts = %d, want 2", body[1].Attempts)
	}
	if body[0].Target != "http://t" {
		t.Errorf("target not carried through: %q", body[0].Target)
	}
}
