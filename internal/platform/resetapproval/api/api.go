// Package api serves the lost-device password-reset approval queue
// (Phase 8 / docs/auth-hardening-plan.md): a client-administrator lists pending
// requests for their client(s) and approves/denies them. Approval emails the
// user an authorised reset link that also clears their 2FA so they re-onboard.
package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/resetapproval"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// ResetLinkSender mints + emails an authorised reset link. The passwordreset
// principalEmailer satisfies it (SendResetEmail).
type ResetLinkSender interface {
	SendResetEmail(ctx context.Context, p *principal.Principal, reset2FA bool) error
}

// State holds the deps the approval handlers need.
type State struct {
	Approvals  *resetapproval.Repository
	Principals *principal.Repository
	Sender     ResetLinkSender // optional; nil = no link emailed on approval
}

const tag = "reset-approvals"

// Register mounts the queue endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listResetApprovals", "/api/reset-approvals", "List pending lost-device reset requests for your client(s)", s.list)
	apiroute.Post(g, "approveResetApproval", "/api/reset-approvals/{id}/approve", "Approve a lost-device reset (emails the user a reset link)", http.StatusOK, s.approve)
	apiroute.Post(g, "denyResetApproval", "/api/reset-approvals/{id}/deny", "Deny a lost-device reset request", http.StatusOK, s.deny)
}

type RequestDTO struct {
	ID          string        `json:"id"`
	PrincipalID string        `json:"principalId"`
	Email       string        `json:"email"`
	Name        string        `json:"name"`
	ClientID    *string       `json:"clientId,omitempty"`
	ExpiresAt   jsontime.Time `json:"expiresAt"`
	CreatedAt   jsontime.Time `json:"createdAt"`
}

type listOutput struct {
	Body struct {
		Requests []RequestDTO `json:"requests"`
	}
}

func (s *State) list(ctx context.Context, _ *struct{}) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	// Client-admins (and anchors) only — they hold a user-write permission.
	if err := auth.CanWritePrincipals(ac); err != nil {
		return nil, err
	}
	// Scope to the caller's clients; anchors see all (nil).
	var clientIDs []string
	if ac != nil && !ac.IsAnchor() {
		clientIDs = ac.Clients
		if clientIDs == nil {
			clientIDs = []string{} // non-anchor with no clients → match nothing
		}
	}
	reqs, err := s.Approvals.ListPending(ctx, clientIDs)
	if err != nil {
		return nil, usecase.Internal("REPO", "list reset-approvals failed", err)
	}
	out := &listOutput{}
	out.Body.Requests = make([]RequestDTO, 0, len(reqs))
	for _, r := range reqs {
		dto := RequestDTO{
			ID:          r.ID,
			PrincipalID: r.PrincipalID,
			ClientID:    r.ClientID,
			ExpiresAt:   jsontime.New(r.ExpiresAt),
			CreatedAt:   jsontime.New(r.CreatedAt),
		}
		if p, _ := s.Principals.FindByID(ctx, r.PrincipalID); p != nil {
			dto.Name = p.Name
			if p.UserIdentity != nil {
				dto.Email = p.UserIdentity.Email
			}
		}
		out.Body.Requests = append(out.Body.Requests, dto)
	}
	return out, nil
}

type idInput struct {
	ID string `path:"id"`
}

type messageOutput struct {
	Body apicommon.StatusChangeResponse
}

func (s *State) approve(ctx context.Context, in *idInput) (*messageOutput, error) {
	ac := auth.FromContext(ctx)
	req, err := s.Approvals.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find reset-approval failed", err)
	}
	if req == nil {
		return nil, httperror.NotFound("ResetApprovalRequest", in.ID)
	}
	if err := auth.RequireUserAdmin(ac, req.ClientID); err != nil {
		return nil, err
	}
	ok, err := s.Approvals.Decide(ctx, req.ID, resetapproval.StatusApproved, ac.PrincipalID)
	if err != nil {
		return nil, usecase.Internal("REPO", "decide reset-approval failed", err)
	}
	if !ok {
		return nil, usecase.Validation("ALREADY_DECIDED", "request is no longer pending")
	}
	// Email the user an authorised reset link (clears 2FA so they re-onboard).
	p, err := s.Principals.FindByID(ctx, req.PrincipalID)
	if err != nil || p == nil {
		return nil, httperror.NotFound("Principal", req.PrincipalID)
	}
	if s.Sender != nil {
		if serr := s.Sender.SendResetEmail(ctx, p, req.Reset2FA); serr != nil {
			slog.Warn("failed to send approved reset link", "principal", p.ID, "err", serr)
		}
	}
	return &messageOutput{Body: apicommon.StatusChangeResponse{Message: "Reset approved — the user has been emailed a link"}}, nil
}

func (s *State) deny(ctx context.Context, in *idInput) (*messageOutput, error) {
	ac := auth.FromContext(ctx)
	req, err := s.Approvals.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find reset-approval failed", err)
	}
	if req == nil {
		return nil, httperror.NotFound("ResetApprovalRequest", in.ID)
	}
	if err := auth.RequireUserAdmin(ac, req.ClientID); err != nil {
		return nil, err
	}
	ok, err := s.Approvals.Decide(ctx, req.ID, resetapproval.StatusDenied, ac.PrincipalID)
	if err != nil {
		return nil, usecase.Internal("REPO", "decide reset-approval failed", err)
	}
	if !ok {
		return nil, usecase.Validation("ALREADY_DECIDED", "request is no longer pending")
	}
	return &messageOutput{Body: apicommon.StatusChangeResponse{Message: "Reset request denied"}}, nil
}
