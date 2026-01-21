package outbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// APIClient sends batches of outbox items to the FlowCatalyst API
type APIClient struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
}

// APIClientConfig holds configuration for the API client
type APIClientConfig struct {
	// BaseURL is the FlowCatalyst API base URL (required)
	BaseURL string

	// AuthToken is the optional Bearer token for authentication
	AuthToken string

	// ConnectionTimeout is the connection timeout
	ConnectionTimeout time.Duration

	// RequestTimeout is the request timeout
	RequestTimeout time.Duration
}

// DefaultAPIClientConfig returns sensible defaults
func DefaultAPIClientConfig() *APIClientConfig {
	return &APIClientConfig{
		ConnectionTimeout: 10 * time.Second,
		RequestTimeout:    30 * time.Second,
	}
}

// NewAPIClient creates a new API client
func NewAPIClient(config *APIClientConfig) *APIClient {
	if config == nil {
		config = DefaultAPIClientConfig()
	}

	return &APIClient{
		baseURL:   config.BaseURL,
		authToken: config.AuthToken,
		httpClient: &http.Client{
			Timeout: config.RequestTimeout,
		},
	}
}

// SendEventBatch sends a batch of events to the API
// POST /api/events/batch
func (c *APIClient) SendEventBatch(ctx context.Context, items []*OutboxItem) (*BatchResult, error) {
	return c.sendBatch(ctx, "/api/events/batch", items)
}

// SendDispatchJobBatch sends a batch of dispatch jobs to the API
// POST /api/dispatch/jobs/batch
func (c *APIClient) SendDispatchJobBatch(ctx context.Context, items []*OutboxItem) (*BatchResult, error) {
	return c.sendBatch(ctx, "/api/dispatch/jobs/batch", items)
}

// sendBatch sends a batch of items to the specified endpoint
func (c *APIClient) sendBatch(ctx context.Context, endpoint string, items []*OutboxItem) (*BatchResult, error) {
	if len(items) == 0 {
		return &BatchResult{}, nil
	}

	// Parse payloads into JSON array
	payloads := make([]json.RawMessage, len(items))
	for i, item := range items {
		payloads[i] = json.RawMessage(item.Payload)
	}

	body, err := json.Marshal(payloads)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal batch: %w", err)
	}

	url := c.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	slog.Debug("Sending batch to API",
		"endpoint", endpoint,
		"batchSize", len(items))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		result := NewBatchResult()
		result.Error = err
		// Mark all as failed with internal error status
		for _, item := range items {
			result.FailedItems[item.ID] = StatusInternalError
		}
		return result, err
	}
	defer resp.Body.Close()

	// Read response body
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	if resp.StatusCode >= 400 {
		err := fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
		slog.Error("API batch request failed",
			"statusCode", resp.StatusCode,
			"endpoint", endpoint,
			"response", string(respBody))
		result := NewBatchResult()
		result.Error = err
		// Determine status based on HTTP code
		status := StatusFromHTTPCode(resp.StatusCode)
		for _, item := range items {
			result.FailedItems[item.ID] = status
		}
		return result, err
	}

	slog.Debug("Batch sent successfully",
		"endpoint", endpoint,
		"batchSize", len(items),
		"statusCode", resp.StatusCode)

	result := NewBatchResult()
	result.SuccessIDs = extractIDs(items)
	return result, nil
}

// extractIDs extracts IDs from a slice of items
func extractIDs(items []*OutboxItem) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return ids
}
