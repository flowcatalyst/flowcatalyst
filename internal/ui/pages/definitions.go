package pages

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

// Definition export and import (docs/ui/definition-export-import.md): the
// drawer shows a record's safe values as JSON, and the create form fills
// itself in from such a file. Nothing here writes: export reads what the
// drawer already shows, import only fills the form. The format is shared with
// the SPA; internal/ui/pages/testdata/definitions/*.json are its examples.

const (
	defKindApplication    = "application"
	defKindServiceAccount = "service-account"
	defKindConnection     = "connection"
	defKindDispatchPool   = "dispatch-pool"
	defKindSubscription   = "subscription"

	defMarker  = "definition"
	defVersion = 1
)

// ── the five definitions (key order = export order) ────────────────────

type appDefinition struct {
	Code           string `json:"code"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Type           string `json:"type,omitempty"`
	DefaultBaseURL string `json:"defaultBaseUrl,omitempty"`
	IconURL        string `json:"iconUrl,omitempty"`
	Website        string `json:"website,omitempty"`
	Logo           string `json:"logo,omitempty"`
	LogoMimeType   string `json:"logoMimeType,omitempty"`
}

type saDefinition struct {
	Code              string   `json:"code"`
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	Scope             string   `json:"scope,omitempty"`
	ClientIdentifiers []string `json:"clientIdentifiers,omitempty"`
	ApplicationCode   string   `json:"applicationCode,omitempty"`
	Roles             []string `json:"roles,omitempty"`
}

type connDefinition struct {
	Code               string `json:"code"`
	Name               string `json:"name"`
	Description        string `json:"description,omitempty"`
	ExternalID         string `json:"externalId,omitempty"`
	ServiceAccountCode string `json:"serviceAccountCode,omitempty"`
	ClientIdentifier   string `json:"clientIdentifier,omitempty"`
}

type poolDefinition struct {
	Code             string `json:"code"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	RateLimit        *int32 `json:"rateLimit,omitempty"`
	Concurrency      *int32 `json:"concurrency,omitempty"`
	ClientIdentifier string `json:"clientIdentifier,omitempty"`
}

type subEventTypeRef struct {
	EventTypeCode string `json:"eventTypeCode"`
	SpecVersion   string `json:"specVersion,omitempty"`
}

type subDefinition struct {
	Code             string            `json:"code"`
	ApplicationCode  string            `json:"applicationCode,omitempty"`
	Name             string            `json:"name"`
	Description      string            `json:"description,omitempty"`
	Endpoint         string            `json:"endpoint,omitempty"`
	ClientScoped     *bool             `json:"clientScoped,omitempty"`
	ClientIdentifier string            `json:"clientIdentifier,omitempty"`
	EventTypes       []subEventTypeRef `json:"eventTypes,omitempty"`
	ConnectionCode   string            `json:"connectionCode,omitempty"`
	Queue            string            `json:"queue,omitempty"`
	DispatchPoolCode string            `json:"dispatchPoolCode,omitempty"`
	Mode             string            `json:"mode,omitempty"`
	MaxAgeSeconds    *int32            `json:"maxAgeSeconds,omitempty"`
	TimeoutSeconds   *int32            `json:"timeoutSeconds,omitempty"`
	DelaySeconds     *int32            `json:"delaySeconds,omitempty"`
	Sequence         *int32            `json:"sequence,omitempty"`
}

// ── serialisation ──────────────────────────────────────────────────────

type defEnvelope struct {
	Flowcatalyst string `json:"flowcatalyst"`
	Kind         string `json:"kind"`
	Version      int    `json:"version"`
	Definition   any    `json:"definition"`
}

// marshalDefinition writes the file: the envelope, then the definition's keys
// in declaration order with unset ones omitted (zero numbers and false are
// values, held as pointers), two-space indentation and a trailing newline.
func marshalDefinition(kind string, def any) []byte {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false) // a logo's `<svg` stays readable
	enc.SetIndent("", "  ")
	if err := enc.Encode(defEnvelope{Flowcatalyst: defMarker, Kind: kind, Version: defVersion, Definition: def}); err != nil {
		// The definitions are plain strings, numbers and booleans.
		panic("pages: definition not encodable: " + err.Error())
	}
	return b.Bytes()
}

// ── parsing ────────────────────────────────────────────────────────────

