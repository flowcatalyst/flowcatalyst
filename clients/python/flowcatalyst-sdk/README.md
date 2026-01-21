# flowcatalyst-sdk

Official Python SDK for FlowCatalyst platform.

## Installation

```bash
pip install flowcatalyst-sdk
```

## Usage

```python
from flowcatalyst import FlowCatalystClient

# Initialize the client
client = FlowCatalystClient(
    base_url="http://localhost:8080",
    api_key="your-api-key",  # optional
    timeout=30  # optional, defaults to 30s
)

# Get all event types
event_types = client.get_event_types()
for et in event_types:
    print(f"Event Type: {et.name} (v{et.version})")

# Create a new event type
new_event_type = client.create_event_type(
    name="user.created",
    version="1.0.0",
    schema={
        "type": "object",
        "properties": {
            "userId": {"type": "string"},
            "email": {"type": "string"},
        },
    },
)

# Create a subscription
subscription = client.create_subscription(
    event_type_id="event-type-id",
    endpoint="https://myapp.com/webhooks",
    status="active",
)

# Get dispatch jobs
jobs = client.get_dispatch_jobs()
for job in jobs:
    print(f"Job {job.id}: {job.status}")
```

## API Reference

### FlowCatalystClient

#### Constructor

```python
FlowCatalystClient(base_url: str, api_key: Optional[str] = None, timeout: int = 30)
```

**Parameters:**
- `base_url` (required): Base URL of the FlowCatalyst platform
- `api_key` (optional): API key for authentication
- `timeout` (optional): Request timeout in seconds (default: 30)

#### Event Types

- `get_event_types() -> List[EventType]`: Get all event types
- `get_event_type(event_type_id: str) -> EventType`: Get a specific event type
- `create_event_type(name: str, version: str, schema: dict) -> EventType`: Create a new event type

#### Subscriptions

- `get_subscriptions() -> List[Subscription]`: Get all subscriptions
- `get_subscription(subscription_id: str) -> Subscription`: Get a specific subscription
- `create_subscription(event_type_id: str, endpoint: str, status: str = "active") -> Subscription`: Create a new subscription

#### Dispatch Jobs

- `get_dispatch_jobs() -> List[DispatchJob]`: Get all dispatch jobs
- `get_dispatch_job(job_id: str) -> DispatchJob`: Get a specific dispatch job

## Type Safety

This SDK uses Pydantic for data validation and type safety. All responses are properly typed:

```python
from flowcatalyst.types import EventType, Subscription, DispatchJob
```

## Error Handling

```python
from requests.exceptions import HTTPError

try:
    event_types = client.get_event_types()
except HTTPError as e:
    print(f"API Error: {e}")
```

## Development

```bash
# Install dev dependencies
pip install -e ".[dev]"

# Run tests
pytest

# Format code
black flowcatalyst

# Type check
mypy flowcatalyst
```

## Requirements

- Python 3.9+
- requests >= 2.31.0
- pydantic >= 2.5.0

## License

Apache-2.0
