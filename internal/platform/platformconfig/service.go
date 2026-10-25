package platformconfig

import (
	"context"
	"encoding/json"
	"maps"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// AppLookup is the slice of the application repository the confinement needs.
type AppLookup interface {
	FindByCode(ctx context.Context, code string) (*application.Application, error)
}

// ClientLookup is the slice of the client repository clientId resolution needs.
type ClientLookup interface {
	FindByID(ctx context.Context, id string) (*client.Client, error)
	FindByIdentifier(ctx context.Context, identifier string) (*client.Client, error)
}

// ClientLinks answers which clients an application's service account is
// linked to (see Service.ReachFor). *Repository implements it.
type ClientLinks interface {
	// ServiceAccountClients is the explicit client ids of the service account
	// behind a principal; ok is false when the principal is not a service
	// account.
	ServiceAccountClients(ctx context.Context, principalID string) (clients []string, ok bool, err error)
	// ApplicationEnabledClients is the clients an application is enabled for
	// (app_client_configs.enabled).
	ApplicationEnabledClients(ctx context.Context, applicationID string) ([]string, error)
}

// Service is the access rules of platform config that the HTTP API and the
// server-rendered UI share: who may read, write, delete and reveal, the
// application and client confinement, client resolution, and the audited
// secret reveal. Both callers run these, so the two surfaces cannot drift.
type Service struct {
	Repo *Repository
	UoW  *usecasepgx.UnitOfWork
	// Apps resolves an application code to an application, for confining
	// application-scoped principals (see ConfineApp). Nil uses Repo.
	Apps AppLookup
	// Clients resolves a request's clientId (a clt_ id or the client's
	// identifier) to the client id; see ResolveClientID. Nil passes the
	// reference through (tests without a client repository).
	Clients ClientLookup
	// Links names the clients an application's service account is linked to
	// (see ReachFor). Nil uses Repo.
	Links ClientLinks
	// Audit records every secret reveal. Required for Reveal: a reveal that
	// cannot be recorded is refused.
	Audit *audit.Repository
}

// repoApps adapts the repository's application lookup to AppLookup.
type repoApps struct{ r *Repository }

func (a repoApps) FindByCode(ctx context.Context, code string) (*application.Application, error) {
	id, err := a.r.ApplicationIDByCode(ctx, code)
	if err != nil || id == "" {
		return nil, err
	}
	return &application.Application{ID: ids.ApplicationID(id), Code: code}, nil
}

func (s *Service) apps() AppLookup {
	if s.Apps != nil {
		return s.Apps
	}
	if s.Repo != nil {
		return repoApps{s.Repo}
	}
	return nil
}

func (s *Service) links() ClientLinks {
	if s.Links != nil {
		return s.Links
	}
	if s.Repo != nil {
		return s.Repo
	}
	return nil
}

// ScopeFor derives the config scope from an optional (already resolved)
// clientId, matching the set-property operation's own scoping logic.
func ScopeFor(clientID string) (Scope, *string) {
	if clientID != "" {
		cid := clientID
		return ScopeClient, &cid
	}
	return ScopeGlobal, nil
}

// ResolveClientID canonicalises a request's client reference (the clt_ id or
// the client's identifier, e.g. "acme") to the client id, so a client has one
// scope however it is named. An empty reference means the global value. An
// unknown client is a 404: it must not create (or read) a row keyed by a
// string that is no client. Callers authorise the application first, so an
// unauthorised caller cannot use it to learn which clients exist.
// ClientIdentifiers maps each client id in ids to its identifier, for
// answers that name a client both ways. An id with no client (or no client
// repository) is left out; a lookup failure is an error.
func (s *Service) ClientIdentifiers(ctx context.Context, ids []string) (map[string]string, error) {
	out := make(map[string]string, len(ids))
	if s.Clients == nil {
		return out, nil
	}
	for _, id := range ids {
		if _, seen := out[id]; seen || id == "" {
			continue
		}
		c, err := s.Clients.FindByID(ctx, id)
		if err != nil {
			return nil, usecase.Internal("REPO", "client lookup failed", err)
		}
		if c != nil {
			out[id] = c.Identifier
		}
	}
	return out, nil
}

func (s *Service) ResolveClientID(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" || s.Clients == nil {
		return ref, nil
	}
	c, err := s.Clients.FindByID(ctx, ref)
	if err != nil {
		return "", usecase.Internal("REPO", "client lookup failed", err)
	}
	if c == nil {
		// Identifiers are stored lower-case.
		c, err = s.Clients.FindByIdentifier(ctx, strings.ToLower(ref))
		if err != nil {
			return "", usecase.Internal("REPO", "client lookup failed", err)
		}
	}
	if c == nil {
		return "", httperror.NotFound("Client", ref)
	}
	return string(c.ID), nil
}

