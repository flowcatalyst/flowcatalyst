package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
)

// FindSetByName loads a set by its unique name; nil when there is none.
func (r *Repository) FindSetByName(ctx context.Context, name string) (*Set, error) {
	res, err := r.q.IngestionSetFindByName(ctx, name)
	row, err := repocommon.One(res, err, "ingestion repo")
	if row == nil || err != nil {
		return nil, err
	}
	return &Set{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

// SetNames returns the names of the sets with these ids, keyed by id.
func (r *Repository) SetNames(ctx context.Context, setIDs []string) (map[string]string, error) {
	rows, err := r.q.IngestionSetFindByIDs(ctx, setIDs)
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: set names: %w", err)
	}
	names := make(map[string]string, len(rows))
	for _, row := range rows {
		names[row.ID] = row.Name
	}
	return names, nil
}

// ListSets returns every set by name, with how many ingestions each owns.
func (r *Repository) ListSets(ctx context.Context) ([]Set, error) {
	rows, err := r.q.IngestionSetList(ctx)
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: list sets: %w", err)
	}
	sets := make([]Set, 0, len(rows))
	for _, row := range rows {
		sets = append(sets, Set{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Ingestions: int(row.Ingestions)})
	}
	return sets, nil
}

// ListManagedBy returns the ingestions a set owns, by client and code.
func (r *Repository) ListManagedBy(ctx context.Context, setID string) ([]*Ingestion, error) {
	rows, err := r.q.IngestionListManagedBy(ctx, &setID)
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: list managed: %w", err)
	}
	out := make([]*Ingestion, 0, len(rows))
	for _, row := range rows {
		i, err := rowToIngestion(row)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, nil
}

// ListSetApplies returns the recorded applies of a set, newest first.
func (r *Repository) ListSetApplies(ctx context.Context, setID string) ([]SetApply, error) {
	rows, err := r.q.IngestionSetApplyList(ctx, setID)
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: list set applies: %w", err)
	}
	out := make([]SetApply, 0, len(rows))
	for _, row := range rows {
		a, err := rowToSetApply(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, nil
}

// FindSetApply loads one recorded apply of a set; nil when the set has no such
// apply (it never had one, or it was forgotten).
func (r *Repository) FindSetApply(ctx context.Context, setID, applyID string) (*SetApply, error) {
	res, err := r.q.IngestionSetApplyFind(ctx, dbq.IngestionSetApplyFindParams{SetID: setID, ID: applyID})
	row, err := repocommon.One(res, err, "ingestion repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToSetApply(*row)
}

// LockSets serialises the applies of every set until the transaction ends, so
// two sets cannot take the same ingestion at once and two applies of one set
// wait for each other. Applies are rare; one lock is enough.
func (r *Repository) LockSets(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('ingestion-sets'))`); err != nil {
		return fmt.Errorf("ingestion repo: lock sets: %w", err)
	}
	return nil
}

// InsertSet saves a new set inside the caller's transaction.
func (r *Repository) InsertSet(ctx context.Context, tx pgx.Tx, s Set) error {
	err := r.q.WithTx(tx).IngestionSetInsert(ctx, dbq.IngestionSetInsertParams{
		ID: s.ID, Name: s.Name, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	})
	if err != nil {
		return fmt.Errorf("ingestion repo: insert set: %w", err)
	}
	return nil
}

// TouchSet records that the set was applied or released.
func (r *Repository) TouchSet(ctx context.Context, tx pgx.Tx, setID string, at time.Time) error {
	if err := r.q.WithTx(tx).IngestionSetTouch(ctx, dbq.IngestionSetTouchParams{ID: setID, UpdatedAt: at}); err != nil {
		return fmt.Errorf("ingestion repo: touch set: %w", err)
	}
	return nil
}

// RecordApply saves an apply inside the caller's transaction, then forgets the
// applies beyond the newest MaxSetApplies, returning the digests of the
// archives to remove from the store.
func (r *Repository) RecordApply(ctx context.Context, tx pgx.Tx, a SetApply) (forgotten []string, err error) {
	summary, err := json.Marshal(a.Summary)
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: marshal apply summary: %w", err)
	}
	q := r.q.WithTx(tx)
	err = q.IngestionSetApplyInsert(ctx, dbq.IngestionSetApplyInsertParams{
		ID: a.ID, SetID: a.SetID, ArchiveUri: &a.ArchiveDigest, Summary: summary, AppliedBy: a.AppliedBy, AppliedAt: a.AppliedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: insert set apply: %w", err)
	}
	gone, err := q.IngestionSetApplyForget(ctx, dbq.IngestionSetApplyForgetParams{SetID: a.SetID, Keep: MaxSetApplies})
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: forget set applies: %w", err)
	}
	for _, digest := range gone {
		if digest != nil {
			forgotten = append(forgotten, *digest)
		}
	}
	return forgotten, nil
}

func rowToSetApply(row dbq.IngSetApply) (*SetApply, error) {
	var summary SetApplySummary
	if err := json.Unmarshal(row.Summary, &summary); err != nil {
		return nil, fmt.Errorf("ingestion repo: set apply %s summary does not parse: %w", row.ID, err)
	}
	a := &SetApply{ID: row.ID, SetID: row.SetID, Summary: summary, AppliedBy: row.AppliedBy, AppliedAt: row.AppliedAt}
	if row.ArchiveUri != nil {
		a.ArchiveDigest = *row.ArchiveUri
	}
	return a, nil
}
