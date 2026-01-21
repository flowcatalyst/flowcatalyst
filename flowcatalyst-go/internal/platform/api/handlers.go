package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.mongodb.org/mongo-driver/mongo"

	"go.flowcatalyst.tech/internal/config"
	"go.flowcatalyst.tech/internal/platform/application"
	"go.flowcatalyst.tech/internal/platform/audit"
	"go.flowcatalyst.tech/internal/platform/auth/oidc"
	"go.flowcatalyst.tech/internal/platform/client"
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/dispatchjob"
	"go.flowcatalyst.tech/internal/platform/dispatchpool"
	"go.flowcatalyst.tech/internal/platform/event"
	"go.flowcatalyst.tech/internal/platform/eventtype"
	"go.flowcatalyst.tech/internal/platform/permission"
	"go.flowcatalyst.tech/internal/platform/principal"
	"go.flowcatalyst.tech/internal/platform/role"
	"go.flowcatalyst.tech/internal/platform/serviceaccount"
	"go.flowcatalyst.tech/internal/platform/subscription"
)

// Handlers contains all API handlers
type Handlers struct {
	db     *mongo.Database
	config *config.Config

	// UnitOfWork for atomic operations
	unitOfWork common.UnitOfWork

	// Repositories
	eventRepo          event.Repository
	eventTypeRepo      eventtype.Repository
	subscriptionRepo   subscription.Repository
	dispatchPoolRepo   dispatchpool.Repository
	dispatchJobRepo    dispatchjob.Repository
	clientRepo         client.Repository
	principalRepo      principal.Repository
	roleRepo           role.Repository
	permissionRepo     permission.Repository
	applicationRepo    *application.Repository
	serviceAccountRepo *serviceaccount.Repository
	auditRepo          *audit.Repository
	oidcRepo           *oidc.Repository

	// Services
	auditService *audit.Service

	// Individual handlers
	eventHandler            *EventHandler
	eventTypeHandler        *EventTypeHandler        // Uses UseCases
	subscriptionHandler     *SubscriptionHandler     // Uses UseCases
	dispatchPoolHandler     *DispatchPoolHandler     // Uses UseCases
	clientHandler           *ClientAdminHandler      // Uses UseCases
	principalHandler        *PrincipalAdminHandler   // Uses UseCases
	roleHandler             *RoleHandler             // Uses UseCases
	serviceAccountHandler   *ServiceAccountHandler   // Uses UseCases
	bffEventHandler         *EventBffHandler
	bffDispatchHandler      *DispatchJobBffHandler
	bffEventTypeHandler     *EventTypeBffHandler     // BFF for EventTypes
	bffRoleHandler          *RoleBffHandler          // BFF for Roles
	bffRawEventHandler      *RawEventBffHandler      // Debug BFF for raw events
	bffRawDispatchHandler   *RawDispatchJobBffHandler // Debug BFF for raw dispatch jobs
	dispatchJobHandler      *DispatchJobHandler
	auditLogHandler         *AuditLogHandler
	authConfigHandler       *AuthConfigHandler
	anchorDomainHandler     *AnchorDomainHandler
	oauthClientHandler      *OAuthClientAdminHandler
	applicationAdminHandler *ApplicationAdminHandler // Uses UseCases
}

