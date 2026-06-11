// Package repocommon holds shared repository-layer helpers: the
// ErrNoRows→nil single-row convention and the positional-argument
// filter builder used by the hand-rolled dynamic list queries
// (see docs/sqlc.md for why those stay hand-written).
package repocommon

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// One adapts a single-row sqlc lookup to the repo convention: a missing
// row is (nil, nil), not an error; other errors wrap as "<label>: %w".
// The label keeps each repo's existing error text intact.
func One[R any](row R, err error, label string) (*R, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	return &row, nil
}

// Filter accumulates positional WHERE conditions and their arguments
// for a hand-rolled dynamic query. The zero value is ready to use.
type Filter struct {
	conds []string
	args  []any
}

// Eq appends `col = $n`.
func (f *Filter) Eq(col string, v any) {
	f.args = append(f.args, v)
	f.conds = append(f.conds, fmt.Sprintf("%s = $%d", col, len(f.args)))
}

// EqPtr appends `col = $n` when v is non-nil; nil means "no filter".
func (f *Filter) EqPtr(col string, v *string) {
	if v == nil {
		return
	}
	f.Eq(col, *v)
}

// Any appends `col = ANY($n)` when vs is non-empty.
func (f *Filter) Any(col string, vs []string) {
	if len(vs) == 0 {
		return
	}
	f.args = append(f.args, vs)
	f.conds = append(f.conds, fmt.Sprintf("%s = ANY($%d)", col, len(f.args)))
}

// Clause appends a custom condition; format receives the new argument's
// positional index, e.g. f.Clause("created_at >= $%d", since) or
// f.Clause("(client_id IS NULL OR client_id = ANY($%d))", ids).
func (f *Filter) Clause(format string, v any) {
	f.args = append(f.args, v)
	f.conds = append(f.conds, fmt.Sprintf(format, len(f.args)))
}

// Arg appends a bare argument outside the WHERE clause (LIMIT/OFFSET)
// and returns its positional index.
func (f *Filter) Arg(v any) int {
	f.args = append(f.args, v)
	return len(f.args)
}

// Where renders "" when no conditions were added, otherwise
// " WHERE c1 AND c2 …" (leading space included, so call sites
// concatenate it directly onto the base query).
func (f *Filter) Where() string {
	if len(f.conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(f.conds, " AND ")
}

// Args returns the accumulated positional arguments.
func (f *Filter) Args() []any { return f.args }
