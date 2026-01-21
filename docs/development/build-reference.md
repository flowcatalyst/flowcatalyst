# Build Quick Reference

Common build commands for FlowCatalyst development.

## Development Mode

```bash
# Message Router (most common)
./gradlew :core:flowcatalyst-message-router:quarkusDev

# Full-stack application
./gradlew :core:flowcatalyst-app:quarkusDev

# Platform only
./gradlew :core:flowcatalyst-platform:quarkusDev

# Embedded queue (no external broker)
./gradlew :core:flowcatalyst-message-router:quarkusDev -Dmessage-router.queue-type=EMBEDDED
```

## Building

### JVM Build

```bash
# Build specific module
./gradlew :core:flowcatalyst-message-router:build
./gradlew :core:flowcatalyst-app:build

# Build all modules
./gradlew build

# Build without tests
./gradlew build -x test
```

### Native Build

```bash
# Production native (SQS/ActiveMQ)
./gradlew :core:flowcatalyst-message-router:build -Dquarkus.package.type=native

# Developer native (Embedded SQLite queue)
./gradlew :core:flowcatalyst-message-router:build \
  -Dquarkus.package.type=native \
  -Dmessage-router.queue-type=EMBEDDED
```

### Docker Build

```bash
# Build Docker image with Jib
./gradlew :core:flowcatalyst-app:jibDockerBuild

# Build with Quarkus container-image
./gradlew :core:flowcatalyst-app:build \
  -Dquarkus.container-image.build=true
```

## Testing

```bash
# Run all tests
./gradlew test

# Run specific module tests
./gradlew :core:flowcatalyst-message-router:test

# Run single test class
./gradlew :core:flowcatalyst-message-router:test --tests "QueueManagerTest"

# Run with coverage
./gradlew test jacocoTestReport
```

## Running Built JARs

### JVM Mode

```bash
# Message Router
java -jar core/flowcatalyst-message-router/build/quarkus-app/quarkus-run.jar

# Full-stack
java -jar core/flowcatalyst-app/build/quarkus-app/quarkus-run.jar
```

### With Environment Variables

```bash
MESSAGE_ROUTER_QUEUE_TYPE=SQS \
AWS_REGION=us-east-1 \
java -jar core/flowcatalyst-message-router/build/quarkus-app/quarkus-run.jar
```

### Native Binary

```bash
./core/flowcatalyst-message-router/build/flowcatalyst-message-router-*-runner
```

## Cleaning

```bash
# Clean specific module
./gradlew :core:flowcatalyst-message-router:clean

# Clean all
./gradlew clean

# Clean and rebuild
./gradlew clean build
```

## Dependencies

```bash
# Show dependencies
./gradlew :core:flowcatalyst-message-router:dependencies

# Refresh dependencies
./gradlew build --refresh-dependencies

# Check for updates
./gradlew dependencyUpdates
```

## Gradle Options

| Option | Description |
|--------|-------------|
| `-x test` | Skip tests |
| `--parallel` | Parallel build |
| `--info` | Verbose output |
| `--stacktrace` | Show stack traces |
| `--no-daemon` | Don't use daemon |
| `--refresh-dependencies` | Force dependency refresh |

## Module Paths

| Module | Path |
|--------|------|
| Platform | `:core:flowcatalyst-platform` |
| Message Router | `:core:flowcatalyst-message-router` |
| Event Processor | `:core:flowcatalyst-event-processor` |
| Dispatch Scheduler | `:core:flowcatalyst-dispatch-scheduler` |
| Queue Client | `:core:flowcatalyst-queue-client` |
| Standby | `:core:flowcatalyst-standby` |
| Full-Stack App | `:core:flowcatalyst-app` |
| Router App | `:core:flowcatalyst-router-app` |
| SDK | `:core:flowcatalyst-sdk` |

## Output Locations

| Type | Location |
|------|----------|
| JAR | `build/quarkus-app/quarkus-run.jar` |
| Native | `build/*-runner` |
| Docker | Local Docker images |
| Reports | `build/reports/` |

## Profiles

```bash
# Default (dev)
./gradlew quarkusDev

# Embedded queue dev (no external broker)
./gradlew quarkusDev -Dmessage-router.queue-type=EMBEDDED

# Custom profile
./gradlew quarkusDev -Dquarkus.profile=custom
```

## Docker Services

```bash
# Start all dev services
docker-compose -f docker-compose.dev.yml up -d

# Stop services
docker-compose -f docker-compose.dev.yml down

# View logs
docker-compose -f docker-compose.dev.yml logs -f
```

## Troubleshooting

```bash
# Gradle daemon issues
./gradlew --stop
./gradlew build --no-daemon

# Out of memory
GRADLE_OPTS="-Xmx4g" ./gradlew build

# Lock file issues
rm -rf .gradle/
./gradlew build
```

## See Also

- [Development Setup](setup.md) - Environment setup
- [Testing Guide](testing.md) - Running tests
- [Queue Configuration](../guides/queue-configuration.md) - Queue setup
