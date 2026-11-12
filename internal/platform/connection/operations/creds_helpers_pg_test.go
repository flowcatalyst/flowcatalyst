//go:build integration

package operations_test

import (
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	subscriptionops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// plainAccounts are the service-account ids the older tests in this package
// name on connections. CreateConnection checks the account exists and the
// caller may use it, so each is seeded (active, no client links, no
// application): usable by an all-applications operator.
var plainAccounts = []string{
	"sva_x", "sac_identconn0001", "sva_conntestseed1", "sva_conncrthappy1", "sva_conndup2",
	"sva_subsync1",
}

// accts returns the service-account repository, with plainAccounts seeded.
func accts(t *testing.T) *serviceaccount.Repository {
	t.Helper()
	for _, id := range plainAccounts {
		testpg.SeedServiceAccount(t, id, testpg.ServiceAccountSpec{})
	}
	return serviceaccount.NewRepository(testpg.Pool(t))
}

// subCreds are the lookups CreateSubscription needs.
func subCreds(t *testing.T) subscriptionops.CredentialRepos {
	t.Helper()
	pool := testpg.Pool(t)
	return subscriptionops.CredentialRepos{
		Accounts: accts(t), Connections: connection.NewRepository(pool), Apps: application.NewRepository(pool),
	}
}
