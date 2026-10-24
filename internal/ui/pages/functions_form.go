package pages

import (
	"cmp"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	fnops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// The function's own dialogs (kit form-dialog.md): create, edit, and the
// shared plumbing of the drawer's dialogs (aliases, settings, routes, delete).

const (
	fnsFormDialog = "fns-form"
	fnsFormSignal = "fnsForm"
)

// fnsFieldCodes maps the use cases' validation codes to the form field that
// caused them; anything else is a form-level error.
var fnsFieldCodes = map[string]string{
	"APPLICATION_ID_REQUIRED":   "application",
	"NAME_REQUIRED":             "name",
	"INVALID_NAME_FORMAT":       "name",
	"ADDRESS_EXISTS":            "name",
	"SERVICE_ACCOUNT_REQUIRED":  "serviceAccountId",
	"SERVICE_ACCOUNT_NOT_FOUND": "serviceAccountId",
	"SERVICE_ACCOUNT_INACTIVE":  "serviceAccountId",
	"SERVICE_ACCOUNT_CONFLICT":  "serviceAccountId",
	"MEMORY_MB_REQUIRED":        "memoryMb",
	"MEMORY_MB_TOO_LARGE":       "memoryMb",
	"MAX_CONCURRENCY_REQUIRED":  "maxConcurrency",
	"MAX_CONCURRENCY_TOO_LARGE": "maxConcurrency",
	"TIMEOUT_MS_REQUIRED":       "timeoutMs",
	"TIMEOUT_MS_TOO_LARGE":      "timeoutMs",
	"MAX_BODY_BYTES_REQUIRED":   "maxBodyBytes",
	"MAX_BODY_BYTES_TOO_LARGE":  "maxBodyBytes",
}

// ── shared plumbing ────────────────────────────────────────────────────

// fnsBase is what the function dialogs share: the dialog's id, its errors and
// the order its fields are checked in (focus goes to the first invalid one).
type fnsBase struct {
	dialog string
	keys   []string
	Errs   formErrors
}

func newFnsBase(dialog string, keys ...string) fnsBase { return fnsBase{dialog: dialog, keys: keys} }

func (b fnsBase) id(key string) string         { return fieldID(b.dialog, key) }
func (b fnsBase) fieldError(key string) string { return b.Errs.Fields[key] }

func (b fnsBase) firstInvalid() string {
	for _, k := range b.keys {
		if b.fieldError(k) != "" {
			return b.id(k)
		}
	}
	return ""
}

func (b fnsBase) describedBy(key string) string {
	if b.fieldError(key) != "" {
		return b.id(key) + "-err"
	}
	return ""
}

// fieldProps is a field.md field whose root carries the patch-target id.
func (b fnsBase) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: b.id(key), Required: required, Description: help, Error: b.fieldError(key), Attrs: templ.Attributes{"id": b.id(key) + "-field"}}
}

func (b fnsBase) inputProps(key, value string, p kit.InputProps) kit.InputProps {
	p.ID, p.Name, p.Value = b.id(key), key, value
	p.Invalid = b.fieldError(key) != ""
	p.DescribedBy = b.describedBy(key)
	return p
}

// fnsDialogForm is a form dialog the page can show and, when a submit fails,
// patch field by field.
type fnsDialogForm interface {
	dialogID() string
	component() templ.Component
	fields() []templ.Component
	errs() formErrors
	firstInvalid() string
}

func (b fnsBase) dialogID() string { return b.dialog }
func (b fnsBase) errs() formErrors { return b.Errs }

// showDialog answers a request for a form dialog: patched into the page's
// dialog slot for Datastar, or the list page (with the function's drawer when
// id names one) with the dialog open.
func (h *Functions) showDialog(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id string, f fnsDialogForm, status int) {
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), fnsDialogSlot, f.component()); err != nil {
			slog.DebugContext(r.Context(), "ui: function dialog patch not delivered", "dialog", f.dialogID(), "err", err)
		}
		return
	}
	ctx := r.Context()
	view, err := h.listView(ctx, ac, parseListQuery(nil, fnsList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if id != "" && auth.CanReadFunctions(ac) == nil {
		fn, err := h.scopedFunction(ctx, ac, id)
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		d, err := h.buildDetail(ctx, ac, fn, r.PostFormValue("tab"))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		view.Drawer = &d
	}
	view.Dialog = f.component()
	h.renderPage(w, r, status, view)
}

// formFailed answers a submit that did not go through: field and form errors
// patched into the open dialog (Datastar), or the page with the dialog open
// and the errors (a native post; status 422 for validation).
func (h *Functions) formFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id string, f fnsDialogForm) {
	errs := f.errs()
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, f.dialogID(), f.fields(), errs.Form, f.firstInvalid())
		return
	}
	h.showDialog(w, r, ac, id, f, cmp.Or(errs.Status, http.StatusUnprocessableEntity))
}

