package appusers

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

const (
	tag             = "application-users"
	defaultPageSize = 50
	maxPageSize     = 200
)

// InviteEmailer mints and delivers the first-time set-password invitation. The
// passwordreset principalEmailer satisfies it; minting invalidates the user's
// earlier tokens, so a resend replaces the old link.
type InviteEmailer interface {
	SendInviteRedirect(ctx context.Context, p *principal.Principal, redirectURI *string) error
	InviteLink(ctx context.Context, p *principal.Principal, redirectURI *string) (string, error)
}

// State bundles the deps.
type State struct {
	Repo          *Repository
	Principals    *principal.Repository
	Roles         *role.Repository
	Apps          *application.Repository
	ClientConfigs *application.ClientConfigRepo
	Clients       *client.Repository
	// Mappings and IdentityProviders (optional) tell whether an invitee's email
	// domain is federated, whose users are never sent a set-password invitation.
	Mappings          *emaildomainmapping.Repository
	IdentityProviders *identityprovider.Repository
	UoW               *usecasepgx.UnitOfWork
	Invites           InviteEmailer
}

// Register mounts the endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listApplicationUsers", "/api/applications/{appCode}/users", "List an application's users (CLIENT users of its enabled clients holding its roles)", s.list)
	apiroute.Get(g, "getApplicationUser", "/api/applications/{appCode}/users/{id}", "Get one of an application's users", s.get)
	apiroute.Post(g, "inviteApplicationUser", "/api/applications/{appCode}/users", "Invite (create) a user holding the application's roles at one of its enabled clients", http.StatusCreated, s.invite)
	apiroute.Get(g, "listApplicationInvitations", "/api/applications/{appCode}/invitations", "List an application's outstanding invitations (users who have not set a password)", s.invitations)
	apiroute.Post(g, "resendApplicationInvitation", "/api/applications/{appCode}/invitations/{id}/resend", "Resend an application user's invitation (replaces the earlier link)", http.StatusOK, s.resend)
	apiroute.Delete(g, "revokeApplicationInvitation", "/api/applications/{appCode}/invitations/{id}", "Revoke an application user's invitation (removes the link and deactivates the user)", http.StatusNoContent, s.revoke)
}

// ── wire types ───────────────────────────────────────────────────────────

// ApplicationUser is one of an application's users. No password hash, token or
// second-factor state is ever included.
type ApplicationUser struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Active   bool   `json:"active"`
	ClientID string `json:"clientId"`
	// IdpType is INTERNAL (a platform password) or OIDC (federated).
	IdpType string `json:"idpType"`
	// Roles are the application's own roles the user holds (`app:role`).
	Roles []string `json:"roles"`
	// Invitation is PENDING or EXPIRED while the user is awaiting an
	// invitation (has not set a password); absent otherwise.
	Invitation  string         `json:"invitation,omitempty"`
	LastLoginAt *jsontime.Time `json:"lastLoginAt,omitempty"`
	CreatedAt   jsontime.Time  `json:"createdAt"`
}

// ApplicationUserList is a page of users.
type ApplicationUserList struct {
	Users    []ApplicationUser `json:"users"`
	Total    int               `json:"total"`
	Page     int               `json:"page"`
	PageSize int               `json:"pageSize"`
}

// ApplicationInvitation is one outstanding invitation. The invite link is
// never listed; it is returned only at creation or resend, to the caller that
// asked for it.
type ApplicationInvitation struct {
	UserID   string   `json:"userId"`
	Email    string   `json:"email"`
	Name     string   `json:"name"`
	ClientID string   `json:"clientId"`
	Roles    []string `json:"roles"`
	// Status is PENDING (a live link exists) or EXPIRED (it lapsed: resend).
	Status    string         `json:"status"`
	CreatedAt jsontime.Time  `json:"createdAt"`
	ExpiresAt *jsontime.Time `json:"expiresAt,omitempty"`
}

// ApplicationInvitationList is a page of invitations.
type ApplicationInvitationList struct {
	Invitations []ApplicationInvitation `json:"invitations"`
	Total       int                     `json:"total"`
	Page        int                     `json:"page"`
	PageSize    int                     `json:"pageSize"`
}

