// Package kit is the templ implementation of the FlowCatalyst UI kit
// (flowcatalyst-framework/ui-kit/components/*.md). One templ function per
// spec; its Props struct's fields mirror the spec's Props table (Go field =
// the camelCase prop name with a capital first letter; the json tag is the
// prop name, so the language-neutral fixtures decode straight into it).
//
// Conventions (see docs/ui/README.md):
//
//   - Markup and class strings are the spec's, unchanged. A component never
//     merges classes; Class is appended after the spec's classes, so callers
//     override with `[&]:` variants as the spec says.
//   - Attrs (templ.Attributes) is passed through to the component's root (or
//     the part the spec names, e.g. the <input> inside a checkbox).
//   - Slots are templ children ({ children... }) for the default slot and
//     templ.Component fields for named slots.
//   - Ids and signal names end up inside Datastar expressions (which are
//     JavaScript), so they are validated: an invalid one panics, because it is
//     a programming error and must never reach a page with user data in it.
//     User data never goes into a data-* expression; it goes into plain
//     attributes or text, which templ escapes.
package kit

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/a-h/templ"
)

// DefaultSpriteURL is the icon sprite path the specs and fixtures use. Pages
// put the served (hashed) URL in the context with WithSpriteURL.
const DefaultSpriteURL = "/icons/lucide-sprite.svg"

type spriteKey struct{}

// WithSpriteURL sets the URL icons reference (<use href="URL#i-name">).
func WithSpriteURL(ctx context.Context, url string) context.Context {
	return context.WithValue(ctx, spriteKey{}, url)
}

func spriteURL(ctx context.Context) string {
	if u, ok := ctx.Value(spriteKey{}).(string); ok && u != "" {
		return u
	}
	return DefaultSpriteURL
}

// IconHref is the sprite reference for a Lucide icon name ("x", "search").
func IconHref(ctx context.Context, name string) string {
	return spriteURL(ctx) + "#i-" + name
}

// cx joins class lists, skipping empty parts.
func cx(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}

var (
	idPattern     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
	signalPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)
	kebabBoundary = regexp.MustCompile(`([a-z0-9])([A-Z])`)
)

// ID validates an element id that a Datastar expression or an aria reference
// will contain. It panics on anything else (programming error).
func ID(id string) string {
	if !idPattern.MatchString(id) {
		panic(fmt.Sprintf("kit: invalid element id %q (want [A-Za-z][A-Za-z0-9_-]*)", id))
	}
	return id
}

// Signal validates a camelCase Datastar signal name.
func Signal(name string) string {
	if !signalPattern.MatchString(name) {
		panic(fmt.Sprintf("kit: invalid signal name %q (want camelCase [A-Za-z][A-Za-z0-9]*)", name))
	}
	return name
}

// SignalAttr is the kebab-case form of a signal name for attribute keys
// (`data-bind:` + SignalAttr("qPools") = "data-bind:q-pools"): HTML lower-cases
// attribute names, Datastar maps kebab-case back to camelCase.
func SignalAttr(name string) string {
	return strings.ToLower(kebabBoundary.ReplaceAllString(Signal(name), "$1-$2"))
}

// ShowModal is the Datastar expression that opens the <dialog> with this id
// (dialog.md: Datastar handlers, not commandfor, for browser support).
func ShowModal(id string) string {
	return "document.getElementById('" + ID(id) + "').showModal()"
}

// CloseDialog closes the dialog containing the element (close/cancel buttons).
const CloseDialog = "el.closest('dialog').close()"

// LightDismiss closes a full-viewport dialog when its mask is clicked.
const LightDismiss = "evt.target === el && el.close()"

// OpenModal returns the attributes a trigger needs to open dialog id:
// the click handler and aria-haspopup.
func OpenModal(id string) templ.Attributes {
	return templ.Attributes{"data-on:click": ShowModal(id), "aria-haspopup": "dialog"}
}

// SubmitForm is the submit handler of a form that posts through Datastar
// (form-dialog.md, confirm-dialog.md): the form's own fields go to its own
// action, network retries are off (a POST that reached the server must not be
// sent twice). Reading el.action keeps the URL out of the expression.
const SubmitForm = "@post(el.action, {contentType: 'form', retryMaxCount: 0})"

// SubmitConfirm submits a confirm dialog's form and closes the dialog; the
// outcome is reported in the page's flash stack (confirm-dialog.md).
const SubmitConfirm = SubmitForm + "; el.closest('dialog').close()"

// OpenConfirm is the trigger expression for a confirm dialog shared by many
// records (confirm-dialog.md, "One dialog for many records"): the trigger's
// data-action attribute (a plain attribute, so a URL never enters the
// expression) becomes the form's action, then the dialog opens.
func OpenConfirm(id string) string {
	return "const d = document.getElementById('" + ID(id) + "'); d.querySelector('form').action = el.dataset.action; d.showModal()"
}

var pathPattern = regexp.MustCompile(`^/[A-Za-z0-9/_-]*$`)

// Path validates a local path that a Datastar expression will contain (an
// autocomplete's source). It panics on anything else (programming error).
func Path(p string) string {
	if !pathPattern.MatchString(p) {
		panic(fmt.Sprintf("kit: invalid path %q (want /[A-Za-z0-9/_-]*)", p))
	}
	return p
}

// ValidID reports whether id may be used as an element id or inside a
// Datastar expression (the rule ID enforces). Handlers check ids that come
// from a request with it before they reach ID.
func ValidID(id string) bool { return idPattern.MatchString(id) }

var recordIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ValidRecordID reports whether id may be a record's id in a URL path: letters,
// digits, '_' and '-'. Unlike ValidID it allows a leading digit, because an
// untyped TSID (an event's, a dispatch job's) can start with one, and the
// ones made now do. Such an id is never an element id or part of a Datastar
// expression: build those from a prefix ("dj-" + id).
func ValidRecordID(id string) bool { return recordIDPattern.MatchString(id) }

// orDefault returns v, or def when v is empty.
func orDefault[T ~string](v, def T) T {
	if v == "" {
		return def
	}
	return v
}
