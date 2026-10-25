package pages

import (
	"context"
	"encoding/csv"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	principalapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

// The drawer's actions and their dialogs: state, account actions, roles,
// application and client access, and the CSV import.

const (
	usrRolesDialog  = "usr-roles-form"
	usrAppsDialog   = "usr-apps-form"
	usrGrantDialog  = "usr-grant-form"
	usrPwdDialog    = "usr-pwd-form"
	usrImportDialog = "usr-import-form"
)

// ── state and account actions ──────────────────────────────────────────

// usrAction is a drawer action that needs no form: the API's coarse check for
// it, what it runs and what it flashes.
type usrAction struct {
	check func(*auth.AuthContext) error
	run   func(context.Context, UserService, string) error
	flash string
}

var (
	activateUsrAction = usrAction{
		check: auth.CanUpdatePrincipals, flash: flashUsrActivated,
		run: func(ctx context.Context, s UserService, id string) error { return s.ActivatePrincipal(ctx, id) },
	}
	deactivateUsrAction = usrAction{
		check: auth.CanUpdatePrincipals, flash: flashUsrDeactivated,
		run: func(ctx context.Context, s UserService, id string) error { return s.DeactivatePrincipal(ctx, id) },
	}
	sendResetUsrAction = usrAction{
		check: auth.CanUpdatePrincipals, flash: flashUsrResetSent,
		run: func(ctx context.Context, s UserService, id string) error {
			return s.SendPrincipalPasswordReset(ctx, id)
		},
	}
	resetTwoFactorUsrAction = usrAction{
		check: auth.CanUpdatePrincipals, flash: flashUsrTwoFactorReset,
		run: func(ctx context.Context, s UserService, id string) error { return s.ResetPrincipalTwoFactor(ctx, id) },
	}
)

func (h *Users) activate(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, activateUsrAction)
}

func (h *Users) deactivate(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, deactivateUsrAction)
}

func (h *Users) sendReset(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, sendResetUsrAction)
}

func (h *Users) resetTwoFactor(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, resetTwoFactorUsrAction)
}

func (h *Users) runAction(w http.ResponseWriter, r *http.Request, a usrAction) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := a.check(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	if err := a.run(ctx, h.Users, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, id, "", a.flash, "", false)
}

// delete removes the user (DELETE /api/principals/{id}). The last active
// all-applications administrator cannot be deleted: the API refuses it
// (LAST_ALL_APPLICATIONS_ADMIN) and the answer is that message.
func (h *Users) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanDeletePrincipals(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	// The name for the flash (the SPA's `User "name" deleted`); the delete
	// itself is checked and run by the API.
	var detail string
	if u, err := h.Users.GetPrincipal(ctx, id); err == nil {
		detail = `User "` + u.Name + `" deleted`
	}
	if err := h.Users.DeletePrincipal(ctx, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, id, "", flashUsrDeleted, detail, true)
}

// ── password ───────────────────────────────────────────────────────────

// usrPasswordForm is the direct password reset (the SPA's Reset Password
// dialog): a new password and its confirmation. Neither is ever echoed.
type usrPasswordForm struct {
	usrBase
	UserID, Name, Email string
	Open                bool
}

// usrPasswordCodes maps the password policy's refusals to the new password.
var usrPasswordCodes = map[string]string{
	"PASSWORD_TOO_SHORT":         "newPassword",
	"PASSWORD_TOO_LONG":          "newPassword",
	"PASSWORD_TOO_WEAK":          "newPassword",
	"PASSWORD_TOO_COMMON":        "newPassword",
	"PASSWORD_CONTAINS_IDENTITY": "newPassword",
}

func newUsrPasswordForm(id string) usrPasswordForm {
	return usrPasswordForm{usrBase: newUsrBase(usrPwdDialog, "newPassword", "confirmPassword"), UserID: id}
}

func (f usrPasswordForm) fields() []templ.Component {
	return []templ.Component{usrPasswordField(f, "newPassword"), usrPasswordField(f, "confirmPassword")}
}

func (f usrPasswordForm) component() templ.Component { return usrPasswordDialogC(f) }

