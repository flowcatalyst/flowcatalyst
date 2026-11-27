package pages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// ── list ───────────────────────────────────────────────────────────────

type djRow struct {
	ID, Href         string
	Descriptor, Code string
	Client           string
	ClientIsID       bool
	Platform         bool
	// CanRequeue and CanRedrive show the row's Requeue (a status the API
	// resets) or Redrive (a COMPLETED job, for holders of redrive-completed).
	CanRequeue, CanRedrive bool
	RedriveAction          string
	Group, GroupHref       string
	Status                 string
	Severity               kit.TagSeverity
	Meta, MetaFull         string
	Created                time.Time
	RequeueAction          string
	ValidID                bool
}

type djView struct {
	Query  djQuery
	Rows   []djRow
	Facets map[string][]kit.Option
	// ClientNames labels the client filter in the confirm summary.
	ClientNames map[string]string
	// OlderHref is the next page (empty on the last).
	OlderHref string
	CanSign   bool
	// CanRequeue shows the requeue actions (dispatch-job:requeue).
	CanRequeue bool
	// CanCancel shows "Ignore selected" (dispatch-job:cancel).
	CanCancel bool
	// CanRedrive adds the redrive of COMPLETED jobs (dispatch-job:redrive-completed
	// on top of the requeue permission).
	CanRedrive bool
	// Window is the created_at window the list covers (the one the query
	// applied, defaulted or not).
	Window dispatchjob.Window
	// WindowText is the line above the table ("Showing the last 24 hours").
	WindowText string
	// EventTypeOptions are the distinct codes seen in the widest window, the
	// event type filter's suggestions.
	EventTypeOptions []kit.Option
	Dialog           templ.Component
	Drawer           *djDetail
}

func (h *DispatchJobs) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, djViewPerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseDJQuery(r.URL.Query()))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := view.patchList(sse, r.Header.Get(djToolbarHeader) == "full"); err != nil {
			slog.DebugContext(ctx, "ui: dispatch jobs patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *DispatchJobs) renderPage(w http.ResponseWriter, r *http.Request, status int, view djView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Dispatch Jobs", Path: djPath}, djPage(view))
}

// patchList patches the table, the pager, the requeue bar and the
// toolbar's live parts (or, with full, the whole toolbar), resets the
// selection and replaces the address.
func (v djView) patchList(sse *ds.SSE, full bool) error {
	errs := []error{
		patchDjTable(sse, v), sse.Patch(kit.CursorPager(v.pager())), sse.Patch(djBulkBar(v)), sse.Patch(djWindowBar(v)),
		sse.PatchSignals(map[string]any{djSelSig: 0}),
	}
	if full {
		errs = append(errs, sse.Patch(kit.TableToolbar(v.toolbar())))
	} else {
		errs = append(errs, sse.Patch(kit.TableToolbarState(v.toolbar().State())))
		for _, f := range djFacets {
			p := v.facetProps(f)
			errs = append(errs, sse.Patch(kit.MultiSelectSummary(p)))
			if f != djFacetClients && f != djFacetStatuses && f != djFacetApps {
				// The cascading facets' options follow the parents' choice.
				errs = append(errs, sse.Patch(kit.MultiSelectOptions(p)))
			}
		}
	}
	errs = append(errs, sse.ReplaceURL(v.Query.URL()))
	return errors.Join(errs...)
}

