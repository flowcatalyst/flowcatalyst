package pages

import (
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	idpops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

// The create and edit dialogs (the SPA's IdentityProviderCreateDrawer and the
// drawer's edit form).

const (
	idpFormDialog = "idp-form"
	idpFormSignal = "idpForm"
)

// idpFieldCodes maps the use cases' codes to the field that caused them. The
// scope codes are the SPA's MAPPING_SCOPE_ERROR_CODES, shown next to the
// scope choice; a missing client is shown on the client field.
var idpFieldCodes = map[string]string{
	"CODE_REQUIRED":              "code",
	"CODE_EXISTS":                "code",
	"NAME_REQUIRED":              "name",
	"INVALID_TYPE":               "type",
	"OIDC_ISSUER_REQUIRED":       "issuerUrl",
	"OIDC_CLIENT_ID_REQUIRED":    "clientId",
	"INVALID_EMAIL_DOMAIN":       "domains",
	"MAPPING_SCOPE_REQUIRED":     "scope",
	"INVALID_MAPPING_SCOPE":      "scope",
	"PRIMARY_CLIENT_NOT_ALLOWED": "scope",
	"PRIMARY_CLIENT_REQUIRED":    "client",
	"ENCRYPTION_NOT_CONFIGURED":  "secret",
	"UNSUPPORTED_SECRET_SCHEME":  "secret",
}

// The SPA's validators: the code (CODE_PATTERN) and a domain (the chip
// input's check).
var (
	idpCodePattern   = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	emailDomainShape = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*\.[a-z]{2,}$`)
)

const (
	idpCodeHelp     = "Lowercase letters, numbers, and hyphens only. Must start with a letter."
	idpUnknownRole  = "Choose roles from the list."
	idpTypeOIDC     = string(identityprovider.TypeOIDC)
	idpTypeInternal = string(identityprovider.TypeInternal)
)

// idpTypeOptions are the SPA's type choices.
var idpTypeOptions = []kit.Option{
	{Value: idpTypeInternal, Label: "Internal (Local): internal authentication (username/password)"},
	{Value: idpTypeOIDC, Label: "OIDC (External): external OpenID Connect provider"},
}

// idpScopeOptions are the SPA's mapping scope choices (PARTNER mappings are
// managed on the Email Domains page).
var idpScopeOptions = []kit.Option{
	{Value: "ANCHOR", Label: "Anchor: platform admin, access to all clients"},
	{Value: "CLIENT", Label: "Client: bound to a single client"},
}

// idpSecretKind is one of the SPA's SecretRefInput providers: how the client
// secret is held. Local Encrypted takes the secret itself (the API encrypts
// it); the others take the reference to a secret held elsewhere.
type idpSecretKind struct {
	Value, Label, Prefix, Placeholder, Help string
}

var idpSecretKinds = []idpSecretKind{
	{"encrypt", "Local Encrypted", "encrypt:", "your-client-secret", "Encrypt and store locally (requires APP_KEY)"},
	{"aws-sm", "AWS Secrets Manager", "aws-sm://", "secret-name", "Reference a secret in AWS Secrets Manager"},
	{"aws-ps", "AWS Parameter Store", "aws-ps://", "/path/to/parameter", "Reference a parameter in AWS SSM Parameter Store"},
	{"gcp-sm", "GCP Secret Manager", "gcp-sm://", "secret-name", "Reference a secret in Google Cloud Secret Manager"},
	{"vault", "HashiCorp Vault", "vault://", "path/to/secret#key", "Reference a secret in HashiCorp Vault"},
}

func idpSecretKindFor(value string) (idpSecretKind, bool) {
	for _, k := range idpSecretKinds {
		if k.Value == value {
			return k, true
		}
	}
	return idpSecretKind{}, false
}

func idpSecretKindOptions() []kit.Option {
	opts := make([]kit.Option, len(idpSecretKinds))
	for i, k := range idpSecretKinds {
		opts[i] = kit.Option{Value: k.Value, Label: k.Label}
	}
	return opts
}

// idpForm is the create or edit form: what is rendered, or what was
// submitted with its errors. The secret itself is in neither: a typed secret
// is never rendered back (a refused form keeps what the browser holds), only
// a reference to a secret held elsewhere is.
type idpForm struct {
	Edit     bool
	ID       string
	Subtitle string

	Type, Code, Name                   string
	MultiTenant                        bool
	IssuerURL, IssuerPattern, ClientID string
	SecretKind, SecretRef              string
	HasSecret                          bool
	// Domains is the textarea: the domains routed to the provider, one per line.
	Domains string
	Scope   string
	Client  string
	Sync    bool
	Roles   []string

	// Release lists the domains an edit would release; the form asks for a
	// confirmation before it saves.
	Release []string

	Errs formErrors
	Open bool

	CanPickClient bool
	ClientOptions []kit.Option
	// RoleOptions are the roles to choose from; without permission to list
	// roles the roles the provider holds are posted back unchanged.
	RoleOptions  []kit.Option
	CanPickRoles bool
}

func (f idpForm) id(key string) string         { return fieldID(idpFormDialog, key) }
func (f idpForm) fieldError(key string) string { return f.Errs.Fields[key] }

func (f idpForm) oidc() bool { return f.Type == idpTypeOIDC }

func (f idpForm) hasDomains() bool { return strings.TrimSpace(f.Domains) != "" }

func (f idpForm) action() string {
	if f.Edit {
		return idpURL(f.ID) + "/edit"
	}
	return idpPath + "/new"
}

// keys are the fields a failed submit patches back. The secret is left to
// the browser (nothing typed there is sent back) unless it is the field at
// fault.
func (f idpForm) keys() []string {
	var keys []string
	if f.Edit {
		keys = []string{"name", "multiTenant", "issuerUrl", "issuerPattern", "clientId", "domains", "scope", "client", "release", "sync", "roles"}
	} else {
		keys = []string{"code", "name", "type", "multiTenant", "issuerUrl", "issuerPattern", "clientId", "domains", "scope", "client", "sync", "roles"}
	}
	if f.fieldError("secret") != "" {
		keys = append(keys, "secret")
	}
	return keys
}

func (f idpForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.fieldError(k) != "" {
			if k == "secret" {
				return f.id("secret-value")
			}
			return f.id(k)
		}
	}
	if len(f.Release) > 0 {
		return f.id("release")
	}
	return ""
}

func (f idpForm) fields() []templ.Component {
	out := make([]templ.Component, 0, len(f.keys()))
	for _, k := range f.keys() {
		out = append(out, idpField(f, k))
	}
	return out
}

func (f idpForm) dialog() kit.FormDialogProps {
	p := kit.FormDialogProps{
		ID: idpFormDialog, Signal: idpFormSignal, Width: kit.WidthWide, Action: f.action(), Open: f.Open, Error: f.Errs.Form,
		Title: "Create Identity Provider", Description: "Configure a new identity provider for federated authentication", SubmitLabel: "Create Identity Provider",
	}
	if f.Edit {
		p.Title, p.Description, p.SubmitLabel, p.RequireChange = "Edit Identity Provider", f.Subtitle, "Save", true
		p.Hidden = []kit.HiddenField{drawerField(idpDrawerSig)}
	}
	return p
}

// signals declares what the form's conditional parts follow: the type, the
// multi-tenant switch, the sync switch, whether domains are listed, the
// scope and the secret's kind. Every value is one of a fixed set.
func (f idpForm) signals() string {
	typ := idpTypeOIDC
	if f.Type == idpTypeInternal {
		typ = idpTypeInternal
	}
	scope := ""
	if f.Scope == "ANCHOR" || f.Scope == "CLIENT" {
		scope = f.Scope
	}
	kind := "encrypt"
	if k, ok := idpSecretKindFor(f.SecretKind); ok {
		kind = k.Value
	}
	return "{idpFormType: '" + typ + "', idpFormMt: " + boolLit(f.MultiTenant) + ", idpFormSync: " + boolLit(f.Sync) +
		", idpFormHasDomains: " + boolLit(f.hasDomains()) + ", idpFormScope: '" + scope + "', idpFormSecretKind: '" + kind + "'}"
}

func boolLit(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func (f idpForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.id(key), Required: required, Description: help, Error: f.fieldError(key), Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f idpForm) describedBy(key string) string {
	if f.fieldError(key) != "" {
		return f.id(key) + "-err"
	}
	return ""
}

func (f idpForm) inputProps(key, value string, p kit.InputProps) kit.InputProps {
	p.ID, p.Name, p.Value = f.id(key), key, value
	p.Invalid = f.fieldError(key) != ""
	p.DescribedBy = f.describedBy(key)
	return p
}

func (f idpForm) clientHelp() string {
	if !f.CanPickClient {
		return "Choosing a client needs permission to view clients."
	}
	return "Linked on mappings that are new or not yet linked to a primary client. A domain's existing client link is never overwritten."
}

func (f idpForm) scopeHelp() string {
	if f.Edit {
		return "Choose Client and pick a client to link it to domains that don't have one yet. Existing links and scopes are not changed. Required when a new domain is added: every new domain mapping needs an explicit scope, it never falls back to Anchor."
	}
	return "Required because domains are listed above. Every new domain mapping needs an explicit scope: it never falls back to Anchor."
}

func (f idpForm) secretHelp() string {
	if f.Edit && f.HasSecret {
		return "Current secret is configured. Enter a new value to replace it, or leave blank to keep it."
	}
	if f.Edit {
		return "Enter the client secret"
	}
	return "Required for confidential clients"
}

func (f idpForm) secretKindValue() string {
	if k, ok := idpSecretKindFor(f.SecretKind); ok {
		return k.Value
	}
	return "encrypt"
}

// parseDomains reads the domains textarea: separated by lines, commas,
// semicolons or spaces, lower-cased, de-duplicated in order. bad is the
// first entry that is not a domain name (the SPA's chip validator).
func parseDomains(raw string) (domains []string, bad string) {
	for _, d := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\r' || r == '\t' }) {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" || slices.Contains(domains, d) {
			continue
		}
		if !emailDomainShape.MatchString(d) && bad == "" {
			bad = d
		}
		domains = append(domains, d)
	}
	return domains, bad
}

func submittedIdpForm(r *http.Request, edit bool) idpForm {
	f := idpForm{
		Edit:          edit,
		Type:          strings.TrimSpace(r.PostFormValue("type")),
		Code:          strings.TrimSpace(r.PostFormValue("code")),
		Name:          strings.TrimSpace(r.PostFormValue("name")),
		MultiTenant:   r.PostFormValue("multiTenant") == "true",
		IssuerURL:     strings.TrimSpace(r.PostFormValue("issuerUrl")),
		IssuerPattern: strings.TrimSpace(r.PostFormValue("issuerPattern")),
		ClientID:      strings.TrimSpace(r.PostFormValue("clientId")),
		SecretKind:    r.PostFormValue("secretKind"),
		SecretRef:     strings.TrimSpace(r.PostFormValue("secretRef")),
		Domains:       r.PostFormValue("domains"),
		Scope:         r.PostFormValue("scope"),
		Client:        strings.TrimSpace(r.PostFormValue("client")),
		Sync:          r.PostFormValue("sync") == "true",
		Roles:         facetValues(r.PostForm["roles"]),
	}
	if f.Type != idpTypeInternal {
		f.Type = idpTypeOIDC
	}
	return f
}

// secretRef is the client secret reference the SPA's SecretRefInput builds:
// the kind's prefix and the value, nil when nothing was entered. The typed
// secret is read here and goes only into the command.
func (f idpForm) secretRefFrom(r *http.Request) (*string, bool) {
	kind, ok := idpSecretKindFor(f.secretKindValue())
	if !ok {
		return nil, false
	}
	value := strings.TrimSpace(f.SecretRef)
	if kind.Value == "encrypt" {
		value = strings.TrimSpace(r.PostFormValue("secretValue"))
	}
	if value == "" {
		return nil, true
	}
	ref := kind.Prefix + value
	return &ref, true
}

// ── choices ────────────────────────────────────────────────────────────

// withChoices fills what the form offers: the clients (only when the user
// may list them, as the SPA's call would succeed) and the roles.
func (h *IdentityProviders) withChoices(r *http.Request, ac *auth.AuthContext, f *idpForm) {
	ctx := r.Context()
	f.CanPickClient = canPickClient(h.Clients, ac)
	if opts, err := clientSuggestions(ctx, h.Clients, ac, f.Client); err != nil {
		slog.WarnContext(ctx, "ui: client suggestions unavailable", "err", err)
	} else {
		f.ClientOptions = opts
	}
	f.CanPickRoles = h.Roles != nil && auth.CanReadAllRoles(ac)
	if !f.CanPickRoles {
		return
	}
	roles, err := h.Roles.FindAll(ctx)
	if err != nil {
		slog.WarnContext(ctx, "ui: roles unavailable", "err", err)
		f.CanPickRoles = false
		return
	}
	f.RoleOptions = make([]kit.Option, 0, len(roles))
	for _, rl := range roles {
		f.RoleOptions = append(f.RoleOptions, kit.Option{Value: rl.ID, Label: cmpOrString(rl.DisplayName, rl.Name) + " (" + rl.Name + ")"})
	}
	slices.SortFunc(f.RoleOptions, func(a, b kit.Option) int { return strings.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label)) })
}

// knownRoles refuses a posted role that is not in the list the form offered.
func (f idpForm) unknownRole() bool {
	if !f.CanPickRoles {
		return false
	}
	return slices.ContainsFunc(f.Roles, func(id string) bool {
		return !slices.ContainsFunc(f.RoleOptions, func(o kit.Option) bool { return o.Value == id })
	})
}

// ── create ─────────────────────────────────────────────────────────────

func (h *IdentityProviders) createForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	// POST /api/identity-providers's check.
	if err := auth.CanCreateIdentityProviders(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := idpForm{Open: true, Type: idpTypeOIDC, SecretKind: "encrypt"}
	h.withChoices(r, ac, &f)
	h.showForm(w, r, ac, f)
}

func (h *IdentityProviders) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedIdpForm(r, false)
	if err := auth.CanCreateIdentityProviders(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	h.withChoices(r, ac, &f)
	cmd, ok := h.createCommand(r, ac, &f)
	if !ok {
		h.formFailed(w, r, ac, f)
		return
	}
	res, err := h.Ops.Create(ctx, cmd)
	if err != nil {
		f.Errs.apply(r, err, idpFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashIdpCreated)
		http.Redirect(w, r, idpPath, http.StatusSeeOther)
		return
	}
	// As the SPA: close the form and open the new provider's drawer.
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(idpFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: identity provider form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, res.IdentityProviderID, false)
	if p, err := h.load(ctx, res.IdentityProviderID); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := openIdpDrawer(sse, h.toDetail(ctx, ac, p)); err != nil {
		slog.DebugContext(ctx, "ui: identity provider drawer patch not delivered", "err", err)
	}
	if err := flashKey(sse, flashIdpCreated); err != nil {
		slog.DebugContext(ctx, "ui: identity provider flash not delivered", "err", err)
	}
}

// createCommand builds the SPA's POST body from the submitted form; ok=false
// when a field is wrong (recorded on f).
func (h *IdentityProviders) createCommand(r *http.Request, ac *auth.AuthContext, f *idpForm) (idpops.CreateCommand, bool) {
	cmd := idpops.CreateCommand{Code: f.Code, Name: f.Name, Type: f.Type}
	if f.Code != "" && !idpCodePattern.MatchString(f.Code) {
		f.Errs.field("code", idpCodeHelp)
	}
	domains, bad := parseDomains(f.Domains)
	if bad != "" {
		f.Errs.field("domains", "\""+bad+"\" is not a valid domain name.")
	}
	if len(domains) > 0 {
		cmd.AllowedEmailDomains = domains
		h.applyScope(r, ac, f, &cmd.MappingScope, &cmd.PrimaryClientID)
	}
	if f.oidc() {
		cmd.SyncRolesFromIDP = f.Sync
		if len(f.Roles) > 0 {
			cmd.AllowedRoleIDs = f.Roles
		}
		if f.unknownRole() {
			f.Errs.field("roles", idpUnknownRole)
		}
		cmd.OIDCIssuerURL = optionalText(f.IssuerURL)
		cmd.OIDCClientID = &f.ClientID
		cmd.OIDCMultiTenant = &f.MultiTenant
		cmd.OIDCIssuerPattern = optionalText(f.IssuerPattern)
		secret, ok := f.secretRefFrom(r)
		if !ok {
			f.Errs.field("secret", "Choose where the secret is held.")
		}
		cmd.OIDCClientSecretRef = secret
	}
	return cmd, !f.Errs.any()
}

// applyScope sets the mapping scope the SPA sends once domains are listed
// (the scope when one is chosen, the client only with CLIENT). A client that
// names no client is a field error.
func (h *IdentityProviders) applyScope(r *http.Request, ac *auth.AuthContext, f *idpForm, scope, clientID **string) {
	if f.Scope != "ANCHOR" && f.Scope != "CLIENT" {
		return
	}
	s := f.Scope
	*scope = &s
	if f.Scope != "CLIENT" {
		return
	}
	id, msg, err := resolveClient(r.Context(), h.Clients, ac, f.Client)
	switch {
	case err != nil:
		f.Errs.apply(r, err, nil)
	case msg != "":
		f.Errs.field("client", msg)
	}
	*clientID = id
}

// ── edit ───────────────────────────────────────────────────────────────

func (h *IdentityProviders) editForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// The SPA loads the provider (GET) and saves it with PUT.
	if err := firstError(auth.CanReadIdentityProviders(ac), auth.CanUpdateIdentityProviders(ac)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := idpID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	p, err := h.load(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// The provider stores no scope, so every edit starts with nothing chosen
	// (never inferred); the client secret is never read into the form.
	f := idpForm{
		Edit: true, ID: p.ID, Subtitle: p.Name + " (" + p.Code + ")", Open: true,
		Type: string(p.Type), Name: p.Name, MultiTenant: p.OIDCMultiTenant,
		IssuerURL: deref(p.OIDCIssuerURL), IssuerPattern: deref(p.OIDCIssuerPattern), ClientID: deref(p.OIDCClientID),
		SecretKind: "encrypt", HasSecret: p.HasClientSecret(),
		Domains: strings.Join(p.AllowedEmailDomains, "\n"), Sync: p.SyncRolesFromIDP, Roles: slices.Clone(p.AllowedRoleIDs),
	}
	h.withChoices(r, ac, &f)
	h.showForm(w, r, ac, f)
}

func (h *IdentityProviders) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedIdpForm(r, true)
	if err := firstError(auth.CanReadIdentityProviders(ac), auth.CanUpdateIdentityProviders(ac)); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	id, err := idpID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	p, err := h.load(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// The type is the provider's own; it cannot be changed.
	f.ID, f.Subtitle, f.Type, f.HasSecret = p.ID, p.Name+" ("+p.Code+")", string(p.Type), p.HasClientSecret()
	h.withChoices(r, ac, &f)
	cmd, ok := h.updateCommand(r, ac, p, &f)
	if !ok {
		h.formFailed(w, r, ac, f)
		return
	}
	// Removing a domain flips its users back to password auth: confirm first
	// (the SPA's "Remove Email Domains" dialog).
	if released := releasedDomains(p.AllowedEmailDomains, cmd.AllowedEmailDomains); len(released) > 0 && r.PostFormValue("confirmRelease") != "yes" {
		f.Release = released
		h.formFailed(w, r, ac, f)
		return
	}
	if _, err := h.Ops.Update(ctx, cmd); err != nil {
		f.Errs.apply(r, err, idpFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashIdpUpdated)
		http.Redirect(w, r, idpPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(idpFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: identity provider form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, id, false)
	if err := flashKey(sse, flashIdpUpdated); err != nil {
		slog.DebugContext(ctx, "ui: identity provider flash not delivered", "err", err)
	}
}

// releasedDomains are the current domains the desired set drops.
func releasedDomains(current, desired []string) []string {
	var out []string
	for _, d := range current {
		if !slices.Contains(desired, d) {
			out = append(out, d)
		}
	}
	return out
}

// updateCommand builds the SPA's PUT body: the name and the whole domain
// set, the scope (and the client with CLIENT) only when one is chosen, and
// for an OIDC provider its settings, the secret only when one was entered
// (blank keeps the stored one), the sync switch and the allowed roles.
func (h *IdentityProviders) updateCommand(r *http.Request, ac *auth.AuthContext, p *identityprovider.IdentityProvider, f *idpForm) (idpops.UpdateCommand, bool) {
	cmd := idpops.UpdateCommand{ID: p.ID, Name: &f.Name}
	domains, bad := parseDomains(f.Domains)
	if bad != "" {
		f.Errs.field("domains", "\""+bad+"\" is not a valid domain name.")
	}
	if domains == nil {
		domains = []string{}
	}
	cmd.AllowedEmailDomains = domains
	h.applyScope(r, ac, f, &cmd.MappingScope, &cmd.PrimaryClientID)
	if f.oidc() {
		cmd.OIDCIssuerURL = optionalText(f.IssuerURL)
		cmd.OIDCClientID = &f.ClientID
		cmd.OIDCMultiTenant = &f.MultiTenant
		cmd.OIDCIssuerPattern = optionalText(f.IssuerPattern)
		secret, ok := f.secretRefFrom(r)
		if !ok {
			f.Errs.field("secret", "Choose where the secret is held.")
		}
		cmd.OIDCClientSecretRef = secret
		cmd.SyncRolesFromIDP = &f.Sync
		roles := f.Roles
		if !f.CanPickRoles {
			// Not offered the list: what the provider holds stays as it is.
			roles = p.AllowedRoleIDs
		} else if f.unknownRole() {
			f.Errs.field("roles", idpUnknownRole)
		}
		if roles == nil {
			roles = []string{}
		}
		cmd.AllowedRoleIDs = roles
	}
	if f.IssuerURL == "" && f.oidc() {
		f.Errs.field("issuerUrl", "Issuer URL is required for OIDC providers.")
	}
	if f.ClientID == "" && f.oidc() {
		f.Errs.field("clientId", "Client ID is required for OIDC providers.")
	}
	if f.Name == "" {
		f.Errs.field("name", "Name is required.")
	}
	return cmd, !f.Errs.any()
}

// ── rendering ──────────────────────────────────────────────────────────

func (h *IdentityProviders) showForm(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f idpForm) {
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), idpDialogSlot, idpFormDialogC(f)); err != nil {
			slog.DebugContext(r.Context(), "ui: identity provider form patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, idpList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Dialog = idpFormDialogC(f)
	h.renderPage(w, r, http.StatusOK, view)
}

// formFailed answers a refused submit (or the release confirmation) with the
// form again: field errors, the form error region and focus.
func (h *IdentityProviders) formFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f idpForm) {
	if f.ClientOptions == nil && f.RoleOptions == nil {
		h.withChoices(r, ac, &f)
	}
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, idpFormDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, idpList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	view.Dialog = idpFormDialogC(f)
	status := f.Errs.Status
	if status == 0 {
		status = http.StatusOK
	}
	h.renderPage(w, r, status, view)
}
