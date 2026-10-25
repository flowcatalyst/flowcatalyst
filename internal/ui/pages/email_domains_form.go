package pages

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	edmops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// The create and edit dialogs (the SPA's EmailDomainMappingCreateDrawer and
// the drawer's edit form).

const (
	edmFormDialog = "edm-form"
	edmFormSignal = "edmForm"
)

// edmFieldCodes maps the use cases' codes to the field that caused them.
var edmFieldCodes = map[string]string{
	"EMAIL_DOMAIN_REQUIRED":   "emailDomain",
	"INVALID_EMAIL_DOMAIN":    "emailDomain",
	"DOMAIN_ALREADY_MAPPED":   "emailDomain",
	"IDP_REQUIRED":            "identityProviderId",
	"INVALID_SCOPE_TYPE":      "scopeType",
	"PRIMARY_CLIENT_REQUIRED": "client",
	"2FA_METHOD_REQUIRED":     "methods",
	"INVALID_2FA_METHOD":      "methods",
}

// edmScopeOptions are the SPA's scope choices.
var edmScopeOptions = []kit.Option{
	{Value: "ANCHOR", Label: "Anchor: platform admin, access to all clients"},
	{Value: "PARTNER", Label: "Partner: partner user, access to multiple clients"},
	{Value: "CLIENT", Label: "Client: client user, bound to a single client"},
}

// edmMethodOptions are the two-factor methods a domain may allow.
var edmMethodOptions = []kit.Option{
	{Value: "TOTP", Label: "Authenticator app"},
	{Value: "EMAIL_PIN", Label: "Email code"},
}

const (
	edmDomainHelp  = "Please enter a valid domain name."
	edmDefaultDays = 30
	edmDaysHelp    = "Enter a whole number of days from 1 to 365."
)

// edmProviderKind is what the form needs to know of a provider to show its
// parts: external (two-factor does not apply) and multi-tenant (a tenant is
// required). It is sent to the page as signals, so the parts follow the
// select.
type edmProviderKind struct {
	OIDC        bool `json:"o"`
	MultiTenant bool `json:"m"`
}

// edmForm is the create or edit form: what is rendered, or what was
// submitted with its errors.
type edmForm struct {
	Edit     bool
	ID       string
	Subtitle string
	// Domain is the mapping's domain (edit: shown, not editable); Scope its
	// scope (edit: shown, the API cannot change it).
	Domain string

	EmailDomain, ProviderID, Scope, Client, Tenant string
	Require2FA, Remember                           bool
	Methods                                        []string
	Days                                           string

	// Move is set when the save would change the provider and waits for the
	// confirmation: the names of both providers and the direction.
	Move *edmMove

	Errs formErrors
	Open bool

	Providers     []kit.Option
	Kinds         map[string]edmProviderKind
	CanPickClient bool
	ClientOptions []kit.Option
}

// edmMove is the SPA's "Change Identity Provider" confirmation.
type edmMove struct {
	From, To   string
	ToInternal bool
}

func (f edmForm) id(key string) string         { return fieldID(edmFormDialog, key) }
func (f edmForm) fieldError(key string) string { return f.Errs.Fields[key] }

func (f edmForm) action() string {
	if f.Edit {
		return edmURL(f.ID) + "/edit"
	}
	return edmPath + "/new"
}

func (f edmForm) keys() []string {
	if f.Edit {
		return []string{"identityProviderId", "move", "client", "tenant", "require2fa", "methods", "remember", "days"}
	}
	return []string{"emailDomain", "identityProviderId", "scopeType", "client", "tenant", "require2fa", "methods", "remember", "days"}
}

func (f edmForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.fieldError(k) != "" {
			if k == "methods" {
				return f.id("methods-totp")
			}
			return f.id(k)
		}
	}
	if f.Move != nil {
		return f.id("move")
	}
	return ""
}

func (f edmForm) fields() []templ.Component {
	out := make([]templ.Component, 0, len(f.keys()))
	for _, k := range f.keys() {
		out = append(out, edmField(f, k))
	}
	return out
}

