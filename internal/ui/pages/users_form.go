package pages

import (
	"cmp"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	principalapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// The user dialogs (kit form-dialog.md): create, edit and the shared plumbing
// of the drawer's other dialogs (roles, applications, client access, password).

const (
	usrFormDialog = "usr-form"
	usrFormSignal = "usrForm"
)

// usrEmailPattern is the SPA's email check (UserCreateDrawer).
var usrEmailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// usrFieldCodes maps the use cases' validation codes to the form field that
// caused them; anything else is a form-level error.
var usrFieldCodes = map[string]string{
	"NAME_REQUIRED":              "name",
	"EMAIL_REQUIRED":             "email",
	"INVALID_EMAIL":              "email",
	"EMAIL_EXISTS":               "email",
	"CLIENT_REQUIRED":            "clientId",
	"CLIENT_NOT_ALLOWED":         "clientId",
	"PASSWORD_TOO_SHORT":         "password",
	"PASSWORD_TOO_LONG":          "password",
	"PASSWORD_TOO_WEAK":          "password",
	"PASSWORD_TOO_COMMON":        "password",
	"PASSWORD_CONTAINS_IDENTITY": "password",
}

// usrFlash is a flash message of the table, with the detail the server built
// when it has one (a flash key carries no request text).
func usrFlash(sse *ds.SSE, key, detail string) error {
	msg, ok := FlashMessages[key]
	if !ok {
		return errors.New("ui: unknown flash key " + key)
	}
	if detail != "" {
		msg.Detail = detail
	}
	return shell.Flash(sse, msg)
}

// ── shared plumbing ────────────────────────────────────────────────────

// usrBase is what the user dialogs share: the dialog's id, its errors and the
// order its fields are checked in (focus goes to the first invalid one).
type usrBase struct {
	dialog string
	keys   []string
	Errs   formErrors
}

func newUsrBase(dialog string, keys ...string) usrBase { return usrBase{dialog: dialog, keys: keys} }

func (b usrBase) id(key string) string         { return fieldID(b.dialog, key) }
func (b usrBase) fieldError(key string) string { return b.Errs.Fields[key] }
func (b usrBase) dialogID() string             { return b.dialog }
func (b usrBase) errs() formErrors             { return b.Errs }

func (b usrBase) firstInvalid() string {
	for _, k := range b.keys {
		if b.fieldError(k) != "" {
			return b.id(k)
		}
	}
	return ""
}

func (b usrBase) describedBy(key string) string {
	if b.fieldError(key) != "" {
		return b.id(key) + "-err"
	}
	return ""
}

// fieldProps is a field.md field whose root carries the patch-target id.
func (b usrBase) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: b.id(key), Required: required, Description: help, Error: b.fieldError(key), Attrs: templ.Attributes{"id": b.id(key) + "-field"}}
}

func (b usrBase) inputProps(key, value string, p kit.InputProps) kit.InputProps {
	p.ID, p.Name, p.Value = b.id(key), key, value
	p.Invalid = b.fieldError(key) != ""
	p.DescribedBy = b.describedBy(key)
	return p
}

// usrDialogForm is a form dialog the page can show and, when a submit fails,
// patch field by field.
type usrDialogForm interface {
	dialogID() string
	component() templ.Component
	fields() []templ.Component
	errs() formErrors
	firstInvalid() string
}

// showDialog answers a request for a form dialog: patched into the page's
// dialog slot for Datastar, or the list page (with the user's drawer when id
// names one) with the dialog open.
func (h *Users) showDialog(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id string, f usrDialogForm, status int) {
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), usrDialogSlot, f.component()); err != nil {
			slog.DebugContext(r.Context(), "ui: user dialog patch not delivered", "dialog", f.dialogID(), "err", err)
		}
		return
	}
	ctx := r.Context()
	view, err := h.listView(ctx, ac, parseListQuery(nil, usrList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if id != "" {
		d, err := h.buildDetail(ctx, ac, id)
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
func (h *Users) formFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id string, f usrDialogForm) {
	errs := f.errs()
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, f.dialogID(), f.fields(), errs.Form, f.firstInvalid())
		return
	}
	h.showDialog(w, r, ac, id, f, cmp.Or(errs.Status, http.StatusUnprocessableEntity))
}

