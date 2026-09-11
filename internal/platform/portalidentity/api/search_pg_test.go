//go:build integration

package api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestListPortalUsersSearchAndState pins GET /api/portal-users: TERM%
// prefix search on email and name (case-insensitive, LIKE metacharacters
// literal), the per-app filter, pagination totals, and the derived state —
// an ensure with the platform mailer records the invite (INVITED) and a set
// password moves it to ACTIVE.
func TestListPortalUsersSearchAndState(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	clients := client.NewRepository(pool)
	identities := portalidentity.NewRepository(pool)
	apps := portalidentity.NewAppRepository(pool)
	uow := testpg.NewUoW(t)

	clientEv, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Search Co", Identifier: "portal-search-co"}, testpg.TestEC())
	require.NoError(t, err)
	tenantID := clientEv.ClientID
	_, err = usecaseop.Run(ctx, uow, portalidentity.CreateApp(apps, clients),
		portalidentity.CreateAppCommand{ClientID: tenantID, Code: "suppliers", Name: "Supplier Portal"}, testpg.TestEC())
	require.NoError(t, err)

	s := &State{Identities: identities, Apps: apps, Clients: clients, UoW: uow, Invites: &fakeInviteMinter{}}
	actx := testpg.AnchorCtx()
	ensure := func(email, name string, appCode *string) PortalUserResponse {
		out, err := s.ensure(actx, &apicommon.In[PortalUserRequest]{Body: PortalUserRequest{
			ClientID: tenantID, Email: email, Name: &name, PortalAppCode: appCode,
		}})
		require.NoError(t, err)
		return out.Body
	}
	suppliers := "SUPPLIERS"
	pat := ensure("pat.jones@search.test", "Pat Jones", &suppliers)
	assert.Equal(t, "INVITED", pat.State, "a delivered invite is INVITED")
	require.NotNil(t, pat.PortalAppCode)
	assert.Equal(t, "suppliers", *pat.PortalAppCode)
	ensure("sam@search.test", "Jonas Smith", nil)
	ensure("under_score@search.test", "Literal", nil)

	list := func(q, app string, page, size int) PortalUserListResponse {
		out, err := s.list(actx, &listInput{ClientID: tenantID, Q: q, PortalAppCode: app, Page: page, Size: size})
		require.NoError(t, err)
		return out.Body
	}
	emails := func(r PortalUserListResponse) []string {
		out := []string{}
		for _, u := range r.PortalUsers {
			out = append(out, u.Email)
		}
		return out
	}

	assert.EqualValues(t, 3, list("", "", 0, 0).Total)
	assert.ElementsMatch(t, []string{"pat.jones@search.test"}, emails(list("PAT", "", 0, 0)), "email prefix, case-insensitive")
	assert.ElementsMatch(t, []string{"sam@search.test"}, emails(list("jonas", "", 0, 0)), "name prefix")
	assert.Empty(t, emails(list("jones", "", 0, 0)), "prefix only — no infix match on 'Pat Jones'")
	assert.ElementsMatch(t, []string{"under_score@search.test"}, emails(list("under_", "", 0, 0)))
	assert.Empty(t, emails(list("u%", "", 0, 0)), "% is literal, not a wildcard")
	assert.ElementsMatch(t, []string{"pat.jones@search.test"}, emails(list("", "suppliers", 0, 0)), "app filter")

	paged := list("", "", 1, 2)
	assert.EqualValues(t, 3, paged.Total)
	assert.Len(t, paged.PortalUsers, 1, "page 1 of size 2 holds the third row")

	row := list("pat", "", 0, 0).PortalUsers[0]
	assert.Equal(t, "INVITED", row.State)
	require.Len(t, row.Apps, 1)
	assert.Equal(t, "suppliers", row.Apps[0].Code)
	require.NotNil(t, row.InviteExpiresAt)

	// Accepting the invite (password set) moves the state to ACTIVE.
	require.NoError(t, identities.SetPasswordHash(ctx, pat.IdentityID, "argon2id$test"))
	assert.Equal(t, "ACTIVE", list("pat", "", 0, 0).PortalUsers[0].State)

	// A lapsed, never-completed invite reads INVITE_EXPIRED.
	sam := list("sam", "", 0, 0).PortalUsers[0]
	past := time.Now().Add(-time.Hour)
	require.NoError(t, identities.MarkInvited(ctx, sam.IdentityID, past.Add(-72*time.Hour), &past))
	assert.Equal(t, "INVITE_EXPIRED", list("sam", "", 0, 0).PortalUsers[0].State)

	// Unknown app code → 404, not an unfiltered list.
	_, err = s.list(actx, &listInput{ClientID: tenantID, PortalAppCode: "nope"})
	require.Error(t, err)
}
