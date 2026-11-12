//go:build integration

package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/secretref"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// PUT /api/config/...: a caller without write access to the application gets
// the same 403 for a client that exists and one that does not (the
// application is authorised before the client is resolved); an authorised
// caller is told an unknown client is not found.
func TestSetProperty_NoClientExistenceOracle(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	testpg.EnsureApplication(t, "pcapi-oracle")
	_, err := pool.Exec(ctx, `INSERT INTO tnt_clients (id, name, identifier) VALUES ($1, 'Real', 'pcapi-real')`, tsid.Generate(tsid.Client))
	require.NoError(t, err)
	s := &State{Svc: &platformconfig.Service{
		Repo: platformconfig.NewRepository(pool), UoW: testpg.NewUoW(t), Clients: client.NewRepository(pool), Audit: audit.NewRepository(pool),
	}}
	put := func(ac *auth.AuthContext, clientRef string) error {
		_, err := s.setProperty(testpg.WithAuth(ctx, ac), &setPropertyInput{
			App: "pcapi-oracle", Section: "s", Property: "p", ClientID: clientRef, Body: SetPropertyRequest{Value: "v"},
		})
		return err
	}
	outsider := &auth.AuthContext{PrincipalID: "prn_pcapioutside", Scope: auth.ScopeClient, Roles: []string{"none"}}
	for _, ref := range []string{"pcapi-real", "pcapi-no-such"} {
		testpg.RequireUsecaseError(t, put(outsider, ref), usecase.KindAuthorization, "FORBIDDEN")
	}
	admin := &auth.AuthContext{PrincipalID: "prn_pcapiadmin01", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:admin:config:update"}}
	testpg.RequireUsecaseError(t, put(admin, "pcapi-no-such"), usecase.KindNotFound, "Client_NOT_FOUND")
	require.NoError(t, put(admin, "pcapi-real"))
}

// GET /api/platform-config/{app}: a client row names the client both ways
// (its stored id and its identifier), so a caller holding either can match it.
func TestListProperties_NamesTheClientIdentifier(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	testpg.EnsureApplication(t, "pcapi-list")
	cid := tsid.Generate(tsid.Client)
	_, err := pool.Exec(ctx, `INSERT INTO tnt_clients (id, name, identifier) VALUES ($1, 'Listed', 'pcapi-listed')`, cid)
	require.NoError(t, err)
	s := &State{Svc: &platformconfig.Service{
		Repo: platformconfig.NewRepository(pool), UoW: testpg.NewUoW(t), Clients: client.NewRepository(pool), Audit: audit.NewRepository(pool),
	}}
	admin := &auth.AuthContext{PrincipalID: "prn_pcapiadmin02", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:admin:config:update", "platform:admin:config:view"}}
	actx := testpg.WithAuth(ctx, admin)
	for _, ref := range []string{"", "pcapi-listed"} {
		_, err := s.setProperty(actx, &setPropertyInput{App: "pcapi-list", Section: "s", Property: "p", ClientID: ref, Body: SetPropertyRequest{Value: "v"}})
		require.NoError(t, err)
	}
	out, err := s.listProperties(actx, &listPropsInput{App: "pcapi-list"})
	require.NoError(t, err)
	require.Len(t, out.Body.Items, 2)
	for _, r := range out.Body.Items {
		if r.ClientID == nil {
			require.Nil(t, r.ClientIdentifier, "the global row names no client")
			continue
		}
		require.Equal(t, cid, *r.ClientID)
		require.NotNil(t, r.ClientIdentifier)
		require.Equal(t, "pcapi-listed", *r.ClientIdentifier)
	}
}

// refsFake resolves a reference to a value, or to the failure "ERR:<code>".
type refsFake map[string]string

func (m refsFake) Resolve(_ context.Context, ref string, _ secretref.Owner) (string, error) {
	v, ok := m[ref]
	switch {
	case !ok:
		return "", &secretref.Error{Code: secretref.CodeNotFound, Ref: ref, Msg: "no such secret"}
	case len(v) > 4 && v[:4] == "ERR:":
		return "", &secretref.Error{Code: v[4:], Ref: ref, Msg: "fake failure"}
	}
	return v, nil
}

// A SECRET that is a secret reference: the list shows the reference where a
// secret is masked, the reveal endpoint answers the secret it points at, and a
// reference that does not resolve is 409 with its code, or 503
// SECRET_REFERENCE_UNAVAILABLE when the secret store cannot be reached.
func TestSecretReference_ListShowsReferenceAndRevealResolves(t *testing.T) {
	// Not parallel: it swaps the process's resolver (the write check).
	restore := secretref.SetDefault(secretref.New(secretref.Config{Allow: []string{"aws-sm://prod/"}}, nil))
	defer restore()
	ctx := context.Background()
	pool := testpg.Pool(t)
	testpg.EnsureApplication(t, "pcapi-refs")
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	enc, err := encryption.New(key)
	require.NoError(t, err)
	s := &State{Svc: &platformconfig.Service{
		Repo: platformconfig.NewRepository(pool).WithEncryption(enc), UoW: testpg.NewUoW(t), Clients: client.NewRepository(pool), Audit: audit.NewRepository(pool),
		Refs: refsFake{"aws-sm://prod/a#k": "resolved-value", "aws-sm://prod/down": "ERR:" + secretref.CodeUnavailable},
	}}
	admin := &auth.AuthContext{
		PrincipalID: "prn_pcapiadmin03", Scope: auth.ScopeAnchor, AllApplications: true,
		Permissions: []string{"platform:admin:config:update", "platform:admin:config:view", "platform:admin:config:reveal"},
	}
	actx := testpg.WithAuth(ctx, admin)
	secret := "SECRET"
	for prop, v := range map[string]string{"ok": "aws-sm://prod/a#k", "down": "aws-sm://prod/down", "gone": "aws-sm://prod/gone", "plain": "plain-secret"} {
		_, err := s.setProperty(actx, &setPropertyInput{App: "pcapi-refs", Section: "s", Property: prop, Body: SetPropertyRequest{Value: v, ValueType: &secret}})
		require.NoError(t, err, prop)
	}
	_, err = s.setProperty(actx, &setPropertyInput{App: "pcapi-refs", Section: "s", Property: "bad", Body: SetPropertyRequest{Value: "aws-sm://elsewhere/x", ValueType: &secret}})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, secretref.CodeNotAllowed)

	out, err := s.listProperties(actx, &listPropsInput{App: "pcapi-refs"})
	require.NoError(t, err)
	shown := map[string]string{}
	for _, r := range out.Body.Items {
		shown[r.Property] = r.Value
	}
	require.Equal(t, "aws-sm://prod/a#k", shown["ok"])
	require.Equal(t, "***", shown["plain"])

	rev, err := s.revealSecret(actx, &propertyActInput{App: "pcapi-refs", Section: "s", Property: "ok"})
	require.NoError(t, err)
	require.Equal(t, "resolved-value", rev.Body.Value)

	_, err = s.revealSecret(actx, &propertyActInput{App: "pcapi-refs", Section: "s", Property: "gone"})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, secretref.CodeNotFound)

	_, err = s.revealSecret(actx, &propertyActInput{App: "pcapi-refs", Section: "s", Property: "down"})
	var em *httpcompat.ErrorModel
	require.ErrorAs(t, err, &em)
	require.Equal(t, http.StatusServiceUnavailable, em.GetStatus())
	require.Equal(t, secretref.CodeUnavailable, em.Code)
}
