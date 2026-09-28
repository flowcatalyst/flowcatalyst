package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// LimitsInput is the wire/command shape for [function.Limits]. A nil field
// means "use the default" on create, "leave unchanged" on update.
type LimitsInput struct {
	MemoryMB       *int32 `json:"memoryMb,omitempty"`
	MaxConcurrency *int32 `json:"maxConcurrency,omitempty"`
	TimeoutMs      *int32 `json:"timeoutMs,omitempty"`
	MaxBodyBytes   *int64 `json:"maxBodyBytes,omitempty"`
}

// applyTo merges non-nil fields of li onto base, returning the result.
func (li *LimitsInput) applyTo(base function.Limits) function.Limits {
	if li == nil {
		return base
	}
	if li.MemoryMB != nil {
		base.MemoryMB = *li.MemoryMB
	}
	if li.MaxConcurrency != nil {
		base.MaxConcurrency = *li.MaxConcurrency
	}
	if li.TimeoutMs != nil {
		base.TimeoutMs = *li.TimeoutMs
	}
	if li.MaxBodyBytes != nil {
		base.MaxBodyBytes = *li.MaxBodyBytes
	}
	return base
}

// validateLimits checks l against the platform ceilings (plan §7.1). Lives
// here (not on function.Limits) so entity.go stays free of the
// operations-layer error taxonomy — see docs/usecase-pattern.md.
func validateLimits(l function.Limits) error {
	switch {
	case l.MemoryMB < 1:
		return usecase.Validation("MEMORY_MB_REQUIRED", "memoryMb must be >= 1")
	case l.MemoryMB > function.MaxMemoryMB:
		return usecase.Validation("MEMORY_MB_TOO_LARGE", "memoryMb exceeds the platform ceiling")
	case l.MaxConcurrency < 1:
		return usecase.Validation("MAX_CONCURRENCY_REQUIRED", "maxConcurrency must be >= 1")
	case l.MaxConcurrency > function.MaxMaxConcurrency:
		return usecase.Validation("MAX_CONCURRENCY_TOO_LARGE", "maxConcurrency exceeds the platform ceiling")
	case l.TimeoutMs < 1:
		return usecase.Validation("TIMEOUT_MS_REQUIRED", "timeoutMs must be >= 1")
	case l.TimeoutMs > function.MaxTimeoutMs:
		return usecase.Validation("TIMEOUT_MS_TOO_LARGE", "timeoutMs exceeds the platform ceiling")
	case l.MaxBodyBytes < 1:
		return usecase.Validation("MAX_BODY_BYTES_REQUIRED", "maxBodyBytes must be >= 1")
	case l.MaxBodyBytes > function.MaxMaxBodyBytes:
		return usecase.Validation("MAX_BODY_BYTES_TOO_LARGE", "maxBodyBytes exceeds the platform ceiling")
	}
	return nil
}

// CreateCommand is the input DTO.
type CreateCommand struct {
	ApplicationID string       `json:"applicationId"`
	Name          string       `json:"name"`
	ClientID      *string      `json:"clientId,omitempty"`
	Description   *string      `json:"description,omitempty"`
	Pool          *string      `json:"pool,omitempty"`
	Warm          *bool        `json:"warm,omitempty"`
	Limits        *LimitsInput `json:"limits,omitempty"`
}

// CreateFunction validates cmd, enforces per-resource client scope,
// resolves the owning application (for the address's applicationCode
// label), enforces address uniqueness, persists the function, and
// atomically emits [FunctionCreated].
func CreateFunction(repo *function.Repository, apps *application.Repository) usecaseop.Operation[CreateCommand, FunctionCreated] {
	return usecaseop.Operation[CreateCommand, FunctionCreated]{
		Name: "CreateFunction",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			if strings.TrimSpace(cmd.ApplicationID) == "" {
				return usecase.Validation("APPLICATION_ID_REQUIRED", "applicationId is required")
			}
			name := strings.TrimSpace(cmd.Name)
			if name == "" {
				return usecase.Validation("NAME_REQUIRED", "name is required")
			}
			if !function.ValidName(name) {
				return usecase.Validation("INVALID_NAME_FORMAT",
					"name must start with a lowercase letter and contain only lowercase alphanumerics and hyphens (max 63 chars)")
			}
			limits := cmd.Limits.applyTo(function.DefaultLimits())
			return validateLimits(limits)
		},
		// Resource-level authorization (the coarse "may manage functions"
		// permission is enforced at the controller). A function bound to a
		// client may only be created by a principal with access to that
		// client; a platform-owned function (nil ClientID) requires anchor —
		// exactly auth.CheckScopeAccess on the target client.
		Authorize: func(ctx context.Context, cmd CreateCommand) error {
			return auth.CheckScopeAccess(auth.FromContext(ctx), cmd.ClientID)
		},
		Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[FunctionCreated], error) {
			app, err := apps.FindByID(ctx, cmd.ApplicationID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_application_by_id failed", err)
			}
			if app == nil {
				return nil, httperror.NotFound("Application", cmd.ApplicationID)
			}

			name := strings.TrimSpace(cmd.Name)
			address := function.BuildAddress(app.Code, name)
			existing, err := repo.FindByAddress(ctx, address)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_address failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("ADDRESS_EXISTS", "Function with address '"+address+"' already exists")
			}

			f := function.New(cmd.ApplicationID, app.Code, name)
			f.ClientID = cmd.ClientID
			f.Description = cmd.Description
			f.Pool = cmd.Pool
			if cmd.Warm != nil {
				f.Warm = *cmd.Warm
			}
			f.Limits = cmd.Limits.applyTo(f.Limits)
			f.CreatedBy = &ec.PrincipalID

			event := FunctionCreated{
				Metadata:   usecase.NewEventMetadata(ec, FunctionCreatedType, Source, subjectFor(f.ID)),
				FunctionID: f.ID,
				Address:    f.Address,
				Name:       f.Name,
			}
			return usecaseop.Save(f, repo, event), nil
		},
	}
}
