package api

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"slices"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

func (s *State) create(ctx context.Context, in *apicommon.In[CreatePrincipalRequest]) (*apicommon.Out[CreatePrincipalResponse], error) {
	ac := auth.FromContext(ctx)
	// Anchors create any scope/client. A non-anchor administrator
	// (client-admin) may only create CLIENT-scope users in a client they can
	// access — never ANCHOR/PARTNER users, and never a clientless principal.
	if !ac.IsAnchor() && in.Body.Scope != "CLIENT" {
		return nil, httperror.Forbidden("Client administrators can only create client-scope users")
	}
	cmd := in.Body.toCommand()
	clientID := in.Body.ClientID
	// ANCHOR/PARTNER creation gets exactly the checks POST /api/principals/users
	// applies: user:manage-access, and the email domain must back the scope.
	// The home client and identity-provider type then come from that same
	// derivation, never from the caller's body.
	if reqScope := strings.ToUpper(strings.TrimSpace(in.Body.Scope)); reqScope != "CLIENT" && reqScope != "" {
		// Who may do this is answered before anything about the domain is.
		if perr := auth.CanCreatePrincipals(ac); perr != nil {
			return nil, perr
		}
		if perr := auth.CanManagePrincipalAccess(ac); perr != nil {
			return nil, perr
		}
		_, isAnchorDomain, mapping, idpType, ferr := s.userDomainFacts(ctx, in.Body.Email)
		if ferr != nil {
			return nil, ferr
		}
		scope, derivedClient, serr := s.resolveUserScope(ctx, ac, &in.Body.Scope, in.Body.ClientID, isAnchorDomain, mapping)
		if serr != nil {
			return nil, serr
		}
		cmd.Scope, cmd.ClientID, cmd.IDPType = scope, derivedClient, &idpType
		clientID = derivedClient
	}
	if err := auth.RequireUserAdmin(ac, clientID, auth.CanCreatePrincipals); err != nil {
		return nil, err
	}
	inviteRedirect, err := resolveInviteRedirect(in.Body.InviteRedirectURI)
	if err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateUser(s.Repo), cmd, ec)
	if err != nil {
		return nil, err
	}
	resp := CreatePrincipalResponse{ID: string(event.UserID)}
	if created, ferr := s.Repo.FindByID(ctx, event.UserID); ferr == nil && created != nil {
		resp.InviteLink = s.notifyNewUser(ctx, created, in.Body.Password,
			boolDefaultTrue(in.Body.SendInvitation), boolOrFalse(in.Body.ReturnInviteLink), inviteRedirect)
	}
	return &apicommon.Out[CreatePrincipalResponse]{Body: resp}, nil
}

// bulkImport onboards a list of CLIENT users under one client (CSV import).
// Each missing user is created (passwordless → invite email) with its listed
// roles; existing users are skipped. Roles are validated against the client's
// applications for a non-anchor administrator — exactly like single role
// assignment — so a client-admin can't grant a role the client isn't entitled to.
func (s *State) bulkImport(ctx context.Context, in *apicommon.In[BulkImportRequest]) (*apicommon.Out[BulkImportResponse], error) {
	ac := auth.FromContext(ctx)
	clientID := strings.TrimSpace(in.Body.ClientID)
	if clientID == "" {
		return nil, httperror.BadRequest("CLIENT_REQUIRED", "A target client is required")
	}
	// Non-anchor administrators create only CLIENT-scope users in a client they
	// can access; RequireUserAdmin enforces both.
	if err := auth.RequireUserAdmin(ac, &clientID, auth.CanCreatePrincipals); err != nil {
		return nil, err
	}
	if len(in.Body.Users) == 0 {
		return nil, httperror.BadRequest("NO_ROWS", "No users to import")
	}
	if len(in.Body.Users) > 1000 {
		return nil, httperror.BadRequest("TOO_MANY", "Import is limited to 1000 users at a time")
	}

	ec := usecase.NewExecutionContext(ac.PrincipalID)
	out := BulkImportResponse{Results: make([]BulkImportResult, 0, len(in.Body.Users))}
	seen := make(map[string]struct{}, len(in.Body.Users))

	for i, u := range in.Body.Users {
		email := strings.ToLower(strings.TrimSpace(u.Email))
		status, msg := s.importRow(ctx, ac, ec, clientID, strings.TrimSpace(u.Name), email, cleanRoles(u.Roles), seen)
		switch status {
		case "created":
			out.Created++
		case "exists", "dropped":
			// Both are intentional skips (already-present, or a domain owned by
			// another client) — surfaced per-row so the caller can report them.
			out.Skipped++
		default:
			out.Failed++
		}
		out.Results = append(out.Results, BulkImportResult{Row: i + 1, Email: email, Status: status, Message: msg})
	}
	return &apicommon.Out[BulkImportResponse]{Body: out}, nil
}

