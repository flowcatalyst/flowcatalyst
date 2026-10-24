package pages

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// developerRole is the seeded role that lets a user hold a developer API
// credential (the same literal the principal API and use cases carry).
const developerRole = "platform:developer"

// DeveloperUserStore is the slice of the principal repository the page reads:
// GET /api/principals/developer-users (FindByRole), the grant dialog's
// search (GET /api/principals, FindAll) and the target of an action.
type DeveloperUserStore interface {
	FindByRole(ctx context.Context, roleName string) ([]principal.Principal, error)
	FindAll(ctx context.Context) ([]principal.Principal, error)
	FindByID(ctx context.Context, id ids.PrincipalID) (*principal.Principal, error)
}

// DeveloperUserCommands are the operations the page runs. SetCredential
// returns the new secret, which exists nowhere else.
type DeveloperUserCommands interface {
	GrantRole(ctx context.Context, userID string) error
	RemoveRole(ctx context.Context, userID string) error
	SetCredential(ctx context.Context, userID string) (secret string, err error)
	RevokeCredential(ctx context.Context, userID string) error
}

// DeveloperUserOps runs the principal use cases through the unit of work, as
// the principal API does: the role changes are AssignRoles (the SPA replaces
// the user's role set with the developer role added or removed), the
// credential is SetDeveloperCredential and RevokeDeveloperCredential.
type DeveloperUserOps struct {
	Repo  *principal.Repository
	Roles *role.Repository
	UoW   *usecasepgx.UnitOfWork
}

// devRoleNames are a principal's role names.
func devRoleNames(p *principal.Principal) []string {
	out := make([]string, 0, len(p.Roles))
	for _, ra := range p.Roles {
		out = append(out, ra.Role.String())
	}
	return out
}

// GrantRole adds the developer role to the user's roles.
func (o DeveloperUserOps) GrantRole(ctx context.Context, userID string) error {
	p, err := o.Repo.FindByID(ctx, ids.PrincipalID(userID))
	if err != nil {
		return usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return httperror.NotFound("Principal", userID)
	}
	roles := devRoleNames(p)
	if !slices.Contains(roles, developerRole) {
		roles = append(roles, developerRole)
	}
	return o.assign(ctx, p.ID, roles)
}

// RemoveRole takes the developer role off the user's roles (a user without
// it is left alone, as the API's remove-role does).
func (o DeveloperUserOps) RemoveRole(ctx context.Context, userID string) error {
	p, err := o.Repo.FindByID(ctx, ids.PrincipalID(userID))
	if err != nil {
		return usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return httperror.NotFound("Principal", userID)
	}
	roles := devRoleNames(p)
	if !slices.Contains(roles, developerRole) {
		return nil
	}
	return o.assign(ctx, p.ID, slices.DeleteFunc(roles, func(r string) bool { return r == developerRole }))
}

func (o DeveloperUserOps) assign(ctx context.Context, id ids.PrincipalID, roles []string) error {
	_, err := usecaseop.Run(ctx, o.UoW, operations.AssignRoles(o.Repo, o.Roles),
		operations.AssignRolesCommand{UserID: id, Roles: roles}, auth.NewExecutionContext(ctx))
	return err
}

// SetCredential creates or rotates the secret and collects the plaintext the
// use case stashes for exactly this purpose.
func (o DeveloperUserOps) SetCredential(ctx context.Context, userID string) (string, error) {
	ev, err := usecaseop.Run(ctx, o.UoW, operations.SetDeveloperCredential(o.Repo),
		operations.SetDeveloperCredentialCommand{PrincipalID: ids.PrincipalID(userID)}, auth.NewExecutionContext(ctx))
	if err != nil {
		return "", err
	}
	secret, ok := operations.PopDevClientSecret(string(ev.UserID))
	if !ok {
		return "", usecase.Internal("SECRET", "the new developer secret is no longer available", nil)
	}
	return secret, nil
}

