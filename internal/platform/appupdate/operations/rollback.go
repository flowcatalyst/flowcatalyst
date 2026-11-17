package operations

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Rollback targets.
const (
	// RollbackToPrevious makes the update before the branch's current one live again.
	RollbackToPrevious = "PREVIOUS"
	// RollbackToEmbedded tells the branch's phones to run their embedded bundle.
	RollbackToEmbedded = "EMBEDDED"
)

// RollbackCommand rolls a branch back.
type RollbackCommand struct {
	ApplicationCode string  `json:"applicationCode"`
	AppKey          string  `json:"appKey"`
	Platform        string  `json:"platform"`
	RuntimeVersion  string  `json:"runtimeVersion"`
	Channel         string  `json:"channel"`
	To              string  `json:"to"`
	Message         *string `json:"message,omitempty"`
}

// RollbackResult is the branch's new newest row and whether this call wrote it
// (a rollback to the embedded bundle of a branch already rolled back to it
// writes nothing).
type RollbackResult struct {
	Update  *appupdate.Update
	Created bool
}

// Rollback rolls a branch back, always to every phone (rollout 100):
//
//   - PREVIOUS republishes the newest update older than the branch's current
//     one (see Republish for why it is a new update). The branch's newest row
//     must be an update (409 NOTHING_TO_ROLL_BACK when it is a rollback to the
//     embedded bundle: republish an update by id instead) with an update
//     before it (409 NO_PREVIOUS_UPDATE: roll back to the embedded bundle).
//   - EMBEDDED writes a rollback row: phones get a signed rollBackToEmbedded
//     directive, whose commitTime is the row's, until something newer is
//     published.
//
// Either supersedes a partial rollout in progress.
func Rollback(repo *appupdate.Repository, apps AppLookup) usecaseop.TxOperation[RollbackCommand, RollbackResult] {
	return usecaseop.TxOperation[RollbackCommand, RollbackResult]{
		Name: "RollbackAppUpdates",
		Validate: func(_ context.Context, cmd RollbackCommand) error {
			if err := ValidateBranch(cmd.ApplicationCode, cmd.AppKey, cmd.Platform, cmd.RuntimeVersion, cmd.Channel); err != nil {
				return err
			}
			if cmd.To != RollbackToPrevious && cmd.To != RollbackToEmbedded {
				return usecase.Validation("INVALID_ROLLBACK_TARGET", `to must be "PREVIOUS" or "EMBEDDED"`)
			}
			return ValidateMessage(cmd.Message, nil)
		},
		Authorize: func(ctx context.Context, cmd RollbackCommand) error {
			_, err := ReachApplication(ctx, apps, cmd.ApplicationCode)
			return err
		},
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd RollbackCommand, ec usecase.ExecutionContext) (RollbackResult, error) {
			var zero RollbackResult
			app, err := ReachApplication(ctx, apps, cmd.ApplicationCode)
			if err != nil {
				return zero, err
			}
			branch := appupdate.Branch{
				ApplicationID: string(app.ID), AppKey: cmd.AppKey, Platform: cmd.Platform,
				RuntimeVersion: cmd.RuntimeVersion, Channel: cmd.Channel,
			}
			head, err := lockAndHead(ctx, s, repo, branch)
			if err != nil {
				return zero, err
			}

			if cmd.To == RollbackToEmbedded {
				if len(head) > 0 && head[0].Kind == appupdate.KindRollbackToEmbedded {
					return RollbackResult{Update: &head[0]}, nil
				}
				now := time.Now().UTC()
				var prev time.Time
				if len(head) > 0 {
					prev = head[0].CreatedAt
				}
				u := &appupdate.Update{
					ID: tsid.Generate(tsid.AppUpdate), ApplicationID: branch.ApplicationID, ApplicationCode: app.Code,
					Branch: branch, Kind: appupdate.KindRollbackToEmbedded,
					CreatedAt: appupdate.NextCommitTime(now, prev), RolloutPercent: 100,
					Message: cmd.Message, PublishedBy: ec.PrincipalID, UpdatedAt: now,
				}
				event := RolledBackToEmbedded{
					eventBase: newBase(ec, RolledBackToEmbeddedType, u), BranchData: branchData(u),
					CommitTime: appupdate.FormatTime(u.CreatedAt), Message: cmd.Message,
				}
				if r := usecasepgx.CommitScoped(ctx, s, u, repo, event, cmd); !usecase.IsSuccess(r) {
					_, e := usecase.Into(r)
					return zero, e
				}
				return RollbackResult{Update: u, Created: true}, nil
			}

			if len(head) == 0 {
				return zero, usecase.Conflict("NOTHING_TO_ROLL_BACK", "the branch has no updates")
			}
			if head[0].Kind != appupdate.KindUpdate {
				return zero, usecase.Conflict("NOTHING_TO_ROLL_BACK",
					"the branch is rolled back to the embedded bundle; republish an update by id to make it live again")
			}
			var prevUpdate *appupdate.Update
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				var err error
				prevUpdate, err = repo.FindPreviousUpdateTx(ctx, tx, branch, head[0].CreatedAt, head[0].ID)
				return err
			}); err != nil {
				return zero, usecase.Internal("REPO", "find previous update failed", err)
			}
			if prevUpdate == nil {
				return zero, usecase.Conflict("NO_PREVIOUS_UPDATE",
					"the branch has no update before its current one; roll back to the embedded bundle instead")
			}
			u, err := republishTx(ctx, s, repo, prevUpdate, head, ec, cmd, ReasonRollback, cmd.Message)
			if err != nil {
				return zero, err
			}
			return RollbackResult{Update: u, Created: true}, nil
		},
	}
}
