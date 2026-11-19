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

// BinaryRepository is the only place app_binary_files and app_binaries are
// read and written.
type BinaryRepository struct {
	pool *pgxpool.Pool
}

// NewBinaryRepository returns a BinaryRepository over pool.
func NewBinaryRepository(pool *pgxpool.Pool) *BinaryRepository { return &BinaryRepository{pool: pool} }

const factColumns = `platform, package_name, version_code, version_name, min_sdk, debuggable, runtime_version,
	update_url, update_channel, update_key_id, signing_cert_sha256, signing_cert_subject, debug_certificate, signing_schemes`

func factArgs(f *BinaryFacts) []any {
	schemes := make([]int32, 0, len(f.SigningSchemes))
	for _, s := range f.SigningSchemes {
		schemes = append(schemes, int32(s)) //nolint:gosec // G115: 2 or 3
	}
	return []any{
		f.Platform, f.PackageName, f.VersionCode, f.VersionName, f.MinSDK, f.Debuggable, f.RuntimeVersion,
		f.UpdateURL, f.UpdateChannel, f.UpdateKeyID, f.SigningCertSHA256, f.SigningCertSubject, f.DebugCertificate, schemes,
	}
}

func factDest(f *BinaryFacts, schemes *[]int32) []any {
	return []any{
		&f.Platform, &f.PackageName, &f.VersionCode, &f.VersionName, &f.MinSDK, &f.Debuggable, &f.RuntimeVersion,
		&f.UpdateURL, &f.UpdateChannel, &f.UpdateKeyID, &f.SigningCertSHA256, &f.SigningCertSubject, &f.DebugCertificate, schemes,
	}
}

func setSchemes(f *BinaryFacts, schemes []int32) {
	f.SigningSchemes = make([]int, 0, len(schemes))
	for _, s := range schemes {
		f.SigningSchemes = append(f.SigningSchemes, int(s))
	}
}

// RecordFile registers a binary the store has accepted for an application,
// with what was read from it. Idempotent: a second registration of the same
// file changes nothing (its facts are those of the same bytes).
func (r *BinaryRepository) RecordFile(ctx context.Context, applicationID string, f BinaryFile, by string) (created bool, err error) {
	args := append([]any{applicationID, f.SHA256, f.SizeBytes, by, time.Now().UTC()}, factArgs(&f.BinaryFacts)...)
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO app_binary_files (application_id, sha256, size_bytes, created_by, created_at, `+factColumns+`)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		ON CONFLICT DO NOTHING`, args...)
	if err != nil {
		return false, fmt.Errorf("record binary file: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// FindFile is the application's registry row of a binary (nil when it has not
// uploaded it).
func (r *BinaryRepository) FindFile(ctx context.Context, applicationID, sha256Hex string) (*BinaryFile, error) {
	return r.findFile(ctx, r.pool, applicationID, sha256Hex)
}

// FindFileTx is FindFile read in tx.
func (r *BinaryRepository) FindFileTx(ctx context.Context, tx pgx.Tx, applicationID, sha256Hex string) (*BinaryFile, error) {
	return r.findFile(ctx, tx, applicationID, sha256Hex)
}

type querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (r *BinaryRepository) findFile(ctx context.Context, q querier, applicationID, sha256Hex string) (*BinaryFile, error) {
	rows, err := q.Query(ctx, `SELECT sha256, size_bytes, `+factColumns+`
		FROM app_binary_files WHERE application_id = $1 AND sha256 = $2`, applicationID, sha256Hex)
	if err != nil {
		return nil, err
	}
	f, err := pgx.CollectOneRow(rows, func(row pgx.CollectableRow) (BinaryFile, error) {
		var f BinaryFile
		var schemes []int32
		err := row.Scan(append([]any{&f.SHA256, &f.SizeBytes}, factDest(&f.BinaryFacts, &schemes)...)...)
		setSchemes(&f.BinaryFacts, schemes)
		return f, err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &f, nil
}

const binaryColumns = `b.id, b.application_id, COALESCE(a.code, ''), b.app_key, b.channel, b.variant, b.sha256, b.size_bytes,
	b.platform, b.package_name, b.version_code, b.version_name, b.min_sdk, b.debuggable, b.runtime_version,
	b.update_url, b.update_channel, b.update_key_id, b.signing_cert_sha256, b.signing_cert_subject, b.debug_certificate,
	b.signing_schemes, b.message, b.source_commit, b.published_by, p.name, b.created_at`

const binaryFrom = `FROM app_binaries b
	LEFT JOIN app_applications a ON a.id = b.application_id
	LEFT JOIN iam_principals p ON p.id = b.published_by`

func scanBinary(row pgx.CollectableRow) (Binary, error) {
	var b Binary
	var schemes []int32
	f := &b.BinaryFacts
	err := row.Scan(&b.ID, &b.ApplicationID, &b.ApplicationCode, &b.AppKey, &b.Channel, &b.Variant, &b.SHA256, &b.SizeBytes,
		&f.Platform, &f.PackageName, &f.VersionCode, &f.VersionName, &f.MinSDK, &f.Debuggable, &f.RuntimeVersion,
		&f.UpdateURL, &f.UpdateChannel, &f.UpdateKeyID, &f.SigningCertSHA256, &f.SigningCertSubject, &f.DebugCertificate,
		&schemes, &b.Message, &b.SourceCommit, &b.PublishedBy, &b.PublishedName, &b.CreatedAt)
	setSchemes(f, schemes)
	b.CreatedAt = b.CreatedAt.UTC()
	return b, err
}

// Persist writes a new binary. Binaries are immutable: a second Persist of the
// same id changes nothing. Called by the unit of work inside the committing
// transaction.
func (r *BinaryRepository) Persist(ctx context.Context, b *Binary, tx *usecasepgx.DbTx) error {
	args := append([]any{
		b.ID, b.ApplicationID, b.AppKey, b.Channel, b.Variant, b.SHA256, b.SizeBytes,
		b.Message, b.SourceCommit, b.PublishedBy, b.CreatedAt,
	}, factArgs(&b.BinaryFacts)...)
	_, err := tx.Inner().Exec(ctx, `
		INSERT INTO app_binaries (id, application_id, app_key, channel, variant, sha256, size_bytes,
			message, source_commit, published_by, created_at, `+factColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25)
		ON CONFLICT (id) DO NOTHING`, args...)
	if err != nil {
		return fmt.Errorf("persist app binary: %w", err)
	}
	return nil
}