// ConfineApp keeps an application-scoped principal (one that does not hold
// all-applications access, e.g. an application's service account) to its own
// application's config. Anchor tier alone is not enough: every application's
// service account is anchor tier, and without this it could read (secret
// values included), write, delete or grant access to any other application's
// config. An application code that names no application is refused for such a
// principal, so it cannot probe or squat codes it does not own.
func (s *Service) ConfineApp(ctx context.Context, ac *auth.AuthContext, appCode string) error {
	// Non-anchor callers are gated by the role-based access grants and the
	// client confinement (ReachFor); the gap is anchor tier, which skips both.
	if !ac.IsAnchor() || !ac.IsApplicationScoped() {
		return nil
	}
	apps := s.apps()
	if apps == nil {
		return usecase.Internal("CONFIG", "platform config has no application lookup", nil)
	}
	app, err := apps.FindByCode(ctx, appCode)
	if err != nil {
		return usecase.Internal("REPO", "find application failed", err)
	}
	if app == nil || !ac.CanAccessApplication(string(app.ID)) {
		return httperror.Forbidden("Not authorised for application '" + appCode + "'")
	}
	return nil
}

// Reach is which of one application's values a caller may touch, by client.
// The application-wide (global) value is readable by every caller that passes
// the read rule.
type Reach struct {
	// All is every client's values and the global value: an anchor-tier
	// administrator.
	All bool
	// Clients are the clients whose values the caller may touch, when not All.
	Clients map[string]bool
	// GlobalWrite allows setting and deleting the application-wide value, when
	// not All: an application's own service account (the value is its
	// application's), never a caller below anchor tier.
	GlobalWrite bool
}

// CanRead reports whether a value scoped to clientID ("" for the global
// value) may be read.
func (r Reach) CanRead(clientID string) bool {
	return clientID == "" || r.All || r.Clients[clientID]
}

// CanWrite reports whether a value scoped to clientID ("" for the global
// value) may be set or deleted.
func (r Reach) CanWrite(clientID string) bool {
	if clientID == "" {
		return r.All || r.GlobalWrite
	}
	return r.All || r.Clients[clientID]
}

// clientSet is the client ids of a list, an "id:identifier" pair counting as
// its id (the shape of the clients claim).
func clientSet(clients []string) map[string]bool {
	out := make(map[string]bool, len(clients))
	for _, c := range clients {
		id, _, _ := strings.Cut(c, ":")
		if id = strings.TrimSpace(id); id != "" && id != "*" {
			out[id] = true
		}
	}
	return out
}

// ReachFor is the client confinement of a caller on an application's config:
//
//   - an anchor-tier administrator (all-applications access, or a person
//     confined only to some applications) reaches every client and the global
//     value, as before;
//   - an application's service account (anchor tier, bound to specific
//     applications) reaches only the clients it is linked to: its explicit
//     client ids when it has any, otherwise the clients its application is
//     enabled for; plus the application's global value;
//   - a caller below anchor tier (decided by the per-role access grants, which
//     have no client axis) reaches only its own clients (ac.Clients); it may
//     read the global value but not set or delete it.
func (s *Service) ReachFor(ctx context.Context, ac *auth.AuthContext, app string) (Reach, error) {
	if !ac.IsAnchor() {
		var clients []string
		if ac != nil {
			clients = ac.Clients
		}
		return Reach{Clients: clientSet(clients)}, nil
	}
	if !ac.IsApplicationScoped() {
		return Reach{All: true}, nil
	}
	links := s.links()
	if links == nil {
		return Reach{}, usecase.Internal("CONFIG", "platform config has no client links lookup", nil)
	}
	linked, isServiceAccount, err := links.ServiceAccountClients(ctx, ac.PrincipalID)
	if err != nil {
		return Reach{}, usecase.Internal("REPO", "service account clients lookup failed", err)
	}
	if !isServiceAccount {
		return Reach{All: true}, nil
	}
	if len(linked) == 0 {
		apps := s.apps()
		if apps == nil {
			return Reach{}, usecase.Internal("CONFIG", "platform config has no application lookup", nil)
		}
		a, err := apps.FindByCode(ctx, app)
		if err != nil {
			return Reach{}, usecase.Internal("REPO", "find application failed", err)
		}
		if a != nil {
			if linked, err = links.ApplicationEnabledClients(ctx, string(a.ID)); err != nil {
				return Reach{}, usecase.Internal("REPO", "application clients lookup failed", err)
			}
		}
	}
	return Reach{Clients: clientSet(linked), GlobalWrite: true}, nil
}

