package pages

import (
	"context"
	"net/http"
	"strconv"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// Definition export and import for dispatch pools (definitions.go).

const poolImportDialog = "pool-import"

func (f poolForm) importDialog() defImportDialog {
	return defImportDialog{ID: poolImportDialog, Action: poolsPath + "/new/import", Kind: defKindDispatchPool, Open: f.ImportOpen, Error: f.ImportError}
}

// exportOf reads a pool with the drawer's checks and writes its definition.
func (h *DispatchPools) exportOf(ctx context.Context, ac *auth.AuthContext, id string) (defExport, error) {
	if err := auth.CanReadDispatchPools(ac); err != nil {
		return defExport{}, err
	}
	p, err := h.loadPool(ctx, ac, id)
	if err != nil {
		return defExport{}, err
	}
	concurrency := p.Concurrency
	def := poolDefinition{
		Code: p.Code, Name: p.Name, Description: deref(p.Description),
		RateLimit: p.RateLimit, Concurrency: &concurrency, ClientIdentifier: deref(p.ClientIdentifier),
	}
	return defExport{Kind: defKindDispatchPool, Code: p.Code, Href: poolURL(p.ID) + "/export.json", Body: marshalDefinition(defKindDispatchPool, def)}, nil
}

// export answers the drawer's Export JSON action.
func (h *DispatchPools) export(w http.ResponseWriter, r *http.Request) {
	if e, ok := h.export1(w, r); ok {
		e.serve(w, r, poolDialogSlot)
	}
}

// exportFile answers the download.
func (h *DispatchPools) exportFile(w http.ResponseWriter, r *http.Request) {
	if e, ok := h.export1(w, r); ok {
		e.serveFile(w)
	}
}

func (h *DispatchPools) export1(w http.ResponseWriter, r *http.Request) (defExport, bool) {
	id, err := poolID(r)
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
func (h *DispatchPools) importDefinition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanCreateDispatchPools(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	def, unknown, err := parseDefinition[poolDefinition](defKindDispatchPool, definitionText(r))
	if err != nil {
		refuseImport(w, r, poolImportDialog, err, func(msg string) {
			f := poolForm{Concurrency: "10", Open: true, ImportOpen: true, ImportError: msg}
			h.withClients(r, ac, &f)
			h.showForm(w, r, ac, f)
		})
		return
	}
	f := poolForm{Concurrency: "10", Open: true, Code: def.Code, Name: def.Name, Description: def.Description}
	if def.RateLimit != nil {
		f.RateLimit = strconv.Itoa(int(*def.RateLimit))
	}
	if def.Concurrency != nil {
		f.Concurrency = strconv.Itoa(int(*def.Concurrency))
	}
	h.withClients(r, ac, &f)
	var notes defNotes
	if def.ClientIdentifier != "" {
		f.Client = importClient(ctx, h.Clients, ac, def.ClientIdentifier, &notes)
	}
	notes.unknownKeys(unknown)
	f.Import = notes.state()
	h.showForm(w, r, ac, f)
}
