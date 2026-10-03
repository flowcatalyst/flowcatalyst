package router

import "sync"

// maxTargetCacheEntries bounds the per-target parse cache. Targets are few
// (one per subscription endpoint); past the cap new targets are computed
// each time and not stored, so growth is bounded.
const maxTargetCacheEntries = 4096

type hostKeyResult struct {
	key HostKey
	err error
}

var (
	breakerKeyCache sync.Map // string -> string
	breakerKeyCount counterBox
	hostKeyCache    sync.Map // string -> hostKeyResult
	hostKeyCount    counterBox
)

type counterBox struct {
	mu sync.Mutex
	n  int
}

// reserve reports whether another entry may be stored, counting it if so.
func (c *counterBox) reserve() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.n >= maxTargetCacheEntries {
		return false
	}
	c.n++
	return true
}

// cachedBreakerKey is breakerKey memoised per target string.
func cachedBreakerKey(target string) string {
	if v, ok := breakerKeyCache.Load(target); ok {
		return v.(string)
	}
	k := breakerKey(target)
	if breakerKeyCount.reserve() {
		breakerKeyCache.Store(target, k)
	}
	return k
}

// cachedHostKey is HostKeyFromURL memoised per target string (errors too).
func cachedHostKey(target string) (HostKey, error) {
	if v, ok := hostKeyCache.Load(target); ok {
		r := v.(hostKeyResult)
		return r.key, r.err
	}
	k, err := HostKeyFromURL(target)
	if hostKeyCount.reserve() {
		hostKeyCache.Store(target, hostKeyResult{k, err})
	}
	return k, err
}
