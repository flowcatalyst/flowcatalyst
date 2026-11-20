package pages

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/imageproc"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Field-worker photos on the console (docs/field-worker-photo-spec.md
// section 9): the worker's photo in the drawer and the list, "Add photo" /
// "Change photo" and "Remove photo" for a manager, and the client's two
// settings on the field policy page. The rules are the admin API's: the same
// image package decides what is accepted and the same use cases write.

// FieldPhotos is the photo processor and the platform-wide switch
// (FC_FIELD_PHOTOS) the field pages work with. With Off set, or no Processor,
// the pages show no photo, no avatar and no photo control.
type FieldPhotos struct {
	Processor *imageproc.Processor
	Off       bool
}

func (p FieldPhotos) on() bool { return !p.Off && p.Processor != nil }

// photoPanel is how the drawer's Photo section is drawn.
type photoPanel string

const (
	// photoPanelShown: the photo (or its absence) with the controls.
	photoPanelShown photoPanel = "shown"
	// photoPanelClientOff: the client has switched photos off; no controls.
	photoPanelClientOff photoPanel = "client-off"
	// photoPanelNone: the platform has them off; the section is not drawn.
	photoPanelNone photoPanel = "none"
)

const (
	fwPhotoDialog   = "fw-photo-form"
	fwPhotoSignal   = "fwPhotoForm"
	fwPhotoRemoveID = "fw-photo-remove"
	fwPhotoField    = "photo"
	// photoUploadSlack is the multipart framing allowed on top of the limit.
	photoUploadSlack = 64 << 10
)

// photoSrc is the thumbnail's address: the admin API's route, with the version
// so a changed photo is a new address.
func photoSrc(id, version string) string {
	return "/api/field-workers/" + url.PathEscape(id) + "/photo?size=thumb&v=" + url.QueryEscape(version)
}

