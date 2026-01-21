# Message Processing Debug Guide

## Recent Changes - Enhanced Logging

Enhanced INFO-level logging has been added to trace message processing from queue to completion. This document shows what logs to expect and how to diagnose issues.

## Expected Log Flow for a Message

For message ID `01K97FHM11EKYSXT135MVM6AC7`:

### 1. Message Received from Queue
```
INFO: QueueConsumer: Received 1 message(s) from queue [FC-staging-cldc-release-staging.fifo]
INFO: QueueConsumer: Parsing message [01K97FHM11EKYSXT135MVM6AC7] from queue [FC-staging-cldc-release-staging.fifo] - poolCode: staging-twinsaver-logistics_portal-RATE_LIMIT-120, target: https://staging-integral.inhancesc.com/api/integral/api/v1/message-pointer
```

**If you DON'T see this:** Messages are not being consumed from SQS. Check:
- Is the queue consumer running?
- Is standby mode enabled and is this instance PRIMARY?
- Are there messages in the queue?

### 2. Message Routed to Pool
```
INFO: Processing message [01K97FHM11EKYSXT135MVM6AC7] in pool [staging-twinsaver-logistics_portal-RATE_LIMIT-120] via mediator to [https://staging-integral.inhancesc.com/api/integral/api/v1/message-pointer]
```

**If you DON'T see this:** Message is stuck before reaching the pool. Check:
- Is the pool full? (concurrency limit reached)
- Is rate limiting blocking it?

### 3. HTTP Request Preparation
```
INFO: HttpMediator: Attempting to process message [01K97FHM11EKYSXT135MVM6AC7] via HTTP POST to [https://staging-integral.inhancesc.com/api/integral/api/v1/message-pointer]
INFO: HttpMediator: Payload: {"messageId":"01K97FHM11EKYSXT135MVM6AC7"}
INFO: HttpMediator: Authorization token (first 20 chars): 4730a4fdc549d7ba680f...
INFO: HttpMediator: HTTP request built. Sending to [https://staging-integral.inhancesc.com/api/integral/api/v1/message-pointer] with HTTP version [HTTP_2], timeout: 900000ms
```

**If you DON'T see this:** The mediator is not being invoked. Check for errors in previous steps.

### 4. HTTP Response Received
```
INFO: HttpMediator: HTTP request completed in 250ms. Status code: 200
INFO: Message [01K97FHM11EKYSXT135MVM6AC7] received 200 OK. Raw response body: {"ack":true,"message":"Record is blocked. Will be processed later"}
INFO: Message [01K97FHM11EKYSXT135MVM6AC7] parsed MediationResponse - ack: true, message: Record is blocked. Will be processed later
INFO: Message [01K97FHM11EKYSXT135MVM6AC7] processed successfully with ack=true - will ACK
```

**If you see different status codes:**
- **401/403:** Authorization failure - check auth token
- **404:** Endpoint not found - check URL
- **500:** Server error - check PHP application logs
- **Timeout:** Check if endpoint is responding

**If you see `ack: false`:**
```
WARN: Message [01K97FHM11EKYSXT135MVM6AC7] received 200 OK but ack=false - will NACK and retry. Reason: Record is blocked...
```
This means the endpoint returned `ack: false` and the message will be retried.

### 5. Processing Complete
```
INFO: Message [01K97FHM11EKYSXT135MVM6AC7] processing completed with result [SUCCESS] in 250ms
INFO: Message [01K97FHM11EKYSXT135MVM6AC7] processed successfully - ACKing and removing from queue
```

**If you see `ERROR_PROCESS`:**
```
WARN: Message [01K97FHM11EKYSXT135MVM6AC7] encountered transient error - NACKing for retry: ERROR_PROCESS
```
This means the message will be retried (either `ack: false` or server error).

### 6. SQS Delete Operation
```
INFO: SQS: ACKing message [01K97FHM11EKYSXT135MVM6AC7] - calling SQS DeleteMessage API
INFO: SQS: Successfully deleted message [01K97FHM11EKYSXT135MVM6AC7] from queue
```

**If you see receipt handle error:**
```
INFO: SQS: Receipt handle expired for message [01K97FHM11EKYSXT135MVM6AC7] (SQS ID: xxx) - added to pending delete set
```
**This means** the visibility timeout expired before the delete could complete. The router tracks these messages and will delete them when they reappear.

**Root Cause:** Processing took longer than the SQS visibility timeout.

**Note:** The router automatically extends message visibility every 55 seconds for long-running tasks. If you see this error, either:
1. The endpoint took extremely long to respond (>55 seconds between extensions)
2. There was a network issue preventing the visibility extension
3. The SQS queue visibility timeout is configured too short (recommend 120 seconds)

## Common Issues

### Issue 1: No Logs At All

**Symptoms:** No "QueueConsumer: Received" logs appearing

