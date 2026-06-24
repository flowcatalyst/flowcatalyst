//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	edmops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// noopInviteEmailer satisfies InviteEmailer so notifyNewUser short-circuits
// before touching the (nil) Notifier.
type noopInviteEmailer struct{}

func (noopInviteEmailer) SendInvite(context.Context, *principal.Principal) error { return nil }

// TestBulkImport_DropsForeignDomainAndExisting pins the platform-admin import
// rules: a row whose email domain is registered to a DIFFERENT client is
// dropped, an already-present email is skipped, a domain owned by the TARGET
// client is allowed, and a free domain is created — all reported per row.
func TestBulkImport_DropsForeignDomainAndExisting(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	uow := testpg.NewUoW(t)
	edm := emaildomainmapping.NewRepository(pool)

	targetClient := "cli_bulkimp_tgt1" // ≤ VARCHAR(17)
	otherClient := "cli_bulkimp_oth1"

	// "foreign.test" is owned by another client; "mine.test" is owned by the
	// import target. CreateMapping validates formats only — dummy ids are ok.
	mustMapping(t, ctx, edm, uow, "foreign.test", otherClient)
	mustMapping(t, ctx, edm, uow, "mine.test", targetClient)

	// Seed an already-existing user so that row reports "exists".
	name := "Dupe"
	_, err := operations.CreateUser(ctx, repo, uow,
		operations.CreateCommand{Email: "dupe@free.test", Name: &name, Scope: "ANCHOR"}, testpg.TestEC())
	require.NoError(t, err)

	s := &State{Repo: repo, UoW: uow, Mappings: edm, InviteEmailer: noopInviteEmailer{}}
	authCtx := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "p_bulkimp_admin", Scope: auth.ScopeAnchor, // platform admin
	})

	out, err := s.bulkImport(authCtx, &apicommon.In[BulkImportRequest]{Body: BulkImportRequest{
		ClientID: targetClient,
		Users: []BulkImportUser{
			{Name: "Alice", Email: "alice@foreign.test"}, // dropped: foreign domain
			{Name: "Bob", Email: "bob@free.test"},        // created: unmapped domain
			{Name: "Carol", Email: "carol@mine.test"},    // created: target-owned domain
			{Name: "Dupe", Email: "dupe@free.test"},      // exists
		},
	}})
	require.NoError(t, err)

	assert.Equal(t, 2, out.Body.Created, "bob + carol")
	assert.Equal(t, 2, out.Body.Skipped, "alice (dropped) + dupe (exists)")
	assert.Equal(t, 0, out.Body.Failed)

	byEmail := map[string]BulkImportResult{}
	for _, r := range out.Body.Results {
		byEmail[r.Email] = r
	}
	assert.Equal(t, "dropped", byEmail["alice@foreign.test"].Status)
	assert.Contains(t, byEmail["alice@foreign.test"].Message, "another client")
	assert.Equal(t, "created", byEmail["bob@free.test"].Status)
	assert.Equal(t, "created", byEmail["carol@mine.test"].Status)
	assert.Equal(t, "exists", byEmail["dupe@free.test"].Status)

	// The dropped user must NOT have been created.
	got, err := repo.FindByEmail(ctx, "alice@foreign.test")
	require.NoError(t, err)
	assert.Nil(t, got, "a dropped row creates no user")
}

func mustMapping(t *testing.T, ctx context.Context, edm *emaildomainmapping.Repository, uow *usecasepgx.UnitOfWork, domain, primaryClient string) {
	t.Helper()
	pc := primaryClient
	_, err := edmops.CreateMapping(ctx, edm, uow, edmops.CreateCommand{
		EmailDomain:        domain,
		IdentityProviderID: "idp_bulkimpseed01",
		ScopeType:          "CLIENT",
		PrimaryClientID:    &pc,
	}, testpg.TestEC())
	require.NoError(t, err)
}
