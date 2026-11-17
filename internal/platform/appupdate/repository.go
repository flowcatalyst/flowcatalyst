package appupdate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Repository is the only place the app_update_* tables are read and written.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository returns a Repository over pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const updateColumns = `u.id, u.application_id, COALESCE(a.code, ''), u.app_key, u.platform, u.runtime_version, u.channel,
	u.kind, COALESCE(u.manifest_id::text, ''), u.created_at, u.rollout_percent, u.message, u.source_commit,
	u.republished_from, u.expo_client, u.published_by, p.name, u.updated_at`

const updateFrom = `FROM app_updates u
	LEFT JOIN app_applications a ON a.id = u.application_id
	LEFT JOIN iam_principals p ON p.id = u.published_by`

func scanUpdate(row pgx.CollectableRow) (Update, error) {
	var u Update
	var kind string
	err := row.Scan(&u.ID, &u.ApplicationID, &u.ApplicationCode, &u.AppKey, &u.Platform, &u.RuntimeVersion, &u.Channel,
		&kind, &u.ManifestID, &u.CreatedAt, &u.RolloutPercent, &u.Message, &u.SourceCommit,
		&u.RepublishedFrom, &u.ExpoClient, &u.PublishedBy, &u.PublishedName, &u.UpdatedAt)
	u.Kind = Kind(kind)
	u.Branch.ApplicationID = u.ApplicationID
	u.CreatedAt = u.CreatedAt.UTC()
	u.UpdatedAt = u.UpdatedAt.UTC()
	return u, err
}

// Persist writes an update: a new row with its assets, or (an existing row)
// its rollout. Rows are otherwise immutable. Called by the unit of work inside
// the committing transaction.
func (r *Repository) Persist(ctx context.Context, u *Update, tx *usecasepgx.DbTx) error {
	var manifestID any
	if u.ManifestID != "" {
		manifestID = u.ManifestID
	}
	_, err := tx.Inner().Exec(ctx, `
		INSERT INTO app_updates (id, application_id, app_key, platform, runtime_version, channel, kind, manifest_id,
			created_at, rollout_percent, message, source_commit, republished_from, expo_client, published_by, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (id) DO UPDATE SET rollout_percent = EXCLUDED.rollout_percent, updated_at = EXCLUDED.updated_at`,
		u.ID, u.ApplicationID, u.AppKey, u.Platform, u.RuntimeVersion, u.Channel, string(u.Kind), manifestID,
		u.CreatedAt, u.RolloutPercent, u.Message, u.SourceCommit, u.RepublishedFrom, nullJSON(u.ExpoClient), u.PublishedBy, u.UpdatedAt)
	if err != nil {
		return fmt.Errorf("persist app update: %w", err)
	}
	if len(u.Assets) == 0 {
		return nil
	}
	n := len(u.Assets)
	ordinals := make([]int32, n)
	shas, md5s, exts, cts := make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	launch := make([]bool, n)
	for i, a := range u.Assets {
		ordinals[i] = int32(i) //nolint:gosec // G115: at most MaxAssets
		shas[i], md5s[i], exts[i], cts[i], launch[i] = a.SHA256, a.MD5, a.FileExtension, a.ContentType, a.IsLaunchAsset
	}
	if _, err := tx.Inner().Exec(ctx, `
		INSERT INTO app_update_assets (update_id, ordinal, sha256, md5, file_extension, content_type, is_launch_asset)
		SELECT $1, * FROM UNNEST($2::int[], $3::text[], $4::text[], $5::text[], $6::text[], $7::bool[])
		ON CONFLICT DO NOTHING`, u.ID, ordinals, shas, md5s, exts, cts, launch); err != nil {
		return fmt.Errorf("persist app update assets: %w", err)
	}
	return nil
}

// Delete is not supported: updates are kept (retention is a later slice).
func (r *Repository) Delete(_ context.Context, _ *Update, _ *usecasepgx.DbTx) error {
	return errors.New("app updates are not deleted")
}

func nullJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