func (f edmForm) dialog() kit.FormDialogProps {
	p := kit.FormDialogProps{
		ID: edmFormDialog, Signal: edmFormSignal, Width: kit.WidthWide, Action: f.action(), Open: f.Open, Error: f.Errs.Form,
		Title: "Create Email Domain Mapping", Description: "Map an email domain to an identity provider and define user scope", SubmitLabel: "Create Mapping",
	}
	if f.Edit {
		p.Title, p.Description, p.SubmitLabel, p.RequireChange = "Edit Email Domain Mapping", f.Subtitle, "Save", true
		p.Hidden = []kit.HiddenField{drawerField(edmDrawerSig)}
	}
	return p
}

// signals declares what the form's conditional parts follow: the chosen
// provider, the ids of the OIDC providers and of the multi-tenant ones (the
// parts test the chosen id against them), the scope and the two-factor
// switches. The ids are JSON-encoded lists; the scope is one of a fixed set.
func (f edmForm) signals() string {
	var oidc, multi []string
	for id, k := range f.Kinds {
		if k.OIDC {
			oidc = append(oidc, id)
		}
		if k.MultiTenant {
			multi = append(multi, id)
		}
	}
	slices.Sort(oidc)
	slices.Sort(multi)
	oidcJSON, _ := json.Marshal(append([]string{}, oidc...))
	multiJSON, _ := json.Marshal(append([]string{}, multi...))
	idp, _ := json.Marshal(f.ProviderID)
	scope := "CLIENT"
	if f.Scope == "ANCHOR" || f.Scope == "PARTNER" {
		scope = f.Scope
	}
	return "{edmFormIdp: " + string(idp) + ", edmFormOidc: " + string(oidcJSON) + ", edmFormMt: " + string(multiJSON) + ", edmFormScope: '" + scope + "', edmFormReq: " + boolLit(f.Require2FA) + ", edmFormRem: " + boolLit(f.Remember) + "}"
}

func (f edmForm) days() string { return cmpOrString(f.Days, strconv.Itoa(edmDefaultDays)) }

func (f edmForm) clientShown() bool { return f.Scope != "ANCHOR" }

func (f edmForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.id(key), Required: required, Description: help, Error: f.fieldError(key), Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f edmForm) describedBy(key string) string {
	if f.fieldError(key) != "" {
		return f.id(key) + "-err"
	}
	return ""
}

func (f edmForm) inputProps(key, value string, p kit.InputProps) kit.InputProps {
	p.ID, p.Name, p.Value = f.id(key), key, value
	p.Invalid = f.fieldError(key) != ""
	p.DescribedBy = f.describedBy(key)
	return p
}

func (f edmForm) clientHelp() string {
	if !f.CanPickClient {
		return "Choosing a client needs permission to view clients."
	}
	if f.Scope == "PARTNER" {
		return "The partner mapping's primary client. Partner users can be granted access to more clients after login."
	}
	return "Users from this domain will be bound to this client"
}

func submittedEdmForm(r *http.Request, edit bool) edmForm {
	return edmForm{
		Edit:        edit,
		EmailDomain: strings.ToLower(strings.TrimSpace(r.PostFormValue("emailDomain"))),
		ProviderID:  strings.TrimSpace(r.PostFormValue("identityProviderId")),
		Scope:       r.PostFormValue("scopeType"),
		Client:      strings.TrimSpace(r.PostFormValue("client")),
		Tenant:      strings.TrimSpace(r.PostFormValue("tenant")),
		Require2FA:  r.PostFormValue("require2fa") == "true",
		Remember:    r.PostFormValue("remember") == "true",
		Methods:     formSet(r.PostForm["methods"]),
		Days:        strings.TrimSpace(r.PostFormValue("days")),
	}
}

// ── choices ────────────────────────────────────────────────────────────