// ── create ─────────────────────────────────────────────────────────────

// usrCreateForm is the create form: for a platform administrator the SPA's
// UserCreateDrawer (name, email, the email domain's check, a client when the
// domain needs one), for a client administrator ClientUserCreateDrawer (name,
// email, an optional password, the client when it may act in several). The
// password is never echoed.
type usrCreateForm struct {
	usrBase
	ClientMode            bool
	Name, Email, ClientID string
	ShowLink              bool
	// Check is the email domain's check (platform form); CheckErr why it
	// could not be made.
	Check    *principalapi.CheckEmailDomainResponse
	CheckErr string
	// Choices are the clients the select offers; CanPickClient whether the
	// caller may choose one at all.
	Choices       []kit.Option
	CanPickClient bool
	// DropStale forgets a chosen client that is no longer offered (the email
	// changed to another domain).
	DropStale bool
	Open      bool
}

func newUsrCreateForm(clientMode bool) usrCreateForm {
	f := usrCreateForm{ClientMode: clientMode, usrBase: newUsrBase(usrFormDialog, "name", "email", "password", "clientId")}
	return f
}

func submittedUsrCreateForm(r *http.Request, clientMode bool) usrCreateForm {
	f := newUsrCreateForm(clientMode)
	f.Name = strings.TrimSpace(r.PostFormValue("name"))
	f.Email = strings.TrimSpace(r.PostFormValue("email"))
	f.ClientID = strings.TrimSpace(r.PostFormValue("clientId"))
	f.ShowLink = r.PostFormValue("returnLink") != ""
	return f
}

// internal is whether the new user signs in with a platform password.
func (f usrCreateForm) internal() bool {
	return f.ClientMode || (f.Check != nil && f.Check.AuthProvider == "INTERNAL")
}

// needsClient is whether the form asks for a client.
func (f usrCreateForm) needsClient() bool {
	if f.ClientMode {
		return len(f.Choices) > 1
	}
	return f.Check != nil && f.Check.RequiresClientID
}

// linkOffered is whether the sign-in link switch is shown: for a user who will
// set a password of their own.
func (f usrCreateForm) linkOffered() bool {
	if f.ClientMode {
		return true
	}
	return f.Check != nil && !f.Check.EmailExists && f.internal()
}

func (f usrCreateForm) dialogProps() kit.FormDialogProps {
	p := kit.FormDialogProps{
		ID: usrFormDialog, Signal: usrFormSignal, Action: usersPath + "/new", Open: f.Open, Error: f.Errs.Form,
		Title: "Add User", Description: "Create a new platform user", SubmitLabel: "Create User", Hidden: usrHidden(),
	}
	if f.ClientMode {
		p.Description = "Create a user for your client"
		p.SubmitLabel = "Create"
	}
	return p
}

func (f usrCreateForm) fields() []templ.Component {
	out := []templ.Component{usrField(f, "name"), usrField(f, "email")}
	if f.ClientMode {
		out = append(out, usrField(f, "password"))
	}
	if f.needsClient() {
		out = append(out, usrField(f, "clientId"))
	}
	return out
}

func (f usrCreateForm) component() templ.Component { return usrCreateDialogC(f) }

// clientHelp is the SPA's help under the client select.
func (f usrCreateForm) clientHelp() string {
	switch {
	case !f.CanPickClient:
		return "Choosing a client needs permission to view clients."
	case f.ClientMode:
		return ""
	case f.Check != nil && f.Check.DerivedScope == "PARTNER":
		return "This partner domain restricts users to specific clients."
	}
	return "Required for non-anchor users — sets the user's home client."
}

