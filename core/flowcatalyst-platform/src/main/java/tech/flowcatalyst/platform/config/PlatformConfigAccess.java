package tech.flowcatalyst.platform.config;

import java.time.Instant;

/**
 * Domain model for role-based access control to application configurations.
 */
public class PlatformConfigAccess {

    public String id;
    public String applicationCode;
    public String roleCode;
    public boolean canRead;
    public boolean canWrite;
    public Instant createdAt;
}
