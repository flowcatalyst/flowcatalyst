package pages

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

// The create and edit dialogs (the SPA's New / Edit Portal App dialog) and
// the one-time credentials of the OAuth client a new portal app provisions.

const (
	portalAppFormDialog = "portal-app-form"
	portalAppFormSignal = "portalAppForm"
	portalAppCredsID    = "portal-app-credentials"
)

var portalAppFieldCodes = map[string]string{
	"CODE_INVALID":         "code",
	"CODE_EXISTS":          "code",
	"NAME_REQUIRED":        "name",
	"REDIRECT_URI_INVALID": "redirectUris",
	"INVALID_CLIENT_TYPE":  "clientType",
}

var portalClientTypes = []kit.Option{
	{Value: "CONFIDENTIAL", Label: "Confidential — server-side portal (client secret + PKCE)"},
	{Value: "PUBLIC", Label: "Public — browser-only portal (PKCE, no secret)"},
}

type portalAppForm struct {
	Edit   bool
	Client string
	AppID  string
	// Subtitle names the app being edited.
	Subtitle string

	Name, Code, Description, RedirectURIs, ClientType string
	Active                                            bool

	Errs formErrors
	Open bool
}

func (f portalAppForm) id(key string) string         { return fieldID(portalAppFormDialog, key) }
func (f portalAppForm) fieldError(key string) string { return f.Errs.Fields[key] }

func (f portalAppForm) keys() []string {
	if f.Edit {
		return []string{"name", "code", "description", "active"}
	}
	return []string{"name", "code", "description", "redirectUris", "clientType"}
}

func (f portalAppForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.fieldError(k) != "" {
			return f.id(k)
		}
	}
	return ""
}

func (f portalAppForm) fields() []templ.Component {
	out := make([]templ.Component, 0, len(f.keys()))
	for _, k := range f.keys() {
		out = append(out, portalAppField(f, k))
	}
	return out
}

func (f portalAppForm) dialog() kit.FormDialogProps {
	p := kit.FormDialogProps{
		ID: portalAppFormDialog, Signal: portalAppFormSignal, Width: kit.WidthWide, Action: portalAppsPath + "/new", Open: f.Open, Error: f.Errs.Form,
		Title: "New Portal App", SubmitLabel: "Create",
		Hidden: []kit.HiddenField{{Name: portalClientParam, Value: f.Client}},
	}
	if f.Edit {
		p.Action = portalAppsPath + "/" + url.PathEscape(f.AppID) + "/edit"
		p.Title, p.Description, p.SubmitLabel, p.RequireChange = "Edit Portal App", f.Subtitle, "Save", true
	}
	return p
}

func (f portalAppForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.id(key), Required: required, Description: help, Error: f.fieldError(key), Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f portalAppForm) describedBy(key string) string {
	if f.fieldError(key) != "" {
		return f.id(key) + "-err"
	}
	return ""
}

// submittedPortalAppForm reads the dialog: the SPA trims the name and the
// description and lower-cases the code.
func submittedPortalAppForm(r *http.Request, edit bool) portalAppForm {
	return portalAppForm{
		Edit: edit, Client: strings.TrimSpace(r.PostFormValue(portalClientParam)),
		Name: strings.TrimSpace(r.PostFormValue("name")), Code: strings.ToLower(strings.TrimSpace(r.PostFormValue("code"))),
		Description: strings.TrimSpace(r.PostFormValue("description")), RedirectURIs: r.PostFormValue("redirectUris"),
		ClientType: r.PostFormValue("clientType"), Active: r.PostFormValue("active") != "",
	}
}

func (h *PortalApps) createForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	v, err := h.manageView(r.Context(), ac, r.URL.Query().Get(portalClientParam))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showForm(w, r, v, portalAppForm{Open: true, Client: v.Client, ClientType: "CONFIDENTIAL"})
}

func (h *PortalApps) editForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	v, err := h.manageView(r.Context(), ac, r.URL.Query().Get(portalClientParam))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := portalAppID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	app, ok := v.findApp(id)
	if !ok {
		h.Shell.Fail(w, r, errPortalAppNotFound(id))
		return
	}
	h.showForm(w, r, v, portalAppForm{
		Edit: true, Open: true, Client: v.Client, AppID: app.ID, Subtitle: app.Name + " (" + app.Code + ")",
		Name: app.Name, Code: app.Code, Description: app.Description, Active: app.Active,
	})
}

func (h *PortalApps) showForm(w http.ResponseWriter, r *http.Request, v portalAppsView, f portalAppForm) {
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), portalAppDialogSlot, portalAppFormDialogC(f)); err != nil {
			slog.DebugContext(r.Context(), "ui: portal app form patch not delivered", "err", err)
		}
		return
	}
	v.Dialog = portalAppFormDialogC(f)
	h.renderPage(w, r, http.StatusOK, v)
}

func (h *PortalApps) formFailed(w http.ResponseWriter, r *http.Request, v portalAppsView, f portalAppForm) {
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, portalAppFormDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	f.Open = true
	v.Dialog = portalAppFormDialogC(f)
	h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), v)
}

// failedView is the page behind a refused post: the view for the client the
// form named when the caller may read it, else the bare page (the refusal
// itself is in the form).
func (h *PortalApps) failedView(ctx context.Context, ac *auth.AuthContext, clientID string) portalAppsView {
	if v, err := h.view(ctx, ac, clientID); err == nil {
		return v
	}
	return portalAppsView{}
}

