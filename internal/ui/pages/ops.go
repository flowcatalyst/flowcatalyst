package pages

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
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
