package pages

import (
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// ── URL state ──────────────────────────────────────────────────────────

// djQuery is one view of the list: the facets, the source search (q), the
// message group, the created-at day range, the sort, the result size and
// the keyset position. Everything but the size, sort and position is the
// filter a requeue-by-filter acts on.
type djQuery struct {
	listQuery
	Group    string
	From, To string // YYYY-MM-DD, UTC days
	// Window is a preset key (djWindowPresets); empty is the default window.
	// Dates, when chosen, win over it.
	Window string
	// Lens is "stuck" while the view is the Stuck quick view (PENDING and
	// QUEUED jobs older than five minutes); the statuses alone cannot say.
	Lens string
	// EventTypes are the event types asked for (a job's code is its event
	// type); Subscription is a subscription name and Connection a connection
	// code, each exact or ending in * for a prefix.
	EventTypes               []string
	Subscription, Connection string
	Size                     int
	// After is the page's keyset position: rows after this one in the
	// list's order ("Older"). Nil is the first page.
	After *dispatchjob.ListCursor
}

func parseDJQuery(v url.Values) djQuery {
	q := djQuery{listQuery: parseListQuery(v, djList), Size: djDefaultSize}
	q.Group = strings.TrimSpace(v.Get("group"))
	if len(q.Group) > maxQueryLen {
		q.Group = q.Group[:maxQueryLen]
	}
	q.From, q.To = validDay(v.Get("from")), validDay(v.Get("to"))
	if _, ok := djWindowPresets[v.Get("window")]; ok {
		q.Window = v.Get("window")
	}
	q.EventTypes = parseEventTypes(v["eventTypes"])
	// Picking other statuses by hand leaves the Stuck cap behind.
	if v.Get("lens") == djLensStuck && sameSet(q.facet(djFacetStatuses), djStuckStatuses) {
		q.Lens = djLensStuck
	}
	q.Subscription, q.Connection = clipQuery(v.Get("subscription")), clipQuery(v.Get("connection"))
	if n, err := strconv.Atoi(v.Get("size")); err == nil && slices.Contains(djSizes, n) {
		q.Size = n
	}
	if at, id, ok := parseCursor(v.Get("after")); ok {
		q.After = &dispatchjob.ListCursor{CreatedAt: at, ID: id}
	}
	return q
}

func clipQuery(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > maxQueryLen {
		s = s[:maxQueryLen]
	}
	return s
}

// parseEventTypes reads the event type filter: values, each possibly a
// comma-separated list, trimmed and de-duplicated. The count is not capped
// here (validate says so, as the API's 400 does).
func parseEventTypes(raw []string) []string {
	var out []string
	for _, r := range raw {
		for part := range strings.SplitSeq(r, ",") {
			part = strings.TrimSpace(part)
			if part != "" && len(part) <= maxQueryLen && !slices.Contains(out, part) {
				out = append(out, part)
			}
		}
	}
	return out
}

// validate refuses a view the API would: more than 10 event types.
func (q djQuery) validate() error {
	if len(q.EventTypes) > djMaxEventTypes {
		return usecase.Validation("TOO_MANY_EVENT_TYPES", fmt.Sprintf("at most %d event types can be filtered at once (got %d)", djMaxEventTypes, len(q.EventTypes)))
	}
	return nil
}

// filterValues is the filter alone: what "requeue all matching" acts on.
func (q djQuery) filterValues() url.Values {
	v := url.Values{}
	for _, name := range djFacets {
		for _, val := range q.Facets[name] {
			v.Add(name, val)
		}
	}
	if q.Q != "" {
		v.Set("q", q.Q)
	}
	if q.Group != "" {
		v.Set("group", q.Group)
	}
	if q.From != "" {
		v.Set("from", q.From)
	}
	if q.To != "" {
		v.Set("to", q.To)
	}
	if q.Window != "" {
		v.Set("window", q.Window)
	}
	if q.Lens != "" {
		v.Set("lens", q.Lens)
	}
	if len(q.EventTypes) > 0 {
		v.Set("eventTypes", strings.Join(q.EventTypes, ","))
	}
	if q.Subscription != "" {
		v.Set("subscription", q.Subscription)
	}
	if q.Connection != "" {
		v.Set("connection", q.Connection)
	}
	return v
}

func (q djQuery) values() url.Values {
	v := q.listQuery.values()
	maps.Copy(v, q.filterValues())
	if q.Size != djDefaultSize {
		v.Set("size", strconv.Itoa(q.Size))
	}
	if q.After != nil {
		v.Set("after", formatCursor(q.After.CreatedAt, q.After.ID))
	}
	return v
}

func encodeURL(path string, v url.Values) string {
	if enc := v.Encode(); enc != "" {
		return path + "?" + enc
	}
	return path
}

// sizeOnly keeps only a non-default result size and the time window (the
// error preset's URL).
func (q djQuery) sizeOnly() url.Values {
	v := url.Values{}
	if q.Size != djDefaultSize {
		v.Set("size", strconv.Itoa(q.Size))
	}
	// The error preset keeps the time window: its run is held to it.
	for k, val := range map[string]string{"window": q.Window, "from": q.From, "to": q.To} {
		if val != "" {
			v.Set(k, val)
		}
	}
	return v
}

// URL is the canonical URL of this view.
func (q djQuery) URL() string { return encodeURL(djPath, q.values()) }

// first is this view's first page.
func (q djQuery) first() djQuery {
	q.After = nil
	return q
}

// clearURL keeps the sort and the size (table-toolbar.md: Clear All clears
// what the user typed and picked).
func (q djQuery) clearURL() string {
	clear := djQuery{Size: q.Size}
	clear.spec, clear.Sort, clear.Dir = djList, q.Sort, q.Dir
	return clear.URL()
}

// sortURL flips the created-at order, from the first page.
func (q djQuery) sortURL() string {
	next := q.first()
	next.Sort = djList.sortKeys[0]
	next.Dir = kit.SortAsc
	if q.Dir == kit.SortAsc {
		next.Dir = kit.SortDesc
	}
	return next.URL()
}

// withGroup is the first page of this view narrowed to one message group.
func (q djQuery) withGroup(group string) djQuery {
	next := q.first()
	next.Group = group
	return next
}

func (q djQuery) filtered() bool {
	return q.listQuery.filtered() || q.Group != "" || q.From != "" || q.To != "" || q.Window != "" ||
		len(q.EventTypes) > 0 || q.Subscription != "" || q.Connection != ""
}

// failedLens reports whether the view is exactly the Failed quick view:
// FAILED and ERROR (their legacy alias) and nothing else in the status facet.
func (q djQuery) failedLens() bool {
	st := q.facet(djFacetStatuses)
	return len(st) == len(djErrorStatuses) && slices.ContainsFunc(st, func(s string) bool { return s == "FAILED" }) &&
		slices.ContainsFunc(st, func(s string) bool { return s == "ERROR" })
}

func sameSet(a, b []string) bool {
	return len(a) == len(b) && !slices.ContainsFunc(a, func(s string) bool { return !slices.Contains(b, s) })
}

// stuckLens reports whether the view is the Stuck quick view.
func (q djQuery) stuckLens() bool { return q.Lens == djLensStuck }

// failedLensURL toggles the Failed quick view on this view: it sets the status
// facet to FAILED and ERROR, or, when it is already on, clears it.
func (q djQuery) failedLensURL() string { return q.lensURL(q.failedLens(), djErrorStatuses, "") }

// stuckLensURL toggles the Stuck quick view: PENDING and QUEUED jobs created
// more than five minutes ago.
func (q djQuery) stuckLensURL() string { return q.lensURL(q.stuckLens(), djStuckStatuses, djLensStuck) }

func (q djQuery) lensURL(on bool, statuses []string, lens string) string {
	next := q.first()
	next.Lens = ""
	if on {
		next.Facets = q.withFacet(djFacetStatuses, nil).Facets
	} else {
		next.Facets = q.withFacet(djFacetStatuses, slices.Clone(statuses)).Facets
		next.Lens = lens
	}
	return next.URL()
}

// windowURL is this view with a window preset ("" is the default) and no
// chosen dates.
func (q djQuery) windowURL(key string) string {
	next := q.first()
	next.Window, next.From, next.To = key, "", ""
	return next.URL()
}

// activeFilters is the Filters badge: the facets in use plus the group, the
// event type, subscription and connection filters. The window and its dates
// sit in their own bar above the table, so they are not counted here.
func (q djQuery) activeFilters() int {
	n := len(q.Facets)
	for _, s := range []string{q.Group, q.Subscription, q.Connection} {
		if s != "" {
			n++
		}
	}
	if len(q.EventTypes) > 0 {
		n++
	}
	return n
}

// filterParams converts the filter: since is the start of the "from" day,
// until the start of the day after "to" (the SPA's whole-day bounds, here
// in UTC).
func (q djQuery) filterParams() dispatchjob.FilterParams {
	p := dispatchjob.FilterParams{
		ClientIDs: q.facet(djFacetClients), Statuses: q.facet(djFacetStatuses),
		Applications: q.facet(djFacetApps), Subdomains: q.facet(djFacetSubs),
		Aggregates: q.facet(djFacetAggs), Codes: q.facet(djFacetCodes),
	}
	if q.Q != "" {
		source := q.Q
		p.Source = &source
	}
	if q.Group != "" {
		group := q.Group
		p.MessageGroup = &group
	}
	p.Since, p.Until = dayBounds(q.From, q.To)
	p.EventTypes = q.EventTypes
	if q.Subscription != "" {
		name := q.Subscription
		p.SubscriptionName = &name
	}
	if q.Connection != "" {
		code := q.Connection
		p.ConnectionCode = &code
	}
	return p
}

// djReferer is the list view the request came from (see refererList).
func djReferer(r *http.Request) djQuery {
	ref, err := url.Parse(r.Referer())
	if err != nil || ref.Path != djPath {
		return parseDJQuery(nil)
	}
	return parseDJQuery(ref.Query())
}
