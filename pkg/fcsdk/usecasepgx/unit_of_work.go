package usecasepgx

import "github.com/jackc/pgx/v5/pgxpool"

// UnitOfWork is the pgx-backed UnitOfWork. Each top-level Commit /
// CommitDelete / CommitAll / EmitEvent opens its own transaction and
// commits or rolls back on exit. For orchestrated transactions that
// span multiple use cases (or mix use cases with ad-hoc writes), use
// Run.
//
// Construct with New, then pass the *UnitOfWork into every use case
// that needs to commit. The Sink wired here decides where domain events
// and audit logs land.
type UnitOfWork struct {
	pool *pgxpool.Pool
	sink Sink
}

// New wires a UnitOfWork against an existing pgx pool and a sink.
func New(pool *pgxpool.Pool, sink Sink) *UnitOfWork {
	return &UnitOfWork{pool: pool, sink: sink}
}

// Pool exposes the underlying pgx pool for callers that need to run
// read-only queries outside a use case (e.g. handlers loading data
// for GET endpoints). Writes must still go through Commit / CommitDelete
// / CommitAll / EmitEvent.
func (u *UnitOfWork) Pool() *pgxpool.Pool { return u.pool }

// Sink returns the configured sink. Useful for testing.
func (u *UnitOfWork) Sink() Sink { return u.sink }
