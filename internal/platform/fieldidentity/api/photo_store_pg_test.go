//go:build integration

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/imageproc"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// ── fixtures ─────────────────────────────────────────────────────────────

// photoOf makes a processed photo of one colour (size varies the picture so
// two photos get two versions).
func photoOf(t testing.TB, c color.RGBA, size int) *imageproc.Photo {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	var b bytes.Buffer
	require.NoError(t, jpeg.Encode(&b, img, nil))
	p, err := imageproc.Process(b.Bytes(), "image/jpeg", imageproc.DefaultLimits())
	require.NoError(t, err)
	return p
}

var (
	pRed  = color.RGBA{200, 20, 20, 255}
	pBlue = color.RGBA{20, 20, 200, 255}
)

func (f *fixture) setPhoto(ctx context.Context, workerID string, actor fieldidentity.PhotoActor, p *imageproc.Photo) error {
	f.t.Helper()
	cmd := fieldidentity.NewSetPhotoCommand(fieldidentity.WorkerRef{ID: workerID}, actor, "prn_optestrunner1", p)
	_, err := usecaseop.RunTx(ctx, f.s.UoW, f.s.Ops.SetWorkerPhoto(), cmd, testpg.TestEC())
	return err
}

func (f *fixture) removePhoto(ctx context.Context, workerID string, actor fieldidentity.PhotoActor) error {
	f.t.Helper()
	cmd := fieldidentity.RemovePhotoCommand{ID: workerID, Actor: actor, ActorID: "prn_optestrunner1"}
	_, err := usecaseop.RunTx(ctx, f.s.UoW, f.s.Ops.RemoveWorkerPhoto(), cmd, testpg.TestEC())
	return err
}

func (f *fixture) putPolicy(cmd fieldidentity.PutPolicyCommand) error {
	f.t.Helper()
	cmd.ClientID = f.clientID
	_, err := usecaseop.RunTx(f.actx, f.s.UoW, f.s.Ops.PutPolicy(), cmd, testpg.TestEC())
	return err
}

func (f *fixture) policy() fieldidentity.Policy {
	f.t.Helper()
	p, err := f.s.Ops.R.Policies.Effective(context.Background(), f.clientID)
	require.NoError(f.t, err)
	return p
}

func (f *fixture) photoRows(clientID string) int {
	f.t.Helper()
	n, err := f.s.Ops.R.Photos.CountForClient(context.Background(), clientID)
	require.NoError(f.t, err)
	return n
}

func (f *fixture) worker(id string) *fieldidentity.Identity {
	f.t.Helper()
	i, err := f.s.Ops.R.Identities.FindByID(context.Background(), id)
	require.NoError(f.t, err)
	require.NotNil(f.t, i)
	return i
}

// ── set, replace, remove ─────────────────────────────────────────────────

func TestSetReplaceRemovePhoto(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "ph-001", "Ada Driver", f.codeA).Body.ID
	require.Empty(t, f.worker(w).PhotoVersion, "no photo to begin with")

	red := photoOf(t, pRed, 64)
	require.NoError(t, f.setPhoto(f.actx, w, fieldidentity.PhotoByAdmin, red))
	got := f.worker(w)
	assert.Equal(t, red.Version, got.PhotoVersion)
	require.NotNil(t, got.PhotoUpdatedAt)
	assert.Equal(t, 1, f.photoRows(f.clientID))
	assert.Equal(t, 1, f.eventCount(fieldidentity.WorkerPhotoSetType, w))

	full, ok, err := f.s.Ops.R.Photos.Rendition(context.Background(), w, fieldidentity.PhotoFull)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, red.Full.JPEG, full.Bytes, "the stored bytes are the processed rendition")
	assert.Equal(t, "image/jpeg", full.ContentType)
	assert.Equal(t, red.Full.Width, full.Width)
	thumb, _, _ := f.s.Ops.R.Photos.Rendition(context.Background(), w, fieldidentity.PhotoThumb)
	assert.Equal(t, red.Thumb.JPEG, thumb.Bytes)

	// Replace: still one row, the new version, the old picture gone.
	blue := photoOf(t, pBlue, 90)
	require.NotEqual(t, red.Version, blue.Version)
	require.NoError(t, f.setPhoto(f.actx, w, fieldidentity.PhotoByAdmin, blue))
	assert.Equal(t, 1, f.photoRows(f.clientID))
	assert.Equal(t, blue.Version, f.worker(w).PhotoVersion)
	full, _, _ = f.s.Ops.R.Photos.Rendition(context.Background(), w, fieldidentity.PhotoFull)
	assert.Equal(t, blue.Full.JPEG, full.Bytes)
	assert.Equal(t, 2, f.eventCount(fieldidentity.WorkerPhotoSetType, w))

	// Remove.
	require.NoError(t, f.removePhoto(f.actx, w, fieldidentity.PhotoByAdmin))
	assert.Empty(t, f.worker(w).PhotoVersion)
	assert.Nil(t, f.worker(w).PhotoUpdatedAt)
	assert.Zero(t, f.photoRows(f.clientID))
	_, ok, err = f.s.Ops.R.Photos.Rendition(context.Background(), w, fieldidentity.PhotoThumb)
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, 1, f.eventCount(fieldidentity.WorkerPhotoRemovedType, w))

	// Removing again is a no-op: no error, no second event.
	require.NoError(t, f.removePhoto(f.actx, w, fieldidentity.PhotoByAdmin))
	assert.Equal(t, 1, f.eventCount(fieldidentity.WorkerPhotoRemovedType, w))
}

