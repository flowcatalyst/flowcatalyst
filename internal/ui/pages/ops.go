package pages

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	connops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	djops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/operations"
	dispatchprocessing "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	etops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	sjops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	subops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
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

// ApplicationCommands are the application operations the applications page
// runs; ApplicationOps runs the API's use cases.
type ApplicationCommands interface {
	Create(ctx context.Context, cmd appops.CreateCommand) (string, error)
	Update(ctx context.Context, cmd appops.UpdateCommand) error
	Activate(ctx context.Context, id string) error
	Deactivate(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
	ProvisionServiceAccount(ctx context.Context, id string) (appops.ProvisionServiceAccountResult, error)
	ProvisionLoginClient(ctx context.Context, cmd appops.ProvisionLoginClientCommand) (appops.ProvisionLoginClientResult, error)
}

// ApplicationOps runs the application use cases through the unit of work,
// exactly as internal/platform/application/api does.
type ApplicationOps struct {
	Repo            *application.Repository
	ServiceAccounts *serviceaccount.Repository
	Principals      *principal.Repository
	OAuthClients    *platformauth.OAuthClientRepo
	// Roles backs the role ceiling on provisioning (F8).
	Roles *role.Repository
	UoW   *usecasepgx.UnitOfWork
}

// Create runs CreateApplication and returns the new application's id.
func (o ApplicationOps) Create(ctx context.Context, cmd appops.CreateCommand) (string, error) {
	ev, err := usecaseop.Run(ctx, o.UoW, appops.CreateApplication(o.Repo), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return "", err
	}
	return ev.ApplicationID, nil
}

// Update runs UpdateApplication.
func (o ApplicationOps) Update(ctx context.Context, cmd appops.UpdateCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, appops.UpdateApplication(o.Repo), cmd, auth.NewExecutionContext(ctx))
	return err
}

