package pages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	connops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// ServiceAccountDirectory lists the service accounts a connection can use
// (the SPA's serviceAccountsApi.list(): GET /api/service-accounts) and reads
// one for a drawer that names it (GET /api/service-accounts/{id}).
type ServiceAccountDirectory interface {
	FindAll(ctx context.Context) ([]serviceaccount.ServiceAccount, error)
	FindByID(ctx context.Context, id string) (*serviceaccount.ServiceAccount, error)
}

const (
	connFormID     = "conn-form"
	connFormSignal = "connForm"
)

var connFieldCodes = map[string]string{
	"CODE_REQUIRED":            "code",
	"INVALID_CODE_FORMAT":      "code",
	"CODE_EXISTS":              "code",
	"NAME_REQUIRED":            "name",
	"SERVICE_ACCOUNT_REQUIRED": "serviceAccountId",
}

// connForm is the create or edit form (the SPA's ConnectionCreateDrawer and
// the drawer's edit form).
type connForm struct {
	Edit     bool
	ConnID   string
	Subtitle string

	Code, Name, Description, ExternalID string
	ServiceAccountID, Client            string

	Errs formErrors
	Open bool

	CanPickClient         bool
	ClientOptions         []kit.Option
	CanPickServiceAccount bool
	ServiceAccounts       []kit.Option
}

func (f connForm) id(key string) string         { return fieldID(connFormID, key) }
func (f connForm) fieldError(key string) string { return f.Errs.Fields[key] }

func (f connForm) action() string {
	if f.Edit {
		return connURL(f.ConnID) + "/edit"
	}
	return connsPath + "/new"
}

func (f connForm) keys() []string {
	if f.Edit {
		return []string{"name", "description", "externalId"}
	}
	return []string{"code", "name", "description", "externalId", "serviceAccountId", "client"}
}

func (f connForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.fieldError(k) != "" {
			return f.id(k)
		}
	}
	return ""
}

func (f connForm) fields() []templ.Component {
	out := make([]templ.Component, 0, 6)
	for _, k := range f.keys() {
		out = append(out, connField(f, k))
	}
	return out
}

func (f connForm) dialog() kit.FormDialogProps {
	p := kit.FormDialogProps{
		ID: connFormID, Signal: connFormSignal, Width: kit.WidthWide, Action: f.action(), Open: f.Open, Error: f.Errs.Form,
		Title: "Create Connection", Description: "Configure a new webhook connection for event delivery", SubmitLabel: "Create Connection",
	}
	if f.Edit {
		p.Title, p.Description, p.SubmitLabel, p.RequireChange = "Edit Connection", f.Subtitle, "Save", true
		p.Hidden = []kit.HiddenField{drawerField(connDrawerSig)}
	}
	return p
}

func submittedConnForm(r *http.Request, edit bool) connForm {
	return connForm{
		Edit:             edit,
		Code:             r.PostFormValue("code"),
		Name:             r.PostFormValue("name"),
		Description:      r.PostFormValue("description"),
		ExternalID:       r.PostFormValue("externalId"),
		ServiceAccountID: strings.TrimSpace(r.PostFormValue("serviceAccountId")),
		Client:           strings.TrimSpace(r.PostFormValue("client")),
	}
}

// ── create ─────────────────────────────────────────────────────────────

func (h *Connections) createForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	// POST /api/connections's check.
	if err := auth.CanCreateConnections(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := connForm{Open: true}
	h.withChoices(r, ac, &f)
	h.showForm(w, r, ac, f)
}

