package flowcatalyst

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client represents a FlowCatalyst API client
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// Config holds client configuration
type Config struct {
	BaseURL string
	APIKey  string
	Timeout time.Duration
}

// NewClient creates a new FlowCatalyst client
func NewClient(config Config) *Client {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	return &Client{
		baseURL: config.BaseURL,
		apiKey:  config.APIKey,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// GetEventTypes retrieves all event types
func (c *Client) GetEventTypes() ([]EventType, error) {
	var eventTypes []EventType
	err := c.doRequest("GET", "/api/event-types", nil, &eventTypes)
	return eventTypes, err
}

// GetEventType retrieves a specific event type
func (c *Client) GetEventType(id string) (*EventType, error) {
	var eventType EventType
	err := c.doRequest("GET", fmt.Sprintf("/api/event-types/%s", id), nil, &eventType)
	return &eventType, err
}

// CreateEventType creates a new event type
func (c *Client) CreateEventType(eventType CreateEventTypeRequest) (*EventType, error) {
	var result EventType
	err := c.doRequest("POST", "/api/event-types", eventType, &result)
	return &result, err
}

// GetSubscriptions retrieves all subscriptions
func (c *Client) GetSubscriptions() ([]Subscription, error) {
	var subscriptions []Subscription
	err := c.doRequest("GET", "/api/subscriptions", nil, &subscriptions)
	return subscriptions, err
}

// GetSubscription retrieves a specific subscription
func (c *Client) GetSubscription(id string) (*Subscription, error) {
	var subscription Subscription
	err := c.doRequest("GET", fmt.Sprintf("/api/subscriptions/%s", id), nil, &subscription)
	return &subscription, err
}

// CreateSubscription creates a new subscription
func (c *Client) CreateSubscription(subscription CreateSubscriptionRequest) (*Subscription, error) {
	var result Subscription
	err := c.doRequest("POST", "/api/subscriptions", subscription, &result)
	return &result, err
}

// GetDispatchJobs retrieves all dispatch jobs
func (c *Client) GetDispatchJobs() ([]DispatchJob, error) {
	var jobs []DispatchJob
	err := c.doRequest("GET", "/api/dispatch-jobs", nil, &jobs)
	return jobs, err
}

// GetDispatchJob retrieves a specific dispatch job
func (c *Client) GetDispatchJob(id string) (*DispatchJob, error) {
	var job DispatchJob
	err := c.doRequest("GET", fmt.Sprintf("/api/dispatch-jobs/%s", id), nil, &job)
	return &job, err
}

// doRequest performs an HTTP request
func (c *Client) doRequest(method, path string, body interface{}, result interface{}) error {
	url := c.baseURL + path

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}
