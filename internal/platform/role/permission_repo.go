package role

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// PermissionRepo is the Postgres-backed catalog of permission
// definitions. Mirrors Rust crates/fc-platform/src/role/repository.rs'
// permission catalog half. The Permission rows live in iam_permissions;
// individual roles still grant permissions via iam_role_permissions
// (handled by Repository).
type PermissionRepo struct{ q *dbq.Queries }

// NewPermissionRepo wires the catalog repo.
func NewPermissionRepo(pool *pgxpool.Pool) *PermissionRepo {
	return &PermissionRepo{q: dbq.New(pool)}
}

// FindAll returns every catalog row, ordered by code.
func (r *PermissionRepo) FindAll(ctx context.Context) ([]Permission, error) {
	rows, err := r.q.PermissionFindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("permission_find_all: %w", err)
	}
	out := make([]Permission, 0, len(rows))
	for _, row := range rows {
		out = append(out, permissionFromRow(row))
	}
	return out, nil
}

// FindByCode loads one catalog row or returns nil if absent.
func (r *PermissionRepo) FindByCode(ctx context.Context, code string) (*Permission, error) {
	row, err := r.q.PermissionFindByCode(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p := permissionFromRow(row)
	return &p, nil
}

// DeleteByCode removes a catalog row. Idempotent — succeeds even when
// the code wasn't present.
func (r *PermissionRepo) DeleteByCode(ctx context.Context, code string) error {
	return r.q.PermissionDeleteByCode(ctx, code)
}

// Upsert writes a permission definition into the catalogue (idempotent by
// code). The code is the canonical four-segment string
// "application:context:aggregate:action"; subdomain carries the application
// segment so the row round-trips through permissionFromRow's Category. Unlike
// permissions that exist only as strings attached to roles, catalogue rows
// persist independently of any role and survive SDK role re-syncs.
func (r *PermissionRepo) Upsert(ctx context.Context, p Permission) error {
	parts := strings.Split(p.Permission, ":")
	if len(parts) != 4 {
		return fmt.Errorf("permission upsert: malformed code %q (want application:context:aggregate:action)", p.Permission)
	}
	return r.q.PermissionUpsert(ctx, dbq.PermissionUpsertParams{
		ID:          tsid.Generate(tsid.Permission),
		Code:        p.Permission,
		Subdomain:   parts[0],
		Context:     parts[1],
		Aggregate:   parts[2],
		Action:      parts[3],
		Description: p.Description,
	})
}

func permissionFromRow(row dbq.IamPermission) Permission {
	out := Permission{
		Permission: row.Code,
		Name:       row.Code, // catalog has no display name; Rust falls back to code
	}
	if row.Description != nil {
		out.Description = row.Description
	}
	// Construct a Category from the subdomain/context/aggregate triple,
	// mirroring how Rust groups permissions in the filter UI.
	cat := row.Subdomain + ":" + row.Context + ":" + row.Aggregate
	out.Category = &cat
	return out
}
