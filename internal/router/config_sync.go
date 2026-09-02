package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// ConfigSource fetches the live RouterConfig from one or more remote
// endpoints. When FLOWCATALYST_CONFIG_URL is comma-separated, all URLs are
// fetched in parallel (each with its own retry) and the results are merged
// (union, first-wins). A source that fails contributes its last
// successfully fetched definition instead of dropping out — see
// recordSuccess/recordFailure — so per-URL failures are tolerated as long as
// every URL has succeeded at least once; only a source that has NEVER
// succeeded is dropped from the merge on failure, same as before.
type ConfigSource struct {
	URLs   []string
	Client *http.Client
	// MaxAttempts/RetryDelay govern per-URL retry (defaults: 12 / 5s).
	MaxAttempts int
	RetryDelay  time.Duration

	mu   sync.Mutex
	last []byte // last merged config (marshaled) for change detection

	// warnings is optional (SetWarnings); nil → the stale-config warning is
	// skipped, everything else is unaffected.
	warnings atomic.Pointer[WarningService]

	// cacheMu guards cache and staleWarnIDs together — every read/write
	// touches the pair, and the write path (recordFailure) must resolve
	// "already warned for this streak" and "record the new warning id"
	// as one atomic step, so a plain mutex (not RWMutex) keeps that simple.
	cacheMu sync.Mutex
	// cache holds each URL's last successfully fetched RouterConfig
	// (last-known-good). Populated on every successful fetch; a source with
	// no entry has never succeeded.
	cache map[string]common.RouterConfig
	// staleWarnIDs holds the WarningService id of the currently-active
	// "serving stale config" warning per URL, so a failure streak emits
	// exactly one warning (not one per tick) and recovery can resolve it.
	staleWarnIDs map[string]string
}

// NewConfigSource builds a source from a (possibly comma-separated) URL.
func NewConfigSource(url string) *ConfigSource {
	var urls []string
	for _, u := range strings.Split(url, ",") {
		if u = strings.TrimSpace(u); u != "" {
			urls = append(urls, u)
		}
	}
	return &ConfigSource{
		URLs:        urls,
		Client:      &http.Client{Timeout: 10 * time.Second},
		MaxAttempts: 12,
		RetryDelay:  5 * time.Second,
	}
}

// ErrUnchanged is returned by Fetch when the merged config matches the
// previous fetch — callers can skip reconfigure in that case.
var ErrUnchanged = errors.New("config unchanged")

type sourceConfig struct {
	url string
	cfg common.RouterConfig
}

// SetWarnings wires a WarningService so a source serving stale (cached)
// config because its live fetch is failing surfaces a CONFIGURATION warning
// on /warnings. Opt-in; nil detaches. Set once at startup before Watch runs.
func (cs *ConfigSource) SetWarnings(w *WarningService) { cs.warnings.Store(w) }

// Fetch fetches every configured URL in parallel (each retried up to
// MaxAttempts) and returns the merged config. A URL that fails but has a
// cached last-known-good definition (see recordSuccess) contributes that
// definition instead of dropping out, so its pools/queues stay in the merged
// result — and its consumers keep running — while it recovers; a
// CONFIGURATION warning is raised once per failure streak and cleared on
// recovery. Returns ErrUnchanged when the merged result (live + cached)
// matches the previous fetch, or an error only when every URL has failed AND
// none has ever succeeded (first boot, nothing to fall back to).
func (cs *ConfigSource) Fetch(ctx context.Context) (*common.RouterConfig, error) {
	if len(cs.URLs) == 0 {
		return nil, errors.New("config: no URLs configured")
	}

	cfgs := make([]*common.RouterConfig, len(cs.URLs))
	errs := make([]error, len(cs.URLs))
	var wg sync.WaitGroup
	for i, u := range cs.URLs {
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			cfgs[i], errs[i] = cs.fetchWithRetry(ctx, u)
		}(i, u)
	}
	wg.Wait()

	// Collect successes in URL order so the merge is deterministic
	// (first-wins). A failing URL with a cache falls back to its
	// last-known-good definition rather than dropping out.
	var ok []sourceConfig
	for i, u := range cs.URLs {
		if errs[i] == nil {
			cs.recordSuccess(u, *cfgs[i])
			ok = append(ok, sourceConfig{url: u, cfg: *cfgs[i]})
			continue
		}
		slog.Warn("config fetch failed for source", "url", u, "err", errs[i])
		if cached, hasCache := cs.staleConfig(u); hasCache {
			cs.recordFailure(u, true)
			ok = append(ok, sourceConfig{url: u, cfg: cached})
			continue
		}
		cs.recordFailure(u, false)
	}
	if len(ok) == 0 {
		return nil, fmt.Errorf("config: all %d source(s) failed", len(cs.URLs))
	}

	merged := mergeConfigs(ok)

	// Change detection on the merged config (marshaled).
	body, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("config marshal: %w", err)
	}
	cs.mu.Lock()
	unchanged := len(cs.last) > 0 && bytesEqual(cs.last, body)
	cs.last = body
	cs.mu.Unlock()
	if unchanged {
		return nil, ErrUnchanged
	}
	return &merged, nil
}

