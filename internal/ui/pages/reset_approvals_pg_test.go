//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/resetapproval"
	resetapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/resetapproval/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// resetMail records the reset links the approvals send.
type resetMail struct{ sent []string }

func (m *resetMail) SendResetEmail(_ context.Context, p *principal.Principal, reset2FA bool) error {
	if reset2FA {
		m.sent = append(m.sent, p.UserIdentity.Email+" (2fa cleared)")
	} else {
		m.sent = append(m.sent, p.UserIdentity.Email)
	}
	return nil
}

// The reset approvals page end to end against Postgres: the handlers are the
// API's (resetapi.State), so the permission, the client scoping of the queue,
// the per-request client check and the single decision are the API's.
func TestResetApprovalsPageAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	clients := client.NewRepository(pool)
	principals := principal.NewRepository(pool)
	approvals := resetapproval.NewRepository(pool)
	mail := &resetMail{}
	page := &ResetApprovals{Service: &resetapi.State{Approvals: approvals, Principals: principals, Sender: mail}, Shell: shellForTest()}

	mkClient := func(name, ident string) string {
		t.Helper()
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(clients), clientops.CreateCommand{Name: name, Identifier: ident}, testpg.TestEC())
		require.NoError(t, err)
		return ev.ClientID
	}
	clientA, clientB := mkClient("RA A", "ui-ra-a"), mkClient("RA B", "ui-ra-b")
	// mkRequest seeds a user (of the client, or a platform user) and its pending request.
	mkRequest := func(email, name string, clientID *string) string {
		t.Helper()
		scope := principal.ScopeClient
		if clientID == nil {
			scope = principal.ScopeAnchor
		}
		u := principal.NewUser(email, scope)
		u.Name = name
		if clientID != nil {
			u.ClientID = ids.PtrOf[ids.ClientID](clientID)
		}
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, principals.Persist(ctx, u, usecasepgx.WrapTxForBootstrap(tx)))
		require.NoError(t, tx.Commit(ctx))
		r := resetapproval.New(string(u.ID), clientID, 24*time.Hour)
		require.NoError(t, approvals.Insert(ctx, r))
		return r.ID
	}
	reqA := mkRequest("ra-ada@example.com", "Ada <A>", &clientA)
	reqB := mkRequest("ra-bob@example.com", "Bob B", &clientB)
	reqPlat := mkRequest("ra-plat@example.com", "Plat User", nil)
	status := func(id string) resetapproval.Status {
		t.Helper()
		r, err := approvals.FindByID(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, r)
		return r.Status
	}
	as := func(ac *auth.AuthContext, q req) (int, string) {
		t.Helper()
		rec := serveMounted(t, ac, q, page)
		return rec.Code, rec.Body.String()
	}
	post := func(ac *auth.AuthContext, id, action string) string {
		t.Helper()
		_, body := as(ac, req{method: http.MethodPost, target: "/ui/reset-approvals/" + id + "/" + action, datastar: true, form: url.Values{}})
		return body
	}
	admin := anchorAdmin()
	admin.PrincipalID = "prn_ui_ra_admin"
	adminA := &auth.AuthContext{
		PrincipalID: "prn_ui_ra_adm_a", Scope: auth.ScopeClient, Clients: []string{clientA}, Roles: []string{"r"}, PrincipalType: auth.PrincipalTypeUser,
		Permissions: []string{"platform:iam:user:update"},
	}
	noPerm := &auth.AuthContext{
		PrincipalID: "prn_ui_ra_noperm", Scope: auth.ScopeClient, Clients: []string{clientA}, Roles: []string{"r"}, PrincipalType: auth.PrincipalTypeUser,
		Permissions: []string{"platform:iam:user:view"},
	}

	// The queue is the caller's: an anchor sees every client's, a client
	// administrator its own, and nobody without user:update any.
	_, all := as(admin, req{target: "/ui/reset-approvals"})
	assert.Contains(t, all, "ra-ada@example.com")
	assert.Contains(t, all, "ra-bob@example.com")
	assert.Contains(t, all, "ra-plat@example.com")
	assert.Contains(t, all, "Ada &lt;A&gt;")
	_, own := as(adminA, req{target: "/ui/reset-approvals"})
	assert.Contains(t, own, "ra-ada@example.com")
	assert.NotContains(t, own, "ra-bob@example.com")
	assert.NotContains(t, own, "ra-plat@example.com")
	code, denied := as(noPerm, req{target: "/ui/reset-approvals"})
	assert.Equal(t, http.StatusForbidden, code)
	assert.NotContains(t, denied, "ra-ada@example.com")

	// Forged decisions: another client's request, a platform user's, or no
	// permission at all. Each is refused, nothing changes, no link is sent.
	for _, f := range []struct {
		name string
		ac   *auth.AuthContext
		id   string
	}{{"other client", adminA, reqB}, {"platform user", adminA, reqPlat}, {"no permission", noPerm, reqA}} {
		for _, action := range []string{"approve", "deny"} {
			body := post(f.ac, f.id, action)
			assert.Contains(t, body, "fc-flash-error", f.name+" "+action)
			assert.Equal(t, resetapproval.StatusPending, status(f.id), f.name+" "+action)
		}
	}
	assert.Empty(t, mail.sent)

	// A client administrator approves its own: the user is emailed a link that
	// clears their 2FA, the decision is recorded, and the queue no longer lists it.
	body := post(adminA, reqA, "approve")
	assert.Contains(t, body, "The user has been emailed a reset link.")
	assert.NotContains(t, body, "ra-ada@example.com")
	assert.Equal(t, resetapproval.StatusApproved, status(reqA))
	assert.Equal(t, []string{"ra-ada@example.com (2fa cleared)"}, mail.sent)
	decided, err := approvals.FindByID(ctx, reqA)
	require.NoError(t, err)
	require.NotNil(t, decided.DecidedBy)
	assert.Equal(t, adminA.PrincipalID, *decided.DecidedBy)

	// Deciding twice is refused and sends nothing more.
	again := post(adminA, reqA, "approve")
	assert.Contains(t, again, "request is no longer pending")
	assert.Equal(t, resetapproval.StatusApproved, status(reqA))
	assert.Len(t, mail.sent, 1)
	assert.Contains(t, post(admin, reqA, "deny"), "request is no longer pending")
	assert.Equal(t, resetapproval.StatusApproved, status(reqA), "a denial cannot undo an approval")

	// An anchor denies another client's request: no link is sent.
	denial := post(admin, reqB, "deny")
	assert.Contains(t, denial, "Request denied.")
	assert.Equal(t, resetapproval.StatusDenied, status(reqB))
	assert.Len(t, mail.sent, 1)
	// And approves the platform user's.
	post(admin, reqPlat, "approve")
	assert.Equal(t, resetapproval.StatusApproved, status(reqPlat))
	assert.Equal(t, "ra-plat@example.com (2fa cleared)", mail.sent[1])
	_, empty := as(admin, req{target: "/ui/reset-approvals"})
	assert.NotContains(t, empty, "ra-bob@example.com")
}