// InviteApplicationUserRequest is the body of POST /api/applications/{appCode}/users.
type InviteApplicationUserRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	// ClientID is the user's home client: a `clt_` id or the client's
	// identifier. The application must be enabled for it.
	ClientID string `json:"clientId"`
	// Roles are the application's roles to give the user (`app:role`, or the
	// short name). At least one: the roles are what make the user the
	// application's.
	Roles []string `json:"roles" minItems:"1"`
	// SendInvitation (default true) has the platform email the invitation.
	SendInvitation *bool `json:"sendInvitation,omitempty"`
	// ReturnInviteLink, when true, returns the live 72h link as inviteLink and
	// suppresses the platform email. A bearer credential: never log it.
	ReturnInviteLink *bool `json:"returnInviteLink,omitempty"`
	// InviteRedirectURI is where the invitee is sent after setting a password
	// (the application's own page; any absolute http(s) URL).
	InviteRedirectURI *string `json:"inviteRedirectUri,omitempty"`
}

// InviteApplicationUserResponse is the answer to an invitation.
type InviteApplicationUserResponse struct {
	ID string `json:"id"`
	// InviteLink is the live set-password link, only when returnInviteLink was
	// asked for.
	InviteLink *string `json:"inviteLink,omitempty"`
	// InvitationSent is true when the platform emailed the invitation. False
	// when the caller asked for the link or no email, the user is federated
	// (no password to set), or delivery failed (resend it).
	InvitationSent bool `json:"invitationSent"`
}

// ResendInvitationRequest is the body of the resend route; every field optional.
type ResendInvitationRequest struct {
	SendInvitation    *bool   `json:"sendInvitation,omitempty"`
	ReturnInviteLink  *bool   `json:"returnInviteLink,omitempty"`
	InviteRedirectURI *string `json:"inviteRedirectUri,omitempty"`
}

// ResendInvitationResponse is the answer to a resend.
type ResendInvitationResponse struct {
	// InviteLink is set only when returnInviteLink was asked for.
	InviteLink *string        `json:"inviteLink,omitempty"`
	ExpiresAt  *jsontime.Time `json:"expiresAt,omitempty"`
}

// ── access ───────────────────────────────────────────────────────────────

// notFoundApp is the one answer for every way the application in the path is
// not the caller's: it does not exist, it is another application's, or the
// credential is bound to none. Distinguishing them would let a credential probe
// which applications exist (F32; the same principle as F23).
func notFoundApp(code string) error {
	return httperror.NotFound("Application", code)
}

// access resolves the application in the path and enforces the application
// binding (the SDK syncs' rule, sdksync.requireAppAccess) and returns the
// caller's scope over its users. An unknown code, another
// application's code and an unbound credential are all the same 404
// Application_NOT_FOUND (F32), so the answer does not say which applications
// exist.
func (s *State) access(ctx context.Context, ac *auth.AuthContext, code string) (*application.Application, Scope, error) {
	app, err := s.Apps.FindByCode(ctx, code)
	if err != nil {
		return nil, Scope{}, usecase.Internal("REPO", "find_by_code failed", err)
	}
	if app == nil {
		return nil, Scope{}, notFoundApp(code)
	}
	if !ac.CanAccessApplication(string(app.ID)) {
		return nil, Scope{}, notFoundApp(code)
	}
	sc := Scope{ApplicationID: string(app.ID), ApplicationCode: app.Code}
	if !ac.IsAnchor() {
		clients := append([]string{}, ac.Clients...)
		sc.ClientIDs = &clients
	}
	return app, sc, nil
}

func pageOf(page, size int) (int, int) {
	if size <= 0 {
		size = defaultPageSize
	}
	return max(page, 0), min(size, maxPageSize)
}

func userDTO(u User) ApplicationUser {
	d := ApplicationUser{
		ID: u.ID, Email: u.Email, Name: u.Name, Active: u.Active, ClientID: u.ClientID,
		IdpType: u.IdpType, Roles: u.Roles, Invitation: u.Invitation, CreatedAt: jsontime.New(u.CreatedAt),
	}
	if d.Roles == nil {
		d.Roles = []string{}
	}
	if u.LastLoginAt != nil {
		t := jsontime.New(*u.LastLoginAt)
		d.LastLoginAt = &t
	}
	return d
}

// ── users ────────────────────────────────────────────────────────────────

type listInput struct {
	AppCode   string `path:"appCode" doc:"Application code"`
	Q         string `query:"q" doc:"Case-insensitive substring of name or email"`
	Active    string `query:"active" doc:"true | false; absent = both"`
	ClientID  string `query:"clientId" doc:"Only users homed at this client"`
	Roles     string `query:"roles" doc:"CSV of the application's roles; users holding any"`
	Page      int    `query:"page" doc:"0-based page (default 0)"`
	PageSize  int    `query:"pageSize" doc:"Page size (default 50, max 200)"`
	SortField string `query:"sortField" doc:"name | email | createdAt (default createdAt)"`
	SortOrder string `query:"sortOrder" doc:"asc | desc (default asc)"`
}

