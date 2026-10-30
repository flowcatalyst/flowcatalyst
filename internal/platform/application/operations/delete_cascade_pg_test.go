//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	pcops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// adminCtx is a full platform administrator: anchor tier, the super-admin
// wildcard, and all-applications access (testpg.AnchorCtx is anchor tier
// without all-applications access, which platform config confines to the
// applications it is bound to).
func adminCtx() context.Context {
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_optestrunner1", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:*:*:*"},
	})
}

// Deleting an application deletes its platform configuration (values, secrets
// included, and access grants) in the same transaction, records how many went
// with it, and a new application with the same code starts with none.
func TestDeleteApplication_CascadesPlatformConfig(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := application.NewRepository(pool)
	pcRepo := platformconfig.NewRepository(pool)
	uow := testpg.NewUoW(t)
	const code = "appdelpcfg1"
	seeded := mustCreateInactiveApp(t, repo, uow, code, "Configured")

	secret := "SECRET"
	for _, p := range []string{"api-key", "sftp-password"} {
		_, err := usecaseop.Run(adminCtx(), uow, pcops.SetProperty(pcRepo), pcops.SetPropertyCommand{
			ApplicationCode: code, Section: "secrets", Property: p, Value: "s3cr3t", ValueType: &secret,
		}, testpg.TestEC())
		require.NoError(t, err)
	}
	_, err := usecaseop.Run(adminCtx(), uow, pcops.GrantAccess(pcRepo), pcops.GrantAccessCommand{ApplicationCode: code, RoleCode: code + ":editor", CanWrite: true}, testpg.TestEC())
	require.NoError(t, err)

	ev, err := runAuthorized(uow, operations.DeleteApplication(repo), operations.DeleteCommand{ID: seeded.ApplicationID})
	require.NoError(t, err, "platform configuration does not block the delete")
	assert.Equal(t, int64(2), ev.PlatformConfigPropertiesDeleted)
	assert.Equal(t, int64(1), ev.PlatformConfigGrantsDeleted)

	var data string
	require.NoError(t, pool.QueryRow(ctx, `SELECT data::text FROM msg_events WHERE id = $1`, ev.EventID()).Scan(&data))
	assert.Contains(t, data, `"platformConfigPropertiesDeleted": 2`)

	again := mustCreateApp(t, repo, uow, code, "Same code, new application")
	rows, err := pcRepo.FindConfigsByApplication(ctx, code)
	require.NoError(t, err)
	assert.Empty(t, rows, "the new application inherits no values or secrets")
	grants, err := pcRepo.FindAccessByApplication(ctx, code)
	require.NoError(t, err)
	assert.Empty(t, grants, "nor access grants")
	_ = again
}