// importRow processes one CSV row and returns its outcome status
// ("created" | "exists" | "error") plus a human message.
func (s *State) importRow(ctx context.Context, ac *auth.AuthContext, ec usecase.ExecutionContext, clientID, name, email string, roles []string, seen map[string]struct{}) (string, string) {
	if email == "" || !strings.Contains(email, "@") {
		return "error", "invalid email address"
	}
	if name == "" {
		return "error", "name is required"
	}
	if _, dup := seen[email]; dup {
		return "error", "duplicate email in file"
	}
	seen[email] = struct{}{}

	// Validate the row's roles are available to the client (non-anchor admins).
	if !ac.IsAnchor() {
		allowed, err := s.clientAppIDs(ctx, clientID)
		if err != nil {
			return "error", errMessage(err)
		}
		if err := s.assertAssignableRoles(ctx, roles, allowed); err != nil {
			return "error", errMessage(err)
		}
	}
	// Only create missing users; an existing email is skipped untouched.
	existing, err := s.Repo.FindByEmail(ctx, email)
	if err != nil {
		return "error", "lookup failed"
	}
	if existing != nil {
		return "exists", "already exists — skipped"
	}
	// Drop a row whose email domain is registered to a DIFFERENT client. An
	// admin importing into client A must never pull in a user from a domain
	// owned by client B (matters most for platform admins importing across
	// clients, but the guard is unconditional). A domain with no client owner
	// (unmapped, or an anchor-scope mapping) is allowed.
	if s.Mappings != nil {
		domain := email[strings.IndexByte(email, '@')+1:]
		mapping, derr := s.Mappings.FindByEmailDomain(ctx, domain)
		if derr != nil {
			return "error", "domain lookup failed"
		}
		if owners := domainOwnerClientIDs(mapping); len(owners) > 0 && !slices.Contains(owners, clientID) {
			return "dropped", "email domain is registered to another client — skipped"
		}
	}
	// Create the CLIENT user with no password → notifyNewUser sends the invite
	// ("set your password") so they can onboard.
	cid := clientID
	uname := name
	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateUser(s.Repo), operations.CreateCommand{
		Email: email, Name: &uname, Scope: "CLIENT", ClientID: &cid,
	}, ec)
	if err != nil {
		return "error", errMessage(err)
	}
	userID := event.UserID
	// The row's roles go through AssignRoles, which needs user:assign-roles: a
	// caller holding only user:create gets the user created and the row
	// reported "roles not applied" (a row without roles needs no more).
	if len(roles) > 0 {
		if _, err := usecaseop.Run(ctx, s.UoW, operations.AssignRoles(s.Repo, s.Roles),
			operations.AssignRolesCommand{UserID: userID, Roles: roles}, ec); err != nil {
			return "created", "created, but roles not applied: " + errMessage(err)
		}
	}
	if created, gerr := s.Repo.FindByID(ctx, userID); gerr == nil && created != nil {
		// Bulk import never gains the sendInvitation/returnInviteLink flags —
		// always the original passwordless-invite behaviour.
		s.notifyNewUser(ctx, created, nil, true, false, nil)
	}
	return "created", ""
}

// domainOwnerClientIDs returns the clients an email-domain mapping is owned by
// (its primary plus any additional clients). An anchor-scope or unmapped domain
// has no owners, so it never blocks an import. GrantedClientIDs are deliberately
// excluded — a grant is access, not ownership.
func domainOwnerClientIDs(m *emaildomainmapping.EmailDomainMapping) []string {
	if m == nil {
		return nil
	}
	owners := make([]string, 0, 1+len(m.AdditionalClientIDs))
	if m.PrimaryClientID != nil && *m.PrimaryClientID != "" {
		owners = append(owners, string(*m.PrimaryClientID))
	}
	owners = append(owners, m.AdditionalClientIDs...)
	return owners
}