func (h *Users) passwordForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := firstError(auth.CanReadPrincipals(ac), auth.CanUpdatePrincipals(ac)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	u, err := h.Users.GetPrincipal(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := newUsrPasswordForm(id)
	f.Name, f.Email, f.Open = u.Name, deref(u.Email), true
	h.showDialog(w, r, ac, id, f, http.StatusOK)
}

// resetPassword sets the user's password (POST .../reset-password): the use
// case holds the password policy and the SPA's mismatch check is repeated
// here. The password is in the request and nowhere else.
func (h *Users) resetPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	f := newUsrPasswordForm(id)
	f.Open = true
	password, confirm := r.PostFormValue("newPassword"), r.PostFormValue("confirmPassword")
	if u, err := h.Users.GetPrincipal(ctx, id); err == nil {
		f.Name, f.Email = u.Name, deref(u.Email)
	}
	if err := auth.CanUpdatePrincipals(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, id, f)
		return
	}
	switch {
	case password == "":
		f.Errs.field("newPassword", "Password is required.")
	case password != confirm:
		f.Errs.field("confirmPassword", "Passwords do not match.")
	}
	if f.Errs.any() {
		h.formFailed(w, r, ac, id, f)
		return
	}
	if err := h.Users.ResetPrincipalPassword(ctx, id, principalapi.ResetPasswordRequest{NewPassword: password}); err != nil {
		f.Errs.apply(r, err, usrPasswordCodes)
		h.formFailed(w, r, ac, id, f)
		return
	}
	h.done(w, r, ac, id, usrPwdDialog, flashUsrPasswordReset, "", false)
}

// ── roles ──────────────────────────────────────────────────────────────

// usrRolesForm is the role picker (the SPA's Manage Roles dialog).
type usrRolesForm struct {
	usrBase
	UserID, Name string
	Choices      usrRoleChoices
	Open         bool
}

func newUsrRolesForm(id string) usrRolesForm {
	return usrRolesForm{usrBase: newUsrBase(usrRolesDialog, "roles"), UserID: id}
}

func (f usrRolesForm) fields() []templ.Component { return nil }

func (f usrRolesForm) component() templ.Component { return usrRolesDialogC(f) }

func (h *Users) rolesForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// The SPA reads the user and the role list, and saves with
	// user:assign-roles.
	if err := firstError(auth.CanReadPrincipals(ac), auth.CanAssignPrincipalRoles(ac), auth.CanReadRoles(ac)); err != nil {
		h.Shell.Fail(w, r, err)
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
	f := newUsrRolesForm(id)
	f.Name, f.Open = d.Name, true
	if f.Choices, err = h.roleChoices(ctx, ac, d); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showDialog(w, r, ac, id, f, http.StatusOK)
}

// saveRoles replaces the user's role set (PUT /api/principals/{id}/roles,
// user:assign-roles). The ceiling and a client administrator's bounds are the
// API's: roles above the caller are kept by the form (hidden fields) and
// refused by the API when a forged post adds or removes one.
func (h *Users) saveRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	f := newUsrRolesForm(id)
	f.Open = true
	if err := auth.CanAssignPrincipalRoles(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, id, f)
		return
	}
	roles := formSet(r.PostForm["roles"])
	res, err := h.Users.AssignPrincipalRoles(ctx, id, roles)
	if err != nil {
		f.Errs.apply(r, err, nil)
		// The dialog is shown again with the choices the caller has.
		if d, derr := h.buildDetail(ctx, ac, id); derr == nil {
			f.Name = d.Name
			f.Choices, _ = h.roleChoices(ctx, ac, d)
		}
		h.formFailed(w, r, ac, id, f)
		return
	}
	h.done(w, r, ac, id, usrRolesDialog, flashUsrRoles, countText("role", len(res.Added), len(res.Removed)), false)
}

// countText is the SPA's "Added 2 role(s), removed 1 role(s)".
func countText(noun string, added, removed int) string {
	n := func(v int) string { return strconv.Itoa(v) + " " + noun + "(s)" }
	switch {
	case added > 0 && removed > 0:
		return "Added " + n(added) + ", removed " + n(removed)
	case added > 0:
		return "Added " + n(added)
	case removed > 0:
		return "Removed " + n(removed)
	}
	return ""
}

// ── application access ─────────────────────────────────────────────────

// usrAppsForm is the application picker (the SPA's Manage Application Access
// dialog).
type usrAppsForm struct {
	usrBase
	UserID, Name string
	Options      []kit.Option
	Selected     []string
	// Kept are grants outside the offered applications (an inactive
	// application): a platform administrator's save keeps them, as the SPA's
	// selection did; a client administrator's is bounded by the API, which
	// preserves what is out of reach.
	Kept []usrAppRow
	Open bool
}

func newUsrAppsForm(id string) usrAppsForm {
	return usrAppsForm{usrBase: newUsrBase(usrAppsDialog, "applicationIds"), UserID: id}
}

func (f usrAppsForm) fields() []templ.Component { return nil }

