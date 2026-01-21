use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StreamConfig {
    pub name: String,
    pub source_database: String,
    pub source_collection: String,
    pub batch_max_size: u32,
    pub batch_max_wait_ms: u64,
    pub watch_operations: Vec<String>,
    /// Maximum concurrent batch processors
    pub concurrency: u32,
    /// Field name to use as aggregate ID for ordering guarantees
    /// Documents with the same aggregate ID will not be in concurrent batches
    pub aggregate_id_field: String,
}

impl Default for StreamConfig {
    fn default() -> Self {
        Self {
            name: "default".to_string(),
            source_database: "test".to_string(),
            source_collection: "events".to_string(),
            batch_max_size: 100,
            batch_max_wait_ms: 100,
            watch_operations: vec!["insert".to_string(), "update".to_string(), "replace".to_string()],
            concurrency: 10,
            aggregate_id_field: "_id".to_string(),
        }
    }
}
