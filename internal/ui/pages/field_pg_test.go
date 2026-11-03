//go:build integration

package pages

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/pin"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// The field pages end to end against Postgres: the handlers run the real
// operations (FieldOps over the unit of work), so the rows, the PIN hash, the
// enrolment code, the policy and the sessions are the API's. Permissions are
// the real auth checks on real client ids.
func TestFieldPagesAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	clients := client.NewRepository(pool)
	hasher, err := pin.NewHasher("field-ui-test-app-key")
	require.NoError(t, err)
	n := time.Now().UnixNano() % 1_000_000_000
	mkClient := func(name string) string {
		ev, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients),
			clientops.CreateCommand{Name: name, Identifier: fmt.Sprintf("field-ui-%s-%d", strings.ToLower(name), n)}, testpg.TestEC())
		require.NoError(t, err)
		return ev.ClientID
	}
	acme, globex := mkClient("Acme"), mkClient("Globex")
	appCode := fmt.Sprintf("fldui%d", n)
	appID := testpg.EnsureApplication(t, appCode)

	repos := fieldidentity.NewRepos(pool)
	apps := application.NewRepository(pool)
	store := FieldReader{Repos: repos, Apps: apps}
	ops := FieldOps{Ops: &fieldidentity.Ops{R: repos, Clients: clients, Apps: apps, Hasher: hasher}, UoW: uow}
	sh := shellForTest()
	serve := func(ac *auth.AuthContext, q req) *fieldResult {
		rec := serveMounted(t, ac, q,
			&FieldWorkers{Store: store, Ops: ops, Clients: clients, Shell: sh},
			&FieldDevices{Store: store, Ops: ops, Clients: clients, BaseURL: "https://fc.example.test", Shell: sh},
			&FieldPolicy{Store: store, Ops: ops, Clients: clients, Shell: sh})
		return &fieldResult{rec.Code, rec.Body.String(), rec.Header()}
	}
	admin := &auth.AuthContext{
		PrincipalID: "prn_optestrunner1", Scope: auth.ScopeAnchor, Permissions: []string{"platform:*:*:*"},
		AllApplications: true, PrincipalType: auth.PrincipalTypeUser,
	}
	acmeManager := fieldCaller([]string{acme}, permFieldWorkerView, permFieldWorkerManage, permFieldDeviceView, permFieldDeviceManage)

	// Create a worker through the dialog; the row and its grant exist.
	res := serve(acmeManager, postForm("/ui/field-workers/new", "client", acme, "workerCode", "ui-ann", "name", "Ann UI", "apps", appCode))
	require.Equal(t, http.StatusOK, res.code, res.body)
	assert.Contains(t, res.body, "UI-ANN created")
	w, err := repos.Identities.FindByClientAndCode(ctx, acme, "UI-ANN")
	require.NoError(t, err)
	require.NotNil(t, w)
	assert.True(t, w.HasApp(appID))
	assert.False(t, w.HasPIN(), "a new worker has no PIN until a reset")

	// The list shows her, the other client's list is refused.
	res = serve(acmeManager, req{target: "/ui/field-workers?client=" + acme})
	assert.Contains(t, res.body, "UI-ANN")
	assert.Contains(t, res.body, "No PIN")
	assert.Equal(t, http.StatusForbidden, serve(acmeManager, req{target: "/ui/field-workers?client=" + globex}).code)

	// Reset PIN: the temporary PIN is in this answer only; the stored value is
	// a hash that verifies it.
	res = serve(acmeManager, postForm("/ui/field-workers/"+w.ID+"/reset-pin", "drawer", w.ID))
	require.Equal(t, http.StatusOK, res.code, res.body)
	m := regexp.MustCompile(`id="fw-pin-value"[^>]*>([^<]+)<`).FindStringSubmatch(res.body)
	require.Len(t, m, 2, res.body)
	temp := m[1]
	assert.Equal(t, "no-store", res.header.Get("Cache-Control"))
	assert.Contains(t, res.body, "will not be shown again")
	w, err = repos.Identities.FindByID(ctx, w.ID)
	require.NoError(t, err)
	require.True(t, w.HasPIN())
	assert.True(t, w.PINChangeRequired)
	assert.NotContains(t, *w.PINHash, temp, "the PIN is stored hashed")
	for _, q := range []req{{target: "/ui/field-workers?client=" + acme}, {target: "/ui/field-workers/" + w.ID, datastar: true}, {target: "/ui/field-workers/" + w.ID}} {
		assert.NotContains(t, serve(acmeManager, q).body, temp, "GET %s re-serves the PIN", q.target)
	}

	// Suspend, activate, grant refused for an unknown app, then delete.
	res = serve(acmeManager, postForm("/ui/field-workers/"+w.ID+"/suspend"))
	assert.Contains(t, res.body, "UI-ANN suspended")
	w, _ = repos.Identities.FindByID(ctx, w.ID)
	assert.Equal(t, fieldidentity.StatusDisabled, w.Status)
	res = serve(acmeManager, postForm("/ui/field-workers/"+w.ID+"/activate"))
	assert.Contains(t, res.body, "UI-ANN activated")
	res = serve(acmeManager, postForm("/ui/field-workers/"+w.ID+"/grant", "app", "no-such-app"))
	assert.Contains(t, res.body, "Unknown or inactive application")

	// A worker of another client is not found for a manager of acme and
	// nothing is written.
	actx := testpg.WithAuth(ctx, admin)
	gw, err := usecaseop.Run(actx, uow, ops.Ops.EnsureWorker(), fieldidentity.EnsureWorkerCommand{ClientID: globex, WorkerCode: "GUS", Name: "Gus", AppCodes: []string{appCode}}, auth.NewExecutionContext(actx))
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, serve(acmeManager, nativePost("/ui/field-workers/"+gw.ID+"/delete")).code)
	assert.Equal(t, http.StatusNotFound, serve(acmeManager, nativePost("/ui/field-workers/"+gw.ID+"/reset-pin")).code)
	g, err := repos.Identities.FindByID(ctx, gw.ID)
	require.NoError(t, err)
	require.NotNil(t, g, "the other client's worker survived")
	assert.False(t, g.HasPIN())

	// Enrol a device: the code is shown once, only its hash is stored.
	res = serve(acmeManager, postForm("/ui/field-devices/new", "client", acme, "mode", "PERSONAL", "apps", appCode, "worker", "ui-ann", "label", "Handheld"))
	require.Equal(t, http.StatusOK, res.code, res.body)
	m = regexp.MustCompile(`id="fd-code-value"[^>]*>([^<]+)<`).FindStringSubmatch(res.body)
	require.Len(t, m, 2, res.body)
	code := m[1]
	assert.Contains(t, res.body, "<svg")
	assert.Equal(t, "no-store", res.header.Get("Cache-Control"))
	stored, err := repos.Codes.FindByHash(ctx, fieldidentity.HashToken(code))
	require.NoError(t, err)
	require.NotNil(t, stored, "the code is stored under its hash")
	assert.Equal(t, acme, stored.ClientID)
	assert.Equal(t, []string{appID}, stored.AppIDs)
	assert.Equal(t, w.ID, *stored.AssignedIdentityID)
	byPlain, err := repos.Codes.FindByHash(ctx, code)
	require.NoError(t, err)
	assert.Nil(t, byPlain, "the plaintext code is not stored")
	assert.NotContains(t, res.header.Get("Location"), code)
	assert.NotContains(t, serve(acmeManager, req{target: "/ui/field-devices?client=" + acme}).body, code)

	// A device: list, suspend and revoke through the page.
	d := &fieldidentity.Device{
		ID: tsid.Generate(tsid.FieldDevice), ClientID: acme, Mode: fieldidentity.ModeShared, Label: "Dock",
		JKT: "jkt-" + tsid.GenerateUntyped(), PublicKeyJWK: "{}", AttestationLevel: "TEE",
		Status: fieldidentity.DeviceActive, EnrolledAt: time.Now().UTC(), AppIDs: []string{appID},
	}
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, repos.Devices.Persist(ctx, d, usecasepgx.WrapTxForBootstrap(tx)))
	require.NoError(t, tx.Commit(ctx))
	res = serve(acmeManager, req{target: "/ui/field-devices?client=" + acme})
	assert.Contains(t, res.body, "Dock")
	assert.Contains(t, res.body, "TEE")
	serve(acmeManager, postForm("/ui/field-devices/"+d.ID+"/suspend"))
	got, _ := repos.Devices.FindByID(ctx, d.ID)
	assert.Equal(t, fieldidentity.DeviceSuspended, got.Status)
	serve(acmeManager, postForm("/ui/field-devices/"+d.ID+"/revoke", "drawer", d.ID))
	got, _ = repos.Devices.FindByID(ctx, d.ID)
	assert.Equal(t, fieldidentity.DeviceRevoked, got.Status)
	assert.Equal(t, http.StatusForbidden, serve(fieldCaller([]string{acme}, permFieldDeviceView), nativePost("/ui/field-devices/"+d.ID+"/activate")).code)

	// Policy: defaults first, a refused value writes nothing, a good one is
	// stored and read back.
	assert.Contains(t, serve(acmeManager, req{target: "/ui/field-policy?client=" + acme}).body, "Default")
	res = serve(acmeManager, postForm("/ui/field-policy", "client", acme, "pinMinLength", "3"))
	assert.Contains(t, res.body, "Minimum PIN length must be between 4 and 12")
	p, err := repos.Policies.Effective(ctx, acme)
	require.NoError(t, err)
	assert.True(t, p.IsDefault)
	res = serve(acmeManager, postForm("/ui/field-policy", "client", acme, "pinMinLength", "8", "softLockSecs", "600", "requireAttestation", "1"))
	assert.Contains(t, res.body, "Field policy saved")
	p, err = repos.Policies.Effective(ctx, acme)
	require.NoError(t, err)
	assert.False(t, p.IsDefault)
	assert.Equal(t, 8, p.PINMinLength)
	assert.Equal(t, 600, p.SoftLockSecs)
	assert.True(t, p.RequireAttestation)
	assert.Contains(t, serve(acmeManager, req{target: "/ui/field-policy?client=" + acme}).body, "Custom")
	assert.Equal(t, http.StatusForbidden, serve(acmeManager, nativePost("/ui/field-policy", "client", globex, "pinMinLength", "9")).code)

	// Delete last: her sessions go with her.
	res = serve(acmeManager, postForm("/ui/field-workers/"+w.ID+"/delete", "drawer", w.ID))
	assert.Contains(t, res.body, "UI-ANN deleted")
	gone, err := repos.Identities.FindByID(ctx, w.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)
}

type fieldResult struct {
	code   int
	body   string
	header http.Header
}
