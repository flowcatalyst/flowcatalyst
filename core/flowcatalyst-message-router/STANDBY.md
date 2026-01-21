# Hot Standby Mode

Flow Catalyst Message Router supports optional **hot standby mode** for high availability deployments. When enabled, one instance acts as the primary processor while another waits in standby. If the primary fails, the standby automatically takes over without data loss.

## Features

- **Opt-in**: Disabled by default, no Redis dependency if not used
- **Automatic Failover**: Standby takes over in ~30 seconds if primary fails
- **Graceful Takeover**: When primary shuts down, standby takes over immediately
- **Redis-Based Lock**: Uses Redis for atomic leader election
- **Health-Aware**: Reports degraded health if Redis is unavailable
- **Critical Warnings**: Fires warnings if Redis fails in distributed setup

## Architecture

### Lock Mechanism

The system uses Redis distributed locks for leader election:

```
Primary Instance:
  1. Startup: Attempt to acquire lock
  2. Success: Become primary, start processing messages
  3. Every 10 seconds: Refresh lock with 30-second TTL
  4. Shutdown: Release lock immediately

Standby Instance:
  1. Startup: Attempt to acquire lock
  2. Fail: Detect another instance holds lock
  3. Every 10 seconds: Check if lock expired/holder changed
  4. Lock expires (30s): Acquire lock, become primary
```

### Failover Sequence

```
Time 0s:    Primary holds lock (expires at 30s)
Time 0s:    Standby detects lock held by other instance
Time 10s:   Primary refreshes lock (now expires at 40s)
Time 10s:   Standby tries to acquire lock (fails)
...
Time 27s:   Primary crashes
Time 27s:   Primary stops refreshing lock
Time 37s:   Lock naturally expires in Redis
Time 40s:   Standby refresh task detects lock doesn't exist
Time 40s:   Standby acquires lock, becomes primary
Time 40s:   Standby starts processing queued messages
```

**Total failover time**: ~10-30 seconds (configurable)

## Configuration

### Enable Hot Standby

Set in `application.properties`:

```properties
# Enable hot standby mode
standby.enabled=true

# Unique identifier for this instance (defaults to HOSTNAME)
standby.instance-id=router-1

# Redis lock settings
standby.lock-key=message-router-primary-lock
standby.lock-ttl-seconds=30
# Note: Lock refresh interval is hardcoded to 10 seconds
```

Or via environment variables:

```bash
export STANDBY_ENABLED=true
export STANDBY_INSTANCE_ID=router-1

# Simple Redis URL (dev/test)
export REDIS_HOSTS=redis://redis.example.com:6379

# Or for production with TLS and credentials
export REDIS_USERNAME=admin
export REDIS_PASSWORD=secret
export REDIS_HOST=redis.example.com
export REDIS_PORT=6379
```

### Single Instance Mode (Default)

When `standby.enabled=false` (default):

```properties
standby.enabled=false
```

- Single instance operates as primary
- No Redis dependency
- No lock acquisition/refresh overhead
- No standby failover capability

### Redis Configuration

Redis is only required if `standby.enabled=true`:

```properties
# Redis connection (required only if standby.enabled=true)
# Defaults to redis://localhost:6379 if not set
quarkus.redis.hosts=${REDIS_HOSTS:redis://localhost:6379}
quarkus.redis.max-pool-size=4

# Production: Use TLS-encrypted Redis
%prod.quarkus.redis.hosts=rediss://${REDIS_USERNAME}:${REDIS_PASSWORD}@${REDIS_HOST}:${REDIS_PORT}
```

## Monitoring

### Health Checks

When standby is enabled:

- **Liveness probe** (`/health/live`): DOWN if Redis unavailable
- **Readiness probe** (`/health/ready`): DOWN if Redis unavailable
- Kubernetes automatically removes unhealthy instances from service

### Standby Status Endpoint

Query the standby status:

```bash
# Check standby mode
curl http://localhost:8080/monitoring/standby-status
```

Response:

```json
{
  "standbyEnabled": true,
  "instanceId": "router-1",
  "role": "PRIMARY",
  "redisAvailable": true,
  "currentLockHolder": "router-1",
  "lastSuccessfulRefresh": "2024-11-17T10:30:45Z",
  "hasWarning": false
}
```

Or if standby is disabled:

```json
{
  "standbyEnabled": false
}
```

