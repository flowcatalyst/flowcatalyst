//go:build integration

package api

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	saops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
)

// The /api/principals/{id} routes are the user administrator's. They must not
// act on SERVICE principals under user permissions (the service-account API
// has its own gate), must not let a client-tier caller widen anyone to every
// application, and must not hand a principal's credentials to an administrator
// who lacks that principal's authority (the role ceiling).

const (
	pgUserUpdate = "platform:iam:user:update"
	pgUserDelete = "platform:iam:user:delete"
	pgSAUpdate   = "platform:iam:service-account:update"
	pgReset2FA   = "platform:iam:user:reset-2fa"
	pgUserView   = "platform:iam:user:view"
	pgUserCreate = "platform:iam:user:create"
)

// clientTierAdmin is a client-tier administrator at clientID. It holds
// all-applications access, as an ordinary user does by default.
func clientTierAdmin(id, clientID string, perms ...string) context.Context {
	return auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID:     id,
		Scope:           auth.ScopeClient,
		Clients:         []string{clientID},
		AllApplications: true,
		Permissions:     perms,
	})
}

func anchorAllApps(id string, perms ...string) context.Context {
	return auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID:     id,
		Scope:           auth.ScopeAnchor,
		AllApplications: true,
		Permissions:     perms,
	})
}

func gateState(t *testing.T) *State {
	t.Helper()
	t.Setenv("FLOWCATALYST_APP_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	pool := testpg.Pool(t)
	return &State{
		Repo:            principal.NewRepository(pool),
		Roles:           role.NewRepository(pool),
		ServiceAccounts: serviceaccount.NewRepository(pool),
		Applications:    application.NewRepository(pool),
		ClientConfigs:   application.NewClientConfigRepo(pool),
		UoW:             testpg.NewUoW(t),
		InviteEmailer:   noopInviteEmailer{},
	}
}

// seedServiceAccountAt creates a service account through the production
// operation and homes its SERVICE principal at clientID (so a client-tier
// caller reaches it by scope: only the new gates are in its way).
func seedServiceAccountAt(t *testing.T, s *State, code, clientID string) string {
	t.Helper()
	pool := testpg.Pool(t)
	res, err := usecaseop.RunTx(testpg.AnchorCtx(), s.UoW,
		saops.CreateServiceAccountWithCredentials(s.ServiceAccounts, s.Repo,
			platformauth.NewRepository(pool).OAuthClients, client.NewRepository(pool), principal.NewClientAccessGrantRepo(pool)),
		saops.CreateCommand{Code: code, Name: "Gate SA " + code}, testpg.TestEC())
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(),
		`UPDATE iam_principals SET scope = 'CLIENT', client_id = $2 WHERE id = $1`, res.PrincipalID, clientID)
	require.NoError(t, err)
	return res.PrincipalID
}

func loadP(t *testing.T, s *State, id string) *principal.Principal {
	t.Helper()
	p, err := s.Repo.FindByID(context.Background(), ids.PrincipalID(id))
	require.NoError(t, err)
	require.NotNil(t, p, id)
	return p
}

func requireCode(t *testing.T, err error, kind usecase.Kind, code, label string) {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "%s: want a refusal, got %v", label, err)
	assert.Equal(t, kind, ue.Kind, label)
	assert.Equal(t, code, ue.Code, label)
}

// ── TA1: application access on a SERVICE principal ──────────────────────