func (h *EmailDomains) withChoices(r *http.Request, ac *auth.AuthContext, f *edmForm, providers edmProviders) {
	ctx := r.Context()
	f.Providers = providers.options
	f.Kinds = make(map[string]edmProviderKind, len(providers.byID))
	for id, ip := range providers.byID {
		f.Kinds[id] = edmProviderKind{OIDC: ip.Type == identityprovider.TypeOIDC, MultiTenant: ip.OIDCMultiTenant}
	}
	f.CanPickClient = canPickClient(h.Clients, ac)
	if opts, err := clientSuggestions(ctx, h.Clients, ac, f.Client); err != nil {
		slog.WarnContext(ctx, "ui: client suggestions unavailable", "err", err)
	} else {
		f.ClientOptions = opts
	}
}

// show2FA is the SPA's rule: two-factor controls need a provider that is not
// OIDC.
func (f edmForm) show2FA() bool {
	k, ok := f.Kinds[f.ProviderID]
	return ok && !k.OIDC
}

func (f edmForm) showTenant() bool {
	k, ok := f.Kinds[f.ProviderID]
	return ok && k.MultiTenant
}

// twoFactor reads the two-factor policy the way the SPA sends it: only for a
// provider that takes it.
func (f *edmForm) twoFactor() (days int, ok bool) {
	days = edmDefaultDays
	if f.Days != "" {
		n, err := strconv.Atoi(f.Days)
		if err != nil || n < 1 || n > 365 {
			f.Errs.field("days", edmDaysHelp)
			return 0, false
		}
		days = n
	}
	return days, true
}

// ── create ─────────────────────────────────────────────────────────────

func (h *EmailDomains) createForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	// POST /api/email-domain-mappings's check.
	if err := auth.CanCreateEmailDomainMappings(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	providers, err := h.providers(r.Context())
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := edmForm{Open: true, Scope: "CLIENT"}
	h.withChoices(r, ac, &f, providers)
	h.showForm(w, r, ac, f)
}

func (h *EmailDomains) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedEdmForm(r, false)
	if err := auth.CanCreateEmailDomainMappings(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	providers, err := h.providers(ctx)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.withChoices(r, ac, &f, providers)
	cmd, ok := h.createCommand(r, ac, &f)
	if !ok {
		h.formFailed(w, r, ac, f)
		return
	}
	id, err := h.Ops.Create(ctx, cmd)
	if err != nil {
		f.Errs.apply(r, err, edmFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashEdmCreated)
		http.Redirect(w, r, edmPath, http.StatusSeeOther)
		return
	}
	// As the SPA: close the form and open the new mapping's drawer.
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(edmFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: email domain form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, id, false)
	if m, err := h.load(ctx, id); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := openEdmDrawer(sse, h.toDetail(ctx, ac, m)); err != nil {
		slog.DebugContext(ctx, "ui: email domain drawer patch not delivered", "err", err)
	}
	if err := flashKey(sse, flashEdmCreated); err != nil {
		slog.DebugContext(ctx, "ui: email domain flash not delivered", "err", err)
	}
}

// createCommand builds the SPA's POST body: the primary client for a client
// scope (the platform also requires it for a partner scope, which the SPA
// never sent), the tenant only for a multi-tenant provider, the two-factor
// policy only for a provider that takes it.
func (h *EmailDomains) createCommand(r *http.Request, ac *auth.AuthContext, f *edmForm) (edmops.CreateCommand, bool) {
	cmd := edmops.CreateCommand{EmailDomain: f.EmailDomain, IdentityProviderID: f.ProviderID, ScopeType: f.Scope}
	if f.EmailDomain != "" && !emailDomainShape.MatchString(f.EmailDomain) {
		f.Errs.field("emailDomain", edmDomainHelp)
	}
	if _, known := f.Kinds[f.ProviderID]; f.ProviderID != "" && !known {
		f.Errs.field("identityProviderId", "Choose an identity provider from the list.")
	}
	if f.ProviderID == "" {
		f.Errs.field("identityProviderId", "Choose an identity provider.")
	}
	if !slices.ContainsFunc(edmScopeOptions, func(o kit.Option) bool { return o.Value == f.Scope }) {
		f.Errs.field("scopeType", "Choose a scope.")
	}
	if f.Scope == "CLIENT" || f.Scope == "PARTNER" {
		h.applyClient(r, ac, f, &cmd.PrimaryClientID)
	}
	if f.showTenant() {
		if f.Tenant == "" {
			f.Errs.field("tenant", "The required OIDC tenant ID is needed for a multi-tenant provider.")
		} else {
			cmd.RequiredOIDCTenantID = &f.Tenant
		}
	}
	if f.show2FA() {
		days, ok := f.twoFactor()
		if ok {
			cmd.Require2FA = f.Require2FA
			if f.Require2FA {
				cmd.Allowed2FAMethods = f.Methods
			}
			cmd.RememberDeviceEnabled = f.Remember
			cmd.RememberDeviceDays = days
		}
	}
	return cmd, !f.Errs.any()
}

