//go:build integration

package operations_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	edmops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// runAuthorized / runAuthorizedTx drive ops through the full use-case
// envelope as an anchor principal. The coarse anchor-only write permission is
// enforced at the controller, not in the use case.
func runAuthorized[C any, E usecase.DomainEvent](
	uow *usecasepgx.UnitOfWork, op usecaseop.Operation[C, E], cmd C,
) (E, error) {
	return usecaseop.Run(testpg.AnchorCtx(), uow, op, cmd, testpg.TestEC())
}

func runAuthorizedTx[C any, R any](
	uow *usecasepgx.UnitOfWork, op usecaseop.TxOperation[C, R], cmd C,
) (R, error) {
	return usecaseop.RunTx(testpg.AnchorCtx(), uow, op, cmd, testpg.TestEC())
}

// deps assembles the orchestration dependency bundle over the shared pool.
func deps(t *testing.T) operations.Deps {
	t.Helper()
	pool := testpg.Pool(t)
	idpRepo := identityprovider.NewRepository(pool)
	return operations.Deps{
		Repo: idpRepo,
		MoveDeps: edmops.MoveDeps{
			Mappings:   emaildomainmapping.NewRepository(pool),
			IDPs:       idpRepo,
			Principals: principal.NewRepository(pool),
		},
	}
}

// mustCreate seeds an INTERNAL-type IdP through the public operation —
// the same path production uses (INTERNAL needs no OIDC fields). Codes
// are hand-unique per test: the fixture never truncates between tests,
// so tests own their rows and never assert table-wide.
func mustCreate(t *testing.T, d operations.Deps, uow *usecasepgx.UnitOfWork, code, name string) operations.CreateResult {
	t.Helper()
	res, err := runAuthorizedTx(uow, operations.CreateIdentityProvider(d),
		operations.CreateCommand{Code: code, Name: name, Type: "INTERNAL"})
	require.NoError(t, err)
	return res
}

// mustCreateOIDC seeds an OIDC IdP (issuer/client filled with test values).
func mustCreateOIDC(t *testing.T, d operations.Deps, uow *usecasepgx.UnitOfWork, code string, domains []string) operations.CreateResult {
	t.Helper()
	issuer := "https://login." + code + ".example.com/v2.0"
	clientID := code + "-client-id"
	res, err := runAuthorizedTx(uow, operations.CreateIdentityProvider(d), operations.CreateCommand{
		Code: code, Name: code, Type: "OIDC",
		OIDCIssuerURL: &issuer, OIDCClientID: &clientID,
		AllowedEmailDomains: domains,
	})
	require.NoError(t, err)
	return res
}

// ensureInternalIdP guarantees the seeded `internal` provider exists (the
// production seed creates it at boot; the test fixture only runs
// migrations). Race-tolerant: parallel tests may both attempt the create.
func ensureInternalIdP(t *testing.T, d operations.Deps, uow *usecasepgx.UnitOfWork) string {
	t.Helper()
	ctx := context.Background()
	if ip, err := d.Repo.FindByCode(ctx, "internal"); err == nil && ip != nil {
		return ip.ID
	}
	res, err := runAuthorizedTx(uow, operations.CreateIdentityProvider(d),
		operations.CreateCommand{Code: "internal", Name: "Internal Authentication", Type: "INTERNAL"})
	if err == nil {
		return res.IdentityProviderID
	}
	ip, ferr := d.Repo.FindByCode(ctx, "internal")
	require.NoError(t, ferr)
	require.NotNil(t, ip, "internal idp neither creatable nor findable")
	return ip.ID
}

// ── Create ────────────────────────────────────────────────────────────────

