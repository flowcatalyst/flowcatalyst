####
# This Dockerfile builds a Docker image using Alpine base image with Java 21 JRE
# Alpine already includes wget for health checks (no additional installation needed)
#
# Build with:
#   docker build -f src/main/docker/Dockerfile -t flowcatalyst/router:latest-vertx .
#
####

FROM eclipse-temurin:21-jre-alpine

# Set working directory
WORKDIR /deployments

# Copy the built application from build/quarkus-app
COPY build/quarkus-app/lib/ /deployments/lib/
COPY build/quarkus-app/*.jar /deployments/
COPY build/quarkus-app/app/ /deployments/app/
COPY build/quarkus-app/quarkus/ /deployments/quarkus/

# Create non-root user (185 is standard Quarkus user)
RUN adduser -u 185 -D appuser

# Set ownership to appuser
RUN chown -R 185:185 /deployments

# Switch to non-root user
USER 185

# Expose port
EXPOSE 8080

# Health check using wget (already available in Alpine)
HEALTHCHECK --interval=10s --timeout=5s --retries=3 --start-period=30s \
  CMD wget -q -O- http://localhost:8080/health/ready || exit 1

# Start the application
ENTRYPOINT [ "java", "-cp", "/deployments/app:/deployments/lib/*", "-Dquarkus.http.host=0.0.0.0", "-jar", "/deployments/quarkus-run.jar" ]
