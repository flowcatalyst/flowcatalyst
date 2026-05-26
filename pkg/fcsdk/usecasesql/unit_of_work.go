package usecasesql

import "database/sql"

// UnitOfWork is the database/sql-backed UnitOfWork. Each top-level
// Commit / CommitDelete / CommitAll / EmitEvent opens its own
// transaction and commits or rolls back on exit. For orchestrated
// transactions spanning multiple use cases, use Run.
type UnitOfWork struct {
	db   *sql.DB
	sink Sink
	opts *sql.TxOptions
}

// Option configures a UnitOfWork.
type Option func(*UnitOfWork)

// WithTxOptions sets the transaction options (isolation level,
// read-only flag) used when opening each new transaction. Defaults to
// nil — driver defaults.
func WithTxOptions(opts *sql.TxOptions) Option {
	return func(u *UnitOfWork) { u.opts = opts }
}

// New wires a UnitOfWork against an existing *sql.DB and a sink.
func New(db *sql.DB, sink Sink, opts ...Option) *UnitOfWork {
	u := &UnitOfWork{db: db, sink: sink}
	for _, opt := range opts {
		opt(u)
	}
	return u
}

// DB exposes the underlying *sql.DB for read-only queries outside use
// cases. Writes must still go through Commit / CommitDelete /
// CommitAll / EmitEvent.
func (u *UnitOfWork) DB() *sql.DB { return u.db }

// Sink returns the configured sink. Useful for testing.
func (u *UnitOfWork) Sink() Sink { return u.sink }
