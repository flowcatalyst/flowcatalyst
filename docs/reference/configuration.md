# Configuration Reference

Complete reference for FlowCatalyst configuration options.

## Configuration Sources

Configuration can be provided via:

1. **Environment variables** - Recommended for production
2. **application.properties** - Module-specific defaults
3. **System properties** - Runtime overrides (`-Dkey=value`)

Environment variables use `_` instead of `.` and are uppercase:
- `flowcatalyst.auth.mode` → `FLOWCATALYST_AUTH_MODE`

## Platform Configuration

### Application Key

```properties
# Required for secret encryption
# Generate with: openssl rand -base64 32
flowcatalyst.app-key=${FLOWCATALYST_APP_KEY:}
```

| Variable | Description | Default |
|----------|-------------|---------|
| `FLOWCATALYST_APP_KEY` | AES-256 key for local secret encryption | Required |

### Authentication Mode

```properties
# Mode: "embedded" (full IdP) or "remote" (token validation only)
flowcatalyst.auth.mode=${FLOWCATALYST_AUTH_MODE:embedded}
flowcatalyst.auth.external-base-url=${FLOWCATALYST_EXTERNAL_BASE_URL:}
```

| Variable | Description | Default |
|----------|-------------|---------|
| `FLOWCATALYST_AUTH_MODE` | `embedded` (full IdP) or `remote` (validation only) | `embedded` |
| `FLOWCATALYST_EXTERNAL_BASE_URL` | Public URL for OAuth callbacks | None |

### JWT Configuration

```properties
flowcatalyst.auth.jwt.issuer=${FLOWCATALYST_JWT_ISSUER:flowcatalyst}
flowcatalyst.auth.jwt.private-key-path=${FLOWCATALYST_JWT_PRIVATE_KEY_PATH:}
flowcatalyst.auth.jwt.public-key-path=${FLOWCATALYST_JWT_PUBLIC_KEY_PATH:}
flowcatalyst.auth.jwt.access-token-expiry=${FLOWCATALYST_ACCESS_TOKEN_EXPIRY:PT1H}
flowcatalyst.auth.jwt.session-token-expiry=${FLOWCATALYST_SESSION_TOKEN_EXPIRY:PT8H}
flowcatalyst.auth.jwt.refresh-token-expiry=${FLOWCATALYST_REFRESH_TOKEN_EXPIRY:P30D}
```

| Variable | Description | Default |
|----------|-------------|---------|
| `FLOWCATALYST_JWT_ISSUER` | Token issuer claim | `flowcatalyst` |
| `FLOWCATALYST_JWT_PRIVATE_KEY_PATH` | Path to RSA private key | Auto-generated |
| `FLOWCATALYST_JWT_PUBLIC_KEY_PATH` | Path to RSA public key | Auto-generated |
| `FLOWCATALYST_ACCESS_TOKEN_EXPIRY` | Access token lifetime (ISO-8601) | `PT1H` (1 hour) |
| `FLOWCATALYST_SESSION_TOKEN_EXPIRY` | Session token lifetime | `PT8H` (8 hours) |
| `FLOWCATALYST_REFRESH_TOKEN_EXPIRY` | Refresh token lifetime | `P30D` (30 days) |

### Session Cookies

```properties
flowcatalyst.auth.session.secure=${FLOWCATALYST_SESSION_SECURE:true}
flowcatalyst.auth.session.same-site=${FLOWCATALYST_SESSION_SAME_SITE:Strict}
flowcatalyst.auth.session.cookie-name=${FLOWCATALYST_SESSION_COOKIE:FLOWCATALYST_SESSION}
```

| Variable | Description | Default |
|----------|-------------|---------|
| `FLOWCATALYST_SESSION_SECURE` | Require HTTPS for cookies | `true` |
| `FLOWCATALYST_SESSION_SAME_SITE` | SameSite cookie policy | `Strict` |
| `FLOWCATALYST_SESSION_COOKIE` | Session cookie name | `FLOWCATALYST_SESSION` |

### Remote Auth Mode

When `flowcatalyst.auth.mode=remote`:

```properties
flowcatalyst.auth.remote.issuer=${FLOWCATALYST_AUTH_REMOTE_ISSUER:}
flowcatalyst.auth.remote.jwks-url=${FLOWCATALYST_AUTH_REMOTE_JWKS_URL:}
flowcatalyst.auth.remote.login-url=${FLOWCATALYST_AUTH_REMOTE_LOGIN_URL:}
flowcatalyst.auth.remote.logout-url=${FLOWCATALYST_AUTH_REMOTE_LOGOUT_URL:}
flowcatalyst.auth.remote.jwks-cache-duration=${FLOWCATALYST_AUTH_REMOTE_JWKS_CACHE:PT1H}
```

