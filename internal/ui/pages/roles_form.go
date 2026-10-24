package pages

import (
	"cmp"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/bff"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// The create dialog (the SPA's RoleListPage dialog) and the role editor page
// (RoleEditPage).

const (
	rolesFormDialog = "role-form"
	rolesFormSignal = "roleForm"

	rolesEditFormID  = "role-edit"
	rolesPickerID    = "role-edit-perms"
	rolesNewPermID   = "role-edit-newperm"
	rolesPickerField = "role-edit-permissions-field"
)

var rolesFieldCodes = map[string]string{
	"APPLICATION_REQUIRED":           "application",
	"APPLICATION_NOT_FOUND":          "application",
	"ROLE_NAME_REQUIRED":             "roleName",
	"INVALID_ROLE_NAME":              "roleName",
	"ROLE_EXISTS":                    "roleName",
	"DISPLAY_NAME_REQUIRED":          "displayName",
	"INVALID_PERMISSION":             "permissions",
	"PERMISSION_ABOVE_CALLER":        "permissions",
	"PERMISSION_OUTSIDE_APPLICATION": "permissions",
}

// ── create ─────────────────────────────────────────────────────────────

type rolesCreateForm struct {
	Application, Name, DisplayName, Description string
	// Apps are the active applications the application select offers.
	Apps []kit.Option
	Errs formErrors
	Open bool
}

func (f rolesCreateForm) id(key string) string         { return fieldID(rolesFormDialog, key) }
func (f rolesCreateForm) fieldError(key string) string { return f.Errs.Fields[key] }

// createKeys are the form's fields, in order.
var rolesCreateKeys = []string{"application", "roleName", "displayName", "description"}

func (f rolesCreateForm) firstInvalid() string {
	for _, k := range rolesCreateKeys {
		if f.fieldError(k) != "" {
			return f.id(k)
		}
	}
	return ""
}

func (f rolesCreateForm) fields() []templ.Component {
	out := make([]templ.Component, 0, len(rolesCreateKeys))
	for _, k := range rolesCreateKeys {
		out = append(out, rolesCreateField(f, k))
	}
	return out
}

func (f rolesCreateForm) dialog() kit.FormDialogProps {
	return kit.FormDialogProps{
		ID: rolesFormDialog, Signal: rolesFormSignal, Width: kit.WidthWide, Action: rolesPath + "/new", Open: f.Open, Error: f.Errs.Form,
		Title: "Create Role", Description: "Roles group permissions; assign them to users and service accounts", SubmitLabel: "Create Role",
	}
}

func (f rolesCreateForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.id(key), Required: required, Description: help, Error: f.fieldError(key), Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f rolesCreateForm) describedBy(key string) string {
	if f.fieldError(key) != "" {
		return f.id(key) + "-err"
	}
	return ""
}

func submittedRolesCreate(r *http.Request) rolesCreateForm {
	return rolesCreateForm{
		Application: r.PostFormValue("application"),
		Name:        r.PostFormValue("roleName"),
		DisplayName: r.PostFormValue("displayName"),
		Description: r.PostFormValue("description"),
	}
}

func (h *RolesPage) createForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := rolesWrite(ac, auth.CanCreateRoles); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := rolesCreateForm{Open: true, Apps: h.applicationOptions(ctx)}
	// As the SPA, the first application is chosen.
	if len(f.Apps) > 0 {
		f.Application = f.Apps[0].Value
	}
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), rolesDialogSlot, rolesCreateDialog(f)); err != nil {
			slog.DebugContext(ctx, "ui: role form patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(nil, rolesList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Dialog = rolesCreateDialog(f)
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *RolesPage) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedRolesCreate(r)
	if err := rolesWrite(ac, auth.CanCreateRoles); err != nil {
		f.Errs.apply(r, err, nil)
		h.createFailed(w, r, ac, f)
		return
	}
	// The SPA's POST body: the application, the name as typed, the display name
	// (the name when blank) and the description when set; no permissions.
	_, err := h.Ops.Create(ctx, operations.CreateCommand{
		ApplicationCode: f.Application,
		RoleName:        f.Name,
		DisplayName:     cmp.Or(f.DisplayName, f.Name),
		Description:     optionalText(f.Description),
	})
	if err != nil {
		f.Errs.apply(r, err, rolesFieldCodes)
		h.createFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashRoleCreated)
		http.Redirect(w, r, rolesPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(rolesFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: role form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac)
	if err := flashKey(sse, flashRoleCreated); err != nil {
		slog.DebugContext(ctx, "ui: role flash not delivered", "err", err)
	}
}

func (h *RolesPage) createFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f rolesCreateForm) {
	f.Apps = h.applicationOptions(r.Context())
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, rolesFormDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, rolesList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	view.Dialog = rolesCreateDialog(f)
	h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), view)
}

