# FlowCatalyst Rust SDK

Official Rust SDK for the FlowCatalyst Platform - a multi-tenant event routing platform with webhook delivery, FIFO ordering, and comprehensive subscription management.

## Features

- **Type-safe API Client** - Auto-generated from OpenAPI spec using [Progenitor](https://github.com/oxidecomputer/progenitor)
- **OIDC Authentication** - Client credentials flow with automatic token refresh
- **Webhook Validation** - HMAC-SHA256 signature verification
- **Outbox Pattern** - Transactional outbox for reliable event publishing
- **TSID Support** - Time-sorted ID generation in Crockford Base32 format

## Installation

Add to your `Cargo.toml`:

```toml
[dependencies]
flowcatalyst-sdk = "0.1"
tokio = { version = "1.48", features = ["rt-multi-thread", "macros"] }
```

## Quick Start

```rust
use flowcatalyst_sdk::{Client, Config};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Configure the client
    let config = Config::new("https://api.flowcatalyst.tech")
        .with_client_credentials("your-client-id", "your-client-secret");

    let client = Client::new(config).await?;

    // Use the generated API client methods
    // client.event_types().list(...).await?;

    Ok(())
}
```

## Webhook Validation

Validate incoming webhooks from FlowCatalyst:

```rust
use flowcatalyst_sdk::webhook::WebhookValidator;

let validator = WebhookValidator::new("your-signing-secret");

// Extract headers from your HTTP framework
let signature = request.headers().get("X-FlowCatalyst-Signature").unwrap();
let timestamp = request.headers().get("X-FlowCatalyst-Timestamp").unwrap();
let body = request.body();

match validator.validate(signature, timestamp, body) {
    Ok(()) => println!("Valid webhook!"),
    Err(e) => println!("Invalid webhook: {}", e),
}
```

## Outbox Pattern

Use the transactional outbox for reliable event publishing:

```rust
use flowcatalyst_sdk::outbox::{OutboxManager, CreateEventDto, PostgresDriver};

// Create outbox manager
let driver = PostgresDriver::new("postgres://localhost/mydb").await?;
let outbox = OutboxManager::new(driver, "tenant_123");

// Create an event
let event = CreateEventDto::new(
    "order.created",
    serde_json::json!({"order_id": "123", "total": 99.99}),
    "orders"
)
.with_correlation_id("corr-123")
.with_source("order-service");

let message_id = outbox.create_event(event).await?;
```

## TSID Generation

Generate time-sorted IDs:

```rust
use flowcatalyst_sdk::tsid::TsidGenerator;

let generator = TsidGenerator::new();

// Generate a TSID string (13 characters, Crockford Base32)
let id = generator.generate();  // e.g., "0HZXEQ5Y8JY5Z"

// Convert between formats
let long_value = TsidGenerator::to_long(&id).unwrap();
let back_to_string = TsidGenerator::to_string(long_value);
```

## Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `base_url` | FlowCatalyst API base URL | Required |
| `client_id` | OAuth2 client ID | None |
| `client_secret` | OAuth2 client secret | None |
| `token_url` | OAuth2 token endpoint | `{base_url}/oauth/token` |
| `timeout` | Request timeout | 30 seconds |
| `retry_attempts` | Max retry attempts | 3 |
| `retry_delay` | Initial retry delay | 100ms |

## Dependencies

This SDK uses modern, actively maintained crates:

- `reqwest` 0.12 - HTTP client
- `tokio` 1.48 - Async runtime
- `serde` 1.0 - Serialization
- `chrono` 0.4 - Date/time handling
- `hmac` 0.12 / `sha2` 0.10 - Cryptography

## License

MIT