func TestSetPhotoValidation(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "ph-002", "Bo", f.codeA).Body.ID
	p := photoOf(t, pRed, 32)
	for name, cmd := range map[string]fieldidentity.SetPhotoCommand{
		"no worker id":   fieldidentity.NewSetPhotoCommand(fieldidentity.WorkerRef{}, fieldidentity.PhotoByAdmin, "x", p),
		"unknown actor":  fieldidentity.NewSetPhotoCommand(fieldidentity.WorkerRef{ID: w}, fieldidentity.PhotoActor("ROBOT"), "x", p),
		"no image":       {WorkerRef: fieldidentity.WorkerRef{ID: w}, Actor: fieldidentity.PhotoByAdmin},
		"empty renditon": {WorkerRef: fieldidentity.WorkerRef{ID: w}, Actor: fieldidentity.PhotoByAdmin, Image: &imageproc.Photo{}},
	} {
		_, err := usecaseop.RunTx(f.actx, f.s.UoW, f.s.Ops.SetWorkerPhoto(), cmd, testpg.TestEC())
		ue := usecase.AsError(err)
		require.NotNil(t, ue, name)
		assert.Equal(t, usecase.KindValidation, ue.Kind, name)
	}
	assert.Zero(t, f.photoRows(f.clientID))

	// An unknown worker is not found.
	err := f.setPhoto(f.actx, "fwu_doesnotexist", fieldidentity.PhotoByAdmin, p)
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")
}

func TestPhotoActorOf(t *testing.T) {
	assert.Equal(t, fieldidentity.PhotoByServiceAccount, fieldidentity.PhotoActorOf(&auth.AuthContext{PrincipalType: auth.PrincipalTypeService}))
	assert.Equal(t, fieldidentity.PhotoByAdmin, fieldidentity.PhotoActorOf(&auth.AuthContext{PrincipalType: auth.PrincipalTypeUser}))
	assert.Equal(t, fieldidentity.PhotoByAdmin, fieldidentity.PhotoActorOf(nil))
}

// Every actor is stored and read back, and the database's CHECK holds the same
// members as the constants.
func TestPhotoActorsRoundTrip(t *testing.T) {
	f := newFixture(t, hasher(t))
	require.NoError(t, f.putPolicy(fieldidentity.PutPolicyCommand{WorkerMaySetOwnPhoto: true}))
	w := f.ensure(f.actx, "ph-003", "Cy", f.codeA).Body.ID
	for _, a := range []fieldidentity.PhotoActor{fieldidentity.PhotoByAdmin, fieldidentity.PhotoByServiceAccount, fieldidentity.PhotoByWorker} {
		require.True(t, a.Valid())
		require.NoError(t, f.setPhoto(f.actx, w, a, photoOf(t, pRed, 40)))
		var got string
		require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
			`SELECT set_by_type FROM field_worker_photos WHERE identity_id = $1`, w).Scan(&got))
		assert.Equal(t, string(a), got)
	}
	_, err := testpg.Pool(t).Exec(context.Background(),
		`UPDATE field_worker_photos SET set_by_type = 'ROBOT' WHERE identity_id = $1`, w)
	assert.Error(t, err, "the CHECK refuses a member the constants do not have")
	assert.False(t, fieldidentity.PhotoActor("ROBOT").Valid())
	for _, s := range []fieldidentity.PhotoSize{fieldidentity.PhotoThumb, fieldidentity.PhotoFull} {
		assert.True(t, s.Valid())
	}
	assert.False(t, fieldidentity.PhotoSize("huge").Valid())
}