// fnsOptionalInt64 is optionalInt32 for the body limit.
func fnsOptionalInt64(raw string) (*int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, false
	}
	return &n, true
}

// ── the function form ──────────────────────────────────────────────────

// fnsForm is the create or edit form (the SPA's FunctionCreateDrawer and the
// drawer's edit form): what is rendered, or what was submitted with its errors.
type fnsForm struct {
	fnsBase
	Edit       bool
	FunctionID string
	Subtitle   string

	Application, Name, Client, Description, Pool, ServiceAccount string
	Warm                                                         bool
	Memory, Concurrency, Timeout, Body                           string
	Open                                                         bool

	Apps          []kit.Option
	CanPickApp    bool
	CanPickClient bool
	ClientOptions []kit.Option
	// ServiceAccounts are the active service accounts (the current one is
	// always offered); without CanReadServiceAccounts the field is disabled
	// and the current account stays as it is.
	ServiceAccounts       []kit.Option
	CanPickServiceAccount bool
}

func newFnsForm(edit bool) fnsForm {
	f := fnsForm{Edit: edit, fnsBase: newFnsBase(fnsFormDialog)}
	f.keys = []string{"application", "name", "client", "pool", "serviceAccountId", "memoryMb", "maxConcurrency", "timeoutMs", "maxBodyBytes"}
	if edit {
		f.keys = []string{"description", "pool", "serviceAccountId", "memoryMb", "maxConcurrency", "timeoutMs", "maxBodyBytes"}
	}
	return f
}

func fnsEditForm(fn *function.Function) fnsForm {
	f := newFnsForm(true)
	f.FunctionID, f.Subtitle = fn.ID, fn.Address
	f.Description, f.Pool, f.Warm = deref(fn.Description), deref(fn.Pool), fn.Warm
	f.ServiceAccount = deref(fn.ServiceAccountID)
	f.Memory, f.Concurrency = strconv.Itoa(int(fn.Limits.MemoryMB)), strconv.Itoa(int(fn.Limits.MaxConcurrency))
	f.Timeout, f.Body = strconv.Itoa(int(fn.Limits.TimeoutMs)), strconv.FormatInt(fn.Limits.MaxBodyBytes, 10)
	return f
}

func submittedFnsForm(r *http.Request, edit bool) fnsForm {
	f := newFnsForm(edit)
	f.Open = true
	f.Application = strings.TrimSpace(r.PostFormValue("application"))
	f.Name = strings.TrimSpace(r.PostFormValue("name"))
	f.Client = strings.TrimSpace(r.PostFormValue("client"))
	f.Description = r.PostFormValue("description")
	f.Pool = strings.TrimSpace(r.PostFormValue("pool"))
	f.Warm = r.PostFormValue("warm") != ""
	f.ServiceAccount = strings.TrimSpace(r.PostFormValue("serviceAccountId"))
	f.Memory, f.Concurrency = r.PostFormValue("memoryMb"), r.PostFormValue("maxConcurrency")
	f.Timeout, f.Body = r.PostFormValue("timeoutMs"), r.PostFormValue("maxBodyBytes")
	return f
}

func (f fnsForm) action() string {
	if f.Edit {
		return fnsURL(f.FunctionID) + "/edit"
	}
	return fnsPath + "/new"
}

func (f fnsForm) dialogProps() kit.FormDialogProps {
	p := kit.FormDialogProps{
		ID: fnsFormDialog, Signal: fnsFormSignal, Width: kit.WidthWide, Action: f.action(), Open: f.Open, Error: f.Errs.Form,
		Title: "Create Function", Description: "Register a new function; publish and promote a version separately", SubmitLabel: "Create Function",
	}
	if f.Edit {
		p.Title, p.Description, p.SubmitLabel, p.RequireChange = "Edit Function", f.Subtitle, "Save", true
		p.Hidden = fnsHidden()
	}
	return p
}

func (f fnsForm) fields() []templ.Component {
	out := make([]templ.Component, 0, len(f.keys)+1)
	for _, k := range f.keys {
		out = append(out, fnsField(f, k))
	}
	return append(out, fnsField(f, "warm"))
}

func (f fnsForm) component() templ.Component { return fnsFormDialogC(f) }