### Warning System Integration

Critical warnings are fired when Redis becomes unavailable:

- **Title**: "Standby Redis Connection Lost"
- **Severity**: CRITICAL
- **Includes**: Instance ID, last successful refresh, configured Redis endpoint
- **Auto-clears**: When Redis is restored
- **Triggers**: Email, Slack, Teams notifications (if configured)

### Logs

Look for these log messages:

```
# Startup - becoming primary
[INFO] Acquired primary lock. Starting message processing.

# Startup - standby mode
[INFO] Standby mode: Waiting for primary to fail. Will attempt takeover in 30 seconds.

# Failover
[INFO] Acquired primary lock. Taking over message processing.

# Lock loss
[WARNING] Lost primary lock. Switching to standby mode.

# Redis failure
[SEVERE] Redis unavailable at startup. System will not process messages until Redis is restored.
```

## Error Handling

### Redis Unavailable at Startup

```
Primary: Cannot acquire lock → Blocks, system unhealthy
Standby: Cannot acquire lock → Blocks, system unhealthy
```

**Action**: Restore Redis connection

### Redis Unavailable While Running

```
Primary: Cannot refresh lock → Loses it to another instance
Standby: Cannot check lock status → Waits, doesn't take over
```

**Result**: System stops processing (safe failure)
**Action**: Restore Redis or restart with `standby.enabled=false`

### Network Split (Rare)

If primary and standby can't reach Redis but are both running:

```
Primary: Cannot refresh lock → Reverts to standby
Standby: Cannot acquire lock → Stays standby
No instance processes messages (safe)
```

**Result**: System safe but non-operational
**Action**: Restore network or Redis connectivity

## Deployment Examples

### Docker - Two Instances with Redis

```dockerfile
# Primary instance
docker run -d \
  --name message-router-primary \
  -e STANDBY_ENABLED=true \
  -e STANDBY_INSTANCE_ID=router-primary \
  -e REDIS_HOSTS=redis://redis:6379 \
  -p 8080:8080 \
  --link redis:redis \
  message-router:latest

# Standby instance
docker run -d \
  --name message-router-standby \
  -e STANDBY_ENABLED=true \
  -e STANDBY_INSTANCE_ID=router-standby \
  -e REDIS_HOSTS=redis://redis:6379 \
  -p 8081:8080 \
  --link redis:redis \
  message-router:latest

# Redis (persistent)
docker run -d \
  --name redis \
  -v redis-data:/data \
  redis:latest redis-server --appendonly yes
```

### Kubernetes with Standby

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: message-router-config
data:
  application.properties: |
    standby.enabled=true
    standby.lock-key=message-router-primary-lock
    standby.lock-ttl-seconds=30

---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: message-router
spec:
  serviceName: message-router
  replicas: 2
  selector:
    matchLabels:
      app: message-router
  template:
    metadata:
      labels:
        app: message-router
    spec:
      containers:
      - name: message-router
        image: message-router:latest
        env:
        - name: STANDBY_ENABLED
          value: "true"
        - name: STANDBY_INSTANCE_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: REDIS_HOSTS
          value: redis://redis.default.svc.cluster.local:6379
        ports:
        - containerPort: 8080
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
          failureThreshold: 2
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "1Gi"
            cpu: "1000m"

---
apiVersion: v1
kind: Service
metadata:
  name: message-router
spec:
  selector:
    app: message-router
  ports:
  - port: 8080
  clusterIP: None

---
# Redis for distributed lock
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis:latest
        command: ["redis-server", "--appendonly", "yes"]
        ports:
        - containerPort: 6379
        volumeMounts:
        - name: redis-storage
          mountPath: /data
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
      volumes:
      - name: redis-storage
        emptyDir: {}
```

### Kubernetes with Redis Helm Chart

For production, use the official Redis Helm chart:

```bash
helm repo add bitnami https://charts.bitnami.com/bitnami
helm install redis bitnami/redis \
  --set auth.enabled=true \
  --set auth.password=your-secure-password \
  --set persistence.enabled=true \
  --set persistence.size=10Gi
```

Then set Redis connection in Message Router:

```yaml
env:
- name: REDIS_HOSTS
  value: redis://redis-master.default.svc.cluster.local:6379
# Or for authenticated Redis
- name: REDIS_USERNAME
  valueFrom:
    secretKeyRef:
      name: redis-secret
      key: username
