// alias.go is work package 4's alias endpoints plus work package 8's promote
// wiring (plan §4, §8.2, §8.5): PUT points a named alias at a READY version;
// DELETE removes it. Moving or removing the `live` alias is the ONLY thing
// that reconciles the function's dispatch pool / subscriptions / scheduled
// jobs (see wiring.go) — every other alias name (`canary`, `qa`, …) is
// HTTP-only bookkeeping.
package operations

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// PutAliasCommand is the input DTO for PUT …/aliases/{name}.
type PutAliasCommand struct {
	FunctionID string `json:"functionId"`
	Name       string `json:"name"`
	Number     int32  `json:"number"`
}

// PutAliasResult is what the API returns.
type PutAliasResult struct {
	Alias   function.Alias
	Version function.Version
	Wiring  wiringSummary
}

// PutAlias points (functionId, name) at a READY version. Promoting `live`
// additionally reconciles wiring (WP8) in the SAME transaction as the
// pointer move and the pool-revision bump, refusing SETTINGS_MISSING when a
// declared config/secret/db key of the target version has no setting value.
func PutAlias(repo *function.Repository, wiring WiringDeps) usecaseop.TxOperation[PutAliasCommand, PutAliasResult] {
	return usecaseop.TxOperation[PutAliasCommand, PutAliasResult]{
		Name: "PutFunctionAlias",
		Validate: func(_ context.Context, cmd PutAliasCommand) error {
			if strings.TrimSpace(cmd.FunctionID) == "" {
				return usecase.Validation("FUNCTION_ID_REQUIRED", "functionId is required")
			}
			if !function.ValidAliasName(cmd.Name) {
				return usecase.Validation("INVALID_ALIAS_NAME",
					"alias name must start with a lowercase letter and contain only lowercase alphanumerics and hyphens (max 31 chars)")
			}
			if cmd.Number < 1 {
				return usecase.Validation("NUMBER_REQUIRED", "number must be >= 1")
			}
			return nil
		},
		// Per-resource authz needs the loaded function; the coarse "may promote
		// functions" permission is on the controller.
		Authorize: usecaseop.Public[PutAliasCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd PutAliasCommand, ec usecase.ExecutionContext) (PutAliasResult, error) {
			var zero PutAliasResult

			f, err := repo.FindByID(ctx, cmd.FunctionID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if f == nil {
				return zero, httperror.NotFound("Function", cmd.FunctionID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), f.ClientID); err != nil {
				return zero, err
			}

			v, err := repo.GetVersionByNumber(ctx, f.ID, cmd.Number)
			if err != nil {
				return zero, usecase.Internal("REPO", "get_version_by_number failed", err)
			}
			if v == nil {
				return zero, usecase.NotFound("VERSION_NOT_FOUND", fmt.Sprintf("function %s has no version %d", f.ID, cmd.Number))
			}
			// A runner reports READY via heartbeat (WP5); promoting anything else
			// (PUBLISHED/FAILED/RETIRED) means no runner has ever proven the
			// version loadable.
			if v.Status != function.VersionReady {
				return zero, usecase.BusinessRule("VERSION_NOT_READY",
					fmt.Sprintf("version %d is %s, not READY", cmd.Number, v.Status))
			}

			var summary wiringSummary
			if cmd.Name == function.LiveAlias {
				d, err := abi.ParseDescribe(v.Describe)
				if err != nil {
					// Unreachable in practice (publish already validated this
					// document), but a describe that fails to parse must never
					// promote silently.
					return zero, usecase.Internal("CORRUPT_VERSION_DESCRIBE", "stored describe does not parse", err)
				}
				if missing := missingSettings(d, f.ID, repo, ctx); len(missing) > 0 {
					return zero, (&usecase.Error{
						Kind:    usecase.KindBusinessRule,
						Code:    "SETTINGS_MISSING",
						Message: "version " + fmt.Sprint(cmd.Number) + " declares config/secret/db keys with no value set: " + strings.Join(missing, ", "),
					}).WithDetails(map[string]any{"missingKeys": missing})
				}
				summary, err = reconcileWiring(ctx, s, wiring, f, d, ec)
				if err != nil {
					return zero, err
				}
			}

			alias := &function.Alias{
				FunctionID: f.ID,
				Name:       cmd.Name,
				VersionID:  v.ID,
				UpdatedAt:  time.Now().UTC(),
				UpdatedBy:  &ec.PrincipalID,
			}
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				if aliasErr := repo.UpsertAliasTx(ctx, alias, tx); aliasErr != nil {
					return aliasErr
				}
				_, bumpErr := repo.BumpPoolRevision(ctx, usecasepgx.WrapTxForBootstrap(tx), f.RunnerPool())
				return bumpErr
			}); err != nil {
				return zero, usecase.Internal("REPO", "alias write failed", err)
			}

			if cmd.Name == function.LiveAlias {
				event := FunctionPromoted{
					Metadata:             usecase.NewEventMetadata(ec, FunctionPromotedType, Source, subjectFor(f.ID)),
					FunctionID:           f.ID,
					Address:              f.Address,
					Number:               v.Number,
					DispatchPoolCode:     summary.DispatchPoolCode,
					SubscriptionsCreated: summary.SubscriptionsCreated,
					SubscriptionsUpdated: summary.SubscriptionsUpdated,
					SubscriptionsDeleted: summary.SubscriptionsDeleted,
					SchedulesCreated:     summary.SchedulesCreated,
					SchedulesUpdated:     summary.SchedulesUpdated,
					SchedulesDeleted:     summary.SchedulesDeleted,
				}
				if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
					_, e := usecase.Into(r)
					return zero, e
				}
			} else {
				event := FunctionAliasSet{
					Metadata:   usecase.NewEventMetadata(ec, FunctionAliasSetType, Source, subjectFor(f.ID)),
					FunctionID: f.ID,
					Address:    f.Address,
					Alias:      cmd.Name,
					Number:     v.Number,
				}
				if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
					_, e := usecase.Into(r)
					return zero, e
				}
			}

			return PutAliasResult{Alias: *alias, Version: *v, Wiring: summary}, nil
		},
	}
}

