package tech.flowcatalyst.outbox.repository.mongo;

import com.mongodb.client.MongoClient;
import com.mongodb.client.MongoCollection;
import com.mongodb.client.MongoCursor;
import com.mongodb.client.model.*;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.inject.Alternative;
import jakarta.inject.Inject;
import org.bson.Document;
import org.bson.conversions.Bson;
import org.jboss.logging.Logger;
import tech.flowcatalyst.outbox.config.OutboxProcessorConfig;
import tech.flowcatalyst.outbox.model.OutboxItem;
import tech.flowcatalyst.outbox.model.OutboxItemType;
import tech.flowcatalyst.outbox.model.OutboxStatus;
import tech.flowcatalyst.outbox.repository.OutboxRepository;

import java.time.Instant;
import java.util.ArrayList;
import java.util.Date;
import java.util.List;

/**
 * MongoDB implementation of OutboxRepository.
 * Uses simple find/updateMany with status codes - NO findOneAndUpdate loop.
 * Safe because only one poller runs (enforced by leader election).
 */
@ApplicationScoped
@Alternative
public class MongoOutboxRepository implements OutboxRepository {

    private static final Logger LOG = Logger.getLogger(MongoOutboxRepository.class);

    @Inject
    MongoClient mongoClient;

    @Inject
    OutboxProcessorConfig config;

    @Override
    public List<OutboxItem> fetchPending(OutboxItemType type, int limit) {
        MongoCollection<Document> collection = getCollection(type);
        List<OutboxItem> items = new ArrayList<>();

        Bson filter = Filters.eq("status", OutboxStatus.PENDING.getCode());
        Bson sort = Sorts.ascending("messageGroup", "createdAt");

        try (MongoCursor<Document> cursor = collection.find(filter)
                .sort(sort)
                .limit(limit)
                .iterator()) {
            while (cursor.hasNext()) {
                items.add(mapDocument(cursor.next(), type));
            }
        }

        return items;
    }

    @Override
    public void markAsInProgress(OutboxItemType type, List<String> ids) {
        if (ids.isEmpty()) return;

        MongoCollection<Document> collection = getCollection(type);

        Bson filter = Filters.in("_id", ids);
        Bson update = Updates.combine(
            Updates.set("status", OutboxStatus.IN_PROGRESS.getCode()),
            Updates.set("updatedAt", new Date())
        );

        long updated = collection.updateMany(filter, update).getModifiedCount();
        LOG.debugf("Marked %d items as in-progress in %s", updated, getCollectionName(type));
    }

    @Override
    public void markWithStatus(OutboxItemType type, List<String> ids, OutboxStatus status) {
        if (ids.isEmpty()) return;

        MongoCollection<Document> collection = getCollection(type);

        Bson filter = Filters.in("_id", ids);
        Bson update = Updates.combine(
            Updates.set("status", status.getCode()),
            Updates.set("updatedAt", new Date())
        );

        long updated = collection.updateMany(filter, update).getModifiedCount();
        LOG.debugf("Marked %d items with status %s in %s", updated, status, getCollectionName(type));
    }

    @Override
    public void markWithStatusAndError(OutboxItemType type, List<String> ids, OutboxStatus status, String errorMessage) {
        if (ids.isEmpty()) return;

        MongoCollection<Document> collection = getCollection(type);

        Bson filter = Filters.in("_id", ids);
        Bson update = Updates.combine(
            Updates.set("status", status.getCode()),
            Updates.set("errorMessage", errorMessage),
            Updates.set("updatedAt", new Date())
        );

        long updated = collection.updateMany(filter, update).getModifiedCount();
        LOG.debugf("Marked %d items with status %s and error in %s", updated, status, getCollectionName(type));
    }

    @Override
    public List<OutboxItem> fetchStuckItems(OutboxItemType type) {
        MongoCollection<Document> collection = getCollection(type);
        List<OutboxItem> items = new ArrayList<>();

        Bson filter = Filters.eq("status", OutboxStatus.IN_PROGRESS.getCode());
        Bson sort = Sorts.ascending("createdAt");

        try (MongoCursor<Document> cursor = collection.find(filter).sort(sort).iterator()) {
            while (cursor.hasNext()) {
                items.add(mapDocument(cursor.next(), type));
            }
        }

        return items;
    }

    @Override
    public void resetStuckItems(OutboxItemType type, List<String> ids) {
        if (ids.isEmpty()) return;

        MongoCollection<Document> collection = getCollection(type);

        Bson filter = Filters.in("_id", ids);
        Bson update = Updates.combine(
            Updates.set("status", OutboxStatus.PENDING.getCode()),
            Updates.set("updatedAt", new Date())
        );

        long updated = collection.updateMany(filter, update).getModifiedCount();
        LOG.debugf("Reset %d stuck items in %s", updated, getCollectionName(type));
    }

