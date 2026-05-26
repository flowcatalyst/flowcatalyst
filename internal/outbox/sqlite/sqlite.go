// Package sqlite is the SQLite-backed outbox repository.
//
// Phase 4 ships the schema + claim/mark stubs structured the same way
// as the Postgres backend. Wiring against a real `database/sql` +
// modernc.org/sqlite driver is a focused follow-up — see the TODO
// inside ClaimPending.
//
// The team's pattern: when fc-dev needs an embedded SQLite outbox for
// local-dev consumer apps, swap the registry factory to this package's
// New(). For prod, stick with the Postgres backend.
package sqlite

import (
	"context"
	"errors"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/outbox"
)

// Repository is the SQLite-backed outbox repository (skeleton).
type Repository struct {
	// db *sql.DB — wired in the follow-up alongside modernc.org/sqlite.
}

// New wires a SQLite repository.
//
// TODO(phase-4-follow-up): take a *sql.DB, wire ClaimPending and friends
// against it. Use INSERT OR REPLACE for upsert; SQLite has no UPSERT
// with FOR UPDATE SKIP LOCKED — instead, use a BEGIN IMMEDIATE
// transaction and UPDATE/RETURNING to claim. Single-writer model.
func New() *Repository { return &Repository{} }

// CreateOutboxTableSQL is the SQLite schema. Column names match the
// Postgres outbox; types are SQLite equivalents (INTEGER for status,
// TEXT for everything else, datetime stored as ISO8601 strings).
const CreateOutboxTableSQL = `
CREATE TABLE IF NOT EXISTS outbox_messages (
    id            TEXT PRIMARY KEY,
    item_type     TEXT NOT NULL,
    message_group TEXT,
    payload       TEXT NOT NULL,
    status        INTEGER NOT NULL DEFAULT 0,
    status_message TEXT,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_retry_at DATETIME,
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS outbox_messages_pending_idx
    ON outbox_messages (status, next_retry_at);
`

// InitSchema creates the outbox table.
//
// TODO(phase-4-follow-up): execute CreateOutboxTableSQL against the
// configured *sql.DB.
func (*Repository) InitSchema(_ context.Context) error {
	return errors.New("sqlite outbox: InitSchema wired in phase 4 follow-up")
}

// ClaimPending is the SQLite claim path. Single-writer transaction with
// UPDATE … RETURNING (SQLite >= 3.35).
//
// TODO(phase-4-follow-up).
func (*Repository) ClaimPending(_ context.Context, _ int) ([]outbox.Item, error) {
	return nil, errors.New("sqlite outbox: ClaimPending wired in phase 4 follow-up")
}

// MarkSuccess flips items to SUCCESS.
func (*Repository) MarkSuccess(_ context.Context, _ []string) error {
	return errors.New("sqlite outbox: MarkSuccess wired in phase 4 follow-up")
}

// MarkFailed flips items with the supplied failed status + retry time.
func (*Repository) MarkFailed(_ context.Context, _ []string, _ common.OutboxStatus, _ string, _ time.Time) error {
	return errors.New("sqlite outbox: MarkFailed wired in phase 4 follow-up")
}

// Healthy pings the DB.
func (*Repository) Healthy(_ context.Context) bool { return false }
