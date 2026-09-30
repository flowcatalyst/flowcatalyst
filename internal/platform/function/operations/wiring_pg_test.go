//go:build integration

// wiring_pg_test.go covers work packages 4 and 8 (docs/function-runner-plan.md
// §8.2, §8.5): publish + validation, versions/aliases/settings, and the
// promote wiring reconciliation. Publish uses two kinds of artifact:
//
//   - internal/functions/fnfixture's Rust module (fixed describe: one
//     "/{rest...}" endpoint, auth none, config ["GREETING"]) for the
//     simplest happy-path/idempotency checks.
//   - hand-written JS scripts (run on the shared JS engine,
//     internal/functions/runtimes) whose fc_describe() return value each
//     test controls directly — the only practical way to exercise
//     subscriptions/schedules/emits/settings without a second compiled Rust
//     fixture per scenario.
package operations_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/fnfixture"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ── test infrastructure ─────────────────────────────────────────────────

// newTestLoader builds a tiny wazero engine + loader, mirroring
// internal/functions/runner/js_test.go's jsVersion helper and
// internal/server/wire_services.go's platform-side construction.
func newTestLoader(t *testing.T) *runtimes.Loader {
	t.Helper()
	b, err := budget.New(256<<20, 0)
	require.NoError(t, err)
	e, err := engine.New(context.Background(), engine.Config{Budget: b})
	require.NoError(t, err)
	t.Cleanup(func() { _ = e.Close(context.Background()) })
	return runtimes.NewLoader(e)
}

// jsArtifact builds a minimal JS artifact whose fc_describe() returns
// describeJSON verbatim; handle() is never exercised by these tests (only
// fc_describe runs, no-capability, at publish — plan §3).
func jsArtifact(describeJSON string) []byte {
	return []byte(fmt.Sprintf(`globalThis.__fc = {
  describe() { return %s; },
  handle(meta, body) { return { meta: JSON.stringify({status:200,headers:{}}), body: new Uint8Array(0) }; },
};`, "`"+describeJSON+"`"))
}

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// putArtifact uploads b to store under its own digest and returns the digest.
func putArtifactBytes(t *testing.T, store artifact.Store, b []byte) string {
	t.Helper()
	digest := digestOf(b)
	require.NoError(t, store.Put(context.Background(), digest, byteReader(b)))
	return digest
}

func byteReader(b []byte) *bytesReaderCloser { return &bytesReaderCloser{b: b} }

// bytesReaderCloser adapts a []byte to io.Reader without importing
// bytes.Reader's Close-less type into every call site.
type bytesReaderCloser struct {
	b   []byte
	pos int
}

func (r *bytesReaderCloser) Read(p []byte) (int, error) {
	if r.pos >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.pos:])
	r.pos += n
	return n, nil
}

// testFunctionFixture bundles the common dependencies + a seeded function
// for one test.
type testFunctionFixture struct {
	t         *testing.T
	pool      *pgxpool.Pool
	repo      *function.Repository
	apps      *application.Repository
	sas       *serviceaccount.Repository
	uow       *usecasepgx.UnitOfWork
	artifacts artifact.Store
	loader    *runtimes.Loader
	wiring    operations.WiringDeps

	appID, appCode string
	fn             *function.Function
}

func newTestFunctionFixture(t *testing.T, appIDSeed, appCodeSeed, fnName string) *testFunctionFixture {
	t.Helper()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	apps := application.NewRepository(pool)
	sas := serviceaccount.NewRepository(pool)
	uow := testpg.NewUoW(t)
	store, err := artifact.NewFileStore(t.TempDir())
	require.NoError(t, err)

	seedApplication(t, pool, appIDSeed, appCodeSeed)
	ev := mustCreate(t, repo, apps, uow, appIDSeed, fnName)
	fn, err := repo.FindByID(context.Background(), ev.FunctionID)
	require.NoError(t, err)
	require.NotNil(t, fn)

	return &testFunctionFixture{
		t: t, pool: pool, repo: repo, apps: apps, sas: sas, uow: uow,
		artifacts: store, loader: newTestLoader(t), wiring: testWiring(pool),
		appID: appIDSeed, appCode: appCodeSeed, fn: fn,
	}
}

