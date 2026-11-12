package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/sdk"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// RequireSignAccess is the gate on the "sign" action (the API route and the
// /ui console), applied after the permission and the job's client scope. The
// answer carries a real signature of the job's stored body made with the
// signing service account's secret — and the payload can be rewritten first —
// so it is an operator's tool: the caller must be anchor tier, and one confined
// to some applications only for the jobs of those applications (the confinement
// the other by-id dispatch-job routes apply; a job of another application is
// answered as not found).
func RequireSignAccess(ctx context.Context, ac *auth.AuthContext, apps *application.Repository, j *dispatchjob.DispatchJob) error {
	if !ac.IsAnchor() {
		return usecase.Authorization("SCOPE_FORBIDDEN", "anchor scope required to sign a delivery")
	}
	ok, err := sdk.NewAppScope(apps).CanAccessCode(ctx, ac, sdk.FirstSegment(j.Code))
	if err != nil {
		return err
	}
	if !ok {
		return httperror.NotFound("DispatchJob", j.ID)
	}
	return nil
}
