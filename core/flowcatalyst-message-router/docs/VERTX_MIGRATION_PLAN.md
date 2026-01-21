# Message Router: Vert.x Migration Plan

## Executive Summary

This document outlines the plan to migrate the FlowCatalyst Message Router from a shared-memory virtual threads architecture to a Vert.x verticle-based architecture. The migration aims to eliminate concurrency bugs by design through isolated state ownership.

**Key Insight:** All verticles run on virtual threads. This means we use blocking I/O everywhere - no async callbacks, no futures, just simple sequential code.

**Risk Level:** Medium
**Benefit:** Eliminates entire class of race condition bugs

---

## 1. Current Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           QueueManager                                   │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │ Shared State (ConcurrentHashMaps):                               │   │
│  │  - processPools: Map<String, ProcessPool>                        │   │
│  │  - inPipelineMap: Map<String, MessagePointer>                    │   │
│  │  - messageCallbacks: Map<String, MessageCallback>                │   │
│  │  - appMessageIdToPipelineKey: Map<String, String>                │   │
│  │  - queueConsumers: Map<String, QueueConsumer>                    │   │
│  └─────────────────────────────────────────────────────────────────┘   │
│                                    │                                     │
│                    ┌───────────────┴───────────────┐                    │
│                    ▼                               ▼                    │
│            ┌──────────────┐               ┌──────────────┐              │
│            │ ProcessPool  │               │ ProcessPool  │              │
│            │   (pool-a)   │               │   (pool-b)   │              │
│            └──────────────┘               └──────────────┘              │
│                    │                               │                    │
│         ┌─────────┴─────────┐           ┌─────────┴─────────┐          │
│         ▼                   ▼           ▼                   ▼          │
│   ┌──────────┐       ┌──────────┐ ┌──────────┐       ┌──────────┐     │
│   │ Group    │       │ Group    │ │ Group    │       │ Group    │     │
│   │ Thread   │       │ Thread   │ │ Thread   │       │ Thread   │     │
│   │(order-1) │       │(order-2) │ │(user-1)  │       │(user-2)  │     │
│   └──────────┘       └──────────┘ └──────────┘       └──────────┘     │
└─────────────────────────────────────────────────────────────────────────┘
```

### Concurrency Model

- **Shared mutable state** protected by ConcurrentHashMap, AtomicInteger, volatile
- **Virtual thread per message group** for FIFO processing
- **Semaphore** for pool-level concurrency control
- **Periodic scheduled tasks** for config sync, health checks, visibility extension

### Known Issues (Fixed)

1. Pool removal race during config sync
2. Thread restart check-then-act race
3. Side effects in computeIfAbsent
4. Missing drain check in submit

---

## 2. Target Architecture

### Design Philosophy

**All verticles run on virtual threads.** This means:

- ✅ Blocking I/O is fine (SQS, HTTP, semaphore.acquire())
- ✅ Simple sequential code - no callbacks, no futures
- ✅ Use JDK HttpClient (blocking) - not Vert.x WebClient
- ✅ Use AWS SDK SqsClient (blocking) - not SqsAsyncClient
- ✅ Plain HashMap - not ConcurrentHashMap (each verticle is single-threaded)
- ✅ Event bus for inter-verticle communication

### Verticle Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              Event Bus                                   │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐       │
│  │ router. │  │ router. │  │ pool.   │  │ pool.   │  │ mediator│       │
│  │ batch   │  │ command │  │ pool-a  │  │ pool-b  │  │ .result │       │
│  └─────────┘  └─────────┘  └─────────┘  └─────────┘  └─────────┘       │
└─────────────────────────────────────────────────────────────────────────┘
       ▲              ▲            ▲            ▲            ▲
       │              │            │            │            │
┌──────┴──────┐ ┌─────┴─────┐ ┌────┴────┐ ┌────┴────┐ ┌─────┴─────┐
│ QueueManager│ │ Queue     │ │  Pool   │ │  Pool   │ │  Mediator │
│  Verticle   │ │ Consumer  │ │Verticle │ │Verticle │ │  Verticle │
│ (VT)        │ │ Verticle  │ │ (VT)    │ │ (VT)    │ │  (VT)     │
│             │ │ (VT)      │ │         │ │         │ │           │
└─────────────┘ └───────────┘ └─────────┘ └─────────┘ └───────────┘
                                   │            │
                              ┌────┴────┐  ┌────┴────┐
                              │  Group  │  │  Group  │
                              │ Workers │  │ Workers │
                              │(internal│  │(internal│
                              └─────────┘  └─────────┘

All verticles: ThreadingModel.VIRTUAL_THREAD
```

