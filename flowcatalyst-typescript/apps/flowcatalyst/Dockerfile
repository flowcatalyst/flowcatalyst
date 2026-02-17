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

# Copy workspace config & lockfile first (layer caching)
COPY pnpm-workspace.yaml pnpm-lock.yaml package.json ./

# Copy package.json for each workspace member so pnpm can resolve
COPY apps/flowcatalyst/package.json apps/flowcatalyst/
COPY apps/platform-frontend/package.json apps/platform-frontend/

# Install all dependencies (including devDependencies for build)
RUN pnpm install --frozen-lockfile

# Copy source
COPY apps/flowcatalyst/ apps/flowcatalyst/
COPY apps/platform-frontend/ apps/platform-frontend/

# Build frontend + backend
RUN pnpm --filter @flowcatalyst/flowcatalyst build:full

# Create pruned production-only deploy directory (no devDeps, no .pnpm store)
RUN pnpm --filter @flowcatalyst/flowcatalyst deploy --prod /app/deploy

# ── Stage 2: Production image ──────────────────────────────────────
FROM node:24-alpine

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
