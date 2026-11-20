package pages

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/imageproc"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// ── the fake's photo parts ───────────────────────────────────────────────

func (f *fakeField) PhotoCount(_ context.Context, clientID string) (int, error) {
	n := 0
	for _, w := range f.workers {
		if w.ClientID == clientID && w.PhotoVersion != "" {
			n++
		}
	}
	return n, nil
}

func (f *fakeField) workerIndex(id string) int {
	for i := range f.workers {
		if f.workers[i].ID == id {
			return i
		}
	}
	return -1
}

// SetWorkerPhoto applies the use case's client switch and records the command.
func (f *fakeField) SetWorkerPhoto(_ context.Context, cmd fieldidentity.SetPhotoCommand) error {
	f.lastPhoto = cmd
	if err := f.record("set-photo " + cmd.ID + " " + string(cmd.Actor)); err != nil {
		return err
	}
	if p, _ := f.Policy(context.Background(), f.workers[f.workerIndex(cmd.ID)].ClientID); !p.PhotosEnabled {
		return usecase.Conflict(fieldidentity.CodePhotosDisabledForClient, "this client has switched field-worker photos off")
	}
	now := fieldDay
	i := f.workerIndex(cmd.ID)
	f.workers[i].PhotoVersion, f.workers[i].PhotoUpdatedAt = cmd.Version, &now
	return nil
}

func (f *fakeField) RemoveWorkerPhoto(_ context.Context, cmd fieldidentity.RemovePhotoCommand) error {
	if err := f.record("remove-photo " + cmd.ID + " " + string(cmd.Actor)); err != nil {
		return err
	}
	i := f.workerIndex(cmd.ID)
	f.workers[i].PhotoVersion, f.workers[i].PhotoUpdatedAt = "", nil
	return nil
}

func (f *fakeField) DeleteClientPhotos(_ context.Context, cmd fieldidentity.DeleteClientPhotosCommand) (int, error) {
	if err := f.record("delete-photos " + cmd.ClientID); err != nil {
		return 0, err
	}
	n := 0
	for i := range f.workers {
		if f.workers[i].ClientID == cmd.ClientID && f.workers[i].PhotoVersion != "" {
			f.workers[i].PhotoVersion, f.workers[i].PhotoUpdatedAt = "", nil
			n++
		}
	}
	return n, nil
}

// ── fixtures ─────────────────────────────────────────────────────────────

const annVersion = "0123456789abcdef"

var testPhotoLimits = imageproc.Limits{MaxBytes: 40 << 10, MaxPixels: 1_000_000, MaxSide: 2000}

func testPhotos() FieldPhotos {
	return FieldPhotos{Processor: imageproc.NewProcessor(testPhotoLimits, 2)}
}

// photoFixture is the field fixture with Ann holding a photo.
func photoFixture() *fakeField {
	f := fieldFixture()
	f.workers[0].PhotoVersion = annVersion
	now := fieldDay
	f.workers[0].PhotoUpdatedAt = &now
	return f
}

func serveFieldPhotos(t *testing.T, f *fakeField, ac *auth.AuthContext, q req, photos FieldPhotos) *httptest.ResponseRecorder {
	t.Helper()
	sh, clients := shellForTest(), portalClients()
	return serveMounted(t, ac, q,
		&FieldWorkers{Store: f, Ops: f, Clients: clients, Photos: photos, Shell: sh},
		&FieldPolicy{Store: f, Ops: f, Clients: clients, Photos: photos, Shell: sh})
}

