package pages

import (
	"context"
	"net/http"
	"slices"
	"strconv"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

// Definition export and import for subscriptions (definitions.go).

const subImportDialog = "sub-import"

func (f subForm) importDialog() defImportDialog {
	return defImportDialog{ID: subImportDialog, Action: subsPath + "/new/import", Kind: defKindSubscription, Open: f.ImportOpen, Error: f.ImportError}
}

// exportOf reads a subscription with the drawer's checks and writes its
// definition. The connection is written as its code, read with the drawer's
// own checks; one the viewer cannot read (or that is gone) is left out and
// the viewer is told. The pool's code is already part of the record.
func (h *Subscriptions) exportOf(ctx context.Context, ac *auth.AuthContext, id string) (defExport, error) {
	if err := auth.CanReadSubscriptions(ac); err != nil {
		return defExport{}, err
	}
	s, err := h.loadSubscription(ctx, ac, id)
	if err != nil {
		return defExport{}, err
	}
	scoped, maxAge, timeout, delay, sequence := s.ClientScoped, s.MaxAgeSeconds, s.TimeoutSeconds, s.DelaySeconds, s.Sequence
	def := subDefinition{
		Code: s.Code, ApplicationCode: deref(s.ApplicationCode), Name: s.Name, Description: deref(s.Description),
		Endpoint: s.Endpoint, ClientScoped: &scoped, ClientIdentifier: deref(s.ClientIdentifier),
		Queue: deref(s.Queue), DispatchPoolCode: deref(s.DispatchPoolCode), Mode: string(s.Mode),
		MaxAgeSeconds: &maxAge, TimeoutSeconds: &timeout, DelaySeconds: &delay, Sequence: &sequence,
	}
	for _, b := range s.EventTypes {
		def.EventTypes = append(def.EventTypes, subEventTypeRef{EventTypeCode: b.EventTypeCode, SpecVersion: deref(b.SpecVersion)})
	}
	e := defExport{Kind: defKindSubscription, Code: s.Code, Href: subURL(s.ID) + "/export.json"}
	if s.ConnectionID != nil {
		if d := h.subDetail(ctx, ac, s); d.ConnectionCode != "" {
			def.ConnectionCode = d.ConnectionCode
		} else {
			e.Omitted = append(e.Omitted, omittedReference("Connection", h.Connections != nil && auth.CanReadConnections(ac) == nil))
		}
	}
	if s.DispatchPoolID != nil && def.DispatchPoolCode == "" {
		e.Omitted = append(e.Omitted, omittedReference("Dispatch pool", true))
	}
	e.Body = marshalDefinition(defKindSubscription, def)
	return e, nil
}

// export answers the drawer's Export JSON action.
func (h *Subscriptions) export(w http.ResponseWriter, r *http.Request) {
	if e, ok := h.export1(w, r); ok {
		e.serve(w, r, subDialogSlot)
	}
}

// exportFile answers the download.
func (h *Subscriptions) exportFile(w http.ResponseWriter, r *http.Request) {
	if e, ok := h.export1(w, r); ok {
		e.serveFile(w)
	}
}

func (h *Subscriptions) export1(w http.ResponseWriter, r *http.Request) (defExport, bool) {
	id, err := subID(r)
	if err == nil {
		var e defExport
		if e, err = h.exportOf(r.Context(), auth.FromContext(r.Context()), id); err == nil {
			return e, true
		}
	}
	h.Shell.Fail(w, r, err)
	return defExport{}, false
}

// subDefaults are the create dialog's starting values (the SPA's).
func subDefaults() subForm {
	return subForm{Open: true, Queue: "DEFAULT", Mode: "NEXT_ON_ERROR", MaxAge: "86400", Timeout: "30", Delay: "0", Sequence: "99"}
}

// importDefinition fills the create dialog from a pasted or uploaded
// definition; it writes nothing. References are matched by code against the
// choices the form offers.
func (h *Subscriptions) importDefinition(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanCreateSubscriptions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	def, unknown, err := parseDefinition[subDefinition](defKindSubscription, definitionText(r))
	if err != nil {
		refuseImport(w, r, subImportDialog, err, func(msg string) {
			f := subDefaults()
			f.ImportOpen, f.ImportError = true, msg
			h.showForm(w, r, ac, f)
		})
		return
	}
	f := subDefaults()
	f.Code, f.Name, f.Description, f.Endpoint = def.Code, def.Name, def.Description, def.Endpoint
	num := func(dst *string, v *int32) {
		if v != nil {
			*dst = strconv.Itoa(int(*v))
		}
	}
	num(&f.MaxAge, def.MaxAgeSeconds)
	num(&f.Timeout, def.TimeoutSeconds)
	num(&f.Delay, def.DelaySeconds)
	num(&f.Sequence, def.Sequence)
	f.ClientScoped = def.ClientScoped != nil && *def.ClientScoped
	var notes defNotes
	if def.Queue != "" {
		if slices.ContainsFunc(subQueueOptions, func(o kit.Option) bool { return o.Value == def.Queue }) {
			f.Queue = def.Queue
		} else {
			notes.add("Queue `" + def.Queue + "` is not one of the queues this form offers; left as Default.")
		}
	}
	if def.Mode != "" {
		if slices.ContainsFunc(subModeOptions, func(o kit.Option) bool { return o.Value == def.Mode }) {
			f.Mode = def.Mode
		} else {
			notes.add("Mode `" + def.Mode + "` is not one of the modes this form offers; left as Next on Error.")
		}
	}
	// The choices depend on the scope, so they are read once it is set.
	h.withChoices(r, ac, &f)
	if code := def.ConnectionCode; code != "" {
		switch id, found := f.ConnectionIDs[code]; {
		case found:
			f.ConnectionID = id
		case !f.CanPickConnection:
			notes.add("Connection `" + code + "` could not be checked: choosing a connection needs permission to view connections.")
		default:
			notes.missing("Connection", code)
		}
	}
	if code := def.DispatchPoolCode; code != "" {
		switch id, found := f.PoolIDs[code]; {
		case found:
			f.PoolID = id
		case !f.CanPickPool:
			notes.add("Dispatch pool `" + code + "` could not be checked: choosing a pool needs permission to view dispatch pools.")
		default:
			notes.missing("Dispatch pool", code)
		}
	}
	for _, ref := range def.EventTypes {
		switch id, found := f.EventTypeIDsByCode[ref.EventTypeCode]; {
		case found:
			f.EventTypeIDs = append(f.EventTypeIDs, id)
			if cur := f.EventTypeVersions[ref.EventTypeCode]; ref.SpecVersion != "" && ref.SpecVersion != cur {
				notes.add("Event type `" + ref.EventTypeCode + "` is bound at its current version here (v" + cur + "); the file has v" + ref.SpecVersion + ".")
			}
		case !f.CanPickEventType:
			notes.add("Event type `" + ref.EventTypeCode + "` could not be checked: choosing event types needs permission to view them.")
		default:
			notes.missing("Event type", ref.EventTypeCode)
		}
	}
	if def.ClientIdentifier != "" {
		f.Client = importClient(ctx, h.Clients, ac, def.ClientIdentifier, &notes)
	}
	if def.ApplicationCode != "" {
		notes.notApplied("Application", def.ApplicationCode)
	}
	notes.unknownKeys(unknown)
	f.Import = notes.state()
	h.showForm(w, r, ac, f)
}
