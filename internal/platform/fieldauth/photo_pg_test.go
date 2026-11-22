//go:build integration

package fieldauth

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/imageproc"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// ── helpers ──────────────────────────────────────────────────────────────

// photoLimits are small enough for cheap tests, and the byte limit is above
// the protocol's 64 KiB body cap so the raised cap of the set route shows.
var photoTestLimits = imageproc.Limits{MaxBytes: 256 << 10, MaxPixels: 4_000_000, MaxSide: 4000}

func (e *env) photos() {
	e.s.Photos = imageproc.NewProcessor(photoTestLimits, 2)
}

func solidJPEG(t testing.TB, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{c.R, uint8(x), uint8(y), 255})
		}
	}
	var b bytes.Buffer
	require.NoError(t, jpeg.Encode(&b, img, nil))
	return b.Bytes()
}

// noisyJPEG is a JPEG of random pixels: big for its size (over 64 KiB).
func noisyJPEG(t testing.TB, side int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	rnd := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = uint8(rnd.UintN(256)), uint8(rnd.UintN(256)), uint8(rnd.UintN(256)), 255
	}
	var b bytes.Buffer
	require.NoError(t, jpeg.Encode(&b, img, &jpeg.Options{Quality: 90}))
	return b.Bytes()
}

// adminPhoto sets a worker's photo as an administrator and returns it.
func (e *env) adminPhoto(workerID string, c color.RGBA) *imageproc.Photo {
	e.t.Helper()
	p, err := imageproc.Process(solidJPEG(e.t, 300, 200, c), "image/jpeg", photoTestLimits)
	require.NoError(e.t, err)
	cmd := fieldidentity.NewSetPhotoCommand(fieldidentity.WorkerRef{ID: workerID, ClientID: e.clientID}, fieldidentity.PhotoByAdmin, "prn_optestrunner1", p)
	_, err = usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.SetWorkerPhoto(), cmd, auth.NewExecutionContext(e.actx))
	require.NoError(e.t, err)
	return p
}

func (e *env) photoVersionOf(workerID string) string {
	e.t.Helper()
	var v *string
	require.NoError(e.t, testpg.Pool(e.t).QueryRow(context.Background(), `SELECT photo_version FROM field_identities WHERE id = $1`, workerID).Scan(&v))
	if v == nil {
		return ""
	}
	return *v
}

var (
	cRed  = color.RGBA{200, 10, 10, 255}
	cBlue = color.RGBA{10, 10, 200, 255}
)

// post sends one authenticated request whose body is raw bytes with a content
// type (an image), as the device libraries do for the set route.
func (d *device) postRaw(path string, body []byte, contentType, accessToken string) resp {
	d.t.Helper()
	if d.nonce == "" {
		d.fetchNonce()
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("X-Forwarded-For", d.ip)
	req.Header.Set("DPoP", d.proof(http.MethodPost, path, accessToken, proofOpts{}))
	req.Header.Set("Authorization", "DPoP "+accessToken)
	rec := httptest.NewRecorder()
	d.e.router.ServeHTTP(rec, req)
	if n := rec.Header().Get("DPoP-Nonce"); n != "" {
		d.nonce = n
	}
	return resp{rec}
}

func (d *device) readPhoto(token string, body map[string]any) resp {
	if body == nil {
		body = map[string]any{}
	}
	return d.callBearer("/field/worker/photo", body, token)
}

func (d *device) setPhoto(token string, jpg []byte) resp {
	return d.postRaw("/field/worker/photo/set", jpg, "image/jpeg", token)
}

func (d *device) removePhoto(token string) resp {
	return d.callBearer("/field/worker/photo/remove", map[string]any{}, token)
}

func workerOf(r resp) map[string]any {
	w, _ := r.json()["worker"].(map[string]any)
	return w
}

func jpegSize(t *testing.T, b []byte) [2]int {
	t.Helper()
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(b))
	require.NoError(t, err)
	return [2]int{cfg.Width, cfg.Height}
}

func photoEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t, envOpts{limits: roomyLimits()})
	e.photos()
	return e
}

// ── the sign-in answer ───────────────────────────────────────────────────

