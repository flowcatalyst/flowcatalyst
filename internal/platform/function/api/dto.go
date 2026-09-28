// dto.go contains the wire-format types for the function API.
package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// LimitsDTO is the wire shape of [function.Limits], reused for both the
// create/update request (as *LimitsDTO, all fields optional — omitted
// means "default"/"unchanged") and the response (all fields set).
type LimitsDTO struct {
	MemoryMB       *int32 `json:"memoryMb,omitempty"`
	MaxConcurrency *int32 `json:"maxConcurrency,omitempty"`
	TimeoutMs      *int32 `json:"timeoutMs,omitempty"`
	MaxBodyBytes   *int64 `json:"maxBodyBytes,omitempty"`
}

func (l *LimitsDTO) toCommand() *operations.LimitsInput {
	if l == nil {
		return nil
	}
	return &operations.LimitsInput{
		MemoryMB:       l.MemoryMB,
		MaxConcurrency: l.MaxConcurrency,
		TimeoutMs:      l.TimeoutMs,
		MaxBodyBytes:   l.MaxBodyBytes,
	}
}

func limitsResponse(l function.Limits) LimitsDTO {
	return LimitsDTO{
		MemoryMB:       &l.MemoryMB,
		MaxConcurrency: &l.MaxConcurrency,
		TimeoutMs:      &l.TimeoutMs,
		MaxBodyBytes:   &l.MaxBodyBytes,
	}
}

// CreateFunctionRequest is the wire body for POST /api/functions.
type CreateFunctionRequest struct {
	ApplicationID string     `json:"applicationId" doc:"Owning application id"`
	Name          string     `json:"name" doc:"Function name (lowercase, alphanumeric, hyphens; matches ^[a-z][a-z0-9-]{0,62}$)"`
	ClientID      *string    `json:"clientId,omitempty"`
	Description   *string    `json:"description,omitempty"`
	Pool          *string    `json:"pool,omitempty" doc:"Explicit dispatch pool override; omitted uses the function's implied pool"`
	Warm          *bool      `json:"warm,omitempty"`
	Limits        *LimitsDTO `json:"limits,omitempty"`
}

func (r CreateFunctionRequest) toCommand() operations.CreateCommand {
	return operations.CreateCommand{
		ApplicationID: r.ApplicationID,
		Name:          r.Name,
		ClientID:      r.ClientID,
		Description:   r.Description,
		Pool:          r.Pool,
		Warm:          r.Warm,
		Limits:        r.Limits.toCommand(),
	}
}

// UpdateFunctionRequest is the wire body for PATCH /api/functions/{id}.
type UpdateFunctionRequest struct {
	Description *string    `json:"description,omitempty"`
	Pool        *string    `json:"pool,omitempty"`
	ClearPool   bool       `json:"clearPool,omitempty" doc:"Set true to clear an explicit pool override back to the implied pool"`
	Warm        *bool      `json:"warm,omitempty"`
	Limits      *LimitsDTO `json:"limits,omitempty"`
}

func (r UpdateFunctionRequest) toCommand(id string) operations.UpdateCommand {
	return operations.UpdateCommand{
		ID:          id,
		Description: r.Description,
		Pool:        r.Pool,
		ClearPool:   r.ClearPool,
		Warm:        r.Warm,
		Limits:      r.Limits.toCommand(),
	}
}

// FunctionResponse mirrors function.Function.
type FunctionResponse struct {
	ID              string          `json:"id"`
	ApplicationID   string          `json:"applicationId"`
	ApplicationCode string          `json:"applicationCode"`
	ClientID        *string         `json:"clientId,omitempty"`
	Name            string          `json:"name"`
	Address         string          `json:"address"`
	Description     *string         `json:"description,omitempty"`
	Pool            *string         `json:"pool,omitempty"`
	Warm            bool            `json:"warm"`
	Limits          LimitsDTO       `json:"limits"`
	CreatedBy       *string         `json:"createdBy,omitempty"`
	CreatedAt       httpcompat.Time `json:"createdAt"`
	UpdatedAt       httpcompat.Time `json:"updatedAt"`
}

func fromEntity(f *function.Function) FunctionResponse {
	return FunctionResponse{
		ID:              f.ID,
		ApplicationID:   f.ApplicationID,
		ApplicationCode: f.ApplicationCode,
		ClientID:        f.ClientID,
		Name:            f.Name,
		Address:         f.Address,
		Description:     f.Description,
		Pool:            f.Pool,
		Warm:            f.Warm,
		Limits:          limitsResponse(f.Limits),
		CreatedBy:       f.CreatedBy,
		CreatedAt:       jsontime.New(f.CreatedAt),
		UpdatedAt:       jsontime.New(f.UpdatedAt),
	}
}