// cleanRoles trims, drops blanks, and dedupes the role names from a CSV cell.
func cleanRoles(roles []string) []string {
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		if t := strings.TrimSpace(r); t != "" {
			out = append(out, t)
		}
	}
	return dedupeStrings(out)
}

// errMessage extracts a user-facing message from a usecase error (or falls back
// to the raw error text).
func errMessage(err error) string {
	if ue, ok := errors.AsType[*usecase.Error](err); ok {
		return ue.Message
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

// createUser is the SDK create-user endpoint: anchor-only,
// derives scope + client association from the email domain (anchor-domain check
// + email-domain-mapping), then delegates to the shared CreateUser operation.
// Returns the full principal (the SDK reads it back). The SDK's
// enforcePasswordComplexity is accepted but not enforced (Go's create doesn't
// apply a complexity policy). Magic-link-on-passwordless-create is intentionally
// not implemented — the SDK always supplies a password and the reset emailer
// isn't wired.
func (s *State) createUser(ctx context.Context, in *apicommon.In[CreateUserRequest]) (*apicommon.Out[PrincipalResponse], error) {
	ac := auth.FromContext(ctx)
	// Authorization is enforced after scope derivation below (a client-admin may
	// only create CLIENT-scope users in a client they can access).
	email, isAnchorDomain, mapping, idpType, err := s.userDomainFacts(ctx, in.Body.Email)
	if err != nil {
		return nil, err
	}

	// Client reference resolution, scope derivation and the non-CLIENT
	// authorisation rules are shared with the generic create route.
	scope, clientID, err := s.resolveUserScope(ctx, ac, in.Body.Scope, in.Body.ClientID, isAnchorDomain, mapping)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireUserAdmin(ac, clientID, auth.CanCreatePrincipals); err != nil {
		return nil, err
	}
	// Validated before anything is written: a rejected redirect must not
	// leave behind a user whose invite was never minted.
	inviteRedirect, err := resolveInviteRedirect(in.Body.InviteRedirectURI)
	if err != nil {
		return nil, err
	}

	ec := auth.NewExecutionContext(ctx)

	// Partner-merge: when a PARTNER user already exists for this email, grant
	// access to the requested client rather than recreating (keeps events +
	// audit accurate).
	if scope == "PARTNER" {
		existing, ferr := s.Repo.FindByEmail(ctx, email)
		if ferr != nil {
			return nil, usecase.Internal("REPO", "find_by_email failed", ferr)
		}
		if existing != nil {
			if existing.ClientID != nil && clientID != nil && string(*existing.ClientID) == *clientID {
				return nil, usecase.Conflict("EMAIL_EXISTS", "User with email '"+email+"' already exists")
			}
			if _, gerr := usecaseop.Run(ctx, s.UoW, operations.GrantClientAccess(s.Repo, s.Clients, s.GrantRepo),
				operations.GrantClientAccessCommand{UserID: existing.ID, ClientID: derefStr(clientID)}, ec); gerr != nil {
				return nil, gerr
			}
			refreshed, rerr := s.Repo.FindByID(ctx, existing.ID)
			if rerr != nil || refreshed == nil {
				return nil, httperror.NotFound("Principal", string(existing.ID))
			}
			return &apicommon.Out[PrincipalResponse]{Body: fromEntity(refreshed)}, nil
		}
	}

	name := in.Body.Name
	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateUser(s.Repo), operations.CreateCommand{
		Email:    email,
		Name:     &name,
		Scope:    scope,
		ClientID: clientID,
		Password: in.Body.Password,
		IDPType:  &idpType,
	}, ec)
	if err != nil {
		return nil, err
	}

	// New PARTNER user: grant the requested client.
	if scope == "PARTNER" && clientID != nil {
		if _, gerr := usecaseop.Run(ctx, s.UoW, operations.GrantClientAccess(s.Repo, s.Clients, s.GrantRepo),
			operations.GrantClientAccessCommand{UserID: event.UserID, ClientID: *clientID}, ec); gerr != nil {
			return nil, gerr
		}
	}

	created, err := s.Repo.FindByID(ctx, event.UserID)
	if err != nil || created == nil {
		return nil, httperror.NotFound("Principal", string(event.UserID))
	}
	inviteLink := s.notifyNewUser(ctx, created, in.Body.Password,
		boolDefaultTrue(in.Body.SendInvitation), boolOrFalse(in.Body.ReturnInviteLink), inviteRedirect)
	resp := fromEntity(created)
	resp.InviteLink = inviteLink
	return &apicommon.Out[PrincipalResponse]{Body: resp}, nil
}

// notifyNewUser emails a freshly-created INTERNAL user (best-effort): a
// passwordless account gets a "set your password" invite (which carries the
// invite token + the 2FA enrollment hand-off); an account created WITH a
// password gets a plain "account created" welcome (2FA, if required, is then
// enforced at first sign-in). Federated/OIDC users get nothing.
//
// sendInvitation and returnInviteLink implement CreateUserRequest's flags
// (see its doc comments for the full semantics). Precedence:
//   - returnInviteLink wins whenever it applies (passwordless INTERNAL user,
//     InviteEmailer configured): the link is minted and returned, and the
//     platform's OWN invite email is suppressed regardless of sendInvitation
//     — a second mint to also email it would invalidate the link just
//     returned (each mint invalidates the previous outstanding token).
//   - Otherwise, sendInvitation:false suppresses ALL platform email (neither
//     invite nor welcome) — the caller has taken over notification.
//   - Otherwise, behaviour is unchanged from before these flags existed.
//
// inviteRedirect (validated by resolveInviteRedirect, may be nil) rides on
// whichever invite token is minted, so both delivery modes honour it.
//
// Returns the invite link when one was minted-and-returned, else nil.
func (s *State) notifyNewUser(ctx context.Context, p *principal.Principal, password *string, sendInvitation, returnInviteLink bool, inviteRedirect *string) *string {
	if p == nil || p.UserIdentity == nil {
		return nil // service account
	}
	// Skip federated/OIDC users — they manage credentials at their IdP. NB:
	// the repo loads UserIdentity.Provider from idp_type for ALL users (so it's
	// "INTERNAL" for internal users, not nil) — federated must be detected via
	// ExternalIdentity, or Provider=="OIDC" for an OIDC user created before its
	// first callback populates ExternalIdentity.
	if p.ExternalIdentity != nil ||
		(p.UserIdentity.Provider != nil && *p.UserIdentity.Provider == "OIDC") {
		return nil
	}
	emailAddr := strings.TrimSpace(p.UserIdentity.Email)
	if emailAddr == "" {
		return nil
	}
	passwordless := password == nil || *password == ""

	if returnInviteLink && passwordless && s.InviteEmailer != nil {
		link, err := s.InviteEmailer.InviteLink(ctx, p, inviteRedirect)
		if err != nil {
			slog.Warn("mint invite link failed", "principal", p.ID, "err", err)
			return nil
		}
		return &link
	}

	if !sendInvitation {
		slog.Info("invite suppressed by caller", "principal", p.ID)
		return nil
	}

	if passwordless && s.InviteEmailer != nil {
		if err := s.InviteEmailer.SendInviteRedirect(ctx, p, inviteRedirect); err != nil {
			slog.Warn("send account invite failed", "principal", p.ID, "err", err)
		}
		return nil
	}
	s.Notifier.AccountCreated(ctx, emailAddr)
	return nil
}

// ResolveInviteRedirect exports resolveInviteRedirect for the application
// invitation routes (F20), which take the same inviteRedirectUri.
func ResolveInviteRedirect(raw *string) (*string, error) { return resolveInviteRedirect(raw) }

// resolveInviteRedirect validates create-user's inviteRedirectUri: where the
// calling application wants the invitee sent after set-password — usually its
// own home page, which then starts its own sign-in. Absent or blank is no
// redirect. Otherwise it must be an absolute http(s) URL with a host and no
// userinfo (so no javascript:/data: URLs and no "https://trusted@evil"
// host confusion). It is deliberately NOT tied to OAuth redirect URIs: it is
// set by an authenticated caller already allowed to create the user, and it
// is stored on the invite token rather than read from the link, so the
// set-password page is not an open redirect.
func resolveInviteRedirect(raw *string) (*string, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	uri := strings.TrimSpace(*raw)
	u, err := url.Parse(uri)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return nil, usecase.Validation("INVITE_REDIRECT_URI_INVALID",
			"inviteRedirectUri must be an absolute http or https URL")
	}
	return &uri, nil
}

