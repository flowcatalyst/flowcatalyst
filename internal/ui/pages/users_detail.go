package pages

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	principalapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

// The record drawer (the SPA's UserDetailDrawer and UserDetailBody): the
// user's details, client access, roles, application access and account
// actions, one scrolling column as in the SPA.

type usrRoleRow struct {
	Name, Short, Display string
	Source               string
	Assigned             string
}

type usrAppRow struct {
	ID, Name, Code string
}

type usrGrantRow struct {
	ClientID, Name, Identifier string
	Granted                    string
}

type usrClientRef struct {
	Name, Identifier string
}

type usrDetail struct {
	ID, Name, Email string
	// Scope and ClientID are the stored scope and home client, as the edit form
	// compares against them.
	Scope, ClientID string
	// Auth is the SPA's Authentication field: "Internal" or the provider.
	Auth   string
	Active bool
	// Type is the SPA's userType(); empty when the record has no scope.
	TypeLabel string
	TypeBadge kit.BadgeVariant
	TypeStar  bool
	// ClientName is shown for a CLIENT-scope user.
	ShowClient bool
	ClientName string
	Created    string
	// Internal is whether the user signs in with a platform password: the
	// password actions are offered only then (an OIDC user manages
	// credentials at the identity provider).
	Internal, HasEmail bool
	TwoFactor          string
	HasTwoFactor       bool
	AnchorUser         bool
	Home               *usrClientRef
	Granted            []usrGrantRow
	Roles              []usrRoleRow
	AllApps            bool
	Apps               []usrAppRow
	// LastAdminRisk is whether the user is an active all-applications
	// administrator: deleting or restricting the last one is refused.
	LastAdminRisk bool
	Self          bool
	Perms         usrPerms
	Base          string
}

func (d usrDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: usrDrawerID, Signal: usrDrawerSig, Label: "User", Size: kit.DrawerWide, Title: cmp.Or(d.Name, "User"), Subtitle: d.Email, Extras: usrTags(d)}
}

// usrScopeType is the SPA's userType() of the detail body: the explicit scope,
// else the derived one.
func usrScopeType(u principalapi.PrincipalResponse, granted int) (label string, badge kit.BadgeVariant, star bool) {
	switch u.Scope {
	case "ANCHOR":
		return "Anchor", kit.BadgeWarn, true
	case "PARTNER":
		return "Partner", kit.BadgeInfo, false
	case "CLIENT":
		return "Client", kit.BadgeSecondary, false
	}
	if u.IsAnchorUser {
		return "Anchor", kit.BadgeWarn, true
	}
	if granted > 0 || u.ClientID == nil {
		return "Partner", kit.BadgeInfo, false
	}
	return "Client", kit.BadgeSecondary, false
}

// usrFactorLabel is the SPA's twoFactorSummary names.
func usrFactorLabel(m string) string {
	switch m {
	case "TOTP":
		return "Authenticator app"
	case "EMAIL_PIN":
		return "Email code"
	}
	return m
}

