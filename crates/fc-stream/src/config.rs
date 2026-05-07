/// Configuration for the PostgreSQL projection stream processor.
pub struct StreamProcessorConfig {
    /// Enable the event projection loop
    pub events_enabled: bool,
    /// Max rows per event projection poll cycle
    pub events_batch_size: u32,
    /// Enable the dispatch job projection loop
    pub dispatch_jobs_enabled: bool,
    /// Max rows per dispatch job projection poll cycle
    pub dispatch_jobs_batch_size: u32,
    /// Enable the partition manager (creates forward monthly partitions and
    /// drops expired ones). The manager auto-detects whether the messaging
    /// tables are partitioned, so it's safe to leave on; setting this to
    /// `false` skips even the detection.
    pub partition_manager_enabled: bool,
}

impl Default for StreamProcessorConfig {
    fn default() -> Self {
        Self {
            events_enabled: true,
            events_batch_size: 100,
            dispatch_jobs_enabled: true,
            dispatch_jobs_batch_size: 100,
            partition_manager_enabled: true,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn default_config_values() {
        let config = StreamProcessorConfig::default();

        assert!(config.events_enabled);
        assert_eq!(config.events_batch_size, 100);
        assert!(config.dispatch_jobs_enabled);
        assert_eq!(config.dispatch_jobs_batch_size, 100);
        assert!(config.partition_manager_enabled);
    }

    #[test]
    fn custom_config_overrides() {
        let config = StreamProcessorConfig {
            events_enabled: false,
            events_batch_size: 500,
            dispatch_jobs_enabled: true,
            dispatch_jobs_batch_size: 250,
            partition_manager_enabled: false,
        };

        assert!(!config.events_enabled);
        assert_eq!(config.events_batch_size, 500);
        assert!(config.dispatch_jobs_enabled);
        assert_eq!(config.dispatch_jobs_batch_size, 250);
        assert!(!config.partition_manager_enabled);
    }

    #[test]
    fn both_projections_disabled() {
        let config = StreamProcessorConfig {
            events_enabled: false,
            events_batch_size: 100,
            dispatch_jobs_enabled: false,
            dispatch_jobs_batch_size: 100,
            partition_manager_enabled: false,
        };

        assert!(!config.events_enabled);
        assert!(!config.dispatch_jobs_enabled);
    }
}
