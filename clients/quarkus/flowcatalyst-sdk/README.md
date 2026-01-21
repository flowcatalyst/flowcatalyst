# FlowCatalyst Quarkus SDK

Official Quarkus SDK for the FlowCatalyst Platform - Event-driven architecture made simple.

## Requirements

- Java 21+
- Quarkus 3.17+
- MongoDB 4.4+ (for outbox)

## Installation

Add the dependency to your `build.gradle.kts`:

```kotlin
implementation("tech.flowcatalyst:flowcatalyst-quarkus-sdk:1.0.0")
```

Or for Maven, add to your `pom.xml`:

```xml
<dependency>
    <groupId>tech.flowcatalyst</groupId>
    <artifactId>flowcatalyst-quarkus-sdk</artifactId>
    <version>1.0.0</version>
</dependency>
```

## Configuration

Add the following to your `application.properties`:

```properties
# FlowCatalyst API
flowcatalyst.base-url=https://your-instance.flowcatalyst.io
flowcatalyst.client-id=your_client_id
flowcatalyst.client-secret=your_client_secret

# Outbox (for event creation)
flowcatalyst.tenant-id=your_tenant_id

# Webhook validation (optional)
flowcatalyst.signing-secret=your_signing_secret
```

## Control Plane API

The SDK provides access to FlowCatalyst control plane APIs using OIDC client credentials authentication.

### Injecting the Client

```java
import tech.flowcatalyst.sdk.client.FlowCatalystClient;

@ApplicationScoped
public class MyService {

    @Inject
    FlowCatalystClient flowCatalyst;

    public void doSomething() {
        var eventTypes = flowCatalyst.eventTypes().list();
    }
}
```

### Event Types

```java
import tech.flowcatalyst.sdk.client.FlowCatalystClient;
import tech.flowcatalyst.sdk.client.resources.EventTypes.CreateEventTypeRequest;

// List event types
var result = flowCatalyst.eventTypes().list();
for (var eventType : result.items()) {
    System.out.println(eventType.code() + ": " + eventType.name());
}

// Create an event type
var eventType = flowCatalyst.eventTypes().create(
    new CreateEventTypeRequest(
        "order:fulfillment:order:created",
        "Order Created",
        "Fired when a new order is placed"
    )
);

// Add a schema version
flowCatalyst.eventTypes().addSchema(
    eventType.id(),
    new EventTypes.AddSchemaRequest(
        "1.0",
        "application/json",
        jsonSchemaString,
        SchemaType.JSON_SCHEMA
    )
);

// Finalise the schema
flowCatalyst.eventTypes().finaliseSchema(eventType.id(), "1.0");
```

### Subscriptions

```java
import tech.flowcatalyst.sdk.client.FlowCatalystClient;
import tech.flowcatalyst.sdk.client.resources.Subscriptions.CreateSubscriptionRequest;
import tech.flowcatalyst.sdk.dto.EventTypeBinding;
import tech.flowcatalyst.sdk.enums.DispatchMode;

// Create a subscription
var subscription = flowCatalyst.subscriptions().create(
    CreateSubscriptionRequest.builder()
        .code("notify-warehouse")
        .name("Notify Warehouse")
        .eventTypes(List.of(
            new EventTypeBinding("order:fulfillment:order:created", null)
        ))
        .target("https://warehouse.example.com/webhook")
        .queue("default")
        .dispatchPoolId(poolId)
        .mode(DispatchMode.IMMEDIATE)
        .timeoutSeconds(30)
        .maxRetries(5)
        .build()
);

// Pause/resume a subscription
flowCatalyst.subscriptions().pause(subscription.id());
flowCatalyst.subscriptions().resume(subscription.id());
```

### Dispatch Pools

