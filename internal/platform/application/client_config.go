package application

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// ClientConfig is the per-(application, client) configuration row in
// app_client_configs. It's a separate aggregate from Application — the
// enable/disable ops mutate this row, not the Application aggregate.
type ClientConfig struct {
	ID              string            `json:"id"`
	ApplicationID   ids.ApplicationID `json:"applicationId"`
	ClientID        ids.ClientID      `json:"clientId"`
	Enabled         bool              `json:"enabled"`
	BaseURLOverride *string           `json:"baseUrlOverride,omitempty"` // transient (API-only)
	ConfigJSON      json.RawMessage   `json:"configJson,omitempty"`      // transient (API-only)
	CreatedAt       time.Time         `json:"createdAt"`
	UpdatedAt       time.Time         `json:"updatedAt"`
}

// IDStr satisfies usecase.HasID.
func (c ClientConfig) IDStr() string { return c.ID }

// NewClientConfig constructs an enabled config row.
func NewClientConfig(applicationID, clientID string) *ClientConfig {
	now := time.Now().UTC()
	return &ClientConfig{
		ID:            tsid.Generate(tsid.AppClientConfig),
		ApplicationID: ids.ApplicationID(applicationID),
		ClientID:      ids.ClientID(clientID),
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
	res, err := r.q.ClientConfigFindByAppAndClient(ctx, dbq.ClientConfigFindByAppAndClientParams{
		ApplicationID: ids.ApplicationID(applicationID),
		ClientID:      ids.ClientID(clientID),
	})
	row, err := repocommon.One(res, err, "app_client_configs repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToClientConfig(*row), nil
}

// FindByApplication lists every config for the application.
func (r *ClientConfigRepo) FindByApplication(ctx context.Context, applicationID string) ([]ClientConfig, error) {
	rows, err := r.q.ClientConfigFindByApp(ctx, ids.ApplicationID(applicationID))
	if err != nil {
		return nil, err
	}
	out := make([]ClientConfig, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToClientConfig(row))
	}
	return out, nil
}

// FindByClient lists every (application) config row for a client.
func (r *ClientConfigRepo) FindByClient(ctx context.Context, clientID string) ([]ClientConfig, error) {
	rows, err := r.q.ClientConfigFindByClient(ctx, ids.ClientID(clientID))
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
	q := r.q.WithTx(tx.Inner())
	// The client's field-policy lock first (fieldidentity lock_order.go rule
	// 1): an override change of the client runs wholly before or after this,
	// so it never saves an override for an application that has just stopped
	// serving the client.
	if err := q.FieldPolicyLock(ctx, string(c.ClientID)); err != nil {
		return err
	}
	if err := q.ClientConfigUpsert(ctx, dbq.ClientConfigUpsertParams{
		ID:            c.ID,
		ApplicationID: c.ApplicationID,
		ClientID:      c.ClientID,
		Enabled:       c.Enabled,
		CreatedAt:     c.CreatedAt,
		UpdatedAt:     time.Now().UTC(),
	}); err != nil {
		return err
	}
	if c.Enabled {
		return nil
	}
	// Disabled for the client: the application's override of the client's
	// field policy goes with it (docs/field-policy-application-overrides.md
	// section 6), in the same transaction.
	return q.FieldPolicyOverrideDeleteForApplicationAndClient(ctx, dbq.FieldPolicyOverrideDeleteForApplicationAndClientParams{
		ApplicationID: string(c.ApplicationID), ClientID: string(c.ClientID),
	})
}

// Delete removes a config row, and the application's override of the
// client's field policy with it (a row created again later starts with none),
// under the client's field-policy lock.
func (r *ClientConfigRepo) Delete(ctx context.Context, c *ClientConfig, tx *usecasepgx.DbTx) error {
	q := r.q.WithTx(tx.Inner())
	if err := q.FieldPolicyLock(ctx, string(c.ClientID)); err != nil {
		return err
	}
	if err := q.FieldPolicyOverrideDeleteForApplicationAndClient(ctx, dbq.FieldPolicyOverrideDeleteForApplicationAndClientParams{
		ApplicationID: string(c.ApplicationID), ClientID: string(c.ClientID),
	}); err != nil {
		return err
	}
	return q.ClientConfigDelete(ctx, c.ID)
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
