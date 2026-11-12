//go:build integration

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/passwordreset"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// waiter is the State.Go seam for tests: the send runs on a real goroutine
// and Wait blocks until every one started so far has finished.
type waiter struct{ wg sync.WaitGroup }

func (w *waiter) Go(fn func()) { w.wg.Go(fn) }
func (w *waiter) Wait()        { w.wg.Wait() }

// abuseRig is an sfRig with the limiter on, small policies, and a waitable
// background send. The emailed-link flow is the plain one (no approval queue).
type abuseRig struct {
	*sfRig
	store ratelimit.Store
	wait  *waiter
	ip    string // this test's own source address
}

var abuseSeq int64

func newAbuseRig(t *testing.T, pol ratelimit.Policies) *abuseRig {
	t.Helper()
	// A tag no other test, nor an earlier -count pass, has used.
	tag := fmt.Sprintf("ab%x", time.Now().UnixNano()+abuseSeq)
	abuseSeq++
	r := &abuseRig{sfRig: newSFRig(t, tag), wait: &waiter{}}
	r.store = ratelimit.NewPostgresStore(testpg.Pool(t))
	r.s.RequireStrongFactorForReset = false
	r.s.RateLimit = r.store
	r.s.RateLimitPolicies = pol
	r.s.Go = r.wait.Go
	r.ip = testAddr(fmt.Sprintf("%d.%s", time.Now().UnixNano(), tag))
	return r
}

func hour(limit uint32) ratelimit.Policy { return ratelimit.Policy{Window: time.Hour, Limit: limit} }

// wide is a policy set nothing in a test will reach.
func widePolicies() ratelimit.Policies {
	return ratelimit.Policies{
		PasswordResetIP: hour(10000), PasswordResetEmail: hour(10000), PasswordResetFactor: hour(10000),
	}
}

// testAddr maps a seed to a valid, deterministic documentation-range IPv6
// address: the client address must be an IP, and each test needs its own.
func testAddr(seed string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(seed))
	v := h.Sum64()
	return fmt.Sprintf("2001:db8:%x:%x:%x:%x::1", uint16(v>>48), uint16(v>>32), uint16(v>>16), uint16(v))
}

