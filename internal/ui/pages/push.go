package pages

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/pushgw"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/sdksync"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// The Push pages (docs/push-gateway-spec.md section 11, "Console"): the push gateway's
// applications with their synced rules and keys and the operator's limits, the live streams, the
// recent pushes with the trace window that shows their data, and the test send. Every page reads
// the platform database (the gateway's tables), so the console works whether or not the gateway
// runs in this process; only the test send needs the gateway itself.

// Routes of the Push pages.
const (
	pushPath       = "/ui/push"
	pushAppsPath   = pushPath + "/applications"
	pushLivePath   = pushPath + "/live"
	pushRecentPath = pushPath + "/recent"
	pushTestPath   = pushPath + "/test"
)

// PushConfigStore reads the gateway's per-application configuration (pushgw.ConfigRepo).
type PushConfigStore interface {
	ListConfigured(ctx context.Context) ([]pushgw.ConfiguredApp, error)
	RuleDetails(ctx context.Context, applicationID string) ([]pushgw.RuleDetail, error)
	KeyDetails(ctx context.Context, applicationID string) ([]pushgw.KeyDetail, error)
}

// PushSettingsCommands reads and edits an application's push settings (sdksync.PushSettingsOps:
// the API handler's own code, so anchor scope, manage permission, validation and the audit record
// are its).
type PushSettingsCommands interface {
	Read(ctx context.Context, appID, appCode string) (sdksync.PushSettingsResponse, error)
	Update(ctx context.Context, ac *auth.AuthContext, appID, appCode string, next sdksync.PushSettings) (sdksync.PushSettingsResponse, error)
}

// PushApplications finds an application by its code.
type PushApplications interface {
	FindByCode(ctx context.Context, code string) (*application.Application, error)
}

// PushStats reads the gateway tasks' stats rows, summed (pushgw.StatsRepo).
type PushStats interface {
	Live(ctx context.Context, now time.Time) (pushgw.Live, error)
}

// PushTraces stores the recent-pushes trace windows (pushgw.TraceRepo).
type PushTraces interface {
	Get(ctx context.Context, applicationID string) (*pushgw.TraceWindow, error)
	Enable(ctx context.Context, applicationID string, secs int, by string, now time.Time) (*pushgw.TraceWindow, error)
	Disable(ctx context.Context, applicationID string, now time.Time) (bool, error)
}

// PushPublisher sends the test push through the gateway's publish path. Available is false when
// the gateway does not run in this process.
type PushPublisher interface {
	Available() bool
	PublishTest(ctx context.Context, topic, clientID string) error
}

// PushAudit records an operator's push action against an application.
type PushAudit func(ctx context.Context, ac *auth.AuthContext, appID, operation string, detail map[string]any)

// PushConsole is the Push pages (/ui/push/...). The permission rules: every page needs the
// anchor scope and platform:admin:push:view (manage implies it); editing the limits, the pause,
// the trace window and the test send need platform:admin:push:manage, as the settings API does.
type PushConsole struct {
	Config    PushConfigStore
	Settings  PushSettingsCommands
	Apps      PushApplications
	Clients   ClientStore
	Stats     PushStats
	Log       pushgw.LogReader // nil: the log cannot be read from this process
	Traces    PushTraces
	Publisher PushPublisher
	Audit     PushAudit
	Now       func() time.Time // tests; nil is the system clock
	Shell     *shell.Shell
}

func (h *PushConsole) now() time.Time {
	if h.Now != nil {
		return h.Now().UTC()
	}
	return time.Now().UTC()
}

// Mount registers the pages' routes (POSTs behind parseForm).
func (h *PushConsole) Mount(r chi.Router) {
	r.Get("/push", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, pushAppsPath, http.StatusFound) })
	r.Get("/push/applications", h.appsList)
	r.Get("/push/applications/{code}", h.appDetail)
	post := r.With(parseForm)
	post.Post("/push/applications/{code}/limits", h.saveLimits)
	post.Post("/push/applications/{code}/pause", h.setPaused)
	r.Get("/push/live", h.live)
	r.Get("/push/recent", h.recent)
	post.Post("/push/recent/trace/enable", h.traceEnable)
	post.Post("/push/recent/trace/disable", h.traceDisable)
	r.Get("/push/test", h.testPage)
	post.Post("/push/test", h.testSend)
}

// pushTab is one of the four pages, for the tab links at the top of each.
type pushTab struct{ Label, Href, Key string }

func pushTabs(manage bool) []pushTab {
	tabs := []pushTab{
		{"Applications", pushAppsPath, "applications"},
		{"Live", pushLivePath, "live"},
		{"Recent pushes", pushRecentPath, "recent"},
	}
	if manage {
		tabs = append(tabs, pushTab{"Test send", pushTestPath, "test"})
	}
	return tabs
}

// pushGates are what the caller may do beyond reading.
type pushGates struct{ Manage bool }

func pushGatesFor(ac *auth.AuthContext) pushGates {
	return pushGates{Manage: auth.CanManagePush(ac) == nil}
}

// ── small shared helpers ───────────────────────────────────────────────

func pushAppURL(code string) string { return pushAppsPath + "/" + url.PathEscape(code) }

// pushAge renders how long ago t was, coarsely ("12 s ago", "3 min ago"), "never" for nil.
func pushAge(t *time.Time, now time.Time) string {
	if t == nil || t.IsZero() {
		return "never"
	}
	d := max(now.Sub(*t), 0)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d s ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	}
	return fmt.Sprintf("%d days ago", int(d.Hours()/24))
}

// pushSeconds formats a latency in seconds: milliseconds below a second.
func pushSeconds(s float64, ok bool) string {
	switch {
	case !ok:
		return "-"
	case s < 1:
		return strconv.FormatFloat(s*1000, 'f', 0, 64) + " ms"
	}
	return strconv.FormatFloat(s, 'f', 1, 64) + " s"
}

// pushNumber formats a count with thousands separators.
func pushNumber(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return s
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func pushClientLabel(names map[string]string, id string) string {
	switch id {
	case "", "_platform":
		return "Platform"
	}
	return clientLabel(names, id)
}

func pushClientNames(ctx context.Context, store ClientStore) map[string]string {
	if store == nil {
		return nil
	}
	cs, err := store.FindAll(ctx)
	if err != nil {
		return nil
	}
	return clientNames(cs)
}

// pushTabsNav is the tab links component.
func pushTabsNav(active string, manage bool) templ.Component {
	return pushTabsC(active, pushTabs(manage))
}
