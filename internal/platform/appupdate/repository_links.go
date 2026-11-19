package appupdate

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// LinkRepository is the only place app_install_links is read and written.
type LinkRepository struct {
	pool *pgxpool.Pool
}

// NewLinkRepository returns a LinkRepository over pool.
func NewLinkRepository(pool *pgxpool.Pool) *LinkRepository { return &LinkRepository{pool: pool} }

const linkColumns = `l.id, l.application_id, COALESCE(a.code, ''), l.app_key, l.channel, l.token_hash, l.generation,
	l.created_by, l.created_at, l.updated_by, l.updated_at`

func scanLink(row pgx.CollectableRow) (InstallLink, error) {
	var l InstallLink
	err := row.Scan(&l.ID, &l.ApplicationID, &l.ApplicationCode, &l.AppKey, &l.Channel, &l.TokenHash, &l.Generation,
		&l.CreatedBy, &l.CreatedAt, &l.UpdatedBy, &l.UpdatedAt)
	l.CreatedAt, l.UpdatedAt = l.CreatedAt.UTC(), l.UpdatedAt.UTC()
	return l, err
}

// Persist writes a link: a new row, or (an existing id) its new token. Called
// by the unit of work inside the committing transaction.
func (r *LinkRepository) Persist(ctx context.Context, l *InstallLink, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx, `
		INSERT INTO app_install_links (id, application_id, app_key, channel, token_hash, generation,
			created_by, created_at, updated_by, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (id) DO UPDATE SET token_hash = EXCLUDED.token_hash, generation = EXCLUDED.generation,
			updated_by = EXCLUDED.updated_by, updated_at = EXCLUDED.updated_at`,
		l.ID, l.ApplicationID, l.AppKey, l.Channel, l.TokenHash, l.Generation, l.CreatedBy, l.CreatedAt, l.UpdatedBy, l.UpdatedAt)
	if err != nil {
		return fmt.Errorf("persist install link: %w", err)
	}
	return nil
}

// Delete removes a link; its token stops working when the transaction commits.
func (r *LinkRepository) Delete(ctx context.Context, l *InstallLink, tx *usecasepgx.DbTx) error {
	if _, err := tx.Inner().Exec(ctx, `DELETE FROM app_install_links WHERE id = $1`, l.ID); err != nil {
		return fmt.Errorf("delete install link: %w", err)
	}
	return nil
}

// FindLink loads a link (nil when absent).
func (r *LinkRepository) FindLink(ctx context.Context, id string) (*InstallLink, error) {
	return r.one(ctx, r.pool, `WHERE l.id = $1`, id)
}

// FindLinkByChannelTx is the link of a phone app's channel, read in tx (nil when none).
func (r *LinkRepository) FindLinkByChannelTx(ctx context.Context, tx pgx.Tx, applicationID, appKey, channel string) (*InstallLink, error) {
	return r.one(ctx, tx, `WHERE l.application_id = $1 AND l.app_key = $2 AND l.channel = $3`, applicationID, appKey, channel)
}

// FindLinkTx is FindLink read in tx, locking the row until tx ends.
func (r *LinkRepository) FindLinkTx(ctx context.Context, tx pgx.Tx, id string) (*InstallLink, error) {
	return r.one(ctx, tx, `WHERE l.id = $1 FOR UPDATE OF l`, id)
}

