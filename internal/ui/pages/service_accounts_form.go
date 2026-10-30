package pages

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	saops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The create and edit dialogs (the SPA's ServiceAccountCreateDrawer and the
// detail drawer's inline edit), the roles dialog (its role picker), and the
// credential actions with their shown-once dialogs.

const (
	saFormDialog  = "sa-form"
	saFormSignal  = "saForm"
	saRolesDialog = "sa-roles"
	saRolesSignal = "saRoles"
	saCredsID     = "sa-credentials"
)

var saFieldCodes = map[string]string{
	"CODE_REQUIRED":       "code",
	"RESERVED_CODE":       "code",
	"INVALID_CODE_FORMAT": "code",
	"CODE_EXISTS":         "code",
	"NAME_REQUIRED":       "name",
	// The links and the scope are one fact (SCOPE_REACH_MISMATCH names both).
	"SCOPE_REACH_MISMATCH": "clientIds",
}

// saScopeOptions are the SPA's scope choices.
var saScopeOptions = []kit.Option{
	{Value: "ANCHOR", Label: "Anchor (all clients)"},
	{Value: "PARTNER", Label: "Partner (assigned clients)"},
	{Value: "CLIENT", Label: "Client (single client)"},
}

func validSAScope(s string) bool {
	return slices.ContainsFunc(saScopeOptions, func(o kit.Option) bool { return o.Value == s })
}

// saForm is the create or edit form.
type saForm struct {
	Edit     bool
	SAID     string
	Subtitle string

	Name, Code, Description, Scope string
	ClientIDs                      []string
	AllApplications                bool

	Errs formErrors
	Open bool

	// ClientOptions are the clients to link; CanPickClients is false when the
	// user cannot list clients (the options are then the links already held).
	ClientOptions  []kit.Option
	CanPickClients bool
	// CanGrantAll: the user holds all-applications access, so may grant it.
	CanGrantAll bool
	// ClientIDsByIdentifier maps a client identifier to its id, for an import
	// matching by identifier.
	ClientIDsByIdentifier map[string]string

	// Import is the notice of the last Import from JSON; ImportOpen and
	// ImportError render that dialog open (a native post that was refused).
	Import      defImportState
	ImportOpen  bool
	ImportError string
}

func (f saForm) id(key string) string         { return fieldID(saFormDialog, key) }
func (f saForm) fieldError(key string) string { return f.Errs.Fields[key] }

func (f saForm) keys() []string {
	if f.Edit {
		return []string{"name", "description", "scope", "clientIds"}
	}
	return []string{"name", "code", "description", "scope", "clientIds", "allApplications"}
}

func (f saForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.fieldError(k) != "" {
			return f.id(k)
		}
	}
	return ""
}

func (f saForm) fields() []templ.Component {
	out := make([]templ.Component, 0, len(f.keys()))
	for _, k := range f.keys() {
		out = append(out, saField(f, k))
	}
	return out
}

func (f saForm) dialog() kit.FormDialogProps {
	p := kit.FormDialogProps{
		ID: saFormDialog, Signal: saFormSignal, Width: kit.WidthWide, Action: saPath + "/new", Open: f.Open, Error: f.Errs.Form,
		Title: "Create Service Account", Description: "Create a new service account with webhook credentials", SubmitLabel: "Create Service Account",
	}
	if f.Edit {
		p.Action = saURL(f.SAID) + "/edit"
		p.Title, p.Description, p.SubmitLabel, p.RequireChange = "Edit Service Account", f.Subtitle, "Save", true
		p.Hidden = []kit.HiddenField{drawerField(saDrawerSig)}
	}
	return p
}

