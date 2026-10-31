package io.flowcatalyst.ingestor.functions.context;

import com.zaxxer.hikari.HikariConfig;
import com.zaxxer.hikari.HikariDataSource;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.sql.SQLException;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.util.HashMap;
import java.util.HexFormat;
import java.util.Map;
import java.util.Objects;
import java.util.Properties;
import javax.sql.DataSource;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/// The database connection pools of the function host, one per process: **one pool per distinct DSN, shared by
/// every function** that is bound to it (the Go runner's `dbPools`). At most [Limits#maxPools] pools are open; a
/// new DSN past the cap closes the pool that was used longest ago. A pool nobody has used for [Limits#idleTtl] is
/// closed by [#sweep]. [#close] closes them all. A closed pool is simply made again by the next call that needs it.
///
/// A DSN holds a password: it is the key to nothing visible. Pools are keyed by a hash of it and only the first
/// characters of the hash are ever logged.
public final class DbPools implements AutoCloseable {

    private static final Logger LOG = LoggerFactory.getLogger(DbPools.class);

    /// The Go runner's defaults: `runner.Config.MaxDBPools` 32, 4 connections per pool (`newDBPools(cfg.MaxDBPools, 4)`),
    /// pools closed after 10 minutes idle (`closeIdle(10 * time.Minute)`).
    public record Limits(int maxPools, int maxConnsPerPool, Duration idleTtl) {
        public static final Limits DEFAULTS = new Limits(32, 4, Duration.ofMinutes(10));

        public Limits {
            if (maxPools < 1) {
                throw new IllegalArgumentException("maxPools must be at least 1");
            }
            if (maxConnsPerPool < 1) {
                throw new IllegalArgumentException("maxConnsPerPool must be at least 1");
            }
            if (idleTtl.isNegative() || idleTtl.isZero()) {
                throw new IllegalArgumentException("idleTtl must be positive");
            }
        }
    }

    /// A pool the host opened, as far as [DbPools] is concerned. The production one is a Hikari pool.
    public interface Pool extends AutoCloseable {
        DataSource dataSource();

        @Override
        void close();
    }

    /// Makes the pool for a parsed DSN.
    @FunctionalInterface
    public interface PoolFactory {
        Pool create(PostgresDsn.Target target, int maxConns, String name);
    }

    private static final class Entry {
        final Pool pool;
        Instant lastUsed;

        Entry(Pool pool, Instant lastUsed) {
            this.pool = pool;
            this.lastUsed = lastUsed;
        }
    }

    private final Limits limits;
    private final Clock clock;
    private final PoolFactory factory;
    private final Map<String, Entry> pools = new HashMap<>();
    private boolean closed;

    public DbPools(Limits limits, Clock clock) {
        this(limits, clock, DbPools::hikari);
    }

    public DbPools(Limits limits, Clock clock, PoolFactory factory) {
        this.limits = Objects.requireNonNull(limits, "limits");
        this.clock = Objects.requireNonNull(clock, "clock");
        this.factory = Objects.requireNonNull(factory, "factory");
    }

    public Limits limits() {
        return limits;
    }

    /// A function-facing DataSource for `dsn` (already parsed as `target`): borrowing from it takes the shared
    /// pool for the DSN, making it if need be.
    DataSource shared(String dsn, PostgresDsn.Target target) {
        return new SharedDataSource(this, key(dsn), target);
    }

