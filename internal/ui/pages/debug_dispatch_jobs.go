package pages

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// RawDispatchJobStore is the slice of the dispatch job repository the page
// reads: the write side's most recent rows (GET /bff/debug/dispatch-jobs) and
// one row (GET /api/dispatch-jobs/{id}/raw).
type RawDispatchJobStore interface {
	FindRecentRaw(ctx context.Context, limit int) ([]dispatchjob.DispatchJob, error)
	FindByID(ctx context.Context, id string) (*dispatchjob.DispatchJob, error)
}

// DebugDispatchJobs is the Raw Dispatch Jobs page (/ui/debug/dispatch-jobs),
// mirroring the SPA's RawDispatchJobListPage: the most recent rows of the
// write-side dispatch job table, as stored, with a detail drawer. Like the
// API it is for holders of dispatch-job:view-raw; the list is not scoped to a
// client (the permission is the boundary), a single job opened from it is
// checked against the viewer's clients as GET /api/dispatch-jobs/{id}/raw does.
type DebugDispatchJobs struct {
	Jobs  RawDispatchJobStore
	Shell *shell.Shell
}

const (
	dbgDjPath = "/ui/debug/dispatch-jobs"
	dbgDjForm = "dbg-dj-list"

	dbgDjDrawerID  = "dbg-dj-drawer"
	dbgDjDrawerSig = "selDbgDj"

	dbgDjPerm = "platform:messaging:dispatch-job:view-raw"

	dbgDjLiveGet = "@get('/ui/debug/dispatch-jobs', {contentType: 'form'})"
)

// Mount registers the page's routes (read-only: no POSTs).
func (h *DebugDispatchJobs) Mount(r chi.Router) {
	r.Get("/debug/dispatch-jobs", h.list)
	r.Get("/debug/dispatch-jobs/{id}", h.detail)
}

type dbgDjRow struct {
	ID, ShortID, Href string
	Code, Kind        string
	Status            string
	StatusSeverity    kit.TagSeverity
	Attempts          string
	Payload           string
	Key, KeyShort     string
	Created           time.Time
}

type dbgDjView struct {
	Size   int
	Rows   []dbgDjRow
	Drawer *dbgDjDetail
}

// dbgDjSeverity is the page's getSeverity: anything not named is secondary.
func dbgDjSeverity(status string) kit.TagSeverity {
	switch status {
	case "COMPLETED":
		return kit.TagSuccess
	case "PENDING", "QUEUED":
		return kit.TagInfo
	case "PROCESSING":
		return kit.TagWarn
	case "FAILED":
		return kit.TagDanger
	}
	return kit.TagSecondary
}

// dbgDjAttempts is the page's formatAttempts: "n/max", a missing maximum
// shown as 3.
func dbgDjAttempts(j *dispatchjob.DispatchJob) string {
	return strconv.Itoa(int(j.AttemptCount)) + "/" + strconv.Itoa(int(cmp.Or(j.MaxRetries, 3)))
}

func dbgDjPayloadLen(j *dispatchjob.DispatchJob) int {
	if j.Payload == nil {
		return 0
	}
	return len(*j.Payload)
}

func (h *DebugDispatchJobs) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.CanWritePermission(auth.FromContext(ctx), dbgDjPerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, dbgSize(r.URL.Query()))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(sse.Patch(dbgDjTable(view)), sse.Patch(dbgDjSummary(view)), sse.Patch(kit.TableToolbarState(view.toolbar().State())), sse.ReplaceURL(dbgURL(dbgDjPath, view.Size))); err != nil {
			slog.DebugContext(ctx, "ui: raw dispatch jobs patch not delivered", "err", err)
		}
		return
	}
	h.Shell.Render(w, r, shell.Page{Title: "Raw Dispatch Jobs", Path: dbgDjPath}, dbgDjPage(view))
}