// LockBranch serialises writers of one branch until tx ends (a transaction
// advisory lock), so commit times stay strictly increasing and the
// one-partial-rollout rule holds under concurrent publishes.
func (r *Repository) LockBranch(ctx context.Context, tx pgx.Tx, b Branch) error {
	// "\n" cannot occur in any of the validated fields.
	key := b.ApplicationID + "\n" + b.AppKey + "\n" + b.Platform + "\n" + b.RuntimeVersion + "\n" + b.Channel
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 117))`, key)
	return err
}

// HeadTx is the branch's newest n rows, newest first, read in tx (no assets).
func (r *Repository) HeadTx(ctx context.Context, tx pgx.Tx, b Branch, n int) ([]Update, error) {
	rows, err := tx.Query(ctx, `SELECT `+updateColumns+` `+updateFrom+`
		WHERE u.application_id = $1 AND u.app_key = $2 AND u.platform = $3 AND u.runtime_version = $4 AND u.channel = $5
		ORDER BY u.created_at DESC, u.id DESC LIMIT $6`,
		b.ApplicationID, b.AppKey, b.Platform, b.RuntimeVersion, b.Channel, n)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanUpdate)
}

// PublicHead is what the manifest route reads: the newest two rows of the
// branch of an ACTIVE application named by its code. One indexed query; no
// rows for an unknown or inactive application, app key or channel.
func (r *Repository) PublicHead(ctx context.Context, applicationCode, appKey, platform, runtimeVersion, channel string) ([]Update, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+updateColumns+`
		FROM app_updates u
		JOIN app_applications a ON a.id = u.application_id AND a.active
		LEFT JOIN iam_principals p ON FALSE -- the publisher's name is not needed here
		WHERE a.code = $1 AND u.app_key = $2 AND u.platform = $3 AND u.runtime_version = $4 AND u.channel = $5
		ORDER BY u.created_at DESC, u.id DESC LIMIT 2`,
		applicationCode, appKey, platform, runtimeVersion, channel)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, scanUpdate)
}

// PublicAsset reports the content type and size of a file when it belongs to
// a published update of the phone app (application by code, active; app
// key); found is false otherwise, whatever the reason.
func (r *Repository) PublicAsset(ctx context.Context, applicationCode, appKey, sha256Hex string) (contentType string, size int64, found bool, err error) {
	rows, err := r.pool.Query(ctx, `
		SELECT x.content_type, f.size_bytes
		  FROM app_update_assets x
		  JOIN app_updates u ON u.id = x.update_id
		  JOIN app_applications a ON a.id = u.application_id AND a.active
		  JOIN app_update_files f ON f.application_id = u.application_id AND f.sha256 = x.sha256
		 WHERE x.sha256 = $3 AND a.code = $1 AND u.app_key = $2
		 LIMIT 1`, applicationCode, appKey, sha256Hex)
	if err != nil {
		return "", 0, false, err
	}
	type res struct {
		CT   string
		Size int64
	}
	got, err := pgx.CollectOneRow(rows, func(row pgx.CollectableRow) (res, error) {
		var x res
		return x, row.Scan(&x.CT, &x.Size)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, err
	}
	return got.CT, got.Size, true, nil
}

// FindByID loads an update (nil when absent), with its assets when withAssets.
func (r *Repository) FindByID(ctx context.Context, id string, withAssets bool) (*Update, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+updateColumns+` `+updateFrom+` WHERE u.id = $1`, id)
	if err != nil {
		return nil, err
	}
	u, err := pgx.CollectOneRow(rows, scanUpdate)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if withAssets {
		m, err := r.AssetsOf(ctx, []string{u.ID})
		if err != nil {
			return nil, err
		}
		u.Assets = m[u.ID]
		u.TotalBytes, err = r.totalBytes(ctx, u.ApplicationID, u.Assets)
		if err != nil {
			return nil, err
		}
	}
	return &u, nil
}

// AssetsOf loads the assets of the updates ids, in manifest order.
func (r *Repository) AssetsOf(ctx context.Context, ids []string) (map[string][]Asset, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT update_id, sha256, md5, file_extension, content_type, is_launch_asset
		  FROM app_update_assets WHERE update_id = ANY($1) ORDER BY update_id, ordinal`, ids)
	if err != nil {
		return nil, err
	}
	out := map[string][]Asset{}
	_, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (struct{}, error) {
		var id string
		var a Asset
		if err := row.Scan(&id, &a.SHA256, &a.MD5, &a.FileExtension, &a.ContentType, &a.IsLaunchAsset); err != nil {
			return struct{}{}, err
		}
		out[id] = append(out[id], a)
		return struct{}{}, nil
	})
	return out, err
}

func (r *Repository) totalBytes(ctx context.Context, applicationID string, assets []Asset) (int64, error) {
	shas := make([]string, 0, len(assets))
	for _, a := range assets {
		shas = append(shas, a.SHA256)
	}
	var total int64
	err := r.pool.QueryRow(ctx, `SELECT COALESCE(SUM(size_bytes), 0)::bigint FROM app_update_files
		WHERE application_id = $1 AND sha256 = ANY($2)`, applicationID, shas).Scan(&total)
	return total, err
}

