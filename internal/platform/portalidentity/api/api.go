// Package api serves /api/portal-users — the admin surface of the portal
// identity plane (docs/portal-identity-plan.md Phase 2.5 v2). Portal
// backends' service accounts ensure/invite, list, suspend, and offboard
// their client's portal identities here.
package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// InviteMinter mints portal set-password invites through the shared
// reset-token machinery. Implemented by the password-reset principalEmailer.
type InviteMinter interface {
	// PortalInviteLink mints (without emailing) — the portal-managed path.
	PortalInviteLink(ctx context.Context, identityID string, redirectURI *string) (string, error)
	// SendPortalInvite mints and emails via the platform mailer.
	SendPortalInvite(ctx context.Context, identityID, email string, redirectURI *string) error
}

// State bundles deps.
type State struct {
	Identities *portalidentity.Repository
	Clients    *client.Repository
	// OAuthClients validates invite redirectUris against the client's
	// portal-flagged OAuth clients' registered redirect URIs.
	OAuthClients *platformauth.OAuthClientRepo
	UoW          *usecasepgx.UnitOfWork
	// Invites is optional: nil disables invite delivery/minting (ensure
	// still creates the identity; SSO-only deployments don't need invites).
	Invites InviteMinter
}

const tag = "portal-users"

// Register mounts the portal-users endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Post(g, "ensurePortalUser", "/api/portal-users", "Ensure a portal identity exists for (client, email) and deliver a set-password invite", http.StatusOK, s.ensure)
	apiroute.Get(g, "listPortalUsers", "/api/portal-users", "List a client's portal identities", s.list)
	apiroute.Post(g, "activatePortalUser", "/api/portal-users/{id}/activate", "Reactivate a suspended portal identity", http.StatusOK, s.activate)
	apiroute.Post(g, "deactivatePortalUser", "/api/portal-users/{id}/deactivate", "Suspend a portal identity (blocks portal login, keeps the row)", http.StatusOK, s.deactivate)
	apiroute.Delete(g, "deletePortalUser", "/api/portal-users/{id}", "Delete a portal identity (offboarding)", http.StatusOK, s.delete)
}

// Authorization (docs/portal-identity-plan.md Phase 2.5 v2): portal users
// are CLIENT-delegable. Anchors (platform admins, anchor service accounts)
// pass everywhere; a client-scoped caller — a client administrator signed
// into the platform UI, or the portal application's confined service
// account — needs access to the target client plus the
// platform:iam:portal-user:view/manage permission (granted via a role).

// ── ensure / invite ──────────────────────────────────────────────────────

// PortalUserRequest is the wire body for POST /api/portal-users.
type PortalUserRequest struct {
	ClientID string  `json:"clientId"`
	Email    string  `json:"email"`
	Name     *string `json:"name,omitempty"`
	// ReturnInviteLink skips the platform's invite mailer and returns the
	// set-password link in the response instead, so the portal composes and
	// sends its own fully-branded email. The link is a live 72h bearer
	// credential — do not log it.
	ReturnInviteLink bool `json:"returnInviteLink,omitempty"`
	// RedirectURI is followed after a successful set-password (typically the
	// portal's login entry, so the user lands in the portal signed in). Must
	// exactly match a registered redirect URI of one of the client's
	// portal-flagged OAuth clients.
	RedirectURI *string `json:"redirectUri,omitempty"`
}

// PortalUserResponse reports the idempotent outcome.
type PortalUserResponse struct {
	IdentityID string  `json:"identityId"`
	Created    bool    `json:"created"`
	Invited    bool    `json:"invited"`
	InviteURL  *string `json:"inviteUrl,omitempty"`
}

func (s *State) ensure(ctx context.Context, in *apicommon.In[PortalUserRequest]) (*apicommon.Out[PortalUserResponse], error) {
	ac := auth.FromContext(ctx)
	clientID := strings.TrimSpace(in.Body.ClientID)
	if clientID == "" {
		return nil, usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
	}
	if err := auth.CanManagePortalUsers(ac, clientID); err != nil {
		return nil, err
	}
	var redirectURI *string
	if in.Body.RedirectURI != nil && strings.TrimSpace(*in.Body.RedirectURI) != "" {
		trimmed := strings.TrimSpace(*in.Body.RedirectURI)
		if err := s.validateRedirectURI(ctx, clientID, trimmed); err != nil {
			return nil, err
		}
		redirectURI = &trimmed
	}

	ec := auth.NewExecutionContext(ctx)
	ev, err := usecaseop.Run(ctx, s.UoW, portalidentity.Ensure(s.Identities, s.Clients),
		portalidentity.EnsureCommand{
			ClientID: clientID, Email: in.Body.Email, Name: in.Body.Name,
			Source: string(portalidentity.SourceInvite),
		}, ec)
	if err != nil {
		return nil, err
	}
	ident, err := s.Identities.FindByID(ctx, ev.IdentityID)
	if err != nil || ident == nil {
		return nil, usecase.Internal("REPO", "post-ensure identity lookup failed", err)
	}

	// Invite while the identity cannot yet sign in with a password — on
	// first create AND on later ensures, so a lost or expired invite is
	// recovered by calling ensure again. Identities with a working password
	// are left alone.
	invited := false
	var inviteURL *string
	if s.Invites != nil && (ident.PasswordHash == nil || *ident.PasswordHash == "") {
		if in.Body.ReturnInviteLink {
			link, lerr := s.Invites.PortalInviteLink(ctx, ident.ID, redirectURI)
			if lerr != nil {
				return nil, usecase.Internal("INVITE_LINK", "could not mint the invite link", lerr)
			}
			if link != "" {
				inviteURL = &link
			}
		} else {
			if ierr := s.Invites.SendPortalInvite(ctx, ident.ID, ident.Email, redirectURI); ierr != nil {
				// The identity exists either way; failing keeps the call
				// honest — a retry lands back here and re-sends.
				return nil, usecase.Internal("INVITE_EMAIL", "could not send the invite email", ierr)
			}
			invited = true
		}
	}
	return &apicommon.Out[PortalUserResponse]{Body: PortalUserResponse{
		IdentityID: ident.ID,
		Created:    ev.Created,
		Invited:    invited,
		InviteURL:  inviteURL,
	}}, nil
}

