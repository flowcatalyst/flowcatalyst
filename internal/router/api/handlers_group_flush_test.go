package api_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
	routerapi "github.com/flowcatalyst/flowcatalyst-go/internal/router/api"
)

// ── Test doubles ──────────────────────────────────────────────────────────

type stubBlockedGroups struct{ rows []router.GroupInfo }

func (s stubBlockedGroups) BlockedGroups() []router.GroupInfo { return s.rows }

type stubGroupFlush struct {
	snaps       []routerapi.GroupFlushSnapshot
	clearCalls  []string // "pool/group" per call, in order
	clearResult bool
}

func (s *stubGroupFlush) GroupFlushSnapshots() []routerapi.GroupFlushSnapshot { return s.snaps }
func (s *stubGroupFlush) ClearGroupFlush(poolCode, group string) bool {
	s.clearCalls = append(s.clearCalls, poolCode+"/"+group)
	return s.clearResult
}

func setupGroupMonitoringAPI(t *testing.T, bg routerapi.BlockedGroupsProvider, gf routerapi.GroupFlushProvider) humatest.TestAPI {
	t.Helper()
	ws := router.NewWarningService(router.WarningServiceConfig{})
	hs := router.NewHealthService(router.DefaultHealthServiceConfig(), ws)
	state := &routerapi.State{
		Warnings:      ws,
		Health:        hs,
		BlockedGroups: bg,
		GroupFlush:    gf,
		Mocks:         routerapi.NewMockState(),
	}
	_, api := humatest.New(t)
	routerapi.Register(api, state)
	return api
}

// ── R-04 blocked groups ──────────────────────────────────────────────────

func TestBlockedGroups_ListsAcrossPools(t *testing.T) {
	parked := time.Now().Add(-90 * time.Second)
	until := time.Now().Add(30 * time.Second)
	bg := stubBlockedGroups{rows: []router.GroupInfo{
		{Group: "g1", PoolCode: "FAST", Buffered: 3, Working: true},
		{Group: "g2", PoolCode: "SLOW", Buffered: 1, Working: false, ParkedAt: parked},
		{Group: "g3", PoolCode: "SLOW", Buffered: 5, Working: true, Suppressed: true, SuppressedUntil: until},
	}}
	api := setupGroupMonitoringAPI(t, bg, nil)

	resp := api.Get("/monitoring/blocked-groups")
	if resp.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", resp.Code, resp.Body.String())
	}
	var rows []routerapi.BlockedGroupInfo
	decodeBody(t, resp.Body.Bytes(), &rows)
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3: %+v", len(rows), rows)
	}
	var g2, g3 *routerapi.BlockedGroupInfo
	for i := range rows {
		switch rows[i].Group {
		case "g2":
			g2 = &rows[i]
		case "g3":
			g3 = &rows[i]
		}
	}
	if g2 == nil || g2.ParkedAt == nil {
		t.Fatalf("g2 must carry a non-nil parkedAt: %+v", g2)
	}
	if g3 == nil || !g3.Suppressed || g3.SuppressedUntil == nil {
		t.Fatalf("g3 must carry suppressed=true and a non-nil suppressedUntil: %+v", g3)
	}
}

func TestBlockedGroups_PoolCodeFilter(t *testing.T) {
	bg := stubBlockedGroups{rows: []router.GroupInfo{
		{Group: "g1", PoolCode: "FAST", Buffered: 1},
		{Group: "g2", PoolCode: "SLOW", Buffered: 1},
	}}
	api := setupGroupMonitoringAPI(t, bg, nil)

	resp := api.Get("/monitoring/blocked-groups?poolCode=SLOW")
	if resp.Code != http.StatusOK {
		t.Fatalf("status %d", resp.Code)
	}
	var rows []routerapi.BlockedGroupInfo
	decodeBody(t, resp.Body.Bytes(), &rows)
	if len(rows) != 1 || rows[0].PoolCode != "SLOW" {
		t.Fatalf("poolCode filter not applied: %+v", rows)
	}
}

