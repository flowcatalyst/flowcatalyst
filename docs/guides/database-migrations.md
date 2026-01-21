# Database Migrations Guide

FlowCatalyst uses MongoDB for data persistence. This guide covers migration strategies, data type handling, and best practices for schema changes.

## Migration Philosophy

FlowCatalyst follows the **Expand/Migrate/Contract** pattern:

```
1. EXPAND   - Add new fields, keep old ones
2. MIGRATE  - Transform existing data
3. CONTRACT - Remove old fields (optional)
```

This ensures zero-downtime migrations with backward compatibility.

## MongoDB Considerations

### Document Flexibility

MongoDB's schemaless nature provides flexibility but requires discipline:

- **New fields**: Add with default values
- **Type changes**: Migrate existing documents
- **Field removal**: Optional (old data doesn't hurt)

### TSID Migration

All IDs use TSID format (Crockford Base32 strings).

```javascript
// Example: Check ID format
db.collection.find({ _id: { $type: "string" } }).count()
db.collection.find({ _id: { $type: "long" } }).count()
```

## Common Migration Patterns

### Adding a New Field

```javascript
// Add 'status' field with default value
db.dispatch_jobs.updateMany(
  { status: { $exists: false } },
  { $set: { status: "PENDING" } }
)
```

### Renaming a Field

```javascript
// Rename 'old_field' to 'new_field'
db.collection.updateMany(
  { old_field: { $exists: true } },
  { $rename: { "old_field": "new_field" } }
)
```

### Changing Field Type

```javascript
// Convert string dates to ISODate
db.dispatch_jobs.find({ createdAt: { $type: "string" } }).forEach(function(doc) {
  db.dispatch_jobs.updateOne(
    { _id: doc._id },
    { $set: { createdAt: new Date(doc.createdAt) } }
  )
})
```

### Adding Embedded Document

```javascript
// Add 'webhookCredentials' embedded document
db.service_accounts.updateMany(
  { webhookCredentials: { $exists: false } },
  {
    $set: {
      webhookCredentials: {
        authType: "BEARER",
        authTokenRef: null,
        signingSecretRef: null,
        signingAlgorithm: "HMAC_SHA256",
        createdAt: new Date()
      }
    }
  }
)
```

## Data Type Fixes

### Instant Fields (DATE_TIME)

If `Instant` fields were stored as strings:

```javascript
// Fix Instant fields stored as STRING
db.dispatch_jobs.find({ createdAt: { $type: "string" } }).forEach(function(doc) {
  db.dispatch_jobs.updateOne(
    { _id: doc._id },
    { $set: { createdAt: new Date(doc.createdAt) } }
  )
})

// Verify fix
db.dispatch_jobs.find({ createdAt: { $type: "string" } }).count()
// Should return 0
```

### Enum Fields

Enums are stored as strings in MongoDB:

```javascript
// Update enum values
db.subscriptions.updateMany(
  { mode: "QUEUE_IMMEDIATELY" },  // Old value
  { $set: { mode: "IMMEDIATE" } }  // New value
)
```

### Integer Fields

Convert numeric strings to integers:

```javascript
db.dispatch_jobs.find({ attemptCount: { $type: "string" } }).forEach(function(doc) {
  db.dispatch_jobs.updateOne(
    { _id: doc._id },
    { $set: { attemptCount: parseInt(doc.attemptCount) } }
  )
})
```

## Migration Scripts

### Script Template

```javascript
// Migration: Add 'dataOnly' field to dispatch_jobs
// Date: 2024-01-15
// Reason: Support raw payload delivery mode

print("Starting migration: Add dataOnly field to dispatch_jobs");

// Pre-check
var count = db.dispatch_jobs.find({ dataOnly: { $exists: false } }).count();
print("Documents to update: " + count);

if (count > 0) {
  // Perform migration
  var result = db.dispatch_jobs.updateMany(
    { dataOnly: { $exists: false } },
    { $set: { dataOnly: true } }  // Default to true for existing jobs
  );

  print("Modified: " + result.modifiedCount);
}

// Post-check
var remaining = db.dispatch_jobs.find({ dataOnly: { $exists: false } }).count();
print("Remaining documents without field: " + remaining);

if (remaining === 0) {
  print("Migration completed successfully");
} else {
  print("WARNING: Some documents not migrated");
}
```

### Running Migrations

```bash
# Connect to MongoDB
mongosh mongodb://localhost:27017/flowcatalyst

# Run migration script
load("/path/to/migration_001_add_dataonly.js")
```

## Index Management

### Creating Indexes

```javascript
// Create index for common queries
db.dispatch_jobs.createIndex({ status: 1, scheduledFor: 1 })
db.dispatch_jobs.createIndex({ clientId: 1, createdAt: -1 })
db.dispatch_jobs.createIndex({ messageGroup: 1, sequence: 1 })

// Create unique index
db.subscriptions.createIndex(
  { code: 1, clientId: 1 },
  { unique: true }
)
```

### Listing Indexes

```javascript
db.dispatch_jobs.getIndexes()
```

### Dropping Indexes

```javascript
db.dispatch_jobs.dropIndex("old_index_name")
```

## Safety Guidelines

### DO

1. **Backup before migrations** - Always have a restore point
2. **Test on staging first** - Verify migration logic
3. **Run during low traffic** - Minimize impact
4. **Use transactions when possible** - For multi-document updates
5. **Log progress** - Track migration status

### DON'T

1. **Never drop collections** without explicit permission
2. **Avoid blocking operations** on large collections
3. **Don't assume field existence** - Check with `$exists`
4. **Don't skip validation** - Verify data after migration

## Rollback Strategy

### Reversible Migrations

Keep old data until verified:

```javascript
// Add new field, keep old
db.collection.updateMany({}, [
  { $set: { new_field: "$old_field" } }
])

// Later: remove old field
db.collection.updateMany({}, { $unset: { old_field: "" } })
```

### Point-in-Time Recovery

For critical operations, use MongoDB's backup:

```bash
# Create backup
mongodump --uri="mongodb://localhost:27017/flowcatalyst" --out=/backup/

# Restore if needed
mongorestore --uri="mongodb://localhost:27017/flowcatalyst" /backup/flowcatalyst/
```

## Monitoring Migrations

### Progress Tracking

```javascript
// Count documents in batches
var total = db.collection.count();
var migrated = db.collection.find({ new_field: { $exists: true } }).count();
print("Progress: " + migrated + "/" + total + " (" + Math.round(migrated/total*100) + "%)");
```

### Verify Data Integrity

```javascript
// Check for orphaned references
db.dispatch_jobs.aggregate([
  { $lookup: {
    from: "subscriptions",
    localField: "subscriptionId",
    foreignField: "_id",
    as: "subscription"
  }},
  { $match: { subscription: { $size: 0 }, subscriptionId: { $ne: null } } },
  { $count: "orphaned_jobs" }
])
```

## See Also

- [Entity Overview](../entities/overview.md) - Data model reference
- [Architecture Overview](../architecture/overview.md) - System design
- [Troubleshooting](../operations/troubleshooting.md) - Common issues
