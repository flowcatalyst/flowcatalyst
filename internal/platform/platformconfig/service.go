package platformconfig

import (
	"context"
	"encoding/json"
	"maps"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
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

// Service is the access rules of platform config that the HTTP API and the
// server-rendered UI share: who may read, write, delete and reveal, the
// application confinement, client resolution, and the audited secret reveal.
// Both callers run these, so the two surfaces cannot drift.
type Service struct {
	Repo *Repository
	UoW  *usecasepgx.UnitOfWork
	// Apps resolves an application code to an application, for confining
	// application-scoped principals (see ConfineApp).
	Apps AppLookup
	// Clients resolves a request's clientId (a clt_ id or the client's
	// identifier) to the client id; see ResolveClientID. Nil passes the
	// reference through (tests without a client repository).
	Clients ClientLookup
	// Audit records every secret reveal. Required for Reveal: a reveal that
	// cannot be recorded is refused.
	Audit *audit.Repository
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
// string that is no client.
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
	// Non-anchor callers are already gated by the role-based access grants and
	// see secrets masked; the gap is anchor tier, which skips both.
	if !ac.IsAnchor() || !ac.IsApplicationScoped() {
		return nil
	}
	app, err := s.Apps.FindByCode(ctx, appCode)
	if err != nil {
		return usecase.Internal("REPO", "find application failed", err)
	}
	if app == nil || !ac.CanAccessApplication(string(app.ID)) {
		return httperror.Forbidden("Not authorised for application '" + appCode + "'")
	}
	return nil
}

// AuthorizeRead is the rule for listing and reading an application's config:
// the application confinement, config view for an anchor-tier caller, and
// read access through the role grants for anyone below anchor tier.
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

// AuthorizeUpdate is the gate in front of setting a property: the
// application confinement and config update for an anchor-tier caller. It runs
// before anything is looked up, so an unauthorised caller cannot probe which
// clients exist (the set-property use case checks again, and decides for
// callers below anchor tier by their access grants).
func (s *Service) AuthorizeUpdate(ctx context.Context, ac *auth.AuthContext, app string) error {
	if err := s.ConfineApp(ctx, ac, app); err != nil {
		return err
	}
	return auth.RequireConfigUpdate(ac)
}

// List is an application's properties (secrets masked), for a caller who
// passes AuthorizeRead.
func (s *Service) List(ctx context.Context, ac *auth.AuthContext, app string) ([]Config, error) {
	if err := s.AuthorizeRead(ctx, ac, app); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindConfigsByApplication(ctx, app)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_configs_by_application failed", err)
	}
	return rows, nil
}

// AuthorizeWrite is the rule for setting or deleting an application's config
// as a whole: AuthorizeUpdate, and below anchor tier write access to the
// application through the role grants.
func (s *Service) AuthorizeWrite(ctx context.Context, ac *auth.AuthContext, app string) error {
	if err := s.AuthorizeUpdate(ctx, ac, app); err != nil {
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

// Delete removes one property (idempotent: a property that is not there is
// not an error), for a caller who passes AuthorizeWrite.
func (s *Service) Delete(ctx context.Context, ac *auth.AuthContext, app, section, property, clientRef string) error {
	if err := s.AuthorizeWrite(ctx, ac, app); err != nil {
		return err
	}
	cid, err := s.ResolveClientID(ctx, clientRef)
	if err != nil {
		return err
	}
	scope, clientID := ScopeFor(cid)
	c, err := s.Repo.FindByCoordinate(ctx, app, section, property, scope, clientID)
	if err != nil {
		return usecase.Internal("REPO", "find_by_coordinate failed", err)
	}
	if c == nil {
		return nil
	}
	tx, err := s.UoW.Pool().Begin(ctx)
	if err != nil {
		return usecase.Internal("TX", "begin failed", err)
	}
	defer tx.Rollback(ctx)
	if err := s.Repo.Delete(ctx, c, usecasepgx.WrapTxForBootstrap(tx)); err != nil {
		return usecase.Internal("REPO", "delete failed", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return usecase.Internal("TX", "commit failed", err)
	}
	return nil
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
	a, err := s.Apps.FindByCode(ctx, app)
	if err != nil {
		return usecase.Internal("REPO", "find application failed", err)
	}
	if a == nil || !ac.CanAccessApplication(string(a.ID)) {
		return httperror.Forbidden("Not authorised for application '" + app + "'")
	}
	return nil
}

// RevealReach is the client half of the reveal rule for a value scoped to
// clientID ("" for the application-wide value): a client-scoped secret needs
// access to that client, and below anchor tier only a client's own value may
// be revealed.
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
//     not "its own application" and needs the admin permission instead), and,
//     below anchor tier, to client-scoped values of the clients it may act for
//     (the application-wide value is not one client's).
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
	return clientID, nil
}

// Revealed is a SECRET's plaintext with the property it belongs to.
type Revealed struct {
	Config *Config
	Value  string
}

// Reveal returns a SECRET's plaintext. It is the only place one leaves the
// platform, so it is strict:
//   - the caller is confined to its own application (ConfineApp), and an
//     application's own service account may reveal its application's secrets;
//     every other caller must hold platform:admin:config:reveal;
//   - a client-scoped secret needs access to that client;
//   - the reveal is audited, and refused if the audit row cannot be written.
//
// What the caller does with the value (never cache it) is the caller's.
func (s *Service) Reveal(ctx context.Context, ac *auth.AuthContext, app, section, property, clientRef string) (*Revealed, error) {
	cid, err := s.AuthorizeReveal(ctx, ac, app, clientRef)
	if err != nil {
		return nil, err
	}
	return s.RevealAuthorised(ctx, ac, app, section, property, cid, nil)
}

// RevealAuthorised is Reveal for a caller whose right to this value is decided
// by something other than the reveal permission (the ingestion runtime API
// reveals only the secrets an ingestion names, to the runtime that runs it):
// the caller has done that check, and clientID is the canonical client id of
// the value's scope ("" for the global value). The lookup, the plaintext and
// the audit-or-refuse rule are Reveal's own; detail is added to the audit row
// to say on whose behalf the value was read.
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
	value, err := s.Repo.RevealValue(c)
	if err != nil {
		return nil, usecase.Internal("REVEAL", "secret could not be decrypted", err)
	}
	if s.Audit == nil {
		return nil, usecase.Internal("AUDIT", "secret reveal is not wired to the audit log", nil)
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