// buildDetail reads the user with the API's by-id reads (the record, its
// roles, its application access and, for a platform administrator, its client
// access: each checked by the API) and shapes the drawer.
func (h *Users) buildDetail(ctx context.Context, ac *auth.AuthContext, id string) (usrDetail, error) {
	u, err := h.Users.GetPrincipal(ctx, id)
	if err != nil {
		return usrDetail{}, err
	}
	roles, err := h.Users.ListPrincipalRoles(ctx, id)
	if err != nil {
		return usrDetail{}, err
	}
	apps, err := h.Users.ListPrincipalApplicationAccess(ctx, id)
	if err != nil {
		return usrDetail{}, err
	}
	var grants []principalapi.ClientAccessGrantResponse
	if ac.IsAnchor() {
		if grants, err = h.Users.ListPrincipalClientAccess(ctx, id); err != nil {
			return usrDetail{}, err
		}
	}
	clients, err := h.clientIndex(ctx)
	if err != nil {
		return usrDetail{}, err
	}
	roleIndex, err := h.roleIndex(ctx, ac)
	if err != nil {
		return usrDetail{}, err
	}
	d := usrDetail{
		ID: u.ID, Name: u.Name, Email: deref(u.Email), Scope: u.Scope, ClientID: deref(u.ClientID), Active: u.Active, AnchorUser: u.IsAnchorUser,
		Internal: deref(u.IdpType) == "INTERNAL", HasEmail: deref(u.Email) != "",
		Created: u.CreatedAt.Underlying().UTC().Format("2006-01-02"), AllApps: apps.AllApplications,
		Self: ac != nil && ac.PrincipalID == u.ID, Perms: usrPermsFor(ac), Base: usrURL(u.ID),
		HasTwoFactor: len(u.TwoFactorMethods) > 0, TwoFactor: "None",
	}
	d.Auth = cmp.Or(deref(u.IdpType), "—")
	if d.Auth == "INTERNAL" {
		d.Auth = "Internal"
	}
	d.TypeLabel, d.TypeBadge, d.TypeStar = usrScopeType(u, len(grants))
	if u.Scope == "CLIENT" && u.ClientID != nil {
		d.ShowClient, d.ClientName = true, clients.label(*u.ClientID)
	}
	if d.HasTwoFactor {
		names := make([]string, 0, len(u.TwoFactorMethods))
		for _, m := range u.TwoFactorMethods {
			names = append(names, usrFactorLabel(m))
		}
		d.TwoFactor = strings.Join(names, ", ")
	}
	d.LastAdminRisk = u.Scope == "ANCHOR" && u.Active && apps.AllApplications
	if u.ClientID != nil {
		if c, ok := clients[*u.ClientID]; ok {
			d.Home = &usrClientRef{Name: c.Name, Identifier: c.Identifier}
		}
	}
	for _, g := range grants {
		row := usrGrantRow{ClientID: g.ClientID, Name: clients.label(g.ClientID), Granted: g.GrantedAt.Underlying().UTC().Format("2006-01-02")}
		row.Identifier = clients[g.ClientID].Identifier
		d.Granted = append(d.Granted, row)
	}
	for _, r := range roles {
		row := usrRoleRow{Name: r.RoleName, Short: roleShort(r.RoleName), Display: roleShort(r.RoleName), Source: r.AssignmentSource, Assigned: r.AssignedAt.Underlying().UTC().Format("2006-01-02")}
		if rl, ok := roleIndex[r.RoleName]; ok && rl.DisplayName != "" {
			row.Display = rl.DisplayName
		}
		d.Roles = append(d.Roles, row)
	}
	for _, a := range apps.Applications {
		d.Apps = append(d.Apps, usrAppRow{ID: a.ApplicationID, Name: cmp.Or(a.ApplicationName, a.ApplicationID), Code: a.ApplicationCode})
	}
	return d, nil
}

// roleIndex is the roles by name, for display names and the role picker; empty
// when the caller may not list roles (GET /bff/roles) or the page has none.
func (h *Users) roleIndex(ctx context.Context, ac *auth.AuthContext) (map[string]role.Role, error) {
	out := map[string]role.Role{}
	if h.Roles == nil || auth.CanReadRoles(ac) != nil {
		return out, nil
	}
	rs, err := h.Roles.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rs {
		out[rs[i].Name] = rs[i]
	}
	return out, nil
}

// ── the drawer's request handlers ──────────────────────────────────────

