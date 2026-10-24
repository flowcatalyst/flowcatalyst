package pages

import (
	"context"
	"slices"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// What the two portal pages (portal apps and portal users) share: the
// client the page works on. Both mirror the SPA's pages, where a client
// select above the table scopes everything: an anchor picks from the clients
// it can list, a client-scoped caller from its own.

// portalClientParam is the URL parameter holding the chosen client's id (the
// API's clientId). Changing the client is a navigation, so every dialog and
// confirmation on the page carries the one client the page was rendered for.
const portalClientParam = "client"

// portalClientOptions are the clients the select offers: for an anchor with
// the view-clients permission every client (the SPA's clientsApi.list), for
// a client-scoped caller the clients it can access, named when they can be
// read and otherwise by id (the SPA falls back to the id as well).
func portalClientOptions(ctx context.Context, clients ClientStore, ac *auth.AuthContext) ([]kit.Option, error) {
	if clients == nil {
		return nil, nil
	}
	if ac.IsAnchor() {
		if auth.CanReadClients(ac) != nil {
			return nil, nil
		}
		all, err := clients.FindAll(ctx)
		if err != nil {
			return nil, usecase.Internal("REPO", "find_all failed", err)
		}
		opts := make([]kit.Option, 0, len(all))
		for _, c := range all {
			opts = append(opts, kit.Option{Value: string(c.ID), Label: c.Name})
		}
		return opts, nil
	}
	opts := make([]kit.Option, 0, len(ac.Clients))
	for _, id := range ac.Clients {
		label := id
		if c, err := clients.FindByID(ctx, id); err == nil && c != nil && c.Name != "" {
			label = c.Name
		}
		opts = append(opts, kit.Option{Value: id, Label: label})
	}
	return opts, nil
}

// portalSelectedClient is the client the page works on: the requested one
// (kept in the options even when the caller cannot list clients, so the
// select can show it), else the caller's first client, else the only option
// an anchor has. Empty when the caller has to choose.
func portalSelectedClient(ac *auth.AuthContext, opts []kit.Option, requested string) (string, []kit.Option) {
	requested = strings.TrimSpace(requested)
	switch {
	case requested != "" && kit.ValidID(requested):
		if !slices.ContainsFunc(opts, func(o kit.Option) bool { return o.Value == requested }) {
			opts = append(slices.Clone(opts), kit.Option{Value: requested, Label: requested})
		}
		return requested, opts
	case !ac.IsAnchor() && len(opts) > 0, len(opts) == 1:
		return opts[0].Value, opts
	}
	return "", opts
}

// portalClientName is the select's label for the chosen client.
func portalClientName(opts []kit.Option, id string) string {
	for _, o := range opts {
		if o.Value == id {
			return o.Label
		}
	}
	return id
}
