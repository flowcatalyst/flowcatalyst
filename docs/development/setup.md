# Development Setup

Guide for setting up a FlowCatalyst development environment.

## Prerequisites

### Required

- **Java 21** - JDK with virtual thread support
- **Gradle 8+** - Build tool (wrapper included)
- **MongoDB** - Primary database
- **Docker** - For local services

### Optional

- **IntelliJ IDEA** or **VS Code** - Recommended IDEs
- **LocalStack** - Local AWS services
- **ActiveMQ** - Alternative queue backend

## Quick Start

### 1. Clone Repository

```bash
git clone https://github.com/your-org/flowcatalyst.git
cd flowcatalyst
```

### 2. Start Local Services

```bash
# Using Docker Compose
docker-compose -f docker-compose.dev.yml up -d
```

This starts:
- MongoDB on port 27017
- LocalStack (SQS) on port 4566
- Redis on port 6379

### 3. Run Development Mode

```bash
# Message Router (most common)
./gradlew :core:flowcatalyst-message-router:quarkusDev

# Full-stack application
./gradlew :core:flowcatalyst-app:quarkusDev

# With embedded queue (no external broker)
./gradlew :core:flowcatalyst-message-router:quarkusDev -Dmessage-router.queue-type=EMBEDDED
```

### 4. Access Application

- **API**: http://localhost:8080
- **Health**: http://localhost:8080/health
- **Metrics**: http://localhost:8080/q/metrics
- **Dashboard**: http://localhost:8080/dashboard.html

## Development Scenarios

### Scenario 1: Message Router with SQS

```bash
# Start LocalStack
docker run -d --name localstack -p 4566:4566 localstack/localstack

# Run message router
./gradlew :core:flowcatalyst-message-router:quarkusDev
```

### Scenario 2: Embedded Queue (No Broker)

```bash
# No external services needed
./gradlew :core:flowcatalyst-message-router:quarkusDev \
  -Dmessage-router.queue-type=EMBEDDED
```

SQLite database stored in `./flowcatalyst-queue.db`

### Scenario 3: Full-Stack Development

```bash
# Start MongoDB and queue
docker-compose -f docker-compose.dev.yml up -d

# Run full application
./gradlew :core:flowcatalyst-app:quarkusDev
```

### Scenario 4: Platform API Only

```bash
# Start MongoDB
docker run -d --name mongo -p 27017:27017 mongo:7

# Run platform
./gradlew :core:flowcatalyst-platform:quarkusDev
```

## IDE Setup

### IntelliJ IDEA

1. **Import Project**: File → Open → Select `flowcatalyst` directory
2. **Configure JDK**: Project Structure → Project → SDK → Java 21
3. **Enable Annotation Processing**: Settings → Build → Compiler → Annotation Processors
4. **Install Quarkus Plugin**: Plugins → Marketplace → "Quarkus"

### VS Code

1. **Open Folder**: File → Open Folder → Select `flowcatalyst`
2. **Install Extensions**:
   - Extension Pack for Java
   - Quarkus
   - Gradle for Java

3. **Configure Java**:
   ```json
   // .vscode/settings.json
   {
     "java.configuration.runtimes": [
       {
         "name": "JavaSE-21",
         "path": "/path/to/java-21"
       }
     ]
   }
   ```

## Hot Reload

Quarkus dev mode provides hot reload:

```bash
./gradlew :core:flowcatalyst-message-router:quarkusDev
```

- **Java changes**: Automatic reload
- **Resource changes**: Automatic reload
- **Configuration changes**: Automatic reload (most)

Press `s` in terminal for forced restart.

## Database Setup

### MongoDB

```bash
# Docker
docker run -d --name mongo -p 27017:27017 mongo:7

# Verify connection
mongosh mongodb://localhost:27017
```

### Initial Data

Collections created automatically. For test data:

```javascript
// mongosh
use flowcatalyst

// Create test client
db.auth_clients.insertOne({
  _id: "0HZXEQ5Y8JY5Z",
  name: "Test Client",
  identifier: "test-client",
  status: "ACTIVE",
  createdAt: new Date(),
  updatedAt: new Date()
})
```

## Environment Variables

### Development Defaults

Most defaults work for local development:

```bash
# Optional overrides
export QUARKUS_MONGODB_CONNECTION_STRING=mongodb://localhost:27017
export QUARKUS_MONGODB_DATABASE=flowcatalyst
export MESSAGE_ROUTER_QUEUE_TYPE=SQS
export SQS_ENDPOINT_OVERRIDE=http://localhost:4566
```

### Configuration Files

- `application.properties` - Main configuration
- `application-dev.properties` - Dev profile

## Debugging

### Remote Debugging

```bash
# Start with debug port
./gradlew :core:flowcatalyst-message-router:quarkusDev \
  -Dsuspend=y -Ddebug=5005
```

Connect IDE debugger to port 5005.

### Logging

```bash
# Enable debug logging
QUARKUS_LOG_CATEGORY__TECH_FLOWCATALYST__LEVEL=DEBUG \
  ./gradlew :core:flowcatalyst-message-router:quarkusDev
```

### Dev UI

Access Quarkus Dev UI at http://localhost:8080/q/dev/

## Common Issues

### Port Already in Use

```bash
# Find process
lsof -i :8080

# Kill process
kill -9 <pid>
```

### Gradle Cache Issues

```bash
# Clean build
./gradlew clean

# With refresh dependencies
./gradlew build --refresh-dependencies
```

### MongoDB Connection Failed

```bash
# Check MongoDB is running
docker ps | grep mongo

# Restart if needed
docker restart mongo
```

## See Also

- [Build Reference](build-reference.md) - Build commands
- [Testing Guide](testing.md) - Running tests
- [Coding Standards](coding-standards.md) - Code conventions
