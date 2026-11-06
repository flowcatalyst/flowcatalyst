package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.framework.auth.PlatformTokens;
import io.flowcatalyst.framework.config.Config;
import io.flowcatalyst.framework.config.Setting;
import io.flowcatalyst.framework.core.App;
import io.flowcatalyst.framework.core.AppBuilder;
import io.flowcatalyst.framework.core.Background;
import io.flowcatalyst.framework.core.LoopOptions;
import io.flowcatalyst.framework.core.Module;
import io.flowcatalyst.framework.core.MonotonicClock;
import io.flowcatalyst.framework.flowcatalyst.FlowCatalystModule;
import io.flowcatalyst.framework.flowcatalyst.PlatformClient;
import io.flowcatalyst.framework.http.Routes;
import io.flowcatalyst.framework.http.ratelimit.RateLimiter;
import io.flowcatalyst.framework.httpclient.Auth;
import io.flowcatalyst.framework.httpclient.HttpClients;
import io.flowcatalyst.framework.httpclient.ServiceClient;
import io.flowcatalyst.framework.httpclient.ServiceOptions;
import io.flowcatalyst.framework.httpclient.module.ServiceClients;
import java.net.InetAddress;
import java.net.URI;
import java.net.UnknownHostException;
import io.flowcatalyst.ingestor.functions.FunctionHost;
import io.flowcatalyst.ingestor.functions.context.DbPools;
import io.flowcatalyst.ingestor.functions.context.LogBudget;
import io.flowcatalyst.ingestor.functions.control.HttpControlPlane;
import java.nio.file.Path;
import java.time.Clock;
import java.time.Duration;
import java.time.InstantSource;
import java.util.Optional;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;
import java.util.regex.Pattern;
import org.apache.camel.CamelContext;
import org.apache.camel.ProducerTemplate;
import org.apache.camel.impl.DefaultCamelContext;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * The Camel context, the routes built from ingestions, and the loop that keeps them current. List
 * {@code HttpClientModule} with it, and {@code FlowCatalystModule} before it: every call to the
 * platform's runtime API is made as that module's service account, which holds the platform's
 * {@code platform:ingestion-runtime} role.
 */
public final class RuntimeModule implements Module {

  /** The module id. */
  public static final String ID = "ingest-runtime";

  /** Names this runtime in its polls and reports. */
  public static final Setting<String> RUNTIME_ID =
      Setting.string("ingest.runtime-id")
          .withDefault(defaultRuntimeId())
          .describedAs("Names this runtime to the platform; two running runtimes must never share one. [A-Za-z0-9._:-], at most 100 characters.");

  /** Where the platform tells to reach this runtime. */
  public static final Setting<URI> ADVERTISED_URL =
      Setting.uri("ingest.advertised-url")
          .describedAs("Where this runtime can be reached; shown by the platform for information, never called.");

  /** How often the runtime polls and reports. */
  public static final Setting<Duration> POLL_INTERVAL =
      Setting.duration("ingest.poll-interval")
          .withDefault(Duration.ofMinutes(1))
          .describedAs("How often the runtime reads the live ingestions and sends its report.");

  /** Where SQS and S3 are, when not AWS itself: a local stand-in, or an S3-compatible store. */
  public static final Setting<URI> AWS_ENDPOINT =
      Setting.uri("ingest.aws.endpoint").describedAs("Overrides the AWS endpoint for SQS and S3 sources; leave unset for AWS.");

  /** A directory of extension jars. */
  public static final Setting<String> EXTENSIONS_DIR =
      Setting.string("ingest.extensions-dir")
          .describedAs("A directory of extension jars, loaded at start in addition to the extensions on the class path; unset for none.");

  /** The pinned manifest of extension jars in S3. */
  public static final Setting<String> EXTENSIONS_MANIFEST =
      Setting.string("ingest.extensions-manifest")
          .describedAs("s3://<bucket>/<key> of a manifest pinning the extension jars to load from the same bucket; exclusive with jars in the extensions directory. Unset for none.");

  /** Whether camel-route ingestions are run. */
  public static final Setting<Boolean> CAMEL_ROUTES_ENABLED =
      Setting.bool("ingest.camel-routes-enabled")
          .withDefault(false)
          .describedAs("Whether the runtime reports the camel-route kind and runs its ingestions. A route runs with this runtime's network, identity and environment.");

  /** Whether this process hosts JVM functions. */
  public static final Setting<Boolean> FUNCTIONS_ENABLED =
      Setting.bool("ingest.functions-enabled")
          .withDefault(true)
          .describedAs(
              "Whether this process hosts JVM functions: it fetches the platform's desired state for its pool, loads the jvm versions in it and serves /fn/. The runtime's service account must also hold platform:function-runner.");

