####
# This Dockerfile builds a Docker image for FlowCatalyst (TypeScript)
#
# Build from the monorepo root:
#   docker build --platform linux/amd64 -f apps/flowcatalyst/Dockerfile -t flowcatalyst .
#
####

# ── Stage 1: Install dependencies & build ──────────────────────────
FROM node:24-alpine AS build

# argon2 needs build tools for native compilation
RUN apk add --no-cache python3 make g++

ENV NPM_CONFIG_UPDATE_NOTIFIER=false
RUN corepack enable && corepack prepare pnpm@9.15.4 --activate

WORKDIR /app

# Copy workspace config, lockfile & shared tsconfig first (layer caching)
COPY pnpm-workspace.yaml pnpm-lock.yaml package.json tsconfig.base.json ./

# Copy package.json for each workspace member so pnpm can resolve
COPY apps/flowcatalyst/package.json apps/flowcatalyst/
COPY apps/platform-frontend/package.json apps/platform-frontend/
COPY packages/config/package.json packages/config/
COPY packages/contracts/package.json packages/contracts/
COPY packages/crypto/package.json packages/crypto/
COPY packages/domain/package.json packages/domain/
COPY packages/framework/package.json packages/framework/
COPY packages/http/package.json packages/http/
COPY packages/logging/package.json packages/logging/
COPY packages/message-router/package.json packages/message-router/
COPY packages/outbox-processor/package.json packages/outbox-processor/
COPY packages/persistence/package.json packages/persistence/
COPY packages/platform/package.json packages/platform/
COPY packages/queue-core/package.json packages/queue-core/
COPY packages/stream-processor/package.json packages/stream-processor/
COPY packages/tsid/package.json packages/tsid/

# Install all dependencies (including devDependencies for build)
RUN pnpm install --frozen-lockfile

# Copy source
COPY apps/flowcatalyst/ apps/flowcatalyst/
COPY apps/platform-frontend/ apps/platform-frontend/
COPY packages/ packages/

# Build frontend + backend
RUN pnpm --filter @flowcatalyst/flowcatalyst build:full

# Create pruned production-only deploy directory (no devDeps, no .pnpm store)
RUN pnpm --filter @flowcatalyst/flowcatalyst deploy --prod /app/deploy

# ── Stage 2: Production image ──────────────────────────────────────
FROM node:24-alpine
LABEL org.opencontainers.image.source=https://github.com/flowcatalyst/flowcatalyst

# curl for health checks, RDS CA bundle for AWS Postgres
RUN apk add --no-cache curl \
    && mkdir -p /certs \
    && curl -sf -o /certs/global-bundle.pem https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem

# Create non-root user
RUN adduser -u 1001 -D appuser

WORKDIR /app

# Copy files with correct ownership (--chown avoids a duplicate layer from chown -R)
COPY --chown=1001:1001 apps/flowcatalyst/entrypoint.sh /app/entrypoint.sh
COPY --chown=1001:1001 --from=build /app/apps/flowcatalyst/dist ./dist
COPY --chown=1001:1001 --from=build /app/deploy/node_modules ./node_modules

RUN chmod +x /app/entrypoint.sh

USER 1001

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=30s \
  CMD curl -sf http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]
