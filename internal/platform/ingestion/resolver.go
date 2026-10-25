package ingestion

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// ResolvedTarget is a Target's codes as ids. Nothing stores it: the target
// keeps its codes, so a definition file ports between environments; the
// resolution is the save-time proof that they name real, usable records.
type ResolvedTarget struct {
	PoolID           string
	ConnectionID     *string
	ServiceAccountID *string
}

// Resolver checks the references an ingestion makes and turns codes into
// records. Operations call it at save so a bad reference is a 400 then, not a
// failure in a runtime later.
type Resolver interface {
	// Application finds the owning application by code. It must exist, be
	// active, and be of type INTEGRATION (INGESTION_APPLICATION_INVALID).
	Application(ctx context.Context, code string) (*application.Application, error)
	// ApplicationByID finds an ingestion's application by id, for an ingestion
	// that already exists: it is not checked for being active, so a
	// deactivated application does not freeze the ingestions it owns. Nil when
	// there is none.
	ApplicationByID(ctx context.Context, id string) (*application.Application, error)
	// Client finds a client by id or identifier (INGESTION_CLIENT_INVALID).
	Client(ctx context.Context, idOrIdentifier string) (*client.Client, error)
	// Target checks the target for an ingestion of this client and returns its
	// references as ids (INGESTION_TARGET_INVALID).
	Target(ctx context.Context, applicationCode string, clientID ids.ClientID, t Target) (ResolvedTarget, error)
}

// RepoResolver is the Resolver over the platform's repositories.
type RepoResolver struct {
	applications    *application.Repository
	clients         *client.Repository
	pools           *dispatchpool.Repository
	connections     *connection.Repository
	serviceAccounts *serviceaccount.Repository
	guard           *netguard.Policy
}

// NewResolver wires a resolver. A nil guard is netguard.Default, the policy
// the batch dispatch-job API checks target URLs with.
func NewResolver(
	applications *application.Repository,
	clients *client.Repository,
	pools *dispatchpool.Repository,
	connections *connection.Repository,
	serviceAccounts *serviceaccount.Repository,
	guard *netguard.Policy,
) *RepoResolver {
	if guard == nil {
		guard = netguard.Default
	}
	return &RepoResolver{applications, clients, pools, connections, serviceAccounts, guard}
}

func (r *RepoResolver) Application(ctx context.Context, code string) (*application.Application, error) {
	app, err := r.applications.FindByCode(ctx, code)
	if err != nil {
		return nil, usecase.Internal("REPO", "application lookup failed", err)
	}
	switch {
	case app == nil:
		return nil, usecase.Validation("INGESTION_APPLICATION_INVALID", fmt.Sprintf("no application with code %q", code))
	case !app.IsIntegration():
		return nil, usecase.Validation("INGESTION_APPLICATION_INVALID", fmt.Sprintf("application %q is not of type INTEGRATION", code))
	case !app.Active:
		return nil, usecase.Validation("INGESTION_APPLICATION_INVALID", fmt.Sprintf("application %q is not active", code))
	}
	return app, nil
}

func (r *RepoResolver) ApplicationByID(ctx context.Context, id string) (*application.Application, error) {
	app, err := r.applications.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "application lookup failed", err)
	}
	return app, nil
}

func (r *RepoResolver) Client(ctx context.Context, idOrIdentifier string) (*client.Client, error) {
	var (
		c   *client.Client
		err error
	)
	if _, parseErr := ids.ParseClientID(idOrIdentifier); parseErr == nil {
		c, err = r.clients.FindByID(ctx, idOrIdentifier)
	} else {
		c, err = r.clients.FindByIdentifier(ctx, idOrIdentifier)
	}
	if err != nil {
		return nil, usecase.Internal("REPO", "client lookup failed", err)
	}
	if c == nil {
		return nil, usecase.Validation("INGESTION_CLIENT_INVALID", fmt.Sprintf("no client %q", idOrIdentifier))
	}
	return c, nil
}

