package pages

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// FieldPolicy is the field policy page (/ui/field-policy): a client's
// effective policy (the platform defaults until it has its own), with the
// edit form. Reads make GET /api/field-policies/{clientId}'s check
// (CanReadFieldDevices); a save PUT's (CanManageFieldDevices), and the
// operation refuses an application-confined caller.
type FieldPolicy struct {
	Store   FieldStore
	Ops     FieldCommands
	Clients ClientStore
	// Photos is the platform-wide photo switch (the photo settings are not
	// drawn when photos are off platform-wide).
	Photos FieldPhotos
	// Attestation is the platform's unlock-key attestation state.
	Attestation FieldAttestation
	Shell       *shell.Shell
}

// FieldAttestation is what the policy page says about requiring an unlock
// key's attestation: whether trusted roots are configured and the attestation
// key status list's cache (nil when the revocation check is off).
type FieldAttestation struct {
	RootsConfigured bool
	Status          *attestation.StatusCache
}

// noRootsReason is why "Require a trusted hardware attestation" is disabled on
// a platform without attestation roots (the switch's tooltip and the note
// beside it).
const noRootsReason = "The platform has no trusted attestation roots (FC_FIELD_ATTESTATION_ROOTS), so a trusted attestation cannot be required."

// policyNotSaved is the banner over a refused save: the form shows what was
// submitted, and the banner says it is not what is stored.
const policyNotSaved = "The field policy was not saved: nothing changed. Fix what is marked below and save again."

// note is the page's sentence about the platform's attestation state ("" when
// there is nothing to say).
func (a FieldAttestation) note(now time.Time) (string, bool) {
	switch {
	case !a.RootsConfigured:
		return noRootsReason, false
	case a.Status == nil:
		return "The attestation key status list is switched off on this platform: Require means a trusted chain within its dates, without the revocation check.", false
	}
	if _, ok := a.Status.Usable(now); ok {
		return "", false
	}
	if last := a.Status.Age(); !last.IsZero() {
		return "The attestation key status list is unavailable (last fetched " + last.Format("02 Jan 2006 15:04 UTC") + "): while Require is on, no unlock key can be set up or used until it is fetched again; workers sign in with their PIN meanwhile.", true
	}
	return "The attestation key status list has not been fetched yet: while Require is on, no unlock key can be set up or used until it is; workers sign in with their PIN meanwhile.", true
}

const (
	fieldPolicyPath   = "/ui/field-policy"
	fieldPolicyRegion = "field-policy-region"
	fieldPolicyForm   = "field-policy-form"
	fieldPhotosForm   = "field-policy-photos-form"
	fieldPhotosDelDlg = "field-policy-photos-delete"
)

// Mount registers the page's routes.
func (h *FieldPolicy) Mount(r chi.Router) {
	r.Get("/field-policy", h.page)
	r.With(parseForm).Post("/field-policy", h.save)
	r.With(parseForm).Post("/field-policy/photos", h.savePhotos)
	r.With(parseForm).Post("/field-policy/photos/delete", h.deletePhotos)
}

// policyField is one number of the policy with the range the API enforces
// (fieldidentity.PutPolicyCommand.Build).
type policyField struct {
	Key, Label, Help string
	Min, Max         int
	Unit             string
	get              func(fieldidentity.Policy) int
}

