package pages

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

// The create and edit dialogs (the SPA's ApplicationCreateDrawer and the
// detail drawer's inline edit), and the provisioning of the service account
// and the login client with their shown-once credentials.

const (
	appFormDialog = "app-form"
	appFormSignal = "appForm"
	appLoginID    = "app-login"
	appCredsID    = "app-credentials"
)

var appFieldCodes = map[string]string{
	"CODE_REQUIRED":       "code",
	"INVALID_CODE_FORMAT": "code",
	"CODE_EXISTS":         "code",
	"NAME_REQUIRED":       "name",
}

var appTypeOptions = []kit.Option{
	{Value: string(application.TypeApplication), Label: "Application"},
	{Value: string(application.TypeIntegration), Label: "Integration"},
}

type appForm struct {
	Edit     bool
	AppID    string
	Subtitle string

	Type, Code, Name, Description string
	BaseURL, IconURL, Website     string
	Logo, LogoMimeType            string

	Errs formErrors
	Open bool
}

func (f appForm) id(key string) string         { return fieldID(appFormDialog, key) }
func (f appForm) fieldError(key string) string { return f.Errs.Fields[key] }

func (f appForm) keys() []string {
	k := []string{"name", "description", "defaultBaseUrl", "iconUrl", "website", "logo", "logoMimeType"}
	if f.Edit {
		return k
	}
	return append([]string{"type", "code"}, k...)
}

func (f appForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.fieldError(k) != "" {
			return f.id(k)
		}
	}
	return ""
}

func (f appForm) fields() []templ.Component {
	out := make([]templ.Component, 0, len(f.keys()))
	for _, k := range f.keys() {
		out = append(out, appField(f, k))
	}
	return out
}

func (f appForm) dialog() kit.FormDialogProps {
	p := kit.FormDialogProps{
		ID: appFormDialog, Signal: appFormSignal, Width: kit.WidthWide, Action: appsPath + "/new", Open: f.Open, Error: f.Errs.Form,
		Title: "Create Application", Description: "Add a new application to the platform", SubmitLabel: "Create Application",
	}
	if f.Edit {
		p.Action = appURL(f.AppID) + "/edit"
		p.Title, p.Description, p.SubmitLabel, p.RequireChange = "Edit Application", f.Subtitle, "Save", true
		p.Hidden = []kit.HiddenField{drawerField(appDrawerSig)}
	}
	return p
}

func submittedAppForm(r *http.Request, edit bool) appForm {
	return appForm{
		Edit: edit, Type: r.PostFormValue("type"), Code: strings.TrimSpace(r.PostFormValue("code")),
		Name: r.PostFormValue("name"), Description: r.PostFormValue("description"),
		BaseURL: strings.TrimSpace(r.PostFormValue("defaultBaseUrl")), IconURL: strings.TrimSpace(r.PostFormValue("iconUrl")),
		Website: strings.TrimSpace(r.PostFormValue("website")), Logo: r.PostFormValue("logo"),
		LogoMimeType: strings.TrimSpace(r.PostFormValue("logoMimeType")),
	}
}

func (h *Applications) createForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanCreateApplications(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showForm(w, r, ac, appForm{Open: true, Type: string(application.TypeApplication)})
}