// Every place a session is answered says the same about the worker's photo:
// the login, a refresh, a supervisor acting as the worker, a console sign-in
// code and a badge.
func TestSignInAnswerPhotoFieldsEverywhere(t *testing.T) {
	e := photoEnv(t)
	targetID := e.worker("pa-1", "739204", false, e.codeA)
	e.supervisor("pa-sup", goodPIN, e.codeA)
	bareID := e.worker("pa-2", goodPIN, false, e.codeA) // no photo
	p := e.adminPhoto(targetID, cRed)
	badge := e.badge(targetID)
	principal := e.principal(true)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)

	// Each place answers the target's worker.
	type answer struct {
		name   string
		worker map[string]any
	}
	collect := func() []answer {
		var out []answer
		in := mustLogin(t, d, e.codeA, "pa-1", "739204")
		out = append(out, answer{"login", in.body["worker"].(map[string]any)})
		rr := d.refresh(in.refresh)
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		out = append(out, answer{"refresh", workerOf(rr)})
		br := d.loginBadge(e.codeA, badge, "739204")
		require.Equal(t, http.StatusOK, br.Code, br.Body.String())
		out = append(out, answer{"badge", workerOf(br)})
		sup := mustLogin(t, d, e.codeA, "pa-sup", goodPIN)
		ar := d.callBearer("/field/auth/act-as", map[string]any{"workerCode": "pa-1", "pin": goodPIN}, sup.access)
		require.Equal(t, http.StatusOK, ar.Code, ar.Body.String())
		out = append(out, answer{"act-as", workerOf(ar)})
		sc := d.call("/field/auth/sign-in-code", map[string]any{"code": e.signInCode(targetID, e.codeA, principal)})
		require.Equal(t, http.StatusOK, sc.Code, sc.Body.String())
		out = append(out, answer{"sign-in code", workerOf(sc)})
		return out
	}
	check := func(label string, wantVersion any, wantCan map[string]bool) {
		for _, a := range collect() {
			assert.Equal(t, wantVersion, a.worker["photoVersion"], "%s: %s photoVersion", label, a.name)
			assert.Equal(t, wantCan[a.name], a.worker["canSetOwnPhoto"], "%s: %s canSetOwnPhoto", label, a.name)
		}
	}
	none := map[string]bool{}
	allOwn := map[string]bool{"login": true, "refresh": true, "badge": true, "sign-in code": false, "act-as": false}

	// Default policy: the photo is shown; the worker may not set their own.
	check("default", p.Version, none)
	// Opt-in on: every own session may; a session someone else holds as the
	// worker (a supervisor acting as them, a console sign-in code) may not.
	e.policy("worker_may_set_own_photo", true)
	check("opt-in", p.Version, allOwn)
	// Photos off for the client: none, and no right to set one, though stored.
	e.policy("photos_enabled", false)
	check("client off", nil, none)
	assert.Equal(t, p.Version, e.photoVersionOf(targetID), "kept")
	// On again: back as it was.
	e.policy("photos_enabled", true)
	check("client on again", p.Version, allOwn)
	// Photos off on the platform: none, and no right to set one.
	e.s.PhotosDisabled = true
	check("platform off", nil, none)
	e.s.PhotosDisabled = false

	// A worker with no photo: null, but the right to set one is the policy's.
	in := mustLogin(t, d, e.codeA, "pa-2", goodPIN)
	w := in.body["worker"].(map[string]any)
	assert.Nil(t, w["photoVersion"])
	assert.Equal(t, true, w["canSetOwnPhoto"])
	_ = bareID
}

// ── reading ──────────────────────────────────────────────────────────────