func TestPrincipalAppAccess_ServicePrincipalNeedsServiceAccountGate(t *testing.T) {
	s := gateState(t)
	const cid = "cli_pgate_ta1_01"
	spID := seedServiceAccountAt(t, s, "pgate-ta1-sa", cid)

	// A client-tier administrator holding user:update (the hole): refused,
	// flag and list untouched.
	clientAdmin := clientTierAdmin("prn_pg_ta1_adm", cid, pgUserUpdate)
	allTrue := true
	_, err := s.assignApplicationAccess(clientAdmin, &assignAppAccessInput{
		ID:   ids.PrincipalID(spID),
		Body: AssignApplicationAccessRequest{ApplicationIDs: []string{}, AllApplications: &allTrue},
	})
	require.Error(t, err)
	assert.Equal(t, "ANCHOR_REQUIRED", usecase.AsError(err).Code, "client-tier admin on a service principal")
	_, err = s.assignApplicationAccess(clientAdmin, &assignAppAccessInput{
		ID:   ids.PrincipalID(spID),
		Body: AssignApplicationAccessRequest{ApplicationIDs: []string{}},
	})
	require.Error(t, err)
	p := loadP(t, s, spID)
	assert.False(t, p.AllApplications, "the service principal's flag is unchanged")
	assert.Empty(t, p.AccessibleApplicationIDs)

	// An anchor holding only the service-account update permission succeeds.
	operator := anchorAllApps("prn_pg_ta1_op", pgSAUpdate)
	_, err = s.assignApplicationAccess(operator, &assignAppAccessInput{
		ID:   ids.PrincipalID(spID),
		Body: AssignApplicationAccessRequest{ApplicationIDs: []string{}, AllApplications: &allTrue},
	})
	require.NoError(t, err)
	assert.True(t, loadP(t, s, spID).AllApplications)

	// The service-account permission does not let the same caller touch a USER.
	userID := "prn_pg_ta1_usr"
	mustSeedPrincipalWithRole(t, context.Background(), s.Repo, userID, cid, "")
	_, err = s.assignApplicationAccess(operator, &assignAppAccessInput{
		ID:   ids.PrincipalID(userID),
		Body: AssignApplicationAccessRequest{ApplicationIDs: []string{}},
	})
	requireCode(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED", "service-account:update on a USER")
}

// ── TA2: allApplications:true needs an anchor-tier caller ───────────────

func TestPrincipalAppAccess_AllApplicationsNeedsAnchor(t *testing.T) {
	s := gateState(t)
	const cid = "cli_pgate_ta2_01"
	const userID = "prn_pg_ta2_usr"
	mustSeedPrincipalWithRole(t, context.Background(), s.Repo, userID, cid, "")
	_, err0 := testpg.Pool(t).Exec(context.Background(), `UPDATE iam_principals SET all_applications = FALSE WHERE id = $1`, userID)
	require.NoError(t, err0)

	clientAdmin := clientTierAdmin("prn_pg_ta2_adm", cid, pgUserUpdate)
	allTrue, allFalse := true, false

	_, err := s.assignApplicationAccess(clientAdmin, &assignAppAccessInput{
		ID:   userID,
		Body: AssignApplicationAccessRequest{ApplicationIDs: []string{}, AllApplications: &allTrue},
	})
	require.Error(t, err)
	assert.Equal(t, usecase.KindAuthorization, usecase.AsError(err).Kind)
	assert.Contains(t, err.Error(), "anchor administrator")
	assert.False(t, loadP(t, s, userID).AllApplications, "refused: flag unchanged")

	anchor := anchorAllApps("prn_pg_ta2_anc", pgUserUpdate)
	out, err := s.assignApplicationAccess(anchor, &assignAppAccessInput{
		ID:   userID,
		Body: AssignApplicationAccessRequest{ApplicationIDs: []string{}, AllApplications: &allTrue},
	})
	require.NoError(t, err)
	assert.True(t, out.Body.AllApplications)
	assert.True(t, loadP(t, s, userID).AllApplications)

	// Clearing it is unchanged behaviour for the client-tier administrator.
	_, err = s.assignApplicationAccess(clientAdmin, &assignAppAccessInput{
		ID:   userID,
		Body: AssignApplicationAccessRequest{ApplicationIDs: []string{}, AllApplications: &allFalse},
	})
	require.NoError(t, err)
	assert.False(t, loadP(t, s, userID).AllApplications)
}

// ── TA3: update / activate / deactivate / delete refuse a SERVICE target ─

func TestPrincipalRoutes_RefuseServiceTargets(t *testing.T) {
	s := gateState(t)
	const cid = "cli_pgate_ta3_01"
	spID := seedServiceAccountAt(t, s, "pgate-ta3-sa", cid)
	before := loadP(t, s, spID)

	anchor := anchorAllApps("prn_pg_ta3_anc", pgUserUpdate, pgUserDelete)
	clientAdmin := clientTierAdmin("prn_pg_ta3_adm", cid, pgUserUpdate, pgUserDelete)
	name, inactive := "renamed", false

	for label, ctx := range map[string]context.Context{"anchor": anchor, "client admin": clientAdmin} {
		_, err := s.update(ctx, &updateInput{ID: ids.PrincipalID(spID), Body: UpdatePrincipalRequest{Name: &name, Active: &inactive}})
		requireCode(t, err, usecase.KindConflict, "NOT_A_USER", label+" update")
		requireCode(t, s.DeactivatePrincipal(ctx, spID), usecase.KindConflict, "NOT_A_USER", label+" deactivate")
		requireCode(t, s.ActivatePrincipal(ctx, spID), usecase.KindConflict, "NOT_A_USER", label+" activate")
		requireCode(t, s.DeletePrincipal(ctx, spID), usecase.KindConflict, "NOT_A_USER", label+" delete")
	}
	after := loadP(t, s, spID)
	assert.Equal(t, before.Name, after.Name)
	assert.Equal(t, before.Active, after.Active)
	assert.True(t, after.Active)

	// The same on a USER is as it always was.
	const userID = "prn_pg_ta3_usr"
	mustSeedPrincipalWithRole(t, context.Background(), s.Repo, userID, cid, "")
	_, err := s.update(clientAdmin, &updateInput{ID: userID, Body: UpdatePrincipalRequest{Name: &name}})
	require.NoError(t, err)
	require.NoError(t, s.DeactivatePrincipal(clientAdmin, userID))
	assert.False(t, loadP(t, s, userID).Active)
	require.NoError(t, s.ActivatePrincipal(clientAdmin, userID))
	assert.True(t, loadP(t, s, userID).Active)
	require.NoError(t, s.DeletePrincipal(clientAdmin, userID))
	gone, err := s.Repo.FindByID(context.Background(), userID)
	require.NoError(t, err)
	assert.Nil(t, gone)
}

// ── TB1: credential takeover respects the role ceiling ──────────────────

type countingEmailer struct{ calls int }

func (c *countingEmailer) SendResetEmail(context.Context, *principal.Principal, bool) error {
	c.calls++
	return nil
}

func seedCredTarget(t *testing.T, s *State, id, cid, roleName string) {
	t.Helper()
	mustSeedPrincipalWithRole(t, context.Background(), s.Repo, id, cid, roleName)
	pool := testpg.Pool(t)
	_, err := pool.Exec(context.Background(), `UPDATE iam_principals SET password_hash = 'original-hash' WHERE id = $1`, id)
	require.NoError(t, err)
	repo := mfa.NewRepository(pool)
	m := mfa.NewMethod(id, mfa.MethodTOTP)
	require.NoError(t, repo.InsertMethod(context.Background(), m))
}

func mfaCount(t *testing.T, id string) int {
	t.Helper()
	ms, err := mfa.NewRepository(testpg.Pool(t)).FindMethodsByPrincipal(context.Background(), id)
	require.NoError(t, err)
	return len(ms)
}

func pwHash(t *testing.T, s *State, id string) string {
	t.Helper()
	p := loadP(t, s, id)
	require.NotNil(t, p.UserIdentity)
	require.NotNil(t, p.UserIdentity.PasswordHash)
	return *p.UserIdentity.PasswordHash
}

func TestCredentialTakeover_BoundedByCallersPermissions(t *testing.T) {
	s := gateState(t)
	mailer := &countingEmailer{}
	s.PasswordEmailer = mailer
	s.MFA = mfa.NewService(mfa.NewRepository(testpg.Pool(t)), nil, nil, mfa.Config{})
	const cid = "cli_pgate_tb1_01"

	power := mustRoleWith(t, s.UoW, "pgatetb1", "power", pgUserCreate, pgUserView)
	viewer := mustRoleWith(t, s.UoW, "pgatetb1", "viewer", pgUserView)
	const strong = "Tr0ub4dor&3-horse-staple-X9"

	admin := clientTierAdmin("prn_pg_tb1_adm", cid, pgUserUpdate, pgReset2FA, pgUserView)
	super := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_pg_tb1_sup", Scope: auth.ScopeAnchor, AllApplications: true,
		Permissions: []string{"platform:*:*:*"},
	})

	// Target above the administrator.
	const high = "prn_pg_tb1_high"
	seedCredTarget(t, s, high, cid, power)

	err := s.ResetPrincipalPassword(admin, high, ResetPasswordRequest{NewPassword: strong})
	requireRoleAboveCaller(t, err, "reset-password", power)
	assert.Equal(t, "original-hash", pwHash(t, s, high), "password hash unchanged")

	err = s.ResetPrincipalTwoFactor(admin, high)
	requireRoleAboveCaller(t, err, "reset-2fa", power)
	assert.Equal(t, 1, mfaCount(t, high), "2FA state unchanged")

	_, err = s.sendPasswordReset(admin, &sendPasswordResetInput{ID: high, Body: &struct {
		Reset2FA bool `json:"reset2fa,omitempty"`
	}{Reset2FA: true}})
	requireRoleAboveCaller(t, err, "send-password-reset reset2fa", power)
	assert.Zero(t, mailer.calls, "no reset token minted")

	// Target at or below the administrator.
	const low = "prn_pg_tb1_low"
	seedCredTarget(t, s, low, cid, viewer)
	require.NoError(t, s.ResetPrincipalPassword(admin, low, ResetPasswordRequest{NewPassword: strong}))
	assert.NotEqual(t, "original-hash", pwHash(t, s, low))
	require.NoError(t, s.ResetPrincipalTwoFactor(admin, low))
	assert.Zero(t, mfaCount(t, low))
	_, err = s.sendPasswordReset(admin, &sendPasswordResetInput{ID: low, Body: &struct {
		Reset2FA bool `json:"reset2fa,omitempty"`
	}{Reset2FA: true}})
	require.NoError(t, err)
	assert.Equal(t, 1, mailer.calls)

	// Plain send-password-reset (no 2FA reset) mints a link only the mailbox
	// owner can use: not part of the ceiling.
	_, err = s.sendPasswordReset(admin, &sendPasswordResetInput{ID: high})
	require.NoError(t, err)

	// A super-admin passes for every target.
	require.NoError(t, s.ResetPrincipalPassword(super, high, ResetPasswordRequest{NewPassword: strong}))
	assert.NotEqual(t, "original-hash", pwHash(t, s, high))
	require.NoError(t, s.ResetPrincipalTwoFactor(super, high))
	assert.Zero(t, mfaCount(t, high))
}

