//go:build integration

package api

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/pin"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

var seq atomic.Int64

// fixture is one isolated client with two applications and a State.
type fixture struct {
	t        *testing.T
	s        *State
	clientID string
	appA     string // application ids
	appB     string
	codeA    string
	codeB    string
	actx     context.Context
}

func newFixture(t *testing.T, hasher *pin.Hasher) *fixture {
	t.Helper()
	pool := testpg.Pool(t)
	n := seq.Add(1)
	clients := client.NewRepository(pool)
	uow := testpg.NewUoW(t)
	ev, err := usecaseop.Run(context.Background(), uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: fmt.Sprintf("Field Co %d", n), Identifier: fmt.Sprintf("field-co-%d-%d", n, time.Now().UnixNano()%100000)},
		testpg.TestEC())
	require.NoError(t, err)
	codeA, codeB := fmt.Sprintf("fldappa%d", n), fmt.Sprintf("fldappb%d", n)
	f := &fixture{
		t: t, clientID: ev.ClientID, codeA: codeA, codeB: codeB,
		appA: testpg.EnsureApplication(t, codeA), appB: testpg.EnsureApplication(t, codeB),
		actx: anchorAllApps(),
	}
	f.s = &State{
		Ops: &fieldidentity.Ops{
			R: fieldidentity.NewRepos(pool), Clients: clients, Apps: application.NewRepository(pool), Hasher: hasher,
		},
		UoW:     uow,
		BaseURL: "https://fc.example.test/",
	}
	return f
}

// anchorAllApps is a full platform administrator: anchor reach, the super-admin
// wildcard and access to every application (testpg.AnchorCtx has no
// application access).
func anchorAllApps() context.Context {
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_optestrunner1", Scope: auth.ScopeAnchor, Permissions: []string{"platform:*:*:*"},
		AllApplications: true, PrincipalType: auth.PrincipalTypeUser,
	})
}

func hasher(t *testing.T) *pin.Hasher {
	h, err := pin.NewHasher("field-test-app-key")
	require.NoError(t, err)
	return h
}

func (f *fixture) ensure(ctx context.Context, code, name string, apps ...string) *ensureOutput {
	f.t.Helper()
	out, err := f.s.ensureWorker(ctx, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: f.clientID, WorkerCode: code, Name: name, AppCodes: apps,
	}})
	require.NoError(f.t, err)
	return out
}

// device inserts an ACTIVE device for the given app ids.
func (f *fixture) device(mode fieldidentity.Mode, assigned *string, appIDs ...string) *fieldidentity.Device {
	f.t.Helper()
	d := &fieldidentity.Device{
		ID: tsid.Generate(tsid.FieldDevice), ClientID: f.clientID, Mode: mode, Label: "test",
		JKT: "jkt-" + tsid.GenerateUntyped(), PublicKeyJWK: "{}", AttestationLevel: "NONE",
		AssignedIdentityID: assigned, Status: fieldidentity.DeviceActive, EnrolledAt: time.Now().UTC(), AppIDs: appIDs,
	}
	tx, err := testpg.Pool(f.t).Begin(context.Background())
	require.NoError(f.t, err)
	require.NoError(f.t, f.s.Ops.R.Devices.Persist(context.Background(), d, usecaseTx(tx)))
	require.NoError(f.t, tx.Commit(context.Background()))
	return d
}

// session inserts a live session for (worker, device, app) in a fresh family.
func (f *fixture) session(workerID, deviceID, appID string) *fieldidentity.Session {
	f.t.Helper()
	now := time.Now().UTC()
	id := fieldidentity.NewSessionID()
	rt := "fwr_" + tsid.GenerateUntyped()
	s := &fieldidentity.Session{
		ID: id, FamilyID: id, IdentityID: workerID, DeviceID: deviceID, ApplicationID: appID,
		RefreshHash: fieldidentity.HashToken(rt), AMR: []string{"pin"}, AuthTime: now, IssuedAt: now,
		IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(8 * time.Hour),
	}
	require.NoError(f.t, f.s.Ops.R.Sessions.Insert(context.Background(), s))
	return s
}

func (f *fixture) sessionState(id string) (revoked bool, reason string) {
	f.t.Helper()
	var at *time.Time
	var r *string
	require.NoError(f.t, testpg.Pool(f.t).QueryRow(context.Background(),
		`SELECT revoked_at, revoked_reason FROM field_sessions WHERE id = $1`, id).Scan(&at, &r))
	if r != nil {
		reason = *r
	}
	return at != nil, reason
}

func (f *fixture) eventCount(eventType, subjectID string) int {
	f.t.Helper()
	var n int
	require.NoError(f.t, testpg.Pool(f.t).QueryRow(context.Background(),
		`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND subject LIKE '%' || $2`, eventType, subjectID).Scan(&n))
	return n
}

// callerCtx builds a non-anchor caller.
func callerCtx(clients []string, perms []string, apps []string, allApps bool) context.Context {
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_fieldcaller01", Scope: auth.ScopeClient, Clients: clients,
		Permissions: perms, Applications: apps, AllApplications: allApps, PrincipalType: auth.PrincipalTypeService,
	})
}

func requireErr(t *testing.T, err error, kind usecase.Kind, code string) {
	t.Helper()
	testpg.RequireUsecaseError(t, err, kind, code)
}

// ── ensure ───────────────────────────────────────────────────────────────

