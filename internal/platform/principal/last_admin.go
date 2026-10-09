package principal

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// CodeLastAllApplicationsAdmin is the BUSINESS_RULE code returned when a write
// would leave no active all-applications administrator.
const CodeLastAllApplicationsAdmin = "LAST_ALL_APPLICATIONS_ADMIN"

// lastAdminLockKey is the fixed pg_advisory_xact_lock key serialising every
// write that can reduce the set of all-applications administrators.
const lastAdminLockKey int64 = 0x464341444d494e01 // "FCADMIN" + 1

// IsAllApplicationsAdmin reports whether p is an all-applications
// administrator: an ACTIVE principal of type USER, scope (tier) ANCHOR, with
// the all-applications flag set. Service principals never count: the point of
// the role is a human who can grant all-applications access.
func (p *Principal) IsAllApplicationsAdmin() bool {
	return p.Type == TypeUser && p.Scope == ScopeAnchor && p.Active && p.AllApplications
}

// guardLastAdminOnPersist refuses a write of p that would demote the last
// all-applications administrator. It is called from Persist (every principal
// write funnels through it, whichever persister wraps the repository), so
// deactivate, update, sync, scope change and assign-application-access are all
// covered with one rule. The lock is only taken when the stored row IS an
// administrator and the incoming state is not, so unrelated principal writes
// never contend on it.
func (r *Repository) guardLastAdminOnPersist(ctx context.Context, p *Principal, tx *usecasepgx.DbTx) error {
	if p.IsAllApplicationsAdmin() {
		return nil // not a reduction
	}
	return r.assertAnotherAdmin(ctx, p.ID, "remove all-applications administrator status from", tx)
}

// guardLastAdminOnDelete is the Delete counterpart.
func (r *Repository) guardLastAdminOnDelete(ctx context.Context, p *Principal, tx *usecasepgx.DbTx) error {
	return r.assertAnotherAdmin(ctx, p.ID, "delete", tx)
}

// assertAnotherAdmin refuses when the STORED principal id is an
// all-applications administrator and no other one exists. Race-safe: after a
// cheap unlocked probe, it takes a transaction-scoped advisory lock, re-reads
// the stored row and counts the others inside the same transaction, so two
// administrators demoting each other concurrently serialise and the second is
// refused (READ COMMITTED gives the post-lock statements a fresh snapshot).
func (r *Repository) assertAnotherAdmin(ctx context.Context, id ids.PrincipalID, verb string, tx *usecasepgx.DbTx) error {
	q := tx.Inner()
	const isAdmin = `SELECT type = 'USER' AND scope = 'ANCHOR' AND active AND all_applications
	                   FROM iam_principals WHERE id = $1`
	var admin bool
	err := q.QueryRow(ctx, isAdmin, id).Scan(&admin)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("last-admin probe: %w", err)
	}
	if !admin {
		return nil
	}
	if _, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lastAdminLockKey); err != nil {
		return fmt.Errorf("last-admin lock: %w", err)
	}
	// Re-read under the lock: a concurrent writer may have demoted this row.
	if err := q.QueryRow(ctx, isAdmin, id).Scan(&admin); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("last-admin re-probe: %w", err)
	}
	if !admin {
		return nil
	}
	var others int
	if err := q.QueryRow(ctx,
		`SELECT count(*) FROM iam_principals
		  WHERE id <> $1 AND type = 'USER' AND scope = 'ANCHOR' AND active AND all_applications`,
		id).Scan(&others); err != nil {
		return fmt.Errorf("last-admin count: %w", err)
	}
	if others > 0 {
		return nil
	}
	return usecase.BusinessRule(CodeLastAllApplicationsAdmin,
		"This would "+verb+" the last active all-applications administrator, and nobody could then grant "+
			"all-applications access. Grant all-applications access to another active anchor user first.")
}
