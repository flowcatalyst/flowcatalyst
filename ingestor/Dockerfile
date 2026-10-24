# The ingestor runtime image. Build the jar first (the release workflow does):
#
#   ./mvnw -B -DskipTests package
#   docker build -t flowcatalyst/ingestor:local .
#
# Extensions: mount or copy extension jars into /opt/ingestor/extensions (INGEST_EXTENSIONS_DIR).
FROM eclipse-temurin:25-jre

RUN useradd --system --create-home --home-dir /app ingestor
WORKDIR /app
COPY ingestor-runtime/target/ingestor-runtime-*-all.jar /app/ingestor-runtime.jar
COPY ingestor-runtime/config/application.yaml /app/config/application.yaml
RUN mkdir -p /opt/ingestor/extensions && chown -R ingestor /app /opt/ingestor

USER ingestor
ENV INGEST_EXTENSIONS_DIR=/opt/ingestor/extensions
EXPOSE 8091
ENTRYPOINT ["java", "-XX:MaxRAMPercentage=75", "-jar", "/app/ingestor-runtime.jar"]