// userDomainFacts normalises an email and looks up what the platform knows
// about its domain: whether it is a registered anchor domain, its email-domain
// mapping, and the identity-provider type that mapping implies (INTERNAL for
// an unmapped domain). Shared by both user-create routes so scope validation
// cannot drift between them.
func (s *State) userDomainFacts(ctx context.Context, rawEmail string) (email string, isAnchorDomain bool, mapping *emaildomainmapping.EmailDomainMapping, idpType string, err error) {
	email = strings.ToLower(strings.TrimSpace(rawEmail))
	at := strings.IndexByte(email, '@')
	if at < 0 || at == len(email)-1 {
		return "", false, nil, "", httperror.BadRequest("INVALID_EMAIL", "Invalid email format")
	}
	domain := email[at+1:]

	if s.AnchorDomains != nil {
		ad, ferr := s.AnchorDomains.FindByDomain(ctx, domain)
		if ferr != nil {
			return "", false, nil, "", usecase.Internal("REPO", "anchor_domain lookup failed", ferr)
		}
		isAnchorDomain = ad != nil
	}
	if s.Mappings != nil {
		m, ferr := s.Mappings.FindByEmailDomain(ctx, domain)
		if ferr != nil {
			return "", false, nil, "", usecase.Internal("REPO", "email_domain_mapping lookup failed", ferr)
		}
		mapping = m
	}

	// Resolve IdP type (INTERNAL / OIDC). Unmapped domains default to INTERNAL.
	idpType = "INTERNAL"
	if mapping != nil && s.IdentityProviders != nil {
		if idp, _ := s.IdentityProviders.FindByID(ctx, mapping.IdentityProviderID); idp != nil {
			idpType = string(idp.Type)
		}
	}
	return email, isAnchorDomain, mapping, idpType, nil
}