  /** The function pool this process serves. */
  public static final Setting<String> FUNCTIONS_POOL =
      Setting.string("ingest.functions-pool")
          .withDefault("default")
          .describedAs("The function pool this process serves (the pool of its desired-state request); the pool of the Go function runner by default.");

  /** The most database pools the function host keeps open. */
  public static final Setting<Integer> FUNCTIONS_DB_MAX_POOLS =
      Setting.integer("ingest.functions-db-max-pools")
          .withDefault(DbPools.Limits.DEFAULTS.maxPools())
          .describedAs(
              "The most database connection pools the function host keeps open: one per distinct DSN, shared by every function bound to it. A new DSN past the cap closes the pool used longest ago.");

  /** The connections in each of those pools. */
  public static final Setting<Integer> FUNCTIONS_DB_MAX_CONNS =
      Setting.integer("ingest.functions-db-max-conns")
          .withDefault(DbPools.Limits.DEFAULTS.maxConnsPerPool())
          .describedAs("The most connections each function database pool opens.");

  /** How long an unused pool is kept. */
  public static final Setting<Duration> FUNCTIONS_DB_IDLE_TTL =
      Setting.duration("ingest.functions-db-idle-ttl")
          .withDefault(DbPools.Limits.DEFAULTS.idleTtl())
          .describedAs("How long a function database pool nobody has used is kept before it is closed; the next call opens it again.");

  /** The most lines a second one function may write to the log. */
  public static final Setting<Integer> FUNCTIONS_LOG_RATE =
      Setting.integer("ingest.functions-log-rate")
          .withDefault(LogBudget.Limits.DEFAULTS.rate())
          .describedAs(
              "The most log lines a second one function may write; lines over the budget are dropped and counted, and the host reports the count once a minute.");

  /** The most lines one function may write at once. */
  public static final Setting<Integer> FUNCTIONS_LOG_BURST =
      Setting.integer("ingest.functions-log-burst")
          .withDefault(LogBudget.Limits.DEFAULTS.burst())
          .describedAs("The most log lines one function may write in a burst, before the rate applies.");

  /** Whether this process runs ingestions. */
  public static final Setting<Boolean> INGESTION_ENABLED =
      Setting.bool("ingest.ingestion-enabled")
          .withDefault(true)
          .describedAs(
              "Whether this process runs ingestions. Off, it never polls the platform for ingestions, so it takes no ingestion leases, runs no routes and serves no /ingest/ endpoint: a functions-only process.");

  /** How long a revealed secret is kept. */
  public static final Setting<Duration> SECRETS_CACHE_TTL =
      Setting.duration("ingest.secrets-cache-ttl")
          .withDefault(Duration.ofMinutes(1))
          .describedAs(
              "How long a secret revealed by the platform is kept, and how often each running route's secrets are read again (a route whose secret changed is rebuilt); each reveal is audited on the platform.");

  /** How many trace records wait to be shipped. */
  public static final Setting<Integer> TRACE_QUEUE =
      Setting.integer("ingest.trace-queue")
          .withDefault(TraceShipper.DEFAULT_QUEUE)
          .describedAs(
              "How many trace records (and at most 16 MiB of them) wait to be shipped to the platform while an operator has tracing on for an ingestion; when full the oldest are dropped. Tracing is off unless the platform switches it on.");

  /**
   * The system property that makes Camel's own threads (its consumers, timers and scheduled polls)
   * virtual. Every source is I/O bound. Camel reads it once, when its thread code first loads, which
   * is after this module registers.
   */
  static final String CAMEL_VIRTUAL_THREADS = "camel.threads.virtual.enabled";

  /** The name of the runtime's client for the platform's function control plane. */
  static final String FUNCTIONS_CLIENT = "platform-functions";

  /** The background loop that polls the platform; its health check is {@code background:ingest-poll}. */
  public static final String LOOP = "ingest-poll";

  private static final Pattern RUNTIME_NAME = Pattern.compile("[A-Za-z0-9._:-]{1,100}");

  /** The longest stopping every route may take when the runtime stops; fcdev and most orchestrators wait 15 s or more. */
  static final Duration STOP_WITHIN = Duration.ofSeconds(12);

  /** The longest the last trace records are given to be shipped when the runtime stops. */
  static final Duration TRACE_FLUSH = Duration.ofSeconds(2);