// RecordFile registers a file the store has accepted for an application.
// Idempotent: a second registration of the same file changes nothing.
func (r *Repository) RecordFile(ctx context.Context, applicationID string, f File, by string) (created bool, err error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO app_update_files (application_id, sha256, md5, size_bytes, created_by, created_at)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6) ON CONFLICT DO NOTHING`,
		applicationID, f.SHA256, f.MD5, f.SizeBytes, by, time.Now().UTC())
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// HasFile reports whether the application has stored the file.
func (r *Repository) HasFile(ctx context.Context, applicationID, sha256Hex string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM app_update_files WHERE application_id = $1 AND sha256 = $2)`,
		applicationID, sha256Hex).Scan(&ok)
	return ok, err
}

// MissingFiles is the subset of shas the application has not stored, in the
// order asked.
func (r *Repository) MissingFiles(ctx context.Context, applicationID string, shas []string) ([]string, error) {
	have, err := r.files(ctx, r.pool, applicationID, shas)
	if err != nil {
		return nil, err
	}
	return missingOf(shas, have), nil
}

// FilesTx reads, in tx, the application's registry rows for shas.
func (r *Repository) FilesTx(ctx context.Context, tx pgx.Tx, applicationID string, shas []string) (map[string]File, error) {
	return r.files(ctx, tx, applicationID, shas)
}

func (r *Repository) files(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, applicationID string, shas []string,
) (map[string]File, error) {
	rows, err := q.Query(ctx, `SELECT sha256, md5, size_bytes FROM app_update_files
		WHERE application_id = $1 AND sha256 = ANY($2)`, applicationID, shas)
	if err != nil {
		return nil, err
	}
	out := map[string]File{}
	_, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (struct{}, error) {
		var f File
		if err := row.Scan(&f.SHA256, &f.MD5, &f.SizeBytes); err != nil {
			return struct{}{}, err
		}
		out[f.SHA256] = f
		return struct{}{}, nil
	})
	return out, err
}

func missingOf(shas []string, have map[string]File) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range shas {
		if _, ok := have[s]; !ok && !seen[s] {
			out = append(out, s)
			seen[s] = true
		}
	}
	return out
}

// ListFilter narrows List. ApplicationID is required; the rest are optional.
type ListFilter struct {
	ApplicationID  string
	AppKey         string
	Platform       string
	RuntimeVersion string
	Channel        string
	Limit, Offset  int64
}

// List returns the updates matching f, newest first, the total, and for each
// returned update the share of its branch's phones it is served to (see
// ServedPercent).
func (r *Repository) List(ctx context.Context, f ListFilter) ([]Update, int64, map[string]int, error) {
	where := `u.application_id = $1
		AND ($2 = '' OR u.app_key = $2) AND ($3 = '' OR u.platform = $3)
		AND ($4 = '' OR u.runtime_version = $4) AND ($5 = '' OR u.channel = $5)`
	args := []any{f.ApplicationID, f.AppKey, f.Platform, f.RuntimeVersion, f.Channel}
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM app_updates u WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+updateColumns+` `+updateFrom+` WHERE `+where+`
		ORDER BY u.created_at DESC, u.id DESC LIMIT $6 OFFSET $7`, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, nil, err
	}
	list, err := pgx.CollectRows(rows, scanUpdate)
	if err != nil {
		return nil, 0, nil, err
	}
	served, err := r.servedShares(ctx, f.ApplicationID, f.AppKey)
	if err != nil {
		return nil, 0, nil, err
	}
	return list, total, served, nil
}

