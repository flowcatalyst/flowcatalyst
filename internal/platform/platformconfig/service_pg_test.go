//go:build integration

package platformconfig_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

func newServiceFixture(t *testing.T, app string) *platformconfig.Service {
	t.Helper()
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	enc, err := encryption.New(key)
	require.NoError(t, err)
	pool := testpg.Pool(t)
	repo := platformconfig.NewRepository(pool).WithEncryption(enc)
	uow := testpg.NewUoW(t)
	testpg.EnsureApplication(t, app)
	secret := "SECRET"
	_, err = usecaseop.Run(adminCtx(), uow, operations.SetProperty(repo), operations.SetPropertyCommand{
		ApplicationCode: app, Section: "sftp", Property: "password", Value: "s3cr3t-pw", ValueType: &secret,
	}, testpg.TestEC())
	require.NoError(t, err)
	return &platformconfig.Service{Repo: repo, UoW: uow, Audit: audit.NewRepository(pool)}
}

func revealer() *auth.AuthContext {
	return &auth.AuthContext{
		PrincipalID: "prn_reveal", Scope: auth.ScopeAnchor, AllApplications: true,
		Permissions: []string{"platform:admin:config:reveal", "platform:admin:config:view", "platform:admin:config:update"},
	}
}

// A reveal returns the plaintext and writes its audit row; a caller without
// the reveal permission gets neither; a service with no audit log refuses.
func TestServiceReveal_AuditedAndPermissioned(t *testing.T) {
	ctx := context.Background()
	svc := newServiceFixture(t, "svcreveal")

	r, err := svc.Reveal(ctx, revealer(), "svcreveal", "sftp", "password", "")
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t-pw", r.Value)
	var n int
	require.NoError(t, testpg.Pool(t).QueryRow(ctx,
		`SELECT count(*) FROM aud_logs WHERE entity_id = $1 AND operation = 'SECRET_REVEALED' AND principal_id = 'prn_reveal'`, r.Config.ID).Scan(&n))
	assert.Equal(t, 1, n, "one audit row per reveal")

	viewer := &auth.AuthContext{PrincipalID: "prn_view", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:admin:config:view", "platform:admin:config:update"}}
	_, err = svc.Reveal(ctx, viewer, "svcreveal", "sftp", "password", "")
	require.Error(t, err, "view and update do not imply reveal")

	svc.Audit = nil
	_, err = svc.Reveal(ctx, revealer(), "svcreveal", "sftp", "password", "")
	require.Error(t, err, "a reveal that cannot be audited is refused")
	require.NoError(t, testpg.Pool(t).QueryRow(ctx,
		`SELECT count(*) FROM aud_logs WHERE entity_id = $1 AND operation = 'SECRET_REVEALED'`, r.Config.ID).Scan(&n))
	assert.Equal(t, 1, n, "the refused reveal wrote nothing")
}

// List needs config view; Delete needs config update and is idempotent.
func TestServiceListAndDelete(t *testing.T) {
	ctx := context.Background()
	svc := newServiceFixture(t, "svcdelete")

	noPerm := &auth.AuthContext{PrincipalID: "prn_none", Scope: auth.ScopeAnchor, AllApplications: true}
	_, err := svc.List(ctx, noPerm, "svcdelete")
	require.Error(t, err)
	require.Error(t, svc.Delete(ctx, noPerm, "svcdelete", "sftp", "password", ""))

	rows, err := svc.List(ctx, revealer(), "svcdelete")
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "***", rows[0].MaskedValue())

	require.NoError(t, svc.Delete(ctx, revealer(), "svcdelete", "sftp", "password", ""))
	require.NoError(t, svc.Delete(ctx, revealer(), "svcdelete", "sftp", "password", ""), "deleting twice is not an error")
	rows, err = svc.List(ctx, revealer(), "svcdelete")
	require.NoError(t, err)
	assert.Empty(t, rows)
}

// RevealAuthorised is Reveal for a caller authorised some other way: it needs
// no reveal permission, still audits (adding the caller's detail to the row),
// and is refused when the audit row cannot be written.
func TestServiceRevealAuthorised_AuditsWithDetailAndRefusesWithoutAudit(t *testing.T) {
	ctx := context.Background()
	svc := newServiceFixture(t, "svcauthorised")
	caller := &auth.AuthContext{PrincipalID: "prn_runtime", Scope: auth.ScopeAnchor, AllApplications: true}

	r, err := svc.RevealAuthorised(ctx, caller, "svcauthorised", "sftp", "password", "", map[string]string{"ingestionId": "ing_1"})
	require.NoError(t, err)
	assert.Equal(t, "s3cr3t-pw", r.Value)
	var n int
	require.NoError(t, testpg.Pool(t).QueryRow(ctx, `SELECT count(*) FROM aud_logs WHERE entity_id = $1 AND operation = 'SECRET_REVEALED'
		AND principal_id = 'prn_runtime' AND operation_json->>'ingestionId' = 'ing_1' AND operation_json->>'property' = 'password'`, r.Config.ID).Scan(&n))
	assert.Equal(t, 1, n)

	svc.Audit = nil
	_, err = svc.RevealAuthorised(ctx, caller, "svcauthorised", "sftp", "password", "", nil)
	require.Error(t, err)
	_, err = svc.RevealAuthorised(ctx, caller, "svcauthorised", "sftp", "missing", "", nil)
	require.Error(t, err, "an unknown property is not found")
}