// RevokeCredential clears the secret and leaves the role.
func (o DeveloperUserOps) RevokeCredential(ctx context.Context, userID string) error {
	_, err := usecaseop.Run(ctx, o.UoW, operations.RevokeDeveloperCredential(o.Repo),
		operations.RevokeDeveloperCredentialCommand{PrincipalID: ids.PrincipalID(userID)}, auth.NewExecutionContext(ctx))
	return err
}

// DeveloperUsers is the Developer Users page (/ui/developer-users),
// mirroring the SPA's DeveloperUsersListPage. The list makes the API's check
// (CanReadPrincipals); each action makes the check of the endpoint the SPA
// calls. The role actions are the API's user:assign-roles and, because the
// API's own bounds for a non-anchor administrator (only application roles of
// reachable applications) would refuse the platform role anyway, anchor only.
type DeveloperUsers struct {
	Store DeveloperUserStore
	Ops   DeveloperUserCommands
	Shell *shell.Shell
}

const (
	devUsersPath = "/ui/developer-users"
	devUsersForm = "dev-users-list"

	devUsersDialogSlot = "dev-users-dialog-slot"
	devUsersDialogID   = "dev-users-dialog"
	devSecretDialogID  = "dev-secret-dialog"
)

var devUserList = &listSpec{
	path:        devUsersPath,
	sortKeys:    []string{"name", "email"},
	statusOK:    func(string) bool { return false },
	rows:        []int{50, 100, 250, 500},
	defaultRows: 100,
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *DeveloperUsers) Mount(r chi.Router) {
	r.Get("/developer-users", h.list)
	r.Get("/developer-users/grant", h.grantDialog)
	r.Get("/developer-users/grant/search", h.grantSearch)
	r.Get("/developer-users/{id}/confirm/{action}", h.confirm)
	post := r.With(parseForm)
	post.Post("/developer-users/{id}/grant", h.grant)
	post.Post("/developer-users/{id}/remove-role", h.removeRole)
	post.Post("/developer-users/{id}/credential", h.setCredential)
	post.Post("/developer-users/{id}/revoke", h.revoke)
}

// ── the API's checks ───────────────────────────────────────────────────

// devRoleChangeAccess: PUT /api/principals/{id}/roles and DELETE .../roles/{role}
// need user:assign-roles; the page adds the anchor requirement (above).
func devRoleChangeAccess(ac *auth.AuthContext) error {
	if err := auth.CanAssignPrincipalRoles(ac); err != nil {
		return err
	}
	return auth.RequireAnchor(ac)
}

// devCredentialAccess is the principal API's requireDeveloperCredentialAccess:
// a caller acting on their own credential needs the self-service permission,
// on anyone else's user:update. The use case then checks the target (a
// developer, in the caller's scope).
func devCredentialAccess(ac *auth.AuthContext, targetID string) error {
	if ac != nil && ac.PrincipalID == targetID {
		return auth.CanManageOwnDeveloperCredential(ac)
	}
	return auth.CanUpdatePrincipals(ac)
}

// devVisible is the principal list's scoping: an anchor sees everyone, any
// other caller the principals of the clients they can reach.
func devVisible(ac *auth.AuthContext, p *principal.Principal) bool {
	return ac.IsAnchor() || (p.ClientID != nil && ac.CanAccessClient(string(*p.ClientID)))
}

func devEmail(p *principal.Principal) string {
	if p.UserIdentity == nil {
		return ""
	}
	return p.UserIdentity.Email
}

// ── list ───────────────────────────────────────────────────────────────

type devUserRow struct {
	ID, Name, Email string
	HasCredential   bool
	Rotated         time.Time
	// CanCredential and CanRemove are the controls the caller may use; the
	// handlers check again.
	CanCredential, CanRemove bool
}

type devUsersView struct {
	Query       listQuery
	Rows        []devUserRow
	Total       int
	Page, Pages int
	CanGrant    bool
	Dialog      templ.Component
}

