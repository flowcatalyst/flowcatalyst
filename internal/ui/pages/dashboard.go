package pages

import (
	"context"
	"log/slog"
	"maps"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/bff"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// DashboardStatsStore reads the dashboard's counts (GET /bff/dashboard/stats).
type DashboardStatsStore interface {
	Stats(ctx context.Context) (bff.DashboardStats, error)
}

// Dashboard is the platform dashboard (/ui/dashboard), mirroring the SPA's
// DashboardPage: the quick links, the Platform Sync section (event types,
// roles, OpenAPI, and all of them at once), the Platform Overview counts and
// the approximate Message Plane counts.
//
// Who sees it is the SPA's rule (route permission platform:*:*:*, anchor
// scope: the sidebar's Dashboard entry) plus the stats endpoint's own check
// (CanViewDashboardStats). Each sync makes the check of its API endpoint.
// The counts are the BFF's: exact over the bounded control-plane tables, the
// planner's estimate over the high-volume ones, so the page costs the same
// whatever the table sizes.
type Dashboard struct {
	Stats      DashboardStatsStore
	EventTypes EventTypeCommands
	Roles      RolesCommands
	OpenAPI    OpenAPISyncer
	Shell      *shell.Shell
}

const (
	dashPath = "/ui/dashboard"

	// dashRoutePermission is the SPA's ROUTE_PERMISSIONS entry for /dashboard.
	dashRoutePermission = "platform:*:*:*"

	dashRegionID = "dash-sync"

	flashDashSynced = "dashboard-synced"
)

func init() {
	maps.Copy(FlashMessages, map[string]kit.FlashMessageProps{
		flashDashSynced: {Kind: kit.FlashSuccess, Summary: "Platform Synced", Detail: "The platform's definitions are up to date."},
	})
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *Dashboard) Mount(r chi.Router) {
	r.Get("/dashboard", h.page)
	post := r.With(parseForm)
	post.Post("/dashboard/sync/event-types", h.syncEventTypes)
	post.Post("/dashboard/sync/roles", h.syncRoles)
	post.Post("/dashboard/sync/openapi", h.syncOpenAPI)
	post.Post("/dashboard/sync", h.syncAll)
}

// dashAccess is who may open the dashboard: anchor scope holding the route
// permission (the sidebar entry's rule), and what the stats endpoint asks.
func dashAccess(ac *auth.AuthContext) error {
	if err := auth.RequireAnchor(ac); err != nil {
		return err
	}
	if !ac.HasPermission(dashRoutePermission) {
		return usecase.Authorization("FORBIDDEN", "You do not have access to the dashboard.")
	}
	return auth.CanViewDashboardStats(ac)
}

// The three syncs and the check each makes: the event-type sync and the
// OpenAPI sync those of their endpoints, the role sync anchor scope and all
// three role permissions (it creates, updates and removes roles).
func (h *Dashboard) canSyncEvents(ac *auth.AuthContext) bool {
	return auth.CanSyncPlatformEventTypes(ac) == nil
}

func (h *Dashboard) canSyncRoles(ac *auth.AuthContext) bool {
	return rolesWrite(ac, auth.CanCreateRoles, auth.CanUpdateRoles, auth.CanDeleteRoles) == nil
}

func (h *Dashboard) canSyncOpenAPI(ac *auth.AuthContext) bool {
	return h.OpenAPI != nil && auth.CanSyncPlatformOpenAPI(ac) == nil
}

type dashCard struct {
	Title, Description, Href, Icon string
}

// dashCards are the SPA's quick links, each to the /ui page.
var dashCards = []dashCard{
	{"Applications", "Manage applications in the platform ecosystem", "/ui/applications", "layout-grid"},
	{"Clients", "Manage clients and their configurations", "/ui/clients", "building-2"},
	{"Users", "Manage platform users and their access", "/ui/users", "users"},
	{"Roles", "Configure roles and permissions", "/ui/roles", "shield"},
	{"Event Types", "Define event types and schemas for messaging", "/ui/event-types", "zap"},
	{"Subscriptions", "Manage event subscriptions and routing", "/ui/subscriptions", "bell"},
}

type dashStat struct {
	Label, Value string
}

type dashView struct {
	Name string
	// The sync cards offered to the caller.
	Events, Roles, OpenAPI bool
	// Overview and Plane are the counts; empty when they could not be read
	// (the SPA shows a dash).
	Overview, Plane []dashStat
	StatsFailed     bool
}

func (v dashView) AnySync() bool { return v.Events || v.Roles || v.OpenAPI }

func (h *Dashboard) view(r *http.Request, ac *auth.AuthContext) dashView {
	ctx := r.Context()
	v := dashView{
		Name:    h.Shell.UserName(ctx, ac),
		Events:  h.canSyncEvents(ac),
		Roles:   h.canSyncRoles(ac),
		OpenAPI: h.canSyncOpenAPI(ac),
	}
	var s *bff.DashboardStats
	if h.Stats != nil {
		if got, err := h.Stats.Stats(ctx); err != nil {
			slog.ErrorContext(ctx, "ui: dashboard counts not read", "err", err)
		} else {
			s = &got
		}
	}
	v.StatsFailed = s == nil
	exact := func(label string, pick func(bff.DashboardStats) uint64) dashStat {
		return dashStat{label, dashCount(s, "", pick)}
	}
	approx := func(label string, pick func(bff.DashboardStats) uint64) dashStat {
		return dashStat{label, dashCount(s, "~", pick)}
	}
	v.Overview = []dashStat{
		exact("Total Clients", func(s bff.DashboardStats) uint64 { return s.TotalClients }),
		exact("Active Users", func(s bff.DashboardStats) uint64 { return s.ActiveUsers }),
		exact("Roles Defined", func(s bff.DashboardStats) uint64 { return s.RolesDefined }),
	}
	v.Plane = []dashStat{
		approx("Events", func(s bff.DashboardStats) uint64 { return s.EventsApprox }),
		approx("Dispatch Jobs", func(s bff.DashboardStats) uint64 { return s.DispatchJobsApprox }),
		approx("Audit Logs", func(s bff.DashboardStats) uint64 { return s.AuditLogsApprox }),
		approx("Login Attempts", func(s bff.DashboardStats) uint64 { return s.LoginAttemptsApprox }),
	}
	return v
}

// dashCount formats a count with thousands separators ("—" when the counts
// could not be read), prefixed (the SPA's "~") when it is an estimate.
func dashCount(s *bff.DashboardStats, prefix string, pick func(bff.DashboardStats) uint64) string {
	if s == nil {
		return "—"
	}
	return prefix + groupThousands(strconv.FormatUint(pick(*s), 10))
}

func groupThousands(digits string) string {
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func (h *Dashboard) page(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := dashAccess(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.Shell.Render(w, r, shell.Page{Title: "Dashboard", Path: dashPath}, dashPage(h.view(r, ac)))
}

// ── syncs ──────────────────────────────────────────────────────────────

// dashAction is one sync: its check, its run and its toast, as the SPA's.
type dashAction struct {
	allowed func(*auth.AuthContext) bool
	// check is the endpoint's own error for a caller it refuses.
	check func(*auth.AuthContext) error
	run   func(ctx context.Context) (kit.FlashMessageProps, error)
	fail  string
}

func (h *Dashboard) eventsSync() dashAction {
	return dashAction{
		allowed: h.canSyncEvents,
		check:   auth.CanSyncPlatformEventTypes,
		fail:    "Platform Events Sync Failed",
		run: func(ctx context.Context) (kit.FlashMessageProps, error) {
			res, err := h.EventTypes.SyncPlatform(ctx)
			if err != nil {
				return kit.FlashMessageProps{}, err
			}
			return kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: "Platform Events Synced", Detail: syncDetail(res)}, nil
		},
	}
}

func (h *Dashboard) rolesSync() dashAction {
	return dashAction{
		allowed: h.canSyncRoles,
		check: func(ac *auth.AuthContext) error {
			return rolesWrite(ac, auth.CanCreateRoles, auth.CanUpdateRoles, auth.CanDeleteRoles)
		},
		fail: "Platform Roles Sync Failed",
		run: func(ctx context.Context) (kit.FlashMessageProps, error) {
			res, err := h.Roles.SyncPlatform(ctx)
			if err != nil {
				return kit.FlashMessageProps{}, err
			}
			return kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: "Platform Roles Synced", Detail: rolesSyncDetail(res)}, nil
		},
	}
}

func (h *Dashboard) openAPISync() dashAction {
	return dashAction{
		allowed: h.canSyncOpenAPI,
		check:   auth.CanSyncPlatformOpenAPI,
		fail:    "Platform OpenAPI Sync Failed",
		run: func(ctx context.Context) (kit.FlashMessageProps, error) {
			res, err := h.OpenAPI.SyncPlatformOpenAPI(ctx)
			if err != nil {
				return kit.FlashMessageProps{}, err
			}
			return kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: "Platform OpenAPI Synced", Detail: openAPISyncDetail(res)}, nil
		},
	}
}