// limits parses the four limit fields, recording an error for anything that
// is not a whole number; nil when none is given.
func (f *fnsForm) limits() *fnops.LimitsInput {
	var l fnops.LimitsInput
	var ok bool
	if l.MemoryMB, ok = optionalInt32(f.Memory); !ok {
		f.Errs.field("memoryMb", notWholeNumber)
	}
	if l.MaxConcurrency, ok = optionalInt32(f.Concurrency); !ok {
		f.Errs.field("maxConcurrency", notWholeNumber)
	}
	if l.TimeoutMs, ok = optionalInt32(f.Timeout); !ok {
		f.Errs.field("timeoutMs", notWholeNumber)
	}
	if l.MaxBodyBytes, ok = fnsOptionalInt64(f.Body); !ok {
		f.Errs.field("maxBodyBytes", notWholeNumber)
	}
	if l == (fnops.LimitsInput{}) {
		return nil
	}
	return &l
}

// withChoices loads what the form's selects offer: the applications (create),
// the client suggestions (create) and the active service accounts.
func (h *Functions) withChoices(r *http.Request, ac *auth.AuthContext, f *fnsForm, current *function.Function) {
	ctx := r.Context()
	if !f.Edit {
		f.CanPickApp = h.Apps != nil && auth.CanReadApplications(ac) == nil
		if f.CanPickApp {
			if as, err := h.Apps.FindWithFilters(ctx, nil, nil); err != nil {
				slog.WarnContext(ctx, "ui: applications unavailable", "err", err)
				f.CanPickApp = false
			} else {
				f.Apps = fnsApplicationOptions(as)
			}
		}
		f.CanPickClient = canPickClient(h.Clients, ac)
		if opts, err := clientSuggestions(ctx, h.Clients, ac, f.Client); err != nil {
			slog.WarnContext(ctx, "ui: client suggestions unavailable", "err", err)
		} else {
			f.ClientOptions = opts
		}
	}
	f.CanPickServiceAccount = h.ServiceAccounts != nil && auth.CanReadServiceAccounts(ac) == nil
	if !f.CanPickServiceAccount {
		// The field shows the current account and cannot change it.
		if current != nil && current.ServiceAccountID != nil {
			sa := *current.ServiceAccountID
			label := sa
			if codes, err := h.Functions.ServiceAccountCodes(ctx, []string{sa}); err == nil && codes[sa] != "" {
				label = codes[sa]
			}
			f.ServiceAccounts = []kit.Option{{Value: sa, Label: label}}
		}
		return
	}
	sas, err := h.ServiceAccounts.FindAll(ctx)
	if err != nil {
		slog.WarnContext(ctx, "ui: service accounts unavailable", "err", err)
		f.CanPickServiceAccount = false
		return
	}
	for _, sa := range sas {
		if sa.Active || sa.ID == f.ServiceAccount {
			f.ServiceAccounts = append(f.ServiceAccounts, kit.Option{Value: sa.ID, Label: sa.Name + " (" + sa.Code + ")"})
		}
	}
	// The current account is always selectable, even if it was filtered out.
	if f.ServiceAccount != "" && !slices.ContainsFunc(f.ServiceAccounts, func(o kit.Option) bool { return o.Value == f.ServiceAccount }) {
		f.ServiceAccounts = append([]kit.Option{{Value: f.ServiceAccount, Label: f.ServiceAccount}}, f.ServiceAccounts...)
	}
}

func (f fnsForm) clientHelp() string {
	if !f.CanPickClient {
		return "Choosing a client needs permission to view clients; the function is platform-owned."
	}
	return "Optional. Leave blank for a platform-owned function."
}

func (f fnsForm) appHelp() string {
	if !f.CanPickApp {
		return "Choosing an application needs permission to view applications."
	}
	return "The function's owning application; its code prefixes the address."
}

func (f fnsForm) saHelp() string {
	if !f.CanPickServiceAccount {
		return "Choosing a service account needs permission to view service accounts."
	}
	return "Optional. Verifies webhook deliveries to this function with this account's signing secret: set it to the account on the connection that dispatches here. Blank uses the application's linked account."
}

func (f fnsForm) poolHelp() string {
	return "The runner pool that hosts the function. Left blank, it runs in the default pool."
}

// ── create ─────────────────────────────────────────────────────────────

// createForm opens the create dialog (the SPA's Create Function).
func (h *Functions) createForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	// POST /api/functions's check.
	if err := auth.CanWriteFunctions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := newFnsForm(false)
	f.Open = true
	h.withChoices(r, ac, &f, nil)
	h.showDialog(w, r, ac, "", f, http.StatusOK)
}