// signals declares the scope the client field follows and the code the name
// suggests.
// signals seeds the form's signals from its values. The code is set too:
// signals are page-global, so a dialog rendered again (an import fills it in)
// would otherwise bind its code input to the previous dialog's value.
// saFormCodeKept stops the name field from replacing an imported code, as the
// SPA's generateCode does.
func (f saForm) signals() string {
	scope := "ANCHOR"
	if validSAScope(f.Scope) {
		scope = f.Scope
	}
	b, _ := json.Marshal(map[string]any{
		"saFormScope":    scope,
		"saFormCode":     f.Code,
		"saFormCodeKept": f.Import.Done && f.Code != "",
	})
	return string(b)
}

func (f saForm) clientsShown() bool { return f.Scope != "ANCHOR" && f.Scope != "" }

func (f saForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.id(key), Required: required, Description: help, Error: f.fieldError(key), Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f saForm) describedBy(key string) string {
	if f.fieldError(key) != "" {
		return f.id(key) + "-err"
	}
	return ""
}

func (f saForm) inputProps(key, value, placeholder string, attrs templ.Attributes) kit.InputProps {
	return kit.InputProps{ID: f.id(key), Name: key, Value: value, Placeholder: placeholder, Invalid: f.fieldError(key) != "", DescribedBy: f.describedBy(key), Attrs: attrs}
}

// codeFromName is the SPA's generateCode (run when the name field loses
// focus, unless the code was imported): lowercase, spaces to dashes, only [a-z0-9-], dashes squeezed and
// trimmed.
const saCodeFromName = "el.value && !$saFormCodeKept && ($saFormCode = el.value.toLowerCase().replace(/\\s+/g, '-').replace(/[^a-z0-9-]/g, '').replace(/-+/g, '-').replace(/^-|-$/g, ''))"

func submittedSAForm(r *http.Request, edit bool) saForm {
	return saForm{
		Edit: edit, Name: strings.TrimSpace(r.PostFormValue("name")), Code: strings.TrimSpace(r.PostFormValue("code")),
		Description: r.PostFormValue("description"), Scope: r.PostFormValue("scope"),
		ClientIDs: saPosted(r.PostForm["clientIds"]), AllApplications: r.PostFormValue("allApplications") == "true",
	}
}

// fill adds the options the form renders: the clients (the SPA's
// clientsApi.list, which needs the client list; without it only the links the
// account already has).
func (h *ServiceAccounts) fill(ctx context.Context, ac *auth.AuthContext, f *saForm) {
	clients := h.clients(ctx, ac)
	f.CanPickClients = auth.CanReadClients(ac) == nil
	f.CanGrantAll = ac.AllApplications
	f.ClientOptions = clients.options
	f.ClientIDsByIdentifier = clients.byIdentifier
	have := make(map[string]bool, len(clients.options))
	for _, o := range clients.options {
		have[o.Value] = true
	}
	for _, id := range f.ClientIDs {
		if !have[id] {
			f.ClientOptions = append(f.ClientOptions, kit.Option{Value: id, Label: id})
		}
	}
}

func (h *ServiceAccounts) showForm(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f saForm) {
	h.fill(r.Context(), ac, &f)
	h.showDialog(w, r, ac, saFormDialogC(f))
}

// showDialog puts a dialog into the slot (Datastar) or renders the list with
// it open.
func (h *ServiceAccounts) showDialog(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, dialog templ.Component) {
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), saDialogSlot, dialog); err != nil {
			slog.DebugContext(r.Context(), "ui: service account dialog patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, saList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Dialog = dialog
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *ServiceAccounts) formFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f saForm) {
	h.fill(r.Context(), ac, &f)
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, saFormDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, saList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	view.Dialog = saFormDialogC(f)
	h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), view)
}

func (h *ServiceAccounts) createForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := saops.AuthorizeCreate(ac, false, nil); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showForm(w, r, ac, saForm{Open: true, Scope: "ANCHOR"})
}

