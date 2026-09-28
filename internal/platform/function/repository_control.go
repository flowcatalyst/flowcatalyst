package function

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
)

// This file holds the reads (and small tx-scoped writes) the control plane
// (internal/platform/function/control, WP5) needs that the CRUD repository
// above does not already expose. Hand-rolled pgx SQL rather than sqlc, for
// the same reason WP4's Versions section gives: internal/sqlc/queries/
// function.sql is shared with concurrent work on this repository, so a
// `make sqlc` regen here could collide with it.

// ── Functions by pool ──────────────────────────────────────────────────────

// FindFunctionsByPool returns every function whose runner pool is pool
// (docs/function-runner-plan.md §8.3): fng_functions.pool, or "default" when
// unset — matching FC_FUNCTIONS_POOL's own default
// (internal/server/functions.go LoadFunctionRunnerEnv). Ordered by address
// for a stable, diffable rendering of the desired document.
func (r *Repository) FindFunctionsByPool(ctx context.Context, pool string) ([]Function, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT f.id, f.application_id, f.client_id, f.name, f.address,
		       f.description, f.pool, f.warm, f.limits, f.created_by,
		       f.created_at, f.updated_at, a.code AS application_code
		FROM fng_functions f
		LEFT JOIN app_applications a ON a.id = f.application_id
		WHERE COALESCE(f.pool, 'default') = $1
		ORDER BY f.address`, pool)
	if err != nil {
		return nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[functionJoinRow])
	if err != nil {
		return nil, err
	}
	out := make([]Function, 0, len(collected))
	for _, row := range collected {
		fn, err := rowToFunction(row.FngFunction, row.ApplicationCode)
		if err != nil {
			return nil, err
		}
		out = append(out, *fn)
	}
	return out, nil
}

// ── Settings, decrypted ─────────────────────────────────────────────────────

// ResolvedSettings is a function's platform-held config/secret/DB values,
// decrypted. For the control plane only: the public API's ListSettings
// deliberately blanks SECRET/DB values (they are write-only there), but the
// desired document the runner polls (docs/function-runner-plan.md §8.3) is
// served only to the runner role and is never logged, so it carries the
// plaintext the runner needs to answer config.get/secret.get/db.query host
// calls and to open database connections.
type ResolvedSettings struct {
	Config  map[string]string
	Secrets map[string]string
	DB      map[string]string
}

// ResolveSettingsForControl loads and decrypts every setting for functionID.
func (r *Repository) ResolveSettingsForControl(ctx context.Context, functionID string) (ResolvedSettings, error) {
	out := ResolvedSettings{Config: map[string]string{}, Secrets: map[string]string{}, DB: map[string]string{}}
	rows, err := r.q.FunctionSettingList(ctx, functionID)
	if err != nil {
		return out, err
	}
	for _, row := range rows {
		kind, ok := ParseSettingKind(row.Kind)
		if !ok {
			return out, fmt.Errorf("function repo: setting %s/%s has an unrecognised kind %q", row.FunctionID, row.Key, row.Kind)
		}
		value := row.Value
		if kind == SettingSecret || kind == SettingDB {
			value = decryptSettingValue(r.enc, value)
		}
		switch kind {
		case SettingConfig:
			out.Config[row.Key] = value
		case SettingSecret:
			out.Secrets[row.Key] = value
		case SettingDB:
			out.DB[row.Key] = value
		}
	}
	return out, nil
}

// decryptSettingValue decrypts an at-rest SECRET/DB value, mirroring
// serviceaccount's decryptSecretRef convention: a decrypt failure (a legacy
// plaintext row predating encryption, or an external secret-manager
// reference such as "aws-sm://…" this platform does not yet resolve at read
// time) returns the stored value unchanged rather than dropping it — a
// runner that receives a raw reference instead of a secret fails loudly at
// the point of use, where silently blanking the value would fail silently
// instead.
func decryptSettingValue(enc *encryption.Service, value string) string {
	if enc == nil || value == "" {
		return value
	}
	pt, err := enc.Decrypt(value)
	if err != nil {
		return value
	}
	return pt
}

// ── Artifacts ────────────────────────────────────────────────────────────

// VersionRuntimeForDigest reports the runtime ("wasm"/"js") of some version
// referencing digest, and whether any version does. The control plane's
// artifact route (docs/function-runner-plan.md §8.3) serves only digests a
// version actually references — 404 otherwise — and picks the response
// Content-Type from the runtime.
func (r *Repository) VersionRuntimeForDigest(ctx context.Context, digest string) (runtime string, ok bool, err error) {
	err = r.pool.QueryRow(ctx, `SELECT runtime FROM fng_versions WHERE digest = $1 LIMIT 1`, digest).Scan(&runtime)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return runtime, true, nil
}

// ── Pool revisions (raw-tx sibling of BumpPoolRevision) ─────────────────────

// bumpPoolRevisionTx is BumpPoolRevision's shared implementation, taking a
// raw pgx.Tx so it is callable both from a usecasepgx.DbTx (BumpPoolRevision,
// repository.go) and from the control plane's own short transactions
// (BumpPoolRevisionTx below) — heartbeat processing is runner bookkeeping,
// not a usecaseop/UnitOfWork command with a principal to authorize or audit
// (see UpsertRunnerHeartbeat's doc comment above), so it never holds a
// *usecasepgx.DbTx to begin with.
func (r *Repository) bumpPoolRevisionTx(ctx context.Context, tx pgx.Tx, pool string) (int64, error) {
	rev, err := r.q.WithTx(tx).FunctionPoolRevisionBump(ctx, pool)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_notify('fng_desired', $1)`, pool); err != nil {
		return 0, fmt.Errorf("function repo: notify fng_desired: %w", err)
	}
	return rev, nil
}

// BumpPoolRevisionTx bumps pool's revision (+ NOTIFY, same transaction) on
// an already-open raw pgx.Tx. Use WithPoolTx to open one.
func (r *Repository) BumpPoolRevisionTx(ctx context.Context, tx pgx.Tx, pool string) (int64, error) {
	return r.bumpPoolRevisionTx(ctx, tx, pool)
}

// WithPoolTx runs fn in a fresh transaction on the repository's own pool,
// committing on success and rolling back otherwise (including when fn
// panics, via the deferred Rollback: a transaction pgx has not committed is
// safe to roll back twice, so the plain success path's Commit is not
// shadowed by it).
func (r *Repository) WithPoolTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ── Version status, tx-scoped ────────────────────────────────────────────

// SetVersionReadyTx is SetVersionReady on an already-open tx — heartbeat
// processing pairs it with BumpPoolRevisionTx in one WithPoolTx transaction
// so a version's READY transition and the revision bump that announces it
// land atomically.
func (r *Repository) SetVersionReadyTx(ctx context.Context, tx pgx.Tx, id string, readyAt time.Time) error {
	return r.q.WithTx(tx).FunctionVersionSetReady(ctx, dbq.FunctionVersionSetReadyParams{ID: id, ReadyAt: &readyAt})
}

// SetVersionFailureTx is SetVersionFailure on an already-open tx.
func (r *Repository) SetVersionFailureTx(ctx context.Context, tx pgx.Tx, id string, failure []byte) error {
	return r.q.WithTx(tx).FunctionVersionSetFailure(ctx, dbq.FunctionVersionSetFailureParams{ID: id, Failure: failure})
}
