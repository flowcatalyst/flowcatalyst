package client

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingRepo is a fake identifierLookup that counts FindByID calls and lets
// a test change what it returns for a given id mid-run — the shape T6 asks
// for ("a fake/counting repository") without mocking pgx: NewCachedIdentifierResolver
// is exercised against the identifierLookup interface, not against Postgres.
type countingRepo struct {
	mu    sync.Mutex
	calls map[string]int
	byID  map[string]*Client
	err   map[string]error
}

func newCountingRepo() *countingRepo {
	return &countingRepo{calls: map[string]int{}, byID: map[string]*Client{}, err: map[string]error{}}
}

func (r *countingRepo) FindByID(_ context.Context, id string) (*Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls[id]++
	if err, ok := r.err[id]; ok {
		return nil, err
	}
	return r.byID[id], nil // nil, nil = not found, matching Repository's contract
}

func (r *countingRepo) callCount(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls[id]
}

func (r *countingRepo) set(id string, c *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byID[id] = c
}

// T6a: two deliveries for the same (resolvable) client perform exactly one
// repository lookup. A resolver that queried per-call would leave callCount
// at 2 here.
func TestCachedIdentifierResolver_HitIsCachedForProcessLife(t *testing.T) {
	repo := newCountingRepo()
	repo.set("clt_1", &Client{ID: "clt_1", Identifier: "acme"})
	resolve := NewCachedIdentifierResolver(repo, time.Hour)

	id1, ok1 := resolve(context.Background(), "clt_1")
	id2, ok2 := resolve(context.Background(), "clt_1")

	require.True(t, ok1)
	require.True(t, ok2)
	assert.Equal(t, "acme", id1)
	assert.Equal(t, "acme", id2)
	assert.Equal(t, 1, repo.callCount("clt_1"), "second delivery must hit the cache, not the repository")
}

// T6b: a client that misses once (not yet created) resolves once it exists
// and the negative window elapses — a miss must not be cached forever.
func TestCachedIdentifierResolver_MissIsNotCachedForever(t *testing.T) {
	repo := newCountingRepo() // clt_new not seeded yet: first call misses
	const negativeTTL = 20 * time.Millisecond
	resolve := NewCachedIdentifierResolver(repo, negativeTTL)

	_, ok := resolve(context.Background(), "clt_new")
	require.False(t, ok, "unseeded client is an initial miss")
	assert.Equal(t, 1, repo.callCount("clt_new"))

	// Client now exists (e.g. created after the first delivery attempt), but
	// within the negative window a re-resolve must still serve the cached
	// miss without re-querying.
	repo.set("clt_new", &Client{ID: "clt_new", Identifier: "globex"})
	_, ok = resolve(context.Background(), "clt_new")
	require.False(t, ok, "still within the negative-cache window")
	assert.Equal(t, 1, repo.callCount("clt_new"), "must not re-query before the negative window elapses")

	time.Sleep(negativeTTL + 30*time.Millisecond)

	id, ok := resolve(context.Background(), "clt_new")
	require.True(t, ok, "must resolve once the negative window has elapsed")
	assert.Equal(t, "globex", id)
	assert.Equal(t, 2, repo.callCount("clt_new"), "exactly one re-query after the window elapsed")

	// And the now-positive hit is cached for good, per the immutable-identifier
	// policy: further calls must not re-query even long after negativeTTL.
	time.Sleep(negativeTTL + 30*time.Millisecond)
	_, _ = resolve(context.Background(), "clt_new")
	assert.Equal(t, 2, repo.callCount("clt_new"), "a resolved hit is cached for the process's life, not re-checked")
}

// A repository error resolves to "unknown" (ok=false) rather than failing
// the caller, and is logged at most once per client — proven here by the
// call count staying at one across repeated failures within the window,
// mirroring the miss-caching behaviour above.
func TestCachedIdentifierResolver_RepositoryErrorResolvesUnknown(t *testing.T) {
	repo := newCountingRepo()
	repo.mu.Lock()
	repo.err["clt_boom"] = errors.New("connection reset")
	repo.mu.Unlock()
	resolve := NewCachedIdentifierResolver(repo, time.Hour)

	id, ok := resolve(context.Background(), "clt_boom")
	assert.False(t, ok)
	assert.Equal(t, "", id)

	id, ok = resolve(context.Background(), "clt_boom")
	assert.False(t, ok)
	assert.Equal(t, "", id)
	assert.Equal(t, 1, repo.callCount("clt_boom"), "the failure is cached for the negative window too")
}

func TestCachedIdentifierResolver_EmptyClientIDNeverQueries(t *testing.T) {
	repo := newCountingRepo()
	resolve := NewCachedIdentifierResolver(repo, time.Hour)

	_, ok := resolve(context.Background(), "")
	assert.False(t, ok)
	assert.Equal(t, 0, repo.callCount(""))
}