func (h *ServiceAccounts) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedSAForm(r, false)
	// POST /api/service-accounts's checks: the create permission, anchor
	// scope, the application confinement, and all-applications only from a
	// caller holding it.
	if err := saops.AuthorizeCreate(ac, f.AllApplications, nil); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	if !validSAScope(f.Scope) {
		f.Errs.field("scope", "Choose a scope.")
		h.formFailed(w, r, ac, f)
		return
	}
	// The SPA's POST body: optional fields only when set; no clients for an
	// anchor account (the platform refuses a scope that disagrees with the
	// links).
	cmd := saops.CreateCommand{Code: f.Code, Name: f.Name, Description: optionalText(f.Description), Scope: &f.Scope}
	if f.Scope != "ANCHOR" && len(f.ClientIDs) > 0 {
		cmd.ClientIDs = f.ClientIDs
	}
	if f.AllApplications {
		cmd.AllApplications = &f.AllApplications
	}
	res, err := h.Ops.Create(ctx, cmd)
	if err != nil {
		f.Errs.apply(r, err, saFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	h.showCredentials(w, r, ac, res.ServiceAccount.ID, saCredentials{
		Title: "Service Account Created", Persistent: true, Warning: "Copy these credentials now. They will not be shown again.",
		Groups: []saCredGroup{
			{
				Title: "OAuth Credentials (API Authentication)", Description: "Use these for client_credentials grant to obtain access tokens.",
				Items: []saCredItem{{Label: "Client ID", Value: res.OAuthClientID}, {Label: "Client Secret", Value: res.OAuthClientSecret}},
			},
			{
				Title: "Webhook Credentials", Description: "Use these for outbound webhook authentication and signature verification.",
				Items: []saCredItem{
					{Label: "Auth Token (Bearer)", Value: res.AuthToken, Help: "Sent in the Authorization header: Authorization: Bearer <token>"},
					{Label: "Signing Secret", Value: res.SigningSecret, Help: "Used to verify webhook signatures (HMAC-SHA256)"},
				},
			},
		},
	}, flashSACreated)
}

func (h *ServiceAccounts) editForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := saID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	sa, err := h.writable(ctx, ac, id, auth.CanUpdateServiceAccounts)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showForm(w, r, ac, saForm{
		Edit: true, SAID: sa.ID, Subtitle: sa.Name + " (" + sa.Code + ")", Open: true,
		Name: sa.Name, Description: deref(sa.Description), Scope: saScopeOrDerived(sa), ClientIDs: slices.Clone(sa.ClientIDs),
	})
}

// saScopeOrDerived is the scope the edit form starts from: the stored one, or
// the tier the client links give an account with none stored (so saving the
// form does not clear the links).
func saScopeOrDerived(sa *serviceaccount.ServiceAccount) string {
	if s := deref(sa.Scope); validSAScope(s) {
		return s
	}
	switch len(sa.ClientIDs) {
	case 0:
		return "ANCHOR"
	case 1:
		return "CLIENT"
	}
	return "PARTNER"
}

