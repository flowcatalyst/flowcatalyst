package server

import (
	passwordresetapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/passwordreset/api"
	principalapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/api"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// principalAPIState is the principal API's state. The /api routes and the
// server-rendered users page (internal/ui/pages/users*.go) are built from it,
// so the page runs the same handlers, gates and emailers as the API.
func principalAPIState(cfg EnvCfg, repos *repoSet, svcs *serviceSet, uow *usecasepgx.UnitOfWork) *principalapi.State {
	// The admin-triggered reset and the invitation share the public flow's
	// token repo and mailer.
	emailer := passwordresetapi.NewPrincipalEmailer(repos.resetTokenRepo, cfg.JWTIssuer, svcs.emailSvc, repos.platformConfigRepo)
	return &principalapi.State{
		Repo:              repos.principalRepo,
		Versions:          svcs.principalVersions,
		GrantRepo:         repos.principalGrantRepo,
		Roles:             repos.roleRepo,
		Applications:      repos.applicationRepo,
		ClientConfigs:     repos.applicationClientConfigRepo,
		Clients:           repos.clientRepo,
		Mappings:          repos.edmRepo,
		IdentityProviders: repos.idpRepo,
		AnchorDomains:     repos.authRepo.AnchorDomains,
		PasswordEmailer:   emailer,
		InviteEmailer:     emailer,
		Notifier:          svcs.notifier,
		MFA:               svcs.mfaSvc,
		Audit:             repos.auditRepo,
		UoW:               uow,
	}
}
