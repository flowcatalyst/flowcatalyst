package pages

import (
	"cmp"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// The ingestion drawer's Activity, Failures and History panels (the Nuxt
// control plane's IngestionDrawer sections, IngestionFailures and
// IngestionHistory). The drawer embeds each as a lazy placeholder
// (IngestionActivityTab and its siblings) that asks for its panel with a
// Datastar GET, so a tab costs nothing until it is opened.

// IngestionTabStore is what the Activity and Failures panels read;
// *ingestion.Repository satisfies it.
type IngestionTabStore interface {
	FindByID(ctx context.Context, id string) (*ingestion.Ingestion, error)
	ListRouteStates(ctx context.Context, ingestionID string) ([]ingestion.RouteState, error)
	ActivityOf(ctx context.Context, ingestionID string, now time.Time) (*ingestion.Activity, error)
	ListFailures(ctx context.Context, ingestionID, after string, limit int) (*ingestion.FailurePage, error)
}

// IngestionAudit reads an ingestion's audit rows (GET
// /api/ingestions/{id}/history); *audit.Repository satisfies it.
type IngestionAudit interface {
	FindWithFilters(ctx context.Context, p audit.FilterParams) ([]audit.Log, error)
}

// IngestionTabs serves the three panels. Every route makes the API handler's
// checks: the ingestion view permission, and the ingestion's reach (client
// and application, ingestion.CheckReach).
type IngestionTabs struct {
	Store IngestionTabStore
	Audit IngestionAudit
	Shell *shell.Shell

	// now is the clock of the tests; nil is the wall clock.
	now func() time.Time
}

const (
	ingTabBase = "/ui/ingestions"

	ingActivityID   = "ing-activity"
	ingFailuresID   = "ing-failures"
	ingHistoryID    = "ing-history"
	ingFailuresList = "ing-failures-list"
	ingHistoryList  = "ing-history-list"

	// The SPA's page sizes: 20 failures, and a history long enough for a
	// change log.
	ingFailuresPage = 20
	ingHistoryPage  = 20
)

func (h *IngestionTabs) clock() time.Time {
	if h.now != nil {
		return h.now()
	}
	return time.Now().UTC()
}

// Mount registers the three panels' routes.
func (h *IngestionTabs) Mount(r chi.Router) {
	r.Get("/ingestions/{id}/activity", h.activity)
	r.Get("/ingestions/{id}/failures", h.failures)
	r.Get("/ingestions/{id}/history", h.history)
}

func ingTabURL(id string) string { return ingTabBase + "/" + id }

// load makes the by-id checks of GET /api/ingestions/{id}/...: the view
// permission, the id, the ingestion and its reach. A panel is a fragment: a
// navigation to it goes to the ingestion's own page instead. ok is false when
// the request has been answered.
func (h *IngestionTabs) load(w http.ResponseWriter, r *http.Request) (*ingestion.Ingestion, bool) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadIngestions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return nil, false
	}
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		h.Shell.Fail(w, r, httperror.NotFound("Ingestion", id))
		return nil, false
	}
	i, err := h.Store.FindByID(ctx, id)
	if err == nil && i == nil {
		err = httperror.NotFound("Ingestion", id)
	} else if err != nil {
		err = usecase.Internal("REPO", "find_by_id failed", err)
	}
	if err == nil {
		err = ingestion.CheckReach(ac, i)
	}
	if err != nil {
		h.Shell.Fail(w, r, err)
		return nil, false
	}
	if !ds.IsDatastar(r) {
		http.Redirect(w, r, ingTabURL(id), http.StatusSeeOther) //nolint:gosec // G710: id is a validated element id
		return nil, false
	}
	return i, true
}

// ── activity ───────────────────────────────────────────────────────────

type ingRouteRow struct {
	Runtime, Status string
	Severity        kit.TagSeverity
	Version         int
	// Stale is a route running another version than the live one.
	Stale    bool
	Reported time.Time
	Error    string
}

type ingActivityView struct {
	Routes                    []ingRouteRow
	Day, Week                 string
	LastExchange, LastFailure time.Time
}

func ingRouteSeverity(status string) kit.TagSeverity {
	switch status {
	case "STARTED":
		return kit.TagSuccess
	case "FAILING":
		return kit.TagWarn
	case "FAILED":
		return kit.TagDanger
	}
	return kit.TagSecondary
}

// ingCounts is a period's exchanges in one line.
func ingCounts(e ingestion.Exchanges) string {
	return strconv.FormatInt(e.Completed, 10) + " completed, " + strconv.FormatInt(e.Failed, 10) + " failed, " +
		strconv.FormatInt(e.Records, 10) + " records, " + strconv.FormatInt(e.Jobs, 10) + " jobs"
}