func (f usrAppsForm) component() templ.Component { return usrAppsDialogC(f) }

func (h *Users) appsForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := firstError(auth.CanReadPrincipals(ac), auth.CanUpdatePrincipals(ac)); err != nil {
		h.Shell.Fail(w, r, err)
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
	avail, err := h.Users.ListPrincipalAvailableApplications(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := newUsrAppsForm(id)
	f.Name, f.Open = d.Name, true
	offered := map[string]bool{}
	for _, a := range avail {
		f.Options = append(f.Options, kit.Option{Value: a.ID, Label: a.Name + " (" + a.Code + ")"})
		offered[a.ID] = true
	}
	slices.SortFunc(f.Options, func(a, b kit.Option) int { return strings.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label)) })
	for _, g := range d.Apps {
		switch {
		case offered[g.ID]:
			f.Selected = append(f.Selected, g.ID)
		case ac.IsAnchor():
			f.Kept = append(f.Kept, g)
		}
	}
	h.showDialog(w, r, ac, id, f, http.StatusOK)
}

// saveApps replaces the user's application grants (PUT
// /api/principals/{id}/application-access); the all-applications flag is left
// as it is.
func (h *Users) saveApps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	f := newUsrAppsForm(id)
	f.Open = true
	if err := auth.CanUpdatePrincipals(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, id, f)
		return
	}
	res, err := h.Users.AssignPrincipalApplicationAccess(ctx, id, principalapi.AssignApplicationAccessRequest{ApplicationIDs: formSet(r.PostForm["applicationIds"])})
	if err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, id, f)
		return
	}
	h.done(w, r, ac, id, usrAppsDialog, flashUsrApps, countText("app", res.Added, res.Removed), false)
}

// grantAllApps and restrictAllApps flip the all-applications flag (the SPA's
// "Access to all applications" switch), keeping the explicit grants so turning
// it off restores them. Only an all-applications administrator may grant it
// and removing your own flag needs the confirmation; both are the API's.
func (h *Users) grantAllApps(w http.ResponseWriter, r *http.Request) { h.setAllApps(w, r, true) }

func (h *Users) restrictAllApps(w http.ResponseWriter, r *http.Request) { h.setAllApps(w, r, false) }

