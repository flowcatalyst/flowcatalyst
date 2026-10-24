package pages

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

// The create and edit dialogs (the SPA's OAuthClientCreateDrawer and the
// detail drawer's inline edit), secret rotation, and the shown-once dialogs.

const (
	ocFormDialog = "oc-form"
	ocFormSignal = "ocForm"
	ocCredsID    = "oc-credentials"

	// ocPortalChange re-fetches the portal app select for the chosen owner
	// client (the SPA filters its app list by the client).
	ocPortalChange = "@get('/ui/oauth-clients/portal-app-options?portalClientId=' + encodeURIComponent(el.value))"
)

var ocFieldCodes = map[string]string{
	"CLIENT_NAME_REQUIRED":              "clientName",
	"INVALID_CLIENT_TYPE":               "clientType",
	"PORTAL_API_ACCESS_CONFLICT":        "apiAccess",
	"PORTAL_APP_REQUIRES_PORTAL_CLIENT": "portalAppId",
	"PORTAL_APP_CLIENT_MISMATCH":        "portalAppId",
}

// The SPA's option lists.
var (
	ocTypeOptions = []kit.Option{
		{Value: string(platformauth.OAuthClientPublic), Label: "Public (SPA, Mobile)"},
		{Value: string(platformauth.OAuthClientConfidential), Label: "Confidential (Server)"},
	}
	ocGrantOptions = []kit.Option{
		{Value: "authorization_code", Label: "Authorization Code"},
		{Value: "refresh_token", Label: "Refresh Token"},
		{Value: "client_credentials", Label: "Client Credentials"},
	}
	ocScopeOptions = []kit.Option{
		{Value: "openid", Label: "openid"},
		{Value: "profile", Label: "profile"},
		{Value: "email", Label: "email"},
		{Value: "offline_access", Label: "offline_access"},
	}
)

// ocForm is the create or edit form. The three URL lists are textareas, one
// per line; the *Text fields hold what was typed so a failed submit gives it
// back unchanged.
type ocForm struct {
	Edit     bool
	OCID     string
	Subtitle string

	Name, Type                                string
	RedirectText, PostLogoutText, OriginsText string
	Redirects, PostLogout, Origins            []string
	Grants, Scopes, Apps                      []string
	PKCE, APIAccess                           bool
	PortalClient, PortalApp                   string

	Errs formErrors
	Open bool

	// The options: the fixed lists plus whatever the client already holds, so
	// saving never drops a value the form cannot offer. CanPickApps and
	// CanPickClients are false when the user cannot list them (only the held
	// ones are then offered).
	GrantOptions, ScopeOptions []kit.Option
	AppOptions                 []kit.Option
	ClientOptions              []kit.Option
	PortalAppOptions           []kit.Option
	CanPickApps                bool
	CanPickClients             bool
}

func (f ocForm) id(key string) string         { return fieldID(ocFormDialog, key) }
func (f ocForm) fieldError(key string) string { return f.Errs.Fields[key] }

func (f ocForm) keys() []string {
	k := []string{"clientName", "redirectUris", "postLogoutRedirectUris", "allowedOrigins", "grantTypes", "defaultScopes", "pkceRequired", "applicationIds", "portalClientId", "portalAppId", "apiAccess"}
	if f.Edit {
		return k
	}
	return slices.Insert(k, 1, "clientType")
}

func (f ocForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.fieldError(k) != "" {
			return f.id(k)
		}
	}
	return ""
}

func (f ocForm) fields() []templ.Component {
	out := make([]templ.Component, 0, len(f.keys()))
	for _, k := range f.keys() {
		out = append(out, ocField(f, k))
	}
	return out
}

func (f ocForm) dialog() kit.FormDialogProps {
	p := kit.FormDialogProps{
		ID: ocFormDialog, Signal: ocFormSignal, Width: kit.WidthWide, Action: ocPath + "/new", Open: f.Open, Error: f.Errs.Form,
		Title: "Create OAuth Client", Description: "Register a new OAuth2/OIDC client for applications that use FlowCatalyst as their identity provider",
		SubmitLabel: "Create OAuth Client",
	}
	if f.Edit {
		p.Action = ocURL(f.OCID) + "/edit"
		p.Title, p.Description, p.SubmitLabel, p.RequireChange = "Edit OAuth Client", f.Subtitle, "Save", true
		p.Hidden = []kit.HiddenField{drawerField(ocDrawerSig)}
	}
	return p
}