func (h *ServiceAccounts) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedSAForm(r, true)
	id, err := saID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	sa, err := h.writable(ctx, ac, id, auth.CanUpdateServiceAccounts)
	if err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	f.SAID, f.Subtitle = sa.ID, sa.Name+" ("+sa.Code+")"
	if !validSAScope(f.Scope) {
		f.Errs.field("scope", "Choose a scope.")
		h.formFailed(w, r, ac, f)
		return
	}
	// The SPA's PUT body: the name, the description `|| undefined` (a blank
	// one is not sent), the scope and the links (an anchor has none: the
	// platform refuses a scope that disagrees with them).
	clientIDs := []string{}
	if f.Scope != "ANCHOR" {
		clientIDs = f.ClientIDs
	}
	if err := h.Ops.Update(ctx, saops.UpdateCommand{
		ID: id, Name: &f.Name, Description: optionalText(f.Description), Scope: &f.Scope, ClientIDs: clientIDs,
	}); err != nil {
		f.Errs.apply(r, err, saFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashSAUpdated)
		http.Redirect(w, r, saPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(saFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: service account form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, id, false, false)
	if err := flashKey(sse, flashSAUpdated); err != nil {
		slog.DebugContext(ctx, "ui: service account flash not delivered", "err", err)
	}
}

// ── roles ──────────────────────────────────────────────────────────────

// saRolesForm is the roles dialog (the SPA's Manage Roles picker). Roles
// holding platform permissions the user lacks are locked, as the SPA's
// picker locks them (the use case refuses adding or removing them, 403
// ROLE_ABOVE_CALLER): they cannot be offered, and the ones the account holds
// are posted back unchanged.
type saRolesForm struct {
	SAID, Subtitle string
	Options        []kit.Option
	Selected       []string
	Locked         []string
	Errs           formErrors
	Open           bool
}

func (f saRolesForm) dialog() kit.FormDialogProps {
	return kit.FormDialogProps{
		ID: saRolesDialog, Signal: saRolesSignal, Width: kit.WidthWide, Action: saURL(f.SAID) + "/roles", Open: f.Open, Error: f.Errs.Form,
		Title: "Manage Roles", Description: f.Subtitle, SubmitLabel: "Save Roles", RequireChange: true,
		Hidden: []kit.HiddenField{drawerField(saDrawerSig)},
	}
}

// rolesFor builds the dialog from the role list and the account's roles.
func (h *ServiceAccounts) rolesFor(ctx context.Context, ac *auth.AuthContext, sa *serviceaccount.ServiceAccount) (saRolesForm, error) {
	f := saRolesForm{SAID: sa.ID, Subtitle: sa.Name + " (" + sa.Code + ")", Open: true}
	roles, err := h.Roles.FindAll(ctx)
	if err != nil {
		return f, usecase.Internal("REPO", "find roles failed", err)
	}
	p, err := h.Principals.FindByServiceAccount(ctx, sa.ID)
	if err != nil {
		return f, usecase.Internal("REPO", "find_by_service_account failed", err)
	}
	var held []string
	if p != nil {
		for _, ra := range p.Roles {
			held = append(held, ra.Role.String())
		}
	}
	known := make(map[string]bool, len(roles))
	for _, rl := range roles {
		known[rl.Name] = true
		above := len(role.PermissionsAboveCaller(ac, rl.Permissions)) > 0
		switch {
		case above && slices.Contains(held, rl.Name):
			f.Locked = append(f.Locked, rl.Name)
		case !above:
			f.Options = append(f.Options, kit.Option{Value: rl.Name, Label: cmpOrString(rl.DisplayName, rl.Name) + " (" + rl.Name + ")"})
			if slices.Contains(held, rl.Name) {
				f.Selected = append(f.Selected, rl.Name)
			}
		}
	}
	// A held role that is no longer defined stays on the list, removable.
	for _, name := range held {
		if !known[name] {
			f.Options = append(f.Options, kit.Option{Value: name, Label: name})
			f.Selected = append(f.Selected, name)
		}
	}
	slices.SortFunc(f.Options, func(a, b kit.Option) int { return strings.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label)) })
	return f, nil
}

// rolesAllowed is the gate of the roles dialog and its save: the update
// permission with anchor reach and application confinement, and the role
// list (GET /api/roles) the picker is built from.
func (h *ServiceAccounts) rolesAllowed(ctx context.Context, ac *auth.AuthContext, id string) (*serviceaccount.ServiceAccount, error) {
	sa, err := h.writable(ctx, ac, id, auth.CanUpdateServiceAccounts)
	if err != nil {
		return nil, err
	}
	if !auth.CanReadAllRoles(ac) {
		return nil, usecase.Authorization("PERMISSION_DENIED", "Changing roles needs permission to view roles.")
	}
	return sa, nil
}

func (h *ServiceAccounts) rolesForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := saID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	sa, err := h.rolesAllowed(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f, err := h.rolesFor(ctx, ac, sa)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showDialog(w, r, ac, saRolesDialogC(f))
}

