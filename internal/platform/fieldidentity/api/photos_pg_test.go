//go:build integration

package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/imageproc"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// photoLimits are small so the limit tests stay cheap.
var photoLimits = imageproc.Limits{MaxBytes: 60 << 10, MaxPixels: 4_000_000, MaxSide: 4000}

const (
	permView   = "platform:iam:field-worker:view"
	permDevice = "platform:iam:field-device:manage"
)

// photoWorld serves the field admin API over HTTP, the caller named by the
// bearer value.
type photoWorld struct {
	t    *testing.T
	f    *fixture
	srv  *httptest.Server
	who  map[string]*auth.AuthContext
	appA string
	appB string
}

func newPhotoWorld(t *testing.T, f *fixture) *photoWorld {
	t.Helper()
	httpcompat.Init()
	f.s.Photos = imageproc.NewProcessor(photoLimits, 2)
	w := &photoWorld{t: t, f: f, appA: f.appA, appB: f.appB}
	w.who = map[string]*auth.AuthContext{
		"operator": {PrincipalID: "prn_phoperator001", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:*:*:*"}, PrincipalType: auth.PrincipalTypeUser},
		"manage":   {PrincipalID: "prn_phmanage00001", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{permManage}, PrincipalType: auth.PrincipalTypeUser},
		"view":     {PrincipalID: "prn_phview0000001", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{permView}, PrincipalType: auth.PrincipalTypeUser},
		"none":     {PrincipalID: "prn_phnone0000001", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{permDevice}, PrincipalType: auth.PrincipalTypeUser},
		"client-admin": {
			PrincipalID: "prn_phclient00001", Scope: auth.ScopeClient, Clients: []string{f.clientID}, AllApplications: true,
			Permissions: []string{permView, permManage}, PrincipalType: auth.PrincipalTypeUser,
		},
		"other-client-admin": {
			PrincipalID: "prn_phother000001", Scope: auth.ScopeClient, Clients: []string{"clt_0000000000zzz"}, AllApplications: true,
			Permissions: []string{permView, permManage}, PrincipalType: auth.PrincipalTypeUser,
		},
		"sa-a":        {PrincipalID: "prn_phsaa00000000", Scope: auth.ScopeAnchor, Applications: []string{f.appA}, Permissions: []string{permView, permManage}, PrincipalType: auth.PrincipalTypeService},
		"sa-b":        {PrincipalID: "prn_phsab00000000", Scope: auth.ScopeAnchor, Applications: []string{f.appB}, Permissions: []string{permView, permManage}, PrincipalType: auth.PrincipalTypeService},
		"sa-all-apps": {PrincipalID: "prn_phsaall000000", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{permView, permManage}, PrincipalType: auth.PrincipalTypeService},
	}
	r := chi.NewMux()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			name := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
			if ac, ok := w.who[name]; ok {
				req = req.WithContext(auth.WithContext(req.Context(), ac))
			}
			next.ServeHTTP(rw, req)
		})
	})
	Register(humachi.New(r, apiroute.PlatformAPIConfig("field-photos-test", "test")), f.s)
	w.srv = httptest.NewServer(r)
	t.Cleanup(w.srv.Close)
	return w
}

type reply struct {
	Status int
	Header http.Header
	Body   []byte
}

func (w *photoWorld) do(who, method, path string, body []byte, hdr map[string]string) reply {
	w.t.Helper()
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, w.srv.URL+path, rd)
	require.NoError(w.t, err)
	if who != "" {
		req.Header.Set("Authorization", "Bearer "+who)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := w.srv.Client().Do(req)
	require.NoError(w.t, err)
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	require.NoError(w.t, err)
	return reply{resp.StatusCode, resp.Header, out}
}

func (r reply) code() string {
	var e struct {
		Code string `json:"error"`
	}
	_ = json.Unmarshal(r.Body, &e)
	return e.Code
}

func (r reply) worker(t *testing.T) FieldWorkerResponse {
	t.Helper()
	var w FieldWorkerResponse
	require.NoError(t, json.Unmarshal(r.Body, &w), string(r.Body))
	return w
}

// picture makes an encoded image of the given type.
func picture(t testing.TB, kind imageproc.Type, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{c.R, uint8(x * 3), uint8(y * 3), 255})
		}
	}
	var b bytes.Buffer
	switch kind {
	case imageproc.TypeJPEG:
		require.NoError(t, jpeg.Encode(&b, img, nil))
	case imageproc.TypePNG:
		require.NoError(t, png.Encode(&b, img))
	case imageproc.TypeWebP:
		t.Fatalf("no encoder for %s", kind)
	default:
		t.Fatalf("no encoder for %s", kind)
	}
	return b.Bytes()
}