// ── edit page ──────────────────────────────────────────────────────────

// rolesPicker is the permission picker's content: the permissions the user
// may toggle (a multi-select) and the locked ones (shown, never toggled).
type rolesPicker struct {
	Options  []kit.Option
	Selected []string
	// Locked are the platform permissions the caller does not hold (the SPA's
	// isPermissionAboveUser): the API refuses adding them to a role or removing
	// them from one, so the editor shows them as they are. Held marks the ones
	// the role has; those are posted back unchanged.
	Locked []rolesLockedPermission
}

type rolesLockedPermission struct {
	Code string
	Held bool
}

// newRolesPicker splits every permission the editor knows (the catalogue's,
// the role's, the ones picked or added) into the toggleable and the locked.
// A locked permission is held exactly when the role holds it.
func newRolesPicker(ac *auth.AuthContext, catalogue []bff.PermissionEntry, held, picked []string) rolesPicker {
	seen := map[string]bool{}
	var codes []string
	add := func(code string) {
		if !seen[code] {
			seen[code] = true
			codes = append(codes, code)
		}
	}
	for _, p := range catalogue {
		add(p.Permission)
	}
	for _, code := range slices.Concat(held, picked) {
		add(code)
	}
	slices.Sort(codes)
	var p rolesPicker
	for _, code := range codes {
		if rolesAbove(ac, []string{code}) {
			p.Locked = append(p.Locked, rolesLockedPermission{Code: code, Held: slices.Contains(held, code)})
			continue
		}
		p.Options = append(p.Options, kit.Option{Value: code, Label: code})
		if slices.Contains(picked, code) {
			p.Selected = append(p.Selected, code)
		}
	}
	return p
}

// rolesEditForm is the editor's state: the role, the typed values and the
// picker.
type rolesEditForm struct {
	ID, Name, Application string
	DisplayName           string
	Description           string
	ClientManaged         bool
	// Picked are the permissions checked now (a failed save echoes them).
	Picked []string
	Picker rolesPicker
	// New* are the new-permission builder's segments; AddNote explains why
	// the last Add did nothing.
	NewContext, NewAggregate, NewAction string
	AddNote                             string
	Errs                                formErrors
}

func (f rolesEditForm) id(key string) string         { return fieldID(rolesEditFormID, key) }
func (f rolesEditForm) fieldError(key string) string { return f.Errs.Fields[key] }

// isPlatform: platform permissions are code-defined, so the editor offers no
// new-permission builder for a platform role (the SPA's isPlatformRole).
func (f rolesEditForm) isPlatform() bool { return f.Application == "platform" }

// editKeys are the form's patchable fields, in order.
var rolesEditKeys = []string{"displayName", "description", "permissions"}

func (f rolesEditForm) fields() []templ.Component {
	out := make([]templ.Component, 0, len(rolesEditKeys))
	for _, k := range rolesEditKeys {
		out = append(out, rolesEditField(f, k))
	}
	return out
}

func (f rolesEditForm) firstInvalid() string {
	for _, k := range rolesEditKeys {
		if f.fieldError(k) != "" {
			if k == "permissions" {
				return rolesPickerID + "-field"
			}
			return f.id(k)
		}
	}
	return ""
}