func (h *IngestionTabs) activity(w http.ResponseWriter, r *http.Request) {
	i, ok := h.load(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	routes, err := h.Store.ListRouteStates(ctx, i.ID)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "list_route_states failed", err))
		return
	}
	a, err := h.Store.ActivityOf(ctx, i.ID, h.clock())
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "activity failed", err))
		return
	}
	v := ingActivityView{Day: ingCounts(a.Day), Week: ingCounts(a.Week)}
	for _, rs := range routes {
		row := ingRouteRow{
			Runtime: rs.RuntimeID, Status: rs.Status, Severity: ingRouteSeverity(rs.Status), Version: rs.Version,
			Stale: rs.Version != i.Version, Reported: rs.UpdatedAt, Error: deref(rs.Error),
		}
		v.Routes = append(v.Routes, row)
	}
	if a.LastExchange != nil {
		v.LastExchange = *a.LastExchange
	}
	if a.LastFailure != nil {
		v.LastFailure = *a.LastFailure
	}
	if err := ds.NewSSE(w, r).Patch(ingActivityPanel(v)); err != nil {
		slog.DebugContext(ctx, "ui: ingestion activity patch not delivered", "err", err)
	}
}

// ── failures ───────────────────────────────────────────────────────────

// ingFailureLabels is what each kind of failure means, for people (the Nuxt
// control plane's FAILURE_LABELS).
var ingFailureLabels = map[string]string{
	"REJECTED":       "Payload rejected",
	"NOT_DISPATCHED": "Not dispatched",
	"SOURCE":         "Source error",
	"AUTH":           "Login refused",
}

var ingFailureSeverity = map[string]kit.TagSeverity{
	"REJECTED":       kit.TagDanger,
	"NOT_DISPATCHED": kit.TagWarn,
	"SOURCE":         kit.TagWarn,
	"AUTH":           kit.TagDanger,
}

type ingFailureRow struct {
	ID, Kind, Message, Runtime string
	Severity                   kit.TagSeverity
	Version                    string
	At                         time.Time
	After                      string
	// Excerpt is the start of the payload: only for an ingestion that keeps
	// failed payloads.
	Excerpt string
	// Notice: a rejected failure (the platform refused the runtime's jobs)
	// nobody has dismissed yet.
	Notice bool
	// Dismissed says who dismissed it, "" while nobody has; DismissedAt when.
	Dismissed   string
	DismissedAt time.Time
	// DismissAction is the dismiss route, "" when the viewer may not dismiss
	// it or it is dismissed already.
	DismissAction string
}

type ingFailuresView struct {
	ID       string
	Keep     bool
	Rows     []ingFailureRow
	Next     string
	Appended bool
}

func (v ingFailuresView) moreURL() string {
	return ingTabURL(v.ID) + "/failures?after=" + v.Next
}

// ingFailureRows shapes the failures; canDismiss offers Dismiss on the ones
// nobody has dismissed (the update permission).
func ingFailureRows(i *ingestion.Ingestion, fs []ingestion.Failure, canDismiss bool) []ingFailureRow {
	rows := make([]ingFailureRow, 0, len(fs))
	for _, f := range fs {
		row := ingFailureRow{
			ID: f.ID, Kind: f.Kind, Message: f.Message, Runtime: deref(f.RuntimeID), At: f.OccurredAt,
			Severity: ingFailureSeverity[f.Kind], Notice: f.Notice(),
		}
		if f.Dismissed() {
			row.Dismissed, row.DismissedAt = cmp.Or(deref(f.DismissedBy), "someone"), *f.DismissedAt
		} else if canDismiss {
			row.DismissAction = ingsURL(i.ID) + "/failures/" + url.PathEscape(f.ID) + "/dismiss"
		}
		if label, known := ingFailureLabels[f.Kind]; known {
			row.Kind = label
		}
		if f.Version != nil {
			row.Version = strconv.Itoa(*f.Version)
		}
		if f.AfterRecords > 0 {
			row.After = "After " + strconv.FormatInt(f.AfterRecords, 10) + " record" + ingPlural(f.AfterRecords) + " of the payload."
		}
		// A payload kept before keeping was switched off is not shown.
		if i.Definition.KeepFailedPayloads && f.Excerpt != nil {
			row.Excerpt = *f.Excerpt
		}
		rows = append(rows, row)
	}
	return rows
}

func ingInt(n int) string { return strconv.Itoa(n) }

func ingPlural(n int64) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func (h *IngestionTabs) failures(w http.ResponseWriter, r *http.Request) {
	i, ok := h.load(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	after := r.URL.Query().Get("after")
	if after != "" && !kit.ValidID(after) {
		h.Shell.Fail(w, r, usecase.Validation("CURSOR_INVALID", "after is not a cursor this list gave"))
		return
	}
	page, err := h.Store.ListFailures(ctx, i.ID, after, ingFailuresPage)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "list_failures failed", err))
		return
	}
	canDismiss := auth.CanUpdateIngestions(auth.FromContext(ctx)) == nil
	v := ingFailuresView{ID: i.ID, Keep: i.Definition.KeepFailedPayloads, Rows: ingFailureRows(i, page.Items, canDismiss), Next: page.Next, Appended: after != ""}
	sse := ds.NewSSE(w, r)
	var perr error
	if v.Appended {
		// Older rows go below the ones shown; the button follows the new last page.
		if perr = sse.Patch(ingFailureItems(v.Rows), ds.Append("#"+ingFailuresList)); perr == nil {
			perr = sse.Patch(ingFailuresMore(v))
		}
	} else {
		perr = sse.Patch(ingFailuresPanel(v))
	}
	if perr != nil {
		slog.DebugContext(ctx, "ui: ingestion failures patch not delivered", "err", perr)
	}
}