// fetchWithRetry fetches a single URL, retrying up to MaxAttempts with
// RetryDelay between attempts (ctx-aware).
func (cs *ConfigSource) fetchWithRetry(ctx context.Context, url string) (*common.RouterConfig, error) {
	var lastErr error
	for attempt := 1; attempt <= cs.MaxAttempts; attempt++ {
		cfg, err := cs.fetchOnce(ctx, url)
		if err == nil {
			return cfg, nil
		}
		lastErr = err
		if attempt < cs.MaxAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(cs.RetryDelay):
			}
		}
	}
	return nil, fmt.Errorf("after %d attempts: %w", cs.MaxAttempts, lastErr)
}

func (cs *ConfigSource) fetchOnce(ctx context.Context, url string) (*common.RouterConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := cs.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("config fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("config fetch: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var cfg common.RouterConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("config decode: %w", err)
	}
	return &cfg, nil
}

// recordSuccess caches url's freshly fetched config as its last-known-good
// definition and, if url was in a failure streak, resolves the stale-config
// warning and clears the streak — recovery is "the next successful fetch",
// not a separate health check.
func (cs *ConfigSource) recordSuccess(url string, cfg common.RouterConfig) {
	cs.cacheMu.Lock()
	if cs.cache == nil {
		cs.cache = make(map[string]common.RouterConfig)
	}
	cs.cache[url] = cfg
	warnID, wasFailing := "", false
	if cs.staleWarnIDs != nil {
		warnID, wasFailing = cs.staleWarnIDs[url]
		if wasFailing {
			delete(cs.staleWarnIDs, url)
		}
	}
	cs.cacheMu.Unlock()
	if !wasFailing {
		return
	}
	slog.Info("config source recovered; no longer serving stale config", "url", url)
	if w := cs.warnings.Load(); w != nil && warnID != "" {
		w.Remove(warnID)
	}
}

// staleConfig returns url's last-known-good cached config, if any.
func (cs *ConfigSource) staleConfig(url string) (common.RouterConfig, bool) {
	cs.cacheMu.Lock()
	defer cs.cacheMu.Unlock()
	cfg, ok := cs.cache[url]
	return cfg, ok
}

// recordFailure marks url as currently failing and, the first time in a
// failure streak, raises a CONFIGURATION warning — once per streak, not once
// per tick, so a source down for an hour doesn't flood /warnings on every
// poll. servingStale distinguishes the message: with a cache, url is
// contributing its last-known-good definition to the merge (R-30); without
// one (never succeeded), it is simply absent, same as before this ruling.
func (cs *ConfigSource) recordFailure(url string, servingStale bool) {
	cs.cacheMu.Lock()
	if cs.staleWarnIDs == nil {
		cs.staleWarnIDs = make(map[string]string)
	}
	_, already := cs.staleWarnIDs[url]
	cs.cacheMu.Unlock()
	if already {
		return
	}
	w := cs.warnings.Load()
	if w == nil {
		return
	}
	msg := fmt.Sprintf("config source %q is failing", url)
	if servingStale {
		msg += "; serving last-known-good config"
	}
	id := w.Add(WarningCategoryConfiguration, WarningWarning, msg, "ConfigSource")
	cs.cacheMu.Lock()
	cs.staleWarnIDs[url] = id
	cs.cacheMu.Unlock()
}