// AuthorizeClientRead refuses reading a value scoped to clientID (the
// canonical id, "" for the global value) that the caller's Reach excludes.
func (s *Service) AuthorizeClientRead(ctx context.Context, ac *auth.AuthContext, app, clientID string) error {
	reach, err := s.ReachFor(ctx, ac, app)
	if err != nil {
		return err
	}
	if !reach.CanRead(clientID) {
		return httperror.Forbidden("Not authorised for client '" + clientID + "'")
	}
	return nil
}

// AuthorizeClientWrite refuses setting or deleting a value scoped to clientID
// (the canonical id, "" for the global value) that the caller's Reach
// excludes.
func (s *Service) AuthorizeClientWrite(ctx context.Context, ac *auth.AuthContext, app, clientID string) error {
	reach, err := s.ReachFor(ctx, ac, app)
	if err != nil {
		return err
	}
	if reach.CanWrite(clientID) {
		return nil
	}
	if clientID == "" {
		return httperror.Forbidden("only an anchor-tier caller or the application's own service account may set or delete the application-wide value; pass clientId")
	}
	return httperror.Forbidden("Not authorised for client '" + clientID + "'")
}

// AuthorizeRead is the rule for listing and reading an application's config:
// the application confinement, config view for an anchor-tier caller, and
// read access through the role grants for anyone below anchor tier. Which
// clients' values the caller sees is ReachFor's.
func (s *Service) AuthorizeRead(ctx context.Context, ac *auth.AuthContext, app string) error {
	if err := s.ConfineApp(ctx, ac, app); err != nil {
		return err
	}
	if err := auth.RequireConfigView(ac); err != nil {
		return err
	}
	if !ac.IsAnchor() {
		ok, err := s.Repo.HasAccess(ctx, app, ac.Roles, false)
		if err != nil {
			return usecase.Internal("REPO", "has_access failed", err)
		}
		if !ok {
			return httperror.Forbidden("No read access to platform config for " + app)
		}
	}
	return nil
}

// AuthorizeUpdate is AuthorizeWrite: the full write rule for the application
// (confinement, the permission, and below anchor tier the write grant). It
// runs before any client is resolved, so a caller without write access cannot
// probe which clients exist.
func (s *Service) AuthorizeUpdate(ctx context.Context, ac *auth.AuthContext, app string) error {
	return s.AuthorizeWrite(ctx, ac, app)
}

// List is an application's properties (secrets masked) that the caller's
// Reach includes: the global values and its clients' values.
func (s *Service) List(ctx context.Context, ac *auth.AuthContext, app string) ([]Config, error) {
	if err := s.AuthorizeRead(ctx, ac, app); err != nil {
		return nil, err
	}
	reach, err := s.ReachFor(ctx, ac, app)
	if err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindConfigsByApplication(ctx, app)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_configs_by_application failed", err)
	}
	if reach.All {
		return rows, nil
	}
	out := rows[:0]
	for i := range rows {
		if reach.CanRead(deref(ids.StringPtr(rows[i].ClientID))) {
			out = append(out, rows[i])
		}
	}
	return out, nil
}

// Get is one property (a secret masked by the caller's serialisation), with
// List's rules: the application is authorised before the client is resolved,
// and a client outside the caller's Reach is refused.
func (s *Service) Get(ctx context.Context, ac *auth.AuthContext, app, section, property, clientRef string) (*Config, error) {
	if err := s.AuthorizeRead(ctx, ac, app); err != nil {
		return nil, err
	}
	cid, err := s.ResolveClientID(ctx, clientRef)
	if err != nil {
		return nil, err
	}
	if err := s.AuthorizeClientRead(ctx, ac, app, cid); err != nil {
		return nil, err
	}
	scope, clientID := ScopeFor(cid)
	c, err := s.Repo.FindByCoordinate(ctx, app, section, property, scope, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_coordinate failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("Config", app+"/"+section+"/"+property)
	}
	return c, nil
}