func (f rolesEditForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.id(key), Required: required, Description: help, Error: f.fieldError(key), Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f rolesEditForm) describedBy(key string) string {
	if f.fieldError(key) != "" {
		return f.id(key) + "-err"
	}
	return ""
}

func (f rolesEditForm) action() string { return rolesURL(f.Name) + "/edit" }
func (f rolesEditForm) addAction() string {
	return f.action() + "/add-permission"
}

func (f rolesEditForm) newPermission() string {
	return f.Application + ":" + strings.TrimSpace(f.NewContext) + ":" + strings.TrimSpace(f.NewAggregate) + ":" + strings.TrimSpace(f.NewAction)
}

// editTarget is the checks both the page and its posts make: the SPA loads
// the role (GET /bff/roles/{roleName}) and saves it with PUT.
func (h *RolesPage) editTarget(r *http.Request) (*role.Role, error) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := firstError(auth.CanReadRoles(ac), rolesWrite(ac, auth.CanUpdateRoles)); err != nil {
		return nil, err
	}
	name, err := rolesName(r)
	if err != nil {
		return nil, err
	}
	rl, err := h.loadRole(ctx, name)
	if err != nil {
		return nil, err
	}
	if rl.Source != role.SourceDatabase {
		return nil, errOnlyAdminRoles("edited")
	}
	return rl, nil
}

// editForm completes the editor's state for a role: the picker is built from
// its application's catalogue (the SPA's permissionsApi.list(role.applicationCode)).
func (h *RolesPage) editForm(r *http.Request, ac *auth.AuthContext, rl *role.Role, f rolesEditForm) (rolesEditForm, error) {
	catalogue, err := h.Permissions.Catalogue(r.Context(), rl.ApplicationCode)
	if err != nil {
		return f, err
	}
	f.ID, f.Name, f.Application = rl.ID, rl.Name, rl.ApplicationCode
	f.Picker = newRolesPicker(ac, catalogue, rl.Permissions, f.Picked)
	return f, nil
}

func (h *RolesPage) renderEdit(w http.ResponseWriter, r *http.Request, status int, f rolesEditForm) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Edit Role", Path: rolesPath}, rolesEditPage(f))
}

func (h *RolesPage) editPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	rl, err := h.editTarget(r)
	if err != nil {
		// The SPA warns and returns to the role when it cannot be edited.
		if name, nameErr := rolesName(r); nameErr == nil && auth.CanReadRoles(ac) == nil {
			if key := rolesNotEditableFlash(err); key != "" {
				h.Shell.SetFlash(w, key)
				http.Redirect(w, r, rolesURL(name), http.StatusSeeOther) //nolint:gosec // G710: a local path built from a validated role name
				return
			}
		}
		h.Shell.Fail(w, r, err)
		return
	}
	f, err := h.editForm(r, ac, rl, rolesEditForm{
		DisplayName: rl.DisplayName, Description: deref(rl.Description), ClientManaged: rl.ClientManaged, Picked: rl.Permissions,
	})
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.renderEdit(w, r, http.StatusOK, f)
}

// rolesNotEditableFlash is the SPA's "Not Editable" warning for the reason the
// editor refused ("" for a failure the shell should show).
func rolesNotEditableFlash(err error) string {
	ue, ok := errors.AsType[*usecase.Error](err)
	if !ok {
		return ""
	}
	switch ue.Code {
	case "ROLE_NOT_ADMIN_CREATED":
		return flashRoleNotAdmin
	case "PERMISSION_REQUIRED", "ANCHOR_REQUIRED":
		return flashRoleNoUpdate
	}
	return ""
}

// submittedRolesEdit reads the editor's form: the permissions checked
// (deduplicated, in the order sent), with the locked ones the role holds,
// which are posted back as hidden fields.
func submittedRolesEdit(r *http.Request) rolesEditForm {
	var picked []string
	for _, p := range r.PostForm["permission"] {
		if p = strings.TrimSpace(p); p != "" && !slices.Contains(picked, p) {
			picked = append(picked, p)
		}
	}
	return rolesEditForm{
		DisplayName:   r.PostFormValue("displayName"),
		Description:   r.PostFormValue("description"),
		ClientManaged: r.PostFormValue("clientManaged") != "",
		Picked:        picked,
		NewContext:    r.PostFormValue("newContext"),
		NewAggregate:  r.PostFormValue("newAggregate"),
		NewAction:     r.PostFormValue("newAction"),
	}
}

