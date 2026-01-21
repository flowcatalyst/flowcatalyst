# Message Router Module Reorganization Plan

## Goal

Create a new Vert.x-based message router module that:
- Runs in Quarkus with the same REST endpoints, metrics, dashboard
- Uses verticle architecture internally (from VERTX_MIGRATION_PLAN.md)
- Can run alongside the existing implementation for A/B testing

## Proposed Module Structure

```
core/
├── flowcatalyst-message-router-common/     # NEW - shared code
│   ├── interfaces/
│   ├── models/
│   ├── metrics/
│   ├── endpoints/
│   ├── health/
│   ├── config/
│   └── resources/ (dashboard)
│
├── flowcatalyst-message-router/            # EXISTING - legacy implementation
│   └── (depends on common, keeps QueueManager, ProcessPoolImpl)
│
└── flowcatalyst-message-router-vertx/      # NEW - verticle implementation
    └── (depends on common, has verticles)
```

---

## Module 1: flowcatalyst-message-router-common

Contains all the shared code that both implementations need.

### What Goes Here

#### Interfaces (keep as-is)
```
interfaces/
├── ProcessPool.java                 # Pool interface
├── QueueConsumer.java               # Consumer interface
├── Mediator.java                    # Mediator interface
├── MediatorFactory.java             # Factory interface
├── QueueConsumerFactory.java        # Factory interface
├── MessageCallback.java             # Ack/nack callback
├── MessageVisibilityControl.java    # Visibility extension
└── ReceiptHandleUpdatable.java      # Receipt handle update
```

#### Models (keep as-is)
```
model/
├── MessagePointer.java
├── MediationOutcome.java
├── MediationResponse.java
├── MediationType.java
├── QueueStats.java
├── PoolStats.java
├── CircuitBreakerStats.java
├── HealthStatus.java
├── ReadinessStatus.java
├── InFlightMessage.java
└── Warning.java
```

#### Metrics (keep as-is)
```
metrics/
├── QueueMetricsService.java         # Interface
├── PoolMetricsService.java          # Interface
├── MicrometerQueueMetricsService.java
├── MicrometerPoolMetricsService.java
└── CircuitBreakerMetricsService.java
```

#### REST Endpoints (keep as-is)
```
endpoint/
├── MonitoringResource.java          # Dashboard, metrics endpoints
├── HealthCheckResource.java         # /health endpoint
├── KubernetesHealthResource.java    # K8s probes
├── LocalConfigResource.java         # Dev config endpoint
├── MessageSeedResource.java         # Test seeding (dev only)
└── TestResponseResource.java        # Test responses (dev only)
```

#### Health & Warnings (keep as-is)
```
health/
├── InfrastructureHealthService.java
├── BrokerHealthService.java
├── HealthStatusService.java
├── QueueHealthMonitor.java
└── QueueValidationService.java

warning/
├── WarningService.java
└── Warning.java

notification/
├── NotificationService.java
├── EmailNotificationService.java
├── TeamsWebhookNotificationService.java
└── BatchingNotificationService.java
```

#### Configuration (keep as-is)
```
config/
├── MessageRouterConfig.java         # Record
├── QueueConfig.java                 # Record
├── ProcessingPool.java              # Record
├── QueueType.java                   # Enum
├── AuthenticationConfig.java
├── ActiveMqConnectionFactoryProducer.java
├── NatsConnectionProducer.java
└── DevQueueSetup.java
```

#### Security (keep as-is)
```
security/
├── AuthenticationFilter.java
├── BasicAuthIdentityProvider.java
└── Protected.java (annotation)
```

#### Static Resources
```
resources/META-INF/resources/
├── dashboard.html
├── tailwind.min.css
└── tailwind.css
```

---

## Module 2: flowcatalyst-message-router (Existing)

Keep the legacy virtual-thread implementation. Just add dependency on common.

### What Stays Here