func TestEnsureWorkerIsIdempotentAndAdditive(t *testing.T) {
	f := newFixture(t, hasher(t))
	first := f.ensure(f.actx, "w-001", "Ada Driver", f.codeA)
	assert.Equal(t, 201, first.Status)
	require.NotNil(t, first.Body.Created)
	assert.True(t, *first.Body.Created)
	assert.Equal(t, "W-001", first.Body.WorkerCode, "worker code is normalised upper-case")
	assert.False(t, first.Body.HasPin, "no PIN unless supplied")
	require.Len(t, first.Body.Apps, 1)
	assert.Equal(t, f.codeA, first.Body.Apps[0].Code)
	assert.Equal(t, 1, f.eventCount(fieldidentity.WorkerCreatedType, first.Body.ID))

	// Re-ensure: same id, created=false, name not overwritten, grants added.
	again := f.ensure(f.actx, " w-001 ", "Someone Else", f.codeB)
	assert.Equal(t, 200, again.Status)
	assert.False(t, *again.Body.Created)
	assert.Equal(t, first.Body.ID, again.Body.ID)
	assert.Equal(t, "Ada Driver", again.Body.Name, "an existing worker's fields are not overwritten")
	codes := []string{}
	for _, a := range again.Body.Apps {
		codes = append(codes, a.Code)
	}
	assert.ElementsMatch(t, []string{f.codeA, f.codeB}, codes)

	// Same call again changes nothing.
	third := f.ensure(f.actx, "W-001", "x", f.codeA, f.codeB)
	assert.Len(t, third.Body.Apps, 2)

	// Unknown application.
	_, err := f.s.ensureWorker(f.actx, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: f.clientID, WorkerCode: "W-002", Name: "N", AppCodes: []string{"no-such-app"},
	}})
	requireErr(t, err, usecase.KindValidation, "UNKNOWN_APPLICATION")
}

func TestEnsureWorkerWithSuppliedPin(t *testing.T) {
	f := newFixture(t, hasher(t))
	weak := "123456"
	_, err := f.s.ensureWorker(f.actx, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: f.clientID, WorkerCode: "W-PIN", Name: "N", Pin: &weak,
	}})
	requireErr(t, err, usecase.KindValidation, "PIN_POLICY")
	ident, ferr := f.s.Ops.R.Identities.FindByClientAndCode(context.Background(), f.clientID, "W-PIN")
	require.NoError(t, ferr)
	assert.Nil(t, ident, "a rejected PIN creates nothing")

	good := "482915"
	out, err := f.s.ensureWorker(f.actx, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: f.clientID, WorkerCode: "W-PIN", Name: "N", Pin: &good,
	}})
	require.NoError(t, err)
	assert.True(t, out.Body.HasPin)
	assert.True(t, out.Body.PinChangeRequired, "a supervisor-entered PIN must be changed by the worker")
	ident, _ = f.s.Ops.R.Identities.FindByClientAndCode(context.Background(), f.clientID, "W-PIN")
	ok, _, verr := f.s.Ops.Hasher.Verify(ident.ClientID, ident.ID, good, *ident.PINHash, ident.PINScheme)
	require.NoError(t, verr)
	assert.True(t, ok)
}

func TestMissingAppKeyFailsClosed(t *testing.T) {
	f := newFixture(t, nil) // no pepper
	good := "482915"
	_, err := f.s.ensureWorker(f.actx, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: f.clientID, WorkerCode: "W-NOKEY", Name: "N", Pin: &good,
	}})
	require.Error(t, err)
	assert.Equal(t, 503, statusOf(err))

	// Without a PIN the worker can still be created, and a reset fails closed.
	out := f.ensure(f.actx, "W-NOKEY2", "N")
	_, err = f.s.resetPin(f.actx, &resetPinInput{ID: out.Body.ID})
	require.Error(t, err)
	assert.Equal(t, 503, statusOf(err))
}

func statusOf(err error) int {
	if se, ok := err.(interface{ GetStatus() int }); ok {
		return se.GetStatus()
	}
	return 0
}

// ── pin reset / unlock / suspend ─────────────────────────────────────────