// servedShares is, for the two newest rows of every branch of the
// application (one app key, or all), the share of phones each is served to.
func (r *Repository) servedShares(ctx context.Context, applicationID, appKey string) (map[string]int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, rn, rollout_percent, first_rollout FROM (
			SELECT id, rollout_percent,
			       row_number() OVER w AS rn,
			       first_value(rollout_percent) OVER w AS first_rollout
			  FROM app_updates
			 WHERE application_id = $1 AND ($2 = '' OR app_key = $2)
			WINDOW w AS (PARTITION BY app_key, platform, runtime_version, channel ORDER BY created_at DESC, id DESC)
		) t WHERE rn <= 2`, applicationID, appKey)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	_, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (struct{}, error) {
		var id string
		var rn int64
		var pct, first int
		if err := row.Scan(&id, &rn, &pct, &first); err != nil {
			return struct{}{}, err
		}
		if rn == 1 {
			out[id] = pct
		} else if first < 100 {
			out[id] = 100 - first
		}
		return struct{}{}, nil
	})
	return out, err
}

// BranchHead is one branch with its two newest rows.
type BranchHead struct {
	Branch
	Rows        []Update // newest first, one or two
	UpdateCount int64
}

// Branches lists the application's branches (one app key, or all), each with
// its newest two rows and its row count, ordered by app key, platform, runtime
// version and channel. At most limit branches.
func (r *Repository) Branches(ctx context.Context, applicationID, appKey string, limit int) ([]BranchHead, error) {
	rows, err := r.pool.Query(ctx, `
		WITH ranked AS (
			SELECT u.id, row_number() OVER w AS rn, count(*) OVER (PARTITION BY u.app_key, u.platform, u.runtime_version, u.channel) AS n
			  FROM app_updates u
			 WHERE u.application_id = $1 AND ($2 = '' OR u.app_key = $2)
			WINDOW w AS (PARTITION BY u.app_key, u.platform, u.runtime_version, u.channel ORDER BY u.created_at DESC, u.id DESC)
		)
		SELECT `+updateColumns+`, ranked.rn, ranked.n `+updateFrom+`
		  JOIN ranked ON ranked.id = u.id
		 WHERE ranked.rn <= 2
		 ORDER BY u.app_key, u.platform, u.runtime_version, u.channel, ranked.rn`, applicationID, appKey)
	if err != nil {
		return nil, err
	}
	type headRow struct {
		u  Update
		rn int64
		n  int64
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (headRow, error) {
		var h headRow
		var kind string
		err := row.Scan(&h.u.ID, &h.u.ApplicationID, &h.u.ApplicationCode, &h.u.AppKey, &h.u.Platform, &h.u.RuntimeVersion, &h.u.Channel,
			&kind, &h.u.ManifestID, &h.u.CreatedAt, &h.u.RolloutPercent, &h.u.Message, &h.u.SourceCommit,
			&h.u.RepublishedFrom, &h.u.ExpoClient, &h.u.PublishedBy, &h.u.PublishedName, &h.u.UpdatedAt, &h.rn, &h.n)
		h.u.Kind = Kind(kind)
		h.u.Branch.ApplicationID = h.u.ApplicationID
		h.u.CreatedAt, h.u.UpdatedAt = h.u.CreatedAt.UTC(), h.u.UpdatedAt.UTC()
		return h, err
	})
	if err != nil {
		return nil, err
	}
	var out []BranchHead
	for _, h := range list {
		if h.rn == 1 {
			if len(out) >= limit {
				break
			}
			out = append(out, BranchHead{Branch: h.u.Branch, UpdateCount: h.n})
		}
		if len(out) > 0 {
			last := &out[len(out)-1]
			last.Rows = append(last.Rows, h.u)
		}
	}
	return out, nil
}

// FindPreviousUpdateTx is the newest UPDATE row of the branch created before t
// (nil when none), read in tx.
func (r *Repository) FindPreviousUpdateTx(ctx context.Context, tx pgx.Tx, b Branch, before time.Time, beforeID string) (*Update, error) {
	rows, err := tx.Query(ctx, `SELECT `+updateColumns+` `+updateFrom+`
		WHERE u.application_id = $1 AND u.app_key = $2 AND u.platform = $3 AND u.runtime_version = $4 AND u.channel = $5
		  AND u.kind = 'UPDATE' AND (u.created_at, u.id) < ($6, $7)
		ORDER BY u.created_at DESC, u.id DESC LIMIT 1`,
		b.ApplicationID, b.AppKey, b.Platform, b.RuntimeVersion, b.Channel, before, beforeID)
	if err != nil {
		return nil, err
	}
	u, err := pgx.CollectOneRow(rows, scanUpdate)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// AssetsOfTx loads one update's assets in tx.
func (r *Repository) AssetsOfTx(ctx context.Context, tx pgx.Tx, id string) ([]Asset, error) {
	rows, err := tx.Query(ctx, `
		SELECT sha256, md5, file_extension, content_type, is_launch_asset
		  FROM app_update_assets WHERE update_id = $1 ORDER BY ordinal`, id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Asset, error) {
		var a Asset
		return a, row.Scan(&a.SHA256, &a.MD5, &a.FileExtension, &a.ContentType, &a.IsLaunchAsset)
	})
}

// ServedShare is the share of its branch's phones update u is served to (see
// ServedPercent).
func (r *Repository) ServedShare(ctx context.Context, u *Update) (int, error) {
	m, err := r.servedShares(ctx, u.ApplicationID, u.AppKey)
	if err != nil {
		return 0, err
	}
	return m[u.ID], nil
}
