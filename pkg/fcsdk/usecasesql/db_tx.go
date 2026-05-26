// Package usecasesql is the database/sql-backed UnitOfWork for
// FlowCatalyst use cases. Works against both Postgres (via lib/pq,
// pgx/stdlib, etc.) and MySQL (via go-sql-driver/mysql) — anything that
// implements the standard library *sql.DB / *sql.Tx contract.
//
// Same generic free-function shape as usecasepgx so use case bodies
// look identical; the only difference is the package import and the
// DbTx wrapper.
package usecasesql

import "database/sql"

// DbTx is an opaque write handle passed to repository Persist methods.
// Wraps a *sql.Tx so repository code doesn't import database/sql
// directly through some unrelated path; a future driver swap touches
// this file plus commit.go, nothing else.
//
// Repositories access the underlying *sql.Tx via Inner().
type DbTx struct {
	inner *sql.Tx
}

// Inner exposes the underlying *sql.Tx. Repository methods call this
// to execute SQL via ExecContext / QueryContext / QueryRowContext.
func (t *DbTx) Inner() *sql.Tx { return t.inner }

// newDbTx is internal to the SDK; only commit.go / run.go construct one.
func newDbTx(tx *sql.Tx) *DbTx { return &DbTx{inner: tx} }