// resolveUserScope resolves the (scope, home client) for a create from the
// requested scope/clientId and the email's domain facts, then applies the
// authorisation rules common to both create routes: a non-anchor administrator
// only creates CLIENT-scope users, and a PARTNER or ANCHOR user (a change of
// client reach) also needs user:manage-access (owner ruling 2026-10-25).
func (s *State) resolveUserScope(ctx context.Context, ac *auth.AuthContext, reqScope, reqClient *string, isAnchorDomain bool, mapping *emaildomainmapping.EmailDomainMapping) (string, *string, error) {
	var reqClientID *string
	if reqClient != nil && strings.TrimSpace(*reqClient) != "" {
		resolved, rerr := s.resolveClientRef(ctx, strings.TrimSpace(*reqClient))
		if rerr != nil {
			return "", nil, rerr
		}
		reqClientID = resolved
	}
	scope, clientID, err := deriveUserScope(reqScope, isAnchorDomain, mapping, reqClientID)
	if err != nil {
		return "", nil, err
	}
	if !ac.IsAnchor() && scope != "CLIENT" {
		return "", nil, httperror.Forbidden("Client administrators can only create client-scope users")
	}
	if scope != "CLIENT" {
		if err := auth.CanManagePrincipalAccess(ac); err != nil {
			return "", nil, err
		}
	}
	return scope, clientID, nil
}