var policyFields = []policyField{
	{Key: "pinMinLength", Label: "Minimum PIN length", Min: 4, Max: 12, Unit: "digits", Help: "4 to 12 (default 6). Below 6 the hard-lock threshold may be at most 5.", get: func(p fieldidentity.Policy) int { return p.PINMinLength }},
	{Key: "softLockThreshold", Label: "Soft lock after", Min: 1, Max: 20, Unit: "failed attempts", Help: "1 to 20 (default 5).", get: func(p fieldidentity.Policy) int { return p.SoftLockThreshold }},
	{Key: "softLockSecs", Label: "Soft lock lasts", Min: 30, Max: 86400, Unit: "seconds", Help: "30 to 86400 (default 900).", get: func(p fieldidentity.Policy) int { return p.SoftLockSecs }},
	{Key: "hardLockThreshold", Label: "Hard lock after", Min: 1, Max: 50, Unit: "failed attempts", Help: "From the soft-lock threshold to 50 (default 10). A hard lock needs a supervisor to unlock.", get: func(p fieldidentity.Policy) int { return p.HardLockThreshold }},
	{Key: "sharedIdleSecs", Label: "Shared device idle timeout", Min: 60, Max: 86400, Unit: "seconds", Help: "60 to 86400 (default 900).", get: func(p fieldidentity.Policy) int { return p.SharedIdleSecs }},
	{Key: "personalIdleSecs", Label: "Personal device idle timeout", Min: 60, Max: 86400, Unit: "seconds", Help: "60 to 86400 (default 7200).", get: func(p fieldidentity.Policy) int { return p.PersonalIdleSecs }},
	{Key: "shiftMaxSecs", Label: "Maximum shift length", Min: 3600, Max: 86400, Unit: "seconds", Help: "3600 to 86400 (default 50400). A session on a shared device is also capped at 12 hours.", get: func(p fieldidentity.Policy) int { return p.ShiftMaxSecs }},
	{Key: "pinReauthDays", Label: "PIN re-entry on a personal device", Min: 0, Max: 90, Unit: "days", Help: "0 to 90 (default 0, never). With a number, an unlock key signs a worker in only this many days after they last entered their PIN on the phone; then the PIN is needed once and the key works again. The PIN is always needed after a key is revoked or the PIN is reset.", get: func(p fieldidentity.Policy) int { return p.PINReauthDays }},
	{Key: "unenrolledIdleSecs", Label: "Idle timeout when the device is not enrolled", Min: 60, Max: 86400, Unit: "seconds", Help: "60 to 86400 (default 900). A session from a device that is not enrolled also ends after 12 hours.", get: func(p fieldidentity.Policy) int { return p.UnenrolledIdleSecs }},
	{Key: "impersonationMaxSecs", Label: "Longest session signed in as another worker", Min: 300, Max: 43200, Unit: "seconds", Help: "300 to 43200 (default 14400). Caps a supervisor's act-as session and a session started with a console sign-in code.", get: func(p fieldidentity.Policy) int { return p.ImpersonationMaxSecs }},
	{Key: "workerCodeDigits", Label: "Digits in a generated worker code", Min: fieldidentity.MinWorkerCodeDigits, Max: fieldidentity.MaxWorkerCodeDigits, Unit: "digits", Help: "3 to 6 (default 3). A worker created without a code gets three random letters and this many digits, for example KTM482. Changing it applies to codes generated afterwards; existing codes never change.", get: func(p fieldidentity.Policy) int { return p.WorkerCodeDigits }},
}

// policyFieldByKey is the numeric field with the key (a programming error when
// there is none).
func policyFieldByKey(key string) policyField {
	for _, f := range policyFields {
		if f.Key == key {
			return f
		}
	}
	panic("pages: no policy field " + key)
}

// The policy's switches. requireAttestation is a device rule; the two sign-in
// switches and their two numbers are the sign-in options of
// docs/field-worker-sign-in-options-spec.md. A switch posts nothing when it
// is off, so every field the page shows must be posted back for a save to
// keep it: the save builds the whole PUT body from the form.
const (
	policyRequireEnrolled = "requireEnrolledDevice"
	policyBadgeOnly       = "badgeOnlyLogin"
	// policyUnlockKeys and policyUnlockAttest are the unlock-key switches
	// (docs/field-unlock-keys-spec.md section 7); the second posts REQUIRED.
	policyUnlockKeys   = "unlockKeysEnabled"
	policyUnlockAttest = "unlockKeyAttestation"
)

