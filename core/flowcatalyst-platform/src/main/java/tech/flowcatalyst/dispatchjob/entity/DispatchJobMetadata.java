package tech.flowcatalyst.dispatchjob.entity;

/**
 * Metadata key-value pair embedded in DispatchJob documents.
 * Not a separate collection - embedded for efficient querying.
 */
public class DispatchJobMetadata {

    public String id;
    public String key;
    public String value;

    public DispatchJobMetadata() {
    }

    public DispatchJobMetadata(String key, String value) {
        this.key = key;
        this.value = value;
    }
}