func (h *DebugDispatchJobs) listView(ctx context.Context, size int) (dbgDjView, error) {
	jobs, err := h.Jobs.FindRecentRaw(ctx, size)
	if err != nil {
		return dbgDjView{}, usecase.Internal("REPO", "find_recent_raw failed", err)
	}
	v := dbgDjView{Size: size}
	for i := range jobs {
		j := &jobs[i]
		key := deref(j.IdempotencyKey)
		v.Rows = append(v.Rows, dbgDjRow{
			ID: j.ID, ShortID: truncateID(j.ID), Href: dbgDjPath + "/" + url.PathEscape(j.ID), Code: j.Code, Kind: string(j.Kind),
			Status: string(j.Status), StatusSeverity: dbgDjSeverity(string(j.Status)), Attempts: dbgDjAttempts(j),
			Payload: strconv.Itoa(dbgDjPayloadLen(j)) + " bytes", Key: key, KeyShort: cmp.Or(truncateID(key), "-"), Created: j.CreatedAt,
		})
	}
	return v, nil
}

func (v dbgDjView) toolbar() kit.TableToolbarProps {
	return kit.TableToolbarProps{
		ID: dbgDjForm, Action: dbgDjPath, HideSearch: true, Size: &kit.ResultSize{Options: dbgSizes, Selected: v.Size},
		FormAttrs: templ.Attributes{"data-on:submit": dbgDjLiveGet},
	}
}

// ── drawer ─────────────────────────────────────────────────────────────

type dbgDjDetail struct {
	ID, Code, Kind, Source, TargetURL string
	Status                            string
	StatusSeverity                    kit.TagSeverity
	ClientID, SubscriptionID, Group   string
	Attempts, LastError, Key          string
	Correlation, EventID, Payload     string
	Created, Updated                  time.Time
	Scheduled, Completed              *time.Time
}

func (d dbgDjDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: dbgDjDrawerID, Signal: dbgDjDrawerSig, Label: "Raw dispatch job", Size: kit.DrawerWide, Title: cmp.Or(d.Code, "Raw dispatch job"), Subtitle: d.ID}
}

func toDbgDjDetail(j *dispatchjob.DispatchJob) dbgDjDetail {
	dash := func(s *string) string { return cmp.Or(deref(s), "-") }
	return dbgDjDetail{
		ID: j.ID, Code: j.Code, Kind: string(j.Kind), Source: dash(j.Source), TargetURL: j.TargetURL,
		Status: string(j.Status), StatusSeverity: dbgDjSeverity(string(j.Status)),
		ClientID: dash(ids.StringPtr(j.ClientID)), SubscriptionID: dash(j.SubscriptionID), Group: dash(j.MessageGroup),
		Attempts: dbgDjAttempts(j) + " (" + strconv.Itoa(len(j.Attempts)) + " in history)", LastError: dash(j.LastError), Key: dash(j.IdempotencyKey),
		Correlation: dash(j.CorrelationID), EventID: dash(j.EventID),
		Payload: strconv.Itoa(dbgDjPayloadLen(j)) + " bytes (" + j.PayloadContentType + ")",
		Created: j.CreatedAt, Updated: j.UpdatedAt, Scheduled: j.ScheduledFor, Completed: j.CompletedAt,
	}
}

func (h *DebugDispatchJobs) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, dbgDjPerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		h.Shell.Fail(w, r, httperror.NotFound("DispatchJob", id))
		return
	}
	j, err := h.Jobs.FindByID(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "find_raw failed", err))
		return
	}
	if j == nil {
		h.Shell.Fail(w, r, httperror.NotFound("DispatchJob", id))
		return
	}
	// GET /api/dispatch-jobs/{id}/raw: a client's job needs access to the client.
	if err := auth.CheckScopeAccess(ac, ids.StringPtr(j.ClientID)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d := toDbgDjDetail(j)
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(
			sse.Patch(dbgDjDrawerContent(d), ds.Inner("#"+dbgDjDrawerID)),
			sse.Patch(dbgDjDrawerBody(d), ds.WithMode(ds.ModeReplace)),
			sse.PatchSignals(map[string]any{dbgDjDrawerSig: d.ID}),
			sse.Focus(dbgDjDrawerID+"-title"),
		); err != nil {
			slog.DebugContext(ctx, "ui: raw dispatch job drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, dbgDefaultSize)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.Shell.Render(w, r, shell.Page{Title: "Raw Dispatch Jobs", Path: dbgDjPath}, dbgDjPage(view))
}
