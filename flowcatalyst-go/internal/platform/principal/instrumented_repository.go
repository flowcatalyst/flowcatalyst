package principal

import (
	"context"

	"go.flowcatalyst.tech/internal/common/repository"
)

// instrumentedRepository wraps a Repository with metrics and logging
type instrumentedRepository struct {
	inner Repository
}

// newInstrumentedRepository creates an instrumented wrapper around a Repository
func newInstrumentedRepository(inner Repository) Repository {
	return &instrumentedRepository{inner: inner}
}

// === Query operations ===

func (r *instrumentedRepository) FindByID(ctx context.Context, id string) (*Principal, error) {
	return repository.Instrument(ctx, collectionName, "FindByID", func() (*Principal, error) {
		return r.inner.FindByID(ctx, id)
	})
}

func (r *instrumentedRepository) FindByEmail(ctx context.Context, email string) (*Principal, error) {
	return repository.Instrument(ctx, collectionName, "FindByEmail", func() (*Principal, error) {
		return r.inner.FindByEmail(ctx, email)
	})
}

func (r *instrumentedRepository) FindByClientID(ctx context.Context, clientID string, skip, limit int64) ([]*Principal, error) {
	return repository.Instrument(ctx, collectionName, "FindByClientID", func() ([]*Principal, error) {
		return r.inner.FindByClientID(ctx, clientID, skip, limit)
	})
}

func (r *instrumentedRepository) FindByType(ctx context.Context, principalType PrincipalType, skip, limit int64) ([]*Principal, error) {
	return repository.Instrument(ctx, collectionName, "FindByType", func() ([]*Principal, error) {
		return r.inner.FindByType(ctx, principalType, skip, limit)
	})
}

func (r *instrumentedRepository) FindActive(ctx context.Context, skip, limit int64) ([]*Principal, error) {
	return repository.Instrument(ctx, collectionName, "FindActive", func() ([]*Principal, error) {
		return r.inner.FindActive(ctx, skip, limit)
	})
}

func (r *instrumentedRepository) FindAll(ctx context.Context, skip, limit int64) ([]*Principal, error) {
	return repository.Instrument(ctx, collectionName, "FindAll", func() ([]*Principal, error) {
		return r.inner.FindAll(ctx, skip, limit)
	})
}

func (r *instrumentedRepository) Count(ctx context.Context) (int64, error) {
	return repository.Instrument(ctx, collectionName, "Count", func() (int64, error) {
		return r.inner.Count(ctx)
	})
}

func (r *instrumentedRepository) CountByType(ctx context.Context, principalType PrincipalType) (int64, error) {
	return repository.Instrument(ctx, collectionName, "CountByType", func() (int64, error) {
		return r.inner.CountByType(ctx, principalType)
	})
}

func (r *instrumentedRepository) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	return repository.Instrument(ctx, collectionName, "ExistsByEmail", func() (bool, error) {
		return r.inner.ExistsByEmail(ctx, email)
	})
}

// === Write operations ===

func (r *instrumentedRepository) Insert(ctx context.Context, principal *Principal) error {
	return repository.InstrumentVoid(ctx, collectionName, "Insert", func() error {
		return r.inner.Insert(ctx, principal)
	})
}

func (r *instrumentedRepository) Update(ctx context.Context, principal *Principal) error {
	return repository.InstrumentVoid(ctx, collectionName, "Update", func() error {
		return r.inner.Update(ctx, principal)
	})
}

func (r *instrumentedRepository) UpdateRoles(ctx context.Context, id string, roles []RoleAssignment) error {
	return repository.InstrumentVoid(ctx, collectionName, "UpdateRoles", func() error {
		return r.inner.UpdateRoles(ctx, id, roles)
	})
}

func (r *instrumentedRepository) UpdateLastLogin(ctx context.Context, id string) error {
	return repository.InstrumentVoid(ctx, collectionName, "UpdateLastLogin", func() error {
		return r.inner.UpdateLastLogin(ctx, id)
	})
}

func (r *instrumentedRepository) SetActive(ctx context.Context, id string, active bool) error {
	return repository.InstrumentVoid(ctx, collectionName, "SetActive", func() error {
		return r.inner.SetActive(ctx, id, active)
	})
}

func (r *instrumentedRepository) Delete(ctx context.Context, id string) error {
	return repository.InstrumentVoid(ctx, collectionName, "Delete", func() error {
		return r.inner.Delete(ctx, id)
	})
}