func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[ApplicationUserList], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadApplicationUsers(ac); err != nil {
		return nil, err
	}
	app, sc, err := s.access(ctx, ac, in.AppCode)
	if err != nil {
		return nil, err
	}
	page, size := pageOf(in.Page, in.PageSize)
	q := ListQuery{
		Q: strings.TrimSpace(in.Q), ClientID: strings.TrimSpace(in.ClientID),
		Sort: in.SortField, Desc: strings.EqualFold(in.SortOrder, "desc"), Page: page, PageSize: size,
	}
	switch in.Active {
	case "true":
		t := true
		q.Active = &t
	case "false":
		f := false
		q.Active = &f
	}
	// Only the application's own roles can narrow its list: a foreign role
	// name matches nothing rather than revealing who holds it.
	if wanted := splitCSV(in.Roles); len(wanted) > 0 {
		own := ownRoles(app.Code, wanted)
		if len(own) == 0 {
			return &apicommon.Out[ApplicationUserList]{Body: ApplicationUserList{Users: []ApplicationUser{}, Page: page, PageSize: size}}, nil
		}
		q.Roles = own
	}
	users, total, err := s.Repo.List(ctx, sc, q)
	if err != nil {
		return nil, usecase.Internal("REPO", "list application users failed", err)
	}
	out := make([]ApplicationUser, 0, len(users))
	for _, u := range users {
		out = append(out, userDTO(u))
	}
	return &apicommon.Out[ApplicationUserList]{Body: ApplicationUserList{Users: out, Total: total, Page: page, PageSize: size}}, nil
}

type userInput struct {
	AppCode string `path:"appCode" doc:"Application code"`
	ID      string `path:"id"`
}

func (s *State) get(ctx context.Context, in *userInput) (*apicommon.Out[ApplicationUser], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadApplicationUsers(ac); err != nil {
		return nil, err
	}
	_, sc, err := s.access(ctx, ac, in.AppCode)
	if err != nil {
		return nil, err
	}
	u, err := s.Repo.Get(ctx, sc, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "get application user failed", err)
	}
	if u == nil {
		return nil, httperror.NotFound("User", in.ID)
	}
	return &apicommon.Out[ApplicationUser]{Body: userDTO(*u)}, nil
}

type inviteInput struct {
	AppCode string `path:"appCode" doc:"Application code"`
	Body    InviteApplicationUserRequest
}

// clientRefused is the one answer for a client the application may not invite
// into: unknown, not enabled for the application, or out of the caller's reach.
const clientRefused = "client is not available to this application"