// applyClient resolves the submitted client; a client that names none is a
// field error, and a blank one is for the use case to refuse.
func (h *EmailDomains) applyClient(r *http.Request, ac *auth.AuthContext, f *edmForm, out **string) {
	id, msg, err := resolveClient(r.Context(), h.Clients, ac, f.Client)
	switch {
	case err != nil:
		f.Errs.apply(r, err, nil)
	case msg != "":
		f.Errs.field("client", msg)
	}
	*out = id
}

// ── edit ───────────────────────────────────────────────────────────────

func (h *EmailDomains) editForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// The SPA loads the mapping (GET) and saves it with PUT.
	if err := firstError(auth.CanReadEmailDomainMappings(ac), auth.CanUpdateEmailDomainMappings(ac)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := edmID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	m, err := h.load(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	providers, err := h.providers(ctx)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := edmForm{
		Edit: true, ID: m.ID, Domain: m.EmailDomain, Subtitle: m.EmailDomain, Open: true,
		ProviderID: m.IdentityProviderID, Scope: string(m.ScopeType), Tenant: deref(m.RequiredOIDCTenantID),
		Require2FA: m.Require2FA, Methods: slices.Clone(m.Allowed2FAMethods), Remember: m.RememberDeviceEnabled,
		Days: strconv.Itoa(m.RememberDeviceDays),
	}
	if id := ids.StringPtr(m.PrimaryClientID); id != nil {
		f.Client = clientIdentifierOf(ctx, h.ClientNames, *id)
	}
	h.withChoices(r, ac, &f, providers)
	h.showForm(w, r, ac, f)
}

func (h *EmailDomains) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedEdmForm(r, true)
	if err := firstError(auth.CanReadEmailDomainMappings(ac), auth.CanUpdateEmailDomainMappings(ac)); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	id, err := edmID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	m, err := h.load(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	providers, err := h.providers(ctx)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// The domain and the scope are the mapping's own: the API cannot change
	// them.
	f.ID, f.Domain, f.Subtitle, f.Scope = m.ID, m.EmailDomain, m.EmailDomain, string(m.ScopeType)
	h.withChoices(r, ac, &f, providers)
	update, ok := h.updateCommand(r, ac, m, &f)
	if !ok {
		h.formFailed(w, r, ac, f)
		return
	}
	// A provider change is a real auth-method change: confirm first.
	moved := f.ProviderID != m.IdentityProviderID
	if moved && r.PostFormValue("confirmMove") != "yes" {
		to := providers.byID[f.ProviderID]
		f.Move = &edmMove{From: providers.name(m.IdentityProviderID), To: to.Name, ToInternal: to.Type != identityprovider.TypeOIDC}
		h.formFailed(w, r, ac, f)
		return
	}
	usersReset := 0
	if moved {
		// First, through the dedicated use case (direction-aware side
		// effects), as the SPA does.
		res, err := h.Ops.Move(ctx, edmops.MoveProviderCommand{ID: id, IdentityProviderID: f.ProviderID})
		if err != nil {
			f.Errs.apply(r, err, map[string]string{"IDP_REQUIRED": "identityProviderId", "ALREADY_ON_PROVIDER": "identityProviderId"})
			h.formFailed(w, r, ac, f)
			return
		}
		usersReset = res.UsersReset
	}
	if err := h.Ops.Update(ctx, update); err != nil {
		f.Errs.apply(r, err, edmFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashEdmUpdated)
		http.Redirect(w, r, edmPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(edmFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: email domain form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, id, false)
	if usersReset > 0 {
		// The SPA's toast: the count of SSO-provisioned users converted.
		noun := "users"
		if usersReset == 1 {
			noun = "user"
		}
		err := shell.Flash(sse, kit.FlashMessageProps{
			Kind: kit.FlashSuccess, Summary: "Provider changed",
			Detail: strconv.Itoa(usersReset) + " SSO-provisioned " + noun + " converted to password sign-in (password reset required)",
		})
		if err != nil {
			slog.DebugContext(ctx, "ui: email domain flash not delivered", "err", err)
		}
	}
	if err := flashKey(sse, flashEdmUpdated); err != nil {
		slog.DebugContext(ctx, "ui: email domain flash not delivered", "err", err)
	}
}

// updateCommand builds the SPA's PUT body: the primary client for a client
// or partner scope (cleared for an anchor), the tenant for a multi-tenant
// provider (blank clears it), and the two-factor policy for a provider that
// takes it. The provider the form names decides which parts apply, as the
// mapping will be on it once the move is done.
func (h *EmailDomains) updateCommand(r *http.Request, ac *auth.AuthContext, m *emaildomainmapping.EmailDomainMapping, f *edmForm) (edmops.UpdateCommand, bool) {
	cmd := edmops.UpdateCommand{ID: m.ID}
	if _, known := f.Kinds[f.ProviderID]; !known {
		f.Errs.field("identityProviderId", "Choose an identity provider from the list.")
	}
	switch m.ScopeType {
	case emaildomainmapping.ScopeAnchor:
		none := ""
		cmd.PrimaryClientID = &none
	case emaildomainmapping.ScopeClient, emaildomainmapping.ScopePartner:
		// Without permission to view clients the field is not offered and the
		// stored client stays.
		if f.CanPickClient {
			if f.Client == "" {
				f.Errs.field("client", "A primary client is required.")
			} else {
				h.applyClient(r, ac, f, &cmd.PrimaryClientID)
			}
		}
	}
	if f.showTenant() {
		if f.Tenant == "" {
			f.Errs.field("tenant", "The required OIDC tenant ID is needed for a multi-tenant provider.")
		}
		cmd.RequiredOIDCTenantID = &f.Tenant
	}
	if f.show2FA() {
		if days, ok := f.twoFactor(); ok {
			cmd.Require2FA = &f.Require2FA
			methods := []string{}
			if f.Require2FA {
				methods = f.Methods
			}
			cmd.Allowed2FAMethods = methods
			cmd.RememberDeviceEnabled = &f.Remember
			cmd.RememberDeviceDays = &days
		}
	}
	return cmd, !f.Errs.any()
}

// ── rendering ──────────────────────────────────────────────────────────

func (h *EmailDomains) showForm(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f edmForm) {
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), edmDialogSlot, edmFormDialogC(f)); err != nil {
			slog.DebugContext(r.Context(), "ui: email domain form patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, edmList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Dialog = edmFormDialogC(f)
	h.renderPage(w, r, http.StatusOK, view)
}

// formFailed answers a refused submit (or the provider move confirmation)
// with the form again: field errors, the form error region and focus.
func (h *EmailDomains) formFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f edmForm) {
	if f.Kinds == nil {
		if providers, err := h.providers(r.Context()); err == nil {
			h.withChoices(r, ac, &f, providers)
		}
	}
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, edmFormDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, edmList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	view.Dialog = edmFormDialogC(f)
	status := f.Errs.Status
	if status == 0 {
		status = http.StatusOK
	}
	h.renderPage(w, r, status, view)
}
