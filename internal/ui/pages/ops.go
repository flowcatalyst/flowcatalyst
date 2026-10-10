package pages

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	connops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	etops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	subops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// PoolCommands are the dispatch-pool operations the page runs. The
// production implementation (PoolOps) runs the same use cases as the API
// handlers, so validation, resource authorization, the domain event and the
// audit record are the use case's, not the page's.
type PoolCommands interface {
	Create(ctx context.Context, cmd operations.CreateCommand) (string, error)
	Update(ctx context.Context, cmd operations.UpdateCommand) error
	Suspend(ctx context.Context, id string) error
	Activate(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
}

// PoolOps runs the dispatch-pool use cases through the unit of work, exactly
// as internal/platform/dispatchpool/api does (usecaseop.Run with the
// caller's execution context). The coarse permission check is the caller's,
// as it is the API handler's.
type PoolOps struct {
	Repo *dispatchpool.Repository
	UoW  *usecasepgx.UnitOfWork
}

// Create runs CreateDispatchPool and returns the new pool's id.
func (o PoolOps) Create(ctx context.Context, cmd operations.CreateCommand) (string, error) {
	ev, err := usecaseop.Run(ctx, o.UoW, operations.CreateDispatchPool(o.Repo), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return "", err
	}
	return ev.PoolID, nil
}

// Update runs UpdateDispatchPool.
func (o PoolOps) Update(ctx context.Context, cmd operations.UpdateCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, operations.UpdateDispatchPool(o.Repo), cmd, auth.NewExecutionContext(ctx))
	return err
}

