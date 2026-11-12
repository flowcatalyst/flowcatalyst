package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	saops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// CredentialRepos are the lookups the create and update checks make before a
// subscription may name the credentials that sign its deliveries.
type CredentialRepos struct {
	Accounts    *serviceaccount.Repository
	Connections *connection.Repository
	Apps        *application.Repository
}

// deliveryChange describes the signing-relevant state a subscription write
// would leave behind, and which parts of it the write changed. A create
// changes everything.
type deliveryChange struct {
	clientID        *string
	applicationCode *string
	connectionID    string // resulting, trimmed ("" = none)
	serviceAcctID   string // resulting, trimmed ("" = none)

	urlChanged, accountChanged, connectionChanged bool
}

func trimmed(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

// check applies the rule "whoever sets or changes the URL, or the credentials,
// of signed work must be allowed to use those credentials" to the resulting
// state. An account that is missing or deactivated is refused only where the
// write itself names it (a delivery from an unchanged binding carries no
// credentials of it, so editing another field of the row is never blocked by
// it); a scope refusal always applies.
func (r CredentialRepos) check(ctx context.Context, c deliveryChange) error {
	if !c.urlChanged && !c.accountChanged && !c.connectionChanged {
		return nil
	}
	ac := auth.FromContext(ctx)

	if c.connectionChanged && c.connectionID != "" {
		conn, err := r.Connections.FindByID(ctx, c.connectionID)
		if err != nil {
			return usecase.Internal("REPO", "find_by_id(connection) failed", err)
		}
		if conn == nil {
			return usecase.NotFound("CONNECTION_NOT_FOUND", "Connection '"+c.connectionID+"' not found")
		}
		if conn.ClientID != nil && (c.clientID == nil || string(*conn.ClientID) != *c.clientID) {
			return usecase.Validation("CONNECTION_SCOPE_MISMATCH",
				"connection '"+c.connectionID+"' is scoped to a different client")
		}
		if c.applicationCode != nil {
			if msg := conn.ScopeProblem(*c.applicationCode, c.clientID); msg != "" {
				return usecase.Validation("CONNECTION_SCOPE_MISMATCH", "connection '"+c.connectionID+"' "+msg)
			}
		}
	}

	if c.serviceAcctID != "" && (c.urlChanged || c.accountChanged) {
		if err := r.wield(ctx, ac, c.serviceAcctID, !c.accountChanged); err != nil {
			return err
		}
	}
	// The connection's account signs when no account is set (so clearing the
	// account hands signing to it), and is always checked when the write
	// names the connection.
	if c.connectionID != "" && (c.connectionChanged || (c.serviceAcctID == "" && (c.urlChanged || c.accountChanged))) {
		conn, err := r.Connections.FindByID(ctx, c.connectionID)
		if err != nil {
			return usecase.Internal("REPO", "find_by_id(connection) failed", err)
		}
		if conn == nil {
			if c.connectionChanged {
				return usecase.NotFound("CONNECTION_NOT_FOUND", "Connection '"+c.connectionID+"' not found")
			}
		} else if err := r.wield(ctx, ac, conn.ServiceAccountID, !c.connectionChanged); err != nil {
			return err
		}
	}

	// Nothing names an account: the application's own account signs. Any
	// change that reaches here (a URL moved, or the account or connection
	// just cleared) leaves the row signed by it.
	if c.serviceAcctID == "" && c.connectionID == "" && c.applicationCode != nil {
		app, err := r.Apps.FindByCode(ctx, *c.applicationCode)
		if err != nil {
			return usecase.Internal("REPO", "find_application_by_code failed", err)
		}
		if app == nil {
			return nil // no application, no account: the delivery goes out bare
		}
		return saops.MayWieldApplicationIdentity(ac, string(app.ID))
	}
	return nil
}

// wield loads the account and applies the rule. lenient (the write did not
// name this account) skips the missing/inactive refusals (see check), but an
// inactive account is still checked for scope: it can be reactivated later,
// and the URL set now is where its credentials would then go.
func (r CredentialRepos) wield(ctx context.Context, ac *auth.AuthContext, id string, lenient bool) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	sa, err := r.Accounts.FindByID(ctx, id)
	if err != nil {
		return usecase.Internal("REPO", "service account lookup failed", err)
	}
	if lenient && sa == nil {
		return nil
	}
	if lenient && !sa.Active {
		asActive := *sa
		asActive.Active = true
		return saops.MayWieldServiceAccount(ac, &asActive)
	}
	return saops.MayWieldServiceAccount(ac, sa)
}