### IDP Federation

```properties
flowcatalyst.idp.anchor.enabled=${IDP_ANCHOR_ENABLED:false}
flowcatalyst.idp.anchor.type=${IDP_ANCHOR_TYPE:KEYCLOAK}
flowcatalyst.idp.platform.enabled=${IDP_PLATFORM_ENABLED:false}
flowcatalyst.idp.platform.type=${IDP_PLATFORM_TYPE:KEYCLOAK}
flowcatalyst.idp.sync-on-startup=${FLOWCATALYST_IDP_SYNC_ON_STARTUP:false}
```

## MongoDB Configuration

```properties
quarkus.mongodb.connection-string=${MONGODB_URL:mongodb://localhost:27017}
quarkus.mongodb.database=${MONGODB_DATABASE:flowcatalyst}
```

| Variable | Description | Default |
|----------|-------------|---------|
| `MONGODB_URL` | MongoDB connection string | `mongodb://localhost:27017` |
| `MONGODB_DATABASE` | Database name | `flowcatalyst` |

### Replica Set (Required for Change Streams)

```properties
# Dev profile with single-node replica set
%dev.quarkus.mongodb.connection-string=mongodb://localhost:27017/?replicaSet=rs0&directConnection=true
```

## Message Router Configuration

### Core Settings

```properties
message-router.enabled=true
message-router.queue-type=SQS
message-router.sync-interval=5m
message-router.max-pools=2000
message-router.pool-warning-threshold=1000
```

| Variable | Description | Default |
|----------|-------------|---------|
| `MESSAGE_ROUTER_ENABLED` | Enable message processing | `true` |
| `MESSAGE_ROUTER_QUEUE_TYPE` | Queue backend: `SQS`, `ACTIVEMQ`, `EMBEDDED` | `SQS` |
| `MESSAGE_ROUTER_SYNC_INTERVAL` | Config sync interval | `5m` |
| `MESSAGE_ROUTER_MAX_POOLS` | Maximum processing pools | `2000` |

### SQS Configuration

```properties
quarkus.sqs.endpoint-override=${SQS_ENDPOINT_OVERRIDE:}
quarkus.sqs.aws.region=${AWS_REGION:eu-west-1}
quarkus.sqs.aws.credentials.type=default
message-router.sqs.max-messages-per-poll=10
message-router.sqs.wait-time-seconds=20
```

| Variable | Description | Default |
|----------|-------------|---------|
| `SQS_ENDPOINT_OVERRIDE` | Custom SQS endpoint (LocalStack) | None |
| `AWS_REGION` | AWS region | `eu-west-1` |
| `MESSAGE_ROUTER_SQS_MAX_MESSAGES_PER_POLL` | Messages per poll | `10` |
| `MESSAGE_ROUTER_SQS_WAIT_TIME_SECONDS` | Long poll timeout | `20` |

### ActiveMQ Configuration

```properties
activemq.broker.url=${ACTIVEMQ_BROKER_URL:tcp://localhost:61616}
activemq.username=${ACTIVEMQ_USERNAME:admin}
activemq.password=${ACTIVEMQ_PASSWORD:admin}
message-router.activemq.receive-timeout-ms=1000
```

| Variable | Description | Default |
|----------|-------------|---------|
| `ACTIVEMQ_BROKER_URL` | ActiveMQ broker URL | `tcp://localhost:61616` |
| `ACTIVEMQ_USERNAME` | Broker username | `admin` |
| `ACTIVEMQ_PASSWORD` | Broker password | `admin` |

### Embedded Queue Configuration

```properties
message-router.embedded.visibility-timeout-seconds=30
message-router.embedded.receive-timeout-ms=1000
quarkus.datasource.embedded-queue.jdbc.url=jdbc:sqlite:./flowcatalyst-queue.db
```

### HTTP Mediator

```properties
mediator.http.version=HTTP_2
%dev.mediator.http.version=HTTP_1_1
```

### Config Service Client

```properties
quarkus.rest-client.message-router-config.url=${MESSAGE_ROUTER_CONFIG_URL:http://localhost:8080/api/config}
quarkus.rest-client.message-router-config.connect-timeout=3000
quarkus.rest-client.message-router-config.read-timeout=5000
```