// AuthorizeWrite is the rule for setting or deleting an application's config
// as a whole: the application confinement, config update for an anchor-tier
// caller, and below anchor tier write access to the application through the
// role grants. Which clients' values the caller may write is ReachFor's
// (AuthorizeClientWrite).
func (s *Service) AuthorizeWrite(ctx context.Context, ac *auth.AuthContext, app string) error {
	if err := s.ConfineApp(ctx, ac, app); err != nil {
		return err
	}
	if err := auth.RequireConfigUpdate(ac); err != nil {
		return err
	}
	if !ac.IsAnchor() {
		ok, err := s.Repo.HasAccess(ctx, app, ac.Roles, true)
		if err != nil {
			return usecase.Internal("REPO", "has_access failed", err)
		}
		if !ok {
			return httperror.Forbidden("No write access to platform config for " + app)
		}
	}
	return nil
}

// AuthorizeSet is the whole rule for setting or deleting the value of one
// application scoped to clientID (the canonical id, "" for the global value):
// AuthorizeWrite and AuthorizeClientWrite. The set-property and
// delete-property use cases run it in their Authorize phase.
func (s *Service) AuthorizeSet(ctx context.Context, ac *auth.AuthContext, app, clientID string) error {
	if err := s.AuthorizeWrite(ctx, ac, app); err != nil {
		return err
	}
	return s.AuthorizeClientWrite(ctx, ac, app, clientID)
}

// RequireApplication refuses config for an application code that names no
// application (config rows are keyed by code without a foreign key; an
// all-applications administrator could otherwise pre-seed values that a
// future application with that code would inherit).
func (s *Service) RequireApplication(ctx context.Context, app string) error {
	apps := s.apps()
	if apps == nil {
		return usecase.Internal("CONFIG", "platform config has no application lookup", nil)
	}
	a, err := apps.FindByCode(ctx, app)
	if err != nil {
		return usecase.Internal("REPO", "find application failed", err)
	}
	if a == nil {
		return httperror.NotFound("Application", app)
	}
	return nil
}

// DeleteRequest names one property to delete.
type DeleteRequest struct {
	ApplicationCode, Section, Property string
	// ClientRef is the client's id or identifier, "" for the global value.
	ClientRef string
	// OnBehalfOf is who a service account acts for (see OnBehalfOf), recorded
	// in the audit row.
	OnBehalfOf string
}

// Delete removes one property (idempotent: a property that is not there is
// not an error). See DeleteProperty.
func (s *Service) Delete(ctx context.Context, ac *auth.AuthContext, app, section, property, clientRef string) error {
	return s.DeleteProperty(ctx, ac, DeleteRequest{ApplicationCode: app, Section: section, Property: property, ClientRef: clientRef})
}

// DeleteProperty removes one property through the delete-property use case
// (audited, emits PropertyDeleted). The application is authorised before the
// client is resolved; a property that is not there is not an error.
func (s *Service) DeleteProperty(ctx context.Context, ac *auth.AuthContext, req DeleteRequest) error {
	if err := s.AuthorizeWrite(ctx, ac, req.ApplicationCode); err != nil {
		return err
	}
	cid, err := s.ResolveClientID(ctx, req.ClientRef)
	if err != nil {
		return err
	}
	if err := s.AuthorizeClientWrite(ctx, ac, req.ApplicationCode, cid); err != nil {
		return err
	}
	cmd := DeletePropertyCommand{ApplicationCode: req.ApplicationCode, Section: req.Section, Property: req.Property}
	if cid != "" {
		cmd.ClientID = &cid
	}
	if req.OnBehalfOf != "" {
		obo := req.OnBehalfOf
		cmd.OnBehalfOf = &obo
	}
	// The use case reads its caller from the context: make it ac.
	ctx = auth.WithContext(ctx, ac)
	_, err = usecaseop.Run(ctx, s.UoW, DeletePropertyOp(s), cmd, auth.NewExecutionContext(ctx))
	if httperror.IsNotFound(err) {
		if ue := usecase.AsError(err); ue != nil && ue.Code == configNotFoundCode {
			return nil
		}
	}
	return err
}

// AuthorizeRevealApp is the application half of the reveal rule (see
// AuthorizeReveal): the permission and the application confinement, with no
// client involved. A UI uses it to decide whether to offer a reveal at all.
func (s *Service) AuthorizeRevealApp(ctx context.Context, ac *auth.AuthContext, app string) error {
	if err := s.ConfineApp(ctx, ac, app); err != nil {
		return err
	}
	adminErr := auth.CanRevealPlatformConfigSecret(ac)
	if adminErr == nil {
		return nil
	}
	if auth.CanRevealApplicationConfigSecret(ac) != nil {
		return adminErr
	}
	if !ac.IsApplicationScoped() {
		return httperror.Forbidden("platform:application-service:config:reveal reaches only the account's own applications; this account is not bound to one")
	}
	apps := s.apps()
	if apps == nil {
		return usecase.Internal("CONFIG", "platform config has no application lookup", nil)
	}
	a, err := apps.FindByCode(ctx, app)
	if err != nil {
		return usecase.Internal("REPO", "find application failed", err)
	}
	if a == nil || !ac.CanAccessApplication(string(a.ID)) {
		return httperror.Forbidden("Not authorised for application '" + app + "'")
	}
	return nil
}