func TestBlockedGroups_NilProviderReturnsEmptyList(t *testing.T) {
	api := setupGroupMonitoringAPI(t, nil, nil)

	resp := api.Get("/monitoring/blocked-groups")
	if resp.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", resp.Code, resp.Body.String())
	}
	var rows []routerapi.BlockedGroupInfo
	decodeBody(t, resp.Body.Bytes(), &rows)
	if len(rows) != 0 {
		t.Fatalf("want empty list when unwired, got %+v", rows)
	}
}

// ── R-52/R-53 group-flush suppression ────────────────────────────────────

func TestGroupFlushes_ListsSnapshotsAcrossPools(t *testing.T) {
	until := time.Now().Add(45 * time.Second)
	gf := &stubGroupFlush{snaps: []routerapi.GroupFlushSnapshot{
		{
			PoolCode: "FAST", ActiveCount: 1, TotalFlushes: 4, TotalSuppressed: 9,
			Groups: []router.GroupSuppression{{Group: "g1", Until: until}},
		},
		{PoolCode: "SLOW", ActiveCount: 0, TotalFlushes: 0, TotalSuppressed: 0},
	}}
	api := setupGroupMonitoringAPI(t, nil, gf)

	resp := api.Get("/monitoring/group-flushes")
	if resp.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", resp.Code, resp.Body.String())
	}
	var rows []routerapi.GroupFlushPoolInfo
	decodeBody(t, resp.Body.Bytes(), &rows)
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2: %+v", len(rows), rows)
	}
	var fast *routerapi.GroupFlushPoolInfo
	for i := range rows {
		if rows[i].PoolCode == "FAST" {
			fast = &rows[i]
		}
	}
	if fast == nil || fast.ActiveCount != 1 || fast.TotalFlushes != 4 || fast.TotalSuppressed != 9 {
		t.Fatalf("FAST snapshot mismatch: %+v", fast)
	}
	if len(fast.Groups) != 1 || fast.Groups[0].Group != "g1" {
		t.Fatalf("FAST groups mismatch: %+v", fast.Groups)
	}
}

func TestGroupFlushes_NilProviderReturnsEmptyList(t *testing.T) {
	api := setupGroupMonitoringAPI(t, nil, nil)

	resp := api.Get("/monitoring/group-flushes")
	if resp.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", resp.Code, resp.Body.String())
	}
	var rows []routerapi.GroupFlushPoolInfo
	decodeBody(t, resp.Body.Bytes(), &rows)
	if len(rows) != 0 {
		t.Fatalf("want empty list when unwired, got %+v", rows)
	}
}

func TestClearGroupFlush_Success(t *testing.T) {
	gf := &stubGroupFlush{clearResult: true}
	api := setupGroupMonitoringAPI(t, nil, gf)

	resp := api.Post("/monitoring/group-flushes/FAST/g1/clear")
	if resp.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", resp.Code, resp.Body.String())
	}
	var out routerapi.ClearGroupFlushResponse
	decodeBody(t, resp.Body.Bytes(), &out)
	if !out.Cleared || out.PoolCode != "FAST" || out.Group != "g1" {
		t.Fatalf("unexpected response: %+v", out)
	}
	if len(gf.clearCalls) != 1 || gf.clearCalls[0] != "FAST/g1" {
		t.Fatalf("ClearGroupFlush not called with (FAST, g1): %+v", gf.clearCalls)
	}
}

func TestClearGroupFlush_NotFoundWhenNoActiveSuppression(t *testing.T) {
	gf := &stubGroupFlush{clearResult: false}
	api := setupGroupMonitoringAPI(t, nil, gf)

	resp := api.Post("/monitoring/group-flushes/FAST/g1/clear")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status %d body=%s, want 404", resp.Code, resp.Body.String())
	}
}

func TestClearGroupFlush_NotConfiguredWhenNilProvider(t *testing.T) {
	api := setupGroupMonitoringAPI(t, nil, nil)

	resp := api.Post("/monitoring/group-flushes/FAST/g1/clear")
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d body=%s, want 503", resp.Code, resp.Body.String())
	}
}
