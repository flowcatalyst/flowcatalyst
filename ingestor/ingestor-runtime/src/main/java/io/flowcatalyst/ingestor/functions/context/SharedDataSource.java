package io.flowcatalyst.ingestor.functions.context;

import java.io.PrintWriter;
import java.sql.Connection;
import java.sql.SQLException;
import java.sql.SQLFeatureNotSupportedException;
import java.util.logging.Logger;
import javax.sql.DataSource;
import org.jspecify.annotations.Nullable;

/// What a function gets from `ctx.dataSource(name)`: a [DataSource] that borrows from the host's shared pool for
/// the DSN, looked up on every call, so a pool the host closed (idle, evicted) is made again and a function holding
/// this for the version's lifetime never holds a dead pool. It is **not** closeable and does not unwrap to the
/// pool: the pool belongs to the host and every other function bound to the same DSN.
final class SharedDataSource implements DataSource {

    private final DbPools pools;
    private final String key;
    private final PostgresDsn.Target target;

    SharedDataSource(DbPools pools, String key, PostgresDsn.Target target) {
        this.pools = pools;
        this.key = key;
        this.target = target;
    }

    @Override
    public Connection getConnection() throws SQLException {
        return pools.acquire(key, target).getConnection();
    }

    @Override
    public Connection getConnection(@Nullable String username, @Nullable String password) throws SQLException {
        throw new SQLFeatureNotSupportedException("the connection's user is fixed by the function's database binding");
    }

    @Override
    @SuppressWarnings("unchecked")
    public <T> T unwrap(Class<T> iface) throws SQLException {
        if (iface.isInstance(this)) {
            return (T) this;
        }
        throw new SQLException("this data source wraps nothing a function may use");
    }

    @Override
    public boolean isWrapperFor(Class<?> iface) {
        return iface.isInstance(this);
    }

    @Override
    public @Nullable PrintWriter getLogWriter() {
        return null;
    }

    @Override
    public void setLogWriter(@Nullable PrintWriter out) {
        // the host logs; nothing to set
    }

    @Override
    public void setLoginTimeout(int seconds) {
        // fixed by the DSN's connect_timeout
    }

    @Override
    public int getLoginTimeout() {
        return 0;
    }

    @Override
    public Logger getParentLogger() throws SQLFeatureNotSupportedException {
        throw new SQLFeatureNotSupportedException();
    }

    @Override
    public String toString() {
        return "DataSource[" + DbPools.shortKey(key) + "]";
    }
}
