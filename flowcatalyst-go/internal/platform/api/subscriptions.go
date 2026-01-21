package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/dispatchpool"
	dpops "go.flowcatalyst.tech/internal/platform/dispatchpool/operations"
	"go.flowcatalyst.tech/internal/platform/subscription"
	"go.flowcatalyst.tech/internal/platform/subscription/operations"
)

// SubscriptionHandler handles subscription endpoints using UseCases
// @Description Subscription management API for webhook event delivery
type SubscriptionHandler struct {
	repo subscription.Repository

	// UseCases
	createUseCase *operations.CreateSubscriptionUseCase
	updateUseCase *operations.UpdateSubscriptionUseCase
	pauseUseCase  *operations.PauseSubscriptionUseCase
	resumeUseCase *operations.ResumeSubscriptionUseCase
	deleteUseCase *operations.DeleteSubscriptionUseCase
}

// NewSubscriptionHandler creates a new subscription handler with UseCases
func NewSubscriptionHandler(
	repo subscription.Repository,
	uow common.UnitOfWork,
) *SubscriptionHandler {
	return &SubscriptionHandler{
		repo:          repo,
		createUseCase: operations.NewCreateSubscriptionUseCase(repo, uow),
		updateUseCase: operations.NewUpdateSubscriptionUseCase(repo, uow),
		pauseUseCase:  operations.NewPauseSubscriptionUseCase(repo, uow),
		resumeUseCase: operations.NewResumeSubscriptionUseCase(repo, uow),
		deleteUseCase: operations.NewDeleteSubscriptionUseCase(repo, uow),
	}
}

// Routes returns the router for subscription endpoints
func (h *SubscriptionHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/{id}", h.Get)
	r.Put("/{id}", h.Update)
	r.Delete("/{id}", h.Delete)
	r.Post("/{id}/pause", h.Pause)
	r.Post("/{id}/resume", h.Resume)

	return r
}

// CreateSubscriptionRequest represents a request to create a subscription
type CreateSubscriptionRequest struct {
	Code             string                       `json:"code"`
	Name             string                       `json:"name"`
	Description      string                       `json:"description,omitempty"`
	EventTypes       []subscription.EventTypeBinding `json:"eventTypes"`
	Target           string                       `json:"target"`
	Queue            string                       `json:"queue,omitempty"`
	CustomConfig     []subscription.ConfigEntry   `json:"customConfig,omitempty"`
	DispatchPoolCode string                       `json:"dispatchPoolCode,omitempty"`
	DelaySeconds     int                          `json:"delaySeconds,omitempty"`
	Mode             subscription.DispatchMode    `json:"mode,omitempty"`
	TimeoutSeconds   int                          `json:"timeoutSeconds,omitempty"`
	DataOnly         bool                         `json:"dataOnly"`
}

// UpdateSubscriptionRequest represents a request to update a subscription
type UpdateSubscriptionRequest struct {
	Name           string                     `json:"name,omitempty"`
	Description    string                     `json:"description,omitempty"`
	Target         string                     `json:"target,omitempty"`
	CustomConfig   []subscription.ConfigEntry `json:"customConfig,omitempty"`
	DelaySeconds   int                        `json:"delaySeconds,omitempty"`
	TimeoutSeconds int                        `json:"timeoutSeconds,omitempty"`
}

// List handles GET /api/subscriptions
// @Summary List all subscriptions
// @Description Returns a list of subscriptions the user has access to
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Param status query string false "Filter by status (ACTIVE, PAUSED, ARCHIVED)"
// @Success 200 {array} subscription.Subscription
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/subscriptions [get]
func (h *SubscriptionHandler) List(w http.ResponseWriter, r *http.Request) {
	// Get client ID from authenticated principal for filtering
	p := GetPrincipal(r.Context())

	var subs []*subscription.Subscription
	var err error

	if p != nil && !p.IsAnchor() {
		// Non-anchor users can only see their client's subscriptions
		subs, err = h.repo.FindSubscriptionsByClient(r.Context(), p.ClientID)
	} else {
		// Anchor users can see all subscriptions
		subs, err = h.repo.FindAllSubscriptions(r.Context(), 0, 1000)
	}

	if err != nil {
		slog.Error("Failed to list subscriptions", "error", err)
		WriteInternalError(w, "Failed to list subscriptions")
		return
	}

	WriteJSON(w, http.StatusOK, subs)
}