// saveRoles replaces the account's roles (PUT /api/service-accounts/{id}/roles;
// the role ceiling is the use case's).
func (h *ServiceAccounts) saveRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := saID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	sa, err := h.rolesAllowed(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := h.Ops.AssignRoles(ctx, id, saPosted(r.PostForm["roles"])); err != nil {
		h.rolesFailed(w, r, ac, sa, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashSARoles)
		http.Redirect(w, r, saPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(saRolesDialog); err != nil {
		slog.DebugContext(ctx, "ui: roles dialog close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, id, false, true)
	if err := flashKey(sse, flashSARoles); err != nil {
		slog.DebugContext(ctx, "ui: service account flash not delivered", "err", err)
	}
}

// rolesFailed answers a refused save inside the dialog (the use case's
// message, such as ROLE_ABOVE_CALLER naming the roles).
func (h *ServiceAccounts) rolesFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, sa *serviceaccount.ServiceAccount, err error) {
	f, ferr := h.rolesFor(r.Context(), ac, sa)
	if ferr != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// The dialog keeps what the user ticked.
	f.Selected = slices.DeleteFunc(saPosted(r.PostForm["roles"]), func(v string) bool { return slices.Contains(f.Locked, v) })
	f.Errs.apply(r, err, nil)
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, saRolesDialog, nil, f.Errs.Form, "")
		return
	}
	view, lerr := h.listView(r.Context(), ac, parseListQuery(nil, saList))
	if lerr != nil {
		h.Shell.Fail(w, r, lerr)
		return
	}
	view.Dialog = saRolesDialogC(f)
	h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), view)
}

// ── credentials ───────────────────────────────────────────────────────

// saCredentials is what a shown-once dialog carries: the credentials of one
// action. They exist in the response that carries the dialog only: never in
// a URL, a signal, a redirect or a flash.
type saCredentials struct {
	Title, Warning string
	// Persistent: Escape and the mask do not close the dialog (the SPA's
	// create dialog is not closable); only its button does.
	Persistent bool
	Groups     []saCredGroup
}

type saCredGroup struct {
	Title, Description string
	Items              []saCredItem
}

type saCredItem struct {
	Label, Value, Help string
}

// showCredentials answers an action that produced credentials: the dialog,
// the refreshed table and drawer (an opened one for a new account) and the
// flash. A native post gets the page itself (post/redirect/get would lose
// the credentials). Every such response is no-store.
func (h *ServiceAccounts) showCredentials(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id string, c saCredentials, flash string) {
	ctx := r.Context()
	// The stream helper sets no-cache when it starts: noStore stamps the
	// header on the way out.
	w = noStore{w}
	dialog := saCredentialsDialog(c)
	if !ds.IsDatastar(r) {
		view, err := h.listView(ctx, ac, parseListQuery(nil, saList))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		d, err := h.drawerFor(ctx, ac, id)
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		view.Drawer, view.Dialog = &d, dialog
		h.renderPage(w, r, http.StatusOK, view)
		return
	}
	sse := ds.NewSSE(w, r)
	// Replacing the slot also replaces the create form it may hold.
	if err := patchDialogSlot(sse, saDialogSlot, dialog); err != nil {
		slog.DebugContext(ctx, "ui: credentials dialog not delivered", "err", err)
	}
	if view, err := h.listView(ctx, ac, refererList(r, saList)); err == nil {
		if err := view.patchTable(sse); err != nil {
			slog.DebugContext(ctx, "ui: service accounts table patch not delivered", "err", err)
		}
	}
	if d, err := h.drawerFor(ctx, ac, id); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if r.PostFormValue("drawer") == id {
		if err := sse.Patch(saDrawerContent(d), ds.Inner("#"+saDrawerID)); err != nil {
			slog.DebugContext(ctx, "ui: service account drawer patch not delivered", "err", err)
		}
	} else if err := openSADrawer(sse, d); err != nil {
		slog.DebugContext(ctx, "ui: service account drawer patch not delivered", "err", err)
	}
	if flash != "" {
		if err := flashKey(sse, flash); err != nil {
			slog.DebugContext(ctx, "ui: service account flash not delivered", "err", err)
		}
	}
}

