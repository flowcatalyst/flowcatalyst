package pages

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The OAuth client's "Parameter Store" section for a stand-alone confidential
// client (docs/credential-publishing.md section 8). A client linked to a
// service account is published with it and only says so. The routes are gated
// like the API's: anchor scope and the update permission.

func ocPublishOwner(id string) credentialpublish.Owner {
	return credentialpublish.Owner{Type: credentialpublish.OwnerOAuthClient, ID: id}
}

// publishOwner is whose items carry the client's credentials: the service
// account it is linked to, else the client itself.
func (h *OAuthClients) publishOwner(ctx context.Context, id string) credentialpublish.Owner {
	if publishOn(h.Publish) {
		if sa, err := h.Publish.LinkedServiceAccount(ctx, id); err == nil && sa != "" {
			return saPublishOwner(sa)
		}
	}
	return ocPublishOwner(id)
}

// pubSectionFor is the drawer's section for client c.
func (h *OAuthClients) pubSectionFor(ctx context.Context, ac *auth.AuthContext, c *platformauth.OAuthClient, d ocDetail) pubSection {
	if !publishOn(h.Publish) {
		return pubSection{}
	}
	linked, err := h.Publish.LinkedServiceAccount(ctx, c.ID)
	if err != nil {
		slog.WarnContext(ctx, "ui: linked service account unavailable", "client", c.ID, "err", err)
		return pubSection{Show: true, Err: "The publishing state could not be read."}
	}
	if linked != "" {
		name := linked
		if h.Accounts != nil && auth.CanReadServiceAccounts(ac) == nil {
			if sa, err := h.Accounts.FindByID(ctx, linked); err == nil && sa != nil {
				name = sa.Name
			}
		}
		return pubSection{Show: true, LinkedName: name, LinkedHref: saURL(linked)}
	}
	return readSection(ctx, h.Publish, ocPublishOwner(c.ID), pubSection{
		CanChange: d.Perms.Update, CanRotate: d.Perms.Rotate, Base: d.ActionBase, DrawerSig: ocDrawerSig,
		RotateDialog: ocRotateDialog, RotateAction: d.ActionBase + "/rotate-secret",
	})
}

// publishTarget is the gate of every publishing route.
func (h *OAuthClients) publishTarget(r *http.Request) (*auth.AuthContext, *platformauth.OAuthClient, error) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := ocID(r)
	if err != nil {
		return ac, nil, err
	}
	if err := auth.CanUpdateOAuthClients(ac); err != nil {
		return ac, nil, err
	}
	c, err := h.Clients.FindByID(ctx, id)
	if err != nil {
		return ac, nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return ac, nil, httperror.NotFound("OAuthClient", id)
	}
	if !publishOn(h.Publish) {
		return ac, nil, errPublishNotConfigured
	}
	return ac, c, nil
}

func (h *OAuthClients) enableForm(c *platformauth.OAuthClient) pubEnableForm {
	cfg := h.Publish.Config()
	f := pubEnableForm{
		Action: ocURL(c.ID) + "/publish", DrawerSig: ocDrawerSig, Subtitle: c.ClientName + " (" + c.ClientID + ")",
		Prefix: cfg.Prefix, Open: true,
	}
	if p, err := cfg.BasePath(credentialpublish.OwnerOAuthClient, credentialpublish.Vars{OAuthClient: c.ClientID}); err != nil {
		f.DefaultErr = sentence(err.Error()) + ". Enter a custom path."
	} else {
		f.Default = p
	}
	return f
}

func (h *OAuthClients) publishForm(w http.ResponseWriter, r *http.Request) {
	ac, c, err := h.publishTarget(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showDialog(w, r, ac, pubEnableDialogC(h.enableForm(c)))
}

func (h *OAuthClients) publishEnable(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac, c, err := h.publishTarget(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := h.enableForm(c)
	f.Path = strings.TrimSpace(r.PostFormValue("publishBasePath"))
	if err := h.Publish.Enable(ctx, ocPublishOwner(c.ID), optionalText(f.Path)); err != nil {
		applyPublishErr(&f.Errs, r, err, nil)
		if ds.IsDatastar(r) {
			focus := ""
			if f.Errs.Fields["publishBasePath"] != "" {
				focus = pubFormDialog + "-path"
			}
			patchFormFailure(ds.NewSSE(w, r), r, pubFormDialog, []templ.Component{pubEnableField(f)}, f.Errs.Form, focus)
			return
		}
		view, lerr := h.listView(ctx, ac, parseListQuery(nil, ocList))
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
		http.Redirect(w, r, ocPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(pubFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: publish dialog close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, c.ID, false)
	if err := flashKey(sse, flashPubEnabled); err != nil {
		slog.DebugContext(ctx, "ui: oauth client flash not delivered", "err", err)
	}
}

func (h *OAuthClients) publishStop(w http.ResponseWriter, r *http.Request) {
	ac, c, err := h.publishTarget(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := h.Publish.Disable(r.Context(), ocPublishOwner(c.ID)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, c.ID, flashPubDisabled, false)
}

func (h *OAuthClients) publishRetry(w http.ResponseWriter, r *http.Request) {
	ac, c, err := h.publishTarget(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := h.Publish.Retry(r.Context(), ocPublishOwner(c.ID)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, c.ID, flashPubRetry, false)
}