    /// The shared pool's data source for the DSN `key`, made if absent and marked used.
    DataSource acquire(String key, PostgresDsn.Target target) throws SQLException {
        Pool evicted = null;
        Entry entry;
        synchronized (this) {
            if (closed) {
                throw new SQLException("the function host is shutting down");
            }
            entry = pools.get(key);
            if (entry == null) {
                if (pools.size() >= limits.maxPools()) {
                    Map.Entry<String, Entry> oldest = null;
                    for (Map.Entry<String, Entry> e : pools.entrySet()) {
                        if (oldest == null || e.getValue().lastUsed.isBefore(oldest.getValue().lastUsed)) {
                            oldest = e;
                        }
                    }
                    if (oldest != null) {
                        pools.remove(oldest.getKey());
                        evicted = oldest.getValue().pool;
                        LOG.info("database pool cap {} reached: closing the least recently used pool {}", limits.maxPools(), shortKey(oldest.getKey()));
                    }
                }
                try {
                    entry = new Entry(factory.create(target, limits.maxConnsPerPool(), "fn-db-" + shortKey(key)), clock.instant());
                } catch (RuntimeException e) {
                    // never echo the driver's message wholesale: keep the type only
                    throw new SQLException("the database could not be reached (" + e.getClass().getSimpleName() + ")");
                }
                pools.put(key, entry);
                LOG.debug("database pool {} opened ({} open)", shortKey(key), pools.size());
            }
            entry.lastUsed = clock.instant();
        }
        if (evicted != null) {
            closeQuietly(evicted);
        }
        return entry.pool.dataSource();
    }

    /// Closes the pools unused for longer than the idle TTL.
    public void sweep() {
        java.util.List<Pool> idle = new java.util.ArrayList<>();
        synchronized (this) {
            Instant cutoff = clock.instant().minus(limits.idleTtl());
            pools.entrySet().removeIf(e -> {
                if (e.getValue().lastUsed.isBefore(cutoff)) {
                    LOG.debug("database pool {} idle: closing", shortKey(e.getKey()));
                    idle.add(e.getValue().pool);
                    return true;
                }
                return false;
            });
        }
        idle.forEach(DbPools::closeQuietly);
    }

    /// The number of pools open.
    public synchronized int size() {
        return pools.size();
    }

    @Override
    public void close() {
        java.util.List<Pool> all;
        synchronized (this) {
            closed = true;
            all = pools.values().stream().map(e -> e.pool).toList();
            pools.clear();
        }
        all.forEach(DbPools::closeQuietly);
    }

    private static void closeQuietly(Pool pool) {
        try {
            pool.close();
        } catch (RuntimeException e) {
            LOG.warn("closing a database pool failed: {}", e.getClass().getSimpleName());
        }
    }

    /// Hash of the DSN: the map key. SHA-256 hex.
    static String key(String dsn) {
        try {
            return HexFormat.of().formatHex(MessageDigest.getInstance("SHA-256").digest(dsn.getBytes(StandardCharsets.UTF_8)));
        } catch (NoSuchAlgorithmException e) {
            throw new IllegalStateException("SHA-256 unavailable", e);
        }
    }

    static String shortKey(String key) {
        return key.substring(0, 8);
    }

    private static Pool hikari(PostgresDsn.Target target, int maxConns, String name) {
        HikariConfig config = new HikariConfig();
        config.setPoolName(name);
        config.setJdbcUrl(target.jdbcUrl());
        config.setUsername(target.user());
        config.setPassword(target.password());
        Properties props = new Properties();
        props.putAll(target.properties());
        config.setDataSourceProperties(props);
        String connectTimeout = target.properties().get("connectTimeout");
        // how long a borrower waits for a connection: the DSN's connect_timeout when it has one, else 10 s
        config.setConnectionTimeout(connectTimeout == null ? 10_000 : Math.max(250, Integer.parseInt(connectTimeout) * 1000L));
        config.setMaximumPoolSize(maxConns);
        config.setMinimumIdle(0); // pgxpool MinConns 0
        config.setIdleTimeout(Duration.ofMinutes(5).toMillis()); // pgxpool MaxConnIdleTime
        config.setInitializationFailTimeout(-1); // like pgxpool.NewWithConfig: no connection until first use
        config.setRegisterMbeans(false);
        HikariDataSource ds = new HikariDataSource(config);
        return new Pool() {
            @Override
            public DataSource dataSource() {
                return ds;
            }

            @Override
            public void close() {
                ds.close();
            }
        };
    }
}
