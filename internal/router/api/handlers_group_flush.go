package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// registerGroupMonitoring mounts the R-04 blocked-groups view and the
// R-52/R-53 group-flush suppression listing + operator clear.
func registerGroupMonitoring(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID: "blockedGroups", Method: http.MethodGet, Path: "/monitoring/blocked-groups",
		Summary:       "Message groups currently held by a pool (R-04)",
		Description:   "Every live message group across every pool this router is tracking — buffered awaiting a drainer, being drained, or parked with none running — including a pool still finishing an asynchronous removal-drain (X-11).",
		Tags:          []string{tagMonitoring},
		DefaultStatus: http.StatusOK,
	}, s.blockedGroups)
	huma.Register(api, huma.Operation{
		OperationID: "groupFlushes", Method: http.MethodGet, Path: "/monitoring/group-flushes",
		Summary:       "Active group-flush suppressions across pools (R-52/R-53)",
		Description:   "A snapshot per pool: every message group currently suppressed (a target asked the router to stop sending it for a bounded window) plus that pool's lifetime flush/suppressed counters.",
		Tags:          []string{tagMonitoring},
		DefaultStatus: http.StatusOK,
	}, s.groupFlushes)
	huma.Register(api, huma.Operation{
		OperationID: "clearGroupFlush", Method: http.MethodPost, Path: "/monitoring/group-flushes/{pool}/{group}/clear",
		Summary:       "Lift an active group-flush suppression early (operator override)",
		Tags:          []string{tagMonitoring},
		DefaultStatus: http.StatusOK,
	}, s.clearGroupFlush)
}

type blockedGroupsInput struct {
	// PoolCode filters to one pool when set (matches the dashboard's
	// blocked-groups pool filter).
	PoolCode string `query:"poolCode"`
}

type blockedGroupsOutput struct {
	Body []BlockedGroupInfo
}

func (s *State) blockedGroups(_ context.Context, in *blockedGroupsInput) (*blockedGroupsOutput, error) {
	if s.BlockedGroups == nil {
		return &blockedGroupsOutput{Body: []BlockedGroupInfo{}}, nil
	}
	poolFilter := strings.ToLower(strings.TrimSpace(in.PoolCode))
	rows := s.BlockedGroups.BlockedGroups()
	out := make([]BlockedGroupInfo, 0, len(rows))
	for _, g := range rows {
		if poolFilter != "" && !strings.EqualFold(g.PoolCode, poolFilter) {
			continue
		}
		row := BlockedGroupInfo{
			Group:      g.Group,
			PoolCode:   g.PoolCode,
			Buffered:   g.Buffered,
			Working:    g.Working,
			Suppressed: g.Suppressed,
		}
		if !g.ParkedAt.IsZero() {
			t := g.ParkedAt.UTC()
			row.ParkedAt = &t
		}
		if g.Suppressed && !g.SuppressedUntil.IsZero() {
			t := g.SuppressedUntil.UTC()
			row.SuppressedUntil = &t
		}
		out = append(out, row)
	}
	return &blockedGroupsOutput{Body: out}, nil
}

type groupFlushesOutput struct {
	Body []GroupFlushPoolInfo
}

func (s *State) groupFlushes(_ context.Context, _ *emptyInput) (*groupFlushesOutput, error) {
	if s.GroupFlush == nil {
		return &groupFlushesOutput{Body: []GroupFlushPoolInfo{}}, nil
	}
	snapshots := s.GroupFlush.GroupFlushSnapshots()
	out := make([]GroupFlushPoolInfo, 0, len(snapshots))
	for _, snap := range snapshots {
		groups := make([]GroupFlushEntry, 0, len(snap.Groups))
		for _, g := range snap.Groups {
			groups = append(groups, GroupFlushEntry{Group: g.Group, SuppressedUntil: g.Until.UTC()})
		}
		out = append(out, GroupFlushPoolInfo{
			PoolCode:        snap.PoolCode,
			ActiveCount:     snap.ActiveCount,
			TotalFlushes:    snap.TotalFlushes,
			TotalSuppressed: snap.TotalSuppressed,
			Groups:          groups,
		})
	}
	return &groupFlushesOutput{Body: out}, nil
}

type clearGroupFlushInput struct {
	Pool  string `path:"pool"`
	Group string `path:"group"`
}

type clearGroupFlushOutput struct {
	Body ClearGroupFlushResponse
}

func (s *State) clearGroupFlush(_ context.Context, in *clearGroupFlushInput) (*clearGroupFlushOutput, error) {
	if s.GroupFlush == nil {
		return nil, notConfigured("group flush registry")
	}
	if !s.GroupFlush.ClearGroupFlush(in.Pool, in.Group) {
		return nil, huma.Error404NotFound("no active suppression for pool " + in.Pool + " group " + in.Group)
	}
	return &clearGroupFlushOutput{Body: ClearGroupFlushResponse{
		Cleared: true, PoolCode: in.Pool, Group: in.Group,
	}}, nil
}