// NewHandlers creates all API handlers
func NewHandlers(mongoClient *mongo.Client, db *mongo.Database, cfg *config.Config) *Handlers {
	h := &Handlers{
		db:     db,
		config: cfg,
	}

	// Initialize UnitOfWork for atomic operations
	h.unitOfWork = common.NewMongoUnitOfWork(mongoClient, db)

	// Initialize repositories
	h.eventRepo = event.NewRepository(db)
	h.eventTypeRepo = eventtype.NewRepository(db)
	h.subscriptionRepo = subscription.NewRepository(db)
	h.dispatchPoolRepo = dispatchpool.NewRepository(db)
	h.dispatchJobRepo = dispatchjob.NewRepository(db)
	h.clientRepo = client.NewRepository(db)
	h.principalRepo = principal.NewRepository(db)
	h.roleRepo = role.NewRepository(db)
	h.permissionRepo = permission.NewRepository(db)
	h.applicationRepo = application.NewRepository(db)
	h.serviceAccountRepo = serviceaccount.NewRepository(db)
	h.auditRepo = audit.NewRepository(db)
	h.oidcRepo = oidc.NewRepository(db)

	// Initialize services
	h.auditService = audit.NewService(h.auditRepo)

	// Initialize handlers (with UseCases where applicable)
	h.eventHandler = NewEventHandler(h.eventRepo)
	h.eventTypeHandler = NewEventTypeHandler(h.eventTypeRepo, h.unitOfWork)
	h.subscriptionHandler = NewSubscriptionHandler(h.subscriptionRepo, h.unitOfWork)
	h.dispatchPoolHandler = NewDispatchPoolHandler(h.dispatchPoolRepo, h.unitOfWork)
	h.clientHandler = NewClientAdminHandler(h.clientRepo, h.unitOfWork)
	h.principalHandler = NewPrincipalAdminHandler(h.principalRepo, h.clientRepo, h.unitOfWork)
	h.roleHandler = NewRoleHandler(h.roleRepo, h.unitOfWork)
	h.serviceAccountHandler = NewServiceAccountHandler(h.serviceAccountRepo, h.unitOfWork)
	h.bffEventHandler = NewEventBffHandler(db)
	h.bffDispatchHandler = NewDispatchJobBffHandler(db)
	h.bffEventTypeHandler = NewEventTypeBffHandler(h.eventTypeRepo, h.unitOfWork)
	h.bffRoleHandler = NewRoleBffHandler(h.roleRepo, h.permissionRepo, h.applicationRepo, h.unitOfWork)
	h.bffRawEventHandler = NewRawEventBffHandler(db)
	h.bffRawDispatchHandler = NewRawDispatchJobBffHandler(db)
	h.dispatchJobHandler = NewDispatchJobHandler(h.dispatchJobRepo)
	h.auditLogHandler = NewAuditLogHandler(h.auditRepo, h.principalRepo)
	h.authConfigHandler = NewAuthConfigHandler(h.clientRepo)
	h.anchorDomainHandler = NewAnchorDomainHandler(h.clientRepo)
	h.oauthClientHandler = NewOAuthClientAdminHandler(h.oidcRepo)
	h.applicationAdminHandler = NewApplicationAdminHandler(h.applicationRepo, h.unitOfWork)

	return h
}

// Event handlers

func (h *Handlers) CreateEvent(w http.ResponseWriter, r *http.Request) {
	h.eventHandler.Create(w, r)
}

func (h *Handlers) CreateEventBatch(w http.ResponseWriter, r *http.Request) {
	h.eventHandler.CreateBatch(w, r)
}

func (h *Handlers) GetEvent(w http.ResponseWriter, r *http.Request) {
	h.eventHandler.Get(w, r)
}

// Event Type handlers

func (h *Handlers) ListEventTypes(w http.ResponseWriter, r *http.Request) {
	h.eventTypeHandler.List(w, r)
}

func (h *Handlers) CreateEventType(w http.ResponseWriter, r *http.Request) {
	h.eventTypeHandler.Create(w, r)
}

func (h *Handlers) GetEventType(w http.ResponseWriter, r *http.Request) {
	h.eventTypeHandler.Get(w, r)
}

func (h *Handlers) UpdateEventType(w http.ResponseWriter, r *http.Request) {
	h.eventTypeHandler.Update(w, r)
}

func (h *Handlers) DeleteEventType(w http.ResponseWriter, r *http.Request) {
	h.eventTypeHandler.Delete(w, r)
}

func (h *Handlers) ArchiveEventType(w http.ResponseWriter, r *http.Request) {
	h.eventTypeHandler.Archive(w, r)
}

// Event Type Schema handlers

func (h *Handlers) ListEventTypeSchemas(w http.ResponseWriter, r *http.Request) {
	h.eventTypeHandler.ListSchemas(w, r)
}

func (h *Handlers) GetEventTypeSchema(w http.ResponseWriter, r *http.Request) {
	h.eventTypeHandler.GetSchema(w, r)
}

func (h *Handlers) AddEventTypeSchema(w http.ResponseWriter, r *http.Request) {
	h.eventTypeHandler.AddSchema(w, r)
}

func (h *Handlers) FinaliseEventTypeSchema(w http.ResponseWriter, r *http.Request) {
	h.eventTypeHandler.FinaliseSchema(w, r)
}