| Variable | Description | Default |
|----------|-------------|---------|
| `MESSAGE_ROUTER_CONFIG_URL` | Config service URL | `http://localhost:8080/api/config` |

## Queue Health Monitoring

```properties
queue.health.monitor.enabled=true
queue.health.backlog.threshold=1000
queue.health.growth.threshold=100
```

| Variable | Description | Default |
|----------|-------------|---------|
| `QUEUE_HEALTH_MONITOR_ENABLED` | Enable health monitoring | `true` |
| `QUEUE_HEALTH_BACKLOG_THRESHOLD` | Warning threshold for queue depth | `1000` |
| `QUEUE_HEALTH_GROWTH_THRESHOLD` | Warning threshold for growth rate | `100` |

## Subscription Cache Configuration

The subscription cache accelerates event-to-subscription matching by caching active subscriptions.

```properties
flowcatalyst.subscription-cache.ttl-minutes=5
flowcatalyst.subscription-cache.max-size=10000
```

| Variable | Description | Default |
|----------|-------------|---------|
| `FLOWCATALYST_SUBSCRIPTION_CACHE_TTL_MINUTES` | Cache entry TTL in minutes | `5` |
| `FLOWCATALYST_SUBSCRIPTION_CACHE_MAX_SIZE` | Maximum cache entries | `10000` |

**Cache Behavior**:
- Keyed by `{eventTypeCode}:{clientId|anchor}`
- Automatically invalidated when subscriptions are created, updated, or deleted
- On cache miss, loads from database and caches the result

## Dispatch Scheduler Configuration

The dispatch scheduler manages polling for pending jobs and safety net recovery.

```properties
dispatch-scheduler.enabled=true
dispatch-scheduler.poll-interval=5s
dispatch-scheduler.batch-size=20
dispatch-scheduler.max-concurrent-groups=10
dispatch-scheduler.stale-queued-threshold-minutes=15
dispatch-scheduler.stale-queued-poll-interval=60s
dispatch-scheduler.queue-type=EMBEDDED
dispatch-scheduler.queue-url=
dispatch-scheduler.processing-endpoint=http://localhost:8080/api/dispatch/process
dispatch-scheduler.default-dispatch-pool-code=DISPATCH-POOL
```

| Variable | Description | Default |
|----------|-------------|---------|
| `DISPATCH_SCHEDULER_ENABLED` | Enable the scheduler | `true` |
| `DISPATCH_SCHEDULER_POLL_INTERVAL` | Interval for polling PENDING jobs | `5s` |
| `DISPATCH_SCHEDULER_BATCH_SIZE` | Jobs to fetch per poll | `20` |
| `DISPATCH_SCHEDULER_MAX_CONCURRENT_GROUPS` | Max concurrent message groups | `10` |
| `DISPATCH_SCHEDULER_STALE_QUEUED_THRESHOLD_MINUTES` | Age before QUEUED jobs are considered stale | `15` |
| `DISPATCH_SCHEDULER_STALE_QUEUED_POLL_INTERVAL` | Interval for stale job polling | `60s` |
| `DISPATCH_SCHEDULER_QUEUE_TYPE` | Queue type: `SQS`, `ACTIVEMQ`, `EMBEDDED` | `EMBEDDED` |
| `DISPATCH_SCHEDULER_QUEUE_URL` | SQS queue URL or queue name | None |
| `DISPATCH_SCHEDULER_PROCESSING_ENDPOINT` | Webhook processing endpoint | `http://localhost:8080/api/dispatch/process` |

**Safety Net Pollers**:
- **PendingJobPoller**: Runs every `poll-interval`, picks up PENDING jobs and queues them
- **StaleQueuedJobPoller**: Runs every `stale-queued-poll-interval`, resets QUEUED jobs older than `stale-queued-threshold-minutes` to PENDING

## Hot Standby Configuration

```properties
standby.enabled=false
standby.instance-id=${HOSTNAME:instance-1}
standby.lock-key=message-router-primary-lock
standby.lock-ttl-seconds=30
```

| Variable | Description | Default |
|----------|-------------|---------|
| `STANDBY_ENABLED` | Enable primary/standby mode | `false` |
| `HOSTNAME` | Instance identifier | `instance-1` |
| `STANDBY_LOCK_TTL_SECONDS` | Lock timeout | `30` |

### Redis (Required for Standby Mode)

```properties
quarkus.redisson.single-server-config.address=redis://${REDIS_HOST:localhost}:${REDIS_PORT:6379}
quarkus.redisson.single-server-config.database=0
```

