package payloadstore

import (
	"container/list"
	"sync"
)

// cache holds resolved offloaded payloads by payload id, least recently used
// first out, bounded by the total of their lengths, so the jobs of one event
// and a retry share one read of the store (docs/payload-offload-design.md,
// section 5). A payload larger than an eighth of the bound is not cached. It
// keeps its own copies: what it is given and what it returns may be changed
// by the caller. Safe for concurrent use; a nil *cache caches nothing.
type cache struct {
	mu       sync.Mutex
	maxBytes int64
	maxEntry int64
	used     int64
	order    *list.List // of *cacheEntry, most recently used at the front
	entries  map[string]*list.Element
}

type cacheEntry struct {
	id   string
	data []byte
}

// newCache returns a cache of maxBytes, or nil (no cache) when maxBytes is
// not positive.
func newCache(maxBytes int64) *cache {
	if maxBytes <= 0 {
		return nil
	}
	return &cache{
		maxBytes: maxBytes,
		maxEntry: maxBytes / 8,
		order:    list.New(),
		entries:  map[string]*list.Element{},
	}
}

// get returns a copy of the payload cached under id.
func (c *cache) get(id string) ([]byte, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.entries[id]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(el)
	e, ok := el.Value.(*cacheEntry)
	if !ok {
		return nil, false
	}
	return append([]byte(nil), e.data...), true
}

// put caches a copy of data under id, evicting the least recently used
// payloads until it fits. A payload larger than an eighth of the cache is not
// cached.
func (c *cache) put(id string, data []byte) {
	if c == nil {
		return
	}
	n := int64(len(data))
	if n > c.maxEntry {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[id]; ok {
		return // a payload row never changes, so what is cached is current
	}
	for c.used+n > c.maxBytes {
		if !c.evictOldest() {
			break
		}
	}
	c.entries[id] = c.order.PushFront(&cacheEntry{id: id, data: append([]byte(nil), data...)})
	c.used += n
}

// evictOldest removes the least recently used entry; false when empty.
func (c *cache) evictOldest() bool {
	el := c.order.Back()
	if el == nil {
		return false
	}
	c.order.Remove(el)
	if e, ok := el.Value.(*cacheEntry); ok {
		delete(c.entries, e.id)
		c.used -= int64(len(e.data))
	}
	return true
}

// size reports the cached bytes and entries: for tests.
func (c *cache) size() (bytes int64, entries int) {
	if c == nil {
		return 0, 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.used, len(c.entries)
}