// deriveUserScope resolves (scope, home-client) for create-user. The
// caller's requested scope wins — the email domain can only CONFIRM a
// privileged scope, never grant one unasked:
//   - requested ANCHOR requires the domain to be a registered anchor domain
//     (or an ANCHOR mapping); anchors carry no client.
//   - requested PARTNER requires a PARTNER mapping for the domain and a
//     clientId that mapping allows.
//   - requested CLIENT — or nothing, CLIENT is the default — uses the
//     request's clientId, falling back to a CLIENT mapping's primary.
//
// Pure + unit-tested.
func deriveUserScope(reqScope *string, isAnchorDomain bool, mapping *emaildomainmapping.EmailDomainMapping, reqClientID *string) (string, *string, error) {
	scope := "CLIENT"
	if reqScope != nil && strings.TrimSpace(*reqScope) != "" {
		scope = strings.ToUpper(strings.TrimSpace(*reqScope))
	}
	switch scope {
	case "ANCHOR":
		anchorMapped := mapping != nil && mapping.ScopeType == emaildomainmapping.ScopeAnchor
		if !isAnchorDomain && !anchorMapped {
			return "", nil, usecase.Validation("ANCHOR_DOMAIN_REQUIRED",
				"ANCHOR scope requires the email's domain to be a registered anchor domain")
		}
		return "ANCHOR", nil, nil
	case "PARTNER":
		if mapping == nil || mapping.ScopeType != emaildomainmapping.ScopePartner {
			return "", nil, usecase.Validation("PARTNER_DOMAIN_REQUIRED",
				"PARTNER scope requires a PARTNER email-domain mapping for the email's domain")
		}
		if reqClientID == nil || *reqClientID == "" {
			return "", nil, usecase.Validation("CLIENT_REQUIRED", "clientId is required for partner users")
		}
		allowed := (mapping.PrimaryClientID != nil && string(*mapping.PrimaryClientID) == *reqClientID)
		if slices.Contains(mapping.GrantedClientIDs, *reqClientID) {
			allowed = true
		}
		if !allowed {
			return "", nil, usecase.Validation("CLIENT_NOT_ALLOWED",
				"clientId "+*reqClientID+" is not allowed for partner domain "+mapping.EmailDomain)
		}
		return "PARTNER", reqClientID, nil
	case "CLIENT":
		clientID := reqClientID
		if clientID == nil && mapping != nil && mapping.ScopeType == emaildomainmapping.ScopeClient {
			clientID = ids.StringPtr(mapping.PrimaryClientID)
		}
		return "CLIENT", clientID, nil
	default:
		return "", nil, usecase.Validation("INVALID_SCOPE", "scope must be ANCHOR, PARTNER, or CLIENT")
	}
}

