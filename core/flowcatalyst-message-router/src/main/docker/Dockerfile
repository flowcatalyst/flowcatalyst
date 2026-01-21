####
# This Dockerfile builds a Docker image using Alpine base image with Java 25 JRE
# Alpine already includes wget for health checks (no additional installation needed)
#
# Build with:
#   docker build -f src/main/docker/Dockerfile -t flowcatalyst/message-router:latest .
#
# Virtual Thread Configuration:
#   The entrypoint script automatically configures the virtual thread scheduler
#   with at least 2 carrier threads, even on single-core containers. This prevents
#   thread starvation where a single carrier thread could be blocked while other
#   virtual threads are waiting to run.
#
####

FROM eclipse-temurin:25-jre-alpine

# Set working directory
WORKDIR /deployments

# Copy the entrypoint script first (as root)
COPY src/main/docker/entrypoint.sh /deployments/entrypoint.sh

# Copy the built application from build/quarkus-app
COPY build/quarkus-app/lib/ /deployments/lib/
COPY build/quarkus-app/*.jar /deployments/
COPY build/quarkus-app/app/ /deployments/app/
COPY build/quarkus-app/quarkus/ /deployments/quarkus/

# Create non-root user (185 is standard Quarkus user)
RUN adduser -u 185 -D appuser

# Make entrypoint executable and set ownership
RUN chmod +x /deployments/entrypoint.sh && chown -R 185:185 /deployments

# Switch to non-root user
USER 185

# Expose port
EXPOSE 8080

# Health check using wget (already available in Alpine)
HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=30s \
  CMD wget -q -O- http://localhost:8080/health/ready || exit 1

# Start via entrypoint script (configures virtual thread parallelism)
ENTRYPOINT [ "/deployments/entrypoint.sh" ]