- name: REDIS_PASSWORD
  valueFrom:
    secretKeyRef:
      name: redis-secret
      key: password
- name: REDIS_HOST
  value: redis-master.default.svc.cluster.local
- name: REDIS_PORT
  value: "6379"
```

## Troubleshooting

### Standby instance is stuck waiting

**Symptom**: Second instance logs "Waiting for primary lock" but never takes over

**Diagnosis**:
```bash
# Check if primary is running
ps aux | grep message-router

# Check Redis lock
redis-cli GET message-router-primary-lock

# Check if primary refreshed recently
curl http://primary:8080/monitoring/standby-status
```

**Solution**:
- If primary is dead: Restart it or kill it for standby to take over
- If Redis is stuck: Restart Redis
- Check logs on both instances

### Both instances are standby (no primary)

**Symptom**: Both instances report "STANDBY" role

**Diagnosis**: Redis lock expired without being claimed
```bash
redis-cli GET message-router-primary-lock
# Returns (nil) - lock doesn't exist
```

**Solution**: Restart one instance, it should become primary:
```bash
docker restart message-router-primary
```

### Redis connection failures

**Symptom**:
```
SEVERE: Redis unavailable at startup
SEVERE: Failed to acquire lock: Connection refused
```

**Diagnosis**:
```bash
redis-cli -h redis.host ping
# Should return PONG
```

**Solution**:
- Start Redis: `docker run -d redis:latest`
- Fix network connectivity: Check firewall, DNS, etc.
- Check Redis credentials if password set
- Increase connection timeout if Redis is slow

### Instance stuck as PRIMARY but not processing

**Symptom**: Role shows PRIMARY but queues not processing

**Diagnosis**:
```bash
curl http://instance:8080/monitoring/health
# Check if queue consumers are running

curl http://instance:8080/monitoring/queue-stats
# Should show active queues
```

**Solution**: Restart instance or check message router logs for other issues

## Configuration Reference

| Property | Default | Description |
|----------|---------|-------------|
| `standby.enabled` | false | Enable hot standby mode |
| `standby.instance-id` | instance-1 | Unique instance identifier |
| `standby.lock-key` | message-router-primary-lock | Redis key for the lock |
| `standby.lock-ttl-seconds` | 30 | Lock expiration time (seconds) |
| `quarkus.redis.hosts` | redis://localhost:6379 | Redis connection URL (redis:// or rediss:// for TLS) |
| `quarkus.redis.max-pool-size` | 4 | Redis connection pool size |

**Note**: Lock refresh interval is hardcoded to 10 seconds (1/3 of default TTL).

## Performance Impact

### With Standby Disabled
- No Redis overhead
- Single instance operates normally
- No lock acquisition/refresh

### With Standby Enabled
- **Lock acquisition**: 1 Redis operation on startup
- **Lock refresh**: 1-2 Redis operations every 10 seconds
- **Failover detection**: Negligible (reuses refresh task)
- **Memory**: Minimal (lock metadata only)
- **Total impact**: <1% CPU overhead for 100+ queues

## Best Practices

1. **Always Use Persistent Redis**
   - Use Redis persistence (`--appendonly yes`)
   - Consider Redis Sentinel for Redis HA
   - Never use ephemeral Redis in production

2. **Monitor Redis Health**
   - Set up Redis monitoring/alerting
   - Monitor Message Router standby-status endpoint
   - Alert on CRITICAL "Redis Connection Lost" warnings

3. **Graceful Shutdown**
   - Use `SIGTERM` to shut down (not `SIGKILL`)
   - Wait for drainage before restart
   - Standby will take over immediately

4. **Instance IDs**
   - Use meaningful, unique names: `router-primary`, `router-standby`
   - Or use pod names in K8s: `message-router-0`, `message-router-1`
   - Use DNS names, not IPs

5. **Timing Configuration**
   - Lock refresh interval is hardcoded to 10 seconds (1/3 of default 30s TTL)
   - Adjust `lock-ttl-seconds` for slow networks (e.g., 60s) if needed
   - Refresh interval will remain at 10s

## Related Documentation

- [Authentication Guide](AUTHENTICATION.md) - Optional BasicAuth/OIDC
- [Configuration Guide](src/main/resources/application.properties)
- Redis Documentation: https://redis.io/docs/
