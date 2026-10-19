package function

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ErrVersionGone is returned when a version a caller meant to point an alias
// at was deleted (pruned) between its read and its write.
var ErrVersionGone = fmt.Errorf("function repo: version no longer exists")

// LockFunctionTx takes the function row's write lock (SELECT … FOR UPDATE) —
// the same lock publish takes for version numbering — and reports whether the
// function exists. Prune and alias writes use it to serialise against each
// other and against publish.
func (r *Repository) LockFunctionTx(ctx context.Context, tx pgx.Tx, functionID string) (bool, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM fng_functions WHERE id = $1 FOR UPDATE`, functionID)
	if err != nil {
		return false, fmt.Errorf("function repo: lock function row: %w", err)
	}
	defer rows.Close()
	found := rows.Next()
	rows.Close()
	return found, rows.Err()
}

// ListVersionsTx is ListVersionsByFunction on an open transaction.
func (r *Repository) ListVersionsTx(ctx context.Context, tx pgx.Tx, functionID string) ([]Version, error) {
	rows, err := tx.Query(ctx, `SELECT `+functionVersionColumns+` FROM fng_versions
		WHERE function_id = $1 ORDER BY number DESC`, functionID)
	if err != nil {
		return nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[functionVersionRow])
	if err != nil {
		return nil, err
	}
	out := make([]Version, 0, len(collected))
	for _, row := range collected {
		v, err := rowToVersion(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, nil
}

// AliasedVersionIDsTx returns the ids of every version some alias of
// functionID points at.
func (r *Repository) AliasedVersionIDsTx(ctx context.Context, tx pgx.Tx, functionID string) (map[string]bool, error) {
	rows, err := tx.Query(ctx, `SELECT version_id FROM fng_aliases WHERE function_id = $1`, functionID)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// DeleteUnaliasedVersionTx deletes one version row unless an alias points at
// it (the guard that keeps a tagged version safe even if the caller's view was
// stale). Reports whether a row was deleted.
func (r *Repository) DeleteUnaliasedVersionTx(ctx context.Context, tx pgx.Tx, versionID string) (bool, error) {
	tag, err := tx.Exec(ctx, `DELETE FROM fng_versions v WHERE v.id = $1
		AND NOT EXISTS (SELECT 1 FROM fng_aliases a WHERE a.version_id = v.id)`, versionID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// VersionExistsTx reports whether the version row still exists.
func (r *Repository) VersionExistsTx(ctx context.Context, tx pgx.Tx, versionID string) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM fng_versions WHERE id = $1)`, versionID).Scan(&ok)
	return ok, err
}

// DigestReferencedTx reports whether any version of any function still has
// digest.
func (r *Repository) DigestReferencedTx(ctx context.Context, tx pgx.Tx, digest string) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM fng_versions WHERE digest = $1)`, digest).Scan(&ok)
	return ok, err
}

// DigestReferenced is DigestReferencedTx on the pool.
func (r *Repository) DigestReferenced(ctx context.Context, digest string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM fng_versions WHERE digest = $1)`, digest).Scan(&ok)
	return ok, err
}
