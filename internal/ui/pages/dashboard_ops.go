package pages

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/openapispecs"
	openapiops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/openapispecs/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/bff"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// DashboardCounts reads the dashboard's counts from the pool, as
// GET /bff/dashboard/stats does (bff.LoadDashboardStats).
type DashboardCounts struct {
	Pool *pgxpool.Pool
}

// Stats is the counts: exact for the control plane, the planner's estimate
// for the high-volume tables.
func (c DashboardCounts) Stats(ctx context.Context) (bff.DashboardStats, error) {
	return bff.LoadDashboardStats(ctx, c.Pool)
}

// OpenAPISyncer publishes this build's OpenAPI document as the platform
// application's current spec (POST /bff/developer/sync-platform-openapi).
type OpenAPISyncer interface {
	SyncPlatformOpenAPI(ctx context.Context) (OpenAPISyncResult, error)
}

// OpenAPISyncResult is the sync's outcome (the BFF's answer, the fields the
// dashboard's toast reads).
type OpenAPISyncResult struct {
	Version string
	// ArchivedPriorVersion is the version this one replaced ("" for the first).
	ArchivedPriorVersion string
	HasBreaking          bool
	Unchanged            bool
}

// OpenAPISyncOps runs the OpenAPI sync use case through the unit of work,
// exactly as internal/platform/shared/bff/developer.go does. Spec is the live
// document (the huma API's, marshalled on demand).
type OpenAPISyncOps struct {
	Apps  *application.Repository
	Specs *openapispecs.Repository
	UoW   *usecasepgx.UnitOfWork
	Spec  func() (json.RawMessage, error)
}

// SyncPlatformOpenAPI syncs the live platform document into the seeded
// "platform" application.
func (o OpenAPISyncOps) SyncPlatformOpenAPI(ctx context.Context) (OpenAPISyncResult, error) {
	app, err := o.Apps.FindByCode(ctx, "platform")
	if err != nil {
		return OpenAPISyncResult{}, usecase.Internal("REPO", "find platform application failed", err)
	}
	if app == nil {
		return OpenAPISyncResult{}, usecase.Internal("SEED", "platform application missing — run seed", nil)
	}
	spec, err := o.Spec()
	if err != nil {
		return OpenAPISyncResult{}, usecase.Internal("OPENAPI", "marshal platform spec failed", err)
	}
	cmd := openapiops.SyncOpenApiSpecCommand{ApplicationID: string(app.ID), ApplicationCode: app.Code, Spec: spec}
	ev, err := usecaseop.Run(ctx, o.UoW, openapiops.SyncOpenApiSpec(o.Specs), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return OpenAPISyncResult{}, err
	}
	out := OpenAPISyncResult{Version: ev.Version, HasBreaking: ev.HasBreaking, Unchanged: ev.Unchanged}
	if ev.ArchivedPriorVersion != nil {
		out.ArchivedPriorVersion = *ev.ArchivedPriorVersion
	}
	return out, nil
}
