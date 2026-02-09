# FlowCatalyst

Unified app that runs Platform, Stream Processor, and Message Router in a single process.

## Build Options

### Development (no frontend embedding)

```bash
pnpm --filter @flowcatalyst/flowcatalyst build
```

Compiles TypeScript and copies Drizzle migrations to `dist/`. The platform serves API-only; run the frontend dev server separately with `pnpm --filter @flowcatalyst/platform-frontend dev`.

### Production with embedded frontend

```bash
pnpm --filter @flowcatalyst/flowcatalyst build:full
```

Builds the Vue frontend, compiles TypeScript, then copies the frontend `dist/` into `dist/frontend/`. The platform serves both API and frontend on the same port.

### Single Executable Application (SEA)

```bash
pnpm --filter @flowcatalyst/flowcatalyst build:sea
```

Builds the frontend, compiles TypeScript, packs migrations and frontend into JSON assets, then produces a self-contained Node.js SEA binary. Migrations and frontend are embedded inside the binary and extracted to temp dirs at runtime.

## Running

### Development

```bash
pnpm --filter @flowcatalyst/flowcatalyst dev
```

Runs with `tsx watch` for hot-reload. Uses `.env` in the app directory.

### Production

```bash
# After build or build:full
node apps/flowcatalyst/dist/index.js serve

# After build:sea
./flowcatalyst serve
```

### CLI Commands

| Command   | Description                          |
| --------- | ------------------------------------ |
| `serve`   | Start all enabled services (default) |
| `migrate` | Run database migrations and exit     |
| `version` | Print version and exit               |
| `help`    | Show help message                    |

### Debug / Verbose Logging

Set `LOG_LEVEL` to get more detail:

```bash
# Trace-level logging (most verbose)
LOG_LEVEL=trace node dist/index.js serve

# Debug-level logging
LOG_LEVEL=debug node dist/index.js serve

# Pretty-print logs in development
NODE_ENV=development LOG_LEVEL=debug node dist/index.js serve
```

With `tsx` in development, pretty-printed logs are enabled automatically.

## Environment Variables

### Service Toggles

| Variable                   | Default       | Description                                |
| -------------------------- | ------------- | ------------------------------------------ |
| `PLATFORM_ENABLED`         | `true`        | Enable Platform (IAM/OIDC/Admin API)       |
| `STREAM_PROCESSOR_ENABLED` | `true`        | Enable Stream Processor (CQRS read models) |
| `MESSAGE_ROUTER_ENABLED`   | `false`       | Enable Message Router (queue processing)   |
| `AUTO_MIGRATE`             | `true` in dev | Run DB migrations on startup               |

### Server

| Variable        | Default       | Description                                                   |
| --------------- | ------------- | ------------------------------------------------------------- |
| `DATABASE_URL`  | _(required)_  | PostgreSQL connection string                                  |
| `PLATFORM_PORT` | `3000`        | Platform HTTP port                                            |
| `ROUTER_PORT`   | `8080`        | Message Router port                                           |
| `HOST`          | `0.0.0.0`     | Bind address                                                  |
| `NODE_ENV`      | `development` | Environment mode                                              |
| `LOG_LEVEL`     | `info`        | Log level: `trace`, `debug`, `info`, `warn`, `error`, `fatal` |

### Frontend

| Variable       | Default           | Description                                                                                                                             |
| -------------- | ----------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| `FRONTEND_DIR` | _(auto-detected)_ | Path to built frontend assets. When omitted, checked in order: SEA embedded asset, `dist/frontend/`, sibling `platform-frontend/dist/`. |

See `.env.example` for the full list including OIDC, encryption, and bootstrap variables.