func photoPath(id string) string { return "/api/field-workers/" + id + "/photo" }

func (w *photoWorld) put(who, id string, body []byte, contentType string) reply {
	w.t.Helper()
	h := map[string]string{}
	if contentType != "" {
		h["Content-Type"] = contentType
	}
	return w.do(who, http.MethodPut, photoPath(id), body, h)
}

// ── authorisation ────────────────────────────────────────────────────────

// Every kind of caller against PUT, GET and DELETE of a worker of application
// A in the fixture's client.
func TestPhotoRoutesAuthorisationTable(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := newPhotoWorld(t, f)
	id := f.ensure(f.actx, "au-001", "Ann", f.codeA).Body.ID
	jpg := picture(t, imageproc.TypeJPEG, 40, 40, pRed)

	type row struct {
		who                  string
		put, get, del        int
		putCode, getCode, dc string
	}
	rows := []row{
		{"operator", 200, 200, 200, "", "", ""},
		{"manage", 200, 200, 200, "", "", ""},
		{"view", 403, 200, 403, "PERMISSION_REQUIRED", "", "PERMISSION_REQUIRED"},
		{"none", 403, 403, 403, "PERMISSION_REQUIRED", "PERMISSION_REQUIRED", "PERMISSION_REQUIRED"},
		{"client-admin", 200, 200, 200, "", "", ""},
		{"other-client-admin", 404, 404, 404, "FieldWorker_NOT_FOUND", "FieldWorker_NOT_FOUND", "FieldWorker_NOT_FOUND"},
		{"sa-a", 200, 200, 200, "", "", ""},
		{"sa-b", 404, 404, 404, "FieldWorker_NOT_FOUND", "FieldWorker_NOT_FOUND", "FieldWorker_NOT_FOUND"},
		{"sa-all-apps", 200, 200, 200, "", "", ""},
		{"", 403, 403, 403, "UNAUTHENTICATED", "UNAUTHENTICATED", "UNAUTHENTICATED"}, // the auth middleware answers 401 before a route is reached
	}
	for _, r := range rows {
		t.Run("caller "+r.who, func(t *testing.T) {
			// A photo for the GET and the DELETE to act on.
			require.Equal(t, 200, w.put("operator", id, jpg, "image/jpeg").Status)
			before := f.worker(id).PhotoVersion

			got := w.put(r.who, id, picture(t, imageproc.TypeJPEG, 50, 50, pBlue), "image/jpeg")
			assert.Equal(t, r.put, got.Status, "PUT: %s", got.Body)
			if r.putCode != "" {
				assert.Equal(t, r.putCode, got.code())
			}
			if r.put == 200 {
				assert.NotEqual(t, before, f.worker(id).PhotoVersion, "an allowed PUT replaces the photo")
			} else {
				assert.Equal(t, before, f.worker(id).PhotoVersion, "a refused PUT changes nothing")
			}
			require.Equal(t, 200, w.put("operator", id, jpg, "image/jpeg").Status)
			before = f.worker(id).PhotoVersion

			got = w.do(r.who, http.MethodGet, photoPath(id), nil, nil)
			assert.Equal(t, r.get, got.Status, "GET: %s", got.Body)
			if r.getCode != "" {
				assert.Equal(t, r.getCode, got.code())
			}
			if r.get == 200 {
				assert.Equal(t, "image/jpeg", got.Header.Get("Content-Type"))
			} else {
				assert.False(t, bytes.HasPrefix(got.Body, []byte{0xFF, 0xD8}), "a refused GET carries no image")
			}

			got = w.do(r.who, http.MethodDelete, photoPath(id), nil, nil)
			assert.Equal(t, r.del, got.Status, "DELETE: %s", got.Body)
			if r.dc != "" {
				assert.Equal(t, r.dc, got.code())
			}
			if r.del == 200 {
				assert.Empty(t, f.worker(id).PhotoVersion)
			} else {
				assert.Equal(t, before, f.worker(id).PhotoVersion, "a refused DELETE changes nothing")
			}
		})
	}
}

