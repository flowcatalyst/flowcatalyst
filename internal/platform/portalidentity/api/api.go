// Package api serves /api/portal-users and /api/portal-apps — the admin
// surface of the portal identity plane (docs/portal-identity-plan.md Phase
// 2.5 v2). Portal backends' service accounts ensure/invite, grant/revoke,
// search, suspend, and offboard their client's portal identities here; the
// platform UI uses the same surface (minus invites, which the portal app
// initiates).
package api

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// InviteMinter mints portal set-password invites through the shared
// reset-token machinery. Implemented by the password-reset principalEmailer.
type InviteMinter interface {
	// PortalInviteLink mints (without emailing) — the portal-managed path —
	// and reports when the link expires.
	PortalInviteLink(ctx context.Context, identityID string, redirectURI *string) (string, time.Time, error)
	// SendPortalInvite mints and emails via the platform mailer, reporting
	// when the link expires.
	SendPortalInvite(ctx context.Context, identityID, email string, redirectURI *string) (time.Time, error)
	// SendPortalSSOInvite emails the SSO-org invite (no set-password step).
	SendPortalSSOInvite(ctx context.Context, email, portalURL string) error
}

// State bundles deps.
type State struct {
	Identities *portalidentity.Repository
	Apps       *portalidentity.AppRepository
	Clients    *client.Repository
	// OAuthClients validates invite redirectUris against the client's
	// portal-flagged OAuth clients' registered redirect URIs.
	OAuthClients *platformauth.OAuthClientRepo
	UoW          *usecasepgx.UnitOfWork
	// Invites is optional: nil disables invite delivery/minting (ensure
	// still creates the identity; SSO-only deployments don't need invites).
	Invites InviteMinter
	// IdPs routes SSO-owned domains: an invitee whose email domain is owned
	// by an OIDC IdP gets an "open the portal" invite instead of a
	// set-password link (their org signs them in; a password would be an
	// SSO bypass). Optional — nil skips the check.
	IdPs *identityprovider.Repository
}

const (
	tag     = "portal-users"
	appsTag = "portal-apps"
)

// Register mounts the portal-users and portal-apps endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Post(g, "ensurePortalUser", "/api/portal-users", "Ensure a portal identity exists for (client, email), grant a portal app, and deliver a set-password invite", http.StatusOK, s.ensure)
	apiroute.Get(g, "listPortalUsers", "/api/portal-users", "Search a client's portal identities (prefix match on email and name)", s.list)
	apiroute.Post(g, "activatePortalUser", "/api/portal-users/{id}/activate", "Reactivate a suspended portal identity", http.StatusOK, s.activate)
	apiroute.Post(g, "deactivatePortalUser", "/api/portal-users/{id}/deactivate", "Suspend a portal identity (blocks portal login, keeps the row)", http.StatusOK, s.deactivate)
	apiroute.Delete(g, "deletePortalUser", "/api/portal-users/{id}", "Delete a portal identity (offboarding from every portal of the client)", http.StatusOK, s.delete)
	apiroute.Post(g, "grantPortalUserApp", "/api/portal-users/{id}/apps", "Grant a portal identity access to one of the client's portal apps", http.StatusOK, s.grantApp)
	apiroute.Delete(g, "revokePortalUserApp", "/api/portal-users/{id}/apps/{portalAppCode}", "Revoke a portal identity's access to one portal app (the identity stays)", http.StatusOK, s.revokeApp)

	a := apiroute.New(api, appsTag)
	apiroute.Get(a, "listPortalApps", "/api/portal-apps", "List portal apps (a client's, or every client's for anchors)", s.listApps)
	apiroute.Post(a, "createPortalApp", "/api/portal-apps", "Register a portal app for a client and provision its portal OAuth client", http.StatusCreated, s.createApp)
	apiroute.Put(a, "updatePortalApp", "/api/portal-apps/{id}", "Update a portal app's name, description, or active flag", http.StatusOK, s.updateApp)
	apiroute.Delete(a, "deletePortalApp", "/api/portal-apps/{id}", "Delete a portal app together with its portal OAuth clients", http.StatusOK, s.deleteApp)
	apiroute.Post(a, "assignUnassignedPortalUsers", "/api/portal-apps/{id}/assign-unassigned", "Grant a portal app to every one of the client's portal users that has no portal app", http.StatusOK, s.assignUnassigned)
}