// signals declares the client type (the PKCE switch follows it) and the portal
// owner (API access follows it); both are ids or enum values the server has
// checked.
func (f ocForm) signals() string {
	typ := string(platformauth.OAuthClientPublic)
	if f.Type == string(platformauth.OAuthClientConfidential) {
		typ = f.Type
	}
	portal := ""
	if kit.ValidID(f.PortalClient) {
		portal = f.PortalClient
	}
	return "{ocFormType: '" + typ + "', ocFormPortal: '" + portal + "'}"
}

func (f ocForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.id(key), Required: required, Description: help, Error: f.fieldError(key), Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f ocForm) describedBy(key string) string {
	if f.fieldError(key) != "" {
		return f.id(key) + "-err"
	}
	return ""
}

func (f ocForm) textareaProps(key, value, placeholder string) kit.TextareaProps {
	return kit.TextareaProps{ID: f.id(key), Name: key, Value: value, Placeholder: placeholder, Rows: 3, Invalid: f.fieldError(key) != "", DescribedBy: f.describedBy(key), Attrs: templ.Attributes{"autocomplete": "off", "spellcheck": "false"}}
}

// holding adds the values the client already has to an option list.
func holding(opts []kit.Option, held []string, label func(string) string) []kit.Option {
	for _, v := range held {
		if !slices.ContainsFunc(opts, func(o kit.Option) bool { return o.Value == v }) {
			opts = append(opts, kit.Option{Value: v, Label: label(v)})
		}
	}
	return opts
}

func plainLabel(s string) string { return s }

// ocLines is a textarea's lines: trimmed, without blanks and repeats.
func ocLines(raw string) []string {
	out := []string{}
	for line := range strings.SplitSeq(raw, "\n") {
		if line = strings.TrimSpace(line); line != "" && !slices.Contains(out, line) {
			out = append(out, line)
		}
	}
	return out
}

func submittedOCForm(r *http.Request, edit bool) ocForm {
	return ocForm{
		Edit: edit, Name: strings.TrimSpace(r.PostFormValue("clientName")), Type: r.PostFormValue("clientType"),
		RedirectText: r.PostFormValue("redirectUris"), PostLogoutText: r.PostFormValue("postLogoutRedirectUris"), OriginsText: r.PostFormValue("allowedOrigins"),
		Grants: saPosted(r.PostForm["grantTypes"]), Scopes: saPosted(r.PostForm["defaultScopes"]), Apps: saPosted(r.PostForm["applicationIds"]),
		PKCE: r.PostFormValue("pkceRequired") == "true", APIAccess: r.PostFormValue("apiAccess") == "true",
		PortalClient: strings.TrimSpace(r.PostFormValue("portalClientId")), PortalApp: strings.TrimSpace(r.PostFormValue("portalAppId")),
	}
}

// validate holds the SPA's client-side checks as field errors: the name, the
// grant types, the redirect URIs (required for the create form, and on edit
// for the authorization_code and refresh_token grants) and the URL shapes.
// Origins are stored as scheme and host, as the SPA stores url.origin.
func (f *ocForm) validate() {
	f.Redirects, f.PostLogout, f.Origins = ocLines(f.RedirectText), ocLines(f.PostLogoutText), ocLines(f.OriginsText)
	// PKCE is always on for a public client (its checkbox is disabled, so it
	// is not posted).
	if f.Type == string(platformauth.OAuthClientPublic) {
		f.PKCE = true
	}
	if f.Name == "" {
		f.Errs.field("clientName", "Client name is required")
	}
	if !f.Edit && !slices.ContainsFunc(ocTypeOptions, func(o kit.Option) bool { return o.Value == f.Type }) {
		f.Errs.field("clientType", "Choose a client type.")
	}
	if len(f.Grants) == 0 {
		f.Errs.field("grantTypes", "At least one grant type is required")
	}
	switch {
	case !f.Edit && len(f.Redirects) == 0:
		f.Errs.field("redirectUris", "At least one redirect URI is required")
	case f.Edit && len(f.Redirects) == 0 && (slices.Contains(f.Grants, "authorization_code") || slices.Contains(f.Grants, "refresh_token")):
		f.Errs.field("redirectUris", "At least one redirect URI is required for authorization_code or refresh_token grants")
	}
	for _, u := range f.Redirects {
		if !absoluteURL(u) {
			f.Errs.field("redirectUris", "Not a valid URL: "+u)
		}
	}
	for _, u := range f.PostLogout {
		if !absoluteURL(u) {
			f.Errs.field("postLogoutRedirectUris", "Not a valid URL: "+u)
		}
	}
	for i, o := range f.Origins {
		p, err := url.Parse(o)
		switch {
		case err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "":
			f.Errs.field("allowedOrigins", "Not a valid origin: "+o)
		case p.Path != "" && p.Path != "/":
			f.Errs.field("allowedOrigins", "Origin should not include a path (e.g., https://example.com)")
		default:
			f.Origins[i] = p.Scheme + "://" + p.Host
		}
	}
	f.Origins = dedupe(f.Origins)
}

