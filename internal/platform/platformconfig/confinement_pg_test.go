//go:build integration

package platformconfig_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// confinementFixture is one application with three clients, of which it is
// enabled for two (a, b) and disabled for one (c), a service account bound to
// it with no explicit clients, one bound to it with client c explicitly, and a
// value for each client and a global one.
type confinementFixture struct {
	svc               *platformconfig.Service
	app, appID        string
	a, b, c           string // client ids
	aIdent            string
	saApp, saExplicit *auth.AuthContext
	appSvcPermissions []string
}

func newConfinementFixture(t *testing.T, app string) *confinementFixture {
	t.Helper()
	ctx := context.Background()
	pool := testpg.Pool(t)
	key, _ := encryption.GenerateKey()
	enc, _ := encryption.New(key)
	repo := platformconfig.NewRepository(pool).WithEncryption(enc)
	f := &confinementFixture{
		svc: &platformconfig.Service{Repo: repo, UoW: testpg.NewUoW(t), Clients: client.NewRepository(pool), Audit: audit.NewRepository(pool)},
		app: app, appID: testpg.EnsureApplication(t, app),
		appSvcPermissions: []string{
			"platform:application-service:config:view", "platform:application-service:config:update", "platform:application-service:config:reveal",
		},
	}
	clientIDs := make([]string, 3)
	for i, enabled := range []bool{true, true, false} {
		id := tsid.Generate(tsid.Client)
		ident := strings.ToLower(app) + "-" + string(rune('a'+i))
		_, err := pool.Exec(ctx, `INSERT INTO tnt_clients (id, name, identifier) VALUES ($1, $2, $2)`, id, ident)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO app_client_configs (id, application_id, client_id, enabled) VALUES ($1, $2, $3, $4)`,
			tsid.Generate(tsid.AppClientConfig), f.appID, id, enabled)
		require.NoError(t, err)
		clientIDs[i] = id
		if i == 0 {
			f.aIdent = ident
		}
	}
	f.a, f.b, f.c = clientIDs[0], clientIDs[1], clientIDs[2]
	newSA := func(code string, clients []string) *auth.AuthContext {
		saID, prnID := tsid.Generate(tsid.ServiceAccount), tsid.Generate(tsid.Principal)
		_, err := pool.Exec(ctx, `INSERT INTO iam_service_accounts (id, code, name, client_ids, application_id) VALUES ($1, $2, $2, $3, $4)`,
			saID, code, clients, f.appID)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO iam_principals (id, type, scope, name, active, service_account_id) VALUES ($1, 'SERVICE', 'ANCHOR', $2, true, $3)`,
			prnID, code, saID)
		require.NoError(t, err)
		return &auth.AuthContext{
			PrincipalID: prnID, Scope: auth.ScopeAnchor, Applications: []string{f.appID},
			Permissions: f.appSvcPermissions, PrincipalType: auth.PrincipalTypeService,
		}
	}
	f.saApp = newSA("app:"+app, nil)
	f.saExplicit = newSA("app:"+app+"-explicit", []string{f.c})

	secret := "SECRET"
	for _, cid := range []*string{nil, &f.a, &f.b, &f.c} {
		_, err := usecaseop.Run(adminCtx(), f.svc.UoW, operations.SetProperty(repo), operations.SetPropertyCommand{
			ApplicationCode: app, Section: "secrets", Property: "api-key", Value: "v-" + deref(cid), ValueType: &secret, ClientID: cid,
		}, testpg.TestEC())
		require.NoError(t, err)
	}
	return f
}

// adminCtx is a full platform administrator: anchor tier, the super-admin
// wildcard, and all-applications access (testpg.AnchorCtx is anchor tier
// without all-applications access, which platform config confines to the
// applications it is bound to).
func adminCtx() context.Context {
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_optestrunner1", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:*:*:*"},
	})
}

func deref(s *string) string {
	if s == nil {
		return "global"
	}
	return *s
}

func listedClients(t *testing.T, rows []platformconfig.Config) []string {
	t.Helper()
	out := make([]string, 0, len(rows))
	for i := range rows {
		out = append(out, deref(ids.StringPtr(rows[i].ClientID)))
	}
	return out
}

func (f *confinementFixture) set(ac *auth.AuthContext, clientID *string, value string) error {
	plain := "PLAIN"
	return func() error {
		_, err := usecaseop.Run(testpg.WithAuth(context.Background(), ac), f.svc.UoW, operations.SetProperty(f.svc.Repo), operations.SetPropertyCommand{
			ApplicationCode: f.app, Section: "settings", Property: "colour", Value: value, ValueType: &plain, ClientID: clientID,
		}, testpg.TestEC())
		return err
	}()
}