func TestCreateIdentityProvider_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := deps(t)
	uow := testpg.NewUoW(t)

	issuer := "https://login.idpcrt.example.com/v2.0"
	clientID := "idpcrt-client-id"
	secretRef := "secret-ref-idpcrt"
	pattern := "https://login\\.idpcrt\\.example\\.com/.*"
	res, err := runAuthorizedTx(uow, operations.CreateIdentityProvider(d), operations.CreateCommand{
		Code:                "idpcrt-happy",
		Name:                "IdP Create Happy",
		Type:                "OIDC",
		OIDCIssuerURL:       &issuer,
		OIDCClientID:        &clientID,
		OIDCClientSecretRef: &secretRef,
		OIDCMultiTenant:     true,
		OIDCIssuerPattern:   &pattern,
		AllowedEmailDomains: []string{"IDPCRT-A.Example.com", "idpcrt-b.example.com"}, // mixed case: op must lowercase
		SyncRolesFromIDP:    true,
		AllowedRoleIDs:      []string{"rol_idpcrtrole1"},
	})
	require.NoError(t, err)

	assert.NotEmpty(t, res.IdentityProviderID)
	assert.Equal(t, "idpcrt-happy", res.Code)
	assert.ElementsMatch(t, []string{"idpcrt-a.example.com", "idpcrt-b.example.com"}, res.DomainsCreated)
	assert.Empty(t, res.DomainsClaimed)

	got, err := d.Repo.FindByID(ctx, res.IdentityProviderID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "idpcrt-happy", got.Code)
	assert.Equal(t, identityprovider.TypeOIDC, got.Type)
	require.NotNil(t, got.OIDCIssuerURL)
	assert.Equal(t, issuer, *got.OIDCIssuerURL)
	require.NotNil(t, got.OIDCClientSecretRef)
	assert.Equal(t, secretRef, *got.OIDCClientSecretRef)
	assert.True(t, got.OIDCMultiTenant)
	assert.True(t, got.SyncRolesFromIDP)
	assert.ElementsMatch(t, []string{"rol_idpcrtrole1"}, got.AllowedRoleIDs)
	// The domain list is derived from the mapping table.
	assert.ElementsMatch(t, []string{"idpcrt-a.example.com", "idpcrt-b.example.com"}, got.AllowedEmailDomains)

	// The orchestration materialised real mappings.
	m, err := d.MoveDeps.Mappings.FindByEmailDomain(ctx, "idpcrt-a.example.com")
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, res.IdentityProviderID, m.IdentityProviderID)
	assert.Equal(t, emaildomainmapping.ScopeAnchor, m.ScopeType)
	assert.Nil(t, m.PrimaryClientID)
}

// A domain already mapped elsewhere is claimed (re-pointed), not duplicated —
// and an existing primary-client link is never overwritten, while a missing
// one is filled from the command.
func TestCreateIdentityProvider_ClaimsExistingDomain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := deps(t)
	uow := testpg.NewUoW(t)

	// Seed two mappings on a plain internal-type IdP: one with a client, one
	// without.
	first := mustCreate(t, d, uow, "idpclaim-src", "Claim Source")
	withClient := "cli_idpclaimkeep"
	_, err := usecaseop.Run(testpg.AnchorCtx(), uow, edmops.CreateMapping(d.MoveDeps.Mappings), edmops.CreateCommand{
		EmailDomain: "idpclaim-hasclient.example.com", IdentityProviderID: first.IdentityProviderID,
		ScopeType: "CLIENT", PrimaryClientID: &withClient,
	}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, edmops.CreateMapping(d.MoveDeps.Mappings), edmops.CreateCommand{
		EmailDomain: "idpclaim-noclient.example.com", IdentityProviderID: first.IdentityProviderID,
		ScopeType: "ANCHOR",
	}, testpg.TestEC())
	require.NoError(t, err)

	newClient := "cli_idpclaimnew"
	issuer := "https://login.idpclaim.example.com/v2.0"
	clientID := "idpclaim-client-id"
	res, err := runAuthorizedTx(uow, operations.CreateIdentityProvider(d), operations.CreateCommand{
		Code: "idpclaim-new", Name: "Claimer", Type: "OIDC",
		OIDCIssuerURL: &issuer, OIDCClientID: &clientID,
		AllowedEmailDomains: []string{"idpclaim-hasclient.example.com", "idpclaim-noclient.example.com", "idpclaim-fresh.example.com"},
		PrimaryClientID:     &newClient,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"idpclaim-fresh.example.com"}, res.DomainsCreated)
	assert.ElementsMatch(t, []string{"idpclaim-hasclient.example.com", "idpclaim-noclient.example.com"}, res.DomainsClaimed)

	kept, err := d.MoveDeps.Mappings.FindByEmailDomain(ctx, "idpclaim-hasclient.example.com")
	require.NoError(t, err)
	require.NotNil(t, kept)
	assert.Equal(t, res.IdentityProviderID, kept.IdentityProviderID)
	require.NotNil(t, kept.PrimaryClientID)
	assert.Equal(t, withClient, *kept.PrimaryClientID, "existing client link must not be overwritten")

	filled, err := d.MoveDeps.Mappings.FindByEmailDomain(ctx, "idpclaim-noclient.example.com")
	require.NoError(t, err)
	require.NotNil(t, filled)
	require.NotNil(t, filled.PrimaryClientID)
	assert.Equal(t, newClient, *filled.PrimaryClientID, "unclaimed mapping takes the command's client")

	fresh, err := d.MoveDeps.Mappings.FindByEmailDomain(ctx, "idpclaim-fresh.example.com")
	require.NoError(t, err)
	require.NotNil(t, fresh)
	assert.Equal(t, emaildomainmapping.ScopeClient, fresh.ScopeType, "client supplied → CLIENT scope on new mappings")
	require.NotNil(t, fresh.PrimaryClientID)
	assert.Equal(t, newClient, *fresh.PrimaryClientID)
}