// seedActiveServiceAccount inserts an active SERVICE account bound to the
// fixture's application, with the caller supplying webhook credentials so
// the row is a realistic signing account.
func (f *testFunctionFixture) seedActiveServiceAccount(code string) *serviceaccount.ServiceAccount {
	f.t.Helper()
	sa := serviceaccount.New(code, code)
	sa.ApplicationID = ids.PtrOf[ids.ApplicationID](&f.appID)
	token, secret := "tok_"+code, "sig_"+code
	sa.WebhookCredentials = serviceaccount.WebhookCredentials{
		AuthType:      serviceaccount.AuthBearer,
		Token:         &token,
		SigningSecret: &secret,
	}
	tx, err := f.pool.Begin(context.Background())
	require.NoError(f.t, err)
	require.NoError(f.t, f.sas.Persist(context.Background(), sa, usecasepgx.WrapTxForBootstrap(tx)))
	require.NoError(f.t, tx.Commit(context.Background()))
	f.t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM iam_service_accounts WHERE id = $1`, sa.ID)
	})
	return sa
}

// publish runs the Publish operation as anchor and requires success.
func (f *testFunctionFixture) publish(digest, runtime string) operations.PublishResult {
	f.t.Helper()
	res, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.Publish(f.repo, f.artifacts, f.loader),
		operations.PublishCommand{FunctionID: f.fn.ID, Digest: digest, Runtime: runtime}, testpg.TestEC())
	require.NoError(f.t, err)
	return res
}

// markReady flips a version straight to READY, bypassing the runner
// heartbeat (WP5) this package doesn't own — the same shortcut
// repository_pg_test.go's TestRepository_Version_RoundTrip uses.
func (f *testFunctionFixture) markReady(number int32) {
	f.t.Helper()
	v, err := f.repo.GetVersionByNumber(context.Background(), f.fn.ID, number)
	require.NoError(f.t, err)
	require.NotNil(f.t, v)
	require.NoError(f.t, f.repo.SetVersionReady(context.Background(), v.ID, time.Now().UTC()))
}

func (f *testFunctionFixture) putAlias(name string, number int32) (operations.PutAliasResult, error) {
	f.t.Helper()
	return usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.PutAlias(f.repo, f.wiring),
		operations.PutAliasCommand{FunctionID: f.fn.ID, Name: name, Number: number}, testpg.TestEC())
}

func (f *testFunctionFixture) deleteAlias(name string) (operations.DeleteAliasResult, error) {
	f.t.Helper()
	return usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.DeleteAlias(f.repo, f.wiring),
		operations.DeleteAliasCommand{FunctionID: f.fn.ID, Name: name}, testpg.TestEC())
}

// ── Publish ──────────────────────────────────────────────────────────────

func TestPublish_Fixture_HappyPath_And_Idempotent(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optpubfix001", "optpubfixapp", "fix-fn")

	digest := putArtifactBytes(t, f.artifacts, fnfixture.Wasm)
	res := f.publish(digest, "")
	assert.True(t, res.Created)
	assert.Equal(t, int32(1), res.Version.Number)
	assert.Equal(t, function.DefaultRuntime, res.Version.Runtime)
	assert.Equal(t, function.VersionPublished, res.Version.Status)

	var d struct {
		Config []string `json:"config"`
	}
	require.NoError(t, json.Unmarshal(res.Version.Describe, &d))
	assert.Contains(t, d.Config, "GREETING")

	// Idempotent replay: same digest, no new row, Created=false.
	replay := f.publish(digest, "")
	assert.False(t, replay.Created)
	assert.Equal(t, res.Version.ID, replay.Version.ID)

	versions, err := f.repo.ListVersionsByFunction(context.Background(), f.fn.ID)
	require.NoError(t, err)
	assert.Len(t, versions, 1, "a same-digest replay must not create a second row")
}

func TestPublish_ArtifactNotFound(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optpubnf00001", "optpubnfapp", "nf-fn")
	_, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.Publish(f.repo, f.artifacts, f.loader),
		operations.PublishCommand{FunctionID: f.fn.ID, Digest: digestOf([]byte("never uploaded"))}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "ARTIFACT_NOT_FOUND")
}

func TestPublish_DescribeInvalid_Surfaced(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optpubbad0001", "optpubbadapp", "bad-fn")
	// fc_describe() returns a string that isn't a JSON object at all —
	// abi.ParseDescribe must reject it and the problems must reach the
	// caller (plan §8.2 WP4 task 2).
	src := jsArtifact(`"not an object"`)
	digest := putArtifactBytes(t, f.artifacts, src)

	_, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.Publish(f.repo, f.artifacts, f.loader),
		operations.PublishCommand{FunctionID: f.fn.ID, Digest: digest, Runtime: runtimes.JS}, testpg.TestEC())
	ue := usecase.AsError(err)
	require.NotNil(t, ue)
	assert.Equal(t, usecase.KindValidation, ue.Kind)
	assert.NotEmpty(t, ue.Details["problems"], "the describe problems must be listed on the error")
}

func TestPublish_EmitNotOwned(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optpubemit001", "optpubemitapp", "emit-fn")
	src := jsArtifact(`{"abi":1,"endpoints":[{"path":"/x","auth":"none"}],"emits":["someoneelse:orders:order:shipped"]}`)
	digest := putArtifactBytes(t, f.artifacts, src)

	_, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.Publish(f.repo, f.artifacts, f.loader),
		operations.PublishCommand{FunctionID: f.fn.ID, Digest: digest, Runtime: runtimes.JS}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "EMIT_NOT_OWNED")
}

func TestPublish_BadCron(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optpubcron001", "optpubcronapp", "cron-fn")
	src := jsArtifact(`{"abi":1,"endpoints":[{"path":"/tick","auth":"webhook"}],"schedules":[{"cron":"not a cron","path":"/tick"}]}`)
	digest := putArtifactBytes(t, f.artifacts, src)

	_, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.Publish(f.repo, f.artifacts, f.loader),
		operations.PublishCommand{FunctionID: f.fn.ID, Digest: digest, Runtime: runtimes.JS}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SCHEDULE_CRON_INVALID")
}

// ── Alias / promote preconditions ───────────────────────────────────────

func TestAlias_VersionNotReady(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optaliasnr001", "optaliasnrapp", "nr-fn")
	digest := putArtifactBytes(t, f.artifacts, fnfixture.Wasm)
	res := f.publish(digest, "")
	require.Equal(t, function.VersionPublished, res.Version.Status)

	_, err := f.putAlias("live", res.Version.Number)
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "VERSION_NOT_READY")
}

func TestAlias_SettingsMissing(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optaliassm001", "optaliassmapp", "sm-fn")
	digest := putArtifactBytes(t, f.artifacts, fnfixture.Wasm) // declares config ["GREETING"], none set
	res := f.publish(digest, "")
	f.markReady(res.Version.Number)

	_, err := f.putAlias("live", res.Version.Number)
	ue := usecase.AsError(err)
	require.NotNil(t, ue)
	assert.Equal(t, "SETTINGS_MISSING", ue.Code)
	assert.Contains(t, ue.Message, "GREETING")
}

func TestAlias_SucceedsAfterSettingsSet(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optaliasok001", "optaliasokapp", "ok-fn")
	digest := putArtifactBytes(t, f.artifacts, fnfixture.Wasm)
	res := f.publish(digest, "")
	f.markReady(res.Version.Number)

	_, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.PutSetting(f.repo),
		operations.PutSettingCommand{FunctionID: f.fn.ID, Kind: function.SettingConfig, Key: "GREETING", Value: "hi"}, testpg.TestEC())
	require.NoError(t, err)

	got, err := f.putAlias("live", res.Version.Number)
	require.NoError(t, err)
	assert.Equal(t, res.Version.Number, got.Version.Number)

	// A non-`live` alias never needs settings satisfied and never wires
	// anything (plan §4: "other names are HTTP-only").
	f2 := newTestFunctionFixture(t, "app_optaliascan01", "optaliascanapp", "canary-fn")
	d2 := putArtifactBytes(t, f2.artifacts, fnfixture.Wasm)
	r2 := f2.publish(d2, "")
	f2.markReady(r2.Version.Number)
	canary, err := f2.putAlias("canary", r2.Version.Number)
	require.NoError(t, err, "a non-live alias must not require declared settings")
	assert.Zero(t, canary.Wiring.DispatchPoolCode, "a non-live alias must not wire anything")
}

// ── Promote wiring (WP8) ────────────────────────────────────────────────

// jsDescribeWithSub returns a describe JSON declaring one subscription, one
// schedule, and the config key GREETING, all under paths the endpoints list
// accepts as webhook targets. Literal JSON text — jsArtifact's describe()
// returns its argument verbatim as a template-literal string (no code
// execution), so this must already BE the describe document's JSON bytes,
// not a JS expression that would produce them.
func jsDescribeWithSub(eventType string) string {
	return fmt.Sprintf(`{
		"abi":1,
		"endpoints":[{"path":"/on-order","auth":"webhook"},{"path":"/tick","auth":"webhook"}],
		"subscriptions":[{"eventType":%q,"path":"/on-order"}],
		"schedules":[{"cron":"0 */5 * * * *","path":"/tick"}],
		"config":["GREETING"]
	}`, eventType)
}

func TestAlias_Live_PromoteWiring_CreatesPoolSubscriptionAndSchedule(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optwirepromo1", "optwirepromoapp", "promo-fn")
	sa := f.seedActiveServiceAccount("optwirepromoapp-sa")

	src := jsArtifact(jsDescribeWithSub("optwirepromoapp:orders:order:created"))
	digest := putArtifactBytes(t, f.artifacts, src)
	res := f.publish(digest, runtimes.JS)
	f.markReady(res.Version.Number)

	require.NoError(t, setConfig(f, "GREETING", "hi"))
	got, err := f.putAlias("live", res.Version.Number)
	require.NoError(t, err)

	wantPool := "fn-" + f.appCode + "-promo-fn"
	assert.Equal(t, wantPool, got.Wiring.DispatchPoolCode)
	assert.Equal(t, 1, got.Wiring.SubscriptionsCreated)
	assert.Equal(t, 1, got.Wiring.SchedulesCreated)

	pool, err := dispatchpool.NewRepository(f.pool).FindByCode(context.Background(), wantPool, nil)
	require.NoError(t, err)
	require.NotNil(t, pool, "promote must create the function's dispatch pool")
	assert.Equal(t, dispatchpool.StatusActive, pool.Status)
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM msg_dispatch_pools WHERE code = $1`, wantPool)
	})

	subs, err := subscription.NewRepository(f.pool).FindByFunctionID(context.Background(), f.fn.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	sub := subs[0]
	assert.Equal(t, subscription.SourceFunction, sub.Source)
	require.NotNil(t, sub.FunctionID)
	assert.Equal(t, f.fn.ID, *sub.FunctionID)
	require.NotNil(t, sub.ServiceAccountID)
	assert.Equal(t, sa.ID, *sub.ServiceAccountID, "the subscription's explicit service account must be the application's oldest active account")
	assert.Equal(t, common.DefaultDispatchMode, sub.Mode, "an omitted describe mode must default to NEXT_ON_ERROR, never IMMEDIATE")
	require.Len(t, sub.EventTypes, 1)
	assert.Equal(t, "optwirepromoapp:orders:order:created", sub.EventTypes[0].EventTypeCode)
	require.NotNil(t, sub.DispatchPoolCode)
	assert.Equal(t, wantPool, *sub.DispatchPoolCode)
	require.NotNil(t, sub.DispatchPoolID, "dispatch jobs take the subscription's pool ID; the code alone leaves every job without a pool")
	assert.Equal(t, pool.ID, *sub.DispatchPoolID)
	assert.Contains(t, sub.Endpoint, "/fn/"+f.fn.Address+"/on-order")

	jobs, err := scheduledjob.NewRepository(f.pool).FindByFunctionID(context.Background(), f.fn.ID)
	require.NoError(t, err)
	require.Len(t, jobs, 1)
	job := jobs[0]
	require.NotNil(t, job.FunctionID)
	assert.Equal(t, f.fn.ID, *job.FunctionID)
	require.NotNil(t, job.ApplicationID)
	assert.Equal(t, f.appID, string(*job.ApplicationID))
	require.NotNil(t, job.TargetURL)
	assert.Contains(t, *job.TargetURL, "/fn/"+f.fn.Address+"/tick")
	assert.Equal(t, []string{"0 */5 * * * *"}, job.Crons)

	// Confirming the resolver chain: the SAME lookup
	// (FindFirstByApplicationID) that server.newDeliveryCredsResolver's step
	// 1 ("the subscription's own serviceAccountId") would use when the
	// dispatcher resolves this subscription's delivery credentials.
	saRepo := serviceaccount.NewRepository(f.pool)
	resolved, err := saRepo.FindFirstByApplicationID(context.Background(), f.appID)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, sa.ID, resolved.ID)
}

