package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// tokenSrc is a TokenSource the tests steer: it counts fetches, can block them
// and answers with numbered tokens of a fixed lifetime.
type tokenSrc struct {
	calls     atomic.Int32
	expiresIn int64
	err       *abi.Error
	gate      chan struct{} // when set, a fetch waits for it to close
}

func (s *tokenSrc) Token(_ context.Context, _ control.TokenRequest) (*control.TokenResponse, *abi.Error) {
	n := s.calls.Add(1)
	if s.gate != nil {
		<-s.gate
	}
	if s.err != nil {
		return nil, s.err
	}
	return &control.TokenResponse{AccessToken: "tok-" + strconv.Itoa(int(n)), TokenType: "Bearer", ExpiresIn: s.expiresIn}, nil
}

// tokenClock is a settable time source.
type tokenClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *tokenClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *tokenClock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newTestTokenCache(src TokenSource) (*tokenCache, *tokenClock) {
	clk := &tokenClock{t: time.UnixMilli(1_793_000_000_000)}
	return newTokenCache(src, clk.now), clk
}

func TestTokenCacheServesFromCacheWhileValid(t *testing.T) {
	src := &tokenSrc{expiresIn: 3600}
	c, clk := newTestTokenCache(src)
	first, aerr := c.get(t.Context(), "fnc_1", 1, false)
	if aerr != nil {
		t.Fatal(aerr)
	}
	clk.add(10 * time.Minute)
	second, aerr := c.get(t.Context(), "fnc_1", 1, false)
	if aerr != nil || second.token != first.token || src.calls.Load() != 1 {
		t.Fatalf("second = %+v err=%v calls=%d: want the cached token", second, aerr, src.calls.Load())
	}
	if want := time.UnixMilli(1_793_000_000_000).Add(time.Hour); !first.expires.Equal(want) {
		t.Fatalf("expires = %v, want fetch time + expiresIn", first.expires)
	}
	// Another function has its own entry.
	if _, aerr := c.get(t.Context(), "fnc_2", 1, false); aerr != nil || src.calls.Load() != 2 {
		t.Fatalf("calls=%d err=%v", src.calls.Load(), aerr)
	}
}

// Renewal happens when no more than max(60 s, 10% of the lifetime) remains.
func TestTokenCacheRenewsNearExpiry(t *testing.T) {
	src := &tokenSrc{expiresIn: 3600} // margin = 360 s
	c, clk := newTestTokenCache(src)
	first, _ := c.get(t.Context(), "fnc_1", 1, false)
	clk.add(3600*time.Second - 361*time.Second) // 361 s left: still served
	if e, _ := c.get(t.Context(), "fnc_1", 1, false); e.token != first.token || src.calls.Load() != 1 {
		t.Fatalf("with 361 s left the token must be served: calls=%d", src.calls.Load())
	}
	clk.add(2 * time.Second) // 359 s left: renew
	e, aerr := c.get(t.Context(), "fnc_1", 1, false)
	if aerr != nil || e.token == first.token || src.calls.Load() != 2 {
		t.Fatalf("with 359 s left the token must be renewed: %+v calls=%d", e, src.calls.Load())
	}

	// A short-lived token uses the 60 s floor.
	short := &tokenSrc{expiresIn: 300} // 10% = 30 s, floor 60 s
	c, clk = newTestTokenCache(short)
	_, _ = c.get(t.Context(), "fnc_1", 1, false)
	clk.add(300*time.Second - 61*time.Second)
	_, _ = c.get(t.Context(), "fnc_1", 1, false)
	if short.calls.Load() != 1 {
		t.Fatalf("61 s left must be served, calls=%d", short.calls.Load())
	}
	clk.add(2 * time.Second)
	_, _ = c.get(t.Context(), "fnc_1", 1, false)
	if short.calls.Load() != 2 {
		t.Fatalf("59 s left must renew, calls=%d", short.calls.Load())
	}
}

func TestTokenCacheSharesOneFetchBetweenConcurrentCallers(t *testing.T) {
	src := &tokenSrc{expiresIn: 3600, gate: make(chan struct{})}
	c, _ := newTestTokenCache(src)
	const n = 20
	var wg sync.WaitGroup
	tokens := make([]string, n)
	for i := range n {
		wg.Go(func() {
			e, aerr := c.get(t.Context(), "fnc_1", 1, false)
			if aerr != nil {
				t.Errorf("caller %d: %v", i, aerr)
			}
			tokens[i] = e.token
		})
	}
	// Let every caller reach the flight before the fetch returns.
	deadline := time.Now().Add(5 * time.Second)
	for src.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	close(src.gate)
	wg.Wait()
	if src.calls.Load() != 1 {
		t.Fatalf("%d concurrent callers made %d fetches, want 1", n, src.calls.Load())
	}
	for i, tok := range tokens {
		if tok != "tok-1" {
			t.Fatalf("caller %d got %q", i, tok)
		}
	}
}

