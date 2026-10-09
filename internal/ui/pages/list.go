package pages

import (
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

// The URL is the list state: a list page's search, status filter and sort
// are query parameters, so every view has a URL (docs/ui/README.md).

// listSpec describes one list page's URL state.
type listSpec struct {
	// path is the list's URL path ("/ui/dispatch-pools").
	path string
	// sortKeys are the sortable columns; the first is the default sort.
	sortKeys []string
	// statusOK accepts a status filter value (the entity's ParseStatus).
	statusOK func(string) bool
}

// listQuery is one view of a list.
type listQuery struct {
	spec   *listSpec
	Q      string
	Status string
	Sort   string
	Dir    kit.SortDirection
}

// parseListQuery reads q, status, sort and dir; anything unknown falls back
// to the default rather than erroring (a hand-edited URL still shows a list).
func parseListQuery(v url.Values, spec *listSpec) listQuery {
	q := listQuery{spec: spec, Q: strings.TrimSpace(v.Get("q")), Sort: spec.sortKeys[0], Dir: kit.SortAsc}
	if len(q.Q) > maxQueryLen {
		q.Q = q.Q[:maxQueryLen]
	}
	if s := v.Get("status"); s != "" && spec.statusOK(s) {
		q.Status = s
	}
	if s := v.Get("sort"); slices.Contains(spec.sortKeys, s) {
		q.Sort = s
	}
	if v.Get("dir") == string(kit.SortDesc) {
		q.Dir = kit.SortDesc
	}
	return q
}

// refererList is the list view the request came from (the page's address,
// which live filtering keeps current with ReplaceURL), so an action
// re-renders the table the user is looking at. Only the view is taken from
// it; anything else is the default view.
func refererList(r *http.Request, spec *listSpec) listQuery {
	ref, err := url.Parse(r.Referer())
	if err != nil || ref.Path != spec.path {
		return parseListQuery(nil, spec)
	}
	return parseListQuery(ref.Query(), spec)
}

func (q listQuery) defaultSort() bool { return q.Sort == q.spec.sortKeys[0] && q.Dir == kit.SortAsc }

// values encodes the non-default parts.
func (q listQuery) values() url.Values {
	v := url.Values{}
	if q.Q != "" {
		v.Set("q", q.Q)
	}
	if q.Status != "" {
		v.Set("status", q.Status)
	}
	if !q.defaultSort() {
		v.Set("sort", q.Sort)
		v.Set("dir", string(q.Dir))
	}
	return v
}

// URL is the canonical URL of this view.
func (q listQuery) URL() string {
	if enc := q.values().Encode(); enc != "" {
		return q.spec.path + "?" + enc
	}
	return q.spec.path
}

// clearURL is the view without search and filters, keeping the sort
// (table-toolbar.md: Clear All clears what the user typed and picked).
func (q listQuery) clearURL() string {
	return listQuery{spec: q.spec, Sort: q.Sort, Dir: q.Dir}.URL()
}

// sortURL is the URL a column heading links to: same filters, sorted by key,
// toggling the direction when key is already the sort.
func (q listQuery) sortURL(key string) string {
	next := q
	next.Sort = key
	next.Dir = kit.SortAsc
	if q.Sort == key && q.Dir == kit.SortAsc {
		next.Dir = kit.SortDesc
	}
	return next.URL()
}

func (q listQuery) direction(key string) kit.SortDirection {
	if q.Sort == key {
		return q.Dir
	}
	return kit.SortNone
}

// hidden are the toolbar's hidden sort fields (none for the default sort).
func (q listQuery) hidden() []kit.HiddenField {
	if q.defaultSort() {
		return nil
	}
	return []kit.HiddenField{{Name: "sort", Value: q.Sort}, {Name: "dir", Value: string(q.Dir)}}
}

// filtered reports whether search or a filter narrows the list.
func (q listQuery) filtered() bool { return q.Q != "" || q.Status != "" }
