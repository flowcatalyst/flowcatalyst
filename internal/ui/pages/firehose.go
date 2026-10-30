package pages

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Helpers shared by the firehose lists (dispatch jobs, events): "most recent
// N" with a result size, keyset paging, whole-day ranges, facets derived
// from type codes, and cached distinct values.

// validDay keeps a YYYY-MM-DD day; anything else is no bound (a hand-edited
// URL still shows a list).
func validDay(s string) string {
	if _, err := time.Parse(time.DateOnly, s); err != nil {
		return ""
	}
	return s
}

// dayBounds is the SPA's whole-day range, in UTC: since is the start of the
// "from" day, until the start of the day after "to".
func dayBounds(from, to string) (since, until *time.Time) {
	if t, err := time.Parse(time.DateOnly, from); err == nil {
		since = &t
	}
	if t, err := time.Parse(time.DateOnly, to); err == nil {
		next := t.AddDate(0, 0, 1)
		until = &next
	}
	return since, until
}

// Cursors are "<created_at in unix microseconds>.<id>" (Postgres keeps
// microseconds, so the position is exact).
func formatCursor(at time.Time, id string) string {
	return strconv.FormatInt(at.UnixMicro(), 10) + "." + id
}

func parseCursor(s string) (at time.Time, id string, ok bool) {
	micros, id, found := strings.Cut(s, ".")
	if !found || id == "" || len(id) > maxQueryLen {
		return time.Time{}, "", false
	}
	n, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return time.Time{}, "", false
	}
	return time.UnixMicro(n).UTC(), id, true
}

// distinctTTL is how long a firehose list reuses its distinct values.
const distinctTTL = 30 * time.Second

// distinctLimit is the API's filter-options cap per facet.
const distinctLimit = 200

// distinctFetch reads one column's distinct values for a tenant scope (the
// repositories' DistinctValues: nil scope = every tenant, for an anchor).
type distinctFetch func(ctx context.Context, column string, limit int, accessibleClientIDs *[]string) ([]string, error)

// distinctCache keeps a list's distinct facet values for distinctTTL, per
// tenant scope (the values are scoped as the list is: docs/platform-bug-fixes.md
// bug 18): the SPA reads them once per visit, a live list on every request,
// and a DISTINCT over a projection is the costliest query of a request.
type distinctCache struct {
	mu      sync.Mutex
	entries map[string]distinctEntry
}

type distinctEntry struct {
	at   time.Time
	vals map[string][]string
}

// distinctScope is the tenant scope of a caller's facet values: nil (every
// tenant) for an anchor, else its own clients (platform rows are always in).
func distinctScope(ac *auth.AuthContext) *[]string {
	if ac.IsAnchor() {
		return nil
	}
	clients := slices.Clone(ac.Clients)
	slices.Sort(clients)
	return &clients
}

func distinctScopeKey(scope *[]string) string {
	if scope == nil {
		return "*"
	}
	return "c:" + strings.Join(*scope, ",")
}

// get returns the distinct values of each column for the scope (fetched
// together when that scope's entry is cold or stale).
func (c *distinctCache) get(ctx context.Context, fetch distinctFetch, scope *[]string, columns ...string) (map[string][]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := distinctScopeKey(scope)
	if e, ok := c.entries[key]; ok && time.Since(e.at) < distinctTTL {
		return e.vals, nil
	}
	vals := make(map[string][]string, len(columns))
	for _, col := range columns {
		v, err := fetch(ctx, col, distinctLimit, scope)
		if err != nil {
			return nil, usecase.Internal("REPO", "distinct "+col+" failed", err)
		}
		vals[col] = v
	}
	if c.entries == nil {
		c.entries = map[string]distinctEntry{}
	}
	for k, e := range c.entries { // drop stale scopes so the map stays small
		if time.Since(e.at) >= distinctTTL {
			delete(c.entries, k)
		}
	}
	c.entries[key] = distinctEntry{at: time.Now(), vals: vals}
	return vals, nil
}

// codeLevels derives the application, subdomain and aggregate options and
// the codes from "app:subdomain:aggregate:event" codes, each level narrowed
// by the values chosen above it; sorted, without duplicates.
func codeLevels(codes, apps, subs, aggs []string) [4][]string {
	chosen := [3][]string{apps, subs, aggs}
	under := func(seg []string, level int) bool {
		for i := range level {
			if len(chosen[i]) > 0 && (len(seg) <= i || !slices.Contains(chosen[i], seg[i])) {
				return false
			}
		}
		return true
	}
	var levels [4][]string
	for _, code := range codes {
		seg := strings.Split(code, ":")
		for level := range 3 {
			if len(seg) > level && under(seg, level) {
				levels[level] = append(levels[level], seg[level])
			}
		}
		if under(seg, 3) {
			levels[3] = append(levels[3], code)
		}
	}
	for i := range levels {
		slices.Sort(levels[i])
		levels[i] = slices.Compact(levels[i])
	}
	return levels
}

func allClients(ctx context.Context, store ClientStore) ([]client.Client, error) {
	if store == nil {
		return nil, nil
	}
	cs, err := store.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "list clients failed", err)
	}
	return cs, nil
}

func clientNames(cs []client.Client) map[string]string {
	names := make(map[string]string, len(cs))
	for _, c := range cs {
		names[string(c.ID)] = c.Name
	}
	return names
}

// clientFilterOptions is the SPA's ClientFilter (GET
// /bff/filter-options/clients): active clients the caller can access,
// sorted by name.
func clientFilterOptions(cs []client.Client, ac *auth.AuthContext) []kit.Option {
	var opts []kit.Option
	for _, c := range cs {
		if c.Status != client.StatusActive || (!ac.IsAnchor() && !ac.CanAccessClient(string(c.ID))) {
			continue
		}
		opts = append(opts, kit.Option{Value: string(c.ID), Label: c.Name})
	}
	slices.SortFunc(opts, func(a, b kit.Option) int { return strings.Compare(a.Label, b.Label) })
	return opts
}

func plainOptions(vals []string) []kit.Option {
	out := make([]kit.Option, 0, len(vals))
	for _, v := range vals {
		out = append(out, kit.Option{Value: v, Label: v})
	}
	return out
}

// withChosen appends the chosen values that are not options (labelled by
// labels when known, else the value itself), so a URL never silently loses
// a filter.
func withChosen(opts []kit.Option, chosen []string, labels map[string]string) []kit.Option {
	for _, c := range chosen {
		if !slices.ContainsFunc(opts, func(o kit.Option) bool { return o.Value == c }) {
			opts = append(opts, kit.Option{Value: c, Label: cmp.Or(labels[c], c)})
		}
	}
	return opts
}

// thousands is n with the SPA's toLocaleString() separators (en).
func thousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		return "-" + s
	}
	return s
}

// counted is plural with thousands separators ("10,000 jobs").
func counted(n int64, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return thousands(n) + " " + word + "s"
}