func (h *Handlers) DeprecateEventTypeSchema(w http.ResponseWriter, r *http.Request) {
	h.eventTypeHandler.DeprecateSchema(w, r)
}

func (h *Handlers) DeleteEventTypeSchema(w http.ResponseWriter, r *http.Request) {
	h.eventTypeHandler.DeleteSchema(w, r)
}

// Subscription handlers

func (h *Handlers) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
	h.subscriptionHandler.List(w, r)
}

func (h *Handlers) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	h.subscriptionHandler.Create(w, r)
}

func (h *Handlers) GetSubscription(w http.ResponseWriter, r *http.Request) {
	h.subscriptionHandler.Get(w, r)
}

func (h *Handlers) UpdateSubscription(w http.ResponseWriter, r *http.Request) {
	h.subscriptionHandler.Update(w, r)
}

func (h *Handlers) DeleteSubscription(w http.ResponseWriter, r *http.Request) {
	h.subscriptionHandler.Delete(w, r)
}

func (h *Handlers) PauseSubscription(w http.ResponseWriter, r *http.Request) {
	h.subscriptionHandler.Pause(w, r)
}

func (h *Handlers) ResumeSubscription(w http.ResponseWriter, r *http.Request) {
	h.subscriptionHandler.Resume(w, r)
}

// Dispatch Pool handlers (using UseCases)

func (h *Handlers) ListDispatchPools(w http.ResponseWriter, r *http.Request) {
	h.dispatchPoolHandler.List(w, r)
}

func (h *Handlers) CreateDispatchPool(w http.ResponseWriter, r *http.Request) {
	h.dispatchPoolHandler.Create(w, r)
}

func (h *Handlers) GetDispatchPool(w http.ResponseWriter, r *http.Request) {
	h.dispatchPoolHandler.Get(w, r)
}

func (h *Handlers) UpdateDispatchPool(w http.ResponseWriter, r *http.Request) {
	h.dispatchPoolHandler.Update(w, r)
}

func (h *Handlers) DeleteDispatchPool(w http.ResponseWriter, r *http.Request) {
	h.dispatchPoolHandler.Delete(w, r)
}

func (h *Handlers) SuspendDispatchPool(w http.ResponseWriter, r *http.Request) {
	h.dispatchPoolHandler.Suspend(w, r)
}

func (h *Handlers) ArchiveDispatchPool(w http.ResponseWriter, r *http.Request) {
	h.dispatchPoolHandler.Archive(w, r)
}

func (h *Handlers) ActivateDispatchPool(w http.ResponseWriter, r *http.Request) {
	h.dispatchPoolHandler.Activate(w, r)
}

// Dispatch Job handlers

func (h *Handlers) CreateDispatchJob(w http.ResponseWriter, r *http.Request) {
	h.dispatchJobHandler.Create(w, r)
}

func (h *Handlers) CreateDispatchJobBatch(w http.ResponseWriter, r *http.Request) {
	h.dispatchJobHandler.CreateBatch(w, r)
}

func (h *Handlers) SearchDispatchJobs(w http.ResponseWriter, r *http.Request) {
	h.dispatchJobHandler.Search(w, r)
}

func (h *Handlers) GetDispatchJob(w http.ResponseWriter, r *http.Request) {
	h.dispatchJobHandler.Get(w, r)
}

func (h *Handlers) GetDispatchJobAttempts(w http.ResponseWriter, r *http.Request) {
	h.dispatchJobHandler.GetAttempts(w, r)
}

// BFF handlers

func (h *Handlers) BFFSearchEvents(w http.ResponseWriter, r *http.Request) {
	h.bffEventHandler.Search(w, r)
}

func (h *Handlers) BFFEventFilterOptions(w http.ResponseWriter, r *http.Request) {
	h.bffEventHandler.FilterOptions(w, r)
}

func (h *Handlers) BFFGetEvent(w http.ResponseWriter, r *http.Request) {
	h.bffEventHandler.Get(w, r)
}

func (h *Handlers) BFFSearchDispatchJobs(w http.ResponseWriter, r *http.Request) {
	h.bffDispatchHandler.Search(w, r)
}

