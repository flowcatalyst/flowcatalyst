package platformconfig

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
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

// secretAAD is the additional authenticated data a SECRET's ciphertext is
// bound to: its application, section, property and client (empty for the
// application-wide value), NUL-separated after a fixed label. A ciphertext
// copied to another row (another application's, another client's) does not
// open there.
func secretAAD(app, section, property string, clientID *ids.ClientID) []byte {
	var client string
	if clientID != nil {
		client = string(*clientID)
	}
	return []byte("fc:platform-config:v1\x00" + app + "\x00" + section + "\x00" + property + "\x00" + client)
}

func configAAD(c *Config) []byte {
	return secretAAD(c.ApplicationCode, c.Section, c.Property, c.ClientID)
}

// storedValue is the representation written to the value column: SECRET
// values are AES-256-GCM encrypted, bound to their coordinate (secretAAD), and
// carry the "encrypted:" marker, so a legacy plaintext row (no marker) is
// distinguishable from ciphertext.
func (r *Repository) storedValue(c *Config) (string, error) {
	if c.ValueType != ValueSecret {
		return c.Value, nil
	}
	if r.enc == nil {
		return "", ErrNoEncryptionKey
	}
	ct, err := r.enc.EncryptBound(c.Value, configAAD(c))
	if err != nil {
		return "", err
	}
	return EncryptedPrefix + ct, nil
}

// RevealValue returns the plaintext of a SECRET config as stored (c comes from
// a Find*, so c.Value is the stored representation). Without a key it refuses
// every SECRET, legacy plaintext rows included (fail closed). A row without
// the "encrypted:" marker is a legacy plaintext secret and is returned as is
// (EncryptLegacySecrets removes them at start-up). It never writes; Reveal is
// RevealValue plus the read repair.
func (r *Repository) RevealValue(c *Config) (string, error) {
	pt, _, err := r.revealValue(c)
	return pt, err
}

// revealValue is RevealValue, also reporting whether the stored value is
// current: encrypted and bound to its row.
func (r *Repository) revealValue(c *Config) (plaintext string, current bool, err error) {
	if c.ValueType != ValueSecret {
		return "", false, errors.New("platform config: not a secret")
	}
	if r.enc == nil {
		return "", false, ErrNoEncryptionKey
	}
	if !strings.HasPrefix(c.Value, EncryptedPrefix) {
		return c.Value, false, nil
	}
	return r.enc.DecryptBound(c.Value, configAAD(c))
}

// Reveal is RevealValue with read repair: a SECRET still stored as plaintext
// or sealed without its row binding is re-encrypted bound to its row, by a
// compare-and-set on the value that was read (a value changed meanwhile is
// left alone). A failed repair is logged, never the reveal's failure.
func (r *Repository) Reveal(ctx context.Context, c *Config) (string, error) {
	pt, current, err := r.revealValue(c)
	if err != nil {
		return "", err
	}
	if !current {
		if _, err := r.rebind(ctx, c.ID, c.ApplicationCode, c.Section, c.Property, c.ClientID, c.Value, pt); err != nil {
			slog.WarnContext(ctx, "platform config: secret read repair failed", "id", c.ID, "err", err)
		}
	}
	return pt, nil
}

// rebind stores plaintext encrypted and bound to its row, replacing old only
// while it is still the stored value (compare-and-set). It reports whether the
// row was changed.
func (r *Repository) rebind(ctx context.Context, id, app, section, property string, clientID *ids.ClientID, old, plaintext string) (bool, error) {
	ct, err := r.enc.EncryptBound(plaintext, secretAAD(app, section, property, clientID))
	if err != nil {
		return false, err
	}
	n, err := r.q.PlatformConfigCompareAndSetStoredValue(ctx, dbq.PlatformConfigCompareAndSetStoredValueParams{
		ID: id, OldValue: old, NewValue: EncryptedPrefix + ct,
	})
	return n == 1, err
}

// EncryptLegacySecrets brings every SECRET row to the current storage: rows
// still stored as plaintext are encrypted, and encrypted rows sealed before
// the ciphertext was bound to its row are re-encrypted bound (secretAAD). It
// returns how many rows it changed. Each row is replaced by a compare-and-set
// on the value it read, so a conversion running at one replica's start-up
// cannot undo a value set meanwhile. Idempotent; safe to run on every start.
//
// Without a key nothing can be converted or read: the rows are counted and
// logged, SECRET reads and writes fail closed (ErrNoEncryptionKey), and the
// platform still starts. A row that no configured key opens is counted, logged
// and left as it is.
func (r *Repository) EncryptLegacySecrets(ctx context.Context) (int, error) {
	plain, err := r.q.PlatformConfigListPlaintextSecrets(ctx)
	if err != nil {
		return 0, err
	}
	sealed, err := r.q.PlatformConfigListEncryptedSecrets(ctx)
	if err != nil {
		return 0, err
	}
	if r.enc == nil {
		if len(plain)+len(sealed) > 0 {
			slog.ErrorContext(ctx, "platform config: FLOWCATALYST_APP_KEY is not set; SECRET values cannot be read or written until it is",
				"plaintextSecrets", len(plain), "encryptedSecrets", len(sealed))
		}
		return 0, nil
	}
	changed := 0
	for _, row := range plain {
		ok, err := r.rebind(ctx, row.ID, row.ApplicationCode, row.Section, row.Property, row.ClientID, row.Value, row.Value)
		if err != nil {
			return changed, err
		}
		if ok {
			changed++
		}
	}
	unreadable := 0
	for _, row := range sealed {
		pt, bound, err := r.enc.DecryptBound(row.Value, secretAAD(row.ApplicationCode, row.Section, row.Property, row.ClientID))
		if err != nil {
			unreadable++
			continue
		}
		if bound {
			continue
		}
		ok, err := r.rebind(ctx, row.ID, row.ApplicationCode, row.Section, row.Property, row.ClientID, row.Value, pt)
		if err != nil {
			return changed, err
		}
		if ok {
			changed++
		}
	}
	if unreadable > 0 {
		slog.WarnContext(ctx, "platform config: SECRET values that no configured key opens were left as they are (after a key rotation, set FLOWCATALYST_APP_KEY_PREVIOUS)",
			"count", unreadable)
	}
	return changed, nil
}

// ApplicationIDByCode is the id of the application with code, "" when there
// is none (config rows are keyed by code, without a foreign key).
func (r *Repository) ApplicationIDByCode(ctx context.Context, code string) (string, error) {
	row, err := r.q.PlatformConfigApplicationByCode(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(row.ID), nil
}

// ServiceAccountClients implements ClientLinks.
func (r *Repository) ServiceAccountClients(ctx context.Context, principalID string) ([]string, bool, error) {
	clients, err := r.q.PlatformConfigServiceAccountClientIDs(ctx, ids.PrincipalID(principalID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return clients, true, nil
}

// ApplicationEnabledClients implements ClientLinks.
func (r *Repository) ApplicationEnabledClients(ctx context.Context, applicationID string) ([]string, error) {
	rows, err := r.q.PlatformConfigApplicationEnabledClientIDs(ctx, ids.ApplicationID(applicationID))
	if err != nil {
		return nil, err
	}
	out := make([]string, len(rows))
	for i, c := range rows {
		out[i] = string(c)
	}
	return out, nil
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