### Verticle Types

| Verticle | Threading Model | Count | Owns State |
|----------|-----------------|-------|------------|
| QueueManagerVerticle | Virtual Thread | 1 | Dedup maps, pool registry |
| QueueConsumerVerticle | Virtual Thread | 1 per queue | Consumer state |
| PoolVerticle | Virtual Thread | 1 per pool | Group queues, semaphore |
| MediatorVerticle | Virtual Thread | 1 per pool | Circuit breaker |

### Key Design Decisions

1. **All verticles on virtual threads** - Blocking is cheap, code is simple
2. **Blocking HTTP/SQS clients** - No async complexity
3. **Groups processed inside PoolVerticle** - No separate GroupVerticle to reduce complexity
4. **Deduplication in QueueManagerVerticle** - Single owner, no races
5. **Event bus for all inter-verticle communication** - Message passing, not shared memory

---

## 3. Component Design

### 3.1 QueueManagerVerticle

**Threading:** Virtual Thread (blocking OK)
**Purpose:** Central coordinator, owns deduplication state, manages pool lifecycle

```java
public class QueueManagerVerticle extends AbstractVerticle {

    // === OWNED STATE (plain HashMap - single threaded) ===
    private final Map<String, String> poolDeploymentIds = new HashMap<>();
    private final Map<String, MessagePointer> inPipeline = new HashMap<>();
    private final Map<String, MessageCallback> messageCallbacks = new HashMap<>();
    private final Map<String, String> appMessageIdToPipelineKey = new HashMap<>();
    private final Map<String, String> consumerDeploymentIds = new HashMap<>();

    // Blocking SQS client
    private SqsClient sqsClient;

    // === LIFECYCLE ===
    @Override
    public void start() {
        // Initialize blocking SQS client
        this.sqsClient = SqsClient.builder()
            .region(Region.EU_WEST_1)
            .build();

        // Register event bus consumers
        vertx.eventBus().consumer("router.batch", this::handleBatch);
        vertx.eventBus().consumer("router.ack", this::handleAck);
        vertx.eventBus().consumer("router.nack", this::handleNack);

        // Periodic tasks (blocking is fine)
        vertx.setPeriodic(300_000, id -> syncConfiguration());
        vertx.setPeriodic(55_000, id -> extendVisibility());

        // Initial config sync
        syncConfiguration();
    }

    // === MESSAGE HANDLING ===
    private void handleBatch(Message<JsonObject> msg) {
        JsonArray messages = msg.body().getJsonArray("messages");
        String batchId = UUID.randomUUID().toString();

        // Deduplication (safe - single threaded, plain HashMap)
        for (int i = 0; i < messages.size(); i++) {
            JsonObject message = messages.getJsonObject(i);
            String sqsMessageId = message.getString("sqsMessageId");

            if (inPipeline.containsKey(sqsMessageId)) {
                // Duplicate - nack with 0 delay
                nackMessage(sqsMessageId, 0);
                continue;
            }

            // Track in pipeline
            inPipeline.put(sqsMessageId, MessagePointer.fromJson(message));
            messageCallbacks.put(sqsMessageId, createCallback(message));

            // Route to pool
            String poolCode = message.getString("poolCode");
            JsonObject enriched = message.copy().put("batchId", batchId);

            vertx.eventBus().send("pool." + poolCode, enriched);
        }
    }

    private void handleAck(Message<JsonObject> msg) {
        String sqsMessageId = msg.body().getString("sqsMessageId");
        MessageCallback callback = messageCallbacks.remove(sqsMessageId);
        inPipeline.remove(sqsMessageId);

        if (callback != null) {
            // Blocking SQS delete - that's fine
            sqsClient.deleteMessage(DeleteMessageRequest.builder()
                .queueUrl(callback.queueUrl())
                .receiptHandle(callback.receiptHandle())
                .build());
        }
        msg.reply("ok");
    }

    private void handleNack(Message<JsonObject> msg) {
        String sqsMessageId = msg.body().getString("sqsMessageId");
        int delaySeconds = msg.body().getInteger("delaySeconds", 0);

        MessageCallback callback = messageCallbacks.remove(sqsMessageId);
        inPipeline.remove(sqsMessageId);

        if (callback != null) {
            // Blocking SQS visibility change - that's fine
            sqsClient.changeMessageVisibility(ChangeMessageVisibilityRequest.builder()
                .queueUrl(callback.queueUrl())
                .receiptHandle(callback.receiptHandle())
                .visibilityTimeout(delaySeconds)
                .build());
        }
        msg.reply("ok");
    }

    private void nackMessage(String sqsMessageId, int delaySeconds) {
        MessageCallback callback = messageCallbacks.remove(sqsMessageId);
        inPipeline.remove(sqsMessageId);

        if (callback != null) {
            sqsClient.changeMessageVisibility(ChangeMessageVisibilityRequest.builder()
                .queueUrl(callback.queueUrl())
                .receiptHandle(callback.receiptHandle())
                .visibilityTimeout(delaySeconds)
                .build());
        }
    }

    // === CONFIGURATION ===
    private void syncConfiguration() {
        // Blocking HTTP call to fetch config - that's fine
        HttpClient client = HttpClient.newHttpClient();
        HttpRequest request = HttpRequest.newBuilder()
            .uri(URI.create(configUrl))
            .GET()
            .build();

        try {
            HttpResponse<String> response = client.send(request,
                HttpResponse.BodyHandlers.ofString());

            RouterConfig config = parseConfig(response.body());

            // Deploy/undeploy pools as needed
            // Safe - single threaded, plain HashMap
            for (PoolConfig poolConfig : config.pools()) {
                if (!poolDeploymentIds.containsKey(poolConfig.code())) {
                    deployPool(poolConfig);
                }
            }

            // Remove pools no longer in config
            Set<String> configPoolCodes = config.pools().stream()
                .map(PoolConfig::code)
                .collect(Collectors.toSet());

            for (String existingCode : new ArrayList<>(poolDeploymentIds.keySet())) {
                if (!configPoolCodes.contains(existingCode)) {
                    undeployPool(existingCode);
                }
            }
        } catch (Exception e) {
            LOG.error("Failed to sync config", e);
        }
    }

    private void deployPool(PoolConfig config) {
        DeploymentOptions options = new DeploymentOptions()
            .setThreadingModel(ThreadingModel.VIRTUAL_THREAD)
            .setConfig(JsonObject.mapFrom(config));

        // Blocking deployment - that's fine
        String deploymentId = vertx.deployVerticle(new PoolVerticle(), options)
            .toCompletionStage().toCompletableFuture().join();

        poolDeploymentIds.put(config.code(), deploymentId);

        // Also deploy mediator for this pool
        DeploymentOptions mediatorOptions = new DeploymentOptions()
            .setThreadingModel(ThreadingModel.VIRTUAL_THREAD)
            .setConfig(new JsonObject().put("poolCode", config.code()));

        vertx.deployVerticle(new MediatorVerticle(), mediatorOptions);
    }

    private void undeployPool(String poolCode) {
        String deploymentId = poolDeploymentIds.remove(poolCode);
        if (deploymentId != null) {
            vertx.undeploy(deploymentId).toCompletionStage().toCompletableFuture().join();
        }
    }

    // === VISIBILITY EXTENSION ===
    private void extendVisibility() {
        // Iterate over in-pipeline messages and extend visibility
        // Safe - single threaded
        for (var entry : inPipeline.entrySet()) {
            MessageCallback callback = messageCallbacks.get(entry.getKey());
            if (callback != null) {
                try {
                    sqsClient.changeMessageVisibility(ChangeMessageVisibilityRequest.builder()
                        .queueUrl(callback.queueUrl())
                        .receiptHandle(callback.receiptHandle())
                        .visibilityTimeout(120)
                        .build());
                } catch (Exception e) {
                    LOG.warn("Failed to extend visibility for {}", entry.getKey(), e);
                }
            }
        }
    }
}
```

