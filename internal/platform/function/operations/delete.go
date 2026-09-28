package operations

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	dispatchpoolops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteFunctionResult is what the API returns. OrphanedDigests are the
// artifact digests DeleteFunction determined (inside the same transaction
// that removed the version rows referencing them) are no longer referenced
// by any OTHER function's version — the caller (api.go) deletes them from
// the artifact store AFTER this transaction has committed, best-effort: a
// failed store delete is logged, never re-surfaced as a failed function
// delete (the DB rows are already gone; the digest is simply reclaimed
// later by the same idempotent Delete call, or left as a harmless orphan).
type DeleteFunctionResult struct {
	Event           FunctionDeleted
	OrphanedDigests []string
}

// DeleteFunction removes a function: unwires it exactly as DeleteAlias(live)
// would (every function-owned subscription and scheduled job), additionally
// deletes the function's OWN dispatch pool (plan §8.5: "deleted only when
// the function is deleted" — the one case reconcileWiring never handles
// itself), computes which of its versions' artifact digests become
// orphaned, and deletes the function row — cascading to its versions,
// aliases, and settings via the fn_versions/fn_aliases/fn_settings FK ON
// DELETE CASCADE (migration 059). All in one transaction; [FunctionDeleted]
// is emitted atomically with it.
func DeleteFunction(repo *function.Repository, wiring WiringDeps) usecaseop.TxOperation[DeleteCommand, DeleteFunctionResult] {
	return usecaseop.TxOperation[DeleteCommand, DeleteFunctionResult]{
		Name: "DeleteFunction",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Per-resource authz runs post-load in Execute; the coarse "may
		// manage functions" permission is on the controller.
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd DeleteCommand, ec usecase.ExecutionContext) (DeleteFunctionResult, error) {
			var zero DeleteFunctionResult

			f, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if f == nil {
				return zero, httperror.NotFound("Function", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), f.ClientID); err != nil {
				return zero, err
			}

			// Unwire: delete every function-owned subscription/scheduled job
			// (d == nil is reconcileWiring's "delete everything" path — same one
			// DeleteAlias(live) uses).
			if _, err := reconcileWiring(ctx, s, wiring, f, nil, ec); err != nil {
				return zero, err
			}

			// The dispatch pool is the one wiring object reconcileWiring never
			// deletes itself (plan §8.5) — only a function delete removes it.
			if err := deleteDispatchPoolIfPresent(ctx, s, wiring, f, ec); err != nil {
				return zero, err
			}

			// Determine which of this function's version digests are about to
			// become orphaned: a digest still referenced by some OTHER
			// function's version stays in the store. Computed inside this
			// transaction so the reference count is consistent with what's
			// about to be deleted.
			versions, err := repo.ListVersionsByFunction(ctx, f.ID)
			if err != nil {
				return zero, usecase.Internal("REPO", "list_versions_by_function failed", err)
			}
			seen := make(map[string]bool, len(versions))
			var orphaned []string
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				for _, v := range versions {
					if seen[v.Digest] {
						continue
					}
					seen[v.Digest] = true
					var otherCount int
					if qerr := tx.QueryRow(ctx,
						`SELECT COUNT(*) FROM fn_versions WHERE digest = $1 AND function_id != $2`,
						v.Digest, f.ID).Scan(&otherCount); qerr != nil {
						return qerr
					}
					if otherCount == 0 {
						orphaned = append(orphaned, v.Digest)
					}
				}
				return nil
			}); err != nil {
				return zero, usecase.Internal("REPO", "orphan digest check failed", err)
			}

			event := FunctionDeleted{
				Metadata:   usecase.NewEventMetadata(ec, FunctionDeletedType, Source, subjectFor(f.ID)),
				FunctionID: f.ID,
				Address:    f.Address,
			}
			if r := usecasepgx.CommitDeleteScoped(ctx, s, f, repo, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}

			return DeleteFunctionResult{Event: event, OrphanedDigests: orphaned}, nil
		},
	}
}

// deleteDispatchPoolIfPresent removes the function's own dispatch pool
// (found by DispatchPoolCode, scoped to the function's client)
// if one exists. Reuses dispatchpool's own event type for consistency with
// pools deleted through the ordinary API.
func deleteDispatchPoolIfPresent(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, wiring WiringDeps, f *function.Function, ec usecase.ExecutionContext) error {
	code := f.DispatchPoolCode()
	p, err := wiring.DispatchPools.FindByCode(ctx, code, f.ClientID)
	if err != nil {
		return usecase.Internal("REPO", "find dispatch pool by code failed", err)
	}
	if p == nil {
		return nil
	}
	event := dispatchpoolops.DispatchPoolDeleted{
		Metadata: usecase.NewEventMetadata(ec, dispatchpoolops.DispatchPoolDeletedType, dispatchpoolops.Source, "platform.dispatchpool."+p.ID),
		PoolID:   p.ID,
		Code:     p.Code,
	}
	if r := usecasepgx.CommitDeleteScoped(ctx, s, p, wiring.DispatchPools, event, wiringCommand{FunctionID: f.ID, Reason: "function delete"}); !usecase.IsSuccess(r) {
		_, e := usecase.Into(r)
		return e
	}
	return nil
}