// ── the client's two settings ────────────────────────────────────────────

func TestOwnPhotoNeedsThePolicy(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "ph-004", "Di", f.codeA).Body.ID
	p := photoOf(t, pRed, 40)

	// Default: a worker may not set or remove their own photo.
	assert.False(t, f.policy().WorkerMaySetOwnPhoto)
	err := f.setPhoto(f.actx, w, fieldidentity.PhotoByWorker, p)
	requireErr(t, err, usecase.KindAuthorization, fieldidentity.CodeOwnPhotoNotAllowed)
	err = f.removePhoto(f.actx, w, fieldidentity.PhotoByWorker)
	requireErr(t, err, usecase.KindAuthorization, fieldidentity.CodeOwnPhotoNotAllowed)
	assert.Zero(t, f.photoRows(f.clientID))

	// An administrator is not affected by it.
	require.NoError(t, f.setPhoto(f.actx, w, fieldidentity.PhotoByAdmin, p))

	// Opted in: the worker may set, replace and remove.
	require.NoError(t, f.putPolicy(fieldidentity.PutPolicyCommand{WorkerMaySetOwnPhoto: true}))
	require.NoError(t, f.setPhoto(f.actx, w, fieldidentity.PhotoByWorker, photoOf(t, pBlue, 50)))
	require.NoError(t, f.removePhoto(f.actx, w, fieldidentity.PhotoByWorker))
	assert.Zero(t, f.photoRows(f.clientID))

	// Switching the opt-in off again leaves stored photos alone.
	require.NoError(t, f.setPhoto(f.actx, w, fieldidentity.PhotoByWorker, p))
	require.NoError(t, f.putPolicy(fieldidentity.PutPolicyCommand{}))
	assert.Equal(t, 1, f.photoRows(f.clientID))
	assert.Equal(t, p.Version, f.worker(w).PhotoVersion)
}

func TestPhotosOffForTheClientRefusesASet(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "ph-005", "Ed", f.codeA).Body.ID
	require.NoError(t, f.putPolicy(fieldidentity.PutPolicyCommand{PhotosEnabled: new(false)}))
	for _, actor := range []fieldidentity.PhotoActor{fieldidentity.PhotoByAdmin, fieldidentity.PhotoByServiceAccount, fieldidentity.PhotoByWorker} {
		err := f.setPhoto(f.actx, w, actor, photoOf(t, pRed, 40))
		requireErr(t, err, usecase.KindConflict, fieldidentity.CodePhotosDisabledForClient)
	}
	assert.Zero(t, f.photoRows(f.clientID))
	assert.Empty(t, f.worker(w).PhotoVersion)
	// Removing is harmless.
	require.NoError(t, f.removePhoto(f.actx, w, fieldidentity.PhotoByAdmin))
}

// The policy PUT replaces the policy: a value left out goes back to its
// default, and both new fields have a default the opposite of the other's.
func TestPolicyPutOmittedPhotoFieldsTakeTheirDefaults(t *testing.T) {
	f := newFixture(t, hasher(t))
	def := f.policy()
	assert.True(t, def.PhotosEnabled, "a client with no policy row has photos on")
	assert.False(t, def.WorkerMaySetOwnPhoto)
	assert.True(t, def.IsDefault)

	// Both set away from their defaults.
	require.NoError(t, f.putPolicy(fieldidentity.PutPolicyCommand{PhotosEnabled: new(false), WorkerMaySetOwnPhoto: true}))
	got := f.policy()
	assert.False(t, got.PhotosEnabled)
	assert.True(t, got.WorkerMaySetOwnPhoto)
	assert.False(t, got.IsDefault)

	// A PUT that says nothing about them: photosEnabled is true again and
	// workerMaySetOwnPhoto false again.
	require.NoError(t, f.putPolicy(fieldidentity.PutPolicyCommand{}))
	got = f.policy()
	assert.True(t, got.PhotosEnabled, "omitted photosEnabled is true")
	assert.False(t, got.WorkerMaySetOwnPhoto, "omitted workerMaySetOwnPhoto is false")

	// Explicit values are kept.
	require.NoError(t, f.putPolicy(fieldidentity.PutPolicyCommand{PhotosEnabled: new(true), WorkerMaySetOwnPhoto: true}))
	got = f.policy()
	assert.True(t, got.PhotosEnabled)
	assert.True(t, got.WorkerMaySetOwnPhoto)

	// The Build rule on its own, with no database.
	p, err := fieldidentity.PutPolicyCommand{ClientID: "clt_x"}.Build()
	require.NoError(t, err)
	assert.True(t, p.PhotosEnabled)
	assert.False(t, p.WorkerMaySetOwnPhoto)
}

