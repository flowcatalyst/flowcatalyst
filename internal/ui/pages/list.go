package pages

import (
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
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
	// defaultDir is the default sort's direction (ascending when empty).
	defaultDir kit.SortDirection
	// statusOK accepts a status filter value (the entity's ParseStatus).
	statusOK func(string) bool
	// facets are multi-valued filters: each is a query parameter that may
	// repeat (?app=a&app=b), e.g. a multi-select's checkboxes.
	facets []string
	// rows enables paging: the rows-per-page choices; defaultRows is the
	// default (one of them).
	rows        []int
	defaultRows int
}

// maxFacetValues caps the values a facet accepts from a URL.
const maxFacetValues = 100

// listQuery is one view of a list.
type listQuery struct {
	spec   *listSpec
	Q      string
	Status string
	Sort   string
	Dir    kit.SortDirection
	// Facets holds the chosen values of each facet (spec.facets).
	Facets map[string][]string
	// Page (1-based) and Rows apply when the spec pages.
	Page int
	Rows int
}

// parseListQuery reads q, status, sort and dir; anything unknown falls back
// to the default rather than erroring (a hand-edited URL still shows a list).
func parseListQuery(v url.Values, spec *listSpec) listQuery {
	q := listQuery{spec: spec, Q: strings.TrimSpace(v.Get("q")), Sort: spec.sortKeys[0], Dir: spec.dir()}
	if len(q.Q) > maxQueryLen {
		q.Q = q.Q[:maxQueryLen]
	}
	if s := v.Get("status"); s != "" && spec.statusOK(s) {
		q.Status = s
	}
	if s := v.Get("sort"); slices.Contains(spec.sortKeys, s) {
		q.Sort = s
	}
	if d := kit.SortDirection(v.Get("dir")); d == kit.SortAsc || d == kit.SortDesc {
		q.Dir = d
	}
	for _, name := range spec.facets {
		if vals := facetValues(v[name]); len(vals) > 0 {
			if q.Facets == nil {
				q.Facets = map[string][]string{}
			}
			q.Facets[name] = vals
		}
	}
	if spec.paged() {
		q.Page, q.Rows = 1, spec.defaultRows
		if n, err := strconv.Atoi(v.Get("page")); err == nil && n > 1 {
			q.Page = n
		}
		if n, err := strconv.Atoi(v.Get("rows")); err == nil && slices.Contains(spec.rows, n) {
			q.Rows = n
		}
	}
	return q
}

func (s *listSpec) paged() bool { return len(s.rows) > 0 }

// dir is the default sort's direction.
func (s *listSpec) dir() kit.SortDirection {
	if s.defaultDir == "" {
		return kit.SortAsc
	}
	return s.defaultDir
}

// facetValues trims, de-duplicates and caps a facet's values.
func facetValues(raw []string) []string {
	var out []string
	for _, v := range raw {
		v = strings.TrimSpace(v)
		if v == "" || len(v) > maxQueryLen || slices.Contains(out, v) {
			continue
		}
		out = append(out, v)
		if len(out) == maxFacetValues {
			break
		}
	}
	return out
}

// facet is the chosen values of one facet.
func (q listQuery) facet(name string) []string { return q.Facets[name] }

// withFacet is q with a facet's values replaced (pruned to the options).
func (q listQuery) withFacet(name string, vals []string) listQuery {
	next := q
	next.Facets = make(map[string][]string, len(q.Facets))
	maps.Copy(next.Facets, q.Facets)
	if len(vals) == 0 {
		delete(next.Facets, name)
	} else {
		next.Facets[name] = vals
	}
	return next
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

func (q listQuery) defaultSort() bool { return q.Sort == q.spec.sortKeys[0] && q.Dir == q.spec.dir() }

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
	for _, name := range q.spec.facets {
		for _, val := range q.Facets[name] {
			v.Add(name, val)
		}
	}
	if q.spec.paged() {
		if q.Page > 1 {
			v.Set("page", strconv.Itoa(q.Page))
		}
		if q.Rows != q.spec.defaultRows {
			v.Set("rows", strconv.Itoa(q.Rows))
		}
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
	return listQuery{spec: q.spec, Sort: q.Sort, Dir: q.Dir, Page: 1, Rows: q.Rows}.URL()
}

// pageURL is this view on page n.
func (q listQuery) pageURL(n int) string {
	next := q
	next.Page = n
	return next.URL()
}

// pagePattern is this view's URL with the paginator's {page} placeholder
// (kit paginator.md, hrefPattern).
func (q listQuery) pagePattern() string {
	const marker = 999999937
	return strings.Replace(q.pageURL(marker), "page="+strconv.Itoa(marker), "page={page}", 1)
}

// sortURL is the URL a column heading links to: same filters, sorted by key,
// toggling the direction when key is already the sort.
func (q listQuery) sortURL(key string) string {
	next := q
	next.Sort = key
	next.Dir = kit.SortAsc
	next.Page = 1
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
func (q listQuery) filtered() bool { return q.Q != "" || q.Status != "" || len(q.Facets) > 0 }

// pageBounds clamps q's page to a list of total rows and returns the page,
// the page count and the slice bounds [lo, hi) of the rows shown.
func (q listQuery) pageBounds(total int) (page, pages, lo, hi int) {
	rows := max(q.Rows, 1)
	pages = max((total+rows-1)/rows, 1)
	page = min(max(q.Page, 1), pages)
	lo = min((page-1)*rows, total)
	hi = min(lo+rows, total)
	return page, pages, lo, hi
}
