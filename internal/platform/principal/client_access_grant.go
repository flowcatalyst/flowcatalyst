package principal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ClientAccessGrant is the aggregate root for the iam_client_access_grants
// row that records a PARTNER user's access to a specific client. Each grant
// is its own aggregate (UoW-managed) so the grant/revoke ops emit the
// matching iam:user:client-access-* events.
type ClientAccessGrant struct {
	ID          string    `json:"id"`
	PrincipalID string    `json:"principalId"`
	ClientID    string    `json:"clientId"`
	GrantedBy   string    `json:"grantedBy"`
	GrantedAt   time.Time `json:"grantedAt"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// IDStr satisfies usecase.HasID.
func (g ClientAccessGrant) IDStr() string { return g.ID }

// NewClientAccessGrant constructs a new grant row.
func NewClientAccessGrant(principalID, clientID, grantedBy string) *ClientAccessGrant {
	now := time.Now().UTC()
	return &ClientAccessGrant{
		ID:          tsid.Generate(tsid.ClientAccessGrant),
		PrincipalID: principalID,
		ClientID:    clientID,
		GrantedBy:   grantedBy,
		GrantedAt:   now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// ClientAccessGrantRepo is the pgx-backed repo. Implements
// usecasepgx.Persist[ClientAccessGrant] so the grant flows through the
// standard UoW.
type ClientAccessGrantRepo struct{ pool *pgxpool.Pool }

// NewClientAccessGrantRepo wires the repo.
func NewClientAccessGrantRepo(pool *pgxpool.Pool) *ClientAccessGrantRepo {
	return &ClientAccessGrantRepo{pool: pool}
}

const grantSelect = `SELECT id, principal_id, client_id, granted_by, granted_at,
	created_at, updated_at FROM iam_client_access_grants`

// FindByPrincipalAndClient returns the existing grant (nil if none).
func (r *ClientAccessGrantRepo) FindByPrincipalAndClient(ctx context.Context, principalID, clientID string) (*ClientAccessGrant, error) {
	rows, err := r.pool.Query(ctx,
		grantSelect+` WHERE principal_id = $1 AND client_id = $2`, principalID, clientID)
	if err != nil {
		return nil, fmt.Errorf("client_access_grant repo: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	return scanGrant(rows)
}

// FindByPrincipal lists all grants for a principal.
func (r *ClientAccessGrantRepo) FindByPrincipal(ctx context.Context, principalID string) ([]ClientAccessGrant, error) {
	rows, err := r.pool.Query(ctx,
		grantSelect+` WHERE principal_id = $1 ORDER BY granted_at`, principalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClientAccessGrant
	for rows.Next() {
		g, err := scanGrant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

// Persist implements usecasepgx.Persist[ClientAccessGrant].
func (r *ClientAccessGrantRepo) Persist(ctx context.Context, g *ClientAccessGrant, tx *usecasepgx.DbTx) error {
	now := time.Now().UTC()
	_, err := tx.Inner().Exec(ctx,
		`INSERT INTO iam_client_access_grants
		     (id, principal_id, client_id, granted_by, granted_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (id) DO UPDATE SET
		     granted_by = EXCLUDED.granted_by,
		     updated_at = EXCLUDED.updated_at`,
		g.ID, g.PrincipalID, g.ClientID, g.GrantedBy, g.GrantedAt, g.CreatedAt, now)
	return err
}

// Delete removes a grant.
func (r *ClientAccessGrantRepo) Delete(ctx context.Context, g *ClientAccessGrant, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx, `DELETE FROM iam_client_access_grants WHERE id = $1`, g.ID)
	return err
}

func scanGrant(rows pgx.Rows) (*ClientAccessGrant, error) {
	var g ClientAccessGrant
	if err := rows.Scan(&g.ID, &g.PrincipalID, &g.ClientID, &g.GrantedBy,
		&g.GrantedAt, &g.CreatedAt, &g.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &g, nil
}
