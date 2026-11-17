package operations

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// SetRolloutCommand changes the share of a branch's phones an update reaches.
type SetRolloutCommand struct {
	UpdateID       string `json:"updateId"`
	RolloutPercent int    `json:"rolloutPercent"`
}

// SetRollout changes the rollout of the newest update of its branch (only the
// newest row of a branch may be partial: 409 NOT_LATEST_UPDATE otherwise).
// Setting the percentage it already has changes nothing and records nothing.
// Lowering it does not take the update off phones that have it.
func SetRollout(repo *appupdate.Repository) usecaseop.TxOperation[SetRolloutCommand, *appupdate.Update] {
	return usecaseop.TxOperation[SetRolloutCommand, *appupdate.Update]{
		Name:     "SetAppUpdateRollout",
		Validate: func(_ context.Context, cmd SetRolloutCommand) error { return ValidateRollout(cmd.RolloutPercent) },
		Authorize: func(ctx context.Context, cmd SetRolloutCommand) error {
			_, err := ReachUpdate(ctx, repo, cmd.UpdateID, false)
			return err
		},
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd SetRolloutCommand, ec usecase.ExecutionContext) (*appupdate.Update, error) {
			u, err := ReachUpdate(ctx, repo, cmd.UpdateID, false)
			if err != nil {
				return nil, err
			}
			var head []appupdate.Update
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				if err := repo.LockBranch(ctx, tx, u.Branch); err != nil {
					return err
				}
				head, err = repo.HeadTx(ctx, tx, u.Branch, 1)
				return err
			}); err != nil {
				return nil, usecase.Internal("REPO", "read branch failed", err)
			}
			if u.Kind != appupdate.KindUpdate {
				return nil, usecase.BusinessRule("NOT_AN_UPDATE", "a rollback to the embedded bundle has no rollout")
			}
			if len(head) == 0 || head[0].ID != u.ID {
				return nil, usecase.Conflict("NOT_LATEST_UPDATE", "only the newest update of a branch has a rollout that can change")
			}
			cur := head[0]
			if cur.RolloutPercent == cmd.RolloutPercent {
				return &cur, nil
			}
			prev := cur.RolloutPercent
			cur.RolloutPercent = cmd.RolloutPercent
			cur.UpdatedAt = time.Now().UTC()
			event := RolloutChanged{
				eventBase: newBase(ec, RolloutChangedType, &cur), BranchData: branchData(&cur),
				PreviousPercent: prev, RolloutPercent: cmd.RolloutPercent,
			}
			if r := usecasepgx.CommitScoped(ctx, s, &cur, repo, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return nil, e
			}
			return &cur, nil
		},
	}
}
