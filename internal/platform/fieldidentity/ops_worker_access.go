package fieldidentity

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// ── application grants ───────────────────────────────────────────────────

// WorkerAppCommand grants or revokes one application.
type WorkerAppCommand struct {
	WorkerRef
	AppCode string `json:"appCode"`
}

func validateWorkerApp(ctx context.Context, cmd WorkerAppCommand) error {
	if err := validateRef(ctx, cmd.WorkerRef); err != nil {
		return err
	}
	if strings.TrimSpace(cmd.AppCode) == "" {
		return usecase.Validation("APP_CODE_REQUIRED", "appCode is required")
	}
	return nil
}

// GrantWorkerApp lets a worker sign in to an (active) application. Idempotent.
func (o *Ops) GrantWorkerApp() usecaseop.Operation[WorkerAppCommand, Event] {
	return usecaseop.Operation[WorkerAppCommand, Event]{
		Name:      "GrantFieldWorkerApp",
		Validate:  validateWorkerApp,
		Authorize: usecaseop.Public[WorkerAppCommand],
		Execute: func(ctx context.Context, cmd WorkerAppCommand, ec usecase.ExecutionContext) (usecaseop.Plan[Event], error) {
			apps, err := o.resolveApps(ctx, []string{cmd.AppCode}, true)
			if err != nil {
				return nil, err
			}
			// An application-confined caller may grant its own application to
			// a worker it cannot yet see (that is how workers join an app), so
			// the worker is loaded with the client box only.
			ident, err := o.R.Identities.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, repoErr("find_field_worker", err)
			}
			if ident == nil || (cmd.ClientID != "" && ident.ClientID != cmd.ClientID) {
				return nil, httperror.NotFound("FieldWorker", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), &ident.ClientID); err != nil {
				return nil, err
			}
			ident.Grant(apps[0].ID, SourceAdmin)
			ev := NewEvent(ec, WorkerAppGrantedType, "field-worker", ident.ID,
				workerData(ident, map[string]any{"applicationId": apps[0].ID, "applicationCode": apps[0].Code}))
			return usecaseop.Save(ident, o.R.Identities, ev), nil
		},
	}
}

// RevokeWorkerApp removes a worker's access to an application and revokes
// that application's sessions for them. Idempotent.
func (o *Ops) RevokeWorkerApp() usecaseop.TxOperation[WorkerAppCommand, Event] {
	return usecaseop.TxOperation[WorkerAppCommand, Event]{
		Name:      "RevokeFieldWorkerApp",
		Validate:  validateWorkerApp,
		Authorize: usecaseop.Public[WorkerAppCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd WorkerAppCommand, ec usecase.ExecutionContext) (Event, error) {
			apps, err := o.resolveApps(ctx, []string{cmd.AppCode}, false)
			if err != nil {
				return Event{}, err
			}
			ident, err := o.loadWorker(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return Event{}, err
			}
			live, err := countLive(ctx, s, `identity_id = $1`, ident.ID, apps[0].ID)
			if err != nil {
				return Event{}, repoErr("count_sessions", err)
			}
			ident.Revoke(apps[0].ID)
			ident.RevokeSessions(ReasonAdmin, apps[0].ID)
			ev := NewEvent(ec, WorkerAppRevokedType, "field-worker", ident.ID,
				workerData(ident, map[string]any{"applicationId": apps[0].ID, "applicationCode": apps[0].Code}))
			out, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, ident, o.R.Identities, ev, cmd))
			if err != nil {
				return Event{}, err
			}
			if live > 0 {
				if _, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s,
					sessionsRevokedEvent(ec, ident.ClientID, ReasonAdmin, live, ident.ID, "", apps[0].ID), cmd)); err != nil {
					return Event{}, err
				}
			}
			return out, nil
		},
	}
}

// ── platform user link ───────────────────────────────────────────────────

// CodeInvalidLink is the error code of a link target that cannot be linked;
// the admin API answers it 422.
const CodeInvalidLink = "INVALID_LINK"

// LinkWorkerCommand links a worker to one platform user.
type LinkWorkerCommand struct {
	WorkerRef
	PrincipalID string `json:"principalId"`
}

