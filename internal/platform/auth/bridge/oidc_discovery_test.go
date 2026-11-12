package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
)

// OIDC discovery must not run under the bridge's lock, must be shared between
// concurrent resolves of one provider, must be bounded in time, size and
// redirects, and a failure must be remembered for a short window.

// discoveryServer is a fake IdP discovery endpoint. mode picks its behaviour;
// hits counts discovery requests; release unblocks "block" handlers.
type discoveryServer struct {
	srv     *httptest.Server
	hits    atomic.Int32
	arrived chan struct{} // receives once per discovery request
	release chan struct{}
	once    sync.Once
	mode    atomic.Value // string
}

func newDiscoveryServer(t *testing.T, mode string) *discoveryServer {
	t.Helper()
	d := &discoveryServer{arrived: make(chan struct{}, 16), release: make(chan struct{})}
	d.mode.Store(mode)
	d.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/.well-known/openid-configuration") {
			http.NotFound(w, r)
			return
		}
		d.hits.Add(1)
		d.arrived <- struct{}{}
		switch d.mode.Load().(string) {
		case "block": // answers only once released
			select {
			case <-d.release:
			case <-r.Context().Done():
				return
			}
		case "hang": // never answers
			select {
			case <-d.release:
			case <-r.Context().Done():
			}
			return
		case "fail":
			http.Error(w, "down", http.StatusInternalServerError)
			return
		case "redirect":
			http.Redirect(w, r, r.URL.Path, http.StatusFound)
			return
		case "huge":
			// Chunked (no Content-Length), well over the cap.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"issuer":"` + d.srv.URL + `","pad":"`))
			chunk := []byte(strings.Repeat("a", 64*1024))
			for range 32 { // 2 MiB
				if _, err := w.Write(chunk); err != nil {
					return
				}
			}
			_, _ = w.Write([]byte(`"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 d.srv.URL,
			"authorization_endpoint": d.srv.URL + "/auth",
			"token_endpoint":         d.srv.URL + "/token",
			"jwks_uri":               d.srv.URL + "/jwks",
		})
	}))
	t.Cleanup(func() { d.unblock(); d.srv.Close() })
	return d
}

func (d *discoveryServer) unblock() { d.once.Do(func() { close(d.release) }) }

func (d *discoveryServer) idp(id string) *identityprovider.IdentityProvider {
	issuer, client := d.srv.URL, "client-"+id
	return &identityprovider.IdentityProvider{ID: id, Code: id, Type: identityprovider.TypeOIDC, OIDCIssuerURL: &issuer, OIDCClientID: &client}
}

func (d *discoveryServer) waitArrival(t *testing.T) {
	t.Helper()
	select {
	case <-d.arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("the discovery request never arrived")
	}
}

type resolveResult struct {
	r   *resolved
	err error
}

func resolveAsync(ctx context.Context, b *Bridge, idp *identityprovider.IdentityProvider) <-chan resolveResult {
	ch := make(chan resolveResult, 1)
	go func() {
		r, err := b.resolveIdP(ctx, idp)
		ch <- resolveResult{r, err}
	}()
	return ch
}

func mustReceive(t *testing.T, ch <-chan resolveResult, within time.Duration, what string) resolveResult {
	t.Helper()
	select {
	case res := <-ch:
		return res
	case <-time.After(within):
		t.Fatalf("%s: no result within %s", what, within)
		return resolveResult{}
	}
}

// TB1: a discovery that stalls for provider X does not delay a sign-in through
// the already-cached provider Y.
func TestResolveIdP_CacheHitDoesNotWaitOnAnotherProvidersDiscovery(t *testing.T) {
	b := NewBridge(nil, nil, nil)
	y := newDiscoveryServer(t, "ok")
	x := newDiscoveryServer(t, "block")

	_, err := b.resolveIdP(context.Background(), y.idp("tb1-y"))
	require.NoError(t, err, "Y is discovered and cached")

	xres := resolveAsync(context.Background(), b, x.idp("tb1-x"))
	x.waitArrival(t) // X's discovery is now in flight and blocked

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	start := time.Now()
	r, err := b.resolveIdP(ctx, y.idp("tb1-y"))
	require.NoError(t, err, "a cache hit for Y must not wait for X's discovery")
	assert.NotNil(t, r)
	assert.Less(t, time.Since(start), 500*time.Millisecond)

	x.unblock()
	got := mustReceive(t, xres, 5*time.Second, "X after release")
	require.NoError(t, got.err)
}

// TB1 (second form): a first-time provider Z is not held up by X either.
func TestResolveIdP_DifferentProviderDiscoveriesRunConcurrently(t *testing.T) {
	b := NewBridge(nil, nil, nil)
	x := newDiscoveryServer(t, "block")
	z := newDiscoveryServer(t, "ok")

	xres := resolveAsync(context.Background(), b, x.idp("tb1b-x"))
	x.waitArrival(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := b.resolveIdP(ctx, z.idp("tb1b-z"))
	require.NoError(t, err)

	x.unblock()
	require.NoError(t, mustReceive(t, xres, 5*time.Second, "X").err)
}

// TB2: concurrent resolves of one uncached provider share one discovery.
func TestResolveIdP_ConcurrentResolvesShareOneDiscovery(t *testing.T) {
	b := NewBridge(nil, nil, nil)
	x := newDiscoveryServer(t, "block")
	idp := x.idp("tb2")

	first := resolveAsync(context.Background(), b, idp)
	x.waitArrival(t)
	second := resolveAsync(context.Background(), b, idp)
	third := resolveAsync(context.Background(), b, idp)
	time.Sleep(150 * time.Millisecond) // let the others reach the wait
	x.unblock()

	r1 := mustReceive(t, first, 5*time.Second, "first")
	r2 := mustReceive(t, second, 5*time.Second, "second")
	r3 := mustReceive(t, third, 5*time.Second, "third")
	require.NoError(t, r1.err)
	require.NoError(t, r2.err)
	require.NoError(t, r3.err)
	assert.Same(t, r1.r, r2.r)
	assert.Same(t, r1.r, r3.r)
	assert.EqualValues(t, 1, x.hits.Load(), "exactly one discovery request for three resolves")
}

// TB3: a discovery response over 1 MiB fails.
func TestResolveIdP_OversizedDiscoveryResponseFails(t *testing.T) {
	b := NewBridge(nil, nil, nil)
	x := newDiscoveryServer(t, "huge")
	_, err := b.resolveIdP(context.Background(), x.idp("tb3-huge"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "size limit")
}

// TB3: a server that never responds fails within the client timeout.
func TestResolveIdP_UnresponsiveDiscoveryFailsWithinTheClientTimeout(t *testing.T) {
	b := NewBridge(nil, nil, nil)
	b.SetHTTPClient(newBoundedClient(nil, 300*time.Millisecond))
	x := newDiscoveryServer(t, "hang")

	start := time.Now()
	_, err := b.resolveIdP(context.Background(), x.idp("tb3-hang"))
	require.Error(t, err)
	assert.Less(t, time.Since(start), 3*time.Second, "bounded by the client timeout, not the caller")
}

// TB3: more than three redirects are not followed.
func TestResolveIdP_RedirectLoopIsStopped(t *testing.T) {
	b := NewBridge(nil, nil, nil)
	x := newDiscoveryServer(t, "redirect")
	_, err := b.resolveIdP(context.Background(), x.idp("tb3-redir"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too many redirects")
	assert.LessOrEqual(t, int(x.hits.Load()), maxDiscoveryRedirects+1)
}

// TB3: a waiter whose context ends returns promptly with its own error and
// leaves the discovery to finish for the others.
func TestResolveIdP_WaiterHonoursItsOwnContext(t *testing.T) {
	b := NewBridge(nil, nil, nil)
	x := newDiscoveryServer(t, "block")
	idp := x.idp("tb3-waiter")

	owner := resolveAsync(context.Background(), b, idp)
	x.waitArrival(t)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := b.resolveIdP(ctx, idp)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), time.Second)

	x.unblock()
	require.NoError(t, mustReceive(t, owner, 5*time.Second, "owner").err, "the discovery finished for the others")
	_, err = b.resolveIdP(context.Background(), idp)
	require.NoError(t, err, "and is cached")
	assert.EqualValues(t, 1, x.hits.Load())
}

// TB3: the cancelled FIRST requester does not cancel the shared discovery.
func TestResolveIdP_FirstRequesterCancellingDoesNotFailTheOthers(t *testing.T) {
	b := NewBridge(nil, nil, nil)
	x := newDiscoveryServer(t, "block")
	idp := x.idp("tb3-owner-cancel")

	ownerCtx, cancelOwner := context.WithCancel(context.Background())
	owner := resolveAsync(ownerCtx, b, idp)
	x.waitArrival(t)
	other := resolveAsync(context.Background(), b, idp)
	time.Sleep(100 * time.Millisecond)
	cancelOwner()
	require.ErrorIs(t, mustReceive(t, owner, 2*time.Second, "cancelled owner").err, context.Canceled)

	x.unblock()
	require.NoError(t, mustReceive(t, other, 5*time.Second, "other").err)
}

// TB4: after a failed discovery, further attempts within the window return the
// remembered error with no network call; after the window it tries again.
func TestResolveIdP_FailedDiscoveryIsRememberedForTheWindow(t *testing.T) {
	b := NewBridge(nil, nil, nil)
	clock := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	b.now = func() time.Time { return clock }
	x := newDiscoveryServer(t, "fail")
	idp := x.idp("tb4")

	_, err1 := b.resolveIdP(context.Background(), idp)
	require.Error(t, err1)
	require.EqualValues(t, 1, x.hits.Load())

	clock = clock.Add(29 * time.Second)
	_, err2 := b.resolveIdP(context.Background(), idp)
	require.Error(t, err2)
	assert.Equal(t, err1.Error(), err2.Error(), "the remembered error")
	assert.EqualValues(t, 1, x.hits.Load(), "no network call inside the window")

	clock = clock.Add(2 * time.Second) // 31s after the failure
	_, err3 := b.resolveIdP(context.Background(), idp)
	require.Error(t, err3)
	assert.EqualValues(t, 2, x.hits.Load(), "the window elapsed: it tries again")

	// A success clears the memory.
	x.mode.Store("ok")
	clock = clock.Add(31 * time.Second)
	r, err4 := b.resolveIdP(context.Background(), idp)
	require.NoError(t, err4)
	assert.NotNil(t, r)
	assert.Empty(t, b.failed, "a success clears the negative cache")
}