func TestResetPinTemporaryOnceRevokesSessionsClearsLock(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "W-RESET", "Reset Me", f.codeA).Body
	dev := f.device(fieldidentity.ModeShared, nil, f.appA)
	s1, s2 := f.session(w.ID, dev.ID, f.appA), f.session(w.ID, dev.ID, f.appA)
	// Lock the worker.
	_, err := testpg.Pool(t).Exec(context.Background(),
		`UPDATE field_identities SET failed_since_success = 10, hard_locked = TRUE, locked_until = NOW() + INTERVAL '1 hour' WHERE id = $1`, w.ID)
	require.NoError(t, err)

	out, err := f.s.resetPin(f.actx, &resetPinInput{ID: w.ID})
	require.NoError(t, err)
	require.NotNil(t, out.Body.TemporaryPin)
	assert.Len(t, *out.Body.TemporaryPin, pin.DefaultMinLength)
	assert.Empty(t, pin.Check(*out.Body.TemporaryPin, "W-RESET", 6), "the temporary PIN is policy-valid")
	assert.True(t, out.Body.PinChangeRequired)

	ident, err := f.s.Ops.R.Identities.FindByID(context.Background(), w.ID)
	require.NoError(t, err)
	ok, _, verr := f.s.Ops.Hasher.Verify(ident.ClientID, ident.ID, *out.Body.TemporaryPin, *ident.PINHash, ident.PINScheme)
	require.NoError(t, verr)
	assert.True(t, ok, "the stored hash verifies the returned PIN")
	assert.True(t, ident.PINChangeRequired)
	assert.False(t, ident.HardLocked)
	assert.Nil(t, ident.LockedUntil)
	assert.Zero(t, ident.FailedSinceSuccess)

	for _, s := range []*fieldidentity.Session{s1, s2} {
		revoked, reason := f.sessionState(s.ID)
		assert.True(t, revoked)
		assert.Equal(t, fieldidentity.ReasonPinChanged, reason)
	}
	assert.Equal(t, 1, f.eventCount(fieldidentity.WorkerPinResetType, w.ID))
	assert.Equal(t, 1, f.eventCount(fieldidentity.SessionRevokedType, w.ID))

	// The temporary PIN is not stored anywhere readable: not in the event, not
	// in the audit command.
	var n int
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT (SELECT COUNT(*) FROM msg_events WHERE data::text LIKE '%' || $1 || '%')
		      + (SELECT COUNT(*) FROM aud_logs WHERE operation_json::text LIKE '%' || $1 || '%')`,
		*out.Body.TemporaryPin).Scan(&n))
	assert.Zero(t, n, "the PIN must not appear in events or audit")

	// A supervisor-chosen PIN is not echoed back and must pass policy.
	weak := "000000"
	_, err = f.s.resetPin(f.actx, &resetPinInput{ID: w.ID, Body: ResetFieldWorkerPinRequest{Pin: &weak}})
	requireErr(t, err, usecase.KindValidation, "PIN_POLICY")
	chosen := "715284"
	out, err = f.s.resetPin(f.actx, &resetPinInput{ID: w.ID, Body: ResetFieldWorkerPinRequest{Pin: &chosen}})
	require.NoError(t, err)
	assert.Nil(t, out.Body.TemporaryPin)
	assert.True(t, out.Body.PinChangeRequired)
}

func TestUnlockSuspendActivate(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "W-LOCK", "Lock Me", f.codeA).Body
	dev := f.device(fieldidentity.ModePersonal, nil, f.appA)
	sess := f.session(w.ID, dev.ID, f.appA)
	_, err := testpg.Pool(t).Exec(context.Background(),
		`UPDATE field_identities SET failed_since_success = 7, hard_locked = TRUE, locked_until = NOW() + INTERVAL '1 hour' WHERE id = $1`, w.ID)
	require.NoError(t, err)

	_, err = f.s.unlockWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	got, err := f.s.getWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	assert.False(t, got.Body.Locked)
	assert.False(t, got.Body.HardLocked)
	assert.Zero(t, got.Body.FailedSinceSuccess)
	revoked, _ := f.sessionState(sess.ID)
	assert.False(t, revoked, "unlock does not touch sessions")

	_, err = f.s.suspendWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	got, _ = f.s.getWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	assert.Equal(t, "DISABLED", got.Body.Status)
	revoked, reason := f.sessionState(sess.ID)
	assert.True(t, revoked)
	assert.Equal(t, fieldidentity.ReasonWorkerSuspended, reason)
	assert.Equal(t, 1, f.eventCount(fieldidentity.WorkerSuspendedType, w.ID))
	assert.Equal(t, 1, f.eventCount(fieldidentity.SessionRevokedType, w.ID))

	_, err = f.s.activateWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	got, _ = f.s.getWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	assert.Equal(t, "ACTIVE", got.Body.Status)
	assert.Equal(t, 1, f.eventCount(fieldidentity.WorkerActivatedType, w.ID))
}

// ── grants ───────────────────────────────────────────────────────────────

func TestGrantAndRevokeApp(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "W-GRANT", "Grant Me", f.codeA).Body
	dev := f.device(fieldidentity.ModeShared, nil, f.appA, f.appB)

	_, err := f.s.grantApp(f.actx, &grantAppInput{ID: w.ID, Body: GrantFieldWorkerAppRequest{AppCode: f.codeB}})
	require.NoError(t, err)
	_, err = f.s.grantApp(f.actx, &grantAppInput{ID: w.ID, Body: GrantFieldWorkerAppRequest{AppCode: f.codeB}})
	require.NoError(t, err, "granting twice is idempotent")
	got, _ := f.s.getWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	assert.Len(t, got.Body.Apps, 2)

	sA, sB := f.session(w.ID, dev.ID, f.appA), f.session(w.ID, dev.ID, f.appB)
	_, err = f.s.revokeApp(f.actx, &revokeAppInput{ID: w.ID, AppCode: f.codeB})
	require.NoError(t, err)
	got, _ = f.s.getWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	require.Len(t, got.Body.Apps, 1)
	assert.Equal(t, f.codeA, got.Body.Apps[0].Code)
	revokedB, reasonB := f.sessionState(sB.ID)
	assert.True(t, revokedB, "revoking the grant revokes that application's sessions")
	assert.Equal(t, fieldidentity.ReasonAdmin, reasonB)
	revokedA, _ := f.sessionState(sA.ID)
	assert.False(t, revokedA, "the worker's other application is untouched")
	// Revoking again is a no-op.
	_, err = f.s.revokeApp(f.actx, &revokeAppInput{ID: w.ID, AppCode: f.codeB})
	require.NoError(t, err)
}

// ── update / delete / list ───────────────────────────────────────────────

func TestUpdateDeleteAndList(t *testing.T) {
	f := newFixture(t, hasher(t))
	a := f.ensure(f.actx, "W-LIST-A", "Alice Mover", f.codeA).Body
	f.ensure(f.actx, "W-LIST-B", "Bob Picker", f.codeB)
	f.ensure(f.actx, "X-OTHER", "Carol")

	upd, err := f.s.updateWorker(f.actx, &updateWorkerInput{ID: a.ID, Body: UpdateFieldWorkerRequest{Name: "Alice M"}})
	require.NoError(t, err)
	assert.Equal(t, "Alice M", upd.Body.Name)
	assert.Equal(t, 1, f.eventCount(fieldidentity.WorkerUpdatedType, a.ID))

	list := func(in listWorkersInput) FieldWorkerListResponse {
		in.ClientID = f.clientID
		out, err := f.s.listWorkers(f.actx, &in)
		require.NoError(t, err)
		return out.Body
	}
	codes := func(r FieldWorkerListResponse) []string {
		out := []string{}
		for _, w := range r.Workers {
			out = append(out, w.WorkerCode)
		}
		slices.Sort(out)
		return out
	}
	assert.Equal(t, []string{"W-LIST-A", "W-LIST-B", "X-OTHER"}, codes(list(listWorkersInput{})))
	assert.Equal(t, []string{"W-LIST-A", "W-LIST-B"}, codes(list(listWorkersInput{Q: "w-li"})), "worker-code prefix")
	assert.Equal(t, []string{"W-LIST-A"}, codes(list(listWorkersInput{Q: "ALICE"})), "name prefix, case-insensitive")
	assert.Equal(t, []string{"W-LIST-B"}, codes(list(listWorkersInput{AppCode: f.codeB})))
	assert.Empty(t, codes(list(listWorkersInput{Q: "W%"})), "% is literal")
	assert.EqualValues(t, 3, list(listWorkersInput{PageSize: 2}).Total)
	assert.Len(t, list(listWorkersInput{PageSize: 2, Page: 1}).Workers, 1)

	// Delete removes the row and its sessions, and emits the revocation.
	dev := f.device(fieldidentity.ModeShared, nil, f.appA)
	sess := f.session(a.ID, dev.ID, f.appA)
	_, err = f.s.deleteWorker(f.actx, &apicommon.IDInput{ID: a.ID})
	require.NoError(t, err)
	_, err = f.s.getWorker(f.actx, &apicommon.IDInput{ID: a.ID})
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")
	var n int
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT COUNT(*) FROM field_sessions WHERE id = $1`, sess.ID).Scan(&n))
	assert.Zero(t, n)
	assert.Equal(t, 1, f.eventCount(fieldidentity.WorkerDeletedType, a.ID))
	assert.Equal(t, 1, f.eventCount(fieldidentity.SessionRevokedType, a.ID))
}

