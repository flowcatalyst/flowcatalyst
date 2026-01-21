# FlowCatalyst Go SDK

Official Go SDK for FlowCatalyst platform.

## Installation

```bash
go get github.com/yourusername/flowcatalyst-sdk-go
```

## Usage

```go
package main

import (
    "fmt"
    "log"
    "time"

    flowcatalyst "github.com/yourusername/flowcatalyst-sdk-go"
)

func main() {
    // Initialize the client
    client := flowcatalyst.NewClient(flowcatalyst.Config{
        BaseURL: "http://localhost:8080",
        APIKey:  "your-api-key", // optional
        Timeout: 30 * time.Second,
    })

    // Get all event types
    eventTypes, err := client.GetEventTypes()
    if err != nil {
        log.Fatal(err)
    }
    for _, et := range eventTypes {
        fmt.Printf("Event Type: %s (v%s)\n", et.Name, et.Version)
    }

    // Create a new event type
    newEventType, err := client.CreateEventType(flowcatalyst.CreateEventTypeRequest{
        Name:    "user.created",
        Version: "1.0.0",
        Schema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "userId": map[string]interface{}{"type": "string"},
                "email":  map[string]interface{}{"type": "string"},
            },
        },
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Created event type: %s\n", newEventType.ID)

    // Create a subscription
    subscription, err := client.CreateSubscription(flowcatalyst.CreateSubscriptionRequest{
        EventTypeID: "event-type-id",
        Endpoint:    "https://myapp.com/webhooks",
        Status:      "active",
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Created subscription: %s\n", subscription.ID)

    // Get dispatch jobs
    jobs, err := client.GetDispatchJobs()
    if err != nil {
        log.Fatal(err)
    }
    for _, job := range jobs {
        fmt.Printf("Job %s: %s\n", job.ID, job.Status)
    }
}
```

## API Reference

### Client

#### NewClient

```go
func NewClient(config Config) *Client
```

Creates a new FlowCatalyst client.

**Config:**
- `BaseURL` (required): Base URL of the FlowCatalyst platform
- `APIKey` (optional): API key for authentication
- `Timeout` (optional): Request timeout (default: 30s)

#### Event Types

- `GetEventTypes() ([]EventType, error)`: Get all event types
- `GetEventType(id string) (*EventType, error)`: Get a specific event type
- `CreateEventType(request CreateEventTypeRequest) (*EventType, error)`: Create a new event type

#### Subscriptions

- `GetSubscriptions() ([]Subscription, error)`: Get all subscriptions
- `GetSubscription(id string) (*Subscription, error)`: Get a specific subscription
- `CreateSubscription(request CreateSubscriptionRequest) (*Subscription, error)`: Create a new subscription

#### Dispatch Jobs

- `GetDispatchJobs() ([]DispatchJob, error)`: Get all dispatch jobs
- `GetDispatchJob(id string) (*DispatchJob, error)`: Get a specific dispatch job

## Types

All types are properly defined with JSON tags for serialization:

- `EventType`
- `Subscription`
- `DispatchJob`
- `CreateEventTypeRequest`
- `CreateSubscriptionRequest`

## Error Handling

```go
eventTypes, err := client.GetEventTypes()
if err != nil {
    log.Printf("Error: %v", err)
    return
}
```

## Requirements

- Go 1.21+

## License

Apache-2.0
