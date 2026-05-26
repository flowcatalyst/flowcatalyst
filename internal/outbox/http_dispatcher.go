package outbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// HTTPDispatcher sends outbox items to the FlowCatalyst platform API.
// Mirrors fc-outbox/src/http_dispatcher.rs.
type HTTPDispatcher struct {
	platformURL string
	authToken   string
	client      *http.Client
}

// NewHTTPDispatcher wires a dispatcher.
func NewHTTPDispatcher(platformURL, authToken string, timeout time.Duration) *HTTPDispatcher {
	return &HTTPDispatcher{
		platformURL: platformURL,
		authToken:   authToken,
		client:      &http.Client{Timeout: timeout},
	}
}

// DispatchOutcome is the result of sending one item.
type DispatchOutcome struct {
	Status  common.OutboxStatus
	Message string
}

// Send POSTs the item's payload to the appropriate batch endpoint and
// classifies the response into an OutboxStatus.
func (d *HTTPDispatcher) Send(ctx context.Context, item Item) DispatchOutcome {
	endpoint := d.platformURL + item.ItemType.APIPath()
	body, err := json.Marshal(map[string]any{"items": []json.RawMessage{item.Payload}})
	if err != nil {
		return DispatchOutcome{Status: common.OutboxBadRequest, Message: "marshal: " + err.Error()}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return DispatchOutcome{Status: common.OutboxInternalError, Message: "build: " + err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	if d.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.authToken)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return DispatchOutcome{Status: common.OutboxInternalError, Message: "request: " + err.Error()}
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return DispatchOutcome{Status: common.OutboxSuccess}
	case resp.StatusCode == http.StatusUnauthorized:
		return DispatchOutcome{Status: common.OutboxUnauthorized, Message: "401"}
	case resp.StatusCode == http.StatusForbidden:
		return DispatchOutcome{Status: common.OutboxForbidden, Message: "403"}
	case resp.StatusCode == http.StatusBadGateway,
		resp.StatusCode == http.StatusServiceUnavailable,
		resp.StatusCode == http.StatusGatewayTimeout:
		return DispatchOutcome{Status: common.OutboxGatewayError, Message: fmt.Sprintf("%d", resp.StatusCode)}
	case resp.StatusCode >= 400 && resp.StatusCode < 500:
		return DispatchOutcome{Status: common.OutboxBadRequest, Message: fmt.Sprintf("%d", resp.StatusCode)}
	default:
		return DispatchOutcome{Status: common.OutboxInternalError, Message: fmt.Sprintf("%d", resp.StatusCode)}
	}
}