### 3.2 PoolVerticle

**Threading:** Virtual Thread (blocking OK)
**Purpose:** Manages message groups, enforces concurrency and rate limits

```java
public class PoolVerticle extends AbstractVerticle {

    // === OWNED STATE (plain collections - single threaded) ===
    private String poolCode;
    private final Map<String, BlockingQueue<MessagePointer>> groupQueues = new HashMap<>();
    private final Map<String, Boolean> activeGroups = new HashMap<>();
    private final Set<String> failedBatchGroups = new HashSet<>();
    private Semaphore semaphore;
    private RateLimiter rateLimiter;
    private volatile boolean running = true;

    // === LIFECYCLE ===
    @Override
    public void start() {
        JsonObject config = config();
        this.poolCode = config.getString("code");
        this.semaphore = new Semaphore(config.getInteger("concurrency", 10));

        if (config.containsKey("rateLimitPerMinute")) {
            this.rateLimiter = createRateLimiter(config.getInteger("rateLimitPerMinute"));
        }

        // Listen for messages to this pool
        vertx.eventBus().<JsonObject>consumer("pool." + poolCode, this::handleMessage);

        // Listen for config updates
        vertx.eventBus().<JsonObject>consumer("pool." + poolCode + ".config", this::handleConfigUpdate);
    }

    @Override
    public void stop() {
        running = false;
    }

    // === MESSAGE HANDLING ===
    private void handleMessage(Message<JsonObject> msg) {
        JsonObject json = msg.body();
        MessagePointer message = MessagePointer.fromJson(json);
        String groupId = message.messageGroupId() != null ? message.messageGroupId() : "__DEFAULT__";
        String batchId = json.getString("batchId");

        // Check batch+group FIFO failure
        String batchGroupKey = batchId + "|" + groupId;
        if (failedBatchGroups.contains(batchGroupKey)) {
            sendNack(message.sqsMessageId(), 10);
            return;
        }

        // Get or create group queue (plain HashMap - single threaded)
        BlockingQueue<MessagePointer> queue = groupQueues.get(groupId);
        if (queue == null) {
            queue = new LinkedBlockingQueue<>();
            groupQueues.put(groupId, queue);
        }

        // Ensure group worker is running
        if (!activeGroups.containsKey(groupId)) {
            activeGroups.put(groupId, true);
            startGroupWorker(groupId, queue);
        }

        // Add to queue
        queue.offer(message);
    }

    private void startGroupWorker(String groupId, BlockingQueue<MessagePointer> queue) {
        // Run on virtual thread - blocking is fine
        vertx.executeBlocking(() -> {
            processGroup(groupId, queue);
            return null;
        }, false);
    }

    private void processGroup(String groupId, BlockingQueue<MessagePointer> queue) {
        try {
            while (running) {
                // Block waiting for message (5 min idle timeout)
                MessagePointer message = queue.poll(5, TimeUnit.MINUTES);

                if (message == null) {
                    // Idle timeout - cleanup
                    if (queue.isEmpty()) {
                        groupQueues.remove(groupId);
                        activeGroups.remove(groupId);
                        return;
                    }
                    continue;
                }

                // Wait for rate limit permit (blocking)
                waitForRateLimitPermit();

                // Acquire concurrency permit (blocking)
                semaphore.acquire();

                try {
                    // Process via mediator (blocking request-reply)
                    processMessage(message);
                } finally {
                    semaphore.release();
                }
            }
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        } finally {
            activeGroups.remove(groupId);
        }
    }

    private void waitForRateLimitPermit() {
        if (rateLimiter == null) {
            return;
        }

        while (running) {
            if (rateLimiter.acquirePermission()) {
                return;
            }
            try {
                Thread.sleep(100); // Wait and retry
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                return;
            }
        }
    }

    private void processMessage(MessagePointer message) {
        // Blocking request-reply to mediator
        try {
            Message<JsonObject> reply = vertx.eventBus()
                .<JsonObject>request("mediator." + poolCode, JsonObject.mapFrom(message))
                .toCompletionStage()
                .toCompletableFuture()
                .get(120, TimeUnit.SECONDS);

            JsonObject result = reply.body();
            handleMediationResult(message, result);
        } catch (Exception e) {
            // Timeout or error
            sendNack(message.sqsMessageId(), 30);
            markBatchGroupFailed(message);
        }
    }

    private void handleMediationResult(MessagePointer message, JsonObject result) {
        String outcome = result.getString("outcome");

        switch (outcome) {
            case "SUCCESS" -> sendAck(message.sqsMessageId());
            case "NACK" -> {
                int delay = result.getInteger("delaySeconds", 0);
                sendNack(message.sqsMessageId(), delay);
                markBatchGroupFailed(message);
            }
            case "ERROR_CONFIG" -> sendAck(message.sqsMessageId());
        }
    }

    private void sendAck(String sqsMessageId) {
        // Blocking request
        try {
            vertx.eventBus()
                .request("router.ack", new JsonObject().put("sqsMessageId", sqsMessageId))
                .toCompletionStage()
                .toCompletableFuture()
                .get(10, TimeUnit.SECONDS);
        } catch (Exception e) {
            LOG.error("Failed to send ACK for {}", sqsMessageId, e);
        }
    }

    private void sendNack(String sqsMessageId, int delaySeconds) {
        // Blocking request
        try {
            vertx.eventBus()
                .request("router.nack",
                    new JsonObject()
                        .put("sqsMessageId", sqsMessageId)
                        .put("delaySeconds", delaySeconds))
                .toCompletionStage()
                .toCompletableFuture()
                .get(10, TimeUnit.SECONDS);
        } catch (Exception e) {
            LOG.error("Failed to send NACK for {}", sqsMessageId, e);
        }
    }

    private void markBatchGroupFailed(MessagePointer message) {
        String batchGroupKey = message.batchId() + "|" +
            (message.messageGroupId() != null ? message.messageGroupId() : "__DEFAULT__");
        failedBatchGroups.add(batchGroupKey);
    }

    private void handleConfigUpdate(Message<JsonObject> msg) {
        JsonObject config = msg.body();

        // Update concurrency
        int newConcurrency = config.getInteger("concurrency", 10);
        // Recreate semaphore with new permits (existing waiters will complete)
        this.semaphore = new Semaphore(newConcurrency);

        // Update rate limiter
        if (config.containsKey("rateLimitPerMinute")) {
            this.rateLimiter = createRateLimiter(config.getInteger("rateLimitPerMinute"));
        } else {
            this.rateLimiter = null;
        }
    }
}
```

