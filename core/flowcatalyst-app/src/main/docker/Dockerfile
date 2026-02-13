####
# This Dockerfile builds a Docker image using Alpine base image with Java 25 JRE
#
# Build with:
#   cd core/flowcatalyst-app
#   docker build -f src/main/docker/Dockerfile -t flowcatalyst/platform:latest .
#
####

FROM eclipse-temurin:25-jre-noble

# Install curl (needed for health checks) and download RDS CA bundle
RUN apt-get update && apt-get install -y --no-install-recommends curl \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /certs \
    && curl -sf -o /certs/global-bundle.pem https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem

# Create non-root user (185 is standard Quarkus user)
RUN adduser --uid 185 --disabled-password --gecos "" appuser

# Set working directory
WORKDIR /deployments

# Copy the entrypoint script first
COPY src/main/docker/entrypoint.sh /deployments/entrypoint.sh

# Copy the built application from build/quarkus-app
COPY build/quarkus-app/lib/ /deployments/lib/
COPY build/quarkus-app/*.jar /deployments/
COPY build/quarkus-app/app/ /deployments/app/
COPY build/quarkus-app/quarkus/ /deployments/quarkus/

# Make entrypoint executable and set ownership
RUN chmod +x /deployments/entrypoint.sh && chown -R 185:185 /deployments

# Switch to non-root user
USER 185

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=30s \
  CMD curl -sf http://localhost:8080/health/ready || exit 1

# Start via entrypoint script (configures virtual thread parallelism)
ENTRYPOINT [ "/deployments/entrypoint.sh" ]