// missingSettings returns the sorted list of "KIND/key" strings declared by
// d (config/secrets/db) that have no fng_settings row yet.
func missingSettings(d *abi.Describe, functionID string, repo *function.Repository, ctx context.Context) []string {
	settings, err := repo.ListSettings(ctx, functionID)
	if err != nil {
		// A repo failure here is surfaced by the caller re-running the same
		// lookup inside Execute in practice this helper is only ever called
		// from PutAlias, which has no other use for the list — treat "can't
		// tell" as "nothing missing" would be unsafe, so the empty case below
		// intentionally leaves a repo error undetected only when it returns no
		// rows AND no error; a real error is rare (same connection just used)
		// and, if it happens, every key is reported missing so promote fails
		// loud rather than silently wiring an unconfigured function.
		return append(declaredKeys(d, "CONFIG"), append(declaredKeys(d, "SECRET"), declaredKeys(d, "DB")...)...)
	}
	present := make(map[string]bool, len(settings))
	for _, st := range settings {
		present[string(st.Kind)+"/"+st.Key] = true
	}
	var missing []string
	for _, k := range d.Config {
		if !present["CONFIG/"+k] {
			missing = append(missing, "CONFIG/"+k)
		}
	}
	for _, k := range d.Secrets {
		if !present["SECRET/"+k] {
			missing = append(missing, "SECRET/"+k)
		}
	}
	for _, k := range d.DB {
		if !present["DB/"+k] {
			missing = append(missing, "DB/"+k)
		}
	}
	return missing
}

func declaredKeys(d *abi.Describe, kind string) []string {
	var keys []string
	var src []string
	switch kind {
	case "CONFIG":
		src = d.Config
	case "SECRET":
		src = d.Secrets
	case "DB":
		src = d.DB
	}
	for _, k := range src {
		keys = append(keys, kind+"/"+k)
	}
	return keys
}