func (h *ServiceAccounts) regenerateToken(w http.ResponseWriter, r *http.Request) {
	h.credentialAction(w, r, flashSATokenRegenerated, func(ctx context.Context, id string) (saCredentials, error) {
		token, err := h.Ops.RegenerateToken(ctx, id)
		if err != nil {
			return saCredentials{}, err
		}
		return saCredentials{
			Title: "New Auth Token", Warning: "Copy this token now. It will not be shown again.",
			Groups: []saCredGroup{{Items: []saCredItem{{Label: "Auth Token", Value: token}}}},
		}, nil
	})
}

func (h *ServiceAccounts) regenerateSecret(w http.ResponseWriter, r *http.Request) {
	h.credentialAction(w, r, flashSASecretRegenerated, func(ctx context.Context, id string) (saCredentials, error) {
		secret, err := h.Ops.RegenerateSecret(ctx, id)
		if err != nil {
			return saCredentials{}, err
		}
		return saCredentials{
			Title: "New Signing Secret", Warning: "Copy this secret now. It will not be shown again.",
			Groups: []saCredGroup{{Items: []saCredItem{{Label: "Signing Secret", Value: secret}}}},
		}, nil
	})
}

// mintToken is POST /api/service-accounts/{id}/token: a live bearer with the
// account's full permissions, shown once; the mint is audited and bounded by
// the role ceiling (MintAdminToken).
func (h *ServiceAccounts) mintToken(w http.ResponseWriter, r *http.Request) {
	h.credentialAction(w, r, "", func(ctx context.Context, id string) (saCredentials, error) {
		minted, err := h.Ops.MintToken(ctx, id)
		if err != nil {
			return saCredentials{}, err
		}
		return saCredentials{
			Title:   "Bearer Token",
			Warning: "This token carries the service account's full permissions and expires in 1 hour. Copy it now — it will not be shown again.",
			Groups:  []saCredGroup{{Items: []saCredItem{{Label: "Bearer Token", Value: minted.AccessToken}}}},
		}, nil
	})
}

// credentialAction is the shared route of the three credential actions: the
// update permission with anchor reach and application confinement, the
// action, and the dialog.
func (h *ServiceAccounts) credentialAction(w http.ResponseWriter, r *http.Request, flash string, run func(context.Context, string) (saCredentials, error)) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := saID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if _, err := h.writable(ctx, ac, id, auth.CanUpdateServiceAccounts); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	c, err := run(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showCredentials(w, r, ac, id, c, flash)
}

// saCredID is the id of the credential at group gi, item ii (the copy
// buttons read it by id).
func saCredID(gi, ii int) string { return "sa-cred-" + strconv.Itoa(gi) + "-" + strconv.Itoa(ii) }

// saCopyExpr copies a credential (by element id) and says so in the dialog's
// status line. The label is a page constant ("Client ID").
func saCopyExpr(id, label string) string {
	return "navigator.clipboard.writeText(document.getElementById('" + kit.ID(id) + "').textContent).then(() => $saCopied = '" + label + " copied to clipboard')"
}

// saCredentialsAttrs are the credentials dialog's attributes: it opens as a
// modal, removes itself when it closes, and the create dialog also reopens
// when closed by anything but its button (see saCredentialsDialog).
func saCredentialsAttrs(persistent bool) templ.Attributes {
	a := templ.Attributes{"open": true, "data-init": "el.removeAttribute('open'); el.showModal()", "data-signals": "{saCopied: ''}"}
	if persistent {
		a["data-on:cancel"] = "evt.preventDefault()"
		a["data-on:close"] = "el.dataset.saved || el.showModal()"
	} else {
		a["data-on:close"] = "el.remove()"
	}
	return a
}