// initialsOf is the avatar's letters: the first letters of up to two words of
// the name, else the worker code's first letter.
func initialsOf(name, code string) string {
	var out []rune
	for w := range strings.FieldsSeq(name) {
		if r, _ := utf8.DecodeRuneInString(w); unicode.IsLetter(r) || unicode.IsDigit(r) {
			out = append(out, unicode.ToUpper(r))
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		if r, _ := utf8.DecodeRuneInString(code); r != utf8.RuneError {
			out = append(out, unicode.ToUpper(r))
		}
	}
	return string(out)
}

// fieldPhotoView is the drawer's Photo section.
type fieldPhotoView struct {
	Panel    photoPanel
	Src      string // the thumbnail; empty when the worker has none
	Initials string
	// Add is true when there is no photo yet (the button says "Add photo").
	Add          bool
	ChangeHref   string // set for a manager on a shown panel
	RemoveAction string // set for a manager when there is a photo
	PolicyHref   string // the client's field policy, named when the client has photos off
	Name         string
}

// photoViewOf builds the drawer's Photo section for a loaded worker.
func (h *FieldWorkers) photoViewOf(ctx context.Context, i *fieldidentity.Identity, base string, canManage bool) (fieldPhotoView, error) {
	v := fieldPhotoView{Panel: photoPanelNone, Initials: initialsOf(i.Name, i.WorkerCode), Name: i.Name}
	if !h.Photos.on() {
		return v, nil
	}
	pol, err := h.Store.Policy(ctx, i.ClientID)
	if err != nil {
		return v, usecase.Internal("REPO", "find_field_policy failed", err)
	}
	if !pol.PhotosEnabled {
		v.Panel = photoPanelClientOff
		v.PolicyHref = fieldPolicyPath + "?" + fieldClientParam + "=" + url.QueryEscape(i.ClientID)
		return v, nil
	}
	v.Panel = photoPanelShown
	v.Add = i.PhotoVersion == ""
	if !v.Add {
		v.Src = photoSrc(i.ID, i.PhotoVersion)
	}
	if canManage {
		v.ChangeHref = base + "/photo"
		if !v.Add {
			v.RemoveAction = base + "/photo/remove"
		}
	}
	return v, nil
}

func (v fieldPhotoView) changeLabel() string {
	if v.Add {
		return "Add photo"
	}
	return "Change photo"
}

// ── the upload dialog ──────────────────────────────────────────────────

type fieldPhotoForm struct {
	WorkerID, Code string
	MaxBytes       int64
	Errs           formErrors
	Open           bool
}

func (f fieldPhotoForm) id(key string) string { return fieldID(fwPhotoDialog, key) }

func (f fieldPhotoForm) describedBy() string {
	if f.Errs.Fields[fwPhotoField] != "" {
		return f.id(fwPhotoField) + "-err"
	}
	return ""
}

func (f fieldPhotoForm) fields() []templ.Component { return []templ.Component{fieldPhotoField(f)} }

func (f fieldPhotoForm) firstInvalid() string {
	if f.Errs.Fields[fwPhotoField] != "" {
		return f.id(fwPhotoField)
	}
	return ""
}

func (f fieldPhotoForm) action() string {
	return fieldWorkersPath + "/" + url.PathEscape(f.WorkerID) + "/photo"
}

// maxMB is the byte limit in megabytes, for the dialog's words.
func (f fieldPhotoForm) maxMB() string {
	return strconv.FormatFloat(float64(f.MaxBytes)/(1<<20), 'f', -1, 64)
}

// photoFieldMessage is the file field's message for a refusal of the upload:
// the shared refusal's words, as one sentence of how to fix it.
func photoFieldMessage(err error, maxBytes int64) (string, bool) {
	if r, ok := fieldidentity.PhotoRefusalOf(err, maxBytes); ok {
		return r.Message, true
	}
	return "", false
}

func (h *FieldWorkers) photoForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := fieldWorkerID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.loadWorker(ctx, ac, id, true)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !h.Photos.on() {
		h.Shell.Fail(w, r, photosOffError())
		return
	}
	f := fieldPhotoForm{WorkerID: i.ID, Code: i.WorkerCode, MaxBytes: h.Photos.Processor.Limits().MaxBytes, Open: true}
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), fwDialogSlot, fieldPhotoDialog(f)); err != nil {
			slog.DebugContext(ctx, "ui: field photo form patch not delivered", "err", err)
		}
		return
	}
	v, err := h.view(ctx, ac, parseListQuery(nil, fieldWorkerList).withFacet(fieldClientParam, []string{i.ClientID}))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v.Dialog = fieldPhotoDialog(f)
	h.renderPage(w, r, http.StatusOK, v)
}

func photosOffError() error {
	return usecase.NotFound(fieldidentity.CodePhotosDisabled, "field-worker photos are switched off on this platform")
}

