package sdk

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// JobConnections resolves and checks the connectionId an SDK-created dispatch
// job names, memoised for the life of one request (a batch of jobs for one
// connection costs one lookup). Not safe for concurrent use; build one per
// request.
//
// The connection is the job's own (connection_id), as a function's
// fn.dispatch job's is: pausing it holds the job PENDING, and with no
// subscription it is the connection's service account that signs the
// delivery (internal/server/delivery_creds.go, step 2b). The rules:
//
//   - it must exist: 400 CONNECTION_NOT_FOUND;
//   - it must be in the job's scope, the rule a subscription's connectionId
//     is held to (connection.ScopeProblem): global or the job's own client,
//     shared or the job's own application (the first segment of its code).
//     Because the job's client and application are themselves checked
//     against the caller (ResolveJobClient, checkJobApplication), the
//     connection is one the caller reaches: 400 CONNECTION_SCOPE_MISMATCH;
//   - if the job also names a subscription, the subscription's connection must
//     be this one (a subscription's job is signed through the subscription,
//     so a different connectionId would be silently ignored): 400
//     CONNECTION_MISMATCH;
//   - its status is NOT checked: a PAUSED connection holds work, as for
//     fn.dispatch, so the job is created and waits.
type JobConnections struct {
	conns *connection.Repository
	subs  *subscription.Repository
	cache map[string]*connection.Connection // nil value = looked up, none
}

// newJobConnections builds a per-request resolver over s's repositories. A nil
// connection repository resolves every id as unknown; a nil subscription
// repository every subscription.
func (s *DispatchJobsBatchState) newJobConnections() *JobConnections {
	return &JobConnections{conns: s.Connections, subs: s.Subs, cache: map[string]*connection.Connection{}}
}

// Apply checks connectionID (blank = none) for job j and sets it as the job's
// connection. j's client and code must already be final.
func (c *JobConnections) Apply(ctx context.Context, j *dispatchjob.DispatchJob, connectionID *string) error {
	if NonBlank(connectionID) == nil {
		j.ConnectionID = nil
		return nil
	}
	id := strings.TrimSpace(*connectionID)
	conn, seen := c.cache[id]
	if !seen {
		if c.conns != nil {
			var err error
			if conn, err = c.conns.FindByID(ctx, id); err != nil {
				return usecase.Internal("REPO", "connection find_by_id failed", err)
			}
		}
		c.cache[id] = conn
	}
	if conn == nil {
		return httperror.BadRequest("CONNECTION_NOT_FOUND", "Connection not found: "+id)
	}
	if msg := conn.ScopeProblem(FirstSegment(j.Code), j.ClientID); msg != "" {
		return httperror.BadRequest("CONNECTION_SCOPE_MISMATCH", "connection "+id+" "+msg+" than dispatch job "+j.Code)
	}
	if sub := NonBlank(j.SubscriptionID); sub != nil {
		var subConn *string
		if c.subs != nil {
			found, err := c.subs.FindByID(ctx, *sub)
			if err != nil {
				return usecase.Internal("REPO", "subscription find_by_id failed", err)
			}
			if found != nil {
				subConn = found.ConnectionID
			}
		}
		if subConn == nil || *subConn != conn.ID {
			return httperror.BadRequest("CONNECTION_MISMATCH",
				"connectionId "+id+" is not the connection of subscription "+*sub+"; a subscription's job is delivered through the subscription's connection")
		}
	}
	j.ConnectionID = &conn.ID
	return nil
}