  private static final Logger LOG = LoggerFactory.getLogger(RuntimeModule.class);

  private @Nullable ProducerTemplate producer;
  private @Nullable ScheduledExecutorService watchdog;
  private @Nullable FunctionHost functions;

  /** {@code runtime-<hostname>}, with anything the platform would refuse in a runtime id made {@code -}. */
  private static String defaultRuntimeId() {
    String host;
    try {
      host = InetAddress.getLocalHost().getHostName();
    } catch (UnknownHostException e) {
      host = "unknown";
    }
    String id = "runtime-" + host.replaceAll("[^A-Za-z0-9._:-]", "-");
    return id.length() > 100 ? id.substring(0, 100) : id;
  }

  @Override
  public String id() {
    return ID;
  }

  @Override
  public void register(AppBuilder app) {
    // Unless the JVM was started with it set either way.
    if (System.getProperty(CAMEL_VIRTUAL_THREADS) == null) {
      System.setProperty(CAMEL_VIRTUAL_THREADS, "true");
    }
    app.declare(RUNTIME_ID);
    app.declare(POLL_INTERVAL);
    app.declare(AWS_ENDPOINT);
    app.declare(ADVERTISED_URL);
    app.declare(EXTENSIONS_DIR);
    app.declare(EXTENSIONS_MANIFEST);
    app.declare(CAMEL_ROUTES_ENABLED);
    app.declare(FUNCTIONS_ENABLED);
    app.declare(FUNCTIONS_POOL);
    app.declare(FUNCTIONS_DB_MAX_POOLS);
    app.declare(FUNCTIONS_DB_MAX_CONNS);
    app.declare(FUNCTIONS_DB_IDLE_TTL);
    app.declare(FUNCTIONS_LOG_RATE);
    app.declare(FUNCTIONS_LOG_BURST);
    app.declare(INGESTION_ENABLED);
    app.declare(SECRETS_CACHE_TTL);
    app.declare(TRACE_QUEUE);

    app.provide(
        Extensions.class,
        lookup -> {
          Config config = lookup.get(Config.class);
          Path directory = config.find(EXTENSIONS_DIR).map(Path::of).orElse(null);
          Optional<String> manifest = config.find(EXTENSIONS_MANIFEST);
          if (manifest.isEmpty()) {
            return Extensions.load(directory, config.get(CAMEL_ROUTES_ENABLED));
          }
          return Extensions.load(directory, manifest.get(), new S3ExtensionObjects(config.find(AWS_ENDPOINT).orElse(null)), config.get(CAMEL_ROUTES_ENABLED));
        });
    app.provide(
        PlatformApi.class,
        lookup -> {
          Config config = lookup.get(Config.class);
          String runtimeId = config.get(RUNTIME_ID);
          if (!RUNTIME_NAME.matcher(runtimeId).matches()) {
            throw new IllegalStateException("ingest.runtime-id must match " + RUNTIME_NAME.pattern() + ", was '" + runtimeId + "'");
          }
          PlatformClient platform = lookup.get(PlatformClient.class);
          // As the runtime's service account: its token carries platform:ingestion:runtime:operate.
          ServiceClient client =
              lookup
                  .get(ServiceClients.class)
                  .register(
                      ServiceOptions.builder(Capture.PLATFORM_CLIENT, config.get(FlowCatalystModule.BASE_URL))
                          .requestTimeout(config.get(FlowCatalystModule.TIMEOUT))
                          .auth(Auth.bearer(platform::accessToken, platform::clearToken))
                          .build());
          return new PlatformApi(
              client,
              runtimeId,
              config.find(ADVERTISED_URL).map(URI::toString).orElse(null),
              lookup.get(Extensions.class).kinds(),
              platform::clearToken);
        });
    app.provide(OutboundGuard.class, lookup -> OutboundGuard.forEnvironment(lookup.get(Config.class).environment()));
    app.provide(RuntimeApi.class, lookup -> lookup.get(PlatformApi.class));
    app.provide(StateStore.class, lookup -> lookup.get(PlatformApi.class));
    // One breaker for every ingestion's jobs (docs/ingestor-failure-pacing.md, R2).
    app.provide(JobSink.class, lookup -> new HoldingJobSink(lookup.get(PlatformApi.class)));
    app.provide(
        Secrets.class,
        lookup ->
            new Secrets(lookup.get(PlatformApi.class), System::getenv, lookup.get(Config.class).get(SECRETS_CACHE_TTL), System::nanoTime));
    app.provide(RateLimiter.class, lookup -> HttpIngest.limiter(lookup.get(MonotonicClock.class)));
    app.provide(
        SourceEndpoints.class,
        lookup ->
            new CamelSourceEndpoints(
                lookup.get(Secrets.class),
                lookup.get(StateStore.class),
                lookup.get(Config.class).find(AWS_ENDPOINT).orElse(null),
                lookup.get(ServiceClients.class),
                lookup.get(Extensions.class),
                lookup.get(OutboundGuard.class)));
    app.provide(
        TraceShipper.class,
        lookup -> {
          TraceShipper shipper = new TraceShipper(lookup.get(PlatformApi.class), lookup.get(Config.class).get(TRACE_QUEUE));
          shipper.start();
          return shipper;
        });
    app.provide(
        TraceWindows.class,
        lookup -> {
          Secrets secrets = lookup.get(Secrets.class);
          return new TraceWindows(lookup.get(InstantSource.class), lookup.get(TraceShipper.class), secrets::valuesOf, secrets::conceal);
        });
    app.provide(ExchangeCounters.class, lookup -> new ExchangeCounters(lookup.get(InstantSource.class)));
    app.provide(SourceHealth.class, lookup -> new SourceHealth());
    app.provide(
        CamelContext.class,
        lookup -> {
          DefaultCamelContext camel = new DefaultCamelContext();
          // A route stopped is given this long to finish its exchange in flight, then cut off; a
          // stop of the whole runtime stops them all at once (RuntimeModule.shutdown).
          camel.getShutdownStrategy().setTimeout(Reconciler.ROUTE_STOP.toSeconds());
          return camel;
        });
    app.provide(
        Reconciler.class,
        lookup -> {
          SourceEndpoints endpoints = lookup.get(SourceEndpoints.class);
          JobSink sink = lookup.get(JobSink.class);
          ExchangeCounters counters = lookup.get(ExchangeCounters.class);
          SourceHealth health = lookup.get(SourceHealth.class);
          Secrets secrets = lookup.get(Secrets.class);
          TraceWindows traces = lookup.get(TraceWindows.class);
          return new Reconciler(
              lookup.get(CamelContext.class),
              ingestion -> new IngestionRoute(ingestion, endpoints, sink, counters, health, traces),
              health,
              secrets::forget);
        });
    app.provide(
        TestRunner.class,
        lookup ->
            new TestRunner(
                lookup.get(Secrets.class),
                lookup.get(ServiceClients.class),
                lookup.get(Extensions.class),
                lookup.get(CamelContext.class),
                System::nanoTime,
                lookup.get(OutboundGuard.class)));
    app.provide(
        RuntimeLoop.class,
        lookup -> {
          Config config = lookup.get(Config.class);
          return new RuntimeLoop(
              config.get(RUNTIME_ID),
              config.get(POLL_INTERVAL),
              lookup.get(RuntimeApi.class),
              lookup.get(Reconciler.class),
              lookup.get(ExchangeCounters.class),
              lookup.get(TestRunner.class),
              lookup.get(InstantSource.class),
              lookup.get(MonotonicClock.class),
              config.get(FlowCatalystModule.TIMEOUT),
              lookup.get(Secrets.class),
              config.get(SECRETS_CACHE_TTL),
              lookup.get(TraceWindows.class));
        });
  }