// dedupe drops repeats, keeping the first.
func dedupe(s []string) []string {
	out := make([]string, 0, len(s))
	for _, v := range s {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// ocAppsHelp is the applications field's help, with the reason when the list
// cannot be offered.
func ocAppsHelp(f ocForm) string {
	h := "Only users with access to these applications can authenticate. Leave empty for no restrictions."
	if !f.CanPickApps {
		h += " Listing applications needs permission to view applications; only the current ones are offered."
	}
	return h
}

// ocPortalHelp is the owner client field's help.
func ocPortalHelp(f ocForm) string {
	h := "Marks this OAuth client as a portal entry point for the selected client: only that client's ACTIVE portal users can authenticate through it. Clearing this removes the portal gate."
	if !f.CanPickClients {
		h += " Listing clients needs permission to view clients; only the current owner is offered."
	}
	return h
}

// absoluteURL is the SPA's `new URL(value)`: a scheme is required.
func absoluteURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != ""
}

// fill adds the options the form renders: the applications and clients the
// SPA lists (when the user may list them) and the portal apps of the chosen
// owner.
func (h *OAuthClients) fill(ctx context.Context, ac *auth.AuthContext, f *ocForm) {
	apps := h.apps(ctx, ac)
	f.CanPickApps = apps.canPick
	f.AppOptions = holding(slices.Clone(apps.options), f.Apps, apps.label)
	owners := h.owners(ctx, ac)
	f.CanPickClients = owners.canPick
	f.ClientOptions = owners.options
	if f.PortalClient != "" {
		f.ClientOptions = holding(slices.Clone(f.ClientOptions), []string{f.PortalClient}, owners.label)
	}
	f.GrantOptions = holding(slices.Clone(ocGrantOptions), f.Grants, plainLabel)
	f.ScopeOptions = holding(slices.Clone(ocScopeOptions), f.Scopes, plainLabel)
	h.fillPortalApps(ctx, ac, f)
}

// fillPortalApps offers the apps of the chosen owner client, as the SPA's
// portalAppsFor filter does. The SPA treats the list as optional enrichment:
// a user who may not read the client's portal apps gets only the linked one.
func (h *OAuthClients) fillPortalApps(ctx context.Context, ac *auth.AuthContext, f *ocForm) {
	f.PortalAppOptions = nil
	if f.PortalClient == "" || !kit.ValidID(f.PortalClient) {
		return
	}
	if h.Portal != nil && auth.CanReadPortalUsers(ac, f.PortalClient) == nil {
		apps, err := h.Portal.Apps(ctx, f.PortalClient)
		if err != nil {
			slog.WarnContext(ctx, "ui: portal apps unavailable for oauth clients", "err", err)
		}
		for _, a := range apps {
			f.PortalAppOptions = append(f.PortalAppOptions, kit.Option{Value: a.ID, Label: a.Name + " (" + a.Code + ")"})
		}
	}
	if f.PortalApp != "" {
		f.PortalAppOptions = holding(f.PortalAppOptions, []string{f.PortalApp}, plainLabel)
	}
}

func (h *OAuthClients) showForm(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f ocForm) {
	h.fill(r.Context(), ac, &f)
	h.showDialog(w, r, ac, ocFormDialogC(f))
}

// showDialog puts a dialog into the slot (Datastar) or renders the list with
// it open.
func (h *OAuthClients) showDialog(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, dialog templ.Component) {
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), ocDialogSlot, dialog); err != nil {
			slog.DebugContext(r.Context(), "ui: oauth client dialog patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, ocList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Dialog = dialog
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *OAuthClients) formFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f ocForm) {
	h.fill(r.Context(), ac, &f)
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, ocFormDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, ocList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	view.Dialog = ocFormDialogC(f)
	h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), view)
}

