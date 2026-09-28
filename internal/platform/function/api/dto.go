// dto.go contains the wire-format types for the function API.
package api

import (
	"encoding/json"

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

// ── Versions (WP4) ──────────────────────────────────────────────────────

// PublishVersionRequest is the wire body for POST …/versions.
type PublishVersionRequest struct {
	Digest  string  `json:"digest" doc:"Lowercase hex sha256 of a previously-uploaded artifact"`
	Runtime *string `json:"runtime,omitempty" doc:"\"wasm\" (default) or \"js\""`
}

func (r PublishVersionRequest) toCommand(functionID string) operations.PublishCommand {
	cmd := operations.PublishCommand{FunctionID: functionID, Digest: r.Digest}
	if r.Runtime != nil {
		cmd.Runtime = *r.Runtime
	}
	return cmd
}

// VersionResponse mirrors function.Version.
type VersionResponse struct {
	ID          string           `json:"id"`
	FunctionID  string           `json:"functionId"`
	Number      int32            `json:"number"`
	Digest      string           `json:"digest"`
	SizeBytes   int64            `json:"sizeBytes"`
	ABI         int32            `json:"abi"`
	Runtime     string           `json:"runtime"`
	Describe    json.RawMessage  `json:"describe"`
	Status      string           `json:"status"`
	Failure     json.RawMessage  `json:"failure,omitempty"`
	ReadyAt     *httpcompat.Time `json:"readyAt,omitempty"`
	PublishedBy *string          `json:"publishedBy,omitempty"`
	CreatedAt   httpcompat.Time  `json:"createdAt"`
}

func versionResponse(v function.Version) VersionResponse {
	out := VersionResponse{
		ID:          v.ID,
		FunctionID:  v.FunctionID,
		Number:      v.Number,
		Digest:      v.Digest,
		SizeBytes:   v.SizeBytes,
		ABI:         v.ABI,
		Runtime:     v.Runtime,
		Describe:    v.Describe,
		Status:      string(v.Status),
		Failure:     v.Failure,
		PublishedBy: v.PublishedBy,
		CreatedAt:   jsontime.New(v.CreatedAt),
	}
	if v.ReadyAt != nil {
		t := jsontime.New(*v.ReadyAt)
		out.ReadyAt = &t
	}
	return out
}

// ── Aliases (WP4/WP8) ───────────────────────────────────────────────────

// PutAliasRequest is the wire body for PUT …/aliases/{name}.
type PutAliasRequest struct {
	Version int32 `json:"version"`
}

// AliasResponse mirrors function.Alias, denormalised with the version
// number it points at (the wire never needs the raw version row id).
type AliasResponse struct {
	FunctionID string          `json:"functionId"`
	Name       string          `json:"name"`
	Version    int32           `json:"version"`
	UpdatedAt  httpcompat.Time `json:"updatedAt"`
	UpdatedBy  *string         `json:"updatedBy,omitempty"`
}

// WiringResponse is the promote/unwire summary — only populated (non-zero)
// when the alias touched is `live`.
type WiringResponse struct {
	DispatchPoolCode     string `json:"dispatchPoolCode,omitempty"`
	SubscriptionsCreated int    `json:"subscriptionsCreated,omitempty"`
	SubscriptionsUpdated int    `json:"subscriptionsUpdated,omitempty"`
	SubscriptionsDeleted int    `json:"subscriptionsDeleted,omitempty"`
	SchedulesCreated     int    `json:"schedulesCreated,omitempty"`
	SchedulesUpdated     int    `json:"schedulesUpdated,omitempty"`
	SchedulesDeleted     int    `json:"schedulesDeleted,omitempty"`
}

// PutAliasResponse is the wire body for PUT …/aliases/{name}.
type PutAliasResponse struct {
	Alias AliasResponse `json:"alias"`
	// omitempty has no effect on a nested struct field (it is never
	// considered "empty"); omitzero (Go 1.24+) is the field's actual
	// omit-when-zero-value behaviour.
	Wiring WiringResponse `json:"wiring,omitzero"`
}

// ── Settings (WP4) ──────────────────────────────────────────────────────

// PutSettingRequest is the wire body for PUT …/config/{key}, …/secrets/{key}
// and …/db/{name}.
type PutSettingRequest struct {
	Value string `json:"value"`
}

// SettingResponse mirrors function.Setting. Value is empty for SECRET/DB
// kinds — plan §8.2: "Secrets are write-only and never returned."
type SettingResponse struct {
	Kind      string          `json:"kind"`
	Key       string          `json:"key"`
	Value     string          `json:"value,omitempty"`
	UpdatedAt httpcompat.Time `json:"updatedAt"`
}

func settingResponse(s function.Setting) SettingResponse {
	return SettingResponse{
		Kind:      string(s.Kind),
		Key:       s.Key,
		Value:     s.Value,
		UpdatedAt: jsontime.New(s.UpdatedAt),
	}
}

// ── Public routes ────────────────────────────────────────────────────────

// PutRouteRequest is the wire body for PUT /api/functions/{id}/routes.
type PutRouteRequest struct {
	Hostname   string `json:"hostname" doc:"A lowercase DNS hostname covered by a claimed zone (no port, no wildcard)"`
	PathPrefix string `json:"pathPrefix" doc:"The path prefix routed to this function, e.g. \"/\" or \"/webhooks\""`
	Alias      string `json:"alias,omitempty" doc:"An existing alias name to route to; omitted routes to live"`
}

func (r PutRouteRequest) toCommand(functionID string) operations.PutRouteCommand {
	return operations.PutRouteCommand{
		FunctionID: functionID,
		Hostname:   r.Hostname,
		PathPrefix: r.PathPrefix,
		Alias:      r.Alias,
	}
}

// RouteResponse mirrors function.Route.
type RouteResponse struct {
	ID         string          `json:"id"`
	FunctionID string          `json:"functionId"`
	Hostname   string          `json:"hostname"`
	PathPrefix string          `json:"pathPrefix"`
	Alias      *string         `json:"alias,omitempty"`
	CreatedBy  *string         `json:"createdBy,omitempty"`
	CreatedAt  httpcompat.Time `json:"createdAt"`
	UpdatedAt  httpcompat.Time `json:"updatedAt"`
}

func routeResponse(r function.Route) RouteResponse {
	return RouteResponse{
		ID:         r.ID,
		FunctionID: r.FunctionID,
		Hostname:   r.Hostname,
		PathPrefix: r.PathPrefix,
		Alias:      r.Alias,
		CreatedBy:  r.CreatedBy,
		CreatedAt:  jsontime.New(r.CreatedAt),
		UpdatedAt:  jsontime.New(r.UpdatedAt),
	}
}