| Variable | Description | Default |
|----------|-------------|---------|
| `REDIS_HOST` | Redis hostname | `localhost` |
| `REDIS_PORT` | Redis port | `6379` |
| `REDIS_USERNAME` | Redis username (prod) | None |
| `REDIS_PASSWORD` | Redis password (prod) | None |

## Notification Configuration

```properties
notification.enabled=true
notification.batch.interval=5m
notification.min.severity=WARNING
```

### Email Notifications

```properties
notification.email.enabled=false
notification.email.from=flowcatalyst@example.com
notification.email.to=ops@example.com

quarkus.mailer.host=smtp.example.com
quarkus.mailer.port=587
quarkus.mailer.start-tls=REQUIRED
quarkus.mailer.username=
quarkus.mailer.password=
```

### Teams Notifications

```properties
notification.teams.enabled=false
notification.teams.webhook.url=
```

| Variable | Description | Default |
|----------|-------------|---------|
| `NOTIFICATION_TEAMS_WEBHOOK_URL` | Microsoft Teams webhook URL | None |

## Secret Management

```properties
flowcatalyst.secrets.default-provider=${FLOWCATALYST_SECRETS_PROVIDER:encrypted}
```

| Provider | Description |
|----------|-------------|
| `encrypted` | Local AES-256-GCM encryption |
| `aws-sm` | AWS Secrets Manager |
| `aws-ps` | AWS Parameter Store |
| `gcp-sm` | Google Cloud Secret Manager |
| `vault` | HashiCorp Vault |

### Encrypted Provider

```properties
flowcatalyst.secrets.encryption.key=${FLOWCATALYST_SECRETS_ENCRYPTION_KEY:}
flowcatalyst.secrets.encryption.passphrase=${FLOWCATALYST_SECRETS_PASSPHRASE:}
flowcatalyst.secrets.encryption.salt=${FLOWCATALYST_SECRETS_SALT:flowcatalyst-default-salt}
```

### AWS Secrets

```properties
flowcatalyst.secrets.aws.prefix=${FLOWCATALYST_SECRETS_AWS_PREFIX:}
flowcatalyst.secrets.aws.kms-key-id=${FLOWCATALYST_SECRETS_AWS_KMS_KEY:}
```

### GCP Secrets

```properties
flowcatalyst.secrets.gcp.enabled=${FLOWCATALYST_SECRETS_GCP_ENABLED:false}
flowcatalyst.secrets.gcp.project-id=${FLOWCATALYST_SECRETS_GCP_PROJECT:}
```

## Authentication (Message Router Standalone)

```properties
authentication.enabled=false
authentication.mode=NONE
```

| Mode | Description |
|------|-------------|
| `NONE` | No authentication required |
| `BASIC` | HTTP Basic authentication |
| `OIDC` | OpenID Connect with redirect |

### Basic Auth

```properties
authentication.basic-username=${AUTH_BASIC_USERNAME:}
authentication.basic-password=${AUTH_BASIC_PASSWORD:}
```

### OIDC

```properties
quarkus.oidc.auth-server-url=
quarkus.oidc.client-id=
quarkus.oidc.credentials.secret=
quarkus.oidc.application-type=web-app
```

## Logging Configuration

### Development

```properties
%dev.quarkus.log.console.json.enabled=false
%dev.quarkus.log.console.format=%d{HH:mm:ss} %-5p [%c{2.}] (%t) %s%e%n
%dev.quarkus.log.level=INFO
%dev.quarkus.log.category."tech.flowcatalyst".level=DEBUG
```

### Production

```properties
%prod.quarkus.log.console.format=json
%prod.quarkus.log.console.json.pretty-print=false
%prod.quarkus.log.level=INFO
```

## Profiles

| Profile | Description |
|---------|-------------|
| `dev` | Local development |
| `prod` | Production deployment |
| `native` | GraalVM native image build |

Activate profiles:

```bash
# Via Gradle
./gradlew quarkusDev -Dquarkus.profile=dev

# Via environment
QUARKUS_PROFILE=prod java -jar app.jar

# Embedded queue (no external broker)
./gradlew quarkusDev -Dmessage-router.queue-type=EMBEDDED
```

## See Also

- [Development Setup](../development/setup.md) - Environment setup
- [Queue Configuration](../guides/queue-configuration.md) - Queue backends
- [Deployment Guide](../operations/deployment.md) - Production deployment