// parsePhotoUpload reads a photo post's multipart form: the body is capped at
// the photo limit plus framing (the generic form reader's 1 MiB is too small
// for a photograph), then handed to the handler with r.MultipartForm read.
func (h *FieldWorkers) parsePhotoUpload(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		max := imageproc.DefaultLimits().MaxBytes
		if h.Photos.Processor != nil {
			max = h.Photos.Processor.Limits().MaxBytes
		}
		r.Body = http.MaxBytesReader(w, r.Body, max+photoUploadSlack)
		// Parts beyond 1 MiB go to temporary files, never held in memory.
		if err := r.ParseMultipartForm(maxFormBytes); err != nil { //nolint:gosec // G120: the body is capped by the MaxBytesReader above
			slog.InfoContext(r.Context(), "ui: photo form not read", "path", r.URL.Path, "err", err)
			if ds.IsDatastar(r) {
				_ = shell.Flash(ds.NewSSE(w, r), kit.FlashMessageProps{Kind: kit.FlashError, Summary: "The photo was not received.", Detail: "The file may be larger than " + strconv.FormatInt(max>>20, 10) + " MB. Choose a smaller image and try again."})
				return
			}
			http.Error(w, "The photo was not received. Choose a smaller image and try again.", http.StatusRequestEntityTooLarge)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// photoUpload mirrors PUT /api/field-workers/{id}/photo: the same manage
// check, the same image processing and limits, the same use case.
func (h *FieldWorkers) photoUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := fieldWorkerID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.loadWorker(ctx, ac, id, true)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !h.Photos.on() {
		h.Shell.Fail(w, r, photosOffError())
		return
	}
	lim := h.Photos.Processor.Limits()
	f := fieldPhotoForm{WorkerID: i.ID, Code: i.WorkerCode, MaxBytes: lim.MaxBytes}
	if err := h.storePhoto(r, i, lim, &f); err != nil || f.Errs.any() {
		h.photoFailed(w, r, ac, i, f, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashFieldWorkerPhotoSet)
		http.Redirect(w, r, fieldWorkersPath+"?"+fieldClientParam+"="+url.QueryEscape(i.ClientID), http.StatusSeeOther) //nolint:gosec // G710: the target is a page constant; the client id is query-escaped and was checked by the manage check
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(fwPhotoDialog); err != nil {
		slog.DebugContext(ctx, "ui: field photo form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, i.ClientID, id, false)
	if err := portalFlash(sse, flashFieldWorkerPhotoSet, "Photo saved for "+i.WorkerCode); err != nil {
		slog.DebugContext(ctx, "ui: field worker flash not delivered", "err", err)
	}
}

// storePhoto reads the posted file, processes it and runs the use case. A
// refusal of the upload goes to the file field (f.Errs); the returned error is
// what to show when it is not one.
func (h *FieldWorkers) storePhoto(r *http.Request, i *fieldidentity.Identity, lim imageproc.Limits, f *fieldPhotoForm) error {
	ctx := r.Context()
	file, hdr, err := r.FormFile(fwPhotoField)
	if err != nil {
		f.Errs.field(fwPhotoField, "Choose an image file.")
		return nil
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, lim.MaxBytes+1))
	if err != nil {
		f.Errs.field(fwPhotoField, "The file could not be read. Choose it again.")
		return nil
	}
	pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	photo, err := h.Photos.Processor.Process(pctx, data, hdr.Header.Get("Content-Type"))
	if err != nil {
		if msg, ok := photoFieldMessage(err, lim.MaxBytes); ok {
			f.Errs.field(fwPhotoField, msg)
			return nil
		}
		return usecase.Internal("PHOTO_PROCESS", "processing the photo failed", err)
	}
	ac := auth.FromContext(ctx)
	return h.Ops.SetWorkerPhoto(ctx, fieldidentity.NewSetPhotoCommand(refOfWorker(i), fieldidentity.PhotoActorOf(ac), ac.PrincipalID, photo))
}

// photoFailed answers a refused upload: the file field's message (or, for any
// other error, the form's) in the open dialog.
func (h *FieldWorkers) photoFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, i *fieldidentity.Identity, f fieldPhotoForm, cmdErr error) {
	if cmdErr != nil {
		f.Errs.apply(r, cmdErr, nil)
	}
	if !f.Errs.any() {
		f.Errs.field(fwPhotoField, "Choose an image file.")
	}
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, fwPhotoDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	f.Open = true
	v, err := h.view(r.Context(), ac, parseListQuery(nil, fieldWorkerList).withFacet(fieldClientParam, []string{i.ClientID}))
	if err != nil {
		v = fieldWorkersView{}
	}
	v.Dialog = fieldPhotoDialog(f)
	h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), v)
}

// photoRemove mirrors DELETE /api/field-workers/{id}/photo. It is a question,
// not a delete of the record: the confirmation is in the ordinary tone.
func (h *FieldWorkers) photoRemove(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashFieldWorkerPhotoGone, func(ctx context.Context, i *fieldidentity.Identity) (string, error) {
		if !h.Photos.on() {
			return "", photosOffError()
		}
		ac := auth.FromContext(ctx)
		err := h.Ops.RemoveWorkerPhoto(ctx, fieldidentity.RemovePhotoCommand{
			WorkerRef: refOfWorker(i), Actor: fieldidentity.PhotoActorOf(ac), ActorID: ac.PrincipalID,
		})
		return "Photo removed for " + i.WorkerCode, err
	})
}

// photoCountNoun is "1 photo" or "3 photos".
func photoCountNoun(n int) string {
	if n == 1 {
		return "1 photo"
	}
	return strconv.Itoa(n) + " photos"
}