func TestAlias_Live_NoSigningAccount_Refused(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optwirensa001", "optwirensaapp", "nsa-fn")
	// No service account seeded for this application.
	src := jsArtifact(jsDescribeWithSub("optwirensaapp:orders:order:created"))
	digest := putArtifactBytes(t, f.artifacts, src)
	res := f.publish(digest, runtimes.JS)
	f.markReady(res.Version.Number)
	require.NoError(t, setConfig(f, "GREETING", "hi"))

	_, err := f.putAlias("live", res.Version.Number)
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "NO_SIGNING_ACCOUNT")

	subs, err := subscription.NewRepository(f.pool).FindByFunctionID(context.Background(), f.fn.ID)
	require.NoError(t, err)
	assert.Empty(t, subs, "a refused promote must not partially wire")
}

func TestAlias_Live_SecondPromote_UpdatesAndDeletes(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optwire2nd001", "optwire2ndapp", "second-fn")
	f.seedActiveServiceAccount("optwire2ndapp-sa")

	src1 := jsArtifact(jsDescribeWithSub("optwire2ndapp:orders:order:created"))
	d1 := putArtifactBytes(t, f.artifacts, src1)
	r1 := f.publish(d1, runtimes.JS)
	f.markReady(r1.Version.Number)
	require.NoError(t, setConfig(f, "GREETING", "hi"))
	_, err := f.putAlias("live", r1.Version.Number)
	require.NoError(t, err)

	subRepo := subscription.NewRepository(f.pool)
	before, err := subRepo.FindByFunctionID(context.Background(), f.fn.ID)
	require.NoError(t, err)
	require.Len(t, before, 1)
	firstSubID := before[0].ID

	// v2 declares a DIFFERENT event type on the same path: a different
	// identity (function_id, eventType, path), so this must delete the old
	// row and create a new one, not update in place.
	src2 := jsArtifact(jsDescribeWithSub("optwire2ndapp:orders:order:cancelled"))
	d2 := putArtifactBytes(t, f.artifacts, src2)
	r2 := f.publish(d2, runtimes.JS)
	f.markReady(r2.Version.Number)
	got, err := f.putAlias("live", r2.Version.Number)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Wiring.SubscriptionsCreated)
	assert.Equal(t, 1, got.Wiring.SubscriptionsDeleted)

	after, err := subRepo.FindByFunctionID(context.Background(), f.fn.ID)
	require.NoError(t, err)
	require.Len(t, after, 1)
	assert.NotEqual(t, firstSubID, after[0].ID)
	assert.Equal(t, "optwire2ndapp:orders:order:cancelled", after[0].EventTypes[0].EventTypeCode)

	// Rolling back to v1 restores the original wiring.
	rollback, err := f.putAlias("live", r1.Version.Number)
	require.NoError(t, err)
	assert.Equal(t, 1, rollback.Wiring.SubscriptionsCreated)
	assert.Equal(t, 1, rollback.Wiring.SubscriptionsDeleted)
	restored, err := subRepo.FindByFunctionID(context.Background(), f.fn.ID)
	require.NoError(t, err)
	require.Len(t, restored, 1)
	assert.Equal(t, "optwire2ndapp:orders:order:created", restored[0].EventTypes[0].EventTypeCode)
}

