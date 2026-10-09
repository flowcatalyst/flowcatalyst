package pages

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// PoolFinder is the slice of the dispatch-pool repository the page reads
// (the same call GET /api/dispatch-pools makes).
type PoolFinder interface {
	FindWithFilters(ctx context.Context, status, clientID *string) ([]dispatchpool.DispatchPool, error)
}

// DispatchPools is the read-only dispatch pools list (/ui/dispatch-pools).
// Create and edit stay in the SPA for now; the page links there.
type DispatchPools struct {
	Pools PoolFinder
	Shell *shell.Shell
}

const poolsPath = "/ui/dispatch-pools"

// Mount registers the page's routes.
func (h *DispatchPools) Mount(r chi.Router) {
	r.Get("/dispatch-pools", h.list)
}

// list renders the page, or, for a Datastar request (the live search,
// status filter and sort links), patches the table, the summary and the
// Filters button and replaces the address bar URL so the view stays shareable.
func (h *DispatchPools) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// Same check as GET /api/dispatch-pools.
	if err := auth.CanReadDispatchPools(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	q := parsePoolQuery(r.URL.Query())
	var status *string
	if q.Status != "" {
		status = &q.Status
	}
	pools, err := h.Pools.FindWithFilters(ctx, status, nil)
	if err != nil {
		h.Shell.Fail(w, r, fmt.Errorf("find dispatch pools: %w", err))
		return
	}
	// Same per-client visibility as the API list.
	visible := auth.FilterClientScoped(ac, pools, func(p *dispatchpool.DispatchPool) *string { return ids.StringPtr(p.ClientID) })
	view := buildPoolsView(ac, q, visible)

	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(
			sse.Patch(poolsTable(view)),
			sse.Patch(poolsSummary(view)),
			sse.Patch(kit.FiltersButton(kit.FiltersButtonProps{FormID: poolsFormID, ActiveFilterCount: view.FilterCount()}),
				ds.WithSelector("#"+poolsFormID+" [popovertarget]")),
			sse.ReplaceURL(q.URL()),
		); err != nil {
			slog.DebugContext(ctx, "ui: dispatch pools patch not delivered", "err", err)
		}
		return
	}
	h.Shell.Render(w, r, shell.Page{Title: "Dispatch Pools", Path: poolsPath}, poolsPage(view))
}

// ── the URL is the list state ──────────────────────────────────────────

const (
	poolsFormID = "pools-list"
	maxQueryLen = 200
)

var poolSortKeys = []string{"code", "name", "status", "created"}

type poolQuery struct {
	Q      string
	Status string
	Sort   string
	Dir    kit.SortDirection
}

// parsePoolQuery reads q, status, sort and dir; anything unknown falls back
// to the default rather than erroring (a hand-edited URL still shows a list).
func parsePoolQuery(v url.Values) poolQuery {
	q := poolQuery{Q: strings.TrimSpace(v.Get("q")), Sort: "code", Dir: kit.SortAsc}
	if len(q.Q) > maxQueryLen {
		q.Q = q.Q[:maxQueryLen]
	}
	if s, ok := dispatchpool.ParseStatus(v.Get("status")); ok {
		q.Status = string(s)
	}
	if s := v.Get("sort"); slices.Contains(poolSortKeys, s) {
		q.Sort = s
	}
	if v.Get("dir") == string(kit.SortDesc) {
		q.Dir = kit.SortDesc
	}
	return q
}

// values encodes the non-default parts.
func (q poolQuery) values() url.Values {
	v := url.Values{}
	if q.Q != "" {
		v.Set("q", q.Q)
	}
	if q.Status != "" {
		v.Set("status", q.Status)
	}
	if q.Sort != "code" || q.Dir != kit.SortAsc {
		v.Set("sort", q.Sort)
		v.Set("dir", string(q.Dir))
	}
	return v
}

// URL is the canonical URL of this view.
func (q poolQuery) URL() string {
	if enc := q.values().Encode(); enc != "" {
		return poolsPath + "?" + enc
	}
	return poolsPath
}