// Authorization (docs/portal-identity-plan.md Phase 2.5 v2): portal users
// and portal apps are CLIENT-delegable. Anchors (platform admins, anchor
// service accounts) pass everywhere; a client-scoped caller — a client
// administrator signed into the platform UI, or the portal application's
// confined service account — needs access to the target client plus the
// platform:iam:portal-user:view/manage permission (granted via a role).

// ── ensure / invite ──────────────────────────────────────────────────────

// PortalUserRequest is the wire body for POST /api/portal-users.
type PortalUserRequest struct {
	ClientID string  `json:"clientId"`
	Email    string  `json:"email"`
	Name     *string `json:"name,omitempty"`
	// PortalAppCode names the portal app (portal_apps.code) the calling
	// portal is: the identity is granted that app, invite redirects default
	// to that app's portal, and the code is echoed back. Omit only for
	// legacy client-wide portals (OAuth clients not linked to an app).
	PortalAppCode *string `json:"portalAppCode,omitempty"`
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

// PortalUserResponse reports the idempotent outcome. SSOManaged means the
// email domain is owned by an OIDC IdP: there is no set-password
// step — the user signs in through their organisation, JIT-completing the
// identity on first login. With returnInviteLink, inviteUrl is then the
// PORTAL LOGIN destination (the caller's redirectUri, else the portal's
// derived origin) so the portal's own invite email can embed one link
// uniformly: password invitees get the set-password link, SSO invitees get
// the portal entry.
type PortalUserResponse struct {
	IdentityID string  `json:"identityId"`
	Created    bool    `json:"created"`
	Invited    bool    `json:"invited"`
	InviteURL  *string `json:"inviteUrl,omitempty"`
	SSOManaged bool    `json:"ssoManaged,omitempty"`
	// HasPassword distinguishes the deliberate no-op: re-ensuring an identity
	// that already holds a working password sends NOTHING (the account is
	// live; a "resend" would be a password reset, which the user can do
	// themselves from the portal login). Callers surface that instead of
	// reporting a phantom "invite sent".
	HasPassword bool `json:"hasPassword"`
	// PortalAppCode echoes the granted portal app's code (when one was given).
	PortalAppCode *string `json:"portalAppCode,omitempty"`
	// State is the identity's lifecycle after this call: INVITED |
	// INVITE_EXPIRED | ACTIVE | SUSPENDED.
	State string `json:"state" enum:"INVITED,INVITE_EXPIRED,ACTIVE,SUSPENDED"`
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
	var app *portalidentity.App
	if in.Body.PortalAppCode != nil && strings.TrimSpace(*in.Body.PortalAppCode) != "" {
		var err error
		if app, err = s.appByCode(ctx, clientID, *in.Body.PortalAppCode); err != nil {
			return nil, err
		}
	}
	var redirectURI *string
	if in.Body.RedirectURI != nil && strings.TrimSpace(*in.Body.RedirectURI) != "" {
		trimmed := strings.TrimSpace(*in.Body.RedirectURI)
		if err := s.validateRedirectURI(ctx, clientID, trimmed); err != nil {
			return nil, err
		}
		redirectURI = &trimmed
	} else {
		// No explicit redirect: default to the portal's own origin, derived
		// from the (app's, else the client's) portal OAuth client's
		// registered redirect URI — so the invitee lands back at the portal
		// after setting their password instead of dead-ending on the
		// platform login. Best-effort: no portal OAuth client → no redirect
		// (the set-password page then shows a return-to-your-portal message).
		redirectURI = s.defaultPortalRedirect(ctx, clientID, app)
	}

	cmd := portalidentity.EnsureCommand{
		ClientID: clientID, Email: in.Body.Email, Name: in.Body.Name,
		Source: string(portalidentity.SourceInvite),
	}
	var appCode *string
	if app != nil {
		cmd.PortalAppID = app.ID
		appCode = &app.Code
	}
	ec := auth.NewExecutionContext(ctx)
	ev, err := usecaseop.Run(ctx, s.UoW, portalidentity.Ensure(s.Identities, s.Clients, s.Apps), cmd, ec)
	if err != nil {
		return nil, err
	}
	ident, err := s.Identities.FindByID(ctx, ev.IdentityID)
	if err != nil || ident == nil {
		return nil, usecase.Internal("REPO", "post-ensure identity lookup failed", err)
	}
	resp := PortalUserResponse{
		IdentityID:    ident.ID,
		Created:       ev.Created,
		HasPassword:   ident.HasPassword(),
		PortalAppCode: appCode,
	}
	now := time.Now().UTC()

	// SSO-owned domain: never a set-password invite. Platform-mailed mode
	// sends "open the portal" (their org signs them in); returnInviteLink
	// callers get ssoManaged and send their own version of that email.
	if s.IdPs != nil {
		domain := ident.Email[strings.LastIndexByte(ident.Email, '@')+1:]
		if idp, derr := identityprovider.OIDCProviderForDomain(ctx, s.IdPs, domain); derr == nil && idp != nil {
			resp.SSOManaged = true
			pending := ident.LastLoginAt == nil
			switch {
			case in.Body.ReturnInviteLink:
				resp.InviteURL = redirectURI
			case s.Invites != nil && redirectURI != nil && pending:
				if serr := s.Invites.SendPortalSSOInvite(ctx, ident.Email, *redirectURI); serr == nil {
					resp.Invited = true
				}
			}
			if pending && (resp.Invited || resp.InviteURL != nil) {
				s.markInvited(ctx, ident, now, nil)
			}
			resp.State = string(ident.State(now))
			return &apicommon.Out[PortalUserResponse]{Body: resp}, nil
		}
	}

	// Invite while the identity cannot yet sign in with a password — on
	// first create AND on later ensures, so a lost or expired invite is
	// recovered by calling ensure again. Identities with a working password
	// are left alone.
	if s.Invites != nil && !ident.HasPassword() {
		var expires time.Time
		if in.Body.ReturnInviteLink {
			link, exp, lerr := s.Invites.PortalInviteLink(ctx, ident.ID, redirectURI)
			if lerr != nil {
				return nil, usecase.Internal("INVITE_LINK", "could not mint the invite link", lerr)
			}
			if link != "" {
				resp.InviteURL = &link
				expires = exp
			}
		} else {
			exp, ierr := s.Invites.SendPortalInvite(ctx, ident.ID, ident.Email, redirectURI)
			if ierr != nil {
				// The identity exists either way; failing keeps the call
				// honest — a retry lands back here and re-sends.
				return nil, usecase.Internal("INVITE_EMAIL", "could not send the invite email", ierr)
			}
			resp.Invited = true
			expires = exp
		}
		if !expires.IsZero() {
			s.markInvited(ctx, ident, now, &expires)
		}
	}
	resp.State = string(ident.State(now))
	return &apicommon.Out[PortalUserResponse]{Body: resp}, nil
}

// markInvited records the invite on the identity (best-effort bookkeeping:
// the invite itself already went out) and mirrors it on the loaded value so
// the response's state reflects it.
func (s *State) markInvited(ctx context.Context, ident *portalidentity.Identity, at time.Time, expires *time.Time) {
	if err := s.Identities.MarkInvited(ctx, ident.ID, at, expires); err == nil {
		ident.InvitedAt, ident.InviteExpiresAt = &at, expires
	}
}

// appByCode resolves one of the client's portal apps by code (404 when the
// client has no such app).
func (s *State) appByCode(ctx context.Context, clientID, code string) (*portalidentity.App, error) {
	if s.Apps == nil {
		return nil, usecase.Internal("PORTAL_APPS", "portal app repo not wired", nil)
	}
	app, err := s.Apps.FindByClientAndCode(ctx, clientID, code)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_portal_app failed", err)
	}
	if app == nil {
		return nil, httperror.NotFound("PortalApp", portalidentity.NormalizeAppCode(code))
	}
	return app, nil
}