// Suspend runs SuspendDispatchPool.
func (o PoolOps) Suspend(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, operations.SuspendDispatchPool(o.Repo), operations.SuspendCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Activate runs ActivateDispatchPool.
func (o PoolOps) Activate(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, operations.ActivateDispatchPool(o.Repo), operations.ActivateCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Delete runs DeleteDispatchPool.
func (o PoolOps) Delete(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, operations.DeleteDispatchPool(o.Repo), operations.DeleteCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// ConnectionCommands are the connection operations the connections page
// runs; ConnectionOps runs the API's use cases.
type ConnectionCommands interface {
	Create(ctx context.Context, cmd connops.CreateCommand) (string, error)
	Update(ctx context.Context, cmd connops.UpdateCommand) error
	Pause(ctx context.Context, id string) error
	Activate(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
}

// ConnectionOps runs the connection use cases through the unit of work,
// exactly as internal/platform/connection/api does.
type ConnectionOps struct {
	Repo *connection.Repository
	Apps *application.Repository
	Subs *subscription.Repository
	UoW  *usecasepgx.UnitOfWork
}

// Create runs CreateConnection and returns the new connection's id.
func (o ConnectionOps) Create(ctx context.Context, cmd connops.CreateCommand) (string, error) {
	ev, err := usecaseop.Run(ctx, o.UoW, connops.CreateConnection(o.Repo, o.Apps), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return "", err
	}
	return ev.ConnectionID, nil
}

// Update runs UpdateConnection.
func (o ConnectionOps) Update(ctx context.Context, cmd connops.UpdateCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, connops.UpdateConnection(o.Repo, o.Apps), cmd, auth.NewExecutionContext(ctx))
	return err
}

// Pause runs PauseConnection.
func (o ConnectionOps) Pause(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, connops.PauseConnection(o.Repo), connops.PauseCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Activate runs ActivateConnection.
func (o ConnectionOps) Activate(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, connops.ActivateConnection(o.Repo), connops.ActivateCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Delete runs DeleteConnection (refused while a subscription targets it).
func (o ConnectionOps) Delete(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, connops.DeleteConnection(o.Repo, o.Subs), connops.DeleteCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// EventTypeCommands are the event-type operations the event types page runs;
// EventTypeOps runs the use cases the SPA's BFF endpoints run.
type EventTypeCommands interface {
	Create(ctx context.Context, cmd etops.CreateCommand) (string, error)
	Update(ctx context.Context, cmd etops.UpdateCommand) error
	Archive(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
	AddSchema(ctx context.Context, cmd etops.AddSchemaCommand) error
	FinaliseSchema(ctx context.Context, id, version string) error
	DeprecateSchema(ctx context.Context, id, version string) error
	// SyncPlatform re-runs the platform event-type catalogue sync (the SPA's
	// "Sync Platform Events").
	SyncPlatform(ctx context.Context) (EventTypeSyncResult, error)
}

// EventTypeSyncResult is the sync's tally (the BFF's sync-platform answer).
type EventTypeSyncResult struct {
	Created, Updated, Deleted, Total            int
	SchemasCreated, SchemasUpdated, SchemasSame int
}

// EventTypeOps runs the event-type use cases through the unit of work,
// exactly as internal/platform/shared/bff/event_types.go does.
type EventTypeOps struct {
	Repo *eventtype.Repository
	UoW  *usecasepgx.UnitOfWork
}

// Create runs CreateEventType and returns the new event type's id.
func (o EventTypeOps) Create(ctx context.Context, cmd etops.CreateCommand) (string, error) {
	ev, err := usecaseop.Run(ctx, o.UoW, etops.CreateEventType(o.Repo), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return "", err
	}
	return ev.EventTypeID, nil
}

// Update runs UpdateEventType.
func (o EventTypeOps) Update(ctx context.Context, cmd etops.UpdateCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, etops.UpdateEventType(o.Repo), cmd, auth.NewExecutionContext(ctx))
	return err
}

// Archive runs ArchiveEventType.
func (o EventTypeOps) Archive(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, etops.ArchiveEventType(o.Repo), etops.ArchiveCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Delete runs DeleteEventType.
func (o EventTypeOps) Delete(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, etops.DeleteEventType(o.Repo), etops.DeleteCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// AddSchema runs AddSchema.
func (o EventTypeOps) AddSchema(ctx context.Context, cmd etops.AddSchemaCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, etops.AddSchema(o.Repo), cmd, auth.NewExecutionContext(ctx))
	return err
}

// FinaliseSchema runs FinaliseEventTypeSchema.
func (o EventTypeOps) FinaliseSchema(ctx context.Context, id, version string) error {
	_, err := usecaseop.Run(ctx, o.UoW, etops.FinaliseEventTypeSchema(o.Repo), etops.FinaliseSchemaCommand{EventTypeID: id, Version: version}, auth.NewExecutionContext(ctx))
	return err
}

// DeprecateSchema runs DeprecateEventTypeSchema.
func (o EventTypeOps) DeprecateSchema(ctx context.Context, id, version string) error {
	_, err := usecaseop.Run(ctx, o.UoW, etops.DeprecateEventTypeSchema(o.Repo), etops.DeprecateSchemaCommand{EventTypeID: id, Version: version}, auth.NewExecutionContext(ctx))
	return err
}

// SyncPlatform syncs the platform's code-defined event types into the
// "platform" application, removing unlisted ones (the BFF's sync-platform
// with its default application code).
func (o EventTypeOps) SyncPlatform(ctx context.Context) (EventTypeSyncResult, error) {
	defs := seed.PlatformEventTypes()
	inputs := make([]etops.SyncEventTypeInput, 0, len(defs))
	for _, d := range defs {
		inputs = append(inputs, etops.SyncEventTypeInput{Code: d.Code, Name: d.Name, Schema: d.Schema})
	}
	cmd := etops.SyncEventTypesCommand{ApplicationCode: "platform", EventTypes: inputs, RemoveUnlisted: true}
	ev, err := usecaseop.Run(ctx, o.UoW, etops.SyncEventTypes(o.Repo), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return EventTypeSyncResult{}, err
	}
	return EventTypeSyncResult{
		Created: int(ev.Created), Updated: int(ev.Updated), Deleted: int(ev.Deleted), Total: len(defs),
		SchemasCreated: int(ev.SchemasCreated), SchemasUpdated: int(ev.SchemasUpdated), SchemasSame: int(ev.SchemasUnchanged),
	}, nil
}

// SubscriptionCommands are the subscription operations the subscriptions
// page runs; SubscriptionOps runs the API's use cases.
type SubscriptionCommands interface {
	Create(ctx context.Context, cmd subops.CreateCommand) (string, error)
	Update(ctx context.Context, cmd subops.UpdateCommand) error
	Pause(ctx context.Context, id string) error
	Resume(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
}

// SubscriptionOps runs the subscription use cases through the unit of work,
// exactly as internal/platform/subscription/api does.
type SubscriptionOps struct {
	Repo *subscription.Repository
	UoW  *usecasepgx.UnitOfWork
}

// Create runs CreateSubscription and returns the new subscription's id.
func (o SubscriptionOps) Create(ctx context.Context, cmd subops.CreateCommand) (string, error) {
	ev, err := usecaseop.Run(ctx, o.UoW, subops.CreateSubscription(o.Repo), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return "", err
	}
	return ev.SubscriptionID, nil
}

// Update runs UpdateSubscription.
func (o SubscriptionOps) Update(ctx context.Context, cmd subops.UpdateCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, subops.UpdateSubscription(o.Repo), cmd, auth.NewExecutionContext(ctx))
	return err
}

// Pause runs PauseSubscription.
func (o SubscriptionOps) Pause(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, subops.PauseSubscription(o.Repo), subops.PauseCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Resume runs ResumeSubscription.
func (o SubscriptionOps) Resume(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, subops.ResumeSubscription(o.Repo), subops.ResumeCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Delete runs DeleteSubscription.
func (o SubscriptionOps) Delete(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, subops.DeleteSubscription(o.Repo), subops.DeleteCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}
