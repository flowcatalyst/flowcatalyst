//go:build integration

package fieldauth

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecaseop"

	sdkauth "github.com/flowcatalyst/go-sdk/auth"
)

// The platform user a worker is linked to rides in the access token as
// linked_principal_id (docs/field-native-session-spec.md section 5): copied at
// login, at refresh (so a link made or removed after login shows up on the next
// refresh), on act-as and console sign-in code sessions, and reported by
// introspection. Audit correlation only: the token authorises nothing more.

// linkWorker links the worker to the principal through the admin use case.
func (e *env) linkWorker(workerID, principalID string) {
	e.t.Helper()
	cmd := fieldidentity.LinkWorkerCommand{PrincipalID: principalID}
	cmd.ClientID, cmd.ID = e.clientID, workerID
	_, err := usecaseop.Run(e.actx, e.s.UoW, e.s.Ops.LinkWorker(), cmd, auth.NewExecutionContext(e.actx))
	require.NoError(e.t, err)
}

func (e *env) unlinkWorker(workerID string) {
	e.t.Helper()
	_, err := usecaseop.Run(e.actx, e.s.UoW, e.s.Ops.UnlinkWorker(),
		fieldidentity.WorkerRef{ClientID: e.clientID, ID: workerID}, auth.NewExecutionContext(e.actx))
	require.NoError(e.t, err)
}

func TestLinkedPrincipalInTheTokenAtLoginAndIntrospection(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	admin := e.adminAPI()
	e.worker("lp-plain", goodPIN, false, e.codeA)
	linked := e.worker("lp-linked", goodPIN, false, e.codeA)
	pid := e.principal(true)
	e.linkWorker(linked, pid)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)

	// No link: no claim. Linked: the claim, copied from the worker.
	assert.NotContains(t, tokenPayload(t, mustLogin(t, d, e.codeA, "lp-plain", goodPIN).access), "linked_principal_id")
	in := mustLogin(t, d, e.codeA, "lp-linked", goodPIN)
	assert.Equal(t, pid, tokenPayload(t, in.access)["linked_principal_id"])

	// The Go SDK verifier exposes it.
	jwks := e.jwksServer()
	v := sdkauth.NewFieldTokenVerifier(sdkauth.FieldTokenVerifierConfig{Issuer: issuer, AppCode: e.codeA, JWKSURL: jwks.URL})
	appURL := "https://app.example.test/api/orders"
	p, err := v.Verify(context.Background(), "DPoP "+in.access, d.proof(http.MethodPost, "/x", in.access, proofOpts{htu: appURL}), http.MethodPost, appURL)
	require.NoError(t, err)
	assert.Equal(t, pid, p.LinkedPrincipalID)

	// Introspection reports it; an unlinked worker's introspection has none.
	m := adminPost(t, admin, "/api/field-tokens/introspect", map[string]any{"token": in.access})
	assert.Equal(t, true, m["active"])
	assert.Equal(t, pid, m["linkedPrincipalId"])
	un := mustLogin(t, e.enrolled(fieldidentity.ModePersonal, e.codeA), e.codeA, "lp-plain", goodPIN)
	m = adminPost(t, admin, "/api/field-tokens/introspect", map[string]any{"token": un.access})
	assert.Equal(t, true, m["active"])
	assert.NotContains(t, m, "linkedPrincipalId")
}

func TestRefreshPicksUpALinkAddedOrRemovedAfterLogin(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	workerID := e.worker("lp-refresh", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	in := mustLogin(t, d, e.codeA, "lp-refresh", goodPIN)
	assert.NotContains(t, tokenPayload(t, in.access), "linked_principal_id")

	pid := e.principal(true)
	e.linkWorker(workerID, pid)
	r := d.refresh(in.refresh)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.Equal(t, pid, tokenPayload(t, r.str("accessToken"))["linked_principal_id"], "a refresh carries the new link")

	e.unlinkWorker(workerID)
	r = d.refresh(r.str("refreshToken"))
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.NotContains(t, tokenPayload(t, r.str("accessToken")), "linked_principal_id", "a refresh drops a removed link")
}

func TestLinkedPrincipalInActAsAndSignInCodeTokens(t *testing.T) {
	// Console sign-in code: the claim is the TARGET worker's link, not the actor's.
	e := newEnv(t, envOpts{limits: roomyLimits()})
	workerID := e.worker("lp-code", goodPIN, false, e.codeA)
	linkedTo := e.principal(true)
	e.linkWorker(workerID, linkedTo)
	actor := e.principal(true)
	code := e.signInCode(workerID, e.codeA, actor)
	d := e.newDevice()
	r := d.call("/field/auth/sign-in-code", map[string]any{"code": code})
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	claims := tokenPayload(t, r.str("accessToken"))
	assert.Equal(t, linkedTo, claims["linked_principal_id"])
	assert.Equal(t, actor, claims["act"].(map[string]any)["sub"])

	// Refreshing the impersonated session follows the worker's current link.
	e.unlinkWorker(workerID)
	rr := d.refresh(r.str("refreshToken"))
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assert.NotContains(t, tokenPayload(t, rr.str("accessToken")), "linked_principal_id")

	// Act-as: a supervisor signs in as a linked worker; the supervisor's own
	// link is not what the token carries.
	a := newActAsEnv(t, roomyLimits())
	supLink, targetLink := a.e.principal(true), a.e.principal(true)
	a.e.linkWorker(a.supID, supLink)
	a.e.linkWorker(a.targetID, targetLink)
	res := a.actAs(map[string]any{"workerId": a.targetID, "pin": goodPIN})
	require.Equal(t, http.StatusOK, res.Code, res.Body.String())
	asClaims := tokenPayload(t, res.str("accessToken"))
	assert.Equal(t, targetLink, asClaims["linked_principal_id"])
	assert.Equal(t, a.supID, asClaims["act"].(map[string]any)["sub"])
	// The supervisor's own token (minted before the link) is unchanged; a
	// refresh of it carries the supervisor's link.
	sr := a.d.refresh(a.sup.refresh)
	require.Equal(t, http.StatusOK, sr.Code, sr.Body.String())
	assert.Equal(t, supLink, tokenPayload(t, sr.str("accessToken"))["linked_principal_id"])
}