func (h *DeveloperUsers) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadPrincipals(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(r.URL.Query(), devUserList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(view.patchTable(sse), sse.Patch(kit.TableToolbarState(view.toolbar().State())), sse.ReplaceURL(view.Query.URL())); err != nil {
			slog.DebugContext(ctx, "ui: developer users patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *DeveloperUsers) renderPage(w http.ResponseWriter, r *http.Request, status int, view devUsersView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Developer Users", Path: devUsersPath}, devUsersPage(view))
}

func (h *DeveloperUsers) listView(ctx context.Context, ac *auth.AuthContext, q listQuery) (devUsersView, error) {
	all, err := h.Store.FindByRole(ctx, developerRole)
	if err != nil {
		return devUsersView{}, usecase.Internal("REPO", "find_by_role failed", err)
	}
	return buildDevUsersView(ac, q, all), nil
}

func buildDevUsersView(ac *auth.AuthContext, q listQuery, all []principal.Principal) devUsersView {
	v := devUsersView{CanGrant: devRoleChangeAccess(ac) == nil}
	needle := strings.ToLower(q.Q)
	shown := make([]*principal.Principal, 0, len(all))
	for i := range all {
		p := &all[i]
		if !devVisible(ac, p) ||
			(needle != "" && !strings.Contains(strings.ToLower(p.Name), needle) && !strings.Contains(strings.ToLower(devEmail(p)), needle)) {
			continue
		}
		shown = append(shown, p)
	}
	key := func(p *principal.Principal) string {
		if q.Sort == "email" {
			return strings.ToLower(devEmail(p))
		}
		return strings.ToLower(p.Name)
	}
	slices.SortStableFunc(shown, func(a, b *principal.Principal) int {
		c := strings.Compare(key(a), key(b))
		if q.Dir == kit.SortDesc {
			c = -c
		}
		return c
	})
	v.Total = len(shown)
	var lo, hi int
	q.Page, v.Pages, lo, hi = q.pageBounds(len(shown))
	v.Page, v.Query = q.Page, q
	roleOK := devRoleChangeAccess(ac) == nil
	for _, p := range shown[lo:hi] {
		row := devUserRow{
			ID: string(p.ID), Name: p.Name, Email: devEmail(p), HasCredential: p.HasDevClientSecret(),
			CanCredential: devCredentialAccess(ac, string(p.ID)) == nil, CanRemove: roleOK,
		}
		if p.UserIdentity != nil && p.UserIdentity.DevClientSecretUpdatedAt != nil {
			row.Rotated = *p.UserIdentity.DevClientSecretUpdatedAt
		}
		v.Rows = append(v.Rows, row)
	}
	return v
}

func (v devUsersView) patchTable(sse *ds.SSE) error {
	return errors.Join(sse.Patch(devUsersTable(v)), sse.Patch(kit.Paginator(v.pager())))
}

func devUserURL(id, action string) string {
	return devUsersPath + "/" + url.PathEscape(id) + "/" + action
}

// rowActions mirrors the SPA's three buttons: set or rotate the secret,
// revoke it (when there is one), remove the role. A control the caller cannot
// use is left out. Setting a first secret asks nothing; the rest confirm in a
// dialog that names the user.
func (v devUsersView) rowActions(row devUserRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "dev-user-" + row.ID + "-actions", RowLabel: row.Name}
	dialog := templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}
	if row.CanCredential {
		if row.HasCredential {
			p.Actions = append(p.Actions, kit.RowAction{Icon: "key-round", Label: "Rotate Secret", Href: devUserURL(row.ID, "confirm/rotate"), Attrs: dialog})
		} else {
			p.Actions = append(p.Actions, kit.RowAction{Icon: "key-round", Label: "Set Secret", Attrs: templ.Attributes{"data-action": devUserURL(row.ID, "credential"), "data-on:click": "@post(el.dataset.action, {retryMaxCount: 0})"}})
		}
		if row.HasCredential {
			p.Actions = append(p.Actions, kit.RowAction{Icon: "ban", Label: "Revoke Credential", Href: devUserURL(row.ID, "confirm/revoke"), Attrs: dialog})
		}
	}
	if row.CanRemove {
		p.Actions = append(p.Actions, kit.RowAction{Icon: "user-minus", Label: "Remove Developer Role", Destructive: true, Href: devUserURL(row.ID, "confirm/remove"), Attrs: dialog})
	}
	return p
}

