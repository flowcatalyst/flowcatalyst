# The ingestor runtime image. Build the jar first (the release workflow does):
#
#   ./mvnw -B -DskipTests package
#   docker build -t flowcatalyst/ingestor:local .
#
# Extensions: a deployed runtime sets INGEST_EXTENSIONS_MANIFEST=s3://<bucket>/<key> and loads the jars the
# manifest pins (docs/extensions.md); otherwise mount extension jars into /opt/ingestor/extensions
# (INGEST_EXTENSIONS_DIR), or build an image of your own FROM this one that copies them there as root. The
# manifest and a non-empty directory are exclusive. The runtime's user can read the jar,
# its config and the extensions, and write none of them.
FROM eclipse-temurin:25-jre

RUN useradd --system --no-create-home --home-dir /nonexistent --shell /usr/sbin/nologin ingestor
WORKDIR /app
COPY ingestor-runtime/target/ingestor-runtime-*-all.jar /app/ingestor-runtime.jar
COPY ingestor-runtime/config/application.yaml /app/config/application.yaml
RUN mkdir -p /opt/ingestor/extensions \
 && chown -R root:root /app /opt/ingestor \
 && chmod -R a+rX,go-w /app /opt/ingestor

USER ingestor
ENV INGEST_EXTENSIONS_DIR=/opt/ingestor/extensions
# Not local: a platform on plain http is refused, and so is a source on a loopback address. fcdev runs
# the jar itself with APP_ENV=local.
ENV APP_ENV=production
EXPOSE 8091
# Out of memory, the process ends and the orchestrator starts a healthy one, instead of a thread dying.
ENTRYPOINT ["java", "-XX:MaxRAMPercentage=75", "-XX:+ExitOnOutOfMemoryError", "-jar", "/app/ingestor-runtime.jar"]
