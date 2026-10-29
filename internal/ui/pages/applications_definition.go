package pages

import (
	"context"
	"net/http"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// Definition export and import for applications (definitions.go).

const appImportDialog = "app-import"

func (f appForm) importDialog() defImportDialog {
	return defImportDialog{ID: appImportDialog, Action: appsPath + "/new/import", Kind: defKindApplication, Open: f.ImportOpen, Error: f.ImportError}
}

// exportOf reads an application with the drawer's checks and writes its
// definition. The provisioned service account and login client are not part
// of it (they are credentials-bearing records of their own).
func (h *Applications) exportOf(ctx context.Context, ac *auth.AuthContext, id string) (defExport, error) {
	a, err := h.loadApplication(ctx, ac, id)
	if err != nil {
		return defExport{}, err
	}
	def := appDefinition{
		Code: a.Code, Name: a.Name, Description: deref(a.Description), Type: string(a.Type),
		DefaultBaseURL: deref(a.DefaultBaseURL), IconURL: deref(a.IconURL), Website: deref(a.Website),
		Logo: deref(a.Logo), LogoMimeType: deref(a.LogoMimeType),
	}
	return defExport{Kind: defKindApplication, Code: a.Code, Href: appURL(string(a.ID)) + "/export.json", Body: marshalDefinition(defKindApplication, def)}, nil
}

// export answers the drawer's Export JSON action.
func (h *Applications) export(w http.ResponseWriter, r *http.Request) {
	if e, ok := h.export1(w, r); ok {
		e.serve(w, r, appDialogSlot)
	}
}

// exportFile answers the download.
func (h *Applications) exportFile(w http.ResponseWriter, r *http.Request) {
	if e, ok := h.export1(w, r); ok {
		e.serveFile(w)
	}
}

func (h *Applications) export1(w http.ResponseWriter, r *http.Request) (defExport, bool) {
	id, err := appID(r)
	if err == nil {
		var e defExport
		if e, err = h.exportOf(r.Context(), auth.FromContext(r.Context()), id); err == nil {
			return e, true
		}
	}
	h.Shell.Fail(w, r, err)
	return defExport{}, false
}

// importDefinition fills the create dialog from a pasted or uploaded
// definition; it writes nothing.
func (h *Applications) importDefinition(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanCreateApplications(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	def, unknown, err := parseDefinition[appDefinition](defKindApplication, definitionText(r))
	if err != nil {
		refuseImport(w, r, appImportDialog, err, func(msg string) {
			h.showForm(w, r, ac, appForm{Open: true, Type: string(application.TypeApplication), ImportOpen: true, ImportError: msg})
		})
		return
	}
	f := appForm{
		Open: true, Type: string(application.TypeApplication), Code: def.Code, Name: def.Name, Description: def.Description,
		BaseURL: def.DefaultBaseURL, IconURL: def.IconURL, Website: def.Website, Logo: def.Logo, LogoMimeType: def.LogoMimeType,
	}
	var notes defNotes
	if def.Type != "" {
		if t, ok := application.ParseType(def.Type); ok {
			f.Type = string(t)
		} else {
			notes.add("Type `" + def.Type + "` is not Application or Integration; left as Application.")
		}
	}
	notes.unknownKeys(unknown)
	f.Import = notes.state()
	h.showForm(w, r, ac, f)
}