func TestReadOwnPhoto(t *testing.T) {
	e := photoEnv(t)
	me := e.worker("rd-1", goodPIN, false, e.codeA)
	other := e.worker("rd-2", goodPIN, false, e.codeA)
	mine := e.adminPhoto(me, cRed)
	theirs := e.adminPhoto(other, cBlue)
	require.NotEqual(t, mine.Version, theirs.Version)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	in := mustLogin(t, d, e.codeA, "rd-1", goodPIN)

	thumb := d.readPhoto(in.access, nil)
	require.Equal(t, http.StatusOK, thumb.Code, thumb.Body.String())
	assert.Equal(t, "image/jpeg", thumb.Header().Get("Content-Type"))
	assert.Equal(t, `"`+mine.Version+`-thumb"`, thumb.Header().Get("ETag"))
	assert.Equal(t, "private, no-cache", thumb.Header().Get("Cache-Control"))
	assert.Equal(t, mine.Thumb.JPEG, thumb.Body.Bytes())
	assert.Equal(t, [2]int{mine.Thumb.Width, mine.Thumb.Height}, jpegSize(t, thumb.Body.Bytes()))
	full := d.readPhoto(in.access, map[string]any{"size": "full"})
	assert.Equal(t, mine.Full.JPEG, full.Body.Bytes())
	assert.Equal(t, `"`+mine.Version+`-full"`, full.Header().Get("ETag"))

	// The version the device holds: 304 and no body; another: the image.
	nm := d.readPhoto(in.access, map[string]any{"size": "thumb", "version": mine.Version})
	assert.Equal(t, http.StatusNotModified, nm.Code)
	assert.Empty(t, nm.Body.Bytes())
	assert.Equal(t, `"`+mine.Version+`-thumb"`, nm.Header().Get("ETag"))
	assert.Equal(t, http.StatusOK, d.readPhoto(in.access, map[string]any{"version": "0000000000000000"}).Code)

	// There is no way to name another worker: whatever the body says, it is
	// the token's own photo.
	for _, body := range []map[string]any{
		{"workerId": other}, {"id": other}, {"workerCode": "rd-2"}, {"subject": other, "clientId": e.clientID},
	} {
		r := d.readPhoto(in.access, body)
		require.Equal(t, http.StatusOK, r.Code)
		assert.Equal(t, mine.Thumb.JPEG, r.Body.Bytes(), "body %v", body)
	}
	assert.Equal(t, http.StatusBadRequest, d.readPhoto(in.access, map[string]any{"size": "huge"}).Code)

	// A worker with no photo: 404.
	e.worker("rd-3", goodPIN, false, e.codeA)
	in3 := mustLogin(t, d, e.codeA, "rd-3", goodPIN)
	r := d.readPhoto(in3.access, nil)
	assert.Equal(t, http.StatusNotFound, r.Code)
	assert.Equal(t, "PHOTO_NOT_FOUND", r.str("error"))

	// Photos off for the client: as no photo, though stored; the platform: its own code.
	e.policy("photos_enabled", false)
	r = d.readPhoto(in.access, nil)
	assert.Equal(t, http.StatusNotFound, r.Code)
	assert.Equal(t, "PHOTO_NOT_FOUND", r.str("error"))
	e.policy("photos_enabled", true)
	assert.Equal(t, http.StatusOK, d.readPhoto(in.access, nil).Code)
	e.s.PhotosDisabled = true
	r = d.readPhoto(in.access, nil)
	assert.Equal(t, http.StatusNotFound, r.Code)
	assert.Equal(t, "PHOTOS_DISABLED", r.str("error"))
	e.s.PhotosDisabled = false

	// A supervisor acting as the worker may read their photo.
	sup := e.supervisor("rd-sup", goodPIN, e.codeA)
	_ = sup
	s := mustLogin(t, d, e.codeA, "rd-sup", goodPIN)
	ar := d.callBearer("/field/auth/act-as", map[string]any{"workerCode": "rd-1", "pin": goodPIN}, s.access)
	require.Equal(t, http.StatusOK, ar.Code, ar.Body.String())
	assert.Equal(t, mine.Thumb.JPEG, d.readPhoto(ar.str("accessToken"), nil).Body.Bytes())
}

// ── setting and removing ─────────────────────────────────────────────────

