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
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
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
	// Coarse write permission at the controller; the use case enforces per-client
	// resource access on the requested clientId (you may only bind a subscription
	// to a client you can access; platform-wide requires anchor).
	if err := auth.CanWriteSubscriptions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateSubscription(s.Repo), in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.CreatedResponse]{Body: apicommon.CreatedResponse{ID: event.SubscriptionID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateSubscriptionRequest
}

func (s *State) update(ctx context.Context, in *updateInput) (*apicommon.Empty, error) {
	if err := auth.CanWriteSubscriptions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.UpdateSubscription(s.Repo), in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if err := auth.CanDeleteSubscriptions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeleteSubscription(s.Repo), operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) pause(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if err := auth.CanWriteSubscriptions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.PauseSubscription(s.Repo), operations.PauseCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) resume(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if err := auth.CanWriteSubscriptions(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.ResumeSubscription(s.Repo), operations.ResumeCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}
