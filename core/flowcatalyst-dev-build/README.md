# FlowCatalyst Developer Build

A single executable combining all FlowCatalyst services for local development. No need to run multiple services separately.

## What's Included

- **Platform** - Auth, events, dispatch jobs, subscriptions API
- **Message Router** - High-volume message routing with embedded SQLite queue
- **Dispatch Scheduler** - Polls pending jobs and queues for delivery
- **Event Processor** - MongoDB change streams to projection collection
- **Outbox Processor** - Polls your app's outbox table (optional)

## Prerequisites

- **Java 21+** (for JVM mode) or just run the native executable
- **Docker** (optional - for MongoDB, or bring your own)
- **MongoDB 5.0+** configured as a replica set (required for change streams)

## Quick Start

### 1. Start MongoDB

**Option A: Using Docker (recommended)**
```bash
cd core/flowcatalyst-dev-build
docker compose up -d
```

**Option B: External MongoDB**

If you have your own MongoDB replica set, create a `.env` file:
```bash
EXTERNAL_MONGODB=true
MONGODB_URI=mongodb://your-server:27017/?replicaSet=yourReplSet
```

### 2. Start FlowCatalyst

**Unix/macOS:**
```bash
./scripts/start.sh
```

**Windows:**
```cmd
scripts\start.bat
```

### 3. Access the API

- **API:** http://localhost:8080
- **Health:** http://localhost:8080/q/health

### 4. Login

On first startup, the platform seeds development data including a Platform Admin user:

| Email | Password | Role |
|-------|----------|------|
| `admin@flowcatalyst.local` | `DevPassword123!` | Platform Super Admin |
| `alice@acme.com` | `DevPassword123!` | Client Admin (Acme) |
| `bob@acme.com` | `DevPassword123!` | Regular User (Acme) |

