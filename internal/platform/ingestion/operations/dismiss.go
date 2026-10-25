package operations

import (
	"context"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// DismissFailureCommand is the input DTO.
type DismissFailureCommand struct {
	IngestionID string `json:"ingestionId"`
	FailureID   string `json:"failureId"`
}

// DismissFailureResult is the failure as it is after the dismissal. Event is
// nil when it had been dismissed already and nothing was written.
type DismissFailureResult struct {
	Failure *ingestion.Failure
	Event   *IngestionFailureDismissed
}

// DismissFailure records that someone has seen a failure a runtime reported
// and dealt with it, and emits [IngestionFailureDismissed]. A notice (a
// rejected failure) then stops showing on the ingestion. Dismissing one
// already dismissed changes nothing: no event, no audit row. Dismissing is
// not editing the definition, so an ingestion a set owns can have its
// failures dismissed.
func DismissFailure(repo *ingestion.Repository) usecaseop.TxOperation[DismissFailureCommand, DismissFailureResult] {
	return usecaseop.TxOperation[DismissFailureCommand, DismissFailureResult]{
		Name: "DismissIngestionFailure",
		Validate: func(_ context.Context, cmd DismissFailureCommand) error {
			if err := requireID(cmd.IngestionID); err != nil {
				return err
			}
			if strings.TrimSpace(cmd.FailureID) == "" {
				return usecase.Validation("FAILURE_ID_REQUIRED", "failureId is required")
			}
			return nil
		},
		// Per-resource authz (the ingestion's reach) runs post-load in
		// Execute; the coarse "may update ingestions" permission is the
		// controller's.
		Authorize: usecaseop.Public[DismissFailureCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd DismissFailureCommand, ec usecase.ExecutionContext) (DismissFailureResult, error) {
			var zero DismissFailureResult
			i, err := load(ctx, repo, cmd.IngestionID)
			if err != nil {
				return zero, err
			}
			f, err := repo.FindFailure(ctx, i.ID, cmd.FailureID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_failure failed", err)
			}
			if f == nil {
				return zero, usecase.NotFound("INGESTION_FAILURE_NOT_FOUND", "Ingestion "+i.ID+" has no failure "+cmd.FailureID)
			}
			if f.Dismissed() {
				return DismissFailureResult{Failure: f}, nil
			}
			f.Dismiss(ec.PrincipalID, time.Now().UTC())
			event := IngestionFailureDismissed{
				ingestionEvent: newEvent(ec, IngestionFailureDismissedType, i.ID),
				Code:           i.Code, FailureID: f.ID, Category: f.Category,
			}
			event, err = usecase.Into(usecasepgx.CommitScoped(ctx, s, f, repo.Failures(), event, cmd))
			if err != nil {
				return zero, err
			}
			return DismissFailureResult{Failure: f, Event: &event}, nil
		},
	}
}
