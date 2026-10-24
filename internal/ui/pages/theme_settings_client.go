package pages

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// ClientTheme is a client's Login Branding page (/ui/clients/{id}/theme),
// mirroring the SPA's ClientLoginThemePage: the override of the platform-wide
// login theme for one client, with the same editor and preview as the Theme
// settings. Switched off, the client's users see the platform theme and saving
// deletes the override instead of storing a copy.
type ClientTheme struct {
	Clients ClientStore
	Config  ConfigService
	Ops     ConfigCommands
	Shell   *shell.Shell
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *ClientTheme) Mount(r chi.Router) {
	r.Get("/clients/{id}/theme", h.page)
	post := r.With(parseForm)
	post.Post("/clients/{id}/theme", h.save)
	post.Post("/clients/{id}/theme/preview", h.preview)
}

func themeClientPath(id string) string { return cliURL(id) + "/theme" }

// load reads the client and its theme: the platform theme is the starting point
// for a client with no override, so the admin changes the real look rather
// than the built-in one.
func (h *ClientTheme) load(r *http.Request) (thmView, error) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// The SPA opens the client first (GET /api/clients/{id}).
	if err := auth.CanReadClients(ac); err != nil {
		return thmView{}, err
	}
	id, err := cliID(r)
	if err != nil {
		return thmView{}, err
	}
	c, err := (&ClientsPage{Clients: h.Clients}).loadClient(ctx, id)
	if err != nil {
		return thmView{}, err
	}
	global, err := storedTheme(ctx, h.Config, ac, "")
	if err != nil {
		return thmView{}, err
	}
	override, err := storedTheme(ctx, h.Config, ac, id)
	if err != nil {
		return thmView{}, err
	}
	values := defaultTheme()
	if global != "" {
		values = values.overlay(global)
	}
	if override != "" {
		values = values.overlay(override)
	}
	return thmView{
		Values: values, ReadOnly: h.Config.AuthorizeWrite(ctx, ac, themeApp) != nil,
		Action: themeClientPath(id), PreviewAction: themeClientPath(id) + "/preview",
		Client: &thmClient{ID: id, Name: c.Name, Identifier: c.Identifier, Enabled: override != ""},
	}, nil
}

func (h *ClientTheme) page(w http.ResponseWriter, r *http.Request) {
	view, err := h.load(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.Shell.Render(w, r, shell.Page{Title: "Login Branding", Path: clientsPath}, thmClientPage(view))
}

func (h *ClientTheme) save(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := cliID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// PUT /api/config/...: the update gate before anything is looked up; the
	// client is then resolved (an unknown one is refused, not stored), and the
	// use case decides for callers below anchor tier.
	if err := h.Config.AuthorizeUpdate(ctx, ac, themeApp); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	clientID, err := h.Config.ResolveClientID(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.load(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Values = submittedTheme(r)
	enabled := r.PostFormValue("enabled") != ""
	view.Client.Enabled = enabled
	flash := flashThemeClientReset
	if enabled {
		flash = flashThemeClientSaved
		view.Values.validate(&view.Errs)
		if !view.Errs.any() {
			if err := h.Ops.Set(ctx, setThemeCommand(view.Values, clientID)); err != nil {
				view.Errs.apply(r, err, nil)
			}
		}
	} else if err := h.Config.Delete(ctx, ac, themeApp, themeSection, themeProperty, clientID); err != nil {
		view.Errs.apply(r, err, nil)
	}
	if view.Errs.any() {
		themeFailed(w, r, h.Shell, "Login Branding", view, thmClientPage)
		return
	}
	// The SPA reloads the theme after saving: the form again, from what was stored.
	if ds.IsDatastar(r) {
		fresh, err := h.load(r)
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		sse := ds.NewSSE(w, r)
		if err := errors.Join(sse.Patch(thmEditor(fresh)), flashKey(sse, flash)); err != nil {
			slog.DebugContext(ctx, "ui: client theme patch not delivered", "err", err)
		}
		return
	}
	h.Shell.SetFlash(w, flash)
	http.Redirect(w, r, themeClientPath(id), http.StatusSeeOther) //nolint:gosec // G710: a local path built from a validated id (kit.ValidID)
}

func (h *ClientTheme) preview(w http.ResponseWriter, r *http.Request) {
	if err := h.Config.AuthorizeUpdate(r.Context(), auth.FromContext(r.Context()), themeApp); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	patchThemePreview(w, r, submittedTheme(r))
}