// portalAppOptions answers the owner client select: the portal app select for
// the chosen client, nothing chosen (the SPA's app belongs to the old client).
func (h *OAuthClients) portalAppOptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if auth.CanCreateOAuthClients(ac) != nil {
		if err := auth.CanUpdateOAuthClients(ac); err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
	}
	f := ocForm{PortalClient: r.URL.Query().Get("portalClientId")}
	h.fillPortalApps(ctx, ac, &f)
	if err := ds.NewSSE(w, r).Patch(ocField(f, "portalAppId")); err != nil {
		slog.DebugContext(ctx, "ui: portal app options patch not delivered", "err", err)
	}
}

func (h *OAuthClients) createForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanCreateOAuthClients(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showForm(w, r, ac, ocForm{
		Open: true, Type: string(platformauth.OAuthClientPublic), PKCE: true,
		Grants: []string{"authorization_code", "refresh_token"}, Scopes: []string{"openid", "profile", "email"},
	})
}

func (h *OAuthClients) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedOCForm(r, false)
	// POST /api/oauth-clients's check.
	if err := auth.CanCreateOAuthClients(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	if f.validate(); f.Errs.any() {
		h.formFailed(w, r, ac, f)
		return
	}
	// The SPA's POST body: optional fields only when set; PKCE is always on
	// for a public client; the portal app only with its owner.
	cmd := authops.CreateOAuthClientCommand{
		ClientName: f.Name, ClientType: f.Type, RedirectURIs: f.Redirects, GrantTypes: f.Grants, PKCERequired: &f.PKCE,
	}
	if len(f.PostLogout) > 0 {
		cmd.PostLogoutRedirectURIs = f.PostLogout
	}
	if len(f.Origins) > 0 {
		cmd.AllowedOrigins = f.Origins
	}
	if len(f.Scopes) > 0 {
		cmd.Scopes = f.Scopes
	}
	if len(f.Apps) > 0 {
		cmd.ApplicationIDs = f.Apps
	}
	if f.PortalClient != "" {
		cmd.PortalClientID = &f.PortalClient
		if f.PortalApp != "" {
			cmd.PortalAppID = &f.PortalApp
		}
	}
	if f.APIAccess {
		cmd.APIAccess = &f.APIAccess
	}
	issued, err := h.Ops.Create(ctx, cmd)
	if err != nil {
		f.Errs.apply(r, err, ocFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if issued.Secret == "" {
		// PUBLIC: no secret to acknowledge, so the SPA hands off to the drawer.
		h.createdPublic(w, r, ac, issued.ID)
		return
	}
	h.showCredentials(w, r, ac, issued.ID, ocCredentials{
		Title: "Client Secret Generated", Persistent: true, Warning: "Copy this secret now. It will not be shown again.",
		Items: []ocCredItem{{Label: "Client ID", Value: issued.ClientID}, {Label: "Client Secret (shown once)", Value: issued.Secret}},
	}, flashOCCreated)
}

// createdPublic closes the form and shows the new client's drawer.
func (h *OAuthClients) createdPublic(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id string) {
	ctx := r.Context()
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashOCCreated)
		to := ocPath
		if auth.CanReadOAuthClients(ac) == nil {
			to = ocURL(id)
		}
		http.Redirect(w, r, to, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(ocFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: oauth client form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, id, false)
	if auth.CanReadOAuthClients(ac) == nil {
		if d, err := h.drawerFor(ctx, ac, id); err != nil {
			h.Shell.FailSSE(sse, r, err)
		} else if err := openOCDrawer(sse, d); err != nil {
			slog.DebugContext(ctx, "ui: oauth client drawer patch not delivered", "err", err)
		}
	}
	if err := flashKey(sse, flashOCCreated); err != nil {
		slog.DebugContext(ctx, "ui: oauth client flash not delivered", "err", err)
	}
}

func (h *OAuthClients) editForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := ocID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := firstError(auth.CanReadOAuthClients(ac), auth.CanUpdateOAuthClients(ac)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	c, err := h.loadClient(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := ocForm{
		Edit: true, OCID: c.ID, Subtitle: c.ClientName + " (" + c.ClientID + ")", Open: true,
		Name: c.ClientName, Type: string(c.ClientType),
		RedirectText: strings.Join(c.RedirectURIs, "\n"), PostLogoutText: strings.Join(c.PostLogoutRedirectURIs, "\n"), OriginsText: strings.Join(c.AllowedOrigins, "\n"),
		Grants: slices.Clone(c.GrantTypes), Scopes: slices.Clone(c.Scopes), Apps: slices.Clone(c.ApplicationIDs),
		PKCE: c.PKCERequired, APIAccess: c.APIAccess,
		PortalClient: deref(c.PortalClientID), PortalApp: deref(c.PortalAppID),
	}
	h.showForm(w, r, ac, f)
}

func (h *OAuthClients) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedOCForm(r, true)
	id, err := ocID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.OCID = id
	// PUT /api/oauth-clients/{id}'s check.
	if err := auth.CanUpdateOAuthClients(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	// The type does not change; it decides only whether PKCE is editable.
	c, err := h.Clients.FindByID(ctx, id)
	if err != nil || c == nil {
		if err == nil {
			err = httperror.NotFound("OAuthClient", id)
		}
		h.Shell.Fail(w, r, err)
		return
	}
	f.Type, f.Subtitle = string(c.ClientType), c.ClientName+" ("+c.ClientID+")"
	if f.validate(); f.Errs.any() {
		h.formFailed(w, r, ac, f)
		return
	}
	// The SPA's PUT body: everything, the lists as they stand (an emptied one
	// clears it), the portal owner "" to clear it and the portal app only with
	// its owner. A public client's PKCE stays on.
	portalApp := ""
	if f.PortalClient != "" {
		portalApp = f.PortalApp
	}
	if err := h.Ops.Update(ctx, authops.UpdateOAuthClientCommand{
		ID: id, ClientName: &f.Name, RedirectURIs: f.Redirects, PostLogoutRedirectURIs: f.PostLogout, AllowedOrigins: f.Origins,
		GrantTypes: f.Grants, Scopes: nonNil(f.Scopes), ApplicationIDs: nonNil(f.Apps), PKCERequired: &f.PKCE,
		PortalClientID: &f.PortalClient, PortalAppID: &portalApp, APIAccess: &f.APIAccess,
	}); err != nil {
		f.Errs.apply(r, err, ocFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashOCUpdated)
		http.Redirect(w, r, ocPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(ocFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: oauth client form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, id, false)
	if err := flashKey(sse, flashOCUpdated); err != nil {
		slog.DebugContext(ctx, "ui: oauth client flash not delivered", "err", err)
	}
}

// nonNil is the empty list the use case reads as "clear" (a nil one means
// "leave unchanged").
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ── secrets ────────────────────────────────────────────────────────────

// ocCredentials is what a shown-once dialog carries. It exists in the
// response that carries the dialog only: never in a URL, a signal, a redirect
// or a flash.
type ocCredentials struct {
	Title, Warning string
	// Persistent: Escape and the mask do not close the dialog (the SPA's
	// create dialog is not closable); only its button does.
	Persistent bool
	Items      []ocCredItem
	// Note is plain text under the secret (when the old one stops working).
	Note string
}

type ocCredItem struct {
	Label, Value string
}

// showCredentials answers an action that produced a secret: the dialog, the
// refreshed table and drawer (an opened one for a new client) and the flash.
// A native post gets the page itself (post/redirect/get would lose the
// secret). Every such response is no-store.
func (h *OAuthClients) showCredentials(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id string, c ocCredentials, flash string) {
	ctx := r.Context()
	// The stream helper sets no-cache when it starts: noStore stamps the
	// header on the way out.
	w = noStore{w}
	dialog := ocCredentialsDialog(c)
	canRead := auth.CanReadOAuthClients(ac) == nil
	if !ds.IsDatastar(r) {
		view, err := h.listView(ctx, ac, parseListQuery(nil, ocList))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		if canRead {
			d, err := h.drawerFor(ctx, ac, id)
			if err != nil {
				h.Shell.Fail(w, r, err)
				return
			}
			view.Drawer = &d
		}
		view.Dialog = dialog
		h.renderPage(w, r, http.StatusOK, view)
		return
	}
	sse := ds.NewSSE(w, r)
	// Replacing the slot also replaces the create form it may hold.
	if err := patchDialogSlot(sse, ocDialogSlot, dialog); err != nil {
		slog.DebugContext(ctx, "ui: credentials dialog not delivered", "err", err)
	}
	if canRead {
		if view, err := h.listView(ctx, ac, refererList(r, ocList)); err == nil {
			if err := view.patchTable(sse); err != nil {
				slog.DebugContext(ctx, "ui: oauth clients table patch not delivered", "err", err)
			}
		}
		if d, err := h.drawerFor(ctx, ac, id); err != nil {
			h.Shell.FailSSE(sse, r, err)
		} else if r.PostFormValue("drawer") == id {
			if err := sse.Patch(ocDrawerContent(d), ds.Inner("#"+ocDrawerID)); err != nil {
				slog.DebugContext(ctx, "ui: oauth client drawer patch not delivered", "err", err)
			}
		} else if err := openOCDrawer(sse, d); err != nil {
			slog.DebugContext(ctx, "ui: oauth client drawer patch not delivered", "err", err)
		}
	}
	if err := flashKey(sse, flash); err != nil {
		slog.DebugContext(ctx, "ui: oauth client flash not delivered", "err", err)
	}
}

// rotateSecret is POST /api/oauth-clients/{id}/rotate-secret with no body: the
// new secret works at once and the old one for the 24-hour overlap.
func (h *OAuthClients) rotateSecret(w http.ResponseWriter, r *http.Request) {
	h.rotate(w, r, nil)
}

// rotateSecretNow is the same with graceSeconds 0: the old secret stops
// working on the spot, for one believed compromised.
func (h *OAuthClients) rotateSecretNow(w http.ResponseWriter, r *http.Request) {
	none := int64(0)
	h.rotate(w, r, &none)
}

func (h *OAuthClients) rotate(w http.ResponseWriter, r *http.Request, grace *int64) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := ocID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := auth.CanRotateOAuthClientSecrets(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	res, err := h.Ops.Rotate(ctx, id, grace)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	note := "The previous secret stopped working immediately."
	if res.PreviousExpiresAt != nil {
		note = "The previous secret keeps working until " + res.PreviousExpiresAt.UTC().Format(time.DateTime) + " UTC, then stops. Update the applications that use it before then."
	}
	h.showCredentials(w, r, ac, id, ocCredentials{
		Title: "New Client Secret", Warning: "Copy this secret now. It will not be shown again.",
		Items: []ocCredItem{{Label: "Client Secret", Value: res.Secret}}, Note: note,
	}, flashOCRotated)
}

// ocCredID is the id of credential i (the copy buttons read it by id).
func ocCredID(i int) string { return "oc-cred-" + strconv.Itoa(i) }

// ocCopyExpr copies a credential (by element id) and says so for two seconds
// in the status line. The label is a page constant ("Client ID").
func ocCopyExpr(id, label string) string {
	return "navigator.clipboard.writeText(document.getElementById('" + kit.ID(id) + "').textContent).then(() => { $ocCopied = '" + label + " copied to clipboard'; setTimeout(() => $ocCopied = '', 2000) })"
}

// ocCredentialsAttrs are the credentials dialog's attributes: it opens as a
// modal and removes itself when it closes; the create dialog also reopens
// when closed by anything but its button.
func ocCredentialsAttrs(persistent bool) templ.Attributes {
	a := templ.Attributes{"open": true, "data-init": "el.open && el.close(); el.showModal()"}
	if persistent {
		a["data-on:cancel"] = "evt.preventDefault()"
		a["data-on:close"] = "el.dataset.saved || el.showModal()"
	} else {
		a["data-on:close"] = "el.remove()"
	}
	return a
}
