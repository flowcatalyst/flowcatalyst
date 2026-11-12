//go:build integration

package control

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// operatorCtx is an operator: anchor reach, every permission, access to all
// applications (testpg.AnchorCtx is confined on the application axis, which
// the function operations these tests drive now enforce).
func operatorCtx() context.Context {
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID:     "prn_optestrunner1",
		Scope:           auth.ScopeAnchor,
		Permissions:     []string{"platform:*:*:*"},
		AllApplications: true,
	})
}