func (h *Handlers) BFFDispatchJobFilterOptions(w http.ResponseWriter, r *http.Request) {
	h.bffDispatchHandler.FilterOptions(w, r)
}

func (h *Handlers) BFFGetDispatchJob(w http.ResponseWriter, r *http.Request) {
	h.bffDispatchHandler.Get(w, r)
}

// BFF EventType handlers

func (h *Handlers) BFFListEventTypes(w http.ResponseWriter, r *http.Request) {
	h.bffEventTypeHandler.List(w, r)
}

func (h *Handlers) BFFCreateEventType(w http.ResponseWriter, r *http.Request) {
	h.bffEventTypeHandler.Create(w, r)
}

func (h *Handlers) BFFGetEventType(w http.ResponseWriter, r *http.Request) {
	h.bffEventTypeHandler.Get(w, r)
}

func (h *Handlers) BFFUpdateEventType(w http.ResponseWriter, r *http.Request) {
	h.bffEventTypeHandler.Update(w, r)
}

func (h *Handlers) BFFArchiveEventType(w http.ResponseWriter, r *http.Request) {
	h.bffEventTypeHandler.Archive(w, r)
}

func (h *Handlers) BFFEventTypeApplications(w http.ResponseWriter, r *http.Request) {
	h.bffEventTypeHandler.GetApplications(w, r)
}

func (h *Handlers) BFFEventTypeSubdomains(w http.ResponseWriter, r *http.Request) {
	h.bffEventTypeHandler.GetSubdomains(w, r)
}

func (h *Handlers) BFFEventTypeAggregates(w http.ResponseWriter, r *http.Request) {
	h.bffEventTypeHandler.GetAggregates(w, r)
}

func (h *Handlers) BFFAddEventTypeSchema(w http.ResponseWriter, r *http.Request) {
	h.bffEventTypeHandler.AddSchema(w, r)
}

func (h *Handlers) BFFFinaliseEventTypeSchema(w http.ResponseWriter, r *http.Request) {
	h.bffEventTypeHandler.FinaliseSchema(w, r)
}

func (h *Handlers) BFFDeprecateEventTypeSchema(w http.ResponseWriter, r *http.Request) {
	h.bffEventTypeHandler.DeprecateSchema(w, r)
}

// BFF Role handlers

func (h *Handlers) BFFListRoles(w http.ResponseWriter, r *http.Request) {
	h.bffRoleHandler.List(w, r)
}

func (h *Handlers) BFFCreateRole(w http.ResponseWriter, r *http.Request) {
	h.bffRoleHandler.Create(w, r)
}

func (h *Handlers) BFFGetRole(w http.ResponseWriter, r *http.Request) {
	h.bffRoleHandler.Get(w, r)
}

func (h *Handlers) BFFUpdateRole(w http.ResponseWriter, r *http.Request) {
	h.bffRoleHandler.Update(w, r)
}

func (h *Handlers) BFFDeleteRole(w http.ResponseWriter, r *http.Request) {
	h.bffRoleHandler.Delete(w, r)
}

func (h *Handlers) BFFRoleApplications(w http.ResponseWriter, r *http.Request) {
	h.bffRoleHandler.GetApplications(w, r)
}

func (h *Handlers) BFFListPermissions(w http.ResponseWriter, r *http.Request) {
	h.bffRoleHandler.ListPermissions(w, r)
}

func (h *Handlers) BFFGetPermission(w http.ResponseWriter, r *http.Request) {
	h.bffRoleHandler.GetPermission(w, r)
}

// BFF Debug handlers (Raw Event/DispatchJob)

func (h *Handlers) BFFListRawEvents(w http.ResponseWriter, r *http.Request) {
	h.bffRawEventHandler.List(w, r)
}

func (h *Handlers) BFFGetRawEvent(w http.ResponseWriter, r *http.Request) {
	h.bffRawEventHandler.Get(w, r)
}

func (h *Handlers) BFFListRawDispatchJobs(w http.ResponseWriter, r *http.Request) {
	h.bffRawDispatchHandler.List(w, r)
}

func (h *Handlers) BFFGetRawDispatchJob(w http.ResponseWriter, r *http.Request) {
	h.bffRawDispatchHandler.Get(w, r)
}

// Client handlers

func (h *Handlers) ListClients(w http.ResponseWriter, r *http.Request) {
	h.clientHandler.List(w, r)
}