func (r *RepoResolver) Target(ctx context.Context, applicationCode string, clientID ids.ClientID, t Target) (ResolvedTarget, error) {
	var out ResolvedTarget
	invalid := func(format string, args ...any) error {
		return usecase.Validation("INGESTION_TARGET_INVALID", fmt.Sprintf(format, args...))
	}
	cid := string(clientID)

	if err := r.guard.ValidateURL(t.TargetURL); err != nil {
		return out, invalid("target.targetUrl %s", err)
	}
	// The job's code names the application it belongs to, which must be the
	// ingestion's own: the batch dispatch API's rule for an application's
	// credential (checkJobApplication, rule 1), so an ingestion cannot make
	// jobs that read as another application's.
	if owner, _, _ := strings.Cut(t.Code, ":"); !strings.EqualFold(owner, applicationCode) {
		return out, invalid("target.code: %q belongs to application %q; an ingestion of %q makes jobs of its own application only",
			t.Code, owner, applicationCode)
	}
	if t.Connection != nil && t.ServiceAccount != nil {
		return out, invalid("target.serviceAccount may not be set with a connection: the connection's service account signs")
	}

	// A pool of the ingestion's client first, then a pool of no client, which
	// every client may use.
	pool, err := r.pools.FindByCode(ctx, t.Pool, &cid)
	if err == nil && pool == nil {
		pool, err = r.pools.FindByCode(ctx, t.Pool, nil)
	}
	if err != nil {
		return out, usecase.Internal("REPO", "dispatch pool lookup failed", err)
	}
	switch {
	case pool == nil:
		return out, invalid("target.pool: no dispatch pool with code %q usable by this client", t.Pool)
	case pool.Status != dispatchpool.StatusActive:
		return out, invalid("target.pool: dispatch pool %q is %s", t.Pool, strings.ToLower(string(pool.Status)))
	}
	out.PoolID = pool.ID

	if t.Connection != nil {
		conn, err := r.findConnection(ctx, *t.Connection, applicationCode, cid)
		if err != nil {
			return out, usecase.Internal("REPO", "connection lookup failed", err)
		}
		if conn == nil {
			return out, invalid("target.connection: no connection with code %q usable by this application and client", *t.Connection)
		}
		out.ConnectionID = &conn.ID
	}

	if t.ServiceAccount != nil {
		id, err := r.signer(ctx, *t.ServiceAccount, applicationCode, cid)
		if err != nil {
			return out, err
		}
		out.ServiceAccountID = &id
	}
	return out, nil
}

// signer resolves a target's explicit service account: it signs the jobs, so
// it must be one of the ingestion's application's own accounts (bound to that
// application) and reach the ingestion's client. An account linked to no
// client reaches all of its application's clients, which is acceptable only
// because it is bound to this application. A platform account (bound to no
// application) or another application's is refused: signing as either would
// let an ingestion speak for something it does not own.
func (r *RepoResolver) signer(ctx context.Context, code, applicationCode, clientID string) (string, error) {
	invalid := func(format string, args ...any) error {
		return usecase.Validation("INGESTION_TARGET_INVALID", fmt.Sprintf(format, args...))
	}
	sa, err := r.serviceAccounts.FindByCode(ctx, code)
	if err != nil {
		return "", usecase.Internal("REPO", "service account lookup failed", err)
	}
	switch {
	case sa == nil:
		return "", invalid("target.serviceAccount: no service account with code %q", code)
	case !sa.Active:
		return "", invalid("target.serviceAccount: service account %q is not active", code)
	case sa.ApplicationID == nil:
		return "", invalid("target.serviceAccount: service account %q belongs to no application; use one of application %q's", code, applicationCode)
	}
	app, err := r.applications.FindByCode(ctx, applicationCode)
	if err != nil {
		return "", usecase.Internal("REPO", "application lookup failed", err)
	}
	switch {
	case app == nil || *sa.ApplicationID != app.ID:
		return "", invalid("target.serviceAccount: service account %q belongs to another application; use one of application %q's", code, applicationCode)
	case len(sa.ClientIDs) > 0 && !slices.Contains(sa.ClientIDs, clientID):
		return "", invalid("target.serviceAccount: service account %q does not reach this client", code)
	}
	return sa.ID, nil
}

// findConnection looks for the connection by code from the most specific scope
// to the least: this application and client, this application, this client,
// then a shared connection (no application, no client).
func (r *RepoResolver) findConnection(ctx context.Context, code, applicationCode, clientID string) (*connection.Connection, error) {
	for _, scope := range []struct{ app, client *string }{
		{&applicationCode, &clientID},
		{&applicationCode, nil},
		{nil, &clientID},
		{nil, nil},
	} {
		conn, err := r.connections.FindByCode(ctx, code, scope.app, scope.client)
		if err != nil || conn != nil {
			return conn, err
		}
	}
	return nil, nil
}
