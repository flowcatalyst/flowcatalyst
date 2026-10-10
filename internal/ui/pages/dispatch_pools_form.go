package pages

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// The create and edit dialogs (kit form-dialog.md), mirroring the SPA's
// DispatchPoolCreateDrawer and the drawer's edit form.

const (
	poolFormID     = "pool-form"
	poolFormSignal = "poolForm"
)

// poolFieldCodes maps the use cases' validation codes to the form field
// that caused them; anything else is a form-level error.
var poolFieldCodes = map[string]string{
	"CODE_REQUIRED":       "code",
	"INVALID_CODE_FORMAT": "code",
	"CODE_EXISTS":         "code",
	"NAME_REQUIRED":       "name",
	"INVALID_CONCURRENCY": "concurrency",
	"INVALID_RATE_LIMIT":  "rateLimit",
}

// poolForm is the create or edit form: what is rendered, or what was
// submitted with its errors.
type poolForm struct {
	Edit     bool
	PoolID   string
	Subtitle string

	Code, Name, Description string
	RateLimit, Concurrency  string
	Anchor                  bool
	Client                  string

	Errs formErrors
	Open bool

	CanPickClient bool
	ClientOptions []kit.Option
}

func (f poolForm) id(key string) string { return fieldID(poolFormID, key) }

func (f poolForm) fieldError(key string) string { return f.Errs.Fields[key] }

func (f poolForm) action() string {
	if f.Edit {
		return poolURL(f.PoolID) + "/edit"
	}
	return poolsPath + "/new"
}

// keys are the form's fields in display order (focus goes to the first
// invalid one).
func (f poolForm) keys() []string {
	if f.Edit {
		return []string{"name", "description", "rateLimit", "concurrency"}
	}
	return []string{"code", "name", "description", "rateLimit", "concurrency", "client"}
}

func (f poolForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.fieldError(k) != "" {
			return f.id(k)
		}
	}
	return ""
}

// fields are the patchable fields (each root has id "{control}-field").
func (f poolForm) fields() []templ.Component {
	out := make([]templ.Component, 0, 6)
	for _, k := range f.keys() {
		out = append(out, poolField(f, k))
	}
	return out
}

func (f poolForm) dialog() kit.FormDialogProps {
	p := kit.FormDialogProps{
		ID:          poolFormID,
		Signal:      poolFormSignal,
		Width:       kit.WidthWide,
		Action:      f.action(),
		Open:        f.Open,
		Error:       f.Errs.Form,
		Title:       "Create Dispatch Pool",
		Description: "Configure a new pool for dispatch jobs",
		SubmitLabel: "Create Pool",
	}
	if f.Edit {
		p.Title, p.Description, p.SubmitLabel, p.RequireChange = "Edit Dispatch Pool", f.Subtitle, "Save", true
		// Which pool the drawer shows, so a save refreshes it there too.
		p.Hidden = []kit.HiddenField{drawerField(poolDrawerSig)}
	}
	return p
}

// drawerField is the hidden field that tells an action which record the
// drawer shows (the drawer's signal), so the answer can refresh or close it.
func drawerField(signal string) kit.HiddenField {
	return kit.HiddenField{Name: "drawer", Attrs: templ.Attributes{"data-attr:value": "$" + kit.Signal(signal)}}
}

func editForm(p *dispatchpool.DispatchPool) poolForm {
	f := poolForm{Edit: true, PoolID: p.ID, Subtitle: p.Name + " (" + p.Code + ")", Name: p.Name, Description: deref(p.Description), Concurrency: strconv.Itoa(int(p.Concurrency))}
	if p.RateLimit != nil {
		f.RateLimit = strconv.Itoa(int(*p.RateLimit))
	}
	return f
}

func submittedForm(r *http.Request, edit bool) poolForm {
	return poolForm{
		Edit:        edit,
		Code:        r.PostFormValue("code"),
		Name:        r.PostFormValue("name"),
		Description: r.PostFormValue("description"),
		RateLimit:   r.PostFormValue("rateLimit"),
		Concurrency: r.PostFormValue("concurrency"),
		Anchor:      r.PostFormValue("anchor") != "",
		Client:      strings.TrimSpace(r.PostFormValue("client")),
	}
}