func photoBody(t *testing.T, f *fakeField, ac *auth.AuthContext, q req, photos FieldPhotos) string {
	t.Helper()
	return serveFieldPhotos(t, f, ac, q, photos).Body.String()
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{uint8(x), uint8(y), 90, 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// upload posts a multipart form with one file part (named "photo" unless
// field says otherwise) to the page, as the dialog's form does.
func uploadPhoto(t *testing.T, f *fakeField, ac *auth.AuthContext, target, field string, data []byte, contentType string, photos FieldPhotos, datastar bool) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	_ = mw.WriteField("drawer", "fwk_ann")
	if field != "" {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="`+field+`"; filename="me.jpg"`)
		if contentType != "" {
			h.Set("Content-Type", contentType)
		}
		part, err := mw.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write(data)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	mux := chi.NewRouter()
	sh, clients := shellForTest(), portalClients()
	mux.Route("/ui", func(r chi.Router) {
		(&FieldWorkers{Store: f, Ops: f, Clients: clients, Photos: photos, Shell: sh}).Mount(r)
	})
	r := httptest.NewRequest(http.MethodPost, target, &b)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Accept", "text/html")
	if datastar {
		r.Header.Set(ds.RequestHeader, "true")
		r.Header.Set("Accept", "text/event-stream, text/html, application/json")
	}
	r = r.WithContext(auth.WithContext(r.Context(), ac))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}

func managerCtx() *auth.AuthContext {
	return fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)
}

// ── the drawer ───────────────────────────────────────────────────────────

func TestFieldWorkerDrawerPhoto(t *testing.T) {
	f := photoFixture()
	f.workers[1].PhotoVersion = ""
	body := photoBody(t, f, managerCtx(), req{target: "/ui/field-workers/fwk_ann"}, testPhotos())
	mustContain(t, body,
		`src="/api/field-workers/fwk_ann/photo?size=thumb&amp;v=`+annVersion+`"`,
		`class="h-24 w-24 shrink-0 rounded-full object-cover"`,
		"Change photo", "Remove photo", `data-action="/ui/field-workers/fwk_ann/photo/remove"`,
		`href="/ui/field-workers/fwk_ann/photo"`,
		`<dialog id="fw-photo-remove"`, "Remove this worker&#39;s photo? People see their initials until a new photo is added.",
	)
	mustNotContain(t, body, "Add photo")
	// The question is in the ordinary tone, not the red of a delete.
	i := strings.Index(body, `<dialog id="fw-photo-remove"`)
	dlg := body[i : i+strings.Index(body[i:], "</dialog>")]
	mustContain(t, dlg, `class="fc-btn fc-btn-primary"`, ">Remove photo<")
	mustNotContain(t, dlg, "fc-btn-danger")

	// A worker with no photo: initials and "Add photo", no remove.
	body = photoBody(t, f, managerCtx(), req{target: "/ui/field-workers/fwk_bob"}, testPhotos())
	mustContain(t, body, `class="fc-avatar fc-avatar-lg"`, ">B<", "Add photo", "No photo. A photo helps people recognise this worker.")
	mustNotContain(t, body, `data-action="/ui/field-workers/fwk_bob/photo/remove"`, `src="/api/field-workers/fwk_bob`)

	// A viewer sees the photo and no control.
	viewer := fieldCaller([]string{"clt_acme"}, permFieldWorkerView)
	body = photoBody(t, f, viewer, req{target: "/ui/field-workers/fwk_ann"}, testPhotos())
	mustContain(t, body, `src="/api/field-workers/fwk_ann/photo?size=thumb&amp;v=`+annVersion+`"`)
	mustNotContain(t, body, "Change photo", "Remove photo", `href="/ui/field-workers/fwk_ann/photo"`)
}

func TestFieldWorkerDrawerPhotoClientOff(t *testing.T) {
	f := photoFixture()
	f.policies = map[string]fieldidentity.Policy{"clt_acme": {
		ClientID: "clt_acme", PINMinLength: 6, SoftLockThreshold: 5, SoftLockSecs: 900, HardLockThreshold: 10,
		SharedIdleSecs: 900, PersonalIdleSecs: 7200, ShiftMaxSecs: 50400, PINReauthDays: 7, UnenrolledIdleSecs: 900, ImpersonationMaxSecs: 14400, WorkerCodeDigits: 3,
	}}
	body := photoBody(t, f, managerCtx(), req{target: "/ui/field-workers/fwk_ann"}, testPhotos())
	// Kept but hidden: no picture, no control, and the way to switch it on.
	mustContain(t, body, "Photos are switched off for this client, so workers are listed with their initials.", `href="/ui/field-policy?client=clt_acme"`)
	mustNotContain(t, body, "/api/field-workers/fwk_ann/photo", "Change photo", "Add photo", `data-action="/ui/field-workers/fwk_ann/photo/remove"`)
	if f.workers[0].PhotoVersion != annVersion {
		t.Error("the stored photo was touched")
	}
}

func TestFieldWorkerPhotoPlatformOff(t *testing.T) {
	f := photoFixture()
	off := FieldPhotos{Processor: testPhotos().Processor, Off: true}
	for _, target := range []string{"/ui/field-workers/fwk_ann", "/ui/field-workers?client=clt_acme"} {
		body := photoBody(t, f, managerCtx(), req{target: target}, off)
		mustNotContain(t, body, "/api/field-workers/", `class="fc-avatar" aria-hidden`, "data-fw-photo", "Photo", "photo", "fw-photo-remove")
	}
	// The routes answer as the API does.
	for _, q := range []req{
		{target: "/ui/field-workers/fwk_ann/photo"},
		nativePost("/ui/field-workers/fwk_ann/photo/remove"),
	} {
		rec := serveFieldPhotos(t, f, managerCtx(), q, off)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d", q.target, rec.Code)
		}
	}
	rec := uploadPhoto(t, f, managerCtx(), "/ui/field-workers/fwk_ann/photo", "photo", jpegBytes(t, 32, 32), "image/jpeg", off, false)
	if rec.Code != http.StatusNotFound {
		t.Errorf("upload with photos off: status %d", rec.Code)
	}
	for _, c := range f.calls {
		if strings.Contains(c, "photo") {
			t.Errorf("a photo command ran: %s", c)
		}
	}
	// The policy page draws no photo settings and no delete.
	body := photoBody(t, f, managerCtx2(), req{target: "/ui/field-policy?client=clt_acme"}, off)
	mustNotContain(t, body, "Worker photos", "Delete all photos")
}

func managerCtx2() *auth.AuthContext {
	return fieldCaller([]string{"clt_acme"}, permFieldDeviceView, permFieldDeviceManage, permFieldWorkerManage)
}

// ── the list ─────────────────────────────────────────────────────────────

func TestFieldWorkersListPhotos(t *testing.T) {
	f := photoFixture()
	body := photoBody(t, f, managerCtx(), req{target: "/ui/field-workers?client=clt_acme"}, testPhotos())
	// Ann: her photo; Bob: his initials.
	mustContain(t, body,
		`src="/api/field-workers/fwk_ann/photo?size=thumb&amp;v=`+annVersion+`"`, `class="size-8 shrink-0 rounded-full object-cover"`,
		`<span class="fc-avatar" aria-hidden="true">B</span>`)
	if n := strings.Count(body, "/api/field-workers/fwk_"); n != 1 {
		t.Errorf("%d photo addresses in the list, want 1", n)
	}
	// Photos off for the client: initials only, the photo is kept.
	f.policies = map[string]fieldidentity.Policy{"clt_acme": {ClientID: "clt_acme"}}
	body = photoBody(t, f, managerCtx(), req{target: "/ui/field-workers?client=clt_acme"}, testPhotos())
	mustNotContain(t, body, "/api/field-workers/fwk_")
	mustContain(t, body, `<span class="fc-avatar" aria-hidden="true">B</span>`)
}

// ── the upload ───────────────────────────────────────────────────────────

func TestFieldWorkerPhotoUpload(t *testing.T) {
	f := photoFixture()
	// The dialog.
	body := photoBody(t, f, managerCtx(), req{target: "/ui/field-workers/fwk_bob/photo"}, testPhotos())
	mustContain(t, body, `<dialog id="fw-photo-form"`, `enctype="multipart/form-data"`, `type="file"`, `accept="image/jpeg,image/png,image/webp"`,
		"Photo for BOB", "Choose a JPEG, PNG or WebP image of up to 0.0390625 MB", "Save photo", `action="/ui/field-workers/fwk_bob/photo"`)

	// A good upload (native post): redirected, the use case ran once with the
	// processed photo and the caller as the actor.
	data := jpegBytes(t, 64, 48)
	want, err := imageproc.Process(data, "image/jpeg", testPhotoLimits)
	if err != nil {
		t.Fatal(err)
	}
	rec := uploadPhoto(t, f, managerCtx(), "/ui/field-workers/fwk_bob/photo", "photo", data, "image/jpeg", testPhotos(), false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if got := strings.Join(f.calls, ";"); got != "set-photo fwk_bob ADMIN" {
		t.Errorf("calls %q", got)
	}
	if f.lastPhoto.Version != want.Version || f.lastPhoto.Image == nil || f.workers[1].PhotoVersion != want.Version {
		t.Errorf("the use case did not get the processed photo: %+v", f.lastPhoto)
	}
	if f.lastPhoto.ActorID != "prn_fc" {
		t.Errorf("actor %q", f.lastPhoto.ActorID)
	}

	// Replace (Datastar): the dialog closes, the drawer and the list refresh, a flash.
	f.calls = nil
	rec = uploadPhoto(t, f, managerCtx(), "/ui/field-workers/fwk_ann/photo", "photo", jpegBytes(t, 50, 50), "image/jpeg", testPhotos(), true)
	out := rec.Body.String()
	mustContain(t, out, "datastar-patch-elements", "Photo saved", "Photo saved for ANN")
	if got := strings.Join(f.calls, ";"); got != "set-photo fwk_ann ADMIN" {
		t.Errorf("calls %q", got)
	}
	if f.workers[0].PhotoVersion == annVersion {
		t.Error("the photo was not replaced")
	}
}

func TestFieldWorkerPhotoRefusals(t *testing.T) {
	big := append([]byte{0xFF, 0xD8, 0xFF}, bytes.Repeat([]byte{0}, int(testPhotoLimits.MaxBytes)+10)...)
	bomb := jpegBytes(t, 16, 16)
	i := bytes.Index(bomb, []byte{0xFF, 0xC0})
	binary.BigEndian.PutUint16(bomb[i+5:], 60000)
	binary.BigEndian.PutUint16(bomb[i+7:], 60000)
	good := jpegBytes(t, 64, 64)
	cases := []struct {
		name  string
		field string
		data  []byte
		ct    string
		want  string
	}{
		{"no file", "", nil, "", "Choose an image file."},
		{"empty file", "photo", []byte{}, "image/jpeg", "The image is empty. Choose an image file and send it again."},
		{"text", "photo", []byte("not an image"), "image/jpeg", "Choose a JPEG, PNG or WebP image."},
		{"svg", "photo", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), "image/svg+xml", "Choose a JPEG, PNG or WebP image."},
		{"png called jpeg", "photo", pngBytes(t), "image/jpeg", "The file is not the type its Content-Type says. Choose the image file again."},
		{"bomb", "photo", bomb, "image/jpeg", "The image has too many pixels. Choose a smaller one."},
		{"truncated", "photo", good[:len(good)/2], "image/jpeg", "The image cannot be read. Choose another file."},
		{"too large", "photo", big, "image/jpeg", "The photo is larger than 40960 bytes. Choose a smaller image."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := photoFixture()
			// Native: the dialog is on the page again, with the message on the file field.
			rec := uploadPhoto(t, f, managerCtx(), "/ui/field-workers/fwk_ann/photo", c.field, c.data, c.ct, testPhotos(), false)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
			mustContain(t, rec.Body.String(), `<dialog id="fw-photo-form"`, c.want, `id="fw-photo-form-photo-err"`)
			// Datastar: the message is patched onto the file field.
			rec = uploadPhoto(t, f, managerCtx(), "/ui/field-workers/fwk_ann/photo", c.field, c.data, c.ct, testPhotos(), true)
			mustContain(t, rec.Body.String(), "datastar-patch-elements", c.want)
			if len(f.calls) != 0 || f.workers[0].PhotoVersion != annVersion {
				t.Errorf("a refused upload ran %v", f.calls)
			}
		})
	}

	// The client has photos off: the use case's refusal is the form's message.
	f := photoFixture()
	f.policies = map[string]fieldidentity.Policy{"clt_acme": {ClientID: "clt_acme"}}
	rec := uploadPhoto(t, f, managerCtx(), "/ui/field-workers/fwk_ann/photo", "photo", good, "image/jpeg", testPhotos(), false)
	mustContain(t, rec.Body.String(), "This client has switched field-worker photos off")
	if rec.Code != http.StatusConflict {
		t.Errorf("status %d", rec.Code)
	}
	if f.workers[0].PhotoVersion != annVersion {
		t.Error("the photo changed")
	}
}

