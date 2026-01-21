package tech.flowcatalyst.platform.client;

import java.time.Instant;

/**
 * A note attached to a client for audit trail and administrative commentary.
 * Stored as JSONB in the client's notes array.
 */
public class ClientNote {

    /**
     * The note text/message
     */
    public String text;

    /**
     * When the note was added
     */
    public Instant timestamp;

    /**
     * Who added the note (Principal ID or system identifier)
     */
    public String addedBy;

    /**
     * Category/type of note (e.g., "STATUS_CHANGE", "ADMIN_NOTE", "BILLING")
     * Applications can define their own categories
     */
    public String category;

    public ClientNote() {
    }

    public ClientNote(String category, String text, String addedBy) {
        this.category = category;
        this.text = text;
        this.addedBy = addedBy;
        this.timestamp = Instant.now();
    }
}
