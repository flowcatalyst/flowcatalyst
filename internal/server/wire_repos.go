package server

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/loginattempt"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/passwordreset"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/resetapproval"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
)

// repoSet bundles every subdomain repository WirePlatform builds against
// the pool. Field names match the original wire.go locals so the wiring
// code reads as `repos.<old local name>`.
type repoSet struct {
	clientRepo                  *client.Repository
	roleRepo                    *role.Repository
	applicationRepo             *application.Repository
	applicationClientConfigRepo *application.ClientConfigRepo
	principalRepo               *principal.Repository
	principalGrantRepo          *principal.ClientAccessGrantRepo
	serviceAccountRepo          *serviceaccount.Repository
	authRepo                    *auth.Repository
	corsRepo                    *cors.Repository
	connectionRepo              *connection.Repository
	subscriptionRepo            *subscription.Repository
	dispatchPoolRepo            *dispatchpool.Repository
	dispatchJobRepo             *dispatchjob.Repository
	eventTypeRepo               *eventtype.Repository
	eventRepo                   *event.Repository
	auditRepo                   *audit.Repository
	idpRepo                     *identityprovider.Repository
	edmRepo                     *emaildomainmapping.Repository
	loginAttemptRepo            *loginattempt.Repository
	platformConfigRepo          *platformconfig.Repository
	processRepo                 *process.Repository
	scheduledJobRepo            *scheduledjob.Repository
	webauthnCredRepo            *webauthn.Repository
	webauthnCeremonyRepo        *webauthn.CeremonyRepository
	resetTokenRepo              *passwordreset.Repository
	resetApprovalRepo           *resetapproval.Repository
}

func buildRepos(pool *pgxpool.Pool) *repoSet {
	return &repoSet{
		clientRepo:                  client.NewRepository(pool),
		roleRepo:                    role.NewRepository(pool),
		applicationRepo:             application.NewRepository(pool),
		applicationClientConfigRepo: application.NewClientConfigRepo(pool),
		principalRepo:               principal.NewRepository(pool),
		principalGrantRepo:          principal.NewClientAccessGrantRepo(pool),
		serviceAccountRepo:          serviceaccount.NewRepository(pool),
		authRepo:                    auth.NewRepository(pool),
		corsRepo:                    cors.NewRepository(pool),
		connectionRepo:              connection.NewRepository(pool),
		subscriptionRepo:            subscription.NewRepository(pool),
		dispatchPoolRepo:            dispatchpool.NewRepository(pool),
		dispatchJobRepo:             dispatchjob.NewRepository(pool),
		eventTypeRepo:               eventtype.NewRepository(pool),
		eventRepo:                   event.NewRepository(pool),
		auditRepo:                   audit.NewRepository(pool),
		idpRepo:                     identityprovider.NewRepository(pool),
		edmRepo:                     emaildomainmapping.NewRepository(pool),
		loginAttemptRepo:            loginattempt.NewRepository(pool),
		platformConfigRepo:          platformconfig.NewRepository(pool),
		processRepo:                 process.NewRepository(pool),
		scheduledJobRepo:            scheduledjob.NewRepository(pool),
		webauthnCredRepo:            webauthn.NewRepository(pool),
		webauthnCeremonyRepo:        webauthn.NewCeremonyRepository(pool),
		resetTokenRepo:              passwordreset.NewRepository(pool),
		resetApprovalRepo:           resetapproval.NewRepository(pool),
	}
}
