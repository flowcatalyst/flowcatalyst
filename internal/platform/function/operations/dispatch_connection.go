package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// DispatchSignerNotAllowed is the refusal of a dispatch declaration whose
// connection signs with a service account the function may not sign as
// (owner's decision D4, 2026-11-22): 403, before anything is written.
const DispatchSignerNotAllowed = "DISPATCH_SIGNER_NOT_ALLOWED"

// DispatchConnections resolves the connection a function's dispatch
// declaration names and checks that the function may sign as the
// connection's service account. The dispatch control route uses it on every
// call; Publish uses it on every declared dispatch whose connection exists
// when the version is published.
type DispatchConnections struct {
	Connections *connection.Repository
	Accounts    *serviceaccount.Repository
}

// Find maps a declared connection code to the connection a job of fn goes
// through, or nil when none is visible to fn. Visibility, most specific
// first: the function's application's connection of the function's client,
// then of no client, then the shared (application-less) connection of the
// client, then the shared client-less one. A connection of another
// application or another client is never matched; the first match decides
// (no fall-through past a refusal). Its status is not asked: a PAUSED
// connection holds its jobs.
func (d DispatchConnections) Find(ctx context.Context, fn *function.Function, code string) (*connection.Connection, error) {
	if d.Connections == nil {
		return nil, usecase.Internal("DISPATCH_UNWIRED", "the connection repository is not configured", nil)
	}
	apps := []*string{&fn.ApplicationCode, nil}
	clients := []*string{nil}
	if fn.ClientID != nil {
		clients = []*string{fn.ClientID, nil}
	}
	for _, app := range apps {
		for _, client := range clients {
			c, err := d.Connections.FindByCode(ctx, code, app, client)
			if err != nil {
				return nil, usecase.Internal("REPO", "find connection failed", err)
			}
			if c != nil {
				return c, nil
			}
		}
	}
	return nil, nil
}

// SigningIdentityOf is who a function's dispatched work is done as: its
// application, and its own service account (its webhook verifier) when it
// sets one.
func SigningIdentityOf(fn *function.Function) serviceaccount.SigningIdentity {
	id := serviceaccount.SigningIdentity{ApplicationID: fn.ApplicationID}
	if fn.ServiceAccountID != nil {
		id.OwnAccountID = *fn.ServiceAccountID
	}
	return id
}

// CheckSigner refuses 403 DISPATCH_SIGNER_NOT_ALLOWED when the service
// account conn signs with is one fn may not sign as
// (serviceaccount.SigningIdentity.SigningProblem). A connection that names no
// account, or an account that does not exist, signs nothing (the delivery is
// declined with the reason) and is not refused here.
func (d DispatchConnections) CheckSigner(ctx context.Context, fn *function.Function, conn *connection.Connection) error {
	if conn.ServiceAccountID == "" {
		return nil
	}
	if d.Accounts == nil {
		return usecase.Internal("DISPATCH_UNWIRED", "the service account repository is not configured", nil)
	}
	signer, err := d.Accounts.FindSigner(ctx, conn.ServiceAccountID)
	if err != nil {
		return usecase.Internal("REPO", "find service account failed", err)
	}
	if problem := SigningIdentityOf(fn).SigningProblem(signer); problem != "" {
		return usecase.Authorization(DispatchSignerNotAllowed,
			"connection "+conn.Code+" signs its jobs with an account function "+fn.Address+" may not sign as: "+problem+
				"; a function's dispatch may name only a connection whose account is its application's or its own")
	}
	return nil
}
