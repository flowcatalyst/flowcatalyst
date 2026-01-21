package tech.flowcatalyst.sdk.outbox.driver;

import com.mongodb.client.MongoClient;
import com.mongodb.client.MongoCollection;
import org.bson.Document;
import tech.flowcatalyst.sdk.exception.OutboxException;

import java.time.Instant;
import java.util.Date;
import java.util.List;
import java.util.Map;

/**
 * MongoDB driver for outbox messages.
 *
 * <p>Requires the MongoDB client to be provided (e.g., from Quarkus MongoDB extension).
 */
public class MongoDriver implements OutboxDriver {

    private final MongoClient mongoClient;
    private final String databaseName;
    private final String collectionName;

    public MongoDriver(MongoClient mongoClient, String databaseName) {
        this(mongoClient, databaseName, "outbox_messages");
    }

    public MongoDriver(MongoClient mongoClient, String databaseName, String collectionName) {
        this.mongoClient = mongoClient;
        this.databaseName = databaseName;
        this.collectionName = collectionName;
    }

    @Override
    public void insert(Map<String, Object> message) {
        try {
            getCollection().insertOne(prepareDocument(message));
        } catch (Exception e) {
            throw OutboxException.insertFailed(e);
        }
    }

    @Override
    public void insertBatch(List<Map<String, Object>> messages) {
        if (messages == null || messages.isEmpty()) {
            return;
        }

        try {
            var documents = messages.stream()
                .map(this::prepareDocument)
                .toList();
            getCollection().insertMany(documents);
        } catch (Exception e) {
            throw OutboxException.insertFailed(e);
        }
    }

    private Document prepareDocument(Map<String, Object> message) {
        var doc = new Document();
        doc.put("_id", message.get("id"));
        doc.put("tenant_id", message.get("tenant_id"));
        doc.put("partition_id", message.get("partition_id"));
        doc.put("type", message.get("type"));
        doc.put("payload", message.get("payload"));
        doc.put("payload_size", message.get("payload_size"));
        doc.put("status", message.get("status"));

        // Convert created_at to Date
        Object createdAt = message.get("created_at");
        if (createdAt instanceof Instant instant) {
            doc.put("created_at", Date.from(instant));
        } else if (createdAt instanceof String str) {
            doc.put("created_at", Date.from(Instant.parse(str)));
        } else {
            doc.put("created_at", new Date());
        }

        doc.put("headers", message.get("headers"));
        doc.put("processed_at", null);
        doc.put("retry_count", 0);
        doc.put("error_reason", null);

        return doc;
    }

    private MongoCollection<Document> getCollection() {
        return mongoClient.getDatabase(databaseName).getCollection(collectionName);
    }
}
