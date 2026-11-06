package pages

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The "Linked platform user" part of the field workers page
// (docs/field-native-session-spec.md section 5): link a worker to the platform
// user they are the same person as, or remove the link. Audit correlation
// only; it grants nothing. Each action makes the admin API's check (manage, as
// PUT and DELETE /api/field-workers/{id}/link) and runs the same operations,
// which decide whether the user can be linked.

const (
	fwLinkForm = "fw-link-form"
	fwLinkSig  = "fwLinkForm"
)

var fieldLinkCodes = map[string]string{
	"EMAIL_REQUIRED": "email", fieldidentity.CodeInvalidLink: "email", "LINK_EXISTS": "email", "PRINCIPAL_ID_REQUIRED": "email",
}

type fieldLinkFormData struct {
	WorkerID, Code, Email string
	Errs                  formErrors
	Open                  bool
}

func (f fieldLinkFormData) id(key string) string { return fieldID(fwLinkForm, key) }

func (f fieldLinkFormData) dialog() kit.FormDialogProps {
	return kit.FormDialogProps{
		ID: fwLinkForm, Signal: fwLinkSig, Action: fieldWorkersPath + "/" + url.PathEscape(f.WorkerID) + "/link", Open: f.Open, Error: f.Errs.Form,
		Title:       "Link platform user",
		Description: "Link " + f.Code + " to the platform user they are the same person as. It only correlates the two in audit trails; it grants nothing on either side.",
		SubmitLabel: "Link", Hidden: []kit.HiddenField{drawerField(fwDrawerSig)},
	}
}

func (f fieldLinkFormData) describedBy() string {
	if f.Errs.Fields["email"] != "" {
		return f.id("email") + "-err"
	}
	return ""
}

func (f fieldLinkFormData) fields() []templ.Component { return []templ.Component{fieldLinkField(f)} }

func (f fieldLinkFormData) firstInvalid() string {
	if f.Errs.Fields["email"] != "" {
		return f.id("email")
	}
	return ""
}

func (h *FieldWorkers) linkForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := fieldWorkerID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.loadWorker(ctx, ac, id, true)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := fieldLinkFormData{WorkerID: i.ID, Code: i.WorkerCode, Open: true}
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), fwDialogSlot, fieldLinkDialog(f)); err != nil {
			slog.DebugContext(ctx, "ui: field link form patch not delivered", "err", err)
		}
		return
	}
	v, err := h.view(ctx, ac, parseListQuery(nil, fieldWorkerList).withFacet(fieldClientParam, []string{i.ClientID}))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v.Dialog = fieldLinkDialog(f)
	h.renderPage(w, r, http.StatusOK, v)
}

// link mirrors PUT /api/field-workers/{id}/link. The platform user is found by
// email address; the operation decides whether they can be linked (a user,
// active, able to access the worker's client) and whether another worker of
// the client already holds the link.
func (h *FieldWorkers) link(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := fieldWorkerID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.loadWorker(ctx, ac, id, true)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := fieldLinkFormData{WorkerID: i.ID, Code: i.WorkerCode, Email: strings.TrimSpace(r.PostFormValue("email"))}
	var cmdErr error
	var p *fieldidentity.PrincipalRef
	if f.Email == "" {
		cmdErr = usecase.Validation("EMAIL_REQUIRED", "enter the platform user's email address")
	} else if p, err = h.Store.PrincipalByEmail(ctx, f.Email); err != nil {
		cmdErr = usecase.Internal("REPO", "find_principal failed", err)
	} else if p == nil {
		cmdErr = usecase.BusinessRule(fieldidentity.CodeInvalidLink, "no platform user has that email address")
	} else {
		cmdErr = h.Ops.LinkWorker(ctx, fieldidentity.LinkWorkerCommand{WorkerRef: refOfWorker(i), PrincipalID: p.ID})
	}
	if cmdErr != nil {
		f.Errs.apply(r, cmdErr, fieldLinkCodes)
		if ue := usecase.AsError(cmdErr); ue != nil && ue.Code == fieldidentity.CodeInvalidLink && f.Errs.Fields["email"] != "" {
			f.Errs.Status = http.StatusUnprocessableEntity // as the API
		}
		if ds.IsDatastar(r) {
			patchFormFailure(ds.NewSSE(w, r), r, fwLinkForm, f.fields(), f.Errs.Form, f.firstInvalid())
			return
		}
		f.Open = true
		v, verr := h.view(ctx, ac, parseListQuery(nil, fieldWorkerList).withFacet(fieldClientParam, []string{i.ClientID}))
		if verr != nil {
			v = fieldWorkersView{}
		}
		v.Dialog = fieldLinkDialog(f)
		h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), v)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashFieldWorkerLinked)
		http.Redirect(w, r, fieldWorkersPath+"?"+fieldClientParam+"="+url.QueryEscape(i.ClientID), http.StatusSeeOther) //nolint:gosec // G710: the target is a page constant; the client id is query-escaped and was checked by the manage check
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(fwLinkForm); err != nil {
		slog.DebugContext(ctx, "ui: field link form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, i.ClientID, id, false)
	if err := portalFlash(sse, flashFieldWorkerLinked, i.WorkerCode+" is linked to "+p.Email); err != nil {
		slog.DebugContext(ctx, "ui: field worker flash not delivered", "err", err)
	}
}

// unlink mirrors DELETE /api/field-workers/{id}/link.
func (h *FieldWorkers) unlink(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashFieldWorkerUnlinked, func(ctx context.Context, i *fieldidentity.Identity) (string, error) {
		err := h.Ops.UnlinkWorker(ctx, refOfWorker(i))
		return i.WorkerCode + " is no longer linked to a platform user", err
	})
}