// Get handles GET /api/subscriptions/{id}
// @Summary Get subscription by ID
// @Description Returns a single subscription by its ID
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Param id path string true "Subscription ID"
// @Success 200 {object} subscription.Subscription
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/subscriptions/{id} [get]
func (h *SubscriptionHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	sub, err := h.repo.FindSubscriptionByID(r.Context(), id)
	if err != nil {
		if err == subscription.ErrNotFound {
			WriteNotFound(w, "Subscription not found")
			return
		}
		slog.Error("Failed to get subscription", "error", err, "id", id)
		WriteInternalError(w, "Failed to get subscription")
		return
	}

	// Check access
	p := GetPrincipal(r.Context())
	if p != nil && !p.IsAnchor() && sub.ClientID != p.ClientID {
		WriteNotFound(w, "Subscription not found")
		return
	}

	WriteJSON(w, http.StatusOK, sub)
}

// Create handles POST /api/subscriptions (using UseCase)
// @Summary Create a new subscription
// @Description Creates a new subscription for event delivery
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Param request body CreateSubscriptionRequest true "Subscription details"
// @Success 201 {object} subscription.Subscription
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Subscription with code already exists"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/subscriptions [post]
func (h *SubscriptionHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateSubscriptionRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	// Get client ID from authenticated principal
	p := GetPrincipal(r.Context())
	clientID := ""
	if p != nil {
		clientID = p.ClientID
	}

	// Build event type bindings for the command
	eventTypeBindings := make([]operations.EventTypeBindingInput, len(req.EventTypes))
	for i, et := range req.EventTypes {
		eventTypeBindings[i] = operations.EventTypeBindingInput{
			EventTypeID:   et.EventTypeID,
			EventTypeCode: et.EventTypeCode,
			SpecVersion:   et.SpecVersion,
		}
	}

	cmd := operations.CreateSubscriptionCommand{
		Code:             req.Code,
		Name:             req.Name,
		Description:      req.Description,
		ClientID:         clientID,
		Target:           req.Target,
		EventTypes:       eventTypeBindings,
		DispatchPoolCode: req.DispatchPoolCode,
		Mode:             string(req.Mode),
		TimeoutSeconds:   req.TimeoutSeconds,
		DataOnly:         req.DataOnly,
	}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.createUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusCreated)
}

// Update handles PUT /api/subscriptions/{id} (using UseCase)
// @Summary Update a subscription
// @Description Updates an existing subscription
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Param id path string true "Subscription ID"
// @Param request body UpdateSubscriptionRequest true "Updated subscription details"
// @Success 200 {object} subscription.Subscription
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/subscriptions/{id} [put]
func (h *SubscriptionHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req UpdateSubscriptionRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	cmd := operations.UpdateSubscriptionCommand{
		ID:             id,
		Name:           req.Name,
		Description:    req.Description,
		Target:         req.Target,
		TimeoutSeconds: req.TimeoutSeconds,
	}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.updateUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusOK)
}

// Delete handles DELETE /api/subscriptions/{id} (using UseCase)
// @Summary Delete a subscription
// @Description Permanently deletes a subscription
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Param id path string true "Subscription ID"
// @Success 204 "No Content"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/subscriptions/{id} [delete]
func (h *SubscriptionHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cmd := operations.DeleteSubscriptionCommand{ID: id}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.deleteUseCase.Execute(r.Context(), cmd, execCtx)

	if result.IsFailure() {
		WriteUseCaseError(w, result.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Pause handles POST /api/subscriptions/{id}/pause (using UseCase)
// @Summary Pause a subscription
// @Description Pauses event delivery for a subscription
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Param id path string true "Subscription ID"
// @Success 200 {object} subscription.Subscription
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Already paused"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/subscriptions/{id}/pause [post]
func (h *SubscriptionHandler) Pause(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cmd := operations.PauseSubscriptionCommand{ID: id}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.pauseUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusOK)
}

// Resume handles POST /api/subscriptions/{id}/resume (using UseCase)
// @Summary Resume a subscription
// @Description Resumes event delivery for a paused subscription
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Param id path string true "Subscription ID"
// @Success 200 {object} subscription.Subscription
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Not paused"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/subscriptions/{id}/resume [post]
func (h *SubscriptionHandler) Resume(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cmd := operations.ResumeSubscriptionCommand{ID: id}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.resumeUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusOK)
}

// === Dispatch Pool Handler ===

// DispatchPoolHandler handles dispatch pool endpoints using UseCases
// @Description Dispatch pool management API for rate limiting and concurrency control
type DispatchPoolHandler struct {
	repo dispatchpool.Repository

	// UseCases
	createUseCase  *dpops.CreateDispatchPoolUseCase
	updateUseCase  *dpops.UpdateDispatchPoolUseCase
	suspendUseCase *dpops.SuspendDispatchPoolUseCase
	archiveUseCase *dpops.ArchiveDispatchPoolUseCase
}

// NewDispatchPoolHandler creates a new dispatch pool handler with UseCases
func NewDispatchPoolHandler(
	repo dispatchpool.Repository,
	uow common.UnitOfWork,
) *DispatchPoolHandler {
	return &DispatchPoolHandler{
		repo:           repo,
		createUseCase:  dpops.NewCreateDispatchPoolUseCase(repo, uow),
		updateUseCase:  dpops.NewUpdateDispatchPoolUseCase(repo, uow),
		suspendUseCase: dpops.NewSuspendDispatchPoolUseCase(repo, uow),
		archiveUseCase: dpops.NewArchiveDispatchPoolUseCase(repo, uow),
	}
}

// Routes returns the router for dispatch pool endpoints
func (h *DispatchPoolHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/{id}", h.Get)
	r.Put("/{id}", h.Update)
	r.Delete("/{id}", h.Delete)
	r.Post("/{id}/suspend", h.Suspend)
	r.Post("/{id}/archive", h.Archive)
	r.Post("/{id}/activate", h.Activate)

	return r
}

// CreateDispatchPoolRequest represents a request to create a dispatch pool
type CreateDispatchPoolRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	RateLimit   *int   `json:"rateLimit,omitempty"`
	Concurrency int    `json:"concurrency"`
}