// numbers parses the rate limit and concurrency fields, recording field
// errors for anything that is not a whole number.
func (f *poolForm) numbers() (rateLimit, concurrency *int32) {
	rateLimit, ok := optionalInt32(f.RateLimit)
	if !ok {
		f.Errs.field("rateLimit", notWholeNumber)
	}
	concurrency, ok = optionalInt32(f.Concurrency)
	if !ok {
		f.Errs.field("concurrency", notWholeNumber)
	}
	return rateLimit, concurrency
}

// ── create ─────────────────────────────────────────────────────────────

// createForm opens the create dialog (the SPA's Create Pool).
func (h *DispatchPools) createForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	// POST /api/dispatch-pools's check.
	if err := auth.CanCreateDispatchPools(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := poolForm{Concurrency: "10", Open: true}
	h.withClients(r, ac, &f)
	h.showForm(w, r, ac, f)
}

func (h *DispatchPools) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedForm(r, false)
	if err := auth.CanCreateDispatchPools(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	rateLimit, concurrency := f.numbers()
	cmd := operations.CreateCommand{
		Code:        f.Code,
		Name:        f.Name,
		Description: optionalText(f.Description),
		RateLimit:   rateLimit,
		Concurrency: concurrency,
	}
	// The SPA sends no client for an anchor-level pool (the checkbox), and
	// none when the client field is left empty.
	if !f.Anchor {
		id, msg, err := resolveClient(ctx, h.Clients, ac, f.Client)
		switch {
		case err != nil:
			f.Errs.apply(r, err, nil)
		case msg != "":
			f.Errs.field("client", msg)
		}
		cmd.ClientID = id
	}
	if f.Errs.any() {
		h.formFailed(w, r, ac, f)
		return
	}
	newID, err := h.Ops.Create(ctx, cmd)
	if err != nil {
		f.Errs.apply(r, err, poolFieldCodes)
		// The use case refuses a client the caller cannot access: that is
		// the client field's error (without a client it is form-level:
		// an anchor-level pool needs anchor scope).
		if ue, ok := errors.AsType[*usecase.Error](err); ok && ue.Code == "SCOPE_FORBIDDEN" && cmd.ClientID != nil {
			f.Errs.Form = ""
			f.Errs.field("client", sentence(ue.Message))
		}
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashPoolCreated)
		http.Redirect(w, r, poolsPath, http.StatusSeeOther)
		return
	}
	// As the SPA: close the form and open the new pool's detail drawer.
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(poolFormID); err != nil {
		slog.DebugContext(ctx, "ui: pool form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, newID, false)
	if p, err := h.loadPool(ctx, ac, newID); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := openDrawer(sse, toPoolDetail(p, poolPermsFor(ac))); err != nil {
		slog.DebugContext(ctx, "ui: pool drawer patch not delivered", "err", err)
	}
	if err := flashKey(sse, flashPoolCreated); err != nil {
		slog.DebugContext(ctx, "ui: pool flash not delivered", "err", err)
	}
}

// ── edit ───────────────────────────────────────────────────────────────

// editForm opens the edit dialog for a pool (the row's Edit, the drawer's
// Edit). Archived pools are read-only, as in the SPA.
func (h *DispatchPools) editForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// The SPA loads the pool (GET, read permission) and saves it with PUT
	// (write permission).
	if err := firstError(auth.CanReadDispatchPools(ac), auth.CanUpdateDispatchPools(ac)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := poolID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	p, err := h.loadPool(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if p.Status == dispatchpool.StatusArchived {
		h.Shell.Fail(w, r, usecase.Conflict("POOL_ARCHIVED", archivedReadOnly))
		return
	}
	f := editForm(p)
	f.Open = true
	h.showForm(w, r, ac, f)
}

func (h *DispatchPools) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedForm(r, true)
	if err := firstError(auth.CanReadDispatchPools(ac), auth.CanUpdateDispatchPools(ac)); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	id, err := poolID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	p, err := h.loadPool(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.PoolID, f.Subtitle = p.ID, p.Name+" ("+p.Code+")"
	if p.Status == dispatchpool.StatusArchived {
		f.Errs.apply(r, usecase.Conflict("POOL_ARCHIVED", archivedReadOnly), nil)
		h.formFailed(w, r, ac, f)
		return
	}
	rateLimit, concurrency := f.numbers()
	if f.Errs.any() {
		h.formFailed(w, r, ac, f)
		return
	}
	// The SPA's PUT body: the name always; description, rate limit and
	// concurrency only when set (`value || undefined`), so a blank field
	// keeps the stored value (the use case has no way to clear them).
	name := f.Name
	err = h.Ops.Update(ctx, operations.UpdateCommand{
		ID:          id,
		Name:        &name,
		Description: optionalText(f.Description),
		RateLimit:   positive(rateLimit),
		Concurrency: positive(concurrency),
	})
	if err != nil {
		f.Errs.apply(r, err, poolFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashPoolUpdated)
		http.Redirect(w, r, poolsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(poolFormID); err != nil {
		slog.DebugContext(ctx, "ui: pool form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, id, false)
	if err := flashKey(sse, flashPoolUpdated); err != nil {
		slog.DebugContext(ctx, "ui: pool flash not delivered", "err", err)
	}
}

// positive is the SPA's `n || undefined` for a number: 0 is not sent.
func positive(n *int32) *int32 {
	if n == nil || *n == 0 {
		return nil
	}
	return n
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// ── rendering ──────────────────────────────────────────────────────────

func (h *DispatchPools) withClients(r *http.Request, ac *auth.AuthContext, f *poolForm) {
	if f.Edit {
		return
	}
	f.CanPickClient = canPickClient(h.Clients, ac)
	opts, err := clientSuggestions(r.Context(), h.Clients, ac, f.Client)
	if err != nil {
		// The form still works without suggestions (a typed identifier is
		// resolved on submit).
		slog.WarnContext(r.Context(), "ui: client suggestions unavailable", "err", err)
		return
	}
	f.ClientOptions = opts
}

// showForm answers a request for a form dialog: patched into the page's
// dialog slot for Datastar, or the list page with the dialog open.
func (h *DispatchPools) showForm(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f poolForm) {
	if ds.IsDatastar(r) {
		if err := ds.NewSSE(w, r).Patch(poolFormDialog(f), ds.Inner("#"+poolDialogSlot)); err != nil {
			slog.DebugContext(r.Context(), "ui: pool form patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(r.Context(), ac, parsePoolQuery(nil))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Dialog = poolFormDialog(f)
	h.renderPage(w, r, http.StatusOK, view)
}

// formFailed answers a submit that did not go through: field and form errors
// patched into the open dialog (Datastar), or the page with the dialog open
// and the errors (a native post; status 422 for validation).
func (h *DispatchPools) formFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f poolForm) {
	h.withClients(r, ac, &f)
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, poolFormID, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	view, err := h.listView(r.Context(), ac, parsePoolQuery(nil))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	view.Dialog = poolFormDialog(f)
	status := f.Errs.Status
	if status == 0 {
		status = http.StatusUnprocessableEntity
	}
	h.renderPage(w, r, status, view)
}

// fieldProps is a field.md field whose root carries the patch-target id.
func (f poolForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{
		Label:       label,
		For:         f.id(key),
		Required:    required,
		Description: help,
		Error:       f.fieldError(key),
		Attrs:       templ.Attributes{"id": f.id(key) + "-field"},
	}
}

// describedBy links a control to its error (when there is one) and its help.
func (f poolForm) describedBy(key string) string {
	if f.fieldError(key) != "" {
		return f.id(key) + "-err"
	}
	return ""
}

func (f poolForm) inputProps(key, value string, p kit.InputProps) kit.InputProps {
	p.ID, p.Name, p.Value = f.id(key), key, value
	p.Invalid = f.fieldError(key) != ""
	p.DescribedBy = f.describedBy(key)
	return p
}

func (f poolForm) nameAttrs() templ.Attributes {
	a := templ.Attributes{"maxlength": "255", "autocomplete": "off"}
	if f.Edit {
		a["autofocus"] = true
	}
	return a
}

func numberAttrs() templ.Attributes {
	return templ.Attributes{"min": "1", "step": "1", "inputmode": "numeric"}
}

func (f poolForm) rateLimitHelp() string {
	if f.Edit {
		// The SPA's edit form says "Leave blank to run on concurrency only.",
		// but it sends no rate limit for a blank field, so the stored limit
		// stays; the help says what happens.
		return "Leave blank to keep the current setting."
	}
	return "Optional. Leave blank to run on concurrency only."
}

func (f poolForm) concurrencyHelp() string {
	if f.Edit {
		return ""
	}
	return "Maximum concurrent dispatches"
}

func (f poolForm) clientHelp() string {
	if !f.CanPickClient {
		return "Choosing a client needs permission to view clients."
	}
	return "If specified, this pool will only be used for jobs scoped to this client."
}
