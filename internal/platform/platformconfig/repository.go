package platformconfig

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// Repository is the Postgres-backed repo for both Config and Access rows.
// Tables: app_platform_configs + app_platform_config_access.
type Repository struct {
	q *dbq.Queries
	// enc encrypts SECRET values at rest. Nil when no key is configured:
	// reads of PLAIN rows still work, but writing or revealing a SECRET value
	// fails closed (ErrNoEncryptionKey) rather than storing plaintext.
	enc *encryption.Service
}

// ErrNoEncryptionKey means a SECRET value was written or revealed while
// FLOWCATALYST_APP_KEY is not configured.
var ErrNoEncryptionKey = errors.New("platform config: no encryption key configured (FLOWCATALYST_APP_KEY)")

// NewRepository wires a repo.
//
// The encryption service is taken from the environment (FLOWCATALYST_APP_KEY);
// without a key SECRET values cannot be written or revealed (fail closed).
func NewRepository(pool *pgxpool.Pool) *Repository {
	enc, _ := encryption.FromEnv()
	return &Repository{q: dbq.New(pool), enc: enc}
}

// WithEncryption returns the repository using enc for SECRET values.
func (r *Repository) WithEncryption(enc *encryption.Service) *Repository {
	r.enc = enc
	return r
}

// storedValue is the representation written to the value column: SECRET
// values are AES-256-GCM encrypted and carry the "encrypted:" marker, so a
// legacy plaintext row (no marker) is distinguishable from ciphertext.
func (r *Repository) storedValue(c *Config) (string, error) {
	if c.ValueType != ValueSecret {
		return c.Value, nil
	}
	if r.enc == nil {
		return "", ErrNoEncryptionKey
	}
	ct, err := r.enc.Encrypt(c.Value)
	if err != nil {
		return "", err
	}
	return EncryptedPrefix + ct, nil
}

// RevealValue returns the plaintext of a SECRET config as stored (c comes from
// a Find*, so c.Value is the stored representation). A row without the
// "encrypted:" marker is a legacy plaintext secret and is returned as is
// (EncryptLegacySecrets removes them at start-up).
func (r *Repository) RevealValue(c *Config) (string, error) {
	if c.ValueType != ValueSecret {
		return "", errors.New("platform config: not a secret")
	}
	if !strings.HasPrefix(c.Value, EncryptedPrefix) {
		return c.Value, nil
	}
	if r.enc == nil {
		return "", ErrNoEncryptionKey
	}
	return r.enc.Decrypt(c.Value)
}