// resolveClientRef canonicalises a request's client reference — either the
// clt_ id or the client's identifier slug — to the client id. Unknown
// references fail closed (a typo'd identifier must not silently create a
// mis-scoped user).
func (s *State) resolveClientRef(ctx context.Context, ref string) (*string, error) {
	if s.Clients == nil {
		return &ref, nil // handler tests without a client repo: pass through
	}
	id, err := s.Clients.ResolveRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// boolDefaultTrue reads an optional flag whose absence means true (e.g.
// CreateUserRequest.SendInvitation).
func boolDefaultTrue(b *bool) bool {
	return b == nil || *b
}

// boolOrFalse reads an optional flag whose absence means false (e.g.
// CreateUserRequest.ReturnInviteLink).
func boolOrFalse(b *bool) bool {
	return b != nil && *b
}

// ── check-email-domain (admin) ───────────────────────────────────────────

type checkEmailDomainInput struct {
	Email string `query:"email"`
}

func (s *State) checkEmailDomain(ctx context.Context, in *checkEmailDomainInput) (*apicommon.Out[CheckEmailDomainResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadPrincipals(ac); err != nil {
		return nil, err
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	if email == "" {
		return nil, httperror.BadRequest("EMAIL_REQUIRED", "email query param is required")
	}
	at := strings.IndexByte(email, '@')
	if at < 0 || at == len(email)-1 {
		return nil, httperror.BadRequest("INVALID_EMAIL", "Invalid email format")
	}
	domain := email[at+1:]

	// Does a principal already exist for this exact address? Drives the SPA's
	// "email already exists" guard so the create form can't double-create.
	emailExists := false
	if s.Repo != nil {
		existing, err := s.Repo.FindByEmail(ctx, email)
		if err != nil {
			return nil, usecase.Internal("REPO", "find_by_email failed", err)
		}
		emailExists = existing != nil
	}

	// Resolve the domain → scope the domain setup suggests: an anchor domain
	// → ANCHOR; otherwise the email-domain-mapping's scope (or CLIENT when
	// unmapped). createUser no longer derives scope itself — the SPA echoes
	// this DerivedScope back as the request's explicit `scope`.
	isAnchorDomain := false
	if s.AnchorDomains != nil {
		ad, err := s.AnchorDomains.FindByDomain(ctx, domain)
		if err != nil {
			return nil, usecase.Internal("REPO", "anchor_domain lookup failed", err)
		}
		isAnchorDomain = ad != nil
	}

	var mapping *emaildomainmapping.EmailDomainMapping
	if s.Mappings != nil {
		m, err := s.Mappings.FindByEmailDomain(ctx, domain)
		if err != nil {
			return nil, usecase.Internal("REPO", "email_domain_mapping lookup failed", err)
		}
		mapping = m
	}

	// IdP type (INTERNAL / OIDC / SAML). Unmapped domains and missing IDPs
	// default to INTERNAL — the SPA shows a password field for those.
	idpType := "INTERNAL"
	idpIssuer := ""
	if mapping != nil && s.IdentityProviders != nil {
		if idp, _ := s.IdentityProviders.FindByID(ctx, mapping.IdentityProviderID); idp != nil {
			idpType = string(idp.Type)
			if idp.OIDCIssuerURL != nil {
				idpIssuer = *idp.OIDCIssuerURL
			}
		}
	}
	external := idpType == string(identityprovider.TypeOIDC)

	scope := deriveScopeForDomain(isAnchorDomain, mapping)
	resp := CheckEmailDomainResponse{
		AuthMethod:       "internal",
		Domain:           domain,
		AuthProvider:     idpType,
		IsAnchorDomain:   isAnchorDomain,
		HasIDPConfig:     external,
		EmailExists:      emailExists,
		DerivedScope:     scope,
		RequiresClientID: scope != "ANCHOR",
		AllowedClientIDs: allowedClientIDsForDomain(mapping),
	}
	if external {
		resp.AuthMethod = "external"
		resp.LoginURL = "/auth/oidc/login?domain=" + url.QueryEscape(domain)
		resp.IDPIssuer = idpIssuer
	}
	if emailExists {
		w := "A user with this email address already exists."
		resp.Warning = &w
	}
	return &apicommon.Out[CheckEmailDomainResponse]{Body: resp}, nil
}

// deriveScopeForDomain reports the scope a new user from this domain will get,
// independent of any chosen clientId (the check runs before the client is
// picked). Mirrors the scope branch of [deriveUserScope]: an anchor domain or
// an ANCHOR mapping → ANCHOR; a PARTNER mapping → PARTNER; a CLIENT mapping or
// an unmapped domain → CLIENT.
func deriveScopeForDomain(isAnchorDomain bool, mapping *emaildomainmapping.EmailDomainMapping) string {
	if isAnchorDomain {
		return "ANCHOR"
	}
	if mapping == nil {
		return "CLIENT"
	}
	switch mapping.ScopeType {
	case emaildomainmapping.ScopeAnchor:
		return "ANCHOR"
	case emaildomainmapping.ScopePartner:
		return "PARTNER"
	case emaildomainmapping.ScopeClient:
		return "CLIENT"
	default:
		return "ANCHOR"
	}
}

// allowedClientIDsForDomain returns the client IDs the create-user picker
// should be constrained to, or an empty slice when the domain imposes no
// restriction (the form then shows the full active-clients list). A PARTNER
// mapping allows the primary plus any granted clients; a CLIENT mapping allows
// just the primary (when set). Always non-nil so the field serializes as [].
func allowedClientIDsForDomain(mapping *emaildomainmapping.EmailDomainMapping) []string {
	out := []string{}
	if mapping == nil {
		return out
	}
	// ANCHOR mappings restrict nothing: no client ids
	//exhaustive:ignore
	switch mapping.ScopeType {
	case emaildomainmapping.ScopePartner:
		seen := map[string]struct{}{}
		add := func(id string) {
			if id == "" {
				return
			}
			if _, ok := seen[id]; ok {
				return
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
		if mapping.PrimaryClientID != nil {
			add(string(*mapping.PrimaryClientID))
		}
		for _, c := range mapping.GrantedClientIDs {
			add(c)
		}
	case emaildomainmapping.ScopeClient:
		if mapping.PrimaryClientID != nil && *mapping.PrimaryClientID != "" {
			out = append(out, string(*mapping.PrimaryClientID))
		}
	}
	return out
}