// A confined service account of another application cannot reach a worker that
// has no grant of its application, but one the worker is granted both to can.
func TestPhotoApplicationConfinement(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := newPhotoWorld(t, f)
	both := f.ensure(f.actx, "au-002", "Both", f.codeA, f.codeB).Body.ID
	jpg := picture(t, imageproc.TypeJPEG, 40, 40, pRed)
	assert.Equal(t, 200, w.put("sa-b", both, jpg, "image/jpeg").Status)
	assert.Equal(t, 200, w.do("sa-a", http.MethodGet, photoPath(both), nil, nil).Status, "visible to the other application's account too: it shares the worker")
	onlyA := f.ensure(f.actx, "au-003", "OnlyA", f.codeA).Body.ID
	assert.Equal(t, 404, w.put("sa-b", onlyA, jpg, "image/jpeg").Status)
	assert.Zero(t, f.photoRows(f.clientID)-1, "only the shared worker has a photo")
}

// One client's photos cannot be read, set or removed through another client's
// caller, and the worker JSON of one never carries the other's.
func TestPhotoTenancy(t *testing.T) {
	a := newFixture(t, hasher(t))
	b := newFixture(t, hasher(t))
	w := newPhotoWorld(t, a) // serves the shared State of fixture a
	// Serve fixture b's worker through the same State: both are in one database.
	wa := a.ensure(a.actx, "tn-001", "Aa", a.codeA).Body.ID
	wb := b.ensure(b.actx, "tn-001", "Bb", b.codeA).Body.ID
	w.who["admin-a"] = &auth.AuthContext{PrincipalID: "prn_phadmina00000", Scope: auth.ScopeClient, Clients: []string{a.clientID}, AllApplications: true, Permissions: []string{permView, permManage}, PrincipalType: auth.PrincipalTypeUser}
	w.who["admin-b"] = &auth.AuthContext{PrincipalID: "prn_phadminb00000", Scope: auth.ScopeClient, Clients: []string{b.clientID}, AllApplications: true, Permissions: []string{permView, permManage}, PrincipalType: auth.PrincipalTypeUser}
	pa := picture(t, imageproc.TypeJPEG, 40, 40, pRed)
	pb := picture(t, imageproc.TypeJPEG, 60, 60, pBlue)
	require.Equal(t, 200, w.put("admin-a", wa, pa, "image/jpeg").Status)
	require.Equal(t, 200, w.put("admin-b", wb, pb, "image/jpeg").Status)
	vb := a.worker(wb).PhotoVersion
	require.NotEmpty(t, vb)

	assert.Equal(t, 404, w.do("admin-a", http.MethodGet, photoPath(wb), nil, nil).Status, "read B's photo as A")
	assert.Equal(t, 404, w.put("admin-a", wb, pa, "image/jpeg").Status, "overwrite B's photo as A")
	assert.Equal(t, 404, w.do("admin-a", http.MethodDelete, photoPath(wb), nil, nil).Status, "remove B's photo as A")
	assert.Equal(t, vb, a.worker(wb).PhotoVersion, "B's photo is untouched")
	assert.Equal(t, 404, w.do("admin-b", http.MethodGet, photoPath(wa), nil, nil).Status)
	// Each reads its own and gets its own bytes.
	ra := w.do("admin-a", http.MethodGet, photoPath(wa), nil, nil)
	rb := w.do("admin-b", http.MethodGet, photoPath(wb), nil, nil)
	require.Equal(t, 200, ra.Status)
	require.Equal(t, 200, rb.Status)
	assert.NotEqual(t, ra.Body, rb.Body)
	// A's listing never shows B's worker or a photo of it.
	list := w.do("admin-a", http.MethodGet, "/api/field-workers?clientId="+b.clientID, nil, nil)
	assert.Equal(t, 403, list.Status)
	assert.NotContains(t, string(list.Body), vb)
}

// ── reading: headers and validators ──────────────────────────────────────

