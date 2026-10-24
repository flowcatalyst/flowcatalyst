package server

import (
	passwordresetapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/passwordreset/api"
	resetapprovalapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/resetapproval/api"
)

// resetApprovalState is the reset approvals API's state. The /api routes and
// the server-rendered reset approvals page (internal/ui/pages/reset_approvals.go)
// run the same handlers, gates and emailer.
func resetApprovalState(cfg EnvCfg, repos *repoSet, svcs *serviceSet) *resetapprovalapi.State {
	return &resetapprovalapi.State{
		Approvals:  repos.resetApprovalRepo,
		Principals: repos.principalRepo,
		Sender:     passwordresetapi.NewPrincipalEmailer(repos.resetTokenRepo, cfg.JWTIssuer, svcs.emailSvc, repos.platformConfigRepo),
	}
}
