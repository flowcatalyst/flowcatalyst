package portalidentity

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// App is one portal a client runs (e.g. "customer-portal",
// "supplier-portal"). The portal app identifies itself on the admin API by
// Code; OAuth clients link to it (oauth_clients.portal_app_id) and a login
// through a linked OAuth client carries the code back in the id_token.
type App struct {
	ID          string    `json:"id"`
	ClientID    string    `json:"clientId"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// IDStr satisfies usecase.HasID.
func (a App) IDStr() string { return a.ID }

// NewApp constructs an active app. The code is normalised (NormalizeAppCode).
func NewApp(clientID, code, name string) *App {
	now := time.Now().UTC()
	return &App{
		ID:        tsid.Generate(tsid.PortalApp),
		ClientID:  clientID,
		Code:      NormalizeAppCode(code),
		Name:      strings.TrimSpace(name),
		Active:    true,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

var appCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,99}$`)

// NormalizeAppCode trims and lower-cases a code so callers match it
// case-insensitively.
func NormalizeAppCode(code string) string { return strings.ToLower(strings.TrimSpace(code)) }

// ValidAppCode reports whether a (normalised) code is well-formed: lower-case
// letters, digits, '-' and '_', starting alphanumeric, at most 100 chars.
func ValidAppCode(code string) bool { return appCodePattern.MatchString(code) }

// LinkedOAuthClient is the summary of an OAuth client fronting an app.
type LinkedOAuthClient struct {
	ID         string `json:"id"`
	ClientID   string `json:"clientId"`
	ClientName string `json:"clientName"`
}

// AppRepository is the pgx-backed store for portal_apps.
type AppRepository struct{ pool *pgxpool.Pool }

// NewAppRepository wires the repo.
func NewAppRepository(pool *pgxpool.Pool) *AppRepository { return &AppRepository{pool: pool} }

const appSelect = `SELECT id, client_id, code, name, description, active, created_at, updated_at FROM portal_apps`

// FindByID returns the app, nil if absent.
func (r *AppRepository) FindByID(ctx context.Context, id string) (*App, error) {
	return r.one(ctx, appSelect+` WHERE id = $1`, id)
}

// FindByClientAndCode returns the client's app with that code, nil if absent.
func (r *AppRepository) FindByClientAndCode(ctx context.Context, clientID, code string) (*App, error) {
	return r.one(ctx, appSelect+` WHERE client_id = $1 AND code = $2`, clientID, NormalizeAppCode(code))
}

// FindByOAuthClientID returns the app the OAuth client (by its OAuth
// client_id string) is linked to, nil when it is not linked to one.
func (r *AppRepository) FindByOAuthClientID(ctx context.Context, oauthClientID string) (*App, error) {
	return r.one(ctx, `SELECT pa.id, pa.client_id, pa.code, pa.name, pa.description, pa.active,
		pa.created_at, pa.updated_at
		FROM portal_apps pa JOIN oauth_clients oc ON oc.portal_app_id = pa.id
		WHERE oc.client_id = $1`, oauthClientID)
}

// FindByClient lists a client's apps by name; an empty clientID lists every
// client's apps (anchor views).
func (r *AppRepository) FindByClient(ctx context.Context, clientID string) ([]App, error) {
	var rows pgx.Rows
	var err error
	if clientID == "" {
		rows, err = r.pool.Query(ctx, appSelect+` ORDER BY name`)
	} else {
		rows, err = r.pool.Query(ctx, appSelect+` WHERE client_id = $1 ORDER BY name`, clientID)
	}
	if err != nil {
		return nil, fmt.Errorf("portal_apps list: %w", err)
	}
	defer rows.Close()
	out := []App{}
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// GrantCounts returns the number of identities granted each app.
func (r *AppRepository) GrantCounts(ctx context.Context, appIDs []string) (map[string]int, error) {
	out := make(map[string]int, len(appIDs))
	if len(appIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT portal_app_id, COUNT(*) FROM portal_identity_apps
		 WHERE portal_app_id = ANY($1) GROUP BY portal_app_id`, appIDs)
	if err != nil {
		return nil, fmt.Errorf("portal_apps grant counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// LinkedOAuthClients returns the OAuth clients linked to each app.
func (r *AppRepository) LinkedOAuthClients(ctx context.Context, appIDs []string) (map[string][]LinkedOAuthClient, error) {
	out := make(map[string][]LinkedOAuthClient, len(appIDs))
	if len(appIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT portal_app_id, id, client_id, client_name FROM oauth_clients
		 WHERE portal_app_id = ANY($1) ORDER BY client_name`, appIDs)
	if err != nil {
		return nil, fmt.Errorf("portal_apps linked oauth clients: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var appID string
		var c LinkedOAuthClient
		if err := rows.Scan(&appID, &c.ID, &c.ClientID, &c.ClientName); err != nil {
			return nil, err
		}
		out[appID] = append(out[appID], c)
	}
	return out, rows.Err()
}

// Persist implements usecasepgx.Persist[App].
func (r *AppRepository) Persist(ctx context.Context, a *App, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx,
		`INSERT INTO portal_apps (id, client_id, code, name, description, active, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (id) DO UPDATE SET
		     name = EXCLUDED.name,
		     description = EXCLUDED.description,
		     active = EXCLUDED.active,
		     updated_at = EXCLUDED.updated_at`,
		a.ID, a.ClientID, a.Code, a.Name, a.Description, a.Active, a.CreatedAt, time.Now().UTC())
	return err
}

// Delete removes the app; its grants go with it (FK cascade).
func (r *AppRepository) Delete(ctx context.Context, a *App, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx, `DELETE FROM portal_apps WHERE id = $1`, a.ID)
	return err
}

func (r *AppRepository) one(ctx context.Context, sql string, args ...any) (*App, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("portal_apps repo: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	return scanApp(rows)
}

func scanApp(rows pgx.Rows) (*App, error) {
	var a App
	if err := rows.Scan(&a.ID, &a.ClientID, &a.Code, &a.Name, &a.Description, &a.Active,
		&a.CreatedAt, &a.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}