func TestSwitchingPhotosOffDeletesThem(t *testing.T) {
	f := newFixture(t, hasher(t))
	other := newFixture(t, hasher(t)) // another client, to prove its photos stay
	w1 := f.ensure(f.actx, "ph-006", "Fa", f.codeA).Body.ID
	w2 := f.ensure(f.actx, "ph-007", "Gi", f.codeA).Body.ID
	w3 := f.ensure(f.actx, "ph-008", "Hu", f.codeA).Body.ID // no photo
	ow := other.ensure(other.actx, "ph-006", "Fa", other.codeA).Body.ID
	require.NoError(t, f.setPhoto(f.actx, w1, fieldidentity.PhotoByAdmin, photoOf(t, pRed, 40)))
	require.NoError(t, f.setPhoto(f.actx, w2, fieldidentity.PhotoByAdmin, photoOf(t, pBlue, 41)))
	require.NoError(t, other.setPhoto(other.actx, ow, fieldidentity.PhotoByAdmin, photoOf(t, pRed, 42)))
	policyEvents := func() int { return f.eventCount(fieldidentity.PolicyUpdatedType, f.clientID) }
	before := policyEvents()

	// Unconfirmed: refused with the count; nothing changes.
	err := f.putPolicy(fieldidentity.PutPolicyCommand{PhotosEnabled: new(false)})
	requireErr(t, err, usecase.KindConflict, fieldidentity.CodePhotoDeletionNotConfirmed)
	ue := usecase.AsError(err)
	require.NotNil(t, ue)
	assert.EqualValues(t, 2, ue.Details["count"])
	assert.Equal(t, 2, f.photoRows(f.clientID))
	assert.True(t, f.policy().PhotosEnabled, "the refused change was not saved")
	assert.Equal(t, before, policyEvents())

	// Confirmed: photos, versions and nothing else go, with one policy event
	// carrying the count and no per-worker event.
	removedBefore := f.eventCount(fieldidentity.WorkerPhotoRemovedType, "")
	require.NoError(t, f.putPolicy(fieldidentity.PutPolicyCommand{PhotosEnabled: new(false), ConfirmPhotoDeletion: true}))
	assert.False(t, f.policy().PhotosEnabled)
	assert.Zero(t, f.photoRows(f.clientID))
	for _, id := range []string{w1, w2, w3} {
		assert.Empty(t, f.worker(id).PhotoVersion)
		assert.Nil(t, f.worker(id).PhotoUpdatedAt)
	}
	assert.Equal(t, before+1, policyEvents(), "one policy event")
	var removed float64
	var data []byte
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT data::text::bytea FROM msg_events WHERE type = $1 AND subject LIKE '%' || $2 ORDER BY created_at DESC LIMIT 1`,
		fieldidentity.PolicyUpdatedType, f.clientID).Scan(&data))
	var ev map[string]any
	require.NoError(t, json.Unmarshal(data, &ev))
	removed, _ = ev["photosRemoved"].(float64)
	assert.EqualValues(t, 2, removed)
	assert.Equal(t, false, ev["photosEnabled"])
	assert.Equal(t, removedBefore, f.eventCount(fieldidentity.WorkerPhotoRemovedType, ""), "no per-worker events")
	for _, id := range []string{w1, w2} {
		assert.Zero(t, f.eventCount(fieldidentity.WorkerPhotoRemovedType, id))
	}

	// The other client's photo is untouched.
	assert.Equal(t, 1, other.photoRows(other.clientID))
	assert.NotEmpty(t, other.worker(ow).PhotoVersion)

	// Already off, nothing held: a PUT that keeps it off needs no confirmation;
	// and switching on again does not bring the photos back.
	require.NoError(t, f.putPolicy(fieldidentity.PutPolicyCommand{PhotosEnabled: new(false)}))
	require.NoError(t, f.putPolicy(fieldidentity.PutPolicyCommand{}))
	assert.True(t, f.policy().PhotosEnabled)
	assert.Zero(t, f.photoRows(f.clientID))
}

func TestSwitchingOffWithNoPhotosNeedsNoConfirmation(t *testing.T) {
	f := newFixture(t, hasher(t))
	f.ensure(f.actx, "ph-009", "Jo", f.codeA)
	require.NoError(t, f.putPolicy(fieldidentity.PutPolicyCommand{PhotosEnabled: new(false)}))
	assert.False(t, f.policy().PhotosEnabled)
}

// ── deleting a worker ────────────────────────────────────────────────────

func TestDeletingAWorkerDeletesItsPhoto(t *testing.T) {
	f := newFixture(t, hasher(t))
	gone := f.ensure(f.actx, "ph-010", "Ki", f.codeA).Body.ID
	kept := f.ensure(f.actx, "ph-011", "Lo", f.codeA).Body.ID
	require.NoError(t, f.setPhoto(f.actx, gone, fieldidentity.PhotoByAdmin, photoOf(t, pRed, 40)))
	require.NoError(t, f.setPhoto(f.actx, kept, fieldidentity.PhotoByAdmin, photoOf(t, pBlue, 41)))
	_, err := f.s.deleteWorker(f.actx, &apicommon.IDInput{ID: gone})
	require.NoError(t, err)
	var n int
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT COUNT(*) FROM field_worker_photos WHERE identity_id = $1`, gone).Scan(&n))
	assert.Zero(t, n)
	assert.Equal(t, 1, f.photoRows(f.clientID), "the other worker's photo stays")
}