func (s *State) invite(ctx context.Context, in *inviteInput) (*apicommon.Out[InviteApplicationUserResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanInviteApplicationUsers(ac); err != nil {
		return nil, err
	}
	app, _, err := s.access(ctx, ac, in.AppCode)
	if err != nil {
		return nil, err
	}
	clientID, err := s.invitableClient(ctx, ac, app, in.Body.ClientID)
	if err != nil {
		return nil, err
	}
	// Validated before anything is written: a rejected redirect must not leave
	// behind a user whose invite was never minted.
	redirect, err := principalapi.ResolveInviteRedirect(in.Body.InviteRedirectURI)
	if err != nil {
		return nil, err
	}
	idpType := s.idpTypeFor(ctx, in.Body.Email)

	cmd := operations.InviteApplicationUserCommand{
		ApplicationCode: app.Code, Email: in.Body.Email, Name: in.Body.Name,
		ClientID: clientID, Roles: in.Body.Roles, IDPType: &idpType,
	}
	ev, err := usecaseop.Run(ctx, s.UoW, operations.InviteApplicationUser(s.Principals, s.Roles), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, err
	}
	resp := InviteApplicationUserResponse{ID: string(ev.UserID)}
	if idpType == "OIDC" || s.Invites == nil {
		return &apicommon.Out[InviteApplicationUserResponse]{Body: resp}, nil
	}
	p, err := s.Principals.FindByID(ctx, ev.UserID)
	if err != nil || p == nil {
		slog.WarnContext(ctx, "invited user not reloaded; invitation not delivered", "principal", ev.UserID, "err", err)
		return &apicommon.Out[InviteApplicationUserResponse]{Body: resp}, nil
	}
	link, sent := s.deliver(ctx, p, boolDefaultTrue(in.Body.SendInvitation), boolOrFalse(in.Body.ReturnInviteLink), redirect)
	resp.InviteLink, resp.InvitationSent = link, sent
	return &apicommon.Out[InviteApplicationUserResponse]{Body: resp}, nil
}

// deliver mints the invitation: returnInviteLink gives the link back (and
// suppresses the email: the token can only be minted once per delivery),
// otherwise sendInvitation emails it.
func (s *State) deliver(ctx context.Context, p *principal.Principal, send, returnLink bool, redirect *string) (link *string, sent bool) {
	switch {
	case returnLink:
		l, err := s.Invites.InviteLink(ctx, p, redirect)
		if err != nil {
			slog.WarnContext(ctx, "mint invite link failed", "principal", p.ID, "err", err)
			return nil, false
		}
		return &l, false
	case send:
		if err := s.Invites.SendInviteRedirect(ctx, p, redirect); err != nil {
			slog.WarnContext(ctx, "send application invite failed", "principal", p.ID, "err", err)
			return nil, false
		}
		return nil, true
	}
	return nil, false
}

// invitableClient resolves the request's client reference (`clt_` id or
// identifier) to an id, and requires the application to be enabled for it and
// the caller to reach it. Every failure is the same 403.
func (s *State) invitableClient(ctx context.Context, ac *auth.AuthContext, app *application.Application, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", httperror.BadRequest("CLIENT_REQUIRED", "clientId is required")
	}
	c, err := s.Clients.FindByID(ctx, ref)
	if err != nil {
		return "", usecase.Internal("REPO", "client lookup failed", err)
	}
	if c == nil {
		if c, err = s.Clients.FindByIdentifier(ctx, strings.ToLower(ref)); err != nil {
			return "", usecase.Internal("REPO", "client lookup failed", err)
		}
	}
	if c == nil || !ac.CanAccessClient(string(c.ID)) {
		return "", httperror.Forbidden(clientRefused)
	}
	cfg, err := s.ClientConfigs.FindByApplicationAndClient(ctx, string(app.ID), string(c.ID))
	if err != nil {
		return "", usecase.Internal("REPO", "client config lookup failed", err)
	}
	if cfg == nil || !cfg.Enabled {
		return "", httperror.Forbidden(clientRefused)
	}
	return string(c.ID), nil
}

// idpTypeFor answers OIDC when the email's domain is mapped to an OIDC
// identity provider, INTERNAL otherwise (as create-user does).
func (s *State) idpTypeFor(ctx context.Context, email string) string {
	at := strings.LastIndexByte(email, '@')
	if at < 0 || s.Mappings == nil || s.IdentityProviders == nil {
		return "INTERNAL"
	}
	m, err := s.Mappings.FindByEmailDomain(ctx, strings.ToLower(strings.TrimSpace(email[at+1:])))
	if err != nil || m == nil {
		return "INTERNAL"
	}
	if idp, _ := s.IdentityProviders.FindByID(ctx, m.IdentityProviderID); idp != nil && string(idp.Type) == "OIDC" {
		return "OIDC"
	}
	return "INTERNAL"
}

// ── invitations ──────────────────────────────────────────────────────────

type invitationsInput struct {
	AppCode  string `path:"appCode" doc:"Application code"`
	Status   string `query:"status" doc:"PENDING | EXPIRED; absent = both"`
	Page     int    `query:"page" doc:"0-based page (default 0)"`
	PageSize int    `query:"pageSize" doc:"Page size (default 50, max 200)"`
}

func (s *State) invitations(ctx context.Context, in *invitationsInput) (*apicommon.Out[ApplicationInvitationList], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadApplicationInvitations(ac); err != nil {
		return nil, err
	}
	_, sc, err := s.access(ctx, ac, in.AppCode)
	if err != nil {
		return nil, err
	}
	status := strings.ToUpper(strings.TrimSpace(in.Status))
	if status != "" && status != "PENDING" && status != "EXPIRED" {
		return nil, httperror.BadRequest("INVALID_STATUS", "status must be PENDING or EXPIRED")
	}
	page, size := pageOf(in.Page, in.PageSize)
	rows, total, err := s.Repo.Invitations(ctx, sc, status, page, size)
	if err != nil {
		return nil, usecase.Internal("REPO", "list application invitations failed", err)
	}
	out := make([]ApplicationInvitation, 0, len(rows))
	for _, i := range rows {
		out = append(out, invitationDTO(i))
	}
	return &apicommon.Out[ApplicationInvitationList]{Body: ApplicationInvitationList{Invitations: out, Total: total, Page: page, PageSize: size}}, nil
}

