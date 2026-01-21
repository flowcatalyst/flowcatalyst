# Troubleshooting Guide

Common issues and solutions for FlowCatalyst deployments.

## Connection Issues

### MongoDB Connection Failed

**Symptoms**: Application fails to start, "Connection refused" errors

**Solutions**:
```bash
# Check MongoDB is running
mongosh --eval "db.runCommand({ping:1})"

# Verify connection string
echo $QUARKUS_MONGODB_CONNECTION_STRING

# Test connectivity
nc -zv mongodb-host 27017
```

### SQS Connection Failed

**Symptoms**: Queue consumer not starting, AWS errors

**Solutions**:
```bash
# Check AWS credentials
aws sts get-caller-identity

# Test SQS access
aws sqs list-queues --region us-east-1

# For LocalStack
curl http://localhost:4566/health
```

### ActiveMQ Connection Refused

**Symptoms**: "Connection refused" on port 61616

**Solutions**:
```bash
# Check broker is running
docker ps | grep activemq

# Test connection
nc -zv localhost 61616

# Check broker logs
docker logs activemq
```

## Message Processing Issues

### Messages Not Processing

**Symptoms**: Messages stuck in queue, no processing activity

**Checklist**:
1. Check QueueManager is initialized: `GET /health/ready`
2. Verify config endpoint is accessible
3. Check pool configuration returned from config endpoint
4. Review logs for routing errors

```bash
# Check health
curl http://localhost:8080/health/ready

# Check config
curl http://localhost:8080/api/config

# Check logs
grep "QueueManager" application.log
```

### High Error Rate

**Symptoms**: Many failed deliveries, circuit breaker open

**Investigation**:
```bash
# Check circuit breaker status
curl http://localhost:8080/monitoring/circuit-breakers

# Review error logs
grep "ERROR" application.log | grep "HttpMediator"

# Check target endpoint
curl -X POST https://target-endpoint.com/test
```

### Messages Redelivered Repeatedly

**Symptoms**: Same message processed multiple times

**Causes & Solutions**:
1. **Processing timeout** - Increase `timeoutSeconds`
2. **Visibility timeout too short** - Adjust SQS settings
3. **Application crash during processing** - Check for OOM errors

## Performance Issues

### High Latency

**Symptoms**: Slow message processing, timeouts

**Investigation**:
```bash
# Check pool statistics
curl http://localhost:8080/monitoring/pool-stats

# Check rate limiting
# Look for "rate-limited" in logs
grep "rate-limited" application.log
```

**Solutions**:
1. Increase pool concurrency
2. Adjust rate limits
3. Scale horizontally
4. Optimize target endpoints

### Memory Issues

**Symptoms**: OOM errors, GC pauses

**Solutions**:
```bash
# Check memory usage
jcmd <pid> GC.heap_info

# Increase heap
java -Xmx4g -jar flowcatalyst.jar

# Monitor with JFR
java -XX:StartFlightRecording=duration=60s,filename=recording.jfr -jar flowcatalyst.jar
```

### Pool Limit Reached

**Symptoms**: Warning about pool count, new pools not created

**Solutions**:
```bash
# Check current pool count
curl http://localhost:8080/monitoring/pool-stats | jq '.pools | length'

# Increase limit if needed
MESSAGE_ROUTER_MAX_POOLS=3000 java -jar flowcatalyst.jar
```

## Authentication Issues

### OIDC Login Fails

**Symptoms**: Redirect errors, "Invalid issuer" errors

**Investigation**:
1. Check ClientAuthConfig for domain
2. Verify OIDC issuer URL is accessible
3. Check client ID and secret

```bash
# Test OIDC discovery
curl https://your-idp.com/.well-known/openid-configuration

# Check auth config
mongosh flowcatalyst --eval "db.client_auth_config.find({emailDomain: 'example.com'})"
```

### Service Account Authentication Failed

**Symptoms**: 401/403 on API calls

**Checklist**:
1. Verify API key format (`fc_sk_...`)
2. Check service account is active
3. Verify roles and permissions

```bash
# Check service account
mongosh flowcatalyst --eval "db.service_accounts.find({code: 'my-service'})"
```

## Dispatch Job Issues

### Jobs Stuck in PENDING

**Symptoms**: Jobs not being queued

**Investigation**:
```bash
# Check scheduler is running
grep "DispatchScheduler" application.log

# Check job count
mongosh flowcatalyst --eval "db.dispatch_jobs.countDocuments({status: 'PENDING'})"
```

### Jobs Blocked (BLOCK_ON_ERROR)

**Symptoms**: Message group not processing

**Resolution**:
```bash
# Find blocking job
mongosh flowcatalyst --eval "
  db.dispatch_jobs.find({
    status: 'ERROR',
    mode: 'BLOCK_ON_ERROR'
  })
"

# Manual retry or change mode
mongosh flowcatalyst --eval "
  db.dispatch_jobs.updateOne(
    {_id: 'blocking-job-id'},
    {\$set: {status: 'PENDING', attemptCount: 0}}
  )
"
```

### Webhook Signature Verification Failed

**Symptoms**: Target returns 401, signature mismatch

**Checklist**:
1. Verify signing secret matches
2. Check timestamp tolerance (default 5 minutes)
3. Ensure payload not modified by proxy

## Data Issues

### TSID Decode Error

**Symptoms**: "Cannot decode TSID" errors

**Cause**: Old Long IDs mixed with new String IDs

**Solution**:
```javascript
// Migrate Long IDs to String
db.collection.find({_id: {$type: "long"}}).forEach(function(doc) {
  var newId = TsidGenerator.toString(doc._id);
  // Create new document with string ID
  // Delete old document
})
```

### Type Mismatch Errors

**Symptoms**: "expected 'DATE_TIME' got 'STRING'" errors

**Solution**:
```javascript
// Fix date fields
db.dispatch_jobs.find({createdAt: {$type: "string"}}).forEach(function(doc) {
  db.dispatch_jobs.updateOne(
    {_id: doc._id},
    {$set: {createdAt: new Date(doc.createdAt)}}
  )
})
```

## Diagnostic Commands

### Health Check

```bash
# Full health status
curl http://localhost:8080/health

# Just liveness
curl http://localhost:8080/health/live

# Just readiness
curl http://localhost:8080/health/ready
```

### Metrics

```bash
# All metrics
curl http://localhost:8080/q/metrics

# Specific metric
curl http://localhost:8080/q/metrics | grep flowcatalyst
```

### Logs

```bash
# Enable debug logging
QUARKUS_LOG_CATEGORY__TECH_FLOWCATALYST__LEVEL=DEBUG java -jar flowcatalyst.jar

# Filter specific component
grep "QueueManager" application.log
grep "HttpMediator" application.log
```

## Getting Help

1. **Check logs** - Most issues are visible in logs
2. **Review metrics** - Prometheus metrics show trends
3. **Health endpoints** - Quick status check
4. **GitHub Issues** - Report bugs and get support

## See Also

- [Monitoring Guide](monitoring.md) - Observability setup
- [Deployment Guide](deployment.md) - Deployment configuration
- [Queue Configuration](../guides/queue-configuration.md) - Queue setup