See [Seeded Data](#seeded-data) for the full list of what's created.

## Configuration

Copy `env.example` to `.env` and customize:

```bash
cp env.example .env
```

### Key Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | 8080 | API server port |
| `EXTERNAL_MONGODB` | false | Use external MongoDB instead of Docker |
| `MONGODB_URI` | localhost replica set | MongoDB connection string |
| `MONGODB_DATABASE` | flowcatalyst | Database name |
| `QUEUE_DB_PATH` | ./data/flowcatalyst-queue.db | SQLite queue location |
| `OUTBOX_ENABLED` | false | Enable outbox processor |

## MongoDB Setup

FlowCatalyst requires MongoDB configured as a **replica set** for change streams support.

### Docker (Default)

The included `docker-compose.yml` starts a single-node replica set:

```bash
docker compose up -d
```

### External MongoDB

For your own MongoDB:

1. Ensure it's configured as a replica set
2. Set environment variables:

```bash
EXTERNAL_MONGODB=true
MONGODB_URI=mongodb://host1:27017,host2:27017/?replicaSet=myReplSet
MONGODB_DATABASE=flowcatalyst
```

### MongoDB Atlas

Works out of the box - Atlas clusters are always replica sets:

```bash
EXTERNAL_MONGODB=true
MONGODB_URI=mongodb+srv://user:pass@cluster.mongodb.net/?retryWrites=true&w=majority
```

## Outbox Processor

The outbox processor connects to **your application's database** where the FlowCatalyst SDK has created outbox tables.

### Flow

1. Your app uses the FlowCatalyst SDK (Laravel, Node.js, etc.)
2. SDK migrations create outbox tables in your app's database
3. Your app writes events/dispatch jobs within transactions
4. The outbox processor polls your database and sends to the platform

### Enable Outbox Processing

```bash
OUTBOX_ENABLED=true
OUTBOX_DB_TYPE=POSTGRESQL
OUTBOX_DB_URL=jdbc:postgresql://localhost:5432/your_app_database
OUTBOX_DB_USER=your_db_user
OUTBOX_DB_PASS=your_db_password
```

Supported databases: `POSTGRESQL`, `MYSQL`, `MONGODB`

## Scripts

| Script | Description |
|--------|-------------|
| `start.sh` / `start.bat` | Start MongoDB (if not external) and FlowCatalyst |
| `stop.sh` / `stop.bat` | Stop all services |
| `reset.sh` / `reset.bat` | Delete all data and start fresh |

## Building Native Executable

Build a native executable for faster startup:

```bash
# From repository root
./gradlew :core:flowcatalyst-dev-build:build -Dquarkus.native.enabled=true -Dquarkus.package.jar.enabled=false
```

The native executable will be at:
- **Unix/macOS:** `build/flowcatalyst-dev-build-1.0.0-SNAPSHOT-runner`
- **Windows:** `build/flowcatalyst-dev-build-1.0.0-SNAPSHOT-runner.exe`

The start scripts automatically detect and use the native executable if present.

## Running Unsigned Executables

Native executables are not code-signed. Users may need to bypass OS security warnings.

### macOS

If you see *"cannot be opened because the developer cannot be verified"*:

**Option 1: Right-click method**
1. Right-click (or Control-click) the executable
2. Select "Open"
3. Click "Open" in the dialog

**Option 2: Terminal**
```bash
xattr -d com.apple.quarantine ./flowcatalyst-dev-build-1.0.0-SNAPSHOT-runner
```

**Option 3: System Settings**
1. Go to System Settings → Privacy & Security
2. Find the blocked app and click "Open Anyway"

### Windows

If you see *"Windows protected your PC"* (SmartScreen):

1. Click "More info"
2. Click "Run anyway"

### Linux

No additional steps required.

## Troubleshooting

### MongoDB connection failed

Ensure MongoDB is running as a replica set:

```bash
# Check Docker container
docker logs fc-dev-mongo

# Verify replica set status
docker exec fc-dev-mongo mongosh --eval "rs.status()"
```

### Port already in use

Change the port in `.env`:
```bash
HTTP_PORT=8081
```

### Outbox processor not picking up messages

1. Verify `OUTBOX_ENABLED=true`
2. Check database connection settings
3. Ensure SDK migrations have been run in your app
4. Check logs for connection errors

### SQLite database locked

Stop any other processes using the queue database, or change the path:
```bash
QUEUE_DB_PATH=./data/my-queue.db
```

## Seeded Data

On first startup, the platform automatically seeds development data. This is idempotent - it won't duplicate data on subsequent restarts.

### Users

| Email | Name | Role | Scope |
|-------|------|------|-------|
| `admin@flowcatalyst.local` | Platform Administrator | `platform:super-admin` | ANCHOR (all clients) |
| `alice@acme.com` | Alice Johnson | `acme:client-admin`, `dispatch:admin` | CLIENT (Acme) |
| `bob@acme.com` | Bob Smith | `dispatch:user` | CLIENT (Acme) |
| `charlie@globex.com` | Charlie Brown | `dispatch:admin` | CLIENT (Globex) |
| `diana@partner.io` | Diana Partner | `dispatch:viewer` | PARTNER (Acme, Globex) |

**Password for all users:** `DevPassword123!`

### Clients

| Name | Identifier | Status |
|------|------------|--------|
| Acme Corporation | acme | ACTIVE |
| Globex Industries | globex | ACTIVE |
| Initech Solutions | initech | ACTIVE |
| Umbrella Corp | umbrella | SUSPENDED |

### Applications

- **tms** - Transport Management System
- **wms** - Warehouse Management System
- **oms** - Order Management System
- **track** - Shipment Tracking
- **yard** - Yard Management System
- **platform** - Platform Services

### Event Types

55+ event types are seeded across all applications, including:
- TMS: load, shipment, billing events
- WMS: inventory, outbound, labor events
- OMS: order, fulfillment, returns events
- Track: visibility, alerts events
- Yard: gate, dock, trailer events
- Platform: webhook, audit, control-plane events

### Roles & Permissions

Platform roles are defined in code and registered at startup:
- `platform:super-admin` - Full access to everything
- `platform:platform-admin` - Client and application management
- `platform:iam-admin` - User and role management
- `platform:messaging-admin` - Event types and subscriptions

## Development

### JVM Dev Mode (Hot Reload)

```bash
./gradlew :core:flowcatalyst-dev-build:quarkusDev
```

### View Logs

Logs are written to stdout. In dev mode, they're formatted for readability.

### Mongo Express UI

Start with the tools profile:
```bash
docker compose --profile tools up -d
```

Access at http://localhost:8081 (admin/admin)
