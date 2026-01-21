# Build stage
FROM node:22-alpine AS builder

# Install pnpm
RUN corepack enable && corepack prepare pnpm@9.15.4 --activate

WORKDIR /app

# Copy package files
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY packages/shared-types/package.json ./packages/shared-types/
COPY packages/logging/package.json ./packages/logging/
COPY packages/config/package.json ./packages/config/
COPY packages/queue-core/package.json ./packages/queue-core/
COPY apps/message-router/package.json ./apps/message-router/

# Install dependencies
RUN pnpm install --frozen-lockfile

# Copy source
COPY tsconfig.base.json ./
COPY packages/ ./packages/
COPY apps/message-router/ ./apps/message-router/

# Build all packages
RUN pnpm -r build

# Production stage
FROM node:22-alpine AS runner

# Install pnpm for production
RUN corepack enable && corepack prepare pnpm@9.15.4 --activate

WORKDIR /app

# Copy built artifacts
COPY --from=builder /app/package.json ./
COPY --from=builder /app/pnpm-lock.yaml ./
COPY --from=builder /app/pnpm-workspace.yaml ./
COPY --from=builder /app/packages/shared-types/package.json ./packages/shared-types/
COPY --from=builder /app/packages/shared-types/dist ./packages/shared-types/dist
COPY --from=builder /app/packages/logging/package.json ./packages/logging/
COPY --from=builder /app/packages/logging/dist ./packages/logging/dist
COPY --from=builder /app/packages/config/package.json ./packages/config/
COPY --from=builder /app/packages/config/dist ./packages/config/dist
COPY --from=builder /app/packages/queue-core/package.json ./packages/queue-core/
COPY --from=builder /app/packages/queue-core/dist ./packages/queue-core/dist
COPY --from=builder /app/apps/message-router/package.json ./apps/message-router/
COPY --from=builder /app/apps/message-router/dist ./apps/message-router/dist

# Install production dependencies only
RUN pnpm install --frozen-lockfile --prod

# Set environment
ENV NODE_ENV=production
ENV PORT=8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q --spider http://localhost:8080/health/ready || exit 1

# Expose port
EXPOSE 8080

# Run
WORKDIR /app/apps/message-router
CMD ["node", "dist/index.js"]