type fieldPolicyView struct {
	Client        string
	ClientName    string
	ClientOptions []kit.Option
	Policy        fieldidentity.Policy
	CanManage     bool
	// Values are the form's inputs as shown (the saved ones, or what was
	// submitted when a save was refused).
	Values        map[string]string
	Attestation   bool
	RequireEnroll bool
	BadgeOnly     bool
	// UnlockKeys and UnlockAttest are the unlock-key switches (attestation
	// REQUIRED when on); AttestNote is the platform's attestation state in a
	// sentence, AttestWarn whether it is a warning. AttestBlocked is set when
	// the platform has no attestation roots: the attestation switch is then
	// disabled with AttestNote as its reason, and a save keeps the stored rule.
	UnlockKeys, UnlockAttest bool
	AttestNote               string
	AttestWarn               bool
	AttestBlocked            bool
	// PhotosShown is true when photos are on platform-wide (the photo settings
	// are drawn); PhotosOn and OwnPhoto are the client's two settings as saved.
	PhotosShown, PhotosOn, OwnPhoto bool
	// CanDeletePhotos is the delete route's check: worker manage for the client
	// and not an application-confined caller.
	CanDeletePhotos bool
	// PhotoCount is how many photos the client's workers hold (the question
	// before "Delete all photos" names it).
	PhotoCount int
	Errs       formErrors
	UpdatedAt  time.Time
	Status     int
}

func newPolicyView(p fieldidentity.Policy) fieldPolicyView {
	v := fieldPolicyView{
		Policy: p, Values: map[string]string{}, Attestation: p.RequireAttestation, UpdatedAt: p.UpdatedAt,
		RequireEnroll: p.RequireEnrolledDevice, BadgeOnly: p.BadgeOnlyLogin,
		UnlockKeys: p.UnlockKeysEnabled, UnlockAttest: p.UnlockKeyAttestation == fieldidentity.UnlockAttestationRequired,
	}
	for _, f := range policyFields {
		v.Values[f.Key] = strconv.Itoa(f.get(p))
	}
	return v
}

func (v fieldPolicyView) clientSelect() kit.SearchSelectProps {
	return clientSearchSelect("fp-client", "fpClientSearch", fieldPolicyPath, v.ClientOptions, v.Client)
}

func (h *FieldPolicy) read(ctx context.Context, ac *auth.AuthContext, requested string) (fieldPolicyView, error) {
	opts, err := portalClientOptions(ctx, h.Clients, ac)
	if err != nil {
		return fieldPolicyView{}, err
	}
	clientID, opts := portalSelectedClient(ac, opts, requested)
	v := fieldPolicyView{Client: clientID, ClientOptions: opts, ClientName: portalClientName(opts, clientID)}
	if clientID == "" {
		return v, nil
	}
	if err := auth.CanReadFieldDevices(ac, clientID); err != nil {
		return fieldPolicyView{}, err
	}
	p, err := h.Store.Policy(ctx, clientID)
	if err != nil {
		return fieldPolicyView{}, usecase.Internal("REPO", "find_field_policy failed", err)
	}
	nv := newPolicyView(p)
	nv.PhotosShown, nv.PhotosOn, nv.OwnPhoto = h.Photos.on(), p.PhotosEnabled, p.WorkerMaySetOwnPhoto
	nv.CanDeletePhotos = auth.CanManageFieldWorkers(ac, clientID) == nil && !ac.IsApplicationScoped()
	if nv.PhotosShown {
		if nv.PhotoCount, err = h.Store.PhotoCount(ctx, clientID); err != nil {
			return fieldPolicyView{}, usecase.Internal("REPO", "count_field_worker_photos failed", err)
		}
	}
	nv.Client, nv.ClientOptions, nv.ClientName = clientID, opts, v.ClientName
	nv.CanManage = auth.CanManageFieldDevices(ac, clientID) == nil && !ac.IsApplicationScoped()
	nv.AttestNote, nv.AttestWarn = h.Attestation.note(time.Now().UTC())
	nv.AttestBlocked = !h.Attestation.RootsConfigured
	return nv, nil
}

