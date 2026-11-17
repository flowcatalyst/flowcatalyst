package operations

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// RepublishCommand makes an earlier update of a branch live again.
type RepublishCommand struct {
	UpdateID string  `json:"updateId"`
	Message  *string `json:"message,omitempty"`
}

// Republish publishes an earlier update's files again as a NEW update of the
// same branch: new manifest id, a commit time later than every row of the
// branch, rollout 100, republishedFrom set. A phone loads an update only when
// it is newer than the one it runs (the client's selection policy compares
// createdAt), so re-serving the old manifest would change nothing on a phone
// that took the newer one. Republishing the branch's newest row is refused
// (409 ALREADY_LIVE: change its rollout instead).
func Republish(repo *appupdate.Repository) usecaseop.TxOperation[RepublishCommand, *appupdate.Update] {
	return usecaseop.TxOperation[RepublishCommand, *appupdate.Update]{
		Name: "RepublishAppUpdate",
		Validate: func(_ context.Context, cmd RepublishCommand) error {
			return ValidateMessage(cmd.Message, nil)
		},
		Authorize: func(ctx context.Context, cmd RepublishCommand) error {
			_, err := ReachUpdate(ctx, repo, cmd.UpdateID, false)
			return err
		},
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd RepublishCommand, ec usecase.ExecutionContext) (*appupdate.Update, error) {
			src, err := ReachUpdate(ctx, repo, cmd.UpdateID, false)
			if err != nil {
				return nil, err
			}
			if src.Kind != appupdate.KindUpdate {
				return nil, usecase.BusinessRule("NOT_AN_UPDATE", "a rollback to the embedded bundle cannot be republished; roll back to the embedded bundle again instead")
			}
			head, err := lockAndHead(ctx, s, repo, src.Branch)
			if err != nil {
				return nil, err
			}
			if len(head) > 0 && head[0].ID == src.ID {
				return nil, usecase.Conflict("ALREADY_LIVE", "this is the branch's newest update; change its rollout instead")
			}
			return republishTx(ctx, s, repo, src, head, ec, cmd, ReasonRepublish, cmd.Message)
		},
	}
}

// lockAndHead takes the branch lock and reads its newest row, in s's transaction.
func lockAndHead(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, repo *appupdate.Repository, b appupdate.Branch) ([]appupdate.Update, error) {
	var head []appupdate.Update
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := repo.LockBranch(ctx, tx, b); err != nil {
			return err
		}
		var err error
		head, err = repo.HeadTx(ctx, tx, b, 1)
		return err
	}); err != nil {
		return nil, usecase.Internal("REPO", "read branch failed", err)
	}
	return head, nil
}

// republishTx writes a copy of src as the branch's newest update. The branch
// lock is held and head is its newest row.
func republishTx(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, repo *appupdate.Repository, src *appupdate.Update,
	head []appupdate.Update, ec usecase.ExecutionContext, cmd any, reason string, message *string,
) (*appupdate.Update, error) {
	var assets []appupdate.Asset
	if err := s.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		assets, err = repo.AssetsOfTx(ctx, tx, src.ID)
		return err
	}); err != nil {
		return nil, usecase.Internal("REPO", "read assets failed", err)
	}
	if len(assets) == 0 {
		return nil, usecase.Internal("REPO", fmt.Sprintf("update %s has no files", src.ID), nil)
	}
	now := time.Now().UTC()
	var prev time.Time
	if len(head) > 0 {
		prev = head[0].CreatedAt
	}
	if message == nil {
		m := "Republish of " + src.ID
		if reason == ReasonRollback {
			m = "Rollback to " + src.ID
		}
		message = &m
	}
	from := src.ID
	u := &appupdate.Update{
		ID: tsid.Generate(tsid.AppUpdate), ApplicationID: src.ApplicationID, ApplicationCode: src.ApplicationCode,
		Branch: src.Branch, Kind: appupdate.KindUpdate, ManifestID: uuid.NewString(),
		CreatedAt: appupdate.NextCommitTime(now, prev), RolloutPercent: 100,
		Message: message, SourceCommit: src.SourceCommit, RepublishedFrom: &from, ExpoClient: src.ExpoClient,
		PublishedBy: ec.PrincipalID, UpdatedAt: now, Assets: assets,
	}
	event := UpdateRepublished{
		eventBase: newBase(ec, UpdateRepublishedType, u), BranchData: branchData(u),
		RepublishedFrom: src.ID, ManifestID: u.ManifestID, CreatedAt: appupdate.FormatTime(u.CreatedAt),
		Reason: reason, Message: message,
	}
	if r := usecasepgx.CommitScoped(ctx, s, u, repo, event, cmd); !usecase.IsSuccess(r) {
		_, e := usecase.Into(r)
		return nil, e
	}
	return u, nil
}
