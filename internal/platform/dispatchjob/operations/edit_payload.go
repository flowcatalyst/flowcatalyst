package operations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// MaxEditedPayloadBytes caps an edited payload: the same cap a function's
// fn.dispatch applies to the jobs it creates.
const MaxEditedPayloadBytes = 256 << 10

// EditPayloadCommand rewrites one job's payload. PayloadContentType nil keeps
// the job's content type.
type EditPayloadCommand struct {
	ID                 string  `json:"id"`
	Payload            string  `json:"payload"`
	PayloadContentType *string `json:"payloadContentType,omitempty"`
}

// AuditMaskedFields keeps the payload text out of aud_logs.operation_json, where
// the command is recorded: the audit row holds who, when and the sizes/digests
// (see DispatchJobPayloadEdited), not data that may be personal.
func (EditPayloadCommand) AuditMaskedFields() []string { return []string{"payload"} }

// EditDispatchJobPayload lets an operator correct a bad payload on a job that
// has not been delivered (PENDING) or has failed (FAILED / legacy ERROR),
// before retrying it. Any other status is a 409: a QUEUED or PROCESSING job is
// being delivered, and a settled COMPLETED / CANCELLED / EXPIRED job is
// history. It changes the payload (and optionally the content type) only: not
// the status, attempts or schedule; the operator retries with the ordinary
// requeue. The audit event records sizes and digests, not the payload.
func EditDispatchJobPayload(repo *dispatchjob.Repository) usecaseop.Operation[EditPayloadCommand, DispatchJobPayloadEdited] {
	persist := repo.EditPayloadPersister()
	return usecaseop.Operation[EditPayloadCommand, DispatchJobPayloadEdited]{
		Name: "EditDispatchJobPayload",
		Validate: func(_ context.Context, cmd EditPayloadCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			if cmd.Payload == "" {
				return usecase.Validation("PAYLOAD_REQUIRED", "payload is required")
			}
			if len(cmd.Payload) > MaxEditedPayloadBytes {
				return usecase.Validation("PAYLOAD_TOO_LARGE", "payload exceeds the 256 KiB limit")
			}
			if !utf8.ValidString(cmd.Payload) {
				return usecase.Validation("PAYLOAD_NOT_UTF8", "payload must be valid UTF-8 text")
			}
			if cmd.PayloadContentType != nil {
				ct := strings.TrimSpace(*cmd.PayloadContentType)
				if ct == "" || len(ct) > 100 || !strings.Contains(ct, "/") {
					return usecase.Validation("CONTENT_TYPE_INVALID", "payloadContentType must be a media type such as application/json")
				}
			}
			return nil
		},
		// Coarse permission is the controller's; resource scope is checked
		// post-load below, as the neighbouring operations do.
		Authorize: usecaseop.Public[EditPayloadCommand],
		Execute: func(ctx context.Context, cmd EditPayloadCommand, ec usecase.ExecutionContext) (usecaseop.Plan[DispatchJobPayloadEdited], error) {
			j, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if j == nil {
				return nil, httperror.NotFound("DispatchJob", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), ids.StringPtr(j.ClientID)); err != nil {
				return nil, err
			}
			if !slices.Contains(dispatchjob.EditableStatuses, j.Status) {
				return nil, usecase.Conflict("NOT_EDITABLE",
					"dispatch job is "+string(j.Status)+"; a payload can be edited only while the job is PENDING or FAILED")
			}
			contentType := j.PayloadContentType
			if cmd.PayloadContentType != nil {
				contentType = strings.TrimSpace(*cmd.PayloadContentType)
			}
			if isJSONType(contentType) && !json.Valid([]byte(cmd.Payload)) {
				return nil, usecase.Validation("PAYLOAD_NOT_JSON", "the payload is not valid JSON for content type "+contentType)
			}
			previous := ""
			if j.Payload != nil {
				previous = *j.Payload
			}
			ev := DispatchJobPayloadEdited{
				Metadata:            usecase.NewEventMetadata(ec, DispatchJobPayloadEditedType, Source, subjectFor(j.ID)),
				DispatchJobID:       j.ID,
				Status:              string(j.Status),
				PreviousSize:        len(previous),
				PreviousSHA256:      digest(previous),
				Size:                len(cmd.Payload),
				SHA256:              digest(cmd.Payload),
				PreviousContentType: j.PayloadContentType,
				ContentType:         contentType,
			}
			j.Payload = &cmd.Payload
			j.PayloadContentType = contentType
			return usecaseop.Save(j, persist, ev), nil
		},
	}
}

func digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// isJSONType reports whether a media type is JSON (application/json or a
// +json suffix type), ignoring parameters and case.
func isJSONType(ct string) bool {
	mt, _, _ := strings.Cut(strings.ToLower(ct), ";")
	mt = strings.TrimSpace(mt)
	return mt == "application/json" || strings.HasSuffix(mt, "+json")
}