// ── the grant dialog ───────────────────────────────────────────────────

// devGrantMax is how many matches the dialog offers (the SPA asks for 15).
const devGrantMax = 15

type devGrantResult struct {
	ID, Name, Email, Initials, Action string
}

type devGrantView struct {
	Q       string
	Results []devGrantResult
}

func (h *DeveloperUsers) grantDialog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := h.grantAccess(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	dialog := devGrantDialog(devGrantView{})
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), devUsersDialogSlot, dialog); err != nil {
			slog.DebugContext(ctx, "ui: grant dialog not delivered", "err", err)
		}
		return
	}
	h.renderWithDialog(w, r, ac, dialog, http.StatusOK)
}

// grantAccess: the dialog searches users (GET /api/principals) to change a
// role set (PUT .../roles).
func (h *DeveloperUsers) grantAccess(ac *auth.AuthContext) error {
	if err := auth.CanReadPrincipals(ac); err != nil {
		return err
	}
	return devRoleChangeAccess(ac)
}

func (h *DeveloperUsers) renderWithDialog(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, dialog templ.Component, status int) {
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, devUserList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Dialog = dialog
	h.renderPage(w, r, status, view)
}

// grantSearch answers the dialog's search: USER principals matching the text
// that are not developers yet, as GET /api/principals?q=&type=USER does.
func (h *DeveloperUsers) grantSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := h.grantAccess(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		http.Redirect(w, r, devUsersPath+"/grant", http.StatusSeeOther)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > maxQueryLen {
		q = q[:maxQueryLen]
	}
	view := devGrantView{Q: q}
	if q != "" {
		results, err := h.grantResults(ctx, ac, q)
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		view.Results = results
	}
	if err := ds.NewSSE(w, r).Patch(devGrantResults(view)); err != nil {
		slog.DebugContext(ctx, "ui: grant search not delivered", "err", err)
	}
}

func (h *DeveloperUsers) grantResults(ctx context.Context, ac *auth.AuthContext, q string) ([]devGrantResult, error) {
	all, err := h.Store.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	needle := strings.ToLower(q)
	var out []devGrantResult
	for i := range all {
		p := &all[i]
		if p.Type != principal.TypeUser || !devVisible(ac, p) || slices.Contains(devRoleNames(p), developerRole) ||
			(!strings.Contains(strings.ToLower(p.Name), needle) && !strings.Contains(strings.ToLower(devEmail(p)), needle)) {
			continue
		}
		out = append(out, devGrantResult{
			ID: string(p.ID), Name: p.Name, Email: devEmail(p), Initials: devInitials(p.Name),
			Action: devUserURL(string(p.ID), "grant"),
		})
		if len(out) == devGrantMax {
			break
		}
	}
	return out, nil
}

// devInitials is the SPA's: the first letter of the first and last word.
func devInitials(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return "?"
	}
	out := []rune(strings.ToUpper(string([]rune(parts[0])[:1])))
	if len(parts) > 1 {
		out = append(out, []rune(strings.ToUpper(string([]rune(parts[len(parts)-1])[:1])))...)
	}
	return string(out)
}

// ── actions ────────────────────────────────────────────────────────────

func devUserID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("Principal", id)
	}
	return id, nil
}

