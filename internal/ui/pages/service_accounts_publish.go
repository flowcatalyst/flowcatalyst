package pages

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The service account's "Parameter Store" section: switching publishing on and
// off, retrying, and "Rotate and publish" (docs/credential-publishing.md
// sections 3 and 8). Each is gated like the API's: the account's update
// permission with anchor reach and application confinement.

func saPublishOwner(id string) credentialpublish.Owner {
	return credentialpublish.Owner{Type: credentialpublish.OwnerServiceAccount, ID: id}
}

// publishedPaths is where the account's credentials were queued to.
func (h *ServiceAccounts) publishedPaths(ctx context.Context, id string) map[string]string {
	return pubPaths(ctx, h.Publish, saPublishOwner(id))
}

var errPublishNotConfigured = usecase.Validation(credentialpublish.CodeUnavailable, "credential publishing is not configured on this platform")

// publishTarget is the gate of every publishing route: publishing configured,
// then the account for a write.
func (h *ServiceAccounts) publishTarget(r *http.Request) (*auth.AuthContext, *serviceaccount.ServiceAccount, error) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := saID(r)
	if err != nil {
		return ac, nil, err
	}
	sa, err := h.writable(ctx, ac, id, auth.CanUpdateServiceAccounts)
	if err != nil {
		return ac, nil, err
	}
	if !publishOn(h.Publish) {
		return ac, nil, errPublishNotConfigured
	}
	return ac, sa, nil
}

// defaultPath is the base path the template gives the account.
func (h *ServiceAccounts) defaultPath(ctx context.Context, sa *serviceaccount.ServiceAccount) (path, why string) {
	v := credentialpublish.Vars{Account: credentialpublish.AccountSegment(sa.Code)}
	if sa.ApplicationID != nil && h.Apps != nil {
		if app, err := h.Apps.FindByID(ctx, string(*sa.ApplicationID)); err == nil && app != nil {
			v.App = app.Code
		}
	}
	p, err := h.Publish.Config().BasePath(credentialpublish.OwnerServiceAccount, v)
	if err != nil {
		return "", sentence(err.Error()) + ". Enter a custom path."
	}
	return p, ""
}

func (h *ServiceAccounts) enableForm(ctx context.Context, sa *serviceaccount.ServiceAccount) pubEnableForm {
	f := pubEnableForm{
		Action: saURL(sa.ID) + "/publish", DrawerSig: saDrawerSig, Subtitle: sa.Name + " (" + sa.Code + ")",
		Prefix: h.Publish.Config().Prefix, Open: true,
	}
	f.Default, f.DefaultErr = h.defaultPath(ctx, sa)
	return f
}

// publishForm is the "Publish credentials" dialog.
func (h *ServiceAccounts) publishForm(w http.ResponseWriter, r *http.Request) {
	ac, sa, err := h.publishTarget(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showDialog(w, r, ac, pubEnableDialogC(h.enableForm(r.Context(), sa)))
}

// publishEnable switches publishing on (PUT .../credential-publication).
func (h *ServiceAccounts) publishEnable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac, sa, err := h.publishTarget(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := h.enableForm(ctx, sa)
	f.Path = strings.TrimSpace(r.PostFormValue("publishBasePath"))
	if err := h.Publish.Enable(ctx, saPublishOwner(sa.ID), optionalText(f.Path)); err != nil {
		applyPublishErr(&f.Errs, r, err, nil)
		if ds.IsDatastar(r) {
			focus := ""
			if f.Errs.Fields["publishBasePath"] != "" {
				focus = pubFormDialog + "-path"
			}
			patchFormFailure(ds.NewSSE(w, r), r, pubFormDialog, []templ.Component{pubEnableField(f)}, f.Errs.Form, focus)
			return
		}
		view, lerr := h.listView(ctx, ac, parseListQuery(nil, saList))
		if lerr != nil {
			h.Shell.Fail(w, r, lerr)
			return
		}
		view.Dialog = pubEnableDialogC(f)
		h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), view)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashPubEnabled)
		http.Redirect(w, r, saPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(pubFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: publish dialog close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, sa.ID, false, true)
	if err := flashKey(sse, flashPubEnabled); err != nil {
		slog.DebugContext(ctx, "ui: service account flash not delivered", "err", err)
	}
}

// publishStop switches publishing off (DELETE .../credential-publication).
func (h *ServiceAccounts) publishStop(w http.ResponseWriter, r *http.Request) {
	ac, sa, err := h.publishTarget(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := h.Publish.Disable(r.Context(), saPublishOwner(sa.ID)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, sa.ID, flashPubDisabled, false, true)
}

// publishRetry makes the waiting items due now.
func (h *ServiceAccounts) publishRetry(w http.ResponseWriter, r *http.Request) {
	ac, sa, err := h.publishTarget(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := h.Publish.Retry(r.Context(), saPublishOwner(sa.ID)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, sa.ID, flashPubRetry, false, true)
}

// publishRotate is "Rotate and publish": the account's OAuth client's secret
// rotation with its default overlap, which queues the new secret for
// publishing, then the one-time reveal. It needs the rotation's permission as
// well as the account's.
func (h *ServiceAccounts) publishRotate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac, sa, err := h.publishTarget(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := auth.CanRotateOAuthClientSecrets(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	p, err := h.Principals.FindByServiceAccount(ctx, sa.ID)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "find_by_service_account failed", err))
		return
	}
	var clientRow string
	if p != nil {
		ocs, err := h.OAuthClients.FindByPrincipalID(ctx, string(p.ID))
		if err != nil {
			h.Shell.Fail(w, r, usecase.Internal("REPO", "find_by_principal failed", err))
			return
		}
		if len(ocs) > 0 {
			clientRow = ocs[0].ID
		}
	}
	if clientRow == "" {
		h.Shell.Fail(w, r, httperror.NotFound("OAuthClient", sa.ID))
		return
	}
	res, err := h.Publish.RotateClientSecret(ctx, clientRow)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	note := "The previous secret stopped working immediately."
	if res.PreviousExpiresAt != nil {
		note = "The previous secret keeps working until " + res.PreviousExpiresAt.UTC().Format(time.DateTime) + " UTC, then stops."
	}
	c := saCredentials{
		Title: "New Client Secret", Warning: "Copy this secret now. It will not be shown again.",
		Groups: []saCredGroup{{Items: []saCredItem{{Label: "Client Secret", Value: res.Secret, Help: note, Cred: credentialpublish.CredClientSecret}}}},
	}
	c.markPublished(h.publishedPaths(ctx, sa.ID))
	h.showCredentials(w, r, ac, sa.ID, c, flashOCRotated)
}
