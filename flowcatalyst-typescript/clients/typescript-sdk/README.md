# @flowcatalyst/sdk

Official TypeScript/JavaScript SDK for FlowCatalyst platform.

## Installation

```bash
npm install @flowcatalyst/sdk
# or
yarn add @flowcatalyst/sdk
# or
bun add @flowcatalyst/sdk
```

## Usage

```typescript
import { FlowCatalystClient } from '@flowcatalyst/sdk'

// Initialize the client
const client = new FlowCatalystClient({
  baseUrl: 'http://localhost:8080',
  apiKey: 'your-api-key', // optional
  timeout: 30000, // optional, defaults to 30s
})

// Get all event types
const { data: eventTypes, error } = await client.getEventTypes()
if (error) {
  console.error('Error:', error)
} else {
  console.log('Event types:', eventTypes)
}

// Create a new event type
const { data: newEventType, error: createError } = await client.createEventType({
  name: 'user.created',
  version: '1.0.0',
  schema: {
    type: 'object',
    properties: {
      userId: { type: 'string' },
      email: { type: 'string' },
    },
  },
})

// Create a subscription
const { data: subscription } = await client.createSubscription({
  eventTypeId: 'event-type-id',
  endpoint: 'https://myapp.com/webhooks',
  status: 'active',
})

// Get dispatch jobs
const { data: jobs } = await client.getDispatchJobs()
```

## API Reference

### FlowCatalystClient

#### Constructor

```typescript
new FlowCatalystClient(config: FlowCatalystConfig)
```

**Config Options:**
- `baseUrl` (required): Base URL of the FlowCatalyst platform
- `apiKey` (optional): API key for authentication
- `timeout` (optional): Request timeout in milliseconds (default: 30000)

#### Event Types

- `getEventTypes()`: Get all event types
- `getEventType(id)`: Get a specific event type
- `createEventType(eventType)`: Create a new event type

#### Subscriptions

- `getSubscriptions()`: Get all subscriptions
- `getSubscription(id)`: Get a specific subscription
- `createSubscription(subscription)`: Create a new subscription

#### Dispatch Jobs

- `getDispatchJobs()`: Get all dispatch jobs
- `getDispatchJob(id)`: Get a specific dispatch job

## TypeScript Support

This SDK is written in TypeScript and provides full type definitions. All API responses are properly typed.

```typescript
import type { EventType, Subscription, DispatchJob } from '@flowcatalyst/sdk'
```

## Error Handling

All API methods return a response object with either `data` or `error`:

```typescript
const { data, error } = await client.getEventTypes()

if (error) {
  // Handle error
  console.error('API Error:', error)
} else {
  // Use data
  console.log('Event types:', data)
}
```

## Development

```bash
# Install dependencies
npm install

# Build
npm run build

# Watch mode
npm run dev

# Type check
npm run lint
```

## License

Apache-2.0
