package dispatchpool

import (
	"context"

	"go.flowcatalyst.tech/internal/common/repository"
)

const collectionName = "dispatch_pools"

// instrumentedRepository wraps a Repository with metrics and logging
type instrumentedRepository struct {
	inner Repository
}

// newInstrumentedRepository creates an instrumented wrapper around a Repository
func newInstrumentedRepository(inner Repository) Repository {
	return &instrumentedRepository{inner: inner}
}

func (r *instrumentedRepository) FindByID(ctx context.Context, id string) (*DispatchPool, error) {
	return repository.Instrument(ctx, collectionName, "FindByID", func() (*DispatchPool, error) {
		return r.inner.FindByID(ctx, id)
	})
}

func (r *instrumentedRepository) FindByCode(ctx context.Context, code string) (*DispatchPool, error) {
	return repository.Instrument(ctx, collectionName, "FindByCode", func() (*DispatchPool, error) {
		return r.inner.FindByCode(ctx, code)
	})
}

func (r *instrumentedRepository) FindAll(ctx context.Context) ([]*DispatchPool, error) {
	return repository.Instrument(ctx, collectionName, "FindAll", func() ([]*DispatchPool, error) {
		return r.inner.FindAll(ctx)
	})
}

func (r *instrumentedRepository) FindAllEnabled(ctx context.Context) ([]*DispatchPool, error) {
	return repository.Instrument(ctx, collectionName, "FindAllEnabled", func() ([]*DispatchPool, error) {
		return r.inner.FindAllEnabled(ctx)
	})
}

func (r *instrumentedRepository) FindAllActive(ctx context.Context) ([]*DispatchPool, error) {
	return repository.Instrument(ctx, collectionName, "FindAllActive", func() ([]*DispatchPool, error) {
		return r.inner.FindAllActive(ctx)
	})
}

func (r *instrumentedRepository) FindByStatus(ctx context.Context, status DispatchPoolStatus) ([]*DispatchPool, error) {
	return repository.Instrument(ctx, collectionName, "FindByStatus", func() ([]*DispatchPool, error) {
		return r.inner.FindByStatus(ctx, status)
	})
}

func (r *instrumentedRepository) FindAnchorLevel(ctx context.Context) ([]*DispatchPool, error) {
	return repository.Instrument(ctx, collectionName, "FindAnchorLevel", func() ([]*DispatchPool, error) {
		return r.inner.FindAnchorLevel(ctx)
	})
}

func (r *instrumentedRepository) FindAllNonArchived(ctx context.Context) ([]*DispatchPool, error) {
	return repository.Instrument(ctx, collectionName, "FindAllNonArchived", func() ([]*DispatchPool, error) {
		return r.inner.FindAllNonArchived(ctx)
	})
}

func (r *instrumentedRepository) FindByClientID(ctx context.Context, clientID string) ([]*DispatchPool, error) {
	return repository.Instrument(ctx, collectionName, "FindByClientID", func() ([]*DispatchPool, error) {
		return r.inner.FindByClientID(ctx, clientID)
	})
}

func (r *instrumentedRepository) Insert(ctx context.Context, pool *DispatchPool) error {
	return repository.InstrumentVoid(ctx, collectionName, "Insert", func() error {
		return r.inner.Insert(ctx, pool)
	})
}

func (r *instrumentedRepository) Update(ctx context.Context, pool *DispatchPool) error {
	return repository.InstrumentVoid(ctx, collectionName, "Update", func() error {
		return r.inner.Update(ctx, pool)
	})
}

func (r *instrumentedRepository) UpdateConfig(ctx context.Context, id string, concurrency, queueCapacity int, rateLimitPerMin *int) error {
	return repository.InstrumentVoid(ctx, collectionName, "UpdateConfig", func() error {
		return r.inner.UpdateConfig(ctx, id, concurrency, queueCapacity, rateLimitPerMin)
	})
}

func (r *instrumentedRepository) SetEnabled(ctx context.Context, id string, enabled bool) error {
	return repository.InstrumentVoid(ctx, collectionName, "SetEnabled", func() error {
		return r.inner.SetEnabled(ctx, id, enabled)
	})
}

func (r *instrumentedRepository) SetStatus(ctx context.Context, id string, status DispatchPoolStatus) error {
	return repository.InstrumentVoid(ctx, collectionName, "SetStatus", func() error {
		return r.inner.SetStatus(ctx, id, status)
	})
}

func (r *instrumentedRepository) Delete(ctx context.Context, id string) error {
	return repository.InstrumentVoid(ctx, collectionName, "Delete", func() error {
		return r.inner.Delete(ctx, id)
	})
}

func (r *instrumentedRepository) Count(ctx context.Context) (int64, error) {
	return repository.Instrument(ctx, collectionName, "Count", func() (int64, error) {
		return r.inner.Count(ctx)
	})
}

func (r *instrumentedRepository) CountEnabled(ctx context.Context) (int64, error) {
	return repository.Instrument(ctx, collectionName, "CountEnabled", func() (int64, error) {
		return r.inner.CountEnabled(ctx)
	})
}

func (r *instrumentedRepository) CountActive(ctx context.Context) (int64, error) {
	return repository.Instrument(ctx, collectionName, "CountActive", func() (int64, error) {
		return r.inner.CountActive(ctx)
	})
}

func (r *instrumentedRepository) CountByStatus(ctx context.Context, status DispatchPoolStatus) (int64, error) {
	return repository.Instrument(ctx, collectionName, "CountByStatus", func() (int64, error) {
		return r.inner.CountByStatus(ctx, status)
	})
}

func (r *instrumentedRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	return repository.Instrument(ctx, collectionName, "ExistsByCode", func() (bool, error) {
		return r.inner.ExistsByCode(ctx, code)
	})
}