func TestSetAndRemoveOwnPhoto(t *testing.T) {
	e := photoEnv(t)
	me := e.worker("so-1", goodPIN, false, e.codeA)
	other := e.worker("so-2", goodPIN, false, e.codeA)
	theirs := e.adminPhoto(other, cBlue)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	in := mustLogin(t, d, e.codeA, "so-1", goodPIN)
	jpg := solidJPEG(t, 400, 300, cRed)
	want, err := imageproc.Process(jpg, "image/jpeg", photoTestLimits)
	require.NoError(t, err)

	// The default: no upload from the device.
	r := d.setPhoto(in.access, jpg)
	assert.Equal(t, http.StatusForbidden, r.Code)
	assert.Equal(t, "OWN_PHOTO_NOT_ALLOWED", r.str("error"))
	r = d.removePhoto(in.access)
	assert.Equal(t, http.StatusForbidden, r.Code)
	assert.Equal(t, "OWN_PHOTO_NOT_ALLOWED", r.str("error"))
	assert.Empty(t, e.photoVersionOf(me))

	// Opted in: it is set, as the worker.
	e.policy("worker_may_set_own_photo", true)
	r = d.setPhoto(in.access, jpg)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.Equal(t, want.Version, r.str("photoVersion"))
	assert.Equal(t, want.Version, e.photoVersionOf(me))
	assert.Equal(t, theirs.Version, e.photoVersionOf(other), "no other worker is touched")
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_worker_photos WHERE identity_id = $1 AND set_by_type = 'WORKER' AND set_by = $1`, me))
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND subject LIKE '%' || $2 AND data->>'setByType' = 'WORKER'`, fieldidentity.WorkerPhotoSetType, me))
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM aud_logs WHERE entity_id = $1 AND operation = 'SetOwnPhotoCommand' AND principal_id = $1`, me))
	e.assertNoLeak("/9j/")
	// The sign-in answer now carries the version; the device reads it back.
	assert.Equal(t, want.Thumb.JPEG, d.readPhoto(in.access, nil).Body.Bytes())

	// Replace; a content type of the family or none is fine, the bytes decide.
	r = d.postRaw("/field/worker/photo/set", solidJPEG(t, 100, 100, cBlue), "application/octet-stream", in.access)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.NotEqual(t, want.Version, e.photoVersionOf(me))
	r = d.postRaw("/field/worker/photo/set", solidJPEG(t, 100, 100, cRed), "", in.access)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_worker_photos WHERE identity_id = $1`, me))

	// Remove; removing none is not an error; the others stay.
	r = d.removePhoto(in.access)
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.Nil(t, r.json()["photoVersion"])
	assert.Empty(t, e.photoVersionOf(me))
	assert.Equal(t, http.StatusOK, d.removePhoto(in.access).Code)
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND subject LIKE '%' || $2 AND data->>'removedByType' = 'WORKER'`, fieldidentity.WorkerPhotoRemovedType, me))
	// Naming another worker in the body of a remove changes nothing of theirs.
	r = d.callBearer("/field/worker/photo/remove", map[string]any{"workerId": other, "id": other}, in.access)
	assert.Equal(t, http.StatusOK, r.Code)
	assert.Equal(t, theirs.Version, e.photoVersionOf(other))

	// The opt-in off again: nothing more from the device, what is stored stays.
	require.Equal(t, http.StatusOK, d.setPhoto(in.access, jpg).Code)
	e.policy("worker_may_set_own_photo", false)
	assert.Equal(t, http.StatusForbidden, d.removePhoto(in.access).Code)
	assert.Equal(t, want.Version, e.photoVersionOf(me))

	// Photos off for the client; and on the platform.
	e.policy("worker_may_set_own_photo", true)
	e.policy("photos_enabled", false)
	for name, r := range map[string]resp{"set": d.setPhoto(in.access, jpg), "remove": d.removePhoto(in.access)} {
		assert.Equal(t, http.StatusConflict, r.Code, name)
		assert.Equal(t, "PHOTOS_DISABLED_FOR_CLIENT", r.str("error"), name)
	}
	assert.Equal(t, want.Version, e.photoVersionOf(me), "kept while hidden")
	e.policy("photos_enabled", true)
	e.s.PhotosDisabled = true
	for name, r := range map[string]resp{"set": d.setPhoto(in.access, jpg), "remove": d.removePhoto(in.access)} {
		assert.Equal(t, http.StatusNotFound, r.Code, name)
		assert.Equal(t, "PHOTOS_DISABLED", r.str("error"), name)
	}
	e.s.PhotosDisabled = false
}

func TestSetOwnPhotoRefusals(t *testing.T) {
	e := photoEnv(t)
	e.policy("worker_may_set_own_photo", true)
	me := e.worker("sr-1", goodPIN, false, e.codeA)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	in := mustLogin(t, d, e.codeA, "sr-1", goodPIN)
	good := solidJPEG(t, 100, 100, cRed)
	var pngB bytes.Buffer
	require.NoError(t, png.Encode(&pngB, image.NewRGBA(image.Rect(0, 0, 8, 8))))
	bomb := solidJPEG(t, 16, 16, cRed)
	i := bytes.Index(bomb, []byte{0xFF, 0xC0})
	binary.BigEndian.PutUint16(bomb[i+5:], 60000)
	binary.BigEndian.PutUint16(bomb[i+7:], 60000)
	tooBig := append([]byte{0xFF, 0xD8, 0xFF}, bytes.Repeat([]byte{0}, int(photoTestLimits.MaxBytes)+10)...)

	for _, c := range []struct {
		name   string
		body   []byte
		ct     string
		status int
		code   string
	}{
		{"empty", []byte{}, "image/jpeg", 400, "PHOTO_EMPTY"},
		{"text", []byte("not an image"), "image/jpeg", 415, "PHOTO_UNSUPPORTED_TYPE"},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), "image/svg+xml", 415, "PHOTO_UNSUPPORTED_TYPE"},
		{"png called jpeg", pngB.Bytes(), "image/jpeg", 400, "PHOTO_TYPE_MISMATCH"},
		{"declared as text", good, "text/plain", 415, "PHOTO_UNSUPPORTED_TYPE"},
		{"bomb", bomb, "image/jpeg", 400, "PHOTO_DIMENSIONS_TOO_LARGE"},
		{"truncated", good[:len(good)/2], "image/jpeg", 400, "PHOTO_UNREADABLE"},
		{"over the limit", tooBig, "image/jpeg", 413, "PHOTO_TOO_LARGE"},
	} {
		r := d.postRaw("/field/worker/photo/set", c.body, c.ct, in.access)
		assert.Equal(t, c.status, r.Code, "%s: %s", c.name, r.Body.String())
		assert.Equal(t, c.code, r.str("error"), c.name)
	}
	assert.Empty(t, e.photoVersionOf(me), "no refused upload stored anything")

	// The protocol's 64 KiB body cap is raised for the set route only: a
	// photograph of 100 KiB or more is accepted here, and a JSON route still
	// refuses a body that size.
	big := noisyJPEG(t, 320)
	require.Greater(t, len(big), 80<<10)
	require.Less(t, len(big), int(photoTestLimits.MaxBytes))
	r := d.setPhoto(in.access, big)
	assert.Equal(t, http.StatusOK, r.Code, r.Body.String())
	pad := map[string]any{"size": "thumb", "pad": strings.Repeat("x", 70<<10)}
	assert.NotEqual(t, http.StatusOK, d.readPhoto(in.access, pad).Code, "a JSON route keeps its small cap")

	// A Content-Length over the limit is refused without reading the body.
	d.nonce = ""
	req := httptest.NewRequest(http.MethodPost, "/field/worker/photo/set", &neverRead{})
	req.ContentLength = photoTestLimits.MaxBytes + 1
	d.fetchNonce()
	req.Header.Set("Content-Type", "image/jpeg")
	req.Header.Set("DPoP", d.proof(http.MethodPost, "/field/worker/photo/set", in.access, proofOpts{}))
	req.Header.Set("Authorization", "DPoP "+in.access)
	req.Header.Set("X-Forwarded-For", d.ip)
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, rec.Body.String())
}

// neverRead is a body that fails the test if read.
type neverRead struct{}

func (*neverRead) Read([]byte) (int, error) { panic("the body was read") }

// ── what each kind of session and device gets ────────────────────────────

func TestPhotoRoutesByKindOfSessionAndDevice(t *testing.T) {
	// A session that may only change its PIN.
	t.Run("pin change required", func(t *testing.T) {
		e := photoEnv(t)
		e.policy("worker_may_set_own_photo", true)
		id := e.worker("kd-1", goodPIN, true, e.codeA) // PIN set by a supervisor: change required
		e.adminPhoto(id, cRed)
		d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
		in := mustLogin(t, d, e.codeA, "kd-1", goodPIN)
		require.Equal(t, true, in.body["pinChangeRequired"])
		for name, r := range map[string]resp{
			"read": d.readPhoto(in.access, nil), "set": d.setPhoto(in.access, solidJPEG(t, 50, 50, cBlue)), "remove": d.removePhoto(in.access),
		} {
			assert.Equal(t, http.StatusUnauthorized, r.Code, name)
			assert.Equal(t, "invalid_token", r.str("error"), name)
		}
	})

	// A supervisor acting as the worker: may read, may not set or remove.
	t.Run("impersonating session", func(t *testing.T) {
		e := photoEnv(t)
		e.policy("worker_may_set_own_photo", true)
		targetID := e.worker("kd-2", "739204", false, e.codeA)
		p := e.adminPhoto(targetID, cRed)
		e.supervisor("kd-sup", goodPIN, e.codeA)
		d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
		sup := mustLogin(t, d, e.codeA, "kd-sup", goodPIN)
		ar := d.callBearer("/field/auth/act-as", map[string]any{"workerCode": "kd-2", "pin": goodPIN}, sup.access)
		require.Equal(t, http.StatusOK, ar.Code, ar.Body.String())
		imp := ar.str("accessToken")
		assert.Equal(t, http.StatusOK, d.readPhoto(imp, nil).Code)
		for name, r := range map[string]resp{"set": d.setPhoto(imp, solidJPEG(t, 50, 50, cBlue)), "remove": d.removePhoto(imp)} {
			assert.Equal(t, http.StatusForbidden, r.Code, name)
			assert.Equal(t, "IMPERSONATING", r.str("error"), name)
		}
		assert.Equal(t, p.Version, e.photoVersionOf(targetID))
		// The supervisor's own session is the supervisor's, as any worker's.
		assert.Equal(t, http.StatusOK, d.setPhoto(sup.access, solidJPEG(t, 50, 50, cBlue)).Code)
	})

	// An unenrolled key: allowed unless the client requires an enrolled device.
	t.Run("unenrolled device", func(t *testing.T) {
		e := photoEnv(t)
		e.policy("worker_may_set_own_photo", true)
		id := e.worker("kd-3", goodPIN, false, e.codeA)
		un := e.newDevice()
		in := mustLoginUn(t, un, e.codeA, "kd-3", goodPIN)
		r := un.setPhoto(in.access, solidJPEG(t, 50, 50, cBlue))
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		assert.NotEmpty(t, e.photoVersionOf(id))
		assert.Equal(t, http.StatusOK, un.readPhoto(in.access, nil).Code)
		e.policy("require_enrolled_device", true) // the client changes its mind
		for name, r := range map[string]resp{
			"read": un.readPhoto(in.access, nil), "set": un.setPhoto(in.access, solidJPEG(t, 50, 50, cRed)), "remove": un.removePhoto(in.access),
		} {
			assert.Equal(t, http.StatusUnauthorized, r.Code, name)
			assert.Equal(t, "UNKNOWN_DEVICE", r.str("error"), name)
		}
	})

	// A suspended or revoked device, and a suspended worker.
	t.Run("suspended and revoked device", func(t *testing.T) {
		e := photoEnv(t)
		e.policy("worker_may_set_own_photo", true)
		e.worker("kd-4", goodPIN, false, e.codeA)
		d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
		in := mustLogin(t, d, e.codeA, "kd-4", goodPIN)
		_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.SuspendDevice(), fieldidentity.DeviceRef{ClientID: e.clientID, ID: d.id}, auth.NewExecutionContext(e.actx))
		require.NoError(t, err)
		for name, r := range map[string]resp{"read": d.readPhoto(in.access, nil), "set": d.setPhoto(in.access, solidJPEG(t, 50, 50, cRed)), "remove": d.removePhoto(in.access)} {
			assert.Equal(t, http.StatusForbidden, r.Code, name)
			assert.Equal(t, "DEVICE_SUSPENDED", r.str("error"), name)
		}
		_, err = usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.RevokeDevice(), fieldidentity.DeviceRef{ClientID: e.clientID, ID: d.id}, auth.NewExecutionContext(e.actx))
		require.NoError(t, err)
		for name, r := range map[string]resp{"read": d.readPhoto(in.access, nil), "set": d.setPhoto(in.access, solidJPEG(t, 50, 50, cRed))} {
			assert.Equal(t, http.StatusForbidden, r.Code, name)
			assert.Equal(t, "DEVICE_REVOKED", r.str("error"), name)
		}
	})

	t.Run("suspended worker", func(t *testing.T) {
		e := photoEnv(t)
		e.policy("worker_may_set_own_photo", true)
		id := e.worker("kd-5", goodPIN, false, e.codeA)
		d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
		in := mustLogin(t, d, e.codeA, "kd-5", goodPIN)
		_, err := usecaseop.RunTx(e.actx, e.s.UoW, e.s.Ops.SuspendWorker(), fieldidentity.WorkerRef{ClientID: e.clientID, ID: id}, auth.NewExecutionContext(e.actx))
		require.NoError(t, err)
		for name, r := range map[string]resp{"read": d.readPhoto(in.access, nil), "set": d.setPhoto(in.access, solidJPEG(t, 50, 50, cRed)), "remove": d.removePhoto(in.access)} {
			assert.Equal(t, http.StatusUnauthorized, r.Code, name)
			assert.Equal(t, "invalid_token", r.str("error"), name)
		}
		assert.Empty(t, e.photoVersionOf(id))
	})

	// No bearer, a bad token, no proof.
	t.Run("no credentials", func(t *testing.T) {
		e := photoEnv(t)
		d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
		assert.Equal(t, http.StatusUnauthorized, d.call("/field/worker/photo", map[string]any{}).Code)
		assert.Equal(t, http.StatusUnauthorized, d.readPhoto("not-a-token", nil).Code)
		assert.Equal(t, http.StatusUnauthorized, d.postRaw("/field/worker/photo/set", solidJPEG(t, 20, 20, cRed), "image/jpeg", "x.y.z").Code)
		req := httptest.NewRequest(http.MethodPost, "/field/worker/photo", strings.NewReader("{}"))
		rec := httptest.NewRecorder()
		e.router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code, "no DPoP proof")
	})
}

// ── the rate limit ───────────────────────────────────────────────────────

func TestOwnPhotoRateLimit(t *testing.T) {
	p := roomyLimits()
	p.FieldPhotoRead = ratelimit.Policy{Window: time.Hour, Limit: 2}
	p.FieldPhotoWrite = ratelimit.Policy{Window: time.Hour, Limit: 2}
	e := newEnv(t, envOpts{limits: p})
	e.photos()
	e.policy("worker_may_set_own_photo", true)
	a := e.worker("rl-1", goodPIN, false, e.codeA)
	b := e.worker("rl-2", goodPIN, false, e.codeA)
	e.adminPhoto(a, cRed)
	d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
	ia := mustLogin(t, d, e.codeA, "rl-1", goodPIN)
	ib := mustLogin(t, d, e.codeA, "rl-2", goodPIN)

	assert.Equal(t, http.StatusOK, d.readPhoto(ia.access, nil).Code)
	assert.Equal(t, http.StatusOK, d.readPhoto(ia.access, nil).Code)
	r := d.readPhoto(ia.access, nil)
	assert.Equal(t, http.StatusTooManyRequests, r.Code)
	assert.NotEmpty(t, r.Header().Get("Retry-After"))
	// Each worker has their own budget, and reads and changes are counted apart.
	assert.NotEqual(t, http.StatusTooManyRequests, d.readPhoto(ib.access, nil).Code)
	jpg := solidJPEG(t, 50, 50, cBlue)
	assert.Equal(t, http.StatusOK, d.setPhoto(ia.access, jpg).Code)
	assert.Equal(t, http.StatusOK, d.removePhoto(ia.access).Code)
	r = d.setPhoto(ia.access, jpg)
	assert.Equal(t, http.StatusTooManyRequests, r.Code)
	assert.Equal(t, "TOO_MANY_REQUESTS", r.str("error"))
	assert.NotEmpty(t, r.Header().Get("Retry-After"))
	assert.Empty(t, e.photoVersionOf(a), "the refused change did not run")
	assert.Equal(t, http.StatusOK, d.setPhoto(ib.access, jpg).Code)
	assert.NotEmpty(t, e.photoVersionOf(b))
}