func (h *Handlers) CreateClient(w http.ResponseWriter, r *http.Request) {
	h.clientHandler.Create(w, r)
}

func (h *Handlers) GetClient(w http.ResponseWriter, r *http.Request) {
	h.clientHandler.Get(w, r)
}

func (h *Handlers) UpdateClient(w http.ResponseWriter, r *http.Request) {
	h.clientHandler.Update(w, r)
}

func (h *Handlers) SuspendClient(w http.ResponseWriter, r *http.Request) {
	h.clientHandler.Suspend(w, r)
}

func (h *Handlers) ActivateClient(w http.ResponseWriter, r *http.Request) {
	h.clientHandler.Activate(w, r)
}

func (h *Handlers) SearchClients(w http.ResponseWriter, r *http.Request) {
	h.clientHandler.Search(w, r)
}

func (h *Handlers) GetClientByIdentifier(w http.ResponseWriter, r *http.Request) {
	h.clientHandler.GetByIdentifier(w, r)
}

// Principal handlers

func (h *Handlers) ListPrincipals(w http.ResponseWriter, r *http.Request) {
	h.principalHandler.List(w, r)
}

func (h *Handlers) CreatePrincipal(w http.ResponseWriter, r *http.Request) {
	h.principalHandler.Create(w, r)
}

func (h *Handlers) GetPrincipal(w http.ResponseWriter, r *http.Request) {
	h.principalHandler.Get(w, r)
}

func (h *Handlers) UpdatePrincipal(w http.ResponseWriter, r *http.Request) {
	h.principalHandler.Update(w, r)
}

func (h *Handlers) ActivatePrincipal(w http.ResponseWriter, r *http.Request) {
	h.principalHandler.Activate(w, r)
}

func (h *Handlers) DeactivatePrincipal(w http.ResponseWriter, r *http.Request) {
	h.principalHandler.Deactivate(w, r)
}

func (h *Handlers) AssignPrincipalRoles(w http.ResponseWriter, r *http.Request) {
	h.principalHandler.AssignRoles(w, r)
}

func (h *Handlers) RemovePrincipalRole(w http.ResponseWriter, r *http.Request) {
	h.principalHandler.RemoveRole(w, r)
}

func (h *Handlers) GrantPrincipalClientAccess(w http.ResponseWriter, r *http.Request) {
	h.principalHandler.GrantClientAccess(w, r)
}

func (h *Handlers) RevokePrincipalClientAccess(w http.ResponseWriter, r *http.Request) {
	h.principalHandler.RevokeClientAccess(w, r)
}

func (h *Handlers) ResetPrincipalPassword(w http.ResponseWriter, r *http.Request) {
	h.principalHandler.ResetPassword(w, r)
}

// Role handlers (using UseCases)

func (h *Handlers) ListRoles(w http.ResponseWriter, r *http.Request) {
	h.roleHandler.List(w, r)
}

func (h *Handlers) CreateRole(w http.ResponseWriter, r *http.Request) {
	h.roleHandler.Create(w, r)
}

func (h *Handlers) GetRole(w http.ResponseWriter, r *http.Request) {
	h.roleHandler.Get(w, r)
}

func (h *Handlers) UpdateRole(w http.ResponseWriter, r *http.Request) {
	h.roleHandler.Update(w, r)
}

func (h *Handlers) DeleteRole(w http.ResponseWriter, r *http.Request) {
	h.roleHandler.Delete(w, r)
}

// Permission handlers

func (h *Handlers) ListPermissions(w http.ResponseWriter, r *http.Request) {
	permissions, err := h.permissionRepo.FindAll(r.Context())
	if err != nil {
		WriteInternalError(w, "Failed to list permissions")
		return
	}
	WriteJSON(w, http.StatusOK, permissions)
}

func (h *Handlers) GetPermission(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	perm, err := h.permissionRepo.FindByCode(r.Context(), code)
	if err != nil {
		WriteInternalError(w, "Failed to get permission")
		return
	}
	if perm == nil {
		WriteNotFound(w, "Permission not found")
		return
	}
	WriteJSON(w, http.StatusOK, perm)
}

// Application handlers

func (h *Handlers) ListApplications(w http.ResponseWriter, r *http.Request) {
	h.applicationAdminHandler.List(w, r)
}