func TestCreateIdentityProvider_Validation(t *testing.T) {
	t.Parallel()
	d := deps(t)
	uow := testpg.NewUoW(t)

	issuer := "https://login.idpcrt.example.com/v2.0"
	cases := []struct {
		name string
		cmd  operations.CreateCommand
		code string
	}{
		{"empty code", operations.CreateCommand{Name: "X", Type: "INTERNAL"}, "CODE_REQUIRED"},
		{"empty name", operations.CreateCommand{Code: "idpcrt-noname", Type: "INTERNAL"}, "NAME_REQUIRED"},
		{"oidc without issuer", operations.CreateCommand{
			Code: "idpcrt-noissuer", Name: "X", Type: "OIDC",
		}, "OIDC_ISSUER_REQUIRED"},
		{"oidc without client id", operations.CreateCommand{
			Code: "idpcrt-noclient", Name: "X", Type: "OIDC", OIDCIssuerURL: &issuer,
		}, "OIDC_CLIENT_ID_REQUIRED"},
		{"bad domain", operations.CreateCommand{
			Code: "idpcrt-baddomain", Name: "X", Type: "INTERNAL",
			AllowedEmailDomains: []string{"nodot"},
		}, "INVALID_EMAIL_DOMAIN"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorizedTx(uow, operations.CreateIdentityProvider(d), tc.cmd)
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, tc.code)
		})
	}
}

// Conflict is pinned by seeding through the operation itself: the first
// create IS the seed for the second.
func TestCreateIdentityProvider_DuplicateCode_Conflict(t *testing.T) {
	t.Parallel()
	d := deps(t)
	uow := testpg.NewUoW(t)
	mustCreate(t, d, uow, "idpdup", "First")

	_, err := runAuthorizedTx(uow, operations.CreateIdentityProvider(d),
		operations.CreateCommand{Code: "idpdup", Name: "Second", Type: "INTERNAL"})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "CODE_EXISTS")
}

// ── Update ────────────────────────────────────────────────────────────────

func TestUpdateIdentityProvider_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := deps(t)
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, d, uow, "idpupd-happy", "Before")

	newName := "  After  " // op must trim
	issuer := "https://login.idpupd.example.com"
	multiTenant := true
	sync := true
	res, err := runAuthorizedTx(uow, operations.UpdateIdentityProvider(d), operations.UpdateCommand{
		ID:                  seeded.IdentityProviderID,
		Name:                &newName,
		OIDCIssuerURL:       &issuer,
		OIDCMultiTenant:     &multiTenant,
		AllowedEmailDomains: []string{"idpupd.example.com"},
		SyncRolesFromIDP:    &sync,
		AllowedRoleIDs:      []string{"rol_idpupdrole1"},
	})
	require.NoError(t, err)
	assert.Equal(t, seeded.IdentityProviderID, res.IdentityProviderID)
	assert.Equal(t, "idpupd-happy", res.Code)
	assert.ElementsMatch(t, []string{"idpupd.example.com"}, res.DomainsCreated)

	got, err := d.Repo.FindByID(ctx, seeded.IdentityProviderID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "After", got.Name, "name must be trimmed")
	assert.Equal(t, "idpupd-happy", got.Code, "code is immutable on update")
	require.NotNil(t, got.OIDCIssuerURL)
	assert.Equal(t, issuer, *got.OIDCIssuerURL)
	assert.True(t, got.OIDCMultiTenant)
	assert.True(t, got.SyncRolesFromIDP)
	assert.ElementsMatch(t, []string{"rol_idpupdrole1"}, got.AllowedRoleIDs)
	assert.ElementsMatch(t, []string{"idpupd.example.com"}, got.AllowedEmailDomains)
}

