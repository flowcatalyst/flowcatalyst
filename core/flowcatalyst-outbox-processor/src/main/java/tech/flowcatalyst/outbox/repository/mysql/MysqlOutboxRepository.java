package tech.flowcatalyst.outbox.repository.mysql;

import io.agroal.api.AgroalDataSource;
import io.quarkus.agroal.DataSource;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.inject.Alternative;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.outbox.config.OutboxProcessorConfig;
import tech.flowcatalyst.outbox.model.OutboxItem;
import tech.flowcatalyst.outbox.model.OutboxItemType;
import tech.flowcatalyst.outbox.model.OutboxStatus;
import tech.flowcatalyst.outbox.repository.OutboxRepository;

import java.sql.*;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.stream.Collectors;

/**
 * MySQL implementation of OutboxRepository.
 * Uses simple SELECT/UPDATE with status codes - NO row locking.
 * Safe because only one poller runs (enforced by leader election).
 */
@ApplicationScoped
@Alternative
public class MysqlOutboxRepository implements OutboxRepository {

    private static final Logger LOG = Logger.getLogger(MysqlOutboxRepository.class);

    @Inject
    @DataSource("outbox")
    AgroalDataSource dataSource;

    @Inject
    OutboxProcessorConfig config;

    @Override
    public List<OutboxItem> fetchPending(OutboxItemType type, int limit) {
        String table = getTableName(type);

        String sql = """
            SELECT id, type, message_group, payload, status, retry_count, created_at, updated_at, error_message
            FROM %s
            WHERE status = %d
            ORDER BY message_group, created_at
            LIMIT ?
            """.formatted(table, OutboxStatus.PENDING.getCode());

        List<OutboxItem> items = new ArrayList<>();

        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(sql)) {

            stmt.setInt(1, limit);

            try (ResultSet rs = stmt.executeQuery()) {
                while (rs.next()) {
                    items.add(mapRow(rs, type));
                }
            }

        } catch (SQLException e) {
            LOG.errorf(e, "Failed to fetch pending items from %s", table);
            throw new RuntimeException("Failed to fetch pending items", e);
        }

