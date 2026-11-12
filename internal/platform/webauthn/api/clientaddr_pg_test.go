//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/loginbackoff"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/loginattempt"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/clientaddr"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The passkey path keys its attempt record and its backoff on the address the
// edge resolved (carried in the request context), not on a header.
func TestPasskeyAttemptAddressComesFromContext(t *testing.T) {
	r := newRig(t, "caddr")
	pool := testpg.Pool(t)
	r.s.LoginAttempts = loginattempt.NewRepository(pool)
	r.s.Principals = principal.NewRepository(pool)
	r.s.BackoffPolicy = loginbackoff.Policy{FreeAttempts: 0, BaseDelaySecs: 3600, MaxDelaySecs: 3600}
	alice := &principal.Principal{ID: r.alice.ID, Active: true, UserIdentity: &principal.UserIdentity{Email: "alice@caddr.test"}}

	atkCtx := clientaddr.With(context.Background(), "203.0.113.7")
	r.s.recordPasskeyAttempt(atkCtx, alice, &authenticateCompleteInput{}, loginattempt.OutcomeFailure, "bad")

	var ip *string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT ip_address FROM iam_login_attempts WHERE identifier = 'alice@caddr.test'`).Scan(&ip))
	require.NotNil(t, ip)
	require.Equal(t, "203.0.113.7", *ip)

	// The backoff is per (email, address): that address is held back, another is not.
	_, err := r.s.authenticateBegin(atkCtx, &authenticateBeginInput{Body: AuthenticateBeginRequest{Email: "alice@caddr.test"}})
	require.Error(t, err)
	other := clientaddr.With(context.Background(), "198.51.100.9")
	_, err = r.s.authenticateBegin(other, &authenticateBeginInput{Body: AuthenticateBeginRequest{Email: "alice@caddr.test"}})
	require.NoError(t, err)
}