func (h *Users) setAllApps(w http.ResponseWriter, r *http.Request, on bool) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanUpdatePrincipals(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	current, err := h.Users.ListPrincipalApplicationAccess(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	body := principalapi.AssignApplicationAccessRequest{AllApplications: &on}
	for _, a := range current.Applications {
		body.ApplicationIDs = append(body.ApplicationIDs, a.ApplicationID)
	}
	if !on && r.PostFormValue("confirmSelf") == "1" {
		yes := true
		body.ConfirmSelfRestriction = &yes
	}
	if _, err := h.Users.AssignPrincipalApplicationAccess(ctx, id, body); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	flash := flashUsrAllApps
	if !on {
		flash = flashUsrRestricted
	}
	h.done(w, r, ac, id, "", flash, "", false)
}

// ── client access ──────────────────────────────────────────────────────

// usrGrantForm is the Grant Client Access dialog.
type usrGrantForm struct {
	usrBase
	UserID, Name string
	ClientID     string
	Choices      []kit.Option
	CanPick      bool
	Open         bool
}

func newUsrGrantForm(id string) usrGrantForm {
	return usrGrantForm{usrBase: newUsrBase(usrGrantDialog, "clientId"), UserID: id}
}

func (f usrGrantForm) fields() []templ.Component { return []templ.Component{usrGrantField(f)} }

func (f usrGrantForm) component() templ.Component { return usrGrantDialogC(f) }

// withChoices offers the clients the user has no access to yet (the SPA's
// availableClients: every client but the home client and the granted ones).
func (h *Users) grantChoices(ctx context.Context, ac *auth.AuthContext, id string, f *usrGrantForm) error {
	f.CanPick = auth.CanReadClients(ac) == nil && h.Clients != nil
	if !f.CanPick {
		return nil
	}
	u, err := h.Users.GetPrincipal(ctx, id)
	if err != nil {
		return err
	}
	f.Name = u.Name
	grants, err := h.Users.ListPrincipalClientAccess(ctx, id)
	if err != nil {
		return err
	}
	has := map[string]bool{deref(u.ClientID): true}
	for _, g := range grants {
		has[g.ClientID] = true
	}
	all, err := h.Clients.FindAll(ctx)
	if err != nil {
		return err
	}
	slices.SortFunc(all, func(a, b client.Client) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	f.Choices = nil
	for i := range all {
		if !has[string(all[i].ID)] {
			f.Choices = append(f.Choices, kit.Option{Value: string(all[i].ID), Label: all[i].Name + " (" + all[i].Identifier + ")"})
		}
	}
	return nil
}

func (h *Users) grantForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// The client-access routes need anchor tier and user:manage-access.
	if err := firstError(auth.CanReadPrincipals(ac), auth.CanManagePrincipalAccess(ac)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	f := newUsrGrantForm(id)
	f.Open = true
	if err := h.grantChoices(ctx, ac, id, &f); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showDialog(w, r, ac, id, f, http.StatusOK)
}

func (h *Users) grantClient(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	f := newUsrGrantForm(id)
	f.Open = true
	f.ClientID = strings.TrimSpace(r.PostFormValue("clientId"))
	fail := func() {
		if err := h.grantChoices(ctx, ac, id, &f); err != nil {
			slog.WarnContext(ctx, "ui: client choices unavailable", "err", err)
		}
		h.formFailed(w, r, ac, id, f)
	}
	if err := auth.CanManagePrincipalAccess(ac); err != nil {
		f.Errs.apply(r, err, nil)
		fail()
		return
	}
	if f.ClientID == "" || !kit.ValidID(f.ClientID) {
		f.Errs.field("clientId", "Select a client.")
		fail()
		return
	}
	if err := h.Users.GrantPrincipalClientAccess(ctx, id, f.ClientID); err != nil {
		f.Errs.apply(r, err, usrFieldCodes)
		fail()
		return
	}
	h.done(w, r, ac, id, usrGrantDialog, flashUsrClientGranted, "", false)
}

func (h *Users) revokeClient(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanManagePrincipalAccess(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, ok := h.requireID(w, r)
	if !ok {
		return
	}
	clientID := chi.URLParam(r, "clientId")
	if !kit.ValidID(clientID) {
		h.Shell.Fail(w, r, httperror.NotFound("Client", clientID))
		return
	}
	if err := h.Users.RevokePrincipalClientAccess(ctx, id, clientID); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, id, "", flashUsrClientRevoked, "", false)
}

// ── CSV import ─────────────────────────────────────────────────────────

// maxImportRows is the API's limit (POST /api/principals/bulk-import).
const maxImportRows = 1000

// usrImportForm is the CSV import dialog (the SPA's UserImportDialog): a
// client, and a file of Full Name, Email and pipe-separated Roles.
type usrImportForm struct {
	usrBase
	ClientID string
	Choices  []kit.Option
	// CanAssign is whether the caller can assign the rows' roles; without it
	// the users are created without them.
	CanAssign bool
	Open      bool
}

func newUsrImportForm() usrImportForm {
	return usrImportForm{usrBase: newUsrBase(usrImportDialog, "clientId", "file")}
}

func (f usrImportForm) fields() []templ.Component {
	return []templ.Component{usrImportField(f, "clientId"), usrImportField(f, "file")}
}

func (f usrImportForm) component() templ.Component { return usrImportDialogC(f) }

func (h *Users) importChoices(r *http.Request, ac *auth.AuthContext, f *usrImportForm) {
	f.CanAssign = auth.CanAssignPrincipalRoles(ac) == nil
	cs, err := h.clientChoices(r.Context(), ac)
	if err != nil {
		slog.WarnContext(r.Context(), "ui: clients unavailable", "err", err)
		return
	}
	f.Choices = clientOptionsOf(cs)
	// One client is implicit; several are a choice.
	if f.ClientID == "" && len(f.Choices) == 1 {
		f.ClientID = f.Choices[0].Value
	}
}

func (h *Users) importForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanCreatePrincipals(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := newUsrImportForm()
	f.Open = true
	h.importChoices(r, ac, &f)
	h.showDialog(w, r, ac, "", f, http.StatusOK)
}

// importTemplate is the SPA's template file.
func (h *Users) importTemplate(w http.ResponseWriter, r *http.Request) {
	if err := auth.CanCreatePrincipals(auth.FromContext(r.Context())); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="user-import-template.csv"`)
	_, _ = io.WriteString(w, "Full Name,Email,Roles (| separated)\nJane Doe,jane.doe@example.com,role-one|role-two\nJohn Smith,john.smith@example.com,\n")
}

// parseUsrCSV reads the SPA's import file: a header row (recognised by
// "name" and "email") is skipped, then Full Name, Email and Roles separated by
// "|"; rows without a name and an email are dropped.
func parseUsrCSV(src io.Reader) ([]principalapi.BulkImportUser, error) {
	cr := csv.NewReader(src)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	records, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}
	var rows []principalapi.BulkImportUser
	for i, rec := range records {
		if len(rec) == 0 || (len(rec) == 1 && strings.TrimSpace(rec[0]) == "") {
			continue
		}
		if i == 0 {
			first := strings.ToLower(strings.Join(rec, ","))
			if strings.Contains(first, "email") && strings.Contains(first, "name") {
				continue
			}
		}
		col := func(n int) string {
			if n < len(rec) {
				return strings.TrimSpace(rec[n])
			}
			return ""
		}
		u := principalapi.BulkImportUser{Name: col(0), Email: col(1)}
		for role := range strings.SplitSeq(col(2), "|") {
			if role = strings.TrimSpace(role); role != "" {
				u.Roles = append(u.Roles, role)
			}
		}
		if u.Name == "" && u.Email == "" {
			continue
		}
		rows = append(rows, u)
	}
	return rows, nil
}

// importCSV creates the file's users under the chosen client (POST
// /api/principals/bulk-import). The API skips existing users and the ones whose
// domain belongs to another client, and reports each row.
func (h *Users) importCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := newUsrImportForm()
	f.Open = true
	f.ClientID = strings.TrimSpace(r.PostFormValue("clientId"))
	fail := func() {
		h.importChoices(r, ac, &f)
		h.formFailed(w, r, ac, "", f)
	}
	if err := auth.CanCreatePrincipals(ac); err != nil {
		f.Errs.apply(r, err, nil)
		fail()
		return
	}
	if f.ClientID == "" {
		f.Errs.field("clientId", "Select a client.")
	}
	var rows []principalapi.BulkImportUser
	file, _, err := r.FormFile("file")
	switch {
	case err != nil:
		f.Errs.field("file", "Choose a CSV file with at least one user.")
	default:
		defer func() { _ = file.Close() }()
		if rows, err = parseUsrCSV(file); err != nil {
			f.Errs.field("file", "Could not read the file.")
		} else if len(rows) == 0 {
			f.Errs.field("file", "No user rows found in the file.")
		} else if len(rows) > maxImportRows {
			f.Errs.field("file", "Import is limited to "+strconv.Itoa(maxImportRows)+" users at a time.")
		}
	}
	if f.Errs.any() {
		fail()
		return
	}
	res, err := h.Users.BulkImportPrincipals(ctx, principalapi.BulkImportRequest{ClientID: f.ClientID, Users: rows})
	if err != nil {
		f.Errs.apply(r, err, nil)
		fail()
		return
	}
	report := usrImportReportC(res)
	if !ds.IsDatastar(r) {
		view, err := h.listView(ctx, ac, parseListQuery(nil, usrList))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		view.Dialog = report
		h.renderPage(w, r, http.StatusOK, view)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(usrImportDialog); err != nil {
		slog.DebugContext(ctx, "ui: import close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, "", false)
	if err := patchDialogSlot(sse, usrDialogSlot, report); err != nil {
		slog.DebugContext(ctx, "ui: import report not delivered", "err", err)
	}
	detail := strconv.Itoa(res.Created) + " created, " + strconv.Itoa(res.Skipped) + " skipped, " + strconv.Itoa(res.Failed) + " failed"
	if err := usrFlash(sse, flashUsrImported, detail); err != nil {
		slog.DebugContext(ctx, "ui: import flash not delivered", "err", err)
	}
}

// importStatusBadge is the SPA's rowSeverity: existing and dropped rows are
// skips (amber), anything else not created an error (red).
func importStatusBadge(status string) kit.BadgeVariant {
	if status == "exists" || status == "dropped" {
		return kit.BadgeWarn
	}
	return kit.BadgeDestructive
}

// picker is the application picker: the offered applications, the granted
// ones ticked.
func (f usrAppsForm) picker() kit.PickListProps {
	items := make([]kit.PickItem, 0, len(f.Options))
	for i, o := range f.Options {
		items = append(items, kit.PickItem{Value: o.Value, Key: "a" + strconv.Itoa(i), Name: o.Label, Checked: slices.Contains(f.Selected, o.Value)})
	}
	return kit.PickListProps{
		ID: "usr-apps-pick", Signal: "usrAppsPicked", FilterSignal: "usrAppQ", Name: "applicationIds", ItemIDPrefix: "usr-app-",
		Items: items, AvailableTitle: "Available applications (" + strconv.Itoa(len(items)) + ")", SelectedTitle: "Selected",
		FilterPlaceholder: "Filter applications", EmptyText: "No applications selected", Bulk: true,
	}
}