func invitationDTO(i Invitation) ApplicationInvitation {
	d := ApplicationInvitation{
		UserID: i.PrincipalID, Email: i.Email, Name: i.Name, ClientID: i.ClientID,
		Roles: i.Roles, Status: i.Status, CreatedAt: jsontime.New(i.CreatedAt),
	}
	if d.Roles == nil {
		d.Roles = []string{}
	}
	if i.ExpiresAt != nil {
		t := jsontime.New(*i.ExpiresAt)
		d.ExpiresAt = &t
	}
	return d
}

type resendInput struct {
	AppCode string `path:"appCode" doc:"Application code"`
	ID      string `path:"id" doc:"The invited user's id"`
	Body    ResendInvitationRequest
}

func (s *State) resend(ctx context.Context, in *resendInput) (*apicommon.Out[ResendInvitationResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanManageApplicationInvitations(ac); err != nil {
		return nil, err
	}
	_, sc, err := s.access(ctx, ac, in.AppCode)
	if err != nil {
		return nil, err
	}
	inv, err := s.Repo.GetInvitation(ctx, sc, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "get application invitation failed", err)
	}
	if inv == nil {
		return nil, httperror.NotFound("Invitation", in.ID)
	}
	redirect, err := principalapi.ResolveInviteRedirect(in.Body.InviteRedirectURI)
	if err != nil {
		return nil, err
	}
	send, returnLink := boolDefaultTrue(in.Body.SendInvitation), boolOrFalse(in.Body.ReturnInviteLink)
	if !send && !returnLink {
		return nil, httperror.BadRequest("INVITATION_DELIVERY_REQUIRED",
			"a resend needs sendInvitation or returnInviteLink")
	}
	if s.Invites == nil {
		return nil, usecase.Internal("INVITES_NOT_WIRED", "invitations are not available on this server", nil)
	}
	p, err := s.Principals.FindByID(ctx, ids.PrincipalID(in.ID))
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Invitation", in.ID)
	}
	link, sent := s.deliver(ctx, p, send, returnLink, redirect)
	// A resend that delivered nothing is an error, never a silent success: the
	// administrator would believe the invitee has a fresh link.
	if link == nil && !sent {
		return nil, usecase.Internal("INVITE_DELIVERY_FAILED", "the invitation could not be delivered; try again", nil)
	}
	resp := ResendInvitationResponse{InviteLink: link}
	if fresh, ferr := s.Repo.GetInvitation(ctx, sc, in.ID); ferr == nil && fresh != nil && fresh.ExpiresAt != nil {
		t := jsontime.New(*fresh.ExpiresAt)
		resp.ExpiresAt = &t
	}
	return &apicommon.Out[ResendInvitationResponse]{Body: resp}, nil
}

func (s *State) revoke(ctx context.Context, in *userInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanManageApplicationInvitations(ac); err != nil {
		return nil, err
	}
	_, sc, err := s.access(ctx, ac, in.AppCode)
	if err != nil {
		return nil, err
	}
	inv, err := s.Repo.GetInvitation(ctx, sc, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "get application invitation failed", err)
	}
	if inv == nil {
		return nil, httperror.NotFound("Invitation", in.ID)
	}
	// Deactivate first: a passwordless active user can ask for a fresh
	// set-password link at the login page, so deleting the link alone would not
	// revoke anything. A retry after a partial failure finds the user
	// deactivated (no longer an invitee) and the link, if it survived, expired
	// by its own TTL.
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeactivateUser(s.Principals),
		operations.DeactivateCommand{ID: ids.PrincipalID(in.ID)}, auth.NewExecutionContext(ctx)); err != nil {
		return nil, err
	}
	if _, err := s.Repo.RevokeInvites(ctx, in.ID); err != nil {
		return nil, usecase.Internal("REPO", "revoke invites failed", err)
	}
	return &apicommon.Empty{}, nil
}

// ── helpers ──────────────────────────────────────────────────────────────

func splitCSV(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if t := strings.ToLower(strings.TrimSpace(part)); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// ownRoles keeps the role names that belong to the application, prefixing a
// short name with the application code.
func ownRoles(appCode string, names []string) []string {
	prefix := strings.ToLower(appCode) + ":"
	out := make([]string, 0, len(names))
	for _, n := range names {
		if !strings.Contains(n, ":") {
			n = prefix + n
		}
		if strings.HasPrefix(n, prefix) {
			out = append(out, n)
		}
	}
	return out
}

func boolDefaultTrue(b *bool) bool { return b == nil || *b }
func boolOrFalse(b *bool) bool     { return b != nil && *b }