// target reads the user an action is about, for the names in its messages
// and to answer "not found" for one the caller may not reach, as the API
// does.
func (h *DeveloperUsers) target(ctx context.Context, ac *auth.AuthContext, id string) (*principal.Principal, error) {
	p, err := h.Store.FindByID(ctx, ids.PrincipalID(id))
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	// A caller's own record is always theirs (the use case's self branch).
	if p == nil || (ac.PrincipalID != id && !auth.CanAccessScope(ac, ids.StringPtr(p.ClientID))) {
		return nil, httperror.NotFound("Principal", id)
	}
	return p, nil
}

// devConfirm is one of the three confirmations. The wording is the SPA's
// (credentialConfirm.ts for the rotation).
type devConfirm struct {
	Title, Message, Label, Action string
}

func devConfirmFor(action string, p *principal.Principal) (devConfirm, func(*auth.AuthContext, string) error, bool) {
	base := devUserURL(string(p.ID), "")
	switch action {
	case "rotate":
		return devConfirm{
			Title: "Rotate developer secret?", Label: "Rotate secret", Action: base + "credential",
			Message: fmt.Sprintf("The current developer client secret for %q stops working immediately. Any scripts or tools using it will fail to authenticate until they are given the new one. The new value is shown once, so copy it before closing.", p.Name),
		}, devCredentialAccess, true
	case "revoke":
		return devConfirm{
			Title: "Revoke Credential", Label: "Revoke", Action: base + "revoke",
			Message: fmt.Sprintf("Revoke the developer API credential for %q? Any scripts using it will stop working immediately.", p.Name),
		}, devCredentialAccess, true
	case "remove":
		return devConfirm{
			Title: "Remove Developer Role", Label: "Remove", Action: base + "remove-role",
			Message: fmt.Sprintf("Remove the developer role from %q? Their API credential (if set) will be revoked too — re-granting the role later will require setting a fresh one.", p.Name),
		}, func(ac *auth.AuthContext, _ string) error { return devRoleChangeAccess(ac) }, true
	}
	return devConfirm{}, nil, false
}

// confirm answers the confirmation dialog of an action, after the same check
// the action makes.
func (h *DeveloperUsers) confirm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := devUserID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	p, err := h.target(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	c, check, ok := devConfirmFor(chi.URLParam(r, "action"), p)
	if !ok {
		h.Shell.Fail(w, r, httperror.NotFound("Action", chi.URLParam(r, "action")))
		return
	}
	if err := check(ac, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	dialog := devConfirmDialog(c)
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), devUsersDialogSlot, dialog); err != nil {
			slog.DebugContext(ctx, "ui: confirmation not delivered", "err", err)
		}
		return
	}
	h.renderWithDialog(w, r, ac, dialog, http.StatusOK)
}

// devAction is one posted action: its check, the operation and what the
// user is told.
type devAction struct {
	check func(*auth.AuthContext, string) error
	run   func(*DeveloperUsers, context.Context, *principal.Principal) error
	// flash is the toast after a Datastar post; key the one after a native
	// post (which is redirected).
	flash func(*principal.Principal) kit.FlashMessageProps
	key   string
}

var (
	grantDevAction = devAction{
		check: func(ac *auth.AuthContext, _ string) error { return devRoleChangeAccess(ac) },
		run: func(h *DeveloperUsers, ctx context.Context, p *principal.Principal) error {
			return h.Ops.GrantRole(ctx, string(p.ID))
		},
		flash: func(p *principal.Principal) kit.FlashMessageProps {
			return devFlash("Granted", p.Name+" can now hold a developer API credential")
		},
		key: flashDevGranted,
	}
	removeDevAction = devAction{
		check: func(ac *auth.AuthContext, _ string) error { return devRoleChangeAccess(ac) },
		run:   (*DeveloperUsers).removeRoleAndCredential,
		flash: func(p *principal.Principal) kit.FlashMessageProps {
			return devFlash("Removed", "Developer role removed from "+p.Name)
		},
		key: flashDevRemoved,
	}
	revokeDevAction = devAction{
		check: devCredentialAccess,
		run: func(h *DeveloperUsers, ctx context.Context, p *principal.Principal) error {
			return h.Ops.RevokeCredential(ctx, string(p.ID))
		},
		flash: func(p *principal.Principal) kit.FlashMessageProps {
			return devFlash("Revoked", "Developer credential revoked for "+p.Name)
		},
		key: flashDevRevoked,
	}
)