func (h *DispatchJobs) listView(ctx context.Context, ac *auth.AuthContext, q djQuery) (djView, error) {
	v := djView{
		Query: q, CanSign: auth.CanWritePermission(ac, djRawPerm) == nil && ac.IsAnchor(),
		CanRequeue: auth.CanWritePermission(ac, djRequeuePerm) == nil,
		CanCancel:  auth.CanWritePermission(ac, djCancelPerm) == nil,
	}
	v.CanRedrive = v.CanRequeue && auth.CanWritePermission(ac, djRedrivePerm) == nil
	facets, names, err := h.filterOptions(ctx, ac, q)
	if err != nil {
		return djView{}, err
	}
	v.Facets, v.ClientNames = facets, names
	p, win, err := h.scoped(ctx, ac, q)
	if err != nil {
		return djView{}, err
	}
	v.Window, v.EventTypeOptions = win, facets["eventTypes"]
	v.WindowText = windowText(win, h.now())
	p.Limit, p.SortAscending, p.Cursor = q.Size, q.Dir == kit.SortAsc, q.After
	jobs, err := h.Jobs.FindWithFilters(ctx, p)
	if err != nil {
		return djView{}, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	for i := range jobs {
		row := h.toRow(ctx, q, &jobs[i])
		row.CanRequeue = v.CanRequeue && slices.Contains(dispatchjob.RequeueableStatuses, jobs[i].Status)
		row.CanRedrive = v.CanRedrive && jobs[i].Status == common.DispatchCompleted
		v.Rows = append(v.Rows, row)
	}
	// A full page may have an older one; a short page is the last.
	if len(jobs) == q.Size {
		last := jobs[len(jobs)-1]
		next := q
		next.After = &dispatchjob.ListCursor{CreatedAt: last.CreatedAt, ID: last.ID}
		v.OlderHref = next.URL()
	}
	return v, nil
}

// filterOptions are the SPA's: the clients the caller can see (active,
// sorted by name), the distinct statuses and codes the API's filter-options
// returns (scoped to the caller's tenants, as the list), and (new: the SPA leaves them empty, see docs/ui/README.md) the
// application, subdomain and aggregate segments of those codes, each
// narrowed by the levels chosen above it. A chosen value that is not among
// the options is kept as an option, so a URL never silently loses a filter.
func (h *DispatchJobs) filterOptions(ctx context.Context, ac *auth.AuthContext, q djQuery) (map[string][]kit.Option, map[string]string, error) {
	out := map[string][]kit.Option{}
	cs, err := allClients(ctx, h.Clients)
	if err != nil {
		return nil, nil, err
	}
	names := clientNames(cs)
	out[djFacetClients] = clientFilterOptions(cs, ac)
	// The values are read from the widest window a view may ask for, so every
	// date range the page allows finds its options; the cache keeps the read to
	// one per scope per TTL.
	since := time.Now().Add(-dispatchjob.MaxWindow(h.MaxWindowDays))
	recent := func(ctx context.Context, column string, limit int, scope *[]string) ([]string, error) {
		return h.Jobs.DistinctValues(ctx, column, limit, scope, &since, nil)
	}
	distinct, err := h.facetCache.get(ctx, recent, distinctScope(ac), "status", "code")
	if err != nil {
		return nil, nil, err
	}
	out[djFacetStatuses] = plainOptions(distinct["status"])
	levels := codeLevels(distinct["code"], q.facet(djFacetApps), q.facet(djFacetSubs), q.facet(djFacetAggs))
	for i, f := range []string{djFacetApps, djFacetSubs, djFacetAggs, djFacetCodes} {
		out[f] = plainOptions(levels[i])
	}
	for _, f := range djFacets {
		out[f] = withChosen(out[f], q.facet(f), names)
	}
	// Not a facet: the suggestions of the event type filter (every code seen).
	out["eventTypes"] = plainOptions(distinct["code"])
	return out, names, nil
}

func djURL(id string) string { return djPath + "/" + url.PathEscape(id) }

// djSeverity is the list's statusSeverity (requeueByFilter.ts).
func djSeverity(status string) kit.TagSeverity {
	switch status {
	case "COMPLETED":
		return kit.TagSuccess
	case "PENDING", "QUEUED":
		return kit.TagInfo
	case "PROCESSING":
		return kit.TagWarn
	case "FAILED", "ERROR":
		return kit.TagDanger
	}
	return kit.TagSecondary
}

// metadataPreview is the SPA's: key=value pairs, clipped at 20 characters.
func metadataPreview(meta []dispatchjob.Metadata) (preview, full string) {
	if len(meta) == 0 {
		return "", ""
	}
	pairs, lines := make([]string, 0, len(meta)), make([]string, 0, len(meta))
	for _, m := range meta {
		pairs = append(pairs, m.Key+"="+m.Value)
		lines = append(lines, m.Key+": "+m.Value)
	}
	preview = strings.Join(pairs, ", ")
	if r := []rune(preview); len(r) > 20 {
		preview = string(r[:20]) + "…"
	}
	return preview, strings.Join(lines, "\n")
}

func (h *DispatchJobs) toRow(ctx context.Context, q djQuery, j *dispatchjob.DispatchJob) djRow {
	row := djRow{
		ID: j.ID, Href: djURL(j.ID), Descriptor: deref(j.Descriptor), Code: j.Code,
		Status: string(j.Status), Severity: djSeverity(string(j.Status)), Created: j.CreatedAt,
		RequeueAction: djURL(j.ID) + "/requeue", RedriveAction: djURL(j.ID) + "/redrive", ValidID: kit.ValidRecordID(j.ID),
	}
	row.Meta, row.MetaFull = metadataPreview(j.Metadata)
	switch {
	case j.ClientID == nil || *j.ClientID == "":
		row.Platform = true
	case h.ClientIdentifier != nil:
		if ident, ok := h.ClientIdentifier(ctx, *j.ClientID); ok {
			row.Client = ident
		} else {
			row.Client, row.ClientIsID = *j.ClientID, true
		}
	default:
		row.Client, row.ClientIsID = *j.ClientID, true
	}
	if j.MessageGroup != nil && *j.MessageGroup != "" {
		row.Group = *j.MessageGroup
		row.GroupHref = q.withGroup(row.Group).URL()
	}
	return row
}

// windowText is the line above the table: "Showing the last 24 hours" when
// the window ends about now, the dates otherwise (jobWindow.ts describeWindow).
func windowText(w dispatchjob.Window, now time.Time) string {
	span := w.To.Sub(w.From)
	if d := now.Sub(w.To); d < -5*time.Minute || d > 5*time.Minute {
		return "Showing " + w.From.UTC().Format("2006-01-02 15:04") + " to " + w.To.UTC().Format("2006-01-02 15:04") + " UTC"
	}
	switch hours := int(span.Round(time.Hour) / time.Hour); {
	case span < 90*time.Minute && span >= 30*time.Minute:
		return "Showing the last hour"
	case span < 90*time.Minute:
		return "Showing the last " + strconv.Itoa(max(1, int(span.Round(time.Minute)/time.Minute))) + " minutes"
	case hours >= 72 && hours%24 == 0:
		return "Showing the last " + strconv.Itoa(hours/24) + " days"
	default:
		return "Showing the last " + strconv.Itoa(hours) + " hours"
	}
}

// djWindowChoice is one preset button of the window bar.
type djWindowChoice struct {
	Label, Href string
	Active      bool
}

func (v djView) windowChoices() []djWindowChoice {
	custom := v.Query.From != "" || v.Query.To != ""
	out := make([]djWindowChoice, 0, len(djWindowOrder))
	for _, key := range djWindowOrder {
		label := "Last 24 hours"
		if p, ok := djWindowPresets[key]; ok {
			label = p.label
		}
		out = append(out, djWindowChoice{Label: label, Href: v.Query.windowURL(key), Active: !custom && v.Query.Window == key})
	}
	return out
}

// hidden is the toolbar form's hidden state: the sort, and the window preset
// and quick view (typing in the toolbar must not drop them).
func (v djView) hidden() []kit.HiddenField {
	h := v.Query.hidden()
	if v.Query.Window != "" {
		h = append(h, kit.HiddenField{Name: "window", Value: v.Query.Window})
	}
	if v.Query.Lens != "" {
		h = append(h, kit.HiddenField{Name: "lens", Value: v.Query.Lens})
	}
	return h
}

func djConfirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: djPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}
