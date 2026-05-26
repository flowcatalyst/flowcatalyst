// Package bff hosts the /bff/* endpoints — frontend-only, session-auth,
// response shapes tuned to screens. Mirrors fc-platform/src/shared/bff_*.rs.
//
// Phase 3g ships dashboard.go as the worked example. Other BFF endpoints
// (event_types, roles, scheduled_jobs, developer, ...) follow the same
// pattern: chi handler → permission check → repository call → JSON shape
// tailored to a specific screen.
package bff

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// DashboardState bundles the dashboard endpoint's deps.
type DashboardState struct {
	Pool *pgxpool.Pool
}

// DashboardStats is the response for GET /bff/dashboard/stats.
//
// Mix of exact counts (control plane — bounded) and approximate counts
// (message plane — uses pg_class.reltuples to stay constant-time).
type DashboardStats struct {
	TotalClients        uint64 `json:"totalClients"`
	ActiveUsers         uint64 `json:"activeUsers"`
	RolesDefined        uint64 `json:"rolesDefined"`
	EventsApprox        uint64 `json:"eventsApprox"`
	DispatchJobsApprox  uint64 `json:"dispatchJobsApprox"`
	AuditLogsApprox     uint64 `json:"auditLogsApprox"`
	LoginAttemptsApprox uint64 `json:"loginAttemptsApprox"`
}

// RegisterRoutes mounts /bff/dashboard/* endpoints.
func RegisterRoutes(r chi.Router, s *DashboardState) {
	r.Route("/bff/dashboard", func(r chi.Router) {
		r.Get("/stats", s.stats)
	})
}

func (s *DashboardState) stats(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.IsAdmin(ac); err != nil {
		httperror.Write(w, err)
		return
	}

	exact, err := exactCounts(r.Context(), s.Pool)
	if err != nil {
		httperror.Write(w, usecase.Internal("DB", "exact counts failed", err))
		return
	}
	approx, err := reltuples(r.Context(), s.Pool,
		"msg_events", "msg_dispatch_jobs", "aud_logs", "iam_login_attempts")
	if err != nil {
		httperror.Write(w, usecase.Internal("DB", "approximate counts failed", err))
		return
	}
	resp := DashboardStats{
		TotalClients:        exact.totalClients,
		ActiveUsers:         exact.activeUsers,
		RolesDefined:        exact.rolesDefined,
		EventsApprox:        approx["msg_events"],
		DispatchJobsApprox:  approx["msg_dispatch_jobs"],
		AuditLogsApprox:     approx["aud_logs"],
		LoginAttemptsApprox: approx["iam_login_attempts"],
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

type exactCountsResult struct {
	totalClients uint64
	activeUsers  uint64
	rolesDefined uint64
}

func exactCounts(ctx context.Context, pool *pgxpool.Pool) (exactCountsResult, error) {
	var out exactCountsResult
	row := pool.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM tnt_clients),
		  (SELECT COUNT(*) FROM iam_principals WHERE type = 'USER' AND active = TRUE),
		  (SELECT COUNT(*) FROM iam_roles)
	`)
	if err := row.Scan(&out.totalClients, &out.activeUsers, &out.rolesDefined); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, nil
		}
		return out, err
	}
	return out, nil
}

// reltuples reads pg_class.reltuples for the supplied tables. Returns
// the planner's row estimate — constant-time, accurate to within a few
// percent after ANALYZE. Frontend prefixes a `~` to make the
// approximation explicit.
func reltuples(ctx context.Context, pool *pgxpool.Pool, tables ...string) (map[string]uint64, error) {
	rows, err := pool.Query(ctx,
		`SELECT relname, GREATEST(reltuples, 0)::bigint
		   FROM pg_class WHERE relname = ANY($1) AND relkind = 'r'`,
		tables)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]uint64, len(tables))
	for _, t := range tables {
		out[t] = 0
	}
	for rows.Next() {
		var name string
		var n int64
		if err := rows.Scan(&name, &n); err != nil {
			return nil, err
		}
		if n < 0 {
			n = 0
		}
		out[name] = uint64(n)
	}
	return out, rows.Err()
}