// LinkWorker links a worker to a platform user, for audit correlation only (it
// grants nothing on either side). The principal must exist, be a USER (not a
// service account), be ACTIVE and be able to access the worker's client
// (anchor, the client itself, or a granted/partner client): 422 INVALID_LINK
// otherwise. A user already linked to another worker of the client is 409
// LINK_EXISTS. Re-linking the same user to the same worker changes nothing but
// still records the event; linking a different user replaces the link.
func (o *Ops) LinkWorker() usecaseop.Operation[LinkWorkerCommand, Event] {
	return usecaseop.Operation[LinkWorkerCommand, Event]{
		Name: "LinkFieldWorker",
		Validate: func(ctx context.Context, cmd LinkWorkerCommand) error {
			if err := validateRef(ctx, cmd.WorkerRef); err != nil {
				return err
			}
			if strings.TrimSpace(cmd.PrincipalID) == "" {
				return usecase.Validation("PRINCIPAL_ID_REQUIRED", "principalId is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[LinkWorkerCommand],
		Execute: func(ctx context.Context, cmd LinkWorkerCommand, ec usecase.ExecutionContext) (usecaseop.Plan[Event], error) {
			ident, err := o.loadWorker(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return nil, err
			}
			principalID := strings.TrimSpace(cmd.PrincipalID)
			p, err := o.Principals.FindByID(ctx, ids.PrincipalID(principalID))
			if err != nil {
				return nil, repoErr("find_principal", err)
			}
			if err := linkable(p, ident.ClientID); err != nil {
				return nil, err
			}
			other, err := o.R.Identities.FindByClientAndPrincipal(ctx, ident.ClientID, principalID)
			if err != nil {
				return nil, repoErr("find_field_worker", err)
			}
			if other != nil && other.ID != ident.ID {
				return nil, usecase.Conflict("LINK_EXISTS", "that user is already linked to another field worker of this client")
			}
			ident.LinkedPrincipalID = principalID
			ev := NewEvent(ec, WorkerLinkedType, "field-worker", ident.ID,
				workerData(ident, map[string]any{"principalId": principalID}))
			return usecaseop.Save(ident, o.R.Identities, ev), nil
		},
	}
}

// linkable is the rule for a link target: a USER principal that is ACTIVE and
// reaches the client. Its reasons are deliberately coarse.
func linkable(p *principal.Principal, clientID string) error {
	switch {
	case p == nil:
		return usecase.BusinessRule(CodeInvalidLink, "the platform user does not exist")
	case !p.IsUser():
		return usecase.BusinessRule(CodeInvalidLink, "only a platform user (not a service account) can be linked")
	case !p.Active:
		return usecase.BusinessRule(CodeInvalidLink, "the platform user is not active")
	}
	var home *string
	if p.ClientID != nil {
		h := string(*p.ClientID)
		home = &h
	}
	assigned := make([]string, len(p.AssignedClients))
	for n, c := range p.AssignedClients {
		assigned[n] = string(c)
	}
	if !p.Scope.CanAccessClient(clientID, home, assigned) {
		return usecase.BusinessRule(CodeInvalidLink, "the platform user cannot access the worker's client")
	}
	return nil
}

// UnlinkWorker removes a worker's platform user link. Idempotent: an unlinked
// worker stays unlinked (the event is still recorded, with an empty principalId).
func (o *Ops) UnlinkWorker() usecaseop.Operation[WorkerRef, Event] {
	return usecaseop.Operation[WorkerRef, Event]{
		Name:      "UnlinkFieldWorker",
		Validate:  validateRef,
		Authorize: usecaseop.Public[WorkerRef],
		Execute: func(ctx context.Context, cmd WorkerRef, ec usecase.ExecutionContext) (usecaseop.Plan[Event], error) {
			ident, err := o.loadWorker(ctx, cmd.ID, cmd.ClientID)
			if err != nil {
				return nil, err
			}
			previous := ident.LinkedPrincipalID
			ident.LinkedPrincipalID = ""
			ev := NewEvent(ec, WorkerUnlinkedType, "field-worker", ident.ID,
				workerData(ident, map[string]any{"principalId": previous}))
			return usecaseop.Save(ident, o.R.Identities, ev), nil
		},
	}
}