// A caller whose invocation ends does not take the shared fetch down with it.
func TestTokenCacheFetchOutlivesItsFirstCaller(t *testing.T) {
	src := &tokenSrc{expiresIn: 3600, gate: make(chan struct{})}
	c, _ := newTestTokenCache(src)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan *abi.Error, 1)
	go func() { _, aerr := c.get(ctx, "fnc_1", 1, false); done <- aerr }()
	for src.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if aerr := <-done; aerr == nil || aerr.Code != abi.CodeDeadline {
		t.Fatalf("the cancelled caller must see DEADLINE, got %v", aerr)
	}
	close(src.gate)
	for range 500 {
		c.mu.Lock()
		_, cached := c.entries["fnc_1"]
		c.mu.Unlock()
		if cached {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the fetch must finish and be cached after its caller left")
}

func TestTokenCacheRefreshReplacesTheEntry(t *testing.T) {
	src := &tokenSrc{expiresIn: 3600}
	c, _ := newTestTokenCache(src)
	first, _ := c.get(t.Context(), "fnc_1", 1, false)
	fresh, aerr := c.get(t.Context(), "fnc_1", 1, true)
	if aerr != nil || fresh.token == first.token || src.calls.Load() != 2 {
		t.Fatalf("refresh: %+v err=%v calls=%d", fresh, aerr, src.calls.Load())
	}
	again, _ := c.get(t.Context(), "fnc_1", 1, false)
	if again.token != fresh.token || src.calls.Load() != 2 {
		t.Fatalf("after a refresh the new token is the cached one: %+v calls=%d", again, src.calls.Load())
	}
}

func TestTokenCacheFailureIsNotCached(t *testing.T) {
	src := &tokenSrc{expiresIn: 3600, err: abi.Errorf(abi.CodeUnavailable, "down")}
	c, _ := newTestTokenCache(src)
	if _, aerr := c.get(t.Context(), "fnc_1", 1, false); aerr == nil || aerr.Code != abi.CodeUnavailable {
		t.Fatalf("err = %v", aerr)
	}
	src.err = nil
	e, aerr := c.get(t.Context(), "fnc_1", 1, false)
	if aerr != nil || e.token == "" || src.calls.Load() != 2 {
		t.Fatalf("the next call must fetch again: %+v err=%v calls=%d", e, aerr, src.calls.Load())
	}
}

func TestTokenCacheDropForgetsTheEntry(t *testing.T) {
	src := &tokenSrc{expiresIn: 3600}
	c, _ := newTestTokenCache(src)
	_, _ = c.get(t.Context(), "fnc_1", 1, false)
	c.drop("fnc_1")
	_, _ = c.get(t.Context(), "fnc_1", 1, false)
	if src.calls.Load() != 2 {
		t.Fatalf("calls = %d: a dropped entry must be fetched again", src.calls.Load())
	}

	// A fetch still running when the function leaves is not stored.
	slow := &tokenSrc{expiresIn: 3600, gate: make(chan struct{})}
	c, _ = newTestTokenCache(slow)
	done := make(chan struct{})
	go func() { _, _ = c.get(t.Context(), "fnc_1", 1, false); close(done) }()
	for slow.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	c.drop("fnc_1")
	close(slow.gate)
	<-done
	c.mu.Lock()
	_, stored := c.entries["fnc_1"]
	c.mu.Unlock()
	if stored {
		t.Fatal("a fetch that finished after the function left must not be cached")
	}
}

// capsFor builds the capabilities of one invocation of a version with the
// given declaration, over a cache on src, logging to buf.
func capsFor(declared bool, c *tokenCache, buf *bytes.Buffer) *capabilities {
	return &capabilities{
		fn:     &function{id: "fnc_1", address: "app.hello"},
		ver:    &version{number: 3, describe: &abi.Describe{PlatformToken: declared}},
		log:    slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})),
		invID:  "inv_1",
		tokens: c,
	}
}

func TestPlatformTokenOpRefusesUndeclaredUse(t *testing.T) {
	src := &tokenSrc{expiresIn: 3600}
	cache, _ := newTestTokenCache(src)
	caps := capsFor(false, cache, &bytes.Buffer{})
	_, _, aerr := caps.Call(t.Context(), abi.OpPlatformToken, nil, nil)
	if aerr == nil || aerr.Code != abi.CodeNotDeclared || src.calls.Load() != 0 {
		t.Fatalf("err = %v calls=%d: the host must refuse before calling the platform", aerr, src.calls.Load())
	}
}