// withChoices loads the clients the select offers: the active clients the
// caller can access (for a platform administrator only with the permission to
// view clients, as the SPA lists them), narrowed to the ones a partner domain
// allows, and preselects the only one left.
func (h *Users) withChoices(r *http.Request, ac *auth.AuthContext, f *usrCreateForm) {
	ctx := r.Context()
	f.CanPickClient = f.ClientMode || auth.CanReadClients(ac) == nil
	if !f.CanPickClient {
		return
	}
	cs, err := h.clientChoices(ctx, ac)
	if err != nil {
		slog.WarnContext(ctx, "ui: clients unavailable", "err", err)
		f.CanPickClient = false
		return
	}
	f.Choices = clientOptionsOf(cs)
	if f.Check != nil && len(f.Check.AllowedClientIDs) > 0 {
		f.Choices = slices.DeleteFunc(f.Choices, func(o kit.Option) bool { return !slices.Contains(f.Check.AllowedClientIDs, o.Value) })
	}
	switch {
	case f.ClientID != "" && f.clientAllowed():
	case f.ClientID != "" && !f.DropStale:
		// Kept as posted: create refuses a client that is not offered.
	case f.Check != nil && f.Check.RequiresClientID && len(f.Check.AllowedClientIDs) == 1 && f.hasChoice(f.Check.AllowedClientIDs[0]):
		f.ClientID = f.Check.AllowedClientIDs[0]
	case f.ClientMode && len(f.Choices) == 1:
		f.ClientID = f.Choices[0].Value
	default:
		f.ClientID = ""
	}
}

func (f usrCreateForm) hasChoice(id string) bool {
	return slices.ContainsFunc(f.Choices, func(o kit.Option) bool { return o.Value == id })
}

// clientAllowed is whether the chosen client is one the form offers (none
// chosen is allowed: that is for the required check).
func (f usrCreateForm) clientAllowed() bool { return f.ClientID == "" || f.hasChoice(f.ClientID) }

// createForm opens the create dialog (the SPA's Add User).
func (h *Users) createForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	// POST /api/principals's check.
	if err := auth.CanCreatePrincipals(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := newUsrCreateForm(!ac.IsAnchor())
	f.Open = true
	h.withChoices(r, ac, &f)
	h.showDialog(w, r, ac, "", f, http.StatusOK)
}

// checkEmail answers the create form's email check (the SPA's check on blur):
// the domain region patched with what the email's domain says (GET
// /api/principals/check-email-domain, read permission), and the email field
// with its error.
func (h *Users) checkEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := firstError(auth.CanReadPrincipals(ac), auth.CanCreatePrincipals(ac)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	q := r.URL.Query()
	f := newUsrCreateForm(false)
	f.Open = true
	f.Name, f.Email = strings.TrimSpace(q.Get("name")), strings.TrimSpace(q.Get("email"))
	f.ClientID, f.ShowLink, f.DropStale = strings.TrimSpace(q.Get("clientId")), q.Get("returnLink") != "", true
	switch {
	case f.Email == "":
		f.Errs.field("email", "Email is required.")
	case !usrEmailPattern.MatchString(f.Email):
		f.Errs.field("email", "Please enter a valid email address.")
	default:
		check, err := h.Users.CheckEmailDomain(ctx, f.Email)
		if err != nil {
			_, msg := shell.Classify(r, err)
			f.CheckErr = msg
		} else {
			f.Check = &check
		}
	}
	h.withChoices(r, ac, &f)
	sse := ds.NewSSE(w, r)
	if err := errors.Join(sse.Patch(usrField(f, "email")), sse.Patch(usrDomainRegion(f))); err != nil {
		slog.DebugContext(ctx, "ui: email check patch not delivered", "err", err)
	}
}

