// Package ds is the platform's thin wrapper over the official Datastar Go SDK
// (github.com/starfederation/datastar-go, pinned in go.mod). Everything under
// internal/ui talks to Datastar through this package only, so the dependency
// stays in one place and can be replaced by a hand-written adapter later.
//
// Datastar's server contract (1.0): a backend action (`@get`, `@post`, ...)
// sends `Datastar-Request: true`; the reply is `text/event-stream` carrying
// `datastar-patch-elements` and `datastar-patch-signals` events. Signals
// arrive as the `datastar` query parameter on GET and DELETE, and as a JSON
// body on every other method (`{contentType: 'form'}` actions send the form
// instead and no signals).
//
// Typical handler:
//
//	if ds.IsDatastar(r) {
//		sse := ds.NewSSE(w, r)
//		err := errors.Join(
//			sse.Patch(PoolRows(view)),                // morphs the element with the same id
//			sse.Patch(PoolCount(view), ds.Inner("#pools-count")),
//		)
//		...
//		return
//	}
package ds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"

	"github.com/starfederation/datastar-go/datastar"
)

// RequestHeader is the header every Datastar backend action sends.
const RequestHeader = "Datastar-Request"

// MaxSignalBytes caps how much of a request body ReadSignals reads. Signals
// are small UI state; anything larger is a mistake or an attack.
const MaxSignalBytes = 1 << 20

// IsDatastar reports whether r was sent by a Datastar backend action (as
// opposed to a browser navigation or a native form post).
func IsDatastar(r *http.Request) bool {
	return r.Header.Get(RequestHeader) == "true"
}

// Component is anything that renders HTML: a templ.Component satisfies it.
// Declared here so this package does not import templ.
type Component interface {
	Render(ctx context.Context, w io.Writer) error
}

// Mode is how a patched element is applied to the page.
type Mode string

// Patch modes (Datastar 1.0). Outer is the default: the patched element
// morphs into the page element with the same id.
const (
	ModeOuter   Mode = "outer"
	ModeInner   Mode = "inner"
	ModeReplace Mode = "replace"
	ModePrepend Mode = "prepend"
	ModeAppend  Mode = "append"
	ModeBefore  Mode = "before"
	ModeAfter   Mode = "after"
)

// PatchOption adjusts one patch-elements event.
type PatchOption func(*patchOptions)

type patchOptions struct {
	selector string
	mode     Mode
}

// WithSelector targets the elements matching a CSS selector instead of the
// element with the patched element's id.
func WithSelector(selector string) PatchOption {
	return func(o *patchOptions) { o.selector = selector }
}

// WithMode sets the patch mode. Every mode but outer and replace needs a
// selector.
func WithMode(m Mode) PatchOption {
	return func(o *patchOptions) { o.mode = m }
}

// Inner replaces the children of the element matching selector.
func Inner(selector string) PatchOption {
	return func(o *patchOptions) { o.selector, o.mode = selector, ModeInner }
}

// Append appends to the element matching selector (e.g. a flash stack).
func Append(selector string) PatchOption {
	return func(o *patchOptions) { o.selector, o.mode = selector, ModeAppend }
}

func (o patchOptions) sdk() []datastar.PatchElementOption {
	var out []datastar.PatchElementOption
	if o.selector != "" {
		out = append(out, datastar.WithSelector(o.selector))
	}
	if o.mode != "" {
		out = append(out, datastar.WithMode(datastar.ElementPatchMode(o.mode)))
	}
	return out
}

// SSE is one Datastar response stream.
type SSE struct {
	gen *datastar.ServerSentEventGenerator
}

// NewSSE starts a `text/event-stream` response: it sets the headers and
// flushes them. The ResponseWriter must support flushing (net/http's does,
// and so do chi's wrappers); one that does not is a programming error and
// the SDK panics, which the router's Recoverer turns into a 500.
func NewSSE(w http.ResponseWriter, r *http.Request) *SSE {
	return &SSE{gen: datastar.NewSSE(w, r)}
}

// Context is the request context the stream is bound to.
func (s *SSE) Context() context.Context { return s.gen.Context() }

