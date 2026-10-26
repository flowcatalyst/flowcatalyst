package operations

import (
	"context"
	"fmt"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// SaveVersionCommand is the input DTO.
type SaveVersionCommand struct {
	ID         string               `json:"id"`
	Definition ingestion.Definition `json:"definition"`
	// Live also makes the saved version the one runtimes receive; otherwise it
	// is kept until MakeVersionLive.
	Live bool    `json:"live"`
	Note *string `json:"note,omitempty"`
}

// SaveVersion records the definition as the ingestion's next version,
// deduplicated by digest as UpdateIngestion is: a definition the live version
// already says writes nothing (UNCHANGED); one a saved version that is not
// live says is made live when Live (MADE_LIVE) and otherwise answered as that
// draft (DRAFT_EXISTS); anything else is saved, live (SAVED, emitting
// [IngestionVersionSaved] with live true) or as a draft (DRAFT). Unlike
// UpdateIngestion it does not need the live version to be unchanged, and a
// version that is not made live runs nowhere.
func SaveVersion(repo *ingestion.Repository, resolver ingestion.Resolver) usecaseop.TxOperation[SaveVersionCommand, SaveResult] {
	return usecaseop.TxOperation[SaveVersionCommand, SaveResult]{
		Name: "SaveIngestionVersion",
		Validate: func(_ context.Context, cmd SaveVersionCommand) error {
			if err := requireID(cmd.ID); err != nil {
				return err
			}
			return validateDefinition(cmd.Definition)
		},
		// Promote when the version is saved live (the controller checks it too);
		// per-resource authz runs post-load in Execute; the coarse update
		// permission is the controller's.
		Authorize: func(ctx context.Context, cmd SaveVersionCommand) error {
			if cmd.Live {
				return requirePromote(ctx, promoteVersion)
			}
			return nil
		},
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd SaveVersionCommand, ec usecase.ExecutionContext) (SaveResult, error) {
			var zero SaveResult
			i, err := loadEditable(ctx, repo, cmd.ID)
			if err != nil {
				return zero, err
			}
			if i.Version > 0 && cmd.Definition.Equal(i.Definition) {
				return SaveResult{Ingestion: i, Outcome: OutcomeUnchanged, Version: i.Version}, nil
			}
			who, err := identityOf(ctx, repo, i)
			if err != nil {
				return zero, err
			}
			effective, err := checkAuthor(ctx, repo, i, who, cmd.Definition)
			if err != nil {
				return zero, err
			}
			if err := checkTarget(ctx, resolver, i, effective.Definition); err != nil {
				return zero, err
			}
			if res, done, err := reuseOrDraft(ctx, s, repo, i, cmd.Definition, cmd.Live, cmd.Note, ec); done || err != nil {
				res.Warnings = effective.Warnings()
				return res, err
			}
			v := i.SaveVersion(cmd.Definition, true, cmd.Note, new(ec.PrincipalID))
			event := IngestionVersionSaved{
				ingestionEvent: newEvent(ec, IngestionVersionSavedType, i.ID),
				Code:           i.Code,
				Version:        v.Number,
				Live:           true,
			}
			if _, err := usecase.Into(usecasepgx.CommitScoped(ctx, s, i, repo, event, cmd)); err != nil {
				return zero, err
			}
			return SaveResult{Ingestion: i, Outcome: OutcomeSaved, Version: v.Number, Warnings: effective.Warnings()}, nil
		},
	}
}

// MakeVersionLiveCommand is the input DTO.
type MakeVersionLiveCommand struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

// MakeVersionLive moves the live pointer to a saved version (forwards or
// back) and emits [IngestionVersionLive].
func MakeVersionLive(repo *ingestion.Repository) usecaseop.Operation[MakeVersionLiveCommand, IngestionVersionLive] {
	return usecaseop.Operation[MakeVersionLiveCommand, IngestionVersionLive]{
		Name: "MakeIngestionVersionLive",
		Validate: func(_ context.Context, cmd MakeVersionLiveCommand) error {
			return requireID(cmd.ID)
		},
		// Per-resource authz runs post-load in Execute; the coarse "may
		// promote ingestions" permission is the controller's.
		Authorize: usecaseop.Public[MakeVersionLiveCommand],
		Execute: func(ctx context.Context, cmd MakeVersionLiveCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IngestionVersionLive], error) {
			i, err := loadEditable(ctx, repo, cmd.ID)
			if err != nil {
				return nil, err
			}
			v, err := repo.FindVersion(ctx, i.ID, cmd.Version)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_version failed", err)
			}
			if v == nil {
				return nil, usecase.NotFound("INGESTION_VERSION_NOT_FOUND",
					fmt.Sprintf("Ingestion %s has no version %d", i.Code, cmd.Version))
			}
			// Judged against the version live now, not the one that was live when
			// the draft was saved: what runtimes would be steered from is this.
			who, err := identityOf(ctx, repo, i)
			if err != nil {
				return nil, err
			}
			if _, err := judge(ctx, repo, i, who, v.Definition, false); err != nil {
				return nil, err
			}
			if err := i.MakeLive(*v); err != nil {
				return nil, usecase.Internal("REPO", "make_live failed", err)
			}
			event := IngestionVersionLive{
				ingestionEvent: newEvent(ec, IngestionVersionLiveType, i.ID),
				Code:           i.Code,
				Version:        v.Number,
			}
			return usecaseop.Save(i, repo, event), nil
		},
	}
}
