package pages

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// RawEventStore is the slice of the event repository the page reads: the write
// side's most recent rows and one row, as GET /bff/debug/events and
// /bff/debug/events/{id}.
type RawEventStore interface {
	FindRecentRaw(ctx context.Context, limit int) ([]event.Event, error)
	FindRawByID(ctx context.Context, id string) (*event.Event, error)
}

// DebugEvents is the Raw Events page (/ui/debug/events), mirroring the SPA's
// RawEventListPage: the most recent rows of the write-side event table, as
// stored, with a detail drawer. Like the API it is for holders of
// event:view-raw and is not scoped to a client (the permission is the
// boundary).
type DebugEvents struct {
	Events RawEventStore
	Shell  *shell.Shell
}

const (
	dbgEvPath = "/ui/debug/events"
	dbgEvForm = "dbg-ev-list"

	dbgEvDrawerID  = "dbg-ev-drawer"
	dbgEvDrawerSig = "selDbgEv"

	dbgEvPerm = "platform:messaging:event:view-raw"

	dbgEvLiveGet = "@get('/ui/debug/events', {contentType: 'form'})"

	// dbgDefaultSize and dbgSizes are the SPA's result sizes (the repository
	// caps at 1000).
	dbgDefaultSize = 200
)

var dbgSizes = []int{50, 100, 200, 500, 1000}

// Mount registers the page's routes (read-only: no POSTs).
func (h *DebugEvents) Mount(r chi.Router) {
	r.Get("/debug/events", h.list)
	r.Get("/debug/events/{id}", h.detail)
}

// dbgSize reads the result size from the query: one of the offered sizes.
func dbgSize(v url.Values) int {
	if n, err := strconv.Atoi(v.Get("size")); err == nil && slices.Contains(dbgSizes, n) {
		return n
	}
	return dbgDefaultSize
}

func dbgURL(path string, size int) string {
	if size == dbgDefaultSize {
		return path
	}
	return path + "?size=" + strconv.Itoa(size)
}

type dbgEvRow struct {
	ID, ShortID, Href string
	Type, Source      string
	Subject           string
	Dedup, DedupShort string
	Time              time.Time
}

type dbgEvView struct {
	Size   int
	Rows   []dbgEvRow
	Drawer *dbgEvDetail
}

func (h *DebugEvents) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, dbgEvPerm); err != nil {
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
		if err := errors.Join(sse.Patch(dbgEvTable(view)), sse.Patch(dbgEvSummary(view)), sse.Patch(kit.TableToolbarState(view.toolbar().State())), sse.ReplaceURL(dbgURL(dbgEvPath, view.Size))); err != nil {
			slog.DebugContext(ctx, "ui: raw events patch not delivered", "err", err)
		}
		return
	}
	h.Shell.Render(w, r, shell.Page{Title: "Raw Events", Path: dbgEvPath}, dbgEvPage(view))
}

func (h *DebugEvents) listView(ctx context.Context, size int) (dbgEvView, error) {
	events, err := h.Events.FindRecentRaw(ctx, size)
	if err != nil {
		return dbgEvView{}, usecase.Internal("REPO", "find_recent_raw failed", err)
	}
	v := dbgEvView{Size: size}
	for i := range events {
		e := &events[i]
		v.Rows = append(v.Rows, dbgEvRow{
			ID: e.ID, ShortID: truncateID(e.ID), Href: dbgEvPath + "/" + url.PathEscape(e.ID), Type: e.Type, Source: e.Source,
			Subject: cmp.Or(e.Subject, "-"), Dedup: e.DeduplicationID, DedupShort: cmp.Or(truncateID(e.DeduplicationID), "-"), Time: e.Time,
		})
	}
	return v, nil
}

func (v dbgEvView) toolbar() kit.TableToolbarProps {
	return kit.TableToolbarProps{
		ID: dbgEvForm, Action: dbgEvPath, HideSearch: true, Size: &kit.ResultSize{Options: dbgSizes, Selected: v.Size},
		FormAttrs: templ.Attributes{"data-on:submit": dbgEvLiveGet},
	}
}

// ── drawer ─────────────────────────────────────────────────────────────

type dbgEvDetail struct {
	ID, Type, Source, Subject string
	Time                      time.Time
	ClientID, MessageGroup    string
	Correlation, Causation    string
	Deduplication             string
	Data                      string
	Context                   []djKV
}

func (d dbgEvDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: dbgEvDrawerID, Signal: dbgEvDrawerSig, Label: "Raw event", Size: kit.DrawerWide, Title: cmp.Or(d.Type, "Raw event"), Subtitle: d.ID}
}

func toDbgEvDetail(e *event.Event) dbgEvDetail {
	d := dbgEvDetail{
		ID: e.ID, Type: e.Type, Source: e.Source, Subject: cmp.Or(e.Subject, "-"), Time: e.Time,
		MessageGroup: cmp.Or(deref(e.MessageGroup), "-"), Correlation: cmp.Or(deref(e.CorrelationID), "-"),
		Causation: cmp.Or(deref(e.CausationID), "-"), Deduplication: cmp.Or(e.DeduplicationID, "-"), Data: formatData(e.Data),
		ClientID: cmp.Or(deref(ids.StringPtr(e.ClientID)), "-"),
	}
	for _, c := range e.Context {
		d.Context = append(d.Context, djKV{Key: c.Key, Value: c.Value})
	}
	return d
}

func (h *DebugEvents) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// The same gate as the list (event:view-raw, not event:view), and not
	// client-scoped, like the list it opens from.
	if err := auth.CanWritePermission(ac, dbgEvPerm); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "id")
	if !eventIDPattern.MatchString(id) {
		h.Shell.Fail(w, r, httperror.NotFound("Event", id))
		return
	}
	e, err := h.Events.FindRawByID(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "find_raw_by_id failed", err))
		return
	}
	if e == nil {
		h.Shell.Fail(w, r, httperror.NotFound("Event", id))
		return
	}
	d := toDbgEvDetail(e)
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(
			sse.Patch(dbgEvDrawerContent(d), ds.Inner("#"+dbgEvDrawerID)),
			sse.Patch(dbgEvDrawerBody(d), ds.WithMode(ds.ModeReplace)),
			sse.PatchSignals(map[string]any{dbgEvDrawerSig: d.ID}),
			sse.Focus(dbgEvDrawerID+"-title"),
		); err != nil {
			slog.DebugContext(ctx, "ui: raw event drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, dbgDefaultSize)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.Shell.Render(w, r, shell.Page{Title: "Raw Events", Path: dbgEvPath}, dbgEvPage(view))
}