### 3.3 MediatorVerticle

**Threading:** Virtual Thread (blocking OK)
**Purpose:** HTTP delivery with circuit breaker

```java
public class MediatorVerticle extends AbstractVerticle {

    private String poolCode;
    private HttpClient httpClient;  // JDK blocking client
    private CircuitBreaker circuitBreaker;

    @Override
    public void start() {
        this.poolCode = config().getString("poolCode");

        // Standard JDK HttpClient - blocking is fine
        this.httpClient = HttpClient.newBuilder()
            .connectTimeout(Duration.ofSeconds(5))
            .build();

        // Resilience4j circuit breaker
        CircuitBreakerConfig cbConfig = CircuitBreakerConfig.custom()
            .failureRateThreshold(50)
            .waitDurationInOpenState(Duration.ofSeconds(5))
            .slidingWindowSize(10)
            .build();
        this.circuitBreaker = CircuitBreaker.of("mediator-" + poolCode, cbConfig);

        vertx.eventBus().<JsonObject>consumer("mediator." + poolCode, this::handleRequest);
    }

    private void handleRequest(Message<JsonObject> msg) {
        JsonObject message = msg.body();
        String endpoint = message.getString("mediationTarget");
        String authToken = message.getString("authToken");
        String messageId = message.getString("id");

        try {
            // Blocking HTTP call wrapped in circuit breaker
            JsonObject result = circuitBreaker.executeSupplier(() -> {
                try {
                    HttpRequest request = HttpRequest.newBuilder()
                        .uri(URI.create(endpoint))
                        .header("Authorization", "Bearer " + authToken)
                        .header("Content-Type", "application/json")
                        .POST(HttpRequest.BodyPublishers.ofString(
                            new JsonObject().put("messageId", messageId).encode()))
                        .timeout(Duration.ofSeconds(30))
                        .build();

                    HttpResponse<String> response = httpClient.send(request,
                        HttpResponse.BodyHandlers.ofString());

                    return interpretResponse(response);
                } catch (Exception e) {
                    throw new RuntimeException(e);
                }
            });

            msg.reply(result);
        } catch (Exception e) {
            // Circuit breaker open or call failed
            msg.reply(new JsonObject()
                .put("outcome", "NACK")
                .put("delaySeconds", 30));
        }
    }

    private JsonObject interpretResponse(HttpResponse<String> response) {
        int status = response.statusCode();

        if (status == 200) {
            JsonObject body = new JsonObject(response.body());
            boolean ack = body.getBoolean("ack", true);
            if (ack) {
                return new JsonObject().put("outcome", "SUCCESS");
            } else {
                return new JsonObject()
                    .put("outcome", "NACK")
                    .put("delaySeconds", body.getInteger("delaySeconds", 0));
            }
        } else if (status == 429) {
            String retryAfter = response.headers().firstValue("Retry-After").orElse("60");
            int delay = Integer.parseInt(retryAfter);
            return new JsonObject()
                .put("outcome", "NACK")
                .put("delaySeconds", delay);
        } else if (status >= 400 && status < 500) {
            return new JsonObject().put("outcome", "ERROR_CONFIG");
        } else {
            return new JsonObject()
                .put("outcome", "NACK")
                .put("delaySeconds", 10);
        }
    }
}
```