func TestPlatformTokenOpResultAndRefreshMeta(t *testing.T) {
	src := &tokenSrc{expiresIn: 3600}
	cache, clk := newTestTokenCache(src)
	caps := capsFor(true, cache, &bytes.Buffer{})
	for _, meta := range [][]byte{nil, []byte(`{}`), []byte(` `)} {
		if _, _, aerr := caps.Call(t.Context(), abi.OpPlatformToken, meta, nil); aerr != nil {
			t.Fatalf("meta %q: %v", meta, aerr)
		}
	}
	if src.calls.Load() != 1 {
		t.Fatalf("calls = %d: empty meta is a plain, cacheable call", src.calls.Load())
	}
	meta, body, aerr := caps.Call(t.Context(), abi.OpPlatformToken, []byte(`{"refresh":true}`), nil)
	if aerr != nil || body != nil || src.calls.Load() != 2 {
		t.Fatalf("refresh: err=%v body=%v calls=%d", aerr, body, src.calls.Load())
	}
	var res abi.PlatformTokenResult
	if err := json.Unmarshal(meta, &res); err != nil || res.AccessToken != "tok-2" || res.ExpiresAt != clk.now().Add(time.Hour).UnixMilli() {
		t.Fatalf("result = %+v err=%v", res, err)
	}
	if _, _, aerr := caps.Call(t.Context(), abi.OpPlatformToken, []byte(`{"refresh":`), nil); aerr == nil || aerr.Code != abi.CodeBadRequest {
		t.Fatalf("malformed meta: %v", aerr)
	}
}

func TestPlatformTokenOpPassesPlatformErrorsThrough(t *testing.T) {
	for _, code := range []string{abi.CodeNotAllowed, abi.CodeUnavailable, abi.CodeDeadline} {
		cache, _ := newTestTokenCache(&tokenSrc{err: abi.Errorf(code, "why")})
		_, _, aerr := capsFor(true, cache, &bytes.Buffer{}).Call(t.Context(), abi.OpPlatformToken, nil, nil)
		if aerr == nil || aerr.Code != code {
			t.Errorf("%s: got %v", code, aerr)
		}
	}
}

// The token is a live credential: nothing the host logs while serving the op,
// in either outcome, may contain it.
func TestPlatformTokenNeverLogged(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	src := &tokenSrc{expiresIn: 3600}
	cache, _ := newTestTokenCache(src)
	caps := capsFor(true, cache, &buf)
	meta, _, aerr := caps.Call(t.Context(), abi.OpPlatformToken, nil, nil)
	if aerr != nil {
		t.Fatal(aerr)
	}
	var res abi.PlatformTokenResult
	_ = json.Unmarshal(meta, &res)
	_, _, _ = caps.Call(t.Context(), abi.OpPlatformToken, []byte(`{"refresh":true}`), nil)
	if res.AccessToken == "" || strings.Contains(buf.String(), res.AccessToken) || strings.Contains(buf.String(), "tok-") {
		t.Fatalf("the host log must not carry a token: %q", buf.String())
	}
}

// End to end through the runner: declared use is served and cached; undeclared
// use is refused; a function that leaves the desired document loses its entry.
func TestPlatformTokenThroughRunner(t *testing.T) {
	declared := ver(1, control.RoleLive)
	declared.Describe = json.RawMessage(strings.Replace(describeDoc, `"emits"`, `"platformToken":true,"emits"`, 1))
	h := start(t, fnDoc(declared))

	code, meta, _ := hostCall(t, h, abi.OpPlatformToken, abi.PlatformTokenCall{}, "")
	var res abi.PlatformTokenResult
	_ = json.Unmarshal([]byte(meta), &res)
	if code != "0" || res.AccessToken != "tok-1" || res.ExpiresAt <= time.Now().UnixMilli() {
		t.Fatalf("first call: %s %+v", code, res)
	}
	if _, meta, _ := hostCall(t, h, abi.OpPlatformToken, map[string]any{}, ""); !strings.Contains(meta, "tok-1") {
		t.Fatalf("second call must be cached: %s", meta)
	}
	if _, meta, _ := hostCall(t, h, abi.OpPlatformToken, abi.PlatformTokenCall{Refresh: true}, ""); !strings.Contains(meta, "tok-2") {
		t.Fatalf("refresh must fetch: %s", meta)
	}
	h.cp.mu.Lock()
	calls := slices.Clone(h.cp.tokenCalls)
	h.cp.mu.Unlock()
	if len(calls) != 2 || calls[0].FunctionID != "fnc_1" || calls[0].Version != 1 {
		t.Fatalf("token requests = %+v", calls)
	}

	// The function leaves desired state: its cached token goes with it.
	h.cp.push(control.Desired{Pool: "default"})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		h.r.fnTokens.mu.Lock()
		n := len(h.r.fnTokens.entries)
		h.r.fnTokens.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the cached token must be dropped when its function leaves desired state")
}

func TestPlatformTokenThroughRunnerUndeclared(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	code, meta, _ := hostCall(t, h, abi.OpPlatformToken, abi.PlatformTokenCall{}, "")
	if code != "1" || !strings.Contains(meta, abi.CodeNotDeclared) {
		t.Fatalf("undeclared: %s %s", code, meta)
	}
	h.cp.mu.Lock()
	n := len(h.cp.tokenCalls)
	h.cp.mu.Unlock()
	if n != 0 {
		t.Fatalf("%d platform fetches for an undeclared function", n)
	}
}