// RevealReach is the client half of the reveal rule that needs no lookup, for
// a value scoped to clientID ("" for the application-wide value): below anchor
// tier only a client's own value may be revealed. AuthorizeReveal adds the
// service-account confinement (ReachFor).
func RevealReach(ac *auth.AuthContext, clientID string) error {
	if clientID != "" && !ac.CanAccessClient(clientID) {
		return httperror.Forbidden("Not authorised for client '" + clientID + "'")
	}
	if clientID == "" && !ac.IsAnchor() {
		return httperror.Forbidden("a caller below anchor tier can reveal only a client's own value; pass clientId")
	}
	return nil
}

// AuthorizeReveal is the reveal rule (see Reveal). Reveal is always a
// permission, never implied by being an application's service account:
//
//   - a platform administrator holds platform:admin:config:reveal (anchor tier);
//   - an application's service account holds
//     platform:application-service:config:reveal, and is then confined to the
//     applications it is bound to (an account with all-applications access is
//     not "its own application" and needs the admin permission instead), and
//     to the clients it is linked to (ReachFor), plus its application's
//     global value at anchor tier; below anchor tier, to client-scoped values
//     of its own clients (the application-wide value is not one client's).
//
// It returns the canonical client id of the requested scope ("" for the global
// value). The client reference is resolved only after the caller has passed the
// permission and application checks, so an unauthorised caller cannot probe
// which clients exist.
func (s *Service) AuthorizeReveal(ctx context.Context, ac *auth.AuthContext, app, clientRef string) (string, error) {
	if err := s.AuthorizeRevealApp(ctx, ac, app); err != nil {
		return "", err
	}
	clientID, err := s.ResolveClientID(ctx, clientRef)
	if err != nil {
		return "", err
	}
	if err := RevealReach(ac, clientID); err != nil {
		return "", err
	}
	if err := s.AuthorizeClientRead(ctx, ac, app, clientID); err != nil {
		return "", err
	}
	return clientID, nil
}

// Revealed is a SECRET's plaintext with the property it belongs to.
type Revealed struct {
	Config *Config
	Value  string
}

// Reveal returns a SECRET's plaintext. It is the only place one leaves the
// platform, so it is strict:
//   - the caller is confined to its own application (ConfineApp) and clients
//     (ReachFor), and must hold a reveal permission (AuthorizeReveal);
//   - the reveal is audited, and refused if the audit row cannot be written.
//
// What the caller does with the value (never cache it) is the caller's.
func (s *Service) Reveal(ctx context.Context, ac *auth.AuthContext, app, section, property, clientRef string) (*Revealed, error) {
	return s.RevealFor(ctx, ac, app, section, property, clientRef, "")
}

// RevealFor is Reveal recording on whose behalf a service account reads the
// value (onBehalfOf, see OnBehalfOf; "" for none) in the audit row.
func (s *Service) RevealFor(ctx context.Context, ac *auth.AuthContext, app, section, property, clientRef, onBehalfOf string) (*Revealed, error) {
	cid, err := s.AuthorizeReveal(ctx, ac, app, clientRef)
	if err != nil {
		return nil, err
	}
	var detail map[string]string
	if onBehalfOf != "" {
		detail = map[string]string{"onBehalfOf": onBehalfOf}
	}
	return s.RevealAuthorised(ctx, ac, app, section, property, cid, detail)
}

// configNotFoundCode is the error code of a missing property
// (httperror.NotFound("Config", ...)).
const configNotFoundCode = "Config_NOT_FOUND"