// ── history ────────────────────────────────────────────────────────────

// ingOperationVerbs are the audit operations of an ingestion, as the Nuxt
// control plane's VERBS say them. The audit log names an operation by its
// command's type.
var ingOperationVerbs = map[string]string{
	"CreateCommand":          "Created",
	"UpdateCommand":          "Changed",
	"SaveVersionCommand":     "Version saved, not live,",
	"MakeVersionLiveCommand": "Made live",
	"PauseCommand":           "Paused",
	"ResumeCommand":          "Resumed",
	"DeleteCommand":          "Removed",
	"setAuditCommand":        "Applied from a set",
	"ReleaseSetCommand":      "Released from its set",
}

type ingHistoryRow struct {
	Verb, Who string
	At        time.Time
	// Changed names the parts of the definition that differ from the
	// definition the previous change carried.
	Changed string
	Note    string
}

type ingHistoryView struct {
	ID       string
	Rows     []ingHistoryRow
	Next     int
	More     bool
	Appended bool
}

func (v ingHistoryView) moreURL() string {
	return ingTabURL(v.ID) + "/history?offset=" + strconv.Itoa(v.Next)
}

// ingAuditCommand is what the history reads of an audit row's command: the
// definition a create, update or saved version carried, and its note.
type ingAuditCommand struct {
	Definition json.RawMessage `json:"definition"`
	Note       *string         `json:"note"`
}

func (c ingAuditCommand) definition() (ingestion.Definition, bool) {
	if len(c.Definition) == 0 {
		return ingestion.Definition{}, false
	}
	d, err := ingestion.ParseDefinition(c.Definition)
	return d, err == nil
}

// ingHistoryRows turns audit rows (newest first; the last one is only the
// predecessor of the one before it, when the page is not the whole log) into
// rows. Which parts of the definition a change touched is read by comparing
// the definitions the commands carried; the audit row holds the command, not
// the state before it, and only field names are ever shown.
func ingHistoryRows(logs []audit.Log, n int) []ingHistoryRow {
	rows := make([]ingHistoryRow, 0, min(len(logs), n))
	for k, l := range logs[:min(len(logs), n)] {
		who := "system"
		switch {
		case l.PrincipalName != nil && *l.PrincipalName != "":
			who = *l.PrincipalName
		case l.PrincipalID != nil:
			who = *l.PrincipalID
		}
		verb, known := ingOperationVerbs[l.Operation]
		if !known {
			verb = l.Operation
		}
		row := ingHistoryRow{Verb: verb, Who: who, At: l.PerformedAt}
		var cmd ingAuditCommand
		if len(l.OperationJSON) > 0 && json.Unmarshal(l.OperationJSON, &cmd) == nil {
			row.Note = deref(cmd.Note)
			if def, ok := cmd.definition(); ok && l.Operation != "CreateCommand" {
				row.Changed = ingChangedAgainstOlder(def, logs[k+1:])
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// ingChangedAgainstOlder lists, comma separated, the parts of def that differ
// from the definition of the newest of older that carries one ("" when none
// does, or nothing differs).
func ingChangedAgainstOlder(def ingestion.Definition, older []audit.Log) string {
	for _, o := range older {
		var cmd ingAuditCommand
		if json.Unmarshal(o.OperationJSON, &cmd) != nil {
			continue
		}
		prev, ok := cmd.definition()
		if !ok {
			continue
		}
		out := ""
		for _, f := range prev.ChangedFields(def) {
			if out != "" {
				out += ", "
			}
			out += f
		}
		return out
	}
	return ""
}

func (h *IngestionTabs) history(w http.ResponseWriter, r *http.Request) {
	i, ok := h.load(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	offset, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || offset < 0 {
		offset = 0
	}
	entityType := "Ingestion"
	logs, err := h.Audit.FindWithFilters(ctx, audit.FilterParams{
		EntityType: &entityType, EntityID: &i.ID, Limit: ingHistoryPage + 1, Offset: offset,
	})
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "audit lookup failed", err))
		return
	}
	v := ingHistoryView{
		ID: i.ID, Rows: ingHistoryRows(logs, ingHistoryPage), More: len(logs) > ingHistoryPage,
		Next: offset + ingHistoryPage, Appended: offset > 0,
	}
	sse := ds.NewSSE(w, r)
	var perr error
	if v.Appended {
		if perr = sse.Patch(ingHistoryItems(v.Rows), ds.Append("#"+ingHistoryList)); perr == nil {
			perr = sse.Patch(ingHistoryMore(v))
		}
	} else {
		perr = sse.Patch(ingHistoryPanel(v))
	}
	if perr != nil {
		slog.DebugContext(ctx, "ui: ingestion history patch not delivered", "err", perr)
	}
}
