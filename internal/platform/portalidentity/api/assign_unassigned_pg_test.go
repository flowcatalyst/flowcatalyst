//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestAssignUnassignedPortalUsers pins the gap-closing bulk use case: every
// portal user of the client holding NO portal app is granted the chosen app
// (suspended ones included — status is untouched); users already holding
// any app are left alone; a second run assigns nobody. The unassigned list
// filter and the apps list's unassignedUsers count track it.
func TestAssignUnassignedPortalUsers(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	clients := client.NewRepository(pool)
	identities := portalidentity.NewRepository(pool)
	apps := portalidentity.NewAppRepository(pool)
	uow := testpg.NewUoW(t)
	s := &State{Identities: identities, Apps: apps, Clients: clients, UoW: uow}
	actx := testpg.AnchorCtx()

	clientEv, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Assign Co", Identifier: "portal-assign-co"}, testpg.TestEC())
	require.NoError(t, err)
	tenantID := clientEv.ClientID
	mkApp := func(code string) string {
		ev, err := usecaseop.Run(ctx, uow, portalidentity.CreateApp(apps, clients),
			portalidentity.CreateAppCommand{ClientID: tenantID, Code: code, Name: code}, testpg.TestEC())
		require.NoError(t, err)
		return ev.AppID
	}
	customers, suppliers := mkApp("customers"), mkApp("suppliers")

	ensure := func(email, appID string) string {
		ev, err := usecaseop.Run(ctx, uow, portalidentity.Ensure(identities, clients, apps),
			portalidentity.EnsureCommand{ClientID: tenantID, Email: email, Source: "INVITE", PortalAppID: appID}, testpg.TestEC())
		require.NoError(t, err)
		return ev.IdentityID
	}
	legacyA := ensure("legacy-a@assign.test", "")
	legacyB := ensure("legacy-b@assign.test", "")
	granted := ensure("granted@assign.test", suppliers)
	_, err = usecaseop.Run(ctx, uow, portalidentity.SetStatus(identities),
		portalidentity.SetStatusCommand{ID: legacyB, ClientID: tenantID, Status: "DISABLED"}, testpg.TestEC())
	require.NoError(t, err)

	listApps := func() PortalAppListResponse {
		out, err := s.listApps(actx, &listAppsInput{ClientID: tenantID})
		require.NoError(t, err)
		return out.Body
	}
	require.NotNil(t, listApps().UnassignedUsers)
	assert.EqualValues(t, 2, *listApps().UnassignedUsers)

	unassigned, err := s.list(actx, &listInput{ClientID: tenantID, Unassigned: true})
	require.NoError(t, err)
	assert.EqualValues(t, 2, unassigned.Body.Total, "the filter finds exactly the users with no app")

	_, err = s.list(actx, &listInput{ClientID: tenantID, Unassigned: true, PortalAppCode: "customers"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FILTER_CONFLICT")

	out, err := s.assignUnassigned(actx, &assignUnassignedInput{ID: customers, Body: AssignUnassignedBody{ClientID: tenantID}})
	require.NoError(t, err)
	assert.Equal(t, "customers", out.Body.PortalAppCode)
	assert.Equal(t, 2, out.Body.Assigned)

	for _, id := range []string{legacyA, legacyB} {
		ident, err := identities.FindByID(ctx, id)
		require.NoError(t, err)
		assert.True(t, ident.HasApp(customers), id)
	}
	suspended, err := identities.FindByID(ctx, legacyB)
	require.NoError(t, err)
	assert.Equal(t, portalidentity.StatusDisabled, suspended.Status, "assignment never reactivates")
	other, err := identities.FindByID(ctx, granted)
	require.NoError(t, err)
	assert.False(t, other.HasApp(customers), "users already holding an app are untouched")
	assert.True(t, other.HasApp(suppliers))

	assert.EqualValues(t, 0, *listApps().UnassignedUsers)
	again, err := s.assignUnassigned(actx, &assignUnassignedInput{ID: customers, Body: AssignUnassignedBody{ClientID: tenantID}})
	require.NoError(t, err)
	assert.Equal(t, 0, again.Body.Assigned, "idempotent: nobody left to assign")

	// An inactive app cannot be bulk-assigned.
	inactive := false
	_, err = usecaseop.Run(ctx, uow, portalidentity.UpdateApp(apps),
		portalidentity.UpdateAppCommand{ClientID: tenantID, ID: suppliers, Active: &inactive}, testpg.TestEC())
	require.NoError(t, err)
	_, err = s.assignUnassigned(actx, &assignUnassignedInput{ID: suppliers, Body: AssignUnassignedBody{ClientID: tenantID}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PORTAL_APP_INACTIVE")
}

// TestPersistKeepsConcurrentGrant pins the persist semantics: saving an
// identity deletes only the grants it explicitly revoked, so a grant another
// request added after this copy was loaded survives (it used to be wiped by
// a "delete whatever isn't in the loaded set" sync).
func TestPersistKeepsConcurrentGrant(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	clients := client.NewRepository(pool)
	identities := portalidentity.NewRepository(pool)
	apps := portalidentity.NewAppRepository(pool)
	uow := testpg.NewUoW(t)

	clientEv, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Race Co", Identifier: "portal-race-co"}, testpg.TestEC())
	require.NoError(t, err)
	tenantID := clientEv.ClientID
	mkApp := func(code string) string {
		ev, err := usecaseop.Run(ctx, uow, portalidentity.CreateApp(apps, clients),
			portalidentity.CreateAppCommand{ClientID: tenantID, Code: code, Name: code}, testpg.TestEC())
		require.NoError(t, err)
		return ev.AppID
	}
	a, b, c := mkApp("race-a"), mkApp("race-b"), mkApp("race-c")
	ev, err := usecaseop.Run(ctx, uow, portalidentity.Ensure(identities, clients, apps),
		portalidentity.EnsureCommand{ClientID: tenantID, Email: "race@race.test", Source: "INVITE", PortalAppID: a}, testpg.TestEC())
	require.NoError(t, err)

	stale, err := identities.FindByID(ctx, ev.IdentityID) // holds {a}
	require.NoError(t, err)

	// Meanwhile another request grants b.
	_, err = usecaseop.Run(ctx, uow, portalidentity.GrantApp(identities, apps),
		portalidentity.AppGrantCommand{ClientID: tenantID, IdentityID: ev.IdentityID, PortalAppID: b}, testpg.TestEC())
	require.NoError(t, err)

	// The stale copy grants c and revokes a, then saves.
	stale.Grant(c, portalidentity.SourceAdmin)
	stale.Revoke(a)
	_, err = usecaseop.Run(ctx, uow, portalidentity.SetStatus(identities),
		portalidentity.SetStatusCommand{ID: ev.IdentityID, ClientID: tenantID, Status: "ACTIVE"}, testpg.TestEC())
	require.NoError(t, err) // (unrelated save, loads fresh — must not disturb grants either)
	saveStale := usecaseop.Operation[struct{}, portalidentity.IdentityAppGranted]{
		Name:      "SaveStaleCopy",
		Authorize: usecaseop.Public[struct{}],
		Execute: func(_ context.Context, _ struct{}, ec usecase.ExecutionContext) (usecaseop.Plan[portalidentity.IdentityAppGranted], error) {
			return usecaseop.Save(stale, identities, portalidentity.IdentityAppGranted{
				Metadata:   usecase.NewEventMetadata(ec, portalidentity.IdentityAppGrantedType, portalidentity.EventSource, "platform.portal-identity."+stale.ID),
				IdentityID: stale.ID, ClientID: stale.ClientID, AppID: c, AppCode: "race-c",
			}), nil
		},
	}
	_, err = usecaseop.Run(ctx, uow, saveStale, struct{}{}, testpg.TestEC())
	require.NoError(t, err)

	fresh, err := identities.FindByID(ctx, ev.IdentityID)
	require.NoError(t, err)
	assert.False(t, fresh.HasApp(a), "explicit revoke applied")
	assert.True(t, fresh.HasApp(b), "concurrent grant survives")
	assert.True(t, fresh.HasApp(c), "own grant applied")
}
