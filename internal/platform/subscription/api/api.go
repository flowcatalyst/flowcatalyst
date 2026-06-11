// Package api wires HTTP routes for the subscription subdomain via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles the dependencies.
type State struct {
	Repo *subscription.Repository
	UoW  *usecasepgx.UnitOfWork
}

const tag = "subscriptions"

// Register mounts the subscription endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listSubscriptions", "/api/subscriptions", "List subscriptions", s.list)
	apiroute.Post(g, "createSubscription", "/api/subscriptions", "Create a subscription", http.StatusCreated, s.create)
	apiroute.Get(g, "getSubscription", "/api/subscriptions/{id}", "Get a subscription by id", s.getByID)
	apiroute.Put(g, "updateSubscription", "/api/subscriptions/{id}", "Update a subscription", http.StatusNoContent, s.update)
	apiroute.Delete(g, "deleteSubscription", "/api/subscriptions/{id}", "Delete a subscription", http.StatusNoContent, s.delete)
	apiroute.Post(g, "pauseSubscription", "/api/subscriptions/{id}/pause", "Pause a subscription", http.StatusNoContent, s.pause)
	apiroute.Post(g, "resumeSubscription", "/api/subscriptions/{id}/resume", "Resume a subscription", http.StatusNoContent, s.resume)
}

type listInput struct {
	Status   string `query:"status"`
	ClientID string `query:"clientId"`
}

func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[SubscriptionListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadSubscriptions(ac); err != nil {
		return nil, err
	}
	status := apicommon.OptStr(in.Status)
	clientID := apicommon.OptStr(in.ClientID)
	rows, err := s.Repo.FindWithFilters(ctx, status, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	visible := auth.FilterClientScoped(ac, rows, func(sub *subscription.Subscription) *string { return sub.ClientID })
	out := apicommon.MapSlice(visible, fromEntity)
	return &apicommon.Out[SubscriptionListResponse]{Body: SubscriptionListResponse{Subscriptions: out, Total: len(out)}}, nil
}

func (s *State) getByID(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[SubscriptionResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadSubscriptions(ac); err != nil {
		return nil, err
	}
	sub, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if sub == nil {
		return nil, httperror.NotFound("Subscription", in.ID)
	}
	if sub.ClientID != nil && !ac.CanAccessClient(*sub.ClientID) {
		return nil, httperror.Forbidden("No access to this subscription")
	}
	return &apicommon.Out[SubscriptionResponse]{Body: fromEntity(sub)}, nil
}

func (s *State) create(ctx context.Context, in *apicommon.In[CreateSubscriptionRequest]) (*apicommon.Out[apicommon.CreatedResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteSubscriptions(ac); err != nil {
		return nil, err
	}
	if in.Body.ClientID != nil && !ac.CanAccessClient(*in.Body.ClientID) {
		return nil, httperror.Forbidden("No access to client: " + *in.Body.ClientID)
	}
	if in.Body.ClientID == nil && !ac.IsAnchor() {
		return nil, httperror.Forbidden("Only anchor users can create anchor-level subscriptions")
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.CreateSubscription(ctx, s.Repo, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.CreatedResponse]{Body: apicommon.CreatedResponse{ID: committed.Event().SubscriptionID}}, nil
}

// requireScopeByID loads the subscription and enforces per-resource scope
// (A2) on top of the coarse permission the caller already checked: a non-anchor
// principal must not mutate another tenant's subscription by guessing its id.
func (s *State) requireScopeByID(ctx context.Context, ac *auth.AuthContext, id string) error {
	sub, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return usecase.Internal("REPO", "find_by_id failed", err)
	}
	if sub == nil {
		return httperror.NotFound("Subscription", id)
	}
	return auth.CheckScopeAccess(ac, sub.ClientID)
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateSubscriptionRequest
}

func (s *State) update(ctx context.Context, in *updateInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteSubscriptions(ac); err != nil {
		return nil, err
	}
	if err := s.requireScopeByID(ctx, ac, in.ID); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.UpdateSubscription(ctx, s.Repo, s.UoW, in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanDeleteSubscriptions(ac); err != nil {
		return nil, err
	}
	if err := s.requireScopeByID(ctx, ac, in.ID); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteSubscription(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) pause(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteSubscriptions(ac); err != nil {
		return nil, err
	}
	if err := s.requireScopeByID(ctx, ac, in.ID); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.PauseSubscription(ctx, s.Repo, s.UoW, operations.PauseCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) resume(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteSubscriptions(ac); err != nil {
		return nil, err
	}
	if err := s.requireScopeByID(ctx, ac, in.ID); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.ResumeSubscription(ctx, s.Repo, s.UoW, operations.ResumeCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}
