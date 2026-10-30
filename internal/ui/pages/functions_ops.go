package pages

import (
	"context"
	"log/slog"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	fnops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// FunctionWiring is what moving or removing the live alias changed: the
// dispatch pool and the subscriptions and schedules it created, updated or
// deleted (the API's wiring summary).
type FunctionWiring struct {
	DispatchPoolCode                                                 string
	SubscriptionsCreated, SubscriptionsUpdated, SubscriptionsDeleted int
	SchedulesCreated, SchedulesUpdated, SchedulesDeleted             int
}

// FunctionCommands are the function operations the page runs; FunctionOps
// runs the API's use cases. The coarse permission check is the caller's, as
// it is the API handler's.
type FunctionCommands interface {
	Create(ctx context.Context, cmd fnops.CreateCommand) (string, error)
	Update(ctx context.Context, cmd fnops.UpdateCommand) error
	Delete(ctx context.Context, id string) error
	RetireVersion(ctx context.Context, id string, number int32) error
	PutAlias(ctx context.Context, id, name string, number int32) (FunctionWiring, error)
	DeleteAlias(ctx context.Context, id, name string) (FunctionWiring, error)
	PutSetting(ctx context.Context, id string, kind function.SettingKind, key, value string) error
	DeleteSetting(ctx context.Context, id string, kind function.SettingKind, key string) error
	PutRoute(ctx context.Context, cmd fnops.PutRouteCommand) error
	DeleteRoute(ctx context.Context, id, routeID string) error
}

// FunctionOps runs the function use cases through the unit of work, exactly
// as internal/platform/function/api does (usecaseop.Run and RunTx with the
// caller's execution context).
type FunctionOps struct {
	Repo      *function.Repository
	Apps      *application.Repository
	Wiring    fnops.WiringDeps
	Domains   *functiondomain.Repository
	Artifacts artifact.Store
	UoW       *usecasepgx.UnitOfWork
}

// Create runs CreateFunction and returns the new function's id.
func (o FunctionOps) Create(ctx context.Context, cmd fnops.CreateCommand) (string, error) {
	ev, err := usecaseop.Run(ctx, o.UoW, fnops.CreateFunction(o.Repo, o.Apps), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return "", err
	}
	return ev.FunctionID, nil
}

// Update runs UpdateFunction.
func (o FunctionOps) Update(ctx context.Context, cmd fnops.UpdateCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, fnops.UpdateFunction(o.Repo), cmd, auth.NewExecutionContext(ctx))
	return err
}

// Delete runs DeleteFunction, then removes the artifacts no other function
// references from the store. As the API does, that cleanup is best effort
// after the commit: a failure leaves a harmless orphan, never a failed delete.
func (o FunctionOps) Delete(ctx context.Context, id string) error {
	res, err := usecaseop.RunTx(ctx, o.UoW, fnops.DeleteFunction(o.Repo, o.Wiring), fnops.DeleteCommand{ID: id}, auth.NewExecutionContext(ctx))
	if err != nil {
		return err
	}
	for _, digest := range res.OrphanedDigests {
		if delErr := o.Artifacts.Delete(ctx, digest); delErr != nil {
			slog.ErrorContext(ctx, "function delete: orphaned artifact cleanup failed", "digest", digest, "err", delErr)
		}
	}
	return nil
}

// RetireVersion runs RetireFunctionVersion.
func (o FunctionOps) RetireVersion(ctx context.Context, id string, number int32) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, fnops.RetireVersion(o.Repo), fnops.RetireCommand{FunctionID: id, Number: number}, auth.NewExecutionContext(ctx))
	return err
}

// PutAlias runs PutFunctionAlias. The wiring is reported for the live alias
// only (another alias materialises none).
func (o FunctionOps) PutAlias(ctx context.Context, id, name string, number int32) (FunctionWiring, error) {
	res, err := usecaseop.RunTx(ctx, o.UoW, fnops.PutAlias(o.Repo, o.Wiring), fnops.PutAliasCommand{FunctionID: id, Name: name, Number: number}, auth.NewExecutionContext(ctx))
	if err != nil || name != function.LiveAlias {
		return FunctionWiring{}, err
	}
	return FunctionWiring(res.Wiring), nil
}

// DeleteAlias runs DeleteFunctionAlias; removing live unwires the function.
func (o FunctionOps) DeleteAlias(ctx context.Context, id, name string) (FunctionWiring, error) {
	res, err := usecaseop.RunTx(ctx, o.UoW, fnops.DeleteAlias(o.Repo, o.Wiring), fnops.DeleteAliasCommand{FunctionID: id, Name: name}, auth.NewExecutionContext(ctx))
	if err != nil || name != function.LiveAlias {
		return FunctionWiring{}, err
	}
	return FunctionWiring(res.Wiring), nil
}

// PutSetting runs PutFunctionSetting for a config value, a secret or a
// database DSN (the use case encrypts the last two and masks them in the audit).
func (o FunctionOps) PutSetting(ctx context.Context, id string, kind function.SettingKind, key, value string) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, fnops.PutSetting(o.Repo), fnops.PutSettingCommand{FunctionID: id, Kind: kind, Key: key, Value: value}, auth.NewExecutionContext(ctx))
	return err
}

// DeleteSetting runs DeleteFunctionSetting.
func (o FunctionOps) DeleteSetting(ctx context.Context, id string, kind function.SettingKind, key string) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, fnops.DeleteSetting(o.Repo), fnops.DeleteSettingCommand{FunctionID: id, Kind: kind, Key: key}, auth.NewExecutionContext(ctx))
	return err
}

// PutRoute runs PutFunctionRoute.
func (o FunctionOps) PutRoute(ctx context.Context, cmd fnops.PutRouteCommand) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, fnops.PutRoute(o.Repo, o.Domains), cmd, auth.NewExecutionContext(ctx))
	return err
}

// DeleteRoute runs DeleteFunctionRoute.
func (o FunctionOps) DeleteRoute(ctx context.Context, id, routeID string) error {
	_, err := usecaseop.RunTx(ctx, o.UoW, fnops.DeleteRoute(o.Repo), fnops.DeleteRouteCommand{FunctionID: id, RouteID: routeID}, auth.NewExecutionContext(ctx))
	return err
}