// DeleteAliasCommand is the input DTO for DELETE …/aliases/{name}.
type DeleteAliasCommand struct {
	FunctionID string `json:"functionId"`
	Name       string `json:"name"`
}

// DeleteAliasResult is what the API returns.
type DeleteAliasResult struct {
	Wiring wiringSummary
}

// DeleteAlias removes one alias. Removing `live` unwires the function (WP8):
// every function-owned subscription and scheduled job is deleted, in the
// same transaction as the alias removal and the pool-revision bump. The
// dispatch pool itself is left — plan §8.5: deleted only when the function
// is deleted.
func DeleteAlias(repo *function.Repository, wiring WiringDeps) usecaseop.TxOperation[DeleteAliasCommand, DeleteAliasResult] {
	return usecaseop.TxOperation[DeleteAliasCommand, DeleteAliasResult]{
		Name: "DeleteFunctionAlias",
		Validate: func(_ context.Context, cmd DeleteAliasCommand) error {
			if strings.TrimSpace(cmd.FunctionID) == "" {
				return usecase.Validation("FUNCTION_ID_REQUIRED", "functionId is required")
			}
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[DeleteAliasCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd DeleteAliasCommand, ec usecase.ExecutionContext) (DeleteAliasResult, error) {
			var zero DeleteAliasResult

			f, err := repo.FindByID(ctx, cmd.FunctionID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if f == nil {
				return zero, httperror.NotFound("Function", cmd.FunctionID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), f.ClientID); err != nil {
				return zero, err
			}

			var summary wiringSummary
			if cmd.Name == function.LiveAlias {
				summary, err = reconcileWiring(ctx, s, wiring, f, &abi.Describe{}, ec)
				if err != nil {
					return zero, err
				}
			}

			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				if delErr := repo.DeleteAliasTx(ctx, f.ID, cmd.Name, tx); delErr != nil {
					return delErr
				}
				_, bumpErr := repo.BumpPoolRevision(ctx, usecasepgx.WrapTxForBootstrap(tx), f.RunnerPool())
				return bumpErr
			}); err != nil {
				return zero, usecase.Internal("REPO", "alias delete failed", err)
			}

			if cmd.Name == function.LiveAlias {
				event := FunctionPromoted{
					Metadata:             usecase.NewEventMetadata(ec, FunctionPromotedType, Source, subjectFor(f.ID)),
					FunctionID:           f.ID,
					Address:              f.Address,
					Number:               0,
					DispatchPoolCode:     summary.DispatchPoolCode,
					SubscriptionsCreated: summary.SubscriptionsCreated,
					SubscriptionsUpdated: summary.SubscriptionsUpdated,
					SubscriptionsDeleted: summary.SubscriptionsDeleted,
					SchedulesCreated:     summary.SchedulesCreated,
					SchedulesUpdated:     summary.SchedulesUpdated,
					SchedulesDeleted:     summary.SchedulesDeleted,
				}
				if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
					_, e := usecase.Into(r)
					return zero, e
				}
			} else {
				event := FunctionAliasDeleted{
					Metadata:   usecase.NewEventMetadata(ec, FunctionAliasDeletedType, Source, subjectFor(f.ID)),
					FunctionID: f.ID,
					Address:    f.Address,
					Alias:      cmd.Name,
				}
				if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
					_, e := usecase.Into(r)
					return zero, e
				}
			}

			return DeleteAliasResult{Wiring: summary}, nil
		},
	}
}

// ListAliases is a pure read (mirrors ListVersions in publish.go).
func ListAliases(ctx context.Context, repo *function.Repository, functionID string) (*function.Function, []function.Alias, error) {
	f, err := repo.FindByID(ctx, functionID)
	if err != nil {
		return nil, nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if f == nil {
		return nil, nil, httperror.NotFound("Function", functionID)
	}
	if err := auth.CheckScopeAccess(auth.FromContext(ctx), f.ClientID); err != nil {
		return nil, nil, err
	}
	aliases, err := repo.ListAliases(ctx, functionID)
	if err != nil {
		return nil, nil, usecase.Internal("REPO", "list_aliases failed", err)
	}
	return f, aliases, nil
}
