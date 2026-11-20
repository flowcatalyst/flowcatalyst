package operations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// MaxEditedPayloadBytes caps an edited payload when the repository has no
// payload service: the largest payload the database takes by default. With a
// service the cap is its largest payload accepted
// (dispatchjob.Repository.MaxPayloadBytes: FC_PAYLOAD_MAX_BYTES while
// offloading, which needs the payload table written, and
// FC_PAYLOAD_INLINE_MAX_BYTES otherwise, the switch off included), so a large
// payload can be corrected (docs/payload-offload-design.md, section 8).
const MaxEditedPayloadBytes = payloadstore.DefaultInlineMaxBytes

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
// requeue. The audit event records sizes and digests, not the payload, and
// the ids of the payload rows before and after the edit.
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
			if limit := repo.MaxPayloadBytes(); len(cmd.Payload) > limit {
				return usecase.Validation("PAYLOAD_TOO_LARGE", fmt.Sprintf("payload exceeds the %d-byte limit", limit))
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
			// The payload row the job points at now, before the edit
			// replaces it ("" for a payload in the legacy column).
			previousPayloadID := ""
			if j.StoredPayload != nil {
				previousPayloadID = j.StoredPayload.ID
			}
			prevSize, prevDigest, err := previousPayload(ctx, repo, j)
			if err != nil {
				return nil, err
			}
			ev := DispatchJobPayloadEdited{
				Metadata:            usecase.NewEventMetadata(ec, DispatchJobPayloadEditedType, Source, subjectFor(j.ID)),
				DispatchJobID:       j.ID,
				Status:              string(j.Status),
				PreviousSize:        prevSize,
				PreviousSHA256:      prevDigest,
				Size:                len(cmd.Payload),
				SHA256:              digest(cmd.Payload),
				PreviousContentType: j.PayloadContentType,
				ContentType:         contentType,
				PreviousPayloadID:   previousPayloadID,
			}
			j.Payload = &cmd.Payload
			j.PayloadContentType = contentType
			if err := prepareEditedPayload(ctx, repo, j); err != nil {
				return nil, err
			}
			// The edit's own payload row, when the payload table is written;
			// with it not written the payload column is written and
			// payload_id cleared, and there is none.
			if j.NewPayload != nil {
				ev.PayloadID = j.NewPayload.ID
			}
			return usecaseop.Save(j, persist, ev), nil
		},
	}
}

// previousPayload answers the size and digest of the payload an edit replaces,
// for the audit event. A payload that was offloaded is read from the payload
// store first (docs/payload-offload-design.md). A store that cannot be read
// right now refuses the edit (PAYLOAD_STORE_UNAVAILABLE, try again), so the
// audit never loses the digest to a passing fault. A payload that is gone for
// good (payloadstore.ErrMissing, or a reference with no store configured)
// does not block the edit, which is how an operator repairs a job that failed
// PAYLOAD_MISSING: the size is the payload row's and the digest is empty
// (unknown).
func previousPayload(ctx context.Context, repo *dispatchjob.Repository, j *dispatchjob.DispatchJob) (int, string, error) {
	err := repo.ResolvePayload(ctx, j)
	switch {
	case err == nil:
		if j.Payload == nil {
			return 0, digest(""), nil
		}
		return len(*j.Payload), digest(*j.Payload), nil
	case errors.Is(err, payloadstore.ErrMissing), errors.Is(err, payloadstore.ErrStoreNotConfigured):
		return j.PayloadLength(), "", nil
	default:
		return 0, "", usecase.Internal("PAYLOAD_STORE_UNAVAILABLE",
			"the job's current payload could not be read from the payload store; try again", err)
	}
}

// prepareEditedPayload readies the edited payload (j.Payload) as a new
// msg_payloads row of the job's own, under its created_at, when the payload
// table is written (docs/payload-offload-design.md, section 3.1). It runs in
// Execute, before the transaction, because it may write the payload store;
// the persister inserts the row and re-points the job in one statement. The
// event and the other jobs that shared the job's payload row keep it. With
// the payload table not written nothing is prepared and the persister writes
// the payload column.
func prepareEditedPayload(ctx context.Context, repo *dispatchjob.Repository, j *dispatchjob.DispatchJob) error {
	if !repo.WritesPayloadTable() {
		return nil
	}
	one := []dispatchjob.DispatchJob{*j}
	err := repo.PreparePayloads(ctx, one)[0]
	switch {
	case err == nil:
		j.NewPayload = one[0].NewPayload
		return nil
	case errors.Is(err, payloadstore.ErrTooLarge):
		return usecase.Validation("PAYLOAD_TOO_LARGE", err.Error())
	case errors.Is(err, payloadstore.ErrNotText):
		return usecase.Validation("PAYLOAD_NOT_UTF8", "payload must be valid UTF-8 text with no NUL byte")
	default:
		return usecase.Internal("PAYLOAD_STORE_UNAVAILABLE",
			"the edited payload could not be written to the payload store; try again", err)
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
