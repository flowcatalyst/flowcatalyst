//go:build integration

// Pinning tests for PR-3/PR-4 (docs/owner-rulings-todo.md #3) at the HTTP
// handler layer: the by-id read's cross-tenant response changes from 403 to
// 404 (a deliberate wire change — see CONVENTIONS.md §5a on the openapi
// lockfile), and every /{id}/... sub-route carries the identical client-scope
// check, answering the SAME body a genuinely missing id would.
package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// envelopeOf renders err exactly the way httperror.Write does, so tests can
// assert two errors produce a BYTE-IDENTICAL wire response, not just the
// same Kind/Code.
func envelopeOf(err error) (status int, env httperror.Envelope) {
	status = httperror.Status(err)
	env = httperror.Envelope{Code: "INTERNAL", Message: "Internal server error"}
	if uc := usecase.AsError(err); uc != nil {
		env.Code, env.Message, env.Details = uc.Code, uc.Message, uc.Details
	}
	return status, env
}

// assertCanonicalNotFound asserts got is EXACTLY the envelope
// httperror.NotFound(resource, id) would produce — i.e. the response is
// indistinguishable from the genuine not-found path for that same id: same
// Code, same Message template, no extra Details, nothing marking it as a
// scope-denial rather than a real miss. "Byte-identical to not-found" can't
// mean the literal message text matches ACROSS two different ids (a
// cross-tenant real id vs. a fabricated missing one) — the id is part of the
// message by construction — so this checks each path against its own
// canonical not-found shape instead of against each other's literal string.
func assertCanonicalNotFound(t *testing.T, resource, id string, got httperror.Envelope, label string) {
	t.Helper()
	_, want := envelopeOf(httperror.NotFound(resource, id))
	assert.Equal(t, want, got, "%s response must be the exact canonical %s not-found shape (no distinguishing marker)", label, resource)
}

// scopeFixture seeds two clients and a CLIENT-scope user homed at clientB,
// plus a clientA-scoped admin AuthContext — the shared setup for every
// cross-tenant test in this file.
type scopeFixture struct {
	s         *State
	adminCtx  context.Context
	targetID  string
	missingID string
}

func newScopeFixture(t *testing.T, suffix string) scopeFixture {
	t.Helper()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	clients := client.NewRepository(pool)
	uow := testpg.NewUoW(t)

	evA, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Scope Client A " + suffix, Identifier: "scope-cli-a-" + suffix}, testpg.TestEC())
	require.NoError(t, err)
	evB, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Scope Client B " + suffix, Identifier: "scope-cli-b-" + suffix}, testpg.TestEC())
	require.NoError(t, err)
	clientBID := evB.ClientID

	target, err := usecaseop.Run(testpg.AnchorCtx(), uow, principalops.CreateUser(repo),
		principalops.CreateCommand{Email: "prn-scope-" + suffix + "@example.com", Scope: "CLIENT", ClientID: &clientBID}, testpg.TestEC())
	require.NoError(t, err)

	adminCtx := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_scopeadmin_" + suffix,
		Scope:       auth.ScopeClient,
		Clients:     []string{evA.ClientID},
		Permissions: []string{"platform:iam:user:view", "platform:iam:user:create", "platform:iam:user:update", "platform:iam:user:delete"},
	})

	return scopeFixture{
		s:         &State{Repo: repo, UoW: uow, Clients: clients},
		adminCtx:  adminCtx,
		targetID:  target.UserID,
		missingID: "prn_doesnotexist_" + suffix,
	}
}

// TestGetByID_CrossTenant_NotFoundByteIdenticalToMissing pins the wire
// change: GET /api/principals/{id}'s old cross-tenant 403 FORBIDDEN becomes
// 404, byte-identical to a genuinely unknown id.
func TestGetByID_CrossTenant_NotFoundByteIdenticalToMissing(t *testing.T) {
	fx := newScopeFixture(t, "getbyid")

	_, crossErr := fx.s.getByID(fx.adminCtx, &apicommon.IDInput{ID: fx.targetID})
	_, missingErr := fx.s.getByID(fx.adminCtx, &apicommon.IDInput{ID: fx.missingID})

	require.Error(t, crossErr)
	require.Error(t, missingErr)
	crossStatus, crossEnv := envelopeOf(crossErr)
	missingStatus, missingEnv := envelopeOf(missingErr)
	assert.Equal(t, http.StatusNotFound, crossStatus, "cross-tenant read must be 404, not 403")
	assert.Equal(t, http.StatusNotFound, missingStatus)
	assertCanonicalNotFound(t, "Principal", fx.targetID, crossEnv, "cross-tenant")
	assertCanonicalNotFound(t, "Principal", fx.missingID, missingEnv, "missing-id")
}