// An application's service account with no explicit clients reaches the
// clients its application is enabled for, and the application-wide value: on
// list, get, set, delete and reveal.
func TestConfinement_ServiceAccountReachesTheApplicationsEnabledClients(t *testing.T) {
	ctx := context.Background()
	f := newConfinementFixture(t, "pcconf-enabled")
	sa := f.saApp

	rows, err := f.svc.List(ctx, sa, f.app)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"global", f.a, f.b}, listedClients(t, rows), "the disabled client's value is not listed")

	r, err := f.svc.Reveal(ctx, sa, f.app, "secrets", "api-key", f.aIdent)
	require.NoError(t, err, "an enabled client, named by its identifier")
	assert.Equal(t, "v-"+f.a, r.Value)
	_, err = f.svc.Reveal(ctx, sa, f.app, "secrets", "api-key", "")
	require.NoError(t, err, "the application-wide value")
	_, err = f.svc.Reveal(ctx, sa, f.app, "secrets", "api-key", f.c)
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")

	_, err = f.svc.Get(ctx, sa, f.app, "secrets", "api-key", f.c)
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
	_, err = f.svc.Get(ctx, sa, f.app, "secrets", "api-key", f.b)
	require.NoError(t, err)

	require.NoError(t, f.set(sa, &f.b, "blue"))
	require.NoError(t, f.set(sa, nil, "green"), "its application's global value")
	testpg.RequireUsecaseError(t, f.set(sa, &f.c, "red"), usecase.KindAuthorization, "FORBIDDEN")

	err = f.svc.Delete(ctx, sa, f.app, "secrets", "api-key", f.c)
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
	require.NoError(t, f.svc.Delete(ctx, sa, f.app, "secrets", "api-key", f.b))
}

// A service account with explicit client ids reaches those, not the clients
// its application is enabled for.
func TestConfinement_ServiceAccountExplicitClientsWin(t *testing.T) {
	ctx := context.Background()
	f := newConfinementFixture(t, "pcconf-explicit")
	sa := f.saExplicit

	rows, err := f.svc.List(ctx, sa, f.app)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"global", f.c}, listedClients(t, rows))
	_, err = f.svc.Reveal(ctx, sa, f.app, "secrets", "api-key", f.c)
	require.NoError(t, err)
	_, err = f.svc.Reveal(ctx, sa, f.app, "secrets", "api-key", f.a)
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
	testpg.RequireUsecaseError(t, f.set(sa, &f.a, "red"), usecase.KindAuthorization, "FORBIDDEN")
	require.NoError(t, f.set(sa, &f.c, "red"))
}

// An anchor platform administrator is not confined to clients.
func TestConfinement_AnchorAdministratorUnchanged(t *testing.T) {
	ctx := context.Background()
	f := newConfinementFixture(t, "pcconf-admin")
	rows, err := f.svc.List(ctx, revealer(), f.app)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"global", f.a, f.b, f.c}, listedClients(t, rows))
	_, err = f.svc.Reveal(ctx, revealer(), f.app, "secrets", "api-key", f.c)
	require.NoError(t, err)
}

// Below anchor tier the per-role grants (which have no client axis) reach only
// the caller's own clients: other clients' plain values are not listed, other
// clients' rows and the application-wide value cannot be set or deleted.
func TestConfinement_GrantsBelowAnchorReachOwnClientsOnly(t *testing.T) {
	ctx := context.Background()
	f := newConfinementFixture(t, "pcconf-grant")
	_, err := usecaseop.Run(adminCtx(), f.svc.UoW, operations.GrantAccess(f.svc.Repo),
		operations.GrantAccessCommand{ApplicationCode: f.app, RoleCode: "pcconf-grant:editor", CanWrite: true}, testpg.TestEC())
	require.NoError(t, err)
	tenant := &auth.AuthContext{PrincipalID: "prn_pcconfgrant01", Scope: auth.ScopeClient, Clients: []string{f.a}, Roles: []string{"pcconf-grant:editor"}}

	require.NoError(t, f.set(testpgAnchor(), &f.b, "b-plain"))
	rows, err := f.svc.List(ctx, tenant, f.app)
	require.NoError(t, err)
	assert.NotContains(t, listedClients(t, rows), f.b, "another client's values are not listed")
	assert.Contains(t, listedClients(t, rows), f.a)
	assert.Contains(t, listedClients(t, rows), "global")
	_, err = f.svc.Get(ctx, tenant, f.app, "settings", "colour", f.b)
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")

	require.NoError(t, f.set(tenant, &f.a, "mine"))
	testpg.RequireUsecaseError(t, f.set(tenant, &f.b, "theirs"), usecase.KindAuthorization, "FORBIDDEN")
	testpg.RequireUsecaseError(t, f.set(tenant, nil, "everyone's"), usecase.KindAuthorization, "FORBIDDEN")
	testpg.RequireUsecaseError(t, f.svc.Delete(ctx, tenant, f.app, "settings", "colour", f.b), usecase.KindAuthorization, "FORBIDDEN")
	testpg.RequireUsecaseError(t, f.svc.Delete(ctx, tenant, f.app, "secrets", "api-key", ""), usecase.KindAuthorization, "FORBIDDEN")
	require.NoError(t, f.svc.Delete(ctx, tenant, f.app, "settings", "colour", f.a))
}

func testpgAnchor() *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_optestrunner1", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:*:*:*"}}
}

