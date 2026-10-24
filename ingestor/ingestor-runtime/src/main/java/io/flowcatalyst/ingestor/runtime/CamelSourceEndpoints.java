package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.CamelRoutes;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Source.Track;
import io.flowcatalyst.framework.config.Secret;
import io.flowcatalyst.framework.httpclient.module.ServiceClients;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.regex.Pattern;
import org.apache.camel.CamelContext;
import org.apache.camel.Component;
import org.apache.camel.Endpoint;
import org.apache.camel.component.aws2.s3.AWS2S3Configuration;
import org.apache.camel.component.aws2.s3.AWS2S3Endpoint;
import org.apache.camel.component.aws2.sqs.Sqs2Configuration;
import org.apache.camel.component.aws2.sqs.Sqs2Endpoint;
import org.apache.camel.component.aws2.sqs.client.Sqs2ClientFactory;
import org.apache.camel.component.file.remote.SftpConfiguration;
import org.apache.camel.component.file.remote.SftpEndpoint;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import software.amazon.awssdk.core.exception.SdkException;
import software.amazon.awssdk.services.sqs.SqsClient;

/**
 * The sources this runtime can run. Every endpoint is built new for its route and is not shared,
 * so two ingestions on the same queue or directory keep their own credentials. Logins and secrets
 * are set on the endpoint, never written into its URI.
 */
final class CamelSourceEndpoints implements SourceEndpoints {

  private static final Logger LOG = LoggerFactory.getLogger(CamelSourceEndpoints.class);

  private static final Duration FASTEST_POLL = Duration.ofSeconds(1);

  /**
   * The AWS components refuse to build an endpoint that names no credentials, so every endpoint
   * starts as the runtime's own identity and is given an ingestion's access key afterwards.
   */
  private static final String OWN_IDENTITY = "&useDefaultCredentialsProvider=true";

  private static final Pattern HOST = Pattern.compile("[A-Za-z0-9.-]{1,253}");
  private static final Pattern DIRECTORY = Pattern.compile("/?[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*");
  private static final Pattern REGION = Pattern.compile("[a-z]{2}(-[a-z]+)+-\\d");
  private static final Pattern SQS_QUEUE = Pattern.compile("[A-Za-z0-9_-]{1,80}(\\.fifo)?");
  private static final Pattern BUCKET = Pattern.compile("[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]");
  private static final Pattern S3_PREFIX = Pattern.compile("([A-Za-z0-9!_.*'()-]+/)+");
  private static final Pattern TABLE = Pattern.compile("[A-Za-z_][A-Za-z0-9_]*(\\.[A-Za-z_][A-Za-z0-9_]*)?");
  private static final Pattern COLUMN = Pattern.compile("[A-Za-z_][A-Za-z0-9_]*");

  private final Secrets secrets;
  private final StateStore state;
  private final @Nullable URI awsEndpoint;
  private final @Nullable ServiceClients http;
  private final Extensions extensions;

  /**
   * Creates the endpoints.
   *
   * @param secrets resolves the secrets ingestions name
   * @param state where a database poll keeps its watermark
   * @param awsEndpoint where SQS and S3 are, or null for AWS itself
   */
  CamelSourceEndpoints(Secrets secrets, StateStore state, @Nullable URI awsEndpoint) {
    this(secrets, state, awsEndpoint, null);
  }

  /**
   * Creates the endpoints, with the HTTP engine API polls use.
   *
   * @param secrets resolves the secrets ingestions name
   * @param state where a database poll keeps its watermark and an API poll its cursor
   * @param awsEndpoint where SQS and S3 are, or null for AWS itself
   * @param http the framework's HTTP engine, or null when API polls are not run
   */
  CamelSourceEndpoints(Secrets secrets, StateStore state, @Nullable URI awsEndpoint, @Nullable ServiceClients http) {
    this(secrets, state, awsEndpoint, http, Extensions.none());
  }

  /**
   * Creates the endpoints, with the extensions that make custom sources.
   *
   * @param secrets resolves the secrets ingestions name
   * @param state where polls keep what they have read
   * @param awsEndpoint where SQS and S3 are, or null for AWS itself
   * @param http the framework's HTTP engine, or null when API polls are not run
   * @param extensions the extensions built into the runtime
   */
  CamelSourceEndpoints(
      Secrets secrets, StateStore state, @Nullable URI awsEndpoint, @Nullable ServiceClients http, Extensions extensions) {
    this.secrets = secrets;
    this.state = state;
    this.awsEndpoint = awsEndpoint;
    this.http = http;
    this.extensions = extensions;
  }