func TestPhotoReadHeadersAndValidators(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := newPhotoWorld(t, f)
	id := f.ensure(f.actx, "rd-001", "Ray", f.codeA).Body.ID

	none := w.do("operator", http.MethodGet, photoPath(id), nil, nil)
	assert.Equal(t, 404, none.Status)
	assert.Equal(t, "PHOTO_NOT_FOUND", none.code())

	put := w.put("operator", id, picture(t, imageproc.TypeJPEG, 900, 600, pRed), "image/jpeg")
	require.Equal(t, 200, put.Status, string(put.Body))
	v := put.worker(t).Photo
	require.NotNil(t, v)
	assert.Len(t, v.Version, 16)

	thumb := w.do("operator", http.MethodGet, photoPath(id), nil, nil)
	require.Equal(t, 200, thumb.Status)
	assert.Equal(t, "image/jpeg", thumb.Header.Get("Content-Type"))
	assert.Equal(t, `"`+v.Version+`-thumb"`, thumb.Header.Get("ETag"))
	assert.Equal(t, "private, no-cache", thumb.Header.Get("Cache-Control"))
	assert.Equal(t, "nosniff", thumb.Header.Get("X-Content-Type-Options"))
	assert.Equal(t, `inline; filename="photo.jpg"`, thumb.Header.Get("Content-Disposition"))
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(thumb.Body))
	require.NoError(t, err)
	assert.Equal(t, [2]int{128, 85}, [2]int{cfg.Width, cfg.Height})

	full := w.do("operator", http.MethodGet, photoPath(id)+"?size=full", nil, nil)
	require.Equal(t, 200, full.Status)
	assert.Equal(t, `"`+v.Version+`-full"`, full.Header.Get("ETag"))
	cfg, err = jpeg.DecodeConfig(bytes.NewReader(full.Body))
	require.NoError(t, err)
	assert.Equal(t, [2]int{512, 341}, [2]int{cfg.Width, cfg.Height})
	assert.Greater(t, len(full.Body), len(thumb.Body))

	// Conditional reads: 304 with no body, for a list, a weak form and "*".
	for _, inm := range []string{thumb.Header.Get("ETag"), `"nope", ` + thumb.Header.Get("ETag"), "W/" + thumb.Header.Get("ETag"), "*"} {
		c := w.do("operator", http.MethodGet, photoPath(id), nil, map[string]string{"If-None-Match": inm})
		assert.Equal(t, 304, c.Status, inm)
		assert.Empty(t, c.Body)
		assert.Equal(t, thumb.Header.Get("ETag"), c.Header.Get("ETag"))
		assert.Equal(t, "private, no-cache", c.Header.Get("Cache-Control"))
	}
	// The thumb's validator does not match the full rendition, nor an old one.
	assert.Equal(t, 200, w.do("operator", http.MethodGet, photoPath(id)+"?size=full", nil, map[string]string{"If-None-Match": thumb.Header.Get("ETag")}).Status)
	assert.Equal(t, 200, w.do("operator", http.MethodGet, photoPath(id), nil, map[string]string{"If-None-Match": `"0000000000000000-thumb"`}).Status)
	// A 304 is still gated: no permission, no 304.
	assert.Equal(t, 403, w.do("none", http.MethodGet, photoPath(id), nil, map[string]string{"If-None-Match": thumb.Header.Get("ETag")}).Status)

	// Replacing changes the version, so the old validator no longer matches.
	rep := w.put("operator", id, picture(t, imageproc.TypeJPEG, 300, 300, pBlue), "image/jpeg")
	require.Equal(t, 200, rep.Status)
	assert.NotEqual(t, v.Version, rep.worker(t).Photo.Version)
	assert.Equal(t, 200, w.do("operator", http.MethodGet, photoPath(id), nil, map[string]string{"If-None-Match": thumb.Header.Get("ETag")}).Status)

	// size is thumb or full.
	assert.Equal(t, 400, w.do("operator", http.MethodGet, photoPath(id)+"?size=huge", nil, nil).Status)
}

// ── uploads: accepted and refused ────────────────────────────────────────

