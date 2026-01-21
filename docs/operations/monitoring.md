# Monitoring Guide

FlowCatalyst provides comprehensive observability through metrics, health checks, structured logging, and a real-time monitoring dashboard.

## Metrics (Prometheus)

### Endpoint

Metrics are exposed at `/q/metrics` in Prometheus format.

### Key Metrics

#### Queue Manager

| Metric | Type | Description |
|--------|------|-------------|
| `flowcatalyst.queuemanager.pools.active` | Gauge | Active processing pools |
| `flowcatalyst.queuemanager.pipeline.size` | Gauge | Messages in pipeline |

#### Queue Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `flowcatalyst.queue.messages.received` | Counter | Messages received |
| `flowcatalyst.queue.messages.processed` | Counter | Messages processed |
| `flowcatalyst.queue.messages.failed` | Counter | Messages failed |

#### Pool Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `flowcatalyst.pool.active.workers` | Gauge | Active workers per pool |
| `flowcatalyst.pool.success.count` | Counter | Successful deliveries |
| `flowcatalyst.pool.error.count` | Counter | Failed deliveries |
| `flowcatalyst.pool.duration.ms` | Histogram | Processing duration |

#### Broker Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `flowcatalyst.broker.connection.attempts` | Counter | Connection attempts |
| `flowcatalyst.broker.connection.successes` | Counter | Successful connections |
| `flowcatalyst.broker.connection.failures` | Counter | Failed connections |
| `flowcatalyst.broker.available` | Gauge | Broker availability (0/1) |

### Prometheus Configuration

```yaml
scrape_configs:
  - job_name: 'flowcatalyst'
    scrape_interval: 15s
    metrics_path: /q/metrics
    static_configs:
      - targets: ['flowcatalyst:8080']
```

## Health Checks

### Endpoints

| Endpoint | Purpose | Checks |
|----------|---------|--------|
| `/health/live` | Liveness | App running, not deadlocked |
| `/health/ready` | Readiness | Ready for traffic |
| `/health/startup` | Startup | Initialization complete |

### Readiness Checks

- QueueManager initialized
- Message broker accessible
- Processing pools operational

### Example Response

```json
{
  "status": "UP",
  "checks": [
    {
      "name": "QueueManager",
      "status": "UP",
      "data": {
        "pools": 5,
        "consumers": 2
      }
    },
    {
      "name": "BrokerConnection",
      "status": "UP",
      "data": {
        "type": "SQS",
        "region": "us-east-1"
      }
    }
  ]
}
```

## Monitoring Dashboard

Real-time dashboard available at `/dashboard.html`.

### Features

- Queue statistics (received, processed, failed, throughput)
- Pool metrics (concurrency, active workers, success/error rates)
- System warnings and alerts
- Circuit breaker status

### API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /monitoring/health` | Overall system health |
| `GET /monitoring/queue-stats` | Queue statistics |
| `GET /monitoring/pool-stats` | Pool statistics |
| `GET /monitoring/warnings` | Active warnings |
| `GET /monitoring/circuit-breakers` | Circuit breaker states |

## Structured Logging

### Format

Production uses JSON-formatted logs:

```json
{
  "timestamp": "2024-01-15T10:30:00Z",
  "level": "INFO",
  "logger": "tech.flowcatalyst.messagerouter.manager.QueueManager",
  "message": "Message processed successfully",
  "mdc": {
    "messageId": "0HZXEQ5Y8JY5Z",
    "poolCode": "POOL-A",
    "queueName": "dispatch-queue",
    "targetUri": "https://api.example.com/webhook",
    "result": "SUCCESS",
    "durationMs": "150"
  }
}
```

### MDC Fields

| Field | Description |
|-------|-------------|
| `messageId` | Unique message identifier |
| `poolCode` | Processing pool |
| `queueName` | Source queue |
| `targetUri` | Destination endpoint |
| `result` | Processing result |
| `durationMs` | Duration in milliseconds |
| `correlationId` | Request correlation |

### Log Queries

```bash
# Find failed messages for a pool
level:ERROR AND poolCode:POOL-A

# Find slow processing
durationMs:>5000

# Track specific message
messageId:0HZXEQ5Y8JY5Z
```

### Configuration

```properties
# Development - human readable
%dev.quarkus.log.console.format=%d{HH:mm:ss} %-5p [%c{2.}] (%t) %s%e%n

# Production - JSON
%prod.quarkus.log.console.json.enabled=true
%prod.quarkus.log.console.json.pretty-print=false
```

## Alerting

### Recommended Alerts

#### Critical

| Alert | Condition | Description |
|-------|-----------|-------------|
| BrokerDown | `flowcatalyst.broker.available == 0` | Broker unavailable |
| HighErrorRate | `error_rate > 10%` | Too many failures |
| QueueBacklog | `queue_depth > 10000` | Large backlog |

#### Warning

| Alert | Condition | Description |
|-------|-----------|-------------|
| HighLatency | `p99_latency > 5s` | Slow processing |
| PoolsNearLimit | `pools > 1500` | Approaching pool limit |
| CircuitOpen | Circuit breaker open | Downstream issues |

### Prometheus Alerting Rules

```yaml
groups:
- name: flowcatalyst
  rules:
  - alert: BrokerUnavailable
    expr: flowcatalyst_broker_available == 0
    for: 1m
    labels:
      severity: critical
    annotations:
      summary: "Message broker unavailable"

  - alert: HighErrorRate
    expr: rate(flowcatalyst_pool_error_count[5m]) / rate(flowcatalyst_pool_success_count[5m]) > 0.1
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "High error rate detected"
```

## Grafana Dashboard

### Import Dashboard

```json
{
  "title": "FlowCatalyst Overview",
  "panels": [
    {
      "title": "Messages Processed",
      "type": "graph",
      "targets": [
        {
          "expr": "rate(flowcatalyst_queue_messages_processed[1m])"
        }
      ]
    },
    {
      "title": "Active Pools",
      "type": "stat",
      "targets": [
        {
          "expr": "flowcatalyst_queuemanager_pools_active"
        }
      ]
    }
  ]
}
```

## Configuration

```properties
# Enable metrics
quarkus.micrometer.enabled=true
quarkus.micrometer.export.prometheus.enabled=true

# Health check configuration
quarkus.smallrye-health.root-path=/health

# Logging
quarkus.log.level=INFO
quarkus.log.category."tech.flowcatalyst".level=DEBUG
```

## See Also

- [Deployment Guide](deployment.md) - Deployment setup
- [Troubleshooting](troubleshooting.md) - Common issues
- [Scaling Guide](scaling.md) - Scaling strategies