// Delete is not supported: binaries are kept (retention is a later slice).
func (r *BinaryRepository) Delete(_ context.Context, _ *Binary, _ *usecasepgx.DbTx) error {
	return errors.New("app binaries are not deleted")
}

// Place is where a binary is published: one variant of a phone app's channel
// on one platform.
type Place struct {
	ApplicationID string
	AppKey        string
	Platform      string
	Channel       string
	Variant       string
}

// LockPlace serialises publishers of one place until tx ends, so the
// version-code and same-file checks hold under concurrent publishes.
func (r *BinaryRepository) LockPlace(ctx context.Context, tx pgx.Tx, p Place) error {
	key := p.ApplicationID + "\n" + p.AppKey + "\n" + p.Platform + "\n" + p.Channel + "\n" + p.Variant
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 120))`, key)
	return err
}

// PlaceStateTx is what a publish checks, read in tx: the binary of the place
// that is this file or has this version code (nil when none), and the newest
// binary of the place (nil when the place is empty).
func (r *BinaryRepository) PlaceStateTx(ctx context.Context, tx pgx.Tx, p Place, sha256Hex string, versionCode int64) (same, newest *Binary, err error) {
	rows, err := tx.Query(ctx, `SELECT `+binaryColumns+`, (b.sha256 = $6 OR b.version_code = $7) AS clash `+binaryFrom+`
		WHERE b.application_id = $1 AND b.app_key = $2 AND b.platform = $3 AND b.channel = $4 AND b.variant = $5
		  AND (b.sha256 = $6 OR b.version_code = $7 OR b.version_code = (
			SELECT max(version_code) FROM app_binaries
			 WHERE application_id = $1 AND app_key = $2 AND platform = $3 AND channel = $4 AND variant = $5))
		ORDER BY b.version_code DESC`,
		p.ApplicationID, p.AppKey, p.Platform, p.Channel, p.Variant, sha256Hex, versionCode)
	if err != nil {
		return nil, nil, err
	}
	type res struct {
		b     Binary
		clash bool
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (res, error) {
		var x res
		var schemes []int32
		f := &x.b.BinaryFacts
		err := row.Scan(&x.b.ID, &x.b.ApplicationID, &x.b.ApplicationCode, &x.b.AppKey, &x.b.Channel, &x.b.Variant, &x.b.SHA256, &x.b.SizeBytes,
			&f.Platform, &f.PackageName, &f.VersionCode, &f.VersionName, &f.MinSDK, &f.Debuggable, &f.RuntimeVersion,
			&f.UpdateURL, &f.UpdateChannel, &f.UpdateKeyID, &f.SigningCertSHA256, &f.SigningCertSubject, &f.DebugCertificate,
			&schemes, &x.b.Message, &x.b.SourceCommit, &x.b.PublishedBy, &x.b.PublishedName, &x.b.CreatedAt, &x.clash)
		setSchemes(f, schemes)
		return x, err
	})
	if err != nil {
		return nil, nil, err
	}
	for i := range list {
		if i == 0 {
			b := list[0].b
			newest = &b
		}
		if list[i].clash && same == nil {
			b := list[i].b
			same = &b
		}
	}
	return same, newest, nil
}

// FindBinary loads a binary (nil when absent).
func (r *BinaryRepository) FindBinary(ctx context.Context, id string) (*Binary, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+binaryColumns+` `+binaryFrom+` WHERE b.id = $1`, id)
	if err != nil {
		return nil, err
	}
	b, err := pgx.CollectOneRow(rows, scanBinary)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// BinaryFilter narrows ListBinaries. ApplicationID is required; the rest are
// optional ("" is any).
type BinaryFilter struct {
	ApplicationID  string
	AppKey         string
	Platform       string
	Channel        string
	Variant        string
	RuntimeVersion string
	Limit, Offset  int64
}

// ListBinaries returns the binaries matching f, newest version code first
// (then newest published), the total, and the ids of those that are the
// newest of their place (what an install link gives).
func (r *BinaryRepository) ListBinaries(ctx context.Context, f BinaryFilter) ([]Binary, int64, map[string]bool, error) {
	where := `b.application_id = $1
		AND ($2 = '' OR b.app_key = $2) AND ($3 = '' OR b.platform = $3) AND ($4 = '' OR b.channel = $4)
		AND ($5 = '' OR b.variant = $5) AND ($6 = '' OR b.runtime_version = $6)`
	args := []any{f.ApplicationID, f.AppKey, f.Platform, f.Channel, f.Variant, f.RuntimeVersion}
	var total int64
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM app_binaries b WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, nil, err
	}
	rows, err := r.pool.Query(ctx, `SELECT `+binaryColumns+`,
			b.version_code = max(b.version_code) OVER (PARTITION BY b.app_key, b.platform, b.channel, b.variant) AS newest
		`+binaryFrom+` WHERE `+where+`
		ORDER BY b.app_key, b.channel, b.platform, b.version_code DESC, b.created_at DESC, b.id DESC
		LIMIT $7 OFFSET $8`, append(args, f.Limit, f.Offset)...)
	if err != nil {
		return nil, 0, nil, err
	}
	newest := map[string]bool{}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Binary, error) {
		var b Binary
		var schemes []int32
		var isNewest bool
		bf := &b.BinaryFacts
		err := row.Scan(&b.ID, &b.ApplicationID, &b.ApplicationCode, &b.AppKey, &b.Channel, &b.Variant, &b.SHA256, &b.SizeBytes,
			&bf.Platform, &bf.PackageName, &bf.VersionCode, &bf.VersionName, &bf.MinSDK, &bf.Debuggable, &bf.RuntimeVersion,
			&bf.UpdateURL, &bf.UpdateChannel, &bf.UpdateKeyID, &bf.SigningCertSHA256, &bf.SigningCertSubject, &bf.DebugCertificate,
			&schemes, &b.Message, &b.SourceCommit, &b.PublishedBy, &b.PublishedName, &b.CreatedAt, &isNewest)
		setSchemes(bf, schemes)
		b.CreatedAt = b.CreatedAt.UTC()
		if isNewest {
			newest[b.ID] = true
		}
		return b, err
	})
	if err != nil {
		return nil, 0, nil, err
	}
	return list, total, newest, nil
}

// HasRuntimeVersion reports whether a binary of the runtime version is
// published on the channel (any variant), and how many.
func (r *BinaryRepository) HasRuntimeVersion(ctx context.Context, applicationID, appKey, platform, channel, runtimeVersion string) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM app_binaries
		WHERE application_id = $1 AND app_key = $2 AND platform = $3 AND channel = $4 AND runtime_version = $5`,
		applicationID, appKey, platform, channel, runtimeVersion).Scan(&n)
	return n, err
}

// HasRuntimeVersionTx is HasRuntimeVersion read in tx.
func (r *BinaryRepository) HasRuntimeVersionTx(ctx context.Context, tx pgx.Tx, applicationID, appKey, platform, channel, runtimeVersion string) (int64, error) {
	var n int64
	err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM app_binaries
		WHERE application_id = $1 AND app_key = $2 AND platform = $3 AND channel = $4 AND runtime_version = $5`,
		applicationID, appKey, platform, channel, runtimeVersion).Scan(&n)
	return n, err
}
