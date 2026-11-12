//go:build integration

package portalauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/grantstore"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// gateSender blocks each portal reset send until released and records what it
// saw. Unlike fakeResetSender it is safe to read while a send is in flight.
type gateSender struct {
	release chan struct{}
	err     error

	mu       sync.Mutex
	sent     []string
	ctxAlive []bool
	deadline []bool
}

func (g *gateSender) SendPortalReset(ctx context.Context, identityID, _ string, _ *string) error {
	<-g.release
	g.mu.Lock()
	defer g.mu.Unlock()
	g.sent = append(g.sent, identityID)
	g.ctxAlive = append(g.ctxAlive, ctx.Err() == nil)
	_, ok := ctx.Deadline()
	g.deadline = append(g.deadline, ok)
	return g.err
}

func (g *gateSender) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.sent)
}

// T6: the portal forgot-password mail is sent off the request path.
func TestPortalForgotPassword_MailIsSentOffTheRequestPath(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	authRepo := platformauth.NewRepository(pool)
	clients := client.NewRepository(pool)
	identities := portalidentity.NewRepository(pool)
	uow := testpg.NewUoW(t)
	tag := fmt.Sprintf("bg%x", time.Now().UnixNano())

	clientEv, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Portal BG " + tag, Identifier: "portal-" + tag}, testpg.TestEC())
	require.NoError(t, err)
	tenantID := clientEv.ClientID
	redirect := "https://" + tag + ".flow.test/cb"
	oauthEv, err := usecaseop.Run(ctx, uow, authops.CreateOAuthClient(authRepo.OAuthClients),
		authops.CreateOAuthClientCommand{
			ClientName: "Portal BG App " + tag, ClientType: "PUBLIC",
			RedirectURIs: []string{redirect},
			GrantTypes:   []string{"authorization_code"}, PortalClientID: &tenantID,
		}, testpg.TestEC())
	require.NoError(t, err)
	addr := "bg@" + tag + ".test"
	identEv, err := usecaseop.Run(ctx, uow, portalidentity.Ensure(identities, clients, nil),
		portalidentity.EnsureCommand{ClientID: tenantID, Email: addr, Source: "INVITE"}, testpg.TestEC())
	require.NoError(t, err)

	var wg sync.WaitGroup
	s := &State{
		OAuthClients: authRepo.OAuthClients, Identities: identities,
		IdPs: identityprovider.NewRepository(pool), Flows: NewFlowRepo(pool),
		AuthCodes: grantstore.NewAuthorizationCodeRepository(pool),
		Go:        func(fn func()) { wg.Go(fn) },
	}
	rec := httptest.NewRecorder()
	s.Authorize(rec, httptest.NewRequest(http.MethodGet,
		"/portal/authorize?response_type=code&client_id="+url.QueryEscape(oauthEv.ClientID)+
			"&redirect_uri="+url.QueryEscape(redirect)+"&state=s&code_challenge=c", nil))
	require.Equal(t, http.StatusTemporaryRedirect, rec.Code)
	flowID, _ := url.QueryUnescape(strings.TrimPrefix(rec.Header().Get("Location"), "/portal/login?flow="))

	forgot := func(email string, reqCtx context.Context) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := postRequest("/portal/auth/password-reset", map[string]string{"flowId": flowID, "email": email}).WithContext(reqCtx)
		s.RequestPasswordReset(w, req)
		return w
	}

	t.Run("returns before the send completes", func(t *testing.T) {
		gate := &gateSender{release: make(chan struct{})}
		s.PasswordReset = gate
		reqCtx, cancel := context.WithCancel(ctx)
		var got *httptest.ResponseRecorder
		done := make(chan struct{})
		go func() { got = forgot(addr, reqCtx); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("the request did not return while the mail send was blocked: SMTP is on the request path")
		}
		cancel() // the request context dies with the response
		require.Equal(t, http.StatusOK, got.Code, got.Body.String())
		assert.Equal(t, 0, gate.count(), "the send must still be pending")

		close(gate.release)
		wg.Wait()
		require.Equal(t, 1, gate.count(), "sent exactly once")
		assert.Equal(t, identEv.IdentityID, gate.sent[0])
		assert.True(t, gate.ctxAlive[0], "the send must not inherit the request's cancellation")
		assert.True(t, gate.deadline[0], "the send must be bounded")
	})

	t.Run("a failing mailer does not change the answer", func(t *testing.T) {
		ok := &gateSender{release: make(chan struct{})}
		close(ok.release)
		s.PasswordReset = ok
		want := forgot(addr, ctx)
		wg.Wait()
		bad := &gateSender{release: make(chan struct{}), err: errors.New("smtp down")}
		close(bad.release)
		s.PasswordReset = bad
		got := forgot(addr, ctx)
		wg.Wait()
		assert.Equal(t, 1, bad.count())
		assert.Equal(t, want.Code, got.Code)
		assert.Equal(t, want.Body.String(), got.Body.String())
	})
}

func postRequest(path string, body map[string]string) *http.Request {
	var sb strings.Builder
	sb.WriteString("{")
	first := true
	for k, v := range body {
		if !first {
			sb.WriteString(",")
		}
		first = false
		fmt.Fprintf(&sb, "%q:%q", k, v)
	}
	sb.WriteString("}")
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(sb.String()))
	req.Header.Set("Content-Type", "application/json")
	return req
}
