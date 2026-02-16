####
# This Dockerfile builds a Docker image for FlowCatalyst (TypeScript)
#
# Build from the monorepo root:
#   docker build -f apps/flowcatalyst/Dockerfile -t flowcatalyst .
#
####

# ── Stage 1: Install dependencies & build ──────────────────────────
FROM node:24-alpine AS build

# argon2 needs build tools for native compilation
RUN apk add --no-cache python3 make g++

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

# ── Stage 2: Production image ──────────────────────────────────────
FROM node:24-alpine

# curl for health checks, RDS CA bundle for AWS Postgres
RUN apk add --no-cache curl \
    && mkdir -p /certs \
    && curl -sf -o /certs/global-bundle.pem https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem

# Create non-root user
RUN adduser -u 1001 -D appuser

WORKDIR /app

# Copy built output
COPY --from=build /app/apps/flowcatalyst/dist ./dist

# Copy production node_modules (argon2 native addon, optional AWS SDKs)
COPY --from=build /app/node_modules ./node_modules
COPY --from=build /app/apps/flowcatalyst/node_modules ./apps/flowcatalyst/node_modules

# Set ownership
RUN chown -R 1001:1001 /app

USER 1001

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=30s \
  CMD curl -sf http://localhost:8080/health/ready || exit 1

CMD ["node", "dist/index.js"]
