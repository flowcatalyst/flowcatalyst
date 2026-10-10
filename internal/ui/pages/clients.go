package pages

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// ClientDirectory is the slice of the client repository the client select
// (kit autocomplete.md) reads: the same search the SPA's ClientSelect calls
// (GET /api/clients/search?q=, at most 50 matches by name or identifier),
// and the lookup a submitted identifier is resolved with.
type ClientDirectory interface {
	Search(ctx context.Context, term string) ([]client.Client, error)
	FindByIdentifier(ctx context.Context, identifier string) (*client.Client, error)
}

// clientOptionsPath is the client select's search endpoint.
const clientOptionsPath = "/ui/clients/options"

// ClientOptions answers the client select's search: the datalist of
// matching clients, patched by id.
type ClientOptions struct {
	Clients ClientDirectory
	Shell   *shell.Shell
}

// Mount registers the search endpoint.
func (h *ClientOptions) Mount(r chi.Router) {
	r.Get("/clients/options", h.search)
}

func (h *ClientOptions) search(w http.ResponseWriter, r *http.Request) {
	list := r.URL.Query().Get("list")
	if !kit.ValidID(list) {
		h.Shell.Fail(w, r, usecase.Validation("INVALID_LIST", "Unknown suggestion list."))
		return
	}
	// With CanReadClients (the SPA's GET /api/clients/search) every client
	// matches; a client-scoped user's matches are the clients it can access
	// (as /bff/filter-options/clients); anyone else gets an empty list, not
	// an error flash on every keystroke (autocomplete.md, Behaviour).
	opts, err := clientSuggestions(r.Context(), h.Clients, auth.FromContext(r.Context()), r.URL.Query().Get("q"))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.Patch(kit.AutocompleteOptions(kit.AutocompleteOptionsProps{ID: list, Options: opts})); err != nil {
		slog.DebugContext(r.Context(), "ui: client options patch not delivered", "err", err)
	}
}

// clientSuggestions searches clients by name or identifier: every client
// for a caller who may read clients, the active clients it can access for a
// client-scoped caller (it may bind a resource only to those), none for
// anyone else.
func clientSuggestions(ctx context.Context, dir ClientDirectory, ac *auth.AuthContext, term string) ([]kit.Option, error) {
	if !canPickClient(dir, ac) {
		return nil, nil
	}
	all := auth.CanReadClients(ac) == nil
	term = strings.TrimSpace(term)
	if len(term) > maxQueryLen {
		term = term[:maxQueryLen]
	}
	found, err := dir.Search(ctx, term)
	if err != nil {
		return nil, usecase.Internal("REPO", "client search failed", err)
	}
	opts := make([]kit.Option, 0, len(found))
	for _, c := range found {
		if !all && !ownClient(ac, &c) {
			continue
		}
		opts = append(opts, kit.Option{Value: c.Identifier, Label: c.Name})
	}
	return opts, nil
}

// canPickClient reports whether the caller may choose a client in a form:
// with CanReadClients any client; a client-scoped caller (not an anchor)
// one of the clients it can access. Without this a client user could not
// create a pool, connection or subscription at all: the client field was
// disabled and a platform-wide resource needs anchor scope
// (docs/platform-bug-fixes.md).
func canPickClient(dir ClientDirectory, ac *auth.AuthContext) bool {
	if dir == nil {
		return false
	}
	return auth.CanReadClients(ac) == nil || (ac != nil && !ac.IsAnchor() && len(ac.Clients) > 0)
}

// ownClient reports whether a client-scoped caller may choose c: an active
// client it can access.
func ownClient(ac *auth.AuthContext, c *client.Client) bool {
	return c.Status == client.StatusActive && ac.CanAccessClient(string(c.ID))
}

const noSuchClient = "No client with this identifier."

// resolveClient turns a submitted client identifier into the client id the
// use case takes: nil for blank; ok=false with a field message when the
// identifier matches no client (or the caller may not choose one).
func resolveClient(ctx context.Context, dir ClientDirectory, ac *auth.AuthContext, identifier string) (id *string, fieldMsg string, err error) {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return nil, "", nil
	}
	if !canPickClient(dir, ac) {
		return nil, "You cannot choose a client: that needs permission to view clients.", nil
	}
	c, err := dir.FindByIdentifier(ctx, identifier)
	if err != nil {
		return nil, "", usecase.Internal("REPO", "client lookup failed", err)
	}
	// A client-scoped caller is told the same for a client it may not use as
	// for one that does not exist.
	if c == nil || (auth.CanReadClients(ac) != nil && !ownClient(ac, c)) {
		return nil, noSuchClient, nil
	}
	cid := string(c.ID)
	return &cid, "", nil
}
