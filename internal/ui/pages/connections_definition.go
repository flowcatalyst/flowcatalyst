package pages

import (
	"context"
	"net/http"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// Definition export and import for connections (definitions.go): the drawer's
// Export JSON, and the create dialog's Import from JSON.

const connImportDialog = "conn-import"

func (f connForm) importDialog() defImportDialog {
	return defImportDialog{ID: connImportDialog, Action: connsPath + "/new/import", Kind: defKindConnection, Open: f.ImportOpen, Error: f.ImportError}
}

// exportOf reads a connection with the drawer's checks and writes its
// definition. The service account is written as its code; a viewer who may
// not read service accounts (or a account that is gone) gets the key omitted
// and is told.
func (h *Connections) exportOf(ctx context.Context, ac *auth.AuthContext, id string) (defExport, error) {
	if err := auth.CanReadConnections(ac); err != nil {
		return defExport{}, err
	}
	c, err := h.loadConnection(ctx, ac, id)
	if err != nil {
		return defExport{}, err
	}
	def := connDefinition{Code: c.Code, Name: c.Name, Description: deref(c.Description), ExternalID: deref(c.ExternalID), ClientIdentifier: deref(c.ClientIdentifier)}
	e := defExport{Kind: defKindConnection, Code: c.Code, Href: connURL(c.ID) + "/export.json"}
	if sa := resolveSA(ctx, h.ServiceAccounts, ac, c.ServiceAccountID); sa.Code != "" {
		def.ServiceAccountCode = sa.Code
	} else {
		e.Omitted = append(e.Omitted, omittedReference("Service account", h.ServiceAccounts != nil && auth.CanReadServiceAccounts(ac) == nil))
	}
	e.Body = marshalDefinition(defKindConnection, def)
	return e, nil
}

// export answers the drawer's Export JSON action.
func (h *Connections) export(w http.ResponseWriter, r *http.Request) {
	if e, ok := h.export1(w, r); ok {
		e.serve(w, r, connDialogSlot)
	}
}

// exportFile answers the download.
func (h *Connections) exportFile(w http.ResponseWriter, r *http.Request) {
	if e, ok := h.export1(w, r); ok {
		e.serveFile(w)
	}
}

func (h *Connections) export1(w http.ResponseWriter, r *http.Request) (defExport, bool) {
	id, err := connID(r)
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
// definition. It writes nothing: the use case runs only when the user presses
// Create, with the usual checks.
func (h *Connections) importDefinition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanCreateConnections(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	def, unknown, err := parseDefinition[connDefinition](defKindConnection, definitionText(r))
	if err != nil {
		refuseImport(w, r, connImportDialog, err, func(msg string) {
			f := connForm{Open: true, ImportOpen: true, ImportError: msg}
			h.withChoices(r, ac, &f)
			h.showForm(w, r, ac, f)
		})
		return
	}
	f := connForm{Open: true, Code: def.Code, Name: def.Name, Description: def.Description, ExternalID: def.ExternalID}
	h.withChoices(r, ac, &f)
	var notes defNotes
	if code := def.ServiceAccountCode; code != "" {
		switch id, found := f.ServiceAccountIDs[code]; {
		case found:
			f.ServiceAccountID = id
		case !f.CanPickServiceAccount:
			notes.add("Service account `" + code + "` could not be checked: choosing a service account needs permission to view service accounts.")
		default:
			notes.missing("Service account", code)
		}
	}
	if def.ClientIdentifier != "" {
		f.Client = importClient(ctx, h.Clients, ac, def.ClientIdentifier, &notes)
	}
	notes.unknownKeys(unknown)
	f.Import = notes.state()
	h.showForm(w, r, ac, f)
}