// validateRedirectURI checks the post-set-password redirect against the
// registered redirect URIs of the client's portal-flagged OAuth clients
// (exact match — the OAuth rule). Fail closed.
func (s *State) validateRedirectURI(ctx context.Context, clientID, redirectURI string) error {
	if s.OAuthClients == nil {
		return usecase.Internal("PORTAL_REDIRECT", "OAuth client repo not wired", nil)
	}
	oclients, err := s.OAuthClients.FindByPortalClient(ctx, clientID)
	if err != nil {
		return usecase.Internal("REPO", "find_portal_oauth_clients failed", err)
	}
	for _, oc := range oclients {
		for _, u := range oc.RedirectURIs {
			if u == redirectURI {
				return nil
			}
		}
	}
	return usecase.Validation("REDIRECT_URI_INVALID",
		"redirectUri must exactly match a registered redirect URI of one of the client's portal OAuth clients")
}

// ── list / status / delete ───────────────────────────────────────────────

// PortalUserListItem is one row of GET /api/portal-users.
type PortalUserListItem struct {
	IdentityID  string           `json:"identityId"`
	Email       string           `json:"email"`
	Name        string           `json:"name"`
	Status      string           `json:"status"`
	Source      string           `json:"source"`
	HasPassword bool             `json:"hasPassword"`
	LastLoginAt *httpcompat.Time `json:"lastLoginAt,omitempty"`
	CreatedAt   httpcompat.Time  `json:"createdAt"`
	UpdatedAt   httpcompat.Time  `json:"updatedAt"`
}

// PortalUserListResponse is the GET /api/portal-users envelope.
type PortalUserListResponse struct {
	PortalUsers []PortalUserListItem `json:"portalUsers"`
}

type listInput struct {
	ClientID string `query:"clientId" doc:"Tenant client whose portal identities to list"`
}

func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[PortalUserListResponse], error) {
	ac := auth.FromContext(ctx)
	clientID := strings.TrimSpace(in.ClientID)
	if clientID == "" {
		return nil, usecase.Validation("CLIENT_ID_REQUIRED", "clientId query param is required")
	}
	if err := auth.CanReadPortalUsers(ac, clientID); err != nil {
		return nil, err
	}
	rows, err := s.Identities.FindByClient(ctx, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_portal_identities failed", err)
	}
	items := make([]PortalUserListItem, 0, len(rows))
	for _, i := range rows {
		item := PortalUserListItem{
			IdentityID:  i.ID,
			Email:       i.Email,
			Name:        i.Name,
			Status:      string(i.Status),
			Source:      string(i.Source),
			HasPassword: i.PasswordHash != nil && *i.PasswordHash != "",
			CreatedAt:   jsontime.New(i.CreatedAt),
			UpdatedAt:   jsontime.New(i.UpdatedAt),
		}
		if i.LastLoginAt != nil {
			t := jsontime.New(*i.LastLoginAt)
			item.LastLoginAt = &t
		}
		items = append(items, item)
	}
	return &apicommon.Out[PortalUserListResponse]{Body: PortalUserListResponse{PortalUsers: items}}, nil
}

// PortalUserClientBody carries the tenant client a status change targets —
// the per-client scoping check for id-addressed mutations.
type PortalUserClientBody struct {
	ClientID string `json:"clientId"`
}

type statusInput struct {
	ID   string `path:"id"`
	Body PortalUserClientBody
}

func (s *State) setStatus(ctx context.Context, in *statusInput, status portalidentity.Status) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	ac := auth.FromContext(ctx)
	clientID := strings.TrimSpace(in.Body.ClientID)
	if clientID == "" {
		return nil, usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
	}
	if err := auth.CanManagePortalUsers(ac, clientID); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, portalidentity.SetStatus(s.Identities),
		portalidentity.SetStatusCommand{ID: in.ID, ClientID: clientID, Status: string(status)}, ec); err != nil {
		return nil, err
	}
	msg := "Portal user activated"
	if status == portalidentity.StatusDisabled {
		msg = "Portal user deactivated"
	}
	return &apicommon.Out[apicommon.StatusChangeResponse]{Body: apicommon.StatusChangeResponse{Message: msg}}, nil
}

func (s *State) activate(ctx context.Context, in *statusInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	return s.setStatus(ctx, in, portalidentity.StatusActive)
}

func (s *State) deactivate(ctx context.Context, in *statusInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	return s.setStatus(ctx, in, portalidentity.StatusDisabled)
}

type deleteInput struct {
	ID       string `path:"id"`
	ClientID string `query:"clientId" doc:"Tenant client whose portal identity to delete"`
}

func (s *State) delete(ctx context.Context, in *deleteInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	ac := auth.FromContext(ctx)
	clientID := strings.TrimSpace(in.ClientID)
	if clientID == "" {
		return nil, usecase.Validation("CLIENT_ID_REQUIRED", "clientId query param is required")
	}
	if err := auth.CanManagePortalUsers(ac, clientID); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, portalidentity.Delete(s.Identities),
		portalidentity.DeleteCommand{ID: in.ID, ClientID: clientID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.StatusChangeResponse]{Body: apicommon.StatusChangeResponse{Message: "Portal user deleted"}}, nil
}