func TestFieldWorkerPhotoRemove(t *testing.T) {
	f := photoFixture()
	rec := serveFieldPhotos(t, f, managerCtx(), nativePost("/ui/field-workers/fwk_ann/photo/remove"), testPhotos())
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d", rec.Code)
	}
	if got := strings.Join(f.calls, ";"); got != "remove-photo fwk_ann ADMIN" {
		t.Errorf("calls %q", got)
	}
	if f.workers[0].PhotoVersion != "" {
		t.Error("not removed")
	}
	// Datastar: the flash.
	f = photoFixture()
	rec = serveFieldPhotos(t, f, managerCtx(), postForm("/ui/field-workers/fwk_ann/photo/remove", "drawer", "fwk_ann"), testPhotos())
	mustContain(t, rec.Body.String(), "Photo removed")
}

func TestFieldWorkerPhotoAccess(t *testing.T) {
	good := jpegBytes(t, 64, 64)
	viewer := fieldCaller([]string{"clt_acme"}, permFieldWorkerView)
	other := fieldCaller([]string{"clt_globex"}, permFieldWorkerView, permFieldWorkerManage)
	for _, who := range []struct {
		name string
		ac   *auth.AuthContext
		want int
	}{{"viewer", viewer, http.StatusForbidden}, {"another client's manager", other, http.StatusNotFound}} {
		t.Run(who.name, func(t *testing.T) {
			f := photoFixture()
			if rec := serveFieldPhotos(t, f, who.ac, req{target: "/ui/field-workers/fwk_ann/photo"}, testPhotos()); rec.Code != who.want {
				t.Errorf("dialog: status %d", rec.Code)
			}
			if rec := uploadPhoto(t, f, who.ac, "/ui/field-workers/fwk_ann/photo", "photo", good, "image/jpeg", testPhotos(), false); rec.Code != who.want {
				t.Errorf("upload: status %d", rec.Code)
			}
			if rec := serveFieldPhotos(t, f, who.ac, nativePost("/ui/field-workers/fwk_ann/photo/remove"), testPhotos()); rec.Code != who.want {
				t.Errorf("remove: status %d", rec.Code)
			}
			if len(f.calls) != 0 || f.workers[0].PhotoVersion != annVersion {
				t.Errorf("a refused call ran %v", f.calls)
			}
		})
	}
	// Another client's worker is not in the manager's page at all.
	f := photoFixture()
	body := photoBody(t, f, managerCtx(), req{target: "/ui/field-workers?client=clt_acme"}, testPhotos())
	mustNotContain(t, body, "fwk_gus")
}

