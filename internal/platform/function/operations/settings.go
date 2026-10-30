// settings.go is work package 4's settings endpoints (plan §8.2): platform-
// held config/secret/db values, keyed by (function_id, kind, key). Every
// write bumps the function's pool revision in the same transaction (a
// setting change can flip whether a promote would refuse SETTINGS_MISSING,
// and a runner needs the new value without waiting for the next promote).
package operations

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// settingKeyPattern / settingDBNamePattern are the describe key/db-name
// rules (plan §5.4: "Keys match ^[A-Za-z][A-Za-z0-9_./-]{0,99}$"; a `db`
// entry additionally matches the stricter, lowercase-only
// ^[a-z][a-z0-9_-]{0,62}$), duplicated from internal/functions/abi
// (unexported there: keyPattern / dbNamePattern) so a setting can only ever
// be written under a key/name shape a describe document could itself
// declare.
var (
	settingKeyPattern    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_./-]{0,99}$`)
	settingDBNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
)

// PutSettingCommand is the input DTO for PUT …/config/{key}, …/secrets/{key}
// and …/db/{name}.
type PutSettingCommand struct {
	FunctionID string               `json:"functionId"`
	Kind       function.SettingKind `json:"kind"`
	Key        string               `json:"key"`
	Value      string               `json:"value"`
}

// AuditMaskedFields keeps SECRET and DB values out of aud_logs and the event
// payload; a CONFIG value is not sensitive and stays visible.
func (c PutSettingCommand) AuditMaskedFields() []string {
	if c.Kind == function.SettingConfig {
		return nil
	}
	return []string{"value"}
}

// PutSetting writes one CONFIG/SECRET/DB value. SECRET and DB are encrypted
// at rest (function.Repository.UpsertSettingTx); the write and the pool
// revision bump commit atomically.
func PutSetting(repo *function.Repository) usecaseop.TxOperation[PutSettingCommand, function.Setting] {
	return usecaseop.TxOperation[PutSettingCommand, function.Setting]{
		Name: "PutFunctionSetting",
		Validate: func(_ context.Context, cmd PutSettingCommand) error {
			if strings.TrimSpace(cmd.FunctionID) == "" {
				return usecase.Validation("FUNCTION_ID_REQUIRED", "functionId is required")
			}
			if _, ok := function.ParseSettingKind(string(cmd.Kind)); !ok {
				return usecase.Validation("INVALID_KIND", "kind must be CONFIG, SECRET or DB")
			}
			pattern := settingKeyPattern
			if cmd.Kind == function.SettingDB {
				pattern = settingDBNamePattern
			}
			if !pattern.MatchString(cmd.Key) {
				return usecase.Validation("INVALID_KEY_FORMAT",
					"key/name does not match the shape a describe document could declare for this kind")
			}
			if cmd.Value == "" {
				return usecase.Validation("VALUE_REQUIRED", "value is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[PutSettingCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd PutSettingCommand, ec usecase.ExecutionContext) (function.Setting, error) {
			var zero function.Setting

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

			setting := function.Setting{
				FunctionID: f.ID,
				Kind:       cmd.Kind,
				Key:        cmd.Key,
				Value:      cmd.Value,
			}
			var configKeys []string
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				if setErr := repo.UpsertSettingTx(ctx, &setting, tx); setErr != nil {
					return setErr
				}
				if cmd.Kind == function.SettingConfig {
					keys, keysErr := repo.ListSettingKeysTx(ctx, f.ID, function.SettingConfig, tx)
					if keysErr != nil {
						return keysErr
					}
					configKeys = keys
				}
				_, bumpErr := repo.BumpPoolRevision(ctx, usecasepgx.WrapTxForBootstrap(tx), f.RunnerPool())
				return bumpErr
			}); err != nil {
				switch {
				case errors.Is(err, encryption.ErrNotConfigured):
					return zero, usecase.Validation("ENCRYPTION_NOT_CONFIGURED",
						"cannot store a SECRET or DB setting: FLOWCATALYST_APP_KEY is not configured")
				case errors.Is(err, encryption.ErrUnsupportedScheme):
					return zero, usecase.Validation("UNSUPPORTED_SECRET_SCHEME", err.Error())
				}
				return zero, usecase.Internal("REPO", "setting write failed", err)
			}

			event := settingEvent(ec, f, cmd.Kind, cmd.Key, configKeys, true)
			if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}

			// Value is never echoed back for SECRET/DB (plan §8.2: "write-only,
			// never returned"); CONFIG round-trips as given.
			out := setting
			if cmd.Kind != function.SettingConfig {
				out.Value = ""
			}
			return out, nil
		},
	}
}

// DeleteSettingCommand is the input DTO for DELETE …/config/{key},
// …/secrets/{key} and …/db/{name}.
type DeleteSettingCommand struct {
	FunctionID string               `json:"functionId"`
	Kind       function.SettingKind `json:"kind"`
	Key        string               `json:"key"`
}

// DeleteSetting removes one setting; the delete and the pool revision bump
// commit atomically.
func DeleteSetting(repo *function.Repository) usecaseop.TxOperation[DeleteSettingCommand, struct{}] {
	return usecaseop.TxOperation[DeleteSettingCommand, struct{}]{
		Name: "DeleteFunctionSetting",
		Validate: func(_ context.Context, cmd DeleteSettingCommand) error {
			if strings.TrimSpace(cmd.FunctionID) == "" {
				return usecase.Validation("FUNCTION_ID_REQUIRED", "functionId is required")
			}
			if _, ok := function.ParseSettingKind(string(cmd.Kind)); !ok {
				return usecase.Validation("INVALID_KIND", "kind must be CONFIG, SECRET or DB")
			}
			if strings.TrimSpace(cmd.Key) == "" {
				return usecase.Validation("KEY_REQUIRED", "key is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[DeleteSettingCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd DeleteSettingCommand, ec usecase.ExecutionContext) (struct{}, error) {
			var zero struct{}

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

			var configKeys []string
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				if delErr := repo.DeleteSettingTx(ctx, f.ID, cmd.Kind, cmd.Key, tx); delErr != nil {
					return delErr
				}
				if cmd.Kind == function.SettingConfig {
					keys, keysErr := repo.ListSettingKeysTx(ctx, f.ID, function.SettingConfig, tx)
					if keysErr != nil {
						return keysErr
					}
					configKeys = keys
				}
				_, bumpErr := repo.BumpPoolRevision(ctx, usecasepgx.WrapTxForBootstrap(tx), f.RunnerPool())
				return bumpErr
			}); err != nil {
				return zero, usecase.Internal("REPO", "setting delete failed", err)
			}

			event := settingEvent(ec, f, cmd.Kind, cmd.Key, configKeys, false)
			if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}
			return zero, nil
		},
	}
}

// settingEvent is the event a setting write emits (docs/platform-bug-fixes.md
// F11): a CONFIG write [ConfigUpdated] with every CONFIG key after it
// (configKeys), a SECRET write [SecretSet] / [SecretDeleted], a DB write the
// Go-only [DBSettingSet] / [DBSettingDeleted]. None carries a value.
func settingEvent(ec usecase.ExecutionContext, f *function.Function, kind function.SettingKind, key string, configKeys []string, set bool) usecase.DomainEvent {
	switch {
	case kind == function.SettingConfig:
		return ConfigUpdated{
			Metadata:   usecase.NewEventMetadata(ec, ConfigUpdatedType, Source, subjectFor(f.ID)),
			FunctionID: f.ID, Address: f.Address, Keys: configKeys,
		}
	case kind == function.SettingSecret && set:
		return SecretSet{
			Metadata:   usecase.NewEventMetadata(ec, SecretSetType, Source, subjectFor(f.ID)),
			FunctionID: f.ID, Address: f.Address, Key: key,
		}
	case kind == function.SettingSecret:
		return SecretDeleted{
			Metadata:   usecase.NewEventMetadata(ec, SecretDeletedType, Source, subjectFor(f.ID)),
			FunctionID: f.ID, Address: f.Address, Key: key,
		}
	case set:
		return DBSettingSet{
			Metadata:   usecase.NewEventMetadata(ec, DBSettingSetType, AdminSource, subjectFor(f.ID)),
			FunctionID: f.ID, Key: key,
		}
	default:
		return DBSettingDeleted{
			Metadata:   usecase.NewEventMetadata(ec, DBSettingDeletedType, AdminSource, subjectFor(f.ID)),
			FunctionID: f.ID, Key: key,
		}
	}
}

// ListSettings is a pure read (mirrors ListVersions in publish.go).
func ListSettings(ctx context.Context, repo *function.Repository, functionID string) (*function.Function, []function.Setting, error) {
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
	settings, err := repo.ListSettings(ctx, functionID)
	if err != nil {
		return nil, nil, usecase.Internal("REPO", "list_settings failed", err)
	}
	return f, settings, nil
}
