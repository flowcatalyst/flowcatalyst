package pages

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// The grant dialog (the SPA's "Grant portal app"): lets an identity sign in
// to another of the client's portals.

const (
	portalGrantDialog = "portal-grant-form"
	portalGrantSignal = "portalGrantForm"
)

type portalGrantForm struct {
	Client, UserID, Email string
	// Apps are the active apps the identity does not hold yet.
	Apps []kit.Option
	App  string
	Errs formErrors
	Open bool
}

func (f portalGrantForm) id(key string) string { return fieldID(portalGrantDialog, key) }

func (f portalGrantForm) dialog() kit.FormDialogProps {
	return kit.FormDialogProps{
		ID: portalGrantDialog, Signal: portalGrantSignal, Action: portalUsersPath + "/" + url.PathEscape(f.UserID) + "/grant", Open: f.Open, Error: f.Errs.Form,
		Title: "Grant portal app", Description: "Let " + f.Email + " sign in to another of this client's portals. Their password and other portals are unchanged.",
		SubmitLabel: "Grant", Hidden: []kit.HiddenField{{Name: portalClientParam, Value: f.Client}},
	}
}

func portalGrantDescribedBy(f portalGrantForm) string {
	if f.Errs.Fields["app"] != "" {
		return f.id("app") + "-err"
	}
	return ""
}

func (f portalGrantForm) fields() []templ.Component {
	return []templ.Component{portalGrantField(f)}
}

func (f portalGrantForm) firstInvalid() string {
	if f.Errs.Fields["app"] != "" {
		return f.id("app")
	}
	return ""
}

func (h *PortalUsers) grantForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	v, err := h.manageView(r, ac)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.identity(r, v.Client)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f, err := h.grantFormFor(r, v.Client, i)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), portalUserDialogSlot, portalGrantDialogC(f)); err != nil {
			slog.DebugContext(ctx, "ui: portal grant form patch not delivered", "err", err)
		}
		return
	}
	v.Dialog = portalGrantDialogC(f)
	h.renderPage(w, r, http.StatusOK, v)
}

// grantFormFor is the dialog for an identity: the active apps it does not
// hold, the first chosen (the SPA preselects it).
func (h *PortalUsers) grantFormFor(r *http.Request, clientID string, i *portalidentity.Identity) (portalGrantForm, error) {
	apps, err := h.Store.Apps(r.Context(), clientID)
	if err != nil {
		return portalGrantForm{}, usecase.Internal("REPO", "list_portal_apps failed", err)
	}
	f := portalGrantForm{Client: clientID, UserID: i.ID, Email: i.Email}
	for _, a := range grantableApps(apps, i) {
		f.Apps = append(f.Apps, kit.Option{Value: a.Code, Label: a.Name + " (" + a.Code + ")"})
	}
	if len(f.Apps) == 0 {
		return portalGrantForm{}, usecase.Validation("NO_GRANTABLE_APP", "There is no other active portal app to grant.")
	}
	f.App = f.Apps[0].Value
	return f, nil
}

func (h *PortalUsers) grantFailed(w http.ResponseWriter, r *http.Request, v portalUsersView, f portalGrantForm) {
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, portalGrantDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	f.Open = true
	v.Dialog = portalGrantDialogC(f)
	h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), v)
}

// grant mirrors POST /api/portal-users/{id}/apps: the app is resolved by code
// among the client's (404 otherwise) and must be active.
func (h *PortalUsers) grant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := portalGrantForm{Client: strings.TrimSpace(r.PostFormValue(portalClientParam)), App: strings.TrimSpace(r.PostFormValue("app"))}
	v, err := h.manageView(r, ac)
	if err != nil {
		f.Errs.apply(r, err, nil)
		h.grantFailed(w, r, portalUsersView{}, f)
		return
	}
	i, err := h.identity(r, v.Client)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if af, err := h.grantFormFor(r, v.Client, i); err == nil {
		f.Apps = af.Apps
	}
	f.UserID, f.Email = i.ID, i.Email
	app, err := h.Store.AppByCode(ctx, v.Client, f.App)
	switch {
	case err != nil:
		err = usecase.Internal("REPO", "find_portal_app failed", err)
	case app == nil:
		err = httperror.NotFound("PortalApp", portalidentity.NormalizeAppCode(f.App))
	case !app.Active:
		err = usecase.Validation("PORTAL_APP_INACTIVE", "portal app '"+app.Code+"' is inactive")
	default:
		err = h.Ops.GrantApp(ctx, portalidentity.AppGrantCommand{ClientID: v.Client, IdentityID: i.ID, PortalAppID: app.ID})
	}
	if err != nil {
		f.Errs.apply(r, err, map[string]string{"PORTAL_APP_INACTIVE": "app"})
		h.grantFailed(w, r, v, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashPortalAppGranted)
		http.Redirect(w, r, v.url(), http.StatusSeeOther) //nolint:gosec // G710: the target is a page constant; the client id is query-escaped and checked by the manage check
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(portalGrantDialog); err != nil {
		slog.DebugContext(ctx, "ui: portal grant form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, v.Query)
	if err := portalFlash(sse, flashPortalAppGranted, i.Email+" can now sign in to "+app.Name); err != nil {
		slog.DebugContext(ctx, "ui: portal user flash not delivered", "err", err)
	}
}