// defaultPortalRedirect derives the portal's origin ("https://portal.example.com/")
// from the first registered redirect URI of the client's portal OAuth
// clients — preferring those linked to app, when given. Safe by
// construction — the origin comes from admin-registered OAuth config, never
// caller input.
func (s *State) defaultPortalRedirect(ctx context.Context, clientID string, app *portalidentity.App) *string {
	if s.OAuthClients == nil {
		return nil
	}
	oclients, err := s.OAuthClients.FindByPortalClient(ctx, clientID)
	if err != nil {
		return nil
	}
	if app != nil {
		// Stable partition: the app's own OAuth clients first.
		slices.SortStableFunc(oclients, func(a, b platformauth.OAuthClient) int {
			return boolRank(linkedTo(b, app.ID)) - boolRank(linkedTo(a, app.ID))
		})
	}
	for _, oc := range oclients {
		for _, raw := range oc.RedirectURIs {
			u, perr := url.Parse(raw)
			// A wildcard *-pattern is a matching rule, not a real origin —
			// skip it rather than mailing out an unusable link.
			if perr != nil || u.Scheme == "" || u.Host == "" || strings.Contains(u.Host, "*") {
				continue
			}
			origin := u.Scheme + "://" + u.Host + "/"
			return &origin
		}
	}
	return nil
}