// ── the policy page ──────────────────────────────────────────────────────

func policyCtx() *auth.AuthContext {
	return fieldCaller([]string{"clt_acme"}, permFieldDeviceView, permFieldDeviceManage, permFieldWorkerView, permFieldWorkerManage)
}

func TestFieldPolicyPhotoSettings(t *testing.T) {
	f := photoFixture()
	f.workers[1].PhotoVersion = "fedcba9876543210" // two photos in Acme
	body := photoBody(t, f, policyCtx(), req{target: "/ui/field-policy?client=clt_acme"}, testPhotos())
	mustContain(t, body,
		`id="field-policy-photos-form"`, `action="/ui/field-policy/photos"`, "Worker photos", "Photos for this client", "Workers may set their own photo",
		`name="photosEnabled"`, `name="workerMaySetOwnPhoto"`, "Save photo settings",
		"Switching photos off hides them and keeps them", "Only Delete all photos, below, deletes them.",
		"Delete all photos", "Delete the 2 photos stored for Acme Corp's workers, whether photos are on or off. This cannot be undone.",
		`<dialog id="field-policy-photos-delete"`, "Delete all photos for Acme Corp?", "This deletes the 2 photos stored for Acme Corp&#39;s workers.", "Delete 2 photos",
		`data-action="/ui/field-policy/photos/delete"`, `fc-btn-danger`)
	mustNotContain(t, body, "has no effect until they are switched on")

	// Off: the setting says it has no effect, and the delete is still offered.
	f.policies = map[string]fieldidentity.Policy{"clt_acme": {
		ClientID: "clt_acme", PINMinLength: 6, SoftLockThreshold: 5, SoftLockSecs: 900, HardLockThreshold: 10,
		SharedIdleSecs: 900, PersonalIdleSecs: 7200, ShiftMaxSecs: 50400, PINReauthDays: 7, UnenrolledIdleSecs: 900, ImpersonationMaxSecs: 14400, WorkerCodeDigits: 3,
		WorkerMaySetOwnPhoto: true,
	}}
	body = photoBody(t, f, policyCtx(), req{target: "/ui/field-policy?client=clt_acme"}, testPhotos())
	mustContain(t, body, "Photos are off for this client, so the setting above has no effect until they are switched on.", "Delete all photos", "Delete 2 photos")

	// Nothing stored: the button is disabled and says why.
	f = fieldFixture()
	body = photoBody(t, f, policyCtx(), req{target: "/ui/field-policy?client=clt_acme"}, testPhotos())
	mustContain(t, body, "Acme Corp has no stored photos to delete.")
	mustNotContain(t, body, `<dialog id="field-policy-photos-delete"`)

	// A viewer: no delete, no save.
	viewer := fieldCaller([]string{"clt_acme"}, permFieldDeviceView)
	body = photoBody(t, f, viewer, req{target: "/ui/field-policy?client=clt_acme"}, testPhotos())
	mustNotContain(t, body, `data-action="/ui/field-policy/photos/delete"`, `fc-danger-item`, "Save photo settings")
}

