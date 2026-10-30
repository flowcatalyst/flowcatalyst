package operations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// PruneCommand is the input DTO for pruning a function's versions.
type PruneCommand struct {
	FunctionID string `json:"functionId"`
	// Keep is how many untagged versions to keep; must be >= 1. Callers
	// resolve their default (FC_FUNCTIONS_KEEP_VERSIONS) before calling.
	Keep int `json:"keep"`
	// DryRun reports what would be pruned and changes nothing.
	DryRun bool `json:"dryRun,omitempty"`
}

// PrunedVersion identifies one version pruned (or, on a dry run, that would be).
type PrunedVersion struct {
	ID     string
	Number int32
	Digest string
}

// PruneResult is what pruning did.
type PruneResult struct {
	Function function.Function
	Keep     int
	DryRun   bool
	// Untagged is the number of untagged versions before pruning.
	Untagged int
	// Pruned is the versions deleted (would-be on a dry run), oldest first.
	Pruned []PrunedVersion
	// ReleasedDigests are digests of pruned versions that no remaining
	// version of ANY function references: their artifacts can be deleted
	// from the store (see [DeleteReleasedArtifacts]). Empty on a dry run.
	ReleasedDigests []string
}

// PruneVersions deletes a function's oldest untagged versions beyond keep
// (selection and protections: [function.SelectPrunable]). It runs in its own
// transaction holding the function row lock — the one publish and alias
// writes also take — so concurrent prunes, publishes and promotes are
// serialised: a second prune finds nothing left, and a promote never points
// an alias at a version this deleted. Idempotent. Emits [VersionPruned] per
// deleted version. Artifact objects are NOT touched here: the store is not
// transactional, so the caller removes them best-effort after commit with
// [DeleteReleasedArtifacts].
func PruneVersions(repo *function.Repository) usecaseop.TxOperation[PruneCommand, PruneResult] {
	return usecaseop.TxOperation[PruneCommand, PruneResult]{
		Name: "PruneFunctionVersions",
		Validate: func(_ context.Context, cmd PruneCommand) error {
			if strings.TrimSpace(cmd.FunctionID) == "" {
				return usecase.Validation("FUNCTION_ID_REQUIRED", "functionId is required")
			}
			if cmd.Keep < 1 {
				return usecase.Validation("KEEP_INVALID", "keep must be >= 1")
			}
			return nil
		},
		Authorize: usecaseop.Public[PruneCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd PruneCommand, ec usecase.ExecutionContext) (PruneResult, error) {
			var zero PruneResult

			f, err := repo.FindByID(ctx, cmd.FunctionID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if f == nil {
				return zero, httperror.NotFound("Function", cmd.FunctionID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), f.ClientID); err != nil {
				return zero, err
			}

			res := PruneResult{Function: *f, Keep: cmd.Keep, DryRun: cmd.DryRun}
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				ok, lockErr := repo.LockFunctionTx(ctx, tx, f.ID)
				if lockErr != nil {
					return lockErr
				}
				if !ok {
					return errFunctionGone
				}
				// Read AFTER the lock: the state a concurrent publish or
				// promote left behind, not a stale snapshot.
				versions, listErr := repo.ListVersionsTx(ctx, tx, f.ID)
				if listErr != nil {
					return listErr
				}
				aliased, aliasErr := repo.AliasedVersionIDsTx(ctx, tx, f.ID)
				if aliasErr != nil {
					return aliasErr
				}
				for _, v := range versions {
					if !aliased[v.ID] {
						res.Untagged++
					}
				}
				picked := function.SelectPrunable(versions, aliased, cmd.Keep, time.Now().UTC())
				if cmd.DryRun {
					for _, v := range picked {
						res.Pruned = append(res.Pruned, PrunedVersion{ID: v.ID, Number: v.Number, Digest: v.Digest})
					}
					return nil
				}
				anyPublished := false
				for _, v := range picked {
					deleted, delErr := repo.DeleteUnaliasedVersionTx(ctx, tx, v.ID)
					if delErr != nil {
						return delErr
					}
					if !deleted {
						continue // gained an alias since we looked: tagged now, keep it
					}
					anyPublished = anyPublished || v.Status == function.VersionPublished
					res.Pruned = append(res.Pruned, PrunedVersion{ID: v.ID, Number: v.Number, Digest: v.Digest})
				}
				for _, p := range res.Pruned {
					referenced, refErr := repo.DigestReferencedTx(ctx, tx, p.Digest)
					if refErr != nil {
						return refErr
					}
					if !referenced {
						res.ReleasedDigests = append(res.ReleasedDigests, p.Digest)
					}
				}
				if anyPublished {
					// A PUBLISHED version is a runner-visible candidate; its
					// removal changes the desired document.
					_, bumpErr := repo.BumpPoolRevision(ctx, usecasepgx.WrapTxForBootstrap(tx), f.RunnerPool())
					return bumpErr
				}
				return nil
			}); err != nil {
				if errors.Is(err, errFunctionGone) {
					return zero, httperror.NotFound("Function", cmd.FunctionID)
				}
				return zero, usecase.Internal("REPO", "prune write failed", err)
			}

			if !cmd.DryRun {
				for _, p := range res.Pruned {
					event := VersionPruned{
						Metadata:   usecase.NewEventMetadata(ec, VersionPrunedType, AdminSource, subjectFor(f.ID)),
						FunctionID: f.ID,
						Address:    f.Address,
						VersionID:  p.ID,
						Version:    p.Number,
						Digest:     p.Digest,
					}
					if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
						_, e := usecase.Into(r)
						return zero, e
					}
				}
			}
			return res, nil
		},
	}
}