// defRefusal is why an import file was not read: shown to the user, form
// untouched.
type defRefusal string

func (e defRefusal) Error() string { return string(e) }

// parseDefinition reads an import file for kind: the envelope is checked
// (marker, kind, version), the definition decoded into T, and the keys T does
// not know returned (sorted) so the notice can list them.
func parseDefinition[T any](kind, text string) (def T, unknown []string, err error) {
	text = strings.TrimPrefix(strings.TrimSpace(text), "\ufeff")
	if text == "" {
		return def, nil, defRefusal("Paste the definition JSON or choose a file.")
	}
	var env struct {
		Flowcatalyst string          `json:"flowcatalyst"`
		Kind         string          `json:"kind"`
		Version      json.RawMessage `json:"version"`
		Definition   json.RawMessage `json:"definition"`
	}
	if err := json.Unmarshal([]byte(text), &env); err != nil {
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) {
			return def, nil, defRefusal("This is not a FlowCatalyst definition file.")
		}
		return def, nil, defRefusal("This is not valid JSON.")
	}
	if env.Flowcatalyst != defMarker {
		return def, nil, defRefusal("This is not a FlowCatalyst definition file.")
	}
	if env.Kind != kind {
		if env.Kind == "" {
			return def, nil, defRefusal("The file does not say what kind of definition it is.")
		}
		return def, nil, defRefusal("This file is a " + env.Kind + " definition; this form takes a " + kind + " definition.")
	}
	if string(bytes.TrimSpace(env.Version)) != fmt.Sprint(defVersion) {
		return def, nil, defRefusal("This file's version is not supported; this form reads version " + fmt.Sprint(defVersion) + ".")
	}
	var fields map[string]json.RawMessage
	if len(env.Definition) == 0 || json.Unmarshal(env.Definition, &fields) != nil || fields == nil {
		return def, nil, defRefusal("The file has no definition object.")
	}
	if err := json.Unmarshal(env.Definition, &def); err != nil {
		var te *json.UnmarshalTypeError
		if errors.As(err, &te) {
			return def, nil, defRefusal("The definition's " + te.Field + " has the wrong type (expected " + te.Type.String() + ").")
		}
		return def, nil, defRefusal("The definition could not be read.")
	}
	known := defKeys(reflect.TypeFor[T]())
	for k := range fields {
		if !slices.Contains(known, k) {
			unknown = append(unknown, k)
		}
	}
	slices.Sort(unknown)
	return def, unknown, nil
}

// defKeys are the JSON keys of a definition struct.
func defKeys(t reflect.Type) []string {
	var keys []string
	for f := range t.Fields() {
		if name, _, _ := strings.Cut(f.Tag.Get("json"), ","); name != "" && name != "-" {
			keys = append(keys, name)
		}
	}
	return keys
}