// ── no image data in an event or an audit row ───────────────────────────

func TestNoImageBytesInAuditOrEvents(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "ph-012", "Mo", f.codeA).Body.ID
	p := photoOf(t, pRed, 64)
	require.NoError(t, f.setPhoto(f.actx, w, fieldidentity.PhotoByAdmin, p))
	require.NoError(t, f.removePhoto(f.actx, w, fieldidentity.PhotoByAdmin))

	// The command's own JSON.
	cmd, err := json.Marshal(fieldidentity.NewSetPhotoCommand(fieldidentity.WorkerRef{ID: w}, fieldidentity.PhotoByAdmin, "prn_x", p))
	require.NoError(t, err)
	assert.NotContains(t, string(cmd), "Image")
	assert.NotContains(t, string(cmd), "/9j/", "no base64 JPEG")
	assert.Contains(t, string(cmd), p.Version)

	pool := testpg.Pool(t)
	ctx := context.Background()
	rows, err := pool.Query(ctx, `SELECT operation, operation_json::text FROM aud_logs WHERE entity_id = $1`, w)
	require.NoError(t, err)
	defer rows.Close()
	ops := map[string]bool{}
	for rows.Next() {
		var op, js string
		require.NoError(t, rows.Scan(&op, &js))
		ops[op] = true
		assert.NotContains(t, js, "/9j/", "audit row of %s", op)
		assert.NotContains(t, strings.ToLower(js), "image\"", "audit row of %s", op)
		assert.LessOrEqual(t, len(js), 2000, "audit row of %s is small", op)
	}
	require.NoError(t, rows.Err())
	assert.True(t, ops["SetPhotoCommand"] && ops["RemovePhotoCommand"], "both operations are audited: %v", ops)

	erows, err := pool.Query(ctx, `SELECT data::text FROM msg_events WHERE subject LIKE '%' || $1 AND type LIKE 'platform:iam:field-worker:photo-%'`, w)
	require.NoError(t, err)
	defer erows.Close()
	n := 0
	for erows.Next() {
		var js string
		require.NoError(t, erows.Scan(&js))
		n++
		assert.NotContains(t, js, "/9j/")
		assert.LessOrEqual(t, len(js), 600)
	}
	require.NoError(t, erows.Err())
	assert.Equal(t, 2, n)
}