    @Override
    public void incrementRetryCount(OutboxItemType type, List<String> ids) {
        if (ids.isEmpty()) return;

        MongoCollection<Document> collection = getCollection(type);

        Bson filter = Filters.in("_id", ids);
        Bson update = Updates.combine(
            Updates.set("status", OutboxStatus.PENDING.getCode()),
            Updates.inc("retryCount", 1),
            Updates.set("updatedAt", new Date())
        );

        long updated = collection.updateMany(filter, update).getModifiedCount();
        LOG.debugf("Incremented retry count for %d items in %s", updated, getCollectionName(type));
    }

    @Override
    public List<OutboxItem> fetchRecoverableItems(OutboxItemType type, int timeoutSeconds, int limit) {
        MongoCollection<Document> collection = getCollection(type);
        List<OutboxItem> items = new ArrayList<>();

        // Calculate cutoff time
        Date cutoff = new Date(System.currentTimeMillis() - (timeoutSeconds * 1000L));

        // Filter for error statuses older than timeout
        Bson filter = Filters.and(
            Filters.in("status",
                OutboxStatus.IN_PROGRESS.getCode(),
                OutboxStatus.BAD_REQUEST.getCode(),
                OutboxStatus.INTERNAL_ERROR.getCode(),
                OutboxStatus.UNAUTHORIZED.getCode(),
                OutboxStatus.FORBIDDEN.getCode(),
                OutboxStatus.GATEWAY_ERROR.getCode()
            ),
            Filters.lt("updatedAt", cutoff)
        );
        Bson sort = Sorts.ascending("createdAt");

        try (MongoCursor<Document> cursor = collection.find(filter)
                .sort(sort)
                .limit(limit)
                .iterator()) {
            while (cursor.hasNext()) {
                items.add(mapDocument(cursor.next(), type));
            }
        }

        return items;
    }

    @Override
    public void resetRecoverableItems(OutboxItemType type, List<String> ids) {
        if (ids.isEmpty()) return;

        MongoCollection<Document> collection = getCollection(type);

        Bson filter = Filters.in("_id", ids);
        Bson update = Updates.combine(
            Updates.set("status", OutboxStatus.PENDING.getCode()),
            Updates.set("updatedAt", new Date())
        );

        long updated = collection.updateMany(filter, update).getModifiedCount();
        LOG.debugf("Reset %d recoverable items in %s", updated, getCollectionName(type));
    }

    @Override
    public long countPending(OutboxItemType type) {
        MongoCollection<Document> collection = getCollection(type);
        Bson filter = Filters.eq("status", OutboxStatus.PENDING.getCode());
        return collection.countDocuments(filter);
    }

    @Override
    public String getTableName(OutboxItemType type) {
        return getCollectionName(type);
    }

    @Override
    public void createSchema() {
        for (OutboxItemType type : OutboxItemType.values()) {
            MongoCollection<Document> collection = getCollection(type);
            String collName = getCollectionName(type);

            // Index for fetching pending items (status=0, ordered by messageGroup, createdAt)
            // MongoDB supports partial indexes via partialFilterExpression
            IndexOptions pendingIndexOptions = new IndexOptions()
                .name("idx_pending")
                .partialFilterExpression(Filters.eq("status", OutboxStatus.PENDING.getCode()));

            collection.createIndex(
                Indexes.ascending("status", "messageGroup", "createdAt"),
                pendingIndexOptions
            );

            // Index for finding stuck items (status=9)
            IndexOptions stuckIndexOptions = new IndexOptions()
                .name("idx_stuck")
                .partialFilterExpression(Filters.eq("status", OutboxStatus.IN_PROGRESS.getCode()));

            collection.createIndex(
                Indexes.ascending("status", "createdAt"),
                stuckIndexOptions
            );

            LOG.infof("Created indexes on %s", collName);
        }
    }

    private MongoCollection<Document> getCollection(OutboxItemType type) {
        return mongoClient
            .getDatabase(config.mongoDatabase())
            .getCollection(getCollectionName(type));
    }

    private String getCollectionName(OutboxItemType type) {
        return type == OutboxItemType.EVENT ? config.eventsTable() : config.dispatchJobsTable();
    }

    private OutboxItem mapDocument(Document doc, OutboxItemType type) {
        // Status is now stored as integer
        int statusCode = doc.getInteger("status", 0);

        return new OutboxItem(
            doc.getString("_id"),
            type,
            doc.getString("messageGroup"),
            doc.getString("payload"),
            OutboxStatus.fromCode(statusCode),
            doc.getInteger("retryCount", 0),
            toInstant(doc.getDate("createdAt")),
            toInstant(doc.getDate("updatedAt")),
            doc.getString("errorMessage")
        );
    }

    private Instant toInstant(Date date) {
        return date != null ? date.toInstant() : null;
    }
}