func (h *FieldPolicy) render(w http.ResponseWriter, r *http.Request, status int, v fieldPolicyView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Field Policy", Path: fieldPolicyPath}, fieldPolicyPage(v))
}

func (h *FieldPolicy) page(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	v, err := h.read(ctx, ac, r.URL.Query().Get(fieldClientParam))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.render(w, r, http.StatusOK, v)
}

// policyFieldCodes maps the operation's field name (the error detail) to the
// form's input.
func policyErrorField(err error) string {
	if ue := usecase.AsError(err); ue != nil && ue.Code == "POLICY_INVALID" {
		if f, ok := ue.Details["field"].(string); ok {
			return f
		}
	}
	return ""
}

// policyMessage restates the operation's validation message with the form's
// labels in place of the API's field names ("softLockSecs must be between 30
// and 86400" becomes "Soft lock lasts must be between 30 and 86400"). The
// API's error is unchanged; this is the UI layer only.
func policyMessage(field, msg string) string {
	label := func(f policyField) string { return f.Label }
	if field == policyUnlockAttest {
		return "A trusted attestation cannot be required: the platform has no attestation roots configured (FC_FIELD_ATTESTATION_ROOTS)."
	}
	rest := strings.TrimPrefix(msg, field)
	if rest == msg {
		return sentence(msg)
	}
	for _, f := range policyFields {
		l := label(f)
		rest = strings.ReplaceAll(rest, f.Key, strings.ToLower(l[:1])+l[1:])
	}
	for _, f := range policyFields {
		if f.Key == field {
			return label(f) + rest
		}
	}
	return sentence(msg)
}