func TestPhotoUploadFormsAndRefusals(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := newPhotoWorld(t, f)
	id := f.ensure(f.actx, "up-001", "Uma", f.codeA).Body.ID
	jpg := picture(t, imageproc.TypeJPEG, 80, 60, pRed)
	pngB := picture(t, imageproc.TypePNG, 80, 60, pRed)

	// Accepted: every declared form that agrees with the bytes.
	for name, c := range map[string]struct {
		body []byte
		ct   string
	}{
		"jpeg":                         {jpg, "image/jpeg"},
		"png":                          {pngB, "image/png"},
		"octet-stream":                 {jpg, "application/octet-stream"},
		"no content type":              {pngB, ""},
		"with a parameter":             {jpg, "image/jpeg; charset=binary"},
		"jpeg whose name is upper":     {jpg, "IMAGE/JPEG"},
		"png declared as octet":        {pngB, "application/octet-stream"},
		"declared type is a hint only": {jpg, "application/octet-stream; foo=bar"},
	} {
		got := w.put("operator", id, c.body, c.ct)
		assert.Equal(t, 200, got.Status, "%s: %s", name, got.Body)
	}
	good := f.worker(id).PhotoVersion

	bomb := func() []byte { // a JPEG whose header claims 60000 x 60000
		j := picture(t, imageproc.TypeJPEG, 16, 16, pRed)
		i := bytes.Index(j, []byte{0xFF, 0xC0})
		binary.BigEndian.PutUint16(j[i+5:], 60000)
		binary.BigEndian.PutUint16(j[i+7:], 60000)
		return j
	}()
	var gifB bytes.Buffer
	require.NoError(t, gif.Encode(&gifB, image.NewPaletted(image.Rect(0, 0, 8, 8), color.Palette{color.Black, color.White}), nil))

	refused := []struct {
		name   string
		body   []byte
		ct     string
		status int
		code   string
	}{
		{"empty", []byte{}, "image/jpeg", 400, "PHOTO_EMPTY"},
		{"text", []byte("not an image at all"), "image/jpeg", 415, "PHOTO_UNSUPPORTED_TYPE"},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), "image/svg+xml", 415, "PHOTO_UNSUPPORTED_TYPE"},
		{"gif", gifB.Bytes(), "image/gif", 415, "PHOTO_UNSUPPORTED_TYPE"},
		{"png declared as jpeg", pngB, "image/jpeg", 400, "PHOTO_TYPE_MISMATCH"},
		{"jpeg declared as text", jpg, "text/plain", 415, "PHOTO_UNSUPPORTED_TYPE"},
		{"truncated", jpg[:len(jpg)/2], "image/jpeg", 400, "PHOTO_UNREADABLE"},
		{"bomb", bomb, "image/jpeg", 400, "PHOTO_DIMENSIONS_TOO_LARGE"},
		{"over the limit", bytes.Repeat([]byte{0xFF, 0xD8, 0xFF}, int(photoLimits.MaxBytes)/3+10), "image/jpeg", 413, "PHOTO_TOO_LARGE"},
	}
	for _, r := range refused {
		got := w.put("operator", id, r.body, r.ct)
		assert.Equal(t, r.status, got.Status, "%s: %s", r.name, got.Body)
		assert.Equal(t, r.code, got.code(), r.name)
		assert.Equal(t, good, f.worker(id).PhotoVersion, "%s changed the photo", r.name)
	}
	assert.Equal(t, 1, f.photoRows(f.clientID))
	// Nothing of any upload but our own encoder's output is stored.
	full, _, err := f.s.Ops.R.Photos.Rendition(context.Background(), id, fieldidentity.PhotoFull)
	require.NoError(t, err)
	assert.True(t, bytes.HasPrefix(full.Bytes, []byte{0xFF, 0xD8}))
}

// The declared length is judged before a byte of the body is read: a request
// that announces more than the limit and then sends nothing is answered at once.
func TestPhotoContentLengthCheckedBeforeTheBodyIsRead(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := newPhotoWorld(t, f)
	id := f.ensure(f.actx, "cl-001", "Cal", f.codeA).Body.ID
	u := strings.TrimPrefix(w.srv.URL, "http://")
	conn, err := net.Dial("tcp", u)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	require.NoError(t, conn.SetDeadline(time.Now().Add(5*time.Second)))
	_, err = conn.Write([]byte("PUT " + photoPath(id) + " HTTP/1.1\r\nHost: x\r\nAuthorization: Bearer operator\r\n" +
		"Content-Type: image/jpeg\r\nContent-Length: 99999999\r\n\r\n")) // and no body
	require.NoError(t, err)
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err, "the answer came without the body being sent")
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, 413, resp.StatusCode, string(body))
	assert.Contains(t, string(body), "PHOTO_TOO_LARGE")
	assert.Zero(t, f.photoRows(f.clientID))
}

// ── who set it ───────────────────────────────────────────────────────────

func TestPhotoRecordsWhoSetIt(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := newPhotoWorld(t, f)
	id := f.ensure(f.actx, "ws-001", "Wes", f.codeA).Body.ID
	jpg := picture(t, imageproc.TypeJPEG, 40, 40, pRed)
	by := func() (string, string) {
		var typ, who string
		require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
			`SELECT set_by_type, set_by FROM field_worker_photos WHERE identity_id = $1`, id).Scan(&typ, &who))
		return typ, who
	}
	require.Equal(t, 200, w.put("operator", id, jpg, "image/jpeg").Status)
	typ, who := by()
	assert.Equal(t, "ADMIN", typ)
	assert.Equal(t, "prn_phoperator001", who)
	require.Equal(t, 200, w.put("sa-a", id, picture(t, imageproc.TypeJPEG, 41, 41, pBlue), "image/jpeg").Status)
	typ, who = by()
	assert.Equal(t, "SERVICE_ACCOUNT", typ)
	assert.Equal(t, "prn_phsaa00000000", who)
	assert.Equal(t, 2, f.eventCount(fieldidentity.WorkerPhotoSetType, id))
}