// RevealAuthorised is Reveal for a caller whose right to this value has been
// decided (RevealFor, after AuthorizeReveal): clientID is the canonical client
// id of the value's scope ("" for the global value). The lookup, the plaintext
// and the audit-or-refuse rule are Reveal's own; detail is added to the audit
// row to say on whose behalf the value was read. (The ingestion runtime does
// not use it: its fetches are FetchBoundSecretForIngestionRuntime.)
func (s *Service) RevealAuthorised(ctx context.Context, ac *auth.AuthContext, app, section, property, clientID string, detail map[string]string) (*Revealed, error) {
	scope, scopeClient := ScopeFor(clientID)
	c, err := s.Repo.FindByCoordinate(ctx, app, section, property, scope, scopeClient)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_coordinate failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("Config", app+"/"+section+"/"+property)
	}
	if c.ValueType != ValueSecret {
		return nil, usecase.Validation("NOT_A_SECRET", "this property is not a SECRET; read it with GET /api/config/{app}/{section}/{property}")
	}
	if s.Audit == nil {
		return nil, usecase.Internal("AUDIT", "secret reveal is not wired to the audit log", nil)
	}
	value, err := s.Repo.Reveal(ctx, c)
	if err != nil {
		return nil, usecase.Internal("REVEAL", "secret could not be decrypted", err)
	}
	fields := map[string]string{"applicationCode": c.ApplicationCode, "section": c.Section, "property": c.Property}
	maps.Copy(fields, detail)
	operationJSON, _ := json.Marshal(fields)
	actor := ac.PrincipalID
	if err := s.Audit.Insert(ctx, &audit.Log{
		ID:            tsid.Generate(tsid.AuditLog),
		EntityType:    "PLATFORM_CONFIG",
		EntityID:      c.ID,
		Operation:     "SECRET_REVEALED",
		OperationJSON: operationJSON,
		PrincipalID:   &actor,
		ClientID:      ids.StringPtr(c.ClientID),
		PerformedAt:   time.Now().UTC(),
	}); err != nil {
		return nil, usecase.Internal("AUDIT", "secret reveal could not be audited", err)
	}
	return &Revealed{Config: c, Value: value}, nil
}

// FetchBoundSecretForIngestionRuntime returns the plaintext of a SECRET for
// the ingestion runtime, which fetches the secrets its ingestion's live
// definition binds (owner ruling 2026-10-25). It is NOT a reveal: no reveal
// permission is asked and no aud_logs row is written. A runtime re-reads each
// bound secret about once a minute per route, and an audit row per read
// buried the audit log; that read is security telemetry instead (a log line,
// a counter and a last-fetched record, written by the caller).
//
// One caller, by convention: the ingestion runtime API's secrets call
// (internal/platform/ingestion/runtimeapi), which has already decided the
// caller's right to this value: the runtime's identity, the lease, the live
// definition naming the secret, the client being active. Nothing else may
// call it; a person or an application reads a secret's value only through
// Reveal, which is audited. clientID is the canonical client id of the
// value's scope; there is no global fallback (the lookup is at that scope
// only). Not found is httperror.NotFound("Config", ...); a property that is
// not a SECRET is NOT_A_SECRET.
func (s *Service) FetchBoundSecretForIngestionRuntime(ctx context.Context, app, section, property, clientID string) (*Revealed, error) {
	scope, scopeClient := ScopeFor(clientID)
	c, err := s.Repo.FindByCoordinate(ctx, app, section, property, scope, scopeClient)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_coordinate failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("Config", app+"/"+section+"/"+property)
	}
	if c.ValueType != ValueSecret {
		return nil, usecase.Validation("NOT_A_SECRET", "this property is not a SECRET")
	}
	value, err := s.Repo.Reveal(ctx, c)
	if err != nil {
		return nil, usecase.Internal("REVEAL", "secret could not be decrypted", err)
	}
	return &Revealed{Config: c, Value: value}, nil
}

// MaxOnBehalfOf is the longest X-On-Behalf-Of value the platform records.
const MaxOnBehalfOf = 256

// OnBehalfOf is the person a service account says it acts for (the
// X-On-Behalf-Of header an application sends when its own signed-in user
// changes or reveals the application's configuration), to be recorded in the
// audit row. It is honoured only from a service account: from any other
// caller it is ignored (""), since a person acts for themselves. A value
// longer than MaxOnBehalfOf characters or holding a control character is a
// 400.
func OnBehalfOf(ac *auth.AuthContext, raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" || ac == nil || ac.PrincipalType != auth.PrincipalTypeService {
		return "", nil
	}
	if utf8.RuneCountInString(v) > MaxOnBehalfOf || !utf8.ValidString(v) || strings.ContainsFunc(v, unicode.IsControl) {
		return "", usecase.Validation("INVALID_ON_BEHALF_OF", "X-On-Behalf-Of is at most 256 printable characters")
	}
	return v, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
