package api

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// The filter-options facets are DISTINCT reads over the created_at window. They are
// bounded by the window (24 hours by default, 31 days at most), but they are the most
// expensive reads the console issues and it issues them on every page open, so the
// answer is kept for a short time.
//
// Keyed by the caller's tenant scope (a client user's values are those of platform jobs
// and its own tenants, never another's) and by the window cut to whole buckets, so two
// opens a few seconds apart, whose "last 24 hours" differ by those seconds, share an
// entry. The window echoed with a cached answer is the one the first request computed.
const (
	// filterOptionsTTL is how long a computed answer is reused.
	filterOptionsTTL = time.Minute
	// filterOptionsBucket is the granularity the window's ends are cut to for the key.
	filterOptionsBucket = 5 * time.Minute
	// filterOptionsMaxEntries bounds the cache; past it the oldest entries go.
	filterOptionsMaxEntries = 256
)

type filterOptionsEntry struct {
	at   time.Time
	resp DispatchJobFilterOptionsResponse
}

// filterOptionsCache is a small TTL cache of filter-options answers. The zero value is
// ready to use.
type filterOptionsCache struct {
	mu      sync.Mutex
	entries map[string]filterOptionsEntry
}

func filterOptionsKey(scope *[]string, from, to time.Time) string {
	var b strings.Builder
	if scope == nil {
		b.WriteString("*")
	} else {
		ids := append([]string(nil), *scope...)
		sort.Strings(ids)
		b.WriteString("c:" + strings.Join(ids, ","))
	}
	b.WriteString("|" + from.UTC().Truncate(filterOptionsBucket).Format(time.RFC3339))
	b.WriteString("|" + to.UTC().Truncate(filterOptionsBucket).Format(time.RFC3339))
	return b.String()
}

func (c *filterOptionsCache) get(key string, now time.Time) (DispatchJobFilterOptionsResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || now.Sub(e.at) >= filterOptionsTTL {
		return DispatchJobFilterOptionsResponse{}, false
	}
	return e.resp, true
}

func (c *filterOptionsCache) put(key string, now time.Time, resp DispatchJobFilterOptionsResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]filterOptionsEntry{}
	}
	for k, e := range c.entries { // drop what has expired so the map stays small
		if now.Sub(e.at) >= filterOptionsTTL {
			delete(c.entries, k)
		}
	}
	for len(c.entries) >= filterOptionsMaxEntries {
		oldest, first := "", true
		var at time.Time
		for k, e := range c.entries {
			if first || e.at.Before(at) {
				oldest, at, first = k, e.at, false
			}
		}
		delete(c.entries, oldest)
	}
	c.entries[key] = filterOptionsEntry{at: now, resp: resp}
}
