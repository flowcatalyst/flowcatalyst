//go:build integration

package testpg

import (
	"context"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// OperatorCtx is an anchor principal with every permission and
// all-applications access: an ordinary platform operator. AnchorCtx is anchor
// with wildcard permissions but is NOT all-applications, which is how an
// application's own service account looks.
func OperatorCtx() context.Context {
	return WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID:     "prn_optestrunner1",
		Scope:           auth.ScopeAnchor,
		Permissions:     []string{"platform:*:*:*"},
		AllApplications: true,
	})
}

// ServiceAccountSpec is what SeedServiceAccount writes.
type ServiceAccountSpec struct {
	// ClientIDs are the clients the account is linked to (none: an
	// application-provisioned, anchor-only account).
	ClientIDs []string
	// ApplicationID binds the account to an application ("" = none).
	ApplicationID string
	// Inactive seeds a deactivated account.
	Inactive bool
}

// SeedServiceAccount inserts (or resets) the service account id, with the
// code equal to the id, and returns id. Delivery-credential checks load the
// real row, so tests that name an account must seed it.
func SeedServiceAccount(t *testing.T, id string, spec ServiceAccountSpec) string {
	t.Helper()
	var app *string
	if spec.ApplicationID != "" {
		app = &spec.ApplicationID
	}
	clients := spec.ClientIDs
	if clients == nil {
		clients = []string{}
	}
	if _, err := Pool(t).Exec(context.Background(),
		`INSERT INTO iam_service_accounts (id, code, name, application_id, client_ids, active, created_at, updated_at)
		 VALUES ($1, $1, $1, $2, $3, $4, now(), now())
		 ON CONFLICT (id) DO UPDATE SET application_id = $2, client_ids = $3, active = $4`,
		id, app, clients, !spec.Inactive); err != nil {
		t.Fatalf("testpg: seed service account %q: %v", id, err)
	}
	return id
}