// do posts a JSON body as client ip, with xff the X-Forwarded-For header.
func (r *abuseRig) do(h http.HandlerFunc, path, xff string, body any) *httptest.ResponseRecorder {
	r.t.Helper()
	b, err := json.Marshal(body)
	require.NoError(r.t, err)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("X-Forwarded-For", xff)
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func (r *abuseRig) reset(addr string) *httptest.ResponseRecorder {
	return r.do(r.s.requestReset, "/auth/password-reset/request", r.ip, map[string]any{"email": addr})
}

func (r *abuseRig) tokenCount(pid string) int {
	r.t.Helper()
	var n int
	require.NoError(r.t, testpg.Pool(r.t).QueryRow(r.ctx,
		`SELECT count(*) FROM iam_password_reset_tokens WHERE principal_id = $1`, pid).Scan(&n))
	return n
}

func (r *abuseRig) tokenExists(raw string) *passwordreset.Token {
	r.t.Helper()
	tok, err := r.s.Tokens.FindByTokenHash(r.ctx, hashToken(raw))
	require.NoError(r.t, err)
	return tok
}

// T1 + T8: past the per-email limit a request is answered exactly like an
// accepted one and does nothing: no token deleted, none minted, no mail.
func TestAbuse_PerEmailLimitIsASilentDrop(t *testing.T) {
	pol := widePolicies()
	pol.PasswordResetEmail = hour(2)
	r := newAbuseRig(t, pol)
	pid, addr := r.user("a", true)
	otherPid, other := r.user("b", true)

	first := r.reset(addr)
	require.Equal(t, http.StatusOK, first.Code)
	r.wait.Wait()
	_ = r.reset(addr)
	r.wait.Wait()
	accepted := r.mail.lastToken(t, addr)
	require.Equal(t, 2, r.mail.count(addr))

	// Upper-case, padded: the key is the lower-cased, trimmed address.
	over := r.reset("  " + strings.ToUpper(addr) + " ")
	r.wait.Wait()
	assert.Equal(t, first.Code, over.Code)
	assert.Equal(t, first.Body.String(), over.Body.String(), "over-limit answer must match an accepted one byte for byte")
	assert.Equal(t, 2, r.mail.count(addr), "no mail past the limit")
	assert.NotNil(t, r.tokenExists(accepted), "the token from the last ACCEPTED request must survive")
	assert.Equal(t, 1, r.tokenCount(pid))

	// A different email is unaffected.
	require.Equal(t, http.StatusOK, r.reset(other).Code)
	r.wait.Wait()
	assert.Equal(t, 1, r.mail.count(other))
	assert.Equal(t, 1, r.tokenCount(otherPid))
}

// T8: unknown, accepted and over-limit emails are indistinguishable.
func TestAbuse_GenericResponseIsIdentical(t *testing.T) {
	pol := widePolicies()
	pol.PasswordResetEmail = hour(1)
	r := newAbuseRig(t, pol)
	_, addr := r.user("g", true)

	unknown := r.reset("nobody-" + addr)
	accepted := r.reset(addr)
	r.wait.Wait()
	over := r.reset(addr)
	r.wait.Wait()
	require.Equal(t, 1, r.mail.count(addr))

	for name, rec := range map[string]*httptest.ResponseRecorder{"accepted": accepted, "over": over} {
		assert.Equal(t, unknown.Code, rec.Code, name)
		assert.Equal(t, unknown.Body.String(), rec.Body.String(), name)
		assert.Equal(t, unknown.Header(), rec.Header(), name)
	}
}

// T2: per-IP, keyed on the rightmost X-Forwarded-For hop.
func TestAbuse_PerIPLimit(t *testing.T) {
	pol := widePolicies()
	pol.PasswordResetIP = hour(2)
	r := newAbuseRig(t, pol)

	for _, route := range []struct {
		name string
		h    http.HandlerFunc
		path string
	}{
		{"reset", r.s.requestReset, "/auth/password-reset/request"},
		{"setup", r.s.requestPasswordSetup, "/auth/password-setup/request"},
	} {
		t.Run(route.name, func(t *testing.T) {
			hop := testAddr(route.name + r.ip) // this subtest's own rightmost hop
			send := func(xff string, n int) *httptest.ResponseRecorder {
				return r.do(route.h, route.path, xff, map[string]any{"email": fmt.Sprintf("x%d-%s@nowhere.test", n, route.name+r.ip)})
			}
			assert.Equal(t, http.StatusOK, send("1.1.1.1, "+hop, 1).Code)
			assert.Equal(t, http.StatusOK, send("2.2.2.2, "+hop, 2).Code)
			over := send("3.3.3.3, "+hop, 3) // only the leftmost hop changed
			assert.Equal(t, http.StatusTooManyRequests, over.Code)
			assert.Contains(t, over.Body.String(), "TOO_MANY_REQUESTS")
			assert.NotEmpty(t, over.Header().Get("Retry-After"))
			// Another client address is untouched.
			assert.Equal(t, http.StatusOK, send("1.1.1.1, "+testAddr(hop+"-other"), 4).Code)
		})
	}
}

// T3: a pending 72-hour invitation survives a burst of reset requests past
// the per-email limit.
func TestAbuse_PendingInviteSurvivesResetBurst(t *testing.T) {
	pol := widePolicies()
	pol.PasswordResetEmail = hour(2)
	r := newAbuseRig(t, pol)
	pid, addr := r.user("inv", true)

	// The address has used up its budget.
	for range 2 {
		require.Nil(t, ratelimit.Enforce(r.ctx, r.store, ratelimit.BucketPasswordResetEmail, strings.ToLower(addr), pol.PasswordResetEmail))
	}
	pe := &principalEmailer{tokens: r.s.Tokens, base: "https://fc.test", mail: r.s.Emailer}
	link, err := pe.mintInviteLink(r.ctx, pid, nil)
	require.NoError(t, err)
	raw := link[strings.Index(link, "token=")+len("token="):]
	require.NotNil(t, r.tokenExists(raw))

	for range 6 {
		require.Equal(t, http.StatusOK, r.reset(addr).Code)
	}
	r.wait.Wait()
	tok := r.tokenExists(raw)
	require.NotNil(t, tok, "the invitation must not be deleted by throttled requests")
	assert.Equal(t, passwordreset.PurposeInvite, tok.Purpose)
	assert.Equal(t, 0, r.mail.count(addr), "no reset mail either")
}

// T4: factor proofs are counted per principal, across tokens.
func TestAbuse_FactorProofsCountPerPrincipal(t *testing.T) {
	pol := widePolicies()
	pol.PasswordResetFactor = hour(3)
	r := newAbuseRig(t, pol)
	pid, addr := r.user("fa", true)
	secret := r.enrollTOTP(pid, addr)
	otherPid, otherAddr := r.user("fb", true)
	otherSecret := r.enrollTOTP(otherPid, otherAddr)

	// Three failures, each on a freshly requested token (one attempt apiece:
	// the per-token counter never gets near its own limit).
	for i := range 3 {
		require.Equal(t, http.StatusOK, r.reset(addr).Code)
		r.wait.Wait()
		rec := r.confirm(map[string]any{"token": r.mail.lastToken(t, addr), "password": sfPassword, "factorCode": "000000"})
		require.Contains(t, rec.Body.String(), "INVALID_FACTOR", "failure %d", i+1)
	}

	// A fourth token, and the CORRECT code: refused as an exhausted token is.
	before := r.passwordHash(pid)
	require.Equal(t, http.StatusOK, r.reset(addr).Code)
	r.wait.Wait()
	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)
	rec := r.confirm(map[string]any{"token": r.mail.lastToken(t, addr), "password": sfPassword, "factorCode": code})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), `"INVALID_TOKEN"`)
	assert.Contains(t, rec.Body.String(), "Invalid or expired reset token.")
	assert.Equal(t, before, r.passwordHash(pid), "the password must not change")

	// Another principal is unaffected.
	require.Equal(t, http.StatusOK, r.reset(otherAddr).Code)
	r.wait.Wait()
	otherCode, err := totp.GenerateCode(otherSecret, time.Now())
	require.NoError(t, err)
	rec = r.confirm(map[string]any{"token": r.mail.lastToken(t, otherAddr), "password": sfPassword, "factorCode": otherCode})
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// gateEmailer blocks each send until released, and records what it saw.
type gateEmailer struct {
	Emailer
	release chan struct{}
	err     error

	mu       sync.Mutex
	sent     []string
	ctxAlive []bool // the send's context was not cancelled
	deadline []bool // ...and carried a deadline
}

