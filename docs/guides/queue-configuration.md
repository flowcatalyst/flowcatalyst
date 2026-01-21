# Queue Configuration Guide

FlowCatalyst supports multiple queue backends: AWS SQS, Apache ActiveMQ, and an embedded SQLite queue. This guide covers setup and configuration for each.

## Queue Type Selection

| Queue | Use Case | Persistence | Scalability |
|-------|----------|-------------|-------------|
| **SQS** | Production AWS deployments | Yes | High |
| **ActiveMQ** | On-premise, hybrid | Yes | Medium |
| **Embedded** | Development, testing | SQLite file | Single node |

### Setting Queue Type

```properties
message-router.queue-type=SQS  # or ACTIVEMQ, EMBEDDED
```

Or via environment variable:
```bash
MESSAGE_ROUTER_QUEUE_TYPE=SQS
```

## AWS SQS

### Prerequisites

- AWS account with SQS access
- IAM credentials configured

### Configuration

```properties
# Queue type
message-router.queue-type=SQS

# AWS region
quarkus.sqs.aws.region=us-east-1

# Optional: Custom endpoint (for LocalStack)
sqs.endpoint-override=http://localhost:4566
```

### IAM Permissions

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "sqs:SendMessage",
        "sqs:ReceiveMessage",
        "sqs:DeleteMessage",
        "sqs:GetQueueUrl",
        "sqs:GetQueueAttributes"
      ],
      "Resource": "arn:aws:sqs:*:*:flowcatalyst-*"
    }
  ]
}
```

### Development with LocalStack

```bash
# Start LocalStack
docker run -d --name localstack \
  -p 4566:4566 \
  -e SERVICES=sqs \
  localstack/localstack

# Configure FlowCatalyst
SQS_ENDPOINT_OVERRIDE=http://localhost:4566 \
AWS_REGION=us-east-1 \
AWS_ACCESS_KEY_ID=test \
AWS_SECRET_ACCESS_KEY=test \
./gradlew :core:flowcatalyst-message-router:quarkusDev
```

### SQS Features

- Long polling (20 seconds)
- Up to 10 messages per poll
- Visibility timeout for redelivery
- FIFO queues supported (optional)

## Apache ActiveMQ

### Prerequisites

- ActiveMQ Artemis broker running
- Network access to broker

### Configuration

```properties
# Queue type
message-router.queue-type=ACTIVEMQ

# Broker connection
activemq.broker.url=tcp://localhost:61616
activemq.username=admin
activemq.password=admin
```

### Running ActiveMQ

```bash
# Docker
docker run -d --name activemq \
  -p 61616:61616 \
  -p 8161:8161 \
  apache/activemq-artemis:latest

# Web console: http://localhost:8161
# Default credentials: admin/admin
```

### ActiveMQ Features

- INDIVIDUAL_ACKNOWLEDGE mode (no head-of-line blocking)
- Redelivery policy: 30-second delay
- Shared connections with per-thread sessions
- Supports clustering for HA

### Connection Configuration

```properties
# Connection pool
activemq.connections=5

# Redelivery settings
activemq.redelivery.delay=30000
activemq.redelivery.max-attempts=5
```

## Embedded SQLite Queue

Embedded, SQLite-based queue for development and testing. Provides full SQS FIFO semantics without external dependencies.

### Prerequisites

None - the embedded queue uses SQLite which is bundled with the application.

### Configuration

```properties
# Queue type
message-router.queue-type=EMBEDDED

# Database file location
quarkus.datasource.embedded-queue.jdbc.url=jdbc:sqlite:./flowcatalyst-queue.db

# Visibility timeout (seconds)
message-router.embedded.visibility-timeout-seconds=30

# Receive timeout (milliseconds)
message-router.embedded.receive-timeout-ms=1000
```

### Development Mode

```bash
# Run with embedded queue (no external broker needed)
./gradlew :core:flowcatalyst-message-router:quarkusDev \
  -Dmessage-router.queue-type=EMBEDDED
