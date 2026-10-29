package pages

import (
	"context"
	"net/http"
	"slices"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	saops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// Definition export and import for service accounts (definitions.go). No
// credential is exported: not the OAuth client id or secret, the webhook auth
// token or signing secret, nor the principal id.

const saImportDialog = "sa-import"

func (f saForm) importDialog() defImportDialog {
	return defImportDialog{ID: saImportDialog, Action: saPath + "/new/import", Kind: defKindServiceAccount, Open: f.ImportOpen, Error: f.ImportError}
}

// exportOf reads an account with the drawer's checks and writes its
// definition. Clients and the application are written as codes; a viewer who
// may not read them (or a record that is gone) gets the key left out and is
// told. The roles are the ones the drawer lists.
func (h *ServiceAccounts) exportOf(ctx context.Context, ac *auth.AuthContext, id string) (defExport, error) {
	sa, err := h.loadAccount(ctx, ac, id)
	if err != nil {
		return defExport{}, err
	}
	def := saDefinition{Code: sa.Code, Name: sa.Name, Description: deref(sa.Description), Scope: deref(sa.Scope)}
	e := defExport{Kind: defKindServiceAccount, Code: sa.Code, Href: saURL(sa.ID) + "/export.json"}

	if len(sa.ClientIDs) > 0 {
		clients := h.clients(ctx, ac)
		missing := false
		for _, cid := range sa.ClientIDs {
			if ident := clients.identifiers[cid]; ident != "" {
				def.ClientIdentifiers = append(def.ClientIdentifiers, ident)
			} else {
				missing = true
			}
		}
		if missing {
			e.Omitted = append(e.Omitted, omittedReference("Client", auth.CanReadClients(ac) == nil && h.Clients != nil))
		}
	}
	if appID := deref(ids.StringPtr(sa.ApplicationID)); appID != "" {
		canRead := h.Apps != nil && auth.CanReadApplication(ac, appID) == nil
		if canRead {
			if a, err := h.Apps.FindByID(ctx, appID); err == nil && a != nil {
				def.ApplicationCode = a.Code
			}
		}
		if def.ApplicationCode == "" {
			e.Omitted = append(e.Omitted, omittedReference("Application", canRead))
		}
	}
	if p, err := h.Principals.FindByServiceAccount(ctx, sa.ID); err == nil && p != nil {
		for _, ra := range p.Roles {
			if name := ra.Role.String(); !slices.Contains(def.Roles, name) {
				def.Roles = append(def.Roles, name)
			}
		}
		slices.Sort(def.Roles)
	}
	e.Body = marshalDefinition(defKindServiceAccount, def)
	return e, nil
}

// export answers the drawer's Export JSON action.
func (h *ServiceAccounts) export(w http.ResponseWriter, r *http.Request) {
	if e, ok := h.export1(w, r); ok {
		e.serve(w, r, saDialogSlot)
	}
}

// exportFile answers the download.
func (h *ServiceAccounts) exportFile(w http.ResponseWriter, r *http.Request) {
	if e, ok := h.export1(w, r); ok {
		e.serveFile(w)
	}
}

func (h *ServiceAccounts) export1(w http.ResponseWriter, r *http.Request) (defExport, bool) {
	id, err := saID(r)
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
// definition; it writes nothing. The create form takes no roles and no
// application, so those are listed for the user to do after creating.
func (h *ServiceAccounts) importDefinition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := saops.AuthorizeCreate(ac, false, nil); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	def, unknown, err := parseDefinition[saDefinition](defKindServiceAccount, definitionText(r))
	if err != nil {
		refuseImport(w, r, saImportDialog, err, func(msg string) {
			h.showForm(w, r, ac, saForm{Open: true, Scope: "ANCHOR", ImportOpen: true, ImportError: msg})
		})
		return
	}
	f := saForm{Open: true, Scope: "ANCHOR", Code: def.Code, Name: def.Name, Description: def.Description}
	h.fill(ctx, ac, &f)
	var notes defNotes
	if def.Scope != "" {
		if validSAScope(def.Scope) {
			f.Scope = def.Scope
		} else {
			notes.add("Scope `" + def.Scope + "` is not Anchor, Partner or Client; left as Anchor.")
		}
	}
	for _, ident := range def.ClientIdentifiers {
		switch id, found := f.ClientIDsByIdentifier[ident]; {
		case found:
			f.ClientIDs = append(f.ClientIDs, id)
		case !f.CanPickClients:
			notes.add("Client `" + ident + "` could not be checked: choosing a client needs permission to view clients.")
		default:
			notes.missing("Client", ident)
		}
	}
	if f.Scope == "ANCHOR" && len(f.ClientIDs) > 0 {
		notes.add("Clients `" + joinTicks(def.ClientIdentifiers) + "` are selected, but an Anchor account does not use them. Choose Partner or Client scope to apply them.")
	}
	if def.ApplicationCode != "" {
		notes.notApplied("Application", def.ApplicationCode)
	}
	if len(def.Roles) > 0 {
		notes.add("Assign roles after creating: `" + joinTicks(def.Roles) + "`.")
	}
	notes.unknownKeys(unknown)
	f.Import = notes.state()
	h.showForm(w, r, ac, f)
}