func (r *LinkRepository) one(ctx context.Context, q querier, where string, args ...any) (*InstallLink, error) {
	rows, err := q.Query(ctx, `SELECT `+linkColumns+` FROM app_install_links l
		LEFT JOIN app_applications a ON a.id = l.application_id `+where, args...)
	if err != nil {
		return nil, err
	}
	l, err := pgx.CollectOneRow(rows, scanLink)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// LockChannel serialises writers of one phone app channel's link until tx ends.
func (r *LinkRepository) LockChannel(ctx context.Context, tx pgx.Tx, applicationID, appKey, channel string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 121))`,
		applicationID+"\n"+appKey+"\n"+channel)
	return err
}

// ListLinks is the application's links (one app key, or all), by app key and channel.
func (r *LinkRepository) ListLinks(ctx context.Context, applicationID, appKey string) ([]InstallLink, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+linkColumns+` FROM app_install_links l
		LEFT JOIN app_applications a ON a.id = l.application_id
		WHERE l.application_id = $1 AND ($2 = '' OR l.app_key = $2)
		ORDER BY l.app_key, l.channel`, applicationID, appKey)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanLink)
}

// PublicInstall is what the install page shows for a link: the application's
// name and the newest Android binary of each variant of the link's channel.
type PublicInstall struct {
	Link            InstallLink
	ApplicationName string
	// Binaries is the newest binary of each variant, in VariantOrder.
	Binaries []Binary
}

// PublicInstallByToken is the install page's read: the link whose token hash
// is tokenHash, of an ACTIVE application, with the newest Android binary of
// each variant of its channel. One indexed query whatever the outcome; nil
// when no link has the hash. The caller still compares the hash in constant
// time and checks the application code and app key of the address.
func (r *LinkRepository) PublicInstallByToken(ctx context.Context, tokenHash string) (*PublicInstall, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT l.id, l.application_id, a.code, l.app_key, l.channel, l.token_hash, l.generation,
		       l.created_by, l.created_at, l.updated_by, l.updated_at, a.name,
		       b.id, b.variant, b.sha256, b.size_bytes, b.package_name, b.version_code, b.version_name,
		       b.signing_cert_sha256, b.debuggable, b.created_at
		  FROM app_install_links l
		  JOIN app_applications a ON a.id = l.application_id AND a.active
		  LEFT JOIN LATERAL (
		        SELECT DISTINCT ON (x.variant) x.id, x.variant, x.sha256, x.size_bytes, x.package_name, x.version_code,
		               x.version_name, x.signing_cert_sha256, x.debuggable, x.created_at
		          FROM app_binaries x
		         WHERE x.application_id = l.application_id AND x.app_key = l.app_key AND x.channel = l.channel
		           AND x.platform = 'android'
		         ORDER BY x.variant, x.version_code DESC
		  ) b ON TRUE
		 WHERE l.token_hash = $1`, tokenHash)
	if err != nil {
		return nil, err
	}
	var out *PublicInstall
	_, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (struct{}, error) {
		var l InstallLink
		var name string
		var bid, variant, sha, pkg, cert *string
		var size, vcode *int64
		var vname *string
		var debuggable *bool
		var created *time.Time
		if err := row.Scan(&l.ID, &l.ApplicationID, &l.ApplicationCode, &l.AppKey, &l.Channel, &l.TokenHash, &l.Generation,
			&l.CreatedBy, &l.CreatedAt, &l.UpdatedBy, &l.UpdatedAt, &name,
			&bid, &variant, &sha, &size, &pkg, &vcode, &vname, &cert, &debuggable, &created); err != nil {
			return struct{}{}, err
		}
		if out == nil {
			out = &PublicInstall{Link: l, ApplicationName: name}
		}
		if bid != nil {
			b := Binary{
				ID: *bid, ApplicationID: l.ApplicationID, ApplicationCode: l.ApplicationCode, AppKey: l.AppKey,
				Channel: l.Channel, Variant: *variant, SHA256: *sha, SizeBytes: *size, CreatedAt: created.UTC(),
			}
			b.Platform, b.PackageName, b.VersionCode, b.VersionName = PlatformAndroid, *pkg, *vcode, vname
			b.SigningCertSHA256, b.Debuggable = *cert, *debuggable
			out.Binaries = append(out.Binaries, b)
		}
		return struct{}{}, nil
	})
	if err != nil {
		return nil, err
	}
	if out != nil {
		slices.SortFunc(out.Binaries, func(a, b Binary) int { return VariantOrder(a.Variant, b.Variant) })
	}
	return out, nil
}