func (h *FieldPolicy) save(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	clientID := strings.TrimSpace(r.PostFormValue(fieldClientParam))
	if clientID == "" {
		h.Shell.Fail(w, r, usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required"))
		return
	}
	// PUT /api/field-policies/{clientId}'s check, before anything is read.
	if err := auth.CanManageFieldDevices(ac, clientID); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// The policy is client-wide: an application-confined caller may not change
	// it (the operation refuses it too).
	if ac.IsApplicationScoped() {
		h.Shell.Fail(w, r, usecase.Authorization("APPLICATION_FORBIDDEN", "an application-confined caller cannot change the client's field policy"))
		return
	}
	cur, err := h.read(ctx, ac, clientID)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v := cur
	v.Attestation = r.PostFormValue("requireAttestation") != ""
	v.RequireEnroll = r.PostFormValue(policyRequireEnrolled) != ""
	v.BadgeOnly = r.PostFormValue(policyBadgeOnly) != ""
	v.UnlockKeys = r.PostFormValue(policyUnlockKeys) != ""
	if !v.AttestBlocked {
		v.UnlockAttest = r.PostFormValue(policyUnlockAttest) != ""
	} // else the disabled switch keeps the stored rule (a forged post cannot change it)
	attest := fieldidentity.UnlockAttestationNone
	if v.UnlockAttest {
		attest = fieldidentity.UnlockAttestationRequired
	}
	cmd := fieldidentity.PutPolicyCommand{
		ClientID: clientID, RequireAttestation: v.Attestation,
		RequireEnrolledDevice: v.RequireEnroll, BadgeOnlyLogin: v.BadgeOnly,
		UnlockKeysEnabled: v.UnlockKeys, UnlockKeyAttestation: attest,
		// The page has no photo controls yet (docs/field-worker-photo-spec.md
		// section 9): the save keeps the two photo settings as they are, and
		// so never deletes photos.
		PhotosEnabled: &cur.Policy.PhotosEnabled, WorkerMaySetOwnPhoto: cur.Policy.WorkerMaySetOwnPhoto,
	}
	slots := make(map[string]*int, len(policyFields))
	for _, f := range policyFields {
		raw := strings.TrimSpace(r.PostFormValue(f.Key))
		v.Values[f.Key] = raw
		if raw == "" {
			continue // blank takes the default, as the API's omitted field
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			v.Errs.field(f.Key, "Enter a whole number.")
			continue
		}
		slots[f.Key] = &n
	}
	cmd.PINMinLength, cmd.SoftLockThreshold, cmd.SoftLockSecs = slots["pinMinLength"], slots["softLockThreshold"], slots["softLockSecs"]
	cmd.HardLockThreshold, cmd.SharedIdleSecs, cmd.PersonalIdleSecs = slots["hardLockThreshold"], slots["sharedIdleSecs"], slots["personalIdleSecs"]
	cmd.ShiftMaxSecs, cmd.PINReauthDays = slots["shiftMaxSecs"], slots["pinReauthDays"]
	cmd.UnenrolledIdleSecs, cmd.ImpersonationMaxSecs = slots["unenrolledIdleSecs"], slots["impersonationMaxSecs"]
	cmd.WorkerCodeDigits = slots["workerCodeDigits"]
	if !v.Errs.any() {
		if err := h.Ops.PutPolicy(ctx, cmd); err != nil {
			if f := policyErrorField(err); f != "" {
				if ue := usecase.AsError(err); ue != nil {
					v.Errs.field(f, policyMessage(f, ue.Message))
				}
			} else {
				v.Errs.apply(r, err, nil)
			}
		}
	}
	if v.Errs.any() {
		if v.Errs.Status == 0 {
			v.Errs.Status = http.StatusUnprocessableEntity
		}
		// A refused save must never look like a saved one: the form shows what
		// was submitted, so the banner over it says nothing was stored.
		if v.Errs.Form == "" {
			v.Errs.Form = policyNotSaved
		} else {
			v.Errs.Form = "The field policy was not saved: " + v.Errs.Form
		}
		if ds.IsDatastar(r) {
			sse := ds.NewSSE(w, r)
			if err := sse.Patch(fieldPolicyBody(v)); err != nil {
				slog.DebugContext(ctx, "ui: field policy patch not delivered", "err", err)
			}
			return
		}
		h.render(w, r, v.Errs.Status, v)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashFieldPolicySaved)
		http.Redirect(w, r, fieldPolicyPath+"?"+fieldClientParam+"="+url.QueryEscape(clientID), http.StatusSeeOther) //nolint:gosec // G710: the target is a page constant; the client id is query-escaped and was checked by the manage check
		return
	}
	saved, err := h.read(ctx, ac, clientID)
	sse := ds.NewSSE(w, r)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := sse.Patch(fieldPolicyBody(saved)); err != nil {
		slog.DebugContext(ctx, "ui: field policy patch not delivered", "err", err)
	}
	if err := portalFlash(sse, flashFieldPolicySaved, "Field policy saved for "+saved.ClientName); err != nil {
		slog.DebugContext(ctx, "ui: field policy flash not delivered", "err", err)
	}
}

// putPolicyCommandOf is the stored policy as a PUT: every setting as it is, so
// that changing the photo settings alone changes nothing else.
func putPolicyCommandOf(p fieldidentity.Policy) fieldidentity.PutPolicyCommand {
	return fieldidentity.PutPolicyCommand{
		ClientID: p.ClientID, PINMinLength: &p.PINMinLength, SoftLockThreshold: &p.SoftLockThreshold,
		SoftLockSecs: &p.SoftLockSecs, HardLockThreshold: &p.HardLockThreshold, SharedIdleSecs: &p.SharedIdleSecs,
		PersonalIdleSecs: &p.PersonalIdleSecs, ShiftMaxSecs: &p.ShiftMaxSecs, RequireAttestation: p.RequireAttestation,
		PINReauthDays: &p.PINReauthDays, RequireEnrolledDevice: p.RequireEnrolledDevice, UnenrolledIdleSecs: &p.UnenrolledIdleSecs,
		BadgeOnlyLogin: p.BadgeOnlyLogin, ImpersonationMaxSecs: &p.ImpersonationMaxSecs, WorkerCodeDigits: &p.WorkerCodeDigits,
		PhotosEnabled: &p.PhotosEnabled, WorkerMaySetOwnPhoto: p.WorkerMaySetOwnPhoto,
		UnlockKeysEnabled: p.UnlockKeysEnabled, UnlockKeyAttestation: p.UnlockKeyAttestation,
	}
}