// Activate runs ActivateApplication.
func (o ApplicationOps) Activate(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, appops.ActivateApplication(o.Repo), appops.ActivateCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Deactivate runs DeactivateApplication.
func (o ApplicationOps) Deactivate(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, appops.DeactivateApplication(o.Repo), appops.DeactivateCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Delete runs DeleteApplication (refused while references remain).
func (o ApplicationOps) Delete(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, appops.DeleteApplication(o.Repo), appops.DeleteCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// ProvisionServiceAccount runs ProvisionServiceAccount (one transaction).
func (o ApplicationOps) ProvisionServiceAccount(ctx context.Context, id string) (appops.ProvisionServiceAccountResult, error) {
	if err := appops.RequireProvisionWithinCaller(ctx, o.Roles); err != nil {
		return appops.ProvisionServiceAccountResult{}, err
	}
	return usecaseop.RunTx(ctx, o.UoW, appops.ProvisionServiceAccount(o.Repo, o.ServiceAccounts, o.Principals, o.OAuthClients),
		appops.ProvisionServiceAccountCommand{ApplicationID: id}, auth.NewExecutionContext(ctx))
}

// ProvisionLoginClient runs operations.ProvisionLoginClient.
func (o ApplicationOps) ProvisionLoginClient(ctx context.Context, cmd appops.ProvisionLoginClientCommand) (appops.ProvisionLoginClientResult, error) {
	return appops.ProvisionLoginClient(ctx, o.UoW, o.Repo, o.OAuthClients, cmd, auth.NewExecutionContext(ctx))
}

// ClientCommands are the client operations the clients page runs;
// ClientOps runs the API's use cases.
type ClientCommands interface {
	Create(ctx context.Context, cmd clientops.CreateCommand) (string, error)
	Update(ctx context.Context, cmd clientops.UpdateCommand) error
	Activate(ctx context.Context, id string) error
	Suspend(ctx context.Context, id, reason string) error
	// Deactivate runs what POST /api/clients/{id}/deactivate runs
	// (DeactivateClient: INACTIVE with the reason).
	Deactivate(ctx context.Context, id, reason string) error
	UpdateApplications(ctx context.Context, id string, enabledApplicationIDs []string) error
}

// ClientOps runs the client use cases through the unit of work, exactly as
// internal/platform/client/api does.
type ClientOps struct {
	Repo    *client.Repository
	Apps    *application.Repository
	Configs *application.ClientConfigRepo
	UoW     *usecasepgx.UnitOfWork
}

// Create runs CreateClient and returns the new client's id.
func (o ClientOps) Create(ctx context.Context, cmd clientops.CreateCommand) (string, error) {
	ev, err := usecaseop.Run(ctx, o.UoW, clientops.CreateClient(o.Repo), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return "", err
	}
	return ev.ClientID, nil
}

// Update runs UpdateClient.
func (o ClientOps) Update(ctx context.Context, cmd clientops.UpdateCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, clientops.UpdateClient(o.Repo), cmd, auth.NewExecutionContext(ctx))
	return err
}

// Activate runs ActivateClient.
func (o ClientOps) Activate(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, clientops.ActivateClient(o.Repo), clientops.ActivateCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Suspend runs SuspendClient.
func (o ClientOps) Suspend(ctx context.Context, id, reason string) error {
	_, err := usecaseop.Run(ctx, o.UoW, clientops.SuspendClient(o.Repo), clientops.SuspendCommand{ID: id, Reason: reason}, auth.NewExecutionContext(ctx))
	return err
}

// Deactivate runs DeactivateClient, as the API's deactivate does.
func (o ClientOps) Deactivate(ctx context.Context, id, reason string) error {
	_, err := usecaseop.Run(ctx, o.UoW, clientops.DeactivateClient(o.Repo), clientops.DeactivateCommand{ID: id, Reason: reason}, auth.NewExecutionContext(ctx))
	return err
}

// UpdateApplications runs UpdateClientApplications.
func (o ClientOps) UpdateApplications(ctx context.Context, id string, enabled []string) error {
	_, err := usecaseop.Run(ctx, o.UoW, appops.UpdateClientApplications(o.Apps, o.Repo, o.Configs),
		appops.UpdateClientApplicationsCommand{ClientID: id, EnabledApplicationIDs: enabled}, auth.NewExecutionContext(ctx))
	return err
}

// ScheduledJobCommands are the scheduled-job operations the scheduled jobs
// page runs; ScheduledJobOps runs the API's use cases.
type ScheduledJobCommands interface {
	Create(ctx context.Context, cmd sjops.CreateCommand) (string, error)
	Update(ctx context.Context, cmd sjops.UpdateCommand) error
	Pause(ctx context.Context, id string) error
	Resume(ctx context.Context, id string) error
	Archive(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
	// Fire runs FireNow and returns the new instance's id.
	Fire(ctx context.Context, id string) (string, error)
}

// ScheduledJobOps runs the scheduled-job use cases through the unit of
// work, exactly as internal/platform/scheduledjob/api does.
type ScheduledJobOps struct {
	Repo      *scheduledjob.Repository
	Instances *scheduledjob.InstanceRepository
	UoW       *usecasepgx.UnitOfWork
}

// Create runs CreateScheduledJob and returns the new job's id.
func (o ScheduledJobOps) Create(ctx context.Context, cmd sjops.CreateCommand) (string, error) {
	ev, err := usecaseop.Run(ctx, o.UoW, sjops.CreateScheduledJob(o.Repo), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return "", err
	}
	return ev.ScheduledJobID, nil
}

// Update runs UpdateScheduledJob.
func (o ScheduledJobOps) Update(ctx context.Context, cmd sjops.UpdateCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, sjops.UpdateScheduledJob(o.Repo), cmd, auth.NewExecutionContext(ctx))
	return err
}

// Pause runs PauseScheduledJob.
func (o ScheduledJobOps) Pause(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, sjops.PauseScheduledJob(o.Repo), sjops.PauseCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Resume runs ResumeScheduledJob.
func (o ScheduledJobOps) Resume(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, sjops.ResumeScheduledJob(o.Repo), sjops.ResumeCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Archive runs ArchiveScheduledJob.
func (o ScheduledJobOps) Archive(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, sjops.ArchiveScheduledJob(o.Repo), sjops.ArchiveCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Delete runs DeleteScheduledJob.
func (o ScheduledJobOps) Delete(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, sjops.DeleteScheduledJob(o.Repo), sjops.DeleteCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Fire runs FireNow.
func (o ScheduledJobOps) Fire(ctx context.Context, id string) (string, error) {
	ev, err := usecaseop.Run(ctx, o.UoW, sjops.FireNow(o.Repo, o.Instances), sjops.FireNowCommand{ID: id}, auth.NewExecutionContext(ctx))
	if err != nil {
		return "", err
	}
	return ev.InstanceID, nil
}

// DispatchJobCommands are the dispatch-job operations the dispatch jobs page
// runs; DispatchJobOps runs what internal/platform/dispatchjob/api runs.
type DispatchJobCommands interface {
	// Requeue runs ResendDispatchJobs and returns how many jobs were reset
	// (ids outside the caller's tenants are dropped by the use case).
	Requeue(ctx context.Context, ids []string) (int, error)
	// Preview counts what a requeue-by-filter would touch (SummarizeMatches).
	Preview(ctx context.Context, f dispatchjob.FilterParams) (djops.MatchSummary, error)
	// RequeueByFilter runs ResendByFilter with the confirmed eligible count
	// and the default (maximum) limit. f already carries its time window.
	RequeueByFilter(ctx context.Context, f dispatchjob.FilterParams, confirmCount int64) (djops.ResendByFilterResult, error)
	// Plan builds the job's delivery without sending it (the "sign" action);
	// ErrPlanUnavailable when the server has no planner wired.
	Plan(ctx context.Context, j *dispatchjob.DispatchJob) (*dispatchprocessing.DeliveryPlan, error)
	// EditPayload runs EditDispatchJobPayload: the job's payload (and, when
	// contentType is non-nil, its content type) is replaced; status and
	// attempts are untouched. The use case enforces the status rule, the
	// client scope and the payload validation.
	EditPayload(ctx context.Context, id, payload string, contentType *string) error
	// Cancel runs CancelDispatchJob (FAILED -> CANCELLED) and Complete runs
	// CompleteDispatchJob (FAILED -> COMPLETED, delivered out of band). Each
	// is a 409 unless the job is FAILED.
	Cancel(ctx context.Context, id string) error
	Complete(ctx context.Context, id string) error
}

// ErrPlanUnavailable is DispatchJobOps.Plan without a planner (the API's 501).
var ErrPlanUnavailable = usecase.Internal("PLAN_UNAVAILABLE", "delivery planning is not wired on this server", nil)

// DispatchJobOps runs the dispatch-job operations exactly as the API does.
type DispatchJobOps struct {
	Repo *dispatchjob.Repository
	UoW  *usecasepgx.UnitOfWork
	// Planner is processing.Handler.Plan (the API's State.Plan).
	Planner func(ctx context.Context, j *dispatchjob.DispatchJob) (*dispatchprocessing.DeliveryPlan, error)
}

// Requeue runs ResendDispatchJobs.
func (o DispatchJobOps) Requeue(ctx context.Context, ids []string) (int, error) {
	ev, err := usecaseop.Run(ctx, o.UoW, djops.ResendDispatchJobs(o.Repo), djops.ResendCommand{IDs: ids}, auth.NewExecutionContext(ctx))
	if err != nil {
		return 0, err
	}
	return len(ev.IDs), nil
}

// EditPayload runs EditDispatchJobPayload.
func (o DispatchJobOps) EditPayload(ctx context.Context, id, payload string, contentType *string) error {
	_, err := usecaseop.Run(ctx, o.UoW, djops.EditDispatchJobPayload(o.Repo),
		djops.EditPayloadCommand{ID: id, Payload: payload, PayloadContentType: contentType}, auth.NewExecutionContext(ctx))
	return err
}

// Cancel runs CancelDispatchJob.
func (o DispatchJobOps) Cancel(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, djops.CancelDispatchJob(o.Repo), djops.CancelCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Complete runs CompleteDispatchJob.
func (o DispatchJobOps) Complete(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, djops.CompleteDispatchJob(o.Repo), djops.CompleteCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Preview runs SummarizeMatches.
func (o DispatchJobOps) Preview(ctx context.Context, f dispatchjob.FilterParams) (djops.MatchSummary, error) {
	return djops.SummarizeMatches(ctx, o.Repo, f)
}

// RequeueByFilter runs ResendByFilter (limit 0: the default, which is the cap;
// no redrive: COMPLETED jobs and jobs in flight are left alone).
func (o DispatchJobOps) RequeueByFilter(ctx context.Context, f dispatchjob.FilterParams, confirmCount int64) (djops.ResendByFilterResult, error) {
	return djops.ResendByFilter(ctx, o.UoW, o.Repo, f, confirmCount, 0, djops.ResendByFilterOptions{}, auth.NewExecutionContext(ctx))
}

// Plan runs the planner.
func (o DispatchJobOps) Plan(ctx context.Context, j *dispatchjob.DispatchJob) (*dispatchprocessing.DeliveryPlan, error) {
	if o.Planner == nil {
		return nil, ErrPlanUnavailable
	}
	return o.Planner(ctx, j)
}