func (h *Connections) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedConnForm(r, false)
	if err := auth.CanCreateConnections(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	// The SPA's POST body: no application (shared), description and
	// external id only when set, the client when one was chosen.
	cmd := connops.CreateCommand{
		Code:             f.Code,
		Name:             f.Name,
		Description:      optionalText(f.Description),
		ExternalID:       optionalText(f.ExternalID),
		ServiceAccountID: f.ServiceAccountID,
	}
	id, msg, err := resolveClient(ctx, h.Clients, ac, f.Client)
	switch {
	case err != nil:
		f.Errs.apply(r, err, nil)
	case msg != "":
		f.Errs.field("client", msg)
	}
	cmd.ClientID = id
	if f.Errs.any() {
		h.formFailed(w, r, ac, f)
		return
	}
	newID, err := h.Ops.Create(ctx, cmd)
	if err != nil {
		f.Errs.apply(r, err, connFieldCodes)
		if ue, ok := errors.AsType[*usecase.Error](err); ok && ue.Code == "SCOPE_FORBIDDEN" && cmd.ClientID != nil {
			f.Errs.Form = ""
			f.Errs.field("client", sentence(ue.Message))
		}
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashConnCreated)
		http.Redirect(w, r, connsPath, http.StatusSeeOther)
		return
	}
	// As the SPA: close the form and open the new connection's drawer.
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(connFormID); err != nil {
		slog.DebugContext(ctx, "ui: connection form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, newID, false)
	if c, err := h.loadConnection(ctx, ac, newID); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := openConnDrawer(sse, h.connDetail(ctx, ac, c)); err != nil {
		slog.DebugContext(ctx, "ui: connection drawer patch not delivered", "err", err)
	}
	if err := flashKey(sse, flashConnCreated); err != nil {
		slog.DebugContext(ctx, "ui: connection flash not delivered", "err", err)
	}
}

// ── edit ───────────────────────────────────────────────────────────────

func (h *Connections) editForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// The SPA loads the connection (GET) and saves it with PUT.
	if err := firstError(auth.CanReadConnections(ac), auth.CanUpdateConnections(ac)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := connID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	c, err := h.loadConnection(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := connForm{Edit: true, ConnID: c.ID, Subtitle: c.Name + " (" + c.Code + ")", Name: c.Name, Description: deref(c.Description), ExternalID: deref(c.ExternalID), Open: true}
	h.showForm(w, r, ac, f)
}

func (h *Connections) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedConnForm(r, true)
	if err := firstError(auth.CanReadConnections(ac), auth.CanUpdateConnections(ac)); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	id, err := connID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	c, err := h.loadConnection(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.ConnID, f.Subtitle = c.ID, c.Name+" ("+c.Code+")"
	// The SPA's PUT body: name; description and external id `|| undefined`
	// (the use case stores what it gets, so a blank field clears it).
	err = h.Ops.Update(ctx, connops.UpdateCommand{
		ID:          id,
		Name:        f.Name,
		Description: optionalText(f.Description),
		ExternalID:  optionalText(f.ExternalID),
	})
	if err != nil {
		f.Errs.apply(r, err, connFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashConnUpdated)
		http.Redirect(w, r, connsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(connFormID); err != nil {
		slog.DebugContext(ctx, "ui: connection form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, id, false)
	if err := flashKey(sse, flashConnUpdated); err != nil {
		slog.DebugContext(ctx, "ui: connection flash not delivered", "err", err)
	}
}

// ── rendering ──────────────────────────────────────────────────────────

// withChoices fills the client suggestions and the service accounts (each
// only when the user may list them, as the SPA's calls would succeed).
func (h *Connections) withChoices(r *http.Request, ac *auth.AuthContext, f *connForm) {
	if f.Edit {
		return
	}
	ctx := r.Context()
	f.CanPickClient = canPickClient(h.Clients, ac)
	if opts, err := clientSuggestions(ctx, h.Clients, ac, f.Client); err != nil {
		slog.WarnContext(ctx, "ui: client suggestions unavailable", "err", err)
	} else {
		f.ClientOptions = opts
	}
	f.CanPickServiceAccount = h.ServiceAccounts != nil && auth.CanReadServiceAccounts(ac) == nil
	if !f.CanPickServiceAccount {
		return
	}
	sas, err := h.ServiceAccounts.FindAll(ctx)
	if err != nil {
		slog.WarnContext(ctx, "ui: service accounts unavailable", "err", err)
		f.CanPickServiceAccount = false
		return
	}
	f.ServiceAccounts = make([]kit.Option, 0, len(sas))
	for _, sa := range sas {
		f.ServiceAccounts = append(f.ServiceAccounts, kit.Option{Value: sa.ID, Label: sa.Name + " (" + sa.Code + ")"})
	}
}

func (h *Connections) showForm(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f connForm) {
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), connDialogSlot, connFormDialog(f)); err != nil {
			slog.DebugContext(r.Context(), "ui: connection form patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, connList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Dialog = connFormDialog(f)
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *Connections) formFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f connForm) {
	h.withChoices(r, ac, &f)
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, connFormID, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, connList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	view.Dialog = connFormDialog(f)
	status := f.Errs.Status
	if status == 0 {
		status = http.StatusUnprocessableEntity
	}
	h.renderPage(w, r, status, view)
}

func (f connForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.id(key), Required: required, Description: help, Error: f.fieldError(key), Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f connForm) describedBy(key string) string {
	if f.fieldError(key) != "" {
		return f.id(key) + "-err"
	}
	return ""
}

func (f connForm) inputProps(key, value string, p kit.InputProps) kit.InputProps {
	p.ID, p.Name, p.Value = f.id(key), key, value
	p.Invalid = f.fieldError(key) != ""
	p.DescribedBy = f.describedBy(key)
	return p
}

func (f connForm) serviceAccountHelp() string {
	if !f.CanPickServiceAccount {
		return "Choosing a service account needs permission to view service accounts."
	}
	return "The service account used for authentication when delivering events"
}

func (f connForm) clientHelp() string {
	if !f.CanPickClient {
		return "Choosing a client needs permission to view clients; the connection is anchor-level."
	}
	return "Leave empty for an anchor-level connection, or select a specific client."
}

func connNameAttrs(edit bool) templ.Attributes {
	a := templ.Attributes{"maxlength": "255", "autocomplete": "off"}
	if edit {
		a["autofocus"] = true
	}
	return a
}

func connExternalIDHelp(edit bool) string {
	if edit {
		return ""
	}
	return "An optional external reference for this connection"
}