### 3.4 QueueConsumerVerticle

**Threading:** Virtual Thread (blocking OK)
**Purpose:** Poll messages from SQS

```java
public class QueueConsumerVerticle extends AbstractVerticle {

    private String queueIdentifier;
    private String queueUrl;
    private SqsClient sqsClient;  // Blocking client
    private volatile boolean running = true;

    @Override
    public void start() {
        this.queueIdentifier = config().getString("queueIdentifier");
        this.queueUrl = config().getString("queueUrl");

        // Blocking SQS client
        this.sqsClient = SqsClient.builder()
            .region(Region.EU_WEST_1)
            .build();

        // Start polling in a blocking loop
        vertx.executeBlocking(() -> {
            pollLoop();
            return null;
        }, false);
    }

    @Override
    public void stop() {
        running = false;
    }

    private void pollLoop() {
        while (running) {
            try {
                // Blocking long poll
                ReceiveMessageResponse response = sqsClient.receiveMessage(
                    ReceiveMessageRequest.builder()
                        .queueUrl(queueUrl)
                        .maxNumberOfMessages(10)
                        .waitTimeSeconds(20)
                        .messageAttributeNames("All")
                        .build());

                if (!response.messages().isEmpty()) {
                    // Convert to batch and send to router
                    JsonArray batch = new JsonArray();
                    for (var sqsMsg : response.messages()) {
                        batch.add(convertToJson(sqsMsg));
                    }

                    // Blocking send - wait for acknowledgement
                    vertx.eventBus()
                        .request("router.batch", new JsonObject().put("messages", batch))
                        .toCompletionStage()
                        .toCompletableFuture()
                        .get(30, TimeUnit.SECONDS);
                }
            } catch (SdkException e) {
                LOG.error("SQS poll error", e);
                try {
                    Thread.sleep(5000);  // Backoff on error
                } catch (InterruptedException ie) {
                    Thread.currentThread().interrupt();
                    return;
                }
            } catch (Exception e) {
                LOG.error("Unexpected error in poll loop", e);
            }
        }
    }

    private JsonObject convertToJson(software.amazon.awssdk.services.sqs.model.Message sqsMsg) {
        return new JsonObject()
            .put("sqsMessageId", sqsMsg.messageId())
            .put("receiptHandle", sqsMsg.receiptHandle())
            .put("body", sqsMsg.body())
            .put("messageGroupId", sqsMsg.attributes().get(MessageSystemAttributeName.MESSAGE_GROUP_ID))
            // ... other fields
            ;
    }
}
```