func (h *Handlers) CreateApplication(w http.ResponseWriter, r *http.Request) {
	h.applicationAdminHandler.Create(w, r)
}

func (h *Handlers) GetApplication(w http.ResponseWriter, r *http.Request) {
	h.applicationAdminHandler.Get(w, r)
}

func (h *Handlers) GetApplicationByCode(w http.ResponseWriter, r *http.Request) {
	h.applicationAdminHandler.GetByCode(w, r)
}

func (h *Handlers) UpdateApplication(w http.ResponseWriter, r *http.Request) {
	h.applicationAdminHandler.Update(w, r)
}

func (h *Handlers) ActivateApplication(w http.ResponseWriter, r *http.Request) {
	h.applicationAdminHandler.Activate(w, r)
}

func (h *Handlers) DeactivateApplication(w http.ResponseWriter, r *http.Request) {
	h.applicationAdminHandler.Deactivate(w, r)
}

func (h *Handlers) DeleteApplication(w http.ResponseWriter, r *http.Request) {
	h.applicationAdminHandler.Delete(w, r)
}

// Service Account handlers (using UseCases)

func (h *Handlers) ListServiceAccounts(w http.ResponseWriter, r *http.Request) {
	h.serviceAccountHandler.List(w, r)
}

func (h *Handlers) CreateServiceAccount(w http.ResponseWriter, r *http.Request) {
	h.serviceAccountHandler.Create(w, r)
}

func (h *Handlers) GetServiceAccount(w http.ResponseWriter, r *http.Request) {
	h.serviceAccountHandler.Get(w, r)
}

func (h *Handlers) UpdateServiceAccount(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	existing, err := h.serviceAccountRepo.FindByID(r.Context(), id)
	if err != nil {
		WriteInternalError(w, "Failed to get service account")
		return
	}
	if existing == nil {
		WriteNotFound(w, "Service account not found")
		return
	}

	var account serviceaccount.ServiceAccount
	if err := DecodeJSON(r, &account); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}
	account.ID = id
	account.CreatedAt = existing.CreatedAt
	account.WebhookCredentials = existing.WebhookCredentials

	if err := h.serviceAccountRepo.Update(r.Context(), &account); err != nil {
		WriteInternalError(w, "Failed to update service account")
		return
	}
	WriteJSON(w, http.StatusOK, account)
}

func (h *Handlers) DeleteServiceAccount(w http.ResponseWriter, r *http.Request) {
	h.serviceAccountHandler.Delete(w, r)
}

func (h *Handlers) RegenerateServiceAccountCredentials(w http.ResponseWriter, r *http.Request) {
	h.serviceAccountHandler.RotateCredentials(w, r)
}

// OAuth Client handlers

func (h *Handlers) ListOAuthClients(w http.ResponseWriter, r *http.Request) {
	h.oauthClientHandler.List(w, r)
}

func (h *Handlers) CreateOAuthClient(w http.ResponseWriter, r *http.Request) {
	h.oauthClientHandler.Create(w, r)
}

func (h *Handlers) GetOAuthClient(w http.ResponseWriter, r *http.Request) {
	h.oauthClientHandler.Get(w, r)
}

func (h *Handlers) GetOAuthClientByClientID(w http.ResponseWriter, r *http.Request) {
	h.oauthClientHandler.GetByClientID(w, r)
}

func (h *Handlers) UpdateOAuthClient(w http.ResponseWriter, r *http.Request) {
	h.oauthClientHandler.Update(w, r)
}

func (h *Handlers) RotateOAuthClientSecret(w http.ResponseWriter, r *http.Request) {
	h.oauthClientHandler.RotateSecret(w, r)
}

func (h *Handlers) ActivateOAuthClient(w http.ResponseWriter, r *http.Request) {
	h.oauthClientHandler.Activate(w, r)
}

func (h *Handlers) DeactivateOAuthClient(w http.ResponseWriter, r *http.Request) {
	h.oauthClientHandler.Deactivate(w, r)
}

func (h *Handlers) DeleteOAuthClient(w http.ResponseWriter, r *http.Request) {
	h.oauthClientHandler.Delete(w, r)
}

