package runner

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// Cache rules of the platform-token capability (docs/function-platform-token.md
// section 5): a token is served while more than max(minTokenRemaining, a tenth
// of its lifetime) remains, so a guest never gets one about to expire under it.
const (
	minTokenRemaining = 60 * time.Second
	// tokenFetchTimeout bounds one platform fetch. The fetch outlives the
	// caller that started it (others share its result), so the caller's own
	// deadline cannot be the bound.
	tokenFetchTimeout = 30 * time.Second
)

// tokenCache holds one FlowCatalyst access token per function and shares one
// fetch between concurrent callers. The tokens are live credentials: nothing
// here logs them, and the error messages never carry one.
type tokenCache struct {
	src TokenSource
	now func() time.Time

	mu      sync.Mutex
	entries map[string]tokenEntry // by function id
	flights map[string]*tokenFlight
}

type tokenEntry struct {
	token   string
	fetched time.Time
	expires time.Time
}

// fresh reports whether enough lifetime remains to hand the token out.
func (e tokenEntry) fresh(now time.Time) bool {
	remaining := e.expires.Sub(now)
	return remaining > max(minTokenRemaining, e.expires.Sub(e.fetched)/10)
}

// tokenFlight is one in-progress fetch for a function.
type tokenFlight struct {
	done    chan struct{}
	entry   tokenEntry
	err     *abi.Error
	dropped bool // the function left desired state while the fetch ran
}

func newTokenCache(src TokenSource, now func() time.Time) *tokenCache {
	return &tokenCache{src: src, now: now, entries: map[string]tokenEntry{}, flights: map[string]*tokenFlight{}}
}

// get returns the function's token: from the cache while fresh, otherwise from
// one fetch shared by every concurrent caller. refresh skips the cache and
// replaces the entry. A failed fetch is not cached.
func (c *tokenCache) get(ctx context.Context, fnID string, version int, refresh bool) (tokenEntry, *abi.Error) {
	c.mu.Lock()
	if e, ok := c.entries[fnID]; ok && !refresh && e.fresh(c.now()) {
		c.mu.Unlock()
		return e, nil
	}
	fl, running := c.flights[fnID]
	if !running {
		fl = &tokenFlight{done: make(chan struct{})}
		c.flights[fnID] = fl
		// The fetch is detached from this caller: if the leader's invocation
		// ends, the callers sharing its result must still get one.
		go c.fetch(context.WithoutCancel(ctx), fnID, version, fl)
	}
	c.mu.Unlock()

	select {
	case <-fl.done:
		return fl.entry, fl.err
	case <-ctx.Done():
		return tokenEntry{}, abi.Errorf(abi.CodeDeadline, "the invocation's deadline passed while fetching a token")
	}
}

func (c *tokenCache) fetch(ctx context.Context, fnID string, version int, fl *tokenFlight) {
	ctx, cancel := context.WithTimeout(ctx, tokenFetchTimeout)
	defer cancel()
	resp, aerr := c.src.Token(ctx, control.TokenRequest{FunctionID: fnID, Version: version})
	now := c.now()

	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case aerr != nil:
		fl.err = aerr
	case resp == nil || resp.AccessToken == "":
		fl.err = abi.Errorf(abi.CodeUnavailable, "the platform's token answer was empty")
	default:
		fl.entry = tokenEntry{token: resp.AccessToken, fetched: now, expires: now.Add(time.Duration(resp.ExpiresIn) * time.Second)}
		if !fl.dropped {
			c.entries[fnID] = fl.entry
		}
	}
	delete(c.flights, fnID)
	close(fl.done)
}

// drop forgets a function's token, and keeps a fetch still running for it from
// being stored: called when the function leaves the desired document.
func (c *tokenCache) drop(fnID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, fnID)
	if fl, ok := c.flights[fnID]; ok {
		fl.dropped = true
	}
}

// platformToken serves the platform.token op: the version must declare it; the
// platform checks again. The result meta carries the token, so it is never
// logged here.
func (c *capabilities) platformToken(ctx context.Context, meta []byte) ([]byte, []byte, *abi.Error) {
	if !c.ver.describe.PlatformToken {
		return nil, nil, abi.Errorf(abi.CodeNotDeclared, "this version does not declare platformToken")
	}
	var call abi.PlatformTokenCall
	if len(bytes.TrimSpace(meta)) > 0 {
		var aerr *abi.Error
		if call, aerr = decode[abi.PlatformTokenCall](meta); aerr != nil {
			return nil, nil, aerr
		}
	}
	e, aerr := c.tokens.get(ctx, c.fn.id, c.ver.number, call.Refresh)
	if aerr != nil {
		return nil, nil, aerr
	}
	return encode(abi.PlatformTokenResult{AccessToken: e.token, ExpiresAt: e.expires.UnixMilli()}), nil, nil
}