func savedPolicy(f *fakeField) fieldidentity.Policy { return f.policies["clt_acme"] }

func TestFieldPolicyPhotoSettingsSave(t *testing.T) {
	f := photoFixture()
	f.policies = map[string]fieldidentity.Policy{"clt_acme": {
		ClientID: "clt_acme", PINMinLength: 8, SoftLockThreshold: 5, SoftLockSecs: 600, HardLockThreshold: 10,
		SharedIdleSecs: 900, PersonalIdleSecs: 7200, ShiftMaxSecs: 50400, PINReauthDays: 7, UnenrolledIdleSecs: 900, ImpersonationMaxSecs: 14400, WorkerCodeDigits: 3,
		PhotosEnabled: true, RequireAttestation: true,
	}}

	// Switching photos off is just the setting: one plain PUT, nothing asked,
	// nothing deleted, every other setting as it was.
	rec := serveFieldPhotos(t, f, policyCtx(), postForm("/ui/field-policy/photos", "client", "clt_acme", "workerMaySetOwnPhoto", "1"), testPhotos())
	mustContain(t, rec.Body.String(), "Photo settings saved")
	if got := strings.Join(f.calls, ";"); got != "put-policy clt_acme" {
		t.Errorf("calls %q", got)
	}
	p := savedPolicy(f)
	if p.PhotosEnabled || !p.WorkerMaySetOwnPhoto {
		t.Errorf("photo settings %+v", p)
	}
	if p.PINMinLength != 8 || p.SoftLockSecs != 600 || !p.RequireAttestation {
		t.Errorf("the save changed another setting: %+v", p)
	}
	if f.workers[0].PhotoVersion != annVersion {
		t.Error("a photo was touched by switching off")
	}

	// And on again: the photo is still there.
	f.calls = nil
	serveFieldPhotos(t, f, policyCtx(), postForm("/ui/field-policy/photos", "client", "clt_acme", "photosEnabled", "1"), testPhotos())
	if p = savedPolicy(f); !p.PhotosEnabled || p.WorkerMaySetOwnPhoto || p.PINMinLength != 8 {
		t.Errorf("photo settings %+v", p)
	}
	if f.workers[0].PhotoVersion != annVersion {
		t.Error("the photo did not survive")
	}

	// An unrelated save changes neither photo setting.
	f.policies["clt_acme"] = fieldidentity.Policy{
		ClientID: "clt_acme", PINMinLength: 6, SoftLockThreshold: 5, SoftLockSecs: 900, HardLockThreshold: 10,
		SharedIdleSecs: 900, PersonalIdleSecs: 7200, ShiftMaxSecs: 50400, PINReauthDays: 7, UnenrolledIdleSecs: 900, ImpersonationMaxSecs: 14400, WorkerCodeDigits: 3,
		PhotosEnabled: false, WorkerMaySetOwnPhoto: true,
	}
	serveFieldPhotos(t, f, policyCtx(), postForm("/ui/field-policy", "client", "clt_acme", "pinMinLength", "9"), testPhotos())
	if p = savedPolicy(f); p.PINMinLength != 9 || p.PhotosEnabled || !p.WorkerMaySetOwnPhoto {
		t.Errorf("an unrelated save changed the photo settings: %+v", p)
	}
}