```java
import tech.flowcatalyst.sdk.client.FlowCatalystClient;
import tech.flowcatalyst.sdk.client.resources.DispatchPools.CreateDispatchPoolRequest;

// Create a dispatch pool for rate limiting
var pool = flowCatalyst.dispatchPools().create(
    CreateDispatchPoolRequest.builder()
        .code("warehouse-webhooks")
        .name("Warehouse Webhooks")
        .rateLimit(100)      // Max 100 requests per minute
        .concurrency(10)     // Max 10 concurrent requests
        .build()
);

// Suspend/activate a pool
flowCatalyst.dispatchPools().suspend(pool.id());
flowCatalyst.dispatchPools().activate(pool.id());
```

### Roles & Permissions

```java
import tech.flowcatalyst.sdk.client.FlowCatalystClient;
import tech.flowcatalyst.sdk.client.resources.Roles.*;

// List roles
var result = flowCatalyst.roles().list();

// Sync roles for your application (SDK-managed roles)
var syncResult = flowCatalyst.roles().sync("myapp", List.of(
    new SyncRoleDefinition(
        "admin",
        "Administrator",
        "Full access to all features",
        Set.of("myapp:users:read", "myapp:users:write", "myapp:settings:manage")
    ),
    new SyncRoleDefinition(
        "viewer",
        "Viewer",
        null,
        Set.of("myapp:users:read")
    )
), true);

// List permissions
var permissions = flowCatalyst.permissions().list();
```

### Applications

```java
import tech.flowcatalyst.sdk.client.FlowCatalystClient;
import tech.flowcatalyst.sdk.client.resources.Applications.*;

// List applications
var result = flowCatalyst.applications().list();

// Get by code
var app = flowCatalyst.applications().getByCode("myapp");

// Create an application
var newApp = flowCatalyst.applications().create(
    new CreateApplicationRequest(
        "myapp",
        "My Application",
        "My awesome application",
        "https://myapp.example.com",
        null
    )
);
```

## Outbox (Event Creation)

The outbox allows your application to create events and dispatch jobs using the transactional outbox pattern. Events are written to a local database and then processed by FlowCatalyst.

### Setup

Add the MongoDB extension:

```kotlin
implementation("io.quarkus:quarkus-mongodb-client")
```

Create the collection with indexes:

```javascript
db.createCollection("outbox_messages");
db.outbox_messages.createIndex(
    { tenant_id: 1, partition_id: 1, status: 1, created_at: 1 },
    { name: "idx_outbox_pending" }
);
db.outbox_messages.createIndex(
    { status: 1, created_at: 1 },
    { name: "idx_outbox_status" }
);
```

### Creating Events

```java
import tech.flowcatalyst.sdk.outbox.OutboxManager;
import tech.flowcatalyst.sdk.outbox.driver.MongoDriver;
import tech.flowcatalyst.sdk.outbox.dto.CreateEventDto;
import com.mongodb.client.MongoClient;

@ApplicationScoped
public class EventProducer {

    @Inject
    MongoClient mongoClient;

    @ConfigProperty(name = "flowcatalyst.tenant-id")
    String tenantId;

    OutboxManager outbox;

    @PostConstruct
    void init() {
        var driver = new MongoDriver(mongoClient, "flowcatalyst");
        outbox = new OutboxManager(driver, tenantId);
    }

    public String createOrderEvent(String orderId, BigDecimal total) {
        return outbox.createEvent(
            CreateEventDto.create(
                "order.created",
                Map.of("orderId", orderId, "total", total, "currency", "USD"),
                "orders"
            )
            .withCorrelationId("corr-abc-123")
            .withSource("order-service")
            .build()
        );
    }

    // Batch create events
    public List<String> createBatchEvents() {
        return outbox.createEvents(List.of(
            CreateEventDto.create("order.created", Map.of("orderId", "ORD-001"), "orders").build(),
            CreateEventDto.create("order.created", Map.of("orderId", "ORD-002"), "orders").build(),
            CreateEventDto.create("order.created", Map.of("orderId", "ORD-003"), "orders").build()
        ));
    }
}
```