  @Override
  public Endpoint endpoint(CamelContext camel, Ingestion ingestion) {
    return switch (ingestion.source()) {
      case Source.Http _ -> camel.getEndpoint(HttpIngest.endpoint(ingestion));
      case Source.Sftp sftp -> sftp(camel, ingestion, sftp);
      case Source.RabbitMq rabbit -> rabbitMq(camel, ingestion, rabbit);
      case Source.Sqs sqs -> sqs(camel, ingestion, sqs);
      case Source.S3 s3 -> s3(camel, ingestion, s3);
      case Source.DatabasePoll poll -> databasePoll(camel, ingestion, poll);
      case Source.ApiPoll api -> apiPoll(camel, ingestion, api);
      case Source.Custom custom when custom.type().equals(CamelRoutes.TYPE) -> camelRoute(camel, ingestion);
      case Source.Custom custom -> extensions.endpoint(camel, ingestion, custom, secrets, state, http);
    };
  }

  /**
   * The route's own from is in its YAML; this is where it hands the payload over. Not guessable, so
   * no other route can send into this ingestion. The YAML is checked here, as the platform stores it
   * without knowing what Camel accepts.
   */
  private Endpoint camelRoute(CamelContext camel, Ingestion ingestion) {
    require(extensions.camelRoutes(), "this runtime does not run camel routes (INGEST_CAMEL_ROUTES_ENABLED is not true)");
    String problem = CamelRoutes.problem(CamelRoutes.yaml(ingestion.source()));
    require(problem == null, "the route " + problem);
    return camel.getEndpoint("direct:ingest-tail-" + java.util.UUID.randomUUID());
  }

  private ServiceClients engine() {
    ServiceClients engine = http;
    if (engine == null) {
      throw new UnsupportedIngestionException("this runtime was built without an HTTP client, so it cannot poll an API");
    }
    return engine;
  }

  private Endpoint apiPoll(CamelContext camel, Ingestion ingestion, Source.ApiPoll api) {
    ServiceClients engine = engine();
    require(api.every().compareTo(FASTEST_POLL) >= 0 && api.idleDelay().compareTo(FASTEST_POLL) >= 0, "an API is polled at most once a second");
    return new ApiPollEndpoint(camel, ingestion, api, new ApiPollClient(ingestion, api, secrets, ApiPollClient.name("api-poll", ingestion.id(), "v" + ingestion.version()), engine, System::nanoTime), state);
  }

  private static void require(boolean valid, String otherwise) {
    if (!valid) {
      throw new UnsupportedIngestionException(otherwise);
    }
  }

  private Secret secret(Ingestion ingestion, SecretRef ref) {
    return secrets.require(ingestion, ref);
  }

  /** A new endpoint of its own, not the one the context would share between equal URIs. */
  private static <E extends Endpoint> E create(CamelContext camel, String scheme, String uri, Class<E> type) {
    Component component = camel.getComponent(scheme);
    if (component == null) {
      throw new UnsupportedIngestionException("the " + scheme + " component is not in this build");
    }
    Endpoint endpoint;
    try {
      endpoint = component.createEndpoint(uri);
    } catch (RuntimeException e) {
      throw e;
    } catch (Exception e) {
      throw new UnsupportedIngestionException("the " + scheme + " source could not be built: " + e.getMessage());
    }
    if (endpoint == null) {
      throw new UnsupportedIngestionException("the " + scheme + " source could not be built");
    }
    return type.cast(endpoint);
  }

  private Endpoint rabbitMq(CamelContext camel, Ingestion ingestion, Source.RabbitMq rabbit) {
    URI uri;
    try {
      uri = URI.create(rabbit.uri());
    } catch (IllegalArgumentException e) {
      throw new UnsupportedIngestionException("the rabbitmq uri is not valid");
    }
    require(
        ("amqp".equals(uri.getScheme()) || "amqps".equals(uri.getScheme())) && uri.getHost() != null,
        "the rabbitmq uri must be amqp://host or amqps://host");
    require(uri.getUserInfo() == null, "the rabbitmq uri must not carry a login; use the username and password fields");
    require(!rabbit.queue().isBlank(), "the rabbitmq queue is not set");
    return new RabbitMqEndpoint(camel, ingestion.id(), rabbit, secret(ingestion, rabbit.password()));
  }