```

### Features

The embedded SQLite queue implements full SQS FIFO semantics:

- **Message Groups** - FIFO ordering per group, concurrent across groups
- **Visibility Timeout** - Message locking during processing
- **Deduplication** - Prevents duplicate message processing
- **ACK/NACK** - Configurable retry delays
- **Thread-safe** - SQLite handles concurrent access via database locking
- **Virtual thread compatible** - No thread affinity issues

### Database Schema

Messages are stored in a `queue_messages` table with:

| Column | Description |
|--------|-------------|
| `id` | Auto-increment primary key |
| `message_id` | Unique message identifier |
| `message_group_id` | FIFO ordering group |
| `message_json` | Message payload |
| `visible_at` | Timestamp when message becomes visible |
| `receipt_handle` | Handle for ACK/NACK operations |
| `receive_count` | Number of delivery attempts |
| `first_received_at` | First delivery timestamp |

## Environment Variables

### Common

| Variable | Description | Default |
|----------|-------------|---------|
| `MESSAGE_ROUTER_QUEUE_TYPE` | Queue backend | `SQS` |
| `MESSAGE_ROUTER_CONFIG_URL` | Config endpoint | `http://localhost:8080/api/config` |

### SQS

| Variable | Description | Default |
|----------|-------------|---------|
| `SQS_ENDPOINT_OVERRIDE` | Custom endpoint | (none) |
| `AWS_REGION` | AWS region | `eu-west-1` |
| `AWS_ACCESS_KEY_ID` | AWS access key | (default chain) |
| `AWS_SECRET_ACCESS_KEY` | AWS secret key | (default chain) |

### ActiveMQ

| Variable | Description | Default |
|----------|-------------|---------|
| `ACTIVEMQ_BROKER_URL` | Broker URL | `tcp://localhost:61616` |
| `ACTIVEMQ_USERNAME` | Username | `admin` |
| `ACTIVEMQ_PASSWORD` | Password | `admin` |

### Embedded SQLite

| Variable | Description | Default |
|----------|-------------|---------|
| `EMBEDDED_QUEUE_DB_URL` | SQLite database URL | `jdbc:sqlite:./flowcatalyst-queue.db` |
| `EMBEDDED_VISIBILITY_TIMEOUT_SECONDS` | Visibility timeout | `30` |
| `EMBEDDED_RECEIVE_TIMEOUT_MS` | Poll timeout | `1000` |

## Profiles

### Default (dev)

SQS with LocalStack endpoint:
```bash
./gradlew quarkusDev
```

### Embedded Dev

Embedded SQLite queue, no external broker:
```bash
./gradlew quarkusDev -Dmessage-router.queue-type=EMBEDDED
```

### Production

Configure via environment:
```bash
MESSAGE_ROUTER_QUEUE_TYPE=SQS \
AWS_REGION=us-east-1 \
java -jar flowcatalyst.jar
```

## Best Practices

### SQS

1. **Use FIFO queues** for strict ordering
2. **Configure dead letter queues** for failed messages
3. **Set appropriate visibility timeout** (> processing time)
4. **Monitor queue depth** for backpressure

### ActiveMQ

1. **Use clustering** for high availability
2. **Configure persistence** for durability
3. **Monitor broker metrics** (connections, memory)
4. **Set up DLQ** for poison messages

### Embedded SQLite

1. **Use for development/testing** - not for distributed production
2. **Monitor database size** - file grows over time
3. **Clean processed messages** - messages are deleted on ACK
4. **Single instance only** - not suitable for multi-node deployments

## Troubleshooting

### SQS Connection Issues

```bash
# Test connectivity
aws sqs list-queues --region us-east-1

# Check LocalStack
curl http://localhost:4566/health
```

### ActiveMQ Connection Refused

```bash
# Check broker is running
docker ps | grep activemq

# Test connection
nc -zv localhost 61616
```

### Embedded Queue Issues

```bash
# Check database file
ls -la ./flowcatalyst-queue.db

# Verify SQLite
sqlite3 ./flowcatalyst-queue.db "SELECT COUNT(*) FROM queue_messages"

# Check pending messages
sqlite3 ./flowcatalyst-queue.db "SELECT * FROM queue_messages WHERE visible_at <= strftime('%s','now')*1000"
```

## See Also

- [Message Router](../architecture/message-router.md) - Router architecture
- [Development Setup](../development/setup.md) - Getting started
- [Deployment Guide](../operations/deployment.md) - Production deployment
