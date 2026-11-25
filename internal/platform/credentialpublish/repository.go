package credentialpublish

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Publication is an owner's publishing setting.
type Publication struct {
	Owner     Owner
	Store     string
	BasePath  string
	EnabledAt time.Time
	EnabledBy string
}

// Item is one credential's publishing state, as the console shows it.
type Item struct {
	Credential, Path, State string
	Attempts                int
	NextAttemptAt           *time.Time
	PublishedAt             *time.Time
	LastError               string
	UpdatedAt               time.Time
}

// Repository reads publishing state and is the worker's claim queue. The
// writes of an operation go through QueueInTx and friends, in its transaction.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository wires a repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// Status is the owner's publication (nil when off) and its items, ordered by
// credential. Items still being removed after a disable or delete come back
// with a nil publication, so the page can say "removing".
func (r *Repository) Status(ctx context.Context, owner Owner) (*Publication, []Item, error) {
	var pub *Publication
	var by *string
	p := Publication{Owner: owner}
	err := r.pool.QueryRow(ctx,
		`SELECT store, base_path, enabled_at, enabled_by FROM iam_credential_publications
		  WHERE owner_type = $1 AND owner_id = $2`, string(owner.Type), owner.ID).
		Scan(&p.Store, &p.BasePath, &p.EnabledAt, &by)
	switch {
	case err == nil:
		if by != nil {
			p.EnabledBy = *by
		}
		pub = &p
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, nil, fmt.Errorf("credential publication: %w", err)
	}
	rows, err := r.pool.Query(ctx,
		`SELECT credential, path, state, attempts, next_attempt_at, published_at, last_error, updated_at
		   FROM iam_credential_publication_items
		  WHERE owner_type = $1 AND owner_id = $2 ORDER BY credential`, string(owner.Type), owner.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("credential publication: %w", err)
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		var it Item
		var lastErr *string
		if err := rows.Scan(&it.Credential, &it.Path, &it.State, &it.Attempts,
			&it.NextAttemptAt, &it.PublishedAt, &lastErr, &it.UpdatedAt); err != nil {
			return nil, nil, fmt.Errorf("credential publication: %w", err)
		}
		if lastErr != nil {
			it.LastError = *lastErr
		}
		items = append(items, it)
	}
	return pub, items, wrap(rows.Err())
}

// LinkedServiceAccount is the service account row id an OAuth client
// publishes with, "" when it is stand-alone.
func (r *Repository) LinkedServiceAccount(ctx context.Context, oauthClientRowID string) (string, error) {
	return linkedServiceAccount(ctx, r.pool, oauthClientRowID)
}

// RetryNow makes the owner's waiting items due now. Attempts are kept, so the
// page still shows how long it has been failing.
func (r *Repository) RetryNow(ctx context.Context, owner Owner) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE iam_credential_publication_items SET next_attempt_at = now()
		  WHERE owner_type = $1 AND owner_id = $2 AND state IN ('PENDING_PUT', 'PENDING_DELETE')`,
		string(owner.Type), owner.ID)
	return wrap(err)
}

// claimed is an item the worker holds a lease on.
type claimed struct {
	owner        Owner
	credential   string
	path         string
	state        string
	pendingValue *string
	attempts     int
	generation   int64
}

// claim leases the next due item: it moves next_attempt_at forward by lease
// in one short transaction, so the AWS call runs with no transaction open and
// a replica that takes over after a crash finds the item again once the lease
// lapses. nil when nothing is due.
func (r *Repository) claim(ctx context.Context, now time.Time, lease time.Duration) (*claimed, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, wrap(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var c claimed
	var ot string
	err = tx.QueryRow(ctx,
		`SELECT owner_type, owner_id, credential, path, state, pending_value, attempts, generation
		   FROM iam_credential_publication_items
		  WHERE state IN ('PENDING_PUT', 'PENDING_DELETE') AND next_attempt_at <= $1
		  ORDER BY next_attempt_at
		  LIMIT 1
		    FOR UPDATE SKIP LOCKED`, now).
		Scan(&ot, &c.owner.ID, &c.credential, &c.path, &c.state, &c.pendingValue, &c.attempts, &c.generation)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, wrap(err)
	}
	c.owner.Type = OwnerType(ot)
	if _, err := tx.Exec(ctx,
		`UPDATE iam_credential_publication_items SET next_attempt_at = $4
		  WHERE owner_type = $1 AND owner_id = $2 AND credential = $3`,
		ot, c.owner.ID, c.credential, now.Add(lease)); err != nil {
		return nil, wrap(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, wrap(err)
	}
	return &c, nil
}

// Each outcome applies only while the item is still the generation that was
// claimed: an operation that re-queued it meanwhile (a regenerate, a disable)
// has the newer say, and its item is due now.

func (r *Repository) recordPut(ctx context.Context, c *claimed, now time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE iam_credential_publication_items
		    SET state = 'PUBLISHED', published_at = $5, pending_value = NULL, last_error = NULL,
		        attempts = 0, next_attempt_at = NULL, updated_at = $5
		  WHERE owner_type = $1 AND owner_id = $2 AND credential = $3 AND generation = $4`,
		string(c.owner.Type), c.owner.ID, c.credential, c.generation, now)
	return wrap(err)
}

func (r *Repository) recordDeleted(ctx context.Context, c *claimed) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM iam_credential_publication_items
		  WHERE owner_type = $1 AND owner_id = $2 AND credential = $3 AND generation = $4`,
		string(c.owner.Type), c.owner.ID, c.credential, c.generation)
	return wrap(err)
}

// recordGone turns a put whose credential no longer exists into a delete.
func (r *Repository) recordGone(ctx context.Context, c *claimed, now time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE iam_credential_publication_items
		    SET state = 'PENDING_DELETE', pending_value = NULL, attempts = 0, last_error = NULL,
		        next_attempt_at = $5, generation = generation + 1, updated_at = $5
		  WHERE owner_type = $1 AND owner_id = $2 AND credential = $3 AND generation = $4`,
		string(c.owner.Type), c.owner.ID, c.credential, c.generation, now)
	return wrap(err)
}

func (r *Repository) recordFailure(ctx context.Context, c *claimed, text string, now time.Time) error {
	attempts := c.attempts + 1
	_, err := r.pool.Exec(ctx,
		`UPDATE iam_credential_publication_items
		    SET attempts = $5, last_error = $6, next_attempt_at = $7, updated_at = $8
		  WHERE owner_type = $1 AND owner_id = $2 AND credential = $3 AND generation = $4`,
		string(c.owner.Type), c.owner.ID, c.credential, c.generation,
		attempts, text, now.Add(backoff(attempts)), now)
	return wrap(err)
}

// pending is the number of items waiting to be written or deleted.
func (r *Repository) pending(ctx context.Context) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM iam_credential_publication_items WHERE state IN ('PENDING_PUT', 'PENDING_DELETE')`).Scan(&n)
	return n, wrap(err)
}

// backoff is the wait after the nth consecutive failure: 30 s, doubling, up to
// an hour.
func backoff(attempts int) time.Duration {
	d := 30 * time.Second
	for i := 1; i < attempts && d < time.Hour; i++ {
		d *= 2
	}
	return min(d, time.Hour)
}
