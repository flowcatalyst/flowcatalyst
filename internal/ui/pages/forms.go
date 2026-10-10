package pages

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// Shared plumbing for the form dialogs (kit form-dialog.md): reading a
// submitted form, mapping use-case errors to fields, and answering a failed
// submit with patches.

// formErrors holds a submitted form's errors: per field (keyed by the
// field's key, e.g. "code") and one form-level message.
type formErrors struct {
	Fields map[string]string
	Form   string
	// Status is what a native post answers with (422 for validation).
	Status int
}

func (e *formErrors) field(key, msg string) {
	if e.Fields == nil {
		e.Fields = map[string]string{}
	}
	if _, set := e.Fields[key]; !set {
		e.Fields[key] = msg
	}
	if e.Status == 0 {
		e.Status = http.StatusUnprocessableEntity
	}
}

func (e *formErrors) any() bool { return len(e.Fields) > 0 || e.Form != "" }

// apply records a use-case error: a code listed in fieldCodes goes to that
// field, anything else to the form-level region with the message the shell
// would show (a 4xx keeps the use case's message; anything else is logged
// and generic).
func (e *formErrors) apply(r *http.Request, err error, fieldCodes map[string]string) {
	if ue, ok := errors.AsType[*usecase.Error](err); ok {
		if key, mapped := fieldCodes[ue.Code]; mapped {
			e.field(key, sentence(ue.Message))
			if s := ue.HTTPStatus(); s >= 400 && s < 500 {
				e.Status = s
			}
			return
		}
	}
	status, msg := shell.Classify(r, err)
	e.Form = sentence(msg)
	e.Status = status
	if status < http.StatusInternalServerError {
		slog.InfoContext(r.Context(), "ui: form refused", "path", r.URL.Path, "status", status, "message", msg)
	}
}

// sentence capitalises a use-case message for display ("code is required"
// → "Code is required").
func sentence(s string) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[size:]
}

const notWholeNumber = "Enter a whole number."

// optionalInt32 parses a number field: blank is nil; anything that is not a
// whole number in int32 range reports ok=false.
func optionalInt32(raw string) (*int32, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return nil, false
	}
	v := int32(n)
	return &v, true
}

// optionalText is nil for a blank value (the SPA's `value || undefined`).
func optionalText(raw string) *string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return &raw
}

// patchFormFailure answers a failed form-dialog submit (form-dialog.md,
// Behaviour): every field again (values echoed, errors set), the form-level
// error region, and focus on the first invalid control.
func patchFormFailure(sse *ds.SSE, r *http.Request, dialogID string, fields []templ.Component, formError, focusID string) {
	errs := make([]error, 0, len(fields)+2)
	for _, f := range fields {
		errs = append(errs, sse.Patch(f))
	}
	errs = append(errs, sse.Patch(kit.FormDialogError(dialogID, formError)))
	if focusID != "" {
		errs = append(errs, sse.Focus(focusID))
	}
	if err := errors.Join(errs...); err != nil {
		slog.DebugContext(r.Context(), "ui: form failure patch not delivered", "err", err)
	}
}

// fieldID is the id of a form control: "{form}-{key}" ("pool-form-rate-limit").
func fieldID(form, key string) string { return form + "-" + kebab(key) }

func kebab(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('-')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// maxFormBytes caps a submitted form (the UI's forms are a few fields).
const maxFormBytes = 1 << 20

// parseForm reads a POST's form before the handler runs. Datastar posts a
// form as multipart (FormData), native forms as urlencoded; both land in
// r.PostForm. Reading it first matters: once a handler has opened its
// Datastar stream (headers flushed), net/http may refuse further reads of
// the body, so a field read afterwards would silently be empty.
func parseForm(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
		err := r.ParseMultipartForm(maxFormBytes) //nolint:gosec // G120: the body is capped by the MaxBytesReader above
		if errors.Is(err, http.ErrNotMultipart) {
			err = r.ParseForm()
		}
		if err != nil {
			// The handler sees an empty form and answers with its usual
			// validation errors; the cause is logged.
			slog.InfoContext(r.Context(), "ui: form not read", "path", r.URL.Path, "err", err)
		}
		next.ServeHTTP(w, r)
	})
}

// patchDialogSlot puts a form dialog into its slot by replacing the slot
// element (Datastar replace mode), not by morphing the slot's children. A
// morph keeps the <dialog> already there when the new one has the same id (a
// create dialog, closed, then an edit dialog): its data-init is unchanged,
// so Datastar does not run it again, the dialog is only `open` (non-modal,
// below the drawer, no focus trap). Replaced nodes are new, so data-init
// reopens the dialog as a modal every time.
func patchDialogSlot(sse *ds.SSE, slotID string, dialog templ.Component) error {
	slot := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if _, err := io.WriteString(w, `<div id="`+kit.ID(slotID)+`">`); err != nil {
			return err
		}
		if err := dialog.Render(ctx, w); err != nil {
			return err
		}
		_, err := io.WriteString(w, `</div>`)
		return err
	})
	return sse.Patch(slot, ds.WithMode(ds.ModeReplace))
}