  /**
   * An SQS queue, long-polled. The queue must exist. A message is deleted when its exchange ends
   * without failing, which includes a message that was rejected as unreadable; a failed one is
   * left alone and comes back when its visibility timeout is over.
   */
  private Endpoint sqs(CamelContext camel, Ingestion ingestion, Source.Sqs sqs) {
    require(REGION.matcher(sqs.region()).matches(), "the sqs region is not valid");
    require(SQS_QUEUE.matcher(sqs.queue()).matches(), "the sqs queue name is not valid");
    Sqs2Endpoint endpoint =
        create(camel, "aws2-sqs", "aws2-sqs://" + sqs.queue() + "?delay=100&maxMessagesPerPoll=10" + OWN_IDENTITY, Sqs2Endpoint.class);
    Sqs2Configuration configuration = endpoint.getConfiguration();
    configuration.setRegion(sqs.region());
    configuration.setAutoCreateQueue(false);
    configuration.setDeleteAfterRead(true);
    configuration.setWaitTimeSeconds(10);
    if (sqs.credential() != null) {
      configuration.setUseDefaultCredentialsProvider(false);
      configuration.setAccessKey(sqs.credential().accessKeyId());
      configuration.setSecretKey(secret(ingestion, sqs.credential().secretAccessKey()).reveal());
    }
    if (awsEndpoint != null) {
      configuration.setOverrideEndpoint(true);
      configuration.setUriEndpointOverride(awsEndpoint.toString());
    }
    // Left to itself the component lists the account's queues to find this one, and starts even
    // when it is not there. Asking for the one queue needs less permission and fails now.
    try (SqsClient client = Sqs2ClientFactory.getSqsClient(configuration)) {
      configuration.setQueueUrl(client.getQueueUrl(request -> request.queueName(sqs.queue())).queueUrl());
    } catch (SdkException e) {
      throw new UnsupportedIngestionException("the sqs queue '" + sqs.queue() + "' cannot be used: " + e.getMessage());
    }
    return endpoint;
  }

  /**
   * An S3 bucket polled for the objects directly under a prefix. An object is read as a stream and
   * then moved, within the bucket, under the folder its exchange ended with: {@code .done/} or
   * {@code .rejected/}, in front of its key. A failed exchange leaves the object where it is.
   */
  private Endpoint s3(CamelContext camel, Ingestion ingestion, Source.S3 s3) {
    require(REGION.matcher(s3.region()).matches(), "the s3 region is not valid");
    require(BUCKET.matcher(s3.bucket()).matches(), "the s3 bucket name is not valid");
    require(s3.prefix() == null || S3_PREFIX.matcher(s3.prefix()).matches(), "the s3 prefix must be folder names, each ending in '/'");
    require(s3.prefix() == null || !s3.prefix().startsWith("."), "the s3 prefix must not start with '.'");
    require(s3.every().compareTo(FASTEST_POLL) >= 0, "an s3 bucket is polled at most once a second");
    AWS2S3Endpoint endpoint =
        create(
            camel,
            "aws2-s3",
            "aws2-s3://" + s3.bucket() + "?delay=" + s3.every().toMillis() + "&maxMessagesPerPoll=5" + OWN_IDENTITY,
            AWS2S3Endpoint.class);
    AWS2S3Configuration configuration = endpoint.getConfiguration();
    configuration.setRegion(s3.region());
    configuration.setAutoCreateBucket(false);
    if (s3.prefix() != null) {
      configuration.setPrefix(s3.prefix());
    }
    // Only the objects at this level: what was moved under .done/ is one level down.
    configuration.setDelimiter("/");
    configuration.setIncludeFolders(false);
    configuration.setIncludeBody(false);
    configuration.setAutocloseBody(true);
    configuration.setMoveAfterRead(true);
    configuration.setDeleteAfterRead(true);
    configuration.setDestinationBucket(s3.bucket());
    configuration.setDestinationBucketPrefix("${exchangeProperty." + IngestionRoute.FOLDER + "}/");
    if (s3.credential() != null) {
      configuration.setUseDefaultCredentialsProvider(false);
      configuration.setAccessKey(s3.credential().accessKeyId());
      configuration.setSecretKey(secret(ingestion, s3.credential().secretAccessKey()).reveal());
    }
    if (awsEndpoint != null) {
      configuration.setOverrideEndpoint(true);
      configuration.setUriEndpointOverride(awsEndpoint.toString());
      configuration.setForcePathStyle(true);
    }
    return endpoint;
  }