// Patch renders c and sends it as one datastar-patch-elements event.
func (s *SSE) Patch(c Component, opts ...PatchOption) error {
	var b strings.Builder
	if err := c.Render(s.gen.Context(), &b); err != nil {
		return fmt.Errorf("ds: render patch: %w", err)
	}
	return s.PatchHTML(b.String(), opts...)
}

// PatchHTML sends already-rendered (and already-escaped) HTML as one
// datastar-patch-elements event. Prefer Patch with a component.
func (s *SSE) PatchHTML(elements string, opts ...PatchOption) error {
	var o patchOptions
	for _, opt := range opts {
		opt(&o)
	}
	return s.gen.PatchElements(elements, o.sdk()...)
}

// Remove removes the elements matching selector.
func (s *SSE) Remove(selector string) error {
	return s.gen.RemoveElement(selector)
}

// PatchSignals JSON-encodes v and merges it into the page's signals.
// A nil field value in a map removes that signal (Datastar merge-patch rules).
func (s *SSE) PatchSignals(v any) error {
	return s.gen.MarshalAndPatchSignals(v)
}

// ErrUnsafeRedirect is returned by Redirect for anything but a local path.
var ErrUnsafeRedirect = errors.New("ds: redirect target must be a local path")

// Redirect navigates the browser to path, which must be a local absolute
// path ("/ui/x?y=1"); an absolute URL or a protocol-relative "//host" is
// refused so a redirect can never leave the origin.
//
// It does not use the SDK's Redirect (a patched <script> element), because
// the UI's Content-Security-Policy does not allow inline scripts. Instead it
// appends an element whose `data-init` runs the navigation: Datastar
// evaluates attribute expressions itself. The path is JSON-encoded (which
// escapes <, > and &) and then attribute-escaped.
func (s *SSE) Redirect(path string) error {
	return s.runOnce("window.location.assign(", path, ")")
}

// ReplaceURL sets the address bar to path (history.replaceState, no
// navigation and no new history entry), so a view filtered or sorted through
// Datastar keeps a URL that reloads to the same view. The appended element
// removes itself. path must be local, as for Redirect.
func (s *SSE) ReplaceURL(path string) error {
	return s.runOnce("history.replaceState(history.state, '', ", path, "); el.remove()")
}

func (s *SSE) runOnce(prefix, path, suffix string) error {
	if !IsLocalPath(path) {
		return ErrUnsafeRedirect
	}
	target, err := json.Marshal(path)
	if err != nil {
		return fmt.Errorf("ds: encode url: %w", err)
	}
	el := `<div hidden data-init="` + html.EscapeString(prefix+string(target)+suffix) + `"></div>`
	return s.gen.PatchElements(el, datastar.WithSelector("body"), datastar.WithModeAppend())
}

// IsLocalPath reports whether p is a same-origin absolute path: it starts with
// a single "/" (not "//" or "/\", which browsers treat as another host) and
// contains no control characters.
func IsLocalPath(p string) bool {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.HasPrefix(p, `/\`) {
		return false
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// ReadSignals decodes the request's Datastar signals into v: from the
// `datastar` query parameter on GET and DELETE, from the JSON body otherwise.
// A request without signals leaves v untouched and returns nil. The body is
// capped at MaxSignalBytes.
//
// Datastar 1.0.4 sends DELETE signals in the query string, like GET (the
// kit's _conventions.md listed DELETE with the body methods; that is wrong
// for 1.0.4 and recorded as a kit defect).
func ReadSignals(r *http.Request, v any) error {
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		if r.Body == nil || r.Body == http.NoBody {
			return nil
		}
		raw, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, MaxSignalBytes))
		if err != nil {
			return fmt.Errorf("ds: read signals: %w", err)
		}
		if len(strings.TrimSpace(string(raw))) == 0 {
			return nil
		}
		if err := json.Unmarshal(raw, v); err != nil {
			return fmt.Errorf("ds: decode signals: %w", err)
		}
		return nil
	}
	if err := datastar.ReadSignals(r, v); err != nil {
		return fmt.Errorf("ds: decode signals: %w", err)
	}
	return nil
}