// ── the worker JSON and the policy ───────────────────────────────────────

func TestWorkerJSONCarriesThePhoto(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := newPhotoWorld(t, f)
	id := f.ensure(f.actx, "js-001", "Jo", f.codeA).Body.ID
	other := f.ensure(f.actx, "js-002", "Ki", f.codeA).Body.ID
	assert.Nil(t, w.do("operator", http.MethodGet, "/api/field-workers/"+id, nil, nil).worker(t).Photo)
	put := w.put("operator", id, picture(t, imageproc.TypeJPEG, 40, 40, pRed), "image/jpeg").worker(t)
	require.NotNil(t, put.Photo)
	got := w.do("operator", http.MethodGet, "/api/field-workers/"+id, nil, nil).worker(t)
	assert.Equal(t, put.Photo.Version, got.Photo.Version)
	assert.Equal(t, f.worker(id).PhotoVersion, got.Photo.Version)

	var list struct {
		Items []FieldWorkerResponse `json:"workers"`
	}
	raw := w.do("operator", http.MethodGet, "/api/field-workers?clientId="+f.clientID, nil, nil)
	require.Equal(t, 200, raw.Status, string(raw.Body))
	require.NoError(t, json.Unmarshal(raw.Body, &list))
	seen := map[string]*FieldWorkerPhoto{}
	for _, it := range list.Items {
		seen[it.ID] = it.Photo
	}
	require.Len(t, seen, 2, string(raw.Body))
	assert.NotNil(t, seen[id])
	assert.Nil(t, seen[other])
	assert.Contains(t, string(raw.Body), `"photo":null`, "no photo is null, not absent")

	// Delete: the JSON is null again; removing twice is fine.
	del := w.do("operator", http.MethodDelete, photoPath(id), nil, nil)
	assert.Equal(t, 200, del.Status)
	assert.Nil(t, del.worker(t).Photo)
	assert.Equal(t, 200, w.do("operator", http.MethodDelete, photoPath(id), nil, nil).Status)
	assert.Equal(t, 1, f.eventCount(fieldidentity.WorkerPhotoRemovedType, id))
}

