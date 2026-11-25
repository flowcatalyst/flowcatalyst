// Package api holds the wire shapes and the status read that the service
// account and OAuth client endpoints share for credential publishing
// (docs/credential-publishing.md §7). The routes themselves live with their
// owners, whose permissions and reach gates they follow.
package api

import (
	"context"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// EnableRequest is the body of PUT .../credential-publication.
type EnableRequest struct {
	BasePath *string `json:"basePath,omitempty" doc:"Replaces the template's path before the credential name; must start and end with '/' and lie under the deployment's prefix. Omitted takes the template."`
}

// ItemResponse is one credential's publishing state.
type ItemResponse struct {
	Credential    string           `json:"credential" doc:"client-id, client-secret, auth-token or signing-secret"`
	Path          string           `json:"path" doc:"The full parameter name"`
	Reference     string           `json:"reference" doc:"The secret reference a function or ingestion uses to read it back"`
	State         string           `json:"state" enum:"PENDING_PUT,PUBLISHED,PENDING_DELETE,NOT_PUBLISHABLE"`
	Attempts      int              `json:"attempts"`
	LastError     *string          `json:"lastError,omitempty"`
	PublishedAt   *httpcompat.Time `json:"publishedAt,omitempty"`
	NextAttemptAt *httpcompat.Time `json:"nextAttemptAt,omitempty"`
}

// PublicationResponse is GET .../credential-publication.
type PublicationResponse struct {
	Available              bool             `json:"available" doc:"Whether this platform is configured to publish credentials at all"`
	Enabled                bool             `json:"enabled"`
	Store                  *string          `json:"store,omitempty"`
	BasePath               *string          `json:"basePath,omitempty"`
	EnabledAt              *httpcompat.Time `json:"enabledAt,omitempty"`
	EnabledBy              *string          `json:"enabledBy,omitempty"`
	LinkedServiceAccountID *string          `json:"linkedServiceAccountId,omitempty" doc:"OAuth clients only: set when the client belongs to a service account, whose publication carries its credentials"`
	Items                  []ItemResponse   `json:"items"`
}

// Status reads an owner's publishing state. linkedServiceAccountID is the
// account an OAuth client belongs to, "" otherwise; such a client has no
// publication of its own, so its items are not read.
func Status(ctx context.Context, cfg credentialpublish.Config, pubs *credentialpublish.Repository,
	owner credentialpublish.Owner, linkedServiceAccountID string) (PublicationResponse, error) {
	resp := PublicationResponse{Available: cfg.Enabled(), Items: []ItemResponse{}}
	if linkedServiceAccountID != "" {
		resp.LinkedServiceAccountID = &linkedServiceAccountID
		return resp, nil
	}
	pub, items, err := pubs.Status(ctx, owner)
	if err != nil {
		return resp, usecase.Internal("REPO", "publication status failed", err)
	}
	if pub != nil {
		resp.Enabled = true
		resp.Store, resp.BasePath, resp.EnabledBy = &pub.Store, &pub.BasePath, strPtr(pub.EnabledBy)
		t := jsontime.New(pub.EnabledAt)
		resp.EnabledAt = &t
	}
	for _, it := range items {
		out := ItemResponse{
			Credential: it.Credential, Path: it.Path, Reference: credentialpublish.Reference(it.Path),
			State: it.State, Attempts: it.Attempts, LastError: strPtr(it.LastError),
			PublishedAt: timePtr(it.PublishedAt), NextAttemptAt: timePtr(it.NextAttemptAt),
		}
		resp.Items = append(resp.Items, out)
	}
	return resp, nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func timePtr(t *time.Time) *httpcompat.Time {
	if t == nil {
		return nil
	}
	v := jsontime.New(*t)
	return &v
}
