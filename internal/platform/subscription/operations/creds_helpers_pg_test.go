//go:build integration

package operations_test

import (
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// plainAccounts are the service-account ids the older tests in this package
// name. The delivery-credential checks load the real row, so each is seeded
// (active, no client links, no application): usable by an all-applications
// operator.
var plainAccounts = []string{"sva_subcrthappy1", "sva_subupdafter1", "sva_subsync1"}

// credRepos are the lookups the create and update operations check
// delivery credentials with, over the shared test database.
func credRepos(t *testing.T) operations.CredentialRepos {
	t.Helper()
	pool := testpg.Pool(t)
	for _, id := range plainAccounts {
		testpg.SeedServiceAccount(t, id, testpg.ServiceAccountSpec{})
	}
	return operations.CredentialRepos{
		Accounts:    serviceaccount.NewRepository(pool),
		Connections: connection.NewRepository(pool),
		Apps:        application.NewRepository(pool),
	}
}
