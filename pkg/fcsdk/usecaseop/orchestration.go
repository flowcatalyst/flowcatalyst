package usecaseop

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// TxOperation is the multi-aggregate sibling of [Operation], for business
// operations that must write several aggregates (and/or emit several domain
// events) atomically and return a custom result R that isn't a single domain
// event — e.g. provisioning a service account together with its principal and
// OAuth client.
//
// It keeps the same enforced phase order as [Operation] — Validate →
// Authorize → Execute — but Execute receives a transaction-scoped unit of
// work and performs its writes directly via the scoped commit helpers
// (usecasepgx.CommitScoped / CommitDeleteScoped / EmitEventScoped, and
// s.WithTx for the rare raw write). [RunTx] runs the phases, opens ONE
// transaction, invokes Execute inside it, and commits on success / rolls back
// on error.
//
// Prefer the single-event [Operation] + [Plan] for ordinary CRUD; reach for
// TxOperation only when one logical command genuinely spans multiple
// aggregates in one transaction.
type TxOperation[C any, R any] struct {
	// Name identifies the operation in diagnostics. Optional but recommended.
	Name string

	// Validate checks command shape. Pure, no DB. Optional.
	Validate func(ctx context.Context, cmd C) error

	// Authorize is the resource-level access check. REQUIRED — a real check,
	// or [Public]. RunTx fails closed if it is nil, and the uowseal analyzer
	// flags any TxOperation literal that omits it.
	Authorize func(ctx context.Context, cmd C) error

	// Execute performs the orchestrated writes inside the open transaction s
	// and returns the custom result on success, or an error to roll back.
	// Every aggregate change MUST go through a scoped commit helper
	// (usecasepgx.CommitScoped / EmitEventScoped / …) so it is written with
	// its domain event + audit log atomically. REQUIRED.
	Execute func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd C, ec usecase.ExecutionContext) (R, error)
}

// RunTx executes op's phases — Validate, Authorize — then opens one
// transaction and runs Execute inside it, committing on success and rolling
// back on error or panic. Returns Execute's custom result.
func RunTx[C any, R any](
	ctx context.Context,
	uow *usecasepgx.UnitOfWork,
	op TxOperation[C, R],
	cmd C,
	ec usecase.ExecutionContext,
) (R, error) {
	var zero R

	if op.Authorize == nil {
		return zero, usecase.Internal("USECASE_MISCONFIGURED",
			"tx operation "+op.Name+" has no Authorize phase (set it, or use usecaseop.Public)", nil)
	}
	if op.Execute == nil {
		return zero, usecase.Internal("USECASE_MISCONFIGURED",
			"tx operation "+op.Name+" has no Execute phase", nil)
	}

	if op.Validate != nil {
		if err := op.Validate(ctx, cmd); err != nil {
			return zero, err
		}
	}
	if err := op.Authorize(ctx, cmd); err != nil {
		return zero, err
	}

	var out R
	err := usecasepgx.RunErr(ctx, uow, func(s *usecasepgx.TxScopedUnitOfWork) error {
		r, err := op.Execute(ctx, s, cmd, ec)
		if err != nil {
			return err
		}
		out = r
		return nil
	})
	if err != nil {
		return zero, err
	}
	return out, nil
}