// create makes the user. The SPA's platform form sends the scope the email's
// domain derives and the client when the domain needs one; the client
// administrator's form creates a CLIENT user, optionally with a password.
func (h *Users) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// The answer may carry a sign-in link: never cached.
	w = noStore{w}
	clientMode := !ac.IsAnchor()
	f := submittedUsrCreateForm(r, clientMode)
	password := r.PostFormValue("password")
	fail := func() {
		h.withChoices(r, ac, &f)
		f.Open = true
		h.formFailed(w, r, ac, "", f)
	}
	if err := auth.CanCreatePrincipals(ac); err != nil {
		f.Errs.apply(r, err, nil)
		fail()
		return
	}
	// The SPA's checks: a name, a valid email, a client where one is needed.
	if f.Name == "" {
		f.Errs.field("name", "Name is required.")
	}
	switch {
	case f.Email == "":
		f.Errs.field("email", "Email is required.")
	case !usrEmailPattern.MatchString(f.Email):
		f.Errs.field("email", "Please enter a valid email address.")
	}
	var created createdUser
	if clientMode {
		h.withChoices(r, ac, &f)
		// The client is one the administrator can act in.
		if f.ClientID == "" || !f.clientAllowed() {
			if f.needsClient() {
				f.Errs.field("clientId", "Select a client.")
			} else {
				f.Errs.Form, f.Errs.Status = "You cannot add users to that client.", http.StatusForbidden
			}
		}
		if f.Errs.any() {
			fail()
			return
		}
		body := principalapi.CreatePrincipalRequest{Email: f.Email, Name: &f.Name, Scope: "CLIENT", ClientID: &f.ClientID}
		if password != "" {
			body.Password = &password
		}
		// The link is for a user who sets their own password.
		if f.ShowLink && password == "" {
			yes := true
			body.ReturnInviteLink = &yes
		}
		res, err := h.Users.CreatePrincipal(ctx, body)
		if err != nil {
			f.Errs.apply(r, err, usrFieldCodes)
			fail()
			return
		}
		created = createdUser{ID: res.ID, Link: deref(res.InviteLink), Detail: f.Name + " was added"}
	} else {
		if f.Errs.any() {
			fail()
			return
		}
		check, err := h.Users.CheckEmailDomain(ctx, f.Email)
		if err != nil {
			f.Errs.apply(r, err, nil)
			fail()
			return
		}
		f.Check = &check
		if check.EmailExists {
			f.Errs.field("email", cmp.Or(deref(check.Warning), "A user with this email address already exists."))
		}
		h.withChoices(r, ac, &f)
		if check.RequiresClientID && (f.ClientID == "" || (f.CanPickClient && !f.clientAllowed())) {
			f.Errs.field("clientId", "Select a client.")
		}
		if f.Errs.any() {
			fail()
			return
		}
		// The backend defaults to CLIENT and never promotes the scope from the
		// email domain on its own: the form echoes the scope the domain check
		// derived, so what was previewed is what is created.
		body := principalapi.CreateUserRequest{Email: f.Email, Name: f.Name, Scope: &check.DerivedScope}
		if check.RequiresClientID {
			body.ClientID = &f.ClientID
		}
		if f.ShowLink && f.internal() {
			yes := true
			body.ReturnInviteLink = &yes
		}
		u, err := h.Users.CreateDomainUser(ctx, body)
		if err != nil {
			f.Errs.apply(r, err, usrFieldCodes)
			fail()
			return
		}
		created = createdUser{ID: u.ID, Link: deref(u.InviteLink), Detail: "They can sign in via their identity provider."}
		if f.internal() {
			created.Detail = "We've emailed them a one-time sign-in link to set their password."
			if created.Link != "" {
				created.Detail = "Give them the sign-in link to set their password."
			}
		}
	}
	h.created(w, r, ac, created)
}

// createdUser is what a successful create reports.
type createdUser struct {
	ID string
	// Link is the sign-in link, when one was asked for.
	Link   string
	Detail string
}