### Creating Dispatch Jobs

```java
import tech.flowcatalyst.sdk.outbox.OutboxManager;
import tech.flowcatalyst.sdk.outbox.dto.CreateDispatchJobDto;

// Create a dispatch job (direct webhook without subscription matching)
var jobId = outbox.createDispatchJob(
    CreateDispatchJobDto.create(
        "order-service",
        "notify-warehouse",
        "https://warehouse.example.com/webhook",
        Map.of("orderId", "ORD-123", "action", "prepare"),
        warehousePoolId,
        "warehouse-notifications"
    )
    .withCorrelationId("corr-abc-123")
    .withHeaders(Map.of("X-Priority", "high"))
    .build()
);
```

## Webhook Validation

Validate incoming webhooks from FlowCatalyst using HMAC-SHA256 signatures.

### JAX-RS Resource

```java
import tech.flowcatalyst.sdk.webhook.WebhookValidator;
import tech.flowcatalyst.sdk.exception.WebhookValidationException;

@Path("/webhooks")
@ApplicationScoped
public class WebhookResource {

    @ConfigProperty(name = "flowcatalyst.signing-secret")
    String signingSecret;

    @POST
    @Path("/flowcatalyst")
    @Consumes(MediaType.APPLICATION_JSON)
    public Response handleWebhook(
        @Context HttpHeaders headers,
        String requestBody
    ) {
        try {
            var validator = new WebhookValidator(signingSecret);
            validator.validateRequest(headers, requestBody);
        } catch (WebhookValidationException e) {
            return Response.status(401)
                .entity(Map.of("error", "Invalid signature"))
                .build();
        }

        // Process the webhook
        var payload = new ObjectMapper().readTree(requestBody);

        return Response.ok(Map.of("received", true)).build();
    }
}
```

### Manual Validation

```java
import tech.flowcatalyst.sdk.webhook.WebhookValidator;
import tech.flowcatalyst.sdk.exception.WebhookValidationException;

var validator = new WebhookValidator(signingSecret);

try {
    validator.validate(
        requestBody,               // Raw request body
        signatureHeader,           // X-FlowCatalyst-Signature header value
        timestampHeader            // X-FlowCatalyst-Timestamp header value
    );
    // Signature is valid
} catch (WebhookValidationException e) {
    // Invalid signature, timestamp expired, or missing headers
}
```

## Error Handling

The SDK throws specific exceptions for different error types:

```java
import tech.flowcatalyst.sdk.exception.*;

try {
    flowCatalyst.eventTypes().create(...);
} catch (AuthenticationException e) {
    // Invalid credentials or token expired
} catch (ValidationException e) {
    // Validation errors
    var errors = e.getErrors();
} catch (OutboxException e) {
    // Outbox insertion error
} catch (FlowCatalystException e) {
    // General API error
    var statusCode = e.getStatusCode();
}
```

## TSID Generation

The SDK includes a utility for generating Time-Sorted IDs (TSIDs), which are used for all entity IDs:

```java
import tech.flowcatalyst.sdk.support.TsidGenerator;

// Generate a new TSID
String id = TsidGenerator.generate();  // e.g., "0HZXEQ5Y8JY5Z"

// Convert between formats
long numericId = TsidGenerator.toLong("0HZXEQ5Y8JY5Z");
String stringId = TsidGenerator.toString(786259737685263979L);
```

## Testing

For testing, you can use Quarkus mocking:

```java
import io.quarkus.test.junit.QuarkusTest;
import io.quarkus.test.junit.mockito.InjectMock;

@QuarkusTest
class MyServiceTest {

    @InjectMock
    FlowCatalystClient flowCatalyst;

    @Test
    void testEventTypeCreation() {
        when(flowCatalyst.eventTypes().list())
            .thenReturn(ListResult.of(List.of()));

        // Your test code
    }
}
```

## License

MIT License. See [LICENSE](LICENSE) for details.
