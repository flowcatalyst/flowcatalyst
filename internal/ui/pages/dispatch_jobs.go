package pages

import (
	"context"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	djops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/framework/go/ui/kit"
)

// DispatchJobStore is the slice of the dispatch-job repository the page
// reads (the SPA's GET /api/dispatch-jobs, /{id}, /{id}/attempts and
// /filter-options).
type DispatchJobStore interface {
	FindWithFilters(ctx context.Context, p dispatchjob.FilterParams) ([]dispatchjob.DispatchJob, error)
	FindByID(ctx context.Context, id string) (*dispatchjob.DispatchJob, error)
	// ResolvePayloadForView reads the payload of a job opened in the drawer
	// when it is in the payload store, or marks it unavailable (as GET
	// /api/dispatch-jobs/{id} does).
	ResolvePayloadForView(ctx context.Context, j *dispatchjob.DispatchJob)
	// FindByIDs reads the ticked rows' statuses (the requeue dialog counts
	// the COMPLETED ones).
	FindByIDs(ctx context.Context, ids []string) ([]dispatchjob.DispatchJob, error)
	AttemptsByJob(ctx context.Context, jobID string) ([]dispatchjob.Attempt, error)
	DistinctValues(ctx context.Context, column string, limit int, accessibleClientIDs *[]string, since, until *time.Time) ([]string, error)
	// ResolveRefs resolves the by-name subscription and connection filters to
	// ids (the API does the same before its list query; an unresolved name
	// selects nothing).
	ResolveRefs(ctx context.Context, p dispatchjob.FilterParams) (dispatchjob.FilterParams, error)
}

// DispatchJobAudit reads a job's audit rows (the SPA's
// GET /api/audit-logs/entity/Dispatchjob/{id}); *audit.Repository satisfies it.
type DispatchJobAudit interface {
	FindWithFilters(ctx context.Context, p audit.FilterParams) ([]audit.Log, error)
}

// DispatchJobs is the dispatch jobs page (/ui/dispatch-jobs), mirroring the
// SPA's DispatchJobListPage, DispatchJobDetailDrawer and
// RequeueByFilterDialog. Every route makes the API's checks: the
// dispatch-job view permission, SQL tenant scoping (operations.VisibleTo)
// and per-job client scope; "sign" needs the raw-view permission.
type DispatchJobs struct {
	Jobs    DispatchJobStore
	Ops     DispatchJobCommands
	Clients ClientStore
	// Apps confines an application-scoped caller's sign action to its own
	// applications' jobs (the API's State.Apps). nil refuses such a caller.
	Apps *application.Repository
	// Subs and Connections name the drawer's subscription and connection, as
	// the SPA does best effort (a caller without their view permissions sees
	// the bare id); Audit fills the drawer's Audit section for callers with
	// the audit-log view permission. Any may be nil.
	Subs        SubscriptionStore
	Connections ConnectionStore
	Audit       DispatchJobAudit
	// ClientIdentifier resolves a job's client id to the client's
	// identifier for the grid (the API's cached resolver); nil shows ids.
	ClientIdentifier func(ctx context.Context, clientID string) (string, bool)
	Shell            *shell.Shell
	// MaxWindowDays is the widest created_at window a list or a requeue may
	// cover (FC_DISPATCH_JOB_MAX_WINDOW_DAYS; < 1: the default, 31). A view with
	// no dates is held to the last 24 hours, as the API's list is.
	MaxWindowDays int
	// Now is the clock the time window is cut from (nil: time.Now).
	Now func() time.Time
	// PayloadFormBytes caps the payload edit's form, which carries one
	// payload up to the largest the deployment accepts: the API edit's
	// request cap (payloadstore.Service.PayloadRequestMaxBytes). < 1: the cap
	// of every other form (maxFormBytes).
	PayloadFormBytes int64

	// facetCache holds the distinct statuses and codes (firehose.go): the
	// SPA reads them once per visit, the live list on every request, and a
	// DISTINCT over the projection is the costliest query of a request.
	facetCache distinctCache
}

