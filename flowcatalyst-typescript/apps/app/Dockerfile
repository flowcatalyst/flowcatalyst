# ─── Stage 1: Build ─────────────────────────────────────────────────────────
FROM node:22-slim AS builder

WORKDIR /build

# Install pnpm
RUN corepack enable && corepack prepare pnpm@latest --activate

# Copy workspace structure
COPY pnpm-workspace.yaml package.json pnpm-lock.yaml ./
COPY packages/ packages/
COPY apps/platform/ apps/platform/
COPY apps/stream-processor/ apps/stream-processor/
COPY apps/app/ apps/app/

# Install dependencies
RUN pnpm install --frozen-lockfile

# Build all packages and apps
RUN pnpm -r build

# ─── Stage 2: Production ────────────────────────────────────────────────────
FROM node:22-slim AS production

WORKDIR /app

# Install pnpm for workspace protocol resolution
RUN corepack enable && corepack prepare pnpm@latest --activate

# Copy workspace manifests
COPY --from=builder /build/pnpm-workspace.yaml /build/package.json /build/pnpm-lock.yaml ./

# Copy built packages (dist only)
COPY --from=builder /build/packages/ packages/
COPY --from=builder /build/apps/platform/dist/ apps/platform/dist/
COPY --from=builder /build/apps/platform/package.json apps/platform/
COPY --from=builder /build/apps/stream-processor/dist/ apps/stream-processor/dist/
COPY --from=builder /build/apps/stream-processor/package.json apps/stream-processor/
COPY --from=builder /build/apps/app/dist/ apps/app/dist/
COPY --from=builder /build/apps/app/package.json apps/app/

# Install production dependencies only
RUN pnpm install --frozen-lockfile --prod

# Runtime config
ENV NODE_ENV=production
ENV PORT=3000
ENV HOST=0.0.0.0

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -f http://localhost:3000/health || exit 1

CMD ["node", "apps/app/dist/index.js"]