var errFunctionGone = errors.New("function no longer exists")

// DeleteReleasedArtifacts removes the stored artifacts of digests a prune
// released, best-effort and AFTER the prune committed: a failure is logged and
// skipped (it leaves a harmless orphan object in the store, never a failed
// publish or a surviving version row). Each digest is re-checked against the
// database immediately before deleting, so an artifact another function (or a
// concurrent publish) has taken a reference to in the meantime is kept.
// Returns the digests actually deleted.
func DeleteReleasedArtifacts(ctx context.Context, repo *function.Repository, store artifact.Store, digests []string) []string {
	var deleted []string
	for _, d := range digests {
		referenced, err := repo.DigestReferenced(ctx, d)
		if err != nil {
			slog.Error("function prune: artifact reference check failed; leaving the artifact", "digest", d, "err", err)
			continue
		}
		if referenced {
			continue
		}
		if err := store.Delete(ctx, d); err != nil {
			slog.Error("function prune: artifact delete failed; orphan left in the store", "digest", d, "err", err)
			continue
		}
		deleted = append(deleted, d)
	}
	return deleted
}

// PruneAfterPublish is the post-publish hook: it prunes functionID down to
// keep untagged versions and removes released artifacts. It never fails the
// caller — errors are logged — because the publish it follows has already
// committed. The returned result is empty when nothing was pruned or the prune
// failed. keep < 1 (an unconfigured caller) uses [function.DefaultKeepVersions].
func PruneAfterPublish(ctx context.Context, uow *usecasepgx.UnitOfWork, repo *function.Repository, store artifact.Store, functionID string, keep int, ec usecase.ExecutionContext) PruneResult {
	if keep < 1 {
		keep = function.DefaultKeepVersions
	}
	res, err := usecaseop.RunTx(ctx, uow, PruneVersions(repo), PruneCommand{FunctionID: functionID, Keep: keep}, ec)
	if err != nil {
		slog.Error("function prune after publish failed", "functionId", functionID, "err", err)
		return PruneResult{}
	}
	if len(res.Pruned) > 0 {
		slog.Info("function versions pruned", "address", res.Function.Address, "pruned", len(res.Pruned), "keep", keep,
			"versions", prunedNumbers(res.Pruned))
	}
	DeleteReleasedArtifacts(ctx, repo, store, res.ReleasedDigests)
	return res
}

func prunedNumbers(ps []PrunedVersion) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = fmt.Sprint(p.Number)
	}
	return strings.Join(parts, ",")
}