// window holds p to the created_at window every list-shaped read carries (the
// API's rule: the last 24 hours when the view names no dates, at most
// MaxWindowDays).
func (h *DispatchJobs) window(p dispatchjob.FilterParams) (dispatchjob.FilterParams, dispatchjob.Window, error) {
	return dispatchjob.EnforceWindow(p, h.now(), h.MaxWindowDays)
}

func (h *DispatchJobs) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// scoped is the filter a read runs on, in the API's order: the view's filter
// (a window preset becomes its since), scoped to the caller's tenants, held
// to the time window, and its by-name subscription and connection references
// resolved to ids. A view that cannot be run (too many event types) is a 400.
func (h *DispatchJobs) scoped(ctx context.Context, ac *auth.AuthContext, q djQuery) (dispatchjob.FilterParams, dispatchjob.Window, error) {
	if err := q.validate(); err != nil {
		return dispatchjob.FilterParams{}, dispatchjob.Window{}, err
	}
	p := q.filterParams()
	if preset, ok := djWindowPresets[q.Window]; ok && p.Since == nil && p.Until == nil {
		since := h.now().Add(-preset.d)
		p.Since = &since
	}
	if q.stuckLens() {
		// Nothing newer than five minutes; the cap only ever lowers the bound.
		if cap := h.now().Add(-djStuckAfter); p.Until == nil || cap.Before(*p.Until) {
			p.Until = &cap
		}
	}
	p, win, err := h.window(djops.VisibleTo(ac, p))
	if err != nil {
		return p, win, err
	}
	p, err = h.Jobs.ResolveRefs(ctx, p)
	return p, win, err
}

const (
	djPath   = "/ui/dispatch-jobs"
	djFormID = "dj-list"

	djDrawerID   = "dj-drawer"
	djDrawerSig  = "selDj"
	djDialogSlot = "dj-dialog-slot"
	djRBFDialog  = "dj-rbf"
	// The drawer's confirm dialogs; their form action comes from the button.
	djCancelDialog = "dj-cancel"
	// djCancelInFlightDialog is the Cancel confirmation of a job in flight;
	// djRequeueInFlightDialog the Requeue confirmation of one.
	djCancelInFlightDialog  = "dj-cancel-inflight"
	djRequeueInFlightDialog = "dj-requeue-inflight"
	djCompleteDialog        = "dj-complete"
	// djSelDialog is the requeue dialog of ticked rows that include COMPLETED
	// jobs (the SPA's RequeueSelectedDialog).
	djSelDialog = "dj-sel"
	// djIgnoreDialog is the confirmation of "Ignore selected".
	djIgnoreDialog = "dj-ignore"

	// djSelSig counts the ticked rows (the "Requeue selected" label).
	djSelSig = "djSel"

	djViewPerm        = "platform:messaging:dispatch-job:view"
	djRawPerm         = "platform:messaging:dispatch-job:view-raw"
	djRequeuePerm     = auth.DispatchJobRequeuePerm
	djEditPayloadPerm = auth.DispatchJobEditPayloadPerm
	djCancelPerm      = auth.DispatchJobCancelPerm
	djCompletePerm    = auth.DispatchJobCompletePerm
	djRedrivePerm     = auth.DispatchJobRedriveCompletedPerm
	djAuditPerm       = "platform:admin:audit-log:view"
	// djAuditEntity is the audit entity type of a dispatch job: its domain
	// events use the subject platform.dispatchjob.<id>, which the audit
	// writer capitalises.
	djAuditEntity = "Dispatchjob"

	djLiveGet = "@get('/ui/dispatch-jobs', {contentType: 'form'})"
	// djNavLink is a link that changes the filters (the group link, the
	// error preset): its answer re-renders the whole toolbar, which a typed
	// search must never be (it would drop keystrokes made in flight).
	djNavLink = "@get(el.href, {headers: {'" + djToolbarHeader + "': 'full'}})"

	djToolbarHeader = "X-FC-Toolbar"

	// Facets: the SPA's URL keys.
	djFacetClients  = "clients"
	djFacetApps     = "applications"
	djFacetSubs     = "subdomains"
	djFacetAggs     = "aggregates"
	djFacetCodes    = "codes"
	djFacetStatuses = "statuses"

	// djStuckAfter is how old a PENDING or QUEUED job must be to count as
	// stuck (jobFilters.ts STUCK_AFTER_MS).
	djStuckAfter = 5 * time.Minute
	// djLensStuck is the lens parameter's one value.
	djLensStuck = "stuck"

	djDefaultSize = 200
	// djMaxRequeue is ResendByFilterMaxLimit: one requeue-by-filter run's cap.
	djMaxRequeue = djops.ResendByFilterMaxLimit
)