  /**
   * The verifier of platform tokens posted to HTTP ingestions. A platform on plain {@code http} (a
   * local {@code fcdev}) is accepted only while {@code APP_ENV} is {@code local} or {@code test};
   * anywhere else an {@code http} base URL fails start-up.
   */
  private static PlatformTokens platformTokens(App app) {
    Config config = app.config();
    return PlatformTokens.forEnvironment(
        config.environment(), config.get(FlowCatalystModule.BASE_URL), config.get(FlowCatalystModule.TIMEOUT), app.get(InstantSource.class));
  }

  @Override
  public void boot(App app) {
    if (app.config().get(INGESTION_ENABLED)) {
      bootIngestion(app);
    } else {
      LOG.info("ingestion is off (ingest.ingestion-enabled): this process takes no ingestion leases");
    }
    if (app.config().get(FUNCTIONS_ENABLED)) {
      bootFunctions(app);
    }
  }

  /** The function role: the control plane as the runtime's service account, the listener, the reconcile loop. */
  private void bootFunctions(App app) {
    Config config = app.config();
    PlatformClient platform = app.get(PlatformClient.class);
    ServiceClient control =
        app.get(ServiceClients.class)
            .register(
                ServiceOptions.builder(FUNCTIONS_CLIENT, config.get(FlowCatalystModule.BASE_URL))
                    .requestTimeout(config.get(FlowCatalystModule.TIMEOUT))
                    .maxResponseBytes(HttpControlPlane.MAX_ARTIFACT_BYTES + 1)
                    .auth(Auth.bearer(platform::accessToken, platform::clearToken))
                    .build());
    String version = IngestRuntime.class.getPackage().getImplementationVersion();
    functions =
        FunctionHost.start(
            config.get(FUNCTIONS_POOL),
            config.get(RUNTIME_ID),
            version == null ? "dev" : version,
            new HttpControlPlane(control),
            app.get(HttpClients.class),
            platformTokens(app),
            app.get(Routes.class).forModule(ID),
            Clock.systemUTC(),
            new DbPools.Limits(
                config.get(FUNCTIONS_DB_MAX_POOLS), config.get(FUNCTIONS_DB_MAX_CONNS), config.get(FUNCTIONS_DB_IDLE_TTL)),
            new LogBudget.Limits(config.get(FUNCTIONS_LOG_RATE), config.get(FUNCTIONS_LOG_BURST)));
  }