// EncryptLegacySecrets encrypts SECRET rows still stored as plaintext and
// returns how many it converted. Idempotent; safe to run on every start.
func (r *Repository) EncryptLegacySecrets(ctx context.Context) (int, error) {
	rows, err := r.q.PlatformConfigListPlaintextSecrets(ctx)
	if err != nil {
		return 0, err
	}
	if len(rows) > 0 && r.enc == nil {
		return 0, ErrNoEncryptionKey
	}
	for _, row := range rows {
		ct, err := r.enc.Encrypt(row.Value)
		if err != nil {
			return 0, err
		}
		if err := r.q.PlatformConfigSetStoredValue(ctx, dbq.PlatformConfigSetStoredValueParams{ID: row.ID, Value: EncryptedPrefix + ct}); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

// FindByID loads a config row by id.
func (r *Repository) FindByID(ctx context.Context, id string) (*Config, error) {
	res, err := r.q.PlatformConfigFindByID(ctx, id)
	row, err := repocommon.One(res, err, "platform_config repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToConfig(*row)
}

// FindByCoordinate loads a config row by its (app, section, property, scope, clientID) key.
func (r *Repository) FindByCoordinate(ctx context.Context, app, section, property string, scope Scope, clientID *string) (*Config, error) {
	var (
		res dbq.AppPlatformConfig
		err error
	)
	if clientID != nil {
		res, err = r.q.PlatformConfigFindByCoordinateClient(ctx, dbq.PlatformConfigFindByCoordinateClientParams{
			ApplicationCode: app,
			Section:         section,
			Property:        property,
			Scope:           string(scope),
			ClientID:        ids.PtrOf[ids.ClientID](clientID),
		})
	} else {
		res, err = r.q.PlatformConfigFindByCoordinateAnchor(ctx, dbq.PlatformConfigFindByCoordinateAnchorParams{
			ApplicationCode: app,
			Section:         section,
			Property:        property,
			Scope:           string(scope),
		})
	}
	row, err := repocommon.One(res, err, "platform_config repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToConfig(*row)
}

// FindConfigsByApplication returns all configs for an app.
func (r *Repository) FindConfigsByApplication(ctx context.Context, app string) ([]Config, error) {
	rows, err := r.q.PlatformConfigFindByApplication(ctx, app)
	if err != nil {
		return nil, err
	}
	// A corrupted scope or value type on any one row fails the WHOLE list
	// read (X-06: "a list containing the row fails too") rather than
	// silently skipping or coercing that row.
	out := make([]Config, 0, len(rows))
	for _, row := range rows {
		c, err := rowToConfig(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, nil
}

// Persist implements usecasepgx.Persist[Config].
func (r *Repository) Persist(ctx context.Context, c *Config, tx *usecasepgx.DbTx) error {
	value, err := r.storedValue(c)
	if err != nil {
		return err
	}
	return r.q.WithTx(tx.Inner()).PlatformConfigUpsert(ctx, dbq.PlatformConfigUpsertParams{
		ID:              c.ID,
		ApplicationCode: c.ApplicationCode,
		Section:         c.Section,
		Property:        c.Property,
		Scope:           string(c.Scope),
		ClientID:        ids.PtrOf[ids.ClientID](ids.StringPtr(c.ClientID)),
		ValueType:       string(c.ValueType),
		Value:           value,
		Description:     c.Description,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       time.Now().UTC(),
	})
}

// Delete implements usecasepgx.Persist[Config].Delete.
func (r *Repository) Delete(ctx context.Context, c *Config, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).PlatformConfigDelete(ctx, c.ID)
}

// ── Access repository (sibling table) ─────────────────────────────────────

// FindAccessByID loads an access row.
func (r *Repository) FindAccessByID(ctx context.Context, id string) (*Access, error) {
	res, err := r.q.PlatformConfigAccessFindByID(ctx, id)
	row, err := repocommon.One(res, err, "platform_config_access repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToAccess(*row), nil
}

// FindAccessByRole returns the access row for a (app, role) pair, if any.
func (r *Repository) FindAccessByRole(ctx context.Context, app, role string) (*Access, error) {
	res, err := r.q.PlatformConfigAccessFindByRole(ctx, dbq.PlatformConfigAccessFindByRoleParams{
		ApplicationCode: app, RoleCode: role,
	})
	row, err := repocommon.One(res, err, "platform_config_access repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToAccess(*row), nil
}

// FindAccessByApplication returns all access grants for an app.
func (r *Repository) FindAccessByApplication(ctx context.Context, app string) ([]Access, error) {
	rows, err := r.q.PlatformConfigAccessFindByApplication(ctx, app)
	if err != nil {
		return nil, err
	}
	out := make([]Access, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToAccess(row))
	}
	return out, nil
}

// PersistAccess persists an access row.
func (r *Repository) PersistAccess(ctx context.Context, a *Access, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).PlatformConfigAccessUpsert(ctx, dbq.PlatformConfigAccessUpsertParams{
		ID:              a.ID,
		ApplicationCode: a.ApplicationCode,
		RoleCode:        a.RoleCode,
		CanRead:         a.CanRead,
		CanWrite:        a.CanWrite,
		CreatedAt:       a.CreatedAt,
	})
}

// DeleteAccess removes an access row.
func (r *Repository) DeleteAccess(ctx context.Context, a *Access, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).PlatformConfigAccessDelete(ctx, a.ID)
}

// HasAccess reports whether the caller (described by app + role set) may
// perform the requested operation. wantWrite=true requires can_write.
func (r *Repository) HasAccess(ctx context.Context, app string, roleCodes []string, wantWrite bool) (bool, error) {
	if len(roleCodes) == 0 {
		return false, nil
	}
	if wantWrite {
		return r.q.PlatformConfigAccessHasWriteByRoles(ctx, dbq.PlatformConfigAccessHasWriteByRolesParams{
			ApplicationCode: app, RoleCodes: roleCodes,
		})
	}
	return r.q.PlatformConfigAccessHasReadByRoles(ctx, dbq.PlatformConfigAccessHasReadByRolesParams{
		ApplicationCode: app, RoleCodes: roleCodes,
	})
}

// rowToConfig hydrates the entity from its row. A scope or value type
// value that isn't one of the known constants (junk written before
// write-boundary validation existed, or a hand-edited row) is a loud read
// error — never round-tripped as-is and never coerced to a default, per the
// X-06 ruling. The row id is logged so the bad row can be found and fixed
// without a debugger.
func rowToConfig(row dbq.AppPlatformConfig) (*Config, error) {
	scope, ok := ParseScope(row.Scope)
	if !ok {
		slog.Error("platform config row has unrecognised scope", "id", row.ID, "scope", row.Scope)
		return nil, usecase.Internal("CORRUPT_PLATFORM_CONFIG_SCOPE",
			fmt.Sprintf("platform config %s has an unrecognised scope", row.ID), nil)
	}
	valueType, ok := ParseValueType(row.ValueType)
	if !ok {
		slog.Error("platform config row has unrecognised value type", "id", row.ID, "value_type", row.ValueType)
		return nil, usecase.Internal("CORRUPT_PLATFORM_CONFIG_VALUE_TYPE",
			fmt.Sprintf("platform config %s has an unrecognised value type", row.ID), nil)
	}
	return &Config{
		ID:              row.ID,
		ApplicationCode: row.ApplicationCode,
		Section:         row.Section,
		Property:        row.Property,
		Scope:           scope,
		ClientID:        ids.PtrOf[ids.ClientID](ids.StringPtr(row.ClientID)),
		ValueType:       valueType,
		Value:           row.Value,
		Description:     row.Description,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}, nil
}

func rowToAccess(row dbq.AppPlatformConfigAccess) *Access {
	return &Access{
		ID:              row.ID,
		ApplicationCode: row.ApplicationCode,
		RoleCode:        row.RoleCode,
		CanRead:         row.CanRead,
		CanWrite:        row.CanWrite,
		CreatedAt:       row.CreatedAt,
	}
}