func (h *PortalApps) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedPortalAppForm(r, false)
	v, err := h.manageView(ctx, ac, f.Client)
	if err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, h.failedView(ctx, ac, f.Client), f)
		return
	}
	w = noStore{w}
	// The SPA's POST body: the description only when set, the callbacks one
	// per line.
	uris := redirectURIs(f.RedirectURIs)
	res, err := h.Ops.CreateApp(ctx, portalidentity.CreateAppWithOAuthClientCommand{
		ClientID: f.Client, Code: f.Code, Name: f.Name, Description: optionalText(f.Description),
		RedirectURIs: uris, ClientType: f.ClientType,
	})
	if err != nil {
		f.Errs.apply(r, err, portalAppFieldCodes)
		h.formFailed(w, r, v, f)
		return
	}
	creds := portalAppCredentials{
		AppName: f.Name, AppCode: f.Code, ClientID: res.OAuthClientID, ClientSecret: res.ClientSecret,
		OAuthRowID: res.OAuthClientRowID, Origin: requestOrigin(r), NoCallback: len(uris) == 0,
	}
	// The secret is in this response only: never in a URL, a redirect, a
	// flash or a signal. A native post gets the page itself (post/redirect/get
	// would lose it); the response is no-store.
	if !ds.IsDatastar(r) {
		nv, err := h.view(ctx, ac, f.Client)
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		nv.Dialog = portalAppCredentialsDialog(creds)
		h.renderPage(w, r, http.StatusOK, nv)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := patchDialogSlot(sse, portalAppDialogSlot, portalAppCredentialsDialog(creds)); err != nil {
		slog.DebugContext(ctx, "ui: portal app credentials not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, f.Client)
	if err := portalFlash(sse, flashPortalAppCreated, "Portal app \""+f.Name+"\" created"); err != nil {
		slog.DebugContext(ctx, "ui: portal app flash not delivered", "err", err)
	}
}

func (h *PortalApps) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedPortalAppForm(r, true)
	v, err := h.manageView(ctx, ac, f.Client)
	if err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, h.failedView(ctx, ac, f.Client), f)
		return
	}
	id, err := portalAppID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	app, ok := v.findApp(id)
	if !ok {
		h.Shell.Fail(w, r, errPortalAppNotFound(id))
		return
	}
	f.AppID, f.Subtitle, f.Code = app.ID, app.Name+" ("+app.Code+")", app.Code
	// The SPA's PUT body: name, description (a blank one clears it) and the
	// active flag; the code is fixed.
	if err := h.Ops.UpdateApp(ctx, portalidentity.UpdateAppCommand{
		ClientID: f.Client, ID: id, Name: &f.Name, Description: &f.Description, Active: &f.Active,
	}); err != nil {
		f.Errs.apply(r, err, portalAppFieldCodes)
		h.formFailed(w, r, v, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashPortalAppUpdated)
		http.Redirect(w, r, v.url(), http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(portalAppFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: portal app form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, f.Client)
	if err := portalFlash(sse, flashPortalAppUpdated, "Portal app \""+f.Name+"\" updated"); err != nil {
		slog.DebugContext(ctx, "ui: portal app flash not delivered", "err", err)
	}
}

// ── credentials ────────────────────────────────────────────────────────

// portalAppCredentials is what the dialog after a create shows, once.
type portalAppCredentials struct {
	AppName, AppCode, ClientID, ClientSecret string
	// OAuthRowID is the OAuth client's platform id, for the link to it.
	OAuthRowID string
	// Origin is the platform's origin as the browser addressed it (the
	// SPA's window.location.origin).
	Origin string
	// NoCallback is set when no callback URL was registered.
	NoCallback bool
}

type portalEndpoint struct{ Label, Value string }

func (c portalAppCredentials) endpoints() []portalEndpoint {
	return []portalEndpoint{
		{"Authorization endpoint", c.Origin + "/portal/authorize"},
		{"Token endpoint", c.Origin + "/oauth/token"},
		{"JWKS", c.Origin + "/.well-known/jwks.json"},
		{"Issuer / discovery", c.Origin + "/.well-known/openid-configuration"},
	}
}

// envSnippet is the drop-in environment for the Laravel SDK's portal mode
// (TS/Fastify: portal: true with the same client id and secret).
func (c portalAppCredentials) envSnippet() string {
	lines := []string{
		"FLOWCATALYST_BASE_URL=" + c.Origin,
		"FLOWCATALYST_OIDC_ENABLED=true",
		"FLOWCATALYST_OIDC_PORTAL=true",
		"FLOWCATALYST_OIDC_CLIENT_ID=" + c.ClientID,
	}
	if c.ClientSecret != "" {
		lines = append(lines, "FLOWCATALYST_OIDC_CLIENT_SECRET="+c.ClientSecret)
	}
	lines = append(lines, "# portalAppCode for /api/portal-users: "+c.AppCode)
	return strings.Join(lines, "\n")
}

func (c portalAppCredentials) oauthClientHref() string {
	return "/authentication/oauth-clients/" + url.PathEscape(c.OAuthRowID)
}

// requestOrigin is the origin the browser addressed (its Origin header on
// the post, which a cookie-session post must carry), else the request's own
// scheme and host. It is only shown to the caller who made the request.
func requestOrigin(r *http.Request) string {
	if o, err := url.Parse(r.Header.Get("Origin")); err == nil && (o.Scheme == "https" || o.Scheme == "http") && o.Host != "" {
		return o.Scheme + "://" + o.Host
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// portalCopyExpr copies a credential (by element id) and says so in the
// dialog's status line. The label is a page constant.
func portalCopyExpr(id, label string) string {
	return "navigator.clipboard.writeText(document.getElementById('" + kit.ID(id) + "').textContent).then(() => $portalCopied = '" + label + " copied to clipboard')"
}
