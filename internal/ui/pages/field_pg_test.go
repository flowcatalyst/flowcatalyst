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

// The sign-in options of the field pages against Postgres: a badge issued,
// reissued, revoked and printed in bulk; the test and supervisor flags with
// their permission; a console sign-in code; the policy keeping its sign-in
// fields across a save. The operations are the API's, so the rows are too.
func TestFieldSignInOptionsAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	clients := client.NewRepository(pool)
	hasher, err := pin.NewHasher("field-ui-test-app-key")
	require.NoError(t, err)
	n := time.Now().UnixNano() % 1_000_000_000
	mkClient := func(name string) string {
		ev, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients),
			clientops.CreateCommand{Name: name, Identifier: fmt.Sprintf("field-so-%s-%d", strings.ToLower(name), n)}, testpg.TestEC())
		require.NoError(t, err)
		return ev.ClientID
	}
	acme, globex := mkClient("Acme"), mkClient("Globex")
	appCode := fmt.Sprintf("fldso%d", n)
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
	manager := fieldCaller([]string{acme}, permFieldWorkerView, permFieldWorkerManage, permFieldDeviceView, permFieldDeviceManage)
	supervisorAdmin := fieldCaller([]string{acme}, permFieldWorkerView, permFieldWorkerManage, permFieldWorkerImpers, permFieldDeviceView, permFieldDeviceManage)
	tokenIn := regexp.MustCompile(`FWB1[0-9A-Z]{26}`)

	// Two workers (one a test identity), made through the dialog.
	res := serve(manager, postForm("/ui/field-workers/new", "client", acme, "workerCode", "so-ann", "name", "Ann", "apps", appCode))
	require.Equal(t, http.StatusOK, res.code, res.body)
	res = serve(manager, postForm("/ui/field-workers/new", "client", acme, "workerCode", "so-tess", "name", "Tess", "apps", appCode, "isTest", "true"))
	require.Equal(t, http.StatusOK, res.code, res.body)
	ann, err := repos.Identities.FindByClientAndCode(ctx, acme, "SO-ANN")
	require.NoError(t, err)
	tess, err := repos.Identities.FindByClientAndCode(ctx, acme, "SO-TESS")
	require.NoError(t, err)
	require.NotNil(t, ann)
	require.NotNil(t, tess)
	assert.False(t, ann.IsTest)
	assert.True(t, tess.IsTest)
	assert.Contains(t, serve(manager, req{target: "/ui/field-workers?client=" + acme}).body, ">TEST<")
	filtered := serve(manager, req{target: "/ui/field-workers?client=" + acme + "&test=false"}).body
	assert.Contains(t, filtered, "SO-ANN")
	assert.NotContains(t, filtered, "SO-TESS")

	// The supervisor flag: refused without the impersonate permission, set with it.
	assert.Equal(t, http.StatusForbidden, serve(manager, nativePost("/ui/field-workers/new", "client", acme, "workerCode", "so-sup", "name", "Sup", "canImpersonate", "true")).code)
	none, err := repos.Identities.FindByClientAndCode(ctx, acme, "SO-SUP")
	require.NoError(t, err)
	assert.Nil(t, none, "a refused create wrote nothing")
	assert.Equal(t, http.StatusForbidden, serve(manager, nativePost("/ui/field-workers/"+ann.ID+"/edit", "name", "Ann", "canImpersonate", "true")).code)
	ann, _ = repos.Identities.FindByID(ctx, ann.ID)
	assert.False(t, ann.CanImpersonate)
	res = serve(supervisorAdmin, postForm("/ui/field-workers/"+ann.ID+"/edit", "name", "Ann Lee", "email", "ann@acme.test", "flags", "1", "impersonateField", "1", "canImpersonate", "true"))
	require.Equal(t, http.StatusOK, res.code, res.body)
	ann, _ = repos.Identities.FindByID(ctx, ann.ID)
	assert.True(t, ann.CanImpersonate)
	assert.Equal(t, "Ann Lee", ann.Name)
	assert.Equal(t, "ann@acme.test", ann.Email)
	assert.Contains(t, serve(manager, req{target: "/ui/field-workers?client=" + acme}).body, ">SUPERVISOR<")

	// Issue a badge: the token is in the answer once; only its hash is stored.
	res = serve(manager, postForm("/ui/field-workers/"+ann.ID+"/badge", "drawer", ann.ID))
	require.Equal(t, http.StatusOK, res.code, res.body)
	first := tokenIn.FindString(res.body)
	require.NotEmpty(t, first, res.body)
	assert.Equal(t, 1, strings.Count(res.body, first))
	assert.Equal(t, "no-store", res.header.Get("Cache-Control"))
	b1, err := repos.Badges.ActiveByIdentity(ctx, ann.ID)
	require.NoError(t, err)
	require.NotNil(t, b1)
	byHash, err := repos.Badges.FindByHash(ctx, fieldidentity.HashBadgeToken(first))
	require.NoError(t, err)
	require.NotNil(t, byHash, "the token is stored under its hash")
	assert.Equal(t, b1.ID, byHash.ID)
	plain, err := repos.Badges.FindByHash(ctx, first)
	require.NoError(t, err)
	assert.Nil(t, plain, "the token itself is not stored")
	drawer := serve(manager, req{target: "/ui/field-workers/" + ann.ID, datastar: true}).body
	assert.Contains(t, drawer, "Reissue badge")
	assert.NotContains(t, drawer, first)

	// Reissue: the old badge is cancelled in the same operation.
	res = serve(manager, postForm("/ui/field-workers/"+ann.ID+"/badge", "drawer", ann.ID))
	require.Equal(t, http.StatusOK, res.code, res.body)
	second := tokenIn.FindString(res.body)
	require.NotEmpty(t, second)
	assert.NotEqual(t, first, second)
	assert.Contains(t, res.body, "Their previous badge has been cancelled")
	old, err := repos.Badges.FindByID(ctx, b1.ID)
	require.NoError(t, err)
	assert.Equal(t, fieldidentity.BadgeRevoked, old.Status)
	b2, err := repos.Badges.ActiveByIdentity(ctx, ann.ID)
	require.NoError(t, err)
	require.NotNil(t, b2)
	assert.NotEqual(t, b1.ID, b2.ID)

	// Revoke it.
	res = serve(manager, postForm("/ui/field-workers/"+ann.ID+"/badge/revoke", "drawer", ann.ID))
	require.Equal(t, http.StatusOK, res.code, res.body)
	gone, err := repos.Badges.ActiveByIdentity(ctx, ann.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)

	// A sheet for both workers, in one operation; the foreign worker fails the
	// whole request and nothing is issued.
	actx := testpg.WithAuth(ctx, fieldAnchor())
	gw, err := usecaseop.Run(actx, uow, ops.Ops.EnsureWorker(),
		fieldidentity.EnsureWorkerCommand{ClientID: globex, WorkerCode: "SO-GUS", Name: "Gus", AppCodes: []string{appCode}}, auth.NewExecutionContext(actx))
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, serve(manager, nativePost("/ui/field-workers/badges/print", "ids", ann.ID, "ids", gw.ID)).code)
	noBadge, err := repos.Badges.ActiveByIdentity(ctx, ann.ID)
	require.NoError(t, err)
	assert.Nil(t, noBadge, "a refused sheet issued nothing")
	res = serve(manager, postForm("/ui/field-workers/badges/print", "ids", ann.ID, "ids", tess.ID))
	require.Equal(t, http.StatusOK, res.code, res.body)
	tokens := tokenIn.FindAllString(res.body, -1)
	require.Len(t, tokens, 2, res.body)
	assert.Equal(t, 2, strings.Count(res.body, `class="fw-card"`))
	for _, id := range []string{ann.ID, tess.ID} {
		b, err := repos.Badges.ActiveByIdentity(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, b, "a badge for %s", id)
	}
	for _, tok := range tokens {
		b, err := repos.Badges.FindByHash(ctx, fieldidentity.HashBadgeToken(tok))
		require.NoError(t, err)
		require.NotNil(t, b)
	}
	assert.Equal(t, http.StatusForbidden, serve(fieldCaller([]string{acme}, permFieldWorkerView), nativePost("/ui/field-workers/badges/print", "ids", ann.ID)).code)

	// Sign in as: a manager may for the test identity only; the permission for a
	// real worker. The code is stored under its hash.
	assert.Equal(t, http.StatusForbidden, serve(manager, nativePost("/ui/field-workers/"+ann.ID+"/sign-in-as", "app", appCode)).code)
	assert.NotContains(t, serve(manager, req{target: "/ui/field-workers/" + ann.ID, datastar: true}).body, "sign-in-as")
	assert.Contains(t, serve(manager, req{target: "/ui/field-workers/" + tess.ID, datastar: true}).body, "/sign-in-as")
	res = serve(manager, postForm("/ui/field-workers/"+tess.ID+"/sign-in-as", "app", appCode))
	require.Equal(t, http.StatusOK, res.code, res.body)
	code := regexp.MustCompile(`FWS1[0-9A-Z]{26}`).FindString(res.body)
	require.NotEmpty(t, code, res.body)
	assert.Contains(t, res.body, "data-expires=")
	var codeRows int
	require.NoError(t, pool.QueryRow(ctx, `SELECT COUNT(*) FROM field_sign_in_codes WHERE identity_id = $1 AND code_hash = $2 AND used_at IS NULL`,
		tess.ID, fieldidentity.HashBadgeToken(code)).Scan(&codeRows))
	assert.Equal(t, 1, codeRows, "the code is stored under its hash")
	res = serve(supervisorAdmin, postForm("/ui/field-workers/"+ann.ID+"/sign-in-as", "app", appCode))
	require.Equal(t, http.StatusOK, res.code, res.body)
	assert.NotEmpty(t, regexp.MustCompile(`FWS1[0-9A-Z]{26}`).FindString(res.body))

	// The history shows the method and resolves the actor.
	require.NoError(t, repos.Sessions.RecordLoginAttempt(ctx, fieldidentity.LoginAttempt{
		ClientID: acme, IdentityID: tess.ID, WorkerCode: "SO-TESS", ApplicationID: appID, AuthMethod: fieldidentity.MethodActAs,
		ActorID: ann.ID, Outcome: fieldidentity.OutcomeSuccess,
	}))
	hist := serve(manager, req{target: "/ui/field-workers/" + tess.ID + "/sign-ins", datastar: true}).body
	assert.Contains(t, hist, "ACT_AS")
	assert.Contains(t, hist, "Ann Lee (SO-ANN)")

	// Policy: the sign-in options are saved and read back, and a save of the
	// page keeps them.
	res = serve(manager, postForm("/ui/field-policy", "client", acme, "requireEnrolledDevice", "1", "badgeOnlyLogin", "1",
		"unenrolledIdleSecs", "1200", "impersonationMaxSecs", "7200"))
	assert.Contains(t, res.body, "Field policy saved")
	p, err := repos.Policies.Effective(ctx, acme)
	require.NoError(t, err)
	assert.True(t, p.RequireEnrolledDevice)
	assert.True(t, p.BadgeOnlyLogin)
	assert.Equal(t, 1200, p.UnenrolledIdleSecs)
	assert.Equal(t, 7200, p.ImpersonationMaxSecs)
	page := serve(manager, req{target: "/ui/field-policy?client=" + acme}).body
	assert.Regexp(t, `name="requireEnrolledDevice"[^>]*checked`, page)
	assert.Regexp(t, `name="badgeOnlyLogin"[^>]*checked`, page)
	res = serve(manager, postForm("/ui/field-policy", "client", acme, "requireEnrolledDevice", "1", "badgeOnlyLogin", "1",
		"unenrolledIdleSecs", "1200", "impersonationMaxSecs", "7200", "pinMinLength", "7"))
	assert.Contains(t, res.body, "Field policy saved")
	p, err = repos.Policies.Effective(ctx, acme)
	require.NoError(t, err)
	assert.True(t, p.RequireEnrolledDevice && p.BadgeOnlyLogin, "a save of the page lost a sign-in switch")
	assert.Equal(t, 7, p.PINMinLength)
}