func linkedTo(oc platformauth.OAuthClient, appID string) bool {
	return oc.PortalAppID != nil && *oc.PortalAppID == appID
}

func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
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
		if slices.Contains(oc.RedirectURIs, redirectURI) {
			return nil
		}
	}
	return usecase.Validation("REDIRECT_URI_INVALID",
		"redirectUri must exactly match a registered redirect URI of one of the client's portal OAuth clients")
}

// ── search / status / delete ─────────────────────────────────────────────

// PortalUserAppRef is one portal app an identity is granted.
type PortalUserAppRef struct {
	ID        string          `json:"id"`
	Code      string          `json:"code"`
	Name      string          `json:"name"`
	Source    string          `json:"source"`
	GrantedAt httpcompat.Time `json:"grantedAt"`
}

// PortalUserListItem is one row of GET /api/portal-users.
type PortalUserListItem struct {
	IdentityID string `json:"identityId"`
	Email      string `json:"email"`
	Name       string `json:"name"`
	// Status is the stored ACTIVE | DISABLED flag (suspension).
	Status string `json:"status"`
	// State is the derived lifecycle: INVITED (invite outstanding),
	// INVITE_EXPIRED (invite lapsed, never completed), ACTIVE (password set
	// or signed in), SUSPENDED.
	State           string             `json:"state" enum:"INVITED,INVITE_EXPIRED,ACTIVE,SUSPENDED"`
	Source          string             `json:"source"`
	HasPassword     bool               `json:"hasPassword"`
	Apps            []PortalUserAppRef `json:"apps"`
	InvitedAt       *httpcompat.Time   `json:"invitedAt,omitempty"`
	InviteExpiresAt *httpcompat.Time   `json:"inviteExpiresAt,omitempty"`
	LastLoginAt     *httpcompat.Time   `json:"lastLoginAt,omitempty"`
	CreatedAt       httpcompat.Time    `json:"createdAt"`
	UpdatedAt       httpcompat.Time    `json:"updatedAt"`
}

// PortalUserListResponse is the GET /api/portal-users envelope.
type PortalUserListResponse struct {
	PortalUsers []PortalUserListItem `json:"portalUsers"`
	Total       int64                `json:"total"`
	Page        int                  `json:"page"`
	Size        int                  `json:"size"`
}

