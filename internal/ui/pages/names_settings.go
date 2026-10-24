package pages

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/branding"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// NamesSettings is the Names settings page (/ui/platform/settings/names),
// mirroring the SPA's PlatformNamesSettingsPage: the platform name shown in
// security emails, the authenticator-app label, passkey prompts and the
// browser tab. It is the platform config row (platform, branding,
// platform-name), read and written through the platform-config service, so the
// rules are those of PUT /api/config/platform/branding/platform-name.
type NamesSettings struct {
	Config ConfigService
	Ops    ConfigCommands
	Shell  *shell.Shell
}

const (
	namesPath = "/ui/platform/settings/names"
	// namesMax keeps the name a name: it is printed in emails and prompts.
	namesMax = 200
)

// Mount registers the page's routes (the POST behind parseForm).
func (h *NamesSettings) Mount(r chi.Router) {
	r.Get("/platform/settings/names", h.page)
	r.With(parseForm).Post("/platform/settings/names", h.save)
}

// namesView is the form: the name as typed and its error.
type namesView struct {
	Name     string
	Error    string
	ReadOnly bool
	// Status is what a native post that failed answers with.
	Status int
}

// storedName is the configured platform name, "" when there is none (the
// SPA shows the default then). List applies the API's read rule.
func (h *NamesSettings) storedName(ctx context.Context, ac *auth.AuthContext) (string, error) {
	rows, err := h.Config.List(ctx, ac, branding.App)
	if err != nil {
		return "", err
	}
	for i := range rows {
		if rows[i].Section == branding.Section && rows[i].Property == branding.Property && ids.StringPtr(rows[i].ClientID) == nil {
			return strings.TrimSpace(rows[i].Value), nil
		}
	}
	return "", nil
}

func (h *NamesSettings) page(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	name, err := h.storedName(ctx, ac)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// "Reset to default" fills the field; it is saved with Save.
	if name == "" || r.URL.Query().Get("default") != "" {
		name = branding.DefaultPlatformName
	}
	h.renderPage(w, r, http.StatusOK, namesView{Name: name, ReadOnly: h.Config.AuthorizeWrite(ctx, ac, branding.App) != nil})
}

func (h *NamesSettings) renderPage(w http.ResponseWriter, r *http.Request, status int, v namesView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Names", Path: namesPath}, namesPage(v))
}

func (h *NamesSettings) save(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// PUT /api/config/...: the confinement and update gate first; the use case
	// decides for callers below anchor tier by their access grants.
	if err := h.Config.AuthorizeUpdate(ctx, ac, branding.App); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v := namesView{Name: strings.TrimSpace(r.PostFormValue("platformName"))}
	switch {
	case v.Name == "":
		v.Error = "Platform name cannot be empty."
	case len([]rune(v.Name)) > namesMax:
		v.Error = "Use at most 200 characters."
	default:
		plain, desc := "PLAIN", "Platform display name"
		err := h.Ops.Set(ctx, operations.SetPropertyCommand{
			ApplicationCode: branding.App, Section: branding.Section, Property: branding.Property,
			Value: v.Name, ValueType: &plain, Description: &desc,
		})
		if err != nil {
			var form formErrors
			form.apply(r, err, nil)
			v.Error, v.Status = form.Form, form.Status
		}
	}
	if v.Error != "" {
		if ds.IsDatastar(r) {
			sse := ds.NewSSE(w, r)
			_ = sse.Patch(namesForm(v))
			_ = sse.Focus("names-platform-name")
			return
		}
		status := v.Status
		if status == 0 {
			status = http.StatusUnprocessableEntity
		}
		h.renderPage(w, r, status, v)
		return
	}
	themeSaved(w, r, h.Shell, namesPath, flashNameSaved)
}