func TestPolicyRoutesCarryThePhotoSettings(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := newPhotoWorld(t, f)
	w.who["policy-admin"] = &auth.AuthContext{
		PrincipalID: "prn_phpolicy00001", Scope: auth.ScopeAnchor, AllApplications: true,
		Permissions: []string{"platform:iam:field-device:view", permDevice, permView, permManage}, PrincipalType: auth.PrincipalTypeUser,
	}
	path := "/api/field-policies/" + f.clientID
	pol := func(r reply) FieldPolicyResponse {
		var p FieldPolicyResponse
		require.NoError(t, json.Unmarshal(r.Body, &p), string(r.Body))
		return p
	}
	put := func(body string) reply {
		// requireAttestation is the one field the body must carry.
		body = `{"requireAttestation":false,` + strings.TrimPrefix(body, "{")
		if body == `{"requireAttestation":false,}` {
			body = `{"requireAttestation":false}`
		}
		return w.do("policy-admin", http.MethodPut, path, []byte(body), map[string]string{"Content-Type": "application/json"})
	}

	def := pol(w.do("policy-admin", http.MethodGet, path, nil, nil))
	assert.True(t, def.PhotosEnabled)
	assert.False(t, def.WorkerMaySetOwnPhoto)
	assert.Contains(t, string(w.do("policy-admin", http.MethodGet, path, nil, nil).Body), `"photosEnabled":true`)

	// Set both away from the defaults, then say nothing: back to the defaults.
	r := put(`{"workerMaySetOwnPhoto":true}`)
	require.Equal(t, 200, r.Status, string(r.Body))
	got := pol(r)
	assert.True(t, got.PhotosEnabled, "photosEnabled omitted is true")
	assert.True(t, got.WorkerMaySetOwnPhoto)
	got = pol(put(`{}`))
	assert.True(t, got.PhotosEnabled)
	assert.False(t, got.WorkerMaySetOwnPhoto, "workerMaySetOwnPhoto omitted is false")

	// Switching off with photos stored is just the setting: the photos are
	// kept, hidden everywhere, and the codes are the distinct ones.
	id := f.ensure(f.actx, "po-001", "Pat", f.codeA).Body.ID
	id2 := f.ensure(f.actx, "po-002", "Quin", f.codeA).Body.ID
	require.Equal(t, 200, w.put("operator", id, picture(t, imageproc.TypeJPEG, 40, 40, pRed), "image/jpeg").Status)
	require.Equal(t, 200, w.put("operator", id2, picture(t, imageproc.TypeJPEG, 41, 41, pBlue), "image/jpeg").Status)
	shownBefore := w.do("operator", http.MethodGet, photoPath(id)+"?size=full", nil, nil)
	require.Equal(t, 200, shownBefore.Status)
	r = put(`{"photosEnabled":false}`)
	require.Equal(t, 200, r.Status, string(r.Body))
	assert.False(t, pol(r).PhotosEnabled)
	assert.Equal(t, 2, f.photoRows(f.clientID), "kept")
	assert.Nil(t, w.do("operator", http.MethodGet, "/api/field-workers/"+id, nil, nil).worker(t).Photo, "hidden in the worker JSON")

	set := w.put("operator", id, picture(t, imageproc.TypeJPEG, 40, 40, pRed), "image/jpeg")
	assert.Equal(t, 409, set.Status)
	assert.Equal(t, "PHOTOS_DISABLED_FOR_CLIENT", set.code())
	get := w.do("operator", http.MethodGet, photoPath(id), nil, nil)
	assert.Equal(t, 404, get.Status)
	assert.Equal(t, "PHOTO_NOT_FOUND", get.code())
	assert.Equal(t, 404, w.do("operator", http.MethodGet, photoPath(id), nil, map[string]string{"If-None-Match": shownBefore.Header.Get("ETag")}).Status, "no 304 either")

	// On again: the same photo comes back, byte for byte.
	require.Equal(t, 200, put(`{}`).Status)
	back := w.do("operator", http.MethodGet, photoPath(id)+"?size=full", nil, nil)
	require.Equal(t, 200, back.Status)
	assert.Equal(t, shownBefore.Body, back.Body)
	assert.Equal(t, shownBefore.Header.Get("ETag"), back.Header.Get("ETag"))
	assert.NotNil(t, w.do("operator", http.MethodGet, "/api/field-workers/"+id, nil, nil).worker(t).Photo)

	// An application-confined account cannot change the policy at all.
	conf := w.do("sa-a", http.MethodPut, path, []byte(`{"requireAttestation":false}`), map[string]string{"Content-Type": "application/json"})
	assert.Contains(t, []int{403, 404}, conf.Status)
}

// ── the platform-wide switch ─────────────────────────────────────────────

func TestPlatformSwitchOff(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := newPhotoWorld(t, f)
	id := f.ensure(f.actx, "sw-001", "Sy", f.codeA).Body.ID
	require.Equal(t, 200, w.put("operator", id, picture(t, imageproc.TypeJPEG, 40, 40, pRed), "image/jpeg").Status)
	assert.NotNil(t, w.do("operator", http.MethodGet, "/api/field-workers/"+id, nil, nil).worker(t).Photo)

	f.s.PhotosDisabled = true
	for _, m := range []string{http.MethodGet, http.MethodDelete} {
		got := w.do("operator", m, photoPath(id), nil, nil)
		assert.Equal(t, 404, got.Status, m)
		assert.Equal(t, "PHOTOS_DISABLED", got.code(), m)
	}
	put := w.put("operator", id, picture(t, imageproc.TypeJPEG, 40, 40, pRed), "image/jpeg")
	assert.Equal(t, 404, put.Status)
	assert.Equal(t, "PHOTOS_DISABLED", put.code())
	// The row is still there but the worker shows none, and the gate still
	// answers first: no permission is a 403, not a hint about the switch.
	assert.Nil(t, w.do("operator", http.MethodGet, "/api/field-workers/"+id, nil, nil).worker(t).Photo)
	assert.Equal(t, 1, f.photoRows(f.clientID))
	assert.Equal(t, 403, w.do("none", http.MethodGet, photoPath(id), nil, nil).Status)

	f.s.PhotosDisabled = false
	assert.Equal(t, 200, w.do("operator", http.MethodGet, photoPath(id), nil, nil).Status)
}