        return items;
    }

    @Override
    public void markAsInProgress(OutboxItemType type, List<String> ids) {
        if (ids.isEmpty()) return;

        String table = getTableName(type);
        String placeholders = ids.stream().map(id -> "?").collect(Collectors.joining(","));

        String sql = """
            UPDATE %s
            SET status = %d, updated_at = NOW()
            WHERE id IN (%s)
            """.formatted(table, OutboxStatus.IN_PROGRESS.getCode(), placeholders);

        executeUpdate(sql, ids, table, "mark as in-progress");
    }

    @Override
    public void markWithStatus(OutboxItemType type, List<String> ids, OutboxStatus status) {
        if (ids.isEmpty()) return;

        String table = getTableName(type);
        String placeholders = ids.stream().map(id -> "?").collect(Collectors.joining(","));

        String sql = """
            UPDATE %s
            SET status = %d, updated_at = NOW()
            WHERE id IN (%s)
            """.formatted(table, status.getCode(), placeholders);

        executeUpdate(sql, ids, table, "mark with status " + status);
    }

    @Override
    public void markWithStatusAndError(OutboxItemType type, List<String> ids, OutboxStatus status, String errorMessage) {
        if (ids.isEmpty()) return;

        String table = getTableName(type);
        String placeholders = ids.stream().map(id -> "?").collect(Collectors.joining(","));

        String sql = """
            UPDATE %s
            SET status = %d, error_message = ?, updated_at = NOW()
            WHERE id IN (%s)
            """.formatted(table, status.getCode(), placeholders);

        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(sql)) {

            stmt.setString(1, errorMessage);
            for (int i = 0; i < ids.size(); i++) {
                stmt.setString(i + 2, ids.get(i));
            }

            int updated = stmt.executeUpdate();
            LOG.debugf("Marked %d items with status %s in %s", updated, status, table);

        } catch (SQLException e) {
            LOG.errorf(e, "Failed to mark items with status and error in %s", table);
            throw new RuntimeException("Failed to mark items with status and error", e);
        }
    }

    @Override
    public List<OutboxItem> fetchStuckItems(OutboxItemType type) {
        String table = getTableName(type);

        String sql = """
            SELECT id, type, message_group, payload, status, retry_count, created_at, updated_at, error_message
            FROM %s
            WHERE status = %d
            ORDER BY created_at
            """.formatted(table, OutboxStatus.IN_PROGRESS.getCode());

        List<OutboxItem> items = new ArrayList<>();

        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(sql);
             ResultSet rs = stmt.executeQuery()) {

            while (rs.next()) {
                items.add(mapRow(rs, type));
            }

        } catch (SQLException e) {
            LOG.errorf(e, "Failed to fetch stuck items from %s", table);
            throw new RuntimeException("Failed to fetch stuck items", e);
        }

        return items;
    }

    @Override
    public void resetStuckItems(OutboxItemType type, List<String> ids) {
        if (ids.isEmpty()) return;

        String table = getTableName(type);
        String placeholders = ids.stream().map(id -> "?").collect(Collectors.joining(","));

        String sql = """
            UPDATE %s
            SET status = %d, updated_at = NOW()
            WHERE id IN (%s)
            """.formatted(table, OutboxStatus.PENDING.getCode(), placeholders);

        executeUpdate(sql, ids, table, "reset stuck items");
    }

    @Override
    public void incrementRetryCount(OutboxItemType type, List<String> ids) {
        if (ids.isEmpty()) return;

        String table = getTableName(type);
        String placeholders = ids.stream().map(id -> "?").collect(Collectors.joining(","));

        String sql = """
            UPDATE %s
            SET status = %d, retry_count = retry_count + 1, updated_at = NOW()
            WHERE id IN (%s)
            """.formatted(table, OutboxStatus.PENDING.getCode(), placeholders);

        executeUpdate(sql, ids, table, "increment retry count");
    }

    @Override
    public List<OutboxItem> fetchRecoverableItems(OutboxItemType type, int timeoutSeconds, int limit) {
        String table = getTableName(type);

        // Fetch items with error statuses that are older than timeout
        String sql = """
            SELECT id, type, message_group, payload, status, retry_count, created_at, updated_at, error_message
            FROM %s
            WHERE status IN (%d, %d, %d, %d, %d, %d)
              AND updated_at < DATE_SUB(NOW(), INTERVAL %d SECOND)
            ORDER BY created_at
            LIMIT ?
            """.formatted(
                table,
                OutboxStatus.IN_PROGRESS.getCode(),
                OutboxStatus.BAD_REQUEST.getCode(),
                OutboxStatus.INTERNAL_ERROR.getCode(),
                OutboxStatus.UNAUTHORIZED.getCode(),
                OutboxStatus.FORBIDDEN.getCode(),
                OutboxStatus.GATEWAY_ERROR.getCode(),
                timeoutSeconds
            );

        List<OutboxItem> items = new ArrayList<>();

        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(sql)) {

            stmt.setInt(1, limit);

            try (ResultSet rs = stmt.executeQuery()) {
                while (rs.next()) {
                    items.add(mapRow(rs, type));
                }
            }

        } catch (SQLException e) {
            LOG.errorf(e, "Failed to fetch recoverable items from %s", table);
            throw new RuntimeException("Failed to fetch recoverable items", e);
        }

        return items;
    }

    @Override
    public void resetRecoverableItems(OutboxItemType type, List<String> ids) {
        if (ids.isEmpty()) return;

        String table = getTableName(type);
        String placeholders = ids.stream().map(id -> "?").collect(Collectors.joining(","));

        String sql = """
            UPDATE %s
            SET status = %d, updated_at = NOW()
            WHERE id IN (%s)
            """.formatted(table, OutboxStatus.PENDING.getCode(), placeholders);

        executeUpdate(sql, ids, table, "reset recoverable items");
    }

    @Override
    public long countPending(OutboxItemType type) {
        String table = getTableName(type);
        String sql = "SELECT COUNT(*) FROM %s WHERE status = %d".formatted(table, OutboxStatus.PENDING.getCode());

        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(sql);
             ResultSet rs = stmt.executeQuery()) {

            if (rs.next()) {
                return rs.getLong(1);
            }
            return 0;

        } catch (SQLException e) {
            LOG.errorf(e, "Failed to count pending items in %s", table);
            throw new RuntimeException("Failed to count pending items", e);
        }
    }

    @Override
    public String getTableName(OutboxItemType type) {
        return type == OutboxItemType.EVENT ? config.eventsTable() : config.dispatchJobsTable();
    }

    @Override
    public void createSchema() {
        for (OutboxItemType type : OutboxItemType.values()) {
            String table = getTableName(type);
            createTableIfNotExists(table);
            createIndexes(table);
        }
    }

    private void createTableIfNotExists(String table) {
        String sql = """
            CREATE TABLE IF NOT EXISTS %s (
                id VARCHAR(26) PRIMARY KEY,
                type VARCHAR(20) NOT NULL,
                message_group VARCHAR(255),
                payload TEXT NOT NULL,
                status SMALLINT NOT NULL DEFAULT 0,
                retry_count SMALLINT NOT NULL DEFAULT 0,
                created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
                updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
                error_message TEXT
            ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
            """.formatted(table);

        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(sql)) {
            stmt.executeUpdate();
            LOG.infof("Created table %s if not exists", table);
        } catch (SQLException e) {
            LOG.errorf(e, "Failed to create table %s", table);
            throw new RuntimeException("Failed to create table", e);
        }
    }

    private void createIndexes(String table) {
        // MySQL doesn't support partial indexes, so we create standard composite indexes
        String pendingIndex = """
            CREATE INDEX idx_%s_pending
            ON %s(status, message_group, created_at)
            """.formatted(table, table);

        String stuckIndex = """
            CREATE INDEX idx_%s_stuck
            ON %s(status, created_at)
            """.formatted(table, table);

        try (Connection conn = dataSource.getConnection()) {
            // Try to create indexes, ignore if they already exist
            try (PreparedStatement stmt = conn.prepareStatement(pendingIndex)) {
                stmt.executeUpdate();
            } catch (SQLException e) {
                if (!e.getMessage().contains("Duplicate key name")) {
                    throw e;
                }
            }
            try (PreparedStatement stmt = conn.prepareStatement(stuckIndex)) {
                stmt.executeUpdate();
            } catch (SQLException e) {
                if (!e.getMessage().contains("Duplicate key name")) {
                    throw e;
                }
            }
            LOG.debugf("Created indexes on %s", table);
        } catch (SQLException e) {
            LOG.errorf(e, "Failed to create indexes on %s", table);
            throw new RuntimeException("Failed to create indexes", e);
        }
    }

    private void executeUpdate(String sql, List<String> ids, String table, String operation) {
        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(sql)) {

            for (int i = 0; i < ids.size(); i++) {
                stmt.setString(i + 1, ids.get(i));
            }

            int updated = stmt.executeUpdate();
            LOG.debugf("%s: %d items in %s", operation, updated, table);

        } catch (SQLException e) {
            LOG.errorf(e, "Failed to %s in %s", operation, table);
            throw new RuntimeException("Failed to " + operation, e);
        }
    }

    private OutboxItem mapRow(ResultSet rs, OutboxItemType type) throws SQLException {
        return new OutboxItem(
            rs.getString("id"),
            type,
            rs.getString("message_group"),
            rs.getString("payload"),
            OutboxStatus.fromCode(rs.getInt("status")),
            rs.getInt("retry_count"),
            toInstant(rs.getTimestamp("created_at")),
            toInstant(rs.getTimestamp("updated_at")),
            rs.getString("error_message")
        );
    }

    private Instant toInstant(Timestamp timestamp) {
        return timestamp != null ? timestamp.toInstant() : null;
    }
}
