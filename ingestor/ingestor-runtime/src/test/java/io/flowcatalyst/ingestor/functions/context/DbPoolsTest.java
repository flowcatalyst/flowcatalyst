package io.flowcatalyst.ingestor.functions.context;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.flowcatalyst.ingestor.functions.Harness.MovableClock;
import java.sql.Connection;
import java.sql.SQLException;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.atomic.AtomicBoolean;
import javax.sql.DataSource;
import org.junit.jupiter.api.Test;

/// Pool sharing, the cap, the idle TTL and shutdown, with a fake pool so no database is needed.
class DbPoolsTest {

    static final class FakePool implements DbPools.Pool {
        final PostgresDsn.Target target;
        final AtomicBoolean closed = new AtomicBoolean();
        final DataSource ds = (DataSource) java.lang.reflect.Proxy.newProxyInstance(
                DataSource.class.getClassLoader(), new Class<?>[] {DataSource.class}, (p, m, a) -> {
                    if (m.getName().equals("getConnection")) {
                        return (Connection) java.lang.reflect.Proxy.newProxyInstance(
                                Connection.class.getClassLoader(), new Class<?>[] {Connection.class}, (p2, m2, a2) -> null);
                    }
                    return null;
                });

        FakePool(PostgresDsn.Target target) {
            this.target = target;
        }

        @Override
        public DataSource dataSource() {
            return ds;
        }

        @Override
        public void close() {
            closed.set(true);
        }
    }

    final MovableClock clock = new MovableClock();
    final List<FakePool> made = new ArrayList<>();

    DbPools pools(int max, Duration ttl) {
        return new DbPools(new DbPools.Limits(max, 3, ttl), clock, (target, conns, name) -> {
            assertThat(conns).isEqualTo(3);
            FakePool p = new FakePool(target);
            made.add(p);
            return p;
        });
    }

    static DataSource source(DbPools pools, String dsn) {
        return pools.shared(dsn, PostgresDsn.parse(dsn));
    }

    @Test
    void goRunnerDefaults() {
        assertThat(DbPools.Limits.DEFAULTS).isEqualTo(new DbPools.Limits(32, 4, Duration.ofMinutes(10)));
    }

    @Test
    void oneSharedPoolPerDsnAcrossDataSources() throws SQLException {
        try (DbPools p = pools(4, Duration.ofMinutes(10))) {
            DataSource a = source(p, "postgres://u:p@h/a");
            DataSource a2 = source(p, "postgres://u:p@h/a");
            DataSource b = source(p, "postgres://u:p@h/b");
            a.getConnection();
            a2.getConnection();
            a.getConnection();
            b.getConnection();
            assertThat(made).hasSize(2);
            assertThat(p.size()).isEqualTo(2);
        }
    }

    @Test
    void capEvictsAndClosesTheOldest() throws SQLException {
        try (DbPools p = pools(2, Duration.ofMinutes(10))) {
            DataSource a = source(p, "postgres://u:p@h/a");
            DataSource b = source(p, "postgres://u:p@h/b");
            DataSource c = source(p, "postgres://u:p@h/c");
            a.getConnection();
            clock.advance(Duration.ofSeconds(1));
            b.getConnection();
            clock.advance(Duration.ofSeconds(1));
            a.getConnection(); // a is now newer than b
            clock.advance(Duration.ofSeconds(1));
            c.getConnection(); // past the cap: b goes
            assertThat(made).hasSize(3);
            assertThat(made.get(1).target.jdbcUrl()).endsWith("/b");
            assertThat(made.get(1).closed).isTrue();
            assertThat(made.get(0).closed).isFalse();
            assertThat(p.size()).isEqualTo(2);
            b.getConnection(); // a closed pool is made again, evicting the oldest (a)
            assertThat(made).hasSize(4);
            assertThat(made.get(0).closed).isTrue();
        }
    }

    @Test
    void idleTtlClosesOnlyIdlePools() throws SQLException {
        try (DbPools p = pools(4, Duration.ofMinutes(10))) {
            DataSource a = source(p, "postgres://u:p@h/a");
            DataSource b = source(p, "postgres://u:p@h/b");
            a.getConnection();
            b.getConnection();
            clock.advance(Duration.ofMinutes(6));
            a.getConnection(); // used again
            clock.advance(Duration.ofMinutes(6));
            p.sweep();
            assertThat(made.get(1).closed).as("b idle 12 minutes").isTrue();
            assertThat(made.get(0).closed).as("a idle 6 minutes").isFalse();
            assertThat(p.size()).isEqualTo(1);
            clock.advance(Duration.ofMinutes(5));
            p.sweep();
            assertThat(made.get(0).closed).isTrue();
            assertThat(p.size()).isZero();
            a.getConnection(); // and it is simply made again
            assertThat(made).hasSize(3);
        }
    }

    @Test
    void closeClosesAllAndRefusesMore() throws SQLException {
        DbPools p = pools(4, Duration.ofMinutes(10));
        DataSource a = source(p, "postgres://u:p@h/a");
        a.getConnection();
        source(p, "postgres://u:p@h/b").getConnection();
        p.close();
        assertThat(made).allMatch(f -> f.closed.get());
        assertThat(p.size()).isZero();
        assertThatThrownBy(a::getConnection).isInstanceOf(SQLException.class);
    }

    @Test
    void theWrapperIsNotCloseableAndUnwrapsNothingHostOnly() throws SQLException {
        try (DbPools p = pools(4, Duration.ofMinutes(10))) {
            DataSource a = source(p, "postgres://u:p@h/a");
            assertThat(a).isNotInstanceOf(AutoCloseable.class);
            assertThat(a.isWrapperFor(com.zaxxer.hikari.HikariDataSource.class)).isFalse();
            assertThat(a.isWrapperFor(AutoCloseable.class)).isFalse();
            assertThatThrownBy(() -> a.unwrap(com.zaxxer.hikari.HikariDataSource.class)).isInstanceOf(SQLException.class);
            assertThat(a.unwrap(DataSource.class)).isSameAs(a);
            assertThatThrownBy(() -> a.getConnection("x", "y")).isInstanceOf(SQLException.class);
        }
    }

    @Test
    void theRealFactoryBuildsALazyPoolWithoutLeakingThePassword() throws Exception {
        try (DbPools p = new DbPools(new DbPools.Limits(2, 2, Duration.ofMinutes(1)), clock)) {
            DataSource a = source(p, "postgres://u:hunter2pw@127.0.0.1:1/d?connect_timeout=1");
            // nothing listens on port 1: the failure surfaces at first use, and says nothing of the password
            Throwable t = org.assertj.core.api.Assertions.catchThrowable(a::getConnection);
            assertThat(t).isInstanceOf(SQLException.class);
            for (Throwable c = t; c != null; c = c.getCause()) {
                assertThat(String.valueOf(c.getMessage())).doesNotContain("hunter2pw");
            }
            assertThat(a.toString()).doesNotContain("hunter2pw");
        }
    }
}