// Removing a domain from the provider's set re-points its mapping to the
// seeded internal provider (the mapping survives; only routing changes).
func TestUpdateIdentityProvider_RemovedDomainFallsBackToInternal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := deps(t)
	uow := testpg.NewUoW(t)
	internalID := ensureInternalIdP(t, d, uow)

	seeded := mustCreateOIDC(t, d, uow, "idpupd-rel", []string{"idpupd-rel-keep.example.com", "idpupd-rel-drop.example.com"})

	res, err := runAuthorizedTx(uow, operations.UpdateIdentityProvider(d), operations.UpdateCommand{
		ID:                  seeded.IdentityProviderID,
		AllowedEmailDomains: []string{"idpupd-rel-keep.example.com"},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"idpupd-rel-drop.example.com"}, res.DomainsReleased)

	dropped, err := d.MoveDeps.Mappings.FindByEmailDomain(ctx, "idpupd-rel-drop.example.com")
	require.NoError(t, err)
	require.NotNil(t, dropped, "released mapping must survive")
	assert.Equal(t, internalID, dropped.IdentityProviderID)

	kept, err := d.MoveDeps.Mappings.FindByEmailDomain(ctx, "idpupd-rel-keep.example.com")
	require.NoError(t, err)
	require.NotNil(t, kept)
	assert.Equal(t, seeded.IdentityProviderID, kept.IdentityProviderID)
}

func TestUpdateIdentityProvider_NilDomainsLeaveMappingsAlone(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := deps(t)
	uow := testpg.NewUoW(t)
	seeded := mustCreateOIDC(t, d, uow, "idpupd-nil", []string{"idpupd-nil.example.com"})

	newName := "Renamed"
	_, err := runAuthorizedTx(uow, operations.UpdateIdentityProvider(d), operations.UpdateCommand{
		ID:   seeded.IdentityProviderID,
		Name: &newName,
	})
	require.NoError(t, err)

	m, err := d.MoveDeps.Mappings.FindByEmailDomain(ctx, "idpupd-nil.example.com")
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.Equal(t, seeded.IdentityProviderID, m.IdentityProviderID, "nil domain list must not touch mappings")
}

func TestUpdateIdentityProvider_Errors(t *testing.T) {
	t.Parallel()
	d := deps(t)
	uow := testpg.NewUoW(t)

	blankName := "  "
	okName := "X"
	cases := []struct {
		name string
		cmd  operations.UpdateCommand
		kind usecase.Kind
		code string
	}{
		{"missing id", operations.UpdateCommand{Name: &okName}, usecase.KindValidation, "ID_REQUIRED"},
		{"blank name when supplied", operations.UpdateCommand{
			ID: "idp_doesnotexist1", Name: &blankName,
		}, usecase.KindValidation, "NAME_REQUIRED"},
		{"unknown id", operations.UpdateCommand{ID: "idp_doesnotexist1", Name: &okName}, usecase.KindNotFound, "IdentityProvider_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorizedTx(uow, operations.UpdateIdentityProvider(d), tc.cmd)
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}
}

// ── Delete ────────────────────────────────────────────────────────────────

func TestDeleteIdentityProvider_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := deps(t)
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, d, uow, "idpdel-happy", "Doomed")

	ev, err := runAuthorized(uow, operations.DeleteIdentityProvider(d),
		operations.DeleteCommand{ID: seeded.IdentityProviderID})
	require.NoError(t, err)
	assert.Equal(t, seeded.IdentityProviderID, ev.IdentityProviderID)
	assert.Equal(t, "idpdel-happy", ev.Code)

	got, err := d.Repo.FindByID(ctx, seeded.IdentityProviderID)
	require.NoError(t, err)
	assert.Nil(t, got, "deleted row must be gone")
}

// Deletion is blocked while mappings still route to the provider.
func TestDeleteIdentityProvider_BlockedWhileDomainsMapped(t *testing.T) {
	t.Parallel()
	d := deps(t)
	uow := testpg.NewUoW(t)
	seeded := mustCreateOIDC(t, d, uow, "idpdel-guard", []string{"idpdel-guard.example.com"})

	_, err := runAuthorized(uow, operations.DeleteIdentityProvider(d),
		operations.DeleteCommand{ID: seeded.IdentityProviderID})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "DOMAINS_STILL_MAPPED")
}