// savePhotos mirrors the photo part of PUT /api/field-policies/{clientId}: the
// stored policy with the two photo settings replaced. Switching photos off is
// just the setting: it hides the photos and keeps them.
func (h *FieldPolicy) savePhotos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	clientID, ok := h.photoClient(w, r, ac, auth.CanManageFieldDevices)
	if !ok {
		return
	}
	cur, err := h.read(ctx, ac, clientID)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	on, own := r.PostFormValue("photosEnabled") != "", r.PostFormValue("workerMaySetOwnPhoto") != ""
	cmd := putPolicyCommandOf(cur.Policy)
	cmd.PhotosEnabled, cmd.WorkerMaySetOwnPhoto = &on, own
	if err := h.Ops.PutPolicy(ctx, cmd); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.photosDone(w, r, ac, clientID, flashFieldPhotosSaved, "Photo settings saved for ")
}

// deletePhotos mirrors DELETE /api/field-policies/{clientId}/photos: every
// stored photo of the client's workers, whatever the setting says. The page
// asked first (a confirmation naming the count); the operation refuses an
// application-confined caller.
func (h *FieldPolicy) deletePhotos(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	clientID, ok := h.photoClient(w, r, ac, auth.CanManageFieldWorkers)
	if !ok {
		return
	}
	n, err := h.Ops.DeleteClientPhotos(ctx, fieldidentity.DeleteClientPhotosCommand{ClientID: clientID})
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	detail := "Deleted " + strconv.Itoa(n) + " photos for "
	if n == 1 {
		detail = "Deleted 1 photo for "
	}
	h.photosDone(w, r, ac, clientID, flashFieldPhotosDeleted, detail)
}

// photoClient is the client a photo post names, after the check the API's
// route makes; ok is false when the answer has been written.
func (h *FieldPolicy) photoClient(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, can func(*auth.AuthContext, string) error) (string, bool) {
	clientID := strings.TrimSpace(r.PostFormValue(fieldClientParam))
	if clientID == "" {
		h.Shell.Fail(w, r, usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required"))
		return "", false
	}
	if err := can(ac, clientID); err != nil {
		h.Shell.Fail(w, r, err)
		return "", false
	}
	if ac.IsApplicationScoped() {
		h.Shell.Fail(w, r, usecase.Authorization("APPLICATION_FORBIDDEN", "an application-confined caller cannot change the client's field policy or delete its photos"))
		return "", false
	}
	if !h.Photos.on() {
		h.Shell.Fail(w, r, photosOffError())
		return "", false
	}
	return clientID, true
}

// photosDone answers a photo change: the page again (native) or the policy
// body patched with a flash (Datastar).
func (h *FieldPolicy) photosDone(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, clientID, flash, detail string) {
	ctx := r.Context()
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flash)
		http.Redirect(w, r, fieldPolicyPath+"?"+fieldClientParam+"="+url.QueryEscape(clientID), http.StatusSeeOther) //nolint:gosec // G710: the target is a page constant; the client id is query-escaped and was checked by the manage check
		return
	}
	saved, err := h.read(ctx, ac, clientID)
	sse := ds.NewSSE(w, r)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := sse.Patch(fieldPolicyBody(saved)); err != nil {
		slog.DebugContext(ctx, "ui: field policy patch not delivered", "err", err)
	}
	if err := portalFlash(sse, flash, detail+saved.ClientName); err != nil {
		slog.DebugContext(ctx, "ui: field policy flash not delivered", "err", err)
	}
}
