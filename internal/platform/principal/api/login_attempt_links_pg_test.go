//go:build integration

package api

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	serviceaccountops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestGetPrincipal_ServiceAccountID pins B2 (docs/spec/login-attempt-links.md,
// T4): a SERVICE principal's response carries serviceAccountId equal to its
// linked account's id — the login-attempt detail dialog (F2) uses this to
// route a DEVELOPER_TOKEN/SERVICE row to the service-account detail screen —
// while a USER principal's response carries no such field at all.
func TestGetPrincipal_ServiceAccountID(t *testing.T) {
	// Provisioning a service account encrypts its OAuth client secret via
	// encryption.FromEnv, which reads the env at call time — this package's
	// shared TestMain doesn't seed it (unlike serviceaccount/api's), so seed
	// it here.
	if os.Getenv("FLOWCATALYST_APP_KEY") == "" {
		key, err := encryption.GenerateKey()
		require.NoError(t, err)
		t.Setenv("FLOWCATALYST_APP_KEY", key)
	}

	pool := testpg.Pool(t)
	principals := principal.NewRepository(pool)
	saRepo := serviceaccount.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	uow := testpg.NewUoW(t)

	res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
		serviceaccountops.CreateServiceAccountWithCredentials(saRepo, principals, oauthRepo,
			client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool)),
		serviceaccountops.CreateCommand{Code: "principal-link-sa", Name: "Principal Link SA"}, testpg.TestEC())
	require.NoError(t, err)

	s := &State{Repo: principals}

	svcOut, err := s.getByID(testpg.AnchorCtx(), &apicommon.IDInput{ID: res.PrincipalID})
	require.NoError(t, err)
	require.NotNil(t, svcOut.Body.ServiceAccountID,
		"a SERVICE principal must report the id of the account it backs")
	assert.Equal(t, res.ServiceAccount.ID, *svcOut.Body.ServiceAccountID,
		"serviceAccountId must equal the linked account's own id")

	// A USER principal carries no serviceAccountId at all.
	name := "Plain User"
	pw := "Correct-Horse-Battery-Staple-9!"
	ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, principalops.CreateUser(principals),
		principalops.CreateCommand{
			Email: "plain-user-satest@example.test", Name: &name, Scope: "ANCHOR", Password: &pw,
		}, testpg.TestEC())
	require.NoError(t, err)

	userOut, err := s.getByID(testpg.AnchorCtx(), &apicommon.IDInput{ID: ev.UserID})
	require.NoError(t, err)
	assert.Nil(t, userOut.Body.ServiceAccountID, "a USER principal must not carry serviceAccountId")
}