func TestFieldPolicyPhotoDeleteAll(t *testing.T) {
	f := photoFixture()
	f.workers[1].PhotoVersion = "fedcba9876543210"
	gus := f.workerIndex("fwk_gus")
	f.workers[gus].PhotoVersion = "1111111111111111" // another client's photo

	// A viewer, and a device manager without the worker permission: refused.
	for _, ac := range []*auth.AuthContext{
		fieldCaller([]string{"clt_acme"}, permFieldDeviceView, permFieldWorkerView),
		fieldCaller([]string{"clt_acme"}, permFieldDeviceView, permFieldDeviceManage),
		fieldCaller([]string{"clt_globex"}, permFieldWorkerManage, permFieldDeviceManage), // not this client
	} {
		rec := serveFieldPhotos(t, f, ac, nativePost("/ui/field-policy/photos/delete", "client", "clt_acme"), testPhotos())
		if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
			t.Errorf("status %d", rec.Code)
		}
	}
	// An application-confined account.
	conf := policyCtx()
	conf.AllApplications, conf.Applications = false, []string{"app_wms"}
	if rec := serveFieldPhotos(t, f, conf, nativePost("/ui/field-policy/photos/delete", "client", "clt_acme"), testPhotos()); rec.Code != http.StatusForbidden {
		t.Errorf("confined: status %d", rec.Code)
	}
	if len(f.calls) != 0 {
		t.Fatalf("a refused call ran %v", f.calls)
	}

	// The manager: both of Acme's photos go, Globex's stays.
	rec := serveFieldPhotos(t, f, policyCtx(), postForm("/ui/field-policy/photos/delete", "client", "clt_acme"), testPhotos())
	mustContain(t, rec.Body.String(), "Deleted 2 photos for Acme Corp")
	if got := strings.Join(f.calls, ";"); got != "delete-photos clt_acme" {
		t.Errorf("calls %q", got)
	}
	if f.workers[0].PhotoVersion != "" || f.workers[1].PhotoVersion != "" || f.workers[gus].PhotoVersion == "" {
		t.Error("wrong photos deleted")
	}
	// Photos off: still offered and still works (repeat: none left).
	f.policies = map[string]fieldidentity.Policy{"clt_acme": {ClientID: "clt_acme"}}
	f.workers[0].PhotoVersion = annVersion
	body := photoBody(t, f, policyCtx(), req{target: "/ui/field-policy?client=clt_acme"}, testPhotos())
	mustContain(t, body, "Delete all photos", "Delete 1 photo")
}