  private Endpoint databasePoll(CamelContext camel, Ingestion ingestion, Source.DatabasePoll poll) {
    require(poll.jdbcUrl().startsWith("jdbc:"), "the database url must start with 'jdbc:'");
    require(TABLE.matcher(poll.table()).matches(), "the table must be a plain name, or schema.table");
    switch (poll.track()) {
      case Track.Watermark watermark -> require(COLUMN.matcher(watermark.column()).matches(), "the watermark column must be a plain name");
      case Track.Status status -> {
        require(COLUMN.matcher(status.column()).matches(), "the status column must be a plain name");
        require(COLUMN.matcher(status.keyColumn()).matches(), "the key column must be a plain name");
        require(!status.pending().equals(status.processed()), "the pending and processed values must differ");
      }
    }
    require(poll.maxRows() >= 1 && poll.maxRows() <= 10_000, "a poll reads between 1 and 10000 rows");
    require(poll.every().compareTo(FASTEST_POLL) >= 0, "a table is polled at most once a second");
    return new DatabasePollEndpoint(camel, ingestion.id(), poll, secret(ingestion, poll.password()), state);
  }

  /**
   * A directory polled over SFTP. A file is taken once it has stopped growing, read as a stream,
   * and then moved into a folder beside it: {@code .done} when its records became dispatch jobs,
   * {@code .rejected} when it could not be read. A file the platform would not take jobs for stays
   * where it is and is read again on the next poll.
   */
  private Endpoint sftp(CamelContext camel, Ingestion ingestion, Source.Sftp sftp) {
    if (!HOST.matcher(sftp.host()).matches() || sftp.port() < 1 || sftp.port() > 65535) {
      throw new UnsupportedIngestionException("the sftp host or port is not valid");
    }
    if (!DIRECTORY.matcher(sftp.directory()).matches()) {
      throw new UnsupportedIngestionException("the sftp directory may hold only letters, digits, '.', '_', '-' and '/'");
    }
    if (sftp.every().compareTo(FASTEST_POLL) < 0) {
      throw new UnsupportedIngestionException("an sftp directory is polled at most once a second");
    }
    Secret credential = secret(ingestion, sftp.credential());

    // A leading slash is a path from the server's root; Camel writes that as a double slash.
    String directory = sftp.directory().startsWith("/") ? "/" + sftp.directory() : sftp.directory();
    String uri =
        "sftp://" + sftp.host() + ":" + sftp.port() + "/" + directory
            + "?delay=" + sftp.every().toMillis()
            + "&streamDownload=true&stepwise=false&disconnect=true&autoCreate=false"
            + "&readLock=changed&readLockCheckInterval=1000"
            + "&move=${exchangeProperty." + IngestionRoute.FOLDER + "}/${file:name}";
    SftpEndpoint endpoint = create(camel, "sftp", uri, SftpEndpoint.class);
    if (sftp.include() != null) {
      endpoint.setAntInclude(sftp.include());
    }

    SftpConfiguration configuration = endpoint.getConfiguration();
    configuration.setUsername(sftp.username());
    String secret = credential.reveal();
    if (secret.startsWith("-----BEGIN")) {
      configuration.setPrivateKey(secret.getBytes(StandardCharsets.UTF_8));
    } else {
      configuration.setPassword(secret);
    }
    configuration.setUseUserKnownHostsFile(false);
    if (sftp.knownHosts() != null) {
      configuration.setKnownHosts(sftp.knownHosts().getBytes(StandardCharsets.UTF_8));
      configuration.setStrictHostKeyChecking("yes");
    } else {
      LOG.warn("sftp host {} has no known host key configured; its identity is not checked", sftp.host());
      configuration.setStrictHostKeyChecking("no");
    }
    return endpoint;
  }
}