func TestAlias_Live_Delete_RemovesWiringButKeepsPool(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optwiredel001", "optwiredelapp", "del-fn")
	f.seedActiveServiceAccount("optwiredelapp-sa")

	src := jsArtifact(jsDescribeWithSub("optwiredelapp:orders:order:created"))
	digest := putArtifactBytes(t, f.artifacts, src)
	res := f.publish(digest, runtimes.JS)
	f.markReady(res.Version.Number)
	require.NoError(t, setConfig(f, "GREETING", "hi"))
	_, err := f.putAlias("live", res.Version.Number)
	require.NoError(t, err)

	wantPool := "fn-" + f.appCode + "-del-fn"
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM msg_dispatch_pools WHERE code = $1`, wantPool)
	})

	unwired, err := f.deleteAlias("live")
	require.NoError(t, err)
	assert.Equal(t, 1, unwired.Wiring.SubscriptionsDeleted)
	assert.Equal(t, 1, unwired.Wiring.SchedulesDeleted)

	subs, err := subscription.NewRepository(f.pool).FindByFunctionID(context.Background(), f.fn.ID)
	require.NoError(t, err)
	assert.Empty(t, subs, "unset live must remove every function-owned subscription")
	jobs, err := scheduledjob.NewRepository(f.pool).FindByFunctionID(context.Background(), f.fn.ID)
	require.NoError(t, err)
	assert.Empty(t, jobs, "unset live must remove every function-owned scheduled job")

	pool, err := dispatchpool.NewRepository(f.pool).FindByCode(context.Background(), wantPool, nil)
	require.NoError(t, err)
	assert.NotNil(t, pool, "the dispatch pool itself is left — deleted only when the function is deleted")
}

// ── Delete cascade ───────────────────────────────────────────────────────

func TestDeleteFunction_CascadesWiringAndReclaimsOrphanArtifact(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optwiredelfn1", "optwiredelfnapp", "cascade-fn")
	f.seedActiveServiceAccount("optwiredelfnapp-sa")

	src := jsArtifact(jsDescribeWithSub("optwiredelfnapp:orders:order:created"))
	digest := putArtifactBytes(t, f.artifacts, src)
	res := f.publish(digest, runtimes.JS)
	f.markReady(res.Version.Number)
	require.NoError(t, setConfig(f, "GREETING", "hi"))
	_, err := f.putAlias("live", res.Version.Number)
	require.NoError(t, err)

	wantPool := "fn-" + f.appCode + "-cascade-fn"
	exists, err := f.artifacts.Exists(context.Background(), digest)
	require.NoError(t, err)
	require.True(t, exists)

	result, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.DeleteFunction(f.repo, f.wiring),
		operations.DeleteCommand{ID: f.fn.ID}, testpg.TestEC())
	require.NoError(t, err)
	require.Contains(t, result.OrphanedDigests, digest)
	require.NoError(t, f.artifacts.Delete(context.Background(), digest)) // mirrors api.go's post-commit cleanup

	stillExists, err := f.artifacts.Exists(context.Background(), digest)
	require.NoError(t, err)
	assert.False(t, stillExists, "an orphaned digest must be reclaimed after delete")

	subs, err := subscription.NewRepository(f.pool).FindByFunctionID(context.Background(), f.fn.ID)
	require.NoError(t, err)
	assert.Empty(t, subs)
	jobs, err := scheduledjob.NewRepository(f.pool).FindByFunctionID(context.Background(), f.fn.ID)
	require.NoError(t, err)
	assert.Empty(t, jobs)
	pool, err := dispatchpool.NewRepository(f.pool).FindByCode(context.Background(), wantPool, nil)
	require.NoError(t, err)
	assert.Nil(t, pool, "the function's own dispatch pool is deleted when the function is deleted")

	got, err := f.repo.FindByID(context.Background(), f.fn.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
}

// ── Settings ─────────────────────────────────────────────────────────────

func TestSettings_SecretAndDB_WriteOnly(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optsetwo00001", "optsetwoapp", "wo-fn")

	for _, kind := range []function.SettingKind{function.SettingSecret, function.SettingDB} {
		key := "K"
		if kind == function.SettingDB {
			key = "maindb"
		}
		setting, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.PutSetting(f.repo),
			operations.PutSettingCommand{FunctionID: f.fn.ID, Kind: kind, Key: key, Value: "s3cr3t"}, testpg.TestEC())
		require.NoError(t, err)
		assert.Empty(t, setting.Value, "PutSetting must never echo a SECRET/DB value back")

		_, list, err := operations_ListSettings(f)
		require.NoError(t, err)
		found := false
		for _, s := range list {
			if s.Kind == kind && s.Key == key {
				found = true
				assert.Empty(t, s.Value, "ListSettings must never return a SECRET/DB value")
			}
		}
		assert.True(t, found)
	}
}

// A SECRET/DB value must never reach the audit log or the event payload, and
// a real DSN or a "://"-bearing secret must be accepted and stored encrypted.
func TestSettings_SecretAndDB_NotAudited_AndURLShapedValuesAccepted(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optsetaud0001", "optsetaudapp", "aud-fn")

	cases := []struct {
		kind  function.SettingKind
		key   string
		value string
	}{
		{function.SettingSecret, "PLAIN", "sentinel-plain-9f3a"},
		{function.SettingSecret, "URLISH", "https://x.example/?token=sentinel-url-7c1d"},
		{function.SettingDB, "maindb", "postgres://u:sentinel-dsn-42be@host/db"},
	}
	for _, c := range cases {
		_, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.PutSetting(f.repo),
			operations.PutSettingCommand{FunctionID: f.fn.ID, Kind: c.kind, Key: c.key, Value: c.value}, testpg.TestEC())
		require.NoError(t, err, c.key)
	}

	// Each stored value is ciphertext, not the input.
	var stored int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM fng_settings WHERE function_id = $1 AND kind IN ('SECRET','DB') AND value LIKE 'encrypted:%'`,
		f.fn.ID).Scan(&stored))
	assert.Equal(t, 3, stored)

	for _, table := range []string{"aud_logs", "msg_events"} {
		var leaked int
		require.NoError(t, f.pool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM `+table+` t WHERE to_jsonb(t)::text LIKE '%sentinel-%' AND to_jsonb(t)::text LIKE '%'||$1||'%'`,
			f.fn.ID).Scan(&leaked))
		assert.Zero(t, leaked, "%s must not contain a SECRET/DB value", table)
	}
}

// ── Retire ───────────────────────────────────────────────────────────────

func TestRetireVersion_RefusesInUse_ThenSucceeds(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optretire0001", "optretireapp", "retire-fn")
	digest := putArtifactBytes(t, f.artifacts, fnfixture.Wasm)
	res := f.publish(digest, "")
	f.markReady(res.Version.Number)
	require.NoError(t, setConfig(f, "GREETING", "hi"))
	_, err := f.putAlias("live", res.Version.Number)
	require.NoError(t, err)

	_, err = usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.RetireVersion(f.repo),
		operations.RetireCommand{FunctionID: f.fn.ID, Number: res.Version.Number}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "RETIRE_IN_USE")

	_, err = f.deleteAlias("live")
	require.NoError(t, err)

	retired, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.RetireVersion(f.repo),
		operations.RetireCommand{FunctionID: f.fn.ID, Number: res.Version.Number}, testpg.TestEC())
	require.NoError(t, err)
	assert.Equal(t, function.VersionRetired, retired.Status)
}

// ── Authz (locked model) ─────────────────────────────────────────────────

// TestPublish_ResourceScope_Denied proves the locked authz model on a
// TxOperation: a client-scoped principal cannot publish to a platform-owned
// function even holding the coarse publish permission.
func TestPublish_ResourceScope_Denied(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optpubscope01", "optpubscopeapp", "scope-fn")
	digest := putArtifactBytes(t, f.artifacts, fnfixture.Wasm)

	clientCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_optpubscope1",
		Scope:       auth.ScopeClient,
		Clients:     []string{"cli_optpubscope01"},
		Permissions: []string{"platform:function:version:publish", "platform:function:alias:promote", "platform:function:function:manage"},
	})
	_, err := usecaseop.RunTx(clientCtx, f.uow, operations.Publish(f.repo, f.artifacts, f.loader),
		operations.PublishCommand{FunctionID: f.fn.ID, Digest: digest}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

	_, err = usecaseop.RunTx(clientCtx, f.uow, operations.PutAlias(f.repo, f.wiring),
		operations.PutAliasCommand{FunctionID: f.fn.ID, Name: "live", Number: 1}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

	_, err = usecaseop.RunTx(clientCtx, f.uow, operations.PutSetting(f.repo),
		operations.PutSettingCommand{FunctionID: f.fn.ID, Kind: function.SettingConfig, Key: "GREETING", Value: "x"}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")
}

// ── small local helpers ───────────────────────────────────────────────────

func setConfig(f *testFunctionFixture, key, value string) error {
	_, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.PutSetting(f.repo),
		operations.PutSettingCommand{FunctionID: f.fn.ID, Kind: function.SettingConfig, Key: key, Value: value}, testpg.TestEC())
	return err
}

func operations_ListSettings(f *testFunctionFixture) (*function.Function, []function.Setting, error) {
	return operations.ListSettings(testpg.AnchorCtx(), f.repo, f.fn.ID)
}

// TestAlias_Live_RunnerPoolVsDispatchPool pins the two pools apart: a
// function's Pool is the RUNNER pool — it fills {pool} in the runner URL and
// its revision is what a promote bumps — while its dispatch pool is always
// its own fn-{address}. Mutant: substitute the dispatch-pool code into the
// runner URL (the endpoint becomes http://fn-fn-…), or bump the dispatch
// pool's revision (the gpu runners never hear of the promote).
func TestAlias_Live_RunnerPoolVsDispatchPool(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optwirepools1", "optwirepoolsapp", "pool-fn")
	f.seedActiveServiceAccount("optwirepoolsapp-sa")
	_, err := f.pool.Exec(context.Background(), `UPDATE fng_functions SET pool = 'gpu' WHERE id = $1`, f.fn.ID)
	require.NoError(t, err)
	f.fn, err = f.repo.FindByID(context.Background(), f.fn.ID)
	require.NoError(t, err)
	f.wiring.RunnerURLTemplate = "http://fn-{pool}:8095"
	before, _, err := f.repo.GetPoolRevision(context.Background(), "gpu")
	require.NoError(t, err)

	src := jsArtifact(jsDescribeWithSub("optwirepoolsapp:orders:order:created"))
	res := f.publish(putArtifactBytes(t, f.artifacts, src), runtimes.JS)
	f.markReady(res.Version.Number)
	require.NoError(t, setConfig(f, "GREETING", "hi"))
	got, err := f.putAlias("live", res.Version.Number)
	require.NoError(t, err)

	dispatchPool := "fn-optwirepoolsapp-pool-fn"
	assert.Equal(t, dispatchPool, got.Wiring.DispatchPoolCode, "the dispatch pool is the function's own, whatever its runner pool")
	t.Cleanup(func() {
		_, _ = f.pool.Exec(context.Background(), `DELETE FROM msg_dispatch_pools WHERE code = $1`, dispatchPool)
	})

	subs, err := subscription.NewRepository(f.pool).FindByFunctionID(context.Background(), f.fn.ID)
	require.NoError(t, err)
	require.Len(t, subs, 1)
	assert.Equal(t, "http://fn-gpu:8095/fn/"+f.fn.Address+"/on-order", subs[0].Endpoint, "{pool} is the runner pool")
	require.NotNil(t, subs[0].DispatchPoolCode)
	assert.Equal(t, dispatchPool, *subs[0].DispatchPoolCode)

	after, _, err := f.repo.GetPoolRevision(context.Background(), "gpu")
	require.NoError(t, err)
	assert.Greater(t, after, before, "the promote must bump the runner pool's revision")
	_, bumped, err := f.repo.GetPoolRevision(context.Background(), dispatchPool)
	require.NoError(t, err)
	assert.False(t, bumped, "the dispatch pool's code is not a runner pool")
}