// update saves the role (PUT /bff/roles/{roleName}): role:update and anchor
// scope. The permissions are sent as the editor holds them; the use case then
// applies the application confinement (F12) and the permission ceiling (F10).
func (h *RolesPage) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	rl, err := h.editTarget(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f, err := h.editForm(r, ac, rl, submittedRolesEdit(r))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if strings.TrimSpace(f.DisplayName) == "" {
		f.Errs.field("displayName", "Display name is required.")
		h.editFailed(w, r, f)
		return
	}
	// The SPA's body: display name and description as typed, the permissions
	// (an empty list clears them) and the client-managed flag.
	desc, perms := f.Description, append([]string{}, f.Picked...)
	cmd := operations.UpdateCommand{ID: rl.ID, DisplayName: &f.DisplayName, Description: &desc, Permissions: perms, ClientManaged: &f.ClientManaged}
	if err := h.Ops.Update(ctx, cmd); err != nil {
		f.Errs.apply(r, err, rolesFieldCodes)
		h.editFailed(w, r, f)
		return
	}
	h.Shell.SetFlash(w, flashRoleUpdated)
	if !ds.IsDatastar(r) {
		http.Redirect(w, r, rolesURL(rl.Name), http.StatusSeeOther)
		return
	}
	if err := ds.NewSSE(w, r).Redirect(rolesURL(rl.Name)); err != nil {
		slog.DebugContext(ctx, "ui: role edit redirect not delivered", "err", err)
	}
}

func (h *RolesPage) editFailed(w http.ResponseWriter, r *http.Request, f rolesEditForm) {
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, rolesEditFormID, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	h.renderEdit(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), f)
}

// addPermission is the editor's "Add" for a new permission of a non-platform
// application: the SPA adds it to the list and selects it, and it reaches the
// role only with Save. The same checks as Save; the form's other values are
// kept as they are.
func (h *RolesPage) addPermission(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	rl, err := h.editTarget(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f, err := h.editForm(r, ac, rl, submittedRolesEdit(r))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.AddNote = h.addNote(&f)
	if f.AddNote == "" {
		// Added: the new permission is checked, the builder is empty again.
		f.Picked = append(f.Picked, f.newPermission())
		f.NewContext, f.NewAggregate, f.NewAction = "", "", ""
		if f, err = h.editForm(r, ac, rl, f); err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
	}
	if !ds.IsDatastar(r) {
		h.renderEdit(w, r, http.StatusOK, f)
		return
	}
	sse := ds.NewSSE(w, r)
	for _, c := range []templ.Component{rolesEditField(f, "permissions"), rolesNewPermission(f)} {
		if err := sse.Patch(c); err != nil {
			slog.DebugContext(ctx, "ui: role permission patch not delivered", "err", err)
		}
	}
}

// addNote is why a new permission is not added ("" when it is): platform
// permissions are code-defined, each segment is a lowercase token, and the
// permission must be new.
func (h *RolesPage) addNote(f *rolesEditForm) string {
	if f.isPlatform() {
		return "Platform permissions are code-defined."
	}
	for _, s := range []string{f.NewContext, f.NewAggregate, f.NewAction} {
		if !bff.ValidPermissionSegment(strings.TrimSpace(s)) {
			return "Each part must be lowercase letters, numbers and hyphens."
		}
	}
	code := f.newPermission()
	if slices.ContainsFunc(f.Picker.Options, func(o kit.Option) bool { return o.Value == code }) ||
		slices.ContainsFunc(f.Picker.Locked, func(l rolesLockedPermission) bool { return l.Code == code }) {
		return code + " already exists."
	}
	return ""
}