**Possible Causes:**
1. **Standby mode** - This instance is STANDBY, not PRIMARY
   - Check: `curl http://localhost:8080/monitoring/standby-status`
   - Should show: `"role": "PRIMARY"`

2. **No messages in queue**
   - Check: `curl http://localhost:8080/monitoring/queue-stats`
   - Look for `pendingMessages` count

3. **Consumer not running**
   - Check application logs for consumer startup
   - Check for errors during initialization

### Issue 2: Logs Show Success But Message Keeps Retrying

**Symptoms:**
- See "processed successfully with ack=true"
- See "ACKing and removing from queue"
- BUT message appears again after visibility timeout expires

**Root Cause:** Receipt handle expired before the DeleteMessage API call completed.

**Look for:**
```
INFO: SQS: Receipt handle expired for message [01K97FHM11...] (SQS ID: xxx) - added to pending delete set
```

**Good news:** The router automatically tracks these and will delete them when they reappear. You'll then see:
```
INFO: SQS message [xxx] was previously processed - deleting from queue now
INFO: SQS message [xxx] deleted successfully
```

**If this happens frequently:**
1. Increase SQS visibility timeout in queue settings (recommended: 120s)
2. Check for network latency between router and SQS
3. The router extends visibility every 55 seconds - ensure this is working

### Issue 3: Endpoint Not Receiving Requests

**Symptoms:**
- See "HttpMediator: Attempting to process message..."
- See "HTTP request built. Sending to..."
- BUT no corresponding logs in PHP application

**Possible Causes:**
1. **Wrong endpoint URL** - Check the `mediationTarget` in logs
2. **Network/firewall issue** - HTTP request is failing silently
3. **DNS resolution failure** - Domain not resolving

**Look for:**
```
ERROR: Connection error processing message: 01K97FHM11...
ERROR: Timeout processing message: 01K97FHM11...
ERROR: IO error processing message: 01K97FHM11...
```

### Issue 4: Endpoint Returns ack=true But Wrong Format

**Symptoms:** Seeing parse errors

**Look for:**
```
WARN: Message [01K97FHM11...] received 200 OK but response was not valid MediationResponse (parse error: ...). Response body: ... - treating as success and ACKing
```

**Cause:** Response format is incorrect. Must be exactly:
```json
{"ack": true, "message": "some message"}
```

Check for:
- Extra fields
- Wrong field names (case sensitive)
- `ack` as string instead of boolean: `"true"` vs `true`

## How to Check Current Status

### Check if this instance is PRIMARY
```bash
curl http://localhost:8080/monitoring/standby-status
```

Expected output:
```json
{
  "standbyEnabled": true,
  "instanceId": "router-1",
  "role": "PRIMARY",
  "redisAvailable": true,
  "currentLockHolder": "locked",
  "lastSuccessfulRefresh": "2025-01-18T10:30:45Z"
}
```

### Check queue statistics
```bash
curl http://localhost:8080/monitoring/queue-stats
```

Look for:
- `pendingMessages`: Messages waiting in queue
- `messagesNotVisible`: Messages currently being processed
- `successRate30min`: Should be > 0.90 (90%)

### Check pool statistics
```bash
curl http://localhost:8080/monitoring/pool-stats
```

Look for your pool and check:
- `successRate30min`: Should be > 0.90 (90%)
- `queueSize`: Messages waiting in pool buffer
- `activeWorkers`: Currently processing
- `maxConcurrency`: Maximum concurrent workers

### Search logs for specific message
```bash
# If using CloudWatch, Datadog, etc.
messageId:"01K97FHM11EKYSXT135MVM6AC7"

# If using local logs
grep "01K97FHM11EKYSXT135MVM6AC7" /var/log/application.log
```

You should see 10 log lines for a successful message.

## Deployment Checklist

After deploying these logging changes:

1. ✅ Build completed successfully
2. ✅ Docker image built
3. ✅ Deployed to staging environment
4. ✅ Application started successfully
5. ✅ Logs showing INFO level for `tech.flowcatalyst.messagerouter.mediator`
6. ✅ Test message sent to queue
7. ✅ Check logs for "QueueConsumer: Received" message
8. ✅ Trace message ID through all 10 log points
9. ✅ Verify message is deleted from SQS (or see receipt handle error)

## Next Steps

If you still don't see logs after deployment:

1. **Verify logging configuration:**
   ```properties
   %prod.quarkus.log.category."tech.flowcatalyst.messagerouter.mediator".level=INFO
   ```

2. **Check log aggregation is working** - Test with a simple log:
   ```bash
   curl http://localhost:8080/some-endpoint
   ```
   Should generate logs that appear in your system.

3. **Check if instance is PRIMARY** - Only PRIMARY processes messages.

4. **Check SQS queue has messages** - Send a test message.

5. **Check for startup errors** - Application might not have started correctly.