// definitionText is the posted definition: the pasted text, or else the
// chosen file.
func definitionText(r *http.Request) string {
	if text := r.PostFormValue("definition"); strings.TrimSpace(text) != "" {
		return text
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()
	b, err := io.ReadAll(io.LimitReader(file, maxFormBytes))
	if err != nil {
		return ""
	}
	return string(b)
}

// ── the notice ─────────────────────────────────────────────────────────

// defNotes collects what an import could not apply.
type defNotes struct{ items []string }

// missing: a reference by code that this environment does not have.
func (n *defNotes) missing(what, code string) {
	n.items = append(n.items, what+" `"+code+"` was not found here. Import or create it first, then choose it.")
}

// notApplied: values the create form has no field for.
func (n *defNotes) notApplied(what string, values ...string) {
	n.items = append(n.items, what+" not applied by this form: `"+joinTicks(values)+"`.")
}

func (n *defNotes) add(s string) { n.items = append(n.items, s) }

func (n *defNotes) unknownKeys(keys []string) {
	if len(keys) > 0 {
		n.items = append(n.items, "Ignored (unknown keys): `"+strings.Join(keys, "`, `")+"`.")
	}
}

func (n defNotes) state() defImportState { return defImportState{Done: true, Notes: n.items} }

// defImportState is what a create form shows after an import: nothing before
// one, then the notice (the notes, or a brief all-clear).
type defImportState struct {
	Done  bool
	Notes []string
}

func (s defImportState) title() string {
	if len(s.Notes) == 0 {
		return "Imported. Review and create."
	}
	return "Imported with notes. Review before creating."
}

// defImportDialog is the props of a create form's "Import from JSON" dialog.
type defImportDialog struct {
	ID, Action, Kind string
	Open             bool
	Error            string
}

// refuseImport answers an import that was not read: the message goes into the
// open import dialog (Datastar) and the form behind it is left as it is; a
// native post gets the page with that dialog open (native).
func refuseImport(w http.ResponseWriter, r *http.Request, dialogID string, err error, native func(msg string)) {
	msg := "The definition could not be read."
	if ref, ok := errors.AsType[defRefusal](err); ok {
		msg = ref.Error()
	}
	if !ds.IsDatastar(r) {
		native(msg)
		return
	}
	if err := ds.NewSSE(w, r).Patch(kit.FormDialogError(dialogID, msg)); err != nil {
		slog.DebugContext(r.Context(), "ui: definition import refusal not delivered", "err", err)
	}
}

// ── the export ─────────────────────────────────────────────────────────

// defExport is one record's export: the file's bytes, and what the viewer
// could not be given.
type defExport struct {
	Kind, Code string
	// Href is the file's address (the Download link).
	Href    string
	Body    []byte
	Omitted []string
}

var unsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

func (e defExport) fileName() string {
	return unsafeFileChars.ReplaceAllString(e.Kind+"-"+e.Code, "_") + ".json"
}

func (e defExport) text() string { return string(e.Body) }

// omittedReference says why a reference is missing from an export: the
// viewer may not read that kind of record, or the record is gone.
func omittedReference(what string, canRead bool) string {
	if !canRead {
		return what + " not included: you can't read " + strings.ToLower(what) + "s."
	}
	return what + " not included: it no longer exists or can't be read."
}

// serveFile answers export.json: the definition as a download.
func (e defExport) serveFile(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+e.fileName()+`"`)
	_, _ = w.Write(e.Body)
}

// serve answers the drawer's Export JSON action: the read-only dialog for
// Datastar, the file for a plain navigation.
func (e defExport) serve(w http.ResponseWriter, r *http.Request, slot string) {
	if !ds.IsDatastar(r) {
		e.serveFile(w)
		return
	}
	if err := patchDialogSlot(ds.NewSSE(w, r), slot, defExportDialog(e)); err != nil {
		slog.DebugContext(r.Context(), "ui: definition export patch not delivered", "err", err)
	}
}

// downloadAttrs is the Download link's attributes.
func (e defExport) downloadAttrs() templ.Attributes {
	return templ.Attributes{"download": e.fileName()}
}

const defExportDialogID = "definition-export"

// defCopyExpr copies the JSON and says so in the dialog.
const defCopyExpr = "navigator.clipboard.writeText(document.getElementById('definition-export-json').textContent).then(() => $defCopied = 'Copied to the clipboard')"

// defReadFileExpr puts the chosen file's text into the dialog's textarea (the
// server reads a posted file as well; this lets the user see what will be
// read).
const defReadFileExpr = "el.files[0] && el.files[0].text().then(t => el.closest('form').elements.definition.value = t)"

func defImportAttrs(p defImportDialog) templ.Attributes {
	if !p.Open {
		return nil
	}
	return templ.Attributes{"open": true, "data-init": "el.removeAttribute('open'); el.showModal()"}
}

// importClient checks an imported client identifier against this
// environment's clients (the form's own resolution): the identifier to fill
// into the form, or "" with a note.
func importClient(ctx context.Context, dir ClientDirectory, ac *auth.AuthContext, identifier string, notes *defNotes) string {
	if !canPickClient(dir, ac) {
		notes.add("Client `" + identifier + "` could not be checked: choosing a client needs permission to view clients.")
		return ""
	}
	_, msg, err := resolveClient(ctx, dir, ac, identifier)
	switch {
	case err != nil:
		slog.WarnContext(ctx, "ui: imported client not resolved", "err", err)
		notes.add("Client `" + identifier + "` could not be checked. Choose it again.")
	case msg != "":
		notes.missing("Client", identifier)
	default:
		return identifier
	}
	return ""
}

// joinTicks lists values between backticks: a, b -> a`, `b (the caller adds
// the outer pair).
func joinTicks(values []string) string { return strings.Join(values, "`, `") }