// UpdateDispatchPoolRequest represents a request to update a dispatch pool
type UpdateDispatchPoolRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	RateLimit   *int   `json:"rateLimit,omitempty"`
	Concurrency int    `json:"concurrency,omitempty"`
}

// List handles GET /api/dispatch-pools
// @Summary List all dispatch pools
// @Description Returns a list of dispatch pools the user has access to
// @Tags Dispatch Pools
// @Accept json
// @Produce json
// @Param status query string false "Filter by status (ACTIVE, SUSPENDED, ARCHIVED)"
// @Success 200 {array} dispatchpool.DispatchPool
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/dispatch-pools [get]
func (h *DispatchPoolHandler) List(w http.ResponseWriter, r *http.Request) {
	p := GetPrincipal(r.Context())

	var pools []*dispatchpool.DispatchPool
	var err error

	if p != nil && !p.IsAnchor() {
		pools, err = h.repo.FindByClientID(r.Context(), p.ClientID)
	} else {
		pools, err = h.repo.FindAll(r.Context())
	}

	if err != nil {
		slog.Error("Failed to list dispatch pools", "error", err)
		WriteInternalError(w, "Failed to list dispatch pools")
		return
	}

	WriteJSON(w, http.StatusOK, pools)
}

// Get handles GET /api/dispatch-pools/{id}
// @Summary Get dispatch pool by ID
// @Description Returns a single dispatch pool by its ID
// @Tags Dispatch Pools
// @Accept json
// @Produce json
// @Param id path string true "Dispatch Pool ID"
// @Success 200 {object} dispatchpool.DispatchPool
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/dispatch-pools/{id} [get]
func (h *DispatchPoolHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	pool, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		if err == dispatchpool.ErrNotFound {
			WriteNotFound(w, "Dispatch pool not found")
			return
		}
		slog.Error("Failed to get dispatch pool", "error", err, "id", id)
		WriteInternalError(w, "Failed to get dispatch pool")
		return
	}

	p := GetPrincipal(r.Context())
	if p != nil && !p.IsAnchor() && pool.ClientID != p.ClientID {
		WriteNotFound(w, "Dispatch pool not found")
		return
	}

	WriteJSON(w, http.StatusOK, pool)
}

// Create handles POST /api/dispatch-pools (using UseCase)
// @Summary Create a new dispatch pool
// @Description Creates a new dispatch pool for rate limiting and concurrency control
// @Tags Dispatch Pools
// @Accept json
// @Produce json
// @Param request body CreateDispatchPoolRequest true "Dispatch pool details"
// @Success 201 {object} dispatchpool.DispatchPool
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Dispatch pool with code already exists"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/dispatch-pools [post]
func (h *DispatchPoolHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateDispatchPoolRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	// Get client ID from authenticated principal
	p := GetPrincipal(r.Context())
	clientID := ""
	if p != nil {
		clientID = p.ClientID
	}

	// Set defaults
	concurrency := req.Concurrency
	if concurrency <= 0 {
		concurrency = 10
	}

	cmd := dpops.CreateDispatchPoolCommand{
		Code:            req.Code,
		Name:            req.Name,
		Description:     req.Description,
		ClientID:        clientID,
		MediatorType:    string(dispatchpool.MediatorTypeHTTPWebhook),
		Concurrency:     concurrency,
		QueueCapacity:   500,
		RateLimitPerMin: req.RateLimit,
	}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.createUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusCreated)
}

