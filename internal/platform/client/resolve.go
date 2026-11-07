package client

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// ResolveRef canonicalises a request's client reference, either the clt_ id or
// the client's identifier slug, to the client's id. Unknown references fail
// closed with a 404 (a typo'd identifier must not silently mis-scope work).
func (r *Repository) ResolveRef(ctx context.Context, ref string) (string, error) {
	c, err := r.FindByID(ctx, ref)
	if err != nil {
		return "", usecase.Internal("REPO", "client lookup failed", err)
	}
	if c == nil {
		// Identifiers are lowercase-normalised at create time, so match
		// case-insensitively here.
		c, err = r.FindByIdentifier(ctx, strings.ToLower(ref))
		if err != nil {
			return "", usecase.Internal("REPO", "client lookup failed", err)
		}
	}
	if c == nil {
		return "", httperror.NotFound("Client", ref)
	}
	return string(c.ID), nil
}