func (g *gateEmailer) note(ctx context.Context, to string) error {
	<-g.release
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sent = append(g.sent, to)
	g.ctxAlive = append(g.ctxAlive, ctx.Err() == nil)
	_, ok := ctx.Deadline()
	g.deadline = append(g.deadline, ok)
	return g.err
}

func (g *gateEmailer) SendResetLink(ctx context.Context, to, _ string) error { return g.note(ctx, to) }

func (g *gateEmailer) SendInviteLink(ctx context.Context, to, _ string) error { return g.note(ctx, to) }

func (g *gateEmailer) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.sent)
}

// returnsBeforeSend runs the request on its own goroutine and fails (instead
// of hanging) when it does not return while the send is still blocked.
func returnsBeforeSend(t *testing.T, cancel context.CancelFunc, call func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { call(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the request did not return while the mail send was blocked: SMTP is on the request path")
	}
	cancel() // the request context dies with the response
}

// T5: mail leaves the request path.
func TestAbuse_MailIsSentOffTheRequestPath(t *testing.T) {
	r := newAbuseRig(t, widePolicies())
	_, addr := r.user("m", true)

	// A password-less user for the setup route.
	ev, err := usecaseop.Run(testpg.AnchorCtx(), r.s.UoW, principalops.CreateUser(r.s.Principals),
		principalops.CreateCommand{Email: "setup-" + addr, Scope: "ANCHOR"}, testpg.TestEC())
	require.NoError(t, err)
	_ = ev
	setupAddr := "setup-" + addr

	for _, tc := range []struct {
		name string
		h    http.HandlerFunc
		path string
		to   string
	}{
		{"reset", r.s.requestReset, "/auth/password-reset/request", addr},
		{"setup", r.s.requestPasswordSetup, "/auth/password-setup/request", setupAddr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gate := &gateEmailer{release: make(chan struct{})}
			r.s.Emailer = gate
			b, _ := json.Marshal(map[string]any{"email": tc.to})
			ctx, cancel := context.WithCancel(context.Background())
			req := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader(b)).WithContext(ctx)
			req.Header.Set("X-Forwarded-For", r.ip)
			rec := httptest.NewRecorder()
			returnsBeforeSend(t, cancel, func() { tc.h(rec, req) })
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, 0, gate.count(), "the send must still be pending")

			close(gate.release)
			r.wait.Wait()
			require.Equal(t, 1, gate.count(), "sent exactly once")
			assert.True(t, gate.ctxAlive[0], "the send must not inherit the request's cancellation")
			assert.True(t, gate.deadline[0], "the send must be bounded")
			assert.Equal(t, tc.to, gate.sent[0])
		})
	}

	// A failing mailer does not change the answer.
	t.Run("failing mailer", func(t *testing.T) {
		ok := &gateEmailer{release: make(chan struct{})}
		close(ok.release)
		r.s.Emailer = ok
		want := r.do(r.s.requestReset, "/auth/password-reset/request", r.ip, map[string]any{"email": addr})
		r.wait.Wait()
		bad := &gateEmailer{release: make(chan struct{}), err: errors.New("smtp down")}
		close(bad.release)
		r.s.Emailer = bad
		got := r.do(r.s.requestReset, "/auth/password-reset/request", r.ip, map[string]any{"email": addr})
		r.wait.Wait()
		assert.Equal(t, 1, bad.count())
		assert.Equal(t, want.Code, got.Code)
		assert.Equal(t, want.Body.String(), got.Body.String())
	})
}

// R7: a no-op store (FC_RATE_LIMIT_DISABLE) turns all three limits off.
func TestAbuse_NoopStoreDisablesEverything(t *testing.T) {
	pol := widePolicies()
	pol.PasswordResetIP, pol.PasswordResetEmail, pol.PasswordResetFactor = hour(1), hour(1), hour(1)
	r := newAbuseRig(t, pol)
	r.s.RateLimit = ratelimit.NoopStore{}
	pid, addr := r.user("noop", true)
	secret := r.enrollTOTP(pid, addr)

	for range 4 {
		require.Equal(t, http.StatusOK, r.reset(addr).Code)
		r.wait.Wait()
		rec := r.confirm(map[string]any{"token": r.mail.lastToken(t, addr), "password": sfPassword, "factorCode": "000000"})
		require.Contains(t, rec.Body.String(), "INVALID_FACTOR")
	}
	assert.Equal(t, 4, r.mail.count(addr), "every request was served")
	code, err := totp.GenerateCode(secret, time.Now())
	require.NoError(t, err)
	rec := r.confirm(map[string]any{"token": r.mail.lastToken(t, addr), "password": sfPassword, "factorCode": code})
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
