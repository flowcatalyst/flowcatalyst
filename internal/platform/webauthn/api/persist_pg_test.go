//go:build integration

package api

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/loginbackoff"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn/webauthntest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

const (
	rpID   = "localhost"
	origin = "http://localhost:8080"
)

type rig struct {
	t     *testing.T
	s     *State
	alice *principal.Principal
	bob   *principal.Principal
}

func newRig(t *testing.T, tag string) *rig {
	t.Helper()
	ctx := context.Background()
	pool := testpg.Pool(t)
	mk := func(id, name string) *principal.Principal {
		_, err := pool.Exec(ctx,
			`INSERT INTO iam_principals (id, type, scope, name, active, email)
			 VALUES ($1, 'USER', 'ANCHOR', $2, TRUE, $3)`, id, name, name+"@"+tag+".test")
		require.NoError(t, err)
		return &principal.Principal{ID: ids.PrincipalID(id), Name: name, Active: true}
	}
	svc, err := webauthn.NewService(webauthn.Config{RPDisplayName: "FC", RPID: rpID, RPOrigins: []string{origin}},
		webauthn.NewRepository(pool), webauthn.NewCeremonyRepository(pool))
	require.NoError(t, err)
	return &rig{
		t: t,
		s: &State{
			Service: svc, Creds: webauthn.NewRepository(pool), UoW: testpg.NewUoW(t),
			BackoffPolicy: loginbackoff.Policy{},
		},
		alice: mk("prn_"+tag+"al", "alice"),
		bob:   mk("prn_"+tag+"bo", "bob"),
	}
}

// register runs the real registration finish path for p and returns the
// stored credential id plus the software authenticator.
func (r *rig) register(p *principal.Principal) (string, *webauthntest.Authenticator) {
	r.t.Helper()
	ctx := auth.WithContext(context.Background(), &auth.AuthContext{PrincipalID: string(p.ID)})
	a := webauthntest.New(r.t)
	user := &webauthn.PrincipalUser{PrincipalID: string(p.ID), DisplayName: p.Name}
	opts, sess, err := r.s.Service.WebAuthn().BeginRegistration(user)
	require.NoError(r.t, err)
	parsed, err := protocol.ParseCredentialCreationResponseBody(io.NopCloser(bytes.NewReader(
		a.Register(opts.Response.Challenge.String(), origin, rpID))))
	require.NoError(r.t, err)
	cred, err := r.s.Service.WebAuthn().CreateCredential(user, *sess, parsed)
	require.NoError(r.t, err)
	name := "key"
	ev, err := usecaseop.Run(ctx, r.s.UoW, operations.Register(r.s.Creds),
		operations.RegisterCommand{StateID: "s", Response: *cred, Name: &name}, usecase.NewExecutionContext(string(p.ID)))
	require.NoError(r.t, err)
	return ev.CredentialID, a
}

func (r *rig) assert(p *principal.Principal, a *webauthntest.Authenticator, counter uint32) (*webauthn.Credential, bool) {
	r.t.Helper()
	ctx := context.Background()
	creds, err := r.s.Service.Credentials().LibraryCredentialsByPrincipal(ctx, string(p.ID))
	require.NoError(r.t, err)
	user := &webauthn.PrincipalUser{PrincipalID: string(p.ID), DisplayName: p.Name, Credentials: creds}
	_, sess, err := r.s.Service.WebAuthn().BeginLogin(user)
	require.NoError(r.t, err)
	parsed, err := protocol.ParseCredentialRequestResponseBody(io.NopCloser(bytes.NewReader(
		a.AssertWithCounter(sess.Challenge, origin, rpID, []byte(p.ID), counter))))
	require.NoError(r.t, err)
	lib, err := r.s.Service.WebAuthn().ValidateLogin(user, *sess, parsed)
	require.NoError(r.t, err)
	r.s.persistAssertion(ctx, p, "state", lib)
	stored, err := r.s.Creds.FindByCredentialID(ctx, a.CredID)
	require.NoError(r.t, err)
	return stored, lib.Authenticator.CloneWarning
}

// The regression this pins: the signature counter and last_used_at were never
// stored (the persist call's own validation rejected it and the error was
// dropped), so counter regression could not be detected across logins.
func TestPersistAssertion_StoresCounterAndLastUsed(t *testing.T) {
	r := newRig(t, "pcnt")
	_, a := r.register(r.alice)

	stored, clone := r.assert(r.alice, a, 7)
	assert.False(t, clone)
	require.NotNil(t, stored)
	assert.EqualValues(t, 7, stored.Credential.Authenticator.SignCount, "counter must be persisted")
	assert.NotNil(t, stored.LastUsedAt, "last_used_at must be persisted")

	// The next assertion with a non-advancing counter is now detectable.
	_, clone = r.assert(r.alice, a, 3)
	assert.True(t, clone, "counter regression must be flagged against the stored counter")
}

// A credential asserted for another principal is never written to.
func TestPersistAssertion_RefusesAnotherPrincipalsCredential(t *testing.T) {
	r := newRig(t, "powner")
	_, a := r.register(r.alice)
	ctx := context.Background()

	stored, err := r.s.Creds.FindByCredentialID(ctx, a.CredID)
	require.NoError(t, err)
	before := stored.Credential.Authenticator.SignCount
	lib := stored.Credential
	lib.Authenticator.SignCount = 99
	r.s.persistAssertion(ctx, r.bob, "state", &lib) // bob asserts alice's credential id

	after, err := r.s.Creds.FindByCredentialID(ctx, a.CredID)
	require.NoError(t, err)
	assert.Equal(t, before, after.Credential.Authenticator.SignCount, "alice's credential must not be modified on bob's behalf")
}

func TestDeleteCredential_OnlyOwnerMayRevoke(t *testing.T) {
	r := newRig(t, "pdel")
	credID, _ := r.register(r.alice)

	bobCtx := auth.WithContext(context.Background(), &auth.AuthContext{PrincipalID: string(r.bob.ID)})
	_, err := r.s.deleteCredential(bobCtx, &deleteCredInput{ID: credID})
	require.Error(t, err, "another principal must not revoke the credential")
	still, err := r.s.Creds.FindByID(context.Background(), credID)
	require.NoError(t, err)
	assert.NotNil(t, still)

	aliceCtx := auth.WithContext(context.Background(), &auth.AuthContext{PrincipalID: string(r.alice.ID)})
	_, err = r.s.deleteCredential(aliceCtx, &deleteCredInput{ID: credID})
	require.NoError(t, err)
	gone, err := r.s.Creds.FindByID(context.Background(), credID)
	require.NoError(t, err)
	assert.Nil(t, gone)
}