// ── TB2: developer credential for another principal ─────────────────────

func TestDeveloperCredential_OtherPrincipalBoundedByCeiling(t *testing.T) {
	t.Setenv("FLOWCATALYST_APP_KEY", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32))))
	s := gateState(t)
	const cid = "cli_pgate_tb2_01"

	testpg.EnsureApplication(t, "platform")
	devRole := ensureDevRole(t, s)
	power := mustRoleWith(t, s.UoW, "pgatetb2", "power", pgUserCreate, pgUserView)

	const high, low = "prn_pg_tb2_high", "prn_pg_tb2_low"
	mustSeedPrincipalWithRole(t, context.Background(), s.Repo, high, cid, devRole)
	_, err := testpg.Pool(t).Exec(context.Background(),
		`INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source) VALUES ($1, $2, 'ADMIN_ASSIGNED')`, high, power)
	require.NoError(t, err)
	mustSeedPrincipalWithRole(t, context.Background(), s.Repo, low, cid, devRole)

	admin := clientTierAdmin("prn_pg_tb2_adm", cid, pgUserUpdate, pgUserView)

	_, err = s.setDeveloperCredential(admin, &apicommon.IDInput{ID: high})
	requireRoleAboveCaller(t, err, "developer credential for a higher principal", power)
	assert.Nil(t, loadP(t, s, high).UserIdentity.DevClientSecretRef, "no secret was set")

	out, err := s.setDeveloperCredential(admin, &apicommon.IDInput{ID: low})
	require.NoError(t, err)
	assert.NotEmpty(t, out.Body.ClientSecret)

	// A developer acting on itself is not an escalation, whatever it holds.
	self := clientTierAdmin(high, cid, "platform:developer:api-credential:manage")
	out, err = s.setDeveloperCredential(self, &apicommon.IDInput{ID: high})
	require.NoError(t, err)
	assert.NotEmpty(t, out.Body.ClientSecret)
}

func ensureDevRole(t *testing.T, s *State) string {
	t.Helper()
	r, err := s.Roles.FindByName(context.Background(), "platform:developer")
	require.NoError(t, err)
	if r != nil {
		return r.Name
	}
	return mustRoleWith(t, s.UoW, "platform", "developer")
}
