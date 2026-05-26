// Package usecasepgx is the pgx-backed UnitOfWork for FlowCatalyst use
// cases. It wraps a pgxpool.Pool and a usecasepgx.Sink (which decides
// where domain events and audit logs are written — typically
// outboxpgx.Sink for consumer apps, or a platform-specific sink that
// writes to msg_events / aud_logs directly).
//
// Use cases call the generic free functions Commit, CommitDelete,
// CommitAll, and EmitEvent. Those are the only paths to a Success-valued
// usecase.Result outside this SDK.
package usecasepgx

import "github.com/jackc/pgx/v5"

// DbTx is an opaque write handle passed to repository Persist methods.
// Wraps a pgx.Tx so repository code doesn't import pgx directly through
// some unrelated path; a future driver swap touches this file plus the
// commit.go file, nothing else.
//
// Repositories access the underlying pgx.Tx via Inner().
type DbTx struct {
	inner pgx.Tx
}

// Inner exposes the underlying pgx.Tx. Repository methods call this to
// execute SQL.
func (t *DbTx) Inner() pgx.Tx { return t.inner }

// newDbTx is internal to the SDK; only commit.go / run.go construct one.
func newDbTx(tx pgx.Tx) *DbTx { return &DbTx{inner: tx} }

// WrapTxForBootstrap exposes a DbTx around an externally-managed pgx.Tx
// for infrastructure-bootstrap callers ONLY (init commands, seeders,
// admin tools). These paths run outside the use-case envelope: no
// executing principal, no domain events emitted. Production use cases
// MUST go through Commit/CommitDelete/CommitAll/EmitEvent — those are
// the only entry points that preserve the sealed-event guarantee.
//
// If you find yourself reaching for this from a production code path,
// you almost certainly want a real use case instead.
func WrapTxForBootstrap(tx pgx.Tx) *DbTx { return newDbTx(tx) }
