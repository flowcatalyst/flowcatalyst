//go:build integration

package pages

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// operatorCtx is an operator: anchor reach with every permission and access
// to all applications, as every ordinary user has. (testpg.AnchorCtx is
// anchor but not all-applications, so it is confined on the application axis.)
func operatorCtx() context.Context {
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID:     "prn_optestrunner1",
		Scope:           auth.ScopeAnchor,
		Permissions:     []string{"platform:*:*:*"},
		AllApplications: true,
	})
}
