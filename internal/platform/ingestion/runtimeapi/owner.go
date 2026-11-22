package runtimeapi

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// A runtime id belongs to the service account whose poll first registered it
// (docs/ingestion-design.md section 7, "The runtime id's owner"). Every route
// that names a runtime id refuses it to any other account with
// 403 RUNTIME_ID_NOT_OWNED, after the runtime permission and before anything
// else, and the refusal is audited: on the secret route as a refused fetch
// (SECRET_FETCH_REFUSED, its reason the code), on every other route as
// RUNTIME_ID_REFUSED on the runtime id.

// RuntimeIDRefusedOperation is the aud_logs operation of a call refused because
// its runtime id is registered to another account (entity type
// IngestionRuntime, entity id the runtime id).
const RuntimeIDRefusedOperation = "RUNTIME_ID_REFUSED"

// RuntimeIDRefusedEvent is the event name (the message and the "event" field)
// of the WARN log line such a refusal writes.
const RuntimeIDRefusedEvent = "security.ingestion_runtime_id_refused"

// The operation ids of the state routes, named in a refusal's audit row.
const (
	GetStateOperationID = "ingestionRuntimeGetState"
	PutStateOperationID = "ingestionRuntimePutState"
)

// ownRuntime refuses a runtime id registered to an account other than the
// caller's (RUNTIME_ID_NOT_OWNED). It audits nothing: the caller does.
func (s *State) ownRuntime(ctx context.Context, runtimeID string) error {
	err := s.Repo.CheckRuntimeOwner(ctx, runtimeID, principalOf(auth.FromContext(ctx)))
	if err != nil && usecase.AsError(err) == nil {
		return usecase.Internal("REPO", "runtime owner check failed", err)
	}
	return err
}

// requireRuntime is the start of every runtime route but the secret fetch: the
// runtime permission, then the runtime id's owner, a refusal of which is
// audited as RUNTIME_ID_REFUSED. ingestionID is the ingestion the call names,
// empty when it names none.
func (s *State) requireRuntime(ctx context.Context, runtimeID, operation, ingestionID string) error {
	if err := auth.RequireIngestionRuntime(auth.FromContext(ctx)); err != nil {
		return err
	}
	err := s.ownRuntime(ctx, runtimeID)
	s.auditRuntimeRefusal(ctx, runtimeID, operation, ingestionID, err)
	return err
}

// auditRuntimeRefusal logs at WARN and audits a call refused because its
// runtime id is registered to another account; any other err (nil included)
// is left alone. An audit row that cannot be written does not change the
// answer and is logged.
func (s *State) auditRuntimeRefusal(ctx context.Context, runtimeID, operation, ingestionID string, err error) {
	if !ingestion.IsRuntimeNotOwned(err) {
		return
	}
	e := usecase.AsError(err)
	principal := principalOf(auth.FromContext(ctx))
	attrs := []any{
		"event", RuntimeIDRefusedEvent, "runtimeId", runtimeID, "principalId", principal,
		"operation", operation, "reason", e.Code,
	}
	if ingestionID != "" {
		attrs = append(attrs, "ingestionId", ingestionID)
	}
	slog.WarnContext(ctx, RuntimeIDRefusedEvent, attrs...)
	if s.Audit == nil {
		slog.ErrorContext(ctx, "ingestion runtime id refusal not audited: the runtime API has no audit log wired",
			"runtimeId", runtimeID, "operation", operation)
		return
	}
	detail := map[string]string{"runtimeId": runtimeID, "operation": operation, "reason": e.Code, "message": e.Message}
	if ingestionID != "" {
		detail["ingestionId"] = ingestionID
	}
	operationJSON, _ := json.Marshal(detail)
	row := &audit.Log{
		ID:            tsid.Generate(tsid.AuditLog),
		EntityType:    "IngestionRuntime",
		EntityID:      runtimeID,
		Operation:     RuntimeIDRefusedOperation,
		OperationJSON: operationJSON,
		PerformedAt:   time.Now().UTC(),
	}
	if principal != "" {
		row.PrincipalID = &principal
	}
	if err := s.Audit.Insert(ctx, row); err != nil {
		slog.ErrorContext(ctx, "ingestion runtime id refusal not audited", "runtimeId", runtimeID,
			"operation", operation, "err", err)
	}
}