// sortURL is the URL a column heading links to: same filters, sorted by key,
// toggling the direction when key is already the sort.
func (q poolQuery) sortURL(key string) string {
	next := q
	next.Sort = key
	next.Dir = kit.SortAsc
	if q.Sort == key && q.Dir == kit.SortAsc {
		next.Dir = kit.SortDesc
	}
	return next.URL()
}

func (q poolQuery) direction(key string) kit.SortDirection {
	if q.Sort == key {
		return q.Dir
	}
	return kit.SortNone
}

// ── view model ─────────────────────────────────────────────────────────

type poolRow struct {
	ID, Code, Name, Scope  string
	RateLimit, Concurrency string
	Status                 string
	StatusVariant          kit.BadgeVariant
	Created, CreatedISO    string
	DetailHref, EditHref   string
}

type poolsView struct {
	Query     poolQuery
	Rows      []poolRow
	Total     int // visible pools before the search (status is filtered in SQL)
	CanCreate bool
	CanEdit   bool
}

// FilterCount is the number of active filters in the Filters popover.
func (v poolsView) FilterCount() int {
	if v.Query.Status != "" {
		return 1
	}
	return 0
}

// Filtered reports whether search or a filter narrows the list.
func (v poolsView) Filtered() bool { return v.Query.Q != "" || v.Query.Status != "" }

func buildPoolsView(ac *auth.AuthContext, q poolQuery, pools []dispatchpool.DispatchPool) poolsView {
	v := poolsView{
		Query:     q,
		Total:     len(pools),
		CanCreate: auth.CanCreateDispatchPools(ac) == nil,
		CanEdit:   auth.CanUpdateDispatchPools(ac) == nil,
	}
	needle := strings.ToLower(q.Q)
	matched := make([]dispatchpool.DispatchPool, 0, len(pools))
	for _, p := range pools {
		// The SPA's quick filter searched code, name and client identifier.
		hay := strings.ToLower(p.Code + " " + p.Name + " " + deref(p.ClientIdentifier))
		if needle == "" || strings.Contains(hay, needle) {
			matched = append(matched, p)
		}
	}
	slices.SortStableFunc(matched, func(a, b dispatchpool.DispatchPool) int {
		var c int
		switch q.Sort {
		case "name":
			c = cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		case "status":
			c = cmp.Compare(a.Status, b.Status)
		case "created":
			c = a.CreatedAt.Compare(b.CreatedAt)
		default:
			c = cmp.Compare(a.Code, b.Code)
		}
		if q.Dir == kit.SortDesc {
			c = -c
		}
		return c
	})
	for _, p := range matched {
		v.Rows = append(v.Rows, toPoolRow(p))
	}
	return v
}

func toPoolRow(p dispatchpool.DispatchPool) poolRow {
	row := poolRow{
		ID:            p.ID,
		Code:          p.Code,
		Name:          p.Name,
		Scope:         "Anchor-level",
		RateLimit:     "—",
		Concurrency:   strconv.Itoa(int(p.Concurrency)),
		Status:        string(p.Status),
		StatusVariant: kit.BadgeSecondary,
		Created:       p.CreatedAt.UTC().Format("2006-01-02"),
		CreatedISO:    p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		// Detail and edit live in the SPA until they are ported.
		DetailHref: "/dispatch-pools/" + url.PathEscape(p.ID),
		EditHref:   "/dispatch-pools/" + url.PathEscape(p.ID) + "?edit=true",
	}
	if p.ClientIdentifier != nil && *p.ClientIdentifier != "" {
		row.Scope = *p.ClientIdentifier
	}
	if p.RateLimit != nil {
		row.RateLimit = strconv.Itoa(int(*p.RateLimit)) + "/min"
	}
	switch p.Status {
	case dispatchpool.StatusActive:
		row.StatusVariant = kit.BadgeSuccess
	case dispatchpool.StatusSuspended:
		row.StatusVariant = kit.BadgeWarn
	case dispatchpool.StatusArchived:
		row.StatusVariant = kit.BadgeSecondary
	}
	return row
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