// Auth handlers (placeholder - will be expanded)

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	// Placeholder - will be implemented with auth package
	WriteBadRequest(w, "Auth not yet implemented")
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     h.config.Auth.Session.CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.config.Auth.Session.Secure,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) GetCurrentUser(w http.ResponseWriter, r *http.Request) {
	p := GetPrincipal(r.Context())
	if p == nil {
		WriteUnauthorized(w, "Not authenticated")
		return
	}
	WriteJSON(w, http.StatusOK, p)
}

func (h *Handlers) CheckDomain(w http.ResponseWriter, r *http.Request) {
	// Placeholder
	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"authMethod": "LOCAL",
	})
}

// OAuth/OIDC handlers (placeholder - will be expanded with Fosite)

func (h *Handlers) OAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	WriteBadRequest(w, "OAuth not yet implemented")
}

func (h *Handlers) OAuthToken(w http.ResponseWriter, r *http.Request) {
	WriteBadRequest(w, "OAuth not yet implemented")
}

func (h *Handlers) OIDCDiscovery(w http.ResponseWriter, r *http.Request) {
	baseURL := h.config.Auth.ExternalBase
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	discovery := map[string]interface{}{
		"issuer":                 h.config.Auth.JWT.Issuer,
		"authorization_endpoint": baseURL + "/oauth/authorize",
		"token_endpoint":         baseURL + "/oauth/token",
		"jwks_uri":               baseURL + "/.well-known/jwks.json",
		"response_types_supported": []string{
			"code",
			"token",
			"id_token",
			"code token",
			"code id_token",
			"token id_token",
			"code token id_token",
		},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"scopes_supported":                      []string{"openid", "profile", "email"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_basic", "client_secret_post"},
		"claims_supported": []string{
			"sub", "iss", "aud", "exp", "iat", "name", "email",
		},
		"code_challenge_methods_supported": []string{"S256"},
	}

	WriteJSON(w, http.StatusOK, discovery)
}

func (h *Handlers) JWKS(w http.ResponseWriter, r *http.Request) {
	// Placeholder - will return actual public keys
	jwks := map[string]interface{}{
		"keys": []interface{}{},
	}
	WriteJSON(w, http.StatusOK, jwks)
}

// Audit log handlers

func (h *Handlers) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	h.auditLogHandler.List(w, r)
}

func (h *Handlers) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	h.auditLogHandler.Get(w, r)
}

func (h *Handlers) GetAuditLogsForEntity(w http.ResponseWriter, r *http.Request) {
	h.auditLogHandler.GetForEntity(w, r)
}

func (h *Handlers) GetAuditEntityTypes(w http.ResponseWriter, r *http.Request) {
	h.auditLogHandler.GetEntityTypes(w, r)
}

func (h *Handlers) GetAuditOperations(w http.ResponseWriter, r *http.Request) {
	h.auditLogHandler.GetOperations(w, r)
}

// GetAuditService returns the audit service for use in other handlers
func (h *Handlers) GetAuditService() *audit.Service {
	return h.auditService
}

// Auth config handlers

func (h *Handlers) ListAuthConfigs(w http.ResponseWriter, r *http.Request) {
	h.authConfigHandler.List(w, r)
}

func (h *Handlers) CreateAuthConfig(w http.ResponseWriter, r *http.Request) {
	h.authConfigHandler.Create(w, r)
}

func (h *Handlers) GetAuthConfig(w http.ResponseWriter, r *http.Request) {
	h.authConfigHandler.Get(w, r)
}

func (h *Handlers) UpdateAuthConfig(w http.ResponseWriter, r *http.Request) {
	h.authConfigHandler.Update(w, r)
}

func (h *Handlers) DeleteAuthConfig(w http.ResponseWriter, r *http.Request) {
	h.authConfigHandler.Delete(w, r)
}

// Anchor domain handlers

func (h *Handlers) ListAnchorDomains(w http.ResponseWriter, r *http.Request) {
	h.anchorDomainHandler.List(w, r)
}

func (h *Handlers) CreateAnchorDomain(w http.ResponseWriter, r *http.Request) {
	h.anchorDomainHandler.Create(w, r)
}

func (h *Handlers) DeleteAnchorDomain(w http.ResponseWriter, r *http.Request) {
	h.anchorDomainHandler.Delete(w, r)
}