// openAPISyncDetail is the SPA's toast text for an OpenAPI sync.
func openAPISyncDetail(r OpenAPISyncResult) string {
	switch {
	case r.Unchanged:
		return "No changes (v" + r.Version + " unchanged)"
	case r.ArchivedPriorVersion != "":
		breaking := ""
		if r.HasBreaking {
			breaking = " · breaking"
		}
		return "v" + r.ArchivedPriorVersion + " → v" + r.Version + breaking
	}
	return "Published initial version v" + r.Version
}

func (h *Dashboard) syncEventTypes(w http.ResponseWriter, r *http.Request) {
	h.syncOne(w, r, h.eventsSync(), flashETSynced)
}

func (h *Dashboard) syncRoles(w http.ResponseWriter, r *http.Request) {
	h.syncOne(w, r, h.rolesSync(), flashRolesSynced)
}

func (h *Dashboard) syncOpenAPI(w http.ResponseWriter, r *http.Request) {
	h.syncOne(w, r, h.openAPISync(), flashDashSynced)
}

// syncOne runs one sync for a caller its endpoint accepts, and answers with
// the SPA's toast.
func (h *Dashboard) syncOne(w http.ResponseWriter, r *http.Request, s dashAction, nativeFlash string) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := dashAccess(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := s.check(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !s.allowed(ac) { // the sync is not wired
		h.Shell.Fail(w, r, usecase.NotFound("SYNC_UNAVAILABLE", "This sync is not available."))
		return
	}
	msg, err := s.run(ctx)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, nativeFlash)
		http.Redirect(w, r, dashPath, http.StatusSeeOther)
		return
	}
	if err := shell.Flash(ds.NewSSE(w, r), msg); err != nil {
		slog.DebugContext(ctx, "ui: sync flash not delivered", "err", err)
	}
}

// syncAll is "Sync All": every sync the caller is offered, each reporting its
// own toast; one failing does not stop the others (the SPA runs them side by
// side, each toasting its own error).
func (h *Dashboard) syncAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := dashAccess(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	var msgs []kit.FlashMessageProps
	var firstErr error
	for _, s := range []dashAction{h.eventsSync(), h.rolesSync(), h.openAPISync()} {
		if !s.allowed(ac) {
			continue
		}
		msg, err := s.run(ctx)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			_, text := shell.Classify(r, err)
			msg = kit.FlashMessageProps{Kind: kit.FlashError, Summary: s.fail, Detail: text}
		}
		msgs = append(msgs, msg)
	}
	if !ds.IsDatastar(r) {
		if firstErr != nil {
			h.Shell.Fail(w, r, firstErr)
			return
		}
		h.Shell.SetFlash(w, flashDashSynced)
		http.Redirect(w, r, dashPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	for _, msg := range msgs {
		if err := shell.Flash(sse, msg); err != nil {
			slog.DebugContext(ctx, "ui: sync flash not delivered", "err", err)
		}
	}
}