// With no Content-Length (a chunked body) the limit is still held: the body is
// read at most one byte past it.
func TestPhotoChunkedBodyOverTheLimit(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := newPhotoWorld(t, f)
	id := f.ensure(f.actx, "ch-001", "Chi", f.codeA).Body.ID
	big := append([]byte{0xFF, 0xD8, 0xFF}, bytes.Repeat([]byte{0}, int(photoLimits.MaxBytes)*2)...)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut, w.srv.URL+photoPath(id), io.NopCloser(bytes.NewReader(big)))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer operator")
	req.Header.Set("Content-Type", "image/jpeg")
	require.Zero(t, req.ContentLength, "unknown length, sent chunked")
	resp, err := w.srv.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, 413, resp.StatusCode, string(body))
	assert.Contains(t, string(body), "PHOTO_TOO_LARGE")
}

// ── deleting a client's photos ───────────────────────────────────────────

func TestDeleteClientPhotosRoute(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := newPhotoWorld(t, f)
	other := newFixture(t, hasher(t))
	path := "/api/field-policies/" + f.clientID + "/photos"
	jpg := picture(t, imageproc.TypeJPEG, 40, 40, pRed)
	var ids []string
	seed := func() {
		ids = ids[:0]
		for _, code := range []string{"dp-001", "dp-002"} {
			id := f.ensure(f.actx, code, code, f.codeA).Body.ID
			ids = append(ids, id)
			require.NoError(t, f.setPhoto(f.actx, id, fieldidentity.PhotoByAdmin, photoOf(t, pRed, 40+len(ids))))
		}
	}
	_ = jpg
	ow := other.ensure(other.actx, "dp-001", "x", other.codeA).Body.ID
	require.NoError(t, other.setPhoto(other.actx, ow, fieldidentity.PhotoByAdmin, photoOf(t, pBlue, 50)))
	w.who["policy-admin"] = &auth.AuthContext{
		PrincipalID: "prn_phpolicy00001", Scope: auth.ScopeAnchor, AllApplications: true,
		Permissions: []string{permDevice}, PrincipalType: auth.PrincipalTypeUser,
	}

	rows := []struct {
		who    string
		status int
		code   string
	}{
		{"operator", 200, ""},
		{"manage", 200, ""},
		{"client-admin", 200, ""},
		{"sa-all-apps", 200, ""},
		{"view", 403, "PERMISSION_REQUIRED"},
		{"none", 403, "PERMISSION_REQUIRED"},
		{"policy-admin", 403, "PERMISSION_REQUIRED"}, // field-device:manage is not field-worker:manage
		{"other-client-admin", 403, "SCOPE_FORBIDDEN"},
		{"sa-a", 403, "APPLICATION_FORBIDDEN"}, // client-wide: a confined account removes workers' photos one at a time
		{"sa-b", 403, "APPLICATION_FORBIDDEN"},
		{"", 403, "UNAUTHENTICATED"},
	}
	for _, r := range rows {
		t.Run("caller "+r.who, func(t *testing.T) {
			seed()
			got := w.do(r.who, http.MethodDelete, path, nil, nil)
			assert.Equal(t, r.status, got.Status, string(got.Body))
			if r.code != "" {
				assert.Equal(t, r.code, got.code())
			}
			if r.status == 200 {
				assert.Equal(t, 2, removedOf(t, got))
				assert.Zero(t, f.photoRows(f.clientID))
				for _, id := range ids {
					assert.Empty(t, f.worker(id).PhotoVersion)
				}
			} else {
				assert.Equal(t, 2, f.photoRows(f.clientID), "a refused call deletes nothing")
			}
			assert.Equal(t, 1, other.photoRows(other.clientID), "another client's photo is never touched")
		})
	}

	// Repeat: removed 0. By the client's identifier too. And with the platform
	// switch off the deletion still works.
	seed()
	assert.Equal(t, 2, removedOf(t, w.do("operator", http.MethodDelete, path, nil, nil)))
	assert.Equal(t, 0, removedOf(t, w.do("operator", http.MethodDelete, path, nil, nil)))
	seed()
	f.s.PhotosDisabled = true
	assert.Equal(t, 2, removedOf(t, w.do("operator", http.MethodDelete, path, nil, nil)))
	f.s.PhotosDisabled = false
	assert.Equal(t, 404, w.do("operator", http.MethodDelete, "/api/field-policies/clt_doesnotexist0/photos", nil, nil).Status)
}

func removedOf(t *testing.T, r reply) int {
	t.Helper()
	var b struct {
		Removed int `json:"removed"`
	}
	require.NoError(t, json.Unmarshal(r.Body, &b), string(r.Body))
	return b.Removed
}