```
manager/
└── QueueManager.java                # Main orchestrator (legacy)

pool/
└── ProcessPoolImpl.java             # Virtual thread pool (legacy)

mediator/
└── HttpMediator.java                # HTTP mediator (legacy)

consumer/
├── AbstractQueueConsumer.java
├── SqsQueueConsumer.java
├── ActiveMqQueueConsumer.java
├── NatsQueueConsumer.java
└── EmbeddedQueueConsumer.java

factory/
├── MediatorFactoryImpl.java
└── QueueConsumerFactoryImpl.java

embedded/
└── (SQLite embedded queue)

standby/
└── (Redis hot standby)

traffic/
└── (Load balancer management)
```

---

## Module 3: flowcatalyst-message-router-vertx (New)

The new verticle-based implementation.

### Package Structure

```
tech.flowcatalyst.messagerouter.vertx/
├── verticle/
│   ├── QueueManagerVerticle.java    # Central coordinator
│   ├── PoolVerticle.java            # Pool with group workers
│   ├── MediatorVerticle.java        # HTTP delivery
│   └── QueueConsumerVerticle.java   # SQS polling
│
├── launcher/
│   └── VertxRouterLauncher.java     # Quarkus startup
│
└── adapter/
    └── VertxQueueManager.java       # Adapts verticles to existing interfaces
```

### How It Works

The key insight: **VertxQueueManager** implements the same interface that MonitoringResource expects, so the REST endpoints work unchanged.

```java
@ApplicationScoped
@Alternative  // Takes precedence over legacy QueueManager when deployed
public class VertxQueueManager {

    @Inject
    Vertx vertx;  // Quarkus provides this

    @Inject
    QueueMetricsService queueMetrics;

    @Inject
    PoolMetricsService poolMetrics;

    // Deployment IDs for verticles
    private final Map<String, String> poolDeploymentIds = new HashMap<>();

    void onStartup(@Observes StartupEvent ev) {
        // Deploy QueueManagerVerticle
        DeploymentOptions options = new DeploymentOptions()
            .setThreadingModel(ThreadingModel.VIRTUAL_THREAD);

        vertx.deployVerticle(new QueueManagerVerticle(queueMetrics, poolMetrics), options)
            .toCompletionStage().toCompletableFuture().join();
    }

    // Expose methods that MonitoringResource needs
    public Map<String, QueueConsumerHealth> getConsumerHealthStatus() {
        // Query via event bus
    }

    public List<InFlightMessage> getInFlightMessages(int limit, String filter) {
        // Query via event bus
    }
}
```

### Quarkus Integration

The verticles need access to CDI beans (metrics, config). Two approaches:

**Option A: Pass beans to verticles via constructor**
```java
public class QueueManagerVerticle extends AbstractVerticle {
    private final QueueMetricsService metrics;
    private final ConfigService configService;

    public QueueManagerVerticle(QueueMetricsService metrics, ConfigService configService) {
        this.metrics = metrics;
        this.configService = configService;
    }
}
```

**Option B: Use Quarkus Arc in verticles**
```java
public class QueueManagerVerticle extends AbstractVerticle {
    @Override
    public void start() {
        QueueMetricsService metrics = Arc.container().instance(QueueMetricsService.class).get();
    }
}
```

Option A is cleaner and more testable.

---

## Build Configuration

### settings.gradle.kts (additions)

```kotlin
// Shared interfaces and models
include("core:flowcatalyst-message-router-common")

// Legacy implementation (existing)
include("core:flowcatalyst-message-router")

// New Vert.x implementation
include("core:flowcatalyst-message-router-vertx")
```

### flowcatalyst-message-router-common/build.gradle.kts

```kotlin
plugins {
    java
    id("io.quarkus")
}

dependencies {
    implementation(enforcedPlatform("${quarkusPlatformGroupId}:${quarkusPlatformArtifactId}:${quarkusPlatformVersion}"))

    // Core Quarkus
    implementation("io.quarkus:quarkus-rest")
    implementation("io.quarkus:quarkus-rest-jackson")
    implementation("io.quarkus:quarkus-arc")
    implementation("io.quarkus:quarkus-security")

    // Metrics
    implementation("io.quarkus:quarkus-micrometer-registry-prometheus")

    // Health
    implementation("io.quarkus:quarkus-smallrye-health")

    // Fault tolerance (for circuit breaker metrics)
    implementation("io.quarkus:quarkus-smallrye-fault-tolerance")

    // Notifications
    implementation("io.quarkus:quarkus-mailer")
}

// This is a library, not an application
tasks.named("quarkusBuild") {
    enabled = false
}
```