---

## 4. Event Bus Contracts

### 4.1 Addresses

| Address | Direction | Payload | Purpose |
|---------|-----------|---------|---------|
| `router.batch` | Consumer → QueueManager | `{messages: [...]}` | Submit batch for routing |
| `router.ack` | Pool → QueueManager | `{sqsMessageId}` | Acknowledge message |
| `router.nack` | Pool → QueueManager | `{sqsMessageId, delaySeconds}` | Reject message |
| `pool.{code}` | QueueManager → Pool | Message JSON | Route message to pool |
| `pool.{code}.config` | QueueManager → Pool | Config JSON | Update pool config |
| `mediator.{code}` | Pool → Mediator | Message JSON | Send for HTTP delivery |

### 4.2 Message Schemas

```typescript
// router.batch
{
  messages: [
    {
      sqsMessageId: string,
      receiptHandle: string,
      id: string,           // Application message ID
      poolCode: string,
      authToken: string,
      mediationTarget: string,
      messageGroupId?: string
    }
  ]
}

// router.ack / router.nack
{
  sqsMessageId: string,
  delaySeconds?: number    // Only for nack
}

// mediator response
{
  outcome: "SUCCESS" | "NACK" | "ERROR_CONFIG",
  delaySeconds?: number
}
```

---

## 5. Why This Architecture Works