// detail opens the drawer: patched into the page for Datastar (a row's name or
// View details), or the list page with the drawer open for a navigation to
// /ui/users/{id}.
func (h *Users) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := usrID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// GET /api/principals/{id}: the read permission and client scope are the
	// API's (GetPrincipal); the page asks first so a caller without them is
	// refused whatever the service does.
	if err := auth.CanReadPrincipals(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d, err := h.buildDetail(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		if err := openUsrDrawer(ds.NewSSE(w, r), d); err != nil {
			slog.DebugContext(ctx, "ui: user drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(nil, usrList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

// openUsrDrawer patches the drawer's content for d, selects it and moves
// focus to its heading (drawer.md, server-loaded record).
func openUsrDrawer(sse *ds.SSE, d usrDetail) error {
	return errors.Join(
		sse.Patch(usrDrawerContent(d), ds.Inner("#"+usrDrawerID)),
		sse.PatchSignals(map[string]any{usrDrawerSig: d.ID}),
		sse.Focus(usrDrawerID+"-title"),
	)
}

// usrHidden are the hidden fields every action inside the drawer carries: the
// record the drawer shows (the drawer's signal), so the answer can refresh the
// drawer.
func usrHidden() []kit.HiddenField { return []kit.HiddenField{drawerField(usrDrawerSig)} }

// afterChange re-renders what a change to user id affects: the table of the
// list the user is on and the drawer when it shows this user. A deleted
// user's drawer is closed. A caller who may write but not read gets neither:
// the reads are the API's.
func (h *Users) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string, deleted bool) {
	ctx := r.Context()
	if auth.CanReadPrincipals(ac) != nil {
		return
	}
	if view, err := h.listView(ctx, ac, refererList(r, usrList)); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := view.patch(sse); err != nil {
		slog.DebugContext(ctx, "ui: users table patch not delivered", "err", err)
	}
	if id == "" || r.PostFormValue("drawer") != id {
		return
	}
	if deleted {
		if err := sse.PatchSignals(map[string]any{usrDrawerSig: ""}); err != nil {
			slog.DebugContext(ctx, "ui: user drawer close not delivered", "err", err)
		}
		return
	}
	d, err := h.buildDetail(ctx, ac, id)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := sse.Patch(usrDrawerContent(d), ds.Inner("#"+usrDrawerID)); err != nil {
		slog.DebugContext(ctx, "ui: user drawer patch not delivered", "err", err)
	}
}

// done answers a successful action: for Datastar the dialog closed, the table
// of the list the user is on, the drawer when it shows this user (closed after
// a delete) and a flash, whose detail is the key's unless detail is given
// (text the server built); for a native post a redirect with the flash key.
func (h *Users) done(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id, dialog, flash, detail string, deleted bool) {
	if !ds.IsDatastar(r) {
		h.nativeDone(w, r, id, flash, deleted)
		return
	}
	sse := ds.NewSSE(w, r)
	h.finish(sse, r, ac, id, dialog, flash, detail, deleted)
}

// finish is done on a stream that is already open.
func (h *Users) finish(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id, dialog, flash, detail string, deleted bool) {
	ctx := r.Context()
	if dialog != "" {
		if err := sse.CloseDialog(dialog); err != nil {
			slog.DebugContext(ctx, "ui: user dialog close not delivered", "dialog", dialog, "err", err)
		}
	}
	h.afterChange(sse, r, ac, id, deleted)
	if err := usrFlash(sse, flash, detail); err != nil {
		slog.DebugContext(ctx, "ui: user flash not delivered", "err", err)
	}
}

// nativeDone answers a successful native post: a redirect to the user's
// drawer (the list after a delete) with the flash key.
func (h *Users) nativeDone(w http.ResponseWriter, r *http.Request, id, flash string, deleted bool) {
	h.Shell.SetFlash(w, flash)
	target := usersPath
	if !deleted {
		target = usrURL(id)
	}
	// The path is under /ui and the id passed kit.ValidID.
	http.Redirect(w, r, target, http.StatusSeeOther) //nolint:gosec // G710: a /ui path built from a validated id
}

// requireID reads the user id of a drawer action from the path.
func (h *Users) requireID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, err := usrID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return "", false
	}
	return id, true
}

// ── assignable roles ───────────────────────────────────────────────────

// usrRoleChoices are the roles the role picker offers a user, as the SPA's
// assignableRoles: every role for an anchor-scope user or one with access to
// all applications, else the roles of the applications the user can access,
// plus the roles already assigned (so they can be revoked). Roles holding
// platform permissions the caller lacks are above the caller and locked (the
// API refuses adding or removing them, ROLE_ABOVE_CALLER).
type usrRoleChoices struct {
	// Options are the roles the caller may add or remove.
	Options []kit.Option
	// Selected are the assigned roles among Options.
	Selected []string
	// Locked are the assigned roles above the caller: kept as they are.
	Locked []usrRoleRow
	// Items are the same roles as the picker's entries, grouped by
	// application; the assigned roles above the caller are locked in them.
	Items []kit.PickItem
	// Hidden counts roles left out because their application is not enabled
	// for the user.
	Hidden int
}

func (h *Users) roleChoices(ctx context.Context, ac *auth.AuthContext, d usrDetail) (usrRoleChoices, error) {
	all, err := h.roleIndex(ctx, ac)
	if err != nil {
		return usrRoleChoices{}, err
	}
	assigned := map[string]bool{}
	for _, r := range d.Roles {
		assigned[r.Name] = true
	}
	accessible := map[string]bool{}
	for _, a := range d.Apps {
		accessible[a.Code] = true
	}
	var c usrRoleChoices
	names := make([]string, 0, len(all))
	for n := range all {
		names = append(names, n)
	}
	slices.SortFunc(names, func(a, b string) int {
		if c := strings.Compare(strings.ToLower(all[a].ApplicationCode), strings.ToLower(all[b].ApplicationCode)); c != 0 {
			return c
		}
		return strings.Compare(strings.ToLower(cmp.Or(all[a].DisplayName, a)), strings.ToLower(cmp.Or(all[b].DisplayName, b)))
	})
	for _, n := range names {
		rl := all[n]
		if d.Scope != "ANCHOR" && !d.AllApps && !accessible[rl.ApplicationCode] && !assigned[n] {
			c.Hidden++
			continue
		}
		item := kit.PickItem{
			Value: n, Key: "r" + strconv.Itoa(len(c.Items)), Name: cmp.Or(rl.DisplayName, roleShort(n)), Code: n,
			Group: cmp.Or(rl.ApplicationCode, "other"), Checked: assigned[n],
		}
		if rl.Description != nil {
			item.Description = *rl.Description
		}
		if len(role.PermissionsAboveCaller(ac, rl.Permissions)) > 0 {
			if assigned[n] {
				c.Locked = append(c.Locked, usrRoleRow{Name: n, Short: roleShort(n), Display: cmp.Or(rl.DisplayName, roleShort(n))})
				item.Locked, item.Note = true, "Above your access"
				c.Items = append(c.Items, item)
			}
			continue
		}
		c.Items = append(c.Items, item)
		label := cmp.Or(rl.DisplayName, n)
		if label != n {
			label += " (" + n + ")"
		}
		c.Options = append(c.Options, kit.Option{Value: n, Label: label})
		if assigned[n] {
			c.Selected = append(c.Selected, n)
		}
	}
	return c, nil
}

// picker is the role picker: the roles grouped by application with their
// descriptions, the ones above the caller locked (kept and posted back).
func (c usrRoleChoices) picker() kit.PickListProps {
	return kit.PickListProps{
		ID: "usr-roles-pick", Signal: "usrRolesPicked", FilterSignal: "usrRoleQ", Name: "roles", ItemIDPrefix: "usr-role-",
		Items: c.Items, AvailableTitle: "Available roles (" + strconv.Itoa(len(c.Items)) + ")", SelectedTitle: "Selected",
		FilterPlaceholder: "Filter roles", EmptyText: "No roles selected", Bulk: true,
	}
}