// TestListRoles_CrossTenant_NotFoundByteIdenticalToMissing pins PR-4: a
// clientA-scoped admin gets 404 (not 200, not 403) for a clientB principal's
// roles, and the SAME body a nonexistent id would produce.
func TestListRoles_CrossTenant_NotFoundByteIdenticalToMissing(t *testing.T) {
	fx := newScopeFixture(t, "listroles")

	out, crossErr := fx.s.listRoles(fx.adminCtx, &apicommon.IDInput{ID: fx.targetID})
	require.Nil(t, out, "must not return a 200 body for an out-of-scope target")
	_, missingErr := fx.s.listRoles(fx.adminCtx, &apicommon.IDInput{ID: fx.missingID})

	require.Error(t, crossErr)
	require.Error(t, missingErr)
	crossStatus, crossEnv := envelopeOf(crossErr)
	missingStatus, missingEnv := envelopeOf(missingErr)
	assert.Equal(t, http.StatusNotFound, crossStatus, "cross-tenant roles list must be 404, not 200 or 403")
	assert.Equal(t, http.StatusNotFound, missingStatus)
	assertCanonicalNotFound(t, "Principal", fx.targetID, crossEnv, "cross-tenant")
	assertCanonicalNotFound(t, "Principal", fx.missingID, missingEnv, "missing-id")
}

// TestListApplicationAccess_CrossTenant_NotFound and
// TestListAvailableApplications_CrossTenant_NotFound pin the same PR-4 shape
// on the other two previously-unscoped sub-routes the IDOR review flagged.
func TestListApplicationAccess_CrossTenant_NotFound(t *testing.T) {
	fx := newScopeFixture(t, "listappaccess")
	out, err := fx.s.listApplicationAccess(fx.adminCtx, &apicommon.IDInput{ID: fx.targetID})
	require.Nil(t, out)
	status, _ := envelopeOf(err)
	assert.Equal(t, http.StatusNotFound, status)
}

func TestListAvailableApplications_CrossTenant_NotFound(t *testing.T) {
	fx := newScopeFixture(t, "listavailapps")
	out, err := fx.s.listAvailableApplications(fx.adminCtx, &apicommon.IDInput{ID: fx.targetID})
	require.Nil(t, out)
	status, _ := envelopeOf(err)
	assert.Equal(t, http.StatusNotFound, status)
}

// TestAddRole_IdempotentSkip_CrossTenant_NotFoundNotLeaked pins the IDOR fix:
// calling addRole with a role the cross-tenant target already holds (or, as
// here, simply doesn't hold — either way the mutation is skipped) must still
// answer 404, not silently fall through the idempotent skip and return the
// target's full PrincipalResponse. Before the fix, the scope check lived only
// inside the AssignRoles use case, which the idempotent branch never reaches.
func TestAddRole_IdempotentSkip_CrossTenant_NotFoundNotLeaked(t *testing.T) {
	fx := newScopeFixture(t, "addrole")

	out, err := fx.s.addRole(fx.adminCtx, &addRoleInput{ID: fx.targetID, Body: AddRoleRequest{Role: "does-not-matter:role"}})
	require.Nil(t, out, "must never leak the cross-tenant principal's data")
	status, _ := envelopeOf(err)
	assert.Equal(t, http.StatusNotFound, status)
}

// TestRemoveRole_IdempotentSkip_CrossTenant_NotFoundNotLeaked mirrors the
// addRole case for DELETE /{id}/roles/{role} — the target holds no roles at
// all, so the removal is idempotent-skipped, and the fix must still deny
// before reaching that skip.
func TestRemoveRole_IdempotentSkip_CrossTenant_NotFoundNotLeaked(t *testing.T) {
	fx := newScopeFixture(t, "removerole")

	out, err := fx.s.removeRole(fx.adminCtx, &removeRoleInput{ID: fx.targetID, Role: "does-not-matter:role"})
	require.Nil(t, out, "must never leak the cross-tenant principal's data")
	status, _ := envelopeOf(err)
	assert.Equal(t, http.StatusNotFound, status)
}

// TestSetDeveloperCredential_NoPermission_NothingTouched pins PR-3(a): a
// caller with NO write-principals permission and no self-service developer
// permission gets 403 before ANY load — proven here by targeting an id that
// doesn't even exist and still getting a plain permission error, not a
// not-found (which would mean the handler tried to load first).
func TestSetDeveloperCredential_NoPermission_NothingTouched(t *testing.T) {
	noPerm := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_nopermcaller1", Scope: auth.ScopeClient,
	})
	s := &State{}
	_, err := s.setDeveloperCredential(noPerm, &apicommon.IDInput{ID: "prn_whatever_nonexistent"})
	require.Error(t, err)
	status, env := envelopeOf(err)
	assert.Equal(t, http.StatusForbidden, status, "no permission must deny before any load — never 404")
	assert.NotEqual(t, "Principal_NOT_FOUND", env.Code)
}