var (
	// djWindowPresets are the window picker's presets (jobWindow.ts). The
	// default (24 hours) is the absence of a preset: the window then comes
	// from the server and is shown as defaulted.
	djWindowPresets = map[string]djWindowPreset{
		"1h":  {"Last hour", time.Hour},
		"3d":  {"Last 3 days", 3 * 24 * time.Hour},
		"7d":  {"Last 7 days", 7 * 24 * time.Hour},
		"31d": {"Last 31 days", 31 * 24 * time.Hour},
	}
	djWindowOrder = []string{"1h", "", "3d", "7d", "31d"}

	djFacets = []string{djFacetClients, djFacetApps, djFacetSubs, djFacetAggs, djFacetCodes, djFacetStatuses}
	djSizes  = []int{50, 100, 200, 500, 1000}

	djList = &listSpec{
		path: djPath, sortKeys: []string{"createdAt"}, defaultDir: kit.SortDesc,
		statusOK: func(string) bool { return false }, facets: djFacets,
	}

	// djErrorStatuses are the "in error" statuses: FAILED and its legacy
	// alias ERROR (requeueByFilter.ts ERROR_STATUSES).
	djErrorStatuses = []string{"FAILED", "ERROR"}
	// djStuckStatuses are the statuses of the Stuck quick view.
	djStuckStatuses = []string{"PENDING", "QUEUED"}
)

type djWindowPreset struct {
	label string
	d     time.Duration
}

// djRedriveWord is the literal a redrive's second step asks to be typed (and
// the API's confirmRedriveCompleted).
const djRedriveWord = dispatchjob.RedriveConfirmation

// djStep sets a dialog form's step and submits it (the footer's buttons sit
// outside the form; both ids are constants).
func djStep(formID, step string) string {
	return "document.getElementById('" + kit.ID(formID) + "').elements.step.value = '" + kit.ID(step) + "'; document.getElementById('" + kit.ID(formID) + "').requestSubmit()"
}

// djMaxEventTypes is the API's cap on eventTypes (TOO_MANY_EVENT_TYPES).
const djMaxEventTypes = 10

// Mount registers the page's routes (POSTs behind parseForm).
func (h *DispatchJobs) Mount(r chi.Router) {
	r.Get("/dispatch-jobs", h.list)
	r.Get("/dispatch-jobs/requeue-by-filter", h.requeueByFilterDialog)
	r.Get("/dispatch-jobs/requeue-errors", h.requeueErrors)
	r.Get("/dispatch-jobs/{id}", h.detail)
	r.Get("/dispatch-jobs/{id}/audit", h.auditSection)
	post := r.With(parseForm)
	post.Post("/dispatch-jobs/requeue", h.requeueSelected)
	post.Post("/dispatch-jobs/cancel", h.cancelSelected)
	post.Post("/dispatch-jobs/requeue-by-filter", h.requeueByFilter)
	post.Post("/dispatch-jobs/{id}/requeue", h.requeueOne)
	post.Post("/dispatch-jobs/{id}/redrive", h.redriveOne)
	post.Post("/dispatch-jobs/{id}/sign", h.sign)
	post.Post("/dispatch-jobs/{id}/cancel", h.cancel)
	post.Post("/dispatch-jobs/{id}/complete", h.complete)
	r.With(parseFormUpTo(h.PayloadFormBytes)).Post("/dispatch-jobs/{id}/payload", h.editPayload)
}
