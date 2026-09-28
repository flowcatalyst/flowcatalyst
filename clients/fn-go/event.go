package fn

import (
	"context"
	"encoding/json"
)

// Event is an event to publish via Emit (plan §5.3 op 5, event.emit). Type
// must be one declared with Emits.
type Event struct {
	Type          string
	Source        string
	Subject       string
	DedupID       string
	CorrelationID string
	CausationID   string
	MessageGroup  string
	ContentType   string
	Data          []byte
}

type eventEmitMeta struct {
	Type          string `json:"type"`
	Source        string `json:"source,omitempty"`
	Subject       string `json:"subject,omitempty"`
	DedupID       string `json:"dedupId,omitempty"`
	CorrelationID string `json:"correlationId,omitempty"`
	CausationID   string `json:"causationId,omitempty"`
	MessageGroup  string `json:"messageGroup,omitempty"`
	ContentType   string `json:"contentType,omitempty"`
}

type eventEmitResult struct {
	EventID string `json:"eventId"`
}

// Emit publishes e and returns the platform-assigned event id. Errors wrap
// *Error; errors.Is(err, ErrRetryable) is true when the failure is
// UNAVAILABLE (platform trouble, safe to retry) as opposed to a permanent
// refusal such as NOT_ALLOWED (the type isn't in Emits/isn't owned by the
// function's application).
func Emit(ctx context.Context, e Event) (eventID string, err error) {
	meta := eventEmitMeta{
		Type:          e.Type,
		Source:        e.Source,
		Subject:       e.Subject,
		DedupID:       e.DedupID,
		CorrelationID: e.CorrelationID,
		CausationID:   e.CausationID,
		MessageGroup:  e.MessageGroup,
		ContentType:   e.ContentType,
	}
	mb, merr := json.Marshal(meta)
	if merr != nil {
		return "", merr
	}
	rm, _, hostErr, err := currentHost.call(opEventEmit, mb, e.Data)
	if err != nil {
		return "", err
	}
	if hostErr != nil {
		return "", hostErr
	}
	var out eventEmitResult
	if len(rm) > 0 {
		if uerr := json.Unmarshal(rm, &out); uerr != nil {
			return "", uerr
		}
	}
	return out.EventID, nil
}