func TestDeleteIdentityProvider_InternalProtected(t *testing.T) {
	t.Parallel()
	d := deps(t)
	uow := testpg.NewUoW(t)
	internalID := ensureInternalIdP(t, d, uow)

	_, err := runAuthorized(uow, operations.DeleteIdentityProvider(d),
		operations.DeleteCommand{ID: internalID})
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "INTERNAL_IDP_PROTECTED")
}

func TestDeleteIdentityProvider_Errors(t *testing.T) {
	t.Parallel()
	d := deps(t)
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.DeleteIdentityProvider(d), operations.DeleteCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.DeleteIdentityProvider(d),
		operations.DeleteCommand{ID: "idp_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "IdentityProvider_NOT_FOUND")
}

// ── Provider moves: OIDC → internal user conversion ───────────────────────

// Moving a domain from an OIDC provider to an internal one converts its
// OIDC-provisioned users back to internal auth: provider marker cleared (so
// password reset works again), external id dropped, IDP_SYNC roles removed —
// admin-assigned roles and internal users untouched. Exercised through the
// identity-provider update op (domain removal), which shares the move
// behaviour with the standalone move op.
func TestUpdateIdentityProvider_DomainRemovalResetsOidcUsers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := deps(t)
	uow := testpg.NewUoW(t)
	pool := testpg.Pool(t)
	ensureInternalIdP(t, d, uow)

	seeded := mustCreateOIDC(t, d, uow, "idpmove-reset", []string{"idpmove-reset.example.com"})

	// An OIDC-provisioned user on the domain (the auto-provision path's
	// shape: no password, provider OIDC) with one synced + one admin role...
	oidcType := "OIDC"
	created, err := usecaseop.Run(testpg.AnchorCtx(), uow, principalops.CreateUser(d.MoveDeps.Principals), principalops.CreateCommand{
		Email: "reset-me@idpmove-reset.example.com", Scope: "ANCHOR", IDPType: &oidcType,
	}, testpg.TestEC())
	require.NoError(t, err)
	user, err := d.MoveDeps.Principals.FindByID(ctx, created.UserID)
	require.NoError(t, err)
	require.NotNil(t, user)
	idpSync := principalops.IdpSyncSource
	adminSrc := "ADMIN_ASSIGNED"
	now := time.Now().UTC()
	user.Roles = []serviceaccount.RoleAssignment{
		{Role: "idpmove:synced-role", AssignmentSource: &idpSync, AssignedAt: now},
		{Role: "idpmove:admin-role", AssignmentSource: &adminSrc, AssignedAt: now},
	}
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, principal.RolesPersister{Repository: d.MoveDeps.Principals}.Persist(ctx, user, usecasepgx.WrapTxForBootstrap(tx)))
	require.NoError(t, tx.Commit(ctx))

	// ...and an internal (hybrid) user on the same domain.
	pw := "hunter2hunter2"
	internalUser, err := usecaseop.Run(testpg.AnchorCtx(), uow, principalops.CreateUser(d.MoveDeps.Principals), principalops.CreateCommand{
		Email: "leave-me@idpmove-reset.example.com", Scope: "ANCHOR", Password: &pw,
	}, testpg.TestEC())
	require.NoError(t, err)

	// Remove the domain from the OIDC provider → falls back to internal.
	res, err := runAuthorizedTx(uow, operations.UpdateIdentityProvider(d), operations.UpdateCommand{
		ID:                  seeded.IdentityProviderID,
		AllowedEmailDomains: []string{},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"idpmove-reset.example.com"}, res.DomainsReleased)
	assert.Equal(t, 1, res.UsersReset, "exactly the OIDC-provisioned user is converted")

	got, err := d.MoveDeps.Principals.FindByID(ctx, created.UserID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.UserIdentity)
	require.NotNil(t, got.UserIdentity.Provider)
	assert.Equal(t, "INTERNAL", *got.UserIdentity.Provider, "provider marker cleared → password reset accepts the user")
	assert.Nil(t, got.UserIdentity.ExternalID)
	roleNames := make([]string, 0, len(got.Roles))
	for _, ra := range got.Roles {
		roleNames = append(roleNames, ra.Role)
	}
	assert.ElementsMatch(t, []string{"idpmove:admin-role"}, roleNames, "IDP_SYNC roles dropped, admin roles kept")

	hybrid, err := d.MoveDeps.Principals.FindByID(ctx, internalUser.UserID)
	require.NoError(t, err)
	require.NotNil(t, hybrid)
	require.NotNil(t, hybrid.UserIdentity)
	require.NotNil(t, hybrid.UserIdentity.PasswordHash, "internal user untouched")
}