  private void bootIngestion(App app) {
    // Loaded now, so that an extension that cannot be carried stops the runtime from starting.
    app.get(Extensions.class);
    CamelContext camel = app.get(CamelContext.class);
    camel.start();
    ProducerTemplate template = camel.createProducerTemplate();
    producer = template;
    PlatformTokens tokens = platformTokens(app);
    HttpIngest http =
        new HttpIngest(
            app.get(Reconciler.class),
            app.get(Secrets.class),
            app.get(RateLimiter.class),
            template,
            tokens,
            app.get(MonotonicClock.class));
    Routes routes = app.get(Routes.class).forModule(ID);
    routes.post(HttpIngest.PATTERN, http::handle);
    routes.post(HttpIngest.PATTERN_WITH_APPLICATION, http::handleWithApplication);
    RuntimeLoop loop = app.get(RuntimeLoop.class);
    LOG.info("runtime {} is instance {} (sent with every poll)", app.config().get(RUNTIME_ID), app.get(PlatformApi.class).instanceId());
    // Also fails while the platform refuses the polls because another live process holds this runtime's id.
    app.health().add("platform", loop::requireRecentPoll);
    // Begins once every module has started, and is stopped before any module's shutdown.
    app.get(Background.class).loop(LOOP, LoopOptions.every(app.config().get(POLL_INTERVAL)), tick -> loop.tick());
    // Apart from the loop, so a round stuck on the platform cannot keep a lapsed lease's routes running.
    ScheduledExecutorService dog =
        Executors.newSingleThreadScheduledExecutor(Thread.ofPlatform().daemon().name("ingest-lease-watchdog").factory());
    dog.scheduleWithFixedDelay(loop::watch, 1, 1, TimeUnit.SECONDS);
    watchdog = dog;
  }

  /**
   * Stops every route at once, each given {@link Reconciler#ROUTE_STOP} to finish its exchange in
   * flight, all within {@link #STOP_WITHIN}; then gives back the leases of the single-owner ones
   * that stopped cleanly and tells the platform this runtime is leaving, within {@link
   * RuntimeLoop#GOODBYE_WITHIN}, so another runtime takes them at its next poll. A route still
   * stopping, or a platform that does not answer or predates the call, leaves the lease to run out
   * (at most the lease after the last poll). The watermark or cursor is on the platform, so nothing
   * read is lost. With the trace flush, about 20 s in all (12 + 5, a second's grace, 2), well inside
   * ECS's 60 s stop timeout.
   */
  @Override
  public void shutdown(App app) {
    FunctionHost host = functions;
    if (host != null) {
      host.close();
    }
    // The loop has already been stopped; the Camel context is closed by the application after this.
    ScheduledExecutorService dog = watchdog;
    if (dog != null) {
      // Booted all the way: the loop and the routes exist.
      dog.shutdownNow();
      RuntimeLoop loop = app.get(RuntimeLoop.class);
      loop.close();
      Reconciler.Stopped stopped = app.get(Reconciler.class).stop(ingestion -> true, STOP_WITHIN);
      if (stopped.count() > 0) {
        LOG.info("stopped {} ingestion route(s), {} of them cleanly", stopped.count(), stopped.clean().size());
      }
      loop.goodbye(stopped.clean());
      // After the routes: what their last exchanges recorded is shipped, within a bound.
      app.get(TraceShipper.class).close(TRACE_FLUSH);
    }
    ProducerTemplate template = producer;
    if (template != null) {
      template.stop();
    }
  }
}