### No Shared Mutable State

Each verticle owns its own state:

| Verticle | Owns | Type |
|----------|------|------|
| QueueManagerVerticle | inPipeline, messageCallbacks | `HashMap` |
| PoolVerticle | groupQueues, activeGroups | `HashMap` |
| MediatorVerticle | circuitBreaker | Instance field |
| QueueConsumerVerticle | sqsClient | Instance field |

No `ConcurrentHashMap` needed. No `synchronized`. No races.

### Virtual Threads Make Blocking Cheap

```java
// This is fine - virtual thread
semaphore.acquire();           // Blocks
httpClient.send(request);      // Blocks
sqsClient.receiveMessage();    // Blocks
Thread.sleep(100);             // Blocks
```

Virtual threads unmount from the carrier thread when blocking, so thousands can exist without issue.

### Event Bus for Communication

```java
// PoolVerticle sends ACK to QueueManagerVerticle
vertx.eventBus().request("router.ack", new JsonObject().put("sqsMessageId", msgId));

// QueueManagerVerticle removes from its private map
inPipeline.remove(sqsMessageId);  // Safe - single threaded
```

No need for locks. The event bus serializes access to each verticle.

---

## 6. Migration Plan

### Phase 1: Preparation

1. **Add Vert.x dependencies**
   ```gradle
   implementation 'io.vertx:vertx-core:4.5.0'
   ```

2. **Create verticle package structure**
   ```
   src/main/java/tech/flowcatalyst/messagerouter/
   ├── verticle/
   │   ├── QueueManagerVerticle.java
   │   ├── PoolVerticle.java
   │   ├── MediatorVerticle.java
   │   └── QueueConsumerVerticle.java
   ```

### Phase 2: Core Verticles

1. **QueueManagerVerticle**
   - Deduplication logic
   - Pool deployment/undeployment
   - Config sync
   - Visibility extension

2. **PoolVerticle**
   - Group queue management
   - Virtual thread workers
   - Rate limiting
   - Concurrency control

3. **MediatorVerticle + QueueConsumerVerticle**
   - HTTP delivery with circuit breaker
   - SQS polling

### Phase 3: Integration

1. Wire up event bus communication
2. Implement graceful shutdown
3. Add metrics and logging
4. Integration testing

### Phase 4: Testing

1. Unit tests for each verticle
2. Integration tests for message flow
3. Load testing for performance comparison