func devFlash(summary, detail string) kit.FlashMessageProps {
	return kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: summary, Detail: detail}
}

// removeRoleAndCredential removes the role, then revokes the credential the
// role no longer allows, so that re-granting the role later does not bring
// the old secret back. As in the SPA the revoke is best effort: it needs
// the credential permission, and its failure does not undo the removal.
func (h *DeveloperUsers) removeRoleAndCredential(ctx context.Context, p *principal.Principal) error {
	if err := h.Ops.RemoveRole(ctx, string(p.ID)); err != nil {
		return err
	}
	if p.HasDevClientSecret() {
		if err := h.Ops.RevokeCredential(ctx, string(p.ID)); err != nil {
			slog.InfoContext(ctx, "ui: developer credential not revoked with the role", "principal", p.ID, "err", err)
		}
	}
	return nil
}

func (h *DeveloperUsers) grant(w http.ResponseWriter, r *http.Request) { h.act(w, r, grantDevAction) }

func (h *DeveloperUsers) removeRole(w http.ResponseWriter, r *http.Request) {
	h.act(w, r, removeDevAction)
}

func (h *DeveloperUsers) revoke(w http.ResponseWriter, r *http.Request) { h.act(w, r, revokeDevAction) }

// act runs an action: the check first (nothing is read for a caller without
// it), the target, the operation, then the refreshed table, the dialog
// removed and a toast; a native post is redirected to the list.
func (h *DeveloperUsers) act(w http.ResponseWriter, r *http.Request, a devAction) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := devUserID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := a.check(ac, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	p, err := h.target(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := a.run(h, ctx, p); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, a.key)
		http.Redirect(w, r, devUsersPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac)
	if err := errors.Join(sse.Remove("#"+devUsersDialogID), shell.Flash(sse, a.flash(p))); err != nil {
		slog.DebugContext(ctx, "ui: developer user feedback not delivered", "err", err)
	}
}

// afterChange re-renders the table the user is on.
func (h *DeveloperUsers) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext) {
	view, err := h.listView(r.Context(), ac, refererList(r, devUserList))
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := view.patchTable(sse); err != nil {
		slog.DebugContext(r.Context(), "ui: developer users table patch not delivered", "err", err)
	}
}

// ── the secret ─────────────────────────────────────────────────────────

// devSecret is what the secret dialog shows. It exists in the response that
// creates it only.
type devSecret struct {
	ClientID, Secret string
}

// setCredential creates or rotates a developer's secret and shows it once:
// in a dialog in this response, which is no-store, with no copy in a URL,
// flash, signal or redirect (a native post gets the page itself, as
// post/redirect/get would lose the value). The dialog removes itself however
// it is closed.
func (h *DeveloperUsers) setCredential(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	w = noStore{w}
	id, err := devUserID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := devCredentialAccess(ac, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	p, err := h.target(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	verb := "set"
	if p.HasDevClientSecret() {
		verb = "rotated"
	}
	secret, err := h.Ops.SetCredential(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	dialog := devSecretDialog(devSecret{ClientID: id, Secret: secret})
	if !ds.IsDatastar(r) {
		h.renderWithDialog(w, r, ac, dialog, http.StatusOK)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac)
	if err := errors.Join(
		patchDialogSlot(sse, devUsersDialogSlot, dialog),
		shell.Flash(sse, devFlash("Success", "Developer credential "+verb+" for "+p.Name)),
	); err != nil {
		slog.DebugContext(ctx, "ui: developer secret dialog not delivered", "err", err)
	}
}