// created answers a successful create as the SPA does: the new user's
// drawer opens over the refreshed list. A sign-in link is shown in a dialog
// in this answer only (no-store, nothing in a URL, signal or flash).
func (h *Users) created(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, c createdUser) {
	ctx := r.Context()
	var link templ.Component
	if c.Link != "" {
		link = usrLinkDialogC(c.Link)
	}
	if !ds.IsDatastar(r) {
		if link == nil {
			h.nativeDone(w, r, c.ID, flashUsrCreated, false)
			return
		}
		// Post/redirect/get would lose the link: the page itself answers.
		view, err := h.listView(ctx, ac, parseListQuery(nil, usrList))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		view.Secret = link
		h.renderPage(w, r, http.StatusOK, view)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(usrFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: user form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, c.ID, false)
	if auth.CanReadPrincipals(ac) == nil {
		if d, err := h.buildDetail(ctx, ac, c.ID); err != nil {
			h.Shell.FailSSE(sse, r, err)
		} else if err := openUsrDrawer(sse, d); err != nil {
			slog.DebugContext(ctx, "ui: user drawer patch not delivered", "err", err)
		}
	}
	if link != nil {
		if err := patchDialogSlot(sse, usrSecretSlot, link); err != nil {
			slog.DebugContext(ctx, "ui: sign-in link dialog not delivered", "err", err)
		}
	}
	if err := usrFlash(sse, flashUsrCreated, c.Detail); err != nil {
		slog.DebugContext(ctx, "ui: user flash not delivered", "err", err)
	}
}

// ── edit ───────────────────────────────────────────────────────────────

// usrEditForm is the info card's edit form (the SPA's UserDetailBody): the
// name, and for a platform administrator the type and client.
type usrEditForm struct {
	usrBase
	UserID, Subtitle string
	Name             string
	// Scope and ClientID are the association; CanScope whether the form
	// offers them (anchor only).
	Scope, ClientID string
	CanScope        bool
	Clients         []kit.Option
	CanPickClient   bool
	Open            bool
}

func newUsrEditForm(canScope bool) usrEditForm {
	return usrEditForm{usrBase: newUsrBase(usrFormDialog, "name", "scope", "clientId"), CanScope: canScope}
}

func (f usrEditForm) dialogProps() kit.FormDialogProps {
	return kit.FormDialogProps{
		ID: usrFormDialog, Signal: usrFormSignal, Action: usrURL(f.UserID) + "/edit", Open: f.Open, Error: f.Errs.Form,
		Title: "Edit User", Description: f.Subtitle, SubmitLabel: "Save", RequireChange: true, Hidden: usrHidden(),
	}
}

func (f usrEditForm) fields() []templ.Component {
	out := []templ.Component{usrEditField(f, "name")}
	if f.CanScope {
		out = append(out, usrEditField(f, "scope"), usrEditField(f, "clientId"))
	}
	return out
}

func (f usrEditForm) component() templ.Component { return usrEditDialogC(f) }

// scopeSignals declares the type the client field follows. The scope is one of
// three constants, never request text.
func (f usrEditForm) scopeSignals() string {
	scope := "CLIENT"
	if slices.Contains([]string{"ANCHOR", "PARTNER", "CLIENT"}, f.Scope) {
		scope = f.Scope
	}
	return "{usrScope: '" + scope + "'}"
}

func (f usrEditForm) clientLabel() string {
	if f.Scope == "PARTNER" {
		return "Client to grant"
	}
	return "Client"
}

func (h *Users) editFormFor(r *http.Request, ac *auth.AuthContext, d usrDetail) usrEditForm {
	f := newUsrEditForm(ac.IsAnchor())
	f.UserID, f.Subtitle, f.Name, f.Scope, f.ClientID = d.ID, d.Email, d.Name, d.Scope, d.ClientID
	h.editChoices(r, ac, &f)
	return f
}

// editChoices loads the clients the select offers: the SPA lists every client
// (clientsApi.list(), the permission to view clients).
func (h *Users) editChoices(r *http.Request, ac *auth.AuthContext, f *usrEditForm) {
	if !f.CanScope {
		return
	}
	f.CanPickClient = auth.CanReadClients(ac) == nil && h.Clients != nil
	if !f.CanPickClient {
		return
	}
	all, err := h.Clients.FindAll(r.Context())
	if err != nil {
		slog.WarnContext(r.Context(), "ui: clients unavailable", "err", err)
		f.CanPickClient = false
		return
	}
	slices.SortFunc(all, func(a, b client.Client) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	for i := range all {
		f.Clients = append(f.Clients, kit.Option{Value: string(all[i].ID), Label: all[i].Name + " (" + all[i].Identifier + ")"})
	}
}

// editForm opens the edit dialog (the row's pencil, the drawer's Edit). The
// SPA loads the user (read) and saves the name (update) and the association
// (anchor).
func (h *Users) editForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadPrincipals(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if auth.CanUpdatePrincipals(ac) != nil && !ac.IsAnchor() {
		h.Shell.Fail(w, r, auth.CanUpdatePrincipals(ac))
		return
	}
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	d, err := h.buildDetail(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := h.editFormFor(r, ac, d)
	f.Open = true
	h.showDialog(w, r, ac, id, f, http.StatusOK)
}

// update saves the name (PUT /api/principals/{id}) and, when the type or the
// client changed, the association (PUT .../client-association, anchor only),
// as the SPA does. The association is checked before anything is written, so a
// refused combination changes nothing.
func (h *Users) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	f := newUsrEditForm(ac.IsAnchor())
	f.UserID = id
	f.Name = strings.TrimSpace(r.PostFormValue("name"))
	f.Scope, f.ClientID = strings.TrimSpace(r.PostFormValue("scope")), strings.TrimSpace(r.PostFormValue("clientId"))
	fail := func() {
		h.editChoices(r, ac, &f)
		f.Open = true
		h.formFailed(w, r, ac, id, f)
	}
	if err := auth.CanReadPrincipals(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	current, err := h.Users.GetPrincipal(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Subtitle = deref(current.Email)
	if f.Name == "" {
		f.Errs.field("name", "Name is required.")
	}
	// The association: a posted type that differs from the stored one, or a
	// different client.
	var assoc *principalapi.ClientAssociationRequest
	if f.CanScope && f.Scope != "" {
		if scopeChanged, clientChanged := f.Scope != current.Scope, f.ClientID != deref(current.ClientID); scopeChanged || clientChanged {
			switch {
			case f.Scope == "ANCHOR":
				assoc = &principalapi.ClientAssociationRequest{ClientID: "*"}
			case (f.Scope == "CLIENT" || f.Scope == "PARTNER") && f.ClientID != "":
				mode := "CHANGE_CLIENT"
				if f.Scope == "PARTNER" {
					mode = "TO_PARTNER"
				}
				assoc = &principalapi.ClientAssociationRequest{ClientID: f.ClientID, Mode: &mode}
			case f.Scope == "CLIENT" || f.Scope == "PARTNER":
				f.Errs.field("clientId", "Select a client for CLIENT/PARTNER scope.")
			}
		}
	}
	// The name needs user:update, the association anchor scope: the API's
	// checks, made here first so a refused change writes nothing.
	if f.Name != current.Name {
		if err := auth.CanUpdatePrincipals(ac); err != nil {
			f.Errs.apply(r, err, nil)
		}
	}
	if assoc != nil {
		if err := auth.RequireAnchor(ac); err != nil {
			f.Errs.apply(r, err, nil)
		}
	}
	if f.Errs.any() {
		fail()
		return
	}
	if f.Name != current.Name {
		if _, err := h.Users.UpdatePrincipal(ctx, id, principalapi.UpdatePrincipalRequest{Name: &f.Name}); err != nil {
			f.Errs.apply(r, err, usrFieldCodes)
			fail()
			return
		}
	}
	if assoc != nil {
		if _, err := h.Users.SetPrincipalClientAssociation(ctx, id, *assoc); err != nil {
			f.Errs.apply(r, err, usrFieldCodes)
			fail()
			return
		}
	}
	h.done(w, r, ac, id, usrFormDialog, flashUsrUpdated, "", false)
}