type listInput struct {
	ClientID      string `query:"clientId" doc:"Tenant client whose portal identities to list"`
	Q             string `query:"q" doc:"Prefix (TERM%) matched case-insensitively against email and name"`
	PortalAppCode string `query:"portalAppCode" doc:"Only identities granted this portal app"`
	Unassigned    bool   `query:"unassigned" doc:"Only identities granted no portal app (cannot be combined with portalAppCode)"`
	Page          int    `query:"page" doc:"0-based page index (default 0)"`
	Size          int    `query:"size" doc:"Page size (default 100, max 1000)"`
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
	filter := portalidentity.SearchFilter{ClientID: clientID, Query: in.Q, Unassigned: in.Unassigned}
	if in.Unassigned && strings.TrimSpace(in.PortalAppCode) != "" {
		return nil, usecase.Validation("FILTER_CONFLICT", "unassigned and portalAppCode cannot be combined")
	}
	if code := strings.TrimSpace(in.PortalAppCode); code != "" {
		app, err := s.appByCode(ctx, clientID, code)
		if err != nil {
			return nil, err
		}
		filter.AppID = app.ID
	}
	page := max(in.Page, 0)
	size := in.Size
	if size <= 0 {
		size = 100
	}
	size = min(size, apicommon.MaxPageSize)
	filter.Offset, filter.Limit = page*size, size

	rows, total, err := s.Identities.Search(ctx, filter)
	if err != nil {
		return nil, usecase.Internal("REPO", "search_portal_identities failed", err)
	}
	apps, err := s.clientApps(ctx, clientID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	items := make([]PortalUserListItem, 0, len(rows))
	for n := range rows {
		items = append(items, listItem(&rows[n], apps, now))
	}
	return &apicommon.Out[PortalUserListResponse]{Body: PortalUserListResponse{
		PortalUsers: items, Total: total, Page: page, Size: size,
	}}, nil
}

// clientApps indexes the client's portal apps by id for grant display.
func (s *State) clientApps(ctx context.Context, clientID string) (map[string]portalidentity.App, error) {
	out := map[string]portalidentity.App{}
	if s.Apps == nil {
		return out, nil
	}
	apps, err := s.Apps.FindByClient(ctx, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_portal_apps failed", err)
	}
	for _, a := range apps {
		out[a.ID] = a
	}
	return out, nil
}

func listItem(i *portalidentity.Identity, apps map[string]portalidentity.App, now time.Time) PortalUserListItem {
	item := PortalUserListItem{
		IdentityID:  i.ID,
		Email:       i.Email,
		Name:        i.Name,
		Status:      string(i.Status),
		State:       string(i.State(now)),
		Source:      string(i.Source),
		HasPassword: i.HasPassword(),
		Apps:        make([]PortalUserAppRef, 0, len(i.Apps)),
		InvitedAt:   optTime(i.InvitedAt),
		LastLoginAt: optTime(i.LastLoginAt),
		CreatedAt:   jsontime.New(i.CreatedAt),
		UpdatedAt:   jsontime.New(i.UpdatedAt),
	}
	item.InviteExpiresAt = optTime(i.InviteExpiresAt)
	for _, g := range i.Apps {
		ref := PortalUserAppRef{
			ID: g.AppID, Code: g.AppID, Name: g.AppID,
			Source: string(g.Source), GrantedAt: jsontime.New(g.GrantedAt),
		}
		if a, ok := apps[g.AppID]; ok {
			ref.Code, ref.Name = a.Code, a.Name
		}
		item.Apps = append(item.Apps, ref)
	}
	return item
}

func optTime(t *time.Time) *httpcompat.Time {
	if t == nil {
		return nil
	}
	v := jsontime.New(*t)
	return &v
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

// ── app grants ───────────────────────────────────────────────────────────

// PortalUserAppGrantBody names the portal app to grant.
type PortalUserAppGrantBody struct {
	ClientID      string `json:"clientId"`
	PortalAppCode string `json:"portalAppCode"`
}

type grantInput struct {
	ID   string `path:"id"`
	Body PortalUserAppGrantBody
}

func (s *State) grantApp(ctx context.Context, in *grantInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	clientID := strings.TrimSpace(in.Body.ClientID)
	if err := s.authorizeManage(ctx, clientID); err != nil {
		return nil, err
	}
	app, err := s.appByCode(ctx, clientID, in.Body.PortalAppCode)
	if err != nil {
		return nil, err
	}
	if !app.Active {
		return nil, usecase.Validation("PORTAL_APP_INACTIVE", "portal app '"+app.Code+"' is inactive")
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, portalidentity.GrantApp(s.Identities, s.Apps),
		portalidentity.AppGrantCommand{ClientID: clientID, IdentityID: in.ID, PortalAppID: app.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.StatusChangeResponse]{Body: apicommon.StatusChangeResponse{Message: "Portal app access granted"}}, nil
}

type revokeInput struct {
	ID            string `path:"id"`
	PortalAppCode string `path:"portalAppCode"`
	ClientID      string `query:"clientId" doc:"Tenant client that owns the portal identity and app"`
}

func (s *State) revokeApp(ctx context.Context, in *revokeInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	clientID := strings.TrimSpace(in.ClientID)
	if err := s.authorizeManage(ctx, clientID); err != nil {
		return nil, err
	}
	app, err := s.appByCode(ctx, clientID, in.PortalAppCode)
	if err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, portalidentity.RevokeApp(s.Identities, s.Apps),
		portalidentity.AppGrantCommand{ClientID: clientID, IdentityID: in.ID, PortalAppID: app.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.StatusChangeResponse]{Body: apicommon.StatusChangeResponse{Message: "Portal app access revoked"}}, nil
}

func (s *State) authorizeManage(ctx context.Context, clientID string) error {
	if clientID == "" {
		return usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
	}
	return auth.CanManagePortalUsers(auth.FromContext(ctx), clientID)
}

// ── portal apps ──────────────────────────────────────────────────────────

// PortalAppResponse is one portal app with its OAuth clients and user count.
type PortalAppResponse struct {
	ID           string                             `json:"id"`
	ClientID     string                             `json:"clientId"`
	Code         string                             `json:"code"`
	Name         string                             `json:"name"`
	Description  *string                            `json:"description,omitempty"`
	Active       bool                               `json:"active"`
	OAuthClients []portalidentity.LinkedOAuthClient `json:"oauthClients"`
	UserCount    int                                `json:"userCount"`
	CreatedAt    httpcompat.Time                    `json:"createdAt"`
	UpdatedAt    httpcompat.Time                    `json:"updatedAt"`
}

// PortalAppListResponse is the GET /api/portal-apps envelope.
type PortalAppListResponse struct {
	PortalApps []PortalAppResponse `json:"portalApps"`
	// UnassignedUsers counts the client's portal users granted no portal
	// app (present only when clientId is given). Such users cannot sign in
	// through any app-linked portal OAuth client.
	UnassignedUsers *int64 `json:"unassignedUsers,omitempty"`
}

type listAppsInput struct {
	ClientID string `query:"clientId" doc:"Tenant client whose portal apps to list (anchors may omit it for every client's)"`
}

func (s *State) listApps(ctx context.Context, in *listAppsInput) (*apicommon.Out[PortalAppListResponse], error) {
	ac := auth.FromContext(ctx)
	clientID := strings.TrimSpace(in.ClientID)
	if clientID == "" {
		if !ac.IsAnchor() {
			return nil, usecase.Validation("CLIENT_ID_REQUIRED", "clientId query param is required")
		}
	} else if err := auth.CanReadPortalUsers(ac, clientID); err != nil {
		return nil, err
	}
	apps, err := s.Apps.FindByClient(ctx, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_portal_apps failed", err)
	}
	out, err := s.appResponses(ctx, apps)
	if err != nil {
		return nil, err
	}
	body := PortalAppListResponse{PortalApps: out}
	if clientID != "" {
		n, err := s.Identities.CountUnassigned(ctx, clientID)
		if err != nil {
			return nil, usecase.Internal("REPO", "count_unassigned failed", err)
		}
		body.UnassignedUsers = &n
	}
	return &apicommon.Out[PortalAppListResponse]{Body: body}, nil
}

func (s *State) appResponses(ctx context.Context, apps []portalidentity.App) ([]PortalAppResponse, error) {
	ids := make([]string, len(apps))
	for n, a := range apps {
		ids[n] = a.ID
	}
	counts, err := s.Apps.GrantCounts(ctx, ids)
	if err != nil {
		return nil, usecase.Internal("REPO", "portal_app_grant_counts failed", err)
	}
	linked, err := s.Apps.LinkedOAuthClients(ctx, ids)
	if err != nil {
		return nil, usecase.Internal("REPO", "portal_app_oauth_clients failed", err)
	}
	out := make([]PortalAppResponse, 0, len(apps))
	for _, a := range apps {
		oc := linked[a.ID]
		if oc == nil {
			oc = []portalidentity.LinkedOAuthClient{}
		}
		out = append(out, PortalAppResponse{
			ID: a.ID, ClientID: a.ClientID, Code: a.Code, Name: a.Name,
			Description: a.Description, Active: a.Active,
			OAuthClients: oc, UserCount: counts[a.ID],
			CreatedAt: jsontime.New(a.CreatedAt), UpdatedAt: jsontime.New(a.UpdatedAt),
		})
	}
	return out, nil
}

// CreatePortalAppRequest is the wire body for POST /api/portal-apps.
type CreatePortalAppRequest struct {
	ClientID string `json:"clientId"`
	// Code is the stable identifier the portal app sends as portalAppCode:
	// lower-case letters, digits, '-' and '_' (case-insensitive on input).
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	// RedirectURIs are the portal's OAuth callback URL(s), registered on the
	// provisioned OAuth client. Optional now, but the portal cannot sign
	// anyone in until one is registered.
	RedirectURIs []string `json:"redirectUris,omitempty"`
	// ClientType of the provisioned OAuth client: CONFIDENTIAL (default,
	// server-side portal with a secret) or PUBLIC (browser-only, PKCE).
	ClientType *string `json:"clientType,omitempty" enum:"CONFIDENTIAL,PUBLIC"`
}

// CreatePortalAppResponse is the new app plus the credentials of the OAuth
// client provisioned for it. ClientSecret (CONFIDENTIAL only) is shown
// exactly once — it is stored hashed.
type CreatePortalAppResponse struct {
	PortalApp     PortalAppResponse `json:"portalApp"`
	OAuthClientID string            `json:"oauthClientId"`
	// OAuthClientRowID is the OAuth client's platform id (oac_…), for
	// deep-linking to it under Identity & Access → OAuth Clients.
	OAuthClientRowID string  `json:"oauthClientRowId"`
	ClientType       string  `json:"clientType"`
	ClientSecret     *string `json:"clientSecret,omitempty"`
}

func (s *State) createApp(ctx context.Context, in *apicommon.In[CreatePortalAppRequest]) (*apicommon.Out[CreatePortalAppResponse], error) {
	clientID := strings.TrimSpace(in.Body.ClientID)
	if err := s.authorizeManage(ctx, clientID); err != nil {
		return nil, err
	}
	cmd := portalidentity.CreateAppWithOAuthClientCommand{
		ClientID: clientID, Code: in.Body.Code, Name: in.Body.Name, Description: in.Body.Description,
		RedirectURIs: in.Body.RedirectURIs,
	}
	if in.Body.ClientType != nil {
		cmd.ClientType = *in.Body.ClientType
	}
	ec := auth.NewExecutionContext(ctx)
	res, err := usecaseop.RunTx(ctx, s.UoW, portalidentity.CreateAppWithOAuthClient(s.Apps, s.Clients, s.OAuthClients), cmd, ec)
	if err != nil {
		return nil, err
	}
	app, err := s.appOut(ctx, res.AppID)
	if err != nil {
		return nil, err
	}
	out := CreatePortalAppResponse{
		PortalApp: app.Body, OAuthClientID: res.OAuthClientID,
		OAuthClientRowID: res.OAuthClientRowID, ClientType: res.ClientType,
	}
	if res.ClientSecret != "" {
		out.ClientSecret = &res.ClientSecret
	}
	return &apicommon.Out[CreatePortalAppResponse]{Body: out}, nil
}

// UpdatePortalAppRequest is the wire body for PUT /api/portal-apps/{id}.
// The code is immutable (portal apps are configured with it).
type UpdatePortalAppRequest struct {
	ClientID    string  `json:"clientId"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Active      *bool   `json:"active,omitempty"`
}

type updateAppInput struct {
	ID   string `path:"id"`
	Body UpdatePortalAppRequest
}

func (s *State) updateApp(ctx context.Context, in *updateAppInput) (*apicommon.Out[PortalAppResponse], error) {
	clientID := strings.TrimSpace(in.Body.ClientID)
	if err := s.authorizeManage(ctx, clientID); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, portalidentity.UpdateApp(s.Apps),
		portalidentity.UpdateAppCommand{
			ClientID: clientID, ID: in.ID, Name: in.Body.Name,
			Description: in.Body.Description, Active: in.Body.Active,
		}, ec); err != nil {
		return nil, err
	}
	return s.appOut(ctx, in.ID)
}

type deleteAppInput struct {
	ID       string `path:"id"`
	ClientID string `query:"clientId" doc:"Tenant client that owns the portal app"`
}

func (s *State) deleteApp(ctx context.Context, in *deleteAppInput) (*apicommon.Out[apicommon.StatusChangeResponse], error) {
	clientID := strings.TrimSpace(in.ClientID)
	if err := s.authorizeManage(ctx, clientID); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	res, err := usecaseop.RunTx(ctx, s.UoW, portalidentity.DeleteApp(s.Apps, s.OAuthClients),
		portalidentity.DeleteAppCommand{ClientID: clientID, ID: in.ID}, ec)
	if err != nil {
		return nil, err
	}
	msg := "Portal app deleted"
	if n := len(res.DeletedOAuthClientIDs); n > 0 {
		msg += " with its OAuth client"
		if n > 1 {
			msg += "s"
		}
	}
	return &apicommon.Out[apicommon.StatusChangeResponse]{Body: apicommon.StatusChangeResponse{Message: msg}}, nil
}

func (s *State) appOut(ctx context.Context, id string) (*apicommon.Out[PortalAppResponse], error) {
	app, err := s.Apps.FindByID(ctx, id)
	if err != nil || app == nil {
		return nil, usecase.Internal("REPO", "portal app lookup failed", err)
	}
	out, err := s.appResponses(ctx, []portalidentity.App{*app})
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[PortalAppResponse]{Body: out[0]}, nil
}

// AssignUnassignedBody names the tenant client that owns the portal app.
type AssignUnassignedBody struct {
	ClientID string `json:"clientId"`
}

// AssignUnassignedResponse reports the bulk assignment.
type AssignUnassignedResponse struct {
	PortalAppCode string `json:"portalAppCode"`
	// Assigned is how many portal users were granted the app.
	Assigned int `json:"assigned"`
}

type assignUnassignedInput struct {
	ID   string `path:"id"`
	Body AssignUnassignedBody
}

func (s *State) assignUnassigned(ctx context.Context, in *assignUnassignedInput) (*apicommon.Out[AssignUnassignedResponse], error) {
	clientID := strings.TrimSpace(in.Body.ClientID)
	if err := s.authorizeManage(ctx, clientID); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	res, err := usecaseop.RunTx(ctx, s.UoW, portalidentity.AssignUnassignedToApp(s.Identities, s.Apps),
		portalidentity.AssignUnassignedCommand{ClientID: clientID, PortalAppID: in.ID}, ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[AssignUnassignedResponse]{Body: AssignUnassignedResponse{
		PortalAppCode: res.AppCode, Assigned: len(res.IdentityIDs),
	}}, nil
}