// mergeConfigs unions multiple source configs, first-wins: a pool is keyed by
// code, a queue by URI; the first source to define a key wins, later
// duplicates are dropped (with a warning on a value conflict). A single
// source passes through unchanged.
func mergeConfigs(sources []sourceConfig) common.RouterConfig {
	if len(sources) == 1 {
		return sources[0].cfg
	}
	var merged common.RouterConfig
	poolOrigin := map[string]string{}
	queueOrigin := map[string]string{}
	for _, s := range sources {
		for _, p := range s.cfg.ProcessingPools {
			if orig, seen := poolOrigin[p.Code]; seen {
				if conflictingPool(merged.ProcessingPools, p) {
					slog.Warn("duplicate pool with conflicting values — keeping first",
						"pool_code", p.Code, "kept_source", orig, "dropped_source", s.url)
				}
				continue
			}
			poolOrigin[p.Code] = s.url
			merged.ProcessingPools = append(merged.ProcessingPools, p)
		}
		for _, q := range s.cfg.Queues {
			if orig, seen := queueOrigin[q.URI]; seen {
				if conflictingQueue(merged.Queues, q) {
					slog.Warn("duplicate queue with conflicting values — keeping first",
						"queue_uri", q.URI, "kept_source", orig, "dropped_source", s.url)
				}
				continue
			}
			queueOrigin[q.URI] = s.url
			merged.Queues = append(merged.Queues, q)
		}
	}
	return merged
}

func conflictingPool(existing []common.PoolConfig, p common.PoolConfig) bool {
	for _, e := range existing {
		if e.Code == p.Code {
			return e.Concurrency != p.Concurrency || !u32PtrEqual(e.RateLimitPerMinute, p.RateLimitPerMinute)
		}
	}
	return false
}

func conflictingQueue(existing []common.QueueConfig, q common.QueueConfig) bool {
	for _, e := range existing {
		if e.URI == q.URI {
			return e.Name != q.Name || e.Connections != q.Connections || e.VisibilityTimeout != q.VisibilityTimeout
		}
	}
	return false
}

func u32PtrEqual(a, b *uint32) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// Watch polls cs every interval and applies the result to manager.
// Blocks until ctx is cancelled. warnings is optional (nil is fine — several
// tests construct the watcher bare); when set, apply() raises a
// CONFIGURATION warning on failure and clears it on recovery (R-30).
func Watch(ctx context.Context, cs *ConfigSource, manager *Manager, interval time.Duration, warnings *WarningService) {
	tick := time.NewTicker(interval)
	defer tick.Stop()

	// watchWarnID tracks the CONFIGURATION warning (if any) raised by this
	// watcher's own apply() failures — distinct from ConfigSource's
	// per-URL "serving stale config" warnings (Fetch already raises those
	// itself). This covers the two cases a per-URL warning can't: every
	// source failing with nothing cached anywhere (Fetch itself errors),
	// and a fetch that succeeds but Reconfigure then rejects (e.g. a
	// malformed queue URI). Both leave the last-applied config running
	// untouched — see ConfigSource's doc comment for why that's correct —
	// the warning exists purely so an operator watching /warnings can see
	// the router isn't keeping up with its config source. One warning per
	// failure streak, not one per tick; cleared the next time apply()
	// succeeds end to end.
	var watchWarnID string

	apply := func() {
		cfg, err := cs.Fetch(ctx)
		if errors.Is(err, ErrUnchanged) {
			clearWatchWarning(warnings, &watchWarnID)
			return
		}
		if err != nil {
			slog.Warn("config fetch failed", "err", err)
			raiseWatchWarning(warnings, &watchWarnID, fmt.Sprintf("config fetch failed: %v", err))
			return
		}
		if err := manager.Reconfigure(ctx, *cfg); err != nil {
			slog.Warn("manager reconfigure failed", "err", err)
			raiseWatchWarning(warnings, &watchWarnID, fmt.Sprintf("manager reconfigure failed: %v", err))
			return
		}
		clearWatchWarning(warnings, &watchWarnID)
	}

	apply()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			apply()
		}
	}
}

// raiseWatchWarning records msg as the config watcher's CONFIGURATION
// warning, once per failure streak: a call while *id is already set is a
// no-op — the same failure persisting, not a new one. nil-safe.
func raiseWatchWarning(warnings *WarningService, id *string, msg string) {
	if warnings == nil || *id != "" {
		return
	}
	*id = warnings.Add(WarningCategoryConfiguration, WarningWarning, msg, "ConfigWatcher")
}

// clearWatchWarning resolves the config watcher's warning, if any, on
// recovery. nil-safe.
func clearWatchWarning(warnings *WarningService, id *string) {
	if *id == "" {
		return
	}
	if warnings != nil {
		warnings.Remove(*id)
	}
	*id = ""
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