func (h *Functions) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedFnsForm(r, false)
	fail := func() {
		h.withChoices(r, ac, &f, nil)
		h.formFailed(w, r, ac, "", f)
	}
	if err := auth.CanWriteFunctions(ac); err != nil {
		f.Errs.apply(r, err, nil)
		fail()
		return
	}
	// The SPA's checks: an application, a valid name.
	if f.Application == "" {
		f.Errs.field("application", "Select an application.")
	}
	if f.Name != "" && !function.ValidName(f.Name) {
		f.Errs.field("name", "Lowercase letters, numbers, hyphens only. Must start with a letter (max 63 characters).")
	} else if f.Name == "" {
		f.Errs.field("name", "Name is required.")
	}
	cmd := fnops.CreateCommand{ApplicationID: f.Application, Name: f.Name, Pool: optionalText(f.Pool), Limits: f.limits()}
	warm := f.Warm
	cmd.Warm = &warm
	cmd.ServiceAccount = optionalText(f.ServiceAccount)
	clientID, msg, err := resolveClient(ctx, h.Clients, ac, f.Client)
	switch {
	case err != nil:
		f.Errs.apply(r, err, nil)
	case msg != "":
		f.Errs.field("client", msg)
	}
	cmd.ClientID = clientID
	if f.Errs.any() {
		fail()
		return
	}
	newID, err := h.Ops.Create(ctx, cmd)
	if err != nil {
		f.Errs.apply(r, err, fnsFieldCodes)
		// The use case refuses a client or a service account the caller
		// cannot access with the same code; the field is told apart by it.
		if ue, ok := errors.AsType[*usecase.Error](err); ok && ue.Code == "SCOPE_FORBIDDEN" {
			f.Errs.Form = ""
			switch {
			case strings.Contains(ue.Message, "service account"):
				f.Errs.field("serviceAccountId", sentence(ue.Message))
			case cmd.ClientID != nil:
				f.Errs.field("client", sentence(ue.Message))
			default:
				f.Errs.Form = sentence(ue.Message)
			}
		}
		fail()
		return
	}
	if !ds.IsDatastar(r) {
		h.nativeDone(w, r, newID, flashFnCreated, false)
		return
	}
	// As the SPA: close the form and open the new function's detail drawer.
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(fnsFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: function form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, newID, false)
	if auth.CanReadFunctions(ac) == nil {
		if fn, err := h.loadFunction(ctx, ac, newID); err != nil {
			h.Shell.FailSSE(sse, r, err)
		} else if d, err := h.buildDetail(ctx, ac, fn, fnsTabOverview); err != nil {
			h.Shell.FailSSE(sse, r, err)
		} else if err := openFnsDrawer(sse, d); err != nil {
			slog.DebugContext(ctx, "ui: function drawer patch not delivered", "err", err)
		}
	}
	if err := flashKey(sse, flashFnCreated); err != nil {
		slog.DebugContext(ctx, "ui: function flash not delivered", "err", err)
	}
}

// ── edit ───────────────────────────────────────────────────────────────

// editForm opens the edit dialog (the row's Edit, the drawer's Edit). The SPA
// loads the function (read) and saves it with PATCH (manage).
func (h *Functions) editForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := firstError(auth.CanReadFunctions(ac), auth.CanWriteFunctions(ac)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := fnsID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	fn, err := h.loadFunction(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := fnsEditForm(fn)
	f.Open = true
	h.withChoices(r, ac, &f, fn)
	h.showDialog(w, r, ac, "", f, http.StatusOK)
}

func (h *Functions) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedFnsForm(r, true)
	id, err := fnsID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.FunctionID = id
	var current *function.Function
	fail := func() {
		h.withChoices(r, ac, &f, current)
		h.formFailed(w, r, ac, id, f)
	}
	if err := auth.CanWriteFunctions(ac); err != nil {
		f.Errs.apply(r, err, nil)
		fail()
		return
	}
	current, err = h.scopedFunction(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Subtitle = current.Address
	limits := f.limits()
	if f.Errs.any() {
		fail()
		return
	}
	cmd := fnops.UpdateCommand{ID: id, Warm: &f.Warm, Limits: limits}
	// The SPA's PATCH body, where it could not clear a description: a changed
	// description is sent (a blank one clears it).
	if desc := strings.TrimSpace(f.Description); desc != deref(current.Description) {
		cmd.Description = &desc
	}
	switch {
	case f.Pool == "":
		cmd.ClearPool = deref(current.Pool) != ""
	case f.Pool != deref(current.Pool):
		cmd.Pool = &f.Pool
	}
	// A disabled service account field is not posted: it stays as it is.
	if _, posted := r.PostForm["serviceAccountId"]; posted && f.ServiceAccount != deref(current.ServiceAccountID) {
		if f.ServiceAccount == "" {
			cmd.ClearServiceAccount = true
		} else {
			cmd.ServiceAccount = &f.ServiceAccount
		}
	}
	if err := h.Ops.Update(ctx, cmd); err != nil {
		f.Errs.apply(r, err, fnsFieldCodes)
		fail()
		return
	}
	h.done(w, r, ac, id, fnsFormDialog, flashFnUpdated, "", false)
}
