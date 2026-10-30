package pages

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	ingops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// IngestionCommands are the ingestion operations the page runs; IngestionOps
// runs the API's use cases. The coarse permission check is the caller's, as
// it is the API handler's; reach, the kind's permission and the set that
// owns an ingestion are the use case's.
type IngestionCommands interface {
	Create(ctx context.Context, clientRef string, cmd ingops.CreateCommand) (string, error)
	// SaveVersion saves a version, deduplicated by digest: the result says
	// whether it was saved, made an existing version live, or matched a draft.
	SaveVersion(ctx context.Context, cmd ingops.SaveVersionCommand) (ingops.SaveResult, error)
	MakeLive(ctx context.Context, id string, version int) error
	Pause(ctx context.Context, id string) error
	Resume(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
	// DismissFailure dismisses one of the ingestion's failures (a notice
	// stops showing).
	DismissFailure(ctx context.Context, id, failureID string) error
	// SetConfig writes the config entry the ingestion's source names, after
	// holding it against every ingestion that names it; DeleteConfig deletes
	// it. Both need only the update permission, also for an ingestion a set
	// owns (docs/ingestion-design.md section 13).
	SetConfig(ctx context.Context, cmd ingops.SetConfigCommand) (ingops.IngestionConfigSet, error)
	DeleteConfig(ctx context.Context, id string) (ingops.IngestionConfigDeleted, error)
}

// IngestionOps runs the ingestion use cases through the unit of work, exactly
// as internal/platform/ingestion/api does (usecaseop.Run with the caller's
// execution context).
type IngestionOps struct {
	Repo     *ingestion.Repository
	Resolver ingestion.Resolver
	// Configs is the platform config repository the entries of the environment
	// are written to (only through SetConfig and DeleteConfig). Left nil, one
	// is made over the unit of work's pool: an entry is a plain value, so it
	// needs none of the wired repository's encryption.
	Configs *platformconfig.Repository
	UoW     *usecasepgx.UnitOfWork
}

// Create runs CreateIngestion and returns the new ingestion's id. The client
// is resolved from its id or identifier as the API does, after the caller's
// permission check.
func (o IngestionOps) Create(ctx context.Context, clientRef string, cmd ingops.CreateCommand) (string, error) {
	c, err := o.Resolver.Client(ctx, clientRef)
	if err != nil {
		return "", err
	}
	cmd.ClientID = c.ID
	ev, err := usecaseop.Run(ctx, o.UoW, ingops.CreateIngestion(o.Repo, o.Resolver), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return "", err
	}
	return ev.IngestionID, nil
}

// SaveVersion runs SaveIngestionVersion.
func (o IngestionOps) SaveVersion(ctx context.Context, cmd ingops.SaveVersionCommand) (ingops.SaveResult, error) {
	return usecaseop.RunTx(ctx, o.UoW, ingops.SaveVersion(o.Repo, o.Resolver), cmd, auth.NewExecutionContext(ctx))
}

// MakeLive runs MakeIngestionVersionLive.
func (o IngestionOps) MakeLive(ctx context.Context, id string, version int) error {
	_, err := usecaseop.Run(ctx, o.UoW, ingops.MakeVersionLive(o.Repo), ingops.MakeVersionLiveCommand{ID: id, Version: version}, auth.NewExecutionContext(ctx))
	return err
}

// Pause runs PauseIngestion.
func (o IngestionOps) Pause(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, ingops.PauseIngestion(o.Repo), ingops.PauseCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Resume runs ResumeIngestion.
func (o IngestionOps) Resume(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, ingops.ResumeIngestion(o.Repo), ingops.ResumeCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Delete runs DeleteIngestion.
func (o IngestionOps) Delete(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, ingops.DeleteIngestion(o.Repo), ingops.DeleteCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// DismissFailure runs DismissIngestionFailure.
func (o IngestionOps) DismissFailure(ctx context.Context, id, failureID string) error {
	cmd := ingops.DismissFailureCommand{IngestionID: id, FailureID: failureID}
	_, err := usecaseop.RunTx(ctx, o.UoW, ingops.DismissFailure(o.Repo), cmd, auth.NewExecutionContext(ctx))
	return err
}

func (o IngestionOps) configs() *platformconfig.Repository {
	if o.Configs != nil {
		return o.Configs
	}
	return platformconfig.NewRepository(o.UoW.Pool())
}

// SetConfig runs SetIngestionConfig.
func (o IngestionOps) SetConfig(ctx context.Context, cmd ingops.SetConfigCommand) (ingops.IngestionConfigSet, error) {
	return usecaseop.Run(ctx, o.UoW, ingops.SetIngestionConfig(o.Repo, o.configs(), o.Resolver), cmd, auth.NewExecutionContext(ctx))
}

// DeleteConfig runs DeleteIngestionConfig.
func (o IngestionOps) DeleteConfig(ctx context.Context, id string) (ingops.IngestionConfigDeleted, error) {
	return usecaseop.Run(ctx, o.UoW, ingops.DeleteIngestionConfig(o.Repo, o.configs()), ingops.DeleteConfigCommand{ID: id}, auth.NewExecutionContext(ctx))
}