// Update handles PUT /api/dispatch-pools/{id} (using UseCase)
// @Summary Update a dispatch pool
// @Description Updates an existing dispatch pool
// @Tags Dispatch Pools
// @Accept json
// @Produce json
// @Param id path string true "Dispatch Pool ID"
// @Param request body UpdateDispatchPoolRequest true "Updated dispatch pool details"
// @Success 200 {object} dispatchpool.DispatchPool
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/dispatch-pools/{id} [put]
func (h *DispatchPoolHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req UpdateDispatchPoolRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	cmd := dpops.UpdateDispatchPoolCommand{
		ID:              id,
		Name:            req.Name,
		Description:     req.Description,
		Concurrency:     req.Concurrency,
		RateLimitPerMin: req.RateLimit,
	}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.updateUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusOK)
}

// Delete handles DELETE /api/dispatch-pools/{id}
// @Summary Delete a dispatch pool
// @Description Permanently deletes a dispatch pool
// @Tags Dispatch Pools
// @Accept json
// @Produce json
// @Param id path string true "Dispatch Pool ID"
// @Success 204 "No Content"
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Pool is in use"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/dispatch-pools/{id} [delete]
func (h *DispatchPoolHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	pool, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		if err == dispatchpool.ErrNotFound {
			WriteNotFound(w, "Dispatch pool not found")
			return
		}
		slog.Error("Failed to get dispatch pool", "error", err, "id", id)
		WriteInternalError(w, "Failed to get dispatch pool")
		return
	}

	p := GetPrincipal(r.Context())
	if p != nil && !p.IsAnchor() && pool.ClientID != p.ClientID {
		WriteNotFound(w, "Dispatch pool not found")
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		slog.Error("Failed to delete dispatch pool", "error", err, "id", id)
		WriteInternalError(w, "Failed to delete dispatch pool")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Suspend handles POST /api/dispatch-pools/{id}/suspend (using UseCase)
// @Summary Suspend a dispatch pool
// @Description Suspends a dispatch pool, stopping all event processing
// @Tags Dispatch Pools
// @Accept json
// @Produce json
// @Param id path string true "Dispatch Pool ID"
// @Success 200 {object} dispatchpool.DispatchPool
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Already suspended"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/dispatch-pools/{id}/suspend [post]
func (h *DispatchPoolHandler) Suspend(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cmd := dpops.SuspendDispatchPoolCommand{ID: id}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.suspendUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusOK)
}

// Archive handles POST /api/dispatch-pools/{id}/archive (using UseCase)
// @Summary Archive a dispatch pool
// @Description Archives a dispatch pool (soft delete)
// @Tags Dispatch Pools
// @Accept json
// @Produce json
// @Param id path string true "Dispatch Pool ID"
// @Success 200 {object} dispatchpool.DispatchPool
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Pool is in use"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/dispatch-pools/{id}/archive [post]
func (h *DispatchPoolHandler) Archive(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cmd := dpops.ArchiveDispatchPoolCommand{ID: id}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.archiveUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusOK)
}

// Activate handles POST /api/dispatch-pools/{id}/activate
// @Summary Activate a dispatch pool
// @Description Activates a suspended or archived dispatch pool
// @Tags Dispatch Pools
// @Accept json
// @Produce json
// @Param id path string true "Dispatch Pool ID"
// @Success 200 {object} dispatchpool.DispatchPool
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Already active"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/dispatch-pools/{id}/activate [post]
func (h *DispatchPoolHandler) Activate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Get pool first to check current status
	pool, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		if err == dispatchpool.ErrNotFound {
			WriteNotFound(w, "Dispatch pool not found")
			return
		}
		slog.Error("Failed to get dispatch pool", "error", err, "id", id)
		WriteInternalError(w, "Failed to get dispatch pool")
		return
	}

	// Check access
	p := GetPrincipal(r.Context())
	if p != nil && !p.IsAnchor() && pool.ClientID != p.ClientID {
		WriteNotFound(w, "Dispatch pool not found")
		return
	}

	if pool.Status == dispatchpool.DispatchPoolStatusArchived {
		WriteBadRequest(w, "Cannot activate archived pool")
		return
	}

	if err := h.repo.SetStatus(r.Context(), id, dispatchpool.DispatchPoolStatusActive); err != nil {
		slog.Error("Failed to activate dispatch pool", "error", err, "id", id)
		WriteInternalError(w, "Failed to activate dispatch pool")
		return
	}

	pool.Status = dispatchpool.DispatchPoolStatusActive
	pool.Enabled = true
	WriteJSON(w, http.StatusOK, pool)
}
