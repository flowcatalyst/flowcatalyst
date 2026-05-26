package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ClientConfig is the per-(application, client) configuration row in
// app_client_configs. It's a separate aggregate from Application — the
// enable/disable ops mutate this row, not the Application aggregate.
type ClientConfig struct {
	ID              string          `json:"id"`
	ApplicationID   string          `json:"applicationId"`
	ClientID        string          `json:"clientId"`
	Enabled         bool            `json:"enabled"`
	BaseURLOverride *string         `json:"baseUrlOverride,omitempty"` // transient (API-only)
	ConfigJSON      json.RawMessage `json:"configJson,omitempty"`      // transient (API-only)
	CreatedAt       time.Time       `json:"createdAt"`
	UpdatedAt       time.Time       `json:"updatedAt"`
}

// IDStr satisfies usecase.HasID.
func (c ClientConfig) IDStr() string { return c.ID }

// NewClientConfig constructs an enabled config row.
func NewClientConfig(applicationID, clientID string) *ClientConfig {
	now := time.Now().UTC()
	return &ClientConfig{
		ID:            tsid.Generate(tsid.AppClientConfig),
		ApplicationID: applicationID,
		ClientID:      clientID,
		Enabled:       true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// Enable / Disable are idempotent state mutators.
func (c *ClientConfig) Enable()  { c.Enabled = true; c.UpdatedAt = time.Now().UTC() }
func (c *ClientConfig) Disable() { c.Enabled = false; c.UpdatedAt = time.Now().UTC() }

// ClientConfigRepo is the pgx-backed repo. Backed by app_client_configs.
type ClientConfigRepo struct{ q *dbq.Queries }

// NewClientConfigRepo wires the repo.
func NewClientConfigRepo(pool *pgxpool.Pool) *ClientConfigRepo {
	return &ClientConfigRepo{q: dbq.New(pool)}
}

// FindByApplicationAndClient returns the per-(app, client) config or nil.
func (r *ClientConfigRepo) FindByApplicationAndClient(ctx context.Context, applicationID, clientID string) (*ClientConfig, error) {
	row, err := r.q.ClientConfigFindByAppAndClient(ctx, dbq.ClientConfigFindByAppAndClientParams{
		ApplicationID: applicationID,
		ClientID:      clientID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("app_client_configs repo: %w", err)
	}
	return rowToClientConfig(row), nil
}

// FindByApplication lists every config for the application.
func (r *ClientConfigRepo) FindByApplication(ctx context.Context, applicationID string) ([]ClientConfig, error) {
	rows, err := r.q.ClientConfigFindByApp(ctx, applicationID)
	if err != nil {
		return nil, err
	}
	out := make([]ClientConfig, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToClientConfig(row))
	}
	return out, nil
}

// Persist implements usecasepgx.Persist[ClientConfig].
func (r *ClientConfigRepo) Persist(ctx context.Context, c *ClientConfig, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).ClientConfigUpsert(ctx, dbq.ClientConfigUpsertParams{
		ID:            c.ID,
		ApplicationID: c.ApplicationID,
		ClientID:      c.ClientID,
		Enabled:       c.Enabled,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     time.Now().UTC(),
	})
}

// Delete removes a config row.
func (r *ClientConfigRepo) Delete(ctx context.Context, c *ClientConfig, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).ClientConfigDelete(ctx, c.ID)
}

func rowToClientConfig(row dbq.AppClientConfig) *ClientConfig {
	return &ClientConfig{
		ID:            row.ID,
		ApplicationID: row.ApplicationID,
		ClientID:      row.ClientID,
		Enabled:       row.Enabled,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}