### Phase 5: Cutover

1. Feature flag for old vs new implementation
2. Shadow mode: run both, compare results
3. Gradual rollout
4. Remove old implementation

---

## 7. Testing Strategy

### Unit Tests

```java
@ExtendWith(VertxExtension.class)
class QueueManagerVerticleTest {

    @Test
    void shouldDeduplicateMessages(Vertx vertx, VertxTestContext ctx) {
        DeploymentOptions options = new DeploymentOptions()
            .setThreadingModel(ThreadingModel.VIRTUAL_THREAD);

        vertx.deployVerticle(new QueueManagerVerticle(), options)
            .onComplete(ctx.succeeding(id -> {
                // Send same message twice
                JsonObject msg = new JsonObject()
                    .put("sqsMessageId", "sqs-123")
                    .put("id", "app-456");

                vertx.eventBus().send("router.batch",
                    new JsonObject().put("messages", new JsonArray().add(msg)));
                vertx.eventBus().send("router.batch",
                    new JsonObject().put("messages", new JsonArray().add(msg)));

                // Verify only one reaches pool
                // ...
                ctx.completeNow();
            }));
    }
}
```

### Integration Tests

```java
@Test
void shouldProcessMessageEndToEnd(Vertx vertx, VertxTestContext ctx) {
    // Deploy all verticles with VIRTUAL_THREAD model
    // Send message through router.batch
    // Verify HTTP call to mediator target
    // Verify ACK sent back to router
}
```

---

## 8. Rollback Plan

### Feature Flag Implementation

```java
public class MessageRouterFactory {

    @ConfigProperty(name = "messagerouter.implementation")
    String implementation;  // "legacy" or "vertx"

    public void start() {
        if ("vertx".equals(implementation)) {
            startVertxRouter();
        } else {
            startLegacyRouter();
        }
    }
}
```

---

## 9. Success Criteria

### Functional

- [ ] All existing tests pass
- [ ] Message deduplication works correctly
- [ ] FIFO ordering preserved within message groups
- [ ] Rate limiting works correctly
- [ ] Config changes apply without message loss
- [ ] Graceful shutdown completes all in-flight messages

### Performance

- [ ] Throughput >= current implementation
- [ ] p99 latency <= current implementation + 5ms
- [ ] Memory usage <= current implementation + 10%

### Safety

- [ ] No ConcurrentHashMap anywhere
- [ ] No check-then-act patterns
- [ ] All state owned by single verticle
- [ ] No shared mutable state between verticles
- [ ] All blocking I/O, no async callbacks

---

## 10. Open Questions

1. **Clustering:** Do we need clustered event bus now or later?
2. **Metrics:** Migrate to Vert.x Micrometer or keep current?
3. **Health checks:** How to expose verticle health?
4. **Backpressure:** How to handle slow pools?

---

## Appendix A: Dependency Versions

```gradle
ext {
    vertxVersion = '4.5.0'
}

dependencies {
    implementation "io.vertx:vertx-core:${vertxVersion}"

    // No need for vertx-web-client - using JDK HttpClient
    // No need for async AWS SDK - using sync SqsClient

    testImplementation "io.vertx:vertx-junit5:${vertxVersion}"
}
```

## Appendix B: File Mapping

| Current File | New File(s) |
|--------------|-------------|
| QueueManager.java | QueueManagerVerticle.java |
| ProcessPoolImpl.java | PoolVerticle.java |
| HttpMediator.java | MediatorVerticle.java |
| SqsQueueConsumer.java | QueueConsumerVerticle.java |

## Appendix C: Blocking vs Async Comparison

| Aspect | Async (Event Loop) | Blocking (Virtual Thread) |
|--------|-------------------|---------------------------|
| Code complexity | High (callbacks) | Low (sequential) |
| Libraries | Vert.x WebClient, SqsAsyncClient | JDK HttpClient, SqsClient |
| Error handling | `.onFailure()` chains | try-catch |
| Debugging | Hard (stack traces fragmented) | Easy (full stack traces) |
| Performance | Same | Same |

Virtual threads make blocking I/O as efficient as async I/O, with simpler code.