// ── enrolment codes ──────────────────────────────────────────────────────

func TestEnrolmentCode(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "W-ENROL", "Enrol Me", f.codeA).Body

	out, err := f.s.createCode(f.actx, &apicommon.In[CreateEnrolmentCodeRequest]{Body: CreateEnrolmentCodeRequest{
		ClientID: f.clientID, Mode: "PERSONAL", AppCodes: []string{f.codeA}, AssignedWorkerID: &w.ID,
	}})
	require.NoError(t, err)
	code := out.Body.Code
	assert.Len(t, code, fieldidentity.EnrolmentCodeLength)
	assert.NotContains(t, code, "0")
	assert.NotContains(t, code, "O")
	assert.Equal(t, "fcfield:1:https://fc.example.test:"+code, out.Body.QrPayload)
	assert.WithinDuration(t, time.Now().Add(fieldidentity.EnrolmentCodeTTL), out.Body.ExpiresAt.Underlying(), 30*time.Second)

	row, err := f.s.Ops.R.Codes.FindByHash(context.Background(), fieldidentity.HashToken(code))
	require.NoError(t, err)
	require.NotNil(t, row, "the code is stored under its SHA-256")
	assert.Equal(t, fieldidentity.ModePersonal, row.Mode)
	assert.Equal(t, []string{f.appA}, row.AppIDs)
	require.NotNil(t, row.AssignedIdentityID)
	assert.Equal(t, w.ID, *row.AssignedIdentityID)
	assert.Nil(t, row.UsedAt)
	var leaked int
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT (SELECT COUNT(*) FROM field_enrolment_codes WHERE code_hash = $1)
		      + (SELECT COUNT(*) FROM aud_logs WHERE operation_json::text LIKE '%' || $1 || '%')
		      + (SELECT COUNT(*) FROM msg_events WHERE data::text LIKE '%' || $1 || '%')`, code).Scan(&leaked))
	assert.Zero(t, leaked, "the plaintext code is nowhere in storage, events or audit")

	// Consuming is single-use.
	tx, err := testpg.Pool(t).Begin(context.Background())
	require.NoError(t, err)
	first, err := f.s.Ops.R.Codes.Consume(context.Background(), usecaseTx(tx), fieldidentity.HashToken(code))
	require.NoError(t, err)
	require.NotNil(t, first)
	second, err := f.s.Ops.R.Codes.Consume(context.Background(), usecaseTx(tx), fieldidentity.HashToken(code))
	require.NoError(t, err)
	assert.Nil(t, second)
	require.NoError(t, tx.Commit(context.Background()))

	// SHARED + assigned worker is rejected; no apps is rejected; bad mode too.
	_, err = f.s.createCode(f.actx, &apicommon.In[CreateEnrolmentCodeRequest]{Body: CreateEnrolmentCodeRequest{
		ClientID: f.clientID, Mode: "SHARED", AppCodes: []string{f.codeA}, AssignedWorkerID: &w.ID,
	}})
	requireErr(t, err, usecase.KindValidation, "SHARED_DEVICE_ASSIGNED")
	_, err = f.s.createCode(f.actx, &apicommon.In[CreateEnrolmentCodeRequest]{Body: CreateEnrolmentCodeRequest{
		ClientID: f.clientID, Mode: "SHARED",
	}})
	requireErr(t, err, usecase.KindValidation, "APP_CODES_REQUIRED")
	_, err = f.s.createCode(f.actx, &apicommon.In[CreateEnrolmentCodeRequest]{Body: CreateEnrolmentCodeRequest{
		ClientID: f.clientID, Mode: "KIOSK", AppCodes: []string{f.codeA},
	}})
	requireErr(t, err, usecase.KindValidation, "MODE_INVALID")
	// An expired code does not consume.
	_, err = testpg.Pool(t).Exec(context.Background(),
		`UPDATE field_enrolment_codes SET used_at = NULL, expires_at = NOW() - INTERVAL '1 second' WHERE code_hash = $1`,
		fieldidentity.HashToken(code))
	require.NoError(t, err)
	tx, _ = testpg.Pool(t).Begin(context.Background())
	expired, err := f.s.Ops.R.Codes.Consume(context.Background(), usecaseTx(tx), fieldidentity.HashToken(code))
	require.NoError(t, err)
	assert.Nil(t, expired)
	_ = tx.Rollback(context.Background())
}

// ── devices ──────────────────────────────────────────────────────────────

func TestDeviceSuspendActivateRevoke(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "W-DEV", "Dev User", f.codeA).Body
	dev := f.device(fieldidentity.ModePersonal, &w.ID, f.appA)
	sess := f.session(w.ID, dev.ID, f.appA)

	got, err := f.s.getDevice(f.actx, &apicommon.IDInput{ID: dev.ID})
	require.NoError(t, err)
	assert.Equal(t, 1, got.Body.ActiveSessions)
	assert.Equal(t, "ACTIVE", got.Body.Status)

	_, err = f.s.suspendDevice(f.actx, &apicommon.IDInput{ID: dev.ID})
	require.NoError(t, err)
	got, _ = f.s.getDevice(f.actx, &apicommon.IDInput{ID: dev.ID})
	assert.Equal(t, "SUSPENDED", got.Body.Status)
	assert.Zero(t, got.Body.ActiveSessions)
	revoked, reason := f.sessionState(sess.ID)
	assert.True(t, revoked)
	assert.Equal(t, fieldidentity.ReasonAdmin, reason)
	assert.Equal(t, 1, f.eventCount(fieldidentity.DeviceSuspendedType, dev.ID))

	_, err = f.s.activateDevice(f.actx, &apicommon.IDInput{ID: dev.ID})
	require.NoError(t, err)
	got, _ = f.s.getDevice(f.actx, &apicommon.IDInput{ID: dev.ID})
	assert.Equal(t, "ACTIVE", got.Body.Status, "suspension is resumable")

	sess2 := f.session(w.ID, dev.ID, f.appA)
	_, err = f.s.revokeDevice(f.actx, &apicommon.IDInput{ID: dev.ID})
	require.NoError(t, err)
	got, _ = f.s.getDevice(f.actx, &apicommon.IDInput{ID: dev.ID})
	assert.Equal(t, "REVOKED", got.Body.Status)
	revoked, reason = f.sessionState(sess2.ID)
	assert.True(t, revoked)
	assert.Equal(t, fieldidentity.ReasonDeviceRevoked, reason)
	assert.Equal(t, 1, f.eventCount(fieldidentity.DeviceRevokedType, dev.ID))

	_, err = f.s.activateDevice(f.actx, &apicommon.IDInput{ID: dev.ID})
	requireErr(t, err, usecase.KindConflict, "DEVICE_REVOKED")
	_, err = f.s.suspendDevice(f.actx, &apicommon.IDInput{ID: dev.ID})
	requireErr(t, err, usecase.KindConflict, "DEVICE_REVOKED")
	_, err = f.s.revokeDevice(f.actx, &apicommon.IDInput{ID: dev.ID})
	require.NoError(t, err, "revoking twice is a no-op")
	assert.Equal(t, 1, f.eventCount(fieldidentity.DeviceRevokedType, dev.ID))

	list, err := f.s.listDevices(f.actx, &listDevicesInput{ClientID: f.clientID, Status: "REVOKED"})
	require.NoError(t, err)
	assert.EqualValues(t, 1, list.Body.Total)
	list, _ = f.s.listDevices(f.actx, &listDevicesInput{ClientID: f.clientID, Mode: "SHARED"})
	assert.Zero(t, list.Body.Total)
}

// ── policy ───────────────────────────────────────────────────────────────

func TestPolicyGetPut(t *testing.T) {
	f := newFixture(t, hasher(t))
	def, err := f.s.getPolicy(f.actx, &policyInput{ClientID: f.clientID})
	require.NoError(t, err)
	assert.True(t, def.Body.IsDefault)
	assert.Equal(t, 6, def.Body.PinMinLength)
	assert.Equal(t, 5, def.Body.SoftLockThreshold)
	assert.Equal(t, 900, def.Body.SoftLockSecs)
	assert.Equal(t, 10, def.Body.HardLockThreshold)
	assert.Equal(t, 900, def.Body.SharedIdleSecs)
	assert.Equal(t, 7200, def.Body.PersonalIdleSecs)
	assert.Equal(t, 50400, def.Body.ShiftMaxSecs)
	assert.False(t, def.Body.RequireAttestation)
	assert.Equal(t, 7, def.Body.PinReauthDays)

	four := 4
	put, err := f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{
		PinMinLength: &four, RequireAttestation: true,
	}})
	require.NoError(t, err)
	assert.False(t, put.Body.IsDefault)
	assert.Equal(t, 4, put.Body.PinMinLength)
	assert.Equal(t, 5, put.Body.HardLockThreshold, "a 4-digit minimum defaults the hard lock to 5")
	assert.True(t, put.Body.RequireAttestation)
	assert.Equal(t, 1, f.eventCount(fieldidentity.PolicyUpdatedType, f.clientID))

	// The policy now governs PIN checks: 4 digits accepted.
	pin4 := "4829"
	_, err = f.s.ensureWorker(f.actx, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: f.clientID, WorkerCode: "W-FOUR", Name: "N", Pin: &pin4,
	}})
	require.NoError(t, err)

	// Range validation.
	three, hard := 3, 9
	_, err = f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{PinMinLength: &three}})
	requireErr(t, err, usecase.KindValidation, "POLICY_INVALID")
	_, err = f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{PinMinLength: &four, HardLockThreshold: &hard}})
	requireErr(t, err, usecase.KindValidation, "POLICY_INVALID")
	soft := 8
	_, err = f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{SoftLockThreshold: &soft, HardLockThreshold: &three}})
	requireErr(t, err, usecase.KindValidation, "POLICY_INVALID")
}

// ── authorization ────────────────────────────────────────────────────────

var allFieldPerms = []string{
	"platform:iam:field-worker:view", "platform:iam:field-worker:manage",
	"platform:iam:field-device:view", "platform:iam:field-device:manage",
	"platform:iam:field-token:introspect",
}

func TestClientBoxing(t *testing.T) {
	f := newFixture(t, hasher(t))
	other := newFixture(t, hasher(t)) // a second client
	mine := f.ensure(f.actx, "W-BOX", "Mine", f.codeA).Body
	theirs := other.ensure(other.actx, "W-THEIRS", "Theirs", other.codeA).Body
	theirDev := other.device(fieldidentity.ModeShared, nil, other.appA)

	// A client administrator of f's client only.
	cctx := callerCtx([]string{f.clientID}, allFieldPerms, nil, true)

	_, err := f.s.getWorker(cctx, &apicommon.IDInput{ID: mine.ID})
	require.NoError(t, err)

	// Another client's resources are invisible: 404, not 403.
	_, err = f.s.getWorker(cctx, &apicommon.IDInput{ID: theirs.ID})
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")
	_, err = f.s.suspendWorker(cctx, &apicommon.IDInput{ID: theirs.ID})
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")
	_, err = f.s.resetPin(cctx, &resetPinInput{ID: theirs.ID})
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")
	_, err = f.s.getDevice(cctx, &apicommon.IDInput{ID: theirDev.ID})
	requireErr(t, err, usecase.KindNotFound, "FieldDevice_NOT_FOUND")
	_, err = f.s.revokeDevice(cctx, &apicommon.IDInput{ID: theirDev.ID})
	requireErr(t, err, usecase.KindNotFound, "FieldDevice_NOT_FOUND")
	// An unknown id is indistinguishable.
	_, err = f.s.getWorker(cctx, &apicommon.IDInput{ID: "fwu_doesnotexist"})
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")

	// Naming another client explicitly is a 403.
	_, err = f.s.listWorkers(cctx, &listWorkersInput{ClientID: other.clientID})
	requireErr(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	_, err = other.s.ensureWorker(cctx, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: other.clientID, WorkerCode: "W-X", Name: "N",
	}})
	requireErr(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	_, err = f.s.createCode(cctx, &apicommon.In[CreateEnrolmentCodeRequest]{Body: CreateEnrolmentCodeRequest{
		ClientID: other.clientID, Mode: "SHARED", AppCodes: []string{other.codeA},
	}})
	requireErr(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	_, err = f.s.getPolicy(cctx, &policyInput{ClientID: other.clientID})
	requireErr(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	_, err = f.s.putPolicy(cctx, &putPolicyInput{ClientID: other.clientID})
	requireErr(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

	// The use cases box on their own too (defence in depth behind the
	// controller): invoking one directly for another client's worker fails.
	_, err = usecaseop.Run(cctx, f.s.UoW, f.s.Ops.UnlockWorker(), fieldidentity.WorkerRef{ID: theirs.ID}, testpg.TestEC())
	requireErr(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

	// No permission at all: 403, with or without a valid id.
	none := callerCtx([]string{f.clientID}, nil, nil, true)
	_, err = f.s.getWorker(none, &apicommon.IDInput{ID: mine.ID})
	requireErr(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	_, err = f.s.getWorker(none, &apicommon.IDInput{ID: "fwu_doesnotexist"})
	requireErr(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	// View-only cannot manage.
	view := callerCtx([]string{f.clientID}, []string{"platform:iam:field-worker:view"}, nil, true)
	_, err = f.s.getWorker(view, &apicommon.IDInput{ID: mine.ID})
	require.NoError(t, err)
	_, err = f.s.suspendWorker(view, &apicommon.IDInput{ID: mine.ID})
	requireErr(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
}

func TestApplicationConfinedServiceAccount(t *testing.T) {
	f := newFixture(t, hasher(t))
	// The application's own service account: manage + introspect, one app.
	sa := callerCtx([]string{f.clientID},
		[]string{"platform:iam:field-worker:manage", "platform:iam:field-token:introspect"},
		[]string{f.appA}, false)

	// It may ensure workers for its own application...
	out, err := f.s.ensureWorker(sa, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: f.clientID, WorkerCode: "W-SA", Name: "SA Made", AppCodes: []string{f.codeA},
	}})
	require.NoError(t, err)
	// ...but not name another application, nor omit its own.
	_, err = f.s.ensureWorker(sa, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: f.clientID, WorkerCode: "W-SA2", Name: "N", AppCodes: []string{f.codeB},
	}})
	requireErr(t, err, usecase.KindAuthorization, "APPLICATION_FORBIDDEN")
	_, err = f.s.ensureWorker(sa, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: f.clientID, WorkerCode: "W-SA3", Name: "N",
	}})
	requireErr(t, err, usecase.KindAuthorization, "APPLICATION_FORBIDDEN")
	_, err = f.s.ensureWorker(sa, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: f.clientID, WorkerCode: "W-SA4", Name: "N", AppCodes: []string{f.codeA, f.codeB},
	}})
	requireErr(t, err, usecase.KindAuthorization, "APPLICATION_FORBIDDEN")

	// It can act on its own application's workers.
	_, err = f.s.resetPin(sa, &resetPinInput{ID: out.Body.ID})
	require.NoError(t, err)
	_, err = f.s.grantApp(sa, &grantAppInput{ID: out.Body.ID, Body: GrantFieldWorkerAppRequest{AppCode: f.codeB}})
	requireErr(t, err, usecase.KindAuthorization, "APPLICATION_FORBIDDEN")

	// A worker of another application is invisible to it.
	foreign := f.ensure(f.actx, "W-FOREIGN", "Foreign", f.codeB).Body
	_, err = f.s.getWorker(sa, &apicommon.IDInput{ID: foreign.ID})
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")
	_, err = f.s.suspendWorker(sa, &apicommon.IDInput{ID: foreign.ID})
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")
	// Called past the controller, the use case refuses too.
	_, err = usecaseop.RunTx(sa, f.s.UoW, f.s.Ops.SuspendWorker(), fieldidentity.WorkerRef{ID: foreign.ID}, testpg.TestEC())
	requireErr(t, err, usecase.KindAuthorization, "APPLICATION_FORBIDDEN")
	// It can add its own application to a worker it cannot yet see.
	_, err = f.s.grantApp(sa, &grantAppInput{ID: foreign.ID, Body: GrantFieldWorkerAppRequest{AppCode: f.codeA}})
	require.NoError(t, err)
	_, err = f.s.getWorker(sa, &apicommon.IDInput{ID: foreign.ID})
	require.NoError(t, err, "now it shares an application with the worker")

	// The list shows only its own application's workers.
	f.ensure(f.actx, "W-ONLYB", "Only B", f.codeB)
	list, err := f.s.listWorkers(sa, &listWorkersInput{ClientID: f.clientID})
	require.NoError(t, err)
	for _, w := range list.Body.Workers {
		assert.NotEqual(t, "W-ONLYB", w.WorkerCode)
	}
	_, err = f.s.listWorkers(sa, &listWorkersInput{ClientID: f.clientID, AppCode: f.codeB})
	requireErr(t, err, usecase.KindAuthorization, "APPLICATION_FORBIDDEN")

	// Enrolment codes: only for its own application. Policy: never.
	suspender := callerCtx([]string{f.clientID}, allFieldPerms, []string{f.appA}, false)
	_, err = f.s.createCode(suspender, &apicommon.In[CreateEnrolmentCodeRequest]{Body: CreateEnrolmentCodeRequest{
		ClientID: f.clientID, Mode: "SHARED", AppCodes: []string{f.codeB},
	}})
	requireErr(t, err, usecase.KindAuthorization, "APPLICATION_FORBIDDEN")
	_, err = f.s.createCode(suspender, &apicommon.In[CreateEnrolmentCodeRequest]{Body: CreateEnrolmentCodeRequest{
		ClientID: f.clientID, Mode: "SHARED", AppCodes: []string{f.codeA},
	}})
	require.NoError(t, err)
	_, err = f.s.putPolicy(suspender, &putPolicyInput{ClientID: f.clientID})
	requireErr(t, err, usecase.KindAuthorization, "APPLICATION_FORBIDDEN")
	// Devices of other applications are invisible.
	devB := f.device(fieldidentity.ModeShared, nil, f.appB)
	_, err = f.s.getDevice(suspender, &apicommon.IDInput{ID: devB.ID})
	requireErr(t, err, usecase.KindNotFound, "FieldDevice_NOT_FOUND")
	devs, err := f.s.listDevices(suspender, &listDevicesInput{ClientID: f.clientID})
	require.NoError(t, err)
	for _, d := range devs.Body.Devices {
		assert.NotEqual(t, devB.ID, d.ID)
	}
}

// ── introspection ────────────────────────────────────────────────────────

type fakeVerifier map[string]*fieldidentity.TokenClaims

func (v fakeVerifier) VerifyFieldAccessToken(_ context.Context, token string) (*fieldidentity.TokenClaims, error) {
	c, ok := v[token]
	if !ok {
		return nil, fmt.Errorf("bad token")
	}
	return c, nil
}

func TestIntrospect(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "W-INTRO", "Intro", f.codeA).Body
	dev := f.device(fieldidentity.ModePersonal, &w.ID, f.appA)
	sess := f.session(w.ID, dev.ID, f.appA)
	claims := &fieldidentity.TokenClaims{
		Subject: w.ID, ClientID: f.clientID, WorkerCode: "W-INTRO", DeviceID: dev.ID, AppCode: f.codeA,
		SessionID: sess.FamilyID, AMR: []string{"pin"}, AuthTime: time.Now().Add(-time.Minute), Expires: time.Now().Add(5 * time.Minute),
	}
	expired := *claims
	expired.Expires = time.Now().Add(-time.Second)
	f.s.Verifier = fakeVerifier{"good": claims, "expired": &expired}
	call := func(ctx context.Context, tok string) FieldTokenIntrospectResponse {
		out, err := f.s.introspect(ctx, &apicommon.In[FieldTokenIntrospectRequest]{Body: FieldTokenIntrospectRequest{Token: tok}})
		require.NoError(t, err)
		return out.Body
	}

	r := call(f.actx, "good")
	assert.True(t, r.Active)
	assert.Equal(t, w.ID, r.Sub)
	assert.Equal(t, "W-INTRO", r.WorkerCode)
	assert.Equal(t, f.codeA, r.AppCode)
	assert.Equal(t, dev.ID, r.DeviceID)
	assert.Equal(t, []string{"pin"}, r.Amr)

	assert.False(t, call(f.actx, "garbage").Active, "a bad token is inactive, not an error")
	assert.False(t, call(f.actx, "expired").Active)
	// Outside the caller's client box: inactive.
	otherCaller := callerCtx([]string{"clt_someoneelse01"}, allFieldPerms, nil, true)
	assert.False(t, call(otherCaller, "good").Active)
	// No permission: 403.
	_, err := f.s.introspect(callerCtx([]string{f.clientID}, nil, nil, true), &apicommon.In[FieldTokenIntrospectRequest]{Body: FieldTokenIntrospectRequest{Token: "good"}})
	requireErr(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")

	// Revoking the session, suspending the worker, or the device each flip it.
	_, err = f.s.suspendWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	assert.False(t, call(f.actx, "good").Active)
	_, err = f.s.activateWorker(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	assert.False(t, call(f.actx, "good").Active, "the revoked session stays revoked after reactivation")
	sess2 := f.session(w.ID, dev.ID, f.appA)
	claims.SessionID = sess2.FamilyID
	assert.True(t, call(f.actx, "good").Active)
	_, err = f.s.suspendDevice(f.actx, &apicommon.IDInput{ID: dev.ID})
	require.NoError(t, err)
	assert.False(t, call(f.actx, "good").Active)

	// No verifier wired (slice B): everything is inactive.
	f.s.Verifier = nil
	assert.False(t, call(f.actx, "good").Active)
}

// ── sign-ins and the session/nonce store ─────────────────────────────────

func TestSignInsAndSessionStore(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "W-SIGN", "Sign", f.codeA).Body
	dev := f.device(fieldidentity.ModeShared, nil, f.appA)
	repo := f.s.Ops.R.Sessions
	ctx := context.Background()

	base := time.Now().UTC().Add(-time.Hour)
	for n, fail := range []string{"INVALID_CREDENTIALS", "", "WORKER_LOCKED"} {
		a := fieldidentity.LoginAttempt{
			ClientID: f.clientID, IdentityID: w.ID, WorkerCode: "W-SIGN", DeviceID: dev.ID, ApplicationID: f.appA,
			Outcome: fieldidentity.OutcomeFailure, FailureReason: fail, AttemptedAt: base.Add(time.Duration(n) * time.Minute),
		}
		if fail == "" {
			a.Outcome = fieldidentity.OutcomeSuccess
		}
		require.NoError(t, repo.RecordLoginAttempt(ctx, a))
	}
	out, err := f.s.signIns(f.actx, &signInsInput{ID: w.ID, PageSize: 2})
	require.NoError(t, err)
	assert.EqualValues(t, 3, out.Body.Total)
	require.Len(t, out.Body.SignIns, 2)
	assert.Equal(t, "WORKER_LOCKED", out.Body.SignIns[0].FailureReason, "newest first")
	assert.Equal(t, f.codeA, out.Body.SignIns[0].AppCode)
	assert.Equal(t, dev.ID, out.Body.SignIns[0].DeviceID)

	// Nonces are single-use and expire.
	require.NoError(t, repo.StoreNonce(ctx, "nonce-1", time.Now().Add(time.Minute)))
	ok, err := repo.ConsumeNonce(ctx, "nonce-1")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, _ = repo.ConsumeNonce(ctx, "nonce-1")
	assert.False(t, ok, "a nonce is single-use")
	require.NoError(t, repo.StoreNonce(ctx, "nonce-old", time.Now().Add(-time.Second)))
	ok, _ = repo.ConsumeNonce(ctx, "nonce-old")
	assert.False(t, ok, "an expired nonce is refused")

	// Rotation marks the old row and a lookup by hash finds it.
	s := f.session(w.ID, dev.ID, f.appA)
	won, err := repo.MarkRotated(ctx, s.ID)
	require.NoError(t, err)
	assert.True(t, won)
	won, _ = repo.MarkRotated(ctx, s.ID)
	assert.False(t, won, "only one rotation wins")
	got, err := repo.FindByRefreshHash(ctx, s.RefreshHash)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, fieldidentity.ReasonRotated, got.RevokedReason)
	assert.NotNil(t, got.RotatedAt)

	// Purge removes the expired nonce and long-dead sessions.
	_, err = testpg.Pool(t).Exec(ctx, `UPDATE field_sessions SET revoked_at = NOW() - INTERVAL '31 days' WHERE id = $1`, s.ID)
	require.NoError(t, err)
	require.NoError(t, repo.StoreNonce(ctx, "nonce-purge", time.Now().Add(-time.Minute)))
	_, _, sessions, err := repo.Purge(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, sessions, int64(1))
	gone, _ := repo.FindByRefreshHash(ctx, s.RefreshHash)
	assert.Nil(t, gone)
	assert.True(t, strings.HasPrefix(s.ID, "fws_"))
}