func (h *Applications) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedAppForm(r, false)
	if err := auth.CanCreateApplications(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	// The SPA's POST body: optional fields only when set.
	newID, err := h.Ops.Create(ctx, appops.CreateCommand{
		Code: f.Code, Name: f.Name, Type: f.Type, Description: optionalText(f.Description),
		DefaultBaseURL: optionalText(f.BaseURL), IconURL: optionalText(f.IconURL), Website: optionalText(f.Website),
		Logo: optionalText(f.Logo), LogoMimeType: optionalText(f.LogoMimeType),
	})
	if err != nil {
		f.Errs.apply(r, err, appFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashAppCreated)
		http.Redirect(w, r, appsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(appFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: application form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, newID, false)
	if a, err := h.loadApplication(ctx, ac, newID); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if d, err := h.detailOf(ctx, ac, a); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := openAppDrawer(sse, d); err != nil {
		slog.DebugContext(ctx, "ui: application drawer patch not delivered", "err", err)
	}
	if err := flashKey(sse, flashAppCreated); err != nil {
		slog.DebugContext(ctx, "ui: application flash not delivered", "err", err)
	}
}

func (h *Applications) editForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanUpdateApplications(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := appID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	a, err := h.loadApplication(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showForm(w, r, ac, appForm{
		Edit: true, AppID: string(a.ID), Subtitle: a.Name + " (" + a.Code + ")", Open: true,
		Name: a.Name, Description: deref(a.Description), BaseURL: deref(a.DefaultBaseURL), IconURL: deref(a.IconURL),
		Website: deref(a.Website), Logo: deref(a.Logo), LogoMimeType: deref(a.LogoMimeType),
	})
}

func (h *Applications) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedAppForm(r, true)
	if err := auth.CanUpdateApplications(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	id, err := appID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	a, err := h.loadApplication(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.AppID, f.Subtitle = string(a.ID), a.Name+" ("+a.Code+")"
	// The SPA's PUT body: the name, the rest `|| undefined` (a blank field
	// is not sent, so the use case keeps the stored value).
	if err := h.Ops.Update(ctx, appops.UpdateCommand{
		ID: id, Name: &f.Name, Description: optionalText(f.Description), DefaultBaseURL: optionalText(f.BaseURL),
		IconURL: optionalText(f.IconURL), Website: optionalText(f.Website), Logo: optionalText(f.Logo), LogoMimeType: optionalText(f.LogoMimeType),
	}); err != nil {
		f.Errs.apply(r, err, appFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashAppUpdated)
		http.Redirect(w, r, appsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(appFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: application form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, id, false)
	if err := flashKey(sse, flashAppUpdated); err != nil {
		slog.DebugContext(ctx, "ui: application flash not delivered", "err", err)
	}
}

func (h *Applications) showForm(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f appForm) {
	if ds.IsDatastar(r) {
		if err := ds.NewSSE(w, r).Patch(appFormDialogC(f), ds.Inner("#"+appDialogSlot)); err != nil {
			slog.DebugContext(r.Context(), "ui: application form patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(r.Context(), ac, parseAppQuery(nil))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Dialog = appFormDialogC(f)
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *Applications) formFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f appForm) {
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, appFormDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	view, err := h.listView(r.Context(), ac, parseAppQuery(nil))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	view.Dialog = appFormDialogC(f)
	h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), view)
}

func (f appForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.id(key), Required: required, Description: help, Error: f.fieldError(key), Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f appForm) describedBy(key string) string {
	if f.fieldError(key) != "" {
		return f.id(key) + "-err"
	}
	return ""
}

func (f appForm) inputProps(key, value, placeholder string, attrs templ.Attributes) kit.InputProps {
	return kit.InputProps{ID: f.id(key), Name: key, Value: value, Placeholder: placeholder, Invalid: f.fieldError(key) != "", DescribedBy: f.describedBy(key), Attrs: attrs}
}

func (f appForm) typeHelpExpr() string {
	return "$appFormType === 'INTEGRATION' ? 'Third-party adapter or connector for integrations' : 'User-facing application that users can log into'"
}

func (f appForm) typeHelp() string {
	if f.Type == string(application.TypeIntegration) {
		return "Third-party adapter or connector for integrations"
	}
	return "User-facing application that users can log into"
}

func (f appForm) signals() string {
	logo := "''"
	if f.Logo != "" {
		logo = "'set'"
	}
	t := "'APPLICATION'"
	if f.Type == string(application.TypeIntegration) {
		t = "'INTEGRATION'"
	}
	return "{appFormType: " + t + ", appFormLogo: " + logo + "}"
}

// ── provisioning ───────────────────────────────────────────────────────

// appLoginForm is the drawer's login-client form.
type appLoginForm struct {
	ClientType   string
	RedirectURIs string
	Errs         formErrors
}

func (f appLoginForm) id(key string) string { return fieldID(appLoginID, key) }

var appClientTypes = []kit.Option{
	{Value: "PUBLIC", Label: "PUBLIC — SPA / native app (PKCE only)"},
	{Value: "CONFIDENTIAL", Label: "CONFIDENTIAL — server-rendered app (has client secret)"},
}

// redirectURIs reads the textarea: one URI per line, blanks and repeats
// dropped (the SPA's chip list).
func redirectURIs(raw string) []string {
	var out []string
	for line := range strings.SplitSeq(raw, "\n") {
		if u := strings.TrimSpace(line); u != "" && !slices.Contains(out, u) {
			out = append(out, u)
		}
	}
	return out
}

// appCredentials is what a provisioning dialog shows once.
type appCredentials struct {
	Title, ClientID, ClientSecret string
	ServiceAccount                string
	ClientType, RedirectURIs      string
	Confidential, LoginClient     bool
}

func (h *Applications) provisionServiceAccount(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// POST /api/applications/{id}/provision-service-account's check.
	if err := auth.RequireAnchor(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := appID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	res, err := h.Ops.ProvisionServiceAccount(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showCredentials(w, r, ac, id, appCredentials{
		Title: "Service Account Provisioned", ClientID: res.OAuthClientID, ClientSecret: res.OAuthClientSecret,
		ServiceAccount: res.ServiceAccountName, Confidential: true,
	})
}

func (h *Applications) provisionLoginClient(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := appID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := appLoginForm{ClientType: r.PostFormValue("clientType"), RedirectURIs: r.PostFormValue("redirectUris")}
	uris := redirectURIs(f.RedirectURIs)
	if len(uris) == 0 {
		f.Errs.field("redirectUris", "At least one redirect URI is required")
	} else {
		res, err := h.Ops.ProvisionLoginClient(ctx, appops.ProvisionLoginClientCommand{ApplicationID: id, ClientType: f.ClientType, RedirectURIs: uris})
		if err == nil {
			h.showCredentials(w, r, ac, id, appCredentials{
				Title: "Login Client Provisioned", ClientID: res.ClientID, ClientSecret: res.ClientSecret,
				ClientType: res.ClientType, RedirectURIs: strings.Join(res.RedirectURIs, ", "),
				Confidential: res.ClientType == "CONFIDENTIAL", LoginClient: true,
			})
			return
		}
		f.Errs.apply(r, err, map[string]string{"REDIRECT_URIS_REQUIRED": "redirectUris"})
	}
	if !ds.IsDatastar(r) {
		h.renderWithDrawer(w, r, ac, id, &f, nil, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity))
		return
	}
	focus := ""
	if f.Errs.Fields["redirectUris"] != "" {
		focus = f.id("redirectUris")
	}
	patchFormFailure(ds.NewSSE(w, r), r, appLoginID, []templ.Component{appLoginField(f, "clientType"), appLoginField(f, "redirectUris")}, f.Errs.Form, focus)
}

// showCredentials answers a provisioning: the credentials dialog (the secret
// is in this response only: never in a URL, a redirect or a flash), the
// refreshed drawer and table. A native post gets the page itself (no
// post/redirect/get, which would lose the secret; responses are no-store).
func (h *Applications) showCredentials(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id string, c appCredentials) {
	ctx := r.Context()
	if !ds.IsDatastar(r) {
		h.renderWithDrawer(w, r, ac, id, nil, appCredentialsDialog(c), http.StatusOK)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.Patch(appCredentialsDialog(c), ds.Inner("#"+appDialogSlot)); err != nil {
		slog.DebugContext(ctx, "ui: credentials dialog not delivered", "err", err)
	}
	if view, err := h.listView(ctx, ac, refererList(r, appList)); err == nil {
		if err := view.patchTable(sse); err != nil {
			slog.DebugContext(ctx, "ui: applications table patch not delivered", "err", err)
		}
	}
	h.refreshDrawer(sse, r, ac, id, nil)
}

func (h *Applications) renderWithDrawer(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id string, login *appLoginForm, dialog templ.Component, status int) {
	ctx := r.Context()
	view, err := h.listView(ctx, ac, parseAppQuery(nil))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	a, err := h.loadApplication(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d, err := h.detailOf(ctx, ac, a)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if login != nil {
		d.LoginForm = *login
	}
	view.Drawer, view.Dialog = &d, dialog
	h.renderPage(w, r, status, view)
}

func appNameAttrs(edit bool) templ.Attributes {
	a := templ.Attributes{"required": true, "maxlength": "100", "autocomplete": "off"}
	if edit {
		a["autofocus"] = true
	}
	return a
}

func cmpOrString(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func loginDescribedBy(f appLoginForm, key string) string {
	if f.Errs.Fields[key] != "" {
		return f.id(key) + "-err"
	}
	return ""
}

// appCopyExpr copies a credential (by element id) and says so in the
// dialog's status line. The label is a page constant ("Client ID").
func appCopyExpr(id, label string) string {
	return "navigator.clipboard.writeText(document.getElementById('" + kit.ID(id) + "').textContent).then(() => $appCopied = '" + label + " copied to clipboard')"
}