// A caller without write access learns nothing about which clients exist: the
// application rule is decided before the client is resolved, so a real and an
// unknown client get the same 403.
func TestConfinement_NoClientExistenceOracle(t *testing.T) {
	ctx := context.Background()
	f := newConfinementFixture(t, "pcconf-oracle")
	outsider := &auth.AuthContext{PrincipalID: "prn_pcconforacle", Scope: auth.ScopeClient, Clients: []string{f.a}, Roles: []string{"nobody"}}
	for _, ref := range []string{f.aIdent, "no-such-client"} {
		err := f.svc.Delete(ctx, outsider, f.app, "secrets", "api-key", ref)
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
		_, err = f.svc.Get(ctx, outsider, f.app, "secrets", "api-key", ref)
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
	}
	// An authorised caller is told the client does not exist.
	err := f.svc.Delete(ctx, revealer(), f.app, "secrets", "api-key", "no-such-client")
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Client_NOT_FOUND")
}

// Deleting a property is a use case: an audit row (with the service account's
// on-behalf-of) and a property-deleted event.
func TestDelete_IsAuditedWithAnEvent(t *testing.T) {
	ctx := context.Background()
	f := newConfinementFixture(t, "pcconf-delaudit")
	c, err := f.svc.Repo.FindByCoordinate(ctx, f.app, "secrets", "api-key", platformconfig.ScopeClient, &f.a)
	require.NoError(t, err)
	require.NotNil(t, c)

	require.NoError(t, f.svc.DeleteProperty(ctx, f.saApp, platformconfig.DeleteRequest{
		ApplicationCode: f.app, Section: "secrets", Property: "api-key", ClientRef: f.a, OnBehalfOf: "alice@example.com",
	}))
	pool := testpg.Pool(t)
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM aud_logs WHERE entity_id = $1 AND operation = 'DeletePropertyCommand'
		AND principal_id = $2 AND operation_json->>'onBehalfOf' = 'alice@example.com' AND operation_json->>'clientId' = $3`,
		c.ID, f.saApp.PrincipalID, f.a).Scan(&n))
	assert.Equal(t, 1, n, "one audit row naming the person")
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM msg_events WHERE type = $1 AND subject = $2`,
		platformconfig.PropertyDeletedType, "platform.platformconfig."+c.ID).Scan(&n))
	assert.Equal(t, 1, n, "one property-deleted event")
}

// A reveal records the on-behalf-of in its audit row.
func TestReveal_RecordsOnBehalfOf(t *testing.T) {
	ctx := context.Background()
	f := newConfinementFixture(t, "pcconf-revobo")
	r, err := f.svc.RevealFor(ctx, f.saApp, f.app, "secrets", "api-key", f.a, "bob@example.com")
	require.NoError(t, err)
	var n int
	require.NoError(t, testpg.Pool(t).QueryRow(ctx, `SELECT count(*) FROM aud_logs WHERE entity_id = $1 AND operation = 'SECRET_REVEALED'
		AND operation_json->>'onBehalfOf' = 'bob@example.com'`, r.Config.ID).Scan(&n))
	assert.Equal(t, 1, n)
}

// Writing the mask back as a secret's value is refused, whether the type is
// named or kept; so is an empty secret. The stored secret is unchanged.
func TestSetProperty_RefusesTheMaskAsASecret(t *testing.T) {
	ctx := context.Background()
	f := newConfinementFixture(t, "pcconf-mask")
	secret := "SECRET"
	for name, cmd := range map[string]operations.SetPropertyCommand{
		"named SECRET": {ApplicationCode: f.app, Section: "secrets", Property: "api-key", Value: "***", ValueType: &secret},
		"type kept":    {ApplicationCode: f.app, Section: "secrets", Property: "api-key", Value: "***"},
	} {
		_, err := usecaseop.Run(adminCtx(), f.svc.UoW, operations.SetProperty(f.svc.Repo), cmd, testpg.TestEC())
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SECRET_VALUE_MASKED")
		_ = name
	}
	_, err := usecaseop.Run(adminCtx(), f.svc.UoW, operations.SetProperty(f.svc.Repo), operations.SetPropertyCommand{
		ApplicationCode: f.app, Section: "secrets", Property: "api-key", Value: "", ValueType: &secret,
	}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SECRET_VALUE_REQUIRED")
	r, err := f.svc.Reveal(ctx, revealer(), f.app, "secrets", "api-key", "")
	require.NoError(t, err)
	assert.Equal(t, "v-global", r.Value)
}

// Config may be set only for an application that exists, so no value waits
// for a future application with that code.
func TestSetProperty_RefusesAnUnknownApplication(t *testing.T) {
	uow := testpg.NewUoW(t)
	repo := platformconfig.NewRepository(testpg.Pool(t))
	_, err := usecaseop.Run(adminCtx(), uow, operations.SetProperty(repo), operations.SetPropertyCommand{
		ApplicationCode: "pcconf-no-such-app", Section: "s", Property: "p", Value: "v",
	}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Application_NOT_FOUND")
	_, err = usecaseop.Run(adminCtx(), uow, operations.GrantAccess(repo),
		operations.GrantAccessCommand{ApplicationCode: "pcconf-no-such-app", RoleCode: "r"}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Application_NOT_FOUND")
}