### flowcatalyst-message-router/build.gradle.kts (modified)

```kotlin
dependencies {
    // Add dependency on common module
    implementation(project(":core:flowcatalyst-message-router-common"))

    // Keep existing dependencies for consumers, embedded queue, etc.
    implementation("io.quarkiverse.amazonservices:quarkus-amazon-sqs")
    implementation("org.apache.activemq:activemq-client:6.1.7")
    implementation("io.nats:jnats:2.24.1")
    // ... rest unchanged
}
```

### flowcatalyst-message-router-vertx/build.gradle.kts

```kotlin
plugins {
    java
    id("io.quarkus")
    id("com.google.cloud.tools.jib") version "3.4.0"
}

dependencies {
    implementation(enforcedPlatform("${quarkusPlatformGroupId}:${quarkusPlatformArtifactId}:${quarkusPlatformVersion}"))

    // Shared common module
    implementation(project(":core:flowcatalyst-message-router-common"))

    // Vert.x (Quarkus provides integration)
    implementation("io.quarkus:quarkus-vertx")

    // SQS (blocking client)
    implementation("io.quarkiverse.amazonservices:quarkus-amazon-sqs")
    implementation("software.amazon.awssdk:url-connection-client")

    // Resilience4j for rate limiting
    implementation("io.github.resilience4j:resilience4j-ratelimiter:${resilience4jVersion}")
    implementation("io.github.resilience4j:resilience4j-circuitbreaker:${resilience4jVersion}")

    // Standby (optional)
    implementation(project(":core:flowcatalyst-standby"))
}
```

---

## Migration Steps

### Phase 1: Create Common Module

1. Create `core/flowcatalyst-message-router-common/` directory
2. Move shared code from message-router to common
3. Update message-router to depend on common
4. Verify all existing tests pass

### Phase 2: Create Vertx Module

1. Create `core/flowcatalyst-message-router-vertx/` directory
2. Create build.gradle.kts
3. Implement verticles (copy from VERTX_MIGRATION_PLAN.md)
4. Create VertxQueueManager adapter
5. Write unit tests

### Phase 3: Integration Testing

1. Run both implementations against same test suite
2. Compare metrics, behavior
3. Load test both

### Phase 4: Deployment Options

**Option A: Separate deployments**
- Deploy message-router OR message-router-vertx
- Config property to choose

**Option B: Dev build includes both**
- flowcatalyst-dev-build depends on one or the other
- Switch via environment variable

---

## What Changes vs Stays Same

### Stays Exactly The Same
- REST API contracts (`/monitoring/*`, `/health/*`)
- Metrics names and labels
- Dashboard HTML
- Configuration properties
- Authentication/security
- Warning/notification system
- Health check logic

### Changes (Internal Only)
- QueueManager → QueueManagerVerticle
- ProcessPoolImpl → PoolVerticle
- HttpMediator → MediatorVerticle
- Shared ConcurrentHashMaps → Plain HashMaps in verticles
- Virtual thread executor → Vert.x executeBlocking
- Scheduled tasks → Vert.x setPeriodic

### New
- Event bus communication between verticles
- Verticle deployment/undeployment lifecycle

---

## Alternative: Minimal Change Approach

If extracting common code seems like too much work, a simpler approach:

1. **Copy entire message-router to message-router-vertx**
2. **Replace only the core classes:**
   - Delete QueueManager.java → Create QueueManagerVerticle.java
   - Delete ProcessPoolImpl.java → Create PoolVerticle.java
   - Delete HttpMediator.java → Create MediatorVerticle.java
3. **Keep everything else identical**

This has more code duplication but is faster to implement and test.

---

## Recommendation

Start with the **minimal change approach** (copy and replace). Once both implementations are proven to work:

1. Extract common code into shared module
2. Remove duplication
3. Deprecate legacy implementation

This reduces risk - if the Vert.x version has issues, the legacy version is unchanged and can be deployed immediately.
