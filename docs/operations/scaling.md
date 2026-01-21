# Scaling Guide

Strategies for scaling FlowCatalyst to handle increased load.

## Scaling Dimensions

| Dimension | Method | When to Scale |
|-----------|--------|---------------|
| **Message throughput** | Horizontal scaling (more instances) | Queue depth growing |
| **Pool concurrency** | Increase pool workers | Target endpoints have capacity |
| **Event processing** | Scale event processor | Change stream lag |
| **API requests** | Load balancer | API latency increasing |

## Horizontal Scaling

### Message Router

The message router is stateless and scales horizontally:

```yaml
# Kubernetes HPA
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: flowcatalyst-router
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: flowcatalyst-router
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

### Considerations

- **Config sync**: All instances sync from same config endpoint
- **Queue partitioning**: SQS handles distribution automatically
- **Hot standby**: Enable for leader election if needed

## Vertical Scaling

### Pool Configuration

Increase concurrency per pool:

```json
{
  "processingPools": [
    {"code": "POOL-A", "concurrency": 50, "rateLimitPerMinute": 3000},
    {"code": "POOL-B", "concurrency": 100, "rateLimitPerMinute": 6000}
  ]
}
```

### JVM Resources

```bash
# Increase heap
java -Xms2g -Xmx4g -jar flowcatalyst.jar

# Virtual thread optimization (Java 21)
java -XX:+UseZGC -jar flowcatalyst.jar
```

## Queue Scaling

### SQS

SQS scales automatically. For very high volumes:

1. **Use multiple queues** - Partition by tenant or type
2. **FIFO queues** - 3000 messages/second per queue
3. **Standard queues** - Nearly unlimited throughput

### ActiveMQ

For high-volume ActiveMQ:

1. **Clustering** - Multiple brokers
2. **Persistent storage** - Fast disk or shared storage
3. **Connection pooling** - Increase connections per consumer

## Database Scaling

### MongoDB

1. **Indexes** - Ensure proper indexes exist
2. **Sharding** - For very large datasets
3. **Read replicas** - For read-heavy workloads

Key indexes:
```javascript
db.dispatch_jobs.createIndex({ status: 1, scheduledFor: 1 })
db.dispatch_jobs.createIndex({ clientId: 1, createdAt: -1 })
db.events.createIndex({ type: 1, time: -1 })
```

## Performance Tuning

### Pool Sizing

| Load | Concurrency | Rate Limit | Instances |
|------|-------------|------------|-----------|
| Low (<100/min) | 5 | null | 1 |
| Medium (<1000/min) | 20 | 1000 | 2 |
| High (<10000/min) | 50 | 5000 | 4 |
| Very High | 100+ | 10000+ | 8+ |

### Buffer Sizing

Pool buffer = max(concurrency × 10, 500)

For high concurrency, this scales automatically.

### Rate Limiting

Match rate limits to downstream capacity:

```json
{
  "processingPools": [
    {
      "code": "external-api",
      "concurrency": 50,
      "rateLimitPerMinute": 1000  // Match API limits
    }
  ]
}
```

## Monitoring for Scale

### Key Metrics

| Metric | Scale Signal |
|--------|--------------|
| Queue depth | Growing = add instances |
| CPU utilization | >70% = add instances or CPU |
| Pool active workers | Near concurrency = increase concurrency |
| Error rate | Increasing = check downstream |

### Alerts

```yaml
# Scale-up alert
- alert: HighQueueDepth
  expr: aws_sqs_approximate_number_of_messages_visible > 1000
  for: 5m
  annotations:
    summary: "Consider scaling up FlowCatalyst"
```

## Best Practices

### Gradual Scaling

1. **Monitor baseline** - Understand normal metrics
2. **Scale incrementally** - Add one dimension at a time
3. **Observe impact** - Wait for metrics to stabilize
4. **Document changes** - Track what was changed

### Capacity Planning

1. **Measure current throughput** - Messages/second
2. **Project growth** - Expected increase
3. **Test limits** - Load test to find ceiling
4. **Plan headroom** - 50% buffer for spikes

### Cost Optimization

1. **Right-size instances** - Don't over-provision
2. **Use autoscaling** - Scale down during low load
3. **Monitor queue depth** - Avoid over-polling
4. **Batch operations** - Reduce API calls

## See Also

- [Deployment Guide](deployment.md) - Deployment options
- [Monitoring Guide](monitoring.md) - Observability
- [Message Router](../architecture/message-router.md) - Router architecture
